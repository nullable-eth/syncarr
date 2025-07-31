package transfer

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

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

// internalTransferer defines the interface for internal transfer implementations (rsync/scp)
// These only handle the actual transfer without common logic like file checks and logging
type internalTransferer interface {
	doTransferFile(sourcePath, destPath string) error
	doTransferFiles(files []types.FileTransfer) error
	close() error
	fileExists(path string) (bool, error)
	getFileSize(path string) (int64, error)
	deleteFile(path string) error
	listDirectoryContents(rootPath string) ([]string, error)
	mapSourcePathToLocal(sourcePath string) (string, error)
	mapLocalPathToDest(localPath string) (string, error)
}

// TransferClient is the unified client that handles common logic and delegates to internal implementations
type TransferClient struct {
	method       TransferMethod
	internal     internalTransferer
	logger       *logger.Logger
	sshConfig    *config.SSHConfig
	serverConfig *config.PlexServerConfig
}

// NewTransferrer creates a new unified file transferrer that automatically chooses the best method
func NewTransferrer(method TransferMethod, cfg *config.Config, log *logger.Logger) (FileTransferrer, error) {
	var internal internalTransferer
	var err error

	switch method {
	case TransferMethodSFTP:
		internal, err = newSCPTransfer(cfg, log)
	case TransferMethodRsync:
		internal, err = newRsyncTransfer(cfg, log)
	default:
		return nil, fmt.Errorf("unsupported transfer method: %s", method)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create %s transferrer: %w", method, err)
	}

	return &TransferClient{
		method:       method,
		internal:     internal,
		logger:       log,
		sshConfig:    &cfg.SSH,
		serverConfig: &cfg.Destination,
	}, nil
}

// TransferFile handles file transfer with unified logic - checks file existence, size, and delegates to internal implementation
func (t *TransferClient) TransferFile(sourcePath, destPath string) error {
	// Get source file info
	fileInfo, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to stat source file: %w", err)
	}

	// Check if destination file already exists with same size (unified logic)
	destExists, err := t.internal.fileExists(destPath)
	if err != nil {
		t.logger.WithError(err).WithField("dest_path", destPath).Debug("Failed to check if destination file exists, proceeding with transfer")
	} else if destExists {
		// Check if sizes match - if so, skip transfer entirely
		destSize, err := t.internal.getFileSize(destPath)
		if err != nil {
			t.logger.WithError(err).WithField("dest_path", destPath).Debug("Failed to get destination file size, proceeding with transfer")
		} else if destSize == fileInfo.Size() {
			// Files are the same size, log skip and return early
			t.logger.LogTransferSkipped(sourcePath, destPath, fileInfo.Size(), "identical_size")
			return nil
		}
	}

	// If we get here, we're actually going to transfer the file
	startTime := time.Now()
	t.logger.LogTransferStarted(sourcePath, destPath, fileInfo.Size())

	// Delegate to internal implementation for actual transfer
	if err := t.internal.doTransferFile(sourcePath, destPath); err != nil {
		return fmt.Errorf("transfer failed using %s: %w", t.method, err)
	}

	// Log successful completion
	duration := time.Since(startTime)
	t.logger.LogTransferCompleted(sourcePath, destPath, fileInfo.Size(), duration)

	return nil
}

// TransferFiles transfers multiple files (delegates to internal implementation)
func (t *TransferClient) TransferFiles(files []types.FileTransfer) error {
	return t.internal.doTransferFiles(files)
}

// Close closes the transfer client
func (t *TransferClient) Close() error {
	return t.internal.close()
}

// FileExists checks if a file exists on the destination
func (t *TransferClient) FileExists(path string) (bool, error) {
	return t.internal.fileExists(path)
}

// GetFileSize gets the size of a file on the destination
func (t *TransferClient) GetFileSize(path string) (int64, error) {
	return t.internal.getFileSize(path)
}

// DeleteFile deletes a file on the destination
func (t *TransferClient) DeleteFile(path string) error {
	return t.internal.deleteFile(path)
}

// ListDirectoryContents lists directory contents on the destination
func (t *TransferClient) ListDirectoryContents(rootPath string) ([]string, error) {
	return t.internal.listDirectoryContents(rootPath)
}

// MapSourcePathToLocal maps source path to local path
func (t *TransferClient) MapSourcePathToLocal(sourcePath string) (string, error) {
	return t.internal.mapSourcePathToLocal(sourcePath)
}

// MapLocalPathToDest maps local path to destination path
func (t *TransferClient) MapLocalPathToDest(localPath string) (string, error) {
	return t.internal.mapLocalPathToDest(localPath)
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
