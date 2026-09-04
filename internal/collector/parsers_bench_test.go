package collector

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Benchmarks for the parse path.
//
// Every Parse* function below runs on each scrape, on output whose size grows
// with the cluster: node_detail emits a line per node per partition, queue a
// line per job. A regression here is paid on every scrape of every deployment,
// and until now nothing measured it — the 2.0 line claims four performance
// changes (#144, #149, #150, #181) with no numbers behind them.
//
// The functions take []byte and return a struct, so the fixtures are the
// natural input. Fixture bytes are read once, outside the measured loop: the
// point is to measure parsing, not os.ReadFile.

func benchInput(b *testing.B, name string) []byte {
	b.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "test_data", name))
	if err != nil {
		b.Skipf("fixture %s unavailable: %v", name, err)
	}
	return raw
}

func BenchmarkParseQueueMetrics(b *testing.B) {
	in := benchInput(b, "queue_all_states.txt")
	b.ReportAllocs()
	for b.Loop() {
		_ = ParseQueueMetrics(in)
	}
}

func BenchmarkParseAccountsMetrics(b *testing.B) {
	in := benchInput(b, "squeue_jobs_accounts_view.txt")
	b.ReportAllocs()
	for b.Loop() {
		_ = ParseAccountsMetrics(in)
	}
}

func BenchmarkParseUsersMetrics(b *testing.B) {
	in := benchInput(b, "squeue_jobs_users_view.txt")
	b.ReportAllocs()
	for b.Loop() {
		_ = ParseUsersMetrics(in)
	}
}

func BenchmarkParseNodeMetrics(b *testing.B) {
	in := benchInput(b, "node_detail.txt")
	b.ReportAllocs()
	for b.Loop() {
		_ = ParseNodeMetrics(in)
	}
}

func BenchmarkParseCPUsMetrics(b *testing.B) {
	in := benchInput(b, "cpus.txt")
	b.ReportAllocs()
	for b.Loop() {
		_ = ParseCPUsMetrics(in)
	}
}

func BenchmarkParseFairShareMetrics(b *testing.B) {
	in := benchInput(b, "fairshare.txt")
	b.ReportAllocs()
	for b.Loop() {
		_ = ParseFairShareMetrics(in)
	}
}

func BenchmarkParseSchedulerMetrics(b *testing.B) {
	in := benchInput(b, "scheduler.txt")
	b.ReportAllocs()
	for b.Loop() {
		_ = ParseSchedulerMetrics(in)
	}
}

func BenchmarkParseLicenseMetrics(b *testing.B) {
	in := benchInput(b, "licenses.txt")
	b.ReportAllocs()
	for b.Loop() {
		_ = ParseLicenseMetrics(in)
	}
}

func BenchmarkParseReservationNodesMetrics(b *testing.B) {
	in := benchInput(b, "scontrol_nodes.txt")
	b.ReportAllocs()
	for b.Loop() {
		_ = ParseReservationNodesMetrics(in)
	}
}

func BenchmarkParseSacctEfficiency(b *testing.B) {
	in := benchInput(b, "sacct_efficiency.txt")
	b.ReportAllocs()
	for b.Loop() {
		_ = ParseSacctEfficiency(in)
	}
}

// The three GPU counters do not take the raw snapshot: they take the three
// column layouts splitGPUViews projects it into. Feeding them the 4-column
// snapshot makes every line fail the field-count check, so the benchmark would
// measure the reject path and report a speedup that says nothing about parsing
// GRES. The projection is reproduced here rather than called, so this file
// builds against a tree that predates splitGPUViews and both sides parse
// byte-identical input.
func gpuViews(b *testing.B) (total, alloc, idle []byte) {
	b.Helper()
	var tv, av, iv strings.Builder
	for _, line := range strings.Split(string(benchInput(b, "gpus_snapshot.txt")), "\n") {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		nodes, state, gres, used := f[0], f[1], f[2], f[3]
		tv.WriteString(nodes + " " + gres + "\n")
		// Mirrors isAvailableGPUState: only nodes that can hold a job.
		switch state {
		case "allocated", "mixed", "idle", "completing", "reserved":
			av.WriteString(nodes + " " + used + "\n")
			iv.WriteString(nodes + " " + gres + " " + used + "\n")
		}
	}
	return []byte(tv.String()), []byte(av.String()), []byte(iv.String())
}

func BenchmarkParseTotalGPUs(b *testing.B) {
	in, _, _ := gpuViews(b)
	b.ReportAllocs()
	for b.Loop() {
		_ = ParseTotalGPUs(in)
	}
}

func BenchmarkParseAllocatedGPUs(b *testing.B) {
	_, in, _ := gpuViews(b)
	b.ReportAllocs()
	for b.Loop() {
		_ = ParseAllocatedGPUs(in)
	}
}

func BenchmarkParseIdleGPUs(b *testing.B) {
	_, _, in := gpuViews(b)
	b.ReportAllocs()
	for b.Loop() {
		_ = ParseIdleGPUs(in)
	}
}
