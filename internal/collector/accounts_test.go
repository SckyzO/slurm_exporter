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

	// gpu_account: 3 RUNNING jobs with GPU
	// job 10500: 2 nodes × gres/gpu:4 = 8 GPUs
	// job 10501: 1 node  × gres/gpu:2 = 2 GPUs
	// job 10502: 4 nodes × gres/gpu:4 = 16 GPUs
	require.Contains(t, am, "gpu_account")
	assert.Equal(t, 3.0, am["gpu_account"].running)
	assert.Equal(t, 26.0, am["gpu_account"].runningGPUs, "2×4 + 1×2 + 4×4 = 26 GPUs")

	// account_d and account_e: RUNNING but N/A TRES → 0 GPUs
	require.Contains(t, am, "account_d")
	assert.Equal(t, 1.0, am["account_d"].running)
	assert.Equal(t, 0.0, am["account_d"].runningGPUs)

	require.Contains(t, am, "account_e")
	assert.Equal(t, 1.0, am["account_e"].running)
	assert.Equal(t, 0.0, am["account_e"].runningGPUs)

	// account_b: only PENDING and SUSPENDED jobs, no running
	require.Contains(t, am, "account_b")
	assert.Equal(t, 0.0, am["account_b"].running)
	assert.Equal(t, 1.0, am["account_b"].suspended)
}

// TestParseGPUsFromTRES verifies the TRES GPU parsing helper
// against all real formats observed from squeue %b output.
func TestParseGPUsFromTRES(t *testing.T) {
	cases := []struct {
		name     string
		tres     string
		expected float64
	}{
		{"simple", "gres/gpu:4", 4},
		{"two GPUs", "gres/gpu:2", 2},
		{"typed GPU", "gres/gpu:a100:2", 2},
		{"nvidia_gb200", "gres/gpu:nvidia_gb200:4", 4},
		{"no GPU", "N/A", 0},
		{"empty", "", 0},
		{"full TRES string", "billing=10,cpu=8,gres/gpu:4,mem=32G,node=1", 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, parseGPUsFromTRES(tc.tres))
		})
	}
}
