// Command fixture-capture generates capture.sh, the script that collects Slurm
// fixtures on a cluster.
//
// The script is generated rather than written by hand for the same reason
// test_data/readme.md is: a hand-maintained list of Slurm commands drifts from
// the collectors that run them, and that drift is what issue #195 is about. The
// command list comes from collector.CommandRegistry, which a test already pins
// against the real *Data() functions.
//
// The generated script is committed. CI regenerates it and fails on a diff, so
// adding a collector without regenerating cannot land.
//
// Usage:
//
//	go generate ./...
//	go run ./tools/fixture-capture -out scripts/capture.sh
//	go run ./tools/fixture-capture -check
package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sckyzo/slurm_exporter/v2/internal/collector"
)

// anonymizeAWK is embedded so the generated script is self-contained. A capture
// script that needs a second file alongside it is a capture script someone
// eventually runs without that file.
//
//go:embed anonymize.awk
var anonymizeAWK string

func main() {
	out := flag.String("out", "scripts/capture.sh", "path of the script to write")
	check := flag.Bool("check", false, "exit non-zero if the file on disk is stale, write nothing")
	flag.Parse()

	want := render()

	if *check {
		got, err := os.ReadFile(*out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fixture-capture: %v\n", err)
			os.Exit(1)
		}
		if !bytes.Equal(got, want) {
			fmt.Fprintf(os.Stderr,
				"fixture-capture: %s is stale.\nRun `make generate` and commit the result.\n", *out)
			os.Exit(1)
		}
		return
	}

	if err := os.WriteFile(*out, want, 0o755); err != nil { //nolint:gosec // G306: it is a script, it has to be executable
		fmt.Fprintf(os.Stderr, "fixture-capture: %v\n", err)
		os.Exit(1)
	}
}

// shQuote wraps a value in single quotes for POSIX sh. Slurm format strings are
// full of characters a shell would otherwise act on: spaces in
// "--Format=Nodes: ,Gres:", pipes in "%N|%E", percent signs, parentheses.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// captureStep is one command as the generated script sees it.
type captureStep struct {
	Name   string
	Binary string
	Args   string // already shell-quoted, empty when the command takes none
	// Expensive marks a command that queries SlurmDBD. Not "has an opt-in
	// flag": queue_default_states carries one too, but it is a *disable* flag
	// on a cheap controller query, and grouping on the flag's presence put it
	// behind --with-sacct where it does not belong.
	Expensive bool
}

// steps flattens the registry into what the script needs. Placeholders become
// their shell expression, which is the whole reason Placeholder carries one.
func steps() []captureStep {
	var out []captureStep
	for i := range collector.CommandRegistry {
		c := &collector.CommandRegistry[i]

		// A per-binary version probe tells you the version of whichever host
		// runs the capture, not a format. It is recorded in the provenance file
		// instead, where it belongs.
		if len(c.EachBinary) > 0 {
			continue
		}

		shell := make(map[string]string, len(c.Placeholders))
		for _, p := range c.Placeholders {
			shell[p.Token] = p.Shell
		}

		parts := make([]string, 0, len(c.Args))
		for _, a := range c.Args {
			if expr, ok := shell[a]; ok {
				parts = append(parts, `"`+expr+`"`)
				continue
			}
			parts = append(parts, shQuote(a))
		}

		out = append(out, captureStep{
			Name:      c.Name,
			Binary:    c.Binary,
			Args:      strings.Join(parts, " "),
			Expensive: c.Binary == "sacct",
		})
	}
	return out
}

