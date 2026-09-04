<!--
GENERATED FILE. DO NOT EDIT.

Source of truth: internal/collector/command_registry.go
Regenerate with:  make generate   (or: go generate ./...)

Editing this file by hand is pointless: CI regenerates it and fails on the diff.
Change the registry instead. command_registry_test.go then proves that the
registry still matches what the collectors actually run.
-->

# Slurm commands used by slurm_exporter

Every Slurm CLI invocation the exporter makes, and the fixture under `test_data/`
that protects it.

This mapping is derived from `internal/collector/command_registry.go`, which is
itself checked against the real collectors: `TestRegistryMatchesCollectors` invokes
each `*Data()` function with `Execute` stubbed and compares what it genuinely
passes to what the table declares. A command cannot change without this file
changing with it.

All fixtures are anonymised: cluster, node, user, account and reservation names
are replaced with generic equivalents. See `CONTRIBUTING.md` § Test Data.


## Index

| Command | Binary | Owned by | Fixtures |
|---|---|---|---|
| [`squeue_jobs`](#squeue_jobs) | `squeue` | `squeue_jobs.go` | 3 |
| [`queue_all_states`](#queue_all_states) | `squeue` | `queue.go` | 1 |
| [`queue_default_states`](#queue_default_states) | `squeue` | `queue.go` | none |
| [`cpus`](#cpus) | `sinfo` | `cpus.go` | 1 |
| [`fairshare`](#fairshare) | `sshare` | `fairshare.go` | 1 |
| [`gpus_snapshot`](#gpus_snapshot) | `sinfo` | `gpus.go` | 2 |
| [`node_detail`](#node_detail) | `sinfo` | `node.go` | 5 |
| [`nodes_global`](#nodes_global) | `sinfo` | `nodes.go` | none |
| [`scontrol_nodes`](#scontrol_nodes) | `scontrol` | `reservation_nodes.go` | 1 |
| [`partitions_cpu`](#partitions_cpu) | `sinfo` | `partitions.go` | 1 |
| [`partitions_gpu`](#partitions_gpu) | `sinfo` | `partitions.go` | 2 |
| [`drain_reason`](#drain_reason) | `sinfo` | `node_drain.go` | none |
| [`reservations`](#reservations) | `scontrol` | `reservations.go` | 3 |
| [`licenses`](#licenses) | `scontrol` | `licenses.go` | 1 |
| [`scheduler`](#scheduler) | `sdiag` | `scheduler.go` | 1 |
| [`binary_version`](#binary_version) | `8 binaries` | `slurm_binary_info.go` | none |
| [`sacct_efficiency`](#sacct_efficiency) | `sacct` | `sacct_efficiency.go` | 1 |

## Commands

### squeue_jobs

```sh
squeue -a -r -h -O 'JobID:|,Account:|,UserName:|,Partition:|,State:|,NumNodes:|,NumCPUs:|,tres-alloc:'
```

One consolidated snapshot of the whole job queue, cached per scrape and shared by the accounts, users and partitions collectors. Before issue #144 these issued up to five separate full-queue dumps to slurmctld every scrape; they now project their views from this single call. The -a -r flags and the default state set match what each collector requested individually, so no metric value changes.

Owned by `squeue_jobs.go`. Also read by `accounts.go`, `users.go` and `partitions.go`.

- queue.go is deliberately NOT a consumer: it omits -a/-r and toggles --states=all, and folding it in here would change job-array counts.
- The trailing colon on every field forces variable-width columns. Without it squeue caps a field at 20 characters and silently drops the tail (issues #10 and #35).
- tres-alloc (effective total allocation) is used instead of the legacy %b (TRES per node) so jobs submitted with --gpus or --gpus-per-node are accounted for (issue #35).

| Fixture | Slurm | What it protects |
|---|---|---|
| `squeue_jobs.txt` | 25.11 | The consolidated layout itself: RUNNING/PENDING/SUSPENDED jobs across several accounts, users and partitions, plus a multi-partition (cpu,gpu) pending job that the partitions projection has to split. |
| `squeue_jobs_accounts_view.txt` | unrecorded | Backs ParseAccountsMetrics, which the accounts projection re-emits verbatim: proves the projection produces the layout the parser expects. |
| `squeue_jobs_users_view.txt` | unrecorded | Same contract as squeue_jobs_accounts_view.txt for the users projection (ParseUsersMetrics). |

### queue_all_states

```sh
squeue -h -o '%P|%T|%C|%r|%u' --states=all
```

Job states, cores, reason and user, pipe-delimited so commas inside the reason field stay inside their column. --states=all is what brings the terminal states into the output: squeue reports only pending and running jobs when it is not told which states to view, so nine of the eleven states the collector counts never appeared and every metric built from them read a constant zero. slurm_jobs_failed said the cluster had never had a failure (issue #27).

Owned by `queue.go`.

- The window is bounded by MinJobAge (slurm.conf, 300s by default): slurmctld forgets a terminated job once it is older than that.
- Dropped by --no-collector.queue.terminal-states, which restores the pre-2.0 query, the queue_default_states entry below.

| Fixture | Slurm | What it protects |
|---|---|---|
| `queue_all_states.txt` | 25.11.2 | Eight states in one capture: RUNNING, PENDING, SUSPENDED, CANCELLED, COMPLETED, FAILED, TIMEOUT, NODE_FAIL. PREEMPTED needs PreemptType configured, COMPLETING lasts as long as an epilog and CONFIGURING as long as a node boots, so those three are covered by a hand-written input in TestParseQueueMetricsUnreachableStates instead. |

### queue_default_states

```sh
squeue -h -o '%P|%T|%C|%r|%u'
```

The same query without --states=all, restoring the pre-2.0 behaviour for sites that would rather not ask slurmctld for the terminal states.

Owned by `queue.go`. Runs only with `--no-collector.queue.terminal-states`.

**No fixture.** Deliberate: the output is a subset of queue_all_states.txt and the parser is the same one, so a second capture would protect nothing. What the flag changes is the query itself, which the contract test pins.

### cpus

```sh
sinfo -h -o '%C'
```

Cluster-wide CPU states as allocated/idle/other/total.

Owned by `cpus.go`.

| Fixture | Slurm | What it protects |
|---|---|---|
| `cpus.txt` | unrecorded | The single A/I/O/T line the parser splits on slashes. |

### fairshare

```sh
sshare -a -P -n -o Account,User,RawShares,NormShares,RawUsage,NormUsage,FairShare
```

Fairshare factor, shares and decay-weighted usage per account and per user.

Owned by `fairshare.go`.

- Lines with RawShares=parent are skipped: they inherit from the parent account.
- Lines with an empty Account field are skipped.
- User-level metrics require --collector.fairshare.user-metrics (default on).

| Fixture | Slurm | What it protects |
|---|---|---|
| `fairshare.txt` | unrecorded | An account tree with both the parent rows that must be skipped and the user rows that must not. |

### gpus_snapshot

```sh
sinfo -a -h '--Format=Nodes: ,StateLong: ,Gres: ,GresUsed:'
```

One consolidated snapshot (node count, state, total GRES and used GRES) from which total, allocated, idle and other GPUs are all derived. A single call removes the race between the three separate snapshots this replaced (issue #145).

Owned by `gpus.go`.

- The trailing colon on each field forces variable column widths; fixed widths silently truncate rich GRES specs on busy GPU nodes: multi-type GPUs, MIG slices (issue #10).
- The per-version slurm-*/sinfo_gpus_{allocated,idle,total}.txt fixtures still back the version matrix in gpus_test.go, which exercises the individual GRES parsers across Slurm releases. splitGPUViews feeds those same parsers from this consolidated snapshot, so the version fixtures keep protecting the GRES parsing even though the three commands they were captured from are retired.
- Coverage gap: the format of the command executed today, meaning four --Format fields in a given order plus the (null) path, is validated on a single Slurm version, while the retired commands are covered on six. See issue #195, gap A.

| Fixture | Slurm | What it protects |
|---|---|---|
| `gpus_snapshot.txt` | 25.05.3 | The four-column consolidated layout the collector parses today. |
| `gpus_snapshot_long_gres.txt` | unrecorded | A GRES string long enough to be truncated by a fixed-width --Format, which is what the trailing colons exist to prevent (issue #10). |

### node_detail

```sh
sinfo -h -N -O 'NodeList: ,AllocMem: ,Memory: ,CPUsState: ,StateLong: ,Partition: ,Gres: ,GresUsed:'
```

Per-node CPU, memory and GRES detail, one row per node and partition. The Gres and GresUsed columns feed slurm_node_gres_total and slurm_node_gres_used, the only metrics in the exporter carrying a GPU model, and the only per-node view of them.

Owned by `node.go`.

- The trailing colon on each field forces variable column widths. Without it node names longer than 20 characters — the default NodeList width — collide with the next column and produce fewer than six whitespace-separated tokens, silently dropping those nodes (issue #10).

| Fixture | Slurm | What it protects |
|---|---|---|
| `node_detail.txt` | unrecorded | The nominal six-column per-node layout. |
| `node_detail_default_partition.txt` | unrecorded | A node in the default partition, whose name carries the * marker that has to be stripped before it becomes a label value. |
| `slurm-25.05/node_detail_gres.txt` | 25.05.3 | The GRES columns on a real GPU cluster: five nodes across allocated, mixed, idle, maint and drained, each in four partitions, plus a CPU node whose columns read "(null)" and must publish nothing. Carries "gpu:model_a:2(IDX:0,3)", the non-contiguous index list whose comma breaks a naive split of the GRES string. |
| `slurm-25.11.2/node_detail_gres_multitype.txt` | 25.11.2 | A node exposing two GPU models at once, with a job holding one of the second: "gpu:model_a:2,gpu:model_b:2" against "gpu:model_a:0(IDX:N/A),gpu:model_b:1(IDX:2)". Two resources separated by a comma, each with its own index list, which is the shape the per-node metrics exist to report and the one that breaks a naive parser. |
| `node_detail_long_names.txt` | unrecorded | A 25-character node name alongside short ones, in the variable-width output the trailing colons produce. Under the old fixed-width format that name collided with the next column and the node vanished from the metrics map; the regression net for issue #10. |

### nodes_global

```sh
sinfo -h -o '%R|%D|%T|%b'
```

Node counts by partition, state and feature set, for every partition in one call. This replaced N per-partition calls (sinfo -p <partition>), cutting the RPC load on slurmctld on clusters with many partitions; the single-partition path was removed as unreachable in #100.

Owned by `nodes.go`.

**No fixture.** Still to do. test_data/sinfo.txt backed this command until d52d93f (#100, v1.8.4) deleted it, and nodes_test.go has worked on inline strings ever since. The most central command of the nodes collector has no captured cluster output at all. Capturing one is the first job of tools/fixture-capture.

### scontrol_nodes

```sh
scontrol show nodes -o
```

Full node detail, one node per line. Read by two collectors — reservation_nodes for per-reservation node membership and state, nodes for the cluster-wide total — behind scontrolNodesCache, so the RPC is sent once per scrape rather than twice.

Owned by `reservation_nodes.go`. Also read by `nodes.go`.

| Fixture | Slurm | What it protects |
|---|---|---|
| `scontrol_nodes.txt` | unrecorded | Nodes carrying reservation membership, including the drained-but-up case that decides whether a reserved node counts as healthy. |

### partitions_cpu

```sh
sinfo -h -o '%R,%C'
```

CPU states per partition.

Owned by `partitions.go`.

| Fixture | Slurm | What it protects |
|---|---|---|
| `slurm-25.11.1-1/partitions_cpu.txt` | 25.11.1-1 | Per-partition A/I/O/T lines. |

### partitions_gpu

```sh
sinfo -h '--Format=Nodes: ,Partition: ,Gres: ,GresUsed:' --state=idle,allocated
```

GPU totals and usage per partition, restricted to idle and allocated nodes.

Owned by `partitions.go`.

| Fixture | Slurm | What it protects |
|---|---|---|
| `slurm-25.11.1-1/partitions_gpu.txt` | 25.11.1-1 | The nominal four-column per-partition GRES layout. |
| `partitions_gpu_long_gres.txt` | unrecorded | GRES strings long enough to overflow a fixed-width column, the per-partition counterpart of the issue #10 trap. |

### drain_reason

```sh
sinfo -h -N -o '%N|%E|%H|%T'
```

Drain or down reason per node, with the timestamp the state was set.

Owned by `node_drain.go`.

**No fixture.** Deliberate: the output depends entirely on which nodes happen to be drained when the capture is taken, so a capture would document one cluster's bad day rather than a format. The tests use inline inputs that pin the timestamp and reason shapes instead.

### reservations

```sh
scontrol show reservation
```

Active reservation details as key=value blocks, one blank-line-separated block per reservation.

Owned by `reservations.go`.

| Fixture | Slurm | What it protects |
|---|---|---|
| `reservations.txt` | unrecorded | The nominal multi-reservation block layout. |
| `reservations_empty.txt` | unrecorded | What scontrol prints when no reservation exists, which is a sentence of prose rather than an empty file. The parser has to produce no metrics from it, rather than one bogus one. |
| `reservations_relative_time.txt` | unrecorded | Reservation timestamps in the relative form Slurm emits for near-term reservations, which the absolute-layout parser must not silently accept. |

### licenses

```sh
scontrol show licenses -o
```

License total, used, free and reserved counts, one license per line.

Owned by `licenses.go`.

| Fixture | Slurm | What it protects |
|---|---|---|
| `licenses.txt` | unrecorded | Several licenses with distinct total/used/free splits. |

### scheduler

```sh
sdiag
```

slurmctld internal scheduler statistics, including per-RPC counters.

Owned by `scheduler.go`.

| Fixture | Slurm | What it protects |
|---|---|---|
| `scheduler.txt` | unrecorded | The header counters (jobs submitted/started/completed/canceled/failed), the main schedule statistics block and the backfill block. It stops there: the Remote Procedure Call tables that scheduler.go also parses, per operation and per user, are absent from this capture, so the RPC metrics rest on TestSchedulerRPCLineRe_HyphenatedUsername alone: one inline line against one regexp, with no captured report behind them. |

### binary_version

```sh
sinfo --version
squeue --version
sdiag --version
scontrol --version
sacct --version
sbatch --version
salloc --version
srun --version
```

The version of each Slurm binary, probed once per process rather than on every scrape (issue #149): a Slurm upgrade under a running exporter is not a supported state, the process is restarted by whatever performed it.

Owned by `slurm_binary_info.go`.

- sinfo, squeue, sdiag, scontrol and sacct are required: a missing one is reported with version="not_found" and value 0 so operators can alert on it.
- sbatch, salloc and srun are optional and emit nothing when absent. The exporter never invokes them, so requiring them would block monitoring-only deployments (issue #24).
- sshare is executed by the fairshare collector but never version-probed.

**No fixture.** Deliberate: the parser reads one field of a one-line output, and the value it reads is the Slurm version of whichever host runs the capture. A fixture would pin that host's version, not a format.

### sacct_efficiency

```sh
sacct -P -n --starttime '<starttime>' --endtime '<endtime>' --format JobID,User,Account,AllocCPUS,Elapsed,TotalCPU,CPUTime,MaxRSS,ReqMem --state COMPLETED,FAILED,TIMEOUT,CANCELLED
```

Completed-job CPU and memory efficiency over the lookback window. Disabled by default because it queries SlurmDBD, which is expensive on a busy cluster; refreshed in the background on --collector.sacct.interval rather than on the scrape path.

Owned by `sacct_efficiency.go`. Runs only with `--collector.sacct_efficiency`.

- --endtime is mandatory: with --state and only --starttime, sacct returns no rows at all: Slurm bounds a state-filtered search to [starttime, endtime] and the default endtime does not cover the window. Without it the whole collector reported nothing, not just memory (issue #143).
- No -X: MaxRSS is a step-level statistic and is empty on the allocation line, so the step lines (<jobid>.batch, <jobid>.0, …) are read and their peak MaxRSS attributed back to the job by JobID. JobID therefore leads --format.
- Populating TotalCPU and MaxRSS requires a working JobAcctGatherType in slurm.conf.

| Fixture | Slurm | What it protects |
|---|---|---|
| `sacct_efficiency.txt` | 25.11 | Allocation lines with their step lines, so the JobID correlation and the MaxRSS attribution are both exercised. The line format is a real capture; the MaxRSS values are representative rather than captured, because the containerised test cluster runs proctrack/linuxproc, which does not gather MaxRSS and leaves the column empty. A cluster with proctrack/cgroup fills it in exactly this shape (issue #143). |

## Coverage gaps

4 of the 17 commands run against no captured cluster output:

| Command | Owned by | Why |
|---|---|---|
| [`queue_default_states`](#queue_default_states) | `queue.go` | Deliberate: the output is a subset of queue_all_states.txt and the parser is the same one, so a second capture would protect nothing. What the flag changes is the query itself, which the contract test pins. |
| [`nodes_global`](#nodes_global) | `nodes.go` | Still to do. test_data/sinfo.txt backed this command until d52d93f (#100, v1.8.4) deleted it, and nodes_test.go has worked on inline strings ever since. The most central command of the nodes collector has no captured cluster output at all. Capturing one is the first job of tools/fixture-capture. |
| [`drain_reason`](#drain_reason) | `node_drain.go` | Deliberate: the output depends entirely on which nodes happen to be drained when the capture is taken, so a capture would document one cluster's bad day rather than a format. The tests use inline inputs that pin the timestamp and reason shapes instead. |
| [`binary_version`](#binary_version) | `slurm_binary_info.go` | Deliberate: the parser reads one field of a one-line output, and the value it reads is the Slurm version of whichever host runs the capture. A fixture would pin that host's version, not a format. |

## Versioned GPU fixtures

GPU `sinfo` output changed shape between Slurm releases, so the GRES parsers
are exercised against a matrix of captures rather than a single one. The
matrix is discovered at run time by `gpus_test.go`, which asserts coverage in
both directions: a declared version with no fixture fails, and a fixture no
version claims fails too. That symmetry exists because the glob once resolved
to a directory that did not exist, so every test body was skipped while the
suite still reported a pass (#148, then #176 and #177).

`Supported` follows the window in `CONTRIBUTING.md` § Releasing: the newest
Slurm major plus the previous three. It rolls with every Slurm release, so a
directory dropping out of the window is expected. Those captures are kept as
free regression protection rather than as peers of the supported ones.

| Directory | Slurm | Supported | Notes |
|---|---|---|---|
| `slurm-20.11.8/` | 20.11.8 | no (historical) | Classic GRES format. |
| `slurm-21.08.5/` | 21.08.5 | no (historical) | IDX format. sinfo_gpus_allocated.txt is empty on purpose — a cluster with GPUs and none allocated — and gpus_test.go asserts a 0 result rather than skipping the case. |
| `slurm-23.11.10/` | 23.11.10 | no (historical) | No idle fixture. The file was a two-column capture where IdleGPUsData emits three, so the idle and other counts derived from it were artefacts of the malformation; #177 deleted it. It cannot be recaptured — 23.11.10 is no longer published upstream — and the real three-column idle format is already covered by 23.11.10-2, so the gap is marked with idleUncovered in gpus_test.go rather than filled. |
| `slurm-23.11.10-2/` | 23.11.10 patch 2 | no (historical) | — |
| `slurm-25.05/` | 25.05.x | yes | nvidia_gb200 GPU type on a large machine: 1058 nodes on the total line, 1056 on the allocated one. Four-digit node counts are what overflow a fixed-width Nodes: column, so this is the capture that shows why the --Format fields end with colons. |
| `slurm-24.11.7/` | 24.11.7 | yes | Oldest end of the support window, captured by scripts/capture.sh on the scripts/testing cluster built from source (#189), with fake GPU nodes so the GRES columns are populated. Holds every command in the registry, which is what make fixture-diff compares against the newest end. |
| `slurm-26.05.2/` | 26.05.2 | yes | Newest end of the support window, same capture procedure. Comparing it with 24.11.7 is what surfaced SuspendTime appearing in scontrol show nodes. |
| `slurm-25.11.2/` | 25.11.2 | yes | Captured on the scripts/testing cluster with a synthetic two-model GPU node. The GPUs are not real: a dynamic slurmd registers Gres=gpu:model_a:2,gpu:model_b:2 against device files created in the container. Slurm treats them as it treats any other GRES, so the output is a genuine capture of a shape no single-model cluster can produce. |
| `slurm-25.11.1-1/` | 25.11.1-1 | yes | Also carries the per-partition CPU and GPU captures. |

