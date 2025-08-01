package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config represents the main application configuration
type Config struct {
	Source            PlexServerConfig  `json:"source"`
	Destination       PlexServerConfig  `json:"destination"`
	SyncLabel         string            `json:"syncLabel"`
	SourceReplaceFrom string            `json:"sourceReplaceFrom"` // Optional: Source path prefix to strip (e.g., "/data/Movies")
	SourceReplaceTo   string            `json:"sourceReplaceTo"`   // Optional: Local path replacement (e.g., "/media/source"). Leave empty for same-volume mounting
	DestRootDir       string            `json:"destRootDir"`       // Required: Destination root path (e.g., "/mnt/data/Movies")
	TransferMethod    string            `json:"transferMethod"`    // Optional: Force transfer method ("rsync" or "scp"), auto-detected if empty
	Interval          time.Duration     `json:"interval"`
	SSH               SSHConfig         `json:"ssh"`
	Performance       PerformanceConfig `json:"performance"`
	Transfer          TransferConfig    `json:"transfer"`
	DryRun            bool              `json:"dryRun"`
	LogLevel          string            `json:"logLevel"`
}

// PlexServerConfig represents Plex server configuration
// Updated to include RequireHTTPS
// Protocol is derived from RequireHTTPS
// Removed FilterConfig and BandwidthConfig
type PlexServerConfig struct {
	Host         string `json:"host"`
	Port         string `json:"port"`
	Token        string `json:"token"`
	Protocol     string `json:"protocol"` // http/https
	RequireHTTPS bool   `json:"requireHttps"`
}

// SSHConfig represents SSH connection configuration
type SSHConfig struct {
	User               string `json:"user"`
	Password           string `json:"password"`
	Port               string `json:"port"`
	KeyPath            string `json:"keyPath,omitempty"`        // Optional, for future key-based auth
	StrictHostKeyCheck bool   `json:"strictHostKeyCheck"`       // Whether to enforce host key verification
	KnownHostsFile     string `json:"knownHostsFile,omitempty"` // Path to known_hosts file
}

// PerformanceConfig represents performance-related configuration
type PerformanceConfig struct {
	WorkerPoolSize         int     `json:"workerPoolSize"`
	PlexAPIRateLimit       float64 `json:"plexApiRateLimit"`
	TransferBufferSize     int     `json:"transferBufferSize"`
	MaxConcurrentTransfers int     `json:"maxConcurrentTransfers"`
}

