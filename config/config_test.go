package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name: "valid config",
			content: `[telegram]
token = "test_token"

[database]
path = "/tmp/test.db"
`,
			wantErr: false,
		},
		{
			name: "missing token",
			content: `[telegram]
token = ""

[database]
path = "/tmp/test.db"
`,
			wantErr: true,
		},
		{
			name: "missing database path",
			content: `[telegram]
token = "test_token"

[database]
path = ""
`,
			wantErr: true,
		},
		{
			name:    "invalid toml",
			content: `invalid toml content [[[`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(tmpDir, tt.name+".toml")
			if err := os.WriteFile(configPath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to write test config: %v", err)
			}

			cfg, err := Load(configPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && cfg == nil {
				t.Error("Load() returned nil config without error")
			}
		})
	}
}

func TestLoadNonExistentFile(t *testing.T) {
	_, err := Load("/non/existent/path.toml")
	if err == nil {
		t.Error("Load() expected error for non-existent file")
	}
}
