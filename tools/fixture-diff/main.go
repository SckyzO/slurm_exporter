// Command fixture-diff reports how Slurm's output format differs between two
// versioned fixture directories.
//
// The version matrix proves the parsers still return the right numbers. It says
// nothing about *why* a version differs, and nothing at all about a format that
// changed in a way the parsers happen to survive today and will not tomorrow.
// This is the other half: a readable account of what Slurm started or stopped
// emitting between two releases.
//
// It reports on shape, never on values. Node counts, GPU counts and timestamps
// differ between two clusters for reasons that have nothing to do with Slurm,
// and a diff that flags those is a diff nobody reads twice.
//
// Usage:
//
//	go run ./tools/fixture-diff test_data/slurm-24.11.7 test_data/slurm-26.05.2
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: fixture-diff <old-dir> <new-dir>")
		os.Exit(2)
	}
	oldDir, newDir := os.Args[1], os.Args[2]

	report, differs, err := compare(oldDir, newDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fixture-diff: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(report)
	if differs {
		// A format change is news, not a failure: it means the fixtures need
		// looking at before the next release, which is the point of wiring this
		// into the release gate.
		os.Exit(3)
	}
}

// shape reduces one line to what a parser actually depends on: how many fields
// it has, which separators hold them apart, and which tokens are words rather
// than numbers. Two lines with the same shape parse the same way whatever the
// values are.
func shape(line string) string {
	if strings.TrimSpace(line) == "" {
		return ""
	}
	var b strings.Builder
	inNum, inWord, inSpace := false, false, false
	for _, r := range line {
		switch {
		case r >= '0' && r <= '9':
			if !inNum {
				b.WriteByte('#')
				inNum, inWord = true, false
			}
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_':
			if !inWord {
				b.WriteByte('W')
				inWord, inNum = true, false
			}
		case r == ' ' || r == '\t':
			// Runs of whitespace collapse to one. Slurm pads columns to the
			// width of the widest value, so two clusters produce different
			// padding for identical output — sdiag reported six shape changes
			// that were nothing but alignment. Parsers here split on fields,
			// not on column positions, so the run length carries no meaning.
			if !inSpace {
				b.WriteByte(' ')
			}
			inSpace, inNum, inWord = true, false, false
			continue
		default:
			// Separators and punctuation are the load-bearing part: a pipe
			// becoming a comma, or a new parenthesised suffix appearing, is
			// exactly the kind of change that breaks a parser silently.
			b.WriteRune(r)
			inNum, inWord = false, false
		}
		inSpace = false
	}
	return b.String()
}

// shapesOf returns the distinct line shapes in a file, sorted. Frequency is
// deliberately dropped: one cluster having more nodes than another is not a
// format change.
func shapesOf(path string) ([]string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path comes from a fixture directory given on the command line
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if s := shape(line); s != "" {
			seen[s] = true
		}
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out, nil
}

func listFixtures(dir string) (map[string]string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.txt"))
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(paths))
	for _, p := range paths {
		out[filepath.Base(p)] = p
	}
	return out, nil
}

func compare(oldDir, newDir string) (string, bool, error) {
	oldFiles, err := listFixtures(oldDir)
	if err != nil {
		return "", false, err
	}
	newFiles, err := listFixtures(newDir)
	if err != nil {
		return "", false, err
	}
	if len(oldFiles) == 0 && len(newFiles) == 0 {
		return "", false, fmt.Errorf("no .txt fixtures in either %s or %s", oldDir, newDir)
	}

	names := map[string]bool{}
	for n := range oldFiles {
		names[n] = true
	}
	for n := range newFiles {
		names[n] = true
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	var b strings.Builder
	differs := false
	fmt.Fprintf(&b, "Format differences: %s → %s\n\n", filepath.Base(oldDir), filepath.Base(newDir))

	for _, name := range sorted {
		oldPath, inOld := oldFiles[name]
		newPath, inNew := newFiles[name]

		switch {
		case !inOld:
			fmt.Fprintf(&b, "  %-28s only in %s\n", name, filepath.Base(newDir))
			differs = true
			continue
		case !inNew:
			fmt.Fprintf(&b, "  %-28s only in %s\n", name, filepath.Base(oldDir))
			differs = true
			continue
		}

		oldShapes, err := shapesOf(oldPath)
		if err != nil {
			return "", false, err
		}
		newShapes, err := shapesOf(newPath)
		if err != nil {
			return "", false, err
		}

		gone := missing(oldShapes, newShapes)
		added := missing(newShapes, oldShapes)
		if len(gone) == 0 && len(added) == 0 {
			fmt.Fprintf(&b, "  %-28s same shape\n", name)
			continue
		}

		differs = true
		fmt.Fprintf(&b, "  %-28s SHAPE CHANGED\n", name)
		for _, s := range gone {
			fmt.Fprintf(&b, "      gone   %s\n", s)
		}
		for _, s := range added {
			fmt.Fprintf(&b, "      new    %s\n", s)
		}
	}

	fmt.Fprintf(&b, "\n  # = digits, W = letters; everything else is literal.\n")
	if differs {
		fmt.Fprintf(&b, "  A shape change is not automatically a bug — but every parser reading\n")
		fmt.Fprintf(&b, "  those files needs a look before the release goes out.\n")
	}
	return b.String(), differs, nil
}

func missing(from, in []string) []string {
	have := make(map[string]bool, len(in))
	for _, s := range in {
		have[s] = true
	}
	var out []string
	for _, s := range from {
		if !have[s] {
			out = append(out, s)
		}
	}
	return out
}
