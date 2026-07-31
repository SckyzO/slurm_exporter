package collector

import (
	"regexp"
	"time"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// ── The command contract ─────────────────────────────────────────────────────
//
// CommandRegistry is the single source of truth for every Slurm CLI invocation
// the exporter makes.
//
// This list used to exist in three places — the *Data() functions, the prose in
// test_data/readme.md, and the memory of whoever last captured a fixture — with
// nothing checking them against each other. They drifted: the readme documented
// a fixed-width sinfo format that the code had already abandoned, pointed at a
// fixture deleted two releases earlier, and omitted the mandatory --endtime on
// sacct. A contributor followed the documentation and had to redo the work
// (issue #195).
//
// Everything is now derived from this table:
//
//   - command_registry_test.go asserts every entry against what the real
//     *Data() function actually passes to Execute, so changing a command
//     without updating the table fails CI;
//   - tools/gen-fixture-doc regenerates test_data/readme.md from it;
//   - tools/fixture-capture generates the cluster capture script from it.
//
// Adding a Slurm call means adding an entry here. Nothing else is maintained
// by hand.

// Fixture is one captured file under test_data/ and the reason it exists.
//
// Why carries most of the value here. Six of the fixtures on disk are the
// regression nets for the subtlest bugs this exporter has had (long-name
// truncation, the default-partition marker, long GRES strings, relative
// reservation timestamps, the empty reservation list), and until issue #195
// none could be traced back to its bug by reading the documentation.
type Fixture struct {
	// File is the path relative to test_data/.
	File string
	// Why states what this capture protects: the bug it caught, or the format
	// quirk it pins. One sentence, written for whoever has to decide years from
	// now whether the file can still be deleted.
	Why string
	// Slurm is the version the capture was taken on. Empty means the
	// provenance was never recorded; new fixtures must set it.
	Slurm string
}

// Placeholder marks an argument the exporter computes at call time, so the
// contract test can match it and the capture script can reproduce it.
type Placeholder struct {
	// Token is the literal that stands in for the value in Command.Args.
	Token string
	// Match is what a real value looks like. The contract test accepts any
	// argument matching it in the token's position.
	Match *regexp.Regexp
	// Shell is a POSIX sh expression producing an equivalent value, used by the
	// generated capture script. GNU date is assumed: every supported Slurm
	// controller platform ships it.
	Shell string
}

// Command is one Slurm CLI invocation made by the exporter.
type Command struct {
	// Name is a stable identifier, used as the anchor in generated docs and as
	// the fixture-capture step name. Renaming one is a documentation change.
	Name string
	// Binary is the Slurm executable. Empty when EachBinary is set.
	Binary string
	// EachBinary makes the command run once per listed binary, with Binary
	// bound to each in turn. Only slurm_binary_info.go needs it.
	EachBinary []string
	// Args are the exact arguments passed to Execute, with Placeholder tokens
	// standing in for values computed at call time. nil means no arguments.
	Args []string
	// Placeholders declares every token appearing in Args.
	Placeholders []Placeholder
	// Source is the collector file that owns the call.
	Source string
	// Consumers are the other collector files that read the same output,
	// usually through a shared cache.
	Consumers []string
	// Doc is the prose for the generated readme: what the command returns and
	// why it is shaped this way.
	Doc string
	// Notes are the caveats worth carrying next to the command in the docs.
	Notes []string
	// OptIn names the flag that must be set for the command to run at all.
	// Empty means the command runs on every scrape by default.
	OptIn string
	// Fixtures are the captures under test_data/ that back this command.
	Fixtures []Fixture
	// NoFixtureReason is required when Fixtures is empty, and forbidden
	// otherwise. A command running against no captured output is a coverage
	// gap, and a gap left implicit reads as coverage, so the generated readme
	// names every one of them and says whether it is a deliberate choice or
	// work still to do. Same principle #177 applied to the GPU matrix: an
	// absent fixture has to be asserted rather than assumed.
	NoFixtureReason string
	// invoke calls the real production path so the contract test observes what
	// the collector genuinely passes to Execute, rather than a copy of it.
	// Unexported: only the in-package test needs it, and it keeps the table
	// consumable by tools/ without exporting a way to shell out.
	invoke func(log *logger.Logger, binary string)
}

// slurmTimestamp is the layout Slurm accepts for --starttime / --endtime, and
// the one sacct_efficiency.go formats with.
var slurmTimestamp = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}$`)

// versionedBinaries are the binaries slurm_binary_info.go probes for a version:
// the required set, always reported, plus the job-submission tools, reported
// only when present. sshare is deliberately absent: the fairshare collector
// executes it, but nothing ever probes its version.
var versionedBinaries = []string{
	"sinfo", "squeue", "sdiag", "scontrol", "sacct",
	"sbatch", "salloc", "srun",
}

// CommandRegistry lists every Slurm CLI invocation the exporter makes.
//
//go:generate go run ../../tools/gen-fixture-doc -out ../../test_data/readme.md
var CommandRegistry = []Command{
	{
		Name:   "squeue_jobs",
		Binary: "squeue",
		Args:   []string{"-a", "-r", "-h", "-O", squeueJobsColumns},
		Source: "squeue_jobs.go",
		Consumers: []string{
			"accounts.go", "users.go", "partitions.go",
		},
		Doc: "One consolidated snapshot of the whole job queue, cached per scrape and " +
			"shared by the accounts, users and partitions collectors. Before issue #144 " +
			"these issued up to five separate full-queue dumps to slurmctld every scrape; " +
			"they now project their views from this single call. The -a -r flags and the " +
			"default state set match what each collector requested individually, so no " +
			"metric value changes.",
		Notes: []string{
			"queue.go is deliberately NOT a consumer: it omits -a/-r and toggles " +
				"--states=all, and folding it in here would change job-array counts.",
			"The trailing colon on every field forces variable-width columns. Without " +
				"it squeue caps a field at 20 characters and silently drops the tail " +
				"(issues #10 and #35).",
			"tres-alloc (effective total allocation) is used instead of the legacy %b " +
				"(TRES per node) so jobs submitted with --gpus or --gpus-per-node are " +
				"accounted for (issue #35).",
		},
		Fixtures: []Fixture{
			{
				File: "squeue_jobs.txt",
				Why: "The consolidated layout itself: RUNNING/PENDING/SUSPENDED jobs across " +
					"several accounts, users and partitions, plus a multi-partition (cpu,gpu) " +
					"pending job that the partitions projection has to split.",
				Slurm: "25.11",
			},
			{
				File: "squeue_jobs_accounts_view.txt",
				Why: "Backs ParseAccountsMetrics, which the accounts projection re-emits " +
					"verbatim: proves the projection produces the layout the parser expects.",
			},
			{
				File: "squeue_jobs_users_view.txt",
				Why: "Same contract as squeue_jobs_accounts_view.txt for the users projection " +
					"(ParseUsersMetrics).",
			},
		},
		invoke: func(log *logger.Logger, _ string) { _, _ = SqueueJobsData(log) },
	},
	{
		Name:   "queue_all_states",
		Binary: "squeue",
		Args:   []string{"-h", "-o", "%P|%T|%C|%r|%u", "--states=all"},
		Source: "queue.go",
		Doc: "Job states, cores, reason and user, pipe-delimited so commas inside the " +
			"reason field stay inside their column. --states=all is what brings the " +
			"terminal states into the output: squeue reports only pending and running " +
			"jobs when it is not told which states to view, so nine of the eleven states " +
			"the collector counts never appeared and every metric built from them read a " +
			"constant zero. slurm_jobs_failed said the cluster had never had a failure " +
			"(issue #27).",
		Notes: []string{
			"The window is bounded by MinJobAge (slurm.conf, 300s by default): slurmctld " +
				"forgets a terminated job once it is older than that.",
			"Dropped by --no-collector.queue.terminal-states, which restores the " +
				"pre-1.9 query, the queue_default_states entry below.",
		},
		Fixtures: []Fixture{
			{
				File: "queue_all_states.txt",
				Why: "Eight states in one capture: RUNNING, PENDING, SUSPENDED, CANCELLED, " +
					"COMPLETED, FAILED, TIMEOUT, NODE_FAIL. PREEMPTED needs PreemptType " +
					"configured, COMPLETING lasts as long as an epilog and CONFIGURING as long " +
					"as a node boots, so those three are covered by a hand-written input in " +
					"TestParseQueueMetricsUnreachableStates instead.",
				Slurm: "25.11.2",
			},
		},
		invoke: func(log *logger.Logger, _ string) { _, _ = QueueData(log, true) },
	},
	{
		Name:   "queue_default_states",
		Binary: "squeue",
		Args:   []string{"-h", "-o", "%P|%T|%C|%r|%u"},
		Source: "queue.go",
		OptIn:  "--no-collector.queue.terminal-states",
		Doc: "The same query without --states=all, restoring the pre-1.9 behaviour for " +
			"sites that would rather not ask slurmctld for the terminal states.",
		NoFixtureReason: "Deliberate: the output is a subset of queue_all_states.txt and the parser is the " +
			"same one, so a second capture would protect nothing. What the flag changes is " +
			"the query itself, which the contract test pins.",
		invoke: func(log *logger.Logger, _ string) { _, _ = QueueData(log, false) },
	},
	{
		Name:   "cpus",
		Binary: "sinfo",
		Args:   []string{"-h", "-o", "%C"},
		Source: "cpus.go",
		Doc:    "Cluster-wide CPU states as allocated/idle/other/total.",
		Fixtures: []Fixture{
			{File: "cpus.txt", Why: "The single A/I/O/T line the parser splits on slashes."},
		},
		invoke: func(log *logger.Logger, _ string) { _, _ = CPUsData(log) },
	},
	{
		Name:   "fairshare",
		Binary: "sshare",
		Args: []string{"-a", "-P", "-n", "-o",
			"Account,User,RawShares,NormShares,RawUsage,NormUsage,FairShare"},
		Source: "fairshare.go",
		Doc: "Fairshare factor, shares and decay-weighted usage per account and per " +
			"user.",
		Notes: []string{
			"Lines with RawShares=parent are skipped: they inherit from the parent account.",
			"Lines with an empty Account field are skipped.",
			"User-level metrics require --collector.fairshare.user-metrics (default on).",
		},
		Fixtures: []Fixture{
			{
				File: "fairshare.txt",
				Why: "An account tree with both the parent rows that must be skipped and the " +
					"user rows that must not.",
			},
		},
		invoke: func(log *logger.Logger, _ string) { _, _ = FairShareData(log) },
	},
	{
		Name:   "gpus_snapshot",
		Binary: "sinfo",
		Args:   []string{"-a", "-h", "--Format=Nodes: ,StateLong: ,Gres: ,GresUsed:"},
		Source: "gpus.go",
		Doc: "One consolidated snapshot (node count, state, total GRES and used GRES) " +
			"from which total, allocated, idle and other GPUs are all derived. A single " +
			"call removes the race between the three separate snapshots this replaced " +
			"(issue #145).",
		Notes: []string{
			"The trailing colon on each field forces variable column widths; fixed widths " +
				"silently truncate rich GRES specs on busy GPU nodes: multi-type GPUs, MIG " +
				"slices (issue #10).",
			"The per-version slurm-*/sinfo_gpus_{allocated,idle,total}.txt fixtures still " +
				"back the version matrix in gpus_test.go, which exercises the individual GRES " +
				"parsers across Slurm releases. splitGPUViews feeds those same parsers from " +
				"this consolidated snapshot, so the version fixtures keep protecting the GRES " +
				"parsing even though the three commands they were captured from are retired.",
			"Coverage gap: the format of the command executed today, meaning four --Format " +
				"fields in a given order plus the (null) path, is validated on a single Slurm " +
				"version, while the retired commands are covered on six. See issue #195, gap A.",
		},
		Fixtures: []Fixture{
			{
				File:  "gpus_snapshot.txt",
				Why:   "The four-column consolidated layout the collector parses today.",
				Slurm: "25.05.3",
			},
			{
				File: "gpus_snapshot_long_gres.txt",
				Why: "A GRES string long enough to be truncated by a fixed-width --Format, " +
					"which is what the trailing colons exist to prevent (issue #10).",
			},
		},
		invoke: func(log *logger.Logger, _ string) { _, _ = GPUsSnapshotData(log) },
	},
	{
		Name:   "node_detail",
		Binary: "sinfo",
		Args: []string{"-h", "-N", "-O",
			"NodeList: ,AllocMem: ,Memory: ,CPUsState: ,StateLong: ,Partition: ,Gres: ,GresUsed:"},
		Source: "node.go",
		Doc: "Per-node CPU, memory and GRES detail, one row per node and partition. " +
			"The Gres and GresUsed columns feed slurm_node_gres_total and " +
			"slurm_node_gres_used, the only metrics in the exporter carrying a GPU " +
			"model, and the only per-node view of them.",
		Notes: []string{
			"The trailing colon on each field forces variable column widths. Without it " +
				"node names longer than 20 characters — the default NodeList width — collide " +
				"with the next column and produce fewer than six whitespace-separated tokens, " +
				"silently dropping those nodes (issue #10).",
		},
		Fixtures: []Fixture{
			{File: "node_detail.txt", Why: "The nominal six-column per-node layout."},
			{
				File: "node_detail_default_partition.txt",
				Why: "A node in the default partition, whose name carries the * marker that " +
					"has to be stripped before it becomes a label value.",
			},
			{
				File: "slurm-25.05/node_detail_gres.txt",
				Why: "The GRES columns on a real GPU cluster: five nodes across allocated, " +
					"mixed, idle, maint and drained, each in four partitions, plus a CPU node " +
					"whose columns read \"(null)\" and must publish nothing. Carries " +
					"\"gpu:model_a:2(IDX:0,3)\", the non-contiguous index list whose comma " +
					"breaks a naive split of the GRES string.",
				Slurm: "25.05.3",
			},
			{
				File: "slurm-25.11.2/node_detail_gres_multitype.txt",
				Why: "A node exposing two GPU models at once, with a job holding one of the " +
					"second: \"gpu:model_a:2,gpu:model_b:2\" against " +
					"\"gpu:model_a:0(IDX:N/A),gpu:model_b:1(IDX:2)\". Two resources separated " +
					"by a comma, each with its own index list, which is the shape the per-node " +
					"metrics exist to report and the one that breaks a naive parser.",
				Slurm: "25.11.2",
			},
			{
				File: "node_detail_long_names.txt",
				Why: "A 25-character node name alongside short ones, in the variable-width " +
					"output the trailing colons produce. Under the old fixed-width format that " +
					"name collided with the next column and the node vanished from the metrics " +
					"map; the regression net for issue #10.",
			},
		},
		invoke: func(log *logger.Logger, _ string) { _, _ = NodeData(log) },
	},
	{
		Name:   "nodes_global",
		Binary: "sinfo",
		Args:   []string{"-h", "-o", "%R|%D|%T|%b"},
		Source: "nodes.go",
		Doc: "Node counts by partition, state and feature set, for every partition in " +
			"one call. This replaced N per-partition calls (sinfo -p <partition>), " +
			"cutting the RPC load on slurmctld on clusters with many partitions; the " +
			"single-partition path was removed as unreachable in #100.",
		NoFixtureReason: "Still to do. test_data/sinfo.txt backed this command until " +
			"d52d93f (#100, v1.8.4) deleted it, and nodes_test.go has worked on inline strings " +
			"ever since. The most central command of the nodes collector has no captured " +
			"cluster output at all. Capturing one is the first job of tools/fixture-capture.",
		invoke: func(log *logger.Logger, _ string) { _, _ = NodesDataGlobal(log) },
	},
	{
		Name:      "scontrol_nodes",
		Binary:    "scontrol",
		Args:      []string{"show", "nodes", "-o"},
		Source:    "reservation_nodes.go",
		Consumers: []string{"nodes.go"},
		Doc: "Full node detail, one node per line. Read by two collectors — " +
			"reservation_nodes for per-reservation node membership and state, nodes for " +
			"the cluster-wide total — behind scontrolNodesCache, so the RPC is sent once " +
			"per scrape rather than twice.",
		Fixtures: []Fixture{
			{
				File: "scontrol_nodes.txt",
				Why: "Nodes carrying reservation membership, including the drained-but-up " +
					"case that decides whether a reserved node counts as healthy.",
			},
		},
		invoke: func(log *logger.Logger, _ string) { _, _ = ReservationNodesData(log) },
	},
	{
		Name:   "partitions_cpu",
		Binary: "sinfo",
		Args:   []string{"-h", "-o", "%R,%C"},
		Source: "partitions.go",
		Doc:    "CPU states per partition.",
		Fixtures: []Fixture{
			{
				File:  "slurm-25.11.1-1/partitions_cpu.txt",
				Why:   "Per-partition A/I/O/T lines.",
				Slurm: "25.11.1-1",
			},
		},
		invoke: func(log *logger.Logger, _ string) { _, _ = PartitionsData(log) },
	},
	{
		Name:   "partitions_gpu",
		Binary: "sinfo",
		Args: []string{"-h", "--Format=Nodes: ,Partition: ,Gres: ,GresUsed:",
			"--state=idle,allocated"},
		Source: "partitions.go",
		Doc:    "GPU totals and usage per partition, restricted to idle and allocated nodes.",
		Fixtures: []Fixture{
			{
				File:  "slurm-25.11.1-1/partitions_gpu.txt",
				Why:   "The nominal four-column per-partition GRES layout.",
				Slurm: "25.11.1-1",
			},
			{
				File: "partitions_gpu_long_gres.txt",
				Why: "GRES strings long enough to overflow a fixed-width column, the " +
					"per-partition counterpart of the issue #10 trap.",
			},
		},
		invoke: func(log *logger.Logger, _ string) { _, _ = PartitionsGpuData(log) },
	},
	{
		Name:   "drain_reason",
		Binary: "sinfo",
		Args:   []string{"-h", "-N", "-o", "%N|%E|%H|%T"},
		Source: "node_drain.go",
		Doc:    "Drain or down reason per node, with the timestamp the state was set.",
		NoFixtureReason: "Deliberate: the output depends entirely on which nodes happen to be drained " +
			"when the capture is taken, so a capture would document one cluster's bad day " +
			"rather than a format. The tests use inline inputs that pin the timestamp and " +
			"reason shapes instead.",
		invoke: func(log *logger.Logger, _ string) { _, _ = DrainReasonData(log) },
	},
	{
		Name:   "reservations",
		Binary: "scontrol",
		Args:   []string{"show", "reservation"},
		Source: "reservations.go",
		Doc: "Active reservation details as key=value blocks, one blank-line-separated " +
			"block per reservation.",
		Fixtures: []Fixture{
			{File: "reservations.txt", Why: "The nominal multi-reservation block layout."},
			{
				File: "reservations_empty.txt",
				Why: "What scontrol prints when no reservation exists, which is a sentence of " +
					"prose rather than an empty file. The parser has to produce no metrics from " +
					"it, rather than one bogus one.",
			},
			{
				File: "reservations_relative_time.txt",
				Why: "Reservation timestamps in the relative form Slurm emits for near-term " +
					"reservations, which the absolute-layout parser must not silently accept.",
			},
		},
		invoke: func(log *logger.Logger, _ string) {
			_, _ = NewReservationsCollector(log).reservationsData()
		},
	},
	{
		Name:   "licenses",
		Binary: "scontrol",
		Args:   []string{"show", "licenses", "-o"},
		Source: "licenses.go",
		Doc:    "License total, used, free and reserved counts, one license per line.",
		Fixtures: []Fixture{
			{File: "licenses.txt", Why: "Several licenses with distinct total/used/free splits."},
		},
		invoke: func(log *logger.Logger, _ string) { _, _ = LicenseData(log) },
	},
	{
		Name:   "scheduler",
		Binary: "sdiag",
		Args:   nil,
		Source: "scheduler.go",
		Doc:    "slurmctld internal scheduler statistics, including per-RPC counters.",
		Fixtures: []Fixture{
			{
				File: "scheduler.txt",
				Why: "The header counters (jobs submitted/started/completed/canceled/failed), " +
					"the main schedule statistics block and the backfill block. It stops there: " +
					"the Remote Procedure Call tables that scheduler.go also parses, per " +
					"operation and per user, are absent from this capture, so the RPC metrics " +
					"rest on TestSchedulerRPCLineRe_HyphenatedUsername alone: one inline line " +
					"against one regexp, with no captured report behind them.",
			},
		},
		invoke: func(log *logger.Logger, _ string) { _, _ = SchedulerData(log) },
	},
	{
		Name:       "binary_version",
		EachBinary: versionedBinaries,
		Args:       []string{"--version"},
		Source:     "slurm_binary_info.go",
		Doc: "The version of each Slurm binary, probed once per process rather than on " +
			"every scrape (issue #149): a Slurm upgrade under a running exporter is not " +
			"a supported state, the process is restarted by whatever performed it.",
		Notes: []string{
			"sinfo, squeue, sdiag, scontrol and sacct are required: a missing one is " +
				"reported with version=\"not_found\" and value 0 so operators can alert on it.",
			"sbatch, salloc and srun are optional and emit nothing when absent. The " +
				"exporter never invokes them, so requiring them would block monitoring-only " +
				"deployments (issue #24).",
			"sshare is executed by the fairshare collector but never version-probed.",
		},
		NoFixtureReason: "Deliberate: the parser reads one field of a one-line output, and the value " +
			"it reads is the Slurm version of whichever host runs the capture. A fixture " +
			"would pin that host's version, not a format.",
		invoke: func(log *logger.Logger, binary string) { _, _ = GetBinaryVersion(log, binary) },
	},
	{
		Name:   "sacct_efficiency",
		Binary: "sacct",
		Args: []string{
			"-P", "-n",
			"--starttime", "{{starttime}}",
			"--endtime", "{{endtime}}",
			"--format", "JobID,User,Account,AllocCPUS,Elapsed,TotalCPU,CPUTime,MaxRSS,ReqMem",
			"--state", "COMPLETED,FAILED,TIMEOUT,CANCELLED",
		},
		Placeholders: []Placeholder{
			{
				Token: "{{starttime}}",
				Match: slurmTimestamp,
				Shell: `$(date -d "-1 hour" +%Y-%m-%dT%H:%M:%S)`,
			},
			{
				Token: "{{endtime}}",
				Match: slurmTimestamp,
				Shell: `$(date +%Y-%m-%dT%H:%M:%S)`,
			},
		},
		Source: "sacct_efficiency.go",
		OptIn:  "--collector.sacct_efficiency",
		Doc: "Completed-job CPU and memory efficiency over the lookback window. " +
			"Disabled by default because it queries SlurmDBD, which is expensive on a " +
			"busy cluster; refreshed in the background on --collector.sacct.interval " +
			"rather than on the scrape path.",
		Notes: []string{
			"--endtime is mandatory: with --state and only --starttime, sacct returns no " +
				"rows at all: Slurm bounds a state-filtered search to [starttime, endtime] " +
				"and the default endtime does not cover the window. Without it the whole " +
				"collector reported nothing, not just memory (issue #143).",
			"No -X: MaxRSS is a step-level statistic and is empty on the allocation line, " +
				"so the step lines (<jobid>.batch, <jobid>.0, …) are read and their peak " +
				"MaxRSS attributed back to the job by JobID. JobID therefore leads --format.",
			"Populating TotalCPU and MaxRSS requires a working JobAcctGatherType in " +
				"slurm.conf.",
		},
		Fixtures: []Fixture{
			{
				File: "sacct_efficiency.txt",
				Why: "Allocation lines with their step lines, so the JobID correlation and the " +
					"MaxRSS attribution are both exercised. The line format is a real capture; " +
					"the MaxRSS values are representative rather than captured, because the " +
					"containerised test cluster runs proctrack/linuxproc, which does not gather " +
					"MaxRSS and leaves the column empty. A cluster with proctrack/cgroup fills " +
					"it in exactly this shape (issue #143).",
				Slurm: "25.11",
			},
		},
		invoke: func(log *logger.Logger, _ string) {
			NewSacctEfficiencyCollector(log, time.Hour, time.Hour).refresh()
		},
	},
}

// ── Fixture directories ──────────────────────────────────────────────────────

// FixtureDir is one versioned subdirectory of test_data/.
//
// GPU sinfo output changed shape between Slurm releases, so the GRES parsers
// are exercised against a matrix of captures rather than a single one.
type FixtureDir struct {
	// Path is the directory name under test_data/.
	Path string
	// Slurm is the release the capture was taken on.
	Slurm string
	// Supported reports whether the release is inside the support window
	// declared in CONTRIBUTING (newest major plus the previous three). A
	// directory below the floor is kept as free regression protection, not as
	// a peer of the supported ones.
	Supported bool
	// Notes is what makes this capture worth keeping.
	Notes string
}

// FixtureDirs describes the versioned GPU fixture matrix.
//
// The support window rolls with every Slurm major, so Supported is re-derived
// rather than frozen: see CONTRIBUTING § Releasing and issue #195, gap B. As of
// Slurm 26.05 the window is 24.11 → 26.05, and neither end has a fixture.
var FixtureDirs = []FixtureDir{
	{
		Path:      "slurm-20.11.8",
		Slurm:     "20.11.8",
		Supported: false,
		Notes:     "Classic GRES format.",
	},
	{
		Path:      "slurm-21.08.5",
		Slurm:     "21.08.5",
		Supported: false,
		Notes: "IDX format. sinfo_gpus_allocated.txt is empty on purpose — a cluster " +
			"with GPUs and none allocated — and gpus_test.go asserts a 0 result rather " +
			"than skipping the case.",
	},
	{
		Path:      "slurm-23.11.10",
		Slurm:     "23.11.10",
		Supported: false,
		Notes: "No idle fixture. The file was a two-column capture where IdleGPUsData emits " +
			"three, so the idle and other counts derived from it were artefacts of the " +
			"malformation; #177 deleted it. It cannot be recaptured — 23.11.10 is no longer " +
			"published upstream — and the real three-column idle format is already covered " +
			"by 23.11.10-2, so the gap is marked with idleUncovered in gpus_test.go rather " +
			"than filled.",
	},
	{
		Path:      "slurm-23.11.10-2",
		Slurm:     "23.11.10 patch 2",
		Supported: false,
	},
	{
		Path:      "slurm-25.05",
		Slurm:     "25.05.x",
		Supported: true,
		Notes: "nvidia_gb200 GPU type on a large machine: 1058 nodes on the total line, " +
			"1056 on the allocated one. Four-digit node counts are what overflow a " +
			"fixed-width Nodes: column, so this is the capture that shows why the " +
			"--Format fields end with colons.",
	},
	{
		Path:      "slurm-25.11.2",
		Slurm:     "25.11.2",
		Supported: true,
		Notes: "Captured on the scripts/testing cluster with a synthetic two-model GPU node. " +
			"The GPUs are not real: a dynamic slurmd registers Gres=gpu:model_a:2,gpu:model_b:2 " +
			"against device files created in the container. Slurm treats them as it treats any " +
			"other GRES, so the output is a genuine capture of a shape no single-model cluster " +
			"can produce.",
	},
	{
		Path:      "slurm-25.11.1-1",
		Slurm:     "25.11.1-1",
		Supported: true,
		Notes:     "Also carries the per-partition CPU and GPU captures.",
	},
}
