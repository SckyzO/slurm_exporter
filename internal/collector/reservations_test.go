package collector

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseReservations(t *testing.T) {
	data, err := os.ReadFile("../../test_data/sreservations.txt")
	require.NoError(t, err)

	reservations, err := parseReservations(data)
	require.NoError(t, err)
	assert.Len(t, reservations, 1)

	res1 := reservations[0]
	assert.Equal(t, "pre-reservation-maintenance", res1.Name)
	assert.Equal(t, "INACTIVE", res1.State)
	assert.Equal(t, "user01", res1.Users)
	assert.Equal(t, "node[001-102]", res1.Nodes)
	assert.Equal(t, "", res1.Partition) // (null) -> empty string
	assert.Equal(t, "SPEC_NODES,ALL_NODES", res1.Flags)
	assert.Equal(t, 102.0, res1.NodeCount)
	assert.Equal(t, 25152.0, res1.CoreCount)

	// Use time.ParseInLocation to match the production code which respects
	// the server local timezone rather than silently using UTC.
	expectedStartTime, _ := time.ParseInLocation(slurmTimeLayout, "2025-08-26T07:00:00", time.Local)
	assert.Equal(t, expectedStartTime, res1.StartTime)
	expectedEndTime, _ := time.ParseInLocation(slurmTimeLayout, "2025-08-29T20:00:00", time.Local)
	assert.Equal(t, expectedEndTime, res1.EndTime)
}
