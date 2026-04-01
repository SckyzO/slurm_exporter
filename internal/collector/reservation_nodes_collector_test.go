package collector

import (
	"os"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

func TestReservationNodesCollector_Collect(t *testing.T) {
	// Reset shared cache
	scontrolNodesCache = &timedCache{ttl: scontrolNodesCache.ttl}

	data, err := os.ReadFile("../../test_data/scontrol_nodes_reservation.txt")
	require.NoError(t, err)

	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		return data, nil
	}

	log := logger.NewLogger("error")
	c := NewReservationNodesCollector(log)
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))

	mfs, err := reg.Gather()
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, mf := range mfs {
		names[mf.GetName()] = true
	}
	assert.True(t, names["slurm_reservation_nodes_alloc"] ||
		names["slurm_reservation_nodes_idle"] ||
		names["slurm_reservation_nodes_healthy"])
}

func TestReservationNodesCollector_Describe(t *testing.T) {
	log := logger.NewLogger("error")
	c := NewReservationNodesCollector(log)
	ch := make(chan *prometheus.Desc, 15)
	c.Describe(ch)
	close(ch)
	count := 0
	for range ch {
		count++
	}
	assert.Equal(t, 8, count)
}
