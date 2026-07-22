package collector

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseReservationNodesMetrics runs against a real "scontrol show nodes -o"
// capture (Slurm 25.11.2) with two reservations, including drained nodes inside
// a maintenance reservation. Regression test for issue #142: DRAIN is a state
// flag, not the primary state, so a MIXED+DRAIN node must land in both the mix
// and drain buckets and must NOT be counted healthy.
func TestParseReservationNodesMetrics(t *testing.T) {
	data, err := os.ReadFile("../../test_data/scontrol_nodes_reservation.txt")
	require.NoError(t, err, "cannot open test data")

	metrics := ParseReservationNodesMetrics(data)

	// Fixture nodes (c6, c7 have no ReservationName and must be ignored):
	//   maintenance-2026:
	//     c1 MIXED+DRAIN+DYNAMIC_NORM+MAINTENANCE+RESERVED       -> mix + drain
	//     c2 MIXED+DYNAMIC_NORM+MAINTENANCE+RESERVED+PLANNED     -> mix + planned
	//     c3 MIXED+DYNAMIC_NORM+MAINTENANCE+RESERVED             -> mix
	//     c4 IDLE+DRAIN+DYNAMIC_NORM+MAINTENANCE+RESERVED        -> idle + drain
	//   gpu-backfill:
	//     c5 IDLE+DYNAMIC_NORM+RESERVED                          -> idle
	require.Len(t, metrics, 2, "two reservations in the capture; unreserved nodes ignored")

	require.Contains(t, metrics, "maintenance-2026")
	m := metrics["maintenance-2026"]
	assert.Equal(t, 0.0, m.alloc)
	assert.Equal(t, 1.0, m.idle, "c4 base state is IDLE")
	assert.Equal(t, 3.0, m.mix, "c1, c2, c3 base state is MIXED")
	assert.Equal(t, 0.0, m.down)
	assert.Equal(t, 2.0, m.drain, "c1 and c4 carry the DRAIN flag")
	assert.Equal(t, 1.0, m.planned, "c2 carries the PLANNED flag")
	assert.Equal(t, 0.0, m.other)
	assert.Equal(t, 2.0, m.healthy, "only c2 and c3 are up and not drained")

	require.Contains(t, metrics, "gpu-backfill")
	b := metrics["gpu-backfill"]
	assert.Equal(t, 1.0, b.idle)
	assert.Equal(t, 0.0, b.drain)
	assert.Equal(t, 1.0, b.healthy)
}

// TestParseReservationNodesStateMatrix exercises the full base-state / flag
// matrix using real Slurm state strings (base state first, flags after '+').
// It replaces the previous version which fabricated State=DRAIN+RESERVED, a form
// scontrol never emits (issue #142).
func TestParseReservationNodesStateMatrix(t *testing.T) {
	input := []byte(
		"NodeName=n1 State=ALLOCATED+MAINTENANCE+RESERVED ReservationName=resv1\n" +
			"NodeName=n2 State=IDLE+RESERVED ReservationName=resv1\n" +
			"NodeName=n3 State=MIXED+RESERVED ReservationName=resv1\n" +
			"NodeName=n4 State=MIXED+DRAIN+MAINTENANCE+RESERVED ReservationName=resv1\n" +
			"NodeName=n5 State=IDLE+DRAIN+RESERVED ReservationName=resv1\n" +
			"NodeName=n6 State=DOWN+DRAIN+RESERVED ReservationName=resv1\n" +
			"NodeName=n7 State=MIXED+RESERVED+PLANNED ReservationName=resv1\n" +
			"NodeName=n8 State=DOWN*+RESERVED ReservationName=resv1\n" + // '*' = not responding
			"NodeName=n9 State=FUTURE+RESERVED ReservationName=resv1\n" +
			"NodeName=n10 State=IDLE\n", // no reservation -> ignored
	)
	metrics := ParseReservationNodesMetrics(input)

	require.Len(t, metrics, 1)
	require.Contains(t, metrics, "resv1")
	rm := metrics["resv1"]

	assert.Equal(t, 1.0, rm.alloc, "n1")
	assert.Equal(t, 2.0, rm.idle, "n2, n5")
	assert.Equal(t, 3.0, rm.mix, "n3, n4, n7")
	assert.Equal(t, 2.0, rm.down, "n6, n8 (DOWN* is still DOWN)")
	assert.Equal(t, 3.0, rm.drain, "n4, n5, n6 carry DRAIN")
	assert.Equal(t, 1.0, rm.planned, "n7 carries PLANNED")
	assert.Equal(t, 1.0, rm.other, "n9 FUTURE maps to other")
	// healthy = up base state (alloc/idle/mix) AND not drained:
	// n1, n2, n3, n7 -> 4. Drained (n4/n5/n6), down (n8) and other (n9) excluded.
	assert.Equal(t, 4.0, rm.healthy)
}
