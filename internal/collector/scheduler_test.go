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
	assert.Equal(t, 0.0, sm.queue_size)
	assert.Equal(t, 0.0, sm.dbd_queue_size)
	assert.Equal(t, 97209.0, sm.last_cycle)
	assert.Equal(t, 74593.0, sm.mean_cycle)
	assert.Equal(t, 63.0, sm.cycle_per_minute)
	assert.Equal(t, 111544.0, sm.total_backfilled_jobs_since_start)
	assert.Equal(t, 793.0, sm.total_backfilled_jobs_since_cycle)
	assert.Equal(t, 10.0, sm.total_backfilled_heterogeneous)
}
