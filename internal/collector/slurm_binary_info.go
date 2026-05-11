package collector

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// SlurmInfoCollector defines a Prometheus collector for Slurm binary and version information
type SlurmInfoCollector struct {
	slurmInfo        *prometheus.Desc
	requiredBinaries []string
	optionalBinaries []string
	logger           *logger.Logger
}

func NewSlurmInfoCollector(logger *logger.Logger) *SlurmInfoCollector {
	// requiredBinaries are always emitted; if missing they appear with
	// version="not_found" and value=0 so operators can alert on their absence.
	requiredBinaries := []string{
		"sinfo", "squeue", "sdiag", "scontrol",
		"sacct",
	}
	// optionalBinaries are emitted only if present on the host (silent if absent).
	// These are job-submission tools that the exporter never invokes — surfacing
	// their version is informational, never required.
	optionalBinaries := []string{
		"sbatch", "salloc", "srun",
	}
	labels := []string{"type", "binary", "version"}
	return &SlurmInfoCollector{
		slurmInfo:        prometheus.NewDesc("slurm_info", "Information on Slurm version and binaries", labels, nil),
		requiredBinaries: requiredBinaries,
		optionalBinaries: optionalBinaries,
		logger:           logger,
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

	// Required binaries: always emit a series (value=0 with version="not_found"
	// if the binary is missing, so absence is visible in alerts).
	for _, binary := range c.requiredBinaries {
		binVersion, binFound := GetBinaryVersion(c.logger, binary)
		binValue := 0.0
		if binFound {
			binValue = 1.0
		}
		ch <- prometheus.MustNewConstMetric(c.slurmInfo, prometheus.GaugeValue, binValue, "binary", binary, binVersion)
	}

	// Optional binaries: emit only if the binary is found on disk. No log,
	// no metric, no error if absent. Lets monitoring-only deployments
	// silently skip job-submission tools without polluting /metrics.
	for _, binary := range c.optionalBinaries {
		if !binaryAvailable(binary) {
			continue
		}
		binVersion, binFound := GetBinaryVersion(c.logger, binary)
		if !binFound {
			continue
		}
		ch <- prometheus.MustNewConstMetric(c.slurmInfo, prometheus.GaugeValue, 1.0, "binary", binary, binVersion)
	}
}

// binaryAvailable reports whether the given binary can be found on disk
// without invoking it (so no log spam or process spawn).
//
// Resolution mirrors what Execute() does:
//   - if --slurm.bin-path was set, look in that directory directly,
//   - otherwise, search the system $PATH via exec.LookPath.
//
// Overridable in tests via binaryAvailableFunc.
var binaryAvailable = func(binary string) bool {
	if binPath != "" {
		info, err := os.Stat(filepath.Join(binPath, binary))
		return err == nil && !info.IsDir()
	}
	_, err := exec.LookPath(binary)
	return err == nil
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
