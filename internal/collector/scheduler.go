package collector

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/mike8161990/slurm_exporter/internal/logger"
	"github.com/prometheus/client_golang/prometheus"
)

// SchedulerMetrics holds performance statistics from the Slurm scheduler daemon
type SchedulerMetrics struct {
	threads                           float64            // Number of scheduler threads
	queue_size                        float64            // Length of the scheduler queue
	dbd_queue_size                    float64            // Length of the DBD agent queue
	last_cycle                        float64            // Last scheduler cycle time (microseconds)
	mean_cycle                        float64            // Mean scheduler cycle time (microseconds)
	cycle_per_minute                  float64            // Number of scheduler cycles per minute
	backfill_last_cycle               float64            // Last backfill cycle time (microseconds)
	backfill_mean_cycle               float64            // Mean backfill cycle time (microseconds)
	backfill_depth_mean               float64            // Mean backfill depth
	total_backfilled_jobs_since_start float64            // Total backfilled jobs since Slurm start
	total_backfilled_jobs_since_cycle float64            // Total backfilled jobs since stats cycle start
	total_backfilled_heterogeneous    float64            // Total backfilled heterogeneous job components
	rpc_stats_count                   map[string]float64 // RPC call counts by operation
	rpc_stats_avg_time                map[string]float64 // RPC average times by operation
	rpc_stats_total_time              map[string]float64 // RPC total times by operation
	user_rpc_stats_count              map[string]float64 // RPC call counts by user
	user_rpc_stats_avg_time           map[string]float64 // RPC average times by user
	user_rpc_stats_total_time         map[string]float64 // RPC total times by user
}

// SchedulerData executes the sdiag command to retrieve scheduler statistics
func SchedulerData(logger *logger.Logger) ([]byte, error) {
	return Execute(logger, "sdiag", nil)
}

// ParseSchedulerMetrics parses the output of the sdiag command
// It handles the fact that 'Last cycle' and 'Mean cycle' appear twice in sdiag output
// (once for main scheduler, once for backfill scheduler)
func ParseSchedulerMetrics(input []byte) *SchedulerMetrics {
	var sm SchedulerMetrics
	lines := strings.Split(string(input), "\n")

	// Counters to handle duplicate metric names in sdiag output
	lastCycleCount := 0
	meanCycleCount := 0
	// Define regex patterns for matching sdiag output lines
	patterns := map[string]*regexp.Regexp{
		"threads":     regexp.MustCompile(`^Server thread`),
		"queue":       regexp.MustCompile(`^Agent queue`),
		"dbd":         regexp.MustCompile(`^DBD Agent`),
		"lastCycle":   regexp.MustCompile(`^[\s]+Last cycle$`),
		"meanCycle":   regexp.MustCompile(`^[\s]+Mean cycle$`),
		"cyclesPer":   regexp.MustCompile(`^[\s]+Cycles per`),
		"depthMean":   regexp.MustCompile(`^[\s]+Depth Mean$`),
		"totalStart":  regexp.MustCompile(`^[\s]+Total backfilled jobs \(since last slurm start\)`),
		"totalCycle":  regexp.MustCompile(`^[\s]+Total backfilled jobs \(since last stats cycle start\)`),
		"totalHetero": regexp.MustCompile(`^[\s]+Total backfilled heterogeneous job components`),
	}

	for _, line := range lines {
		if !strings.Contains(line, ":") {
			continue
		}

		parts := strings.Split(line, ":")
		if len(parts) < 2 {
			continue
		}

		key := parts[0]
		value := strings.TrimSpace(parts[1])
		floatValue, _ := strconv.ParseFloat(value, 64)

		switch {
		case patterns["threads"].MatchString(key):
			sm.threads = floatValue
		case patterns["queue"].MatchString(key):
			sm.queue_size = floatValue
		case patterns["dbd"].MatchString(key):
			sm.dbd_queue_size = floatValue
		case patterns["lastCycle"].MatchString(key):
			if lastCycleCount == 0 {
				sm.last_cycle = floatValue
				lastCycleCount++
			} else {
				sm.backfill_last_cycle = floatValue
			}
		case patterns["meanCycle"].MatchString(key):
			if meanCycleCount == 0 {
				sm.mean_cycle = floatValue
				meanCycleCount++
			} else {
				sm.backfill_mean_cycle = floatValue
			}
		case patterns["cyclesPer"].MatchString(key):
			sm.cycle_per_minute = floatValue
		case patterns["depthMean"].MatchString(key):
			sm.backfill_depth_mean = floatValue
		case patterns["totalStart"].MatchString(key):
			sm.total_backfilled_jobs_since_start = floatValue
		case patterns["totalCycle"].MatchString(key):
			sm.total_backfilled_jobs_since_cycle = floatValue
		case patterns["totalHetero"].MatchString(key):
			sm.total_backfilled_heterogeneous = floatValue
		}
	}

	// Parse RPC statistics sections
	rpcStats := ParseRpcStats(lines)
	sm.rpc_stats_count = rpcStats[0]
	sm.rpc_stats_avg_time = rpcStats[1]
	sm.rpc_stats_total_time = rpcStats[2]
	sm.user_rpc_stats_count = rpcStats[3]
	sm.user_rpc_stats_avg_time = rpcStats[4]
	sm.user_rpc_stats_total_time = rpcStats[5]

	return &sm
}

