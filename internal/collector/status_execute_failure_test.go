package collector

import (
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// collectorSuccessValues gathers reg and returns
// slurm_exporter_collector_success keyed by the collector label.
func collectorSuccessValues(t *testing.T, reg *prometheus.Registry) map[string]float64 {
	t.Helper()

	mfs, err := reg.Gather()
	require.NoError(t, err)

	got := make(map[string]float64)
	for _, mf := range mfs {
		if mf.GetName() != "slurm_exporter_collector_success" {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "collector" {
					got[lp.GetValue()] = m.GetGauge().GetValue()
				}
			}
		}
	}
	return got
}

// TestStatusTracker_SuccessIsZeroWhenSlurmCommandsFail is the non-regression
// test for the health metric being unable to report the failure mode it exists
// for.
//
// slurm_exporter_collector_success is documented in docs/metrics.md as
// "1=OK, 0=FAIL per collector". StatusTracker.Collect only lowered it to 0
// inside its recover() block, but no collector panics when a Slurm command
// fails: every one logs the error and returns. So with slurmctld down, or with
// sinfo exceeding --command.timeout, the metric kept reporting 1 while the
// series it should have produced were absent from /metrics. An alert on
// slurm_exporter_collector_success == 0 could never fire.
//
// Every Slurm command is made to fail here, which is what an unreachable
// slurmctld looks like from inside Execute.
func TestStatusTracker_SuccessIsZeroWhenSlurmCommandsFail(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()

	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		return nil, errors.New("simulated: slurmctld unreachable")
	}

	// Reset the shared caches so a previous test's payload cannot satisfy the
	// collectors that read through them (scontrol nodes, and the squeue snapshot
	// shared by accounts/users/partitions since #144).
	oldCache := scontrolNodesCache
	scontrolNodesCache = &timedCache{ttl: oldCache.ttl}
	defer func() { scontrolNodesCache = oldCache }()
	resetSqueueJobsCache()

	log := logger.NewLogger("error")

	// The full default collector set from cmd/slurm_exporter/main.go, minus
	// sacct_efficiency which is opt-in and serves a background cache rather
	// than running on the scrape path.
	tracker := NewStatusTracker(log)
	tracker.Add("accounts", NewAccountsCollector(log))
	tracker.Add("cpus", NewCPUsCollector(log))
	tracker.Add("nodes", NewNodesCollector(log, true))
	tracker.Add("node", NewNodeCollector(log))
	tracker.Add("drain_reason", NewDrainReasonCollector(log))
	tracker.Add("partitions", NewPartitionsCollector(log))
	tracker.Add("queue", NewQueueCollector(log, true, true))
	tracker.Add("scheduler", NewSchedulerCollector(log))
	tracker.Add("fairshare", NewFairShareCollector(log, true))
	tracker.Add("users", NewUsersCollector(log))
	tracker.Add("gpus", NewGPUsCollector(log))
	tracker.Add("reservations", NewReservationsCollector(log))
	tracker.Add("reservation_nodes", NewReservationNodesCollector(log))
	tracker.Add("licenses", NewLicensesCollector(log))

	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(tracker))

	got := collectorSuccessValues(t, reg)

	for _, name := range []string{
		"accounts", "cpus", "nodes", "node", "drain_reason", "partitions",
		"queue", "scheduler", "fairshare", "users", "gpus", "reservations",
		"reservation_nodes", "licenses",
	} {
		value, present := got[name]
		assert.True(t, present, "collector %q must emit slurm_exporter_collector_success", name)
		assert.Equal(t, 0.0, value,
			"collector %q reported success=%v while every Slurm command failed", name, value)
	}
}

// TestStatusTracker_SuccessStaysOneWhenCommandsSucceed guards the other
// direction, so the fix above cannot be satisfied by hardcoding 0.
func TestStatusTracker_SuccessStaysOneWhenCommandsSucceed(t *testing.T) {
	log := logger.NewLogger("error")

	tracker := NewStatusTracker(log)
	tracker.Add("healthy", newMockCollector("slurm_probe_metric", 1))

	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(tracker))

	got := collectorSuccessValues(t, reg)
	assert.Equal(t, 1.0, got["healthy"], "a collector that returns normally must report success=1")
}
