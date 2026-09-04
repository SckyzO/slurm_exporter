package collector

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sckyzo/slurm_exporter/v2/internal/logger"
)

// testDataDir is where the fixtures live, relative to this package.
const testDataDir = "../../test_data"

// placeholderToken matches the {{name}} form used in Command.Args for values
// the exporter computes at call time.
var placeholderToken = regexp.MustCompile(`{{[a-z]+}}`)

// resetSharedCaches gives the test fresh instances of the caches that sit
// between a collector and Execute, so invoking two entries backed by the same
// cache reaches Execute twice instead of once. Without it, whichever of
// scontrol_nodes and squeue_jobs ran second would observe no call at all, and
// the contract would silently stop being checked.
func resetSharedCaches(t *testing.T) {
	t.Helper()
	oldNodes, oldJobs := scontrolNodesCache, squeueJobsCache
	t.Cleanup(func() {
		scontrolNodesCache, squeueJobsCache = oldNodes, oldJobs
	})
	scontrolNodesCache = &timedCache{ttl: oldNodes.ttl}
	squeueJobsCache = &timedCache{ttl: oldJobs.ttl}
}

// observeCommand runs one registry entry through its real production path with
// Execute stubbed, and reports what that path actually asked for.
func observeCommand(t *testing.T, cmd Command, binary string) (string, []string) {
	t.Helper()
	resetSharedCaches(t)

	var (
		gotBin  string
		gotArgs []string
		calls   int
	)
	old := Execute
	t.Cleanup(func() { Execute = old })
	Execute = func(_ *logger.Logger, command string, args []string) ([]byte, error) {
		calls++
		gotBin, gotArgs = command, args
		return nil, nil
	}

	log, _ := bufferLogger()
	cmd.invoke(log, binary)

	require.Equalf(t, 1, calls,
		"%s reached Execute %d times, expected exactly once — a registry entry maps to "+
			"one command, so either the collector changed or the entry needs splitting",
		cmd.Name, calls)
	return gotBin, gotArgs
}

// requireArgsMatch compares observed arguments to the table, accepting any
// value in a placeholder position as long as it has the declared shape.
func requireArgsMatch(t *testing.T, cmd Command, got []string) {
	t.Helper()

	byToken := make(map[string]Placeholder, len(cmd.Placeholders))
	for _, p := range cmd.Placeholders {
		byToken[p.Token] = p
	}

	require.Equalf(t, len(cmd.Args), len(got),
		"%s passes %d arguments, the registry declares %d\n  registry: %v\n  code:     %v",
		cmd.Name, len(got), len(cmd.Args), cmd.Args, got)

	for i, want := range cmd.Args {
		if p, ok := byToken[want]; ok {
			require.Regexpf(t, p.Match, got[i],
				"%s argument %d is the %s placeholder, but %q does not have the declared shape",
				cmd.Name, i, p.Token, got[i])
			continue
		}
		require.Equalf(t, want, got[i],
			"%s argument %d drifted\n  registry: %q\n  code:     %q",
			cmd.Name, i, want, got[i])
	}
}

// TestRegistryMatchesCollectors is what makes the registry a mirror of the code
// rather than a fourth copy of it. Every entry is invoked through its real
// *Data() path, and what that path passes to Execute has to be what the table
// says. Changing a Slurm command without updating the table fails here.
func TestRegistryMatchesCollectors(t *testing.T) {
	for _, cmd := range CommandRegistry {
		t.Run(cmd.Name, func(t *testing.T) {
			require.NotNilf(t, cmd.invoke, "%s declares no invoke func, so nothing checks it", cmd.Name)

			binaries := cmd.EachBinary
			if len(binaries) == 0 {
				binaries = []string{cmd.Binary}
			}
			for _, binary := range binaries {
				gotBin, gotArgs := observeCommand(t, cmd, binary)
				require.Equalf(t, binary, gotBin, "%s ran the wrong binary", cmd.Name)
				requireArgsMatch(t, cmd, gotArgs)
			}
		})
	}
}

