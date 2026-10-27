package collector

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
The proof for #254.

The node and drain_reason collectors stopped running `sinfo -N` and now read
`scontrol show nodes -o` plus a grouped `sinfo -h -o "%N|%T"`. Nothing about
their output is supposed to change: same nodes, same labels, same values.

Asserting hand-written expectations would only show the new parser agrees with
a transcription of the old one. These tests instead run both implementations
over a capture of all four commands taken at the same moment on the test
cluster, so what is compared is the code that shipped against the code that
replaces it, on output Slurm actually produced.

A capture directory must hold all four files:

	node_detail.txt    sinfo -h -N -O "NodeList: ,AllocMem: ,Memory: ,CPUsState: ,StateLong: ,Partition:"
	drain_reason.txt   sinfo -h -N -o "%N|%E|%H|%T"
	scontrol_nodes.txt scontrol show nodes -o
	node_state.txt     sinfo -h -o "%N|%T"
*/

// pairedCaptureDirs returns every test_data directory holding a complete
// capture. It fails the test when there are none: a table-driven test over an
// empty table passes while proving nothing, which is the one outcome this
// file must not be able to produce.
func pairedCaptureDirs(t *testing.T) []string {
	t.Helper()
	const root = "../../test_data"
	required := []string{"node_detail.txt", "drain_reason.txt", "scontrol_nodes.txt", "node_state.txt"}

	entries, err := os.ReadDir(root)
	require.NoError(t, err)

	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		complete := true
		for _, f := range required {
			if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
				complete = false
				break
			}
		}
		if complete {
			dirs = append(dirs, dir)
		}
	}
	require.NotEmpty(t, dirs,
		"no complete paired capture under %s — the equivalence of the new node path is unproven; capture the four commands with scripts/testing", root)
	return dirs
}

func readFixture(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return b
}

func TestNewNodePathMatchesTheOldSinfoPath(t *testing.T) {
	for _, dir := range pairedCaptureDirs(t) {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			want := ParseNodeMetrics(readFixture(t, filepath.Join(dir, "node_detail.txt")))
			got, derivedStates := BuildNodeMetrics(
				readFixture(t, filepath.Join(dir, "scontrol_nodes.txt")),
				readFixture(t, filepath.Join(dir, "node_state.txt")),
			)

			assert.Zero(t, derivedStates,
				"every node should have been listed by the grouped sinfo call")

			wantNames := mapKeys(want)
			gotNames := mapKeys(got)
			require.Equal(t, wantNames, gotNames, "the two paths must cover the same nodes")

			for _, name := range wantNames {
				w, g := want[name], got[name]
				assert.Equal(t, w.nodeStatus, g.nodeStatus, "%s: status label", name)
				assert.Equal(t, w.memAlloc, g.memAlloc, "%s: memAlloc", name)
				assert.Equal(t, w.memTotal, g.memTotal, "%s: memTotal", name)
				assert.Equal(t, w.cpuAlloc, g.cpuAlloc, "%s: cpuAlloc", name)
				assert.Equal(t, w.cpuIdle, g.cpuIdle, "%s: cpuIdle", name)
				assert.Equal(t, w.cpuOther, g.cpuOther, "%s: cpuOther", name)
				assert.Equal(t, w.cpuTotal, g.cpuTotal, "%s: cpuTotal", name)

				// Compared as sets: the collector emits one series per
				// partition, so the order the two paths happen to list them
				// in is not observable in /metrics.
				wp, gp := slices.Clone(w.partitions), slices.Clone(g.partitions)
				slices.Sort(wp)
				slices.Sort(gp)
				assert.Equal(t, wp, gp, "%s: partitions", name)
			}
		})
	}
}

func TestNewDrainReasonPathMatchesTheOldSinfoPath(t *testing.T) {
	for _, dir := range pairedCaptureDirs(t) {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			want := ParseDrainReasonMetrics(readFixture(t, filepath.Join(dir, "drain_reason.txt")))
			got := BuildDrainReasonMetrics(
				readFixture(t, filepath.Join(dir, "scontrol_nodes.txt")),
				readFixture(t, filepath.Join(dir, "node_state.txt")),
			)

			byNode := func(ms []DrainReasonMetrics) map[string]DrainReasonMetrics {
				out := make(map[string]DrainReasonMetrics, len(ms))
				for _, m := range ms {
					out[m.Node] = m
				}
				return out
			}
			w, g := byNode(want), byNode(got)

			require.Equal(t, mapKeys(w), mapKeys(g),
				"the two paths must report a reason for the same nodes")
			for node, wm := range w {
				assert.Equal(t, wm.Reason, g[node].Reason, "%s: reason label", node)
				assert.Equal(t, wm.Since, g[node].Since, "%s: since label", node)
			}
		})
	}
}

// A capture with no degraded node cannot prove anything about drain_reason:
// both paths return nothing and the comparison above is vacuous. This makes
// that gap visible instead of letting it read as coverage.
func TestAtLeastOneCaptureCarriesADrainReason(t *testing.T) {
	total := 0
	for _, dir := range pairedCaptureDirs(t) {
		total += len(ParseDrainReasonMetrics(readFixture(t, filepath.Join(dir, "drain_reason.txt"))))
	}
	assert.Positive(t, total,
		"no captured node carries a drain reason, so the reason and since labels are untested against real output")
}

func mapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
