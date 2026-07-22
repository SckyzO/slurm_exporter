package collector

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// captureExecuteArgs replaces Execute with a stub recording the command line it
// was handed, so a test can assert on what the exporter asks Slurm for rather
// than on what it does with the answer. The real Execute is restored afterwards.
func captureExecuteArgs(t *testing.T, output string) *[]string {
	t.Helper()

	old := Execute
	t.Cleanup(func() { Execute = old })

	var got []string
	Execute = func(_ *logger.Logger, command string, args []string) ([]byte, error) {
		got = append([]string{command}, args...)
		return []byte(output), nil
	}
	return &got
}

// TestQueueDataAsksSqueueForTerminalStates is the non-regression test for issue
// #27.
//
// squeue reports pending and running jobs unless told otherwise, so nine of the
// eleven states the parser knows how to read were never in its output. The six
// totals built from them, slurm_jobs_failed above all, sat flat at zero on every
// cluster and read as "nothing has ever failed here" rather than as a missing
// measurement.
func TestQueueDataAsksSqueueForTerminalStates(t *testing.T) {
	args := captureExecuteArgs(t, "")

	_, err := QueueData(logger.NewLogger("error"), true)
	require.NoError(t, err)
	assert.Contains(t, *args, "--states=all",
		"without it squeue hides the terminal states and every failure metric stays at zero")
}

// TestQueueDataOmitsTerminalStatesWhenDisabled pins the escape hatch. Asking for
// every state makes slurmctld walk jobs it would otherwise skip, so a site that
// measures the cost and finds it too high must be able to go back to the old
// query without pinning an old release.
func TestQueueDataOmitsTerminalStatesWhenDisabled(t *testing.T) {
	args := captureExecuteArgs(t, "")

	_, err := QueueData(logger.NewLogger("error"), false)
	require.NoError(t, err)
	assert.NotContains(t, *args, "--states=all")
	assert.Equal(t, []string{"squeue", "-h", "-o", "%P|%T|%C|%r|%u"}, *args,
		"disabling terminal states must reproduce the query exactly as it was before #27")
}

// TestQueueCollectorPassesTerminalStatesToSqueue checks the wiring rather than
// the query builder: the flag has to survive the trip from the collector down to
// Execute, which is where a knob wired to nothing would go unnoticed.
func TestQueueCollectorPassesTerminalStatesToSqueue(t *testing.T) {
	for _, tc := range []struct {
		name           string
		terminalStates bool
		wantStatesFlag bool
	}{
		{name: "enabled", terminalStates: true, wantStatesFlag: true},
		{name: "disabled", terminalStates: false, wantStatesFlag: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := captureExecuteArgs(t, "")

			c := NewQueueCollector(logger.NewLogger("error"), true, tc.terminalStates)
			ch := make(chan prometheus.Metric, 128)
			c.Collect(ch)
			close(ch)

			if tc.wantStatesFlag {
				assert.Contains(t, *args, "--states=all")
			} else {
				assert.NotContains(t, *args, "--states=all")
			}
		})
	}
}

// TestQueueCollectorPublishesTerminalStates guards what the flag is for. Feeding
// the collector the states squeue only returns with --states=all must yield the
// series that issue #27 reported missing, per user and partition as well as in
// the cluster-wide totals.
func TestQueueCollectorPublishesTerminalStates(t *testing.T) {
	stubExecute(t, "exclusive|FAILED|8|RaisedSignal:53|alice\n"+
		"exclusive|TIMEOUT|128|TimeLimit|bob\n"+
		"visualisation|COMPLETED|2|None|alice\n")

	log := logger.NewLogger("error")
	c := func() prometheus.Collector { return NewQueueCollector(log, true, true) }

	assert.Equal(t, []string{`slurm_queue_failed{partition="exclusive",user="alice"} 1`},
		gatheredSeries(t, c(), "slurm_queue_failed"))
	assert.Equal(t, []string{`slurm_cores_failed{partition="exclusive",user="alice"} 8`},
		gatheredSeries(t, c(), "slurm_cores_failed"))
	assert.Equal(t, []string{`slurm_queue_timeout{partition="exclusive",user="bob"} 1`},
		gatheredSeries(t, c(), "slurm_queue_timeout"))
	assert.Equal(t, []string{`slurm_queue_completed{partition="visualisation",user="alice"} 1`},
		gatheredSeries(t, c(), "slurm_queue_completed"))
	assert.Equal(t, []string{"slurm_jobs_failed{} 1"}, gatheredSeries(t, c(), "slurm_jobs_failed"))
	assert.Equal(t, []string{"slurm_jobs_timeout{} 1"}, gatheredSeries(t, c(), "slurm_jobs_timeout"))
	assert.Equal(t, []string{"slurm_jobs_completed{} 1"}, gatheredSeries(t, c(), "slurm_jobs_completed"))
}
