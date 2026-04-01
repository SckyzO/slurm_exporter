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
	// Peak memory used (MB)
	MaxRSSMB float64
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

// parseSacctMemory converts Slurm memory strings to MB.
// Formats: "2G", "512M", "1024K", "4096" (bytes)
func parseSacctMemory(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" || s == "16?" {
		return 0
	}
	// ReqMem may have 'n' (per-node) or 'c' (per-cpu) suffix — strip it
	s = strings.TrimRight(s, "nc")
	multiplier := 1.0
	switch {
	case strings.HasSuffix(s, "G"):
		multiplier = 1024
		s = strings.TrimSuffix(s, "G")
	case strings.HasSuffix(s, "M"):
		s = strings.TrimSuffix(s, "M")
	case strings.HasSuffix(s, "K"):
		multiplier = 1.0 / 1024
		s = strings.TrimSuffix(s, "K")
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v * multiplier
}

// ParseSacctEfficiency parses sacct -X -P -n output into per-job records.
// Expected format: User|Account|AllocCPUS|Elapsed|TotalCPU|CPUTime|MaxRSS|ReqMem
func ParseSacctEfficiency(input []byte) []SacctJobRecord {
	var records []SacctJobRecord
	for _, line := range strings.Split(string(input), "\n") {
		fields := strings.Split(line, "|")
		if len(fields) < 8 {
			continue
		}
		user := strings.TrimSpace(fields[0])
		account := strings.TrimSpace(fields[1])
		if user == "" || account == "" {
			continue
		}
		alloc, _ := strconv.ParseFloat(strings.TrimSpace(fields[2]), 64)
		elapsed := parseSacctDuration(fields[3])
		totalCPU := parseSacctDuration(fields[4])
		cpuTime := parseSacctDuration(fields[5])
		maxRSS := parseSacctMemory(fields[6])
		reqMem := parseSacctMemory(fields[7])

		if alloc == 0 || elapsed == 0 {
			continue // skip jobs with no resource usage
		}

		records = append(records, SacctJobRecord{
			User:            user,
			Account:         account,
			AllocCPUs:       alloc,
			ElapsedSeconds:  elapsed,
			TotalCPUSeconds: totalCPU,
			CPUTimeSeconds:  cpuTime,
			MaxRSSMB:        maxRSS,
			ReqMemMB:        reqMem,
		})
	}
	return records
}

// SacctEfficiencyAggregates holds aggregated efficiency stats per user+account.
type SacctEfficiencyAggregates struct {
	JobCount          float64
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
		}
		if r.ReqMemMB > 0 && r.MaxRSSMB >= 0 {
			agg.MemEfficiencyPct += r.MaxRSSMB / r.ReqMemMB * 100
		}
	}

	// Convert sums to averages
	for _, users := range result {
		for _, agg := range users {
			if agg.JobCount > 0 {
				agg.CPUEfficiencyPct /= agg.JobCount
				agg.MemEfficiencyPct /= agg.JobCount
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

	logger *logger.Logger
}

// NewSacctEfficiencyCollector creates the collector and starts the background refresh goroutine.
func NewSacctEfficiencyCollector(log *logger.Logger, interval, lookback time.Duration) *SacctEfficiencyCollector {
	labels := []string{"account", "user"}
	c := &SacctEfficiencyCollector{
		interval: interval,
		lookback: lookback,
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
func (c *SacctEfficiencyCollector) Start(ctx context.Context) {
	go func() {
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

func (c *SacctEfficiencyCollector) refresh() {
	startTime := time.Now().Add(-c.lookback).Format("2006-01-02T15:04:05")
	data, err := Execute(c.logger, "sacct", []string{
		"-X", "-P", "-n",
		"--starttime", startTime,
		"--format", "User,Account,AllocCPUS,Elapsed,TotalCPU,CPUTime,MaxRSS,ReqMem",
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
				prometheus.MustNewConstMetric(c.memEfficiency, prometheus.GaugeValue, agg.MemEfficiencyPct, account, user),
				prometheus.MustNewConstMetric(c.jobsCompleted, prometheus.GaugeValue, agg.JobCount, account, user),
				prometheus.MustNewConstMetric(c.cpuHoursAllocated, prometheus.GaugeValue, agg.CPUHoursAllocated, account, user),
			)
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
