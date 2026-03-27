package collector

import (
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// FairShareData executes the sshare command to retrieve fairshare information.
// New format: Account|User|RawShares|NormShares|RawUsage|NormUsage|FairShare
func FairShareData(logger *logger.Logger) ([]byte, error) {
	return Execute(logger, "sshare", []string{"-a", "-P", "-n", "-o", "Account,User,RawShares,NormShares,RawUsage,NormUsage,FairShare"})
}

type FairShareMetrics struct {
	Account    string
	User       string
	RawShares  float64
	NormShares float64
	RawUsage   float64
	NormUsage  float64
	FairShare  float64
}

// ParseFairShareMetrics parses raw sshare output into a slice of FairShareMetrics.
func ParseFairShareMetrics(input []byte) []FairShareMetrics {
	var metrics []FairShareMetrics
	for line := range strings.SplitSeq(string(input), "\n") {
		if !strings.Contains(line, "|") {
			continue
		}
		fields := strings.Split(line, "|")
		if len(fields) < 7 {
			continue
		}

		m := FairShareMetrics{
			Account: strings.TrimSpace(fields[0]),
			User:    strings.TrimSpace(fields[1]),
		}

		// RawShares can be a number or "parent"
		rawSharesStr := strings.TrimSpace(fields[2])
		if rawSharesStr == "parent" {
			continue
		}
		m.RawShares, _ = strconv.ParseFloat(rawSharesStr, 64)

		m.NormShares, _ = strconv.ParseFloat(strings.TrimSpace(fields[3]), 64)
		m.RawUsage, _ = strconv.ParseFloat(strings.TrimSpace(fields[4]), 64)
		m.NormUsage, _ = strconv.ParseFloat(strings.TrimSpace(fields[5]), 64)
		
		fairShareStr := strings.TrimSpace(fields[6])
		if fairShareStr != "" {
			m.FairShare, _ = strconv.ParseFloat(fairShareStr, 64)
		}

		metrics = append(metrics, m)
	}
	return metrics
}

// FairShareGetMetrics fetches and parses fairshare metrics.
func FairShareGetMetrics(logger *logger.Logger) ([]FairShareMetrics, error) {
	data, err := FairShareData(logger)
	if err != nil {
		return nil, err
	}
	return ParseFairShareMetrics(data), nil
}

type FairShareCollector struct {
	// Account metrics (Backward compatible)
	accountFairShare  *prometheus.Desc
	accountRawShares  *prometheus.Desc
	accountNormShares *prometheus.Desc
	accountRawUsage   *prometheus.Desc
	accountNormUsage  *prometheus.Desc

	// User metrics
	userFairShare  *prometheus.Desc
	userRawShares  *prometheus.Desc
	userNormShares *prometheus.Desc
	userRawUsage   *prometheus.Desc
	userNormUsage  *prometheus.Desc

	logger *logger.Logger
}

func NewFairShareCollector(logger *logger.Logger) *FairShareCollector {
	accountLabels := []string{"account"}
	userLabels := []string{"account", "user"}

	return &FairShareCollector{
		accountFairShare:  prometheus.NewDesc("slurm_account_fairshare", "FairShare factor for account", accountLabels, nil),
		accountRawShares:  prometheus.NewDesc("slurm_account_fairshare_raw_shares", "Raw shares for account", accountLabels, nil),
		accountNormShares: prometheus.NewDesc("slurm_account_fairshare_norm_shares", "Normalized shares for account", accountLabels, nil),
		accountRawUsage:   prometheus.NewDesc("slurm_account_fairshare_raw_usage", "Raw usage for account", accountLabels, nil),
		accountNormUsage:  prometheus.NewDesc("slurm_account_fairshare_norm_usage", "Normalized usage for account", accountLabels, nil),

		userFairShare:  prometheus.NewDesc("slurm_user_fairshare", "FairShare factor for user", userLabels, nil),
		userRawShares:  prometheus.NewDesc("slurm_user_fairshare_raw_shares", "Raw shares for user", userLabels, nil),
		userNormShares: prometheus.NewDesc("slurm_user_fairshare_norm_shares", "Normalized shares for user", userLabels, nil),
		userRawUsage:   prometheus.NewDesc("slurm_user_fairshare_raw_usage", "Raw usage for user", userLabels, nil),
		userNormUsage:  prometheus.NewDesc("slurm_user_fairshare_norm_usage", "Normalized usage for user", userLabels, nil),

		logger: logger,
	}
}

func (fsc *FairShareCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- fsc.accountFairShare
	ch <- fsc.accountRawShares
	ch <- fsc.accountNormShares
	ch <- fsc.accountRawUsage
	ch <- fsc.accountNormUsage
	ch <- fsc.userFairShare
	ch <- fsc.userRawShares
	ch <- fsc.userNormShares
	ch <- fsc.userRawUsage
	ch <- fsc.userNormUsage
}

func (fsc *FairShareCollector) Collect(ch chan<- prometheus.Metric) {
	metrics, err := FairShareGetMetrics(fsc.logger)
	if err != nil {
		fsc.logger.Error("Failed to get fairshare metrics", "err", err)
		return
	}

	seenAccounts := make(map[string]bool)
	seenUsers := make(map[string]bool)
	for _, m := range metrics {
		if m.User == "" {
			if seenAccounts[m.Account] {
				continue
			}
			seenAccounts[m.Account] = true
			// Account-level metrics
			ch <- prometheus.MustNewConstMetric(fsc.accountFairShare, prometheus.GaugeValue, m.FairShare, m.Account)
			ch <- prometheus.MustNewConstMetric(fsc.accountRawShares, prometheus.GaugeValue, m.RawShares, m.Account)
			ch <- prometheus.MustNewConstMetric(fsc.accountNormShares, prometheus.GaugeValue, m.NormShares, m.Account)
			ch <- prometheus.MustNewConstMetric(fsc.accountRawUsage, prometheus.GaugeValue, m.RawUsage, m.Account)
			ch <- prometheus.MustNewConstMetric(fsc.accountNormUsage, prometheus.GaugeValue, m.NormUsage, m.Account)
		} else {
			userKey := m.Account + "|" + m.User
			if seenUsers[userKey] {
				continue
			}
			seenUsers[userKey] = true
			// User-level metrics
			ch <- prometheus.MustNewConstMetric(fsc.userFairShare, prometheus.GaugeValue, m.FairShare, m.Account, m.User)
			ch <- prometheus.MustNewConstMetric(fsc.userRawShares, prometheus.GaugeValue, m.RawShares, m.Account, m.User)
			ch <- prometheus.MustNewConstMetric(fsc.userNormShares, prometheus.GaugeValue, m.NormShares, m.Account, m.User)
			ch <- prometheus.MustNewConstMetric(fsc.userRawUsage, prometheus.GaugeValue, m.RawUsage, m.Account, m.User)
			ch <- prometheus.MustNewConstMetric(fsc.userNormUsage, prometheus.GaugeValue, m.NormUsage, m.Account, m.User)
		}
	}
}
