package collector

import (
	"context"
	"os"
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
	}{
		{"", 0},
		{"0", 0},
		{"512M", 512},
		{"2G", 2048},
		{"1024K", 1},
		{"4Gn", 4096}, // per-node suffix stripped
		{"2Gc", 2048}, // per-cpu suffix stripped
	}
	for _, tc := range tests {
		assert.InDelta(t, tc.expected, parseSacctMemory(tc.input), 0.01, "input: %q", tc.input)
	}
}

// ── ParseSacctEfficiency ──────────────────────────────────────────────────────

func TestParseSacctEfficiency_Basic(t *testing.T) {
	// Format: User|Account|AllocCPUS|Elapsed|TotalCPU|CPUTime|MaxRSS|ReqMem
	input := []byte(`alice|hpc_team|4|01:00:00|03:45:00|04:00:00|1024M|2048M
bob|ml_group|8|00:30:00|03:50:00|04:00:00|3G|4G
`)
	records := ParseSacctEfficiency(input)
	require.Len(t, records, 2)

	assert.Equal(t, "alice", records[0].User)
	assert.Equal(t, "hpc_team", records[0].Account)
	assert.Equal(t, float64(4), records[0].AllocCPUs)
	assert.Equal(t, float64(3600), records[0].ElapsedSeconds)
	assert.Equal(t, float64(4*3600), records[0].CPUTimeSeconds) // 4 CPUs × 1h
	assert.Equal(t, float64(1024), records[0].MaxRSSMB)
	assert.Equal(t, float64(2048), records[0].ReqMemMB)
}

func TestParseSacctEfficiency_Empty(t *testing.T) {
	assert.Empty(t, ParseSacctEfficiency([]byte("")))
	assert.Empty(t, ParseSacctEfficiency([]byte("\n\n")))
}

func TestParseSacctEfficiency_SkipsMalformed(t *testing.T) {
	input := []byte(`only|three|fields
alice|hpc_team|4|01:00:00|03:45:00|04:00:00|1G|2G
`)
	records := ParseSacctEfficiency(input)
	assert.Len(t, records, 1)
}

func TestParseSacctEfficiency_SkipsZeroAlloc(t *testing.T) {
	input := []byte(`alice|hpc_team|0|01:00:00|00:00:00|00:00:00|0|0`)
	assert.Empty(t, ParseSacctEfficiency(input))
}

// ── AggregateSacctEfficiency ─────────────────────────────────────────────────

func TestAggregateSacctEfficiency(t *testing.T) {
	records := []SacctJobRecord{
		{User: "alice", Account: "hpc_team", AllocCPUs: 4, ElapsedSeconds: 3600,
			TotalCPUSeconds: 3600, CPUTimeSeconds: 4 * 3600, MaxRSSMB: 1024, ReqMemMB: 2048},
		{User: "alice", Account: "hpc_team", AllocCPUs: 4, ElapsedSeconds: 3600,
			TotalCPUSeconds: 7200, CPUTimeSeconds: 4 * 3600, MaxRSSMB: 2048, ReqMemMB: 2048},
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

// ── SacctEfficiencyCollector ─────────────────────────────────────────────────

func TestSacctEfficiencyCollector_Collect(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		return []byte(`alice|hpc_team|4|01:00:00|03:00:00|04:00:00|1G|2G
bob|ml_group|8|00:30:00|02:00:00|04:00:00|2G|4G
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

func TestSacctEfficiencyCollector_ErrorKeepsPreviousCache(t *testing.T) {
	callCount := 0
	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		callCount++
		if callCount == 1 {
			return []byte(`alice|hpc_team|4|01:00:00|03:00:00|04:00:00|1G|2G`), nil
		}
		return nil, assert.AnError // second call fails
	}

	log := logger.NewLogger("error")
	c := NewSacctEfficiencyCollector(log, 1*time.Millisecond, 1*time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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

	// All records must have non-empty user and account
	for _, r := range records {
		assert.NotEmpty(t, r.User)
		assert.NotEmpty(t, r.Account)
		assert.Positive(t, r.AllocCPUs)
		assert.Positive(t, r.ElapsedSeconds)
	}

	aggs := AggregateSacctEfficiency(records)
	require.Contains(t, aggs, "hpc_team")
	require.Contains(t, aggs["hpc_team"], "alice")

	alice := aggs["hpc_team"]["alice"]
	assert.Equal(t, float64(2), alice.JobCount, "alice has 2 jobs in fixture")
	assert.Positive(t, alice.CPUHoursAllocated)
}
