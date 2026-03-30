package collector

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

func TestAccountsCollector_Collect(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		return []byte(`1|hpc_team|RUNNING|1|4|N/A
2|hpc_team|RUNNING|1|8|N/A
3|ml_group|PENDING|1|2|N/A
4|ml_group|RUNNING|1|4|gres/gpu:1`), nil
	}

	log := logger.NewLogger("error")
	c := NewAccountsCollector(log)

	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))

	mfs, err := reg.Gather()
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, mf := range mfs {
		names[mf.GetName()] = true
	}
	assert.True(t, names["slurm_account_jobs_running"])
	assert.True(t, names["slurm_account_jobs_pending"])
	assert.True(t, names["slurm_account_cpus_running"])
}

func TestAccountsCollector_Describe(t *testing.T) {
	log := logger.NewLogger("error")
	c := NewAccountsCollector(log)
	ch := make(chan *prometheus.Desc, 10)
	c.Describe(ch)
	close(ch)
	count := 0
	for range ch {
		count++
	}
	assert.Equal(t, 5, count) // pending, running, cpus, gpus, suspended
}

func TestAccountsCollector_ErrorHandling(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		return nil, assert.AnError
	}

	log := logger.NewLogger("error")
	c := NewAccountsCollector(log)
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))
	mfs, err := reg.Gather()
	assert.NoError(t, err)
	assert.Empty(t, mfs)
}
