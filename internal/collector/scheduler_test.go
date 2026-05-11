package collector

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchedulerMetrics(t *testing.T) {
	file, err := os.Open("../../test_data/sdiag.txt")
	require.NoError(t, err, "cannot open test data")
	data, err := io.ReadAll(file)
	require.NoError(t, err, "cannot read test data")

	sm := ParseSchedulerMetrics(data)

	assert.Equal(t, 3.0, sm.threads)
	assert.Equal(t, 0.0, sm.queueSize)
	assert.Equal(t, 0.0, sm.dbdQueueSize)
	assert.Equal(t, 97209.0, sm.lastCycle)
	assert.Equal(t, 74593.0, sm.meanCycle)
	assert.Equal(t, 63.0, sm.cyclePerMinute)
	assert.Equal(t, 111544.0, sm.totalBackfilledJobsSinceStart)
	assert.Equal(t, 793.0, sm.totalBackfilledJobsSinceCycle)
	assert.Equal(t, 10.0, sm.totalBackfilledHeterogeneous)
}

// TestSchedulerRPCLineRe_HyphenatedUsername is the non-regression test for
// PR #28: usernames with hyphens (e.g. "alice-21") were silently truncated
// to "alice" by the character class [A-Za-z0-9_]*. With the fix the full
// username is captured.
func TestSchedulerRPCLineRe_HyphenatedUsername(t *testing.T) {
	cases := []struct {
		name      string
		line      string
		wantUser  string
		wantCount string
	}{
		{
			name:      "plain username",
			line:      "        alice  count:120  ave_time:42  total_time:5040 ",
			wantUser:  "alice",
			wantCount: "120",
		},
		{
			name:      "hyphenated username",
			line:      "        alice-21  count:120  ave_time:42  total_time:5040 ",
			wantUser:  "alice-21",
			wantCount: "120",
		},
		{
			name:      "underscore + digits",
			line:      "        bob_42  count:99  ave_time:7  total_time:693 ",
			wantUser:  "bob_42",
			wantCount: "99",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matches := schedulerRPCLineRe.FindAllStringSubmatch(tc.line, -1)
			require.NotEmpty(t, matches, "regex must match: %q", tc.line)
			require.GreaterOrEqual(t, len(matches[0]), 5, "regex must capture 4 groups + full match")
			assert.Equal(t, tc.wantUser, matches[0][1])
			assert.Equal(t, tc.wantCount, matches[0][2])
		})
	}
}

func TestParseSchedulerMetrics_JobCounters(t *testing.T) {
	// Vérifie que les job counters sdiag sont bien parsés
	data, err := os.ReadFile("../../test_data/sdiag.txt")
	require.NoError(t, err)

	sm := ParseSchedulerMetrics(data)

	// Les counters peuvent être 0 dans le test_data (cluster idle)
	// mais doivent être parsés sans panique
	assert.GreaterOrEqual(t, sm.jobsSubmitted, float64(0))
	assert.GreaterOrEqual(t, sm.jobsStarted, float64(0))
	assert.GreaterOrEqual(t, sm.jobsCompleted, float64(0))
	assert.GreaterOrEqual(t, sm.jobsCanceled, float64(0))
	assert.GreaterOrEqual(t, sm.jobsFailed, float64(0))
}
