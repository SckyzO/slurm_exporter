package collector

import (
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// resetSqueueJobsCache clears the shared snapshot so a warm cache left by an
// earlier test cannot hide or fake the call count under test.
func resetSqueueJobsCache() {
	squeueJobsCache.mu.Lock()
	squeueJobsCache.data = nil
	squeueJobsCache.fetchAt = time.Time{}
	squeueJobsCache.mu.Unlock()
}

func loadSqueueJobsFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../test_data/squeue_jobs.txt")
	require.NoError(t, err, "cannot open squeue_jobs.txt")
	return data
}

// TestSqueueJobsSingleCall is the non-regression test for issue #144: the
// accounts, users and partitions collectors must share one squeue snapshot per
// scrape instead of each dumping the full job queue from slurmctld. Registered
// together and gathered once, they must trigger exactly one squeue invocation.
func TestSqueueJobsSingleCall(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	resetSqueueJobsCache()

	fixture := loadSqueueJobsFixture(t)
	var squeueCalls atomic.Int64
	Execute = func(_ *logger.Logger, command string, args []string) ([]byte, error) {
		switch command {
		case "squeue":
			squeueCalls.Add(1)
			return fixture, nil
		case "sinfo":
			if len(args) >= 3 && args[1] == "-o" && args[2] == "%R,%C" {
				return []byte("cpu,10/20/5/35\ngpu,0/8/0/8\n"), nil
			}
			return []byte(""), nil // GPU --Format query: no gpu GRES on this cluster
		}
		return nil, nil
	}

	log := logger.NewLogger("error")
	reg := prometheus.NewRegistry()
	reg.MustRegister(NewAccountsCollector(log))
	reg.MustRegister(NewUsersCollector(log))
	reg.MustRegister(NewPartitionsCollector(log))

	_, err := reg.Gather()
	require.NoError(t, err)

	assert.Equal(t, int64(1), squeueCalls.Load(),
		"accounts + users + partitions must share one squeue call per scrape, not dump the queue once each")
}

// TestProjectAccountsView proves the accounts projection of the shared snapshot
// yields the same per-account metrics the dedicated squeue call produced: same
// -a -r flags, same default state set, so no value changes (issue #144).
func TestProjectAccountsView(t *testing.T) {
	am := ParseAccountsMetrics(projectAccountsView(loadSqueueJobsFixture(t)))

	require.Contains(t, am, "hpc_team")
	assert.Equal(t, 1.0, am["hpc_team"].running)
	assert.Equal(t, 4.0, am["hpc_team"].runningCpus)
	assert.Equal(t, 1.0, am["hpc_team"].pending)

	require.Contains(t, am, "bio")
	assert.Equal(t, 1.0, am["bio"].running)
	assert.Equal(t, 8.0, am["bio"].runningCpus)
	assert.Equal(t, 1.0, am["bio"].pending)

	require.Contains(t, am, "ml_group")
	assert.Equal(t, 2.0, am["ml_group"].running, "eve's two running jobs")
	assert.Equal(t, 4.0, am["ml_group"].runningCpus, "2 + 2")
	assert.Equal(t, 1.0, am["ml_group"].suspended, "dave's suspended job")

	require.Contains(t, am, "physics")
	assert.Equal(t, 1.0, am["physics"].running)
	assert.Equal(t, 1.0, am["physics"].pending, "carol's multi-partition pending job")
}

// TestProjectUsersView proves the users projection yields the same per-user
// metrics the dedicated squeue call produced (issue #144).
func TestProjectUsersView(t *testing.T) {
	um := ParseUsersMetrics(projectUsersView(loadSqueueJobsFixture(t)))

	require.Contains(t, um, "alice")
	assert.Equal(t, 1.0, um["alice"].running)
	assert.Equal(t, 4.0, um["alice"].runningCpus)
	assert.Equal(t, 1.0, um["alice"].pending)

	require.Contains(t, um, "bob")
	assert.Equal(t, 1.0, um["bob"].running)
	assert.Equal(t, 8.0, um["bob"].runningCpus)
	assert.Equal(t, 1.0, um["bob"].pending)

	require.Contains(t, um, "eve")
	assert.Equal(t, 2.0, um["eve"].running)
	assert.Equal(t, 4.0, um["eve"].runningCpus)

	require.Contains(t, um, "frank")
	assert.Equal(t, 1.0, um["frank"].running)

	require.Contains(t, um, "dave")
	assert.Equal(t, 1.0, um["dave"].suspended)

	require.Contains(t, um, "carol")
	assert.Equal(t, 1.0, um["carol"].pending)
}

// newJobPartitions builds a partitions map holding the given names, the way
// parsePartitionCPUs and parsePartitionGPUs leave it before parsePartitionJobs
// runs. Upstream this lives in partitions_jobs_test.go, which came with the
// multi-partition counting fix (#153) that is not backported here.
func newJobPartitions(names ...string) map[string]*PartitionMetrics {
	partitions := make(map[string]*PartitionMetrics, len(names))
	for _, name := range names {
		partitions[name] = &PartitionMetrics{}
	}
	return partitions
}

// TestProjectPartitionView proves the partition projection reproduces the old
// `squeue -h -o "%P" --states=PENDING|RUNNING` output byte for byte: the
// matching rows only, in order, with multi-partition jobs left comma-separated
// (issue #144). That byte equality is what this backport has to preserve — what
// parsePartitionJobs then does with the lines is a separate question.
func TestProjectPartitionView(t *testing.T) {
	data := loadSqueueJobsFixture(t)

	assert.Equal(t, "cpu\ncpu\ncpu,gpu\n", string(projectPartitionView(data, "PENDING")),
		"the PENDING projection must equal what squeue -o \"%P\" --states=PENDING printed")
	assert.Equal(t, "cpu\ncpu\ncpu\ncpu\ncpu\n", string(projectPartitionView(data, "RUNNING")),
		"the RUNNING projection must equal what squeue -o \"%P\" --states=RUNNING printed")

	partitions := newJobPartitions("cpu", "gpu")
	parsePartitionJobs(
		projectPartitionView(data, "PENDING"),
		projectPartitionView(data, "RUNNING"),
		partitions,
	)

	// carol's job is queued in both cpu and gpu, so squeue prints "cpu,gpu".
	// parsePartitionJobs looks that whole string up as a map key on this branch,
	// finds nothing, and counts the job in neither partition. That is #153,
	// fixed on master and deliberately not backported here: this test pins the
	// behaviour as it is, so the counts move only when that fix is taken.
	assert.Equal(t, 2.0, partitions["cpu"].jobPending, "bob + alice; carol(cpu,gpu) counted nowhere (#153)")
	assert.Equal(t, 0.0, partitions["gpu"].jobPending, "carol(cpu,gpu) counted nowhere (#153)")
	assert.Equal(t, 5.0, partitions["cpu"].jobRunning, "alice, bob, eve, eve, frank")
	assert.Equal(t, 0.0, partitions["gpu"].jobRunning)
}
