package collector

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/mike8161990/slurm_exporter/internal/logger"
	"github.com/prometheus/client_golang/prometheus"
)

// GPUsMetrics holds GPU utilization statistics from Slurm
type GPUsMetrics struct {
	alloc       float64 // Number of allocated GPUs
	idle        float64 // Number of idle GPUs
	other       float64 // Number of GPUs in other states (mixed, down, etc.)
	total       float64 // Total number of GPUs in the cluster
	utilization float64 // GPU utilization ratio (allocated/total)
}

// GPUsGetMetrics retrieves and parses GPU metrics from Slurm
func GPUsGetMetrics(logger *logger.Logger) (*GPUsMetrics, error) {
	return ParseGPUsMetrics(logger)
}

// ParseAllocatedGPUs parses the output of sinfo command to count allocated GPUs
// Expected input format examples:
//   - slurm>=20.11.8: "3 gpu:2"
//   - slurm 21.08.5:  "1 gpu:(null):3(IDX:0-7)"
//   - slurm 21.08.5:  "13 gpu:A30:4(IDX:0-3),gpu:Q6K:4(IDX:0-3)"
func ParseAllocatedGPUs(data []byte) float64 {
	var numGPUs = 0.0
	sinfoLines := string(data)
	// Regex to match GPU specifications: gpu:type:count or gpu:count
	re := regexp.MustCompile(`gpu:(\(null\)|[^:(]*):?([0-9]+)(\([^)]*\))?`)
	if len(sinfoLines) > 0 {
		for _, line := range strings.Split(sinfoLines, "\n") {
			if len(line) > 0 && strings.Contains(line, "gpu:") {
				fields := strings.Fields(line)
				if len(fields) < 2 {
					continue
				}

				numNodes, _ := strconv.ParseFloat(fields[0], 64)
				nodeActiveGPUs := fields[1]
				numNodeActiveGPUs := 0.0

				// Parse GPU specifications separated by commas
				for _, gpuSpec := range strings.Split(nodeActiveGPUs, ",") {
					if strings.Contains(gpuSpec, "gpu:") {
						matches := re.FindStringSubmatch(gpuSpec)
						if len(matches) > 2 {
							gpuCount, _ := strconv.ParseFloat(matches[2], 64)
							numNodeActiveGPUs += gpuCount
						}
					}
				}
				numGPUs += numNodes * numNodeActiveGPUs
			}
		}
	}

	return numGPUs
}

// ParseIdleGPUs calculates idle GPUs by subtracting allocated from total GPUs
// Expected input format examples:
//   - slurm 20.11.8:  "3 gpu:4 gpu:2" (total available, allocated)
//   - slurm 21.08.5:  "1 gpu:8(S:0-1) gpu:(null):3(IDX:0-7)"
//   - slurm 21.08.5:  "13 gpu:A30:4(S:0-1),gpu:Q6K:40(S:0-1) gpu:A30:4(IDX:0-3),gpu:Q6K:4(IDX:0-3)"
func ParseIdleGPUs(data []byte) float64 {
	var numGPUs = 0.0
	sinfoLines := string(data)
	re := regexp.MustCompile(`gpu:(\(null\)|[^:(]*):?([0-9]+)(\([^)]*\))?`)
	if len(sinfoLines) > 0 {
		for _, line := range strings.Split(sinfoLines, "\n") {
			if len(line) > 0 && strings.Contains(line, "gpu:") {
				fields := strings.Fields(line)
				if len(fields) < 1 {
					continue
				}

				numNodes, _ := strconv.ParseFloat(fields[0], 64)

				switch len(fields) {
				case 1:
					// Only node count, no GPU info - assume no idle GPUs
					numGPUs += 0
				case 2:
					// Two columns: nodes and total GPUs (no allocated info)
					totalGPUs := parseGPUCount(fields[1], re)
					numGPUs += numNodes * totalGPUs
				default:
					// Three or more columns: nodes, total GPUs, allocated GPUs
					totalGPUs := parseGPUCount(fields[1], re)
					allocatedGPUs := parseGPUCount(fields[2], re)
					idleGPUs := totalGPUs - allocatedGPUs
					numGPUs += numNodes * idleGPUs
				}
			}
		}
	}

	return numGPUs
}

// parseGPUCount extracts the total GPU count from a GPU specification string
func parseGPUCount(gpuSpec string, re *regexp.Regexp) float64 {
	var count = 0.0
	for _, spec := range strings.Split(gpuSpec, ",") {
		if strings.Contains(spec, "gpu:") {
			matches := re.FindStringSubmatch(spec)
			if len(matches) > 2 {
				gpuCount, _ := strconv.ParseFloat(matches[2], 64)
				count += gpuCount
			}
		}
	}
	return count
}

