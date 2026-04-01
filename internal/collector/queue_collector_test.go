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
