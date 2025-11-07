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
	assert.Equal(t, 10, int(nm.idle))
	assert.Equal(t, 10, int(nm.down))
	assert.Equal(t, 60, int(nm.alloc))
	assert.Equal(t, 10, int(nm.down))
	assert.Equal(t, 66, int(nm.other))
	assert.Equal(t, 8, int(nm.planned))
}
