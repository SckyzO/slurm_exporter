package collector

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/v2/internal/logger"
)

// docMetricPattern pulls metric names out of docs/metrics.md.
//
// Only backticked names count. That is the convention every table in that file
// already follows, and it is what keeps prose out of the comparison: a sentence
// mentioning slurm_queue_* as a family should not read as a metric, and
// `cluster:slurm_job_failure_rate:ratio15m` is a recording rule that lives in
// rules.yml rather than a series this exporter publishes.
var docMetricPattern = regexp.MustCompile("`(slurm_[a-z0-9_]+)`")

// TestMetricsDocMatchesDeclaredSurface makes docs/metrics.md and the collectors
// agree, in both directions.
//
// The release checklist has always asked for this comparison, by diffing a live
// scrape against the document. That diff cannot see a metric the cluster had no
// reason to emit — and it never did: eight slurm_queue_* and eight
// slurm_cores_* terminal-state metrics were collapsed into a single "..." row
// in the document, invisible to a reader searching for slurm_queue_failed and
// invisible to the diff, because a test cluster with no failed jobs publishes
// none of them.
//
// Comparing against the declared surface instead of a scrape removes the
// cluster from the equation, so the check holds regardless of what the cluster
// happens to be doing.
func TestMetricsDocMatchesDeclaredSurface(t *testing.T) {
	raw, err := os.ReadFile("../../docs/metrics.md")
	require.NoError(t, err)

	documented := map[string]bool{}
	for _, m := range docMetricPattern.FindAllStringSubmatch(string(raw), -1) {
		documented[m[1]] = true
	}
	require.NotEmpty(t, documented, "no metric names found in docs/metrics.md; has the table format changed?")

	declared := map[string]bool{}
	for _, group := range [][]SurfaceVariant{MetricSurfaceVariants(), MetricSurfaceInternals()} {
		for _, v := range group {
			metrics, surfErr := SurfaceOf(v.New(logger.NewTextLogger("error")))
			require.NoError(t, surfErr)
			for _, m := range metrics {
				declared[m.Name] = true
			}
		}
	}

	require.Empty(t, diff(declared, documented),
		"declared by a collector but absent from docs/metrics.md.\n"+
			"Add a row for each in the relevant collector's table. An operator who "+
			"greps the documentation for one of these finds nothing.")

	require.Empty(t, diff(documented, declared),
		"present in docs/metrics.md but declared by no collector.\n"+
			"Either the metric was removed and the row is stale, or the name is a typo.")
}

// diff returns the names in a that are missing from b, sorted so a failure
// reads the same way twice.
func diff(a, b map[string]bool) []string {
	var out []string
	for name := range a {
		if !b[name] {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// TestDocMetricPatternIgnoresProse guards the extraction. Loosening it to match
// unbackticked text would pull in metric families written as slurm_queue_* and
// recording rules, and the resulting failures would train a reader to ignore
// this test.
func TestDocMetricPatternIgnoresProse(t *testing.T) {
	found := docMetricPattern.FindAllStringSubmatch(strings.Join([]string{
		"| `slurm_queue_failed` | Failed jobs | `user` |",
		"the slurm_cores_* family is emitted per user",
		"`cluster:slurm_job_failure_rate:ratio15m` is computed by rules.yml",
		"see `slurm_nodes_total` for the cluster-wide count",
	}, "\n"), -1)

	var names []string
	for _, m := range found {
		names = append(names, m[1])
	}
	require.Equal(t, []string{"slurm_queue_failed", "slurm_nodes_total"}, names)
}
