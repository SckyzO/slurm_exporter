package collector

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

/*
For this example data line:

a048,79384,193000,3/13/0/16,mix,long

We want output that looks like:

slurm_node_cpus_allocated{name="a048",status="mix", partition="long"} 3
slurm_node_cpus_idle{name="a048",status="mix", partition="long"} 3
slurm_node_cpus_other{name="a048",status="mix", partition="long"} 0
slurm_node_cpus_total{name="a048",status="mix", partition="long"} 16
slurm_node_mem_allocated{name="a048",status="mix", partition="long"} 179384
slurm_node_mem_total{name="a048",status="mix", partition="long"} 193000
slurm_node_status{name="a048",status="mix", partition="long"} 1

slurm_node_status{name="a048",status="mix", partition="short"} 1
slurm_node_status{name="a048",status="idle", partition="all"} 1

*/

func TestNodeMetrics(t *testing.T) {
	data, err := os.ReadFile("../../test_data/sinfo_mem.txt")
	if err != nil {
		t.Fatalf("Can not open test data: %v", err)
	}
	metrics := ParseNodeMetrics(data)
	t.Logf("%+v", metrics)

	assert.Contains(t, metrics, "a048")
	assert.Equal(t, uint64(163840), metrics["a048"].memAlloc)
	assert.Equal(t, uint64(193000), metrics["a048"].memTotal)
	assert.Equal(t, uint64(16), metrics["a048"].cpuAlloc)
	assert.Equal(t, uint64(0), metrics["a048"].cpuIdle)
	assert.Equal(t, uint64(0), metrics["a048"].cpuOther)
	assert.Equal(t, uint64(16), metrics["a048"].cpuTotal)
	assert.Equal(t, "mixed", metrics["a048"].nodeStatus)

	// Check partitions
	assert.Contains(t, metrics["a048"].partitions, "long")
	assert.Contains(t, metrics["a048"].partitions, "short")
	assert.Contains(t, metrics["a048"].partitions, "all")
	assert.Contains(t, metrics["a048"].partitions, "gpu")

	// Additional checks for other nodes
	assert.Contains(t, metrics, "b003")
	assert.Equal(t, uint64(296960), metrics["b003"].memAlloc)
	assert.Equal(t, uint64(386000), metrics["b003"].memTotal)
	assert.Contains(t, metrics["b003"].partitions, "long")
	assert.Contains(t, metrics["b003"].partitions, "gpu")
	assert.Equal(t, "down", metrics["b003"].nodeStatus)
}
