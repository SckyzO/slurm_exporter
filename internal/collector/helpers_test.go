package collector

import (
	"bytes"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// stubExecute makes every Slurm command return the given output for the rest of
// the test, and puts the real Execute back afterwards.
func stubExecute(t *testing.T, output string) {
	t.Helper()
	old := Execute
	t.Cleanup(func() { Execute = old })
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		return []byte(output), nil
	}
}

// bufferLogger returns a WARN-level logger writing into the returned buffer, so
// a test can assert that a collector reported a problem instead of staying
// silent about it.
func bufferLogger() (*logger.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	return &logger.Logger{Logger: slog.New(handler)}, &buf
}

// unixLocal converts a Slurm timestamp the way the command produced it, in the
// local zone of the host that ran it, which is the exporter host. The layout is
// spelled out rather than read from slurmTimeLayout so that changing the
// production constant surfaces as a test failure.
func unixLocal(t *testing.T, value string) float64 {
	t.Helper()
	ts, err := time.ParseInLocation("2006-01-02T15:04:05", value, time.Local)
	require.NoError(t, err)
	return float64(ts.Unix())
}

// gatheredSeries renders one gauge family as sorted `name{label="value",...} value`
// lines, so a test can assert on exact series identity instead of on a count. A
// family the collector did not publish yields an empty slice, which is how a test
// states that a metric is absent rather than zero.
func gatheredSeries(t *testing.T, c prometheus.Collector, name string) []string {
	t.Helper()
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))
	mfs, err := reg.Gather()
	require.NoError(t, err)

	var series []string
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			pairs := make([]string, 0, len(m.GetLabel()))
			for _, l := range m.GetLabel() {
				pairs = append(pairs, fmt.Sprintf("%s=%q", l.GetName(), l.GetValue()))
			}
			series = append(series, fmt.Sprintf("%s{%s} %g", name, strings.Join(pairs, ","), m.GetGauge().GetValue()))
		}
	}
	sort.Strings(series)
	return series
}
