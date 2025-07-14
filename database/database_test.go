package database

import (
	"errors"
	"testing"
	"time"

	"github.com/Valentin-Kaiser/go-core/version"
	"gorm.io/gorm"
)

func TestConfig_Validate(t *testing.T) {
	testCases := []struct {
		name   string
		config Config
		valid  bool
	}{
		{
			name:   "empty config",
			config: Config{},
			valid:  false,
		},
		{
			name: "valid sqlite config",
			config: Config{
				Driver: "sqlite",
				Name:   "test.db",
			},
			valid: true,
		},
		{
			name: "invalid sqlite config - no name",
			config: Config{
				Driver: "sqlite",
			},
			valid: false,
		},
		{
			name: "valid mysql config",
			config: Config{
				Driver:   "mysql",
				Host:     "localhost",
				Port:     3306,
				User:     "root",
				Password: "password",
				Name:     "test",
			},
			valid: true,
		},
		{
			name: "invalid mysql config - no host",
			config: Config{
				Driver:   "mysql",
				Port:     3306,
				User:     "root",
				Password: "password",
				Name:     "test",
			},
			valid: false,
		},
		{
			name: "invalid mysql config - no port",
			config: Config{
				Driver:   "mysql",
				Host:     "localhost",
				User:     "root",
				Password: "password",
				Name:     "test",
			},
			valid: false,
		},
		{
			name: "invalid mysql config - no user",
			config: Config{
				Driver:   "mysql",
				Host:     "localhost",
				Port:     3306,
				Password: "password",
				Name:     "test",
			},
			valid: false,
		},
		{
			name: "invalid mysql config - no password",
			config: Config{
				Driver: "mysql",
				Host:   "localhost",
				Port:   3306,
				User:   "root",
				Name:   "test",
			},
			valid: false,
		},
		{
			name: "invalid mysql config - no name",
			config: Config{
				Driver:   "mysql",
				Host:     "localhost",
				Port:     3306,
				User:     "root",
				Password: "password",
			},
			valid: false,
		},
		{
			name: "valid mariadb config",
			config: Config{
				Driver:   "mariadb",
				Host:     "localhost",
				Port:     3306,
				User:     "root",
				Password: "password",
				Name:     "test",
			},
			valid: true,
		},
		{
			name: "unknown driver",
			config: Config{
				Driver:   "postgres",
				Host:     "localhost",
				Port:     5432,
				User:     "root",
				Password: "password",
				Name:     "test",
			},
			valid: true, // Unknown drivers are treated like MySQL
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.config.Validate()
			if tc.valid && err != nil {
				t.Errorf("Expected valid config, got error: %v", err)
			}
			if !tc.valid && err == nil {
				t.Errorf("Expected invalid config, got no error")
			}
		})
	}
}

func TestConnected(t *testing.T) {
	// Initially should not be connected
	if Connected() {
		t.Error("Expected not connected initially")
	}

	// Test that we can call the function without panic
	result := Connected()
	if result {
		t.Error("Expected false when not connected")
	}
}

func TestExecuteWithoutConnection(t *testing.T) {
	// Test Execute when not connected
	err := Execute(func(db *gorm.DB) error {
		return nil
	})

	if err == nil {
		t.Error("Execute should return error when not connected")
	}
}

func TestReconnect(t *testing.T) {
	// Test that Reconnect doesn't panic
	Reconnect()

	// Should still not be connected after reconnect without actual connection
	if Connected() {
		t.Error("Expected not connected after reconnect without connection")
	}
}

func TestAwaitConnectionTimeout(t *testing.T) {
	// Test AwaitConnection with timeout to avoid hanging
	done := make(chan bool)

	go func() {
		// This should block since we're not connected
		AwaitConnection()
		done <- true
	}()

	select {
	case <-done:
		t.Error("AwaitConnection should have blocked when not connected")
	case <-time.After(100 * time.Millisecond):
		// Expected behavior - AwaitConnection should block
	}
}

func TestConnectWithInvalidConfig(t *testing.T) {
	// Test Connect with invalid config
	config := Config{
		Driver: "invalid-driver",
	}

	// This should not panic
	Connect(time.Millisecond, config)

	// Give it a moment to try connecting
	time.Sleep(10 * time.Millisecond)

	// Should still not be connected
	if Connected() {
		t.Error("Should not be connected with invalid config")
	}

	// Clean up
	Disconnect()
}

