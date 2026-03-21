package collector

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// licenseLineRe parses one line of "scontrol show licenses -o" output.
// The real format includes additional fields (Remote, LastConsumed, LastDeficit,
// LastUpdate) after the four we care about, so we anchor only up to Reserved.
var licenseLineRe = regexp.MustCompile(`LicenseName=(\S+) Total=(\d+) Used=(\d+) Free=(\d+) Reserved=(\d+)`)

type LicenseMetrics struct {
	total    map[string]float64
	used     map[string]float64
	free     map[string]float64
	reserved map[string]float64
}

// LicenseGetMetrics fetches and parses license metrics.
func LicenseGetMetrics(logger *logger.Logger) (*LicenseMetrics, error) {
	data, err := LicenseData(logger)
	if err != nil {
		return nil, err
	}
	return ParseLicenseMetrics(data), nil
}

// ParseLicenseMetrics parses "scontrol show licenses -o" output.
// Each line contains one license entry with Total, Used, Free, and Reserved counts.
func ParseLicenseMetrics(input []byte) *LicenseMetrics {
	lm := &LicenseMetrics{
		total:    make(map[string]float64),
		used:     make(map[string]float64),
		free:     make(map[string]float64),
		reserved: make(map[string]float64),
	}

	for line := range strings.SplitSeq(string(input), "\n") {
		matches := licenseLineRe.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		name := matches[1]
		lm.total[name], _ = strconv.ParseFloat(matches[2], 64)
		lm.used[name], _ = strconv.ParseFloat(matches[3], 64)
		lm.free[name], _ = strconv.ParseFloat(matches[4], 64)
		lm.reserved[name], _ = strconv.ParseFloat(matches[5], 64)
	}
	return lm
}

// LicenseData runs scontrol to retrieve license information.
func LicenseData(logger *logger.Logger) ([]byte, error) {
	return Execute(logger, "scontrol", []string{"show", "licenses", "-o"})
}

// NewLicensesCollector creates a collector for software license metrics.
func NewLicensesCollector(logger *logger.Logger) *LicenseCollector {
	labels := []string{"license"}
	return &LicenseCollector{
		total:    prometheus.NewDesc("slurm_license_total", "Total count for license", labels, nil),
		used:     prometheus.NewDesc("slurm_license_used", "Used count for license", labels, nil),
		free:     prometheus.NewDesc("slurm_license_free", "Free count for license", labels, nil),
		reserved: prometheus.NewDesc("slurm_license_reserved", "Reserved count for license", labels, nil),
		logger:   logger,
	}
}

type LicenseCollector struct {
	total    *prometheus.Desc
	used     *prometheus.Desc
	free     *prometheus.Desc
	reserved *prometheus.Desc
	logger   *logger.Logger
}

func (lc *LicenseCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- lc.total
	ch <- lc.used
	ch <- lc.free
	ch <- lc.reserved
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
		ch <- prometheus.MustNewConstMetric(lc.reserved, prometheus.GaugeValue, lm.reserved[name], name)
	}
}
