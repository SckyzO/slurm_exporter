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
	// healthy aggregates non-degraded states: alloc + idle + mix + planned.
	healthy float64
}

// ParseReservationNodesMetrics parses "scontrol show nodes -o" output and returns
// per-reservation node state counts. Only nodes with a ReservationName field are
// included. States are compound (e.g. ALLOCATED+MAINTENANCE+RESERVED); the primary
// state (before the first '+') determines the category.
func ParseReservationNodesMetrics(input []byte) map[string]*ReservationNodesMetrics {
	reservations := make(map[string]*ReservationNodesMetrics)

	for _, line := range strings.Split(string(input), "\n") {
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

		// Extract primary state from compound state (e.g. ALLOCATED+MAINTENANCE+RESERVED)
		primaryState := strings.ToLower(strings.SplitN(stateMatches[1], "+", 2)[0])

		if _, exists := reservations[resvName]; !exists {
			reservations[resvName] = &ReservationNodesMetrics{}
		}
		rm := reservations[resvName]

		switch {
		case strings.HasPrefix(primaryState, "alloc"):
			rm.alloc++
			rm.healthy++
		case strings.HasPrefix(primaryState, "idle"):
			rm.idle++
			rm.healthy++
		case strings.HasPrefix(primaryState, "mix"):
			rm.mix++
			rm.healthy++
		case strings.HasPrefix(primaryState, "planned"):
			rm.planned++
			rm.healthy++
		case strings.HasPrefix(primaryState, "down"):
			rm.down++
		case strings.HasPrefix(primaryState, "drain"):
			rm.drain++
		default:
			rm.other++
		}
	}
	return reservations
}

// ReservationNodesData runs scontrol to get all nodes with their state and reservation.
func ReservationNodesData(logger *logger.Logger) ([]byte, error) {
	return Execute(logger, "scontrol", []string{"show", "nodes", "-o"})
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
		healthy: prometheus.NewDesc("slurm_reservation_nodes_healthy", "Healthy nodes in reservation (alloc+idle+mix+planned)", labels, nil),
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

func (rnc *ReservationNodesCollector) Collect(ch chan<- prometheus.Metric) {
	metrics, err := ReservationNodesGetMetrics(rnc.logger)
	if err != nil {
		rnc.logger.Error("Failed to get reservation nodes metrics", "err", err)
		return
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
}
