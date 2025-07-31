// Package transfer provides file transfer implementations for syncarr.
package transfer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nullable-eth/syncarr/internal/config"
	"github.com/nullable-eth/syncarr/internal/logger"
	"github.com/nullable-eth/syncarr/pkg/types"
)

// RsyncTransfer handles file transfers using rsync over SSH
type RsyncTransfer struct {
	sshConfig         *config.SSHConfig
	serverConfig      *config.PlexServerConfig
	sourceReplaceFrom string
	sourceReplaceTo   string
	destRootDir       string
	logger            *logger.Logger
	compressionLevel  int  // 0-9, 0=none, 6=default, 9=max
	parallelStreams   int  // Number of parallel rsync streams
	checksumSkip      bool // Skip checksum verification for speed
}

// newRsyncTransfer creates a new rsync transfer instance (package-private)
func newRsyncTransfer(cfg *config.Config, log *logger.Logger) (*RsyncTransfer, error) {
	return &RsyncTransfer{
		sshConfig:         &cfg.SSH,
		serverConfig:      &cfg.Destination,
		sourceReplaceFrom: cfg.SourceReplaceFrom,
		sourceReplaceTo:   cfg.SourceReplaceTo,
		destRootDir:       cfg.DestRootDir,
		logger:            log,
		compressionLevel:  1,    // Light compression for speed vs bandwidth balance
		parallelStreams:   4,    // Multiple parallel streams
		checksumSkip:      true, // Skip checksums for max speed (trust network)
	}, nil
}

// doTransferFile transfers a single file using rsync (internal implementation without common logic)
func (r *RsyncTransfer) doTransferFile(sourcePath, destPath string) error {
	// Ensure destination directory exists
	if err := r.ensureDestinationDir(destPath); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Build rsync command with optimizations
	args := r.buildRsyncArgs(sourcePath, destPath)

	cmd := exec.Command("rsync", args...)

	// Capture output for debugging
	output, err := cmd.CombinedOutput()
	if err != nil {
		r.logger.WithFields(map[string]interface{}{
			"source_path": sourcePath,
			"dest_path":   destPath,
			"rsync_args":  strings.Join(args, " "),
			"output":      string(output),
		}).Error("Rsync command failed")
		return fmt.Errorf("rsync failed: %w", err)
	}

	return nil
}

// TransferFile transfers a single file using rsync (public interface for backward compatibility)
func (r *RsyncTransfer) TransferFile(sourcePath, destPath string) error {
	return r.doTransferFile(sourcePath, destPath)
}

// doTransferFiles transfers multiple files using rsync (internal implementation)
func (r *RsyncTransfer) doTransferFiles(files []types.FileTransfer) error {
	// For small numbers of files, transfer individually
	if len(files) <= 3 {
		for _, file := range files {
			if err := r.doTransferFile(file.SourcePath, file.DestPath); err != nil {
				return err
			}
		}
		return nil
	}

	// For larger batches, use rsync's batch capabilities
	return r.transferFilesBatch(files)
}

// buildRsyncArgs builds optimized rsync arguments
func (r *RsyncTransfer) buildRsyncArgs(sourcePath, destPath string) []string {
	remoteHost := fmt.Sprintf("%s@%s", r.sshConfig.User, r.serverConfig.Host)
	remoteDest := fmt.Sprintf("%s:%s", remoteHost, destPath)

	args := []string{
		"-avz",       // Archive mode, verbose, compression
		"--progress", // Show progress
		"--partial",  // Keep partial transfers
		"--inplace",  // Update files in place (faster for large files)
	}

	// Compression settings
	if r.compressionLevel > 0 {
		args = append(args, fmt.Sprintf("--compress-level=%d", r.compressionLevel))
	} else {
		// Remove compression if level is 0
		args[0] = "-av"
	}

	// Skip checksum verification for speed
	if r.checksumSkip {
		args = append(args, "--no-whole-file", "--no-compress")
	}

	// SSH options for performance
	sshOpts := []string{
		"-o", "Compression=no", // Handle compression in rsync, not SSH
		"-o", "TCPKeepAlive=yes",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=6",
		"-o", "StrictHostKeyChecking=no",
	}

	if r.sshConfig.Port != "" && r.sshConfig.Port != "22" {
		sshOpts = append(sshOpts, "-p", r.sshConfig.Port)
	}

	// Build SSH command - use sshpass for password authentication
	var sshCmd string
	if r.sshConfig.Password != "" {
		sshCmd = fmt.Sprintf("sshpass -p '%s' ssh %s", r.sshConfig.Password, strings.Join(sshOpts, " "))
		r.logger.Debug("Using sshpass for SSH password authentication")
	} else {
		sshCmd = fmt.Sprintf("ssh %s", strings.Join(sshOpts, " "))
		r.logger.Debug("Using SSH key-based authentication")
	}

	args = append(args, "-e", sshCmd)
	args = append(args, sourcePath, remoteDest)

	return args
}

