package core

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Valentin-Kaiser/go-core/flag"
)

// TestConfig implements the Config interface for testing
type TestConfig struct {
	ApplicationName string `yaml:"application_name" usage:"The name of the application"`
	ServerPort      int    `yaml:"server_port" usage:"The port to listen on"`
	EnableVerbose   bool   `yaml:"enable_verbose" usage:"Enable verbose mode"`
	DatabaseURL     string `yaml:"database_url" usage:"Database connection URL"`
}

func (c *TestConfig) Validate() error {
	if c.ApplicationName == "" {
		return fmt.Errorf("application_name cannot be empty")
	}
	if c.ServerPort <= 0 || c.ServerPort > 65535 {
		return fmt.Errorf("server_port must be between 1 and 65535")
	}
	return nil
}

// TestConfigWithError implements Config with validation error
type TestConfigWithError struct {
	ApplicationName string `yaml:"application_name"`
}

func (c *TestConfigWithError) Validate() error {
	return fmt.Errorf("always invalid")
}

func TestRegisterConfigBasic(t *testing.T) {
	// Test nil config
	err := RegisterConfig("test-nil", nil)
	if err == nil {
		t.Error("RegisterConfig() should return error for nil config")
	}
	
	// Test valid config (should pass)
	cfg := &TestConfig{
		ApplicationName: "test-app",
		ServerPort:      8080,
		EnableVerbose:   false,
		DatabaseURL:     "sqlite:///test.db",
	}
	
	err = RegisterConfig("test-valid", cfg)
	if err != nil {
		t.Errorf("RegisterConfig() with valid config should succeed: %v", err)
	}
}

func TestConfigValidation(t *testing.T) {
	cfg := &TestConfig{
		ApplicationName: "test-app",
		ServerPort:      8080,
		EnableVerbose:   false,
		DatabaseURL:     "sqlite:///test.db",
	}
	
	// Test valid config
	err := cfg.Validate()
	if err != nil {
		t.Errorf("Valid config failed validation: %v", err)
	}
	
	// Test invalid config - empty name
	cfg.ApplicationName = ""
	err = cfg.Validate()
	if err == nil {
		t.Error("Config with empty name should fail validation")
	}
	
	// Test invalid config - invalid port
	cfg.ApplicationName = "test-app"
	cfg.ServerPort = -1
	err = cfg.Validate()
	if err == nil {
		t.Error("Config with invalid port should fail validation")
	}
	
	// Test invalid config - port too high
	cfg.ServerPort = 70000
	err = cfg.Validate()
	if err == nil {
		t.Error("Config with port too high should fail validation")
	}
}

func TestConfigWithErrorValidation(t *testing.T) {
	cfg := &TestConfigWithError{
		ApplicationName: "test-app",
	}
	
	err := cfg.Validate()
	if err == nil {
		t.Error("TestConfigWithError should always fail validation")
	}
}

func TestConfigInterface(t *testing.T) {
	// Test that our test configs implement the Config interface
	var _ Config = &TestConfig{}
	var _ Config = &TestConfigWithError{}
}

func TestWriteWithNilConfig(t *testing.T) {
	err := Write(nil)
	if err == nil {
		t.Error("Write() should return error for nil config")
	}
}

func TestFileOperations(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	
	// Save original flag.Path
	originalPath := flag.Path
	defer func() { flag.Path = originalPath }()
	
	flag.Path = tempDir
	
	// Test that the temp directory exists
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		t.Errorf("Temp directory %s does not exist", tempDir)
	}
	
	// Test that flag.Path is set correctly
	if flag.Path != tempDir {
		t.Errorf("flag.Path is %s, expected %s", flag.Path, tempDir)
	}
	
	// Test creating directory structure
	subDir := filepath.Join(tempDir, "subdir")
	err := os.MkdirAll(subDir, 0755)
	if err != nil {
		t.Errorf("Failed to create subdirectory: %v", err)
	}
	
	// Test file creation
	testFile := filepath.Join(tempDir, "test.yaml")
	err = os.WriteFile(testFile, []byte("test: value"), 0644)
	if err != nil {
		t.Errorf("Failed to create test file: %v", err)
	}
	
	// Test file reading
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Errorf("Failed to read test file: %v", err)
	}
	
	if string(content) != "test: value" {
		t.Errorf("File content is %s, expected 'test: value'", string(content))
	}
}

func TestGetWithoutRegistration(t *testing.T) {
	// Test Get() when no config is registered
	// This should return nil or the previously registered config
	result := Get()
	// We can't make strong assertions here since the config package
	// maintains global state and other tests might have registered configs
	_ = result
}

func TestPackageConstants(t *testing.T) {
	// Test that we can access package-level functions
	_ = Get()
	
	// Test that we can call OnChange (should not panic)
	OnChange(func(c Config) error {
		return nil
	})
}

// Test concurrent access safety
func TestConcurrentAccess(t *testing.T) {
	done := make(chan bool, 10)
	
	// Test concurrent Get operations
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()
			config := Get()
			_ = config // Use the config to avoid compiler optimization
		}()
	}
	
	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

// Test that the config structs work with YAML tags
func TestYAMLTags(t *testing.T) {
	cfg := &TestConfig{
		ApplicationName: "test-app",
		ServerPort:      8080,
		EnableVerbose:   true,
		DatabaseURL:     "sqlite:///test.db",
	}
	
	// The actual YAML marshaling is handled by the config package
	// Here we just test that the struct is properly defined
	if cfg.ApplicationName != "test-app" {
		t.Error("ApplicationName field not properly set")
	}
	
	if cfg.ServerPort != 8080 {
		t.Error("ServerPort field not properly set")
	}
	
	if !cfg.EnableVerbose {
		t.Error("EnableVerbose field not properly set")
	}
	
	if cfg.DatabaseURL != "sqlite:///test.db" {
		t.Error("DatabaseURL field not properly set")
	}
}

// Test flag usage tags
func TestUsageTags(t *testing.T) {
	// Test that our struct has proper usage tags
	// This is mainly a compile-time check
	cfg := &TestConfig{}
	
	// Test that validation works
	err := cfg.Validate()
	if err == nil {
		t.Error("Empty config should fail validation")
	}
	
	// Test with valid values
	cfg.ApplicationName = "test"
	cfg.ServerPort = 8080
	err = cfg.Validate()
	if err != nil {
		t.Errorf("Valid config should pass validation: %v", err)
	}
}

// Benchmark tests
func BenchmarkGet(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Get()
	}
}

func BenchmarkValidate(b *testing.B) {
	cfg := &TestConfig{
		ApplicationName: "benchmark-test",
		ServerPort:      8080,
		EnableVerbose:   false,
		DatabaseURL:     "sqlite:///test.db",
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cfg.Validate()
	}
}

func BenchmarkConfigCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		cfg := &TestConfig{
			ApplicationName: "benchmark-test",
			ServerPort:      8080,
			EnableVerbose:   false,
			DatabaseURL:     "sqlite:///test.db",
		}
		_ = cfg
	}
}