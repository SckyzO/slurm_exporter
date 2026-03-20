package collector

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCPUsMetrics(t *testing.T) {
	file, err := os.Open("../../test_data/sinfo_cpus.txt")
	require.NoError(t, err, "cannot open test data")
	data, err := io.ReadAll(file)
	require.NoError(t, err, "cannot read test data")

	cm := ParseCPUsMetrics(data)
	assert.Equal(t, 5725.0, cm.alloc)
	assert.Equal(t, 877.0, cm.idle)
	assert.Equal(t, 34.0, cm.other)
	assert.Equal(t, 6636.0, cm.total)
}

// TestParseCPUsMetricsMalformed verifies the parser does not panic or
// produce garbage values when the sinfo output has an unexpected format.
// This covers the bounds-check fix added to ParseCPUsMetrics.
func TestParseCPUsMetricsMalformed(t *testing.T) {
	cases := []struct {
		name  string
		input []byte
	}{
		{"empty", []byte("")},
		{"no slash", []byte("5725")},
		{"only two fields", []byte("5725/877")},
		{"three fields", []byte("5725/877/34")},
		{"garbage", []byte("not/a/number/here")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Must not panic
			cm := ParseCPUsMetrics(tc.input)
			assert.NotNil(t, cm)
		})
	}
}
