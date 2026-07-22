package collector

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// reservationKVRe matches key=value pairs in scontrol show reservation output.
var reservationKVRe = regexp.MustCompile(`(\w+)=([^ \n]+)`)

const slurmTimeLayout = "2006-01-02T15:04:05"

// ReservationInfo holds information about a single reservation.
type ReservationInfo struct {
	Name      string
	State     string
	Users     string
	Nodes     string
	Partition string
	Flags     string
	NodeCount float64
	CoreCount float64
	StartTime time.Time
	EndTime   time.Time
	// StartTimeRaw and EndTimeRaw hold the fields exactly as scontrol printed
	// them, so an unreadable value can be named in a log line. A zero time
	// with a non-empty raw field is a parse failure; both empty means scontrol
	// never printed the field.
	StartTimeRaw string
	EndTimeRaw   string
}

// ReservationsCollector collects metrics about Slurm reservations.
type ReservationsCollector struct {
	logger    *logger.Logger
	info      *prometheus.Desc
	startTime *prometheus.Desc
	endTime   *prometheus.Desc
	nodeCount *prometheus.Desc
	coreCount *prometheus.Desc
}

func NewReservationsCollector(logger *logger.Logger) *ReservationsCollector {
	labels := []string{"reservation_name", "state", "users", "nodes", "partition", "flags"}
	return &ReservationsCollector{
		logger: logger,
		info: prometheus.NewDesc(
			"slurm_reservation_info",
			"A metric with a constant '1' value labeled by reservation name, state, users, nodes, partition, and flags.",
			labels, nil,
		),
		startTime: prometheus.NewDesc(
			"slurm_reservation_start_time_seconds",
			"The start time of the reservation in seconds since the Unix epoch.",
			[]string{"reservation_name"}, nil,
		),
		endTime: prometheus.NewDesc(
			"slurm_reservation_end_time_seconds",
			"The end time of the reservation in seconds since the Unix epoch.",
			[]string{"reservation_name"}, nil,
		),
		nodeCount: prometheus.NewDesc(
			"slurm_reservation_node_count",
			"The number of nodes allocated to the reservation.",
			[]string{"reservation_name"}, nil,
		),
		coreCount: prometheus.NewDesc(
			"slurm_reservation_core_count",
			"The number of cores allocated to the reservation.",
			[]string{"reservation_name"}, nil,
		),
	}
}

// Describe sends the super-set of all possible descriptors of metrics
// collected by this Collector to the provided channel.
func (c *ReservationsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.info
	ch <- c.startTime
	ch <- c.endTime
	ch <- c.nodeCount
	ch <- c.coreCount
}

// Collect is called by the Prometheus registry when collecting metrics.
func (c *ReservationsCollector) Collect(ch chan<- prometheus.Metric) { _ = c.tryCollect(ch) }

func (c *ReservationsCollector) tryCollect(ch chan<- prometheus.Metric) error {
	data, err := c.reservationsData()
	if err != nil {
		c.logger.Error("Failed to fetch reservation data", "err", err)
		return err
	}

	reservations, err := parseReservations(data)
	if err != nil {
		c.logger.Error("Failed to parse reservation data", "err", err)
		return err
	}

	for i := range reservations {
		res := &reservations[i]
		labels := []string{res.Name, res.State, res.Users, res.Nodes, res.Partition, res.Flags}
		ch <- prometheus.MustNewConstMetric(c.info, prometheus.GaugeValue, 1, labels...)
		c.emitTime(ch, c.startTime, res.Name, "StartTime", res.StartTime, res.StartTimeRaw)
		c.emitTime(ch, c.endTime, res.Name, "EndTime", res.EndTime, res.EndTimeRaw)
		ch <- prometheus.MustNewConstMetric(c.nodeCount, prometheus.GaugeValue, res.NodeCount, res.Name)
		ch <- prometheus.MustNewConstMetric(c.coreCount, prometheus.GaugeValue, res.CoreCount, res.Name)
	}

	return nil
}

