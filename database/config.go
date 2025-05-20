package database

import "fmt"

// Config holds the configuration for the database connection
// This struct can be used with the config core package.
// You can use this struct or embed it in your own struct.
type Config struct {
	Driver   string `usage:"Database driver. Currently available options are 'mysql', 'mariadb' or 'sqlite'"`
	Host     string `usage:"IP address or hostname of the database server"`
	Port     uint16 `usage:"Port of the database server to connect to"`
	User     string `usage:"Database username"`
	Password string `usage:"Database password"`
	Name     string `usage:"Name of the database or sqlite file"`
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Driver == "" {
		return fmt.Errorf("database driver is required")
	}
	if c.Host == "" {
		return fmt.Errorf("database host is required")
	}
	if c.Port == 0 {
		return fmt.Errorf("database port is required")
	}
	if c.User == "" {
		return fmt.Errorf("database user is required")
	}
	if c.Password == "" {
		return fmt.Errorf("database password is required")
	}
	if c.Name == "" {
		return fmt.Errorf("database name is required")
	}
	return nil
}
