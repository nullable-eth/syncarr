// Package transfer provides file transfer implementations for syncarr.
package transfer

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nullable-eth/syncarr/internal/config"
	"github.com/nullable-eth/syncarr/internal/logger"
	"golang.org/x/crypto/ssh"
)

// fileOperations defines the interface for SSH-based file operations
type fileOperations interface {
	GetFileSize(path string) (int64, error)
	DeleteFile(path string) error
	ListDirectoryContents(rootPath string) ([]string, error)
	CreateDirectory(path string) error
	Close() error
}

// sshClient handles all SSH-based file operations with persistent connection
type sshClient struct {
	sshConfig    *config.SSHConfig
	serverConfig *config.PlexServerConfig
	logger       *logger.Logger
	client       *ssh.Client // Persistent SSH connection (reused for multiple sessions)
}

// getSSHClient creates and returns an SSH client connection
func (s *sshClient) getSSHClient() (*ssh.Client, error) {
	if s.client != nil {
		return s.client, nil
	}

	// Create SSH client config
	config := &ssh.ClientConfig{
		User:            s.sshConfig.User,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // For simplicity, ignore host key verification
		Timeout:         30 * time.Second,
	}

	// Add authentication method
	if s.sshConfig.Password != "" {
		config.Auth = []ssh.AuthMethod{
			ssh.Password(s.sshConfig.Password),
		}
	}

	// Determine port
	port := s.sshConfig.Port
	if port == "" {
		port = "22"
	}

	// Connect to SSH server
	addr := fmt.Sprintf("%s:%s", s.serverConfig.Host, port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SSH server: %w", err)
	}

	s.client = client
	return client, nil
}

// executeCommand executes a command using the persistent SSH connection (creates fresh session each time)
func (s *sshClient) executeCommand(cmd string) ([]byte, error) {
	client, err := s.getSSHClient()
	if err != nil {
		return nil, err
	}

	// Create a fresh session for this command (SSH protocol requirement)
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	output, err := session.Output(cmd)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"command": cmd,
			"error":   err.Error(),
		}).Debug("SSH command failed")
		return nil, err
	}

	s.logger.WithField("command", cmd).Debug("SSH command executed successfully via reused connection")
	return output, nil
}

// GetFileSize returns the size of a remote file using persistent connection
func (s *sshClient) GetFileSize(path string) (int64, error) {
	// Properly escape the path for shell execution
	escapedPath := strings.ReplaceAll(path, "'", "'\"'\"'")
	cmd := fmt.Sprintf("stat -f%%z '%s' 2>/dev/null || stat -c%%s '%s'", escapedPath, escapedPath)

	output, err := s.executeCommand(cmd)
	if err != nil {
		return 0, err
	}

	size, err := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64)
	if err != nil {
		return 0, err
	}

	return size, nil
}

// DeleteFile deletes a file on the remote server using persistent connection
func (s *sshClient) DeleteFile(path string) error {
	// Properly escape the path for shell execution
	escapedPath := strings.ReplaceAll(path, "'", "'\"'\"'")
	cmd := fmt.Sprintf("rm -f '%s'", escapedPath)

	_, err := s.executeCommand(cmd)
	return err
}

// ListDirectoryContents recursively lists all files in a directory using persistent connection
func (s *sshClient) ListDirectoryContents(rootPath string) ([]string, error) {
	// Properly escape the path for shell execution
	escapedRootPath := strings.ReplaceAll(rootPath, "'", "'\"'\"'")

	// Try find first (preferred for recursive file listing)
	findCmd := fmt.Sprintf("find '%s' -type f 2>/dev/null", escapedRootPath)

	s.logger.WithFields(map[string]interface{}{
		"root_path": rootPath,
		"command":   findCmd,
	}).Debug("Executing directory listing with find")

	output, err := s.executeCommand(findCmd)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"root_path":  rootPath,
			"find_error": err.Error(),
		}).Debug("Find command failed, falling back to ls")

		// Fallback: try a basic directory test and simpler find
		testCmd := fmt.Sprintf("test -d '%s' && find '%s' -type f || echo \"\"", escapedRootPath, escapedRootPath)

		s.logger.WithFields(map[string]interface{}{
			"root_path": rootPath,
			"command":   testCmd,
		}).Debug("Executing directory listing with simpler fallback")

		output, err = s.executeCommand(testCmd)
		if err != nil {
			s.logger.WithFields(map[string]interface{}{
				"root_path": rootPath,
				"error":     err.Error(),
			}).Warn("Directory listing failed completely, assuming empty directory")
			// Return empty list instead of failing - cleanup can continue
			return []string{}, nil
		}
	}

	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(files) == 1 && files[0] == "" {
		return []string{}, nil
	}

	s.logger.WithFields(map[string]interface{}{
		"root_path":  rootPath,
		"file_count": len(files),
	}).Debug("Listed directory contents via SSH")

	return files, nil
}

// CreateDirectory creates a directory on the remote server using persistent connection
func (s *sshClient) CreateDirectory(path string) error {
	// Properly escape the path for shell execution
	escapedPath := strings.ReplaceAll(path, "'", "'\"'\"'")
	cmd := fmt.Sprintf("mkdir -p '%s'", escapedPath)

	_, err := s.executeCommand(cmd)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"dest_dir": path,
			"error":    err.Error(),
		}).Warn("Failed to create remote directory (may already exist)")
		// Don't return error - directory might already exist
	} else {
		s.logger.WithField("dest_dir", path).Debug("Remote directory created successfully")
	}

	return nil
}

// Close closes the SSH connection
func (s *sshClient) Close() error {
	if s.client != nil {
		err := s.client.Close()
		s.client = nil
		s.logger.Debug("SSH client connection closed successfully")
		return err
	}
	return nil
}