// ParseTotalGPUs parses the output of sinfo command to count total available GPUs
// Expected input format examples:
//   - slurm 20.11.8:  "3 gpu:4"
//   - slurm 21.08.5:  "1 gpu:8(S:0-1)"
//   - slurm 21.08.5:  "13 gpu:A30:4(S:0-1),gpu:Q6K:40(S:0-1)"
func ParseTotalGPUs(data []byte) float64 {
	var numGPUs = 0.0
	sinfoLines := string(data)
	re := regexp.MustCompile(`gpu:(\(null\)|[^:(]*):?([0-9]+)(\([^)]*\))?`)

	if len(sinfoLines) > 0 {
		for _, line := range strings.Split(sinfoLines, "\n") {
			if len(line) > 0 && strings.Contains(line, "gpu:") {
				fields := strings.Fields(line)
				if len(fields) < 2 {
					continue
				}

				numNodes, _ := strconv.ParseFloat(fields[0], 64)
				nodeGPUs := parseGPUCount(fields[1], re)
				numGPUs += numNodes * nodeGPUs
			}
		}
	}

	return numGPUs
}

// ParseGPUsMetrics collects and parses all GPU metrics from Slurm
func ParseGPUsMetrics(logger *logger.Logger) (*GPUsMetrics, error) {
	var gm GPUsMetrics

	// Get total GPU count
	totalGPUsData, err := TotalGPUsData(logger)
	if err != nil {
		return nil, err
	}
	totalGPUs := ParseTotalGPUs(totalGPUsData)

	// Get allocated GPU count
	allocatedGPUsData, err := AllocatedGPUsData(logger)
	if err != nil {
		return nil, err
	}
	allocatedGPUs := ParseAllocatedGPUs(allocatedGPUsData)

	// Get idle GPU count
	idleGPUsData, err := IdleGPUsData(logger)
	if err != nil {
		return nil, err
	}
	idleGPUs := ParseIdleGPUs(idleGPUsData)

	// Calculate other GPUs (mixed, down, etc.)
	otherGPUs := totalGPUs - allocatedGPUs - idleGPUs

	gm.alloc = allocatedGPUs
	gm.idle = idleGPUs
	gm.other = otherGPUs
	gm.total = totalGPUs

	// Calculate utilization ratio
	if totalGPUs > 0 {
		gm.utilization = allocatedGPUs / totalGPUs
	}

	return &gm, nil
}

// AllocatedGPUsData executes sinfo command to get allocated GPU information
func AllocatedGPUsData(logger *logger.Logger) ([]byte, error) {
	args := []string{"-a", "-h", "--Format=Nodes: ,GresUsed:", "--state=allocated"}
	return Execute(logger, "sinfo", args)
}

// IdleGPUsData executes sinfo command to get idle and allocated GPU information
func IdleGPUsData(logger *logger.Logger) ([]byte, error) {
	args := []string{"-a", "-h", "--Format=Nodes: ,Gres: ,GresUsed:", "--state=idle,allocated"}
	return Execute(logger, "sinfo", args)
}

// TotalGPUsData executes sinfo command to get total GPU information
func TotalGPUsData(logger *logger.Logger) ([]byte, error) {
	args := []string{"-a", "-h", "--Format=Nodes: ,Gres:"}
	return Execute(logger, "sinfo", args)
}

// NewGPUsCollector creates a new GPU metrics collector
func NewGPUsCollector(logger *logger.Logger) *GPUsCollector {
	return &GPUsCollector{
		alloc:       prometheus.NewDesc("slurm_gpus_alloc", "Allocated GPUs", nil, nil),
		idle:        prometheus.NewDesc("slurm_gpus_idle", "Idle GPUs", nil, nil),
		other:       prometheus.NewDesc("slurm_gpus_other", "Other GPUs", nil, nil),
		total:       prometheus.NewDesc("slurm_gpus_total", "Total GPUs", nil, nil),
		utilization: prometheus.NewDesc("slurm_gpus_utilization", "Total GPU utilization", nil, nil),
		logger:      logger,
	}
}

// GPUsCollector implements the Prometheus Collector interface for GPU metrics
type GPUsCollector struct {
	alloc       *prometheus.Desc
	idle        *prometheus.Desc
	other       *prometheus.Desc
	total       *prometheus.Desc
	utilization *prometheus.Desc
	logger      *logger.Logger
}

// Describe sends the descriptors of each metric over to the provided channel
func (cc *GPUsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- cc.alloc
	ch <- cc.idle
	ch <- cc.other
	ch <- cc.total
	ch <- cc.utilization
}

// Collect fetches the GPU metrics from Slurm and sends them to Prometheus
func (cc *GPUsCollector) Collect(ch chan<- prometheus.Metric) {
	metrics, err := GPUsGetMetrics(cc.logger)
	if err != nil {
		cc.logger.Error("Failed to get GPU metrics", "err", err)
		return
	}

	ch <- prometheus.MustNewConstMetric(cc.alloc, prometheus.GaugeValue, metrics.alloc)
	ch <- prometheus.MustNewConstMetric(cc.idle, prometheus.GaugeValue, metrics.idle)
	ch <- prometheus.MustNewConstMetric(cc.other, prometheus.GaugeValue, metrics.other)
	ch <- prometheus.MustNewConstMetric(cc.total, prometheus.GaugeValue, metrics.total)
	ch <- prometheus.MustNewConstMetric(cc.utilization, prometheus.GaugeValue, metrics.utilization)
}
