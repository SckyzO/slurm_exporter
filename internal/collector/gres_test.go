package collector

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseGRESByType covers every GRES shape captured under test_data/, plus
// the ones only a mangled cluster produces.
//
// The strings are not invented: each is taken from a real capture, and the
// comment says which Slurm release it came from. GRES formatting is the thing
// that has drifted most between Slurm majors, so a single-version test here
// would prove very little.
func TestParseGRESByType(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want map[string]uint64
	}{
		{"untyped, 20.11.8", "gpu:2", map[string]uint64{"gpu": 2}},
		{"untyped, 21.08.5", "gpu:8", map[string]uint64{"gpu": 8}},
		{"typed, 23.11.10", "gpu:h100:4", map[string]uint64{"gpu:h100": 4}},
		{"socket suffix, 23.11.10-2", "gpu:mi210:2(S:0)", map[string]uint64{"gpu:mi210": 2}},
		{"MIG profile with dots, 25.11.1-1", "gpu:nvidia_a100_1g.5gb:21(S:0-1)",
			map[string]uint64{"gpu:nvidia_a100_1g.5gb": 21}},
		{"mixed case model, 25.11.1-1", "gpu:NVlink_A100_40GB:2(S:0-1)",
			map[string]uint64{"gpu:NVlink_A100_40GB": 2}},
		{"hyphenated model, 25.11.1-1", "gpu:tesla_v100-sxm3-32gb:16(IDX:0-15)",
			map[string]uint64{"gpu:tesla_v100-sxm3-32gb": 16}},
		{"null model, 21.08.5", "gpu:(null):0(IDX:N/A)", map[string]uint64{"gpu": 0}},
		{"zero used on a drained node, 25.05.3", "gpu:gh200:0(IDX:N/A)",
			map[string]uint64{"gpu:gh200": 0}},

		// The case that made splitting on every comma wrong. A node with
		// non-contiguous GPU indices renders one resource whose suffix contains
		// a comma; naive splitting turns it into two unparseable fragments and
		// the node's GPUs vanish from the count.
		{"non-contiguous indices, 25.11.1-1", "gpu:NVlink_A100_40GB:2(IDX:0,3)",
			map[string]uint64{"gpu:NVlink_A100_40GB": 2}},

		// Non-GPU GRES has to survive too: this collector is about GRES in
		// general, not GPUs alone.
		{"several resources, 23.11.10-2", "gpu:h100:1,shard:h100:2",
			map[string]uint64{"gpu:h100": 1, "shard:h100": 2}},
		{"same type twice", "gpu:A100:4,gpu:H100:2",
			map[string]uint64{"gpu:A100": 4, "gpu:H100": 2}},

		// A token cut in half by a fixed-width sinfo column. The unanchored
		// pattern this replaced read "shard:h10" as model "h1" count 0 and
		// published a phantom series; skipping it is the only honest answer,
		// since the real count is not in the string any more.
		{"truncated token is skipped, not guessed", "gpu:h100:1,shard:h10",
			map[string]uint64{"gpu:h100": 1}},

		{"no GRES", "(null)", map[string]uint64{}},
		{"empty", "", map[string]uint64{}},
		{"N/A", "N/A", map[string]uint64{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.want) == 0 {
				require.Empty(t, parseGRESByType(tc.in))
				return
			}
			require.Equal(t, tc.want, parseGRESByType(tc.in))
		})
	}
}

// TestParseGRESByTypeAcceptsEveryCapturedString runs the parser over every GRES
// string sitting in the versioned fixtures and requires each one to yield at
// least one resource.
//
// The table above pins values on hand-picked examples. This one is the sweep:
// it fails when a future capture introduces a shape the parser silently drops,
// which is the failure mode that hides GPUs rather than reporting them wrong.
func TestParseGRESByTypeAcceptsEveryCapturedString(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join(testDataDir, "slurm-*", "sinfo_gpus_*.txt"))
	require.NoError(t, err)
	require.NotEmpty(t, paths, "the versioned GPU fixtures are the corpus this sweep runs on")

	checked := 0
	for _, path := range paths {
		data, err := os.ReadFile(path) //nolint:gosec // G304: test fixture path from a glob under test_data/
		require.NoError(t, err)

		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			for _, spec := range fields[1:] {
				if spec == "(null)" {
					continue
				}
				require.NotEmptyf(t, parseGRESByType(spec),
					"%s: %q parsed to nothing, so those resources would vanish from the metrics",
					filepath.Base(filepath.Dir(path)), spec)
				checked++
			}
		}
	}
	require.Greaterf(t, checked, 20, "only %d GRES strings swept; the corpus should be larger", checked)
}
