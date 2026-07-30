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
	// gresTotal and gresUsed are keyed "type" or "type:model", as parsed by
	// parseGRESByType. nil when the node exposes no GRES, which is the common
	// case on a CPU cluster and the reason nothing is allocated for it.
	gresTotal map[string]uint64
	gresUsed  map[string]uint64
}

func NodeGetMetrics(logger *logger.Logger) (map[string]*NodeMetrics, error) {
	data, err := NodeData(logger)
	if err != nil {
		return nil, err
	}
	return ParseNodeMetrics(data), nil
}

// ParseNodeMetrics takes the output of sinfo with node data
// It returns a map of metrics per node, including partitions
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

		if _, exists := nodes[nodeName]; !exists {
			nodes[nodeName] = &NodeMetrics{nodeStatus: nodeStatus, partitions: []string{}}
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

		// GRES columns are optional: a Slurm build without GRES configured, or
		// an older sinfo, simply returns fewer fields. Reading them defensively
		// keeps the CPU metrics working rather than dropping the whole line.
		if len(node) >= 8 {
			nodes[nodeName].gresTotal = parseGRESByType(node[6])
			nodes[nodeName].gresUsed = parseGRESByType(node[7])
		}

		nodes[nodeName].partitions = appendUnique(nodes[nodeName].partitions, partition)
	}

	return nodes
}

/*
NodeData executes the sinfo command to get detailed data for each node.
Expected sinfo output format: space-separated columns
"NodeList AllocMem Memory CPUsState StateLong Partition".

The trailing ":" on each field forces sinfo to use variable column widths;
without it, long node names (>20 chars, the default NodeList width) collide
with the next column and produce <6 whitespace-separated tokens — silently
dropping those nodes. See https://github.com/SckyzO/slurm_exporter/issues/10.
*/
func NodeData(logger *logger.Logger) ([]byte, error) {
	args := []string{"-h", "-N", "-O",
		"NodeList: ,AllocMem: ,Memory: ,CPUsState: ,StateLong: ,Partition: ,Gres: ,GresUsed:"}
	return Execute(logger, "sinfo", args)
}

type NodeCollector struct {
	cpuAlloc   *prometheus.Desc
	cpuIdle    *prometheus.Desc
	cpuOther   *prometheus.Desc
	cpuTotal   *prometheus.Desc
	memAlloc   *prometheus.Desc
	memTotal   *prometheus.Desc
	nodeStatus *prometheus.Desc
	gresTotal  *prometheus.Desc
	gresUsed   *prometheus.Desc
	// withGRES gates the two GRES metrics. They add a series per
	// (node, status, partition, gres_type), which multiplies on a cluster
	// exposing several GPU models or MIG profiles, so operators can turn them
	// off the same way --collector.nodes.feature-set works.
	withGRES bool
	logger   *logger.Logger
}

func NewNodeCollector(logger *logger.Logger, withGRES bool) *NodeCollector {
	labels := []string{"node", "status", "partition"}
	gresLabels := []string{"node", "status", "partition", "gres_type"}
	return &NodeCollector{
		cpuAlloc:   prometheus.NewDesc("slurm_node_cpu_alloc", "Allocated CPUs per node", labels, nil),
		cpuIdle:    prometheus.NewDesc("slurm_node_cpu_idle", "Idle CPUs per node", labels, nil),
		cpuOther:   prometheus.NewDesc("slurm_node_cpu_other", "Other CPUs per node", labels, nil),
		cpuTotal:   prometheus.NewDesc("slurm_node_cpu_total", "Total CPUs per node", labels, nil),
		memAlloc:   prometheus.NewDesc("slurm_node_mem_alloc", "Allocated memory per node", labels, nil),
		memTotal:   prometheus.NewDesc("slurm_node_mem_total", "Total memory per node", labels, nil),
		nodeStatus: prometheus.NewDesc("slurm_node_status", "Node Status with partition", labels, nil),
		gresTotal: prometheus.NewDesc("slurm_node_gres_total",
			"Generic resources configured on the node, by resource type. "+
				"gres_type is \"gpu\" for an untyped resource and \"gpu:<model>\" when Slurm reports a model.",
			gresLabels, nil),
		gresUsed: prometheus.NewDesc("slurm_node_gres_used",
			"Generic resources currently allocated on the node, by resource type. "+
				"Same gres_type values as slurm_node_gres_total.",
			gresLabels, nil),
		withGRES: withGRES,
		logger:   logger,
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
	if nc.withGRES {
		ch <- nc.gresTotal
		ch <- nc.gresUsed
	}
}

func (nc *NodeCollector) Collect(ch chan<- prometheus.Metric) { _ = nc.tryCollect(ch) }

func (nc *NodeCollector) tryCollect(ch chan<- prometheus.Metric) error {
	nodes, err := NodeGetMetrics(nc.logger)
	if err != nil {
		nc.logger.Error("Failed to get node metrics", "err", err)
		return err
	}
	if len(nodes) == 0 {
		nc.logger.Warn("node collector parsed zero nodes — sinfo returned no data or output format unexpected; no slurm_node_* series will be exposed this scrape")
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

			if !nc.withGRES {
				continue
			}
			// Ranging a nil map is a no-op, so a node with no GRES costs
			// nothing here and publishes no series.
			for gresType, count := range metrics.gresTotal {
				ch <- prometheus.MustNewConstMetric(nc.gresTotal, prometheus.GaugeValue, float64(count), node, metrics.nodeStatus, partition, gresType)
			}
			for gresType, count := range metrics.gresUsed {
				ch <- prometheus.MustNewConstMetric(nc.gresUsed, prometheus.GaugeValue, float64(count), node, metrics.nodeStatus, partition, gresType)
			}
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
