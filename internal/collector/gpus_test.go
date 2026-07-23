package collector

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gpuFixtureExpect holds the GPU counts each per-version fixture in
// test_data/slurm-* must parse to. The GRES output format changed between
// Slurm releases (bare "gpu:2", "(null)" typed specs, "gpu:type:count(IDX...)",
// MIG slices, commas inside the IDX list), so this table is the version matrix
// that protects the parser and the alert-bearing slurm_gpus_* metrics.
//
// other = total-alloc-idle and util = alloc/total mirror the derivation in
// computeGPUsFromSnapshot; they are asserted per version so a regression in that
// formula — or a negative "other" (issue #145 removed the clamp that #16 added)
// — is caught on every real fixture.
type gpuExpect struct {
	total float64
	alloc float64
	idle  float64
	other float64
	util  float64
}

var gpuFixtureExpect = map[string]gpuExpect{
	"20.11.8": {total: 48, alloc: 7, idle: 41, other: 0, util: 7.0 / 48.0},
	"21.08.5": {total: 16, alloc: 0, idle: 16, other: 0, util: 0},
	// 23.11.10's sinfo_gpus_idle.txt has 2 columns, not the 3 IdleGPUsData emits,
	// so ParseIdleGPUs takes its 2-column branch and idle here is an artefact
	// (nodes×column) rather than total-alloc. This pins current behaviour only;
	// recapturing the fixture is tracked in #177.
	"23.11.10":   {total: 232, alloc: 33, idle: 33, other: 166, util: 33.0 / 232.0},
	"23.11.10-2": {total: 10, alloc: 6, idle: 4, other: 0, util: 0.6},
	"25.05":      {total: 4232, alloc: 4226, idle: 6, other: 0, util: 4226.0 / 4232.0},
	"25.11.1-1":  {total: 154, alloc: 78, idle: 24, other: 52, util: 78.0 / 154.0},
}

// gpuFixtureVersions globs the per-version fixture directories and asserts the
// set matches gpuFixtureExpect exactly. Before issue #148 the glob resolved to a
// non-existent directory, so it returned an empty slice and every test body was
// skipped while still reporting a pass. require.NotEmpty plus the symmetric
// membership checks make the suite fail loudly the next time the fixtures move
// or a new version is captured without expected values.
func gpuFixtureVersions(t *testing.T) map[string]string {
	t.Helper()

	paths, err := filepath.Glob("../../test_data/slurm-*")
	require.NoError(t, err)
	require.NotEmpty(t, paths, "no per-version GPU fixtures found under ../../test_data/slurm-*")

	versions := make(map[string]string, len(paths))
	for _, p := range paths {
		v := strings.TrimPrefix(filepath.Base(p), "slurm-")
		require.Contains(t, gpuFixtureExpect, v,
			"fixture %s has no entry in gpuFixtureExpect; add its expected GPU counts", v)
		versions[v] = p
	}
	for v := range gpuFixtureExpect {
		require.Contains(t, versions, v,
			"gpuFixtureExpect lists %s but no test_data/slurm-%s fixture exists", v, v)
	}
	return versions
}

// TestGPUsMetrics exercises the three pure parsers against every version fixture
// and asserts the exact counts, so a change in GRES parsing that breaks one
// Slurm format is caught even if the others still parse.
func TestGPUsMetrics(t *testing.T) {
	for version, path := range gpuFixtureVersions(t) {
		t.Run(version, func(t *testing.T) {
			want := gpuFixtureExpect[version]

			total := ParseTotalGPUs(readFixture(t, path, "sinfo_gpus_total.txt"))
			assert.Equal(t, want.total, total, "total GPUs")

			alloc := ParseAllocatedGPUs(readFixture(t, path, "sinfo_gpus_allocated.txt"))
			assert.Equal(t, want.alloc, alloc, "allocated GPUs")

			idle := ParseIdleGPUs(readFixture(t, path, "sinfo_gpus_idle.txt"))
			assert.Equal(t, want.idle, idle, "idle GPUs")

			// Mirror computeGPUsFromSnapshot's clampless derivation and pin the
			// invariant issue #145 relies on: on real data, total-alloc-idle is
			// never negative, so no clamp is needed.
			other := total - alloc - idle
			assert.Equal(t, want.other, other, "other GPUs")
			assert.GreaterOrEqual(t, other, 0.0, "other GPUs must never be negative")

			var util float64
			if total > 0 {
				util = alloc / total
			}
			assert.InDelta(t, want.util, util, 1e-9, "GPU utilization")
		})
	}
}

func readFixture(t *testing.T, dir, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	require.NoError(t, err)
	return data
}
