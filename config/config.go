// Package config provides configuration loading and validation for the application.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// Config represents the application configuration.
type Config struct {
	Telegram  Telegram  `toml:"telegram"`
	Base      Base      `toml:"base"`
	Database  Database  `toml:"database"`
	Fetcher   Fetcher   `toml:"fetcher"`
	Holidayer Holidayer `toml:"holidayer"`
	Predictor Predictor `toml:"predictor"`
}

// Base contains base application settings.
type Base struct {
	TimeLocation *time.Location     `toml:"-"`
	AdminIDs     map[int64]struct{} `toml:"-"`
	Timezone     string             `toml:"timezone"`
	Admins       []int64            `toml:"admins"`
	Debug        bool               `toml:"debug"`
}

// Database contains database connection settings.
type Database struct {
	Path         string        `toml:"path"`
	Timeout      time.Duration `toml:"-"`
	QueryTimeout int           `toml:"query_timeout"`
	Threads      uint8         `toml:"threads"`
}

// Fetcher contains fetcher configuration.
type Fetcher struct {
	Token   string        `toml:"token"`
	URL     string        `toml:"url"`
	Timeout time.Duration `toml:"-"`
	Period  int           `toml:"period"`
	Active  bool          `toml:"active"`
}

// Holidayer contains holidayer configuration.
type Holidayer struct {
	URL     string        `toml:"url"`
	Timeout time.Duration `toml:"-"`
	Period  int           `toml:"period"`
	Active  bool          `toml:"active"`
}

// Predictor contains predictor configuration.
type Predictor struct {
	Hours        uint8         `toml:"hours"`
	Active       bool          `toml:"active"`
	LoadSize     int           `toml:"load_size"`
	Timeout      time.Duration `toml:"-"`
	QueryTimeout int           `toml:"query_timeout"`
}

// Telegram contains Telegram bot configuration.
type Telegram struct {
	Token  string `toml:"token"`
	Active bool   `toml:"active"`
}

// Load reads and parses a TOML configuration file.
func Load(path string) (*Config, error) {
	cleanPath := filepath.Clean(path)
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	cfg := new(Config)
	err = toml.Unmarshal(data, cfg)
	if err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	err = cfg.validate()
	if err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

func (c *Config) validate() error {
	err := c.Base.validate()
	if err != nil {
		return fmt.Errorf("base: %w", err)
	}
	err = c.Database.validate()
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	err = c.Fetcher.validate()
	if err != nil {
		return fmt.Errorf("fetcher: %w", err)
	}
	err = c.Holidayer.validate()
	if err != nil {
		return fmt.Errorf("holidayer: %w", err)
	}
	err = c.Predictor.validate()
	if err != nil {
		return fmt.Errorf("predictor: %w", err)
	}
	err = c.Telegram.validate()
	if err != nil {
		return fmt.Errorf("telegram: %w", err)
	}
	return nil
}

func (b *Base) validate() error {
	if b.Timezone == "" {
		b.TimeLocation = time.UTC
	} else {
		location, err := time.LoadLocation(b.Timezone)
		if err != nil {
			return fmt.Errorf("invalid timezone %q: %w", b.Timezone, err)
		}
		b.TimeLocation = location
	}

	b.AdminIDs = make(map[int64]struct{}, len(b.Admins))
	for _, adminID := range b.Admins {
		b.AdminIDs[adminID] = struct{}{}
	}
	return nil
}

func (d *Database) validate() error {
	if d.Path == "" {
		return errors.New("path is required")
	}
	if d.QueryTimeout <= 0 {
		return errors.New("query_timeout must be greater than zero")
	}
	d.Timeout = time.Duration(d.QueryTimeout) * time.Second
	if d.Threads == 0 {
		d.Threads = 1
	}
	return nil
}

// AuthToken returns the authorization token with Bearer prefix.
func (f *Fetcher) AuthToken() string {
	const prefix = "Bearer "
	if f.Token == "" {
		return ""
	}
	return prefix + f.Token
}

func (f *Fetcher) validate() error {
	if !f.Active {
		return nil
	}
	if f.Period <= 0 {
		return errors.New("period must be greater than zero")
	}
	if f.Token == "" {
		return errors.New("token is required")
	}
	err := validateHTTPURL(f.URL)
	if err != nil {
		return fmt.Errorf("url: %w", err)
	}
	f.Timeout = time.Duration(f.Period) * time.Second
	return nil
}

func (h *Holidayer) validate() error {
	if !h.Active {
		return nil
	}
	if h.Period <= 0 {
		return errors.New("period must be greater than zero")
	}
	err := validateHTTPURL(h.URL)
	if err != nil {
		return fmt.Errorf("url: %w", err)
	}
	h.Timeout = time.Duration(h.Period) * time.Second
	return nil
}

func (p *Predictor) validate() error {
	if !p.Active {
		return nil
	}
	if p.Hours < 1 || p.Hours > 24 {
		return errors.New("hours must be between 1 and 24")
	}
	if p.LoadSize < 1 {
		return errors.New("load_size must be greater than zero")
	}
	if p.QueryTimeout <= 0 {
		return errors.New("query_timeout must be greater than zero")
	}
	p.Timeout = time.Duration(p.QueryTimeout) * time.Second
	return nil
}

func (t *Telegram) validate() error {
	if !t.Active {
		return nil
	}
	if t.Token == "" {
		return errors.New("token is required")
	}
	return nil
}

func validateHTTPURL(rawURL string) error {
	if rawURL == "" {
		return errors.New("empty URL")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid scheme %q, must be http or https", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("missing host")
	}
	return nil
}
