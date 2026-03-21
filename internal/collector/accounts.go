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

	// treGPURe matches GPU counts in TRES strings from squeue %b output.
	// Real formats observed: "gres/gpu:4", "gres/gpu:2", "N/A"
	// Also handles typed GPUs: "gres/gpu:a100:2", "gres/gpu:nvidia_gb200:4"
	tresGPURe = regexp.MustCompile(`gres/gpu[^,\s]*[:/](\d+)`)
)

// parseGPUsFromTRES extracts the GPU count per node from a TRES string.
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
// Output format: "%A|%a|%T|%D|%C|%b" (JobID|Account|State|NumNodes|CPUs|TRES).
func AccountsData(logger *logger.Logger) ([]byte, error) {
	return Execute(logger, "squeue", []string{"-a", "-r", "-h", "-o", "%A|%a|%T|%D|%C|%b"})
}

type JobMetrics struct {
	pending     float64
	running     float64
	runningCpus float64
	runningGPUs float64
	suspended   float64
}

// ParseAccountsMetrics parses squeue output into a map of account -> job metrics.
// Input format: "%A|%a|%T|%D|%C|%b" (JobID|Account|State|NumNodes|CPUs|TRES).
func ParseAccountsMetrics(input []byte) map[string]*JobMetrics {
	accounts := make(map[string]*JobMetrics)
	lines := strings.Split(string(input), "\n")
	for _, line := range lines {
		if !strings.Contains(line, "|") {
			continue
		}
		fields := strings.SplitN(line, "|", 6)
		if len(fields) < 6 {
			continue
		}
		account := fields[1]
		if _, exists := accounts[account]; !exists {
			accounts[account] = &JobMetrics{}
		}
		state := strings.ToLower(fields[2])
		numNodes, _ := strconv.ParseFloat(fields[3], 64)
		cpus, _ := strconv.ParseFloat(fields[4], 64)
		switch {
		case accountJobPending.MatchString(state):
			accounts[account].pending++
		case accountJobRunning.MatchString(state):
			accounts[account].running++
			accounts[account].runningCpus += cpus
			// TRES field shows GPUs per node — multiply by node count for total.
			gpusPerNode := parseGPUsFromTRES(fields[5])
			accounts[account].runningGPUs += gpusPerNode * numNodes
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
