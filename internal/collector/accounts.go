package collector

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

var (
	accountJobPending   = regexp.MustCompile(`^pending`)
	accountJobRunning   = regexp.MustCompile(`^running`)
	accountJobSuspended = regexp.MustCompile(`^suspended`)

	// [:/]gpu accepts both "gres/gpu" and "gres:gpu" prefixes — some Slurm
	// versions emit the colon form. [:/=] accepts ":" for legacy %b output
	// and "=" for tres-alloc output. Typed variants ("gres/gpu:a100") work.
	tresGPURe = regexp.MustCompile(`gres[:/]gpu[^,\s]*[:/=](\d+)`)
)

// parseGPUsFromTRES returns 0 when the field is "N/A" or has no GPU entry.
func parseGPUsFromTRES(tres string) float64 {
	matches := tresGPURe.FindStringSubmatch(tres)
	if len(matches) < 2 {
		return 0
	}
	count, _ := strconv.ParseFloat(matches[1], 64)
	return count
}

// AccountsData runs squeue grouped by Slurm account.
//
// The trailing colon on `tres-alloc:` is load-bearing: without it, squeue
// truncates the last field to its 20-char default and the GPU suffix is lost.
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

// ParseAccountsMetrics parses "JobID|Account|State|NumNodes|NumCPUs|tres-alloc".
// TrimSpace is required: squeue -O pads each column to a minimum width.
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
