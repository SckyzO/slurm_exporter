package collector

import (
	"io"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

func TestParseQueueMetrics(t *testing.T) {
	file, err := os.Open("../../test_data/squeue.txt")
	require.NoError(t, err, "cannot open test data")
	data, err := io.ReadAll(file)
	require.NoError(t, err, "cannot read test data")

	qm := ParseQueueMetrics(data)

	// Running, keyed [user][partition]. carol holds jobs in two partitions,
	// the shape that matters and that the previous fixture could not express:
	// its first column held job IDs, so every job had a partition of its own.
	assert.Equal(t, 2.0, qm.running["carol"]["cpu"], "carol runs 2 jobs in cpu")
	assert.Equal(t, 3.0, qm.running["carol"]["high"], "carol runs 3 jobs in high")
	assert.Len(t, qm.running["carol"], 2, "carol runs in cpu and high, and nowhere else")
	assert.Equal(t, 16.0, qm.cRunning["carol"]["cpu"])
	assert.Equal(t, 14.0, qm.cRunning["carol"]["high"])

	// Six users share three partitions: no partition belongs to one user.
	assert.Equal(t, 3.0, qm.running["bob"]["cpu"])
	assert.Equal(t, 2.0, qm.running["frank"]["high"])

	// Pending, keyed [reason][user][partition]. Slurm writes reasons as free
	// text: this one carries spaces and a comma, and reaches Prometheus as a
	// label value unchanged.
	const nodesDown = "Nodes required for job are DOWN, DRAINED or reserved for jobs in higher priority partitions"
	assert.Equal(t, 1.0, qm.pending[nodesDown]["frank"]["debug"])
	assert.Equal(t, 4.0, qm.cPending[nodesDown]["frank"]["debug"])
	assert.Equal(t, 2.0, qm.pending["Priority"]["carol"]["debug"])
	assert.Equal(t, 1.0, qm.pending["Resources"]["eve"]["high"])

	// Suspended, with its cores — regression for the copy-paste that
	// incremented suspended twice instead of populating cSuspended.
	assert.Equal(t, 1.0, qm.suspended["bob"]["high"], "suspended job count")
	assert.Equal(t, 4.0, qm.cSuspended["bob"]["high"], "suspended core count must be populated")

	// The other states the test cluster can reach
	assert.Equal(t, 1.0, qm.timeout["alice"]["cpu"])
	assert.Equal(t, 2.0, qm.cTimeout["alice"]["cpu"])
	assert.Equal(t, 1.0, qm.nodeFail["carol"]["cpu"])
	assert.Equal(t, 1.0, qm.failed["carol"]["high"])
	assert.Equal(t, 1.0, qm.completed["dave"]["cpu"])
	assert.Equal(t, 1.0, qm.completed["eve"]["cpu"])
	assert.Equal(t, 3.0, qm.cancelled["alice"]["high"])
	assert.Equal(t, 2.0, qm.cancelled["carol"]["high"])
}

// TestParseQueueMetricsUnreachableStates covers the three states
// scripts/testing cannot produce: PREEMPTED needs PreemptType configured,
// COMPLETING lasts as long as an epilog and CONFIGURING as long as a node
// boots. These lines are written by hand, which the fixture never is — only
// the state string is invented, the field layout is the documented
// squeue -h -o "%P|%T|%C|%r|%u".
func TestParseQueueMetricsUnreachableStates(t *testing.T) {
	input := []byte(
		"cpu|PREEMPTED|4|None|alice\n" +
			"cpu|COMPLETING|2|None|bob\n" +
			"high|COMPLETING|8|None|bob\n" +
			"debug|CONFIGURING|1|None|carol\n")

	qm := ParseQueueMetrics(input)

	assert.Equal(t, 1.0, qm.preempted["alice"]["cpu"])
	assert.Equal(t, 4.0, qm.cPreempted["alice"]["cpu"])
	assert.Equal(t, 1.0, qm.completing["bob"]["cpu"])
	assert.Equal(t, 1.0, qm.completing["bob"]["high"])
	assert.Equal(t, 10.0, sumNVal(qm.cCompleting), "2 + 8 completing cores")
	assert.Equal(t, 1.0, qm.configuring["carol"]["debug"])
}

// TestParseQueueMetricsEmpty verifies the parser handles empty input gracefully.
func TestParseQueueMetricsEmpty(t *testing.T) {
	qm := ParseQueueMetrics([]byte(""))
	assert.Empty(t, qm.running)
	assert.Empty(t, qm.pending)
}

// collectPushed runs a push helper against a buffered channel and returns what
// it emitted, keyed by label set: `a="1",b="2"`. The metric name is fixed by
// the descriptor the caller hands to the helper, so it is not part of the key.
func collectPushed(t *testing.T, push func(chan<- prometheus.Metric)) map[string]float64 {
	t.Helper()
	ch := make(chan prometheus.Metric, 64)
	push(ch)
	close(ch)

	got := make(map[string]float64)
	for m := range ch {
		var pb dto.Metric
		require.NoError(t, m.Write(&pb))

		pairs := make([]string, 0, len(pb.GetLabel()))
		for _, l := range pb.GetLabel() {
			pairs = append(pairs, l.GetName()+`="`+l.GetValue()+`"`)
		}
		sort.Strings(pairs)
		got[strings.Join(pairs, ",")] = pb.GetGauge().GetValue()
	}
	return got
}

// TestPushAggregatedNVal calls the helper that serves
// --no-collector.queue.user-label and checks the series it emits. The previous
// version of this test reimplemented the aggregation in its own body and never
// called the helper at all.
func TestPushAggregatedNVal(t *testing.T) {
	file, err := os.Open("../../test_data/squeue.txt")
	require.NoError(t, err)
	data, err := io.ReadAll(file)
	require.NoError(t, err)
	qm := ParseQueueMetrics(data)

	// The collector built without the user label carries partition-only descs.
	qc := NewQueueCollector(logger.NewLogger("error"), false, true)

	got := collectPushed(t, func(ch chan<- prometheus.Metric) {
		pushAggregatedNVal(qm.running, ch, qc.running, "")
	})

	// Six users collapse into two partitions: cpu 1+3+2+1+3 = 10, high 3+2 = 5.
	assert.Equal(t, map[string]float64{
		`partition="cpu"`:  10,
		`partition="high"`: 5,
	}, got, "slurm_queue_running: the user dimension must be gone, and nothing else with it")
}

// TestPushAggregatedNNVal calls the pending helper, which collapses the user
// dimension to {partition, reason}.
func TestPushAggregatedNNVal(t *testing.T) {
	file, err := os.Open("../../test_data/squeue.txt")
	require.NoError(t, err)
	data, err := io.ReadAll(file)
	require.NoError(t, err)
	qm := ParseQueueMetrics(data)

	qc := NewQueueCollector(logger.NewLogger("error"), false, true)

	got := collectPushed(t, func(ch chan<- prometheus.Metric) {
		pushAggregatedNNVal(qm.pending, ch, qc.pending)
	})

	// Priority/debug gathers alice(1), carol(2), dave(1), frank(1) = 5.
	const nodesDown = "Nodes required for job are DOWN, DRAINED or reserved for jobs in higher priority partitions"
	assert.Equal(t, map[string]float64{
		`partition="debug",reason="` + nodesDown + `"`: 1,
		`partition="debug",reason="Priority"`:          5,
		`partition="high",reason="Priority"`:           1,
		`partition="high",reason="Resources"`:          1,
	}, got, "slurm_queue_pending: reason must survive aggregation, user must not")
}

// TestSumNVal verifies the global NVal aggregation helper.
func TestSumNVal(t *testing.T) {
	m := NVal{
		"alice": {"gpu": 3, "cpu": 2},
		"bob":   {"gpu": 5},
	}
	assert.Equal(t, 10.0, sumNVal(m), "3+2+5=10")
	assert.Equal(t, 0.0, sumNVal(NVal{}), "empty map returns 0")
}

// TestSumNNVal verifies the global NNVal aggregation helper.
func TestSumNNVal(t *testing.T) {
	m := NNVal{
		"Resources": {"alice": {"gpu": 2}, "bob": {"gpu": 3, "cpu": 1}},
		"Priority":  {"carol": {"gpu": 4}},
	}
	assert.Equal(t, 10.0, sumNNVal(m), "2+3+1+4=10")
	assert.Equal(t, 0.0, sumNNVal(NNVal{}), "empty map returns 0")
}

// TestGlobalQueueMetrics verifies the global totals against the fixture.
// These metrics must always be emitted, even when 0.
func TestGlobalQueueMetrics(t *testing.T) {
	file, err := os.Open("../../test_data/squeue.txt")
	require.NoError(t, err, "cannot open test data")
	data, err := io.ReadAll(file)
	require.NoError(t, err)

	qm := ParseQueueMetrics(data)

	assert.Equal(t, 15.0, sumNVal(qm.running), "15 running jobs")
	assert.Equal(t, 8.0, sumNNVal(qm.pending), "8 pending jobs")
	assert.Equal(t, 78.0, sumNVal(qm.cRunning), "78 running cores")
	assert.Equal(t, 38.0, sumNNVal(qm.cPending), "38 pending cores")

	assert.Equal(t, 1.0, sumNVal(qm.suspended))
	assert.Equal(t, 24.0, sumNVal(qm.cancelled))
	assert.Equal(t, 1.0, sumNVal(qm.failed))
	assert.Equal(t, 1.0, sumNVal(qm.timeout))
	assert.Equal(t, 1.0, sumNVal(qm.nodeFail))
	assert.Equal(t, 2.0, sumNVal(qm.completed))

	// The test cluster cannot produce these three; the parser paths that feed
	// them are covered by TestParseQueueMetricsUnreachableStates. Asserting 0
	// here keeps the fixture honest about what it contains.
	assert.Equal(t, 0.0, sumNVal(qm.preempted))
	assert.Equal(t, 0.0, sumNVal(qm.completing))
	assert.Equal(t, 0.0, sumNVal(qm.configuring))
}

// TestGlobalQueueMetricsEmptyCluster verifies that global totals are 0 (not absent)
// when the cluster has no jobs — this is the key behavior difference vs per-user metrics.
func TestGlobalQueueMetricsEmptyCluster(t *testing.T) {
	qm := ParseQueueMetrics([]byte(""))
	assert.Equal(t, 0.0, sumNVal(qm.running), "running must be 0, not absent")
	assert.Equal(t, 0.0, sumNNVal(qm.pending), "pending must be 0, not absent")
	assert.Equal(t, 0.0, sumNVal(qm.cRunning), "cores_running must be 0, not absent")
}
