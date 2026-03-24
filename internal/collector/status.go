package collector

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// StatusTracker is a single Prometheus collector that runs a set of inner
// collectors and emits per-collector health metrics. Using one shared collector
// avoids duplicate descriptor panics that occur when each inner collector
// independently emits the same status metric descriptor.
type StatusTracker struct {
	entries  []statusEntry
	success  *prometheus.Desc
	duration *prometheus.Desc
	logger   *logger.Logger
}

type statusEntry struct {
	name      string
	collector prometheus.Collector
}

// NewStatusTracker creates a StatusTracker. Register it once with the Prometheus
// registry; add inner collectors via Add().
func NewStatusTracker(log *logger.Logger) *StatusTracker {
	return &StatusTracker{
		logger: log,
		success: prometheus.NewDesc(
			"slurm_exporter_collector_success",
			"Whether the last scrape of the collector succeeded (1=success, 0=failure)",
			[]string{"collector"}, nil,
		),
		duration: prometheus.NewDesc(
			"slurm_exporter_collector_duration_seconds",
			"Duration of the last scrape for the collector in seconds",
			[]string{"collector"}, nil,
		),
	}
}

// Add registers an inner collector under the given name.
func (st *StatusTracker) Add(name string, c prometheus.Collector) {
	st.entries = append(st.entries, statusEntry{name: name, collector: c})
}

// Describe sends the inner collectors' descriptors plus the two status descriptors.
func (st *StatusTracker) Describe(ch chan<- *prometheus.Desc) {
	for _, e := range st.entries {
		e.collector.Describe(ch)
	}
	ch <- st.success
	ch <- st.duration
}

// Collect runs each inner collector, measures its duration, and emits status metrics.
// Each inner collector writes directly into ch — no intermediate channel or extra
// goroutine. Panics are caught via defer/recover in the same goroutine, which is
// standard Go and avoids any buffering overhead regardless of metric volume.
func (st *StatusTracker) Collect(ch chan<- prometheus.Metric) {
	for _, e := range st.entries {
		start := time.Now()
		succeeded := 1.0

		func() {
			defer func() {
				if r := recover(); r != nil {
					st.logger.Error("Collector panicked", "collector", e.name, "panic", r)
					succeeded = 0
				}
			}()
			e.collector.Collect(ch)
		}()

		elapsed := time.Since(start).Seconds()
		ch <- prometheus.MustNewConstMetric(st.success, prometheus.GaugeValue, succeeded, e.name)
		ch <- prometheus.MustNewConstMetric(st.duration, prometheus.GaugeValue, elapsed, e.name)
	}
}
