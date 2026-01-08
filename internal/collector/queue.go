package collector

import (
	"strconv"
	"strings"

	
	
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sckyzo/slurm_exporter/internal/logger"
)

type NNVal map[string]map[string]map[string]float64
type NVal map[string]map[string]float64

type QueueMetrics struct {
	pending       NNVal
	running       NVal
	suspended     NVal
	cancelled     NVal
	completing    NVal
	completed     NVal
	configuring   NVal
	failed        NVal
	timeout       NVal
	preempted     NVal
	node_fail     NVal
	c_pending     NNVal
	c_running     NVal
	c_suspended   NVal
	c_cancelled   NVal
	c_completing  NVal
	c_completed   NVal
	c_configuring NVal
	c_failed      NVal
	c_timeout     NVal
	c_preempted   NVal
	c_node_fail   NVal
}


func QueueGetMetrics(logger *logger.Logger) (*QueueMetrics, error) {
	data, err := QueueData(logger)
	if err != nil {
		return nil, err
	}
	return ParseQueueMetrics(data), nil
}

func (s *NVal) Incr(user string, part string, count float64) {
	child, ok := (*s)[user]
	if !ok {
		child = map[string]float64{}
		(*s)[user] = child
		child[part] = 0
	}
	child[part] += count
}

func (s *NNVal) Incr2(reason string, user string, part string, count float64) {
	_, ok := (*s)[reason]
	if !ok {
		child := map[string]map[string]float64{}
		(*s)[reason] = child
	}
	child2, ok := (*s)[reason][user]
	if !ok {
		child2 = map[string]float64{}
		(*s)[reason][user] = child2
	}
	child2[part] += count
}

/*
ParseQueueMetrics parses the output of the squeue command for queue metrics.
Expected input format: "%P,%T,%C,%r,%u" (Partition,State,CPUs,Reason,User).
*/
func ParseQueueMetrics(input []byte) *QueueMetrics {
	qm := QueueMetrics{
		pending:       make(NNVal),
		running:       make(NVal),
		suspended:     make(NVal),
		cancelled:     make(NVal),
		completing:    make(NVal),
		completed:     make(NVal),
		configuring:   make(NVal),
		failed:        make(NVal),
		timeout:       make(NVal),
		preempted:     make(NVal),
		node_fail:     make(NVal),
		c_pending:     make(NNVal),
		c_running:     make(NVal),
		c_suspended:   make(NVal),
		c_cancelled:   make(NVal),
		c_completing:  make(NVal),
		c_completed:   make(NVal),
		c_configuring: make(NVal),
		c_failed:      make(NVal),
		c_timeout:     make(NVal),
		c_preempted:   make(NVal),
		c_node_fail:   make(NVal),
	}
	lines := strings.Split(string(input), "\n")
	for _, line := range lines {
		if strings.Contains(line, ",") {
			fields := strings.Split(line, ",")
			if len(fields) < 5 {
				continue
			}
			part := strings.TrimSpace(fields[0])
			state := fields[1]
			cores_i, _ := strconv.Atoi(fields[2])
			cores := float64(cores_i)
			user := strings.TrimSpace(fields[4])
			reason := fields[3]
			switch state {
			case "PENDING":
				qm.pending.Incr2(reason, user, part, 1)
				qm.c_pending.Incr2(reason, user, part, cores)
			case "RUNNING":
				qm.running.Incr(user, part, 1)
				qm.c_running.Incr(user, part, cores)
			case "SUSPENDED":
				qm.suspended.Incr(user, part, 1)
				qm.suspended.Incr(user, part, cores)
			case "CANCELLED":
				qm.cancelled.Incr(user, part, 1)
				qm.c_cancelled.Incr(user, part, cores)
			case "COMPLETING":
				qm.completing.Incr(user, part, 1)
				qm.c_completing.Incr(user, part, cores)
			case "COMPLETED":
				qm.completed.Incr(user, part, 1)
				qm.c_completed.Incr(user, part, cores)
			case "CONFIGURING":
				qm.configuring.Incr(user, part, 1)
				qm.c_configuring.Incr(user, part, cores)
			case "FAILED":
				qm.failed.Incr(user, part, 1)
				qm.c_failed.Incr(user, part, cores)
			case "TIMEOUT":
				qm.timeout.Incr(user, part, 1)
				qm.c_timeout.Incr(user, part, cores)
			case "PREEMPTED":
				qm.preempted.Incr(user, part, 1)
				qm.c_preempted.Incr(user, part, cores)
			case "NODE_FAIL":
				qm.node_fail.Incr(user, part, 1)
				qm.c_node_fail.Incr(user, part, cores)
			}
		}
	}
	return &qm
}


