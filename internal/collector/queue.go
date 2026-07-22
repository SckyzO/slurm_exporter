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

	// Cluster-wide counts, keyed by state, incremented once per job. They
	// cannot be derived from the maps above: a job pending in several
	// partitions is counted in each of them, while slurm_jobs_pending must
	// stay a job count — three shipped alerting rules read it as one.
	jobTotals  map[string]float64
	coreTotals map[string]float64
}

func QueueGetMetrics(logger *logger.Logger, withTerminalStates bool) (*QueueMetrics, error) {
	data, err := QueueData(logger, withTerminalStates)
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
		jobTotals:    make(map[string]float64),
		coreTotals:   make(map[string]float64),
	}
	for line := range strings.SplitSeq(string(input), "\n") {
		if strings.Contains(line, "|") {
			// SplitN with 5 keeps a reason field that may itself contain pipes
			fields := strings.SplitN(line, "|", 5)
			if len(fields) < 5 {
				continue
			}
			// A job submitted to several partitions is reported by squeue
			// as a comma-separated list, and the default partition carries
			// a "*" marker. squeuePartitions applies both normalisations —
			// the same ones parsePartitionJobs needs — so the label always
			// names a partition that exists (issues #20 and #154).
			parts := squeuePartitions(fields[0])
			state := fields[1]
			coresI, _ := strconv.Atoi(fields[2])
			cores := float64(coresI)
			reason := fields[3]
			user := strings.TrimSpace(fields[4])

			// A job queued in several partitions contends for each, so it is
			// counted in each — the rule parsePartitionJobs already applies.
			// The cluster-wide totals stay at one per job.
			addN := func(jobs, jobCores NVal) {
				for _, part := range parts {
					jobs.Incr(user, part, 1)
					jobCores.Incr(user, part, cores)
				}
				qm.jobTotals[state]++
				qm.coreTotals[state] += cores
			}
			addNN := func(jobs, jobCores NNVal) {
				for _, part := range parts {
					jobs.Incr2(reason, user, part, 1)
					jobCores.Incr2(reason, user, part, cores)
				}
				qm.jobTotals[state]++
				qm.coreTotals[state] += cores
			}

			switch state {
			case "PENDING":
				addNN(qm.pending, qm.cPending)
			case "RUNNING":
				addN(qm.running, qm.cRunning)
			case "SUSPENDED":
				addN(qm.suspended, qm.cSuspended)
			case "CANCELLED":
				addN(qm.cancelled, qm.cCancelled)
			case "COMPLETING":
				addN(qm.completing, qm.cCompleting)
			case "COMPLETED":
				addN(qm.completed, qm.cCompleted)
			case "CONFIGURING":
				addN(qm.configuring, qm.cConfiguring)
			case "FAILED":
				addN(qm.failed, qm.cFailed)
			case "TIMEOUT":
				addN(qm.timeout, qm.cTimeout)
			case "PREEMPTED":
				addN(qm.preempted, qm.cPreempted)
			case "NODE_FAIL":
				addN(qm.nodeFail, qm.cNodeFail)
			}
		}
	}
	return &qm
}

/*
QueueData executes the squeue command to retrieve queue information.
Expected squeue output format: "%P,%T,%C,%r,%u" (Partition,State,CPUs,Reason,User).

withTerminalStates adds --states=all. squeue reports pending and running jobs
when it is not told which states to view, so nine of the eleven states below
never appeared in its input and every metric built from them read a constant
zero: slurm_jobs_failed said the cluster had never had a failure. Asking for all
states is what turns those series back into a measurement. See issue #27.

The window is bounded by MinJobAge (slurm.conf, 300s by default): slurmctld
forgets a terminated job once it is older than that, so a job counts here for as
long as the controller still remembers it and no longer.
*/
func QueueData(logger *logger.Logger, withTerminalStates bool) ([]byte, error) {
	args := []string{"-h", "-o", "%P|%T|%C|%r|%u"}
	if withTerminalStates {
		args = append(args, "--states=all")
	}
	return Execute(logger, "squeue", args)
}

