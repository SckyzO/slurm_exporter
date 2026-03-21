package collector

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

/*
PartitionsData executes the sinfo command to retrieve partition CPU information.
Expected sinfo output format: "%R,%C" (PartitionName,Alloc/Idle/Other/Total CPUs).
*/
func PartitionsData(logger *logger.Logger) ([]byte, error) {
	return Execute(logger, "sinfo", []string{"-h", "-o", "%R,%C"})
}

/*
PartitionsGpuData executes the sinfo command to retrieve partition GPU information.
Expected sinfo output format: "Partition,Gres,GresUsed" (PartitionName,Total/Alloc GPUs).
*/
func PartitionsGpuData(logger *logger.Logger) ([]byte, error) {
	return Execute(logger, "sinfo", []string{"-h", "--Format=Nodes: ,Partition: ,Gres: ,GresUsed:", "--state=idle,allocated"})
}

/*
PartitionsPendingJobsData executes the squeue command to retrieve pending job counts per partition.
Expected squeue output format: "%P" (PartitionName).
*/
func PartitionsPendingJobsData(logger *logger.Logger) ([]byte, error) {
	return Execute(logger, "squeue", []string{"-a", "-r", "-h", "-o", "%P", "--states=PENDING"})
}

/*
PartitionsRunningJobsData executes the squeue command to retrieve running job counts per partition.
Expected squeue output format: "%P" (PartitionName).
*/
func PartitionsRunningJobsData(logger *logger.Logger) ([]byte, error) {
	return Execute(logger, "squeue", []string{"-a", "-r", "-h", "-o", "%P", "--states=RUNNING"})
}

type PartitionMetrics struct {
	cpuAllocated float64
	cpuIdle      float64
	cpuOther     float64
	cpuTotal     float64
	jobPending   float64
	jobRunning   float64
	gpuIdle      float64
	gpuAllocated float64
}

var (
	partitionGpuRe = regexp.MustCompile(`gpu:(\(null\)|[^:(]*):?([0-9]+)(\([^)]*\))?`)
)

// parseGpuCount extracts GPU count from GPU spec string
func parseGpuCount(gpuSpec string, re *regexp.Regexp) float64 {
	matches := re.FindStringSubmatch(gpuSpec)
	if len(matches) > 2 {
		gpuCount, _ := strconv.ParseFloat(matches[2], 64)
		return gpuCount
	}
	return 0.0
}

// parsePartitionCPUs parses sinfo "%R,%C" output into the partitions map.
func parsePartitionCPUs(data []byte, partitions map[string]*PartitionMetrics) {
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, ",") {
			continue
		}
		splitLine := strings.Split(line, ",")
		if len(splitLine) < 2 {
			continue
		}
		partition := splitLine[0]
		if _, exists := partitions[partition]; !exists {
			partitions[partition] = &PartitionMetrics{}
		}
		statesSplit := strings.Split(splitLine[1], "/")
		if len(statesSplit) < 4 {
			continue
		}
		partitions[partition].cpuAllocated, _ = strconv.ParseFloat(statesSplit[0], 64)
		partitions[partition].cpuIdle, _ = strconv.ParseFloat(statesSplit[1], 64)
		partitions[partition].cpuOther, _ = strconv.ParseFloat(statesSplit[2], 64)
		partitions[partition].cpuTotal, _ = strconv.ParseFloat(statesSplit[3], 64)
	}
}

