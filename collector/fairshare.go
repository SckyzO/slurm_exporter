package collector

import (
	"strconv"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
)

/*
FairShareData executes the sshare command to retrieve fairshare information.
Expected sshare output format: "account,fairshare".
*/
func FairShareData(logger log.Logger) ([]byte, error) {
	return Execute(logger, "sshare", []string{"-n", "-P", "-o", "account,fairshare"})
}

type FairShareMetrics struct {
	fairshare float64
}

/*
ParseFairShareMetrics parses the output of the sshare command for fairshare metrics.
It expects input in the format: "account|fairshare".
*/
func ParseFairShareMetrics(logger log.Logger) (map[string]*FairShareMetrics, error) {
	accounts := make(map[string]*FairShareMetrics)
	fairShareData, err := FairShareData(logger)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(fairShareData), "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, "  ") {
			if strings.Contains(line, "|") {
				account := strings.Trim(strings.Split(line, "|")[0], " ")
				_, key := accounts[account]
				if !key {
					accounts[account] = &FairShareMetrics{0}
				}
				fairshare, _ := strconv.ParseFloat(strings.Split(line, "|")[1], 64)
				accounts[account].fairshare = fairshare
			}
		}
	}
	return accounts, nil
}

type FairShareCollector struct {
	fairshare *prometheus.Desc
	logger    log.Logger
}

func NewFairShareCollector(logger log.Logger) *FairShareCollector {
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
	fsm, err := ParseFairShareMetrics(fsc.logger)
	if err != nil {
		_ = level.Error(fsc.logger).Log("msg", "Failed to parse fairshare metrics", "err", err)
		return
	}
	for f := range fsm {
		ch <- prometheus.MustNewConstMetric(fsc.fairshare, prometheus.GaugeValue, fsm[f].fairshare, f)
	}
}