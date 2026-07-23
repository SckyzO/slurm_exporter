package collector

import (
	"context"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// ── parseSacctDuration ────────────────────────────────────────────────────────

func TestParseSacctDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
	}{
		{"", 0},
		{"0", 0},
		{"00:01:00", 60},
		{"01:00:00", 3600},
		{"1-00:00:00", 86400},
		{"1-01:30:00", 86400 + 3600 + 1800},
		{"2:30", 150},
		{"45", 45},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.expected, parseSacctDuration(tc.input), "input: %q", tc.input)
	}
}

// ── parseSacctMemory ──────────────────────────────────────────────────────────

func TestParseSacctMemory(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
		present  bool
	}{
		{"", 0, false},
		{"0", 0, false},
		{"16?", 0, false},   // Slurm placeholder — no real value
		{"bogus", 0, false}, // unparseable — must not silently become 0-present
		{"512M", 512, true},
		{"2G", 2048, true},
		{"1024K", 1, true},
		{"4Gn", 4096, true},                  // per-node suffix stripped
		{"2Gc", 2048, true},                  // per-cpu suffix stripped
		{"1.5T", 1.5 * 1024 * 1024, true},    // terabytes (issue #143)
		{"2P", 2 * 1024 * 1024 * 1024, true}, // petabytes (issue #143)
	}
	for _, tc := range tests {
		got, present := parseSacctMemory(tc.input)
		assert.Equal(t, tc.present, present, "present for input: %q", tc.input)
		assert.InDelta(t, tc.expected, got, 0.01, "value for input: %q", tc.input)
	}
}

// ── ParseSacctEfficiency ──────────────────────────────────────────────────────

func TestParseSacctEfficiency_Basic(t *testing.T) {
	// Format: JobID|User|Account|AllocCPUS|Elapsed|TotalCPU|CPUTime|MaxRSS|ReqMem
	// MaxRSS is empty on the allocation line and carried by the step lines.
	input := []byte(`1|alice|hpc_team|4|01:00:00|03:45:00|04:00:00||2048M
1.batch|||4|01:00:00|00:00:00|04:00:00|1024M|
2|bob|ml_group|8|00:30:00|03:50:00|04:00:00||4G
2.batch|||8|00:30:00|00:00:00|04:00:00|3G|
`)
	records := ParseSacctEfficiency(input)
	require.Len(t, records, 2)

	assert.Equal(t, "alice", records[0].User)
	assert.Equal(t, "hpc_team", records[0].Account)
	assert.Equal(t, float64(4), records[0].AllocCPUs)
	assert.Equal(t, float64(3600), records[0].ElapsedSeconds)
	assert.Equal(t, float64(4*3600), records[0].CPUTimeSeconds) // 4 CPUs × 1h
	assert.Equal(t, float64(1024), records[0].MaxRSSMB)         // from step line
	assert.True(t, records[0].MaxRSSPresent)
	assert.Equal(t, float64(2048), records[0].ReqMemMB)
}

// TestParseSacctEfficiency_AggregatesMaxRSSFromSteps checks the core of the #143
// query fix: the peak MaxRSS is taken across a job's step lines, and a job with
// no step (so no MaxRSS) is marked absent rather than zero.
func TestParseSacctEfficiency_AggregatesMaxRSSFromSteps(t *testing.T) {
	input := []byte(`10|alice|hpc_team|8|02:00:00|15:00:00|16:00:00||8G
10.extern|||8|02:00:00|00:00:00|16:00:00|4M|
10.batch|||8|02:00:00|00:00:00|16:00:00|2048M|
10.0|||8|02:00:00|00:00:00|16:00:00|6144M|
11|bob|ml_group|4|01:00:00|03:00:00|04:00:00||4G
`)
	records := ParseSacctEfficiency(input)
	require.Len(t, records, 2)

	byUser := map[string]SacctJobRecord{}
	for _, r := range records {
		byUser[r.User] = r
	}

	// Peak across .extern/.batch/.0 → 6144M, not the first or last seen.
	assert.Equal(t, float64(6144), byUser["alice"].MaxRSSMB)
	assert.True(t, byUser["alice"].MaxRSSPresent)

	// No step line at all → MaxRSS absent, not a phantom 0.
	assert.Equal(t, float64(0), byUser["bob"].MaxRSSMB)
	assert.False(t, byUser["bob"].MaxRSSPresent)
}

func TestParseSacctEfficiency_Empty(t *testing.T) {
	assert.Empty(t, ParseSacctEfficiency([]byte("")))
	assert.Empty(t, ParseSacctEfficiency([]byte("\n\n")))
}

func TestParseSacctEfficiency_SkipsMalformed(t *testing.T) {
	input := []byte(`only|four|fields|here
1|alice|hpc_team|4|01:00:00|03:45:00|04:00:00||2G
`)
	records := ParseSacctEfficiency(input)
	assert.Len(t, records, 1)
}