// transferFilesBatch transfers multiple files in batches for efficiency
func (r *RsyncTransfer) transferFilesBatch(files []types.FileTransfer) error {
	// Group files by directory for more efficient transfers
	dirGroups := make(map[string][]types.FileTransfer)

	for _, file := range files {
		sourceDir := filepath.Dir(file.SourcePath)
		dirGroups[sourceDir] = append(dirGroups[sourceDir], file)
	}

	// Transfer each directory group
	for sourceDir, dirFiles := range dirGroups {
		if err := r.transferDirectoryBatch(sourceDir, dirFiles); err != nil {
			return err
		}
	}

	return nil
}

// transferDirectoryBatch transfers all files in a directory efficiently
func (r *RsyncTransfer) transferDirectoryBatch(sourceDir string, files []types.FileTransfer) error {
	if len(files) == 0 {
		return nil
	}

	// Create include file for specific files
	includeFile, err := r.createIncludeFile(sourceDir, files)
	if err != nil {
		return fmt.Errorf("failed to create include file: %w", err)
	}
	defer os.Remove(includeFile)

	// Use first file's destination to determine target directory
	destDir := filepath.Dir(files[0].DestPath)

	// Ensure destination directory exists for all files in batch
	for _, file := range files {
		if err := r.ensureDestinationDir(file.DestPath); err != nil {
			return fmt.Errorf("failed to create destination directory for %s: %w", file.DestPath, err)
		}
	}

	remoteHost := fmt.Sprintf("%s@%s", r.sshConfig.User, r.serverConfig.Host)
	remoteDest := fmt.Sprintf("%s:%s/", remoteHost, destDir)

	args := []string{
		"-avz",
		"--progress",
		"--partial",
		"--inplace",
		fmt.Sprintf("--include-from=%s", includeFile),
		"--exclude=*", // Exclude everything not in include file
	}

	// Add SSH options
	sshOpts := []string{
		"-o", "Compression=no",
		"-o", "TCPKeepAlive=yes",
		"-o", "StrictHostKeyChecking=no",
	}

	if r.sshConfig.Port != "" && r.sshConfig.Port != "22" {
		sshOpts = append(sshOpts, "-p", r.sshConfig.Port)
	}

	// Build SSH command - use sshpass for password authentication
	var sshCmd string
	if r.sshConfig.Password != "" {
		sshCmd = fmt.Sprintf("sshpass -p '%s' ssh %s", r.sshConfig.Password, strings.Join(sshOpts, " "))
		r.logger.Debug("Using sshpass for batch transfer with SSH password authentication")
	} else {
		sshCmd = fmt.Sprintf("ssh %s", strings.Join(sshOpts, " "))
		r.logger.Debug("Using SSH key-based authentication for batch transfer")
	}

	args = append(args, "-e", sshCmd)
	args = append(args, sourceDir+"/", remoteDest)

	cmd := exec.Command("rsync", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		r.logger.WithFields(map[string]interface{}{
			"source_dir": sourceDir,
			"dest_dir":   destDir,
			"file_count": len(files),
			"rsync_args": strings.Join(args, " "),
			"output":     string(output),
		}).Error("Batch rsync failed")
		return fmt.Errorf("batch rsync failed: %w", err)
	}

	return nil
}

// createIncludeFile creates a temporary file listing specific files to include
func (r *RsyncTransfer) createIncludeFile(baseDir string, files []types.FileTransfer) (string, error) {
	tmpFile, err := os.CreateTemp("", "rsync-include-*.txt")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	for _, file := range files {
		// Get relative path from base directory
		relPath, err := filepath.Rel(baseDir, file.SourcePath)
		if err != nil {
			return "", fmt.Errorf("failed to get relative path: %w", err)
		}

		// Write to include file
		if _, err := fmt.Fprintln(tmpFile, relPath); err != nil {
			return "", err
		}
	}

	return tmpFile.Name(), nil
}

