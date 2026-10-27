package collector

import (
	"strconv"
	"strings"
)

// maxExpandedHosts caps how many names a single hostlist expression may
// produce. Slurm never emits anything close to it; the cap is there so a
// garbled line (`n[1-99999999]`) costs a truncated result instead of the
// exporter's memory.
const maxExpandedHosts = 1 << 20

/*
expandHostList expands a Slurm hostlist expression into individual node names.

Slurm groups nodes in its output: `sinfo -h -o "%N|%T"` prints one row per
(partition, state) group, with the nodes of that group collapsed into a single
expression. Expanding it here is what lets the node and drain_reason collectors
read a grouped sinfo call instead of the per-node `sinfo -N`, which costs
minutes on a large multi-partition cluster (#254).

The grammar it accepts, all of which Slurm produces:

	na1                 a plain name
	na1,nb2             a comma-separated list
	n[1-3]              a range
	n[1-3,7,9-10]       a set of ranges and singletons
	n[01-03]            zero-padded, padding preserved
	r[1-2]n[01-02]      several bracket groups in one name
	n[1-2]x             text after a bracket

Commas inside brackets belong to the range set, not to the top-level list, so
the split is bracket-aware.

An expression it cannot parse — an unbalanced bracket, a non-numeric or
descending range — is returned verbatim rather than dropped. A node missing
from the result would silently lose its series, which is the failure this
change exists to avoid; a node named oddly is visible and harmless.
*/
func expandHostList(s string) []string {
	var out []string
	for _, entry := range splitTopLevel(s) {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		out = expandEntry(entry, out)
		if len(out) >= maxExpandedHosts {
			return out
		}
	}
	return out
}

// splitTopLevel splits on commas that sit outside brackets.
func splitTopLevel(s string) []string {
	var parts []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}

// expandEntry expands one bracket group and recurses on what follows, so a
// name with several groups (r[1-2]n[01-02]) is fully expanded.
func expandEntry(entry string, out []string) []string {
	openIdx := strings.IndexByte(entry, '[')
	if openIdx < 0 {
		return append(out, entry)
	}
	closeIdx := strings.IndexByte(entry[openIdx:], ']')
	if closeIdx < 0 {
		// Unbalanced: keep the name as it came rather than lose the node.
		return append(out, entry)
	}
	closeIdx += openIdx

	prefix, set, rest := entry[:openIdx], entry[openIdx+1:closeIdx], entry[closeIdx+1:]
	values, ok := expandSet(set)
	if !ok {
		return append(out, entry)
	}
	for _, v := range values {
		if len(out) >= maxExpandedHosts {
			return out
		}
		out = expandEntry(prefix+v+rest, out)
	}
	return out
}

// expandSet expands the inside of one bracket group ("1-3,7,09-11") into its
// values, preserving zero padding. ok is false when the set is not a numeric
// range set, which tells the caller to keep the expression verbatim.
func expandSet(set string) (values []string, ok bool) {
	if set == "" {
		return nil, false
	}
	for _, item := range strings.Split(set, ",") {
		got, ok := expandSetItem(strings.TrimSpace(item))
		if !ok {
			return nil, false
		}
		values = append(values, got...)
	}
	return values, true
}

// expandSetItem expands one item of a bracket group: either a single number or
// a "lo-hi" range. ok is false when it is not one of those, which makes the
// caller keep the whole expression verbatim.
func expandSetItem(item string) (values []string, ok bool) {
	lo, hi, isRange := strings.Cut(item, "-")
	if !isRange {
		if !isDigits(item) {
			return nil, false
		}
		return []string{item}, true
	}
	if !isDigits(lo) || !isDigits(hi) {
		return nil, false
	}
	first, err := strconv.Atoi(lo)
	if err != nil {
		return nil, false
	}
	last, err := strconv.Atoi(hi)
	if err != nil || last < first || last-first+1 > maxExpandedHosts {
		return nil, false
	}
	// Slurm pads to the width the low bound was written with, so "[08-10]"
	// yields 08,09,10 while "[8-10]" yields 8,9,10.
	width := 0
	if len(lo) > 1 && lo[0] == '0' {
		width = len(lo)
	}
	for n := first; n <= last; n++ {
		s := strconv.Itoa(n)
		if width > 0 && len(s) < width {
			s = strings.Repeat("0", width-len(s)) + s
		}
		values = append(values, s)
	}
	return values, true
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
