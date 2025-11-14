package collector

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sckyzo/slurm_exporter/internal/logger"
)

const (
	reservationsSubsystem = "reservations"
	slurmTimeLayout       = "2006-01-02T15:04:05"
)

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
func (c *ReservationsCollector) Collect(ch chan<- prometheus.Metric) {
	data, err := c.reservationsData()
	if err != nil {
		c.logger.Error("Failed to fetch reservation data", "err", err)
		return
	}

	reservations, err := parseReservations(data)
	if err != nil {
		c.logger.Error("Failed to parse reservation data", "err", err)
		return
	}

	for _, res := range reservations {
		labels := []string{res.Name, res.State, res.Users, res.Nodes, res.Partition, res.Flags}
		ch <- prometheus.MustNewConstMetric(c.info, prometheus.GaugeValue, 1, labels...)
		ch <- prometheus.MustNewConstMetric(c.startTime, prometheus.GaugeValue, float64(res.StartTime.Unix()), res.Name)
		ch <- prometheus.MustNewConstMetric(c.endTime, prometheus.GaugeValue, float64(res.EndTime.Unix()), res.Name)
		ch <- prometheus.MustNewConstMetric(c.nodeCount, prometheus.GaugeValue, res.NodeCount, res.Name)
		ch <- prometheus.MustNewConstMetric(c.coreCount, prometheus.GaugeValue, res.CoreCount, res.Name)
	}
}

/*
reservationsData executes the scontrol command to retrieve reservation information.
Expected scontrol output format: key=value pairs for each reservation, separated by blank lines.
*/
func (c *ReservationsCollector) reservationsData() ([]byte, error) {
	return Execute(c.logger, "scontrol", []string{"show", "reservation"})
}

/*
parseReservations parses the output of the scontrol show reservation command.
It expects input as a series of key=value pairs for each reservation, separated by blank lines.
*/
func parseReservations(data []byte) ([]ReservationInfo, error) {
	var reservations []ReservationInfo
	// Slurm output is a set of records separated by a blank line.
	records := strings.Split(string(data), "\n\n")

	for _, record := range records {
		if strings.TrimSpace(record) == "" {
			continue
		}

		res := ReservationInfo{}
		// Use a regex to find all key=value pairs.
		re := regexp.MustCompile(`(\w+)=([^ \n]+)`)
		matches := re.FindAllStringSubmatch(record, -1)

		for _, match := range matches {
			key, value := match[1], match[2]
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
				res.StartTime, _ = time.Parse(slurmTimeLayout, value)
			case "EndTime":
				res.EndTime, _ = time.Parse(slurmTimeLayout, value)
			}
		}
		reservations = append(reservations, res)
	}
	return reservations, nil
}