// TransferFiles transfers multiple files using rsync (public interface for backward compatibility)
func (r *RsyncTransfer) TransferFiles(files []types.FileTransfer) error {
	return r.doTransferFiles(files)
}

// Internal interface methods (lowercase for internalTransferer interface)
func (r *RsyncTransfer) fileExists(path string) (bool, error) {
	return r.FileExists(path)
}

func (r *RsyncTransfer) getFileSize(path string) (int64, error) {
	return r.GetFileSize(path)
}

func (r *RsyncTransfer) deleteFile(path string) error {
	return r.DeleteFile(path)
}

func (r *RsyncTransfer) listDirectoryContents(rootPath string) ([]string, error) {
	return r.ListDirectoryContents(rootPath)
}

func (r *RsyncTransfer) mapSourcePathToLocal(sourcePath string) (string, error) {
	return r.MapSourcePathToLocal(sourcePath)
}

func (r *RsyncTransfer) mapLocalPathToDest(localPath string) (string, error) {
	return r.MapLocalPathToDest(localPath)
}

func (r *RsyncTransfer) close() error {
	return r.Close()
}

// Close is a no-op for rsync (no persistent connections)
func (r *RsyncTransfer) Close() error {
	return nil
}

// DeleteFile deletes a file on the remote server using SSH
func (r *RsyncTransfer) DeleteFile(path string) error {
	remoteHost := fmt.Sprintf("%s@%s", r.sshConfig.User, r.serverConfig.Host)

	sshCmd := []string{
		"ssh",
		"-o", "StrictHostKeyChecking=no",
		"-o", "ConnectTimeout=10",
	}

	if r.sshConfig.Port != "" && r.sshConfig.Port != "22" {
		sshCmd = append(sshCmd, "-p", r.sshConfig.Port)
	}

	sshCmd = append(sshCmd, remoteHost, fmt.Sprintf("rm -f '%s'", path))

	cmd := exec.Command(sshCmd[0], sshCmd[1:]...)
	return cmd.Run()
}

// ListDirectoryContents recursively lists all files in a directory
func (r *RsyncTransfer) ListDirectoryContents(rootPath string) ([]string, error) {
	remoteHost := fmt.Sprintf("%s@%s", r.sshConfig.User, r.serverConfig.Host)

	sshCmd := []string{
		"ssh",
		"-o", "StrictHostKeyChecking=no",
		"-o", "ConnectTimeout=10",
	}

	if r.sshConfig.Port != "" && r.sshConfig.Port != "22" {
		sshCmd = append(sshCmd, "-p", r.sshConfig.Port)
	}

	sshCmd = append(sshCmd, remoteHost, fmt.Sprintf("find '%s' -type f", rootPath))

	cmd := exec.Command(sshCmd[0], sshCmd[1:]...)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(files) == 1 && files[0] == "" {
		return []string{}, nil
	}

	r.logger.WithFields(map[string]interface{}{
		"root_path":  rootPath,
		"file_count": len(files),
	}).Debug("Listed directory contents")

	return files, nil
}

// FileExists checks if a file exists on the remote server using SSH
func (r *RsyncTransfer) FileExists(path string) (bool, error) {
	remoteHost := fmt.Sprintf("%s@%s", r.sshConfig.User, r.serverConfig.Host)

	sshCmd := []string{
		"ssh",
		"-o", "StrictHostKeyChecking=no",
		"-o", "ConnectTimeout=10",
	}

	if r.sshConfig.Port != "" && r.sshConfig.Port != "22" {
		sshCmd = append(sshCmd, "-p", r.sshConfig.Port)
	}

	sshCmd = append(sshCmd, remoteHost, fmt.Sprintf("test -f '%s'", path))

	cmd := exec.Command(sshCmd[0], sshCmd[1:]...)
	err := cmd.Run()

	return err == nil, nil
}

// GetFileSize returns the size of a remote file
func (r *RsyncTransfer) GetFileSize(path string) (int64, error) {
	remoteHost := fmt.Sprintf("%s@%s", r.sshConfig.User, r.serverConfig.Host)

	sshCmd := []string{
		"ssh",
		"-o", "StrictHostKeyChecking=no",
		"-o", "ConnectTimeout=10",
	}

	if r.sshConfig.Port != "" && r.sshConfig.Port != "22" {
		sshCmd = append(sshCmd, "-p", r.sshConfig.Port)
	}

	sshCmd = append(sshCmd, remoteHost, fmt.Sprintf("stat -f%%z '%s' 2>/dev/null || stat -c%%s '%s'", path, path))

	cmd := exec.Command(sshCmd[0], sshCmd[1:]...)
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	var size int64
	if _, err := fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &size); err != nil {
		return 0, err
	}

	return size, nil
}

