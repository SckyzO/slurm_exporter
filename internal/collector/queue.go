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
	pending      NNVal
	running      NVal
	suspended    NVal
	cancelled    NVal
	completing   NVal
	completed    NVal
	configuring  NVal
	failed       NVal
	timeout      NVal
	preempted    NVal
	nodeFail     NVal
	cPending     NNVal
	cRunning     NVal
	cSuspended   NVal
	cCancelled   NVal
	cCompleting  NVal
	cCompleted   NVal
	cConfiguring NVal
	cFailed      NVal
	cTimeout     NVal
	cPreempted   NVal
	cNodeFail    NVal
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
		pending:      make(NNVal),
		running:      make(NVal),
		suspended:    make(NVal),
		cancelled:    make(NVal),
		completing:   make(NVal),
		completed:    make(NVal),
		configuring:  make(NVal),
		failed:       make(NVal),
		timeout:      make(NVal),
		preempted:    make(NVal),
		nodeFail:     make(NVal),
		cPending:     make(NNVal),
		cRunning:     make(NVal),
		cSuspended:   make(NVal),
		cCancelled:   make(NVal),
		cCompleting:  make(NVal),
		cCompleted:   make(NVal),
		cConfiguring: make(NVal),
		cFailed:      make(NVal),
		cTimeout:     make(NVal),
		cPreempted:   make(NVal),
		cNodeFail:    make(NVal),
	}
	for line := range strings.SplitSeq(string(input), "\n") {
		if strings.Contains(line, "|") {
			// SplitN with 5 keeps a reason field that may itself contain pipes
			fields := strings.SplitN(line, "|", 5)
			if len(fields) < 5 {
				continue
			}
			part := strings.TrimSpace(fields[0])
			state := fields[1]
			coresI, _ := strconv.Atoi(fields[2])
			cores := float64(coresI)
			reason := fields[3]
			user := strings.TrimSpace(fields[4])
			switch state {
			case "PENDING":
				qm.pending.Incr2(reason, user, part, 1)
				qm.cPending.Incr2(reason, user, part, cores)
			case "RUNNING":
				qm.running.Incr(user, part, 1)
				qm.cRunning.Incr(user, part, cores)
			case "SUSPENDED":
				qm.suspended.Incr(user, part, 1)
				qm.cSuspended.Incr(user, part, cores)
			case "CANCELLED":
				qm.cancelled.Incr(user, part, 1)
				qm.cCancelled.Incr(user, part, cores)
			case "COMPLETING":
				qm.completing.Incr(user, part, 1)
				qm.cCompleting.Incr(user, part, cores)
			case "COMPLETED":
				qm.completed.Incr(user, part, 1)
				qm.cCompleted.Incr(user, part, cores)
			case "CONFIGURING":
				qm.configuring.Incr(user, part, 1)
				qm.cConfiguring.Incr(user, part, cores)
			case "FAILED":
				qm.failed.Incr(user, part, 1)
				qm.cFailed.Incr(user, part, cores)
			case "TIMEOUT":
				qm.timeout.Incr(user, part, 1)
				qm.cTimeout.Incr(user, part, cores)
			case "PREEMPTED":
				qm.preempted.Incr(user, part, 1)
				qm.cPreempted.Incr(user, part, cores)
			case "NODE_FAIL":
				qm.nodeFail.Incr(user, part, 1)
				qm.cNodeFail.Incr(user, part, cores)
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
	return Execute(logger, "squeue", []string{"-h", "-o", "%P|%T|%C|%r|%u"})
}

/*
 * Implement the Prometheus Collector interface and feed the
 * Slurm queue metrics into it.
 * https://godoc.org/github.com/prometheus/client_golang/prometheus#Collector
 */

// NewQueueCollector creates a queue metrics collector.
// When withUserLabel is false, the user label is omitted and counts are
// aggregated per partition only, reducing cardinality on large clusters.
func NewQueueCollector(logger *logger.Logger, withUserLabel bool) *QueueCollector {
	var labelsJob, labelsPending []string
	if withUserLabel {
		labelsJob = []string{"user", "partition"}
		labelsPending = []string{"user", "partition", "reason"}
	} else {
		labelsJob = []string{"partition"}
		labelsPending = []string{"partition", "reason"}
	}
	return &QueueCollector{
		withUserLabel:    withUserLabel,
		pending:          prometheus.NewDesc("slurm_queue_pending", "Pending jobs in queue", labelsPending, nil),
		running:          prometheus.NewDesc("slurm_queue_running", "Running jobs in the cluster", labelsJob, nil),
		suspended:        prometheus.NewDesc("slurm_queue_suspended", "Suspended jobs in the cluster", labelsJob, nil),
		cancelled:        prometheus.NewDesc("slurm_queue_cancelled", "Cancelled jobs in the cluster", labelsJob, nil),
		completing:       prometheus.NewDesc("slurm_queue_completing", "Completing jobs in the cluster", labelsJob, nil),
		completed:        prometheus.NewDesc("slurm_queue_completed", "Completed jobs in the cluster", labelsJob, nil),
		configuring:      prometheus.NewDesc("slurm_queue_configuring", "Configuring jobs in the cluster", labelsJob, nil),
		failed:           prometheus.NewDesc("slurm_queue_failed", "Number of failed jobs", labelsJob, nil),
		timeout:          prometheus.NewDesc("slurm_queue_timeout", "Jobs stopped by timeout", labelsJob, nil),
		preempted:        prometheus.NewDesc("slurm_queue_preempted", "Number of preempted jobs", labelsJob, nil),
		nodeFail:         prometheus.NewDesc("slurm_queue_node_fail", "Number of jobs stopped due to node fail", labelsJob, nil),
		coresPending:     prometheus.NewDesc("slurm_cores_pending", "Pending cores in queue", labelsPending, nil),
		coresRunning:     prometheus.NewDesc("slurm_cores_running", "Running cores in the cluster", labelsJob, nil),
		coresSuspended:   prometheus.NewDesc("slurm_cores_suspended", "Suspended cores in the cluster", labelsJob, nil),
		coresCancelled:   prometheus.NewDesc("slurm_cores_cancelled", "Cancelled cores in the cluster", labelsJob, nil),
		coresCompleting:  prometheus.NewDesc("slurm_cores_completing", "Completing cores in the cluster", labelsJob, nil),
		coresCompleted:   prometheus.NewDesc("slurm_cores_completed", "Completed cores in the cluster", labelsJob, nil),
		coresConfiguring: prometheus.NewDesc("slurm_cores_configuring", "Configuring cores in the cluster", labelsJob, nil),
		coresFailed:      prometheus.NewDesc("slurm_cores_failed", "Number of failed cores", labelsJob, nil),
		coresTimeout:     prometheus.NewDesc("slurm_cores_timeout", "Cores stopped by timeout", labelsJob, nil),
		coresPreempted:   prometheus.NewDesc("slurm_cores_preempted", "Number of preempted cores", labelsJob, nil),
		coresNodeFail:    prometheus.NewDesc("slurm_cores_node_fail", "Number of cores stopped due to node fail", labelsJob, nil),
		logger:           logger,
	}
}

type QueueCollector struct {
	pending          *prometheus.Desc
	running          *prometheus.Desc
	suspended        *prometheus.Desc
	cancelled        *prometheus.Desc
	completing       *prometheus.Desc
	completed        *prometheus.Desc
	configuring      *prometheus.Desc
	failed           *prometheus.Desc
	timeout          *prometheus.Desc
	preempted        *prometheus.Desc
	nodeFail         *prometheus.Desc
	coresPending     *prometheus.Desc
	coresRunning     *prometheus.Desc
	coresSuspended   *prometheus.Desc
	coresCancelled   *prometheus.Desc
	coresCompleting  *prometheus.Desc
	coresCompleted   *prometheus.Desc
	coresConfiguring *prometheus.Desc
	coresFailed      *prometheus.Desc
	coresTimeout     *prometheus.Desc
	coresPreempted   *prometheus.Desc
	coresNodeFail    *prometheus.Desc
	withUserLabel    bool
	logger           *logger.Logger
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
	ch <- qc.nodeFail
	ch <- qc.coresPending
	ch <- qc.coresRunning
	ch <- qc.coresSuspended
	ch <- qc.coresCancelled
	ch <- qc.coresCompleting
	ch <- qc.coresCompleted
	ch <- qc.coresConfiguring
	ch <- qc.coresFailed
	ch <- qc.coresTimeout
	ch <- qc.coresPreempted
	ch <- qc.coresNodeFail
}

func (qc *QueueCollector) Collect(ch chan<- prometheus.Metric) {
	qm, err := QueueGetMetrics(qc.logger)
	if err != nil {
		qc.logger.Error("Failed to get queue metrics", "err", err)
		return
	}
	if qc.withUserLabel {
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
		PushMetric(qm.nodeFail, ch, qc.nodeFail, "")
		for reason, value := range qm.cPending {
			PushMetric(value, ch, qc.coresPending, reason)
		}
		PushMetric(qm.cRunning, ch, qc.coresRunning, "")
		PushMetric(qm.cCancelled, ch, qc.coresCancelled, "")
		PushMetric(qm.cCompleting, ch, qc.coresCompleting, "")
		PushMetric(qm.cCompleted, ch, qc.coresCompleted, "")
		PushMetric(qm.cConfiguring, ch, qc.coresConfiguring, "")
		PushMetric(qm.cFailed, ch, qc.coresFailed, "")
		PushMetric(qm.cTimeout, ch, qc.coresTimeout, "")
		PushMetric(qm.cPreempted, ch, qc.coresPreempted, "")
		PushMetric(qm.cNodeFail, ch, qc.coresNodeFail, "")
	} else {
		// user label disabled: aggregate all users per partition
		pushAggregatedNNVal(qm.pending, ch, qc.pending)
		pushAggregatedNVal(qm.running, ch, qc.running, "")
		pushAggregatedNVal(qm.cancelled, ch, qc.cancelled, "")
		pushAggregatedNVal(qm.completing, ch, qc.completing, "")
		pushAggregatedNVal(qm.completed, ch, qc.completed, "")
		pushAggregatedNVal(qm.configuring, ch, qc.configuring, "")
		pushAggregatedNVal(qm.failed, ch, qc.failed, "")
		pushAggregatedNVal(qm.timeout, ch, qc.timeout, "")
		pushAggregatedNVal(qm.preempted, ch, qc.preempted, "")
		pushAggregatedNVal(qm.nodeFail, ch, qc.nodeFail, "")
		pushAggregatedNNVal(qm.cPending, ch, qc.coresPending)
		pushAggregatedNVal(qm.cRunning, ch, qc.coresRunning, "")
		pushAggregatedNVal(qm.cCancelled, ch, qc.coresCancelled, "")
		pushAggregatedNVal(qm.cCompleting, ch, qc.coresCompleting, "")
		pushAggregatedNVal(qm.cCompleted, ch, qc.coresCompleted, "")
		pushAggregatedNVal(qm.cConfiguring, ch, qc.coresConfiguring, "")
		pushAggregatedNVal(qm.cFailed, ch, qc.coresFailed, "")
		pushAggregatedNVal(qm.cTimeout, ch, qc.coresTimeout, "")
		pushAggregatedNVal(qm.cPreempted, ch, qc.coresPreempted, "")
		pushAggregatedNVal(qm.cNodeFail, ch, qc.coresNodeFail, "")
	}
}

// pushAggregatedNVal aggregates NVal (user->partition->count) to {partition},
// collapsing the user dimension. Used when --collector.queue.user-label=false.
func pushAggregatedNVal(m NVal, ch chan<- prometheus.Metric, desc *prometheus.Desc, aLabel string) {
	aggregated := make(map[string]float64)
	for _, partitionMap := range m {
		for partition, val := range partitionMap {
			aggregated[partition] += val
		}
	}
	for partition, val := range aggregated {
		if aLabel != "" {
			ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, val, partition, aLabel)
		} else {
			ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, val, partition)
		}
	}
}

// pushAggregatedNNVal aggregates NNVal (reason->user->partition->count) to {partition, reason}.
func pushAggregatedNNVal(m NNVal, ch chan<- prometheus.Metric, desc *prometheus.Desc) {
	aggregated := make(map[string]map[string]float64)
	for reason, userMap := range m {
		if aggregated[reason] == nil {
			aggregated[reason] = make(map[string]float64)
		}
		for _, partitionMap := range userMap {
			for partition, val := range partitionMap {
				aggregated[reason][partition] += val
			}
		}
	}
	for reason, partitionMap := range aggregated {
		for partition, val := range partitionMap {
			ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, val, partition, reason)
		}
	}
}

func PushMetric(m map[string]map[string]float64, ch chan<- prometheus.Metric, coll *prometheus.Desc, aLabel string) {
	for label1, vals1 := range m {
		for label2, val := range vals1 {
			if aLabel != "" {
				ch <- prometheus.MustNewConstMetric(coll, prometheus.GaugeValue, val, label1, label2, aLabel)
			} else {
				ch <- prometheus.MustNewConstMetric(coll, prometheus.GaugeValue, val, label1, label2)
			}
		}
	}
}
