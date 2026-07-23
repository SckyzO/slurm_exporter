package collector

import (
	"regexp"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// Pre-compiled regexes for parsing scontrol show nodes -o output.
var (
	resvNodeNameRe  = regexp.MustCompile(`NodeName=(\S+)`)
	resvNodeStateRe = regexp.MustCompile(`State=(\S+)`)
	resvNodeResvRe  = regexp.MustCompile(`ReservationName=(\S+)`)
)

// ReservationNodesMetrics holds per-state node counts for a single reservation.
type ReservationNodesMetrics struct {
	alloc   float64
	idle    float64
	mix     float64
	down    float64
	drain   float64
	planned float64
	other   float64
	// healthy counts nodes whose base state is up (alloc/idle/mix) and that are
	// not drained. DRAIN and DOWN nodes are excluded even inside a reservation.
	healthy float64
}

// ParseReservationNodesMetrics parses "scontrol show nodes -o" output and returns
// per-reservation node state counts. Only nodes with a ReservationName field are
// included.
//
// A node state is compound: a single base state followed by any number of flags,
// e.g. MIXED+DRAIN+MAINTENANCE+RESERVED. The base state (before the first '+')
// picks the primary bucket (alloc/idle/mix/down/other); DRAIN and PLANNED are
// flags, counted in their own buckets on top of the base one, so a MIXED+DRAIN
// node lands in both mix and drain. Reading only the head of the string missed
// them entirely and let drained nodes count as healthy (issue #142).
func ParseReservationNodesMetrics(input []byte) map[string]*ReservationNodesMetrics {
	reservations := make(map[string]*ReservationNodesMetrics)

	for line := range strings.SplitSeq(string(input), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		resvMatches := resvNodeResvRe.FindStringSubmatch(line)
		if resvMatches == nil {
			continue // node has no reservation
		}
		resvName := resvMatches[1]

		stateMatches := resvNodeStateRe.FindStringSubmatch(line)
		if stateMatches == nil {
			continue
		}

		// Split the compound state into base + flags. A trailing '*' marks a
		// non-responding node and is irrelevant to these counts, so strip it.
		raw := strings.ReplaceAll(strings.ToUpper(stateMatches[1]), "*", "")
		parts := strings.Split(raw, "+")
		base, flags := parts[0], parts[1:]

		hasFlag := func(name string) bool {
			for _, f := range flags {
				if f == name {
					return true
				}
			}
			return false
		}
		drained := hasFlag("DRAIN")

		if _, exists := reservations[resvName]; !exists {
			reservations[resvName] = &ReservationNodesMetrics{}
		}
		rm := reservations[resvName]

		// Base state picks the primary bucket. upBase marks the states that are
		// otherwise usable and thus eligible for the healthy count.
		upBase := false
		switch {
		case strings.HasPrefix(base, "ALLOC"):
			rm.alloc++
			upBase = true
		case strings.HasPrefix(base, "IDLE"):
			rm.idle++
			upBase = true
		case strings.HasPrefix(base, "MIX"):
			rm.mix++
			upBase = true
		case strings.HasPrefix(base, "DOWN"):
			rm.down++
		default:
			rm.other++
		}

		// Flags are orthogonal to the base state: a node can be both MIXED and
		// DRAIN, or IDLE and PLANNED.
		if drained {
			rm.drain++
		}
		if hasFlag("PLANNED") {
			rm.planned++
		}

		// A drained node is not usable even if its base state is up.
		if upBase && !drained {
			rm.healthy++
		}
	}
	return reservations
}

// ReservationNodesData returns the output of scontrol show nodes -o.
// Uses scontrolNodesCache so that when both the nodes and reservation_nodes
// collectors run in the same scrape cycle, the scontrol RPC is only sent once.
func ReservationNodesData(log *logger.Logger) ([]byte, error) {
	data, err := scontrolNodesCache.GetOrFetch(func() ([]byte, error) {
		return Execute(log, "scontrol", []string{"show", "nodes", "-o"})
	})
	updateCacheAge()
	return data, err
}

// ReservationNodesGetMetrics fetches and parses per-reservation node state metrics.
func ReservationNodesGetMetrics(logger *logger.Logger) (map[string]*ReservationNodesMetrics, error) {
	data, err := ReservationNodesData(logger)
	if err != nil {
		return nil, err
	}
	return ParseReservationNodesMetrics(data), nil
}

// ReservationNodesCollector implements the Prometheus Collector interface for
// per-reservation node state metrics.
type ReservationNodesCollector struct {
	alloc   *prometheus.Desc
	idle    *prometheus.Desc
	mix     *prometheus.Desc
	down    *prometheus.Desc
	drain   *prometheus.Desc
	planned *prometheus.Desc
	other   *prometheus.Desc
	healthy *prometheus.Desc
	logger  *logger.Logger
}

// NewReservationNodesCollector creates a collector for per-reservation node state metrics.
func NewReservationNodesCollector(logger *logger.Logger) *ReservationNodesCollector {
	labels := []string{"reservation"}
	return &ReservationNodesCollector{
		alloc:   prometheus.NewDesc("slurm_reservation_nodes_alloc", "Allocated nodes in reservation", labels, nil),
		idle:    prometheus.NewDesc("slurm_reservation_nodes_idle", "Idle nodes in reservation", labels, nil),
		mix:     prometheus.NewDesc("slurm_reservation_nodes_mix", "Mixed nodes in reservation", labels, nil),
		down:    prometheus.NewDesc("slurm_reservation_nodes_down", "Down nodes in reservation", labels, nil),
		drain:   prometheus.NewDesc("slurm_reservation_nodes_drain", "Drained nodes in reservation", labels, nil),
		planned: prometheus.NewDesc("slurm_reservation_nodes_planned", "Planned nodes in reservation", labels, nil),
		other:   prometheus.NewDesc("slurm_reservation_nodes_other", "Nodes in other states in reservation", labels, nil),
		healthy: prometheus.NewDesc("slurm_reservation_nodes_healthy", "Healthy nodes in reservation (up base state, not drained)", labels, nil),
		logger:  logger,
	}
}

func (rnc *ReservationNodesCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- rnc.alloc
	ch <- rnc.idle
	ch <- rnc.mix
	ch <- rnc.down
	ch <- rnc.drain
	ch <- rnc.planned
	ch <- rnc.other
	ch <- rnc.healthy
}

func (rnc *ReservationNodesCollector) Collect(ch chan<- prometheus.Metric) { _ = rnc.tryCollect(ch) }

func (rnc *ReservationNodesCollector) tryCollect(ch chan<- prometheus.Metric) error {
	metrics, err := ReservationNodesGetMetrics(rnc.logger)
	if err != nil {
		rnc.logger.Error("Failed to get reservation nodes metrics", "err", err)
		return err
	}
	for resvName, rm := range metrics {
		ch <- prometheus.MustNewConstMetric(rnc.alloc, prometheus.GaugeValue, rm.alloc, resvName)
		ch <- prometheus.MustNewConstMetric(rnc.idle, prometheus.GaugeValue, rm.idle, resvName)
		ch <- prometheus.MustNewConstMetric(rnc.mix, prometheus.GaugeValue, rm.mix, resvName)
		ch <- prometheus.MustNewConstMetric(rnc.down, prometheus.GaugeValue, rm.down, resvName)
		ch <- prometheus.MustNewConstMetric(rnc.drain, prometheus.GaugeValue, rm.drain, resvName)
		ch <- prometheus.MustNewConstMetric(rnc.planned, prometheus.GaugeValue, rm.planned, resvName)
		ch <- prometheus.MustNewConstMetric(rnc.other, prometheus.GaugeValue, rm.other, resvName)
		ch <- prometheus.MustNewConstMetric(rnc.healthy, prometheus.GaugeValue, rm.healthy, resvName)
	}

	return nil
}