// MapSourcePathToLocal converts a source Plex server path to a local filesystem path
func (r *RsyncTransfer) MapSourcePathToLocal(sourcePath string) (string, error) {
	if sourcePath == "" {
		return "", fmt.Errorf("source path is empty")
	}

	// If no source replacement configured, use the Plex path as-is
	if r.sourceReplaceFrom == "" || r.sourceReplaceTo == "" {
		return filepath.FromSlash(sourcePath), nil
	}

	// Apply source replacement pattern
	sourcePathNorm := filepath.ToSlash(sourcePath)
	sourceReplaceFromNorm := filepath.ToSlash(r.sourceReplaceFrom)

	if !strings.HasPrefix(sourcePathNorm, sourceReplaceFromNorm) {
		return "", fmt.Errorf("source path %s does not start with replacement pattern %s", sourcePath, r.sourceReplaceFrom)
	}

	relativePath := strings.TrimPrefix(sourcePathNorm, sourceReplaceFromNorm)
	relativePath = strings.TrimPrefix(relativePath, "/")

	localPath := filepath.Join(r.sourceReplaceTo, relativePath)
	return localPath, nil
}

// MapLocalPathToDest converts a local filesystem path to a destination server path
func (r *RsyncTransfer) MapLocalPathToDest(localPath string) (string, error) {
	if localPath == "" {
		return "", fmt.Errorf("local path is empty")
	}

	if r.destRootDir == "" {
		return "", fmt.Errorf("destination root directory not configured")
	}

	var relativePath string

	if r.sourceReplaceTo != "" {
		localPathNorm := filepath.ToSlash(localPath)
		sourceReplaceToNorm := filepath.ToSlash(r.sourceReplaceTo)

		if !strings.HasPrefix(localPathNorm, sourceReplaceToNorm) {
			return "", fmt.Errorf("local path %s does not start with source replacement root %s", localPath, r.sourceReplaceTo)
		}

		relativePath = strings.TrimPrefix(localPathNorm, sourceReplaceToNorm)
		relativePath = strings.TrimPrefix(relativePath, "/")
	} else {
		relativePath = filepath.Base(localPath)
	}

	destPath := strings.TrimSuffix(r.destRootDir, "/") + "/" + relativePath
	return destPath, nil
}

// ensureDestinationDir creates the destination directory on the remote server if it doesn't exist
func (r *RsyncTransfer) ensureDestinationDir(destPath string) error {
	// Extract directory from destination path
	destDir := filepath.Dir(destPath)

	// Build SSH command to create directory
	remoteHost := fmt.Sprintf("%s@%s", r.sshConfig.User, r.serverConfig.Host)
	mkdirCmd := fmt.Sprintf("mkdir -p '%s'", destDir)

	// SSH options
	sshOpts := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "ConnectTimeout=10",
	}

	if r.sshConfig.Port != "" && r.sshConfig.Port != "22" {
		sshOpts = append(sshOpts, "-p", r.sshConfig.Port)
	}

	// Build command with authentication
	var cmd *exec.Cmd
	if r.sshConfig.Password != "" {
		// Use sshpass for password authentication
		args := append([]string{"-p", r.sshConfig.Password, "ssh"}, sshOpts...)
		args = append(args, remoteHost, mkdirCmd)
		cmd = exec.Command("sshpass", args...)
		r.logger.WithField("dest_dir", destDir).Debug("Creating remote directory with sshpass")
	} else {
		// Use SSH key-based authentication
		args := append(sshOpts, remoteHost, mkdirCmd)
		cmd = exec.Command("ssh", args...)
		r.logger.WithField("dest_dir", destDir).Debug("Creating remote directory with SSH keys")
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		r.logger.WithFields(map[string]interface{}{
			"dest_dir": destDir,
			"output":   string(output),
		}).Warn("Failed to create remote directory (may already exist)")
		// Don't return error - directory might already exist
	} else {
		r.logger.WithField("dest_dir", destDir).Debug("Remote directory created successfully")
	}

	return nil
}
