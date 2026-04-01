package collector

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// Pre-compiled regex patterns for sdiag output line matching.
// Defined at package level to avoid recompilation on every Collect() call.
var (
	schedulerPatternThreads     = regexp.MustCompile(`^Server thread`)
	schedulerPatternQueue       = regexp.MustCompile(`^Agent queue`)
	schedulerPatternDBD         = regexp.MustCompile(`^DBD Agent queue size`)
	schedulerPatternLastCycle   = regexp.MustCompile(`^[\s]+Last cycle$`)
	schedulerPatternMeanCycle   = regexp.MustCompile(`^[\s]+Mean cycle$`)
	schedulerPatternCyclesPer   = regexp.MustCompile(`^[\s]+Cycles per`)
	schedulerPatternDepthMean   = regexp.MustCompile(`^[\s]+Depth Mean$`)
	schedulerPatternTotalStart  = regexp.MustCompile(`^[\s]+Total backfilled jobs \(since last slurm start\)`)
	schedulerPatternTotalCycle  = regexp.MustCompile(`^[\s]+Total backfilled jobs \(since last stats cycle start\)`)
	schedulerPatternTotalHetero = regexp.MustCompile(`^[\s]+Total backfilled heterogeneous job components`)
	schedulerRPCLineRe          = regexp.MustCompile(`^\s*([A-Za-z0-9_]*).*count:([0-9]*)\s*ave_time:([0-9]*)\s\s*total_time:([0-9]*)\s*$`)

	// Job counters (sdiag "Jobs submitted/started/completed/canceled/failed")
	schedulerPatternJobsSubmitted = regexp.MustCompile(`^Jobs submitted`)
	schedulerPatternJobsStarted   = regexp.MustCompile(`^Jobs started`)
	schedulerPatternJobsCompleted = regexp.MustCompile(`^Jobs completed`)
	schedulerPatternJobsCanceled  = regexp.MustCompile(`^Jobs canceled`)
	schedulerPatternJobsFailed    = regexp.MustCompile(`^Jobs failed`)
)

// SchedulerMetrics holds performance statistics from the Slurm scheduler daemon
type SchedulerMetrics struct {
	threads                       float64 // Number of scheduler threads
	queueSize                     float64 // Length of the scheduler queue
	dbdQueueSize                  float64 // Length of the DBD agent queue
	lastCycle                     float64 // Last scheduler cycle time (microseconds)
	meanCycle                     float64 // Mean scheduler cycle time (microseconds)
	cyclePerMinute                float64 // Number of scheduler cycles per minute
	backfillLastCycle             float64 // Last backfill cycle time (microseconds)
	backfillMeanCycle             float64 // Mean backfill cycle time (microseconds)
	backfillDepthMean             float64 // Mean backfill depth
	totalBackfilledJobsSinceStart float64 // Total backfilled jobs since Slurm start
	totalBackfilledJobsSinceCycle float64 // Total backfilled jobs since stats cycle start
	totalBackfilledHeterogeneous  float64 // Total backfilled heterogeneous job components
	// Job lifecycle counters (since last stats reset)
	jobsSubmitted         float64            // Jobs submitted since last stats reset
	jobsStarted           float64            // Jobs started (dispatched) since last stats reset
	jobsCompleted         float64            // Jobs completed since last stats reset
	jobsCanceled          float64            // Jobs canceled since last stats reset
	jobsFailed            float64            // Jobs failed since last stats reset
	rpcStatsCount         map[string]float64 // RPC call counts by operation
	rpcStatsAvgTime       map[string]float64 // RPC average times by operation
	rpcStatsTotalTime     map[string]float64 // RPC total times by operation
	userRPCStatsCount     map[string]float64 // RPC call counts by user
	userRPCStatsAvgTime   map[string]float64 // RPC average times by user
	userRPCStatsTotalTime map[string]float64 // RPC total times by user
}

// SchedulerData executes the sdiag command to retrieve scheduler statistics
func SchedulerData(logger *logger.Logger) ([]byte, error) {
	return Execute(logger, "sdiag", nil)
}

