package collector

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLicenseMetrics(t *testing.T) {
	// Read the input data from a file
	file, err := os.Open("../../test_data/slicense.txt")
	if err != nil {
		t.Fatalf("Can not open test data: %v", err)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("Can not read test data: %v", err)
	}
	lm := ParseLicenseMetrics(data)
	assert.Equal(t, 100, int(lm.total["ansys@flex"]))
	assert.Equal(t, 20, int(lm.used["ansys@flex"]))
	assert.Equal(t, 80, int(lm.free["ansys@flex"]))

	assert.Equal(t, 30, int(lm.total["fluent@flex"]))
	assert.Equal(t, 10, int(lm.used["fluent@flex"]))
	assert.Equal(t, 20, int(lm.free["fluent@flex"]))
}