func TestParseSacctEfficiency_SkipsZeroAlloc(t *testing.T) {
	input := []byte(`1|alice|hpc_team|0|01:00:00|00:00:00|00:00:00||0`)
	assert.Empty(t, ParseSacctEfficiency(input))
}

// ── AggregateSacctEfficiency ─────────────────────────────────────────────────

func TestAggregateSacctEfficiency(t *testing.T) {
	records := []SacctJobRecord{
		{User: "alice", Account: "hpc_team", AllocCPUs: 4, ElapsedSeconds: 3600,
			TotalCPUSeconds: 3600, CPUTimeSeconds: 4 * 3600, MaxRSSMB: 1024, MaxRSSPresent: true, ReqMemMB: 2048},
		{User: "alice", Account: "hpc_team", AllocCPUs: 4, ElapsedSeconds: 3600,
			TotalCPUSeconds: 7200, CPUTimeSeconds: 4 * 3600, MaxRSSMB: 2048, MaxRSSPresent: true, ReqMemMB: 2048},
	}

	aggs := AggregateSacctEfficiency(records)
	require.Contains(t, aggs, "hpc_team")
	require.Contains(t, aggs["hpc_team"], "alice")

	alice := aggs["hpc_team"]["alice"]
	assert.Equal(t, float64(2), alice.JobCount)
	// avg CPU eff: (3600/14400*100 + 7200/14400*100) / 2 = (25 + 50) / 2 = 37.5%
	assert.InDelta(t, 37.5, alice.CPUEfficiencyPct, 0.1)
	// avg mem eff: (1024/2048*100 + 2048/2048*100) / 2 = (50 + 100) / 2 = 75%
	assert.InDelta(t, 75.0, alice.MemEfficiencyPct, 0.1)
}

// TestAggregateSacctEfficiency_PartialMemoryJobs is the non-regression test
// for issue #14 / PR #15: averages must use per-metric job counts as their
// denominator. Pre-fix, the memory average was divided by JobCount (total
// jobs), diluting it by every job submitted without `--mem`.
func TestAggregateSacctEfficiency_PartialMemoryJobs(t *testing.T) {
	records := []SacctJobRecord{
		// Job with memory tracked: 80% efficient (1600/2000)
		{User: "alice", Account: "hpc", AllocCPUs: 4, ElapsedSeconds: 3600,
			TotalCPUSeconds: 3600, CPUTimeSeconds: 4 * 3600,
			MaxRSSMB: 1600, MaxRSSPresent: true, ReqMemMB: 2000},
		// Job without memory request — must be EXCLUDED from the mem average
		{User: "alice", Account: "hpc", AllocCPUs: 4, ElapsedSeconds: 3600,
			TotalCPUSeconds: 1800, CPUTimeSeconds: 4 * 3600,
			MaxRSSMB: 0, ReqMemMB: 0},
	}

	aggs := AggregateSacctEfficiency(records)
	alice := aggs["hpc"]["alice"]

	// 2 total jobs, but only 1 with memory data
	assert.Equal(t, float64(2), alice.JobCount)
	assert.Equal(t, float64(2), alice.CPUJobCount)
	assert.Equal(t, float64(1), alice.MemJobCount)

	// Mem avg = only job 1 → 1600/2000 * 100 = 80%
	// (pre-fix value would be 40%, diluted by job 2)
	assert.InDelta(t, 80.0, alice.MemEfficiencyPct, 0.1)

	// CPU avg = both jobs:
	//   job 1: 3600/14400 * 100 = 25%
	//   job 2: 1800/14400 * 100 = 12.5%
	//   avg = 18.75%
	assert.InDelta(t, 18.75, alice.CPUEfficiencyPct, 0.1)
}

// TestAggregateSacctEfficiency_ExcludesJobsWithoutMaxRSS is the regression test
// for issue #143. A job that requested memory (ReqMem > 0) but whose accounting
// never reported a MaxRSS must NOT enter the memory-efficiency average at 0%.
// Pre-fix, the guard was `MaxRSSMB >= 0`, always true, so such a job dragged the
// average toward zero. The fix gates on MaxRSSPresent instead.
func TestAggregateSacctEfficiency_ExcludesJobsWithoutMaxRSS(t *testing.T) {
	records := []SacctJobRecord{
		// Real memory data: 75% efficient (1536/2048).
		{User: "alice", Account: "hpc", AllocCPUs: 4, ElapsedSeconds: 3600,
			TotalCPUSeconds: 3600, CPUTimeSeconds: 4 * 3600,
			MaxRSSMB: 1536, MaxRSSPresent: true, ReqMemMB: 2048},
		// Requested memory but NO MaxRSS recorded — must be EXCLUDED, not
		// counted as 0%. This is the case the old test never exercised.
		{User: "alice", Account: "hpc", AllocCPUs: 4, ElapsedSeconds: 3600,
			TotalCPUSeconds: 1800, CPUTimeSeconds: 4 * 3600,
			MaxRSSMB: 0, MaxRSSPresent: false, ReqMemMB: 2048},
	}

	alice := AggregateSacctEfficiency(records)["hpc"]["alice"]

	assert.Equal(t, float64(2), alice.JobCount, "both jobs counted overall")
	assert.Equal(t, float64(1), alice.MemJobCount, "only the job with MaxRSS has memory data")
	// Mem avg must be the single real job's 75%, not diluted to 37.5% by a phantom 0%.
	assert.InDelta(t, 75.0, alice.MemEfficiencyPct, 0.1)
}

