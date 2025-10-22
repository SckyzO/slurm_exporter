package collector

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sckyzo/slurm_exporter/internal/logger"
)

type NodesMetrics struct {
	alloc   map[string]float64
	comp    map[string]float64
	down    map[string]float64
	drain   map[string]float64
	err     map[string]float64
	fail    map[string]float64
	idle    map[string]float64
	maint   map[string]float64
	mix     map[string]float64
	resv    map[string]float64
	other   map[string]float64
	planned map[string]float64
	total   map[string]float64
}

func NodesGetMetrics(logger *logger.Logger, part string) (*NodesMetrics, error) {
	data, err := NodesData(logger, part)
	if err != nil {
		return nil, err
	}
	return ParseNodesMetrics(data), nil
}



func InitFeatureSet(nm *NodesMetrics, feature_set string) {
	// This function is intentionally left empty.
	// It was previously used to initialize map keys, but this is not necessary in Go.
	// The linter correctly identified self-assignments here.
}

/*
ParseNodesMetrics parses the output of the sinfo command for node metrics.
Expected input format: "%D|%T|%b" (Nodes|State|Features).
*/
func ParseNodesMetrics(input []byte) *NodesMetrics {
	var nm NodesMetrics
	var feature_set string
	lines := strings.Split(string(input), "\n")

	// Sort and remove all the duplicates from the 'sinfo' output
	sort.Strings(lines)
	lines_uniq := RemoveDuplicates(lines)

	nm.alloc = make(map[string]float64)
	nm.comp = make(map[string]float64)
	nm.down = make(map[string]float64)
	nm.drain = make(map[string]float64)
	nm.err = make(map[string]float64)
	nm.fail = make(map[string]float64)
	nm.idle = make(map[string]float64)
	nm.maint = make(map[string]float64)
	nm.mix = make(map[string]float64)
	nm.resv = make(map[string]float64)
	nm.other = make(map[string]float64)
	nm.planned = make(map[string]float64)
	nm.total = make(map[string]float64)

	for _, line := range lines_uniq {
		if strings.Contains(line, "|") {
			split := strings.Split(line, "|")
			state := split[1]
			count, _ := strconv.ParseFloat(strings.TrimSpace(split[0]), 64)
			features := strings.Split(split[2], ",")
			sort.Strings(features)
			feature_set = strings.Join(features[:], ",")
			if feature_set == "(null)" {
				feature_set = "null"
			}
			InitFeatureSet(&nm, feature_set)
			alloc := regexp.MustCompile(`^alloc`)
			comp := regexp.MustCompile(`^comp`)
			down := regexp.MustCompile(`^down`)
			drain := regexp.MustCompile(`^drain`)
			fail := regexp.MustCompile(`^fail`)
			err := regexp.MustCompile(`^err`)
			idle := regexp.MustCompile(`^idle`)
			maint := regexp.MustCompile(`^maint`)
			mix := regexp.MustCompile(`^mix`)
			resv := regexp.MustCompile(`^res`)
			planned := regexp.MustCompile(`^planned`)
			switch {
			case alloc.MatchString(state):
				nm.alloc[feature_set] += count
			case comp.MatchString(state):
				nm.comp[feature_set] += count
			case down.MatchString(state):
				nm.down[feature_set] += count
			case drain.MatchString(state):
				nm.drain[feature_set] += count
			case fail.MatchString(state):
				nm.fail[feature_set] += count
			case err.MatchString(state):
				nm.err[feature_set] += count
			case idle.MatchString(state):
				nm.idle[feature_set] += count
			case maint.MatchString(state):
				nm.maint[feature_set] += count
			case mix.MatchString(state):
				nm.mix[feature_set] += count
			case resv.MatchString(state):
				nm.resv[feature_set] += count
			case planned.MatchString(state):
				nm.planned[feature_set] += count
			default:
				nm.other[feature_set] += count
			}
		}
	}
	return &nm
}


/*
NodesData executes the sinfo command to retrieve node information.
Expected sinfo output format: "%D|%T|%b" (Nodes|State|Features).
*/
func NodesData(logger *logger.Logger, part string) ([]byte, error) {
	return Execute(logger, "sinfo", []string{"-h", "-o", "%D|%T|%b", "-p", part})
}

/*
SlurmGetTotal retrieves the total number of nodes from scontrol.
Expected scontrol output format: one line per node.
*/
func SlurmGetTotal(logger *logger.Logger) (float64, error) {
	out, err := Execute(logger, "scontrol", []string{"show", "nodes", "-o"})
	if err != nil {
		return 0, err
	}
	// Filter out empty lines before counting
	lines := strings.Split(string(out), "\n")
	count := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return float64(count), nil
}

/*
SlurmGetPartitions retrieves a list of all partitions from sinfo.
Expected sinfo output format: "%R" (Partition name).
*/
func SlurmGetPartitions(logger *logger.Logger) ([]string, error) {
	out, err := Execute(logger, "sinfo", []string{"-h", "-o", "%R"})
	if err != nil {
		return nil, err
	}
	partitions := strings.Split(string(out), "\n")
	// Trim whitespace and remove empty strings
	var cleanedPartitions []string
	for _, p := range partitions {
		p = strings.TrimSpace(p)
		if p != "" {
			cleanedPartitions = append(cleanedPartitions, p)
		}
	}
	sort.Strings(cleanedPartitions)
	return RemoveDuplicates(cleanedPartitions), nil
}

/*
 * Implement the Prometheus Collector interface and feed the
 * Slurm scheduler metrics into it.
 * https://godoc.org/github.com/prometheus/client_golang/prometheus#Collector
 */

