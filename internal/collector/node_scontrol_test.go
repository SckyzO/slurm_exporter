package collector

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// resetNodeCaches clears the caches the node and drain_reason collectors share.
// They are package-level with a 25s TTL, so without this a test would read the
// output a previous test's Execute mock produced and pass or fail depending on
// the order the suite happened to run in.
func resetNodeCaches() {
	scontrolNodesCache = &timedCache{ttl: scontrolNodesCache.ttl}
	sinfoNodeStateCache = &timedCache{ttl: sinfoNodeStateCache.ttl}
}

// A line in the shape `scontrol show nodes -o` really emits, including the OS=
// field: it carries spaces and a '#', which is why the parser anchors on field
// names instead of splitting the line into key=value pairs.
const scontrolLineRealistic = `NodeName=na1 Arch=x86_64 CoresPerSocket=16  CPUAlloc=4 CPUEfctv=32 CPUTot=32 CPULoad=8.62 AvailableFeatures=cpu ActiveFeatures=cpu Gres=(null) NodeAddr=172.18.0.11 NodeHostName=na1 Version=24.11.7 OS=Linux 6.18.33.2-generic #1 SMP PREEMPT_DYNAMIC Thu Jun 18 21:54:43 UTC 2026  RealMemory=15867 AllocMem=8192 FreeMem=406 Sockets=1 Boards=1 State=MIXED+DYNAMIC_NORM ThreadsPerCore=2 TmpDisk=0 Weight=1 Owner=N/A MCS_label=N/A Partitions=cpu,debug,high  BootTime=2026-07-24T16:41:28 SlurmdStartTime=2026-07-31T03:59:08 LastBusyTime=2026-07-31T03:59:08 ResumeAfterTime=None CfgTRES=cpu=32,mem=15867M,billing=32 AllocTRES=cpu=4,mem=8G CurrentWatts=0 AveWatts=0`

func TestParseScontrolNodesOnARealisticLine(t *testing.T) {
	nodes := ParseScontrolNodes([]byte(scontrolLineRealistic))

	require.Contains(t, nodes, "na1")
	n := nodes["na1"]
	assert.Equal(t, uint64(4), n.cpuAlloc)
	assert.Equal(t, uint64(32), n.cpuTotal)
	assert.Equal(t, uint64(8192), n.memAlloc)
	assert.Equal(t, uint64(15867), n.memTotal)
	assert.Equal(t, "MIXED+DYNAMIC_NORM", n.state)
	assert.Equal(t, []string{"cpu", "debug", "high"}, n.partitions)
	assert.Empty(t, n.reason, "no Reason= on a healthy node")

	// The OS= field must not be mistaken for the node's own fields.
	assert.Len(t, nodes, 1, "the words inside OS= must not create extra nodes")
}

// State= is a suffix of NextState=, which Slurm emits on a node pending a
// reboot. An unanchored match could read the wrong one.
func TestParseScontrolNodesDoesNotConfuseStateWithNextState(t *testing.T) {
	line := `NodeName=c1 CPUAlloc=0 CPUTot=8 RealMemory=1000 AllocMem=0 State=IDLE+DRAIN NextState=RESUME Partitions=cpu`
	nodes := ParseScontrolNodes([]byte(line))
	require.Contains(t, nodes, "c1")
	assert.Equal(t, "IDLE+DRAIN", nodes["c1"].state)
}

// The line scanner advances through the line itself, so a token that does not
// advance it would spin forever. These are the shapes that could do it.
func TestParseScontrolNodesSurvivesDegenerateLines(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{name: "trailing spaces", line: "NodeName=c1 State=IDLE Partitions=cpu    "},
		{name: "leading spaces", line: "   NodeName=c1 State=IDLE Partitions=cpu"},
		{name: "runs of spaces", line: "NodeName=c1     State=IDLE      Partitions=cpu"},
		{name: "bare equals signs", line: "NodeName=c1 = == = State=IDLE Partitions=cpu"},
		{name: "equals at end of line", line: "NodeName=c1 State=IDLE Partitions=cpu Trailing="},
		{name: "tokens with no equals", line: "NodeName=c1 SMP PREEMPT_DYNAMIC 2026 State=IDLE Partitions=cpu"},
		{name: "only spaces", line: "    "},
		{name: "single equals", line: "="},
		{name: "empty", line: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A hang here is the failure; the assertions only confirm the
			// fields still came through where a node was named.
			nodes := ParseScontrolNodes([]byte(tt.line))
			if strings.Contains(tt.line, "NodeName=") {
				require.Contains(t, nodes, "c1")
				assert.Equal(t, "IDLE", nodes["c1"].state)
				assert.Equal(t, []string{"cpu"}, nodes["c1"].partitions)
			} else {
				assert.Empty(t, nodes)
			}
		})
	}
}

