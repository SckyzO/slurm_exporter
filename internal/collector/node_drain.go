package collector

import (
	"slices"
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

/*
ParseDrainReasonMetrics parses the old "sinfo -h -N -o '%N|%E|%H|%T'" output.
Only nodes with a non-empty, non-"none" reason are returned.

Nothing in the exporter calls this any more — #254 replaced the per-node sinfo
call it reads, which ran at 4.1 s against a 5 s default timeout and so was
killed on some scrapes. It is kept as the reference implementation the
equivalence test compares BuildDrainReasonMetrics against, over a capture of
both commands taken at the same moment.
*/
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

/*
BuildDrainReasonMetrics assembles the drain/down reasons from the two cheap
commands that replaced `sinfo -N` (#254): `scontrol show nodes -o` for the
reason and its timestamp, `sinfo -h -o "%N|%T"` for the state string.

The filter is the one the old path applied, on the same sinfo state string:
only nodes sinfo reports as down or drained, and only those carrying a reason
that says something. Results are sorted by node name so the output is
deterministic, which map iteration is not.
*/
func BuildDrainReasonMetrics(scontrolOut, stateOut []byte) []DrainReasonMetrics {
	nodes := ParseScontrolNodes(scontrolOut)
	states := ParseNodeStates(stateOut)

	names := make([]string, 0, len(nodes))
	for name := range nodes {
		names = append(names, name)
	}
	slices.Sort(names)

	var results []DrainReasonMetrics
	for _, name := range names {
		n := nodes[name]
		state, _ := nodeStateFor(n, states)
		// Only interested in degraded node states
		if !nodeStateDown.MatchString(state) && !nodeStateDrain.MatchString(state) {
			continue
		}
		reasonLower := strings.ToLower(n.reason)
		if n.reason == "" || reasonLower == "none" || reasonLower == "not responding" || reasonLower == "unknown" {
			continue
		}
		results = append(results, DrainReasonMetrics{
			Node:   n.name,
			Reason: n.reason,
			Since:  n.reasonSince,
		})
	}
	return results
}

// DrainReasonGetMetrics fetches and assembles the drain/down reasons.
func DrainReasonGetMetrics(log *logger.Logger) ([]DrainReasonMetrics, error) {
	scontrolOut, err := ScontrolNodesData(log)
	if err != nil {
		return nil, err
	}
	stateOut, err := NodeStateData(log)
	if err != nil {
		return nil, err
	}
	return BuildDrainReasonMetrics(scontrolOut, stateOut), nil
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

func (c *DrainReasonCollector) Collect(ch chan<- prometheus.Metric) { _ = c.tryCollect(ch) }

func (c *DrainReasonCollector) tryCollect(ch chan<- prometheus.Metric) error {
	metrics, err := DrainReasonGetMetrics(c.logger)
	if err != nil {
		c.logger.Error("Failed to get drain reason data", "err", err)
		return err
	}
	for _, m := range metrics {
		ch <- prometheus.MustNewConstMetric(
			c.info,
			prometheus.GaugeValue,
			1,
			m.Node, m.Reason, m.Since,
		)
	}

	return nil
}