func NewNodesCollector(logger *logger.Logger) *NodesCollector {
	labelnames := make([]string, 0, 1)
	labelnames = append(labelnames, "partition")
	labelnames = append(labelnames, "active_feature_set")
	return &NodesCollector{
		alloc:   prometheus.NewDesc("slurm_nodes_alloc", "Allocated nodes", labelnames, nil),
		comp:    prometheus.NewDesc("slurm_nodes_comp", "Completing nodes", labelnames, nil),
		down:    prometheus.NewDesc("slurm_nodes_down", "Down nodes", labelnames, nil),
		drain:   prometheus.NewDesc("slurm_nodes_drain", "Drain nodes", labelnames, nil),
		err:     prometheus.NewDesc("slurm_nodes_err", "Error nodes", labelnames, nil),
		fail:    prometheus.NewDesc("slurm_nodes_fail", "Fail nodes", labelnames, nil),
		idle:    prometheus.NewDesc("slurm_nodes_idle", "Idle nodes", labelnames, nil),
		maint:   prometheus.NewDesc("slurm_nodes_maint", "Maint nodes", labelnames, nil),
		mix:     prometheus.NewDesc("slurm_nodes_mix", "Mix nodes", labelnames, nil),
		resv:    prometheus.NewDesc("slurm_nodes_resv", "Reserved nodes", labelnames, nil),
		other:   prometheus.NewDesc("slurm_nodes_other", "Nodes reported with an unknown state", labelnames, nil),
		planned: prometheus.NewDesc("slurm_nodes_planned", "Planned nodes", labelnames, nil),
		total:   prometheus.NewDesc("slurm_nodes_total", "Total number of nodes", nil, nil),
		logger:  logger,
	}
}

type NodesCollector struct {
	alloc   *prometheus.Desc
	comp    *prometheus.Desc
	down    *prometheus.Desc
	drain   *prometheus.Desc
	err     *prometheus.Desc
	fail    *prometheus.Desc
	idle    *prometheus.Desc
	maint   *prometheus.Desc
	mix     *prometheus.Desc
	resv    *prometheus.Desc
	other   *prometheus.Desc
	planned *prometheus.Desc
	total   *prometheus.Desc
	logger  *logger.Logger
}


func (nc *NodesCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- nc.alloc
	ch <- nc.comp
	ch <- nc.down
	ch <- nc.drain
	ch <- nc.err
	ch <- nc.fail
	ch <- nc.idle
	ch <- nc.maint
	ch <- nc.mix
	ch <- nc.resv
	ch <- nc.other
	ch <- nc.planned
	ch <- nc.total
}

func SendFeatureSetMetric(ch chan<- prometheus.Metric, desc *prometheus.Desc, valueType prometheus.ValueType, featurestate map[string]float64, part string) {
	for set, value := range featurestate {
		ch <- prometheus.MustNewConstMetric(desc, valueType, value, part, set)
	}
}

func (nc *NodesCollector) Collect(ch chan<- prometheus.Metric) {
	partitions, err := SlurmGetPartitions(nc.logger)
	if err != nil {
		nc.logger.Error("Failed to get partitions", "err", err)
		return
	}
	for _, part := range partitions {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		nm, err := NodesGetMetrics(nc.logger, part)
		if err != nil {
			nc.logger.Error("Failed to get nodes metrics", "partition", part, "err", err)
			continue
		}

		// Create a slice of all the metric maps
		allMaps := []map[string]float64{
			nm.alloc, nm.comp, nm.down, nm.drain, nm.err, nm.fail,
			nm.idle, nm.maint, nm.mix, nm.resv, nm.other, nm.planned,
		}

		// Collect all unique feature sets across all maps
		allFeatureSets := make(map[string]struct{})
		for _, metricMap := range allMaps {
			for fs := range metricMap {
				allFeatureSets[fs] = struct{}{}
			}
		}

		// Ensure all maps have all feature sets, defaulting to 0
		for _, metricMap := range allMaps {
			for fs := range allFeatureSets {
				if _, ok := metricMap[fs]; !ok {
					metricMap[fs] = 0
				}
			}
		}

		SendFeatureSetMetric(ch, nc.alloc, prometheus.GaugeValue, nm.alloc, part)
		SendFeatureSetMetric(ch, nc.comp, prometheus.GaugeValue, nm.comp, part)
		SendFeatureSetMetric(ch, nc.down, prometheus.GaugeValue, nm.down, part)
		SendFeatureSetMetric(ch, nc.drain, prometheus.GaugeValue, nm.drain, part)
		SendFeatureSetMetric(ch, nc.err, prometheus.GaugeValue, nm.err, part)
		SendFeatureSetMetric(ch, nc.fail, prometheus.GaugeValue, nm.fail, part)
		SendFeatureSetMetric(ch, nc.idle, prometheus.GaugeValue, nm.idle, part)
		SendFeatureSetMetric(ch, nc.maint, prometheus.GaugeValue, nm.maint, part)
		SendFeatureSetMetric(ch, nc.mix, prometheus.GaugeValue, nm.mix, part)
		SendFeatureSetMetric(ch, nc.resv, prometheus.GaugeValue, nm.resv, part)
		SendFeatureSetMetric(ch, nc.other, prometheus.GaugeValue, nm.other, part)
		SendFeatureSetMetric(ch, nc.planned, prometheus.GaugeValue, nm.planned, part)
	}
	total, err := SlurmGetTotal(nc.logger)
	if err != nil {
		nc.logger.Error("Failed to get total nodes", "err", err)
		return
	}
	ch <- prometheus.MustNewConstMetric(nc.total, prometheus.GaugeValue, total)
}