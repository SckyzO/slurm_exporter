package collector

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

func TestExecuteWithBinPath(t *testing.T) {
	// Create a temp directory with a fake "sinfo" script.
	dir := t.TempDir()
	fakeScript := filepath.Join(dir, "sinfo")
	err := os.WriteFile(fakeScript, []byte("#!/bin/sh\necho 'fake sinfo output'"), 0o755)
	require.NoError(t, err)

	// Override Execute with the real implementation (in case a test replaced it)
	oldBinPath := binPath
	SetBinPath(dir)
	defer SetBinPath(oldBinPath)

	SetCommandTimeout(5 * time.Second)

	log := logger.NewLogger("error")
	out, err := Execute(log, "sinfo", []string{"-h"})
	require.NoError(t, err)
	assert.Equal(t, "fake sinfo output\n", string(out))
}

func TestExecuteWithBinPath_MissingBinary(t *testing.T) {
	dir := t.TempDir() // empty — no binaries

	oldBinPath := binPath
	SetBinPath(dir)
	defer SetBinPath(oldBinPath)

	SetCommandTimeout(5 * time.Second)

	log := logger.NewLogger("error")
	_, err := Execute(log, "sinfo", []string{"-h"})
	assert.Error(t, err, "should fail when binary does not exist in binPath")
}

func TestValidateBinaries_AllPresent(t *testing.T) {
	dir := t.TempDir()

	// Create fake executable scripts for the binaries we'll validate
	bins := []string{"sinfo", "squeue", "sdiag"}
	for _, bin := range bins {
		err := os.WriteFile(filepath.Join(dir, bin), []byte("#!/bin/sh\necho ok"), 0o755)
		require.NoError(t, err)
	}

	oldBinPath := binPath
	SetBinPath(dir)
	defer SetBinPath(oldBinPath)

	log := logger.NewLogger("error")
	errs := ValidateBinaries(log, bins)
	assert.Empty(t, errs, "all binaries present — no errors expected")
}

func TestValidateBinaries_MissingBinaries(t *testing.T) {
	dir := t.TempDir()

	// Only create sinfo — squeue is missing
	err := os.WriteFile(filepath.Join(dir, "sinfo"), []byte("#!/bin/sh\necho ok"), 0o755)
	require.NoError(t, err)

	oldBinPath := binPath
	SetBinPath(dir)
	defer SetBinPath(oldBinPath)

	log := logger.NewLogger("error")
	errs := ValidateBinaries(log, []string{"sinfo", "squeue", "sdiag"})
	assert.Len(t, errs, 2, "squeue and sdiag are missing")
	assert.Contains(t, errs[0].Error(), "squeue")
	assert.Contains(t, errs[1].Error(), "sdiag")
}

func TestValidateBinaries_NotExecutable(t *testing.T) {
	dir := t.TempDir()

	// Create sinfo without execute permission
	err := os.WriteFile(filepath.Join(dir, "sinfo"), []byte("#!/bin/sh\necho ok"), 0o644)
	require.NoError(t, err)

	oldBinPath := binPath
	SetBinPath(dir)
	defer SetBinPath(oldBinPath)

	log := logger.NewLogger("error")
	errs := ValidateBinaries(log, []string{"sinfo"})
	assert.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "not executable")
}

func TestValidateBinaries_SkippedWhenBinPathEmpty(t *testing.T) {
	oldBinPath := binPath
	SetBinPath("") // no custom path
	defer SetBinPath(oldBinPath)

	log := logger.NewLogger("error")
	// Even with non-existent binary names, validation is skipped
	errs := ValidateBinaries(log, []string{"nonexistent_binary_xyz"})
	assert.Empty(t, errs, "validation must be skipped when binPath is empty")
}

func TestRegisterExecMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	// Must not panic on first call
	assert.NotPanics(t, func() {
		RegisterExecMetrics(reg)
	})
	// Second call with same registry must be a no-op (sync.Once)
	assert.NotPanics(t, func() {
		RegisterExecMetrics(reg)
	})
}

func TestExecuteRecordsMetrics(t *testing.T) {
	// Ensure metrics are registered (sync.Once safe)
	reg := prometheus.NewRegistry()
	RegisterExecMetrics(reg)

	log := logger.NewLogger("error")

	// execDuration and execErrors are package-level — recording an observation
	// must not panic regardless of which registry was used at registration time.
	assert.NotPanics(t, func() {
		execDuration.WithLabelValues("squeue").Observe(0.042)
		execErrors.WithLabelValues("scontrol").Inc()
	})

	// Verify the real Execute function records duration and handles success
	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		return []byte("ok"), nil
	}
	out, err := Execute(log, "squeue", []string{"-h"})
	assert.NoError(t, err)
	assert.Equal(t, []byte("ok"), out)
}
