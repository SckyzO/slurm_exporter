package collector

import (
	"os"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

func TestSchedulerCollector_Collect(t *testing.T) {
	data, err := os.ReadFile("../../test_data/sdiag.txt")
	require.NoError(t, err)

	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		return data, nil
	}

	log := logger.NewLogger("error")
	c := NewSchedulerCollector(log)
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))

	mfs, err := reg.Gather()
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, mf := range mfs {
		names[mf.GetName()] = true
	}
	assert.True(t, names["slurm_scheduler_threads"])
	assert.True(t, names["slurm_scheduler_queue_size"])
	assert.True(t, names["slurm_scheduler_last_cycle"])
	assert.True(t, names["slurm_scheduler_backfill_last_cycle"])
	assert.True(t, names["slurm_scheduler_jobs_submitted_total"])
	assert.True(t, names["slurm_scheduler_jobs_completed_total"])
}

func TestSchedulerCollector_Describe(t *testing.T) {
	log := logger.NewLogger("error")
	c := NewSchedulerCollector(log)
	ch := make(chan *prometheus.Desc, 30)
	c.Describe(ch)
	close(ch)
	count := 0
	for range ch {
		count++
	}
	assert.GreaterOrEqual(t, count, 18)
}

func TestSchedulerCollector_ErrorHandling(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		return nil, assert.AnError
	}

	log := logger.NewLogger("error")
	c := NewSchedulerCollector(log)
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))
	mfs, err := reg.Gather()
	assert.NoError(t, err)
	assert.Empty(t, mfs)
}