// TestRegistryEntriesAreWellFormed pins the invariants the doc and capture
// generators rely on, so a malformed entry fails here rather than producing a
// broken readme or an unrunnable capture script.
func TestRegistryEntriesAreWellFormed(t *testing.T) {
	seen := make(map[string]bool, len(CommandRegistry))

	for _, cmd := range CommandRegistry {
		t.Run(cmd.Name, func(t *testing.T) {
			require.NotEmpty(t, cmd.Name, "every entry needs a name: it is the doc anchor and the capture step id")
			require.Falsef(t, seen[cmd.Name], "duplicate registry name %q", cmd.Name)
			seen[cmd.Name] = true
			// The name is used verbatim as a GitHub heading anchor in the
			// generated readme and as a shell identifier in the generated
			// capture script; both stop working outside this alphabet.
			require.Regexpf(t, `^[a-z0-9_]+$`, cmd.Name,
				"registry name %q must be lowercase words joined by underscores", cmd.Name)

			require.NotEmptyf(t, cmd.Source, "%s must name the collector file that owns it", cmd.Name)
			require.NotEmptyf(t, cmd.Doc, "%s must say what it returns: the readme is generated from this", cmd.Name)

			if len(cmd.EachBinary) == 0 {
				require.NotEmptyf(t, cmd.Binary, "%s must set Binary or EachBinary", cmd.Name)
			} else {
				require.Emptyf(t, cmd.Binary, "%s sets both Binary and EachBinary", cmd.Name)
			}

			// Every placeholder token used must be declared, and every declared
			// placeholder must be used — an undeclared token would be compared
			// literally and an unused one would silently rot.
			declared := make(map[string]bool, len(cmd.Placeholders))
			for _, p := range cmd.Placeholders {
				require.NotNilf(t, p.Match, "%s placeholder %s has no Match", cmd.Name, p.Token)
				require.NotEmptyf(t, p.Shell, "%s placeholder %s has no Shell expression, so it cannot be captured", cmd.Name, p.Token)
				declared[p.Token] = true
			}
			used := make(map[string]bool)
			for _, arg := range cmd.Args {
				for _, tok := range placeholderToken.FindAllString(arg, -1) {
					require.Truef(t, declared[tok], "%s uses undeclared placeholder %s", cmd.Name, tok)
					used[tok] = true
				}
			}
			for tok := range declared {
				require.Truef(t, used[tok], "%s declares placeholder %s but never uses it", cmd.Name, tok)
			}

			for _, f := range cmd.Fixtures {
				require.NotEmptyf(t, f.Why, "%s fixture %s must say what it protects", cmd.Name, f.File)
			}

			// A command with no capture behind it is a coverage gap. Requiring
			// the reason in the table is what stops the gap from reading as
			// coverage in the generated readme.
			if len(cmd.Fixtures) == 0 {
				require.NotEmptyf(t, cmd.NoFixtureReason,
					"%s has no fixture: set NoFixtureReason to say whether that is deliberate "+
						"or work still to do", cmd.Name)
			} else {
				require.Emptyf(t, cmd.NoFixtureReason,
					"%s has %d fixtures but still carries a NoFixtureReason",
					cmd.Name, len(cmd.Fixtures))
			}
		})
	}
}

// TestRegistryFixturesExist catches the drift that sent a contributor to a file
// deleted two releases earlier: test_data/sinfo.txt was removed by d52d93f
// (#100) and the documentation kept pointing at it (issue #195, drift 2).
func TestRegistryFixturesExist(t *testing.T) {
	for _, cmd := range CommandRegistry {
		for _, f := range cmd.Fixtures {
			path := filepath.Join(testDataDir, f.File)
			_, err := os.Stat(path)
			require.NoErrorf(t, err,
				"%s declares fixture %s, which is not on disk — either capture it or drop the entry",
				cmd.Name, f.File)
		}
	}
}

// TestEveryFixtureIsClaimed is the other direction. A capture nobody documents
// cannot be traced back to the bug it protects, and six of the twenty fixtures
// were in that state (issue #195, drift 7). Adding a file under test_data/ now
// means saying why it is there.
func TestEveryFixtureIsClaimed(t *testing.T) {
	claimed := make(map[string]bool)
	for _, cmd := range CommandRegistry {
		for _, f := range cmd.Fixtures {
			claimed[filepath.ToSlash(f.File)] = true
		}
	}
	dirs := make(map[string]bool, len(FixtureDirs))
	for _, d := range FixtureDirs {
		dirs[d.Path] = true
	}

	entries, err := os.ReadDir(testDataDir)
	require.NoError(t, err)

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			require.Truef(t, dirs[name],
				"test_data/%s/ is not described in FixtureDirs: say which Slurm release it "+
					"was captured on and whether that release is still in the support window", name)
			continue
		}
		if name == "readme.md" {
			continue
		}
		require.Truef(t, claimed[name],
			"test_data/%s is read by a test but no registry entry claims it — add it to the "+
				"owning command's Fixtures with a Why, or delete it", name)
	}

	// Versioned fixtures named individually must sit in a declared directory.
	for file := range claimed {
		if dir, _, found := strings.Cut(file, "/"); found {
			require.Truef(t, dirs[dir],
				"fixture %s lives in test_data/%s/, which is not declared in FixtureDirs", file, dir)
		}
	}
}

// TestFixtureDirsExist keeps the versioned matrix honest in the direction the
// dynamic discovery in gpus_test.go cannot: a directory declared here but
// absent from disk would silently document coverage that does not exist.
func TestFixtureDirsExist(t *testing.T) {
	for _, d := range FixtureDirs {
		info, err := os.Stat(filepath.Join(testDataDir, d.Path))
		require.NoErrorf(t, err, "FixtureDirs declares test_data/%s/, which is not on disk", d.Path)
		require.Truef(t, info.IsDir(), "test_data/%s is declared as a fixture directory but is a file", d.Path)
		require.NotEmptyf(t, d.Slurm, "test_data/%s/ must record the Slurm release it was captured on", d.Path)
	}
}
