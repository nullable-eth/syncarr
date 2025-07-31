package transfer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nullable-eth/syncarr/internal/config"
	"github.com/nullable-eth/syncarr/internal/logger"
	"github.com/nullable-eth/syncarr/pkg/types"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// SCPTransfer handles file transfers using SCP over SSH
type SCPTransfer struct {
	sshConfig         *config.SSHConfig
	serverConfig      *config.PlexServerConfig
	sourceReplaceFrom string // Optional: Source path pattern to replace
	sourceReplaceTo   string // Optional: Local path replacement
	destRootDir       string // Required: Destination root directory
	logger            *logger.Logger
	sshClient         *ssh.Client
	sftpClient        *sftp.Client
	bufferSize        int // Buffer size for transfers
	maxConcurrent     int // Maximum concurrent transfers
}

// newSCPTransfer creates a new SCP transfer instance (package-private)
func newSCPTransfer(cfg *config.Config, log *logger.Logger) (*SCPTransfer, error) {
	transfer := &SCPTransfer{
		sshConfig:         &cfg.SSH,
		serverConfig:      &cfg.Destination,
		sourceReplaceFrom: cfg.SourceReplaceFrom,
		sourceReplaceTo:   cfg.SourceReplaceTo,
		destRootDir:       cfg.DestRootDir,
		logger:            log,
		bufferSize:        1024 * 1024, // 1MB buffer for better performance
		maxConcurrent:     3,           // Allow up to 3 concurrent transfers
	}

	// Establish SSH connection
	if err := transfer.connect(); err != nil {
		return nil, fmt.Errorf("failed to establish SSH connection: %w", err)
	}

	return transfer, nil
}

