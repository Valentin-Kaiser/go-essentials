// Package config provides a simple, structured, and extensible way to manage
// application configuration in Go.
//
// It builds upon the Viper library and adds
// powerful features like validation, dynamic watching, default value registration,
// environment and flag integration, and structured config registration.
//
// Key Features:
//
//   - Register typed configuration structs with default values.
//   - Parse YAML configuration files and bind fields to CLI flags and environment variables.
//   - Automatically generate flags based on struct field tags.
//   - Validate configuration using custom logic (via `Validate()` method).
//   - Watch configuration files for changes and hot-reload updated values.
//   - Write current configuration back to disk.
//   - Automatically fallbacks to default config creation if no file is found.
//
// All configuration structs must implement the `Config` interface:
//
//	type Config interface {
//	    Validate() error
//	}
//
// Example:
//
//	package config
//
//	import (
//	    "fmt"
//	    "github.com/Valentin-Kaiser/go-core/config"
//	    "github.com/fsnotify/fsnotify"
//	)
//
//	type ServerConfig struct {
//	    Host string `yaml:"host" usage:"The host of the server"`
//	    Port int    `yaml:"port" usage:"The port of the server"`
//	}
//
//	func (c *ServerConfig) Validate() error {
//	    if c.Host == "" {
//	        return fmt.Errorf("host cannot be empty")
//	    }
//	    if c.Port <= 0 {
//	        return fmt.Errorf("port must be greater than 0")
//	    }
//	    return nil
//	}
//
//	func Get() *ServerConfig {
//	    c, ok := config.Get().(*ServerConfig)
//	    if !ok {
//	        return &ServerConfig{}
//	    }
//	    return c
//	}
//
//	func init() {
//	    cfg := &ServerConfig{
//	        Host: "localhost",
//	        Port: 8080,
//	    }
//
//	    if err := config.Register("server", cfg); err != nil {
//	        fmt.Println("Error registering config:", err)
//	        return
//	    }
//
//	    if err := config.Read(); err != nil {
//	        fmt.Println("Error reading config:", err)
//	        return
//	    }
//
//	    config.Watch(func(e fsnotify.Event) {
//	        if err := config.Read(); err != nil {
//	            fmt.Println("Error reloading config:", err)
//	        }
//	    })
//
//	    if err := config.Write(cfg); err != nil {
//	        fmt.Println("Error writing config:", err)
//	    }
//	}
package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Valentin-Kaiser/go-core/apperror"
	"github.com/Valentin-Kaiser/go-core/flag"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/pflag"

	"github.com/spf13/viper"
	"github.com/stoewer/go-strcase"
	"gopkg.in/yaml.v2"
)

var (
	mutex      = &sync.RWMutex{}
	config     Config
	configname string
	onChange   []func(Config) error
	lastChange atomic.Int64
)

// Config is the interface that all configuration structs must implement
// It should contain a Validate method that checks the configuration for errors
type Config interface {
	Validate() error
}

// Register registers a configuration struct and parses its tags
// The name is used as the name of the configuration file and the prefix for the environment variables
func Register(name string, c Config) error {
	if c == nil {
		return apperror.NewError("the configuration provided is nil")
	}

	if reflect.TypeOf(c).Kind() != reflect.Ptr || reflect.TypeOf(c).Elem().Kind() != reflect.Struct {
		return apperror.NewErrorf("the configuration provided is not a pointer to a struct, got %T", c)
	}

	configname = name
	viper.SetEnvPrefix(strings.ReplaceAll(configname, "-", "_"))
	viper.SetTypeByDefaultValue(true)
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	err := parseStructTags(reflect.ValueOf(c), "")
	if err != nil {
		return apperror.Wrap(err)
	}

	apply(c)
	return nil
}

// OnChange registers a function that is called when the configuration changes
func OnChange(f func(Config) error) {
	onChange = append(onChange, f)
}

// Get returns the current configuration
func Get() Config {
	mutex.RLock()
	defer mutex.RUnlock()
	return config
}

// Read reads the configuration from the file, validates it and applies it
// If the file does not exist, it creates a new one with the default values
func Read() error {
	viper.SetConfigName(configname)
	viper.SetConfigType("yaml")
	viper.AddConfigPath(flag.Path)

	if err := viper.ReadInConfig(); err != nil {
		if err := os.MkdirAll(flag.Path, 0750); err != nil {
			return apperror.NewError("creating configuration directory failed").AddError(err)
		}
		// Write the default config using our own save function instead of viper's SafeWriteConfig
		if err := save(); err != nil {
			return apperror.NewError("writing default configuration file failed").AddError(err)
		}
	}

	change, ok := reflect.New(reflect.TypeOf(config).Elem()).Interface().(Config)
	if !ok {
		return apperror.NewErrorf("creating new instance of %T failed", config)
	}

	err := viper.Unmarshal(&change)
	if err != nil {
		return apperror.NewErrorf("unmarshalling configuration data in %T failed", config).AddError(err)
	}

	err = change.Validate()
	if err != nil {
		return apperror.Wrap(err)
	}

	apply(change)
	for _, f := range onChange {
		err = f(config)
		if err != nil {
			return apperror.Wrap(err)
		}
	}

	return nil
}

// Write writes the configuration to the file, validates it and applies it
// If the file does not exist, it creates a new one with the default values
func Write(change Config) error {
	if change == nil {
		return apperror.NewError("the configuration provided is nil")
	}

	err := change.Validate()
	if err != nil {
		return apperror.Wrap(err)
	}

	apply(change)
	err = save()
	if err != nil {
		return apperror.Wrap(err)
	}

	for _, f := range onChange {
		err = f(config)
		if err != nil {
			return apperror.Wrap(err)
		}
	}

	return nil
}

