package collector

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestParseTotalGPUsLongGres is the non-regression test for the GRES truncation
// class of bug fixed alongside issue #10. Before the fix, TotalGPUsData() used
// "--Format=Nodes:10 ,Gres:50", which silently truncated GRES strings longer
// than 50 characters — losing GPU counts on busy multi-type / MIG nodes.
//
// The fixture simulates the corrected variable-width output. With the buggy
// fixed-width format, line 1 would be truncated to:
//
//	"2 gpu:nvidia_a100_80gb:8,mig:nvidia_a100_3g.40gb:7,gp"
//
// causing the trailing "gpu:nvidia_h100_80gb:4" to be lost (4 GPUs × 2 nodes
// = 8 GPUs missing from the total).
func TestParseTotalGPUsLongGres(t *testing.T) {
	data, err := os.ReadFile("../../test_data/sinfo_gpus_total_long_gres.txt")
	if err != nil {
		t.Fatalf("Can not open test data: %v", err)
	}

	total := ParseTotalGPUs(data)

	// Expected, per line (regex skips non-gpu: prefixes like "mig:"):
	//   line 1: 2 nodes × (8 + 4)        = 24
	//   line 2: 1 node  × 16             = 16
	//   line 3: 3 nodes × 4              = 12
	//                                Total = 52
	// With 50-char truncation, line 1 would yield only 8 × 2 = 16 → total 44.
	assert.Equal(t, 52.0, total,
		"all GPUs in long GRES strings must be counted (truncation regression)")
}
