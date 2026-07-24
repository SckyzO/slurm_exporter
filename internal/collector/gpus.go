package collector

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// GPUsMetrics holds GPU utilization statistics from Slurm
type GPUsMetrics struct {
	alloc       float64 // Number of allocated GPUs
	idle        float64 // Number of idle GPUs
	other       float64 // Number of GPUs in other states (mixed, down, etc.)
	total       float64 // Total number of GPUs in the cluster
	utilization float64 // GPU utilization ratio (allocated/total)
}

// GPUsGetMetrics retrieves and parses GPU metrics from Slurm
func GPUsGetMetrics(logger *logger.Logger) (*GPUsMetrics, error) {
	return ParseGPUsMetrics(logger)
}

// ParseAllocatedGPUs parses the output of sinfo command to count allocated GPUs
// Expected input format examples:
//   - slurm>=20.11.8: "3 gpu:2"
//   - slurm 21.08.5:  "1 gpu:(null):3(IDX:0-7)"
//   - slurm 21.08.5:  "13 gpu:A30:4(IDX:0-3),gpu:Q6K:4(IDX:0-3)"
func ParseAllocatedGPUs(data []byte) float64 {
	var numGPUs = 0.0
	sinfoLines := string(data)
	if len(sinfoLines) > 0 {
		for _, line := range strings.Split(sinfoLines, "\n") {
			if len(line) > 0 && strings.Contains(line, "gpu:") {
				fields := strings.Fields(line)
				if len(fields) < 2 {
					continue
				}

				numNodes, _ := strconv.ParseFloat(fields[0], 64)
				numGPUs += numNodes * parseGPUCount(fields[1])
			}
		}
	}

	return numGPUs
}

// ParseIdleGPUs calculates idle GPUs by subtracting allocated from total GPUs
// Expected input format examples:
//   - slurm 20.11.8:  "3 gpu:4 gpu:2" (total available, allocated)
//   - slurm 21.08.5:  "1 gpu:8(S:0-1) gpu:(null):3(IDX:0-7)"
//   - slurm 21.08.5:  "13 gpu:A30:4(S:0-1),gpu:Q6K:40(S:0-1) gpu:A30:4(IDX:0-3),gpu:Q6K:4(IDX:0-3)"
func ParseIdleGPUs(data []byte) float64 {
	var numGPUs = 0.0
	sinfoLines := string(data)
	if len(sinfoLines) > 0 {
		for _, line := range strings.Split(sinfoLines, "\n") {
			if len(line) > 0 && strings.Contains(line, "gpu:") {
				fields := strings.Fields(line)
				if len(fields) < 1 {
					continue
				}

				numNodes, _ := strconv.ParseFloat(fields[0], 64)

				switch len(fields) {
				case 1:
					// Only node count, no GPU info - assume no idle GPUs
					numGPUs += 0
				case 2:
					// Two columns: nodes and total GPUs (no allocated info)
					totalGPUs := parseGPUCount(fields[1])
					numGPUs += numNodes * totalGPUs
				default:
					// Three or more columns: nodes, total GPUs, allocated GPUs
					totalGPUs := parseGPUCount(fields[1])
					allocatedGPUs := parseGPUCount(fields[2])
					idleGPUs := totalGPUs - allocatedGPUs
					numGPUs += numNodes * idleGPUs
				}
			}
		}
	}

	return numGPUs
}

// gpuGresRe matches one GPU entry in a Slurm GRES/GresUsed field —
// "gpu:<type>:<count>" or "gpu:<count>", with an optional "(null)" type and an
// optional "(IDX:…)"/"(S:…)" suffix. Capture group 2 is the count. Compiled
// once at package level rather than per Collect(), the convention the other
// collectors follow (see nodes.go, scheduler.go).
var gpuGresRe = regexp.MustCompile(`gpu:(\(null\)|[^:(]*):?([0-9]+)(\([^)]*\))?`)

