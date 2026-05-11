package collector

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// TestGPUsMetrics_ClampNegativeOther is the non-regression test for issue #16 /
// PR #17. Pre-fix, slurm_gpus_other was computed as total - allocated - idle
// across three separate sinfo invocations, and could be transiently negative
// when cluster state changed between the calls. The fix clamps to zero.
//
// We force the negative case here by mocking Execute to return values such
// that allocated + idle > total.
func TestGPUsMetrics_ClampNegativeOther(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()

	// allocated = 4 (one node × 4 GPUs)
	// idle      = 4 (4 idle GPUs from 1 node with 8 total / 4 used)
	// total     = 5 (1 node with 5 GPUs — intentionally lower than alloc+idle
	//                to simulate the race between sinfo calls)
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		spec := args[2]
		switch {
		case strings.Contains(spec, "GresUsed:") && strings.Contains(spec, "Gres:"):
			// IdleGPUsData: "Nodes Gres GresUsed" — idle = total - allocated
			// 1 node, total 8, used 4 → idle = 4
			return []byte("1 gpu:tesla:8 gpu:tesla:4\n"), nil
		case strings.Contains(spec, "GresUsed:"):
			// AllocatedGPUsData: "Nodes GresUsed" — 1 node × 4 allocated
			return []byte("1 gpu:tesla:4\n"), nil
		case strings.Contains(spec, "Gres:"):
			// TotalGPUsData: "Nodes Gres" — 1 node × 5 (intentionally < alloc+idle)
			return []byte("1 gpu:tesla:5\n"), nil
		}
		return nil, nil
	}

	log := logger.NewLogger("debug")
	metrics, err := GPUsGetMetrics(log)
	require.NoError(t, err)

	assert.Equal(t, float64(5), metrics.total)
	assert.Equal(t, float64(4), metrics.alloc)
	assert.Equal(t, float64(4), metrics.idle)
	// Pre-fix: other = 5 - 4 - 4 = -3
	// Post-fix: clamped to 0
	assert.Equal(t, float64(0), metrics.other,
		"slurm_gpus_other must be clamped to 0 when alloc+idle exceeds total")
}
