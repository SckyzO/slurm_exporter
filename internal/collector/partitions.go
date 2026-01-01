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

/*
ParsePartitionsMetrics parses the output of sinfo and squeue for partition metrics.
It combines CPU allocation data from sinfo ("%R,%C") with pending/running job counts from squeue ("%P,%T").
*/
func ParsePartitionsMetrics(logger *logger.Logger) (map[string]*PartitionMetrics, error) {
	partitions := make(map[string]*PartitionMetrics)
	// partition cpu usage
	partitionsData, err := PartitionsData(logger)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(partitionsData), "\n")
	for _, line := range lines {
		if strings.Contains(line, ",") {

			partition := strings.Split(line, ",")[0]
			_, key := partitions[partition]
			if !key {
				partitions[partition] = &PartitionMetrics{0, 0, 0, 0, 0, 0, 0, 0}
			}
			states := strings.Split(line, ",")[1]
			allocated, _ := strconv.ParseFloat(strings.Split(states, "/")[0], 64)
			idle, _ := strconv.ParseFloat(strings.Split(states, "/")[1], 64)
			other, _ := strconv.ParseFloat(strings.Split(states, "/")[2], 64)
			total, _ := strconv.ParseFloat(strings.Split(states, "/")[3], 64)
			partitions[partition].cpuAllocated = allocated
			partitions[partition].cpuIdle = idle
			partitions[partition].cpuOther = other
			partitions[partition].cpuTotal = total
		}
	}
	// partition gpu usage
	partitionsGPUData, err := PartitionsGpuData(logger)
	if err != nil {
		return nil, err
	}
	gpuLines := strings.Split(string(partitionsGPUData), "\n")
	for _, line := range gpuLines {
		if len(line) > 0 && strings.Contains(line, "gpu:") {
			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}
			numNodes, _ := strconv.ParseFloat(fields[0], 64)
			partition := fields[1]
			nodeGpus := parseGpuCount(fields[2], partitionGpuRe)
			allocatedGpus := parseGpuCount(fields[3], partitionGpuRe)

			partitions[partition].gpuIdle += numNodes * (nodeGpus - allocatedGpus)
			partitions[partition].gpuAllocated += numNodes * allocatedGpus
		}
	}

	// partition jobs
	pendingJobsData, err := PartitionsPendingJobsData(logger)
	if err != nil {
		return nil, err
	}
	list := strings.Split(string(pendingJobsData), "\n")
	for _, partition := range list {

		_, key := partitions[partition]
		if key {
			partitions[partition].jobPending += 1
		}
	}
	runningJobsData, err := PartitionsRunningJobsData(logger)
	if err != nil {
		return nil, err
	}
	list = strings.Split(string(runningJobsData), "\n")
	for _, partition := range list {
		_, key := partitions[partition]
		if key {
			partitions[partition].jobRunning += 1
		}
	}

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
