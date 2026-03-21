package collector

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNodesMetrics(t *testing.T) {
	file, err := os.Open("../../test_data/sinfo.txt")
	require.NoError(t, err, "cannot open test data")
	data, err := io.ReadAll(file)
	require.NoError(t, err)

	nm := ParseNodesMetrics(data)
	assert.Equal(t, 10, int(nm.idle["feature_a,feature_b"]))
	assert.Equal(t, 10, int(nm.down["feature_a,feature_b"]))
	assert.Equal(t, 40, int(nm.alloc["feature_a,feature_b"]))
	assert.Equal(t, 20, int(nm.alloc["feature_a"]))
	assert.Equal(t, 10, int(nm.down["null"]))
	assert.Equal(t, 42, int(nm.other["null"]))
	assert.Equal(t, 24, int(nm.other["feature_a"]))
	assert.Equal(t, 3, int(nm.planned["feature_a"]))
	assert.Equal(t, 5, int(nm.planned["feature_b"]))
	assert.Equal(t, 7, int(nm.inval["null"]))
}

// TestSumMapAggregation verifies that sumMap correctly aggregates node counts
// across feature sets, which is what --collector.nodes.feature-set=false relies on.
func TestSumMapAggregation(t *testing.T) {
	file, err := os.Open("../../test_data/sinfo.txt")
	require.NoError(t, err, "cannot open test data")
	data, err := io.ReadAll(file)
	require.NoError(t, err)

	nm := ParseNodesMetrics(data)

	// alloc: feature_a=20, feature_a,feature_b=40 → total=60
	assert.Equal(t, 60.0, sumMap(nm.alloc))
	// down: feature_a,feature_b=10, null=10 → total=20
	assert.Equal(t, 20.0, sumMap(nm.down))
	// planned: feature_a=3, feature_b=5 → total=8
	assert.Equal(t, 8.0, sumMap(nm.planned))
}