// emitTime publishes one reservation timestamp, or nothing when scontrol printed
// a value slurmTimeLayout rejects.
//
// The zero time.Time was published as -62135596800, which places the reservation
// in year 1 and reads as data: dashboards subtract it, thresholds compare it, and
// nothing marks it invalid. An absent series is the only answer a consumer can
// tell apart from a measurement. The WARN carries the raw value because Execute
// pins SLURM_TIME_FORMAT, so a value that still fails to parse is a layout the
// exporter does not know, and the raw string is what identifies it. See issue
// #158.
func (c *ReservationsCollector) emitTime(ch chan<- prometheus.Metric, desc *prometheus.Desc, name, field string, t time.Time, raw string) {
	if !t.IsZero() {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, float64(t.Unix()), name)
		return
	}
	if raw == "" {
		return // scontrol did not print the field at all
	}
	c.logger.Warn("Unreadable reservation timestamp from scontrol; the metric is omitted for this reservation",
		"reservation", name, "field", field, "value", raw, "expected_layout", slurmTimeLayout,
		"hint", "the exporter pins SLURM_TIME_FORMAT=standard, so this layout is one it does not know: please report it with your Slurm version")
}

/*
reservationsData executes the scontrol command to retrieve reservation information.
Expected scontrol output format: key=value pairs for each reservation, separated by blank lines.
*/
func (c *ReservationsCollector) reservationsData() ([]byte, error) {
	return Execute(c.logger, "scontrol", []string{"show", "reservation"})
}

// setReservationField assigns one parsed key=value pair into res.
// Unknown keys are silently ignored.
func setReservationField(res *ReservationInfo, key, value string) {
	switch key {
	case "ReservationName":
		res.Name = value
	case "State":
		res.State = value
	case "Users":
		res.Users = value
	case "Nodes":
		res.Nodes = value
	case "PartitionName":
		if value == "(null)" {
			res.Partition = ""
		} else {
			res.Partition = value
		}
	case "Flags":
		res.Flags = value
	case "NodeCnt":
		res.NodeCount, _ = strconv.ParseFloat(value, 64)
	case "CoreCnt":
		res.CoreCount, _ = strconv.ParseFloat(value, 64)
	case "StartTime":
		res.StartTimeRaw = value
		res.StartTime = parseSlurmTime(value)
	case "EndTime":
		res.EndTimeRaw = value
		res.EndTime = parseSlurmTime(value)
	}
}

// parseSlurmTime reads one scontrol timestamp field. A value slurmTimeLayout
// rejects yields the zero time.Time, which the collector reads as "do not
// publish" rather than as a date in year 1. See issue #158.
func parseSlurmTime(value string) time.Time {
	t, err := time.ParseInLocation(slurmTimeLayout, value, time.Local)
	if err != nil {
		return time.Time{}
	}
	return t
}

// parseReservations parses the output of `scontrol show reservation`. Records
// are separated by blank lines, each record is a sequence of key=value pairs.
func parseReservations(data []byte) ([]ReservationInfo, error) {
	var reservations []ReservationInfo
	for _, record := range strings.Split(string(data), "\n\n") {
		if strings.TrimSpace(record) == "" {
			continue
		}
		res := ReservationInfo{}
		for _, match := range reservationKVRe.FindAllStringSubmatch(record, -1) {
			setReservationField(&res, match[1], match[2])
		}
		// Skip records that didn't yield a real reservation. scontrol prints
		// "No reservations in the system" on an empty cluster; without this
		// guard, the parser would emit a phantom ReservationInfo with an
		// empty Name and time.Time{} timestamps (Unix = -62135596800 = year
		// 0001), surfacing as fake "1968-01-12" reservations on dashboards.
		// See https://github.com/SckyzO/slurm_exporter/issues/26.
		if res.Name == "" {
			continue
		}
		reservations = append(reservations, res)
	}
	return reservations, nil
}
