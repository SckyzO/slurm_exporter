package collector

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sckyzo/slurm_exporter/v2/internal/logger"
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
// gresTokenRe matches one comma-separated GRES token: a type, an optional
// model, a count, and an optional socket or index suffix.
//
// It is anchored on purpose. An unanchored pattern latches onto whatever digits
// it can find, so a truncated token invents data instead of rejecting it:
// "shard:h10", the tail of "shard:h100:2" cut off by a fixed-width sinfo
// column, yields a phantom model "h1" with a count of 0. Anchoring makes the
// count have to be the last field, so a mangled token is skipped rather than
// guessed at.
var gresTokenRe = regexp.MustCompile(`^(\w+):(?:(\(null\)|[^:(]*):)?(\d+)(?:\([^)]*\))?$`)

// splitGRESTokens splits a GRES specification on the commas that separate
// resources, ignoring the ones inside an index or socket suffix.
//
// Splitting on every comma looks right until a node exposes non-contiguous GPU
// indices: "gpu:NVlink_A100_40GB:2(IDX:0,3)" is one resource, and a naive split
// turns it into two fragments that parse as nothing. That capture is real, it
// sits in test_data/slurm-25.11.1-1/, and the version matrix is what caught it.
func splitGRESTokens(spec string) []string {
	var tokens []string
	depth, start := 0, 0
	for i, r := range spec {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				tokens = append(tokens, spec[start:i])
				start = i + 1
			}
		}
	}
	return append(tokens, spec[start:])
}

// parseGRESByType breaks a comma-separated GRES specification into counts per
// resource, keyed "type" or "type:model" ("gpu:A100:4,gpu:H100:2" → two
// entries). Untyped resources collapse onto the bare type, and a "(null)" model
// is treated as untyped, which is how Slurm reports a GPU with no model set.
//
// Real formats this has to survive, all captured under test_data/slurm-*/
// across six Slurm releases: "gpu:2", "gpu:h100:4", "gpu:mi210:2(S:0)",
// "gpu:nvidia_a100_1g.5gb:21(S:0-1)" (MIG profile, dots), "gpu:NVlink_A100_40GB:2"
// (mixed case), "gpu:tesla_v100-sxm3-32gb:16(IDX:0-15)" (hyphens),
// "gpu:(null):0(IDX:N/A)", and the "(null)" placeholder for a node with none.
// A nil map is returned when there is nothing to report, and the map is only
// allocated once a token actually parses. This matters on the per-node path:
// sinfo -h -N emits one line per node and partition, and on a mostly-CPU
// cluster the large majority carry "(null)". Allocating for those would mean
// hundreds of throwaway maps per scrape on a real cluster, thousands on a large
// one. Reading and ranging a nil map is legal in Go, so callers see no
// difference.
func parseGRESByType(spec string) map[string]uint64 {
	if spec == "" || spec == "(null)" || spec == "N/A" {
		return nil
	}
	var counts map[string]uint64
	for _, token := range splitGRESTokens(spec) {
		m := gresTokenRe.FindStringSubmatch(strings.TrimSpace(token))
		if m == nil {
			continue
		}
		count, err := strconv.ParseUint(m[3], 10, 64)
		if err != nil {
			continue
		}
		key := m[1]
		if model := m[2]; model != "" && model != "(null)" {
			key += ":" + model
		}
		if counts == nil {
			counts = make(map[string]uint64, 2)
		}
		counts[key] += count
	}
	return counts
}

// parseGPUCount sums the GPU counts across every gpu: entry in a comma-separated
// GRES specification. A node can expose several GPU types at once
// ("gpu:A100:4,gpu:H100:2" → 6), and non-gpu GRES (e.g. "shard:…") is ignored.
// Shared by the cluster-wide (gpus.go) and per-partition (partitions.go) paths,
// and derived from parseGRESByType so the two views cannot disagree on what a
// GRES string means.
func parseGPUCount(gpuSpec string) float64 {
	var count = 0.0
	for key, n := range parseGRESByType(gpuSpec) {
		if key == "gpu" || strings.HasPrefix(key, "gpu:") {
			count += float64(n)
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
