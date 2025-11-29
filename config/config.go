// Package config provides configuration loading and validation for the application.
package config

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

// Config represents the application configuration.
type Config struct {
	Telegram Telegram `toml:"telegram"`
	Database Database `toml:"database"`
}

// Telegram contains Telegram bot configuration.
type Telegram struct {
	Token string `toml:"token"`
}

// Database contains SQLite database configuration.
type Database struct {
	Path string `toml:"path"`
}

// Load reads and parses a TOML configuration file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &cfg, nil
}

// validate checks if the configuration is valid.
func (c *Config) validate() error {
	if c.Telegram.Token == "" {
		return fmt.Errorf("telegram token is required")
	}
	if c.Database.Path == "" {
		return fmt.Errorf("database path is required")
	}
	return nil
}