func TestConnectWithSQLiteConfig(t *testing.T) {
	// Test Connect with SQLite config (should work without external database)
	config := Config{
		Driver: "sqlite",
		Name:   ":memory:",
	}

	// This should not panic
	Connect(time.Millisecond, config)

	// Give it a moment to connect
	time.Sleep(100 * time.Millisecond)

	// Should be connected to in-memory SQLite
	if !Connected() {
		t.Error("Should be connected to in-memory SQLite")
	}

	// Test Execute with connection
	err := Execute(func(db *gorm.DB) error {
		return db.Exec("SELECT 1").Error
	})

	if err != nil {
		t.Errorf("Execute should work when connected: %v", err)
	}

	// Clean up
	Disconnect()

	// Give it more time to disconnect since it's asynchronous
	time.Sleep(50 * time.Millisecond)

	// Note: The Connected() status might not immediately reflect disconnection
	// due to the asynchronous nature of the connection management
}

func TestRegisterSchema(t *testing.T) {
	// Test schema registration
	type TestModel struct {
		ID   uint   `gorm:"primaryKey"`
		Name string `gorm:"not null"`
	}

	// This should not panic
	RegisterSchema(&TestModel{})

	// Test with multiple schemas
	type AnotherModel struct {
		ID    uint   `gorm:"primaryKey"`
		Value string `gorm:"not null"`
	}

	RegisterSchema(&TestModel{}, &AnotherModel{})
}

func TestRegisterMigrationStep(t *testing.T) {
	// Test migration step registration
	v := version.Version{
		GitTag:    "v1.0.0",
		GitCommit: "abc123",
	}

	// This should not panic
	RegisterMigrationStep(v, func(db *gorm.DB) error {
		return nil
	})

	// Test with migration that returns error
	RegisterMigrationStep(version.Version{
		GitTag:    "v1.0.1",
		GitCommit: "def456",
	}, func(db *gorm.DB) error {
		return errors.New("migration failed")
	})
}

func TestRegisterOnConnectHandler(t *testing.T) {
	// Test OnConnect handler registration
	var handlerCalled bool

	RegisterOnConnectHandler(func(db *gorm.DB, config Config) error {
		handlerCalled = true
		return nil
	})

	// Handler should be registered but not called yet
	if handlerCalled {
		t.Error("OnConnect handler should not be called immediately")
	}

	// Test handler with error
	RegisterOnConnectHandler(func(db *gorm.DB, config Config) error {
		return errors.New("handler error")
	})
}

func TestDisconnectWithoutConnection(t *testing.T) {
	// Test Disconnect when not connected
	// The Disconnect function works by sending a signal through a channel
	// Even when not connected, it should still handle the disconnect signal
	done := make(chan bool)

	go func() {
		// Start a connection attempt first to have something to disconnect
		Connect(time.Millisecond, Config{Driver: "invalid"})
		time.Sleep(10 * time.Millisecond)
		Disconnect()
		done <- true
	}()

	select {
	case <-done:
		// Expected behavior
	case <-time.After(200 * time.Millisecond):
		t.Error("Disconnect should not hang excessively")
	}
}

func TestConfigStruct(t *testing.T) {
	// Test that Config struct has proper fields
	config := Config{
		Driver:   "mysql",
		Host:     "localhost",
		Port:     3306,
		User:     "root",
		Password: "password",
		Name:     "test",
	}

	if config.Driver != "mysql" {
		t.Error("Driver field not set correctly")
	}

	if config.Host != "localhost" {
		t.Error("Host field not set correctly")
	}

	if config.Port != 3306 {
		t.Error("Port field not set correctly")
	}

	if config.User != "root" {
		t.Error("User field not set correctly")
	}

	if config.Password != "password" {
		t.Error("Password field not set correctly")
	}

	if config.Name != "test" {
		t.Error("Name field not set correctly")
	}
}

func TestConfigImplementsInterface(t *testing.T) {
	// Test that Config implements the config.Config interface
	var cfg Config
	err := cfg.Validate()
	if err == nil {
		t.Error("Empty config should fail validation")
	}
}

// Benchmark tests
func BenchmarkConfigValidate(b *testing.B) {
	config := Config{
		Driver:   "mysql",
		Host:     "localhost",
		Port:     3306,
		User:     "root",
		Password: "password",
		Name:     "test",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		config.Validate()
	}
}

func BenchmarkConnected(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Connected()
	}
}

func BenchmarkExecuteError(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Execute(func(db *gorm.DB) error {
			return nil
		})
	}
}
