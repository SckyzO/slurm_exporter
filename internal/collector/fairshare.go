package collector

import (
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// FairShareData executes the sshare command to retrieve fairshare information.
// Output format: "account|fairshare" (pipe-separated, -P flag).
func FairShareData(logger *logger.Logger) ([]byte, error) {
	return Execute(logger, "sshare", []string{"-n", "-P", "-o", "account,fairshare"})
}

type FairShareMetrics struct {
	fairshare float64
}

// ParseFairShareMetrics parses raw sshare output into a map of account -> fairshare.
// Lines indented with two spaces are sub-account entries and are skipped.
func ParseFairShareMetrics(input []byte) map[string]*FairShareMetrics {
	accounts := make(map[string]*FairShareMetrics)
	for line := range strings.SplitSeq(string(input), "\n") {
		if strings.HasPrefix(line, "  ") || !strings.Contains(line, "|") {
			continue
		}
		fields := strings.Split(line, "|")
		if len(fields) < 2 {
			continue
		}
		account := strings.TrimSpace(fields[0])
		if _, exists := accounts[account]; !exists {
			accounts[account] = &FairShareMetrics{}
		}
		accounts[account].fairshare, _ = strconv.ParseFloat(fields[1], 64)
	}
	return accounts
}

// FairShareGetMetrics fetches and parses fairshare metrics.
func FairShareGetMetrics(logger *logger.Logger) (map[string]*FairShareMetrics, error) {
	data, err := FairShareData(logger)
	if err != nil {
		return nil, err
	}
	return ParseFairShareMetrics(data), nil
}

type FairShareCollector struct {
	fairshare *prometheus.Desc
	logger    *logger.Logger
}

func NewFairShareCollector(logger *logger.Logger) *FairShareCollector {
	labels := []string{"account"}
	return &FairShareCollector{
		fairshare: prometheus.NewDesc("slurm_account_fairshare", "FairShare for account", labels, nil),
		logger:    logger,
	}
}

func (fsc *FairShareCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- fsc.fairshare
}

func (fsc *FairShareCollector) Collect(ch chan<- prometheus.Metric) {
	fsm, err := FairShareGetMetrics(fsc.logger)
	if err != nil {
		fsc.logger.Error("Failed to get fairshare metrics", "err", err)
		return
	}
	for f := range fsm {
		ch <- prometheus.MustNewConstMetric(fsc.fairshare, prometheus.GaugeValue, fsm[f].fairshare, f)
	}
}
