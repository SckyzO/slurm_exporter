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
