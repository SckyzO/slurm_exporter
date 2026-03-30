package collector

import (
	"os"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

func TestParseUsersMetrics_Basic(t *testing.T) {
	// Format: JobID|User|State|NumNodes|CPUs|TRES
	// Format: JobID|User|State|NumNodes|CPUs|TRES
	input := `1|alice|RUNNING|1|4|gres/gpu:1
2|alice|RUNNING|1|8|N/A
3|bob|PENDING|1|2|N/A
4|bob|RUNNING|1|4|N/A
5|carol|SUSPENDED|1|2|N/A
6|alice|PENDING|1|2|N/A`

	um := ParseUsersMetrics([]byte(input))

	require.Contains(t, um, "alice")
	require.Contains(t, um, "bob")
	require.Contains(t, um, "carol")

	// alice: 2 running, 1 pending, 12 running CPUs, 1 GPU
	assert.Equal(t, float64(2), um["alice"].running)
	assert.Equal(t, float64(1), um["alice"].pending)
	assert.Equal(t, float64(12), um["alice"].runningCpus)
	assert.Equal(t, float64(1), um["alice"].runningGPUs)

	// bob: 1 running, 1 pending
	assert.Equal(t, float64(1), um["bob"].running)
	assert.Equal(t, float64(1), um["bob"].pending)

	// carol: 1 suspended
	assert.Equal(t, float64(1), um["carol"].suspended)
}

func TestParseUsersMetrics_Empty(t *testing.T) {
	assert.Empty(t, ParseUsersMetrics([]byte("")))
	assert.Empty(t, ParseUsersMetrics([]byte("\n\n")))
}

func TestParseUsersMetrics_IgnoresMalformed(t *testing.T) {
	input := `not-enough
1|alice|RUNNING|1|4|`
	um := ParseUsersMetrics([]byte(input))
	require.Contains(t, um, "alice")
	assert.Equal(t, float64(1), um["alice"].running)
}

func TestParseUsersMetrics_FromTestData(t *testing.T) {
	data, err := os.ReadFile("../../test_data/squeue.txt")
	require.NoError(t, err)
	um := ParseUsersMetrics(data)
	assert.NotEmpty(t, um)
	for user, m := range um {
		assert.NotEmpty(t, user)
		total := m.pending + m.running + m.suspended
		assert.GreaterOrEqual(t, total, float64(0))
	}
}

func TestUsersCollector_Collect(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		return []byte(`1|alice|RUNNING|1|4|N/A
2|bob|PENDING|1|2|N/A
3|alice|RUNNING|1|8|N/A`), nil
	}

	log := logger.NewLogger("error")
	c := NewUsersCollector(log)

	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))

	mfs, err := reg.Gather()
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, mf := range mfs {
		names[mf.GetName()] = true
	}
	assert.True(t, names["slurm_user_jobs_running"])
	assert.True(t, names["slurm_user_cpus_running"])
	assert.True(t, names["slurm_user_jobs_pending"])
}

func TestUsersCollector_Describe(t *testing.T) {
	log := logger.NewLogger("error")
	c := NewUsersCollector(log)
	ch := make(chan *prometheus.Desc, 10)
	c.Describe(ch)
	close(ch)
	count := 0
	for range ch {
		count++
	}
	assert.Equal(t, 5, count)
}

func TestUsersCollector_ErrorHandling(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		return nil, assert.AnError
	}

	log := logger.NewLogger("error")
	c := NewUsersCollector(log)
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))
	mfs, err := reg.Gather()
	assert.NoError(t, err)
	assert.Empty(t, mfs)
}
