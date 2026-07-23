package collector

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// SacctJobRecord holds the raw fields parsed from one sacct line.
type SacctJobRecord struct {
	User      string
	Account   string
	AllocCPUs float64
	// Wall-clock time actually used (seconds)
	ElapsedSeconds float64
	// Actual CPU time consumed (user + system, seconds)
	TotalCPUSeconds float64
	// CPU time allocated (AllocCPUs × Elapsed, seconds)
	CPUTimeSeconds float64
	// Peak memory used (MB), aggregated as the max over the job's steps.
	MaxRSSMB float64
	// MaxRSSPresent is true only when at least one step reported a MaxRSS value.
	// A job whose accounting has no MaxRSS (steps never ran, jobacct_gather off,
	// or -X used) must be left out of the memory-efficiency average rather than
	// folded in at 0% (issue #143).
	MaxRSSPresent bool
	// Memory requested (MB)
	ReqMemMB float64
}

// parseSacctDuration converts Slurm duration format to seconds.
// Accepted formats: [D-]HH:MM:SS or MM:SS or SS
func parseSacctDuration(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return 0
	}
	days := 0.0
	if idx := strings.Index(s, "-"); idx != -1 {
		d, _ := strconv.ParseFloat(s[:idx], 64)
		days = d
		s = s[idx+1:]
	}
	parts := strings.Split(s, ":")
	var h, m, sec float64
	switch len(parts) {
	case 3:
		h, _ = strconv.ParseFloat(parts[0], 64)
		m, _ = strconv.ParseFloat(parts[1], 64)
		sec, _ = strconv.ParseFloat(parts[2], 64)
	case 2:
		m, _ = strconv.ParseFloat(parts[0], 64)
		sec, _ = strconv.ParseFloat(parts[1], 64)
	case 1:
		sec, _ = strconv.ParseFloat(parts[0], 64)
	}
	return days*86400 + h*3600 + m*60 + sec
}

// parseSacctMemory converts a Slurm memory string to MB (MiB) and reports
// whether it parsed a real value. Accepted suffixes are K, M, G, T and P
// (Slurm's memory units), optionally followed by 'n' (per-node) or 'c'
// (per-cpu). An empty string, "0", the "16?" placeholder, or anything that
// fails to parse returns (0, false) so callers can tell "no data" from a
// genuine zero rather than folding a phantom 0% into an average (issue #143).
func parseSacctMemory(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" || s == "16?" {
		return 0, false
	}
	// ReqMem may have 'n' (per-node) or 'c' (per-cpu) suffix — strip it.
	s = strings.TrimRight(s, "nc")
	multiplier := 1.0
	switch {
	case strings.HasSuffix(s, "P"):
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "P")
	case strings.HasSuffix(s, "T"):
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "T")
	case strings.HasSuffix(s, "G"):
		multiplier = 1024
		s = strings.TrimSuffix(s, "G")
	case strings.HasSuffix(s, "M"):
		s = strings.TrimSuffix(s, "M")
	case strings.HasSuffix(s, "K"):
		multiplier = 1.0 / 1024
		s = strings.TrimSuffix(s, "K")
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v * multiplier, true
}

// ParseSacctEfficiency parses sacct -P -n output (steps included, no -X) into
// per-job records.
// Expected format: JobID|User|Account|AllocCPUS|Elapsed|TotalCPU|CPUTime|MaxRSS|ReqMem
//
// MaxRSS is a step-level statistic: it is empty on the allocation (JobID "123")
// line and only carried by the step lines ("123.batch", "123.0", …). The old
// query used -X, which returns allocation lines only, so MaxRSS came back empty
// for every job and the memory-efficiency metric was pinned at 0 (issue #143).
// We now read steps too, correlate them to their job by the JobID prefix before
// the first '.', and take the peak MaxRSS across the job's steps. Identity and
// requested resources (User, Account, AllocCPUS, times, ReqMem) come from the
// allocation line, which is authoritative for them.
func ParseSacctEfficiency(input []byte) []SacctJobRecord {
	// job accumulates one job's allocation line plus the peak MaxRSS seen across
	// its step lines, regardless of the order they arrive in.
	type job struct {
		rec       SacctJobRecord
		haveAlloc bool
		maxRSS    float64
		rssSeen   bool
	}
	order := make([]string, 0)
	byID := make(map[string]*job)

	for _, line := range strings.Split(string(input), "\n") {
		fields := strings.Split(line, "|")
		if len(fields) < 9 {
			continue
		}
		jobID := strings.TrimSpace(fields[0])
		if jobID == "" {
			continue
		}
		baseID := jobID
		isStep := false
		if dot := strings.IndexByte(jobID, '.'); dot != -1 {
			baseID = jobID[:dot]
			isStep = true
		}

		j := byID[baseID]
		if j == nil {
			j = &job{}
			byID[baseID] = j
			order = append(order, baseID)
		}

		// MaxRSS lives on step lines; keep the peak across all of the job's steps.
		if rss, ok := parseSacctMemory(fields[7]); ok && (!j.rssSeen || rss > j.maxRSS) {
			j.maxRSS = rss
			j.rssSeen = true
		}

		if isStep {
			continue // step lines carry only step-level stats
		}

		// Allocation line: authoritative for identity and requested resources.
		user := strings.TrimSpace(fields[1])
		account := strings.TrimSpace(fields[2])
		if user == "" || account == "" {
			continue
		}
		alloc, _ := strconv.ParseFloat(strings.TrimSpace(fields[3]), 64)
		elapsed := parseSacctDuration(fields[4])
		if alloc == 0 || elapsed == 0 {
			continue // skip jobs with no resource usage
		}
		reqMem, _ := parseSacctMemory(fields[8])
		j.rec = SacctJobRecord{
			User:            user,
			Account:         account,
			AllocCPUs:       alloc,
			ElapsedSeconds:  elapsed,
			TotalCPUSeconds: parseSacctDuration(fields[5]),
			CPUTimeSeconds:  parseSacctDuration(fields[6]),
			ReqMemMB:        reqMem,
		}
		j.haveAlloc = true
	}

	records := make([]SacctJobRecord, 0, len(order))
	for _, id := range order {
		j := byID[id]
		if !j.haveAlloc {
			continue // steps whose allocation line was missing or filtered out
		}
		j.rec.MaxRSSMB = j.maxRSS
		j.rec.MaxRSSPresent = j.rssSeen
		records = append(records, j.rec)
	}
	return records
}