// ── SacctEfficiencyCollector ─────────────────────────────────────────────────

func TestSacctEfficiencyCollector_Collect(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		return []byte(`1|alice|hpc_team|4|01:00:00|03:00:00|04:00:00||2G
1.batch|||4|01:00:00|00:00:00|04:00:00|1G|
2|bob|ml_group|8|00:30:00|02:00:00|04:00:00||4G
2.batch|||8|00:30:00|00:00:00|04:00:00|2G|
`), nil
	}

	log := logger.NewLogger("error")
	c := NewSacctEfficiencyCollector(log, 5*time.Minute, 1*time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Start(ctx)

	// Give the goroutine time to do its first refresh
	time.Sleep(100 * time.Millisecond)

	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))

	mfs, err := reg.Gather()
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, mf := range mfs {
		names[mf.GetName()] = true
	}
	assert.True(t, names["slurm_job_cpu_efficiency_avg"])
	assert.True(t, names["slurm_job_mem_efficiency_avg"])
	assert.True(t, names["slurm_job_count_completed"])
	assert.True(t, names["slurm_sacct_last_refresh_timestamp_seconds"])
}

// TestSacctEfficiencyCollector_NoMemMetricWithoutData verifies the emission gate
// from issue #143: when no job has a MaxRSS (e.g. jobacct_gather disabled), the
// collector must NOT emit slurm_job_mem_efficiency_avg at all — emitting a
// constant 0 would make a SlurmLowMemEfficiency alert fire forever. The CPU and
// job-count metrics, which do not depend on MaxRSS, are still emitted.
func TestSacctEfficiencyCollector_NoMemMetricWithoutData(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		// Allocation lines with ReqMem set but no step line → MaxRSS absent.
		return []byte(`1|alice|hpc_team|4|01:00:00|03:00:00|04:00:00||2G
2|bob|ml_group|8|00:30:00|02:00:00|04:00:00||4G
`), nil
	}

	log := logger.NewLogger("error")
	c := NewSacctEfficiencyCollector(log, 5*time.Minute, 1*time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))
	mfs, err := reg.Gather()
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, mf := range mfs {
		names[mf.GetName()] = true
	}
	assert.False(t, names["slurm_job_mem_efficiency_avg"], "no mem metric when no MaxRSS data")
	assert.True(t, names["slurm_job_cpu_efficiency_avg"], "CPU metric still emitted")
	assert.True(t, names["slurm_job_count_completed"], "job count still emitted")
}

// TestSacctEfficiencyRefresh_QueryShape locks in the two query fixes from issue
// #143: the command must NOT use -X (so step lines carrying MaxRSS are returned),
// must lead its --format with JobID (so steps correlate to their job), and must
// pass --endtime alongside --state (without it, a state-filtered sacct returns
// no rows and the whole collector goes silent).
func TestSacctEfficiencyRefresh_QueryShape(t *testing.T) {
	oldExecute := Execute
	var gotArgs []string
	captured := make(chan struct{}, 1)
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		if command == "sacct" {
			gotArgs = args
			select {
			case captured <- struct{}{}:
			default:
			}
		}
		return []byte(""), nil
	}

	log := logger.NewLogger("error")
	c := NewSacctEfficiencyCollector(log, 1*time.Hour, 1*time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		<-c.Done()
		Execute = oldExecute
	}()
	c.Start(ctx)

	select {
	case <-captured:
	case <-time.After(1 * time.Second):
		t.Fatal("refresh never called sacct")
	}

	joined := strings.Join(gotArgs, " ")
	assert.NotContains(t, gotArgs, "-X", "-X drops step lines and empties MaxRSS")
	assert.True(t, strings.Contains(joined, "--format JobID,"), "format must lead with JobID: %q", joined)
	assert.Contains(t, gotArgs, "--endtime", "state-filtered sacct needs a bounded window")
	assert.Contains(t, gotArgs, "--starttime")
	assert.Contains(t, gotArgs, "--state")
}

