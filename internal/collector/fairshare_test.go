package collector

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sckyzo/slurm_exporter/internal/logger"
	"github.com/stretchr/testify/assert"
)

func TestParseFairShareMetrics(t *testing.T) {
	// Synthetic data replacing the original user-provided test data.
	// Tests root account, direct users, users with 'parent' shares, top-level accounts, and nested accounts/users.
	input := `root|||0.000000|500000||
 root|alice|1|0.100000|0|0.000000|1.000000
 root|ci-bot|parent|0.000000|25000|0.050000|0.500000
 science||1|0.500000|100000|0.200000|
  biology||1000|0.250000|50000|0.100000|
   genetics||1|0.125000|10000|0.020000|
    genetics|bob|1|1.000000|10000|0.020000|0.750000`

	metrics := ParseFairShareMetrics([]byte(input))

	if len(metrics) != 6 {
		t.Fatalf("Expected 6 metrics, got %d", len(metrics))
	}

	// Test root account
	if metrics[0].Account != "root" || metrics[0].User != "" || metrics[0].RawUsage != 500000 {
		t.Errorf("Incorrect root account data: %+v", metrics[0])
	}

	// Test user with numeric shares
	if metrics[1].Account != "root" || metrics[1].User != "alice" || metrics[1].RawShares != 1 {
		t.Errorf("Incorrect user data: %+v", metrics[1])
	}

	// The 'parent' shares entry should be skipped. The next is the top-level 'science' account.
	if metrics[2].Account != "science" || metrics[2].User != "" {
		t.Errorf("Incorrect top-level account data: %+v", metrics[2])
	}

	// Test sub-account (biology)
	if metrics[3].Account != "biology" || metrics[3].User != "" || metrics[3].NormShares != 0.25 {
		t.Errorf("Incorrect sub-account data: %+v", metrics[3])
	}

	// Test nested user (bob)
	if metrics[5].User != "bob" || metrics[5].FairShare != 0.75 {
		t.Errorf("Incorrect nested user data: %+v", metrics[5])
	}
}

func TestFairShareCollector_DuplicateAccounts(t *testing.T) {
	// Mock Execute to return duplicate account data
	oldExecute := Execute
	defer func() { Execute = oldExecute }()

	Execute = func(logger *logger.Logger, command string, args []string) ([]byte, error) {
		// Simulate sshare -a output with duplicate accounts
		// This can happen when an account has multiple parents or is shown multiple times in the tree
		output := `root|||0.000000|500000||
science||1|0.500000|100000|0.200000|
science||1|0.500000|100000|0.200000|
`
		return []byte(output), nil
	}

	log := logger.NewLogger("error")
	collector := NewFairShareCollector(log)
	
	registry := prometheus.NewRegistry()
	err := registry.Register(collector)
	assert.NoError(t, err)

	// Gather metrics. This should now succeed with deduplication.
	_, err = registry.Gather()
	assert.NoError(t, err)
}

func TestFairShareCollector_DuplicateUsers(t *testing.T) {
	// Mock Execute to return duplicate user data
	oldExecute := Execute
	defer func() { Execute = oldExecute }()

	Execute = func(logger *logger.Logger, command string, args []string) ([]byte, error) {
		output := `root|||0.000000|500000||
science||1|0.500000|100000|0.200000|
science|alice|1|0.100000|0|0.000000|1.000000
science|alice|1|0.100000|0|0.000000|1.000000
`
		return []byte(output), nil
	}

	log := logger.NewLogger("error")
	collector := NewFairShareCollector(log)
	
	registry := prometheus.NewRegistry()
	err := registry.Register(collector)
	assert.NoError(t, err)

	// Gather metrics. This should now succeed with deduplication.
	_, err = registry.Gather()
	assert.NoError(t, err)
}
