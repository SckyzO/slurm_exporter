package collector

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/sckyzo/slurm_exporter/internal/logger"
	"github.com/prometheus/client_golang/prometheus"
)

type LicenseMetrics struct {
	total map[string]float64
	used  map[string]float64
	free  map[string]float64
}

func LicenseGetMetrics(logger *logger.Logger) (*LicenseMetrics, error) {
	data, err := LicenseData(logger)
	if err != nil {
		return nil, err
	}
	return ParseLicenseMetrics(data), nil
}

/*
ParseLicenseMetrics parses the output of the scontrol command for license metrics.
Expected scontrol output format: one line per license.
*/
func ParseLicenseMetrics(input []byte) *LicenseMetrics {
	var lm LicenseMetrics
	lm.total = make(map[string]float64)
	lm.used = make(map[string]float64)
	lm.free = make(map[string]float64)

	lineExp := regexp.MustCompile(`LicenseName=(\S+) Total=(\d+) Used=(\d+) Free=(\d+)`)

	lines := strings.Split(string(input), "\n")
	for _, line := range lines {
		matches := lineExp.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		name := matches[1]
		total, _ := strconv.ParseFloat(matches[2], 64)
		used, _ := strconv.ParseFloat(matches[3], 64)
		free, _ := strconv.ParseFloat(matches[4], 64)

		lm.total[name] = total
		lm.used[name] = used
		lm.free[name] = free
	}
	return &lm
}

/*
LicenseData executes the scontrol command to retrieve license information.
Expected scontrol output format: one line per license.
*/
func LicenseData(logger *logger.Logger) ([]byte, error) {
	return Execute(logger, "scontrol", []string{"show", "licenses", "-o"})
}

/*
 * Implement the Prometheus Collector interface and feed the
 * Slurm scheduler metrics into it.
 * https://godoc.org/github.com/prometheus/client_golang/prometheus#Collector
 */
func NewLicensesCollector(logger *logger.Logger) *LicenseCollector {
	labelnames := make([]string, 0, 1)
	labelnames = append(labelnames, "license")
	return &LicenseCollector{
		total:  prometheus.NewDesc("slurm_license_total", "Total licenses", labelnames, nil),
		used:   prometheus.NewDesc("slurm_license_used", "Used licenses", labelnames, nil),
		free:   prometheus.NewDesc("slurm_license_free", "Free licenses", labelnames, nil),
		logger: logger,
	}
}

type LicenseCollector struct {
	total  *prometheus.Desc
	used   *prometheus.Desc
	free   *prometheus.Desc
	logger *logger.Logger
}

func (lc *LicenseCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- lc.total
	ch <- lc.used
	ch <- lc.free
}

func (lc *LicenseCollector) Collect(ch chan<- prometheus.Metric) {
	lm, err := LicenseGetMetrics(lc.logger)
	if err != nil {
		lc.logger.Error("Failed to get license metrics", "err", err)
		return
	}

	for name := range lm.total {
		ch <- prometheus.MustNewConstMetric(lc.total, prometheus.GaugeValue, lm.total[name], name)
		ch <- prometheus.MustNewConstMetric(lc.used, prometheus.GaugeValue, lm.used[name], name)
		ch <- prometheus.MustNewConstMetric(lc.free, prometheus.GaugeValue, lm.free[name], name)
	}
}