// parsePartitionGPUs parses sinfo "--Format=Nodes:,Partition:,Gres:,GresUsed:" output
// into the partitions map. Initializes missing partitions (GPU-only nodes, issue #5).
func parsePartitionGPUs(data []byte, partitions map[string]*PartitionMetrics) {
	for _, line := range strings.Split(string(data), "\n") {
		if len(line) == 0 || !strings.Contains(line, "gpu:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		numNodes, _ := strconv.ParseFloat(fields[0], 64)
		partition := fields[1]
		nodeGpus := parseGpuCount(fields[2], partitionGpuRe)
		allocatedGpus := parseGpuCount(fields[3], partitionGpuRe)
		if _, exists := partitions[partition]; !exists {
			partitions[partition] = &PartitionMetrics{}
		}
		partitions[partition].gpuIdle += numNodes * (nodeGpus - allocatedGpus)
		partitions[partition].gpuAllocated += numNodes * allocatedGpus
	}
}

// parsePartitionJobs counts pending and running jobs per partition from squeue output.
func parsePartitionJobs(pendingData, runningData []byte, partitions map[string]*PartitionMetrics) {
	for _, partition := range strings.Split(string(pendingData), "\n") {
		if _, exists := partitions[partition]; exists {
			partitions[partition].jobPending++
		}
	}
	for _, partition := range strings.Split(string(runningData), "\n") {
		if _, exists := partitions[partition]; exists {
			partitions[partition].jobRunning++
		}
	}
}

// ParsePartitionsMetrics collects CPU, GPU, and job metrics for all Slurm partitions.
func ParsePartitionsMetrics(logger *logger.Logger) (map[string]*PartitionMetrics, error) {
	partitions := make(map[string]*PartitionMetrics)

	cpuData, err := PartitionsData(logger)
	if err != nil {
		return nil, err
	}
	parsePartitionCPUs(cpuData, partitions)

	gpuData, err := PartitionsGpuData(logger)
	if err != nil {
		return nil, err
	}
	parsePartitionGPUs(gpuData, partitions)

	pendingData, err := PartitionsPendingJobsData(logger)
	if err != nil {
		return nil, err
	}
	runningData, err := PartitionsRunningJobsData(logger)
	if err != nil {
		return nil, err
	}
	parsePartitionJobs(pendingData, runningData, partitions)

	return partitions, nil
}

type PartitionsCollector struct {
	cpuAllocated *prometheus.Desc
	cpuIdle      *prometheus.Desc
	cpuOther     *prometheus.Desc
	cpuTotal     *prometheus.Desc
	jobPending   *prometheus.Desc
	jobRunning   *prometheus.Desc
	gpuIdle      *prometheus.Desc
	gpuAllocated *prometheus.Desc
	logger       *logger.Logger
}

func NewPartitionsCollector(logger *logger.Logger) *PartitionsCollector {
	labels := []string{"partition"}
	return &PartitionsCollector{
		cpuAllocated: prometheus.NewDesc("slurm_partition_cpus_allocated", "Allocated CPUs for partition", labels, nil),
		cpuIdle:      prometheus.NewDesc("slurm_partition_cpus_idle", "Idle CPUs for partition", labels, nil),
		cpuOther:     prometheus.NewDesc("slurm_partition_cpus_other", "Other CPUs for partition", labels, nil),
		cpuTotal:     prometheus.NewDesc("slurm_partition_cpus_total", "Total CPUs for partition", labels, nil),
		jobPending:   prometheus.NewDesc("slurm_partition_jobs_pending", "Pending jobs for partition", labels, nil),
		jobRunning:   prometheus.NewDesc("slurm_partition_jobs_running", "Running jobs for partition", labels, nil),
		gpuIdle:      prometheus.NewDesc("slurm_partition_gpus_idle", "Idle GPUs for partition", labels, nil),
		gpuAllocated: prometheus.NewDesc("slurm_partition_gpus_allocated", "Allocated GPUs for partition", labels, nil),
		logger:       logger,
	}
}

func (pc *PartitionsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- pc.cpuAllocated
	ch <- pc.cpuIdle
	ch <- pc.cpuOther
	ch <- pc.cpuTotal
	ch <- pc.jobPending
	ch <- pc.jobRunning
	ch <- pc.gpuIdle
	ch <- pc.gpuAllocated
}

func (pc *PartitionsCollector) Collect(ch chan<- prometheus.Metric) {
	pm, err := ParsePartitionsMetrics(pc.logger)
	if err != nil {
		pc.logger.Error("Failed to parse partitions metrics", "err", err)
		return
	}
	for p := range pm {
		ch <- prometheus.MustNewConstMetric(pc.cpuAllocated, prometheus.GaugeValue, pm[p].cpuAllocated, p)
		ch <- prometheus.MustNewConstMetric(pc.cpuIdle, prometheus.GaugeValue, pm[p].cpuIdle, p)
		ch <- prometheus.MustNewConstMetric(pc.cpuOther, prometheus.GaugeValue, pm[p].cpuOther, p)
		ch <- prometheus.MustNewConstMetric(pc.cpuTotal, prometheus.GaugeValue, pm[p].cpuTotal, p)
		ch <- prometheus.MustNewConstMetric(pc.jobPending, prometheus.GaugeValue, pm[p].jobPending, p)
		ch <- prometheus.MustNewConstMetric(pc.jobRunning, prometheus.GaugeValue, pm[p].jobRunning, p)
		ch <- prometheus.MustNewConstMetric(pc.gpuIdle, prometheus.GaugeValue, pm[p].gpuIdle, p)
		ch <- prometheus.MustNewConstMetric(pc.gpuAllocated, prometheus.GaugeValue, pm[p].gpuAllocated, p)
	}
}
