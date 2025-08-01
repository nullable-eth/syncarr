package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	// Set up test environment variables
	testEnvVars := map[string]string{
		"SOURCE_PLEX_HOST":     "test-source.local",
		"SOURCE_PLEX_PORT":     "32400",
		"SOURCE_PLEX_TOKEN":    "test-source-token",
		"SOURCE_PLEX_PROTOCOL": "http",
		"DEST_PLEX_HOST":       "test-dest.local",
		"DEST_PLEX_PORT":       "32400",
		"DEST_PLEX_TOKEN":      "test-dest-token",
		"DEST_PLEX_PROTOCOL":   "http",
		"SYNC_LABEL":           "test-sync",
		"SYNC_INTERVAL":        "30",
		"SSH_USER":             "testuser",
		"SSH_KEY_PATH":         "/test/keys/id_rsa",
		"DEST_ROOT_DIR":        "/test/dest",
		"LOG_LEVEL":            "DEBUG",
		"DRY_RUN":              "true",
		"FORCE_FULL_SYNC":      "false",
	}

	// Set environment variables
	for key, value := range testEnvVars {
		os.Setenv(key, value)
		defer os.Unsetenv(key)
	}

	// Load configuration
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	// Test source configuration
	if cfg.Source.Host != "test-source.local" {
		t.Errorf("Expected source host 'test-source.local', got '%s'", cfg.Source.Host)
	}

	if cfg.Source.Token != "test-source-token" {
		t.Errorf("Expected source token 'test-source-token', got '%s'", cfg.Source.Token)
	}

	// Test destination configuration
	if cfg.Destination.Host != "test-dest.local" {
		t.Errorf("Expected destination host 'test-dest.local', got '%s'", cfg.Destination.Host)
	}

	// Test sync configuration
	if cfg.SyncLabel != "test-sync" {
		t.Errorf("Expected sync label 'test-sync', got '%s'", cfg.SyncLabel)
	}

	expectedInterval := 30 * time.Minute
	if cfg.Interval != expectedInterval {
		t.Errorf("Expected interval %v, got %v", expectedInterval, cfg.Interval)
	}

	// Test SSH configuration
	if cfg.SSH.User != "testuser" {
		t.Errorf("Expected SSH user 'testuser', got '%s'", cfg.SSH.User)
	}

	if cfg.SSH.KeyPath != "/test/keys/id_rsa" {
		t.Errorf("Expected SSH key path '/test/keys/id_rsa', got '%s'", cfg.SSH.KeyPath)
	}

	// Test boolean flags
	if !cfg.DryRun {
		t.Error("Expected DryRun to be true")
	}

	if cfg.ForceFullSync {
		t.Error("Expected ForceFullSync to be false")
	}

	// Test log level
	if cfg.LogLevel != "DEBUG" {
		t.Errorf("Expected log level 'DEBUG', got '%s'", cfg.LogLevel)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		wantError bool
	}{
		{
			name: "valid config",
			config: Config{
				Source: PlexServerConfig{
					Host:     "source.local",
					Port:     "32400",
					Token:    "source-token",
					Protocol: "http",
				},
				Destination: PlexServerConfig{
					Host:     "dest.local",
					Port:     "32400",
					Token:    "dest-token",
					Protocol: "http",
				},
				SyncLabel: "sync",
				Interval:  time.Hour,
				SSH: SSHConfig{
					User:    "user",
					KeyPath: "/keys/id_rsa",
				},
				LogLevel: "INFO",
				Performance: PerformanceConfig{
					WorkerPoolSize:         4,
					PlexAPIRateLimit:       10.0,
					TransferBufferSize:     65536,
					MaxConcurrentTransfers: 3,
				},
			},
			wantError: false,
		},
		{
			name: "missing source host",
			config: Config{
				Source: PlexServerConfig{
					Host:     "", // Missing
					Port:     "32400",
					Token:    "source-token",
					Protocol: "http",
				},
				Destination: PlexServerConfig{
					Host:     "dest.local",
					Port:     "32400",
					Token:    "dest-token",
					Protocol: "http",
				},
			},
			wantError: true,
		},
		{
			name: "missing destination token",
			config: Config{
				Source: PlexServerConfig{
					Host:     "source.local",
					Port:     "32400",
					Token:    "source-token",
					Protocol: "http",
				},
				Destination: PlexServerConfig{
					Host:     "dest.local",
					Port:     "32400",
					Token:    "", // Missing
					Protocol: "http",
				},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Config.Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}
