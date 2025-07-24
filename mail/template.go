package mail

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Valentin-Kaiser/go-core/apperror"
	"github.com/rs/zerolog/log"
)

var (
	//go:embed template
	embeddedTemplates embed.FS
)

// templateManager implements the TemplateManager interface
type templateManager struct {
	config    TemplateConfig
	templates map[string]*template.Template
	mutex     sync.RWMutex
}

// NewTemplateManager creates a new template manager
func NewTemplateManager(config TemplateConfig) TemplateManager {
	tm := &templateManager{
		config:    config,
		templates: make(map[string]*template.Template),
	}

	// Load templates on initialization
	if err := tm.ReloadTemplates(); err != nil {
		log.Error().Err(err).Msg("[Mail] Failed to load templates during initialization")
	}

	return tm
}

// LoadTemplate loads a template by name
func (tm *templateManager) LoadTemplate(name string) (*template.Template, error) {
	tm.mutex.RLock()
	tmpl, exists := tm.templates[name]
	tm.mutex.RUnlock()

	if exists {
		return tmpl, nil
	}

	// Template not found in cache, try to load it
	return tm.loadTemplateFromDisk(name)
}

// RenderTemplate renders a template with the given data
func (tm *templateManager) RenderTemplate(name string, data interface{}) (string, error) {
	tmpl, err := tm.LoadTemplate(name)
	if err != nil {
		return "", apperror.Wrap(err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", apperror.NewError("failed to execute template").AddError(err)
	}

	return buf.String(), nil
}

// ReloadTemplates reloads all templates
func (tm *templateManager) ReloadTemplates() error {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	// Clear existing templates
	tm.templates = make(map[string]*template.Template)

	// Load templates from custom path if specified
	if tm.config.TemplatesPath != "" {
		if err := tm.loadTemplatesFromPath(tm.config.TemplatesPath); err != nil {
			log.Warn().Err(err).Str("path", tm.config.TemplatesPath).Msg("[Mail] Failed to load custom templates, falling back to embedded")
		}
	}

	// Load embedded templates
	if err := tm.loadEmbeddedTemplates(); err != nil {
		return apperror.Wrap(err)
	}

	log.Info().Int("count", len(tm.templates)).Msg("[Mail] Templates loaded successfully")
	return nil
}

// loadTemplatesFromPath loads templates from the specified file path
func (tm *templateManager) loadTemplatesFromPath(templatesPath string) error {
	if _, err := os.Stat(templatesPath); os.IsNotExist(err) {
		return apperror.NewError("templates path does not exist").AddError(err)
	}

	return filepath.Walk(templatesPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}

		// Get template name relative to templates path
		relPath, err := filepath.Rel(templatesPath, path)
		if err != nil {
			return err
		}

		// Load template content
		content, err := os.ReadFile(path)
		if err != nil {
			return apperror.NewError("failed to read template file").AddError(err)
		}

		// Parse template
		tmpl, err := tm.parseTemplate(relPath, string(content))
		if err != nil {
			return apperror.Wrap(err)
		}

		tm.templates[relPath] = tmpl
		log.Debug().Str("template", relPath).Str("path", path).Msg("[Mail] Loaded custom template")

		return nil
	})
}

// loadEmbeddedTemplates loads templates from embedded filesystem
func (tm *templateManager) loadEmbeddedTemplates() error {
	entries, err := embeddedTemplates.ReadDir("template")
	if err != nil {
		return apperror.NewError("failed to read embedded templates directory").AddError(err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".html") {
			continue
		}

		// Skip if template already loaded from custom path
		if _, exists := tm.templates[entry.Name()]; exists {
			continue
		}

		// Read embedded template content using forward slash path (embed always uses forward slash)
		content, err := embeddedTemplates.ReadFile("template/" + entry.Name())
		if err != nil {
			log.Warn().Err(err).Str("template", entry.Name()).Msg("[Mail] Failed to read embedded template")
			continue
		}

		// Parse template
		tmpl, err := tm.parseTemplate(entry.Name(), string(content))
		if err != nil {
			log.Warn().Err(err).Str("template", entry.Name()).Msg("[Mail] Failed to parse embedded template")
			continue
		}

		tm.templates[entry.Name()] = tmpl
		log.Debug().Str("template", entry.Name()).Msg("[Mail] Loaded embedded template")
	}

	return nil
}

// loadTemplateFromDisk loads a single template from disk
func (tm *templateManager) loadTemplateFromDisk(name string) (*template.Template, error) {
	var content []byte
	var err error

	// Try to load from custom path first
	if tm.config.TemplatesPath != "" {
		customPath := filepath.Join(tm.config.TemplatesPath, name)
		if _, err := os.Stat(customPath); err == nil {
			content, err = os.ReadFile(customPath)
			if err != nil {
				return nil, apperror.NewError("failed to read custom template").AddError(err)
			}
		}
	}

	// Fall back to embedded template
	if content == nil {
		content, err = embeddedTemplates.ReadFile("template/" + name)
		if err != nil {
			return nil, apperror.NewError("template not found").AddError(err)
		}
	}

	// Parse template
	tmpl, err := tm.parseTemplate(name, string(content))
	if err != nil {
		return nil, apperror.Wrap(err)
	}

	// Cache the template
	tm.mutex.Lock()
	tm.templates[name] = tmpl
	tm.mutex.Unlock()

	return tmpl, nil
}

// parseTemplate parses a template with common functions
func (tm *templateManager) parseTemplate(name, content string) (*template.Template, error) {
	// Define template functions
	funcMap := template.FuncMap{
		"add": func(a, b int) int {
			return a + b
		},
		"sub": func(a, b int) int {
			return a - b
		},
		"mul": func(a, b int) int {
			return a * b
		},
		"div": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a / b
		},
		"mod": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a % b
		},
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"title": strings.Title,
		"trim":  strings.TrimSpace,
		"replace": func(old, new, s string) string {
			return strings.ReplaceAll(s, old, new)
		},
		"contains":  strings.Contains,
		"hasPrefix": strings.HasPrefix,
		"hasSuffix": strings.HasSuffix,
		"join": func(sep string, elems []string) string {
			return strings.Join(elems, sep)
		},
		"split": func(sep, s string) []string {
			return strings.Split(s, sep)
		},
		"default": func(defaultValue, value interface{}) interface{} {
			if value == nil || value == "" {
				return defaultValue
			}
			return value
		},
		"dict": func(values ...interface{}) map[string]interface{} {
			if len(values)%2 != 0 {
				return nil
			}
			dict := make(map[string]interface{})
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					continue
				}
				dict[key] = values[i+1]
			}
			return dict
		},
		"formatDate": func(format string, date interface{}) string {
			// This would need proper date formatting implementation
			return fmt.Sprintf("%v", date)
		},
	}

	// Parse template with functions
	tmpl, err := template.New(name).Funcs(funcMap).Parse(content)
	if err != nil {
		return nil, apperror.NewError("failed to parse template").AddError(err)
	}

	return tmpl, nil
}
