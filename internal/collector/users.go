package collector

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/mike8161990/slurm_exporter/internal/logger"
	"github.com/prometheus/client_golang/prometheus"
)

/*
UsersData executes the squeue command to retrieve job information by user.
Expected squeue output format: "%A|%u|%T|%C" (Job ID|User|State|CPUs).
*/
func UsersData(logger *logger.Logger) ([]byte, error) {
	return Execute(logger, "squeue", []string{"-a", "-r", "-h", "-o", "%A|%u|%T|%C"})
}

type UserJobMetrics struct {
	pending      float64
	running      float64
	running_cpus float64
	suspended    float64
}

/*
ParseUsersMetrics parses the output of the squeue command for user-specific job metrics.
It expects input in the format: "JobID|User|State|CPUs".
*/
func ParseUsersMetrics(logger *logger.Logger) (map[string]*UserJobMetrics, error) {
	users := make(map[string]*UserJobMetrics)
	usersData, err := UsersData(logger)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(usersData), "\n")
	for _, line := range lines {
		if strings.Contains(line, "|") {
			user := strings.Split(line, "|")[1]
			_, key := users[user]
			if !key {
				users[user] = &UserJobMetrics{0, 0, 0, 0}
			}
			state := strings.Split(line, "|")[2]
			state = strings.ToLower(state)
			cpus, _ := strconv.ParseFloat(strings.Split(line, "|")[3], 64)
			pending := regexp.MustCompile(`^pending`)
			running := regexp.MustCompile(`^running`)
			suspended := regexp.MustCompile(`^suspended`)
			switch {
			case pending.MatchString(state):
				users[user].pending++
			case running.MatchString(state):
				users[user].running++
				users[user].running_cpus += cpus
			case suspended.MatchString(state):
				users[user].suspended++
			}
		}
	}
	return users, nil
}

type UsersCollector struct {
	pending      *prometheus.Desc
	running      *prometheus.Desc
	running_cpus *prometheus.Desc
	suspended    *prometheus.Desc
	logger       *logger.Logger
}

func NewUsersCollector(logger *logger.Logger) *UsersCollector {
	labels := []string{"user"}
	return &UsersCollector{
		pending:      prometheus.NewDesc("slurm_user_jobs_pending", "Pending jobs for user", labels, nil),
		running:      prometheus.NewDesc("slurm_user_jobs_running", "Running jobs for user", labels, nil),
		running_cpus: prometheus.NewDesc("slurm_user_cpus_running", "Running cpus for user", labels, nil),
		suspended:    prometheus.NewDesc("slurm_user_jobs_suspended", "Suspended jobs for user", labels, nil),
		logger:       logger,
	}
}

func (uc *UsersCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- uc.pending
	ch <- uc.running
	ch <- uc.running_cpus
	ch <- uc.suspended
}

func (uc *UsersCollector) Collect(ch chan<- prometheus.Metric) {
	um, err := ParseUsersMetrics(uc.logger)
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
		if um[u].running_cpus > 0 {
			ch <- prometheus.MustNewConstMetric(uc.running_cpus, prometheus.GaugeValue, um[u].running_cpus, u)
		}
		if um[u].suspended > 0 {
			ch <- prometheus.MustNewConstMetric(uc.suspended, prometheus.GaugeValue, um[u].suspended, u)
		}
	}
}
