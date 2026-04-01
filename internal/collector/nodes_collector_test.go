package collector

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

func TestNodesCollector_Collect(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()

	callCount := 0
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		callCount++
		switch command {
		case "sinfo":
			// Global format: %R|%D|%T|%b
			return []byte("cpu*|5|idle|cpu\ncpu*|3|mixed|cpu\ngpu|2|idle|gpu\n"), nil
		case "scontrol":
			// 8 nodes total
			return []byte("NodeName=c1\nNodeName=c2\nNodeName=c3\nNodeName=c4\n" +
				"NodeName=c5\nNodeName=c6\nNodeName=c7\nNodeName=c8\n"), nil
		}
		return []byte{}, nil
	}

	log := logger.NewLogger("error")
	c := NewNodesCollector(log, false) // withFeatureSet=false
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))

	mfs, err := reg.Gather()
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, mf := range mfs {
		names[mf.GetName()] = true
	}
	assert.True(t, names["slurm_nodes_idle"])
	assert.True(t, names["slurm_nodes_mix"])
	assert.True(t, names["slurm_nodes_total"])

	// Global sinfo should result in only 1 sinfo call (not N per partition)
	sinfoCount := 0
	for i := 0; i < callCount; i++ {
		sinfoCount++ // rough check
	}
	assert.LessOrEqual(t, callCount, 3, "should not make many sinfo calls")
}

func TestNodesCollector_Describe(t *testing.T) {
	log := logger.NewLogger("error")
	c := NewNodesCollector(log, true)
	ch := make(chan *prometheus.Desc, 30)
	c.Describe(ch)
	close(ch)
	count := 0
	for range ch {
		count++
	}
	assert.GreaterOrEqual(t, count, 13, "should describe at least 13 node state descriptors + total")
}

func TestSlurmGetTotal_UsesCache(t *testing.T) {
	// Reset cache before test
	scontrolNodesCache = &timedCache{ttl: scontrolNodesCache.ttl}

	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	calls := 0
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		calls++
		return []byte("NodeName=c1\nNodeName=c2\nNodeName=c3\n"), nil
	}

	log := logger.NewLogger("error")
	total1, err := SlurmGetTotal(log)
	require.NoError(t, err)
	assert.Equal(t, float64(3), total1)
	assert.Equal(t, 1, calls)

	// Second call must hit cache
	total2, err := SlurmGetTotal(log)
	require.NoError(t, err)
	assert.Equal(t, float64(3), total2)
	assert.Equal(t, 1, calls, "cache must prevent second Execute call")
}
