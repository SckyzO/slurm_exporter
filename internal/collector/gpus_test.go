package collector

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mike8161990/slurm_exporter/internal/logger"
)

func TestGPUsMetrics(t *testing.T) {
	test_data_paths, _ := filepath.Glob("../test_data/slurm-*")
	for _, test_data_path := range test_data_paths {
		slurm_version := strings.TrimPrefix(test_data_path, "../test_data/slurm-")
		t.Logf("slurm-%s", slurm_version)

		// Read the input data from a file
		file, err := os.Open(test_data_path + "/sinfo_gpus_allocated.txt")
		if err != nil {
			t.Fatalf("Can not open test data: %v", err)
		}
		data, _ := io.ReadAll(file)
		metrics := ParseAllocatedGPUs(data)
		t.Logf("Allocated: %+v", metrics)

		// Read the input data from a file
		file, err = os.Open(test_data_path + "/sinfo_gpus_idle.txt")
		if err != nil {
			t.Fatalf("Can not open test data: %v", err)
		}
		data, _ = io.ReadAll(file)
		metrics = ParseIdleGPUs(data)
		t.Logf("Idle: %+v", metrics)

		// Read the input data from a file
		file, err = os.Open(test_data_path + "/sinfo_gpus_total.txt")
		if err != nil {
			t.Fatalf("Can not open test data: %v", err)
		}
		data, _ = io.ReadAll(file)
		metrics = ParseTotalGPUs(data)
		t.Logf("Total: %+v", metrics)
	}
}

func TestGPUsGetMetrics(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()

	test_data_paths, _ := filepath.Glob("../../test_data/slurm-*")
	for _, test_data_path := range test_data_paths {
		slurm_version := strings.TrimPrefix(test_data_path, "../test_data/slurm-")
		t.Run(slurm_version, func(t *testing.T) {
			Execute = func(logger *logger.Logger, command string, args []string) ([]byte, error) {
				var file string
				if strings.Contains(args[2], "GresUsed:") && strings.Contains(args[2], "Gres:") {
					file = filepath.Join(test_data_path, "sinfo_gpus_idle.txt")
				} else if strings.Contains(args[2], "GresUsed:") {
					file = filepath.Join(test_data_path, "sinfo_gpus_allocated.txt")
				} else if strings.Contains(args[2], "Gres:") {
					file = filepath.Join(test_data_path, "sinfo_gpus_total.txt")
				}
				data, err := os.ReadFile(file)
				if err != nil {
					return nil, err
				}
				return data, nil
			}

			testLogger := logger.NewLogger("debug")
			metrics, err := GPUsGetMetrics(testLogger)
			if err != nil {
				t.Fatalf("GPUsGetMetrics() error: %v", err)
			}
			t.Logf("slurm-%s: %+v", slurm_version, metrics)
		})
	}
}
