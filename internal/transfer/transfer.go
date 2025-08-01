package transfer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nullable-eth/syncarr/internal/config"
	"github.com/nullable-eth/syncarr/internal/logger"
	"github.com/nullable-eth/syncarr/pkg/types"
)

// TransferMethod represents different transfer methods
type TransferMethod string

const (
	TransferMethodSCP   TransferMethod = "scp"
	TransferMethodRsync TransferMethod = "rsync"
)

// FileTransferrer defines the interface for file transfer implementations
type FileTransferrer interface {
	TransferFile(sourcePath, destPath string) error
	TransferFiles(files []types.FileTransfer) error
	Close() error
	GetFileSize(path string) (int64, error)
	DeleteFile(path string) error
	ListDirectoryContents(rootPath string) ([]string, error)
}

// transferImplementation defines the interface for actual transfer implementations (rsync/scp only)
type transferImplementation interface {
	doTransferFile(sourcePath, destPath string) error
	doTransferFiles(files []types.FileTransfer) error
}

// transferClient is the unified client that handles common logic and delegates to internal implementations
type transferClient struct {
	method   TransferMethod
	fileOps  fileOperations
	transfer transferImplementation
	logger   *logger.Logger
}

// newSSHClient creates a new SSH client for file operations
func newSSHClient(cfg *config.Config, log *logger.Logger) (fileOperations, error) {
	return &sshClient{
		sshConfig:    &cfg.SSH,
		serverConfig: &cfg.Destination,
		logger:       log,
	}, nil
}

// NewTransferrer creates a new unified file transferrer that automatically chooses the best method
func NewTransferrer(method TransferMethod, cfg *config.Config, log *logger.Logger) (FileTransferrer, error) {
	// Create shared SSH client for all file operations
	sshFileOps, err := newSSHClient(cfg, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH client: %w", err)
	}

	// Create transfer implementation
	var transferImpl transferImplementation

	switch method {
	case TransferMethodSCP:
		transferImpl, err = newSCPTransfer(cfg, log)
		if err != nil {
			return nil, fmt.Errorf("failed to create SCP transferrer: %w", err)
		}
	case TransferMethodRsync:
		transferImpl, err = newRsyncTransfer(cfg, log)
		if err != nil {
			return nil, fmt.Errorf("failed to create rsync transferrer: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported transfer method: %s", method)
	}

	log.WithField("transfer_method", string(method)).Info("High-performance file transfer enabled")

	return &transferClient{
		method:   method,
		fileOps:  sshFileOps,
		transfer: transferImpl,
		logger:   log,
	}, nil
}

// TransferFile handles file transfer with unified logic - checks file existence, size, and delegates to internal implementation
func (t *transferClient) TransferFile(sourcePath, destPath string) error {
	// Get source file info
	fileInfo, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to stat source file: %w", err)
	}

	// Check if destination file exists and get its size in one optimized call
	destSize, err := t.fileOps.GetFileSize(destPath)
	if err != nil {
		// File doesn't exist or can't be accessed, proceed with transfer
		t.logger.WithError(err).WithField("dest_path", destPath).Debug("Destination file doesn't exist or can't be accessed, proceeding with transfer")
	} else if destSize == fileInfo.Size() {
		// Files are the same size, log skip and return early
		t.logger.LogTransferSkipped(sourcePath, destPath, fileInfo.Size(), "identical_size")
		return nil
	}

	// Ensure destination directory exists before transfer
	if err := t.ensureDestinationDir(destPath); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// If we get here, we're actually going to transfer the file
	startTime := time.Now()
	t.logger.LogTransferStarted(sourcePath, destPath, fileInfo.Size())

	// Delegate to transfer implementation for actual transfer (directory already created)
	if err := t.transfer.doTransferFile(sourcePath, destPath); err != nil {
		// Check if this is a special "file was skipped" error
		if strings.Contains(err.Error(), "file_skipped") {
			// File was skipped by rsync (already up-to-date), log as skipped
			t.logger.LogTransferSkipped(sourcePath, destPath, fileInfo.Size(), "rsync_skipped")
			return nil
		}
		return fmt.Errorf("transfer failed using %s: %w", t.method, err)
	}

	// Log successful completion
	duration := time.Since(startTime)
	t.logger.LogTransferCompleted(sourcePath, destPath, fileInfo.Size(), duration)

	return nil
}

// TransferFiles transfers multiple files (delegates to transfer implementation)
func (t *transferClient) TransferFiles(files []types.FileTransfer) error {
	return t.transfer.doTransferFiles(files)
}

// Close closes the SSH connection
func (t *transferClient) Close() error {
	return t.fileOps.Close()
}

// GetFileSize gets the size of a file on the destination (via SSH)
func (t *transferClient) GetFileSize(path string) (int64, error) {
	return t.fileOps.GetFileSize(path)
}

// DeleteFile deletes a file on the destination (via SSH)
func (t *transferClient) DeleteFile(path string) error {
	return t.fileOps.DeleteFile(path)
}

// ListDirectoryContents lists directory contents on the destination (via SSH)
func (t *transferClient) ListDirectoryContents(rootPath string) ([]string, error) {
	return t.fileOps.ListDirectoryContents(rootPath)
}

// ensureDestinationDir creates the destination directory using SSH
func (t *transferClient) ensureDestinationDir(destPath string) error {
	destDir := filepath.Dir(destPath)

	// Use SSH to create the directory
	return t.fileOps.CreateDirectory(destDir)
}

// GetOptimalTransferMethod returns the recommended transfer method based on system capabilities
func GetOptimalTransferMethod(log *logger.Logger) TransferMethod {
	// Check if rsync is available
	if IsRsyncAvailable(log) {
		log.Info("rsync detected - using high-performance rsync transfers")
		return TransferMethodRsync
	}

	log.Info("rsync not available - falling back to SCP transfers")
	return TransferMethodSCP
}

// ForceTransferMethod forces a specific transfer method and creates a transfer client (useful for testing)
func ForceTransferMethod(method TransferMethod, cfg *config.Config, log *logger.Logger) (FileTransferrer, error) {
	log.WithField("forced_method", string(method)).Info("Using forced transfer method")
	return NewTransferrer(method, cfg, log)
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
