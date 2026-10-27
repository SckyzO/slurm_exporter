package collector

import (
	"slices"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// NodeMetrics stores metrics for each node
type NodeMetrics struct {
	memAlloc   uint64
	memTotal   uint64
	cpuAlloc   uint64
	cpuIdle    uint64
	cpuOther   uint64
	cpuTotal   uint64
	nodeStatus string
	partitions []string
}

func NodeGetMetrics(logger *logger.Logger) (map[string]*NodeMetrics, int, error) {
	scontrolOut, err := ScontrolNodesData(logger)
	if err != nil {
		return nil, 0, err
	}
	stateOut, err := NodeStateData(logger)
	if err != nil {
		return nil, 0, err
	}
	nodes, derived := BuildNodeMetrics(scontrolOut, stateOut)
	return nodes, derived, nil
}

/*
BuildNodeMetrics assembles the per-node metrics from the two cheap commands
that replaced `sinfo -N` (#254): `scontrol show nodes -o` for the values and
`sinfo -h -o "%N|%T"` for the state string.

Every label and every value is the same as the old path produced. The mapping
was checked field by field against paired captures of both commands taken at
the same moment, which is what node_scontrol_equivalence_test.go asserts.

derivedStates counts the nodes whose state string had to be derived from
scontrol because the grouped sinfo call did not list them. It should always be
zero; the caller logs it when it is not, because the alternative to a visible
count is a status label that quietly stops matching the dashboards.
*/
func BuildNodeMetrics(scontrolOut, stateOut []byte) (nodes map[string]*NodeMetrics, derivedStates int) {
	states := ParseNodeStates(stateOut)
	nodes = make(map[string]*NodeMetrics)

	for name, n := range ParseScontrolNodes(scontrolOut) {
		// A node in no partition produced no row under `sinfo -N` and
		// produces no series here either: every slurm_node_* metric carries
		// a partition label.
		if len(n.partitions) == 0 {
			continue
		}
		state, fromSinfo := nodeStateFor(n, states)
		if !fromSinfo {
			derivedStates++
		}
		cpuIdle, cpuOther := n.cpuIdleOther()
		nodes[name] = &NodeMetrics{
			memAlloc:   n.memAlloc,
			memTotal:   n.memTotal,
			cpuAlloc:   n.cpuAlloc,
			cpuIdle:    cpuIdle,
			cpuOther:   cpuOther,
			cpuTotal:   n.cpuTotal,
			nodeStatus: state,
			partitions: n.partitions,
		}
	}
	return nodes, derivedStates
}

/*
ParseNodeMetrics parses the old `sinfo -N` node output into the same metrics
map BuildNodeMetrics now produces.

Nothing in the exporter calls this any more: the command it parses is the one
#254 removed, because asking sinfo for the partition column of every node took
one to two minutes on a large cluster and the collector never completed.

It is kept as the reference implementation the equivalence test compares the
new path against. Asserting hand-written expectations instead would only prove
the new parser matches a transcription of the old one; running both over the
same capture proves it matches the code that shipped. Delete it and that proof
goes with it.
*/
func ParseNodeMetrics(input []byte) map[string]*NodeMetrics {
	nodes := make(map[string]*NodeMetrics)
	lines := strings.Split(string(input), "\n")

	// Sort and remove all the duplicates from the 'sinfo' output
	slices.Sort(lines)
	linesUniq := slices.Compact(lines)

	for _, line := range linesUniq {
		node := strings.Fields(line)
		if len(node) < 6 {
			continue
		}
		nodeName := node[0]
		nodeStatus := node[4] // mixed, allocated, etc.
		// Strip default-partition "*" marker (sinfo %P); matches nodes.go.
		partition := strings.TrimRight(node[5], "*")

		// Create new node metrics if it doesn't exist
		if _, exists := nodes[nodeName]; !exists {
			nodes[nodeName] = &NodeMetrics{0, 0, 0, 0, 0, 0, nodeStatus, []string{}}
		}

		memAlloc, _ := strconv.ParseUint(node[1], 10, 64)
		memTotal, _ := strconv.ParseUint(node[2], 10, 64)

		cpuInfo := strings.Split(node[3], "/")
		if len(cpuInfo) < 4 {
			continue
		}
		cpuAlloc, _ := strconv.ParseUint(cpuInfo[0], 10, 64)
		cpuIdle, _ := strconv.ParseUint(cpuInfo[1], 10, 64)
		cpuOther, _ := strconv.ParseUint(cpuInfo[2], 10, 64)
		cpuTotal, _ := strconv.ParseUint(cpuInfo[3], 10, 64)

		nodes[nodeName].memAlloc = memAlloc
		nodes[nodeName].memTotal = memTotal
		nodes[nodeName].cpuAlloc = cpuAlloc
		nodes[nodeName].cpuIdle = cpuIdle
		nodes[nodeName].cpuOther = cpuOther
		nodes[nodeName].cpuTotal = cpuTotal

		// Add the partition if it's not already in the list
		nodes[nodeName].partitions = appendUnique(nodes[nodeName].partitions, partition)
	}

	return nodes
}

type NodeCollector struct {
	cpuAlloc   *prometheus.Desc
	cpuIdle    *prometheus.Desc
	cpuOther   *prometheus.Desc
	cpuTotal   *prometheus.Desc
	memAlloc   *prometheus.Desc
	memTotal   *prometheus.Desc
	nodeStatus *prometheus.Desc
	logger     *logger.Logger
}

func NewNodeCollector(logger *logger.Logger) *NodeCollector {
	labels := []string{"node", "status", "partition"}
	return &NodeCollector{
		cpuAlloc:   prometheus.NewDesc("slurm_node_cpu_alloc", "Allocated CPUs per node", labels, nil),
		cpuIdle:    prometheus.NewDesc("slurm_node_cpu_idle", "Idle CPUs per node", labels, nil),
		cpuOther:   prometheus.NewDesc("slurm_node_cpu_other", "Other CPUs per node", labels, nil),
		cpuTotal:   prometheus.NewDesc("slurm_node_cpu_total", "Total CPUs per node", labels, nil),
		memAlloc:   prometheus.NewDesc("slurm_node_mem_alloc", "Allocated memory per node", labels, nil),
		memTotal:   prometheus.NewDesc("slurm_node_mem_total", "Total memory per node", labels, nil),
		nodeStatus: prometheus.NewDesc("slurm_node_status", "Node Status with partition", labels, nil),
		logger:     logger,
	}
}

func (nc *NodeCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- nc.cpuAlloc
	ch <- nc.cpuIdle
	ch <- nc.cpuOther
	ch <- nc.cpuTotal
	ch <- nc.memAlloc
	ch <- nc.memTotal
	ch <- nc.nodeStatus
}

func (nc *NodeCollector) Collect(ch chan<- prometheus.Metric) { _ = nc.tryCollect(ch) }

func (nc *NodeCollector) tryCollect(ch chan<- prometheus.Metric) error {
	nodes, derivedStates, err := NodeGetMetrics(nc.logger)
	if err != nil {
		nc.logger.Error("Failed to get node metrics", "err", err)
		return err
	}
	if len(nodes) == 0 {
		nc.logger.Warn("node collector parsed zero nodes — scontrol returned no data or output format unexpected; no slurm_node_* series will be exposed this scrape")
	}
	if derivedStates > 0 {
		nc.logger.Warn("nodes absent from the grouped sinfo state output; their status label was derived from scontrol and may not match sinfo's wording", "nodes", derivedStates)
	}
	for node, metrics := range nodes {
		for _, partition := range metrics.partitions {
			ch <- prometheus.MustNewConstMetric(nc.cpuAlloc, prometheus.GaugeValue, float64(metrics.cpuAlloc), node, metrics.nodeStatus, partition)
			ch <- prometheus.MustNewConstMetric(nc.cpuIdle, prometheus.GaugeValue, float64(metrics.cpuIdle), node, metrics.nodeStatus, partition)
			ch <- prometheus.MustNewConstMetric(nc.cpuOther, prometheus.GaugeValue, float64(metrics.cpuOther), node, metrics.nodeStatus, partition)
			ch <- prometheus.MustNewConstMetric(nc.cpuTotal, prometheus.GaugeValue, float64(metrics.cpuTotal), node, metrics.nodeStatus, partition)
			ch <- prometheus.MustNewConstMetric(nc.memAlloc, prometheus.GaugeValue, float64(metrics.memAlloc), node, metrics.nodeStatus, partition)
			ch <- prometheus.MustNewConstMetric(nc.memTotal, prometheus.GaugeValue, float64(metrics.memTotal), node, metrics.nodeStatus, partition)
			ch <- prometheus.MustNewConstMetric(nc.nodeStatus, prometheus.GaugeValue, 1, node, metrics.nodeStatus, partition)
		}
	}

	return nil
}

// appendUnique adds a string to a slice if it doesn't already exist.
func appendUnique(slice []string, value string) []string {
	if slices.Contains(slice, value) {
		return slice
	}
	return append(slice, value)
}
