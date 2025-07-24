package mail

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Valentin-Kaiser/go-core/apperror"
	"github.com/rs/zerolog/log"
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

	// Load templates on initialization if filesystem is configured
	if config.FileSystem != nil || config.TemplatesPath != "" {
		if err := tm.ReloadTemplates(); err != nil {
			log.Error().Err(err).Msg("[Mail] Failed to load templates during initialization")
		}
	}

	return tm
}

// WithFS configures the template manager to load templates from a filesystem
func (tm *templateManager) WithFS(filesystem fs.FS) TemplateManager {
	tm.config.FileSystem = filesystem
	tm.config.TemplatesPath = ""

	// Reload templates from the new filesystem
	if err := tm.ReloadTemplates(); err != nil {
		log.Error().Err(err).Msg("[Mail] Failed to load templates from filesystem")
	}

	return tm
}

// WithFileServer configures the template manager to load templates from a file path
func (tm *templateManager) WithFileServer(templatesPath string) TemplateManager {
	if templatesPath != "" {
		if _, err := os.Stat(templatesPath); os.IsNotExist(err) {
			log.Error().Err(err).Str("path", templatesPath).Msg("[Mail] Templates path does not exist")
			return tm
		}
	}

	tm.config.TemplatesPath = templatesPath
	tm.config.FileSystem = nil

	// Reload templates from the new path
	if err := tm.ReloadTemplates(); err != nil {
		log.Error().Err(err).Msg("[Mail] Failed to load templates from file path")
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

	// Load templates from filesystem if configured
	if tm.config.FileSystem != nil {
		if err := tm.loadTemplatesFromFS(tm.config.FileSystem); err != nil {
			return apperror.Wrap(err)
		}
	} else if tm.config.TemplatesPath != "" {
		// Load templates from file path
		if err := tm.loadTemplatesFromPath(tm.config.TemplatesPath); err != nil {
			return apperror.Wrap(err)
		}
	}

	if len(tm.templates) > 0 {
		log.Info().Int("count", len(tm.templates)).Msg("[Mail] Templates loaded successfully")
	} else {
		log.Warn().Msg("[Mail] No templates loaded - templates will be unavailable")
	}

	return nil
}

// loadTemplatesFromFS loads templates from a filesystem
func (tm *templateManager) loadTemplatesFromFS(filesystem fs.FS) error {
	return fs.WalkDir(filesystem, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}

		// Read template content
		content, err := fs.ReadFile(filesystem, path)
		if err != nil {
			return apperror.NewError("failed to read template file").AddError(err)
		}

		// Parse template
		tmpl, err := tm.parseTemplate(path, string(content))
		if err != nil {
			return apperror.Wrap(err)
		}

		tm.templates[path] = tmpl
		log.Debug().Str("template", path).Msg("[Mail] Loaded template from filesystem")

		return nil
	})
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

// loadTemplateFromDisk loads a single template from the configured source
func (tm *templateManager) loadTemplateFromDisk(name string) (*template.Template, error) {
	var content []byte
	var err error

	// Try to load from filesystem first
	if tm.config.FileSystem != nil {
		content, err = fs.ReadFile(tm.config.FileSystem, name)
		if err != nil {
			return nil, apperror.NewError("template not found in filesystem").AddError(err)
		}
	} else if tm.config.TemplatesPath != "" {
		// Try to load from custom path
		customPath := filepath.Join(tm.config.TemplatesPath, name)
		if _, err := os.Stat(customPath); err == nil {
			content, err = os.ReadFile(customPath)
			if err != nil {
				return nil, apperror.NewError("failed to read template file").AddError(err)
			}
		} else {
			return nil, apperror.NewError("template not found in templates path").AddError(err)
		}
	} else {
		return nil, apperror.NewError("no template source configured - use WithFS or WithFileServer")
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
