package collector

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

/*
The node and drain_reason collectors used to read `sinfo -N`, which prints one
row per (node, partition) pair. On a cluster with about 6,000 nodes in several
partitions that is ~23,000 rows, and asking sinfo for the partition column on
top of that costs one to two minutes of CPU inside the sinfo client — past the
default --command.timeout, so the series disappeared entirely (#254).

Everything those two collectors need is already available more cheaply:

  - the per-node values come from `scontrol show nodes -o` (~0.4 s), which the
    exporter already fetches and caches for the nodes and reservation_nodes
    collectors, so this adds no RPC;
  - the state string comes from `sinfo -h -o "%N|%T"` (~1.1 s), one row per
    (partition, state) group, expanded with expandHostList.

The state deliberately stays on sinfo. scontrol's State= flags do not map one
to one onto sinfo's state string — ALLOCATED+RESERVED renders as "allocated"
while IDLE+RESERVED renders as "reserved" — so deriving it would change the
status label, and every dashboard and alert reads that label.
*/

// Field extraction is anchored on a word boundary, which is not decoration:
// `scontrol show nodes -o` puts State= and NextState= on the same line, and
// OS= carries spaces and a '#' ("OS=Linux 6.18.0 #1 SMP ... 2026"), so neither
// an unanchored "State=" match nor a whitespace split into key=value pairs
// survives real output.
var (
	scontrolNodeNameRe  = regexp.MustCompile(`(?:^|\s)NodeName=(\S+)`)
	scontrolCPUAllocRe  = regexp.MustCompile(`(?:^|\s)CPUAlloc=(\d+)`)
	scontrolCPUTotRe    = regexp.MustCompile(`(?:^|\s)CPUTot=(\d+)`)
	scontrolAllocMemRe  = regexp.MustCompile(`(?:^|\s)AllocMem=(\d+)`)
	scontrolRealMemRe   = regexp.MustCompile(`(?:^|\s)RealMemory=(\d+)`)
	scontrolStateRe     = regexp.MustCompile(`(?:^|\s)State=(\S+)`)
	scontrolPartsRe     = regexp.MustCompile(`(?:^|\s)Partitions=(\S+)`)
	scontrolReasonRe    = regexp.MustCompile(`(?:^|\s)Reason=`)
	scontrolReasonStamp = regexp.MustCompile(`^(.*) \[([^\[\]]+)@([^\[\]]+)\]`)
	scontrolNextFieldRe = regexp.MustCompile(` [A-Za-z_][A-Za-z0-9_]*=`)
)

// degradedStateFlags are the State= flags under which sinfo reports the free
// CPUs of a node as "other" rather than "idle". Verified against paired
// `sinfo -N` / `scontrol show nodes -o` captures taken at the same moment.
var degradedStateFlags = []string{"DRAIN", "DOWN", "FAIL", "INVALID_REG", "NOT_RESPONDING"}

// scontrolNode holds the per-node fields the node and drain_reason collectors
// read out of `scontrol show nodes -o`.
type scontrolNode struct {
	name        string
	cpuAlloc    uint64
	cpuTotal    uint64
	memAlloc    uint64
	memTotal    uint64
	state       string // compound State=, e.g. "IDLE+DRAIN"
	partitions  []string
	reason      string // Reason= with the trailing " [user@timestamp]" removed
	reasonSince string // the timestamp inside those brackets
}

// cpuIdleOther splits the node's free CPUs the way sinfo's CPUsState column
// does: on a degraded node they count as "other", otherwise as "idle".
func (n *scontrolNode) cpuIdleOther() (idle, other uint64) {
	var free uint64
	if n.cpuTotal > n.cpuAlloc {
		free = n.cpuTotal - n.cpuAlloc
	}
	for _, flag := range degradedStateFlags {
		if strings.Contains(n.state, flag) {
			return 0, free
		}
	}
	return free, 0
}

// ParseScontrolNodes parses `scontrol show nodes -o` output into per-node
// records, keyed by node name.
func ParseScontrolNodes(input []byte) map[string]*scontrolNode {
	nodes := make(map[string]*scontrolNode)

	for line := range strings.SplitSeq(string(input), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		m := scontrolNodeNameRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		n := &scontrolNode{name: m[1]}
		n.cpuAlloc = uintField(line, scontrolCPUAllocRe)
		n.cpuTotal = uintField(line, scontrolCPUTotRe)
		n.memAlloc = uintField(line, scontrolAllocMemRe)
		n.memTotal = uintField(line, scontrolRealMemRe)
		if s := scontrolStateRe.FindStringSubmatch(line); s != nil {
			n.state = s[1]
		}
		// A node can legitimately belong to no partition, in which case
		// scontrol omits Partitions= entirely and the node produces no
		// series — the same as under sinfo -N, where it has no row.
		if p := scontrolPartsRe.FindStringSubmatch(line); p != nil {
			for _, part := range strings.Split(p[1], ",") {
				// sinfo marks the default partition with "*", scontrol does
				// not; TrimRight keeps the label identical either way.
				if part = strings.TrimRight(strings.TrimSpace(part), "*"); part != "" {
					n.partitions = appendUnique(n.partitions, part)
				}
			}
		}
		n.reason, n.reasonSince = parseScontrolReason(line)
		nodes[n.name] = n
	}
	return nodes
}