// Watch watches the configuration file for changes and calls the provided function when it changes
// It ignores changes that happen within 1 second of each other
// This is to prevent multiple calls when the file is saved
func Watch(onChange func(fsnotify.Event)) {
	viper.WatchConfig()
	viper.OnConfigChange(func(e fsnotify.Event) {
		if time.Now().UnixMilli()-lastChange.Load() < 1000 {
			return
		}
		lastChange.Store(time.Now().UnixMilli())
		onChange(e)
	})
}

// apply applies the configuration to the global variable
func apply(appConfig Config) {
	mutex.Lock()
	defer mutex.Unlock()
	config = appConfig
}

// save saves the configuration to the file
// If the file does not exist, it creates a new one with the default values
func save() error {
	// Ensure the directory exists before trying to create the file
	if err := os.MkdirAll(flag.Path, 0750); err != nil {
		return apperror.NewError("creating configuration directory failed").AddError(err)
	}

	path, err := filepath.Abs(filepath.Join(flag.Path, configname+".yaml"))
	if err != nil {
		return apperror.NewError("building absolute path of configuration file failed").AddError(err)
	}
	file, err := os.OpenFile(filepath.Clean(path), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return apperror.NewError("opening configuration file failed").AddError(err)
	}

	mutex.RLock()
	defer mutex.RUnlock()
	data, err := yaml.Marshal(config)
	if err != nil {
		return apperror.NewError("marshalling configuration data failed").AddError(err)
	}

	_, err = file.Write(data)
	if err != nil {
		return apperror.NewError("writing configuration data to file failed").AddError(err)
	}

	err = file.Close()
	if err != nil {
		return apperror.NewError("closing configuration file failed").AddError(err)
	}

	return nil
}

// declareFlag declares a flag with the given label, usage and default value
// It also binds the flag to viper so that it can be used in the configuration
func declareFlag(label string, usage string, defaultValue interface{}) error {
	viper.SetDefault(label, defaultValue)
	pflagLabel := strcase.KebabCase(label)
	label = strings.ToLower(label)

	// Check if flag already exists to avoid redefinition errors
	if pflag.Lookup(pflagLabel) != nil {
		// Flag already exists, just bind to viper
		if err := viper.BindPFlag(label, pflag.Lookup(pflagLabel)); err != nil {
			return apperror.NewErrorf("binding existing flag %s to viper failed", label).AddError(err)
		}
		return nil
	}

	switch v := defaultValue.(type) {
	case string:
		pflag.String(pflagLabel, v, usage)
	case int:
		pflag.Int(pflagLabel, v, usage)
	case uint:
		pflag.Uint(pflagLabel, v, usage)
	case int8:
		pflag.Int8(pflagLabel, v, usage)
	case uint8:
		pflag.Uint8(pflagLabel, v, usage)
	case int16:
		pflag.Int16(pflagLabel, v, usage)
	case uint16:
		pflag.Uint16(pflagLabel, v, usage)
	case int32:
		pflag.Int32(pflagLabel, v, usage)
	case uint32:
		pflag.Uint32(pflagLabel, v, usage)
	case int64:
		pflag.Int64(pflagLabel, v, usage)
	case uint64:
		pflag.Uint64(pflagLabel, v, usage)
	case float32:
		pflag.Float32(pflagLabel, v, usage)
	case float64:
		pflag.Float64(pflagLabel, v, usage)
	case bool:
		pflag.Bool(pflagLabel, v, usage)
	case []string:
		pflag.StringArray(pflagLabel, v, usage)
	default:
		return nil
	}

	if err := viper.BindPFlag(label, pflag.Lookup(pflagLabel)); err != nil {
		return apperror.NewErrorf("binding flag %s to viper failed", label).AddError(err)
	}

	return nil
}

// parseStructTags parses the struct tags of the given struct and registers the flags
// It also sets the default values of the flags to the values of the struct fields
func parseStructTags(v reflect.Value, labelBase string) error {
	// If the config is a pointer, we need to get the type of the element
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		// If the field is not exported, we skip it
		if t.Field(i).PkgPath != "" {
			continue
		}

		// If the field is a pointer, we need to dereference it
		if v.Field(i).Kind() == reflect.Ptr {
			if v.Field(i).IsNil() {
				v.Field(i).Set(reflect.New(t.Field(i).Type.Elem()))
			}
			err := parseStructTags(v.Field(i).Elem(), t.Field(i).Name)
			if err != nil {
				return apperror.Wrap(err)
			}
			continue
		}

		// If the field is a struct, we need to iterate over its fields
		if t.Field(i).Type.Kind() == reflect.Struct {
			subv := v.Field(i)
			if subv.Kind() == reflect.Ptr {
				subv = subv.Elem()
			}

			label := ""
			if labelBase != "" {
				label += labelBase + "."
			}
			label += t.Field(i).Name
			err := parseStructTags(subv, label)
			if err != nil {
				return apperror.Wrap(err)
			}
			continue
		}

		field := t.Field(i)
		tag := field.Name
		if labelBase != "" {
			tag = labelBase + "." + tag
		}

		err := declareFlag(tag, field.Tag.Get("usage"), v.Field(i).Interface())
		if err != nil {
			return apperror.Wrap(err)
		}
	}

	return nil
}

// Reset clears all global state - primarily for testing purposes
func Reset() {
	mutex.Lock()
	defer mutex.Unlock()
	config = nil
	configname = ""
	onChange = nil
	lastChange.Store(0)

	// Reset viper state
	viper.Reset()
}
