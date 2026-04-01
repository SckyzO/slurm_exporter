package collector

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// DrainReasonMetrics holds the drain/down reason for a single node.
type DrainReasonMetrics struct {
	Node      string
	Partition string
	Reason    string
	Since     string // ISO8601 timestamp when the reason was set
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
			Node:   node,
			Reason: reason,
			Since:  since,
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
	logger *logger.Logger
}

// NewDrainReasonCollector creates a DrainReasonCollector.
func NewDrainReasonCollector(log *logger.Logger) *DrainReasonCollector {
	return &DrainReasonCollector{
		info: prometheus.NewDesc(
			"slurm_node_drain_reason_info",
			"Information about why a node is in drain or down state. "+
				"Value is always 1 — use labels for the reason and timestamp.",
			[]string{"node", "reason", "since"},
			nil,
		),
		logger: log,
	}
}

func (c *DrainReasonCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.info
}

func (c *DrainReasonCollector) Collect(ch chan<- prometheus.Metric) {
	data, err := DrainReasonData(c.logger)
	if err != nil {
		c.logger.Error("Failed to get drain reason data", "err", err)
		return
	}
	metrics := ParseDrainReasonMetrics(data)
	for _, m := range metrics {
		ch <- prometheus.MustNewConstMetric(
			c.info,
			prometheus.GaugeValue,
			1,
			m.Node, m.Reason, m.Since,
		)
	}
}
