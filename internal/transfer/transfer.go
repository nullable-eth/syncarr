package transfer

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/nullable-eth/syncarr/internal/config"
	"github.com/nullable-eth/syncarr/internal/logger"
	"github.com/nullable-eth/syncarr/pkg/types"
)

// TransferMethod represents different transfer methods
type TransferMethod string

const (
	TransferMethodSFTP  TransferMethod = "sftp"
	TransferMethodRsync TransferMethod = "rsync"
)

// FileTransferrer defines the interface for file transfer implementations
type FileTransferrer interface {
	TransferFile(sourcePath, destPath string) error
	TransferFiles(files []types.FileTransfer) error
	Close() error
	FileExists(path string) (bool, error)
	GetFileSize(path string) (int64, error)
	DeleteFile(path string) error
	ListDirectoryContents(rootPath string) ([]string, error)
	MapSourcePathToLocal(sourcePath string) (string, error)
	MapLocalPathToDest(localPath string) (string, error)
}

// NewTransferrer creates a new file transferrer based on the specified method
func NewTransferrer(method TransferMethod, cfg *config.Config, log *logger.Logger) (FileTransferrer, error) {
	switch method {
	case TransferMethodSFTP:
		return NewSCPTransfer(cfg, log)
	case TransferMethodRsync:
		return NewRsyncTransfer(cfg, log)
	default:
		return nil, fmt.Errorf("unsupported transfer method: %s", method)
	}
}

// GetOptimalTransferMethod returns the recommended transfer method based on system capabilities
func GetOptimalTransferMethod(log *logger.Logger) TransferMethod {
	// Check if rsync is available
	if IsRsyncAvailable(log) {
		log.Info("rsync detected - using high-performance rsync transfers")
		return TransferMethodRsync
	}

	log.Info("rsync not available - falling back to SFTP transfers")
	return TransferMethodSFTP
}

// ForceTransferMethod forces a specific transfer method (useful for testing)
func ForceTransferMethod(method TransferMethod, log *logger.Logger) TransferMethod {
	log.WithField("forced_method", string(method)).Info("Using forced transfer method")
	return method
}

// IsRsyncAvailable checks if rsync is installed and available locally
func IsRsyncAvailable(log *logger.Logger) bool {
	// Enhanced debugging for Windows rsync detection
	log.Debug("Starting rsync availability check")

	// Log PATH environment for debugging
	pathEnv := os.Getenv("PATH")
	pathDirs := strings.Split(pathEnv, string(os.PathListSeparator))
	log.WithField("path_dir_count", len(pathDirs)).Debug("PATH environment variable loaded")

	// Check for common rsync locations on Windows
	rsyncDirs := []string{}
	for _, dir := range pathDirs {
		if strings.Contains(strings.ToLower(dir), "rsync") {
			rsyncDirs = append(rsyncDirs, dir)
		}
	}
	if len(rsyncDirs) > 0 {
		log.WithField("rsync_dirs_in_path", rsyncDirs).Debug("Found rsync-related directories in PATH")
	}

	// Try different rsync executable names (Windows compatibility)
	rsyncNames := []string{"rsync", "rsync.exe"}

	for _, name := range rsyncNames {
		log.WithField("checking_name", name).Debug("Checking for rsync executable")

		rsyncPath, err := exec.LookPath(name)
		if err != nil {
			log.WithError(err).WithField("executable_name", name).Debug("LookPath failed for rsync name")
			continue
		}

		log.WithField("rsync_path", rsyncPath).Info("rsync found locally via LookPath")

		// Test if rsync actually runs (quick version check)
		if testRsyncExecution(rsyncPath, log) {
			return true
		}
	}

	// Specifically check user's mentioned location: C:\rsyncd\bin
	specificPaths := []string{
		"C:\\rsyncd\\bin\\rsync.exe",
		"C:\\rsyncd\\bin\\rsync",
		"C:\\Program Files\\Git\\usr\\bin\\rsync.exe",
		"C:\\msys64\\usr\\bin\\rsync.exe",
	}

	log.Debug("LookPath failed, checking specific common Windows rsync locations")
	for _, specificPath := range specificPaths {
		if _, err := os.Stat(specificPath); err == nil {
			log.WithField("rsync_path", specificPath).Info("rsync found at specific Windows location")
			if testRsyncExecution(specificPath, log) {
				return true
			}
		} else {
			log.WithField("path", specificPath).Debug("Specific rsync path does not exist")
		}
	}

	log.Warn("rsync not found in PATH or common Windows locations")
	log.WithField("search_names", rsyncNames).Debug("Searched for these rsync executable names")
	log.WithField("specific_paths", specificPaths).Debug("Also checked these specific Windows paths")
	log.Info("rsync requires installation on both local system and destination system")
	log.Info("On Windows: ensure rsync is installed and available in PATH (current search: rsync, rsync.exe)")
	return false
}

// testRsyncExecution tests if a found rsync executable actually works
func testRsyncExecution(rsyncPath string, log *logger.Logger) bool {
	log.WithField("rsync_path", rsyncPath).Debug("Testing rsync execution with --version")
	cmd := exec.Command(rsyncPath, "--version")

	output, err := cmd.Output()
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"rsync_path": rsyncPath,
			"command":    rsyncPath + " --version",
		}).Warn("rsync found but failed to execute --version command")
		return false
	}

	versionText := string(output)
	if len(versionText) > 100 {
		versionText = versionText[:100] + "..."
	}

	log.WithFields(map[string]interface{}{
		"rsync_path":    rsyncPath,
		"rsync_version": versionText,
	}).Info("rsync version check successful - rsync is available")

	return true
}
