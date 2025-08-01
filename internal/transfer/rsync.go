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
	// Directory creation is now handled by the common transferrer before calling this method

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

	// Check if rsync actually transferred data or skipped the file
	if r.isFileSkipped(string(output), sourcePath) {
		r.logger.WithFields(map[string]interface{}{
			"source_path": sourcePath,
			"dest_path":   destPath,
		}).Debug("File was skipped by rsync (already up-to-date)")
		return nil // Not an error, just skipped
	}

	return nil
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

	// For exec.Command, we don't need shell quoting - Go handles argument separation
	remoteDest := fmt.Sprintf("%s:%s", remoteHost, destPath)

	args := []string{
		"-avz",              // Archive mode, verbose, compression
		"--progress",        // Show progress
		"--partial",         // Keep partial transfers
		"--inplace",         // Update files in place (faster for large files)
		"--itemize-changes", // Show detailed changes (helps detect skips)
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

// isFileSkipped analyzes rsync output to determine if the file was skipped (not transferred)
func (r *RsyncTransfer) isFileSkipped(output, sourcePath string) bool {
	outputLines := strings.Split(output, "\n")
	filename := filepath.Base(sourcePath)

	// Look for itemize-changes output: lines starting with itemize codes
	// If file was transferred, we'd see something like ">f+++++++++" (new file) or ">f.st......" (updated file)
	// If file was skipped, there will be no itemize line for this file, or minimal output

	hasItemizeOutput := false
	hasTransferProgress := false

	for _, line := range outputLines {
		line = strings.TrimSpace(line)

		// Check for itemize-changes output (indicates actual changes)
		if strings.HasPrefix(line, ">f") && strings.Contains(line, filename) {
			hasItemizeOutput = true
			r.logger.WithFields(map[string]interface{}{
				"itemize_line": line,
				"filename":     filename,
			}).Debug("Detected rsync itemize output indicating file transfer")
		}

		// Check for progress output (indicates data transfer)
		if strings.Contains(line, filename) && (strings.Contains(line, "bytes/sec") || strings.Contains(line, "%")) {
			hasTransferProgress = true
			r.logger.WithFields(map[string]interface{}{
				"progress_line": line,
				"filename":      filename,
			}).Debug("Detected rsync progress output indicating file transfer")
		}
	}

	// If we have no itemize output AND no progress, the file was likely skipped
	fileSkipped := !hasItemizeOutput && !hasTransferProgress

	if fileSkipped {
		r.logger.WithFields(map[string]interface{}{
			"filename":              filename,
			"has_itemize_output":    hasItemizeOutput,
			"has_transfer_progress": hasTransferProgress,
			"rsync_output":          output,
		}).Debug("File appears to have been skipped by rsync (up-to-date)")
	}

	return fileSkipped
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

	// Directory creation is now handled by the common transferrer before calling transfer methods

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
