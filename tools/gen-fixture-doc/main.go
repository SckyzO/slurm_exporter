// Command gen-fixture-doc regenerates test_data/readme.md from the command
// registry in internal/collector.
//
// The readme used to be maintained by hand alongside the *Data() functions it
// described, and drifted from them: a fixed-width sinfo format the code had
// abandoned, a fixture deleted two releases earlier, a missing mandatory sacct
// flag (issue #195). Generating it removes the copy. The registry is checked
// against the real collectors by command_registry_test.go, and this file is
// checked against the registry by CI.
//
// Usage:
//
//	go generate ./...          # from anywhere in the module
//	go run ./tools/gen-fixture-doc -out test_data/readme.md
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sckyzo/slurm_exporter/v2/internal/collector"
)

func main() {
	out := flag.String("out", "test_data/readme.md", "path of the readme to write")
	check := flag.Bool("check", false, "exit non-zero if the file on disk is stale, write nothing")
	flag.Parse()

	want := render()

	if *check {
		got, err := os.ReadFile(*out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gen-fixture-doc: %v\n", err)
			os.Exit(1)
		}
		if !bytes.Equal(got, want) {
			fmt.Fprintf(os.Stderr,
				"gen-fixture-doc: %s is stale.\nRun `make generate` (or `go generate ./...`) and commit the result.\n", *out)
			os.Exit(1)
		}
		return
	}

	if err := os.WriteFile(*out, want, 0o644); err != nil { //nolint:gosec // G306: documentation, world-readable on purpose
		fmt.Fprintf(os.Stderr, "gen-fixture-doc: %v\n", err)
		os.Exit(1)
	}
}

// render builds the whole readme. Output is a pure function of the registry:
// no map iteration, no timestamps, no host data, so regenerating on an
// unchanged registry produces a byte-identical file and `git diff` stays a
// reliable staleness signal.
func render() []byte {
	var b strings.Builder

	// Written line by line rather than as one raw string: the prose is full of
	// markdown code spans, and a backtick cannot appear inside a Go raw string.
	writeLines(&b,
		"<!--",
		"GENERATED FILE. DO NOT EDIT.",
		"",
		"Source of truth: internal/collector/command_registry.go",
		"Regenerate with:  make generate   (or: go generate ./...)",
		"",
		"Editing this file by hand is pointless: CI regenerates it and fails on the diff.",
		"Change the registry instead. command_registry_test.go then proves that the",
		"registry still matches what the collectors actually run.",
		"-->",
		"",
		"# Slurm commands used by slurm_exporter",
		"",
		"Every Slurm CLI invocation the exporter makes, and the fixture under `test_data/`",
		"that protects it.",
		"",
		"This mapping is derived from `internal/collector/command_registry.go`, which is",
		"itself checked against the real collectors: `TestRegistryMatchesCollectors` invokes",
		"each `*Data()` function with `Execute` stubbed and compares what it genuinely",
		"passes to what the table declares. A command cannot change without this file",
		"changing with it.",
		"",
		"All fixtures are anonymised: cluster, node, user, account and reservation names",
		"are replaced with generic equivalents. See `CONTRIBUTING.md` § Test Data.",
		"",
		"",
	)

	renderIndex(&b)
	renderCommands(&b)
	renderCoverage(&b)
	renderFixtureDirs(&b)

	return []byte(b.String())
}

// writeLines appends each line followed by a newline.
func writeLines(b *strings.Builder, lines ...string) {
	for _, l := range lines {
		b.WriteString(l)
		b.WriteString("\n")
	}
}

func renderIndex(b *strings.Builder) {
	b.WriteString("## Index\n\n")
	b.WriteString("| Command | Binary | Owned by | Fixtures |\n")
	b.WriteString("|---|---|---|---|\n")
	for i := range collector.CommandRegistry {
		c := &collector.CommandRegistry[i]
		binary := c.Binary
		if len(c.EachBinary) > 0 {
			binary = fmt.Sprintf("%d binaries", len(c.EachBinary))
		}
		fixtures := "none"
		if n := len(c.Fixtures); n > 0 {
			fixtures = fmt.Sprintf("%d", n)
		}
		fmt.Fprintf(b, "| [`%s`](#%s) | `%s` | `%s` | %s |\n",
			c.Name, anchor(c.Name), binary, c.Source, fixtures)
	}
	b.WriteString("\n")
}