// ParseSchedulerMetrics parses the output of the sdiag command.
// It handles the fact that 'Last cycle' and 'Mean cycle' appear twice in sdiag output
// (once for main scheduler, once for backfill scheduler).
func ParseSchedulerMetrics(input []byte) *SchedulerMetrics {
	var sm SchedulerMetrics
	lines := strings.Split(string(input), "\n")

	// Counters to handle duplicate metric names in sdiag output
	lastCycleCount := 0
	meanCycleCount := 0

	for _, line := range lines {
		if !strings.Contains(line, ":") {
			continue
		}

		// SplitN limits to 2 parts so values containing ":" (e.g. timestamps,
		// "Last cycle when: Wed Apr 12 11:03:21 2017") are not silently truncated.
		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			continue
		}

		key := parts[0]
		value := strings.TrimSpace(parts[1])
		floatValue, _ := strconv.ParseFloat(value, 64)

		switch {
		case schedulerPatternThreads.MatchString(key):
			sm.threads = floatValue
		case schedulerPatternQueue.MatchString(key):
			sm.queueSize = floatValue
		case schedulerPatternDBD.MatchString(key):
			sm.dbdQueueSize = floatValue
		case schedulerPatternLastCycle.MatchString(key):
			if lastCycleCount == 0 {
				sm.lastCycle = floatValue
				lastCycleCount++
			} else {
				sm.backfillLastCycle = floatValue
			}
		case schedulerPatternMeanCycle.MatchString(key):
			if meanCycleCount == 0 {
				sm.meanCycle = floatValue
				meanCycleCount++
			} else {
				sm.backfillMeanCycle = floatValue
			}
		case schedulerPatternCyclesPer.MatchString(key):
			sm.cyclePerMinute = floatValue
		case schedulerPatternDepthMean.MatchString(key):
			sm.backfillDepthMean = floatValue
		case schedulerPatternTotalStart.MatchString(key):
			sm.totalBackfilledJobsSinceStart = floatValue
		case schedulerPatternTotalCycle.MatchString(key):
			sm.totalBackfilledJobsSinceCycle = floatValue
		case schedulerPatternTotalHetero.MatchString(key):
			sm.totalBackfilledHeterogeneous = floatValue
		case schedulerPatternJobsSubmitted.MatchString(key):
			sm.jobsSubmitted = floatValue
		case schedulerPatternJobsStarted.MatchString(key):
			sm.jobsStarted = floatValue
		case schedulerPatternJobsCompleted.MatchString(key):
			sm.jobsCompleted = floatValue
		case schedulerPatternJobsCanceled.MatchString(key):
			sm.jobsCanceled = floatValue
		case schedulerPatternJobsFailed.MatchString(key):
			sm.jobsFailed = floatValue
		}
	}

	// Parse RPC statistics sections
	rpcStats := ParseRPCStats(lines)
	sm.rpcStatsCount = rpcStats[0]
	sm.rpcStatsAvgTime = rpcStats[1]
	sm.rpcStatsTotalTime = rpcStats[2]
	sm.userRPCStatsCount = rpcStats[3]
	sm.userRPCStatsAvgTime = rpcStats[4]
	sm.userRPCStatsTotalTime = rpcStats[5]

	return &sm
}

