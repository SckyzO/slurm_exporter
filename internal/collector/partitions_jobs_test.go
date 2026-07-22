package collector

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newJobPartitions builds a partitions map holding the given names, the way
// parsePartitionCPUs and parsePartitionGPUs leave it before
// parsePartitionJobs runs.
func newJobPartitions(names ...string) map[string]*PartitionMetrics {
	partitions := make(map[string]*PartitionMetrics, len(names))
	for _, name := range names {
		partitions[name] = &PartitionMetrics{}
	}
	return partitions
}

// TestParsePartitionJobsCountsMultiPartitionJobs is the non-regression test for
// slurm_partition_jobs_pending and slurm_partition_jobs_running undercounting.
//
// parsePartitionJobs looked partitions up with the raw squeue line, while the
// map keys had been stored normalised: parsePartitionCPUs and parsePartitionGPUs
// strip the default-partition marker. Two shapes of squeue output therefore
// matched no key and were counted nowhere.
//
// A job submitted with "sbatch -p a100,cpu" is reported by squeue -o "%P" as
// "a100,cpu". A default partition is reported as "cpu*" on the Slurm versions
// described in queue.go. Neither string is a key, so the job silently vanished
// from every partition it belonged to, and nothing logged it.
func TestParsePartitionJobsCountsMultiPartitionJobs(t *testing.T) {
	partitions := newJobPartitions("cpu", "a100", "gpu")

	pending := []byte("a100,cpu\ncpu\ngpu\n")
	running := []byte("a100,gpu\na100\n")

	parsePartitionJobs(pending, running, partitions)

	assert.Equal(t, 2.0, partitions["cpu"].jobPending, "cpu: one job of its own plus the a100,cpu job")
	assert.Equal(t, 1.0, partitions["a100"].jobPending, "a100: the a100,cpu job")
	assert.Equal(t, 1.0, partitions["gpu"].jobPending, "gpu: one job of its own")

	assert.Equal(t, 2.0, partitions["a100"].jobRunning, "a100: the a100,gpu job plus one of its own")
	assert.Equal(t, 1.0, partitions["gpu"].jobRunning, "gpu: the a100,gpu job")
	assert.Equal(t, 0.0, partitions["cpu"].jobRunning, "cpu: nothing running")
}

// TestParsePartitionJobsStripsDefaultPartitionMarker pins the version-dependent
// half of the same defect. queue.go:108 records that squeue -o "%P" emits
// "compute*" for the default partition on some Slurm versions. On those, the
// default partition, usually the busiest one on the cluster, reported zero jobs
// forever.
func TestParsePartitionJobsStripsDefaultPartitionMarker(t *testing.T) {
	partitions := newJobPartitions("compute", "gpu")

	parsePartitionJobs([]byte("compute*\ncompute*\n"), []byte("compute*\ngpu*,compute*\n"), partitions)

	assert.Equal(t, 2.0, partitions["compute"].jobPending)
	assert.Equal(t, 2.0, partitions["compute"].jobRunning)
	assert.Equal(t, 1.0, partitions["gpu"].jobRunning)
}

// TestParsePartitionJobsIgnoresUnknownAndEmptyLines guards the other direction,
// so counting more jobs cannot be achieved by counting names that are not
// partitions. A trailing newline yields an empty final element, and a partition
// that holds no nodes appears in squeue output without ever appearing in sinfo.
func TestParsePartitionJobsIgnoresUnknownAndEmptyLines(t *testing.T) {
	partitions := newJobPartitions("cpu")

	parsePartitionJobs([]byte("\ncpu\nretired\n\n"), []byte("  \nretired,unknown\n"), partitions)

	require.Len(t, partitions, 1, "squeue output must not create partitions")
	assert.Equal(t, 1.0, partitions["cpu"].jobPending)
	assert.Equal(t, 0.0, partitions["cpu"].jobRunning)
}
