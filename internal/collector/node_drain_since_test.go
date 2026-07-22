package collector

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// TestDrainReasonSeriesIdentityIsStableAcrossRedrains is the non-regression test
// for unbounded series growth on slurm_node_drain_reason_info.
//
// The drain timestamp from sinfo "%H" was published as a label. Draining the
// same node again yields a different timestamp, so Prometheus saw a brand new
// series and orphaned the previous one. The series count then tracked how often
// operators drained nodes over the lifetime of the TSDB rather than how many
// nodes are drained, and a site that drains routinely for maintenance paid for
// it continuously. See issue #141.
func TestDrainReasonSeriesIdentityIsStableAcrossRedrains(t *testing.T) {
	want := []string{`slurm_node_drain_reason_info{node="c1",reason="disk failure"} 1`}
	log := logger.NewLogger("error")

	stubExecute(t, "c1|disk failure|2026-04-01T10:00:00|drained\n")
	assert.Equal(t, want, gatheredSeries(t, NewDrainReasonCollector(log), "slurm_node_drain_reason_info"))

	// The same node, drained again for the same reason three months later.
	stubExecute(t, "c1|disk failure|2026-07-01T08:30:00|drained\n")
	assert.Equal(t, want, gatheredSeries(t, NewDrainReasonCollector(log), "slurm_node_drain_reason_info"),
		"a re-drain must land on the existing series instead of creating a second one")
}

// TestDrainReasonExportsSinceAsATimestamp pins the replacement for the label.
// The drain time belongs in the value, where it can be plotted and subtracted
// from time() to get a drain duration.
func TestDrainReasonExportsSinceAsATimestamp(t *testing.T) {
	stubExecute(t, "c1|disk failure|2026-04-01T10:00:00|drained\nc2|hw-fail|2026-04-01T09:00:00|down\n")

	want := []string{
		fmt.Sprintf(`slurm_node_drain_since_timestamp_seconds{node="c1"} %g`, unixLocal(t, "2026-04-01T10:00:00")),
		fmt.Sprintf(`slurm_node_drain_since_timestamp_seconds{node="c2"} %g`, unixLocal(t, "2026-04-01T09:00:00")),
	}
	assert.Equal(t, want, gatheredSeries(t, NewDrainReasonCollector(logger.NewLogger("error")), "slurm_node_drain_since_timestamp_seconds"))
}

// TestDrainReasonOmitsTimestampWhenSinfoReportsNone guards the other direction.
// sinfo prints "Unknown" when the reason carries no time, and SLURM_TIME_FORMAT
// can make it print something this parser does not read. Either way the node is
// still drained and its reason still matters, but the timestamp must be absent
// rather than zero: zero reads as 1970 and makes every time() subtraction wrong
// by decades.
func TestDrainReasonOmitsTimestampWhenSinfoReportsNone(t *testing.T) {
	stubExecute(t, "c1|disk failure|Unknown|drained\n")
	log := logger.NewLogger("error")

	assert.Len(t, gatheredSeries(t, NewDrainReasonCollector(log), "slurm_node_drain_reason_info"), 1,
		"the node is drained, so its reason is still published")
	assert.Empty(t, gatheredSeries(t, NewDrainReasonCollector(log), "slurm_node_drain_since_timestamp_seconds"))
}