/*
QueueData executes the squeue command to retrieve queue information.
Expected squeue output format: "%P,%T,%C,%r,%u" (Partition,State,CPUs,Reason,User).
*/
func QueueData(logger *logger.Logger) ([]byte, error) {
	return Execute(logger, "squeue", []string{"-h", "-o", "%P,%T,%C,%r,%u"})
}

/*
 * Implement the Prometheus Collector interface and feed the
 * Slurm queue metrics into it.
 * https://godoc.org/github.com/prometheus/client_golang/prometheus#Collector
 */

func NewQueueCollector(logger *logger.Logger) *QueueCollector {
	return &QueueCollector{
		pending:           prometheus.NewDesc("slurm_queue_pending", "Pending jobs in queue", []string{"user", "partition", "reason"}, nil),
		running:           prometheus.NewDesc("slurm_queue_running", "Running jobs in the cluster", []string{"user", "partition"}, nil),
		suspended:         prometheus.NewDesc("slurm_queue_suspended", "Suspended jobs in the cluster", []string{"user", "partition"}, nil),
		cancelled:         prometheus.NewDesc("slurm_queue_cancelled", "Cancelled jobs in the cluster", []string{"user", "partition"}, nil),
		completing:        prometheus.NewDesc("slurm_queue_completing", "Completing jobs in the cluster", []string{"user", "partition"}, nil),
		completed:         prometheus.NewDesc("slurm_queue_completed", "Completed jobs in the cluster", []string{"user", "partition"}, nil),
		configuring:       prometheus.NewDesc("slurm_queue_configuring", "Configuring jobs in the cluster", []string{"user", "partition"}, nil),
		failed:            prometheus.NewDesc("slurm_queue_failed", "Number of failed jobs", []string{"user", "partition"}, nil),
		timeout:           prometheus.NewDesc("slurm_queue_timeout", "Jobs stopped by timeout", []string{"user", "partition"}, nil),
		preempted:         prometheus.NewDesc("slurm_queue_preempted", "Number of preempted jobs", []string{"user", "partition"}, nil),
		node_fail:         prometheus.NewDesc("slurm_queue_node_fail", "Number of jobs stopped due to node fail", []string{"user", "partition"}, nil),
		cores_pending:     prometheus.NewDesc("slurm_cores_pending", "Pending cores in queue", []string{"user", "partition", "reason"}, nil),
		cores_running:     prometheus.NewDesc("slurm_cores_running", "Running cores in the cluster", []string{"user", "partition"}, nil),
		cores_suspended:   prometheus.NewDesc("slurm_cores_suspended", "Suspended cores in the cluster", []string{"user", "partition"}, nil),
		cores_cancelled:   prometheus.NewDesc("slurm_cores_cancelled", "Cancelled cores in the cluster", []string{"user", "partition"}, nil),
		cores_completing:  prometheus.NewDesc("slurm_cores_completing", "Completing cores in the cluster", []string{"user", "partition"}, nil),
		cores_completed:   prometheus.NewDesc("slurm_cores_completed", "Completed cores in the cluster", []string{"user", "partition"}, nil),
		cores_configuring: prometheus.NewDesc("slurm_cores_configuring", "Configuring cores in the cluster", []string{"user", "partition"}, nil),
		cores_failed:      prometheus.NewDesc("slurm_cores_failed", "Number of failed cores", []string{"user", "partition"}, nil),
		cores_timeout:     prometheus.NewDesc("slurm_cores_timeout", "Cores stopped by timeout", []string{"user", "partition"}, nil),
		cores_preempted:   prometheus.NewDesc("slurm_cores_preempted", "Number of preempted cores", []string{"user", "partition"}, nil),
		cores_node_fail:   prometheus.NewDesc("slurm_cores_node_fail", "Number of cores stopped due to node fail", []string{"user", "partition"}, nil),
		logger:            logger,
	}
}

