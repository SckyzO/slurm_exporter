package collector

import (
	"os"
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
