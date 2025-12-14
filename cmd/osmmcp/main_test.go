package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGenerateClientConfig(t *testing.T) {
	// Create temporary directory for test files
	tmpDir, err := os.MkdirTemp("", "osmmcp-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Change to temp directory for relative path tests
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer os.Chdir(oldDir)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	tests := []struct {
		name      string
		path      string
		mergeOnly bool
		wantErr   bool
	}{
		{
			name:      "valid path",
			path:      "config.json",
			mergeOnly: false,
			wantErr:   false,
		},
		{
			name:      "empty path",
			path:      "",
			mergeOnly: false,
			wantErr:   true,
		},
		{
			name:      "non-json extension",
			path:      "config.txt",
			mergeOnly: false,
			wantErr:   true,
		},
		{
			name:      "path with ..",
			path:      filepath.Join("..", "config.json"),
			mergeOnly: false,
			wantErr:   true,
		},
		{
			name:      "merge with existing",
			path:      "merge.json",
			mergeOnly: true,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create existing config for merge test
			if tt.name == "merge with existing" {
				existing := map[string]interface{}{
					"existing_key": "existing_value",
				}
				data, err := json.Marshal(existing)
				if err != nil {
					t.Fatalf("Failed to marshal existing config: %v", err)
				}
				if err := os.WriteFile(tt.path, data, 0600); err != nil {
					t.Fatalf("Failed to write existing config: %v", err)
				}
			}

			err := generateClientConfig(tt.path, tt.mergeOnly)
			if (err != nil) != tt.wantErr {
				t.Errorf("generateClientConfig() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Check file exists and has correct permissions if no error
			if !tt.wantErr {
				info, err := os.Stat(tt.path)
				if err != nil {
					t.Errorf("Failed to stat config file: %v", err)
				}
				// Skip permission check on Windows - Unix-style permissions don't apply
				if runtime.GOOS != "windows" {
					if mode := info.Mode(); mode != 0600 {
						t.Errorf("Config file has wrong permissions: %v, want 0600", mode)
					}
				}

				// Check config content
				data, err := os.ReadFile(tt.path)
				if err != nil {
					t.Errorf("Failed to read config file: %v", err)
				}

				var config map[string]interface{}
				if err := json.Unmarshal(data, &config); err != nil {
					t.Errorf("Failed to parse config JSON: %v", err)
				}

				// Check required fields
				if _, ok := config["claude"]; !ok {
					t.Error("Config missing 'claude' section")
				}
				if _, ok := config["server"]; !ok {
					t.Error("Config missing 'server' section")
				}

				// Check merged content for merge test
				if tt.name == "merge with existing" {
					if val, ok := config["existing_key"]; !ok || val != "existing_value" {
						t.Error("Merge failed to preserve existing content")
					}
				}
			}
		})
	}
}
