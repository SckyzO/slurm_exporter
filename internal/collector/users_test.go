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
	// Format: JobID|User|State|NumNodes|NumCPUs|tres-alloc
	// Padding omitted for brevity; TrimSpace coverage lives in a dedicated test.
	input := `1|alice|RUNNING|1|4|cpu=4,mem=8G,node=1,gres/gpu=1
2|alice|RUNNING|1|8|cpu=8,mem=16G,node=1
3|bob|PENDING|1|2|cpu=2,mem=4G,node=1
4|bob|RUNNING|1|4|cpu=4,mem=8G,node=1
5|carol|SUSPENDED|1|2|cpu=2,mem=4G,node=1,gres/gpu=2
6|alice|PENDING|1|2|cpu=2,mem=4G,node=1`

	um := ParseUsersMetrics([]byte(input))

	require.Contains(t, um, "alice")
	require.Contains(t, um, "bob")
	require.Contains(t, um, "carol")

	assert.Equal(t, float64(2), um["alice"].running)
	assert.Equal(t, float64(1), um["alice"].pending)
	assert.Equal(t, float64(12), um["alice"].runningCpus)
	assert.Equal(t, float64(1), um["alice"].runningGPUs)

	assert.Equal(t, float64(1), um["bob"].running)
	assert.Equal(t, float64(1), um["bob"].pending)

	assert.Equal(t, float64(1), um["carol"].suspended)
}

func TestParseUsersMetrics_Empty(t *testing.T) {
	assert.Empty(t, ParseUsersMetrics([]byte("")))
	assert.Empty(t, ParseUsersMetrics([]byte("\n\n")))
}

func TestParseUsersMetrics_IgnoresMalformed(t *testing.T) {
	// Lines that lack the expected 6 pipe-separated fields are skipped.
	// Without the guard, fields[5] (tres-alloc) access would panic.
	input := `not-enough
1|alice|RUNNING|1|4|cpu=4,mem=8G,node=1`
	um := ParseUsersMetrics([]byte(input))
	require.Contains(t, um, "alice")
	assert.Equal(t, float64(1), um["alice"].running)
}

func TestParseUsersMetrics_FromTestData(t *testing.T) {
	data, err := os.ReadFile("../../test_data/squeue_tres_users.txt")
	require.NoError(t, err)
	um := ParseUsersMetrics(data)

	// Issue #35: carol used --gpus-per-node=4, dave used --gpus=8. Both were
	// silently dropped under the legacy %b field — now both must be counted.
	require.Contains(t, um, "carol")
	assert.Equal(t, float64(4), um["carol"].runningGPUs, "issue #35: --gpus-per-node")
	require.Contains(t, um, "dave")
	assert.Equal(t, float64(8), um["dave"].runningGPUs, "issue #35: --gpus")

	// eve: 3 RUNNING jobs mixing untyped, typed, and large-allocation GPUs.
	require.Contains(t, um, "eve")
	assert.Equal(t, float64(3), um["eve"].running)
	assert.Equal(t, float64(26), um["eve"].runningGPUs, "8 + 2 + 16 = 26 GPUs")

	// frank: CPU-only RUNNING job, no GPU.
	require.Contains(t, um, "frank")
	assert.Equal(t, float64(1), um["frank"].running)
	assert.Equal(t, float64(0), um["frank"].runningGPUs)
}

func TestUsersCollector_Collect(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		return []byte(`1|alice|RUNNING|1|4|cpu=4,mem=8G,node=1
2|bob|PENDING|1|2|cpu=2,mem=4G,node=1
3|alice|RUNNING|1|8|cpu=8,mem=16G,node=1,gres/gpu=2`), nil
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
	assert.True(t, names["slurm_user_gpus_running"], "GPU metric must be emitted when tres-alloc shows GPUs")
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