// parseGPUCount sums the GPU counts across every gpu: entry in a comma-separated
// GRES specification. A node can expose several GPU types at once
// ("gpu:A100:4,gpu:H100:2" → 6), and non-gpu GRES (e.g. "mig:…") is ignored.
// Shared by the cluster-wide (gpus.go) and per-partition (partitions.go) paths.
func parseGPUCount(gpuSpec string) float64 {
	var count = 0.0
	for _, spec := range strings.Split(gpuSpec, ",") {
		if !strings.Contains(spec, "gpu:") {
			continue
		}
		matches := gpuGresRe.FindStringSubmatch(spec)
		if len(matches) > 2 {
			gpuCount, _ := strconv.ParseFloat(matches[2], 64)
			count += gpuCount
		}
	}
	return count
}

// ParseTotalGPUs parses the output of sinfo command to count total available GPUs
// Expected input format examples:
//   - slurm 20.11.8:  "3 gpu:4"
//   - slurm 21.08.5:  "1 gpu:8(S:0-1)"
//   - slurm 21.08.5:  "13 gpu:A30:4(S:0-1),gpu:Q6K:40(S:0-1)"
func ParseTotalGPUs(data []byte) float64 {
	var numGPUs = 0.0
	sinfoLines := string(data)

	if len(sinfoLines) > 0 {
		for _, line := range strings.Split(sinfoLines, "\n") {
			if len(line) > 0 && strings.Contains(line, "gpu:") {
				fields := strings.Fields(line)
				if len(fields) < 2 {
					continue
				}

				numNodes, _ := strconv.ParseFloat(fields[0], 64)
				nodeGPUs := parseGPUCount(fields[1])
				numGPUs += numNodes * nodeGPUs
			}
		}
	}

	return numGPUs
}

// availableGPUStates lists the node base states whose GPUs are schedulable and
// therefore counted toward alloc/idle. Every other state (down, drained,
// draining, reserved, ...) has its GPUs bucketed into "other". This is the set
// sinfo's own "--states=idle,allocated" filter selected before issue #145
// merged the three calls into one snapshot. "mixed" is included because Slurm
// reports a partially-allocated node as MIXED and its used GPUs still count as
// allocated (see the Slurm node-state definitions).
var availableGPUStates = map[string]bool{
	"idle":      true,
	"allocated": true,
	"mixed":     true,
}

// baseGPUState strips the flag suffixes sinfo appends to a StateLong token
// (e.g. "mixed-", "drained*", "allocated+DRAIN") down to the leading state
// word, so the availability lookup matches sinfo's base-state semantics.
func baseGPUState(state string) string {
	state = strings.ToLower(state)
	for i := 0; i < len(state); i++ {
		if state[i] < 'a' || state[i] > 'z' {
			return state[:i]
		}
	}
	return state
}

// isAvailableGPUState reports whether a node in the given StateLong contributes
// its GPUs to the alloc/idle buckets rather than to "other".
func isAvailableGPUState(state string) bool {
	return availableGPUStates[baseGPUState(state)]
}

// splitGPUViews projects one consolidated
//
//	"<nodes> <StateLong> <Gres> <GresUsed>"
//
// snapshot into the three column layouts the separate sinfo calls used to
// return, so the version-tested GRES parsers keep validating the counts:
//
//	total: "<nodes> <Gres>"            for every node
//	alloc: "<nodes> <GresUsed>"        for available nodes only
//	idle:  "<nodes> <Gres> <GresUsed>" for available nodes only
//
// Deriving all three from a single snapshot removes the alloc/total race
// described in issue #145.
func splitGPUViews(data []byte) (total, alloc, idle []byte) {
	var totalView, allocView, idleView strings.Builder
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		nodes, state, gres, gresUsed := fields[0], fields[1], fields[2], fields[3]

		totalView.WriteString(nodes + " " + gres + "\n")
		if isAvailableGPUState(state) {
			allocView.WriteString(nodes + " " + gresUsed + "\n")
			idleView.WriteString(nodes + " " + gres + " " + gresUsed + "\n")
		}
	}
	return []byte(totalView.String()), []byte(allocView.String()), []byte(idleView.String())
}

