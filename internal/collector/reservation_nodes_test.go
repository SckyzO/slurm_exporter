package collector

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseReservationNodesMetrics(t *testing.T) {
	data, err := os.ReadFile("../../test_data/scontrol_nodes_reservation.txt")
	require.NoError(t, err, "cannot open test data")

	metrics := ParseReservationNodesMetrics(data)

	// The test data has 5 GPU nodes in "maintenance-2026" reservation:
	//   gpu-node-0: ALLOCATED+MAINTENANCE+RESERVED -> alloc
	//   gpu-node-1: IDLE+MAINTENANCE+RESERVED      -> idle
	//   gpu-node-2: IDLE+MAINTENANCE+RESERVED      -> idle
	//   gpu-node-3: IDLE+MAINTENANCE+RESERVED      -> idle
	//   gpu-node-4: IDLE+MAINTENANCE+RESERVED      -> idle
	// CPU nodes have no reservation -> not counted
	require.Contains(t, metrics, "maintenance-2026")
	rm := metrics["maintenance-2026"]

	assert.Equal(t, 1.0, rm.alloc, "1 allocated node in reservation")
	assert.Equal(t, 4.0, rm.idle, "4 idle nodes in reservation")
	assert.Equal(t, 0.0, rm.mix)
	assert.Equal(t, 0.0, rm.down)
	assert.Equal(t, 0.0, rm.drain)
	assert.Equal(t, 0.0, rm.planned)
	assert.Equal(t, 0.0, rm.other)
	assert.Equal(t, 5.0, rm.healthy, "5 healthy nodes (alloc+idle)")

	// CPU nodes have no reservation — must not create any reservation entry for them
	assert.Len(t, metrics, 1, "only one reservation in test data")
}

// TestParseReservationNodesCompoundStates verifies primary state extraction
// from compound Slurm states, based on real scontrol output patterns.
func TestParseReservationNodesCompoundStates(t *testing.T) {
	input := []byte(
		"NodeName=n1 State=ALLOCATED+MAINTENANCE+RESERVED ReservationName=resv1\n" +
			"NodeName=n2 State=IDLE+RESERVED ReservationName=resv1\n" +
			"NodeName=n3 State=MIXED+RESERVED ReservationName=resv1\n" +
			"NodeName=n4 State=DOWN+DRAIN+RESERVED ReservationName=resv1\n" +
			"NodeName=n5 State=DRAIN+RESERVED ReservationName=resv1\n" +
			"NodeName=n6 State=PLANNED+RESERVED ReservationName=resv1\n" +
			"NodeName=n7 State=COMPLETING+RESERVED ReservationName=resv1\n" +
			"NodeName=n8 State=IDLE\n", // no reservation
	)
	metrics := ParseReservationNodesMetrics(input)

	require.Contains(t, metrics, "resv1")
	rm := metrics["resv1"]

	assert.Equal(t, 1.0, rm.alloc)
	assert.Equal(t, 1.0, rm.idle)
	assert.Equal(t, 1.0, rm.mix)
	assert.Equal(t, 1.0, rm.down)
	assert.Equal(t, 1.0, rm.drain)
	assert.Equal(t, 1.0, rm.planned)
	assert.Equal(t, 1.0, rm.other, "COMPLETING maps to other")
	// healthy = alloc + idle + mix + planned = 4
	assert.Equal(t, 4.0, rm.healthy)

	// n8 has no reservation
	assert.Len(t, metrics, 1)
}
