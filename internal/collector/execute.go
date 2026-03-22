package collector

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

var (
	commandTimeout time.Duration
	binPath        string
)

// SetCommandTimeout sets the timeout for external commands.
func SetCommandTimeout(t time.Duration) {
	commandTimeout = t
}

// SetBinPath sets the directory in which Slurm binaries are looked up.
// An empty string (default) means the binaries must be on the system $PATH.
// When set, every Slurm command is resolved as filepath.Join(binPath, command).
func SetBinPath(p string) {
	binPath = p
}

// SlurmBinaries is the list of Slurm CLI tools used by the exporter.
// Used by ValidateBinaries to check that all required tools are present at startup.
var SlurmBinaries = []string{
	"sinfo", "squeue", "sdiag", "scontrol", "sshare",
	"sacct", "sbatch", "salloc", "srun",
}

// ValidateBinaries checks that every binary in the given list is accessible
// at the configured binPath. Returns one error per missing or non-executable
// binary. When binPath is empty the check is skipped (system $PATH is trusted).
func ValidateBinaries(log *logger.Logger, binaries []string) []error {
	if binPath == "" {
		return nil
	}
	var errs []error
	for _, bin := range binaries {
		full := filepath.Join(binPath, bin)
		info, err := os.Stat(full)
		if err != nil {
			errs = append(errs, fmt.Errorf("binary not found: %s", full))
			continue
		}
		// Check execute bit (owner, group, or other)
		if info.Mode()&0o111 == 0 {
			errs = append(errs, fmt.Errorf("binary not executable: %s", full))
			continue
		}
		log.Debug("Binary validated", "path", full)
	}
	return errs
}

// Execute is a wrapper around exec.CommandContext providing logging and timeout.
// When binPath is set, command is resolved as filepath.Join(binPath, command).
var Execute = func(logger *logger.Logger, command string, args []string) ([]byte, error) {
	bin := command
	if binPath != "" {
		bin = filepath.Join(binPath, command)
	}

	logger.Debug("Executing command", "command", bin, "args", strings.Join(args, " "))

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // G204: command is always a controlled Slurm binary, never user input
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			logger.Error("Command timed out", "command", bin, "timeout", commandTimeout)
			return nil, ctx.Err()
		}
		logger.Error("Failed to execute command", "command", bin, "args", strings.Join(args, " "), "output", string(out), "err", err)
		return nil, err
	}

	logger.Debug("Command executed successfully", "command", bin)
	return out, nil
}
