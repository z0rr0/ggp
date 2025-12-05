package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		errContain string
		wantErr    bool
	}{
		{
			name: "valid minimal config",
			content: `
[database]
path = "test.db"
query_timeout = 5
`,
		},
		{
			name: "valid full config",
			content: `
[base]
timezone = "UTC"
admins = [123, 456]
debug = true

[database]
path = "test.db"
query_timeout = 10

[fetcher]
active = true
period = 300
token = "secret"
url = "https://api.example.com/data"

[holidayer]
active = true
period = 86400
url = "https://calendar.example.com/holidays"

[predictor]
active = true
hours = 4

[telegram]
active = true
token = "bot_token"
`,
		},
		{
			name: "inactive components skip validation",
			content: `
[database]
path = "test.db"
query_timeout = 5

[fetcher]
active = false

[holidayer]
active = false

[predictor]
active = false

[telegram]
active = false
`,
		},
		{
			name:       "invalid toml syntax",
			content:    `invalid [[[`,
			wantErr:    true,
			errContain: "parse config file",
		},
		{
			name: "missing database path",
			content: `
[database]
query_timeout = 5
`,
			wantErr:    true,
			errContain: "database",
		},
		{
			name: "invalid timezone",
			content: `
[base]
timezone = "Invalid/Zone"

[database]
path = "test.db"
query_timeout = 5
`,
			wantErr:    true,
			errContain: "timezone",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTempConfig(t, tc.content)

			cfg, err := Load(path)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.errContain != "" && !strings.Contains(err.Error(), tc.errContain) {
					t.Errorf("error %q should contain %q", err.Error(), tc.errContain)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg == nil {
				t.Fatal("expected config, got nil")
			}
		})
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.toml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "read config file") {
		t.Errorf("error should mention file reading: %v", err)
	}
}

func TestBase_Validate(t *testing.T) {
	tests := []struct {
		name    string
		wantTZ  string
		base    Base
		wantErr bool
	}{
		{
			name:   "empty timezone defaults to UTC",
			base:   Base{},
			wantTZ: "UTC",
		},
		{
			name:   "valid timezone",
			base:   Base{Timezone: "America/New_York"},
			wantTZ: "America/New_York",
		},
		{
			name:    "invalid timezone",
			base:    Base{Timezone: "Invalid/Zone"},
			wantErr: true,
		},
		{
			name: "admins populated to map",
			base: Base{Admins: []int64{1, 2, 3}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.base.validate()

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.wantTZ != "" && tc.base.TimeLocation.String() != tc.wantTZ {
				t.Errorf("timezone = %q, want %q", tc.base.TimeLocation.String(), tc.wantTZ)
			}

			if tc.base.Admins != nil {
				for _, id := range tc.base.Admins {
					if _, ok := tc.base.AdminIDs[id]; !ok {
						t.Errorf("admin %d not in AdminIDs map", id)
					}
				}
			}
		})
	}
}

func TestDatabase_Validate(t *testing.T) {
	tests := []struct {
		name        string
		db          Database
		wantErr     bool
		wantTimeout time.Duration
	}{
		{
			name:    "empty path",
			db:      Database{QueryTimeout: 5},
			wantErr: true,
		},
		{
			name:    "zero timeout",
			db:      Database{Path: "test.db", QueryTimeout: 0},
			wantErr: true,
		},
		{
			name:    "negative timeout",
			db:      Database{Path: "test.db", QueryTimeout: -1},
			wantErr: true,
		},
		{
			name:        "valid config",
			db:          Database{Path: "test.db", QueryTimeout: 10},
			wantTimeout: 10 * time.Second,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.db.validate()

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.db.Timeout != tc.wantTimeout {
				t.Errorf("timeout = %v, want %v", tc.db.Timeout, tc.wantTimeout)
			}
		})
	}
}

