/* Copyright 2017-2020 Victor Penso, Matteo Dessalvi, Joeri Hermans

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
	"net/http"
	"os"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	webflag "github.com/prometheus/exporter-toolkit/web/kingpinflag"

	"github.com/mike8161990/slurm_exporter/internal/collector"
	"github.com/mike8161990/slurm_exporter/internal/logger"
)

var (
	// Command-line flags for application configuration
	commandTimeout = kingpin.Flag("command.timeout", "Timeout for executing Slurm commands.").Default("5s").Duration()
	logLevel       = kingpin.Flag("log.level", "Only log messages with the given severity or above. One of: [debug, info, warn, error]").Default("info").Enum("debug", "info", "warn", "error")
	logFormat      = kingpin.Flag("log.format", "Log format. One of: [json, text]").Default("text").Enum("json", "text")
	toolkitFlags   = webflag.AddFlags(kingpin.CommandLine, ":9341")

	// collectorState stores the enabled/disabled state of each collector
	collectorState = make(map[string]*bool)
)

// collectorConstructors maps collector names to their constructor functions
var collectorConstructors = map[string]func(logger *logger.Logger) prometheus.Collector{
	"accounts":     func(l *logger.Logger) prometheus.Collector { return collector.NewAccountsCollector(l) },
	"cpus":         func(l *logger.Logger) prometheus.Collector { return collector.NewCPUsCollector(l) },
	"nodes":        func(l *logger.Logger) prometheus.Collector { return collector.NewNodesCollector(l) },
	"node":         func(l *logger.Logger) prometheus.Collector { return collector.NewNodeCollector(l) },
	"partitions":   func(l *logger.Logger) prometheus.Collector { return collector.NewPartitionsCollector(l) },
	"queue":        func(l *logger.Logger) prometheus.Collector { return collector.NewQueueCollector(l) },
	"scheduler":    func(l *logger.Logger) prometheus.Collector { return collector.NewSchedulerCollector(l) },
	"fairshare":    func(l *logger.Logger) prometheus.Collector { return collector.NewFairShareCollector(l) },
	"users":        func(l *logger.Logger) prometheus.Collector { return collector.NewUsersCollector(l) },
	"info":         func(l *logger.Logger) prometheus.Collector { return collector.NewSlurmInfoCollector(l) },
	"gpus":         func(l *logger.Logger) prometheus.Collector { return collector.NewGPUsCollector(l) },
	"reservations": func(l *logger.Logger) prometheus.Collector { return collector.NewReservationsCollector(l) },
	"licenses":     func(l *logger.Logger) prometheus.Collector { return collector.NewLicensesCollector(l) },
}

// indexHTML is the HTML content displayed on the root page
const indexHTML = `<html>
	<head><title>Slurm Exporter</title></head>
	<body>
		<h1>Slurm Exporter</h1>
		<p>Welcome to the Slurm Exporter. Click <a href='/metrics'>here</a> to see the metrics.</p>
	</body>
</html>`

// registerCollectors registers enabled collectors with Prometheus
func registerCollectors(logger *logger.Logger) {
	for name, constructor := range collectorConstructors {
		if *collectorState[name] {
			prometheus.MustRegister(constructor(logger))
			logger.Info("Collector enabled", "collector", name)
		} else {
			logger.Info("Collector disabled", "collector", name)
		}
	}
}

func main() {
	// Dynamically create command-line flags for each collector
	for name := range collectorConstructors {
		collectorState[name] = kingpin.Flag("collector."+name, "Enable the "+name+" collector.").Default("true").Bool()
	}

	// Configure kingpin command-line parser
	kingpin.Version(version.Print("slurm_exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	// Initialize logger based on configured format and level
	var log *logger.Logger
	if *logFormat == "json" {
		log = logger.NewJSONLogger(*logLevel)
	} else {
		log = logger.NewTextLogger(*logLevel)
	}

	// Configure global command timeout for all collectors
	collector.SetCommandTimeout(*commandTimeout)

	// Register Prometheus build info collector
	prometheus.MustRegister(collectors.NewBuildInfoCollector())

	// Register enabled Slurm collectors
	registerCollectors(log)

	// Log server startup information
	log.Info("Starting Slurm Exporter server...")
	log.Info("Command timeout configured", "timeout", *commandTimeout)

	// Configure HTTP routes
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(indexHTML))
	})
	http.Handle("/metrics", promhttp.Handler())

	// Start HTTP server with exporter toolkit (supports TLS, Basic Auth, etc.)
	server := &http.Server{}
	if err := web.ListenAndServe(server, toolkitFlags, log); err != nil {
		log.Error("Failed to start HTTP server", "err", err)
		os.Exit(1)
	}
}