// TransferConfig represents transfer-related configuration
type TransferConfig struct {
	EnableCompression bool `json:"enableCompression"`
	ResumeTransfers   bool `json:"resumeTransfers"`
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	config := &Config{
		Source: PlexServerConfig{
			Host:         getEnvWithDefault("SOURCE_PLEX_HOST", ""),
			Port:         getEnvWithDefault("SOURCE_PLEX_PORT", "32400"),
			Token:        getEnvWithDefault("SOURCE_PLEX_TOKEN", ""),
			RequireHTTPS: parseBoolEnv("SOURCE_PLEX_REQUIRES_HTTPS", true),
			Protocol:     "https",
		},
		Destination: PlexServerConfig{
			Host:         getEnvWithDefault("DEST_PLEX_HOST", ""),
			Port:         getEnvWithDefault("DEST_PLEX_PORT", "32400"),
			Token:        getEnvWithDefault("DEST_PLEX_TOKEN", ""),
			RequireHTTPS: parseBoolEnv("DEST_PLEX_REQUIRES_HTTPS", true),
			Protocol:     "https",
		},
		SyncLabel:         getEnvWithDefault("SYNC_LABEL", ""),
		SourceReplaceFrom: getEnvWithDefault("SOURCE_REPLACE_FROM", ""),
		SourceReplaceTo:   getEnvWithDefault("SOURCE_REPLACE_TO", ""),
		DestRootDir:       getEnvWithDefault("DEST_ROOT_DIR", ""),
		TransferMethod:    strings.ToLower(getEnvWithDefault("TRANSFER_METHOD", "")), // rsync, scp, or empty for auto-detection
		SSH: SSHConfig{
			User:     getEnvWithDefault("SSH_USER", ""),
			Password: getEnvWithDefault("SSH_PASSWORD", ""),
			Port:     getEnvWithDefault("SSH_PORT", "22"),
			KeyPath:  getEnvWithDefault("SSH_KEY_PATH", ""), // Keep for future use
		},
		DryRun:   parseBoolEnv("DRY_RUN", false),
		LogLevel: getEnvWithDefault("LOG_LEVEL", "INFO"),
	}

	// Set protocol based on RequireHTTPS
	if !config.Source.RequireHTTPS {
		config.Source.Protocol = "http"
	}
	if !config.Destination.RequireHTTPS {
		config.Destination.Protocol = "http"
	}

	// Parse interval
	intervalStr := getEnvWithDefault("SYNC_INTERVAL", "60")
	intervalMinutes, err := strconv.Atoi(intervalStr)
	if err != nil {
		return nil, fmt.Errorf("invalid SYNC_INTERVAL: %w", err)
	}
	config.Interval = time.Duration(intervalMinutes) * time.Minute

	// Parse performance configuration
	config.Performance = PerformanceConfig{
		WorkerPoolSize:         int(parseIntEnv("WORKER_POOL_SIZE", 4)),
		PlexAPIRateLimit:       parseFloatEnv("PLEX_API_RATE_LIMIT", 10.0),
		TransferBufferSize:     int(parseIntEnv("TRANSFER_BUFFER_SIZE", 64)) * 1024, // Convert KB to bytes
		MaxConcurrentTransfers: int(parseIntEnv("MAX_CONCURRENT_TRANSFERS", 3)),
	}

	// Parse transfer configuration
	config.Transfer = TransferConfig{
		EnableCompression: parseBoolEnv("ENABLE_COMPRESSION", true),
		ResumeTransfers:   parseBoolEnv("RESUME_TRANSFERS", true),
	}

	// Validate required fields
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return config, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Source.Host == "" {
		return fmt.Errorf("SOURCE_PLEX_HOST is required")
	}
	if c.Source.Token == "" {
		return fmt.Errorf("SOURCE_PLEX_TOKEN is required")
	}
	if c.Destination.Host == "" {
		return fmt.Errorf("DEST_PLEX_HOST is required")
	}
	if c.Destination.Token == "" {
		return fmt.Errorf("DEST_PLEX_TOKEN is required")
	}
	if c.SyncLabel == "" {
		return fmt.Errorf("SYNC_LABEL is required")
	}

	// SSH is optional - if not provided, run in metadata-only mode
	// No validation required for SSH fields

	// Validate path mapping configuration
	// Source replacement is optional, but if one is provided, both must be provided
	sourceReplaceProvided := c.SourceReplaceFrom != "" || c.SourceReplaceTo != ""
	sourceBothProvided := c.SourceReplaceFrom != "" && c.SourceReplaceTo != ""

	if sourceReplaceProvided && !sourceBothProvided {
		return fmt.Errorf("if source path replacement is desired, both SOURCE_REPLACE_FROM and SOURCE_REPLACE_TO must be provided")
	}

	// DEST_ROOT_DIR is required if SSH is configured (file transfer mode)
	sshConfigured := c.SSH.User != "" && c.SSH.Password != ""
	if sshConfigured && c.DestRootDir == "" {
		return fmt.Errorf("DEST_ROOT_DIR is required when SSH is configured for file transfer")
	}

	// Validate log level
	validLogLevels := []string{"DEBUG", "INFO", "WARN", "ERROR"}
	isValidLogLevel := false
	for _, level := range validLogLevels {
		if c.LogLevel == level {
			isValidLogLevel = true
			break
		}
	}
	if !isValidLogLevel {
		return fmt.Errorf("invalid LOG_LEVEL: %s (must be one of: %s)", c.LogLevel, strings.Join(validLogLevels, ", "))
	}

	// Validate performance settings
	if c.Performance.WorkerPoolSize < 1 {
		return fmt.Errorf("WORKER_POOL_SIZE must be at least 1")
	}
	if c.Performance.PlexAPIRateLimit <= 0 {
		return fmt.Errorf("PLEX_API_RATE_LIMIT must be greater than 0")
	}
	if c.Performance.TransferBufferSize < 1024 {
		return fmt.Errorf("TRANSFER_BUFFER_SIZE must be at least 1KB")
	}
	if c.Performance.MaxConcurrentTransfers < 1 {
		return fmt.Errorf("MAX_CONCURRENT_TRANSFERS must be at least 1")
	}

	return nil
}

