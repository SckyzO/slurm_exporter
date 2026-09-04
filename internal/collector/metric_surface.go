package collector

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// The metric surface is the set of series this exporter can publish: every
// metric name, with the labels that vary on it, for every collector.
//
// It is derived from Describe() rather than from a scrape. A scrape only shows
// what the cluster happened to have — no reservations configured means no
// reservation metrics — so comparing two scrapes cannot tell a metric that was
// removed from one that had nothing to report. Describe() answers what the code
// declares, which is the question a non-regression check needs.
//
// The result is rendered to docs/metric-surface.md by tools/gen-metric-surface
// and checked for staleness in CI, so renaming a metric or adding a label shows
// up as a diff on that file in the pull request that does it.

// SurfaceVariant is one collector built one way.
//
// Five collectors change their own surface depending on a flag: the cardinality
// knobs either gate whole metrics (node's GRES pair, fairshare's user pair) or
// change the label set of every metric they own (nodes, queue). Recording only
// the default build would leave the other half of each knob unchecked, which is
// the half an operator reaches for on a large cluster.
type SurfaceVariant struct {
	// Collector is the name used by --collector.<name>, so an entry here can
	// be matched against the constructor table in cmd/slurm_exporter.
	Collector string
	// Flags describes the build. Empty means the shipped defaults.
	Flags string
	// New builds the collector. Durations are irrelevant to the surface, so
	// sacct_efficiency gets arbitrary ones.
	New func(*logger.Logger) prometheus.Collector
}

// MetricSurfaceVariants lists every collector, and every build of a collector
// whose surface depends on a flag.
//
// TestSurfaceVariantsMatchConstructors in cmd/slurm_exporter asserts this
// covers exactly the collectors main.go registers, so a new collector cannot be
// added to the exporter without appearing here.
func MetricSurfaceVariants() []SurfaceVariant {
	return []SurfaceVariant{
		{"accounts", "", func(l *logger.Logger) prometheus.Collector { return NewAccountsCollector(l) }},
		{"cpus", "", func(l *logger.Logger) prometheus.Collector { return NewCPUsCollector(l) }},
		{"drain_reason", "", func(l *logger.Logger) prometheus.Collector { return NewDrainReasonCollector(l) }},
		{"fairshare", "", func(l *logger.Logger) prometheus.Collector { return NewFairShareCollector(l, true) }},
		{"fairshare", "--no-collector.fairshare.user-metrics", func(l *logger.Logger) prometheus.Collector {
			return NewFairShareCollector(l, false)
		}},
		{"gpus", "", func(l *logger.Logger) prometheus.Collector { return NewGPUsCollector(l) }},
		{"info", "", func(l *logger.Logger) prometheus.Collector { return NewSlurmInfoCollector(l) }},
		{"licenses", "", func(l *logger.Logger) prometheus.Collector { return NewLicensesCollector(l) }},
		{"node", "", func(l *logger.Logger) prometheus.Collector { return NewNodeCollector(l, true) }},
		{"node", "--no-collector.node.gres", func(l *logger.Logger) prometheus.Collector {
			return NewNodeCollector(l, false)
		}},
		{"nodes", "", func(l *logger.Logger) prometheus.Collector { return NewNodesCollector(l, true) }},
		{"nodes", "--no-collector.nodes.feature-set", func(l *logger.Logger) prometheus.Collector {
			return NewNodesCollector(l, false)
		}},
		{"partitions", "", func(l *logger.Logger) prometheus.Collector { return NewPartitionsCollector(l) }},
		// --collector.queue.terminal-states deliberately gets no variant: it
		// changes which states squeue is asked for, not what the collector
		// declares. Both settings produce the same 35 metrics, so a second
		// build would add a copy of an existing section and nothing else.
		{"queue", "", func(l *logger.Logger) prometheus.Collector { return NewQueueCollector(l, true, true) }},
		{"queue", "--no-collector.queue.user-label", func(l *logger.Logger) prometheus.Collector {
			return NewQueueCollector(l, false, true)
		}},
		{"reservation_nodes", "", func(l *logger.Logger) prometheus.Collector {
			return NewReservationNodesCollector(l)
		}},
		{"reservations", "", func(l *logger.Logger) prometheus.Collector { return NewReservationsCollector(l) }},
		{"sacct_efficiency", "", func(l *logger.Logger) prometheus.Collector {
			return NewSacctEfficiencyCollector(l, 5*time.Minute, time.Hour)
		}},
		{"scheduler", "", func(l *logger.Logger) prometheus.Collector { return NewSchedulerCollector(l) }},
		{"users", "", func(l *logger.Logger) prometheus.Collector { return NewUsersCollector(l) }},
	}
}

