package collector

import (
	"os"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// TestParseDescHandlesAwkwardHelp is the guard on the one fragile part of this
// file. parseDesc reads prometheus.Desc.String(), because Desc keeps fqName and
// its variable labels unexported with no accessor. String() puts the help text
// between the two fields we want, so a help string containing quotes, braces or
// the literal text of a later field is exactly what a naive pattern gets wrong.
//
// A metric whose help text broke the parser would not fail loudly: it would
// drop out of the surface document, and the baseline would quietly stop
// covering it.
func TestParseDescHandlesAwkwardHelp(t *testing.T) {
	for _, tc := range []struct {
		name string
		help string
	}{
		{"plain", "Allocated nodes"},
		{"quotes", `Nodes in state "drain" or "down"`},
		{"braces", "Nodes matching {a,b} in the set"},
		{"impersonating a later field", `x variableLabels: {spoofed} constLabels: {}`},
		{"backslash", `Path like C:\slurm\etc`},
		{"empty", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := prometheus.NewDesc("slurm_test_metric", tc.help, []string{"partition", "node"}, nil)
			got, err := parseDesc(d)
			require.NoError(t, err)
			require.Equal(t, "slurm_test_metric", got.Name)
			require.Equal(t, []string{"partition", "node"}, got.Labels)
		})
	}
}

// TestParseDescNoLabels pins the no-variable-label case, which renders as an
// empty brace pair and must not become a one-element slice containing "".
func TestParseDescNoLabels(t *testing.T) {
	got, err := parseDesc(prometheus.NewDesc("slurm_nodes_total", "Total nodes", nil, nil))
	require.NoError(t, err)
	require.Equal(t, "slurm_nodes_total", got.Name)
	require.Empty(t, got.Labels)
	require.Equal(t, "slurm_nodes_total", got.String())
}

// TestDescStringFormatIsAsExpected pins the upstream format this file parses.
//
// prometheus.Desc exposes neither its name nor its labels, so the surface is
// read out of String(). That is an upstream implementation detail: a
// client_golang bump could reword it, and the failure would land in a regexp
// with nothing explaining why. Asserting the exact rendering here means the
// bump fails on a test that names the cause.
//
// If this breaks, update descPattern to the new format, then regenerate
// docs/metric-surface.md and check the diff is empty — a format change must not
// silently alter what we consider the surface.
func TestDescStringFormatIsAsExpected(t *testing.T) {
	d := prometheus.NewDesc("slurm_nodes_alloc", "Allocated nodes", []string{"partition"}, nil)
	require.Equal(t,
		`Desc{fqName: "slurm_nodes_alloc", help: "Allocated nodes", unit: "", constLabels: {}, variableLabels: {partition}}`,
		d.String(),
		"prometheus.Desc.String() changed shape; descPattern in metric_surface.go must follow")
}