// ParseRPCStats parses RPC statistics sections from sdiag output.
// Returns slice of maps: [count_stats, avg_stats, total_stats, user_count_stats, user_avg_stats, user_total_stats]
func ParseRPCStats(lines []string) []map[string]float64 {
	countStats := make(map[string]float64)
	avgStats := make(map[string]float64)
	totalStats := make(map[string]float64)
	userCountStats := make(map[string]float64)
	userAvgStats := make(map[string]float64)
	userTotalStats := make(map[string]float64)

	inRPC := false
	inRPCPerUser := false

	for _, line := range lines {
		if strings.Contains(line, "Remote Procedure Call statistics by message type") {
			inRPC = true
			inRPCPerUser = false
		} else if strings.Contains(line, "Remote Procedure Call statistics by user") {
			inRPC = false
			inRPCPerUser = true
		}

		if inRPC || inRPCPerUser {
			matches := schedulerRPCLineRe.FindAllStringSubmatch(line, -1)
			if matches != nil && len(matches[0]) >= 5 {
				match := matches[0]
				name := match[1]
				count, _ := strconv.ParseFloat(match[2], 64)
				avgTime, _ := strconv.ParseFloat(match[3], 64)
				totalTime, _ := strconv.ParseFloat(match[4], 64)

				if inRPC {
					countStats[name] = count
					avgStats[name] = avgTime
					totalStats[name] = totalTime
				} else {
					userCountStats[name] = count
					userAvgStats[name] = avgTime
					userTotalStats[name] = totalTime
				}
			}
		}
	}

	return []map[string]float64{
		countStats, avgStats, totalStats,
		userCountStats, userAvgStats, userTotalStats,
	}
}

// SchedulerGetMetrics retrieves and parses scheduler metrics from Slurm
func SchedulerGetMetrics(logger *logger.Logger) (*SchedulerMetrics, error) {
	data, err := SchedulerData(logger)
	if err != nil {
		return nil, err
	}
	return ParseSchedulerMetrics(data), nil
}

// SchedulerCollector implements the Prometheus Collector interface for scheduler metrics
type SchedulerCollector struct {
	threads                       *prometheus.Desc
	queueSize                     *prometheus.Desc
	dbdQueueSize                  *prometheus.Desc
	lastCycle                     *prometheus.Desc
	meanCycle                     *prometheus.Desc
	cyclePerMinute                *prometheus.Desc
	backfillLastCycle             *prometheus.Desc
	backfillMeanCycle             *prometheus.Desc
	backfillDepthMean             *prometheus.Desc
	totalBackfilledJobsSinceStart *prometheus.Desc
	totalBackfilledJobsSinceCycle *prometheus.Desc
	totalBackfilledHeterogeneous  *prometheus.Desc
	jobsSubmitted                 *prometheus.Desc
	jobsStarted                   *prometheus.Desc
	jobsCompleted                 *prometheus.Desc
	jobsCanceled                  *prometheus.Desc
	jobsFailed                    *prometheus.Desc
	rpcStatsCount                 *prometheus.Desc
	rpcStatsAvgTime               *prometheus.Desc
	rpcStatsTotalTime             *prometheus.Desc
	userRPCStatsCount             *prometheus.Desc
	userRPCStatsAvgTime           *prometheus.Desc
	userRPCStatsTotalTime         *prometheus.Desc
	logger                        *logger.Logger
}

func (sc *SchedulerCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- sc.jobsSubmitted
	ch <- sc.jobsStarted
	ch <- sc.jobsCompleted
	ch <- sc.jobsCanceled
	ch <- sc.jobsFailed
	ch <- sc.threads
	ch <- sc.queueSize
	ch <- sc.dbdQueueSize
	ch <- sc.lastCycle
	ch <- sc.meanCycle
	ch <- sc.cyclePerMinute
	ch <- sc.backfillLastCycle
	ch <- sc.backfillMeanCycle
	ch <- sc.backfillDepthMean
	ch <- sc.totalBackfilledJobsSinceStart
	ch <- sc.totalBackfilledJobsSinceCycle
	ch <- sc.totalBackfilledHeterogeneous
	ch <- sc.rpcStatsCount
	ch <- sc.rpcStatsAvgTime
	ch <- sc.rpcStatsTotalTime
	ch <- sc.userRPCStatsCount
	ch <- sc.userRPCStatsAvgTime
	ch <- sc.userRPCStatsTotalTime
}

