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

/*
Each line is walked once and split into KEY=VALUE tokens, rather than searched
field by field. Eight anchored regexes over the same ~700-byte line cost about
340 ms on a 6,000-node cluster, against 14 ms for the sinfo output this
replaced; one pass brings it back into the same order of magnitude.

Walking it is also what makes the OS= field harmless. It carries spaces and a
'#' ("OS=Linux 6.18.0 #1 SMP PREEMPT_DYNAMIC Thu Jun 18 21:54:43 UTC 2026"),
so its words arrive as tokens with no '=' in them and are skipped — where a
naive whitespace split into key/value pairs would read "SMP" and "Thu" as
fields. The only token that genuinely has to be read differently is Reason=,
whose value contains spaces by nature.

Keys are matched whole, which is not decoration either: State= is a suffix of
NextState=, a field Slurm emits on a node pending reboot.
*/
var (
	// The " [user@timestamp]" suffix Slurm appends to a reason. Greedy, so
	// the last qualifying group wins; see parseScontrolReason.
	scontrolReasonStamp = regexp.MustCompile(`^(.*) \[([^\[\]]+)@([^\[\]]+)\]`)
	// The start of the next KEY= field, used to bound a reason that carries
	// no timestamp suffix.
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
		n := parseScontrolLine(line)
		if n == nil {
			continue
		}
		nodes[n.name] = n
	}
	return nodes
}

// parseScontrolReason splits everything after "Reason=" into the reason and
// its timestamp, the equivalent of sinfo's %E and %H.
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
func parseScontrolReason(rest string) (reason, since string) {
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

/*
parseScontrolLine reads one `scontrol show nodes -o` line in a single pass.
It returns nil for a line with no NodeName=, which is how a blank or
unexpected line is skipped rather than turned into a phantom node.

Tokens without an '=' are skipped, which is what keeps the words inside OS=
from being read as fields. Reason= is the one value that may contain spaces,
so it takes the rest of the line and is cut by parseScontrolReason.
*/
func parseScontrolLine(line string) *scontrolNode {
	n := &scontrolNode{}
	for i := 0; i < len(line); {
		for i < len(line) && line[i] == ' ' {
			i++
		}
		if i >= len(line) {
			break
		}
		start := i
		for i < len(line) && line[i] != ' ' && line[i] != '=' {
			i++
		}
		if i >= len(line) || line[i] != '=' {
			// Not a KEY=VALUE token — a word from inside OS=, say.
			for i < len(line) && line[i] != ' ' {
				i++
			}
			continue
		}
		key := line[start:i]
		i++ // past '='

		if key == "Reason" {
			// Takes the remainder: the reason text contains spaces, and on a
			// reserved node ReservationName= follows it.
			n.reason, n.reasonSince = parseScontrolReason(line[i:])
			break
		}

		valueStart := i
		for i < len(line) && line[i] != ' ' {
			i++
		}
		value := line[valueStart:i]

		switch key {
		case "NodeName":
			n.name = value
		case "CPUAlloc":
			n.cpuAlloc = parseUint(value)
		case "CPUTot":
			n.cpuTotal = parseUint(value)
		case "AllocMem":
			n.memAlloc = parseUint(value)
		case "RealMemory":
			n.memTotal = parseUint(value)
		case "State":
			n.state = value
		case "Partitions":
			for _, part := range strings.Split(value, ",") {
				// sinfo marks the default partition with "*", scontrol does
				// not; TrimRight keeps the label identical either way.
				if part = strings.TrimRight(part, "*"); part != "" {
					n.partitions = appendUnique(n.partitions, part)
				}
			}
		}
	}
	if n.name == "" {
		return nil
	}
	return n
}

func parseUint(s string) uint64 {
	v, err := strconv.ParseUint(s, 10, 64)
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
