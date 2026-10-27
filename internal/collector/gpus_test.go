package collector

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

func TestGPUsMetrics(t *testing.T) {
	// The glob used to be "../test_data/slurm-*", one level short of the
	// fixtures from this package's directory. It matched nothing, so this
	// test iterated over no versions at all and reported PASS without
	// reading a single fixture.
	testDataPaths, _ := filepath.Glob("../../test_data/slurm-*")
	ran := 0
	for _, testDataPath := range testDataPaths {
		if _, err := os.Stat(filepath.Join(testDataPath, "sinfo_gpus_total.txt")); err != nil {
			continue // not a GPU capture
		}
		ran++
		slurmVersion := strings.TrimPrefix(testDataPath, "../../test_data/slurm-")
		t.Logf("slurm-%s", slurmVersion)

		// Read the input data from a file
		file, err := os.Open(testDataPath + "/sinfo_gpus_allocated.txt")
		if err != nil {
			t.Fatalf("Can not open test data: %v", err)
		}
		data, _ := io.ReadAll(file)
		metrics := ParseAllocatedGPUs(data)
		t.Logf("Allocated: %+v", metrics)

		// Read the input data from a file
		file, err = os.Open(testDataPath + "/sinfo_gpus_idle.txt")
		if err != nil {
			t.Fatalf("Can not open test data: %v", err)
		}
		data, _ = io.ReadAll(file)
		metrics = ParseIdleGPUs(data)
		t.Logf("Idle: %+v", metrics)

		// Read the input data from a file
		file, err = os.Open(testDataPath + "/sinfo_gpus_total.txt")
		if err != nil {
			t.Fatalf("Can not open test data: %v", err)
		}
		data, _ = io.ReadAll(file)
		metrics = ParseTotalGPUs(data)
		t.Logf("Total: %+v", metrics)
	}
	if ran == 0 {
		t.Fatal("no test_data/slurm-*/ directory carries GPU fixtures")
	}
}

func TestGPUsGetMetrics(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()

	testDataPaths, _ := filepath.Glob("../../test_data/slurm-*")
	ran := 0
	for _, testDataPath := range testDataPaths {
		// A versioned directory is not necessarily a GPU capture: it holds
		// whatever commands were recorded for that Slurm version, and #254
		// added one with only node and drain_reason output. Without this the
		// glob makes every future capture for another collector fail here.
		if _, err := os.Stat(filepath.Join(testDataPath, "sinfo_gpus_total.txt")); err != nil {
			continue
		}
		ran++
		slurmVersion := strings.TrimPrefix(testDataPath, "../../test_data/slurm-")
		t.Run(slurmVersion, func(t *testing.T) {
			Execute = func(logger *logger.Logger, command string, args []string) ([]byte, error) {
				var file string
				switch {
				case strings.Contains(args[2], "GresUsed:") && strings.Contains(args[2], "Gres:"):
					file = filepath.Join(testDataPath, "sinfo_gpus_idle.txt")
				case strings.Contains(args[2], "GresUsed:"):
					file = filepath.Join(testDataPath, "sinfo_gpus_allocated.txt")
				case strings.Contains(args[2], "Gres:"):
					file = filepath.Join(testDataPath, "sinfo_gpus_total.txt")
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
			t.Logf("slurm-%s: %+v", slurmVersion, metrics)
		})
	}
	// Skipping is fine per directory, skipping all of them is not: that
	// would leave this test green while testing nothing.
	if ran == 0 {
		t.Fatal("no test_data/slurm-*/ directory carries GPU fixtures")
	}
}
