// Package config provides configuration loading and validation for the application.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

const tokenPrefix = "Bearer "

// Config represents the application configuration.
type Config struct {
	Base      Base      `toml:"base"`
	Database  Database  `toml:"database"`
	Fetcher   Fetcher   `toml:"fetcher"`
	Holidayer Holidayer `toml:"holidayer"`
	Predictor Predictor `toml:"predictor"`
	Telegram  Telegram  `toml:"telegram"`
}

type Base struct {
	Timezone     string             `toml:"timezone"`
	Admins       []int64            `toml:"admins"`
	Debug        bool               `toml:"debug"`
	TimeLocation *time.Location     `toml:"-"`
	AdminIDs     map[int64]struct{} `toml:"-"`
}

type Database struct {
	Path         string        `toml:"path"`
	QueryTimeout int           `toml:"query_timeout"`
	Timeout      time.Duration `toml:"-"`
}

// Fetcher contains fetcher configuration.
type Fetcher struct {
	Active  bool          `toml:"active"`
	Period  int           `toml:"period"`
	Token   string        `toml:"token"`
	URL     string        `toml:"url"`
	Timeout time.Duration `toml:"-"`
}

// AuthToken returns the authorization token with the required prefix.
func (f *Fetcher) AuthToken() string {
	if f.Token != "" && !strings.HasPrefix(f.Token, tokenPrefix) {
		return tokenPrefix + f.Token
	}

	return f.Token
}

// Holidayer contains holidayer configuration.
type Holidayer struct {
	Active  bool          `toml:"active"`
	Period  int           `toml:"period"`
	URL     string        `toml:"url"`
	Timeout time.Duration `toml:"-"`
}

// Predictor contains predictor configuration.
type Predictor struct {
	Active bool  `toml:"active"`
	Hours  uint8 `toml:"hours"`
}

// Telegram contains Telegram bot configuration.
type Telegram struct {
	Active bool   `toml:"active"`
	Token  string `toml:"token"`
}

// Load reads and parses a TOML configuration file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	cfg := new(Config)
	if err = toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	if err = cfg.validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

// validate checks if the configuration is valid.
func (c *Config) validate() error {
	if c.Base.Timezone == "" {
		c.Base.TimeLocation = time.UTC
	} else {
		location, err := time.LoadLocation(c.Base.Timezone)
		if err != nil {
			return fmt.Errorf("load timezone %q: %w", c.Base.Timezone, err)
		}
		c.Base.TimeLocation = location
	}

	if c.Fetcher.Period <= 0 {
		return fmt.Errorf("fetcher period must be greater than zero")
	}
	c.Fetcher.Timeout = time.Duration(c.Fetcher.Period) * time.Second

	if c.Fetcher.Token == "" {
		return fmt.Errorf("fetcher token is required")
	}
	if c.Fetcher.URL == "" {
		return fmt.Errorf("fetcher URL is required")
	}

	if c.Holidayer.Period <= 0 {
		return fmt.Errorf("holidayer period must be greater than zero")
	}
	c.Holidayer.Timeout = time.Duration(c.Holidayer.Period) * time.Second

	if c.Holidayer.URL == "" {
		return fmt.Errorf("holidayer URL is required")
	}

	if c.Predictor.Hours < 1 || c.Predictor.Hours > 24 {
		return fmt.Errorf("predictor hours must be between 1 and 24")
	}

	if c.Telegram.Token == "" {
		return fmt.Errorf("telegram token is required")
	}

	if c.Database.Path == "" {
		return fmt.Errorf("database path is required")
	}
	if c.Database.QueryTimeout <= 0 {
		return fmt.Errorf("database query timeout is required")
	}
	c.Database.Timeout = time.Duration(c.Database.QueryTimeout) * time.Second

	adminsMap := make(map[int64]struct{})
	for _, adminID := range c.Base.Admins {
		adminsMap[adminID] = struct{}{}
	}
	c.Base.AdminIDs = adminsMap

	return nil
}
