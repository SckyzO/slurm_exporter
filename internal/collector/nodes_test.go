package collector

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNodesMetrics(t *testing.T) {
	// Read the input data from a file
	file, err := os.Open("../../test_data/sinfo.txt")
	if err != nil {
		t.Fatalf("Can not open test data: %v", err)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("Can not read test data: %v", err)
	}
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
