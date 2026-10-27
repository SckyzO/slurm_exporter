package collector

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

func TestParseDrainReasonMetrics_Basic(t *testing.T) {
	// Format: %N|%E|%H|%T
	input := `c1|disk-slow maintenance|2026-04-01T10:00:00|drained
c2|hardware failure|2026-04-01T09:00:00|down
c3|none|Unknown|idle
c4||Unknown|idle
c5|not responding|2026-04-01T11:00:00|down
`
	metrics := ParseDrainReasonMetrics([]byte(input))

	// c1 drained with reason → included
	// c2 down with reason → included
	// c3 reason="none" → excluded
	// c4 empty reason → excluded
	// c5 "not responding" → excluded (built-in Slurm reason, not admin-set)
	require.Len(t, metrics, 2)

	nodes := make(map[string]DrainReasonMetrics)
	for _, m := range metrics {
		nodes[m.Node] = m
	}

	assert.Equal(t, "disk-slow maintenance", nodes["c1"].Reason)
	assert.Equal(t, "2026-04-01T10:00:00", nodes["c1"].Since)
	assert.Equal(t, "hardware failure", nodes["c2"].Reason)
}

func TestParseDrainReasonMetrics_Deduplication(t *testing.T) {
	// Same node in multiple partitions → only one entry
	input := `c1|maintenance|2026-04-01T10:00:00|drained
c1|maintenance|2026-04-01T10:00:00|drained
`
	metrics := ParseDrainReasonMetrics([]byte(input))
	assert.Len(t, metrics, 1)
}

func TestParseDrainReasonMetrics_Empty(t *testing.T) {
	assert.Empty(t, ParseDrainReasonMetrics([]byte("")))
	assert.Empty(t, ParseDrainReasonMetrics([]byte("\n\n")))
}

func TestParseDrainReasonMetrics_OnlyHealthyNodes(t *testing.T) {
	input := `c1|none|Unknown|idle
c2||Unknown|mixed
`
	assert.Empty(t, ParseDrainReasonMetrics([]byte(input)))
}

func TestDrainReasonCollector_Collect(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	resetNodeCaches()

	var calls []string
	Execute = recordingExecute(
		`NodeName=c1 CPUAlloc=0 CPUTot=8 RealMemory=1000 AllocMem=0 State=IDLE+DRAIN Reason=audit-drain [alice@2026-04-01T10:00:00] Partitions=cpu
NodeName=c2 CPUAlloc=0 CPUTot=8 RealMemory=1000 AllocMem=0 State=DOWN+DRAIN Reason=hw-fail [root@2026-04-01T09:00:00] ReservationName=maint Partitions=cpu
NodeName=c3 CPUAlloc=0 CPUTot=8 RealMemory=1000 AllocMem=0 State=IDLE Partitions=cpu`,
		"c1|drained\nc2|down\nc3|idle\n",
		&calls,
	)

	log := logger.NewLogger("error")
	c := NewDrainReasonCollector(log)
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))

	mfs, err := reg.Gather()
	require.NoError(t, err)
	require.Len(t, mfs, 1)
	assert.Equal(t, "slurm_node_drain_reason_info", mfs[0].GetName())
	require.Len(t, mfs[0].Metric, 2, "c3 is healthy and must not appear")

	labels := make(map[string]map[string]string)
	for _, m := range mfs[0].Metric {
		got := make(map[string]string)
		for _, l := range m.GetLabel() {
			got[l.GetName()] = l.GetValue()
		}
		labels[got["node"]] = got
	}
	assert.Equal(t, "audit-drain", labels["c1"]["reason"])
	assert.Equal(t, "2026-04-01T10:00:00", labels["c1"]["since"])
	// c2 carries a ReservationName= after its reason, the field that broke
	// the first parser.
	assert.Equal(t, "hw-fail", labels["c2"]["reason"])
	assert.Equal(t, "2026-04-01T09:00:00", labels["c2"]["since"])
}

func TestDrainReasonCollector_EmptyCluster(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	resetNodeCaches()

	var calls []string
	Execute = recordingExecute(
		`NodeName=c1 CPUAlloc=0 CPUTot=8 RealMemory=1000 AllocMem=0 State=IDLE Partitions=cpu`,
		"c1|idle\n",
		&calls,
	)

	log := logger.NewLogger("error")
	c := NewDrainReasonCollector(log)
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))

	mfs, err := reg.Gather()
	assert.NoError(t, err)
	assert.Empty(t, mfs, "no metrics when all nodes are healthy")
}

func TestDrainReasonCollector_ErrorHandling(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	// Without this the collector would be served a previous test's output
	// from the shared caches and the error path would never be reached.
	resetNodeCaches()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		return nil, assert.AnError
	}

	log := logger.NewLogger("error")
	c := NewDrainReasonCollector(log)
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))

	mfs, err := reg.Gather()
	assert.NoError(t, err)
	assert.Empty(t, mfs)
}
