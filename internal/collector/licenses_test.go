package collector

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLicenseMetrics(t *testing.T) {
	data, err := os.ReadFile("../../test_data/slicense.txt")
	require.NoError(t, err, "cannot open test data")

	lm := ParseLicenseMetrics(data)

	assert.Equal(t, 100.0, lm.total["ansys@flex"])
	assert.Equal(t, 20.0, lm.used["ansys@flex"])
	assert.Equal(t, 80.0, lm.free["ansys@flex"])
	assert.Equal(t, 0.0, lm.reserved["ansys@flex"])

	assert.Equal(t, 30.0, lm.total["fluent@flex"])
	assert.Equal(t, 10.0, lm.used["fluent@flex"])
	assert.Equal(t, 20.0, lm.free["fluent@flex"])
	assert.Equal(t, 5.0, lm.reserved["fluent@flex"], "fluent@flex has 5 reserved licenses")
}
