package collector

import (
	"strings"

	"github.com/mike8161990/slurm_exporter/internal/logger"
	"github.com/prometheus/client_golang/prometheus"
)

// SlurmInfoCollector defines a Prometheus collector for Slurm binary and version information
type SlurmInfoCollector struct {
	slurmInfo *prometheus.Desc
	binaries  []string
	logger    *logger.Logger
}

func NewSlurmInfoCollector(logger *logger.Logger) *SlurmInfoCollector {
	binaries := []string{
		"sinfo", "squeue", "sdiag", "scontrol",
		"sacct", "sbatch", "salloc", "srun",
	}
	labels := []string{"type", "binary", "version"}
	return &SlurmInfoCollector{
		slurmInfo: prometheus.NewDesc("slurm_info", "Information on Slurm version and binaries", labels, nil),
		binaries:  binaries,
		logger:    logger,
	}
}

func (c *SlurmInfoCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.slurmInfo
}

func (c *SlurmInfoCollector) Collect(ch chan<- prometheus.Metric) {

	version, found := GetBinaryVersion(c.logger, "sinfo")
	versionValue := 0.0
	if found {
		versionValue = 1.0
	}

	ch <- prometheus.MustNewConstMetric(c.slurmInfo, prometheus.GaugeValue, versionValue, "general", "", version)

	for _, binary := range c.binaries {
		binVersion, binFound := GetBinaryVersion(c.logger, binary)
		binValue := 0.0
		if binFound {
			binValue = 1.0
		}
		ch <- prometheus.MustNewConstMetric(c.slurmInfo, prometheus.GaugeValue, binValue, "binary", binary, binVersion)
	}
}

func GetBinaryVersion(logger *logger.Logger, binary string) (string, bool) {
	output, err := Execute(logger, binary, []string{"--version"})
	if err != nil {
		// The Execute function already logs the error, so we just return.
		return "not_found", false
	}

	// Extract the version number from the output, e.g., "slurm 23.11.6"
	fields := strings.Fields(string(output))
	if len(fields) > 1 {
		return fields[1], true
	}
	return "unknown", true
}