/*
 * Implement the Prometheus Collector interface and feed the
 * Slurm queue metrics into it.
 * https://godoc.org/github.com/prometheus/client_golang/prometheus#Collector
 */

// NewQueueCollector creates a queue metrics collector.
// When withUserLabel is false, the user label is omitted and counts are
// aggregated per partition only, reducing cardinality on large clusters.
func NewQueueCollector(logger *logger.Logger, withUserLabel, withTerminalStates bool) *QueueCollector {
	var labelsJob, labelsPending []string
	if withUserLabel {
		labelsJob = []string{"user", "partition"}
		labelsPending = []string{"user", "partition", "reason"}
	} else {
		labelsJob = []string{"partition"}
		labelsPending = []string{"partition", "reason"}
	}
	return &QueueCollector{
		withUserLabel:      withUserLabel,
		withTerminalStates: withTerminalStates,
		pending:            prometheus.NewDesc("slurm_queue_pending", "Pending jobs in queue", labelsPending, nil),
		running:            prometheus.NewDesc("slurm_queue_running", "Running jobs in the cluster", labelsJob, nil),
		suspended:          prometheus.NewDesc("slurm_queue_suspended", "Suspended jobs in the cluster", labelsJob, nil),
		cancelled:          prometheus.NewDesc("slurm_queue_cancelled", "Cancelled jobs in the cluster", labelsJob, nil),
		completing:         prometheus.NewDesc("slurm_queue_completing", "Completing jobs in the cluster", labelsJob, nil),
		completed:          prometheus.NewDesc("slurm_queue_completed", "Completed jobs in the cluster", labelsJob, nil),
		configuring:        prometheus.NewDesc("slurm_queue_configuring", "Configuring jobs in the cluster", labelsJob, nil),
		failed:             prometheus.NewDesc("slurm_queue_failed", "Number of failed jobs", labelsJob, nil),
		timeout:            prometheus.NewDesc("slurm_queue_timeout", "Jobs stopped by timeout", labelsJob, nil),
		preempted:          prometheus.NewDesc("slurm_queue_preempted", "Number of preempted jobs", labelsJob, nil),
		nodeFail:           prometheus.NewDesc("slurm_queue_node_fail", "Number of jobs stopped due to node fail", labelsJob, nil),
		coresPending:       prometheus.NewDesc("slurm_cores_pending", "Pending cores in queue", labelsPending, nil),
		coresRunning:       prometheus.NewDesc("slurm_cores_running", "Running cores in the cluster", labelsJob, nil),
		coresSuspended:     prometheus.NewDesc("slurm_cores_suspended", "Suspended cores in the cluster", labelsJob, nil),
		coresCancelled:     prometheus.NewDesc("slurm_cores_cancelled", "Cancelled cores in the cluster", labelsJob, nil),
		coresCompleting:    prometheus.NewDesc("slurm_cores_completing", "Completing cores in the cluster", labelsJob, nil),
		coresCompleted:     prometheus.NewDesc("slurm_cores_completed", "Completed cores in the cluster", labelsJob, nil),
		coresConfiguring:   prometheus.NewDesc("slurm_cores_configuring", "Configuring cores in the cluster", labelsJob, nil),
		coresFailed:        prometheus.NewDesc("slurm_cores_failed", "Number of failed cores", labelsJob, nil),
		coresTimeout:       prometheus.NewDesc("slurm_cores_timeout", "Cores stopped by timeout", labelsJob, nil),
		coresPreempted:     prometheus.NewDesc("slurm_cores_preempted", "Number of preempted cores", labelsJob, nil),
		coresNodeFail:      prometheus.NewDesc("slurm_cores_node_fail", "Number of cores stopped due to node fail", labelsJob, nil),
		// Global totals — no labels, always emitted even when 0
		jobsPending:      prometheus.NewDesc("slurm_jobs_pending", "Total pending jobs in the cluster", nil, nil),
		jobsRunning:      prometheus.NewDesc("slurm_jobs_running", "Total running jobs in the cluster", nil, nil),
		jobsSuspended:    prometheus.NewDesc("slurm_jobs_suspended", "Total suspended jobs in the cluster", nil, nil),
		jobsCompleting:   prometheus.NewDesc("slurm_jobs_completing", "Total completing jobs in the cluster", nil, nil),
		jobsCompleted:    prometheus.NewDesc("slurm_jobs_completed", "Total completed jobs in the cluster", nil, nil),
		jobsConfiguring:  prometheus.NewDesc("slurm_jobs_configuring", "Total configuring jobs in the cluster", nil, nil),
		jobsFailed:       prometheus.NewDesc("slurm_jobs_failed", "Total failed jobs in the cluster", nil, nil),
		jobsTimeout:      prometheus.NewDesc("slurm_jobs_timeout", "Total jobs stopped by timeout in the cluster", nil, nil),
		jobsPreempted:    prometheus.NewDesc("slurm_jobs_preempted", "Total preempted jobs in the cluster", nil, nil),
		jobsNodeFail:     prometheus.NewDesc("slurm_jobs_node_fail", "Total jobs stopped due to node fail in the cluster", nil, nil),
		jobsCancelled:    prometheus.NewDesc("slurm_jobs_cancelled", "Total cancelled jobs in the cluster", nil, nil),
		jobsCoresRunning: prometheus.NewDesc("slurm_jobs_cores_running", "Total cores used by running jobs", nil, nil),
		jobsCoresPending: prometheus.NewDesc("slurm_jobs_cores_pending", "Total cores requested by pending jobs", nil, nil),
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
	// withTerminalStates asks squeue for every job state rather than only the
	// pending and running ones it reports by default. See issue #27.
	withTerminalStates bool
	// Global totals — no labels, always emitted even when 0
	jobsPending      *prometheus.Desc
	jobsRunning      *prometheus.Desc
	jobsSuspended    *prometheus.Desc
	jobsCompleting   *prometheus.Desc
	jobsCompleted    *prometheus.Desc
	jobsConfiguring  *prometheus.Desc
	jobsFailed       *prometheus.Desc
	jobsTimeout      *prometheus.Desc
	jobsPreempted    *prometheus.Desc
	jobsNodeFail     *prometheus.Desc
	jobsCancelled    *prometheus.Desc
	jobsCoresRunning *prometheus.Desc
	jobsCoresPending *prometheus.Desc
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
	ch <- qc.jobsPending
	ch <- qc.jobsRunning
	ch <- qc.jobsSuspended
	ch <- qc.jobsCompleting
	ch <- qc.jobsCompleted
	ch <- qc.jobsConfiguring
	ch <- qc.jobsFailed
	ch <- qc.jobsTimeout
	ch <- qc.jobsPreempted
	ch <- qc.jobsNodeFail
	ch <- qc.jobsCancelled
	ch <- qc.jobsCoresRunning
	ch <- qc.jobsCoresPending
}

func (qc *QueueCollector) Collect(ch chan<- prometheus.Metric) { _ = qc.tryCollect(ch) }

func (qc *QueueCollector) tryCollect(ch chan<- prometheus.Metric) error {
	qm, err := QueueGetMetrics(qc.logger, qc.withTerminalStates)
	if err != nil {
		qc.logger.Error("Failed to get queue metrics", "err", err)
		// Emit global totals at 0 so they remain present in Prometheus
		// even when squeue is unavailable. Per-user metrics are omitted.
		// The zero value is enough: emitGlobalTotals reads jobTotals and
		// coreTotals, and a nil map reads as 0.
		qc.emitGlobalTotals(ch, &QueueMetrics{})
		return err
	}
	if qc.withUserLabel {
		for reason, values := range qm.pending {
			PushMetric(values, ch, qc.pending, reason)
		}
		PushMetric(qm.running, ch, qc.running, "")
		PushMetric(qm.suspended, ch, qc.suspended, "")
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
		PushMetric(qm.cSuspended, ch, qc.coresSuspended, "")
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
		pushAggregatedNVal(qm.suspended, ch, qc.suspended, "")
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
		pushAggregatedNVal(qm.cSuspended, ch, qc.coresSuspended, "")
		pushAggregatedNVal(qm.cCancelled, ch, qc.coresCancelled, "")
		pushAggregatedNVal(qm.cCompleting, ch, qc.coresCompleting, "")
		pushAggregatedNVal(qm.cCompleted, ch, qc.coresCompleted, "")
		pushAggregatedNVal(qm.cConfiguring, ch, qc.coresConfiguring, "")
		pushAggregatedNVal(qm.cFailed, ch, qc.coresFailed, "")
		pushAggregatedNVal(qm.cTimeout, ch, qc.coresTimeout, "")
		pushAggregatedNVal(qm.cPreempted, ch, qc.coresPreempted, "")
		pushAggregatedNVal(qm.cNodeFail, ch, qc.coresNodeFail, "")
	}
	// Global totals: always emitted even when 0, regardless of withUserLabel.
	qc.emitGlobalTotals(ch, qm)

	return nil
}

// emitGlobalTotals emits the 13 cluster-wide job/core metrics.
// Called both on success and on squeue error (with empty metrics) so these
// series are always present in Prometheus, enabling reliable alerting.
func (qc *QueueCollector) emitGlobalTotals(ch chan<- prometheus.Metric, qm *QueueMetrics) {
	ch <- prometheus.MustNewConstMetric(qc.jobsPending, prometheus.GaugeValue, qm.jobTotals["PENDING"])
	ch <- prometheus.MustNewConstMetric(qc.jobsRunning, prometheus.GaugeValue, qm.jobTotals["RUNNING"])
	ch <- prometheus.MustNewConstMetric(qc.jobsSuspended, prometheus.GaugeValue, qm.jobTotals["SUSPENDED"])
	ch <- prometheus.MustNewConstMetric(qc.jobsCompleting, prometheus.GaugeValue, qm.jobTotals["COMPLETING"])
	ch <- prometheus.MustNewConstMetric(qc.jobsCompleted, prometheus.GaugeValue, qm.jobTotals["COMPLETED"])
	ch <- prometheus.MustNewConstMetric(qc.jobsConfiguring, prometheus.GaugeValue, qm.jobTotals["CONFIGURING"])
	ch <- prometheus.MustNewConstMetric(qc.jobsFailed, prometheus.GaugeValue, qm.jobTotals["FAILED"])
	ch <- prometheus.MustNewConstMetric(qc.jobsTimeout, prometheus.GaugeValue, qm.jobTotals["TIMEOUT"])
	ch <- prometheus.MustNewConstMetric(qc.jobsPreempted, prometheus.GaugeValue, qm.jobTotals["PREEMPTED"])
	ch <- prometheus.MustNewConstMetric(qc.jobsNodeFail, prometheus.GaugeValue, qm.jobTotals["NODE_FAIL"])
	ch <- prometheus.MustNewConstMetric(qc.jobsCancelled, prometheus.GaugeValue, qm.jobTotals["CANCELLED"])
	ch <- prometheus.MustNewConstMetric(qc.jobsCoresRunning, prometheus.GaugeValue, qm.coreTotals["RUNNING"])
	ch <- prometheus.MustNewConstMetric(qc.jobsCoresPending, prometheus.GaugeValue, qm.coreTotals["PENDING"])
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

// sumNVal sums all values in a NVal (user->partition->count) map.
func sumNVal(m NVal) float64 {
	var total float64
	for _, partitionMap := range m {
		for _, val := range partitionMap {
			total += val
		}
	}
	return total
}

// sumNNVal sums all values in a NNVal (reason->user->partition->count) map.
func sumNNVal(m NNVal) float64 {
	var total float64
	for _, userMap := range m {
		for _, partitionMap := range userMap {
			for _, val := range partitionMap {
				total += val
			}
		}
	}
	return total
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
