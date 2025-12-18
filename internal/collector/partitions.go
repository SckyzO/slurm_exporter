package collector

import (
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
	allocated float64
	idle      float64
	other     float64
	pending   float64
	running   float64
	total     float64
}

/*
ParsePartitionsMetrics parses the output of sinfo and squeue for partition metrics.
It combines CPU allocation data from sinfo ("%R,%C") with pending/running job counts from squeue ("%P,%T").
*/
func ParsePartitionsMetrics(logger *logger.Logger) (map[string]*PartitionMetrics, error) {
	partitions := make(map[string]*PartitionMetrics)
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
				partitions[partition] = &PartitionMetrics{0, 0, 0, 0, 0, 0}
			}
			states := strings.Split(line, ",")[1]
			allocated, _ := strconv.ParseFloat(strings.Split(states, "/")[0], 64)
			idle, _ := strconv.ParseFloat(strings.Split(states, "/")[1], 64)
			other, _ := strconv.ParseFloat(strings.Split(states, "/")[2], 64)
			total, _ := strconv.ParseFloat(strings.Split(states, "/")[3], 64)
			partitions[partition].allocated = allocated
			partitions[partition].idle = idle
			partitions[partition].other = other
			partitions[partition].total = total
		}
	}

	pendingJobsData, err := PartitionsPendingJobsData(logger)
	if err != nil {
		return nil, err
	}
	list := strings.Split(string(pendingJobsData), "\n")
	for _, partition := range list {

		_, key := partitions[partition]
		if key {
			partitions[partition].pending += 1
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
			partitions[partition].running += 1
		}
	}

	return partitions, nil
}

type PartitionsCollector struct {
	allocated *prometheus.Desc
	idle      *prometheus.Desc
	other     *prometheus.Desc
	pending   *prometheus.Desc
	running   *prometheus.Desc
	total     *prometheus.Desc
	logger    *logger.Logger
}

func NewPartitionsCollector(logger *logger.Logger) *PartitionsCollector {
	labels := []string{"partition"}
	return &PartitionsCollector{
		allocated: prometheus.NewDesc("slurm_partition_cpus_allocated", "Allocated CPUs for partition", labels, nil),
		idle:      prometheus.NewDesc("slurm_partition_cpus_idle", "Idle CPUs for partition", labels, nil),
		other:     prometheus.NewDesc("slurm_partition_cpus_other", "Other CPUs for partition", labels, nil),
		pending:   prometheus.NewDesc("slurm_partition_jobs_pending", "Pending jobs for partition", labels, nil),
		running:   prometheus.NewDesc("slurm_partition_jobs_running", "Running jobs for partition", labels, nil),
		total:     prometheus.NewDesc("slurm_partition_cpus_total", "Total CPUs for partition", labels, nil),
		logger:    logger,
	}
}

func (pc *PartitionsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- pc.allocated
	ch <- pc.idle
	ch <- pc.other
	ch <- pc.pending
	ch <- pc.running
	ch <- pc.total
}

func (pc *PartitionsCollector) Collect(ch chan<- prometheus.Metric) {
	pm, err := ParsePartitionsMetrics(pc.logger)
	if err != nil {
		pc.logger.Error("Failed to parse partitions metrics", "err", err)
		return
	}
	for p := range pm {
		if pm[p].allocated > 0 {
			ch <- prometheus.MustNewConstMetric(pc.allocated, prometheus.GaugeValue, pm[p].allocated, p)
		}
		if pm[p].idle > 0 {
			ch <- prometheus.MustNewConstMetric(pc.idle, prometheus.GaugeValue, pm[p].idle, p)
		}
		if pm[p].other > 0 {
			ch <- prometheus.MustNewConstMetric(pc.other, prometheus.GaugeValue, pm[p].other, p)
		}
		if pm[p].pending > 0 {
			ch <- prometheus.MustNewConstMetric(pc.pending, prometheus.GaugeValue, pm[p].pending, p)
		}
		if pm[p].running > 0 {
			ch <- prometheus.MustNewConstMetric(pc.running, prometheus.GaugeValue, pm[p].running, p)
		}
		if pm[p].total > 0 {
			ch <- prometheus.MustNewConstMetric(pc.total, prometheus.GaugeValue, pm[p].total, p)
		}
	}
}
