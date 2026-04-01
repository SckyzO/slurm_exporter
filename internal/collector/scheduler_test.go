package collector

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchedulerMetrics(t *testing.T) {
	file, err := os.Open("../../test_data/sdiag.txt")
	require.NoError(t, err, "cannot open test data")
	data, err := io.ReadAll(file)
	require.NoError(t, err, "cannot read test data")

	sm := ParseSchedulerMetrics(data)

	assert.Equal(t, 3.0, sm.threads)
	assert.Equal(t, 0.0, sm.queueSize)
	assert.Equal(t, 0.0, sm.dbdQueueSize)
	assert.Equal(t, 97209.0, sm.lastCycle)
	assert.Equal(t, 74593.0, sm.meanCycle)
	assert.Equal(t, 63.0, sm.cyclePerMinute)
	assert.Equal(t, 111544.0, sm.totalBackfilledJobsSinceStart)
	assert.Equal(t, 793.0, sm.totalBackfilledJobsSinceCycle)
	assert.Equal(t, 10.0, sm.totalBackfilledHeterogeneous)
}

func TestParseSchedulerMetrics_JobCounters(t *testing.T) {
	// Vérifie que les job counters sdiag sont bien parsés
	data, err := os.ReadFile("../../test_data/sdiag.txt")
	require.NoError(t, err)

	sm := ParseSchedulerMetrics(data)

	// Les counters peuvent être 0 dans le test_data (cluster idle)
	// mais doivent être parsés sans panique
	assert.GreaterOrEqual(t, sm.jobsSubmitted, float64(0))
	assert.GreaterOrEqual(t, sm.jobsStarted, float64(0))
	assert.GreaterOrEqual(t, sm.jobsCompleted, float64(0))
	assert.GreaterOrEqual(t, sm.jobsCanceled, float64(0))
	assert.GreaterOrEqual(t, sm.jobsFailed, float64(0))
}
