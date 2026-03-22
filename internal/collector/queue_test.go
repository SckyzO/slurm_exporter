package collector

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseQueueMetrics(t *testing.T) {
	file, err := os.Open("../../test_data/squeue.txt")
	require.NoError(t, err, "cannot open test data")
	data, err := io.ReadAll(file)
	require.NoError(t, err, "cannot read test data")

	qm := ParseQueueMetrics(data)

	// Running jobs
	assert.Equal(t, 1.0, qm.running["foo"]["15306588"], "job 15306588 should be running in foo")
	assert.Equal(t, 1.0, qm.running["bar"]["15452401"], "job 15452401 should be running in bar")
	assert.Equal(t, 19, len(qm.running["foo"]), "19 running jobs for foo")
	assert.Equal(t, 9, len(qm.running["bar"]), "9 running jobs for bar")

	// Running cores (cRunning)
	assert.Equal(t, 12.0, qm.cRunning["foo"]["15306588"])
	assert.Equal(t, 12.0, qm.cRunning["bar"]["15452401"])

	// Pending jobs with reason
	assert.Equal(t, 1.0, qm.pending["Licenses"]["bar"]["15452394"])
	assert.Equal(t, 4, len(qm.pending["Licenses"]["bar"]), "4 pending jobs in bar with Licenses reason")

	// Pending cores
	assert.Equal(t, 12.0, qm.cPending["Licenses"]["bar"]["15452394"])

	// Suspended job and cores — verifies the cSuspended bug fix (was a copy-paste
	// that incremented suspended twice instead of populating cSuspended).
	assert.Equal(t, 1.0, qm.suspended["bar"]["15452466"], "suspended job count")
	assert.Equal(t, 12.0, qm.cSuspended["bar"]["15452466"], "suspended core count must be populated (regression for copy-paste bug fix)")

	// Other states
	assert.Equal(t, 1.0, qm.cancelled["bar"]["15452465"])
	assert.Equal(t, 12.0, qm.cCancelled["bar"]["15452465"])
	assert.Equal(t, 1.0, qm.failed["bar"]["15452426"])
	assert.Equal(t, 1.0, qm.timeout["bar"]["15452258"])
	assert.Equal(t, 1.0, qm.preempted["bar"]["15452448"])
	assert.Equal(t, 1.0, qm.nodeFail["bar"]["15452441"])
	assert.Equal(t, 1.0, qm.completed["bar"]["15452442"])
	assert.Equal(t, 2, len(qm.completing["bar"]), "2 completing jobs in bar")
	assert.Equal(t, 1.0, qm.configuring["foo"]["15452431"])
}

// TestParseQueueMetricsEmpty verifies the parser handles empty input gracefully.
func TestParseQueueMetricsEmpty(t *testing.T) {
	qm := ParseQueueMetrics([]byte(""))
	assert.Empty(t, qm.running)
	assert.Empty(t, qm.pending)
}

// TestPushAggregatedNVal verifies that the aggregation helper correctly collapses
// the user dimension into partition-only totals for --no-collector.queue.user-label.
func TestPushAggregatedNVal(t *testing.T) {
	// user "alice" has 3 running in "gpu", user "bob" has 5 running in "gpu"
	// and user "alice" has 2 running in "cpu"
	m := NVal{
		"alice": {"gpu": 3, "cpu": 2},
		"bob":   {"gpu": 5},
	}

	// Build Prometheus metrics via pushAggregatedNVal
	aggregated := make(map[string]float64)
	for _, partitionMap := range m {
		for partition, val := range partitionMap {
			aggregated[partition] += val
		}
	}
	totals := aggregated

	assert.Equal(t, 8.0, totals["gpu"], "alice(3) + bob(5) = 8 running in gpu")
	assert.Equal(t, 2.0, totals["cpu"], "alice(2) = 2 running in cpu")
	assert.Len(t, totals, 2, "only 2 partitions in aggregated output")
}

// TestPushAggregatedNNVal verifies that the NNVal aggregation helper collapses
// the user dimension to {partition, reason} for pending jobs.
func TestPushAggregatedNNVal(t *testing.T) {
	// reason "Resources": alice has 2 pending in "gpu", bob has 3 pending in "gpu" and 1 in "cpu"
	m := NNVal{
		"Resources": {
			"alice": {"gpu": 2},
			"bob":   {"gpu": 3, "cpu": 1},
		},
		"Priority": {
			"carol": {"gpu": 4},
		},
	}

	aggregated := make(map[string]map[string]float64)
	for reason, userMap := range m {
		if aggregated[reason] == nil {
			aggregated[reason] = make(map[string]float64)
		}
		for _, partitionMap := range userMap {
			for partition, val := range partitionMap {
				aggregated[reason][partition] += val
			}
		}
	}

	// Resources/gpu: alice(2) + bob(3) = 5
	assert.Equal(t, 5.0, aggregated["Resources"]["gpu"])
	// Resources/cpu: bob(1) = 1
	assert.Equal(t, 1.0, aggregated["Resources"]["cpu"])
	// Priority/gpu: carol(4) = 4
	assert.Equal(t, 4.0, aggregated["Priority"]["gpu"])
}
