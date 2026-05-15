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

	require.Contains(t, am, "gpu_account")
	assert.Equal(t, 3.0, am["gpu_account"].running)
	assert.Equal(t, 26.0, am["gpu_account"].runningGPUs, "8 + 2 + 16 = 26 GPUs")
	assert.Equal(t, 312.0, am["gpu_account"].runningCpus, "16 + 8 + 288 = 312 CPUs")

	require.Contains(t, am, "account_d")
	assert.Equal(t, 1.0, am["account_d"].running)
	assert.Equal(t, 4.0, am["account_d"].runningGPUs, "--gpus-per-node must be counted")

	require.Contains(t, am, "account_e")
	assert.Equal(t, 1.0, am["account_e"].running)
	assert.Equal(t, 8.0, am["account_e"].runningGPUs, "--gpus must be counted")

	require.Contains(t, am, "cpu_account")
	assert.Equal(t, 1.0, am["cpu_account"].running)
	assert.Equal(t, 0.0, am["cpu_account"].runningGPUs)

	require.Contains(t, am, "account_b")
	assert.Equal(t, 0.0, am["account_b"].running)
	assert.Equal(t, 1.0, am["account_b"].suspended)
	assert.Equal(t, 3.0, am["account_b"].pending)
}

// Without TrimSpace, the label carries trailing whitespace and ParseFloat
// on "4   " returns 0 — both silently.
func TestParseAccountsMetrics_TrimsPadding(t *testing.T) {
	input := []byte("9999                |padded_acct         |RUNNING             |1                   |4                   |cpu=4,mem=8G,node=1,gres/gpu=1")
	am := ParseAccountsMetrics(input)
	require.Contains(t, am, "padded_acct")
	assert.NotContains(t, am, "padded_acct           ", "label must not carry trailing whitespace")
	assert.Equal(t, 4.0, am["padded_acct"].runningCpus)
	assert.Equal(t, 1.0, am["padded_acct"].runningGPUs)
}

func TestParseGPUsFromTRES(t *testing.T) {
	cases := []struct {
		name     string
		tres     string
		expected float64
	}{
		// tres-alloc emits "gres/gpu=N"
		{"alloc simple", "gres/gpu=4", 4},
		{"alloc typed", "gres/gpu:a100=2", 2},
		{"alloc nvidia_gb200", "gres/gpu:nvidia_gb200=4", 4},
		{"alloc full TRES", "cpu=8,mem=32G,node=2,gres/gpu=8", 8},
		{"alloc no GPU", "cpu=4,mem=8G,node=1", 0},
		// Legacy %b emits "gres/gpu:N"
		{"legacy simple", "gres/gpu:4", 4},
		{"legacy two GPUs", "gres/gpu:2", 2},
		{"legacy typed GPU", "gres/gpu:a100:2", 2},
		{"legacy nvidia_gb200", "gres/gpu:nvidia_gb200:4", 4},
		{"legacy N/A", "N/A", 0},
		{"empty", "", 0},
		{"legacy full TRES", "billing=10,cpu=8,gres/gpu:4,mem=32G,node=1", 4},
		// Colon-prefix variant: "gres:gpu:N" instead of "gres/gpu:N"
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
