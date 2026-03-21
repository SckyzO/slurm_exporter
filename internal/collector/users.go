package collector

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// Pre-compiled regexes for job state matching in the users collector.
var (
	userJobPending   = regexp.MustCompile(`^pending`)
	userJobRunning   = regexp.MustCompile(`^running`)
	userJobSuspended = regexp.MustCompile(`^suspended`)
)

/*
UsersData executes the squeue command to retrieve job information by user.
Expected squeue output format: "%A|%u|%T|%C" (Job ID|User|State|CPUs).
*/
func UsersData(logger *logger.Logger) ([]byte, error) {
	return Execute(logger, "squeue", []string{"-a", "-r", "-h", "-o", "%A|%u|%T|%C"})
}

type UserJobMetrics struct {
	pending     float64
	running     float64
	runningCpus float64
	suspended   float64
}

/*
ParseUsersMetrics parses the output of the squeue command for user-specific job metrics.
It expects input in the format: "JobID|User|State|CPUs".
*/
// ParseUsersMetrics parses raw squeue output into a map of user -> job metrics.
func ParseUsersMetrics(input []byte) map[string]*UserJobMetrics {
	users := make(map[string]*UserJobMetrics)
	lines := strings.Split(string(input), "\n")
	for _, line := range lines {
		if strings.Contains(line, "|") {
			fields := strings.Split(line, "|")
			if len(fields) < 4 {
				continue
			}
			user := fields[1]
			_, key := users[user]
			if !key {
				users[user] = &UserJobMetrics{}
			}
			state := fields[2]
			state = strings.ToLower(state)
			cpus, _ := strconv.ParseFloat(fields[3], 64)
			switch {
			case userJobPending.MatchString(state):
				users[user].pending++
			case userJobRunning.MatchString(state):
				users[user].running++
				users[user].runningCpus += cpus
			case userJobSuspended.MatchString(state):
				users[user].suspended++
			}
		}
	}
	return users
}

// UsersGetMetrics fetches and parses user job metrics.
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
	suspended   *prometheus.Desc
	logger      *logger.Logger
}

func NewUsersCollector(logger *logger.Logger) *UsersCollector {
	labels := []string{"user"}
	return &UsersCollector{
		pending:     prometheus.NewDesc("slurm_user_jobs_pending", "Pending jobs for user", labels, nil),
		running:     prometheus.NewDesc("slurm_user_jobs_running", "Running jobs for user", labels, nil),
		runningCpus: prometheus.NewDesc("slurm_user_cpus_running", "Running cpus for user", labels, nil),
		suspended:   prometheus.NewDesc("slurm_user_jobs_suspended", "Suspended jobs for user", labels, nil),
		logger:      logger,
	}
}

func (uc *UsersCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- uc.pending
	ch <- uc.running
	ch <- uc.runningCpus
	ch <- uc.suspended
}

func (uc *UsersCollector) Collect(ch chan<- prometheus.Metric) {
	um, err := UsersGetMetrics(uc.logger)
	if err != nil {
		uc.logger.Error("Failed to parse users metrics", "err", err)
		return
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
		if um[u].suspended > 0 {
			ch <- prometheus.MustNewConstMetric(uc.suspended, prometheus.GaugeValue, um[u].suspended, u)
		}
	}
}