// GetSourceURL returns the full URL for the source Plex server
func (c *Config) GetSourceURL() string {
	return fmt.Sprintf("%s://%s:%s", c.Source.Protocol, c.Source.Host, c.Source.Port)
}

// GetDestinationURL returns the full URL for the destination Plex server
func (c *Config) GetDestinationURL() string {
	return fmt.Sprintf("%s://%s:%s", c.Destination.Protocol, c.Destination.Host, c.Destination.Port)
}

// Helper functions for parsing environment variables

func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func parseBoolEnv(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func parseIntEnv(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func parseFloatEnv(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			return parsed
		}
	}
	return defaultValue
}

// MapSourcePathToLocal converts a source Plex server path to a local filesystem path
func (c *Config) MapSourcePathToLocal(sourcePath string) (string, error) {
	if sourcePath == "" {
		return "", fmt.Errorf("source path is empty")
	}

	// If no source replacement configured, use the Plex path as-is
	if c.SourceReplaceFrom == "" {
		return filepath.FromSlash(sourcePath), nil
	}

	// If SourceReplaceFrom is set but SourceReplaceTo is empty,
	// use source path as-is (same volume mounting scenario)
	if c.SourceReplaceTo == "" {
		return filepath.FromSlash(sourcePath), nil
	}

	// Apply source replacement pattern
	sourcePathNorm := filepath.ToSlash(sourcePath)
	sourceReplaceFromNorm := filepath.ToSlash(c.SourceReplaceFrom)

	if !strings.HasPrefix(sourcePathNorm, sourceReplaceFromNorm) {
		return "", fmt.Errorf("source path %s does not start with replacement pattern %s", sourcePath, c.SourceReplaceFrom)
	}

	relativePath := strings.TrimPrefix(sourcePathNorm, sourceReplaceFromNorm)
	relativePath = strings.TrimPrefix(relativePath, "/")

	localPath := filepath.Join(c.SourceReplaceTo, relativePath)
	return localPath, nil
}

// MapLocalPathToDest converts a local filesystem path to a destination server path
func (c *Config) MapLocalPathToDest(localPath string) (string, error) {
	if localPath == "" {
		return "", fmt.Errorf("local path is empty")
	}

	if c.DestRootDir == "" {
		return "", fmt.Errorf("destination root directory not configured")
	}

	var relativePath string

	if c.SourceReplaceTo != "" {
		// Standard case: strip SourceReplaceTo prefix from local path
		localPathNorm := filepath.ToSlash(localPath)
		sourceReplaceToNorm := filepath.ToSlash(c.SourceReplaceTo)

		if !strings.HasPrefix(localPathNorm, sourceReplaceToNorm) {
			return "", fmt.Errorf("local path %s does not start with source replacement root %s", localPath, c.SourceReplaceTo)
		}

		relativePath = strings.TrimPrefix(localPathNorm, sourceReplaceToNorm)
		relativePath = strings.TrimPrefix(relativePath, "/")
	} else if c.SourceReplaceFrom != "" {
		// Same volume mounting: strip SourceReplaceFrom prefix to get relative path
		localPathNorm := filepath.ToSlash(localPath)
		sourceReplaceFromNorm := filepath.ToSlash(c.SourceReplaceFrom)

		if !strings.HasPrefix(localPathNorm, sourceReplaceFromNorm) {
			return "", fmt.Errorf("local path %s does not start with source replacement pattern %s", localPath, c.SourceReplaceFrom)
		}

		relativePath = strings.TrimPrefix(localPathNorm, sourceReplaceFromNorm)
		relativePath = strings.TrimPrefix(relativePath, "/")
	} else {
		// Fallback: use just the filename (preserves original behavior)
		relativePath = filepath.Base(localPath)
	}

	destPath := strings.TrimSuffix(c.DestRootDir, "/") + "/" + relativePath
	return destPath, nil
}