// MetricSurfaceInternals lists the exporter's own instrumentation: the metrics
// it publishes about itself rather than about Slurm.
//
// They are kept apart from MetricSurfaceVariants because they are not behind a
// --collector.<name> flag and have no entry in main.go's constructor table, so
// folding them in would break the check that ties that table to this one. They
// belong in the surface all the same: slurm_exporter_collector_success is what
// the health dashboard and the "is the exporter working" alert read, and #138
// was a bug in exactly that metric.
//
// The Go runtime, process and build-info collectors are not listed. They come
// from client_golang, this project does not name them, and a change there is
// upstream's to announce.
func MetricSurfaceInternals() []SurfaceVariant {
	return []SurfaceVariant{
		{"collector status", "", func(l *logger.Logger) prometheus.Collector { return NewStatusTracker(l) }},
		{"command execution", "", func(*logger.Logger) prometheus.Collector { return execDuration }},
		{"command errors", "", func(*logger.Logger) prometheus.Collector { return execErrors }},
		{"cache age", "", func(*logger.Logger) prometheus.Collector { return cacheAgeGauge }},
	}
}

// Metric is one entry of the surface: a name and the labels that vary on it.
type Metric struct {
	Name   string
	Labels []string
}

// String renders a metric the way the generated document shows it.
func (m Metric) String() string {
	if len(m.Labels) == 0 {
		return m.Name
	}
	return m.Name + "{" + strings.Join(m.Labels, ",") + "}"
}

// descPattern pulls the two fields that make up the surface out of
// Desc.String(), whose format is
//
//	Desc{fqName: %q, help: %q, unit: %q, constLabels: {…}, variableLabels: {…}}
//
// fqName is captured as the quoted literal and unquoted afterwards rather than
// matched as bare characters, because a metric name is Go-quoted here and the
// help text between it and the labels can itself contain quotes and braces.
//
// The `.*` before variableLabels is greedy on purpose. Help text and constant
// label values sit between the two captures and can contain the literal string
// "variableLabels: {"; greedy matching takes the last occurrence, which is
// always the real field, since nothing follows it. A lazy match would take a
// forgery in the help text instead. TestParseDescHandlesAwkwardHelp covers
// both that and the quoting.
var descPattern = regexp.MustCompile(`^Desc\{fqName: ("(?:[^"\\]|\\.)*"), .*variableLabels: \{([^}]*)\}\}$`)

// parseDesc reads a metric name and its variable labels out of a Desc.
//
// prometheus.Desc keeps both unexported with no accessor, so its String() is
// the only supported way to reach them.
func parseDesc(d *prometheus.Desc) (Metric, error) {
	return parseDescString(d.String())
}

// parseDescString is parseDesc's body, split out so the failure paths can be
// tested. They cannot be reached through a real Desc: prometheus.NewDesc
// renders the same format whatever it is given, so even an invalid metric name
// or label produces a well-formed string. The only thing that can break this
// parser is client_golang changing Desc.String(), which no Desc this package
// can build will simulate.
//
// TestDescStringFormatIsAsExpected pins that upstream format, so a dependency
// bump that changes it fails there with an explanation rather than here with a
// regexp that no longer matches.
func parseDescString(s string) (Metric, error) {
	m := descPattern.FindStringSubmatch(s)
	if m == nil {
		return Metric{}, fmt.Errorf("unrecognised Desc format: %s", s)
	}
	name, err := strconv.Unquote(m[1])
	if err != nil {
		return Metric{}, fmt.Errorf("fqName is not a Go string literal in %s: %w", s, err)
	}
	var labels []string
	if trimmed := strings.TrimSpace(m[2]); trimmed != "" {
		labels = strings.Split(trimmed, ",")
	}
	return Metric{Name: name, Labels: labels}, nil
}