// SplitColonValueToFloat extracts a float64 value from a "key: value" formatted string
func SplitColonValueToFloat(input string) float64 {
	parts := strings.Split(input, ":")
	if len(parts) < 2 {
		return 0
	}
	value := strings.TrimSpace(parts[1])
	result, _ := strconv.ParseFloat(value, 64)
	return result
}

// ParseRpcStats parses RPC statistics sections from sdiag output
// Returns slice of maps: [count_stats, avg_stats, total_stats, user_count_stats, user_avg_stats, user_total_stats]
func ParseRpcStats(lines []string) []map[string]float64 {
	// Initialize result maps
	countStats := make(map[string]float64)
	avgStats := make(map[string]float64)
	totalStats := make(map[string]float64)
	userCountStats := make(map[string]float64)
	userAvgStats := make(map[string]float64)
	userTotalStats := make(map[string]float64)

	// State tracking for parsing sections
	inRPC := false
	inRPCPerUser := false

	// Regex to match RPC statistics lines
	statLineRe := regexp.MustCompile(`^\s*([A-Za-z0-9_]*).*count:([0-9]*)\s*ave_time:([0-9]*)\s\s*total_time:([0-9]*)\s*$`)

	for _, line := range lines {
		// Detect section transitions
		if strings.Contains(line, "Remote Procedure Call statistics by message type") {
			inRPC = true
			inRPCPerUser = false
		} else if strings.Contains(line, "Remote Procedure Call statistics by user") {
			inRPC = false
			inRPCPerUser = true
		}

		// Parse statistics lines in current section
		if inRPC || inRPCPerUser {
			matches := statLineRe.FindAllStringSubmatch(line, -1)
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
				} else if inRPCPerUser {
					userCountStats[name] = count
					userAvgStats[name] = avgTime
					userTotalStats[name] = totalTime
				}
			}
		}
	}

	return []map[string]float64{
		countStats,
		avgStats,
		totalStats,
		userCountStats,
		userAvgStats,
		userTotalStats,
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
	threads                           *prometheus.Desc
	queue_size                        *prometheus.Desc
	dbd_queue_size                    *prometheus.Desc
	last_cycle                        *prometheus.Desc
	mean_cycle                        *prometheus.Desc
	cycle_per_minute                  *prometheus.Desc
	backfill_last_cycle               *prometheus.Desc
	backfill_mean_cycle               *prometheus.Desc
	backfill_depth_mean               *prometheus.Desc
	total_backfilled_jobs_since_start *prometheus.Desc
	total_backfilled_jobs_since_cycle *prometheus.Desc
	total_backfilled_heterogeneous    *prometheus.Desc
	rpc_stats_count                   *prometheus.Desc
	rpc_stats_avg_time                *prometheus.Desc
	rpc_stats_total_time              *prometheus.Desc
	user_rpc_stats_count              *prometheus.Desc
	user_rpc_stats_avg_time           *prometheus.Desc
	user_rpc_stats_total_time         *prometheus.Desc
	logger                            *logger.Logger
}

