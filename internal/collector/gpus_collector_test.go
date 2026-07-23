package collector

import (
	"os"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

func TestGPUsCollector_Collect(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		return os.ReadFile("../../test_data/sinfo_gpus_snapshot.txt")
	}

	log := logger.NewLogger("error")
	c := NewGPUsCollector(log)
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))

	mfs, err := reg.Gather()
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, mf := range mfs {
		names[mf.GetName()] = true
	}
	assert.True(t, names["slurm_gpus_total"])
	assert.True(t, names["slurm_gpus_alloc"])
	assert.True(t, names["slurm_gpus_idle"])
}

func TestGPUsCollector_Describe(t *testing.T) {
	log := logger.NewLogger("error")
	c := NewGPUsCollector(log)
	ch := make(chan *prometheus.Desc, 10)
	c.Describe(ch)
	close(ch)
	count := 0
	for range ch {
		count++
	}
	assert.Equal(t, 5, count)
}
