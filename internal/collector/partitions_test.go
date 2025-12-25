package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

func TestParsePartitionsMetricsWithRealOutput(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()

	testDataDirs, _ := filepath.Glob("../../test_data/slurm-*")
	for _, dir := range testDataDirs {
		slurmVersion := filepath.Base(dir)
		if slurmVersion != "slurm-25.11.1-1" { // only tested on 25.11.1-1
			continue
		}
		t.Run(slurmVersion, func(t *testing.T) {
			Execute = func(logger *logger.Logger, command string, args []string) ([]byte, error) {
				var filename string

				switch command {
				case "sinfo":
					// PartitionsData: args = []string{"-h", "-o", "%R,%C"}
					if len(args) >= 3 && args[1] == "-o" && args[2] == "%R,%C" {
						filename = "sinfo_partitions_cpu.txt"
					} else if len(args) >= 2 && strings.Contains(args[1], "--Format=") {
						// PartitionsGpuData: args = []string{"-h", "--Format=Nodes: ,Partition: ,Gres: ,GresUsed:"}
						if strings.Contains(args[1], "Gres") && strings.Contains(args[1], "GresUsed") {
							filename = "sinfo_partitions_gpu.txt"
						}
					}
				case "squeue":
					if len(args) >= 5 {
						// PartitionsPendingJobsData: ... "--states=PENDING"
						if strings.Contains(args[5], "PENDING") {
							filename = "sinfo_partitions_pending_job.txt"
						}
						// PartitionsRunningJobsData: ... "--states=RUNNING"
						if strings.Contains(args[5], "RUNNING") {
							filename = "sinfo_partitions_running_job.txt"
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
			if err != nil {
				t.Fatalf("ParsePartitionsMetrics() error: %v", err)
			}

			for part, pm := range metrics {
				t.Logf("Partition %s: CPU(alloc/idle/total)=(%.0f/%.0f/%.0f), GPU(alloc/idle)=(%.0f/%.0f), Jobs(pending/running)=(%.0f/%.0f)",
					part, pm.cpuAllocated, pm.cpuIdle, pm.cpuTotal,
					pm.gpuAllocated, pm.gpuIdle,
					pm.jobPending, pm.jobRunning)
			}
		})
	}
}