func (c *SchedulerCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.threads
	ch <- c.queue_size
	ch <- c.dbd_queue_size
	ch <- c.last_cycle
	ch <- c.mean_cycle
	ch <- c.cycle_per_minute
	ch <- c.backfill_last_cycle
	ch <- c.backfill_mean_cycle
	ch <- c.backfill_depth_mean
	ch <- c.total_backfilled_jobs_since_start
	ch <- c.total_backfilled_jobs_since_cycle
	ch <- c.total_backfilled_heterogeneous
	ch <- c.rpc_stats_count
	ch <- c.rpc_stats_avg_time
	ch <- c.rpc_stats_total_time
	ch <- c.user_rpc_stats_count
	ch <- c.user_rpc_stats_avg_time
	ch <- c.user_rpc_stats_total_time
}

func (sc *SchedulerCollector) Collect(ch chan<- prometheus.Metric) {
	sm, err := SchedulerGetMetrics(sc.logger)
	if err != nil {
		sc.logger.Error("Failed to get scheduler metrics", "err", err)
		return
	}
	ch <- prometheus.MustNewConstMetric(sc.threads, prometheus.GaugeValue, sm.threads)
	ch <- prometheus.MustNewConstMetric(sc.queue_size, prometheus.GaugeValue, sm.queue_size)
	ch <- prometheus.MustNewConstMetric(sc.dbd_queue_size, prometheus.GaugeValue, sm.dbd_queue_size)
	ch <- prometheus.MustNewConstMetric(sc.last_cycle, prometheus.GaugeValue, sm.last_cycle)
	ch <- prometheus.MustNewConstMetric(sc.mean_cycle, prometheus.GaugeValue, sm.mean_cycle)
	ch <- prometheus.MustNewConstMetric(sc.cycle_per_minute, prometheus.GaugeValue, sm.cycle_per_minute)
	ch <- prometheus.MustNewConstMetric(sc.backfill_last_cycle, prometheus.GaugeValue, sm.backfill_last_cycle)
	ch <- prometheus.MustNewConstMetric(sc.backfill_mean_cycle, prometheus.GaugeValue, sm.backfill_mean_cycle)
	ch <- prometheus.MustNewConstMetric(sc.backfill_depth_mean, prometheus.GaugeValue, sm.backfill_depth_mean)
	ch <- prometheus.MustNewConstMetric(sc.total_backfilled_jobs_since_start, prometheus.GaugeValue, sm.total_backfilled_jobs_since_start)
	ch <- prometheus.MustNewConstMetric(sc.total_backfilled_jobs_since_cycle, prometheus.GaugeValue, sm.total_backfilled_jobs_since_cycle)
	ch <- prometheus.MustNewConstMetric(sc.total_backfilled_heterogeneous, prometheus.GaugeValue, sm.total_backfilled_heterogeneous)
	for rpc_type, value := range sm.rpc_stats_count {
		ch <- prometheus.MustNewConstMetric(sc.rpc_stats_count, prometheus.GaugeValue, value, rpc_type)
	}
	for rpc_type, value := range sm.rpc_stats_avg_time {
		ch <- prometheus.MustNewConstMetric(sc.rpc_stats_avg_time, prometheus.GaugeValue, value, rpc_type)
	}
	for rpc_type, value := range sm.rpc_stats_total_time {
		ch <- prometheus.MustNewConstMetric(sc.rpc_stats_total_time, prometheus.GaugeValue, value, rpc_type)
	}
	for user, value := range sm.user_rpc_stats_count {
		ch <- prometheus.MustNewConstMetric(sc.user_rpc_stats_count, prometheus.GaugeValue, value, user)
	}
	for user, value := range sm.user_rpc_stats_avg_time {
		ch <- prometheus.MustNewConstMetric(sc.user_rpc_stats_avg_time, prometheus.GaugeValue, value, user)
	}
	for user, value := range sm.user_rpc_stats_total_time {
		ch <- prometheus.MustNewConstMetric(sc.user_rpc_stats_total_time, prometheus.GaugeValue, value, user)
	}

}