func (sc *SchedulerCollector) Collect(ch chan<- prometheus.Metric) {
	sm, err := SchedulerGetMetrics(sc.logger)
	if err != nil {
		sc.logger.Error("Failed to get scheduler metrics", "err", err)
		return
	}
	ch <- prometheus.MustNewConstMetric(sc.jobsSubmitted, prometheus.CounterValue, sm.jobsSubmitted)
	ch <- prometheus.MustNewConstMetric(sc.jobsStarted, prometheus.CounterValue, sm.jobsStarted)
	ch <- prometheus.MustNewConstMetric(sc.jobsCompleted, prometheus.CounterValue, sm.jobsCompleted)
	ch <- prometheus.MustNewConstMetric(sc.jobsCanceled, prometheus.CounterValue, sm.jobsCanceled)
	ch <- prometheus.MustNewConstMetric(sc.jobsFailed, prometheus.CounterValue, sm.jobsFailed)
	ch <- prometheus.MustNewConstMetric(sc.threads, prometheus.GaugeValue, sm.threads)
	ch <- prometheus.MustNewConstMetric(sc.queueSize, prometheus.GaugeValue, sm.queueSize)
	ch <- prometheus.MustNewConstMetric(sc.dbdQueueSize, prometheus.GaugeValue, sm.dbdQueueSize)
	ch <- prometheus.MustNewConstMetric(sc.lastCycle, prometheus.GaugeValue, sm.lastCycle)
	ch <- prometheus.MustNewConstMetric(sc.meanCycle, prometheus.GaugeValue, sm.meanCycle)
	ch <- prometheus.MustNewConstMetric(sc.cyclePerMinute, prometheus.GaugeValue, sm.cyclePerMinute)
	ch <- prometheus.MustNewConstMetric(sc.backfillLastCycle, prometheus.GaugeValue, sm.backfillLastCycle)
	ch <- prometheus.MustNewConstMetric(sc.backfillMeanCycle, prometheus.GaugeValue, sm.backfillMeanCycle)
	ch <- prometheus.MustNewConstMetric(sc.backfillDepthMean, prometheus.GaugeValue, sm.backfillDepthMean)
	ch <- prometheus.MustNewConstMetric(sc.totalBackfilledJobsSinceStart, prometheus.GaugeValue, sm.totalBackfilledJobsSinceStart)
	ch <- prometheus.MustNewConstMetric(sc.totalBackfilledJobsSinceCycle, prometheus.GaugeValue, sm.totalBackfilledJobsSinceCycle)
	ch <- prometheus.MustNewConstMetric(sc.totalBackfilledHeterogeneous, prometheus.GaugeValue, sm.totalBackfilledHeterogeneous)
	for rpcType, value := range sm.rpcStatsCount {
		ch <- prometheus.MustNewConstMetric(sc.rpcStatsCount, prometheus.GaugeValue, value, rpcType)
	}
	for rpcType, value := range sm.rpcStatsAvgTime {
		ch <- prometheus.MustNewConstMetric(sc.rpcStatsAvgTime, prometheus.GaugeValue, value, rpcType)
	}
	for rpcType, value := range sm.rpcStatsTotalTime {
		ch <- prometheus.MustNewConstMetric(sc.rpcStatsTotalTime, prometheus.GaugeValue, value, rpcType)
	}
	for user, value := range sm.userRPCStatsCount {
		ch <- prometheus.MustNewConstMetric(sc.userRPCStatsCount, prometheus.GaugeValue, value, user)
	}
	for user, value := range sm.userRPCStatsAvgTime {
		ch <- prometheus.MustNewConstMetric(sc.userRPCStatsAvgTime, prometheus.GaugeValue, value, user)
	}
	for user, value := range sm.userRPCStatsTotalTime {
		ch <- prometheus.MustNewConstMetric(sc.userRPCStatsTotalTime, prometheus.GaugeValue, value, user)
	}
}

