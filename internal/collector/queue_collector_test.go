package collector

import (
	"os"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
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
	c := NewQueueCollector(log, true, true)
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
	c := NewQueueCollector(log, true, true)
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
			c := NewQueueCollector(log, withUserLabel, true)
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
	c := NewQueueCollector(log, false, true)
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

// gatheredValue returns the value of the series `name` carrying every label in
// want, and whether it was found at all.
func gatheredValue(mfs []*dto.MetricFamily, name string, want map[string]string) (float64, bool) {
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
	metric:
		for _, m := range mf.GetMetric() {
			got := make(map[string]string, len(m.GetLabel()))
			for _, l := range m.GetLabel() {
				got[l.GetName()] = l.GetValue()
			}
			for k, v := range want {
				if got[k] != v {
					continue metric
				}
			}
			return m.GetGauge().GetValue(), true
		}
	}
	return 0, false
}

// TestQueueCollectorSplitsMultiPartitionJobs is the non-regression test for
// issue #154. squeue reports a job submitted with `sbatch -p debug,high` as
// "debug,high", and the collector used to pass that string through as a
// partition label, naming a partition no cluster has. The two input lines are
// copied from a capture on the scripts/testing cluster.
//
// The job now counts in each partition it is queued in, while
// slurm_jobs_pending stays a count of jobs: alerts.yml fires on
// `slurm_jobs_pending > 500` and `> 1000`, so inflating it by the number of
// partitions a site submits to would move those thresholds.
func TestQueueCollectorSplitsMultiPartitionJobs(t *testing.T) {
	input := []byte("debug,high|PENDING|2|JobHeldUser|alice\n" +
		"debug|PENDING|4|Priority|bob\n")

	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		return input, nil
	}

	log := logger.NewLogger("error")
	c := NewQueueCollector(log, true, true)
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))

	mfs, err := reg.Gather()
	require.NoError(t, err)

	for _, part := range []string{"debug", "high"} {
		v, ok := gatheredValue(mfs, "slurm_queue_pending",
			map[string]string{"partition": part, "user": "alice", "reason": "JobHeldUser"})
		assert.True(t, ok, "alice's job must appear in partition %q", part)
		assert.Equal(t, 1.0, v)

		cores, ok := gatheredValue(mfs, "slurm_cores_pending",
			map[string]string{"partition": part, "user": "alice", "reason": "JobHeldUser"})
		assert.True(t, ok, "alice's cores must appear in partition %q", part)
		assert.Equal(t, 2.0, cores)
	}

	_, ok := gatheredValue(mfs, "slurm_queue_pending", map[string]string{"partition": "debug,high"})
	assert.False(t, ok, "the raw squeue list must never reach a label")

	// Two jobs, one of them queued in two partitions: the cluster-wide count
	// is 2, not 3.
	total, ok := gatheredValue(mfs, "slurm_jobs_pending", nil)
	require.True(t, ok, "slurm_jobs_pending must always be emitted")
	assert.Equal(t, 2.0, total, "a job queued in two partitions is still one job")

	cores, ok := gatheredValue(mfs, "slurm_jobs_cores_pending", nil)
	require.True(t, ok)
	assert.Equal(t, 6.0, cores, "2 cores for alice + 4 for bob, counted once each")
}
