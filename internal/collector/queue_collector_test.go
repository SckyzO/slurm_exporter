package collector

import (
	"os"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

func TestQueueCollector_Collect(t *testing.T) {
	data, err := os.ReadFile("../../test_data/squeue.txt")
	require.NoError(t, err)

	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		return data, nil
	}

	log := logger.NewLogger("error")
	c := NewQueueCollector(log, true)
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))

	mfs, err := reg.Gather()
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, mf := range mfs {
		names[mf.GetName()] = true
	}
	// Global totals must always be present
	assert.True(t, names["slurm_jobs_running"])
	assert.True(t, names["slurm_jobs_pending"])
	assert.True(t, names["slurm_jobs_cores_running"])
}

func TestQueueCollector_Describe(t *testing.T) {
	log := logger.NewLogger("error")
	c := NewQueueCollector(log, true)
	ch := make(chan *prometheus.Desc, 50)
	c.Describe(ch)
	close(ch)
	count := 0
	for range ch {
		count++
	}
	assert.GreaterOrEqual(t, count, 20)
}

// TestQueueCollector_EmitsSuspendedMetrics is a non-regression test for the
// silent data loss fixed by PR #13 / issue #12: `slurm_queue_suspended` and
// `slurm_cores_suspended` were declared and counted but never pushed to
// Prometheus in Collect(). The fixture contains at least one SUSPENDED job.
func TestQueueCollector_EmitsSuspendedMetrics(t *testing.T) {
	data, err := os.ReadFile("../../test_data/squeue.txt")
	require.NoError(t, err)

	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		return data, nil
	}

	for _, withUserLabel := range []bool{true, false} {
		t.Run(fmtBool("withUserLabel", withUserLabel), func(t *testing.T) {
			log := logger.NewLogger("error")
			c := NewQueueCollector(log, withUserLabel)
			reg := prometheus.NewRegistry()
			require.NoError(t, reg.Register(c))

			mfs, err := reg.Gather()
			require.NoError(t, err)

			names := make(map[string]bool)
			for _, mf := range mfs {
				names[mf.GetName()] = true
			}
			assert.True(t, names["slurm_queue_suspended"],
				"slurm_queue_suspended must be emitted when SUSPENDED jobs are present")
			assert.True(t, names["slurm_cores_suspended"],
				"slurm_cores_suspended must be emitted when SUSPENDED jobs are present")
		})
	}
}

func fmtBool(label string, v bool) string {
	if v {
		return label + "=true"
	}
	return label + "=false"
}

// TestParseQueueMetrics_StripsPartitionAsterisk is the defensive companion
// to issue #20 / PR #21: squeue -o "%P" emits "compute*" for the default
// partition on some Slurm versions, and the queue collector previously
// stored this raw value as the partition label. Now stripped, mirroring
// what partitions.go and nodes.go do.
func TestParseQueueMetrics_StripsPartitionAsterisk(t *testing.T) {
	// One RUNNING job (12 cores) on the default partition "compute*"
	input := []byte("compute*|RUNNING|12||alice\n")
	qm := ParseQueueMetrics(input)

	// Per-user state map for alice should be keyed by bare "compute"
	require.Contains(t, qm.running, "alice")
	require.Contains(t, qm.running["alice"], "compute",
		"asterisk must be stripped from queue partition label")
	assert.NotContains(t, qm.running["alice"], "compute*",
		"raw asterisk-suffixed partition key must not appear")
	assert.Equal(t, 1.0, qm.running["alice"]["compute"])
	assert.Equal(t, 12.0, qm.cRunning["alice"]["compute"])
}

func TestQueueCollector_ErrorEmitsGlobalTotals(t *testing.T) {
	// Even on error, global job totals must be emitted (always-present guarantee)
	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		return nil, assert.AnError
	}

	log := logger.NewLogger("error")
	c := NewQueueCollector(log, false)
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))

	mfs, err := reg.Gather()
	assert.NoError(t, err)

	names := make(map[string]bool)
	for _, mf := range mfs {
		names[mf.GetName()] = true
	}
	assert.True(t, names["slurm_jobs_running"], "global totals must be emitted even on error")
	assert.True(t, names["slurm_jobs_pending"])
}