func TestParseScontrolNodesPartitions(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{
			name: "single partition",
			line: `NodeName=c1 State=IDLE Partitions=cpu`,
			want: []string{"cpu"},
		},
		{
			// scontrol does not mark the default partition, but TrimRight
			// keeps the label identical to the old sinfo path either way.
			name: "default partition marker is stripped",
			line: `NodeName=c1 State=IDLE Partitions=cpu*,debug`,
			want: []string{"cpu", "debug"},
		},
		{
			name: "no Partitions field at all",
			line: `NodeName=c1 State=IDLE`,
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes := ParseScontrolNodes([]byte(tt.line))
			require.Contains(t, nodes, "c1")
			assert.Equal(t, tt.want, nodes["c1"].partitions)
		})
	}
}

// The trap that mis-parsed about a thousand nodes on the first attempt:
// Reason= is not the last field, the reason text contains spaces, and on a
// reserved node ReservationName= follows it.
func TestParseScontrolReason(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantRsn   string
		wantSince string
	}{
		{
			name:      "no reason at all",
			line:      `NodeName=c1 State=IDLE Partitions=cpu`,
			wantRsn:   "",
			wantSince: "",
		},
		{
			name:      "reason is the last field",
			line:      `NodeName=c1 State=IDLE+DRAIN Reason=disk-slow maintenance [alice@2026-04-01T10:00:00]`,
			wantRsn:   "disk-slow maintenance",
			wantSince: "2026-04-01T10:00:00",
		},
		{
			name:      "ReservationName follows the reason",
			line:      `NodeName=c1 State=IDLE+DRAIN Reason=hardware failure [root@2026-04-01T09:00:00] ReservationName=maint_window`,
			wantRsn:   "hardware failure",
			wantSince: "2026-04-01T09:00:00",
		},
		{
			// A reason containing brackets must keep them: only the real
			// " [user@timestamp]" suffix is cut, which is why the match is
			// greedy rather than lazy.
			name:      "reason containing brackets",
			line:      `NodeName=c1 State=DOWN Reason=NHC: check_hw_eth [eth0] failed [nhc@2026-04-02T08:30:00] ReservationName=none`,
			wantRsn:   "NHC: check_hw_eth [eth0] failed",
			wantSince: "2026-04-02T08:30:00",
		},
		{
			// Brackets without an "@" cannot be mistaken for the suffix, so
			// this one is cut correctly whether the match is greedy or not.
			// A reason carrying its own "[user@host]" is what needs the
			// greedy match: only the last qualifying group is the suffix.
			name:      "reason containing its own user@host brackets",
			line:      `NodeName=c1 State=DOWN Reason=mount [nfs@fileserver] stale [root@2026-04-02T08:30:00]`,
			wantRsn:   "mount [nfs@fileserver] stale",
			wantSince: "2026-04-02T08:30:00",
		},
		{
			name:      "reason without a timestamp suffix stops at the next field",
			line:      `NodeName=c1 State=DOWN Reason=Not responding ReservationName=maint_window`,
			wantRsn:   "Not responding",
			wantSince: "",
		},
		{
			name:      "reason without a timestamp as the last field",
			line:      `NodeName=c1 State=DOWN Reason=Not responding`,
			wantRsn:   "Not responding",
			wantSince: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes := ParseScontrolNodes([]byte(tt.line))
			require.Contains(t, nodes, "c1")
			assert.Equal(t, tt.wantRsn, nodes["c1"].reason)
			assert.Equal(t, tt.wantSince, nodes["c1"].reasonSince)
		})
	}
}