// connect establishes SSH and SFTP connections
func (s *SCPTransfer) connect() error {
	// Create SSH client configuration with password authentication and optimized settings
	sshClientConfig := &ssh.ClientConfig{
		User: s.sshConfig.User,
		Auth: []ssh.AuthMethod{
			ssh.Password(s.sshConfig.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // For simplicity, ignore host key verification
		Timeout:         30 * time.Second,
		// Optimize for high throughput
		Config: ssh.Config{
			Ciphers: []string{
				"aes128-ctr", "aes192-ctr", "aes256-ctr", // Faster AES-CTR ciphers
				"aes128-gcm@openssh.com", "aes256-gcm@openssh.com",
			},
		},
	}

	// Connect to SSH server using destination Plex server host
	sshAddr := fmt.Sprintf("%s:%s", s.serverConfig.Host, s.sshConfig.Port)
	sshClient, err := ssh.Dial("tcp", sshAddr, sshClientConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to SSH server %s: %w", sshAddr, err)
	}
	s.sshClient = sshClient

	// Create SFTP client
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		if closeErr := sshClient.Close(); closeErr != nil {
			s.logger.WithError(closeErr).Warn("Failed to close SSH client after SFTP creation error")
		}
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	s.sftpClient = sftpClient

	s.logger.WithField("ssh_host", sshAddr).Info("Successfully connected to SSH/SFTP server")
	return nil
}

// Close closes the SSH and SFTP connections
func (s *SCPTransfer) Close() error {
	var errs []error

	if s.sftpClient != nil {
		if err := s.sftpClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close SFTP client: %w", err))
		}
	}

	if s.sshClient != nil {
		if err := s.sshClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close SSH client: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing connections: %v", errs)
	}

	return nil
}

// doTransferFile transfers a single file from source to destination (internal implementation without common logic)
func (s *SCPTransfer) doTransferFile(sourcePath, destPath string) error {
	// Get source file info first
	fileInfo, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to stat source file: %w", err)
	}

	// Create destination directory if it doesn't exist
	// Use forward slashes for remote paths (SFTP always uses Unix-style paths)
	lastSlash := strings.LastIndex(destPath, "/")
	if lastSlash == -1 {
		return fmt.Errorf("invalid destination path format: %s", destPath)
	}
	destDir := destPath[:lastSlash]

	s.logger.WithFields(map[string]interface{}{
		"dest_dir":  destDir,
		"dest_path": destPath,
	}).Debug("Creating destination directory")

	if err := s.sftpClient.MkdirAll(destDir); err != nil {
		return fmt.Errorf("failed to create destination directory %s: %w", destDir, err)
	}

	// Verify directory was created successfully
	if dirInfo, err := s.sftpClient.Stat(destDir); err != nil {
		return fmt.Errorf("destination directory %s was not created successfully: %w", destDir, err)
	} else if !dirInfo.IsDir() {
		return fmt.Errorf("destination path %s exists but is not a directory", destDir)
	}

	s.logger.WithField("dest_dir", destDir).Debug("Destination directory verified")

	// Test write permissions by trying to create a temporary test file
	testFilePath := destDir + "/.sync_test_" + fmt.Sprintf("%d", time.Now().UnixNano())
	if testFile, testErr := s.sftpClient.Create(testFilePath); testErr != nil {
		s.logger.WithFields(map[string]interface{}{
			"dest_dir":       destDir,
			"test_file_path": testFilePath,
			"test_error":     testErr.Error(),
		}).Error("Cannot create test file in destination directory - permissions issue?")
	} else {
		testFile.Close()
		if removeErr := s.sftpClient.Remove(testFilePath); removeErr != nil {
			s.logger.WithFields(map[string]interface{}{
				"test_file_path": testFilePath,
				"remove_error":   removeErr.Error(),
			}).Warn("Failed to clean up test file")
		}
		s.logger.WithField("dest_dir", destDir).Debug("Write permissions verified with test file")
	}

	// Open source file
	srcFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	// Create destination file with more detailed error context
	s.logger.WithFields(map[string]interface{}{
		"dest_path": destPath,
		"dest_dir":  destDir,
	}).Debug("Creating destination file")

	dstFile, err := s.sftpClient.Create(destPath)
	if err != nil {
		// Check if directory exists to provide better error context
		if dirInfo, dirErr := s.sftpClient.Stat(destDir); dirErr != nil {
			return fmt.Errorf("failed to create destination file %s: destination directory %s does not exist or is not accessible: %w (original error: %v)", destPath, destDir, dirErr, err)
		} else if !dirInfo.IsDir() {
			return fmt.Errorf("failed to create destination file %s: %s exists but is not a directory: %w", destPath, destDir, err)
		} else {
			// Directory exists, let's get more debugging info
			s.logger.WithFields(map[string]interface{}{
				"dest_dir":     destDir,
				"dir_mode":     dirInfo.Mode().String(),
				"dir_size":     dirInfo.Size(),
				"create_error": err.Error(),
			}).Error("Directory exists but file creation failed")

			// Try to list directory contents for debugging
			if entries, listErr := s.sftpClient.ReadDir(destDir); listErr != nil {
				s.logger.WithError(listErr).WithField("dest_dir", destDir).Debug("Could not list directory contents")
			} else {
				s.logger.WithFields(map[string]interface{}{
					"dest_dir":    destDir,
					"entry_count": len(entries),
				}).Debug("Directory contents listed successfully")
			}

			return fmt.Errorf("failed to create destination file %s in existing directory %s (mode: %s): %w", destPath, destDir, dirInfo.Mode().String(), err)
		}
	}
	defer dstFile.Close()

	// Copy file contents with optimized buffer
	buffer := make([]byte, s.bufferSize)
	bytesTransferred, err := io.CopyBuffer(dstFile, srcFile, buffer)
	if err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

	// Verify file size
	if bytesTransferred != fileInfo.Size() {
		return fmt.Errorf("file size mismatch: expected %d, transferred %d",
			fileInfo.Size(), bytesTransferred)
	}

	return nil
}

