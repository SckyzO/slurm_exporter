package collector

import (
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// DrainReasonMetrics holds the drain/down reason for a single node.
type DrainReasonMetrics struct {
	Node   string
	Reason string
	Since  string // raw sinfo "%H" field, kept for diagnostics
	// SinceUnix is Since converted to a Unix timestamp, or 0 when sinfo
	// reported no usable time. Zero means "do not export", never 1970.
	SinceUnix float64
}

// parseDrainTime converts one sinfo "%H" field into a Unix timestamp, using the
// same slurmTimeLayout every other Slurm timestamp in this package is read with.
//
// sinfo renders the field in the local zone of the host running the command,
// which is the exporter host, so that is the zone it is read back in. Slurm
// documents the "standard" SLURM_TIME_FORMAT as
// year-month-dateThour:minute:second, and Execute pins that format on every
// Slurm command, so a string this layout rejects is a layout the exporter does
// not know rather than a site that configured its environment differently.
// Those return 0, which the collector treats as "no timestamp" rather than as
// 1970.
func parseDrainTime(since string) float64 {
	t, err := time.ParseInLocation(slurmTimeLayout, since, time.Local)
	if err != nil {
		return 0
	}
	return float64(t.Unix())
}

// drainTimeIsUnset reports whether sinfo said there is no timestamp, as opposed
// to printing one this parser could not read. The first case is normal and
// silent, the second is a misconfiguration worth logging.
func drainTimeIsUnset(since string) bool {
	switch strings.ToLower(since) {
	case "", "unknown", "none", "n/a":
		return true
	}
	return false
}

// ParseDrainReasonMetrics parses "sinfo -h -N -o '%N|%E|%H|%T'" output.
// Only nodes with a non-empty, non-"none" reason are returned.
func ParseDrainReasonMetrics(input []byte) []DrainReasonMetrics {
	var results []DrainReasonMetrics
	seen := make(map[string]bool) // deduplicate by node (node can appear in multiple partitions)

	for _, line := range strings.Split(string(input), "\n") {
		if !strings.Contains(line, "|") {
			continue
		}
		fields := strings.Split(line, "|")
		if len(fields) < 4 {
			continue
		}
		node := strings.TrimSpace(fields[0])
		reason := strings.TrimSpace(fields[1])
		since := strings.TrimSpace(fields[2])
		state := strings.TrimSpace(fields[3])

		reasonLower := strings.ToLower(reason)
		if node == "" || reason == "" || reasonLower == "none" || reasonLower == "not responding" || reasonLower == "unknown" {
			continue
		}
		// Only interested in degraded node states
		if !nodeStateDown.MatchString(state) && !nodeStateDrain.MatchString(state) {
			continue
		}
		// Deduplicate: same node in multiple partitions gets the same reason
		if seen[node] {
			continue
		}
		seen[node] = true
		results = append(results, DrainReasonMetrics{
			Node:      node,
			Reason:    reason,
			Since:     since,
			SinceUnix: parseDrainTime(since),
		})
	}
	return results
}

// DrainReasonData executes sinfo to retrieve node drain/down reasons.
// Uses -N (per-node) to get one line per node.
func DrainReasonData(log *logger.Logger) ([]byte, error) {
	return Execute(log, "sinfo", []string{"-h", "-N", "-o", "%N|%E|%H|%T"})
}

// DrainReasonCollector collects slurm_node_drain_reason_info for degraded nodes.
type DrainReasonCollector struct {
	info   *prometheus.Desc
	since  *prometheus.Desc
	logger *logger.Logger
}

// NewDrainReasonCollector creates a DrainReasonCollector.
//
// The drain timestamp is a value, not a label. Carried as a label it made every
// re-drain of a node create a fresh series and orphan the previous one, so the
// series count grew with operator activity over the lifetime of the TSDB instead
// of with the number of drained nodes — see issue #141.
func NewDrainReasonCollector(log *logger.Logger) *DrainReasonCollector {
	return &DrainReasonCollector{
		info: prometheus.NewDesc(
			"slurm_node_drain_reason_info",
			"Information about why a node is in drain or down state. "+
				"Always 1; the reason label carries the text and "+
				"slurm_node_drain_since_timestamp_seconds carries the time it was set.",
			[]string{"node", "reason"},
			nil,
		),
		since: prometheus.NewDesc(
			"slurm_node_drain_since_timestamp_seconds",
			"Unix timestamp at which the drain or down reason was set on the node. "+
				"Absent when Slurm reports no timestamp.",
			[]string{"node"},
			nil,
		),
		logger: log,
	}
}

func (c *DrainReasonCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.info
	ch <- c.since
}

func (c *DrainReasonCollector) Collect(ch chan<- prometheus.Metric) { _ = c.tryCollect(ch) }

func (c *DrainReasonCollector) tryCollect(ch chan<- prometheus.Metric) error {
	data, err := DrainReasonData(c.logger)
	if err != nil {
		c.logger.Error("Failed to get drain reason data", "err", err)
		return err
	}
	metrics := ParseDrainReasonMetrics(data)
	for _, m := range metrics {
		ch <- prometheus.MustNewConstMetric(c.info, prometheus.GaugeValue, 1, m.Node, m.Reason)

		if m.SinceUnix > 0 {
			ch <- prometheus.MustNewConstMetric(c.since, prometheus.GaugeValue, m.SinceUnix, m.Node)
			continue
		}
		if !drainTimeIsUnset(m.Since) {
			c.logger.Warn("Unreadable drain timestamp from sinfo; no slurm_node_drain_since_timestamp_seconds for this node",
				"node", m.Node, "since", m.Since, "expected_layout", slurmTimeLayout,
				"hint", "the exporter pins SLURM_TIME_FORMAT=standard, so this layout is one it does not know: please report it with your Slurm version")
		}
	}

	return nil
}