func NewSchedulerCollector(logger *logger.Logger) *SchedulerCollector {
	rpc_stats_labels := make([]string, 0, 1)
	rpc_stats_labels = append(rpc_stats_labels, "operation")
	user_rpc_stats_labels := make([]string, 0, 1)
	user_rpc_stats_labels = append(user_rpc_stats_labels, "user")
	return &SchedulerCollector{
		threads: prometheus.NewDesc(
			"slurm_scheduler_threads",
			"Information provided by the Slurm sdiag command, number of scheduler threads ",
			nil,
			nil),
		queue_size: prometheus.NewDesc(
			"slurm_scheduler_queue_size",
			"Information provided by the Slurm sdiag command, length of the scheduler queue",
			nil,
			nil),
		dbd_queue_size: prometheus.NewDesc(
			"slurm_scheduler_dbd_queue_size",
			"Information provided by the Slurm sdiag command, length of the DBD agent queue",
			nil,
			nil),
		last_cycle: prometheus.NewDesc(
			"slurm_scheduler_last_cycle",
			"Information provided by the Slurm sdiag command, scheduler last cycle time in (microseconds)",
			nil,
			nil),
		mean_cycle: prometheus.NewDesc(
			"slurm_scheduler_mean_cycle",
			"Information provided by the Slurm sdiag command, scheduler mean cycle time in (microseconds)",
			nil,
			nil),
		cycle_per_minute: prometheus.NewDesc(
			"slurm_scheduler_cycle_per_minute",
			"Information provided by the Slurm sdiag command, number scheduler cycles per minute",
			nil,
			nil),
		backfill_last_cycle: prometheus.NewDesc(
			"slurm_scheduler_backfill_last_cycle",
			"Information provided by the Slurm sdiag command, scheduler backfill last cycle time in (microseconds)",
			nil,
			nil),
		backfill_mean_cycle: prometheus.NewDesc(
			"slurm_scheduler_backfill_mean_cycle",
			"Information provided by the Slurm sdiag command, scheduler backfill mean cycle time in (microseconds)",
			nil,
			nil),
		backfill_depth_mean: prometheus.NewDesc(
			"slurm_scheduler_backfill_depth_mean",
			"Information provided by the Slurm sdiag command, scheduler backfill mean depth",
			nil,
			nil),
		total_backfilled_jobs_since_start: prometheus.NewDesc(
			"slurm_scheduler_backfilled_jobs_since_start_total",
			"Information provided by the Slurm sdiag command, number of jobs started thanks to backfilling since last slurm start",
			nil,
			nil),
		total_backfilled_jobs_since_cycle: prometheus.NewDesc(
			"slurm_scheduler_backfilled_jobs_since_cycle_total",
			"Information provided by the Slurm sdiag command, number of jobs started thanks to backfilling since last time stats where reset",
			nil,
			nil),
		total_backfilled_heterogeneous: prometheus.NewDesc(
			"slurm_scheduler_backfilled_heterogeneous_total",
			"Information provided by the Slurm sdiag command, number of heterogeneous job components started thanks to backfilling since last Slurm start",
			nil,
			nil),
		rpc_stats_count: prometheus.NewDesc(
			"slurm_rpc_stats",
			"Information provided by the Slurm sdiag command, rpc count statistic",
			rpc_stats_labels,
			nil),
		rpc_stats_avg_time: prometheus.NewDesc(
			"slurm_rpc_stats_avg_time",
			"Information provided by the Slurm sdiag command, rpc average time statistic",
			rpc_stats_labels,
			nil),
		rpc_stats_total_time: prometheus.NewDesc(
			"slurm_rpc_stats_total_time",
			"Information provided by the Slurm sdiag command, rpc total time statistic",
			rpc_stats_labels,
			nil),
		user_rpc_stats_count: prometheus.NewDesc(
			"slurm_user_rpc_stats",
			"Information provided by the Slurm sdiag command, rpc count statistic per user",
			user_rpc_stats_labels,
			nil),
		user_rpc_stats_avg_time: prometheus.NewDesc(
			"slurm_user_rpc_stats_avg_time",
			"Information provided by the Slurm sdiag command, rpc average time statistic per user",
			user_rpc_stats_labels,
			nil),
		user_rpc_stats_total_time: prometheus.NewDesc(
			"slurm_user_rpc_stats_total_time",
			"Information provided by the Slurm sdiag command, rpc total time statistic per user",
			user_rpc_stats_labels,
			nil),
		logger: logger,
	}
}
