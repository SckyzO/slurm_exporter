package collector

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// TestSlurmInfoCollector_OptionalBinariesSilentlySkipped verifies that the
// optional job-submission binaries (sbatch/salloc/srun) are not emitted as
// slurm_info series when they are absent from the host — and that no error
// is logged. This is the contract that lets monitoring-only containers run
// without the three submission tools installed (issue #24).
func TestSlurmInfoCollector_OptionalBinariesSilentlySkipped(t *testing.T) {
	// Force every optional binary to report "absent".
	oldAvail := binaryAvailable
	defer func() { binaryAvailable = oldAvail }()
	binaryAvailable = func(binary string) bool { return false }

	// Stub Execute so required-binary version probes succeed with a known value
	// (sinfo, squeue, etc.). The optional binaries must never reach this
	// function — binaryAvailable() must short-circuit them.
	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		for _, optional := range []string{"sbatch", "salloc", "srun"} {
			if command == optional {
				t.Fatalf("Execute() called for optional binary %q — binaryAvailable() should have skipped it", command)
			}
		}
		return []byte("slurm 23.11.10"), nil
	}

	log := logger.NewLogger("error")
	c := NewSlurmInfoCollector(log)
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))

	mfs, err := reg.Gather()
	require.NoError(t, err)
	require.Len(t, mfs, 1, "expected one MetricFamily (slurm_info)")
	mf := mfs[0]
	assert.Equal(t, "slurm_info", mf.GetName())

	// Collect (binary, present?) pairs from the emitted series.
	emitted := make(map[string]bool)
	for _, m := range mf.GetMetric() {
		for _, lp := range m.GetLabel() {
			if lp.GetName() == "binary" && lp.GetValue() != "" {
				emitted[lp.GetValue()] = true
			}
		}
	}

	// Required binaries must be emitted.
	for _, req := range []string{"sinfo", "squeue", "sdiag", "scontrol", "sacct"} {
		assert.True(t, emitted[req], "required binary %q must be emitted", req)
	}
	// Optional binaries must NOT appear when absent.
	for _, opt := range []string{"sbatch", "salloc", "srun"} {
		assert.False(t, emitted[opt], "optional binary %q must be skipped when absent", opt)
	}
}

// TestSlurmInfoCollector_OptionalBinariesEmittedWhenAvailable verifies the
// symmetrical case: when an optional binary IS available on the host, its
// version is exposed alongside the required ones.
func TestSlurmInfoCollector_OptionalBinariesEmittedWhenAvailable(t *testing.T) {
	oldAvail := binaryAvailable
	defer func() { binaryAvailable = oldAvail }()
	binaryAvailable = func(binary string) bool { return true } // all present

	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		return []byte("slurm 23.11.10"), nil
	}

	log := logger.NewLogger("error")
	c := NewSlurmInfoCollector(log)
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))

	mfs, err := reg.Gather()
	require.NoError(t, err)
	require.Len(t, mfs, 1)

	emitted := make(map[string]bool)
	for _, m := range mfs[0].GetMetric() {
		for _, lp := range m.GetLabel() {
			if lp.GetName() == "binary" && lp.GetValue() != "" {
				emitted[lp.GetValue()] = true
			}
		}
	}

	for _, opt := range []string{"sbatch", "salloc", "srun"} {
		assert.True(t, emitted[opt], "optional binary %q must appear when available", opt)
	}
}
