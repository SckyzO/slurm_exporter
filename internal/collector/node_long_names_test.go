package collector

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNodeMetricsLongNames is the non-regression test for issue #10:
// long node names previously caused silent column collision in sinfo -O output,
// dropping affected nodes from the metrics map. The fix uses variable-width
// columns (trailing ':') in NodeData(). The parser itself is unchanged because
// strings.Fields() handles both fixed-width-padded and single-space output.
//
// The fixture simulates the output of:
//
//	sinfo -h -N -O "NodeList: ,AllocMem: ,Memory: ,CPUsState: ,StateLong: ,Partition:"
//
// with a 25-char node name that would have collided under the old format.
func TestNodeMetricsLongNames(t *testing.T) {
	data, err := os.ReadFile("../../test_data/sinfo_long_names_fixed.txt")
	if err != nil {
		t.Fatalf("Can not open test data: %v", err)
	}
	metrics := ParseNodeMetrics(data)

	assert.Contains(t, metrics, "my-very-long-hostname-001",
		"long-name node must be parsed once variable-width sinfo format is used")
	assert.Contains(t, metrics, "a048", "short-name node should still parse")
	assert.Contains(t, metrics, "b003", "short-name node should still parse")

	long := metrics["my-very-long-hostname-001"]
	assert.Equal(t, uint64(163840), long.memAlloc)
	assert.Equal(t, uint64(193000), long.memTotal)
	assert.Equal(t, uint64(16), long.cpuTotal)
	assert.Equal(t, "idle", long.nodeStatus)
	assert.Contains(t, long.partitions, "long")
	assert.Contains(t, long.partitions, "short")
}
