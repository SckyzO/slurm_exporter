package collector

import (
	"os"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// ── ParseFairShareMetrics ──────────────────────────────────────────────────────

func TestParseFairShareMetrics_Basic(t *testing.T) {
	input := `root|||0.000000|500000||
root|alice|1|0.100000|0|0.000000|1.000000
root|ci-bot|parent|0.000000|25000|0.050000|0.500000
science||1|0.500000|100000|0.200000|0.600000
biology||1000|0.250000|50000|0.100000|
genetics||1|0.125000|10000|0.020000|
genetics|bob|1|1.000000|10000|0.020000|0.750000`

	metrics := ParseFairShareMetrics([]byte(input))

	// ci-bot (parent) must be skipped → 6 entries
	require.Len(t, metrics, 6)

	// root account
	assert.Equal(t, "root", metrics[0].Account)
	assert.Empty(t, metrics[0].User)
	assert.Equal(t, float64(500000), metrics[0].RawUsage)
	assert.Equal(t, float64(0), metrics[0].FairShare) // empty string → 0

	// alice (user under root)
	assert.Equal(t, "root", metrics[1].Account)
	assert.Equal(t, "alice", metrics[1].User)
	assert.Equal(t, float64(1), metrics[1].RawShares)
	assert.Equal(t, float64(1.0), metrics[1].FairShare)

	// science (top-level account after skipping ci-bot)
	assert.Equal(t, "science", metrics[2].Account)
	assert.Empty(t, metrics[2].User)
	assert.Equal(t, float64(0.6), metrics[2].FairShare)

	// biology sub-account
	assert.Equal(t, "biology", metrics[3].Account)
	assert.Equal(t, float64(0.25), metrics[3].NormShares)

	// bob (nested user)
	assert.Equal(t, "genetics", metrics[5].Account)
	assert.Equal(t, "bob", metrics[5].User)
	assert.Equal(t, float64(0.75), metrics[5].FairShare)
}

func TestParseFairShareMetrics_SkipsEmptyAccount(t *testing.T) {
	input := `||1|0.5|100||
root|alice|1|0.1|0|0|1.0`
	metrics := ParseFairShareMetrics([]byte(input))
	require.Len(t, metrics, 1)
	assert.Equal(t, "alice", metrics[0].User)
}

func TestParseFairShareMetrics_SkipsParent(t *testing.T) {
	input := `root|user1|parent|0|100|0.1|0.8`
	metrics := ParseFairShareMetrics([]byte(input))
	assert.Empty(t, metrics)
}

func TestParseFairShareMetrics_EmptyInput(t *testing.T) {
	assert.Empty(t, ParseFairShareMetrics([]byte("")))
	assert.Empty(t, ParseFairShareMetrics([]byte("\n\n")))
}

func TestParseFairShareMetrics_MalformedLines(t *testing.T) {
	input := `not-enough-fields|only|three
root|alice|1|0.1|0|0.0|1.0`
	metrics := ParseFairShareMetrics([]byte(input))
	require.Len(t, metrics, 1)
	assert.Equal(t, "alice", metrics[0].User)
}

func TestParseFairShareMetrics_IndentedSubaccounts(t *testing.T) {
	// Real sshare -a output uses leading spaces for hierarchy — TrimSpace must handle it
	input := ` science||1|0.5|100000|0.2|0.6
  biology||1000|0.25|50000|0.1|0.4
   genetics|bob|1|1.0|10000|0.02|0.75`
	metrics := ParseFairShareMetrics([]byte(input))
	require.Len(t, metrics, 3)
	assert.Equal(t, "science", metrics[0].Account)
	assert.Equal(t, "biology", metrics[1].Account)
	assert.Equal(t, "bob", metrics[2].User)
}

func TestParseFairShareMetrics_OnlyParentLines(t *testing.T) {
	input := `root|user1|parent|0|100|0.1|0.8
root|user2|parent|0|200|0.2|0.7`
	assert.Empty(t, ParseFairShareMetrics([]byte(input)))
}

func TestParseFairShareMetrics_FromTestData(t *testing.T) {
	data, err := os.ReadFile("../../test_data/sshare_users.txt")
	require.NoError(t, err)
	metrics := ParseFairShareMetrics(data)
	// sshare_users.txt: 10 lines, 2 parent lines (ci-bot, user3) → 8 entries
	assert.Len(t, metrics, 8)
	// Accounts must not be empty
	for _, m := range metrics {
		assert.NotEmpty(t, m.Account)
	}
}

// ── FairShareGetMetrics via Execute mock ───────────────────────────────────────

func TestFairShareGetMetrics_ViaExecuteMock(t *testing.T) {
	data, err := os.ReadFile("../../test_data/sshare_users.txt")
	require.NoError(t, err)

	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		assert.Equal(t, "sshare", command)
		return data, nil
	}

	log := logger.NewLogger("error")
	metrics, err := FairShareGetMetrics(log)
	require.NoError(t, err)
	assert.Len(t, metrics, 8)
}

