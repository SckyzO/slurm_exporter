package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

func TestParsePartitionsMetricsWithRealOutput(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()

	testDataDirs, _ := filepath.Glob("../../test_data/slurm-*")
	for _, dir := range testDataDirs {
		slurmVersion := filepath.Base(dir)
		if slurmVersion != "slurm-25.11.1-1" {
			continue
		}
		t.Run(slurmVersion, func(t *testing.T) {
			Execute = func(logger *logger.Logger, command string, args []string) ([]byte, error) {
				var filename string

				switch command {
				case "sinfo":
					if len(args) >= 3 && args[1] == "-o" && args[2] == "%R,%C" {
						filename = "sinfo_partitions_cpu.txt"
					} else if len(args) >= 2 && strings.Contains(args[1], "--Format=") {
						if strings.Contains(args[1], "Gres") && strings.Contains(args[1], "GresUsed") {
							filename = "sinfo_partitions_gpu.txt"
						}
					}
				case "squeue":
					if len(args) >= 6 {
						if strings.Contains(args[5], "PENDING") {
							filename = "squeue_partitions_pending_job.txt"
						}
						if strings.Contains(args[5], "RUNNING") {
							filename = "squeue_partitions_running_job.txt"
						}
					}
				}

				if filename == "" {
					return nil, fmt.Errorf("unhandled command: %s %v", command, args)
				}

				path := filepath.Join(dir, filename)
				data, err := os.ReadFile(path)
				if err != nil {
					return nil, fmt.Errorf("failed to read %s: %w", path, err)
				}
				return data, nil
			}

			testLogger := logger.NewLogger("debug")
			metrics, err := ParsePartitionsMetrics(testLogger)
			require.NoError(t, err)

			for part, pm := range metrics {
				t.Logf("Partition %s: CPU(alloc/idle/total)=(%.0f/%.0f/%.0f), GPU(alloc/idle)=(%.0f/%.0f), Jobs(pending/running)=(%.0f/%.0f)",
					part, pm.cpuAllocated, pm.cpuIdle, pm.cpuTotal,
					pm.gpuAllocated, pm.gpuIdle,
					pm.jobPending, pm.jobRunning)
			}

			// Verify known partitions exist and have plausible values
			require.Contains(t, metrics, "cpu")
			assert.Greater(t, metrics["cpu"].cpuTotal, 0.0)

			require.Contains(t, metrics, "a100")
			assert.Greater(t, metrics["a100"].gpuAllocated, 0.0, "a100 should have allocated GPUs")
		})
	}
}

// TestParsePartitionsMetricsGPUOnlyPartition is a regression test for issue #5:
// a nil pointer dereference panic when a GPU partition name does not appear in
// the CPU sinfo output (i.e., it exists only in the GPU sinfo output).
// Before the fix, accessing partitions[partition] on a missing key returned nil
// and the next field access caused: "panic: runtime error: invalid memory address
// or nil pointer dereference" at partitions.go:117.
func TestParsePartitionsMetricsGPUOnlyPartition(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()

	// CPU data only knows about "cpu_partition"; GPU data mentions "gpu_only_partition"
	// which is NOT present in the CPU data — this is the exact scenario that caused issue #5.
	Execute = func(l *logger.Logger, command string, args []string) ([]byte, error) {
		switch command {
		case "sinfo":
			if len(args) >= 3 && args[1] == "-o" && args[2] == "%R,%C" {
				// CPU partition data — does NOT include "gpu_only_partition"
				return []byte("cpu_partition,10/20/5/35\n"), nil
			}
			// GPU partition data — includes a partition absent from the CPU output
			return []byte("2 gpu_only_partition gpu:A100:4 gpu:A100:2\n"), nil
		case "squeue":
			return []byte(""), nil
		}
		return nil, fmt.Errorf("unexpected command: %s", command)
	}

	testLogger := logger.NewLogger("debug")

	// Must not panic — this is the regression assertion for issue #5.
	require.NotPanics(t, func() {
		metrics, err := ParsePartitionsMetrics(testLogger)
		require.NoError(t, err)

		// CPU-only partition is present with correct values
		require.Contains(t, metrics, "cpu_partition")
		assert.Equal(t, 10.0, metrics["cpu_partition"].cpuAllocated)

		// GPU-only partition must be initialized (not nil) and contain GPU metrics
		require.Contains(t, metrics, "gpu_only_partition", "GPU-only partition must be created even if absent from CPU data")
		// 2 nodes * 2 allocated GPUs/node = 4 total allocated
		assert.Equal(t, 4.0, metrics["gpu_only_partition"].gpuAllocated)
		// 2 nodes * (4 total - 2 allocated) GPUs/node = 4 idle
		assert.Equal(t, 4.0, metrics["gpu_only_partition"].gpuIdle)
	}, "ParsePartitionsMetrics must not panic when a GPU partition is absent from CPU data (issue #5 regression)")
}