// sinfo reports the free CPUs of a degraded node as "other" and those of a
// healthy one as "idle". Getting this backwards would move CPUs between
// slurm_node_cpu_idle and slurm_node_cpu_other without any other symptom.
func TestCPUIdleOtherSplit(t *testing.T) {
	tests := []struct {
		name      string
		state     string
		alloc     uint64
		total     uint64
		wantIdle  uint64
		wantOther uint64
	}{
		{name: "idle node", state: "IDLE", alloc: 0, total: 32, wantIdle: 32, wantOther: 0},
		{name: "mixed node", state: "MIXED+DYNAMIC_NORM", alloc: 4, total: 32, wantIdle: 28, wantOther: 0},
		{name: "allocated node", state: "ALLOCATED", alloc: 32, total: 32, wantIdle: 0, wantOther: 0},
		{name: "reserved is not degraded", state: "IDLE+RESERVED", alloc: 0, total: 32, wantIdle: 32, wantOther: 0},
		{name: "drained", state: "IDLE+DRAIN", alloc: 0, total: 32, wantIdle: 0, wantOther: 32},
		{name: "draining while busy", state: "MIXED+DRAIN", alloc: 8, total: 32, wantIdle: 0, wantOther: 24},
		{name: "down", state: "DOWN+NOT_RESPONDING", alloc: 0, total: 32, wantIdle: 0, wantOther: 32},
		// INVALID_REG has only ever been seen alongside DRAIN or DOWN, which
		// is what decides the split; it is not in the flag list itself.
		{name: "invalid registration with drain", state: "IDLE+DRAIN+INVALID_REG", alloc: 0, total: 16, wantIdle: 0, wantOther: 16},
		{name: "not responding with down", state: "DOWN+DRAIN+NOT_RESPONDING", alloc: 0, total: 16, wantIdle: 0, wantOther: 16},
		// Observed on the test cluster, not assumed: sinfo reports a failed
		// node's free CPUs as idle. #254 proposed treating FAIL as degraded,
		// which would have moved every failed node's capacity to cpu_other.
		{name: "fail is not degraded, idle node", state: "IDLE+DYNAMIC_NORM+FAIL", alloc: 0, total: 10, wantIdle: 10, wantOther: 0},
		{name: "fail is not degraded, busy node", state: "MIXED+DYNAMIC_NORM+FAIL", alloc: 4, total: 10, wantIdle: 6, wantOther: 0},
		{name: "drain wins over fail", state: "IDLE+DRAIN+DYNAMIC_NORM+FAIL", alloc: 0, total: 10, wantIdle: 0, wantOther: 10},
		{name: "alloc above total does not underflow", state: "IDLE", alloc: 40, total: 32, wantIdle: 0, wantOther: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &scontrolNode{state: tt.state, cpuAlloc: tt.alloc, cpuTotal: tt.total}
			idle, other := n.cpuIdleOther()
			assert.Equal(t, tt.wantIdle, idle, "idle")
			assert.Equal(t, tt.wantOther, other, "other")
		})
	}
}

func TestParseNodeStates(t *testing.T) {
	// Shape of `sinfo -h -o "%N|%T"`: one row per (partition, state) group,
	// nodes collapsed into a hostlist expression.
	input := `na[1-2,5]|mixed
na[3-4,6]|idle
nb[1-4]|idle
nb5|drained
na[1-2,5]|mixed
`
	states := ParseNodeStates([]byte(input))

	assert.Equal(t, "mixed", states["na1"])
	assert.Equal(t, "mixed", states["na2"])
	assert.Equal(t, "idle", states["na3"])
	assert.Equal(t, "idle", states["na4"])
	assert.Equal(t, "mixed", states["na5"])
	assert.Equal(t, "idle", states["na6"])
	assert.Equal(t, "drained", states["nb5"])
	assert.Len(t, states, 11)
}

// The state suffixes sinfo appends (#243) must survive untouched: the status
// label is what dashboards and alerts match on.
func TestParseNodeStatesKeepsSuffixesVerbatim(t *testing.T) {
	states := ParseNodeStates([]byte("g1|idle*\ng2|idle\nc2|mixed-\n"))
	assert.Equal(t, "idle*", states["g1"])
	assert.Equal(t, "idle", states["g2"])
	assert.Equal(t, "mixed-", states["c2"])
}