// ── FairShareCollector — account metrics ──────────────────────────────────────

func TestFairShareCollector_AccountMetrics(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		return []byte(`hpc_team||100|0.4|200000|0.3|0.57
ml_group||50|0.2|100000|0.15|0.65`), nil
	}

	log := logger.NewLogger("error")
	c := NewFairShareCollector(log, false) // user metrics disabled

	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))

	mfs, err := reg.Gather()
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, mf := range mfs {
		names[mf.GetName()] = true
	}

	assert.True(t, names["slurm_account_fairshare"])
	assert.True(t, names["slurm_account_fairshare_raw_shares"])
	assert.True(t, names["slurm_account_fairshare_norm_shares"])
	assert.True(t, names["slurm_account_fairshare_raw_usage_cpu_seconds"])
	assert.True(t, names["slurm_account_fairshare_norm_usage"])

	// User metrics must NOT be present when disabled
	assert.False(t, names["slurm_user_fairshare"])
}

func TestFairShareCollector_UserMetricsEnabled(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		return []byte(`hpc_team||100|0.4|200000|0.3|0.57
hpc_team|alice|50|0.2|80000|0.12|0.72`), nil
	}

	log := logger.NewLogger("error")
	c := NewFairShareCollector(log, true) // user metrics enabled

	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))

	mfs, err := reg.Gather()
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, mf := range mfs {
		names[mf.GetName()] = true
	}

	assert.True(t, names["slurm_user_fairshare"])
	assert.True(t, names["slurm_user_fairshare_raw_usage_cpu_seconds"])
	assert.True(t, names["slurm_user_fairshare_norm_usage"])
}

func TestFairShareCollector_DeduplicateAccounts(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		// Same account appearing twice (hierarchical sshare output)
		return []byte(`science||1|0.5|100000|0.2|0.6
science||1|0.5|100000|0.2|0.6`), nil
	}

	log := logger.NewLogger("error")
	c := NewFairShareCollector(log, false)

	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))

	_, err := reg.Gather()
	assert.NoError(t, err, "duplicate account entries must not cause panic")
}

func TestFairShareCollector_DeduplicateUsers(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		return []byte(`science||1|0.5|100000|0.2|0.6
science|alice|1|0.1|0|0.0|1.0
science|alice|1|0.1|0|0.0|1.0`), nil
	}

	log := logger.NewLogger("error")
	c := NewFairShareCollector(log, true)

	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))

	_, err := reg.Gather()
	assert.NoError(t, err, "duplicate user entries must not cause panic")
}

func TestFairShareCollector_ErrorHandling(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		return nil, assert.AnError
	}

	log := logger.NewLogger("error")
	c := NewFairShareCollector(log, true)

	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(c))

	// Must not panic on error, just return no metrics
	mfs, err := reg.Gather()
	assert.NoError(t, err)
	assert.Empty(t, mfs)
}

func TestFairShareCollector_Describe(t *testing.T) {
	log := logger.NewLogger("error")

	// With user metrics
	c := NewFairShareCollector(log, true)
	ch := make(chan *prometheus.Desc, 20)
	c.Describe(ch)
	close(ch)
	count := 0
	for range ch {
		count++
	}
	assert.Equal(t, 10, count, "should describe 5 account + 5 user descriptors")

	// Without user metrics
	c2 := NewFairShareCollector(log, false)
	ch2 := make(chan *prometheus.Desc, 20)
	c2.Describe(ch2)
	close(ch2)
	count2 := 0
	for range ch2 {
		count2++
	}
	assert.Equal(t, 5, count2, "should describe only 5 account descriptors")
}