// TransferFile implements Phase 2: Single File Transfer (public interface for backward compatibility)
func (s *SCPTransfer) TransferFile(sourcePath, destPath string) error {
	return s.doTransferFile(sourcePath, destPath)
}

// TransferItemFiles implements Phase 3: Directory-Based File Transfer
// Copy all files in the containing directories (including subtitles) to the destination server
func (s *SCPTransfer) TransferItemFiles(item *types.SyncableItem) error {
	s.logger.WithField("item", s.getItemIdentifier(item)).Info("Phase 3: Starting directory-based file transfer")

	// TODO: Uncomment when plexgo library implements proper Media and Part structures for file paths
	// filePaths, err := s.getItemFilePaths(item)
	// if err != nil {
	//     return fmt.Errorf("failed to get file paths for item: %w", err)
	// }

	// PLACEHOLDER: Generate placeholder file paths until proper implementation
	filePaths := s.generatePlaceholderFilePaths(item)

	if len(filePaths) == 0 {
		s.logger.WithField("item", s.getItemIdentifier(item)).Warn("No file paths found for item")
		return nil
	}

	// Transfer entire directories containing the files
	processedDirs := make(map[string]bool)

	for _, filePath := range filePaths {
		sourceDir := filepath.Dir(filePath)

		// Skip if we've already processed this directory
		if processedDirs[sourceDir] {
			continue
		}
		processedDirs[sourceDir] = true

		destDir := s.calculateDestPath(sourceDir)

		s.logger.WithFields(map[string]interface{}{
			"source_dir": sourceDir,
			"dest_dir":   destDir,
		}).Info("Transferring entire directory (includes subtitles, extras, etc.)")

		// Copy entire directory (includes subtitles, extras, etc.)
		if err := s.CopyDirectory(sourceDir, destDir); err != nil {
			return fmt.Errorf("failed to copy directory %s to %s: %w", sourceDir, destDir, err)
		}
	}

	s.logger.WithField("item", s.getItemIdentifier(item)).Info("Phase 3: Directory-based file transfer complete")
	return nil
}

// doTransferFiles transfers multiple files (internal implementation)
func (s *SCPTransfer) doTransferFiles(files []types.FileTransfer) error {
	for _, file := range files {
		if err := s.doTransferFile(file.SourcePath, file.DestPath); err != nil {
			s.logger.LogError(err, map[string]interface{}{
				"source_path": file.SourcePath,
				"dest_path":   file.DestPath,
			})
			return err
		}
	}
	return nil
}

// TransferFiles transfers multiple files (public interface for backward compatibility)
func (s *SCPTransfer) TransferFiles(files []types.FileTransfer) error {
	return s.doTransferFiles(files)
}

// Internal interface methods (lowercase for internalTransferer interface)
func (s *SCPTransfer) fileExists(path string) (bool, error) {
	return s.FileExists(path)
}

func (s *SCPTransfer) getFileSize(path string) (int64, error) {
	return s.GetFileSize(path)
}

func (s *SCPTransfer) deleteFile(path string) error {
	return s.DeleteFile(path)
}

func (s *SCPTransfer) listDirectoryContents(rootPath string) ([]string, error) {
	return s.ListDirectoryContents(rootPath)
}

func (s *SCPTransfer) mapSourcePathToLocal(sourcePath string) (string, error) {
	return s.MapSourcePathToLocal(sourcePath)
}

func (s *SCPTransfer) mapLocalPathToDest(localPath string) (string, error) {
	return s.MapLocalPathToDest(localPath)
}

func (s *SCPTransfer) close() error {
	return s.Close()
}