// computeGPUsFromSnapshot derives every GPU metric from a single consolidated
// sinfo snapshot. Because total, alloc and idle come from the same instant, two
// invariants hold by construction — no clamp is needed (issue #145 removed the
// clamp issue #16 had added to mask the multi-call race):
//
//	GresUsed never exceeds the configured Gres per node, so
//	    alloc = Σ GresUsed(available) ≤ Σ Gres(available) ≤ total  ⇒ util ≤ 1
//	    other = total − alloc − idle  = Σ Gres(non-available)      ≥ 0
func computeGPUsFromSnapshot(data []byte) GPUsMetrics {
	totalView, allocView, idleView := splitGPUViews(data)

	var gm GPUsMetrics
	gm.total = ParseTotalGPUs(totalView)
	gm.alloc = ParseAllocatedGPUs(allocView)
	gm.idle = ParseIdleGPUs(idleView)
	gm.other = gm.total - gm.alloc - gm.idle
	if gm.total > 0 {
		gm.utilization = gm.alloc / gm.total
	}
	return gm
}

// ParseGPUsMetrics collects and parses all GPU metrics from a single sinfo
// snapshot.
func ParseGPUsMetrics(logger *logger.Logger) (*GPUsMetrics, error) {
	data, err := GPUsSnapshotData(logger)
	if err != nil {
		return nil, err
	}
	gm := computeGPUsFromSnapshot(data)
	return &gm, nil
}

// GPUsSnapshotData executes a single sinfo call that carries the node count,
// state, total GRES and used GRES together, so every GPU metric derives from
// one consistent snapshot (issue #145). The trailing ":" on each field forces
// variable column widths; fixed widths silently truncate rich GRES specs on
// busy GPU nodes (multi-type GPUs, MIG slices) — see issue #10.
func GPUsSnapshotData(logger *logger.Logger) ([]byte, error) {
	args := []string{"-a", "-h", "--Format=Nodes: ,StateLong: ,Gres: ,GresUsed:"}
	return Execute(logger, "sinfo", args)
}

// NewGPUsCollector creates a new GPU metrics collector
func NewGPUsCollector(logger *logger.Logger) *GPUsCollector {
	return &GPUsCollector{
		alloc:       prometheus.NewDesc("slurm_gpus_alloc", "Allocated GPUs", nil, nil),
		idle:        prometheus.NewDesc("slurm_gpus_idle", "Idle GPUs", nil, nil),
		other:       prometheus.NewDesc("slurm_gpus_other", "Other GPUs", nil, nil),
		total:       prometheus.NewDesc("slurm_gpus_total", "Total GPUs", nil, nil),
		utilization: prometheus.NewDesc("slurm_gpus_utilization", "Total GPU utilization", nil, nil),
		logger:      logger,
	}
}

// GPUsCollector implements the Prometheus Collector interface for GPU metrics
type GPUsCollector struct {
	alloc       *prometheus.Desc
	idle        *prometheus.Desc
	other       *prometheus.Desc
	total       *prometheus.Desc
	utilization *prometheus.Desc
	logger      *logger.Logger
}

// Describe sends the descriptors of each metric over to the provided channel
func (cc *GPUsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- cc.alloc
	ch <- cc.idle
	ch <- cc.other
	ch <- cc.total
	ch <- cc.utilization
}

// Collect fetches the GPU metrics from Slurm and sends them to Prometheus
func (cc *GPUsCollector) Collect(ch chan<- prometheus.Metric) { _ = cc.tryCollect(ch) }

func (cc *GPUsCollector) tryCollect(ch chan<- prometheus.Metric) error {
	metrics, err := GPUsGetMetrics(cc.logger)
	if err != nil {
		cc.logger.Error("Failed to get GPU metrics", "err", err)
		return err
	}

	ch <- prometheus.MustNewConstMetric(cc.alloc, prometheus.GaugeValue, metrics.alloc)
	ch <- prometheus.MustNewConstMetric(cc.idle, prometheus.GaugeValue, metrics.idle)
	ch <- prometheus.MustNewConstMetric(cc.other, prometheus.GaugeValue, metrics.other)
	ch <- prometheus.MustNewConstMetric(cc.total, prometheus.GaugeValue, metrics.total)
	ch <- prometheus.MustNewConstMetric(cc.utilization, prometheus.GaugeValue, metrics.utilization)

	return nil
}
