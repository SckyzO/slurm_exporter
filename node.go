/* Copyright 2021 Chris Read

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>. */

package main

import (
	"log"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// NodeMetrics stores metrics for each node
type NodeMetrics struct {
	memAlloc   uint64
	memTotal   uint64
	cpuAlloc   uint64
	cpuIdle    uint64
	cpuOther   uint64
	cpuTotal   uint64
	nodeStatus string
	partitions []string
}

func NodeGetMetrics() map[string]*NodeMetrics {
	return ParseNodeMetrics(NodeData())
}

// ParseNodeMetrics takes the output of sinfo with node data
// It returns a map of metrics per node, including partitions
func ParseNodeMetrics(input []byte) map[string]*NodeMetrics {
	nodes := make(map[string]*NodeMetrics)
	lines := strings.Split(string(input), "\n")

	// Sort and remove all the duplicates from the 'sinfo' output
	sort.Strings(lines)
	linesUniq := RemoveDuplicates(lines)

	for _, line := range linesUniq {
		node := strings.Fields(line)
		if len(node) < 6 {
			continue
		}
		nodeName := node[0]
		nodeStatus := node[4] // mixed, allocated, etc.
		partition := node[5]  // Partition name

		// Create new node metrics if it doesn't exist
		if _, exists := nodes[nodeName]; !exists {
			nodes[nodeName] = &NodeMetrics{0, 0, 0, 0, 0, 0, nodeStatus, []string{}}
		}

		memAlloc, _ := strconv.ParseUint(node[1], 10, 64)
		memTotal, _ := strconv.ParseUint(node[2], 10, 64)

		cpuInfo := strings.Split(node[3], "/")
		cpuAlloc, _ := strconv.ParseUint(cpuInfo[0], 10, 64)
		cpuIdle, _ := strconv.ParseUint(cpuInfo[1], 10, 64)
		cpuOther, _ := strconv.ParseUint(cpuInfo[2], 10, 64)
		cpuTotal, _ := strconv.ParseUint(cpuInfo[3], 10, 64)

		nodes[nodeName].memAlloc = memAlloc
		nodes[nodeName].memTotal = memTotal
		nodes[nodeName].cpuAlloc = cpuAlloc
		nodes[nodeName].cpuIdle = cpuIdle
		nodes[nodeName].cpuOther = cpuOther
		nodes[nodeName].cpuTotal = cpuTotal

		// Add the partition if it's not already in the list
		nodes[nodeName].partitions = appendUnique(nodes[nodeName].partitions, partition)
	}

	return nodes
}

// NodeData executes the sinfo command to get data for each node
// It returns the output of the sinfo command
func NodeData() []byte {
	cmd := exec.Command("sinfo", "-h", "-N", "-O", "NodeList,AllocMem,Memory,CPUsState,StateLong,Partition")
	out, err := cmd.Output()
	if err != nil {
		log.Fatal(err)
	}
	return out
}

type NodeCollector struct {
	cpuAlloc   *prometheus.Desc
	cpuIdle    *prometheus.Desc
	cpuOther   *prometheus.Desc
	cpuTotal   *prometheus.Desc
	memAlloc   *prometheus.Desc
	memTotal   *prometheus.Desc
	nodeStatus *prometheus.Desc
}

// NewNodeCollector creates a Prometheus collector to keep all our stats in
// It returns a set of collections for consumption
func NewNodeCollector() *NodeCollector {
	labels := []string{"node", "status", "partition"}
	return &NodeCollector{
		cpuAlloc:   prometheus.NewDesc("slurm_node_cpu_alloc", "Allocated CPUs per node", labels, nil),
		cpuIdle:    prometheus.NewDesc("slurm_node_cpu_idle", "Idle CPUs per node", labels, nil),
		cpuOther:   prometheus.NewDesc("slurm_node_cpu_other", "Other CPUs per node", labels, nil),
		cpuTotal:   prometheus.NewDesc("slurm_node_cpu_total", "Total CPUs per node", labels, nil),
		memAlloc:   prometheus.NewDesc("slurm_node_mem_alloc", "Allocated memory per node", labels, nil),
		memTotal:   prometheus.NewDesc("slurm_node_mem_total", "Total memory per node", labels, nil),
		nodeStatus: prometheus.NewDesc("slurm_node_status", "Node Status with partition", labels, nil),
	}
}

// Send all metric descriptions
func (nc *NodeCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- nc.cpuAlloc
	ch <- nc.cpuIdle
	ch <- nc.cpuOther
	ch <- nc.cpuTotal
	ch <- nc.memAlloc
	ch <- nc.memTotal
	ch <- nc.nodeStatus
}

func (nc *NodeCollector) Collect(ch chan<- prometheus.Metric) {
	nodes := NodeGetMetrics()
	for node, metrics := range nodes {
		for _, partition := range metrics.partitions {
			ch <- prometheus.MustNewConstMetric(nc.cpuAlloc, prometheus.GaugeValue, float64(metrics.cpuAlloc), node, metrics.nodeStatus, partition)
			ch <- prometheus.MustNewConstMetric(nc.cpuIdle, prometheus.GaugeValue, float64(metrics.cpuIdle), node, metrics.nodeStatus, partition)
			ch <- prometheus.MustNewConstMetric(nc.cpuOther, prometheus.GaugeValue, float64(metrics.cpuOther), node, metrics.nodeStatus, partition)
			ch <- prometheus.MustNewConstMetric(nc.cpuTotal, prometheus.GaugeValue, float64(metrics.cpuTotal), node, metrics.nodeStatus, partition)
			ch <- prometheus.MustNewConstMetric(nc.memAlloc, prometheus.GaugeValue, float64(metrics.memAlloc), node, metrics.nodeStatus, partition)
			ch <- prometheus.MustNewConstMetric(nc.memTotal, prometheus.GaugeValue, float64(metrics.memTotal), node, metrics.nodeStatus, partition)
			ch <- prometheus.MustNewConstMetric(nc.nodeStatus, prometheus.GaugeValue, 1, node, metrics.nodeStatus, partition)
		}
	}
}

// appendUnique adds a string to a slice if it doesn't already exist
func appendUnique(slice []string, value string) []string {
	for _, v := range slice {
		if v == value {
			return slice
		}
}
	return append(slice, value)
}