// TransferFilesParallel transfers multiple files in parallel for better performance
func (s *SCPTransfer) TransferFilesParallel(files []types.FileTransfer) error {
	if len(files) == 0 {
		return nil
	}

	// Use a semaphore to limit concurrent transfers
	semaphore := make(chan struct{}, s.maxConcurrent)
	errChan := make(chan error, len(files))

	var wg sync.WaitGroup

	for _, file := range files {
		wg.Add(1)
		go func(f types.FileTransfer) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			if err := s.TransferFile(f.SourcePath, f.DestPath); err != nil {
				s.logger.LogError(err, map[string]interface{}{
					"source_path": f.SourcePath,
					"dest_path":   f.DestPath,
				})
				errChan <- err
				return
			}
		}(file)
	}

	// Wait for all transfers to complete
	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		if err != nil {
			return err // Return first error encountered
		}
	}

	return nil
}

// FileExists checks if a file exists on the remote server
func (s *SCPTransfer) FileExists(path string) (bool, error) {
	_, err := s.sftpClient.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// GetFileSize returns the size of a remote file
func (s *SCPTransfer) GetFileSize(path string) (int64, error) {
	stat, err := s.sftpClient.Stat(path)
	if err != nil {
		return 0, err
	}
	return stat.Size(), nil
}

// DeleteFile deletes a file on the remote server
func (s *SCPTransfer) DeleteFile(path string) error {
	return s.sftpClient.Remove(path)
}

// CreateDirectory creates a directory on the remote server
func (s *SCPTransfer) CreateDirectory(path string) error {
	return s.sftpClient.MkdirAll(path)
}

// ListDirectoryContents recursively lists all files in a directory
func (s *SCPTransfer) ListDirectoryContents(rootPath string) ([]string, error) {
	var allFiles []string

	err := s.walkDirectory(rootPath, func(path string, info os.FileInfo) error {
		if !info.IsDir() {
			allFiles = append(allFiles, path)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	s.logger.WithFields(map[string]interface{}{
		"root_path":  rootPath,
		"file_count": len(allFiles),
	}).Debug("Listed directory contents")

	return allFiles, nil
}

// walkDirectory recursively walks a directory tree on the remote server
func (s *SCPTransfer) walkDirectory(path string, walkFunc func(path string, info os.FileInfo) error) error {
	entries, err := s.sftpClient.ReadDir(path)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", path, err)
	}

	for _, entry := range entries {
		entryPath := strings.TrimRight(path, "/") + "/" + entry.Name()

		// Call the walk function for this entry
		if err := walkFunc(entryPath, entry); err != nil {
			return err
		}

		// If it's a directory, recursively walk it
		if entry.IsDir() {
			if err := s.walkDirectory(entryPath, walkFunc); err != nil {
				return err
			}
		}
	}

	return nil
}

// CopyDirectory copies an entire directory from source to destination
func (s *SCPTransfer) CopyDirectory(sourceDir, destDir string) error {
	s.logger.WithFields(map[string]interface{}{
		"source_dir": sourceDir,
		"dest_dir":   destDir,
	}).Info("Starting directory copy")

	// TODO: Implement recursive directory copying
	// This would involve:
	// 1. Walking the source directory tree
	// 2. Creating destination directories
	// 3. Copying all files including subtitles, extras, etc.

	// PLACEHOLDER: Just create the destination directory for now
	if err := s.sftpClient.MkdirAll(destDir); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	s.logger.WithField("dest_dir", destDir).Warn("Directory copy not fully implemented - only created destination directory")
	return nil
}

// calculateDestPath generates the destination path for a source directory
func (s *SCPTransfer) calculateDestPath(sourceDir string) string {
	// TODO: Implement proper path mapping based on configuration
	// This should map source paths to destination paths based on:
	// - DEST_MEDIA_PATH configuration
	// - Library-specific path mappings
	// - Volume mount configurations

	// PLACEHOLDER: Simple path mapping
	baseName := filepath.Base(sourceDir)
	destPath := filepath.Join("/media/sync", baseName)

	s.logger.WithFields(map[string]interface{}{
		"source_dir": sourceDir,
		"dest_path":  destPath,
	}).Debug("Generated destination path (placeholder logic)")

	return destPath
}

// generatePlaceholderFilePaths generates placeholder file paths for testing
func (s *SCPTransfer) generatePlaceholderFilePaths(item *types.SyncableItem) []string {
	// PLACEHOLDER: Generate fake file paths until proper file path extraction is available
	title := item.Title
	if title == "" {
		title = "unknown"
	}

	// Generate placeholder paths
	placeholderPaths := []string{
		fmt.Sprintf("/media/source/%s/%s.mkv", title, title),
		fmt.Sprintf("/media/source/%s/%s.srt", title, title), // subtitle file
	}

	s.logger.WithFields(map[string]interface{}{
		"item_title":        title,
		"placeholder_paths": placeholderPaths,
	}).Debug("Generated placeholder file paths - not real file paths")

	return placeholderPaths
}

// getItemIdentifier returns a string identifier for the item for logging purposes
func (s *SCPTransfer) getItemIdentifier(item *types.SyncableItem) string {
	identifier := item.RatingKey
	if identifier == "" {
		identifier = item.Title
	}
	if identifier == "" {
		identifier = "unknown"
	}

	return identifier
}

// MapSourcePathToLocal converts a source Plex server path to a local filesystem path
// If source replacement is configured, applies the pattern replacement
// Otherwise, uses the Plex path as-is (useful for mounted volumes)
func (s *SCPTransfer) MapSourcePathToLocal(sourcePath string) (string, error) {
	if sourcePath == "" {
		return "", fmt.Errorf("source path is empty")
	}

	// If no source replacement configured, use the Plex path as-is
	if s.sourceReplaceFrom == "" || s.sourceReplaceTo == "" {
		// Convert to local path separators for the current OS
		localPath := filepath.FromSlash(sourcePath)
		return localPath, nil
	}

	// Apply source replacement pattern
	// Normalize paths for comparison (always use forward slashes)
	sourcePathNorm := filepath.ToSlash(sourcePath)
	sourceReplaceFromNorm := filepath.ToSlash(s.sourceReplaceFrom)

	// Check if the source path starts with the replacement pattern
	if !strings.HasPrefix(sourcePathNorm, sourceReplaceFromNorm) {
		return "", fmt.Errorf("source path %s does not start with replacement pattern %s", sourcePath, s.sourceReplaceFrom)
	}

	// Remove the source pattern and replace with local pattern
	relativePath := strings.TrimPrefix(sourcePathNorm, sourceReplaceFromNorm)
	relativePath = strings.TrimPrefix(relativePath, "/") // Remove leading slash

	// Build local path with proper separators for the target OS
	localPath := filepath.Join(s.sourceReplaceTo, relativePath)
	return localPath, nil
}

// MapLocalPathToDest converts a local filesystem path to a destination server path
func (s *SCPTransfer) MapLocalPathToDest(localPath string) (string, error) {
	if localPath == "" {
		return "", fmt.Errorf("local path is empty")
	}

	if s.destRootDir == "" {
		return "", fmt.Errorf("destination root directory not configured")
	}

	// Extract the relative path from local path
	var relativePath string

	if s.sourceReplaceTo != "" {
		// If source replacement is configured, extract relative path from the replacement root
		localPathNorm := filepath.ToSlash(localPath)
		sourceReplaceToNorm := filepath.ToSlash(s.sourceReplaceTo)

		if !strings.HasPrefix(localPathNorm, sourceReplaceToNorm) {
			return "", fmt.Errorf("local path %s does not start with source replacement root %s", localPath, s.sourceReplaceTo)
		}

		relativePath = strings.TrimPrefix(localPathNorm, sourceReplaceToNorm)
		relativePath = strings.TrimPrefix(relativePath, "/") // Remove leading slash
	} else {
		// If no source replacement, extract filename from the full path
		relativePath = filepath.Base(localPath)
	}

	// Build destination path (always use forward slashes for remote paths)
	destPath := strings.TrimSuffix(s.destRootDir, "/") + "/" + relativePath
	return destPath, nil
}
