package collector

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// TestGPUsSnapshotSingleCall pins the issue #145 fix: the gpus collector must
// derive total, allocated and idle GPUs from ONE sinfo snapshot, not three.
// A single snapshot makes the alloc/total race structurally impossible, so the
// utilization ratio can never exceed 1 (the unclamped >1.0 the issue reported).
//
// The fixture test_data/sinfo_gpus_snapshot.txt is a real, anonymized capture
// of `sinfo -a -h --Format=Nodes: ,StateLong: ,Gres: ,GresUsed:` from a GPU
// cluster (Slurm 25.05.3). Its 16 GPU nodes carry 4 GPUs each:
//
//	 2 mixed     used 4  -> alloc 8,  idle 0
//	 2 mixed     used 3  -> alloc 6,  idle 2
//	 1 allocated used 4  -> alloc 4,  idle 0
//	11 idle      used 0  -> alloc 0,  idle 44
//
// total = 64, alloc = 18, idle = 46, other = 0, util = 18/64.
func TestGPUsSnapshotSingleCall(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()

	calls := 0
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		calls++
		return os.ReadFile("../../test_data/sinfo_gpus_snapshot.txt")
	}

	m, err := GPUsGetMetrics(logger.NewLogger("debug"))
	require.NoError(t, err)

	assert.Equal(t, 1, calls, "gpus collector must issue exactly one sinfo call (issue #145)")
	assert.Equal(t, 64.0, m.total, "total GPUs")
	assert.Equal(t, 18.0, m.alloc, "allocated GPUs")
	assert.Equal(t, 46.0, m.idle, "idle GPUs")
	assert.Equal(t, 0.0, m.other, "other GPUs")
	assert.InDelta(t, 18.0/64.0, m.utilization, 1e-9, "utilization")
	assert.LessOrEqual(t, m.utilization, 1.0, "utilization must never exceed 1 (issue #145)")
}

// TestIsAvailableGPUState pins the node-state classification that decides whether
// a node's GPUs count as alloc/idle or land in "other", including the flag-suffix
// stripping sinfo applies to StateLong tokens (e.g. "mixed-", "drained*",
// "idle~"). These states match sinfo's own "--states=idle,allocated" selection.
func TestIsAvailableGPUState(t *testing.T) {
	for _, s := range []string{"idle", "allocated", "mixed", "mixed-", "MIXED", "idle~"} {
		assert.True(t, isAvailableGPUState(s), "%q should be available", s)
	}
	for _, s := range []string{"drained", "drained*", "draining", "down", "down*", "reserved", "inval"} {
		assert.False(t, isAvailableGPUState(s), "%q should not be available", s)
	}
}

// TestGPUsOtherStateRoutedToOther exercises the branch the real snapshot cannot:
// a GPU node in a non-available state. Every GPU node in the capture was
// idle/mixed/allocated, so this uses format-faithful lines Slurm emits for a
// drained GPU node (which still advertises its GRES with 0 used) to verify those
// GPUs land in "other" — never in idle — and that "other" stays non-negative.
func TestGPUsOtherStateRoutedToOther(t *testing.T) {
	in := []byte(
		"2 drained* gpu:model_a:4(S:0-3) gpu:model_a:0(IDX:N/A)\n" +
			"3 idle gpu:model_a:4(S:0-3) gpu:model_a:0(IDX:N/A)\n")

	gm := computeGPUsFromSnapshot(in)

	assert.Equal(t, 20.0, gm.total, "total counts every node") // (2+3)*4
	assert.Equal(t, 0.0, gm.alloc, "nothing allocated")
	assert.Equal(t, 12.0, gm.idle, "only the 3 idle nodes are schedulable") // 3*4
	assert.Equal(t, 8.0, gm.other, "drained GPUs are 'other', not idle")    // 2*4
	assert.GreaterOrEqual(t, gm.other, 0.0, "other must never be negative")
}
