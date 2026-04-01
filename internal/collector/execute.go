package collector

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

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
		if info.Mode()&0o111 == 0 {
			errs = append(errs, fmt.Errorf("binary not executable: %s", full))
			continue
		}
		log.Debug("Binary validated", "path", full)
	}
	return errs
}

// ── Internal performance metrics ─────────────────────────────────────────────

var (
	execMetricsOnce sync.Once

	execDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "slurm_exporter_command_duration_seconds",
			Help:    "Duration of each Slurm CLI command execution in seconds.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"command"},
	)

	execErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "slurm_exporter_command_errors_total",
			Help: "Total number of Slurm CLI command execution errors.",
		},
		[]string{"command"},
	)
)

// RegisterExecMetrics registers the internal Execute() performance metrics
// with the given Prometheus registry. Must be called once at startup.
func RegisterExecMetrics(reg prometheus.Registerer) {
	execMetricsOnce.Do(func() {
		reg.MustRegister(execDuration)
		reg.MustRegister(execErrors)
	})
}

// Execute is a wrapper around exec.CommandContext providing logging, timeout,
// and performance instrumentation (duration histogram + error counter).
// When binPath is set, command is resolved as filepath.Join(binPath, command).
var Execute = func(log *logger.Logger, command string, args []string) ([]byte, error) {
	bin := command
	if binPath != "" {
		bin = filepath.Join(binPath, command)
	}

	log.Debug("Executing command", "command", bin, "args", strings.Join(args, " "))

	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // G204: command is always a controlled Slurm binary, never user input
	out, err := cmd.CombinedOutput()

	elapsed := time.Since(start).Seconds()
	execDuration.WithLabelValues(command).Observe(elapsed)

	if err != nil {
		execErrors.WithLabelValues(command).Inc()
		if ctx.Err() == context.DeadlineExceeded {
			log.Error("Command timed out", "command", bin, "timeout", commandTimeout, "elapsed", elapsed)
			return nil, ctx.Err()
		}
		log.Error("Failed to execute command", "command", bin, "args", strings.Join(args, " "), "output", string(out), "err", err)
		return nil, err
	}

	log.Debug("Command executed successfully", "command", bin, "elapsed_ms", elapsed*1000)
	return out, nil
}
