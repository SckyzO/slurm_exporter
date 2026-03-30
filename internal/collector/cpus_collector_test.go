package collector

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

func TestCPUsCollector_Collect(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		return []byte("100/200/50/350\n"), nil
	}

	log := logger.NewLogger("error")
	c := NewCPUsCollector(log)

	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))

	mfs, err := reg.Gather()
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, mf := range mfs {
		names[mf.GetName()] = true
	}
	assert.True(t, names["slurm_cpus_alloc"])
	assert.True(t, names["slurm_cpus_idle"])
	assert.True(t, names["slurm_cpus_other"])
	assert.True(t, names["slurm_cpus_total"])
}

func TestCPUsCollector_Describe(t *testing.T) {
	log := logger.NewLogger("error")
	c := NewCPUsCollector(log)
	ch := make(chan *prometheus.Desc, 10)
	c.Describe(ch)
	close(ch)
	count := 0
	for range ch {
		count++
	}
	assert.Equal(t, 4, count)
}
