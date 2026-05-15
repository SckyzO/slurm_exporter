package collector

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// Pre-compiled regexes for job state and TRES GPU parsing.
var (
	accountJobPending   = regexp.MustCompile(`^pending`)
	accountJobRunning   = regexp.MustCompile(`^running`)
	accountJobSuspended = regexp.MustCompile(`^suspended`)

	// tresGPURe matches GPU counts in TRES allocation strings from squeue
	// tres-alloc output (e.g. "cpu=4,mem=32G,node=2,gres/gpu=8").
	// Real prefixes observed across Slurm versions: "gres/gpu", "gres:gpu".
	// Typed GPU variants: "gres/gpu:a100", "gres:gpu:nvidia_gb200".
	// The [:/]gpu prefix tolerates both separators (see issue #28).
	tresGPURe = regexp.MustCompile(`gres[:/]gpu[^,\s]*[:/=](\d+)`)
)

// parseGPUsFromTRES extracts the total GPU count from a TRES allocation string.
// Returns 0 when the field is "N/A" or contains no GPU entry.
func parseGPUsFromTRES(tres string) float64 {
	matches := tresGPURe.FindStringSubmatch(tres)
	if len(matches) < 2 {
		return 0
	}
	count, _ := strconv.ParseFloat(matches[1], 64)
	return count
}

// AccountsData runs squeue to retrieve job/CPU/GPU counts grouped by Slurm account.
// Uses tres-alloc (effective allocation) instead of the legacy %b (TRES_per_node),
// which returned "N/A" for jobs submitted with --gpus or --gpus-per-node and silently
// dropped them from slurm_account_gpus_running (see issue #35).
//
// The trailing colon on `tres-alloc:` forces variable column width. Without it,
// squeue truncates the last field to its default minimum (20 chars) — the same
// class of bug as issue #10 on sinfo --Format. Confirmed against Slurm 25.11.2.
//
// Output format: "JobID|Account|State|NumNodes|NumCPUs|tres-alloc".
func AccountsData(logger *logger.Logger) ([]byte, error) {
	return Execute(logger, "squeue", []string{
		"-a", "-r", "-h",
		"-O", "JobID:|,Account:|,State:|,NumNodes:|,NumCPUs:|,tres-alloc:",
	})
}

type JobMetrics struct {
	pending     float64
	running     float64
	runningCpus float64
	runningGPUs float64
	suspended   float64
}

// ParseAccountsMetrics parses squeue output into a map of account -> job metrics.
// Input format: "JobID|Account|State|NumNodes|NumCPUs|tres-alloc".
// TrimSpace is applied to every field because squeue -O pads each column to a
// minimum width and the trailing tres-alloc field carries no suffix separator.
func ParseAccountsMetrics(input []byte) map[string]*JobMetrics {
	accounts := make(map[string]*JobMetrics)
	for line := range strings.SplitSeq(string(input), "\n") {
		if !strings.Contains(line, "|") {
			continue
		}
		fields := strings.SplitN(line, "|", 6)
		if len(fields) < 6 {
			continue
		}
		account := strings.TrimSpace(fields[1])
		if _, exists := accounts[account]; !exists {
			accounts[account] = &JobMetrics{}
		}
		state := strings.ToLower(strings.TrimSpace(fields[2]))
		cpus, _ := strconv.ParseFloat(strings.TrimSpace(fields[4]), 64)
		switch {
		case accountJobPending.MatchString(state):
			accounts[account].pending++
		case accountJobRunning.MatchString(state):
			accounts[account].running++
			accounts[account].runningCpus += cpus
			accounts[account].runningGPUs += parseGPUsFromTRES(fields[5])
		case accountJobSuspended.MatchString(state):
			accounts[account].suspended++
		}
	}
	return accounts
}

type AccountsCollector struct {
	pending     *prometheus.Desc
	running     *prometheus.Desc
	runningCpus *prometheus.Desc
	runningGPUs *prometheus.Desc
	suspended   *prometheus.Desc
	logger      *logger.Logger
}

// NewAccountsCollector creates a collector for per-account job metrics.
func NewAccountsCollector(logger *logger.Logger) *AccountsCollector {
	labels := []string{"account"}
	return &AccountsCollector{
		pending:     prometheus.NewDesc("slurm_account_jobs_pending", "Pending jobs for account", labels, nil),
		running:     prometheus.NewDesc("slurm_account_jobs_running", "Running jobs for account", labels, nil),
		runningCpus: prometheus.NewDesc("slurm_account_cpus_running", "Running CPUs for account", labels, nil),
		runningGPUs: prometheus.NewDesc("slurm_account_gpus_running", "Running GPUs for account", labels, nil),
		suspended:   prometheus.NewDesc("slurm_account_jobs_suspended", "Suspended jobs for account", labels, nil),
		logger:      logger,
	}
}

func (ac *AccountsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- ac.pending
	ch <- ac.running
	ch <- ac.runningCpus
	ch <- ac.runningGPUs
	ch <- ac.suspended
}

func (ac *AccountsCollector) Collect(ch chan<- prometheus.Metric) {
	data, err := AccountsData(ac.logger)
	if err != nil {
		ac.logger.Error("Failed to get accounts data", "err", err)
		return
	}
	am := ParseAccountsMetrics(data)
	for a := range am {
		if am[a].pending > 0 {
			ch <- prometheus.MustNewConstMetric(ac.pending, prometheus.GaugeValue, am[a].pending, a)
		}
		if am[a].running > 0 {
			ch <- prometheus.MustNewConstMetric(ac.running, prometheus.GaugeValue, am[a].running, a)
		}
		if am[a].runningCpus > 0 {
			ch <- prometheus.MustNewConstMetric(ac.runningCpus, prometheus.GaugeValue, am[a].runningCpus, a)
		}
		if am[a].runningGPUs > 0 {
			ch <- prometheus.MustNewConstMetric(ac.runningGPUs, prometheus.GaugeValue, am[a].runningGPUs, a)
		}
		if am[a].suspended > 0 {
			ch <- prometheus.MustNewConstMetric(ac.suspended, prometheus.GaugeValue, am[a].suspended, a)
		}
	}
}