func TestFetcher_Validate(t *testing.T) {
	tests := []struct {
		name    string
		fetcher Fetcher
		wantErr bool
	}{
		{
			name:    "inactive skips validation",
			fetcher: Fetcher{Active: false},
		},
		{
			name:    "active with zero period",
			fetcher: Fetcher{Active: true, Period: 0},
			wantErr: true,
		},
		{
			name:    "active without token",
			fetcher: Fetcher{Active: true, Period: 60, Token: ""},
			wantErr: true,
		},
		{
			name:    "active with invalid url scheme",
			fetcher: Fetcher{Active: true, Period: 60, Token: "tok", URL: "ftp://example.com"},
			wantErr: true,
		},
		{
			name:    "active with empty url",
			fetcher: Fetcher{Active: true, Period: 60, Token: "tok", URL: ""},
			wantErr: true,
		},
		{
			name:    "valid https",
			fetcher: Fetcher{Active: true, Period: 60, Token: "tok", URL: "https://api.example.com/data"},
		},
		{
			name:    "valid http",
			fetcher: Fetcher{Active: true, Period: 60, Token: "tok", URL: "http://localhost:8080/data"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fetcher.validate()

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.fetcher.Active && tc.fetcher.Timeout != time.Duration(tc.fetcher.Period)*time.Second {
				t.Error("timeout not set correctly")
			}
		})
	}
}

func TestFetcher_AuthToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  string
	}{
		{"empty token", "", ""},
		{"simple token", "abc123", "Bearer abc123"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := Fetcher{Token: tc.token}
			if got := f.AuthToken(); got != tc.want {
				t.Errorf("AuthToken() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHolidayer_Validate(t *testing.T) {
	tests := []struct {
		name      string
		holidayer Holidayer
		wantErr   bool
	}{
		{
			name:      "inactive skips validation",
			holidayer: Holidayer{Active: false},
		},
		{
			name:      "active with zero period",
			holidayer: Holidayer{Active: true, Period: 0},
			wantErr:   true,
		},
		{
			name:      "active with invalid url",
			holidayer: Holidayer{Active: true, Period: 86400, URL: "not-a-url"},
			wantErr:   true,
		},
		{
			name:      "valid config",
			holidayer: Holidayer{Active: true, Period: 86400, URL: "https://calendar.example.com"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.holidayer.validate()

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestPredictor_Validate(t *testing.T) {
	tests := []struct {
		name      string
		predictor Predictor
		wantErr   bool
	}{
		{
			name:      "inactive skips validation",
			predictor: Predictor{Active: false},
		},
		{
			name:      "hours zero",
			predictor: Predictor{Active: true, Hours: 0},
			wantErr:   true,
		},
		{
			name:      "hours above 24",
			predictor: Predictor{Active: true, Hours: 25},
			wantErr:   true,
		},
		{
			name:      "hours min boundary",
			predictor: Predictor{Active: true, Hours: 1},
		},
		{
			name:      "hours max boundary",
			predictor: Predictor{Active: true, Hours: 24},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.predictor.validate()

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestTelegram_Validate(t *testing.T) {
	tests := []struct {
		name     string
		telegram Telegram
		wantErr  bool
	}{
		{
			name:     "inactive skips validation",
			telegram: Telegram{Active: false},
		},
		{
			name:     "active without token",
			telegram: Telegram{Active: true, Token: ""},
			wantErr:  true,
		},
		{
			name:     "valid config",
			telegram: Telegram{Active: true, Token: "123456:ABC"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.telegram.validate()

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateHTTPURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"empty url", "", true},
		{"invalid scheme ftp", "ftp://example.com", true},
		{"invalid scheme file", "file:///path", true},
		{"missing host", "https://", true},
		{"valid https", "https://example.com", false},
		{"valid http", "http://localhost:8080", false},
		{"valid with path", "https://api.example.com/v1/data", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateHTTPURL(tc.url)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateHTTPURL(%q) error = %v, wantErr %v", tc.url, err, tc.wantErr)
			}
		})
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}
