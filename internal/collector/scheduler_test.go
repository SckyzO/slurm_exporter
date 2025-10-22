package collector

import (
	"io"
	"os"
	"testing"
)

func TestSchedulerMetrics(t *testing.T) {
	// Read the input data from a file
	file, err := os.Open("../../test_data/sdiag.txt")
	if err != nil {
		t.Fatalf("Can not open test data: %v", err)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("Can not read test data: %v", err)
	}
	t.Logf("%+v", ParseSchedulerMetrics(data))
}
