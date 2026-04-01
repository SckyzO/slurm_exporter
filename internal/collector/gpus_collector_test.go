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
		data, err := os.ReadFile("../../test_data/slurm-25.11.1-1/sinfo_gpus_idle.txt")
		if err != nil {
			return []byte("1 gres/gpu:a100:4 gres/gpu:a100:0(IDX:0,1,2,3)\n"), nil
		}
		return data, nil
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