func renderCommands(b *strings.Builder) {
	b.WriteString("## Commands\n")
	for i := range collector.CommandRegistry {
		c := &collector.CommandRegistry[i]
		fmt.Fprintf(b, "\n### %s\n\n", c.Name)

		b.WriteString("```sh\n")
		for _, line := range commandLines(c) {
			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString("```\n\n")

		fmt.Fprintf(b, "%s\n\n", c.Doc)

		fmt.Fprintf(b, "Owned by `%s`.", c.Source)
		if len(c.Consumers) > 0 {
			fmt.Fprintf(b, " Also read by %s.", codeList(c.Consumers))
		}
		if c.OptIn != "" {
			fmt.Fprintf(b, " Runs only with `%s`.", c.OptIn)
		}
		b.WriteString("\n")

		if len(c.Notes) > 0 {
			b.WriteString("\n")
			for _, n := range c.Notes {
				fmt.Fprintf(b, "- %s\n", n)
			}
		}

		if len(c.Fixtures) == 0 {
			fmt.Fprintf(b, "\n**No fixture.** %s\n", c.NoFixtureReason)
			continue
		}

		b.WriteString("\n| Fixture | Slurm | What it protects |\n")
		b.WriteString("|---|---|---|\n")
		for _, f := range c.Fixtures {
			version := f.Slurm
			if version == "" {
				version = "unrecorded"
			}
			fmt.Fprintf(b, "| `%s` | %s | %s |\n", f.File, version, f.Why)
		}
	}
	b.WriteString("\n")
}

// renderCoverage names the commands running against no captured output. Left
// implicit, a gap reads as coverage, which is the habit #177 broke for the GPU
// matrix.
func renderCoverage(b *strings.Builder) {
	var gaps []*collector.Command
	for i := range collector.CommandRegistry {
		if c := &collector.CommandRegistry[i]; len(c.Fixtures) == 0 {
			gaps = append(gaps, c)
		}
	}

	b.WriteString("## Coverage gaps\n\n")
	if len(gaps) == 0 {
		b.WriteString("Every command in the registry has at least one captured fixture.\n\n")
		return
	}

	fmt.Fprintf(b, "%d of the %d commands run against no captured cluster output:\n\n",
		len(gaps), len(collector.CommandRegistry))
	b.WriteString("| Command | Owned by | Why |\n")
	b.WriteString("|---|---|---|\n")
	for _, c := range gaps {
		fmt.Fprintf(b, "| [`%s`](#%s) | `%s` | %s |\n", c.Name, anchor(c.Name), c.Source, c.NoFixtureReason)
	}
	b.WriteString("\n")
}

func renderFixtureDirs(b *strings.Builder) {
	b.WriteString("## Versioned GPU fixtures\n\n")
	b.WriteString("GPU `sinfo` output changed shape between Slurm releases, so the GRES parsers\n")
	b.WriteString("are exercised against a matrix of captures rather than a single one. The\n")
	b.WriteString("matrix is discovered at run time by `gpus_test.go`, which asserts coverage in\n")
	b.WriteString("both directions: a declared version with no fixture fails, and a fixture no\n")
	b.WriteString("version claims fails too. That symmetry exists because the glob once resolved\n")
	b.WriteString("to a directory that did not exist, so every test body was skipped while the\n")
	b.WriteString("suite still reported a pass (#148, then #176 and #177).\n\n")
	b.WriteString("`Supported` follows the window in `CONTRIBUTING.md` § Releasing: the newest\n")
	b.WriteString("Slurm major plus the previous three. It rolls with every Slurm release, so a\n")
	b.WriteString("directory dropping out of the window is expected. Those captures are kept as\n")
	b.WriteString("free regression protection rather than as peers of the supported ones.\n\n")

	b.WriteString("| Directory | Slurm | Supported | Notes |\n")
	b.WriteString("|---|---|---|---|\n")
	for _, d := range collector.FixtureDirs {
		supported := "no (historical)"
		if d.Supported {
			supported = "yes"
		}
		notes := d.Notes
		if notes == "" {
			notes = "—"
		}
		fmt.Fprintf(b, "| `%s/` | %s | %s | %s |\n", d.Path, d.Slurm, supported, notes)
	}
	b.WriteString("\n")
}

// commandLines renders one entry as copy-pasteable shell. An EachBinary entry
// becomes one line per binary, because that is what the exporter runs.
func commandLines(c *collector.Command) []string {
	binaries := c.EachBinary
	if len(binaries) == 0 {
		binaries = []string{c.Binary}
	}

	args := make([]string, 0, len(c.Args))
	for _, a := range c.Args {
		args = append(args, shellQuote(placeholderLabel(c, a)))
	}

	lines := make([]string, 0, len(binaries))
	for _, bin := range binaries {
		lines = append(lines, strings.TrimSpace(bin+" "+strings.Join(args, " ")))
	}
	return lines
}

// placeholderLabel turns a registry token into something a reader understands.
// The exact value is computed at call time, so the docs show its shape rather
// than a captured instant.
func placeholderLabel(c *collector.Command, arg string) string {
	for _, p := range c.Placeholders {
		if arg == p.Token {
			return "<" + strings.Trim(p.Token, "{}") + ">"
		}
	}
	return arg
}

// shellQuote quotes an argument only when a shell would otherwise mangle it.
// Slurm format strings are full of characters that need it: spaces in
// `--Format=Nodes: ,Gres:`, pipes in `%N|%E`, percent signs, parentheses.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t|*?%()$&;<>'\"\\`") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// codeList renders a slice as `a`, `b` and `c`.
func codeList(items []string) string {
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = "`" + s + "`"
	}
	if len(quoted) == 1 {
		return quoted[0]
	}
	return strings.Join(quoted[:len(quoted)-1], ", ") + " and " + quoted[len(quoted)-1]
}

// anchor is the GitHub heading slug for a command name. Registry names are
// lowercase words joined by underscores, which GitHub keeps verbatim, so a name
// is its own anchor. TestRegistryEntriesAreWellFormed pins that alphabet, so a
// name that would break these links fails there rather than producing a readme
// with dead internal links.
func anchor(name string) string { return name }
