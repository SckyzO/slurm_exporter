package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// echoEnvBinary installs a fake Slurm binary that prints one environment
// variable, so a test can read the environment the child process was actually
// given instead of the one the exporter holds. binPath and commandTimeout are
// package-level state, so both are restored when the test ends.
func echoEnvBinary(t *testing.T, name, variable string) {
	t.Helper()

	dir := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s' \"$%s\"\n", variable)
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755))

	oldBinPath, oldTimeout := binPath, commandTimeout
	SetBinPath(dir)
	SetCommandTimeout(5 * time.Second)
	t.Cleanup(func() {
		SetBinPath(oldBinPath)
		SetCommandTimeout(oldTimeout)
	})
}

// TestExecutePinsSlurmTimeFormat is the non-regression test for the second half
// of issue #158.
//
// Every Slurm command renders its timestamps according to SLURM_TIME_FORMAT.
// Execute inherited whatever the exporter's own environment held, so a value set
// for interactive use, in /etc/profile.d or in a systemd unit, reached sinfo,
// squeue, scontrol, sdiag, sshare and sacct alike and made their timestamps
// unreadable to the parsers. Pinning the format is what makes the collectors'
// layout a fact rather than a hope.
func TestExecutePinsSlurmTimeFormat(t *testing.T) {
	t.Setenv("SLURM_TIME_FORMAT", "relative")
	echoEnvBinary(t, "scontrol", "SLURM_TIME_FORMAT")

	out, err := Execute(logger.NewLogger("error"), "scontrol", []string{"show", "reservation"})
	require.NoError(t, err)
	assert.Equal(t, "standard", string(out),
		"the exporter's own environment must not decide how Slurm renders timestamps")
}

// TestExecutePinsSlurmTimeFormatWhenUnset pins the format even where nothing set
// it, so the child process does not depend on Slurm keeping "standard" as its
// implicit default.
func TestExecutePinsSlurmTimeFormatWhenUnset(t *testing.T) {
	// t.Setenv records the original value and restores it at the end of the
	// test; unsetting afterwards is what puts the process in the "never set"
	// state without leaking it into the rest of the package.
	t.Setenv("SLURM_TIME_FORMAT", "")
	require.NoError(t, os.Unsetenv("SLURM_TIME_FORMAT"))
	echoEnvBinary(t, "sinfo", "SLURM_TIME_FORMAT")

	out, err := Execute(logger.NewLogger("error"), "sinfo", []string{"-h"})
	require.NoError(t, err)
	assert.Equal(t, "standard", string(out))
}

// TestExecuteKeepsTheRestOfTheEnvironment guards the other direction. Slurm
// commands need the environment they were started with, SLURM_CONF above all:
// replacing it rather than extending it would point every command at the wrong
// cluster, or at no cluster at all.
func TestExecuteKeepsTheRestOfTheEnvironment(t *testing.T) {
	t.Setenv("SLURM_CONF", "/etc/slurm/slurm.conf")
	echoEnvBinary(t, "squeue", "SLURM_CONF")

	out, err := Execute(logger.NewLogger("error"), "squeue", []string{"-h"})
	require.NoError(t, err)
	assert.Equal(t, "/etc/slurm/slurm.conf", string(out))
}
