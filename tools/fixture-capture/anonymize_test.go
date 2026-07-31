package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runAnonymize pipes input through anonymize.awk with the given mapping and
// returns the result. The awk program is the artifact that actually ships
// inside capture.sh, so the tests exercise it directly rather than a Go
// reimplementation that could drift from it.
func runAnonymize(t *testing.T, mapping, input string) string {
	t.Helper()
	if _, err := exec.LookPath("awk"); err != nil {
		t.Skip("awk not available")
	}

	dir := t.TempDir()
	mapPath := filepath.Join(dir, "map.tsv")
	if err := os.WriteFile(mapPath, []byte(mapping), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.CommandContext(t.Context(), "awk", "-v", "mapfile="+mapPath, "-f", "anonymize.awk")
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("awk failed: %v\n%s", err, out)
	}
	return string(out)
}

const testMap = "prefix\tkairosgh\tg\n" +
	"prefix\tkairoscomp\tc\n" +
	"word\talice\tuser1\n" +
	"word\thpc_team\taccount_a\n" +
	"word\tgh200\tmodel_a\n" +
	"word\tmaint_window\tresv1\n"

func TestAnonymizeRewritesNames(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{
			"expanded node name",
			"kairosgh0 425000 876896 144/144/0/288 mixed gpu",
			"g0 425000 876896 144/144/0/288 mixed gpu",
		},
		{
			// Slurm compresses node lists in scontrol output. Mapping the
			// prefix handles the compressed form without parsing range syntax.
			"compressed hostlist",
			"Nodes=kairosgh[0-3,7] NodeCnt=5",
			"Nodes=g[0-3,7] NodeCnt=5",
		},
		{
			"two prefixes on one line",
			"kairoscomp12,kairosgh4",
			"c12,g4",
		},
		{
			"GRES model",
			"gpu:gh200:4(S:0-3) gpu:gh200:2(IDX:0,3)",
			"gpu:model_a:4(S:0-3) gpu:model_a:2(IDX:0,3)",
		},
		{
			"user and account",
			"1234|hpc_team|alice|gpu|RUNNING",
			"1234|account_a|user1|gpu|RUNNING",
		},
		{
			"reservation name",
			"ReservationName=maint_window StartTime=2026-07-30T16:45:06",
			"ReservationName=resv1 StartTime=2026-07-30T16:45:06",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.TrimRight(runAnonymize(t, testMap, tc.in+"\n"), "\n")
			if got != tc.want {
				t.Errorf("\n in:   %s\n got:  %s\n want: %s", tc.in, got, tc.want)
			}
		})
	}
}

// TestAnonymizeLeavesSlurmTokensAlone is the test that matters most. A naive
// anonymiser eats a token that merely looks like a name, and the damage is
// silent: the fixture still parses, and every expected value derived from it is
// wrong from then on.
func TestAnonymizeLeavesSlurmTokensAlone(t *testing.T) {
	// "alice" is a mapped user, so the sentinels here deliberately include
	// strings that contain a mapped name as a substring.
	mapping := testMap + "word\tc1\tn1\n"

	untouched := []string{
		"(null)",
		"N/A",
		"drained*",
		"allocated+DRAIN",
		"idle mixed down maint reserved inval",
		"2026-07-30T16:45:06",
		"192/0/0/192",
		"gpu:tesla_v100-sxm3-32gb:16(IDX:0-15)",
		"Reason=Not responding",
		// c1 is mapped, c10 and c1x are not: a name must not be rewritten when
		// it is only the start of a longer one.
		"c10 c1x xc1",
		// alice is mapped; these are different names that contain it.
		"alice2 malice alice_backup",
	}

	for _, in := range untouched {
		got := strings.TrimRight(runAnonymize(t, mapping, in+"\n"), "\n")
		if got != in {
			t.Errorf("rewrote a token it should not have:\n got:  %s\n want: %s", got, in)
		}
	}
}

// TestAnonymizeIsDeterministic pins the property the whole scheme rests on: the
// same input and mapping produce the same output every time, and across files.
// Fixtures from different commands have to agree on what a node is called, or
// they stop describing the same cluster.
func TestAnonymizeIsDeterministic(t *testing.T) {
	in := "kairosgh0 alice hpc_team gpu:gh200:4\n"
	first := runAnonymize(t, testMap, in)
	for i := 0; i < 5; i++ {
		if got := runAnonymize(t, testMap, in); got != first {
			t.Fatalf("run %d differed:\n got:  %s want: %s", i, got, first)
		}
	}
}

// TestAnonymizeHandlesRealCaptures runs every committed fixture through the
// anonymiser with an empty mapping. Nothing should change: the fixtures are
// already anonymised, and a mapping that names nothing must be a no-op. This
// catches an anonymiser that mangles input on its own, independently of what it
// was asked to rewrite.
func TestAnonymizeHandlesRealCaptures(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "test_data", "*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	versioned, err := filepath.Glob(filepath.Join("..", "..", "test_data", "slurm-*", "*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, versioned...)
	if len(paths) < 20 {
		t.Fatalf("expected the fixture corpus, found %d files", len(paths))
	}

	for _, p := range paths {
		data, err := os.ReadFile(p) //nolint:gosec // G304: path from a glob under test_data/
		if err != nil {
			t.Fatal(err)
		}
		in := string(data)
		got := runAnonymize(t, "# empty mapping\n", in)
		// awk's print adds a trailing newline to a final line that lacked one.
		if strings.TrimRight(got, "\n") != strings.TrimRight(in, "\n") {
			t.Errorf("%s: an empty mapping changed the content", filepath.Base(p))
		}
	}
}