// SurfaceOf returns the metrics a collector declares, sorted by name so the
// generated document does not churn when a Describe() implementation reorders
// its sends.
func SurfaceOf(c prometheus.Collector) ([]Metric, error) {
	// Describe runs on this goroutine and the collecting one drains until the
	// channel closes, so every send completes and nothing is left running when
	// SurfaceOf returns.
	//
	// The obvious arrangement is the other way round — Describe in a goroutine,
	// parse each Desc as it arrives — and it leaks that goroutine on any early
	// return, blocked forever on a send nobody will receive. The only early
	// return here is a parse failure, which is precisely the moment nobody is
	// watching for a leaked goroutine. Collecting first and parsing after costs
	// one slice and removes the failure mode.
	ch := make(chan *prometheus.Desc)
	var descs []*prometheus.Desc
	done := make(chan struct{})
	go func() {
		defer close(done)
		for d := range ch {
			descs = append(descs, d)
		}
	}()
	c.Describe(ch)
	close(ch)
	<-done // also the happens-before edge that publishes descs to this goroutine

	metrics := make([]Metric, 0, len(descs))
	for _, d := range descs {
		m, err := parseDesc(d)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, m)
	}
	sort.Slice(metrics, func(i, j int) bool {
		if metrics[i].Name != metrics[j].Name {
			return metrics[i].Name < metrics[j].Name
		}
		return strings.Join(metrics[i].Labels, ",") < strings.Join(metrics[j].Labels, ",")
	})
	return metrics, nil
}

// RenderMetricSurface produces the document written to docs/metric-surface.md.
//
// It lives here rather than in the generator so the test suite can assert the
// committed file is current without shelling out to the tool: a contributor who
// runs `go test ./...` and nothing else still learns that a metric moved.
func RenderMetricSurface() ([]byte, error) {
	var b strings.Builder
	line := func(s string) { b.WriteString(s + "\n") }

	line("# Metric surface")
	line("")
	line("<!-- Generated by tools/gen-metric-surface. Do not edit by hand. -->")
	line("<!-- Run `make generate` after changing what a collector declares. -->")
	line("")
	line("Every metric each collector can publish, with the labels that vary on it,")
	line("taken from the collectors' `Describe()` output.")
	line("")
	line("This is the declared surface, not a scrape. A scrape shows only what the")
	line("cluster happened to have: with no reservations configured, a deleted")
	line("reservation metric and a reservation metric with nothing to report look")
	line("exactly alike. `Describe()` tells them apart, which is what makes this file")
	line("usable as a non-regression baseline.")
	line("")
	line("A change here is a change operators can see. Renaming a metric, adding or")
	line("removing a label, or gating a metric behind a new flag all show up as a diff")
	line("on this file in the pull request that does it. A change that leaves it")
	line("untouched has moved nothing anyone scrapes.")
	line("")
	line("Collectors whose surface depends on a cardinality flag appear once per")
	line("setting, so both halves of the knob are covered.")
	line("")
	line("The Go runtime, process and build-info collectors are not listed: they come")
	line("from client_golang, this project does not name them, and a change there is")
	line("upstream's to announce.")
	line("")
	line("## Slurm collectors")
	line("")

	log := logger.NewTextLogger("error")
	total := 0

	section := func(variants []SurfaceVariant) error {
		for _, v := range variants {
			title := "### " + v.Collector
			if v.Flags != "" {
				title += " — with `" + v.Flags + "`"
			}
			line(title)
			line("")

			metrics, err := SurfaceOf(v.New(log))
			if err != nil {
				return fmt.Errorf("%s (%s): %w", v.Collector, v.Flags, err)
			}
			if len(metrics) == 0 {
				return fmt.Errorf("%s (%s) declares no metrics", v.Collector, v.Flags)
			}
			line("```")
			for _, m := range metrics {
				line(m.String())
			}
			line("```")
			line("")
			total += len(metrics)
		}
		return nil
	}

	if err := section(MetricSurfaceVariants()); err != nil {
		return nil, err
	}

	line("## Exporter internals")
	line("")
	line("What the exporter publishes about itself. Not behind a `--collector.<name>`")
	line("flag, and read by the health dashboard and the alerting rules rather than by")
	line("anyone asking about Slurm.")
	line("")
	if err := section(MetricSurfaceInternals()); err != nil {
		return nil, err
	}

	line(fmt.Sprintf("%d declarations across %d builds.",
		total, len(MetricSurfaceVariants())+len(MetricSurfaceInternals())))
	return []byte(b.String()), nil
}
