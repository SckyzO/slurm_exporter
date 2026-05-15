package collector

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAccountsMetrics(t *testing.T) {
	data, err := os.ReadFile("../../test_data/squeue_tres.txt")
	require.NoError(t, err, "cannot open test data")

	am := ParseAccountsMetrics(data)

	// gpu_account: 3 RUNNING jobs spanning typed and untyped GPUs.
	// job 10500: gres/gpu=8 (--gres=gpu:4 -N 2, total alloc 8)
	// job 10501: gres/gpu:a100=2 (typed GPU)
	// job 10502: gres/gpu=16 (--gres=gpu:4 -N 4, total alloc 16)
	require.Contains(t, am, "gpu_account")
	assert.Equal(t, 3.0, am["gpu_account"].running)
	assert.Equal(t, 26.0, am["gpu_account"].runningGPUs, "8 + 2 + 16 = 26 GPUs")
	assert.Equal(t, 312.0, am["gpu_account"].runningCpus, "16 + 8 + 288 = 312 CPUs")

	// Issue #35 regression coverage:
	// account_d submitted with --gpus-per-node=4 — tres-alloc reports the
	// total (gres/gpu=4), whereas the legacy %b field returned N/A and the
	// metric silently fell to 0.
	require.Contains(t, am, "account_d")
	assert.Equal(t, 1.0, am["account_d"].running)
	assert.Equal(t, 4.0, am["account_d"].runningGPUs, "issue #35: --gpus-per-node must be counted")

	// account_e submitted with --gpus=8 across 2 nodes — same bug class:
	// tres-alloc reports 8 (total), no per-node multiplication needed.
	require.Contains(t, am, "account_e")
	assert.Equal(t, 1.0, am["account_e"].running)
	assert.Equal(t, 8.0, am["account_e"].runningGPUs, "issue #35: --gpus must be counted")

	// cpu_account: RUNNING but no GPU in tres-alloc → 0 GPUs.
	require.Contains(t, am, "cpu_account")
	assert.Equal(t, 1.0, am["cpu_account"].running)
	assert.Equal(t, 0.0, am["cpu_account"].runningGPUs)

	// account_b: 3 PENDING + 1 SUSPENDED, no running.
	require.Contains(t, am, "account_b")
	assert.Equal(t, 0.0, am["account_b"].running)
	assert.Equal(t, 1.0, am["account_b"].suspended)
	assert.Equal(t, 3.0, am["account_b"].pending)
}

// TestParseAccountsMetrics_TrimsPadding verifies that squeue -O column padding
// (default minimum width 20 characters) is stripped from labels and numeric
// fields. Without TrimSpace, "account_d           " would leak whitespace into
// the Prometheus label and CPU/GPU parsing would silently return 0.
func TestParseAccountsMetrics_TrimsPadding(t *testing.T) {
	input := []byte("9999                |padded_acct         |RUNNING             |1                   |4                   |cpu=4,mem=8G,node=1,gres/gpu=1")
	am := ParseAccountsMetrics(input)
	require.Contains(t, am, "padded_acct")
	assert.NotContains(t, am, "padded_acct           ", "label must not carry trailing whitespace")
	assert.Equal(t, 4.0, am["padded_acct"].runningCpus)
	assert.Equal(t, 1.0, am["padded_acct"].runningGPUs)
}

// TestParseGPUsFromTRES verifies the TRES GPU parsing helper against all real
// formats observed from squeue tres-alloc output (issue #35) and legacy %b
// output (issue #28 colon-form fallback).
func TestParseGPUsFromTRES(t *testing.T) {
	cases := []struct {
		name     string
		tres     string
		expected float64
	}{
		// tres-alloc format: gres/gpu=N (equals sign separator)
		{"alloc simple", "gres/gpu=4", 4},
		{"alloc typed", "gres/gpu:a100=2", 2},
		{"alloc nvidia_gb200", "gres/gpu:nvidia_gb200=4", 4},
		{"alloc full TRES", "cpu=8,mem=32G,node=2,gres/gpu=8", 8},
		{"alloc no GPU", "cpu=4,mem=8G,node=1", 0},
		// Legacy %b format (still supported for back-compat with any caller
		// that might keep using the old field): gres/gpu:N (colon separator)
		{"legacy simple", "gres/gpu:4", 4},
		{"legacy two GPUs", "gres/gpu:2", 2},
		{"legacy typed GPU", "gres/gpu:a100:2", 2},
		{"legacy nvidia_gb200", "gres/gpu:nvidia_gb200:4", 4},
		{"legacy N/A", "N/A", 0},
		{"empty", "", 0},
		{"legacy full TRES", "billing=10,cpu=8,gres/gpu:4,mem=32G,node=1", 4},
		// PR #28: some Slurm versions emit "gres:gpu:N" (colon prefix) instead
		// of "gres/gpu:N" (slash). Both must be parsed identically.
		{"colon prefix simple", "gres:gpu:4", 4},
		{"colon prefix typed", "gres:gpu:a100:2", 2},
		{"colon prefix nvidia_gb200", "gres:gpu:nvidia_gb200:4", 4},
		{"colon prefix full TRES", "billing=10,cpu=8,gres:gpu:4,mem=32G,node=1", 4},
		{"colon prefix with equals", "cpu=4,mem=8G,node=1,gres:gpu=2", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, parseGPUsFromTRES(tc.tres))
		})
	}
}
