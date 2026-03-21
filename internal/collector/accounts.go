package collector

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// Pre-compiled regexes for job state matching in the accounts collector.
var (
	accountJobPending   = regexp.MustCompile(`^pending`)
	accountJobRunning   = regexp.MustCompile(`^running`)
	accountJobSuspended = regexp.MustCompile(`^suspended`)
)

/*
AccountsData executes the squeue command to retrieve job information by account.
Expected squeue output format: "%A|%a|%T|%C" (Job ID|Account|State|CPUs).
*/
func AccountsData(logger *logger.Logger) ([]byte, error) {
	return Execute(logger, "squeue", []string{"-a", "-r", "-h", "-o", "%A|%a|%T|%C"})
}

type JobMetrics struct {
	pending     float64
	running     float64
	runningCpus float64
	suspended   float64
}

/*
ParseAccountsMetrics parses the output of the squeue command for account-specific job metrics.
It expects input in the format: "JobID|Account|State|CPUs".
*/
func ParseAccountsMetrics(input []byte) map[string]*JobMetrics {
	accounts := make(map[string]*JobMetrics)
	lines := strings.Split(string(input), "\n")
	for _, line := range lines {
		if strings.Contains(line, "|") {
			fields := strings.Split(line, "|")
			if len(fields) < 4 {
				continue
			}
			account := fields[1]
			_, key := accounts[account]
			if !key {
				accounts[account] = &JobMetrics{0, 0, 0, 0}
			}
			state := fields[2]
			state = strings.ToLower(state)
			cpus, _ := strconv.ParseFloat(fields[3], 64)
			switch {
			case accountJobPending.MatchString(state):
				accounts[account].pending++
			case accountJobRunning.MatchString(state):
				accounts[account].running++
				accounts[account].runningCpus += cpus
			case accountJobSuspended.MatchString(state):
				accounts[account].suspended++
			}
		}
	}
	return accounts
}

type AccountsCollector struct {
	pending     *prometheus.Desc
	running     *prometheus.Desc
	runningCpus *prometheus.Desc
	suspended   *prometheus.Desc
	logger      *logger.Logger
}

func NewAccountsCollector(logger *logger.Logger) *AccountsCollector {
	labels := []string{"account"}
	return &AccountsCollector{
		pending:     prometheus.NewDesc("slurm_account_jobs_pending", "Pending jobs for account", labels, nil),
		running:     prometheus.NewDesc("slurm_account_jobs_running", "Running jobs for account", labels, nil),
		runningCpus: prometheus.NewDesc("slurm_account_cpus_running", "Running cpus for account", labels, nil),
		suspended:   prometheus.NewDesc("slurm_account_jobs_suspended", "Suspended jobs for account", labels, nil),
		logger:      logger,
	}
}

func (ac *AccountsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- ac.pending
	ch <- ac.running
	ch <- ac.runningCpus
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
		if am[a].suspended > 0 {
			ch <- prometheus.MustNewConstMetric(ac.suspended, prometheus.GaugeValue, am[a].suspended, a)
		}
	}
}
