package collector

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// relativeTimeReservation is real `scontrol show reservation` output, captured on
// the scripts/testing cluster running Slurm 25.11.2 with SLURM_TIME_FORMAT set to
// "relative". Execute never sets cmd.Env, so whatever value the exporter's own
// environment holds reaches every Slurm command.
func relativeTimeReservation(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../test_data/sreservations_relative_time.txt")
	require.NoError(t, err)
	return string(data)
}

// TestReservationOmitsUnreadableTimestamps is the non-regression test for issue
// #158.
//
// The parse error was discarded, leaving the zero time.Time, and the collector
// published it anyway. time.Time{}.Unix() is -62135596800, which places the
// reservation in year 1 and is indistinguishable from a measurement: no
// threshold rejects it, no subtraction fails on it, and nothing logged it. An
// absent series is the only honest answer when the timestamp could not be read.
func TestReservationOmitsUnreadableTimestamps(t *testing.T) {
	stubExecute(t, relativeTimeReservation(t))
	log, logs := bufferLogger()
	c := NewReservationsCollector(log)

	assert.Empty(t, gatheredSeries(t, c, "slurm_reservation_start_time_seconds"),
		"an unreadable StartTime must be absent, not year 1")
	assert.Empty(t, gatheredSeries(t, c, "slurm_reservation_end_time_seconds"),
		"an unreadable EndTime must be absent, not year 1")

	assert.Equal(t,
		[]string{`slurm_reservation_node_count{reservation_name="maint-158"} 4`},
		gatheredSeries(t, c, "slurm_reservation_node_count"),
		"the reservation itself is still reported; only its timestamps are missing")

	assert.Contains(t, logs.String(), "maint-158")
	assert.Contains(t, logs.String(), "14:24:22",
		"the raw value belongs in the log, it is what tells an operator what to fix")
}

// TestReservationOmitsOnlyTheUnreadableTimestamp pins that the two timestamps are
// judged one at a time. Dropping both because one failed would lose data the
// collector actually has.
func TestReservationOmitsOnlyTheUnreadableTimestamp(t *testing.T) {
	stubExecute(t, "ReservationName=maint-158 StartTime=2026-07-22T14:24:22 EndTime=16:24:22 Duration=02:00:00\n"+
		"   Nodes=c[1-4] NodeCnt=4 CoreCnt=64 PartitionName=(null) Flags=MAINT\n"+
		"   Users=root State=ACTIVE\n")
	c := NewReservationsCollector(logger.NewLogger("error"))

	assert.Equal(t,
		[]string{fmt.Sprintf(`slurm_reservation_start_time_seconds{reservation_name="maint-158"} %g`, unixLocal(t, "2026-07-22T14:24:22"))},
		gatheredSeries(t, c, "slurm_reservation_start_time_seconds"))
	assert.Empty(t, gatheredSeries(t, c, "slurm_reservation_end_time_seconds"))
}

// TestReservationStaysSilentWhenScontrolPrintsNoTimestamp separates the two ways
// a timestamp can be missing. A field scontrol never printed is not something an
// operator can fix by changing SLURM_TIME_FORMAT, so it must not raise that
// warning. Only a field that was printed and could not be read does.
func TestReservationStaysSilentWhenScontrolPrintsNoTimestamp(t *testing.T) {
	stubExecute(t, "ReservationName=maint-158 Duration=02:00:00 NodeCnt=4 CoreCnt=64 Users=root State=ACTIVE\n")
	log, logs := bufferLogger()
	c := NewReservationsCollector(log)

	assert.Empty(t, gatheredSeries(t, c, "slurm_reservation_start_time_seconds"))
	assert.Len(t, gatheredSeries(t, c, "slurm_reservation_info"), 1,
		"the reservation is still reported")
	assert.Empty(t, logs.String())
}

// TestReservationExportsReadableTimestamps guards the other direction: the fix
// must not stop publishing timestamps that parse.
func TestReservationExportsReadableTimestamps(t *testing.T) {
	data, err := os.ReadFile("../../test_data/sreservations.txt")
	require.NoError(t, err)
	stubExecute(t, string(data))

	log, logs := bufferLogger()
	c := NewReservationsCollector(log)

	assert.Equal(t,
		[]string{fmt.Sprintf(`slurm_reservation_start_time_seconds{reservation_name="pre-reservation-maintenance"} %g`, unixLocal(t, "2025-08-26T07:00:00"))},
		gatheredSeries(t, c, "slurm_reservation_start_time_seconds"))
	assert.Equal(t,
		[]string{fmt.Sprintf(`slurm_reservation_end_time_seconds{reservation_name="pre-reservation-maintenance"} %g`, unixLocal(t, "2025-08-29T20:00:00"))},
		gatheredSeries(t, c, "slurm_reservation_end_time_seconds"))
	assert.Empty(t, logs.String(), "a reservation that parses cleanly must stay silent")
}