type QueueCollector struct {
	pending           *prometheus.Desc
	running           *prometheus.Desc
	suspended         *prometheus.Desc
	cancelled         *prometheus.Desc
	completing        *prometheus.Desc
	completed         *prometheus.Desc
	configuring       *prometheus.Desc
	failed            *prometheus.Desc
	timeout           *prometheus.Desc
	preempted         *prometheus.Desc
	node_fail         *prometheus.Desc
	cores_pending     *prometheus.Desc
	cores_running     *prometheus.Desc
	cores_suspended   *prometheus.Desc
	cores_cancelled   *prometheus.Desc
	cores_completing  *prometheus.Desc
	cores_completed   *prometheus.Desc
	cores_configuring *prometheus.Desc
	cores_failed      *prometheus.Desc
	cores_timeout     *prometheus.Desc
	cores_preempted   *prometheus.Desc
	cores_node_fail   *prometheus.Desc
	logger            *logger.Logger
}

func (qc *QueueCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- qc.pending
	ch <- qc.running
	ch <- qc.suspended
	ch <- qc.cancelled
	ch <- qc.completing
	ch <- qc.completed
	ch <- qc.configuring
	ch <- qc.failed
	ch <- qc.timeout
	ch <- qc.preempted
	ch <- qc.node_fail
	ch <- qc.cores_pending
	ch <- qc.cores_running
	ch <- qc.cores_suspended
	ch <- qc.cores_cancelled
	ch <- qc.cores_completing
	ch <- qc.cores_completed
	ch <- qc.cores_configuring
	ch <- qc.cores_failed
	ch <- qc.cores_timeout
	ch <- qc.cores_preempted
	ch <- qc.cores_node_fail
}

func (qc *QueueCollector) Collect(ch chan<- prometheus.Metric) {
	qm, err := QueueGetMetrics(qc.logger)
	if err != nil {
		qc.logger.Error("Failed to get queue metrics", "err", err)
		return
	}
	for reason, values := range qm.pending {
		PushMetric(values, ch, qc.pending, reason)
	}

	PushMetric(qm.running, ch, qc.running, "")
	PushMetric(qm.cancelled, ch, qc.cancelled, "")
	PushMetric(qm.completing, ch, qc.completing, "")
	PushMetric(qm.completed, ch, qc.completed, "")
	PushMetric(qm.configuring, ch, qc.configuring, "")
	PushMetric(qm.failed, ch, qc.failed, "")
	PushMetric(qm.timeout, ch, qc.timeout, "")
	PushMetric(qm.preempted, ch, qc.preempted, "")
	PushMetric(qm.node_fail, ch, qc.node_fail, "")
	for reason, value := range qm.c_pending {
		PushMetric(value, ch, qc.cores_pending, reason)
	}
	PushMetric(qm.c_running, ch, qc.cores_running, "")
	PushMetric(qm.c_cancelled, ch, qc.cores_cancelled, "")
	PushMetric(qm.c_completing, ch, qc.cores_completing, "")
	PushMetric(qm.c_completed, ch, qc.cores_completed, "")
	PushMetric(qm.c_configuring, ch, qc.cores_configuring, "")
	PushMetric(qm.c_failed, ch, qc.cores_failed, "")
	PushMetric(qm.c_timeout, ch, qc.cores_timeout, "")
	PushMetric(qm.c_preempted, ch, qc.cores_preempted, "")
	PushMetric(qm.c_node_fail, ch, qc.cores_node_fail, "")
}

func PushMetric(m map[string]map[string]float64, ch chan<- prometheus.Metric, coll *prometheus.Desc, a_label string) {
	for label1, vals1 := range m {
		for label2, val := range vals1 {
			if a_label != "" {
				ch <- prometheus.MustNewConstMetric(coll, prometheus.GaugeValue, val, label1, label2, a_label)
			} else {
				ch <- prometheus.MustNewConstMetric(coll, prometheus.GaugeValue, val, label1, label2)
			}
		}
	}
}