func TestParseNodeStatesIgnoresJunk(t *testing.T) {
	states := ParseNodeStates([]byte("\n\nno-pipe-here\nc1|\n|idle\nc2|idle\n"))
	assert.Equal(t, map[string]string{"c2": "idle"}, states)
}

// ── Command plumbing ─────────────────────────────────────────────────────────

// recordingExecute returns an Execute stub that answers per command and
// records every call, so a test can assert which Slurm commands ran.
func recordingExecute(scontrolOut, sinfoOut string, calls *[]string) func(*logger.Logger, string, []string) ([]byte, error) {
	return func(_ *logger.Logger, command string, args []string) ([]byte, error) {
		*calls = append(*calls, fmt.Sprintf("%s %v", command, args))
		switch command {
		case "scontrol":
			return []byte(scontrolOut), nil
		case "sinfo":
			return []byte(sinfoOut), nil
		}
		return nil, fmt.Errorf("unexpected command %q", command)
	}
}

// The point of #254: neither collector may issue `sinfo -N` any more. This
// pins the two commands they are allowed to run.
func TestNodeCollectorsIssueOnlyTheCheapCommands(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	resetNodeCaches()

	var calls []string
	Execute = recordingExecute(
		`NodeName=c1 CPUAlloc=0 CPUTot=8 RealMemory=1000 AllocMem=0 State=IDLE Partitions=cpu`,
		"c1|idle\n",
		&calls,
	)

	log := logger.NewLogger("error")
	nodes, derived, err := NodeGetMetrics(log)
	require.NoError(t, err)
	require.Contains(t, nodes, "c1")
	assert.Zero(t, derived)

	assert.Equal(t, []string{
		"scontrol [show nodes -o]",
		"sinfo [-h -o %N|%T]",
	}, calls)

	for _, c := range calls {
		assert.NotContains(t, c, "-N", "the per-node sinfo call is what #254 removed")
	}
}

// Both collectors need the same two outputs; the shared caches must mean one
// call each per scrape, not two.
func TestNodeAndDrainReasonShareOneCallEach(t *testing.T) {
	oldExecute := Execute
	defer func() { Execute = oldExecute }()
	resetNodeCaches()

	var calls []string
	Execute = recordingExecute(
		`NodeName=c1 CPUAlloc=0 CPUTot=8 RealMemory=1000 AllocMem=0 State=IDLE+DRAIN Reason=maintenance [alice@2026-04-01T10:00:00] Partitions=cpu`,
		"c1|drained\n",
		&calls,
	)

	log := logger.NewLogger("error")
	_, _, err := NodeGetMetrics(log)
	require.NoError(t, err)
	_, err = DrainReasonGetMetrics(log)
	require.NoError(t, err)

	assert.Len(t, calls, 2, "the second collector must be served from the caches")
}

// A node missing from the grouped sinfo output must keep its series rather
// than disappear, and must say so through the counter the collector logs.
func TestNodeStateFallsBackWithoutDroppingTheNode(t *testing.T) {
	nodes, derived := BuildNodeMetrics(
		[]byte(`NodeName=c1 CPUAlloc=0 CPUTot=8 RealMemory=1000 AllocMem=0 State=IDLE+DRAIN Partitions=cpu`),
		[]byte(""),
	)
	require.Contains(t, nodes, "c1")
	assert.Equal(t, 1, derived)
	assert.Equal(t, "idle", nodes["c1"].nodeStatus)
}

// A node in no partition produced no row under sinfo -N and must produce no
// series here either — every slurm_node_* metric carries a partition label.
func TestNodeInNoPartitionProducesNoSeries(t *testing.T) {
	nodes, _ := BuildNodeMetrics(
		[]byte(`NodeName=c1 CPUAlloc=0 CPUTot=8 RealMemory=1000 AllocMem=0 State=IDLE`),
		[]byte("c1|idle\n"),
	)
	assert.Empty(t, nodes)
}
