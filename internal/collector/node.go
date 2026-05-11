package collector

import (
	"regexp"
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
	gresTotal  map[string]uint64
	gresUsed   map[string]uint64
	nodeStatus string
	partitions []string
}

func NodeGetMetrics(logger *logger.Logger) (map[string]*NodeMetrics, error) {
	data, err := NodeData(logger)
	if err != nil {
		return nil, err
	}
	return ParseNodeMetrics(data), nil
}

// gresRe extracts type, optional model, and count from a GRES token.
// Handles: "gpu:4", "gpu:A30:4(IDX:0-3)", "gpu:(null):2(S:0-1)".
var gresRe = regexp.MustCompile(`(\w+):(\(null\)|[^:(]*):?([0-9]+)(?:\([^)]*\))?`)

func parseGresString(gresStr string) map[string]uint64 {
	result := make(map[string]uint64)
	if gresStr == "" || gresStr == "N/A" || gresStr == "(null)" {
		return result
	}
	for _, m := range gresRe.FindAllStringSubmatch(gresStr, -1) {
		count, err := strconv.ParseUint(m[3], 10, 64)
		if err != nil {
			continue
		}
		key := m[1]
		if m[2] != "" && m[2] != "(null)" {
			key = m[1] + ":" + m[2]
		}
		result[key] += count
	}
	return result
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
		partition := node[5]  // Partition name

		if _, exists := nodes[nodeName]; !exists {
			nodes[nodeName] = &NodeMetrics{
				gresTotal:  make(map[string]uint64),
				gresUsed:   make(map[string]uint64),
				nodeStatus: nodeStatus,
				partitions: []string{},
			}
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

		if len(node) >= 7 {
			nodes[nodeName].gresTotal = parseGresString(node[6])
		}
		if len(node) >= 8 {
			nodes[nodeName].gresUsed = parseGresString(node[7])
		}

		nodes[nodeName].partitions = appendUnique(nodes[nodeName].partitions, partition)
	}

	return nodes
}

/*
NodeData executes the sinfo command to get detailed data for each node.
Expected sinfo output format: "NodeList,AllocMem,Memory,CPUsState,StateLong,Partition,Gres,GresUsed".
*/
func NodeData(logger *logger.Logger) ([]byte, error) {
	args := []string{"-h", "-N", "-O", "NodeList,AllocMem,Memory,CPUsState,StateLong,Partition,Gres:60,GresUsed:80"}
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
	logger     *logger.Logger
}

func NewNodeCollector(logger *logger.Logger) *NodeCollector {
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
		gresTotal:  prometheus.NewDesc("slurm_node_gres_total", "Total GRES per node", gresLabels, nil),
		gresUsed:   prometheus.NewDesc("slurm_node_gres_used", "Used GRES per node", gresLabels, nil),
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
	ch <- nc.gresTotal
	ch <- nc.gresUsed
}

func (nc *NodeCollector) Collect(ch chan<- prometheus.Metric) {
	nodes, err := NodeGetMetrics(nc.logger)
	if err != nil {
		nc.logger.Error("Failed to get node metrics", "err", err)
		return
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

			for gresType, count := range metrics.gresTotal {
				ch <- prometheus.MustNewConstMetric(nc.gresTotal, prometheus.GaugeValue, float64(count), node, metrics.nodeStatus, partition, gresType)
			}
			for gresType, count := range metrics.gresUsed {
				ch <- prometheus.MustNewConstMetric(nc.gresUsed, prometheus.GaugeValue, float64(count), node, metrics.nodeStatus, partition, gresType)
			}
		}
	}
}

// appendUnique adds a string to a slice if it doesn't already exist.
func appendUnique(slice []string, value string) []string {
	if slices.Contains(slice, value) {
		return slice
	}
	return append(slice, value)
}