func render() []byte {
	var b strings.Builder
	w := func(lines ...string) {
		for _, l := range lines {
			b.WriteString(l)
			b.WriteString("\n")
		}
	}

	all := steps()
	var expensive, always []captureStep
	for _, s := range all {
		if s.Expensive {
			expensive = append(expensive, s)
		} else {
			always = append(always, s)
		}
	}

	w(
		"#!/bin/sh",
		"# GENERATED FILE. DO NOT EDIT.",
		"#",
		"# Source of truth: internal/collector/command_registry.go",
		"# Regenerate with:  make generate",
		"#",
		"# Collect Slurm fixtures for slurm_exporter, anonymised on this machine.",
		"#",
		"# What it does, and what it deliberately does not:",
		"#",
		"#   * Runs only the commands the exporter itself runs. The list is generated",
		"#     from the collector registry, so it cannot include a command the exporter",
		"#     does not use, and every one of them is read-only. There is no write",
		"#     command in the registry, so none can appear here.",
		"#   * Anonymises before writing anything. Node, user, account, reservation and",
		"#     GRES-model names are replaced on this machine; the tarball never contains",
		"#     them. That is what makes it safe to run on a production cluster and hand",
		"#     the result to someone else.",
		"#   * Records provenance: Slurm version, date, and the exit status of every",
		"#     command. A partial capture is visible rather than silent.",
		"#   * Leaves sacct alone unless asked. It queries SlurmDBD and is expensive on",
		"#     a busy cluster, so it is behind --with-sacct and bounded to one hour.",
		"#",
		"# Usage:  ./capture.sh [-o OUTDIR] [--with-sacct]",
		"",
		"set -eu",
		"",
		"OUTDIR=${OUTDIR:-slurm-fixtures-$(date +%Y%m%d-%H%M%S)}",
		"WITH_SACCT=0",
		"",
		"while [ $# -gt 0 ]; do",
		"    case $1 in",
		"        -o|--out)     OUTDIR=$2; shift 2 ;;",
		"        --with-sacct) WITH_SACCT=1; shift ;;",
		"        -h|--help)    sed -n '2,25p' \"$0\"; exit 0 ;;",
		"        *)            echo \"unknown option: $1\" >&2; exit 2 ;;",
		"    esac",
		"done",
		"",
		"command -v sinfo >/dev/null 2>&1 || { echo 'sinfo not found in PATH' >&2; exit 1; }",
		"mkdir -p \"$OUTDIR\"",
		"",
		"# The exporter pins this on every command it runs (issue #158). Slurm renders",
		"# every timestamp according to it, and a capture taken under a different",
		"# setting comes back in a layout the parsers reject — so the fixture would",
		"# encode a format the exporter never sees.",
		"SLURM_TIME_FORMAT=standard",
		"export SLURM_TIME_FORMAT",
		"",
		"MAP=\"$OUTDIR/.anonymize.map\"",
		"AWK=\"$OUTDIR/.anonymize.awk\"",
		"PROV=\"$OUTDIR/provenance.txt\"",
		"",
		"# ── Anonymisation map ────────────────────────────────────────────────────────",
		"#",
		"# Built by asking Slurm what exists, so only real names are rewritten and no",
		"# token is rewritten by resemblance. Node names map by alphabetic prefix, which",
		"# covers both the expanded form (c1) and the compressed hostlists Slurm writes",
		"# elsewhere (c[1-10,14]) without parsing range syntax.",
		"",
		"build_map() {",
		"    : > \"$MAP\"",
		"",
		"    # The replacement must not end in a digit: a node prefix is immediately",
		"    # followed by the node number, so mapping c -> n1 turns c1 into n11, which",
		"    # reads as node eleven. Letters keep the numbering intact: c1 -> na1.",
		"    sinfo -h -N -o '%N' 2>/dev/null | sed 's/[0-9].*$//' | sort -u | grep -v '^$' |",
		"        awk '{ printf \"prefix\\t%s\\tn%c\\n\", $1, 96 + NR }' >> \"$MAP\"",
		"",
		"    { squeue -a -h -o '%u' 2>/dev/null; sshare -a -P -n -o User 2>/dev/null; } |",
		"        tr -d ' ' | sort -u | grep -v '^$' |",
		"        awk '{ printf \"word\\t%s\\tuser%d\\n\", $1, NR }' >> \"$MAP\"",
		"",
		"    sshare -a -P -n -o Account 2>/dev/null | tr -d ' ' | sort -u | grep -v '^$' |",
		"        awk '{ printf \"word\\t%s\\taccount%d\\n\", $1, NR }' >> \"$MAP\"",
		"",
		"    scontrol show reservation 2>/dev/null |",
		"        sed -n 's/.*ReservationName=\\([^ ]*\\).*/\\1/p' | sort -u | grep -v '^$' |",
		"        awk '{ printf \"word\\t%s\\tresv%d\\n\", $1, NR }' >> \"$MAP\"",
		"",
		"    # GRES models: the part between the type and the count in gpu:<model>:<n>.",
		"    # A model name identifies the hardware a site bought, which is site data.",
		"    sinfo -h -N -O 'Gres: ,GresUsed:' 2>/dev/null |",
		"        tr ' ,' '\\n\\n' | sed -n 's/^[a-z]*:\\([A-Za-z][A-Za-z0-9_.-]*\\):[0-9].*/\\1/p' |",
		"        sort -u | grep -v '^$' |",
		"        awk '{ printf \"word\\t%s\\tmodel_%c\\n\", $1, 96 + NR }' >> \"$MAP\"",
		"",
		"    printf 'entities to rewrite: %s\\n' \"$(wc -l < \"$MAP\" | tr -d ' ')\"",
		"}",
		"",
		"# ── Embedded anonymiser ──────────────────────────────────────────────────────",
		"write_awk() {",
		"    cat > \"$AWK\" <<'ANONEOF'",
	)

	for _, l := range strings.Split(strings.TrimRight(anonymizeAWK, "\n"), "\n") {
		w(l)
	}

	w(
		"ANONEOF",
		"}",
		"",
		"# ── Capture ──────────────────────────────────────────────────────────────────",
		"",
		"# run_step executes one command, anonymises its output and records whether it",
		"# worked. A command that fails writes an empty file and a FAILED line rather",
		"# than aborting: a partial capture is still useful, as long as it says so.",
		"run_step() {",
		"    name=$1; shift",
		"    if \"$@\" > \"$OUTDIR/.raw\" 2>\"$OUTDIR/.err\"; then",
		"        status=ok",
		"    else",
		"        status=FAILED",
		"    fi",
		"    awk -v mapfile=\"$MAP\" -f \"$AWK\" < \"$OUTDIR/.raw\" > \"$OUTDIR/$name.txt\"",
		"    printf '%-22s %-8s %s\\n' \"$name\" \"$status\" \"$*\" >> \"$PROV\"",
		"    [ \"$status\" = ok ] || printf '    stderr: %s\\n' \"$(head -1 \"$OUTDIR/.err\")\" >> \"$PROV\"",
		"}",
		"",
		"write_awk",
		"build_map",
		"",
		"{",
		"    echo 'slurm_exporter fixture capture'",
		"    echo \"date:    $(date -u +%Y-%m-%dT%H:%M:%SZ)\"",
		"    echo \"slurm:   $(sinfo --version 2>/dev/null)\"",
		"    echo \"host:    $(hostname | cksum | cut -d' ' -f1)  (hashed)\"",
		"    echo \"sacct:   $([ \"$WITH_SACCT\" = 1 ] && echo included || echo skipped)\"",
		"    echo",
		"    echo 'command                status   invocation'",
		"} > \"$PROV\"",
		"",
	)

	for _, s := range always {
		w(strings.TrimRight(fmt.Sprintf("run_step %-22s %s %s", s.Name, s.Binary, s.Args), " "))
	}
	w("")
	if len(expensive) > 0 {
		w("if [ \"$WITH_SACCT\" = 1 ]; then")
		for _, s := range expensive {
			w(strings.TrimRight(fmt.Sprintf("    run_step %-22s %s %s", s.Name, s.Binary, s.Args), " "))
		}
		w("fi", "")
	}

	w(
		"rm -f \"$OUTDIR/.raw\" \"$OUTDIR/.err\" \"$AWK\"",
		"",
		"# The mapping stays on the cluster. It is the one file that could undo the",
		"# anonymisation, so it never goes in the tarball.",
		"mv \"$MAP\" \"./$(basename \"$OUTDIR\").map\" 2>/dev/null || rm -f \"$MAP\"",
		"",
		"tar czf \"$OUTDIR.tar.gz\" \"$OUTDIR\"",
		"echo",
		"echo \"captured: $OUTDIR.tar.gz\"",
		"echo \"mapping kept locally, not in the tarball: $(basename \"$OUTDIR\").map\"",
		"cat \"$PROV\"",
	)

	return []byte(b.String())
}
