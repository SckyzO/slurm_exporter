package collector

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParsePartitionGPUsLongValues is the non-regression test for the
// fixed-width truncation class of bug fixed alongside issue #10. Before the
// fix, PartitionsGpuData() used "--Format=Nodes:10 ,Partition:30 ,Gres:50 ,GresUsed:50",
// which silently truncated partition names longer than 30 chars and GRES
// strings longer than 50 chars.
//
// This test also covers the multi-type GPU undercount fix in parseGpuCount.
// The fixture's GRES line `gpu:nvidia_a100_80gb:8,mig:...,gpu:nvidia_h100_80gb:4`
// sums to 12 GPUs per node (mig spec ignored by the gpu: regex).
func TestParsePartitionGPUsLongValues(t *testing.T) {
	data, err := os.ReadFile("../../test_data/sinfo_partitions_gpu_long.txt")
	require.NoError(t, err)

	partitions := make(map[string]*PartitionMetrics)
	parsePartitionGPUs(data, partitions)

	// Long-name partition (52 chars) must be present under its FULL name,
	// not truncated to 30 chars.
	const longName = "a_partition_with_a_very_long_name_above_thirty_chars"
	require.Contains(t, partitions, longName,
		"long partition name must be preserved (not truncated)")

	// Counts after parseGpuCount fix (sums all gpu: matches):
	//   per node total = 8 + 4 = 12 (mig: spec ignored by gpu: regex)
	//   per node alloc = 3 + 2 = 5
	//   per node idle  = 12 - 5 = 7
	//   × 2 nodes → alloc=10, idle=14
	assert.Equal(t, 10.0, partitions[longName].gpuAllocated,
		"multi-type GRES counts must be summed (regression test for parseGpuCount fix)")
	assert.Equal(t, 14.0, partitions[longName].gpuIdle)

	// Short partition baseline (single-type GPU, unambiguous).
	require.Contains(t, partitions, "short_part")
	assert.Equal(t, 0.0, partitions["short_part"].gpuAllocated)
	assert.Equal(t, 16.0, partitions["short_part"].gpuIdle)
}

// TestParseGpuCountMultiType is a focused unit test for the
// parseGpuCount fix in partitions.go. Pre-fix, FindStringSubmatch returned
// only the first gpu: match; multi-type GRES strings were undercounted.
func TestParseGpuCountMultiType(t *testing.T) {
	cases := []struct {
		name string
		gres string
		want float64
	}{
		{"empty", "", 0},
		{"no_gpu", "cpu=64", 0},
		{"single_type", "gpu:A100:4", 4},
		{"two_types", "gpu:A100:4,gpu:H100:2", 6},
		{"three_types", "gpu:A100:4,gpu:H100:2,gpu:V100:8", 14},
		{"mig_ignored", "gpu:A100:4,mig:nvidia_a100_3g.40gb:7,gpu:H100:2", 6},
		{"with_idx_suffix", "gpu:A100:4(IDX:0-3),gpu:H100:2(IDX:0-1)", 6},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseGpuCount(tc.gres, partitionGpuRe)
			assert.Equal(t, tc.want, got)
		})
	}
}
