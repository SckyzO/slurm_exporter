package collector

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseDrainReasonNotResponding is the non-regression test for issue #198.
//
// slurmctld sets Reason="Not responding" itself when it loses contact with a
// node, and the parser used to drop that row alongside the genuinely empty
// "none" and "unknown". The node went down, the reason existed, and
// slurm_node_drain_reason_info stayed silent about it — the one case the
// reporter needed it for.
//
// The fixture is the sinfo -h -N -o "%N|%E|%H|%T" rendering of the node in the
// report: State=DOWN+NOT_RESPONDING, which sinfo prints as "down*".
func TestParseDrainReasonNotResponding(t *testing.T) {
	input := []byte(
		"gpu02|Not responding|2026-07-25T11:54:03|down*\n" +
			"gpu03|Not responding|2026-07-25T11:54:03|down\n")

	got := ParseDrainReasonMetrics(input)

	require.Len(t, got, 2, "a node slurmctld downed for not responding must be reported")
	require.Equal(t, "gpu02", got[0].Node)
	require.Equal(t, "Not responding", got[0].Reason,
		"the reason text is the only place the distinction between an unreachable node "+
			"and an admin-drained one shows up")
	require.Equal(t, unixLocal(t, "2026-07-25T11:54:03"), got[0].SinceUnix)
	require.Equal(t, "gpu03", got[1].Node)
}

// TestParseDrainReasonEmptyReasons pins the rows that stay filtered. These carry
// no information: sinfo prints them for every healthy node, so exporting them
// would publish one series per node in the cluster to say nothing.
func TestParseDrainReasonEmptyReasons(t *testing.T) {
	input := []byte(
		"c1|none|Unknown|idle\n" +
			"c2|None|Unknown|down\n" +
			"c3|Unknown|Unknown|down\n" +
			"c4||Unknown|down\n" +
			"c5|Hardware fault|2026-07-25T11:54:03|down\n")

	got := ParseDrainReasonMetrics(input)

	require.Len(t, got, 1, "only the node with a real reason is exported")
	require.Equal(t, "c5", got[0].Node)
}
