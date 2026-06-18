package collector

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The default-partition "*" marker (sinfo %P) must not leak into the
// partition label: "gpu*" -> "gpu". Fixture also has an unmarked partition.
func TestNodeMetricsDefaultPartitionMarker(t *testing.T) {
	data, err := os.ReadFile("../../test_data/sinfo_default_partition.txt")
	if err != nil {
		t.Fatalf("Can not open test data: %v", err)
	}
	metrics := ParseNodeMetrics(data)

	assert.Contains(t, metrics, "a048")
	assert.Contains(t, metrics["a048"].partitions, "gpu", "marker must be stripped")
	assert.NotContains(t, metrics["a048"].partitions, "gpu*", "raw marker must not leak")
	assert.Contains(t, metrics["a048"].partitions, "long", "unmarked partition unchanged")

	assert.Contains(t, metrics, "a049")
	assert.Contains(t, metrics["a049"].partitions, "gpu")
	assert.NotContains(t, metrics["a049"].partitions, "gpu*")
}
