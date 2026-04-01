package collector

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// Pre-compiled regexes for node state matching — avoids re-compilation on every loop iteration.
var (
	nodeStateAlloc   = regexp.MustCompile(`^alloc`)
	nodeStateComp    = regexp.MustCompile(`^comp`)
	nodeStateDown    = regexp.MustCompile(`^down`)
	nodeStateDrain   = regexp.MustCompile(`^drain`)
	nodeStateFail    = regexp.MustCompile(`^fail`)
	nodeStateErr     = regexp.MustCompile(`^err`)
	nodeStateIdle    = regexp.MustCompile(`^idle`)
	nodeStateInval   = regexp.MustCompile(`^inval`)
	nodeStateMaint   = regexp.MustCompile(`^maint`)
	nodeStateMix     = regexp.MustCompile(`^mix`)
	nodeStateResv    = regexp.MustCompile(`^res`)
	nodeStatePlanned = regexp.MustCompile(`^planned`)
)

type NodesMetrics struct {
	alloc   map[string]float64
	comp    map[string]float64
	down    map[string]float64
	drain   map[string]float64
	err     map[string]float64
	fail    map[string]float64
	idle    map[string]float64
	inval   map[string]float64
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

// InitFeatureSet is a no-op kept for backwards compatibility.
func InitFeatureSet(_ *NodesMetrics, _ string) {}

/*
ParseNodesMetrics parses the output of the sinfo command for node metrics.
Expected input format: "%D|%T|%b" (Nodes|State|Features).
*/
func ParseNodesMetrics(input []byte) *NodesMetrics {
	var nm NodesMetrics
	var featureSet string
	lines := strings.Split(string(input), "\n")

	// Sort and remove all the duplicates from the 'sinfo' output
	slices.Sort(lines)
	linesUniq := slices.Compact(lines)

	nm.alloc = make(map[string]float64)
	nm.comp = make(map[string]float64)
	nm.down = make(map[string]float64)
	nm.drain = make(map[string]float64)
	nm.err = make(map[string]float64)
	nm.fail = make(map[string]float64)
	nm.idle = make(map[string]float64)
	nm.inval = make(map[string]float64)
	nm.maint = make(map[string]float64)
	nm.mix = make(map[string]float64)
	nm.resv = make(map[string]float64)
	nm.other = make(map[string]float64)
	nm.planned = make(map[string]float64)
	nm.total = make(map[string]float64)

	for _, line := range linesUniq {
		if strings.Contains(line, "|") {
			split := strings.Split(line, "|")
			if len(split) < 3 {
				continue
			}
			state := split[1]
			count, _ := strconv.ParseFloat(strings.TrimSpace(split[0]), 64)
			features := strings.Split(split[2], ",")
			slices.Sort(features)
			featureSet = strings.Join(features, ",")
			if featureSet == "(null)" {
				featureSet = "null"
			}
			InitFeatureSet(&nm, featureSet)
			switch {
			case nodeStateAlloc.MatchString(state):
				nm.alloc[featureSet] += count
			case nodeStateComp.MatchString(state):
				nm.comp[featureSet] += count
			case nodeStateDown.MatchString(state):
				nm.down[featureSet] += count
			case nodeStateDrain.MatchString(state):
				nm.drain[featureSet] += count
			case nodeStateFail.MatchString(state):
				nm.fail[featureSet] += count
			case nodeStateErr.MatchString(state):
				nm.err[featureSet] += count
			case nodeStateIdle.MatchString(state):
				nm.idle[featureSet] += count
			case nodeStateInval.MatchString(state):
				nm.inval[featureSet] += count
			case nodeStateMaint.MatchString(state):
				nm.maint[featureSet] += count
			case nodeStateMix.MatchString(state):
				nm.mix[featureSet] += count
			case nodeStateResv.MatchString(state):
				nm.resv[featureSet] += count
			case nodeStatePlanned.MatchString(state):
				nm.planned[featureSet] += count
			default:
				nm.other[featureSet] += count
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

// NodesDataGlobal executes a single sinfo call for ALL partitions at once.
// Format: "%R|%D|%T|%b" (Partition|Nodes|State|Features).
// This replaces N per-partition calls with one RPC, significantly reducing
// load on slurmctld on clusters with many partitions.
func NodesDataGlobal(log *logger.Logger) ([]byte, error) {
	return Execute(log, "sinfo", []string{"-h", "-o", "%R|%D|%T|%b"})
}

// ParseNodesMetricsGlobal parses the global sinfo output (with partition column)
// into a map of partition name → NodesMetrics.
// Input format: "%R|%D|%T|%b" (Partition|Nodes|State|Features).
func ParseNodesMetricsGlobal(input []byte) map[string]*NodesMetrics {
	result := make(map[string]*NodesMetrics)

	lines := strings.Split(string(input), "\n")
	slices.Sort(lines)
	linesUniq := slices.Compact(lines)

	for _, line := range linesUniq {
		if !strings.Contains(line, "|") {
			continue
		}
		split := strings.Split(line, "|")
		if len(split) < 4 {
			continue
		}
		// Strip the default partition marker (*) for consistent partition names
		part := strings.TrimRight(strings.TrimSpace(split[0]), "*")
		count, _ := strconv.ParseFloat(strings.TrimSpace(split[1]), 64)
		state := split[2]
		features := strings.Split(split[3], ",")
		slices.Sort(features)
		featureSet := strings.Join(features, ",")
		if featureSet == "(null)" {
			featureSet = "null"
		}

		if _, ok := result[part]; !ok {
			result[part] = &NodesMetrics{
				alloc:   make(map[string]float64),
				comp:    make(map[string]float64),
				down:    make(map[string]float64),
				drain:   make(map[string]float64),
				err:     make(map[string]float64),
				fail:    make(map[string]float64),
				idle:    make(map[string]float64),
				inval:   make(map[string]float64),
				maint:   make(map[string]float64),
				mix:     make(map[string]float64),
				resv:    make(map[string]float64),
				other:   make(map[string]float64),
				planned: make(map[string]float64),
				total:   make(map[string]float64),
			}
		}
		nm := result[part]

		switch {
		case nodeStateAlloc.MatchString(state):
			nm.alloc[featureSet] += count
		case nodeStateComp.MatchString(state):
			nm.comp[featureSet] += count
		case nodeStateDown.MatchString(state):
			nm.down[featureSet] += count
		case nodeStateDrain.MatchString(state):
			nm.drain[featureSet] += count
		case nodeStateFail.MatchString(state):
			nm.fail[featureSet] += count
		case nodeStateErr.MatchString(state):
			nm.err[featureSet] += count
		case nodeStateIdle.MatchString(state):
			nm.idle[featureSet] += count
		case nodeStateInval.MatchString(state):
			nm.inval[featureSet] += count
		case nodeStateMaint.MatchString(state):
			nm.maint[featureSet] += count
		case nodeStateMix.MatchString(state):
			nm.mix[featureSet] += count
		case nodeStateResv.MatchString(state):
			nm.resv[featureSet] += count
		case nodeStatePlanned.MatchString(state):
			nm.planned[featureSet] += count
		default:
			nm.other[featureSet] += count
		}
	}
	return result
}

// NodesGetMetricsGlobal fetches and parses node metrics for all partitions
// in a single sinfo call.
func NodesGetMetricsGlobal(log *logger.Logger) (map[string]*NodesMetrics, error) {
	data, err := NodesDataGlobal(log)
	if err != nil {
		return nil, err
	}
	return ParseNodesMetricsGlobal(data), nil
}

/*
SlurmGetTotal retrieves the total number of nodes from scontrol.
Expected scontrol output format: one line per node.
Uses scontrolNodesCache to avoid redundant fetches when both the nodes and
reservation_nodes collectors run in the same scrape cycle.
*/
func SlurmGetTotal(log *logger.Logger) (float64, error) {
	out, err := scontrolNodesCache.GetOrFetch(func() ([]byte, error) {
		return Execute(log, "scontrol", []string{"show", "nodes", "-o"})
	})
	updateCacheAge()
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
	slices.Sort(cleanedPartitions)
	return slices.Compact(cleanedPartitions), nil
}

/*
 * Implement the Prometheus Collector interface and feed the
 * Slurm scheduler metrics into it.
 * https://godoc.org/github.com/prometheus/client_golang/prometheus#Collector
 */

// NewNodesCollector creates a nodes metrics collector.
// When withFeatureSet is false, the active_feature_set label is omitted and
// node counts are aggregated per partition only — useful on homogeneous clusters
// to avoid high metric cardinality (controlled via --collector.nodes.feature-set).
func NewNodesCollector(logger *logger.Logger, withFeatureSet bool) *NodesCollector {
	var labelnames []string
	if withFeatureSet {
		labelnames = []string{"partition", "active_feature_set"}
	} else {
		labelnames = []string{"partition"}
	}
	return &NodesCollector{
		alloc:          prometheus.NewDesc("slurm_nodes_alloc", "Allocated nodes", labelnames, nil),
		comp:           prometheus.NewDesc("slurm_nodes_comp", "Completing nodes", labelnames, nil),
		down:           prometheus.NewDesc("slurm_nodes_down", "Down nodes", labelnames, nil),
		drain:          prometheus.NewDesc("slurm_nodes_drain", "Drain nodes", labelnames, nil),
		err:            prometheus.NewDesc("slurm_nodes_err", "Error nodes", labelnames, nil),
		fail:           prometheus.NewDesc("slurm_nodes_fail", "Fail nodes", labelnames, nil),
		idle:           prometheus.NewDesc("slurm_nodes_idle", "Idle nodes", labelnames, nil),
		inval:          prometheus.NewDesc("slurm_nodes_inval", "Inval nodes", labelnames, nil),
		maint:          prometheus.NewDesc("slurm_nodes_maint", "Maint nodes", labelnames, nil),
		mix:            prometheus.NewDesc("slurm_nodes_mix", "Mix nodes", labelnames, nil),
		resv:           prometheus.NewDesc("slurm_nodes_resv", "Reserved nodes", labelnames, nil),
		other:          prometheus.NewDesc("slurm_nodes_other", "Nodes reported with an unknown state", labelnames, nil),
		planned:        prometheus.NewDesc("slurm_nodes_planned", "Planned nodes", labelnames, nil),
		total:          prometheus.NewDesc("slurm_nodes_total", "Total number of nodes", nil, nil),
		withFeatureSet: withFeatureSet,
		logger:         logger,
	}
}

type NodesCollector struct {
	alloc          *prometheus.Desc
	comp           *prometheus.Desc
	down           *prometheus.Desc
	drain          *prometheus.Desc
	err            *prometheus.Desc
	fail           *prometheus.Desc
	idle           *prometheus.Desc
	inval          *prometheus.Desc
	maint          *prometheus.Desc
	mix            *prometheus.Desc
	resv           *prometheus.Desc
	other          *prometheus.Desc
	planned        *prometheus.Desc
	total          *prometheus.Desc
	withFeatureSet bool
	logger         *logger.Logger
}

func (nc *NodesCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- nc.alloc
	ch <- nc.comp
	ch <- nc.down
	ch <- nc.drain
	ch <- nc.err
	ch <- nc.fail
	ch <- nc.idle
	ch <- nc.inval
	ch <- nc.maint
	ch <- nc.mix
	ch <- nc.resv
	ch <- nc.other
	ch <- nc.planned
	ch <- nc.total
}

// sumMap returns the sum of all values in a float64 map.
func sumMap(m map[string]float64) float64 {
	var total float64
	for _, v := range m {
		total += v
	}
	return total
}

func SendFeatureSetMetric(ch chan<- prometheus.Metric, desc *prometheus.Desc, valueType prometheus.ValueType, featurestate map[string]float64, part string) {
	for set, value := range featurestate {
		ch <- prometheus.MustNewConstMetric(desc, valueType, value, part, set)
	}
}

func (nc *NodesCollector) Collect(ch chan<- prometheus.Metric) {
	// Single global sinfo call for all partitions — replaces N per-partition calls.
	allPartitions, err := NodesGetMetricsGlobal(nc.logger)
	if err != nil {
		nc.logger.Error("Failed to get global nodes metrics", "err", err)
		return
	}
	for part, nm := range allPartitions {
		// Create a slice of all the metric maps
		allMaps := []map[string]float64{
			nm.alloc, nm.comp, nm.down, nm.drain, nm.err, nm.fail,
			nm.idle, nm.inval, nm.maint, nm.mix, nm.resv, nm.other, nm.planned,
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

		if nc.withFeatureSet {
			SendFeatureSetMetric(ch, nc.alloc, prometheus.GaugeValue, nm.alloc, part)
			SendFeatureSetMetric(ch, nc.comp, prometheus.GaugeValue, nm.comp, part)
			SendFeatureSetMetric(ch, nc.down, prometheus.GaugeValue, nm.down, part)
			SendFeatureSetMetric(ch, nc.drain, prometheus.GaugeValue, nm.drain, part)
			SendFeatureSetMetric(ch, nc.err, prometheus.GaugeValue, nm.err, part)
			SendFeatureSetMetric(ch, nc.fail, prometheus.GaugeValue, nm.fail, part)
			SendFeatureSetMetric(ch, nc.idle, prometheus.GaugeValue, nm.idle, part)
			SendFeatureSetMetric(ch, nc.inval, prometheus.GaugeValue, nm.inval, part)
			SendFeatureSetMetric(ch, nc.maint, prometheus.GaugeValue, nm.maint, part)
			SendFeatureSetMetric(ch, nc.mix, prometheus.GaugeValue, nm.mix, part)
			SendFeatureSetMetric(ch, nc.resv, prometheus.GaugeValue, nm.resv, part)
			SendFeatureSetMetric(ch, nc.other, prometheus.GaugeValue, nm.other, part)
			SendFeatureSetMetric(ch, nc.planned, prometheus.GaugeValue, nm.planned, part)
		} else {
			// feature-set disabled: aggregate all feature sets into a single per-partition metric
			ch <- prometheus.MustNewConstMetric(nc.alloc, prometheus.GaugeValue, sumMap(nm.alloc), part)
			ch <- prometheus.MustNewConstMetric(nc.comp, prometheus.GaugeValue, sumMap(nm.comp), part)
			ch <- prometheus.MustNewConstMetric(nc.down, prometheus.GaugeValue, sumMap(nm.down), part)
			ch <- prometheus.MustNewConstMetric(nc.drain, prometheus.GaugeValue, sumMap(nm.drain), part)
			ch <- prometheus.MustNewConstMetric(nc.err, prometheus.GaugeValue, sumMap(nm.err), part)
			ch <- prometheus.MustNewConstMetric(nc.fail, prometheus.GaugeValue, sumMap(nm.fail), part)
			ch <- prometheus.MustNewConstMetric(nc.idle, prometheus.GaugeValue, sumMap(nm.idle), part)
			ch <- prometheus.MustNewConstMetric(nc.inval, prometheus.GaugeValue, sumMap(nm.inval), part)
			ch <- prometheus.MustNewConstMetric(nc.maint, prometheus.GaugeValue, sumMap(nm.maint), part)
			ch <- prometheus.MustNewConstMetric(nc.mix, prometheus.GaugeValue, sumMap(nm.mix), part)
			ch <- prometheus.MustNewConstMetric(nc.resv, prometheus.GaugeValue, sumMap(nm.resv), part)
			ch <- prometheus.MustNewConstMetric(nc.other, prometheus.GaugeValue, sumMap(nm.other), part)
			ch <- prometheus.MustNewConstMetric(nc.planned, prometheus.GaugeValue, sumMap(nm.planned), part)
		}
	}
	total, err := SlurmGetTotal(nc.logger)
	if err != nil {
		nc.logger.Error("Failed to get total nodes", "err", err)
		return
	}
	ch <- prometheus.MustNewConstMetric(nc.total, prometheus.GaugeValue, total)
}