// SacctEfficiencyAggregates holds aggregated efficiency stats per user+account.
type SacctEfficiencyAggregates struct {
	JobCount          float64
	CPUJobCount       float64 // jobs where CPUTime > 0 (denominator for CPUEfficiencyPct)
	MemJobCount       float64 // jobs where ReqMem > 0 (denominator for MemEfficiencyPct)
	CPUEfficiencyPct  float64 // avg(TotalCPU / CPUTime * 100)
	MemEfficiencyPct  float64 // avg(MaxRSS / ReqMem * 100), only for jobs with ReqMem>0
	CPUHoursAllocated float64
}

// AggregateSacctEfficiency groups job records by user+account and computes averages.
func AggregateSacctEfficiency(records []SacctJobRecord) map[string]map[string]*SacctEfficiencyAggregates {
	// result[account][user]
	result := make(map[string]map[string]*SacctEfficiencyAggregates)

	for _, r := range records {
		if _, ok := result[r.Account]; !ok {
			result[r.Account] = make(map[string]*SacctEfficiencyAggregates)
		}
		agg, ok := result[r.Account][r.User]
		if !ok {
			agg = &SacctEfficiencyAggregates{}
			result[r.Account][r.User] = agg
		}

		agg.JobCount++
		agg.CPUHoursAllocated += r.CPUTimeSeconds / 3600

		if r.CPUTimeSeconds > 0 {
			agg.CPUEfficiencyPct += r.TotalCPUSeconds / r.CPUTimeSeconds * 100
			agg.CPUJobCount++
		}
		// Only jobs that both requested memory and have a recorded MaxRSS
		// contribute to the memory-efficiency average. A missing MaxRSS means
		// "no data", not "0% efficient" (issue #143).
		if r.ReqMemMB > 0 && r.MaxRSSPresent {
			agg.MemEfficiencyPct += r.MaxRSSMB / r.ReqMemMB * 100
			agg.MemJobCount++
		}
	}

	// Convert sums to averages using per-metric job counts as denominators.
	// This avoids understating averages when some jobs lack CPU-time or memory data.
	for _, users := range result {
		for _, agg := range users {
			if agg.CPUJobCount > 0 {
				agg.CPUEfficiencyPct /= agg.CPUJobCount
			}
			if agg.MemJobCount > 0 {
				agg.MemEfficiencyPct /= agg.MemJobCount
			}
		}
	}
	return result
}

// ── Collector ─────────────────────────────────────────────────────────────────

// SacctEfficiencyCollector collects job efficiency metrics via sacct.
// It runs sacct in a background goroutine at a configurable interval to avoid
// blocking Prometheus scrapes. Results are cached and served from memory.
// Disabled by default — enable with --collector.sacct_efficiency.
type SacctEfficiencyCollector struct {
	mu          sync.RWMutex
	cached      []prometheus.Metric
	lastRefresh time.Time

	interval time.Duration
	lookback time.Duration

	cpuEfficiency     *prometheus.Desc
	memEfficiency     *prometheus.Desc
	jobsCompleted     *prometheus.Desc
	cpuHoursAllocated *prometheus.Desc
	lastRefreshDesc   *prometheus.Desc

	// done is closed when the background goroutine launched by Start() exits.
	// main() waits on it during graceful shutdown so the goroutine is finished
	// before the process exits (issue #18). Tests also use it to synchronise
	// teardown of package-level state, like the Execute mock, after cancelling
	// the context.
	done chan struct{}

	logger *logger.Logger
}

