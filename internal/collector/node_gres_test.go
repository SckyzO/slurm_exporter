package collector

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/v2/internal/logger"
)

// newTestLogger returns the WARN-level logger the other collector tests use.
func newTestLogger() *logger.Logger { l, _ := bufferLogger(); return l }

const nodeGRESFixture = "../../test_data/slurm-25.05/node_detail_gres.txt"

// TestNodeGRESMetrics reads the per-node GRES capture and checks the series the
// collector publishes, rather than the intermediate map, so a mistake in the
// label order or in the partition fan-out shows up here.
func TestNodeGRESMetrics(t *testing.T) {
	data, err := os.ReadFile(nodeGRESFixture)
	require.NoError(t, err)
	stubExecute(t, string(data))

	c := NewNodeCollector(newTestLogger(), true)

	total := gatheredSeries(t, c, "slurm_node_gres_total")
	used := gatheredSeries(t, c, "slurm_node_gres_used")

	// a001 is allocated with all four GPUs busy, and belongs to four
	// partitions, so it publishes one series per partition.
	require.Contains(t, total, `slurm_node_gres_total{gres_type="gpu:model_a",node="a001",partition="gpu",status="allocated"} 4`)
	require.Contains(t, used, `slurm_node_gres_used{gres_type="gpu:model_a",node="a001",partition="gpu",status="allocated"} 4`)
	require.Contains(t, total, `slurm_node_gres_total{gres_type="gpu:model_a",node="a001",partition="gpu4",status="allocated"} 4`)

	// a005 is the case that broke a naive comma split: its GresUsed reads
	// "gpu:model_a:2(IDX:0,3)", two non-contiguous GPUs in use out of four.
	require.Contains(t, total, `slurm_node_gres_total{gres_type="gpu:model_a",node="a005",partition="gpu",status="mixed"} 4`)
	require.Contains(t, used, `slurm_node_gres_used{gres_type="gpu:model_a",node="a005",partition="gpu",status="mixed"} 2`)

	// A drained node still has its GPUs configured, and none of them in use.
	// The zero has to be published rather than dropped: an operator watching
	// utilisation needs to see capacity sitting idle, not a gap in the series.
	//
	// The status reads "drained*", not "drained": sinfo appends "*" to a state
	// when the node is not responding, and this collector passes the field
	// through as-is. That predates this change and is asserted here rather than
	// silently accepted, because it splits the series from a plain "drained"
	// one — the same shape as the default-partition "*" that issue #69 fixed on
	// the partition label.
	require.Contains(t, total, `slurm_node_gres_total{gres_type="gpu:model_a",node="a002",partition="gpu",status="drained*"} 4`)
	require.Contains(t, used, `slurm_node_gres_used{gres_type="gpu:model_a",node="a002",partition="gpu",status="drained*"} 0`)

	// The CPU node carries "(null)" in both columns and must publish nothing,
	// which is what keeps this metric from costing a series per node on a
	// cluster with no GRES at all.
	for _, s := range append(total, used...) {
		require.NotContains(t, s, `node="c001"`)
	}
}

// TestNodeGRESDisabled pins the flag: with the GRES metrics off, the collector
// publishes no GRES series and does not describe them either, so nothing shows
// up as an empty family on /metrics.
func TestNodeGRESDisabled(t *testing.T) {
	data, err := os.ReadFile(nodeGRESFixture)
	require.NoError(t, err)
	stubExecute(t, string(data))

	c := NewNodeCollector(newTestLogger(), false)

	require.Empty(t, gatheredSeries(t, c, "slurm_node_gres_total"))
	require.Empty(t, gatheredSeries(t, c, "slurm_node_gres_used"))

	// The CPU metrics are untouched by the flag.
	require.NotEmpty(t, gatheredSeries(t, c, "slurm_node_cpu_total"))
}

// TestNodeMetricsWithoutGRESColumns covers a sinfo that returns the six columns
// this collector read before GRES was added. The CPU and memory series must
// keep working rather than the whole line being dropped.
func TestNodeMetricsWithoutGRESColumns(t *testing.T) {
	stubExecute(t, "a001 850000 876896 288/0/0/288 allocated gpu\n")

	c := NewNodeCollector(newTestLogger(), true)

	require.NotEmpty(t, gatheredSeries(t, c, "slurm_node_cpu_total"))
	require.Empty(t, gatheredSeries(t, c, "slurm_node_gres_total"))
}

// TestNodeGRESMultipleTypes covers a node exposing two GPU models at once, with
// a job holding one of the second model.
//
// This is the case the per-node metrics exist for: "are the model_b cards
// saturated while the model_a ones idle" cannot be asked of slurm_gpus_*, which
// has no labels, nor of slurm_partition_gpus_*, which stops at the partition.
// It is also the hardest string to parse — two resources separated by a comma,
// each carrying its own index list, one of which is "(IDX:N/A)".
func TestNodeGRESMultipleTypes(t *testing.T) {
	data, err := os.ReadFile("../../test_data/slurm-25.11.2/node_detail_gres_multitype.txt")
	require.NoError(t, err)
	stubExecute(t, string(data))

	c := NewNodeCollector(newTestLogger(), true)
	total := gatheredSeries(t, c, "slurm_node_gres_total")
	used := gatheredSeries(t, c, "slurm_node_gres_used")

	require.Contains(t, total, `slurm_node_gres_total{gres_type="gpu:model_a",node="g2",partition="gpu",status="mixed"} 2`)
	require.Contains(t, total, `slurm_node_gres_total{gres_type="gpu:model_b",node="g2",partition="gpu",status="mixed"} 2`)
	require.Contains(t, used, `slurm_node_gres_used{gres_type="gpu:model_a",node="g2",partition="gpu",status="mixed"} 0`)
	require.Contains(t, used, `slurm_node_gres_used{gres_type="gpu:model_b",node="g2",partition="gpu",status="mixed"} 1`)
}
