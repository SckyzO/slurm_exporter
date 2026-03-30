package collector

import (
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// FairShareData executes the sshare command to retrieve fairshare information.
// Output format: Account|User|RawShares|NormShares|RawUsage|NormUsage|FairShare
// RawUsage is expressed in CPU-seconds (raw scheduler usage units).
func FairShareData(log *logger.Logger) ([]byte, error) {
	return Execute(log, "sshare", []string{"-a", "-P", "-n", "-o", "Account,User,RawShares,NormShares,RawUsage,NormUsage,FairShare"})
}

// FairShareMetrics holds parsed fairshare data for a single account or user line.
// RawUsage is in CPU-seconds.
type FairShareMetrics struct {
	Account    string
	User       string
	RawShares  float64
	NormShares float64
	RawUsage   float64 // CPU-seconds
	NormUsage  float64
	FairShare  float64
}

// ParseFairShareMetrics parses raw sshare -a output into a slice of FairShareMetrics.
// Lines with "parent" RawShares are skipped (they inherit from parent and carry no
// independent share information). Lines with an empty Account field are also skipped.
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

		account := strings.TrimSpace(fields[0])
		if account == "" {
			continue
		}

		m := FairShareMetrics{
			Account: account,
			User:    strings.TrimSpace(fields[1]),
		}

		// RawShares can be a number or "parent" — skip parent lines entirely.
		rawSharesStr := strings.TrimSpace(fields[2])
		if rawSharesStr == "parent" {
			continue
		}
		m.RawShares, _ = strconv.ParseFloat(rawSharesStr, 64)

		m.NormShares, _ = strconv.ParseFloat(strings.TrimSpace(fields[3]), 64)
		m.RawUsage, _ = strconv.ParseFloat(strings.TrimSpace(fields[4]), 64)
		m.NormUsage, _ = strconv.ParseFloat(strings.TrimSpace(fields[5]), 64)

		if fs := strings.TrimSpace(fields[6]); fs != "" {
			m.FairShare, _ = strconv.ParseFloat(fs, 64)
		}

		metrics = append(metrics, m)
	}
	return metrics
}

// FairShareGetMetrics fetches and parses fairshare metrics.
func FairShareGetMetrics(log *logger.Logger) ([]FairShareMetrics, error) {
	data, err := FairShareData(log)
	if err != nil {
		return nil, err
	}
	return ParseFairShareMetrics(data), nil
}

// FairShareCollector collects per-account and optionally per-user fairshare metrics.
type FairShareCollector struct {
	// Account-level metrics (always collected)
	accountFairShare  *prometheus.Desc
	accountRawShares  *prometheus.Desc
	accountNormShares *prometheus.Desc
	// RawUsage in CPU-seconds — use slurm_account_fairshare_raw_usage_cpu_seconds in PromQL
	accountRawUsage  *prometheus.Desc
	accountNormUsage *prometheus.Desc

	// User-level metrics (can be disabled via --no-collector.fairshare.user-metrics
	// on large clusters to limit cardinality: N_users × 5 time series)
	userFairShare  *prometheus.Desc
	userRawShares  *prometheus.Desc
	userNormShares *prometheus.Desc
	userRawUsage   *prometheus.Desc
	userNormUsage  *prometheus.Desc

	userMetrics bool
	logger      *logger.Logger
}

// NewFairShareCollector creates a new FairShareCollector.
// Set userMetrics=false to disable per-user metrics on clusters with many users.
func NewFairShareCollector(log *logger.Logger, userMetrics bool) *FairShareCollector {
	accountLabels := []string{"account"}
	userLabels := []string{"account", "user"}

	return &FairShareCollector{
		accountFairShare:  prometheus.NewDesc("slurm_account_fairshare", "FairShare factor for account (0=lowest priority, 1=highest)", accountLabels, nil),
		accountRawShares:  prometheus.NewDesc("slurm_account_fairshare_raw_shares", "Raw shares allocated to account", accountLabels, nil),
		accountNormShares: prometheus.NewDesc("slurm_account_fairshare_norm_shares", "Normalized shares for account (fraction of total shares)", accountLabels, nil),
		accountRawUsage:   prometheus.NewDesc("slurm_account_fairshare_raw_usage_cpu_seconds", "Raw CPU-seconds usage for account (decay-weighted)", accountLabels, nil),
		accountNormUsage:  prometheus.NewDesc("slurm_account_fairshare_norm_usage", "Normalized usage for account (fraction of total usage)", accountLabels, nil),

		userFairShare:  prometheus.NewDesc("slurm_user_fairshare", "FairShare factor for user", userLabels, nil),
		userRawShares:  prometheus.NewDesc("slurm_user_fairshare_raw_shares", "Raw shares for user", userLabels, nil),
		userNormShares: prometheus.NewDesc("slurm_user_fairshare_norm_shares", "Normalized shares for user", userLabels, nil),
		userRawUsage:   prometheus.NewDesc("slurm_user_fairshare_raw_usage_cpu_seconds", "Raw CPU-seconds usage for user (decay-weighted)", userLabels, nil),
		userNormUsage:  prometheus.NewDesc("slurm_user_fairshare_norm_usage", "Normalized usage for user", userLabels, nil),

		userMetrics: userMetrics,
		logger:      log,
	}
}

func (fsc *FairShareCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- fsc.accountFairShare
	ch <- fsc.accountRawShares
	ch <- fsc.accountNormShares
	ch <- fsc.accountRawUsage
	ch <- fsc.accountNormUsage
	if fsc.userMetrics {
		ch <- fsc.userFairShare
		ch <- fsc.userRawShares
		ch <- fsc.userNormShares
		ch <- fsc.userRawUsage
		ch <- fsc.userNormUsage
	}
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
			// Account-level line — deduplicate by account name.
			if seenAccounts[m.Account] {
				continue
			}
			seenAccounts[m.Account] = true
			ch <- prometheus.MustNewConstMetric(fsc.accountFairShare, prometheus.GaugeValue, m.FairShare, m.Account)
			ch <- prometheus.MustNewConstMetric(fsc.accountRawShares, prometheus.GaugeValue, m.RawShares, m.Account)
			ch <- prometheus.MustNewConstMetric(fsc.accountNormShares, prometheus.GaugeValue, m.NormShares, m.Account)
			ch <- prometheus.MustNewConstMetric(fsc.accountRawUsage, prometheus.GaugeValue, m.RawUsage, m.Account)
			ch <- prometheus.MustNewConstMetric(fsc.accountNormUsage, prometheus.GaugeValue, m.NormUsage, m.Account)
		} else if fsc.userMetrics {
			// User-level line — deduplicate by account+user composite key.
			key := m.Account + "|" + m.User
			if seenUsers[key] {
				continue
			}
			seenUsers[key] = true
			ch <- prometheus.MustNewConstMetric(fsc.userFairShare, prometheus.GaugeValue, m.FairShare, m.Account, m.User)
			ch <- prometheus.MustNewConstMetric(fsc.userRawShares, prometheus.GaugeValue, m.RawShares, m.Account, m.User)
			ch <- prometheus.MustNewConstMetric(fsc.userNormShares, prometheus.GaugeValue, m.NormShares, m.Account, m.User)
			ch <- prometheus.MustNewConstMetric(fsc.userRawUsage, prometheus.GaugeValue, m.RawUsage, m.Account, m.User)
			ch <- prometheus.MustNewConstMetric(fsc.userNormUsage, prometheus.GaugeValue, m.NormUsage, m.Account, m.User)
		}
	}
}