// NewSacctEfficiencyCollector creates the collector and starts the background refresh goroutine.
func NewSacctEfficiencyCollector(log *logger.Logger, interval, lookback time.Duration) *SacctEfficiencyCollector {
	labels := []string{"account", "user"}
	c := &SacctEfficiencyCollector{
		interval: interval,
		lookback: lookback,
		done:     make(chan struct{}),
		cpuEfficiency: prometheus.NewDesc(
			"slurm_job_cpu_efficiency_avg",
			"Average CPU efficiency of completed jobs (TotalCPU/CPUTime*100) aggregated by account+user over the lookback window.",
			labels, nil),
		memEfficiency: prometheus.NewDesc(
			"slurm_job_mem_efficiency_avg",
			"Average memory efficiency of completed jobs (MaxRSS/ReqMem*100) aggregated by account+user over the lookback window.",
			labels, nil),
		jobsCompleted: prometheus.NewDesc(
			"slurm_job_count_completed",
			"Number of completed jobs aggregated by account+user over the lookback window.",
			labels, nil),
		cpuHoursAllocated: prometheus.NewDesc(
			"slurm_job_cpu_hours_allocated",
			"Total CPU-hours allocated to completed jobs by account+user over the lookback window.",
			labels, nil),
		lastRefreshDesc: prometheus.NewDesc(
			"slurm_sacct_last_refresh_timestamp_seconds",
			"Unix timestamp of the last successful sacct refresh. "+
				"Alert if time()-this > 2*collector.sacct.interval.",
			nil, nil),
		logger: log,
	}
	return c
}

// Start launches the background refresh goroutine. Call once after construction.
// The goroutine exits when ctx is cancelled; Done() can be used to wait for it.
func (c *SacctEfficiencyCollector) Start(ctx context.Context) {
	go func() {
		defer close(c.done)
		c.refresh()
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				c.refresh()
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Done returns a channel that is closed when the background refresh goroutine
// started by Start() has fully exited. Useful in tests to synchronise teardown
// (e.g. restoring a mocked package-level Execute) after cancelling the context.
func (c *SacctEfficiencyCollector) Done() <-chan struct{} {
	return c.done
}

func (c *SacctEfficiencyCollector) refresh() {
	now := time.Now()
	startTime := now.Add(-c.lookback).Format("2006-01-02T15:04:05")
	endTime := now.Format("2006-01-02T15:04:05")
	// No -X: we need the step lines, because MaxRSS is a step-level statistic and
	// is empty on the allocation line. JobID leads the format so ParseSacctEfficiency
	// can correlate steps back to their job (issue #143).
	//
	// --endtime is required: with --state and only --starttime, sacct returns no
	// rows at all (Slurm bounds a state-filtered search to [starttime, endtime]
	// and the endtime default does not cover our window). Without it the whole
	// collector reported nothing, not just memory (issue #143).
	data, err := Execute(c.logger, "sacct", []string{
		"-P", "-n",
		"--starttime", startTime,
		"--endtime", endTime,
		"--format", "JobID,User,Account,AllocCPUS,Elapsed,TotalCPU,CPUTime,MaxRSS,ReqMem",
		"--state", "COMPLETED,FAILED,TIMEOUT,CANCELLED",
	})
	if err != nil {
		c.logger.Error("sacct refresh failed — keeping previous cache", "err", err)
		return
	}

	records := ParseSacctEfficiency(data)
	aggregates := AggregateSacctEfficiency(records)

	var metrics []prometheus.Metric
	for account, users := range aggregates {
		for user, agg := range users {
			metrics = append(metrics,
				prometheus.MustNewConstMetric(c.cpuEfficiency, prometheus.GaugeValue, agg.CPUEfficiencyPct, account, user),
				prometheus.MustNewConstMetric(c.jobsCompleted, prometheus.GaugeValue, agg.JobCount, account, user),
				prometheus.MustNewConstMetric(c.cpuHoursAllocated, prometheus.GaugeValue, agg.CPUHoursAllocated, account, user),
			)
			// Emit memory efficiency only when at least one job had a MaxRSS to
			// average. Without this, sites whose accounting has no MaxRSS would
			// report a permanent 0, so a SlurmLowMemEfficiency alert built on it
			// could never stop firing (issue #143).
			if agg.MemJobCount > 0 {
				metrics = append(metrics,
					prometheus.MustNewConstMetric(c.memEfficiency, prometheus.GaugeValue, agg.MemEfficiencyPct, account, user))
			}
		}
	}

	c.mu.Lock()
	c.cached = metrics
	c.lastRefresh = time.Now()
	c.mu.Unlock()
}

func (c *SacctEfficiencyCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.cpuEfficiency
	ch <- c.memEfficiency
	ch <- c.jobsCompleted
	ch <- c.cpuHoursAllocated
	ch <- c.lastRefreshDesc
}

// Collect returns cached metrics — non-blocking, O(cached metrics) time.
func (c *SacctEfficiencyCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, m := range c.cached {
		ch <- m
	}
	if !c.lastRefresh.IsZero() {
		ch <- prometheus.MustNewConstMetric(
			c.lastRefreshDesc,
			prometheus.GaugeValue,
			float64(c.lastRefresh.Unix()),
		)
	}
}