// NewSchedulerCollector creates a new scheduler metrics collector
func NewSchedulerCollector(logger *logger.Logger) *SchedulerCollector {
	rpcLabels := []string{"operation"}
	userRPCLabels := []string{"user"}
	return &SchedulerCollector{
		jobsSubmitted: prometheus.NewDesc(
			"slurm_scheduler_jobs_submitted_total",
			"Total jobs submitted to the scheduler since last stats reset (sdiag)",
			nil, nil),
		jobsStarted: prometheus.NewDesc(
			"slurm_scheduler_jobs_started_total",
			"Total jobs started (dispatched) since last stats reset (sdiag)",
			nil, nil),
		jobsCompleted: prometheus.NewDesc(
			"slurm_scheduler_jobs_completed_total",
			"Total jobs completed since last stats reset (sdiag)",
			nil, nil),
		jobsCanceled: prometheus.NewDesc(
			"slurm_scheduler_jobs_canceled_total",
			"Total jobs canceled since last stats reset (sdiag)",
			nil, nil),
		jobsFailed: prometheus.NewDesc(
			"slurm_scheduler_jobs_failed_total",
			"Total jobs failed since last stats reset (sdiag)",
			nil, nil),
		threads: prometheus.NewDesc(
			"slurm_scheduler_threads",
			"Number of scheduler threads reported by sdiag",
			nil, nil),
		queueSize: prometheus.NewDesc(
			"slurm_scheduler_queue_size",
			"Length of the scheduler queue reported by sdiag",
			nil, nil),
		dbdQueueSize: prometheus.NewDesc(
			"slurm_scheduler_dbd_queue_size",
			"Length of the DBD agent queue reported by sdiag",
			nil, nil),
		lastCycle: prometheus.NewDesc(
			"slurm_scheduler_last_cycle",
			"Last scheduler cycle time in microseconds reported by sdiag",
			nil, nil),
		meanCycle: prometheus.NewDesc(
			"slurm_scheduler_mean_cycle",
			"Mean scheduler cycle time in microseconds reported by sdiag",
			nil, nil),
		cyclePerMinute: prometheus.NewDesc(
			"slurm_scheduler_cycle_per_minute",
			"Number of scheduler cycles per minute reported by sdiag",
			nil, nil),
		backfillLastCycle: prometheus.NewDesc(
			"slurm_scheduler_backfill_last_cycle",
			"Last backfill cycle time in microseconds reported by sdiag",
			nil, nil),
		backfillMeanCycle: prometheus.NewDesc(
			"slurm_scheduler_backfill_mean_cycle",
			"Mean backfill cycle time in microseconds reported by sdiag",
			nil, nil),
		backfillDepthMean: prometheus.NewDesc(
			"slurm_scheduler_backfill_depth_mean",
			"Mean backfill depth reported by sdiag",
			nil, nil),
		totalBackfilledJobsSinceStart: prometheus.NewDesc(
			"slurm_scheduler_backfilled_jobs_since_start_total",
			"Jobs started via backfilling since last Slurm start, reported by sdiag",
			nil, nil),
		totalBackfilledJobsSinceCycle: prometheus.NewDesc(
			"slurm_scheduler_backfilled_jobs_since_cycle_total",
			"Jobs started via backfilling since last stats cycle reset, reported by sdiag",
			nil, nil),
		totalBackfilledHeterogeneous: prometheus.NewDesc(
			"slurm_scheduler_backfilled_heterogeneous_total",
			"Heterogeneous job components started via backfilling since last Slurm start, reported by sdiag",
			nil, nil),
		rpcStatsCount: prometheus.NewDesc(
			"slurm_rpc_stats",
			"RPC call count by operation, reported by sdiag",
			rpcLabels, nil),
		rpcStatsAvgTime: prometheus.NewDesc(
			"slurm_rpc_stats_avg_time",
			"RPC average time by operation, reported by sdiag",
			rpcLabels, nil),
		rpcStatsTotalTime: prometheus.NewDesc(
			"slurm_rpc_stats_total_time",
			"RPC total time by operation, reported by sdiag",
			rpcLabels, nil),
		userRPCStatsCount: prometheus.NewDesc(
			"slurm_user_rpc_stats",
			"RPC call count per user, reported by sdiag",
			userRPCLabels, nil),
		userRPCStatsAvgTime: prometheus.NewDesc(
			"slurm_user_rpc_stats_avg_time",
			"RPC average time per user, reported by sdiag",
			userRPCLabels, nil),
		userRPCStatsTotalTime: prometheus.NewDesc(
			"slurm_user_rpc_stats_total_time",
			"RPC total time per user, reported by sdiag",
			userRPCLabels, nil),
		logger: logger,
	}
}
