package transfer

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/nullable-eth/syncarr/internal/config"
	"github.com/nullable-eth/syncarr/internal/logger"
	"github.com/nullable-eth/syncarr/pkg/types"
)

// SCPTransfer handles file transfers using actual SCP commands over SSH
type SCPTransfer struct {
	sshConfig         *config.SSHConfig
	serverConfig      *config.PlexServerConfig
	sourceReplaceFrom string
	sourceReplaceTo   string
	destRootDir       string
	logger            *logger.Logger
}

// newSCPTransfer creates a new SCP transfer instance (package-private)
func newSCPTransfer(cfg *config.Config, log *logger.Logger) (*SCPTransfer, error) {
	return &SCPTransfer{
		sshConfig:         &cfg.SSH,
		serverConfig:      &cfg.Destination,
		sourceReplaceFrom: cfg.SourceReplaceFrom,
		sourceReplaceTo:   cfg.SourceReplaceTo,
		destRootDir:       cfg.DestRootDir,
		logger:            log,
	}, nil
}

// doTransferFile transfers a single file using actual SCP command
func (s *SCPTransfer) doTransferFile(sourcePath, destPath string) error {
	// Directory creation is now handled by the common transferrer before calling this method

	// Build SCP command
	args := s.buildSCPArgs(sourcePath, destPath)

	var cmd *exec.Cmd
	if s.sshConfig.Password != "" {
		// Use sshpass for password authentication
		sshpassArgs := []string{"-p", s.sshConfig.Password, "scp"}
		sshpassArgs = append(sshpassArgs, args...)
		cmd = exec.Command("sshpass", sshpassArgs...)
		s.logger.Debug("Using sshpass for SCP password authentication")
	} else {
		// Use regular SCP (key-based auth)
		cmd = exec.Command("scp", args...)
	}

	// Capture output for debugging
	output, err := cmd.CombinedOutput()
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"source_path": sourcePath,
			"dest_path":   destPath,
			"scp_args":    strings.Join(args, " "),
			"output":      string(output),
		}).Error("SCP command failed")
		return fmt.Errorf("scp failed: %w", err)
	}

	return nil
}

// Note: escapeShellPath removed - not needed for exec.Command as Go handles argument separation

// buildSCPArgs builds the SCP command arguments
func (s *SCPTransfer) buildSCPArgs(sourcePath, destPath string) []string {
	remoteHost := fmt.Sprintf("%s@%s", s.sshConfig.User, s.serverConfig.Host)

	// For exec.Command, we don't need shell quoting - Go handles argument separation
	// The remote destination still needs proper formatting for the remote shell
	remoteDest := fmt.Sprintf("%s:%s", remoteHost, destPath)

	args := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=30",
		"-C", // Enable compression
	}

	// Add port if specified
	if s.sshConfig.Port != "" && s.sshConfig.Port != "22" {
		args = append(args, "-P", s.sshConfig.Port)
	}

	// Add source path and remote destination - no quotes needed for exec.Command
	args = append(args, sourcePath, remoteDest)
	return args
}

// doTransferFiles transfers multiple files using SCP
func (s *SCPTransfer) doTransferFiles(files []types.FileTransfer) error {
	// SCP can handle multiple files in one command, but for simplicity and error handling,
	// we'll transfer them individually
	for _, file := range files {
		if err := s.doTransferFile(file.SourcePath, file.DestPath); err != nil {
			return err
		}
	}
	return nil
}