// parseScontrolReason extracts Reason= and its timestamp, the equivalent of
// sinfo's %E and %H.
//
// Reason= is not the last field on the line: on a node inside a reservation,
// ReservationName= follows it, and the reason text itself contains spaces. The
// cut is therefore made at the " [user@timestamp]" suffix Slurm appends, not at
// the next whitespace — getting this wrong mis-parses every reserved node.
//
// What tells the suffix apart from brackets inside the reason itself is the
// "@": a bracket group without one, such as the "[eth0]" an NHC check reports,
// cannot match. The match is greedy on top of that, so the last qualifying
// group wins and a reason that does contain "[user@host]" keeps it.
func parseScontrolReason(line string) (reason, since string) {
	loc := scontrolReasonRe.FindStringIndex(line)
	if loc == nil {
		return "", ""
	}
	rest := line[loc[1]:]

	if m := scontrolReasonStamp.FindStringSubmatch(rest); m != nil {
		return strings.TrimSpace(m[1]), strings.TrimSpace(m[3])
	}

	// No " [user@timestamp]" suffix: stop at the next Key= field so a
	// following ReservationName= does not land inside the reason.
	if cut := scontrolNextFieldRe.FindStringIndex(rest); cut != nil {
		rest = rest[:cut[0]]
	}
	return strings.TrimSpace(rest), ""
}

func uintField(line string, re *regexp.Regexp) uint64 {
	m := re.FindStringSubmatch(line)
	if m == nil {
		return 0
	}
	v, err := strconv.ParseUint(m[1], 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// ── Node state string, from the grouped sinfo call ───────────────────────────

// ParseNodeStates parses `sinfo -h -o "%N|%T"` output into node -> state.
// Each row is a (partition, state) group whose node list is collapsed into a
// hostlist expression, so a node in several partitions appears once per
// partition with the same state; the last one wins, which is the same value.
func ParseNodeStates(input []byte) map[string]string {
	states := make(map[string]string)
	for line := range strings.SplitSeq(string(input), "\n") {
		hosts, state, found := strings.Cut(line, "|")
		if !found {
			continue
		}
		state = strings.TrimSpace(state)
		if state == "" {
			continue
		}
		for _, node := range expandHostList(hosts) {
			states[node] = state
		}
	}
	return states
}

// NodeStateData returns the node state string for every node, grouped.
//
// Without -N this is one row per (partition, state) group instead of one per
// (node, partition) pair: about a second on a large cluster against the minutes
// the per-node form costs (#254). Cached so that the node and drain_reason
// collectors share a single call per scrape.
func NodeStateData(log *logger.Logger) ([]byte, error) {
	data, err := sinfoNodeStateCache.GetOrFetch(func() ([]byte, error) {
		return Execute(log, "sinfo", []string{"-h", "-o", "%N|%T"})
	})
	updateCacheAge()
	return data, err
}

// ScontrolNodesData returns the output of `scontrol show nodes -o`, sharing
// scontrolNodesCache with the nodes and reservation_nodes collectors so the
// RPC is only sent once per scrape.
func ScontrolNodesData(log *logger.Logger) ([]byte, error) {
	data, err := scontrolNodesCache.GetOrFetch(func() ([]byte, error) {
		return Execute(log, "scontrol", []string{"show", "nodes", "-o"})
	})
	updateCacheAge()
	return data, err
}

// nodeStateFor returns the sinfo state string for a node, falling back to the
// primary scontrol flag when the grouped sinfo call did not list it. The
// fallback keeps the node's series alive — a dropped series is the failure this
// change exists to remove — and fromSinfo reports whether the grouped call had
// it, so the caller can say so in the log.
func nodeStateFor(n *scontrolNode, states map[string]string) (state string, fromSinfo bool) {
	if s, found := states[n.name]; found {
		return s, true
	}
	return strings.ToLower(strings.SplitN(n.state, "+", 2)[0]), false
}