// TestParseDescStringRejectsGarbage proves the failure is loud rather than
// silent. A metric that failed to parse must stop the generator, not drop
// quietly out of the document and take its regression coverage with it.
//
// These strings cannot come from a real Desc — NewDesc renders the same shape
// whatever it is handed — so the parser is fed directly. That is the point:
// they simulate the upstream format change that TestDescStringFormatIsAsExpected
// watches for.
func TestParseDescStringRejectsGarbage(t *testing.T) {
	for _, tc := range []struct{ name, in string }{
		{"empty", ""},
		{"not a Desc", "some other type's String()"},
		{"renamed fields", `Desc{name: "slurm_x", labels: {a}}`},
		{"truncated", `Desc{fqName: "slurm_x", help: "h", unit: "", constLabels: {}`},
		{"unquoted name", `Desc{fqName: slurm_x, help: "h", unit: "", constLabels: {}, variableLabels: {a}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseDescString(tc.in)
			require.Error(t, err)
			require.Contains(t, err.Error(), "unrecognised Desc format")
		})
	}
}

// TestParseDescStringUnquotesTheName covers the escape handling the regexp
// leaves to strconv.Unquote.
func TestParseDescStringUnquotesTheName(t *testing.T) {
	got, err := parseDescString(
		`Desc{fqName: "slurm_x", help: "he said \"hi\" and {left}", unit: "", constLabels: {}, variableLabels: {a,b}}`)
	require.NoError(t, err)
	require.Equal(t, "slurm_x", got.Name)
	require.Equal(t, []string{"a", "b"}, got.Labels)
}

// TestSurfaceOfEveryVariant walks every collector build the document covers.
// It is the check that catches a collector added to the exporter whose
// Describe() sends something the parser cannot read.
func TestSurfaceOfEveryVariant(t *testing.T) {
	log := logger.NewTextLogger("error")
	for _, v := range MetricSurfaceVariants() {
		name := v.Collector
		if v.Flags != "" {
			name += " " + v.Flags
		}
		t.Run(name, func(t *testing.T) {
			metrics, err := SurfaceOf(v.New(log))
			require.NoError(t, err)
			require.NotEmpty(t, metrics, "a collector that declares nothing cannot be checked for regression")

			seen := map[string]bool{}
			for _, m := range metrics {
				require.True(t, strings.HasPrefix(m.Name, "slurm_"),
					"every metric this exporter owns is namespaced slurm_, got %q", m.Name)
				require.False(t, seen[m.String()], "duplicate declaration %s", m)
				seen[m.String()] = true
			}
		})
	}
}

// TestSurfaceOfIsSorted guards the ordering. Describe() sends in whatever order
// its implementation happens to use, so without a sort the generated document
// would churn on unrelated edits and its diff would stop meaning anything.
func TestSurfaceOfIsSorted(t *testing.T) {
	metrics, err := SurfaceOf(NewNodeCollector(logger.NewTextLogger("error"), true))
	require.NoError(t, err)
	require.NotEmpty(t, metrics)
	for i := 1; i < len(metrics); i++ {
		require.LessOrEqual(t, metrics[i-1].String(), metrics[i].String(),
			"SurfaceOf must return metrics sorted, so the generated document is stable")
	}
}

// TestFlagsChangeTheSurface proves the variants table is worth having. If a
// cardinality flag stopped changing what the collector declares, recording both
// settings would be dead weight and this test says so.
func TestFlagsChangeTheSurface(t *testing.T) {
	log := logger.NewTextLogger("error")
	for _, tc := range []struct {
		name   string
		on     prometheus.Collector
		off    prometheus.Collector
		reason string
	}{
		{"node.gres", NewNodeCollector(log, true), NewNodeCollector(log, false), "gates the GRES metrics"},
		{"fairshare.user-metrics", NewFairShareCollector(log, true), NewFairShareCollector(log, false), "gates the per-user metrics"},
		{"nodes.feature-set", NewNodesCollector(log, true), NewNodesCollector(log, false), "changes the label set"},
		{"queue.user-label", NewQueueCollector(log, true, true), NewQueueCollector(log, false, true), "changes the label set"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			on, err := SurfaceOf(tc.on)
			require.NoError(t, err)
			off, err := SurfaceOf(tc.off)
			require.NoError(t, err)
			require.NotEqual(t, on, off, "the flag %s", tc.reason)
		})
	}
}

// TestMetricSurfaceDocIsCurrent makes `go test ./...` enough to catch a metric
// or label change that was not regenerated. CI runs the same check through
// `gen-metric-surface -check`; having it here too means a contributor who never
// runs make still gets told, in the run they already do.
func TestMetricSurfaceDocIsCurrent(t *testing.T) {
	want, err := RenderMetricSurface()
	require.NoError(t, err)

	got, err := os.ReadFile("../../docs/metric-surface.md")
	require.NoError(t, err)

	require.Equal(t, string(want), string(got),
		"docs/metric-surface.md is stale: the declared metrics or labels changed.\n"+
			"Run `make generate` and commit the result, so the change is visible in review.")
}

// TestInternalMetricsAreAllInTheSurface closes the same hole for the exporter's
// own instrumentation that TestSurfaceVariantsMatchConstructors closes for the
// Slurm collectors.
//
// MetricSurfaceVariants is tied to main.go's constructor table, so a new
// collector cannot escape the surface. MetricSurfaceInternals is tied to
// nothing: it is a hand-written list, and a new slurm_exporter_* metric added
// next to the existing ones would simply not appear, taking its regression
// coverage with it and saying nothing about it.
//
// The source is scanned rather than a registry consulted, because these metrics
// are package-level vars registered from three different places
// (RegisterExecMetrics, RegisterCacheMetrics, the StatusTracker constructor);
// their names as written in the code are the one thing they have in common.
func TestInternalMetricsAreAllInTheSurface(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	namePattern := regexp.MustCompile(`"(slurm_exporter_[a-z0-9_]+)"`)
	inSource := map[string]string{} // metric name -> file it was found in
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		body, readErr := os.ReadFile(e.Name())
		require.NoError(t, readErr)
		for _, m := range namePattern.FindAllStringSubmatch(string(body), -1) {
			inSource[m[1]] = e.Name()
		}
	}
	require.NotEmpty(t, inSource, "no slurm_exporter_* metric names found in the package; has the naming changed?")

	inSurface := map[string]bool{}
	for _, v := range MetricSurfaceInternals() {
		metrics, surfErr := SurfaceOf(v.New(logger.NewTextLogger("error")))
		require.NoError(t, surfErr)
		for _, m := range metrics {
			inSurface[m.Name] = true
		}
	}

	var missing []string
	for name, file := range inSource {
		if !inSurface[name] {
			missing = append(missing, name+" (declared in "+file+")")
		}
	}
	sort.Strings(missing)
	require.Empty(t, missing,
		"these internal metrics exist in the code but are absent from "+
			"MetricSurfaceInternals(), so docs/metric-surface.md does not cover them")
}

// panickingCollector has a Describe that sends one Desc and then panics. It
// stands in for a collector with a bug in Describe — a nil map, an index out of
// range — which SurfaceOf has no control over, since it accepts any
// prometheus.Collector.
type panickingCollector struct{}

func (panickingCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- prometheus.NewDesc("slurm_probe", "probe", nil, nil)
	panic("Describe exploded")
}

func (panickingCollector) Collect(chan<- prometheus.Metric) {}

// TestSurfaceOfDoesNotLeakOnPanic is the regression test for closing the Desc
// channel with defer rather than with a plain statement after Describe returns.
//
// A plain close never runs when Describe panics, and the goroutine collecting
// from that channel then blocks forever on a receive nobody will satisfy: a
// leaked goroutine riding out on a panic, which is the least likely moment for
// anyone to be watching goroutine counts. Removing the defer makes this test
// fail, counting one goroutine more after the panic than before it.
func TestSurfaceOfDoesNotLeakOnPanic(t *testing.T) {
	before := runtime.NumGoroutine()
	require.Panics(t, func() { _, _ = SurfaceOf(panickingCollector{}) })

	// The collecting goroutine exits once the channel closes, but "once" is not
	// instant: yield until the count comes back down rather than reading it
	// straight away and flaking.
	for range 200 {
		if runtime.NumGoroutine() <= before {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("goroutine leaked: %d before the panic, %d after", before, runtime.NumGoroutine())
}
