// Command gen-metric-surface writes the exporter's metric surface to a
// document, and with -check fails when the document on disk is stale.
//
// The surface is every metric name and its variable labels, per collector,
// taken from Describe(). It is the artefact a non-regression check reads when
// the change under review cannot be expressed as a red-then-green test: if a
// toolchain bump, a dependency bump or a refactor leaves this file untouched,
// nothing an operator can scrape has moved.
//
// It also answers the question docs/metrics.md answers by hand and can get
// wrong: a scrape shows only what the cluster happened to have, so a metric
// that was deleted and a metric that had nothing to report look identical.
// Describe() distinguishes them.
//
// Usage:
//
//	go run ./tools/gen-metric-surface -out docs/metric-surface.md
//	go run ./tools/gen-metric-surface -check
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"

	"github.com/sckyzo/slurm_exporter/internal/collector"
)

func main() {
	out := flag.String("out", "docs/metric-surface.md", "path of the document to write")
	check := flag.Bool("check", false, "exit non-zero if the file on disk is stale, write nothing")
	flag.Parse()

	want, err := collector.RenderMetricSurface()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gen-metric-surface: %v\n", err)
		os.Exit(1)
	}

	if *check {
		got, readErr := os.ReadFile(*out)
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "gen-metric-surface: %v\n", readErr)
			os.Exit(1)
		}
		if !bytes.Equal(got, want) {
			fmt.Fprintf(os.Stderr,
				"gen-metric-surface: %s is stale.\n"+
					"The set of metrics or labels the collectors declare has changed.\n"+
					"Run `make generate` and commit the result, so the change is visible in review.\n", *out)
			os.Exit(1)
		}
		return
	}

	if err := os.WriteFile(*out, want, 0o644); err != nil { //nolint:gosec // G306: a generated doc is world-readable on purpose
		fmt.Fprintf(os.Stderr, "gen-metric-surface: %v\n", err)
		os.Exit(1)
	}
}