func TestSacctEfficiencyCollector_EmptyBeforeFirstRefresh(t *testing.T) {
	log := logger.NewLogger("error")
	c := NewSacctEfficiencyCollector(log, 1*time.Hour, 1*time.Hour)
	// Do NOT call Start() — cache is empty

	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))

	mfs, err := reg.Gather()
	assert.NoError(t, err)
	assert.Empty(t, mfs, "no metrics before first refresh")
}

// TestSacctEfficiencyCollector_DoneClosesOnCancel verifies the Done() channel
// is closed when the context passed to Start() is cancelled. This is the
// mechanism main.go relies on for graceful shutdown on SIGTERM/SIGINT
// (issue #18).
func TestSacctEfficiencyCollector_DoneClosesOnCancel(t *testing.T) {
	log := logger.NewLogger("error")
	c := NewSacctEfficiencyCollector(log, 1*time.Hour, 1*time.Hour)
	ctx, cancel := context.WithCancel(context.Background())

	oldExecute := Execute
	defer func() {
		cancel()
		<-c.Done()
		Execute = oldExecute
	}()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		return []byte(""), nil
	}

	c.Start(ctx)
	cancel()

	select {
	case <-c.Done():
		// success
	case <-time.After(1 * time.Second):
		t.Fatal("Done() did not close within 1s after cancel — graceful shutdown broken")
	}
}

func TestSacctEfficiencyCollector_ErrorKeepsPreviousCache(t *testing.T) {
	// The collector starts a background refresh goroutine that calls the
	// package-level Execute. atomic.Int64 keeps the counter race-free, and
	// we wait on c.Done() before restoring Execute so the goroutine has
	// fully exited (otherwise the defer below races with the goroutine's
	// read of Execute).
	var callCount atomic.Int64

	log := logger.NewLogger("error")
	c := NewSacctEfficiencyCollector(log, 1*time.Millisecond, 1*time.Hour)
	ctx, cancel := context.WithCancel(context.Background())

	oldExecute := Execute
	defer func() {
		cancel()
		<-c.Done() // ensure the refresh goroutine has fully exited
		Execute = oldExecute
	}()

	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		if callCount.Add(1) == 1 {
			return []byte("1|alice|hpc_team|4|01:00:00|03:00:00|04:00:00||2G\n" +
				"1.batch|||4|01:00:00|00:00:00|04:00:00|1G|"), nil
		}
		return nil, assert.AnError // second call fails
	}

	c.Start(ctx)

	time.Sleep(50 * time.Millisecond) // let first refresh + failed second run

	// Should still have metrics from first successful refresh
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))
	mfs, err := reg.Gather()
	assert.NoError(t, err)
	assert.NotEmpty(t, mfs, "previous cache must be preserved after error")
}

func TestParseSacctEfficiency_FromTestData(t *testing.T) {
	data, err := os.ReadFile("../../test_data/sacct_efficiency.txt")
	require.NoError(t, err)

	records := ParseSacctEfficiency(data)
	assert.NotEmpty(t, records)

	// Only allocation lines become records; each must carry identity + usage.
	for _, r := range records {
		assert.NotEmpty(t, r.User)
		assert.NotEmpty(t, r.Account)
		assert.Positive(t, r.AllocCPUs)
		assert.Positive(t, r.ElapsedSeconds)
	}

	aggs := AggregateSacctEfficiency(records)
	require.Contains(t, aggs, "hpc_team")
	require.Contains(t, aggs["hpc_team"], "alice")

	// alice has two jobs; MaxRSS is aggregated from step lines (peak 3G of 4G,
	// then 1G of 2G) → (75 + 50) / 2 = 62.5%.
	alice := aggs["hpc_team"]["alice"]
	assert.Equal(t, float64(2), alice.JobCount, "alice has 2 jobs in fixture")
	assert.Equal(t, float64(2), alice.MemJobCount)
	assert.InDelta(t, 62.5, alice.MemEfficiencyPct, 0.1)
	assert.Positive(t, alice.CPUHoursAllocated)

	// bob's job requests 1T and peaks at 768G → 75%, exercising the T/G suffixes
	// through the real parse path (issue #143 bug 2).
	bob := aggs["ml_group"]["bob"]
	assert.Equal(t, float64(1), bob.MemJobCount)
	assert.InDelta(t, 75.0, bob.MemEfficiencyPct, 0.1)

	// carol's job has no MaxRSS on any step → excluded from the memory average
	// entirely, so no phantom 0% (issue #143 bug 1).
	carol := aggs["physics"]["carol"]
	assert.Equal(t, float64(1), carol.JobCount)
	assert.Equal(t, float64(0), carol.MemJobCount, "no MaxRSS → not in mem average")
}
