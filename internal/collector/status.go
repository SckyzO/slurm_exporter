package collector

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// StatusCollector wraps any prometheus.Collector and emits per-collector
// health metrics: slurm_exporter_collector_success and
// slurm_exporter_collector_duration_seconds.
//
// This follows the Prometheus exporter best-practice of exposing internal
// scrape health so operators can build alerts on individual collector failures
// without waiting for a global scrape_error.
type StatusCollector struct {
	inner    prometheus.Collector
	name     string
	logger   *logger.Logger
	success  *prometheus.Desc
	duration *prometheus.Desc
}

// WrapWithStatus wraps a collector with instrumentation that tracks success
// and duration per named collector.
func WrapWithStatus(name string, c prometheus.Collector, log *logger.Logger) *StatusCollector {
	return &StatusCollector{
		inner:  c,
		name:   name,
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

// Describe forwards the inner collector's descriptors plus the two status descs.
func (s *StatusCollector) Describe(ch chan<- *prometheus.Desc) {
	s.inner.Describe(ch)
	ch <- s.success
	ch <- s.duration
}

// Collect runs the inner collector, measures duration, and emits status metrics.
// If the inner Collect panics, it is recovered and reported as a failure.
func (s *StatusCollector) Collect(ch chan<- prometheus.Metric) {
	start := time.Now()
	succeeded := 1.0

	// Intercept metrics from the inner collector into a buffer so we can
	// detect whether it produced any output (empty = likely an error).
	buf := make(chan prometheus.Metric, 512)
	done := make(chan struct{})

	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("Collector panicked", "collector", s.name, "panic", r)
				succeeded = 0
			}
			close(done)
		}()
		s.inner.Collect(buf)
	}()

	<-done
	close(buf)

	for m := range buf {
		ch <- m
	}

	elapsed := time.Since(start).Seconds()
	ch <- prometheus.MustNewConstMetric(s.success, prometheus.GaugeValue, succeeded, s.name)
	ch <- prometheus.MustNewConstMetric(s.duration, prometheus.GaugeValue, elapsed, s.name)
}
