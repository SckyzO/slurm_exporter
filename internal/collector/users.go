package collector

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

var (
	userJobPending   = regexp.MustCompile(`^pending`)
	userJobRunning   = regexp.MustCompile(`^running`)
	userJobSuspended = regexp.MustCompile(`^suspended`)
)

// UsersData runs squeue grouped by user. The trailing colon on `tres-alloc:`
// is required for the same reason as in AccountsData — see that function.
func UsersData(logger *logger.Logger) ([]byte, error) {
	return Execute(logger, "squeue", []string{
		"-a", "-r", "-h",
		"-O", "JobID:|,UserName:|,State:|,NumNodes:|,NumCPUs:|,tres-alloc:",
	})
}

type UserJobMetrics struct {
	pending     float64
	running     float64
	runningCpus float64
	runningGPUs float64
	suspended   float64
}

// ParseUsersMetrics parses "JobID|User|State|NumNodes|NumCPUs|tres-alloc".
// TrimSpace strips the padding squeue -O adds to every column.
func ParseUsersMetrics(input []byte) map[string]*UserJobMetrics {
	users := make(map[string]*UserJobMetrics)
	for line := range strings.SplitSeq(string(input), "\n") {
		if !strings.Contains(line, "|") {
			continue
		}
		fields := strings.SplitN(line, "|", 6)
		if len(fields) < 6 {
			continue
		}
		user := strings.TrimSpace(fields[1])
		if _, exists := users[user]; !exists {
			users[user] = &UserJobMetrics{}
		}
		state := strings.ToLower(strings.TrimSpace(fields[2]))
		cpus, _ := strconv.ParseFloat(strings.TrimSpace(fields[4]), 64)
		switch {
		case userJobPending.MatchString(state):
			users[user].pending++
		case userJobRunning.MatchString(state):
			users[user].running++
			users[user].runningCpus += cpus
			users[user].runningGPUs += parseGPUsFromTRES(fields[5])
		case userJobSuspended.MatchString(state):
			users[user].suspended++
		}
	}
	return users
}

func UsersGetMetrics(logger *logger.Logger) (map[string]*UserJobMetrics, error) {
	data, err := UsersData(logger)
	if err != nil {
		return nil, err
	}
	return ParseUsersMetrics(data), nil
}

type UsersCollector struct {
	pending     *prometheus.Desc
	running     *prometheus.Desc
	runningCpus *prometheus.Desc
	runningGPUs *prometheus.Desc
	suspended   *prometheus.Desc
	logger      *logger.Logger
}

func NewUsersCollector(logger *logger.Logger) *UsersCollector {
	labels := []string{"user"}
	return &UsersCollector{
		pending:     prometheus.NewDesc("slurm_user_jobs_pending", "Pending jobs for user", labels, nil),
		running:     prometheus.NewDesc("slurm_user_jobs_running", "Running jobs for user", labels, nil),
		runningCpus: prometheus.NewDesc("slurm_user_cpus_running", "Running CPUs for user", labels, nil),
		runningGPUs: prometheus.NewDesc("slurm_user_gpus_running", "Running GPUs for user", labels, nil),
		suspended:   prometheus.NewDesc("slurm_user_jobs_suspended", "Suspended jobs for user", labels, nil),
		logger:      logger,
	}
}

func (uc *UsersCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- uc.pending
	ch <- uc.running
	ch <- uc.runningCpus
	ch <- uc.runningGPUs
	ch <- uc.suspended
}

func (uc *UsersCollector) Collect(ch chan<- prometheus.Metric) { _ = uc.tryCollect(ch) }

func (uc *UsersCollector) tryCollect(ch chan<- prometheus.Metric) error {
	um, err := UsersGetMetrics(uc.logger)
	if err != nil {
		uc.logger.Error("Failed to parse users metrics", "err", err)
		return err
	}
	for u := range um {
		if um[u].pending > 0 {
			ch <- prometheus.MustNewConstMetric(uc.pending, prometheus.GaugeValue, um[u].pending, u)
		}
		if um[u].running > 0 {
			ch <- prometheus.MustNewConstMetric(uc.running, prometheus.GaugeValue, um[u].running, u)
		}
		if um[u].runningCpus > 0 {
			ch <- prometheus.MustNewConstMetric(uc.runningCpus, prometheus.GaugeValue, um[u].runningCpus, u)
		}
		if um[u].runningGPUs > 0 {
			ch <- prometheus.MustNewConstMetric(uc.runningGPUs, prometheus.GaugeValue, um[u].runningGPUs, u)
		}
		if um[u].suspended > 0 {
			ch <- prometheus.MustNewConstMetric(uc.suspended, prometheus.GaugeValue, um[u].suspended, u)
		}
	}

	return nil
}
