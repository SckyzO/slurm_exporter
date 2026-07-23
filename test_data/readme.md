# Slurm Commands Used in slurm_exporter

This file documents all Slurm shell commands executed by `slurm_exporter` and the test
data files that correspond to each command's output.

All test data files are anonymized (real cluster node names, user names, account names
and reservation names have been replaced with generic equivalents).

## Shared squeue snapshot (`collector/squeue_jobs.go`)

- `squeue -a -r -h -O "JobID:|,Account:|,UserName:|,Partition:|,State:|,NumNodes:|,NumCPUs:|,tres-alloc:"`:
  one consolidated snapshot of the whole job queue, cached per scrape and shared
  by the `accounts`, `users` and `partitions` collectors. Before issue #144
  these issued up to five separate full-queue dumps to `slurmctld` every scrape;
  they now project their views from this single call. `-a -r` and the default
  state set match what each collector requested individually, so no metric value
  changes. `queue.go` is deliberately **not** a consumer (it omits `-a`/`-r` and
  toggles `--states=all`; sharing it would change job-array counts).
  - Test file: **`squeue_jobs.txt`** — captured on the `scripts/testing` cluster
    (Slurm 25.11), covering RUNNING/PENDING/SUSPENDED jobs across several
    accounts, users and partitions, plus a multi-partition (`cpu,gpu`) pending
    job.

## `collector/accounts.go`

- Projects the account view (`JobID|Account|State|NumNodes|NumCPUs|tres-alloc`)
  from the shared snapshot above: job/CPU/GPU counts by account.
  - Test file: **`squeue_tres.txt`** — still backs `ParseAccountsMetrics`, which
    is unchanged; the projection re-emits exactly this layout.
  - Uses `tres-alloc` (effective allocation, total) instead of the legacy `%b`
    (TRES per node) so that jobs submitted with `--gpus` or `--gpus-per-node`
    are accounted for (see issue #35).

## `collector/cpus.go`

- `sinfo -h -o "%C"`: CPU state (allocated/idle/other/total) for the cluster.
  - Test file: **`sinfo_cpus.txt`**

## `collector/fairshare.go`

- `sshare -a -P -n -o Account,User,RawShares,NormShares,RawUsage,NormUsage,FairShare`: fairshare factor, shares and decay-weighted usage per account and user.
  - Test file: **`sshare_users.txt`**
  - Lines with `RawShares=parent` are skipped (inherit from parent account).
  - Lines with an empty Account field are skipped.
  - User-level metrics require `--collector.fairshare.user-metrics=true` (default).

## `collector/gpus.go`

- `sinfo -a -h --Format=Nodes: ,StateLong: ,Gres: ,GresUsed:`: one consolidated
  snapshot — node count, state, total GRES and used GRES — from which total,
  allocated, idle and other GPUs are all derived. A single call removes the race
  between the three separate snapshots this replaced (issue #145).
  - Test file: **`sinfo_gpus_snapshot.txt`**

  The per-version **`slurm-*/sinfo_gpus_{allocated,idle,total}.txt`** fixtures
  still back the version matrix in `gpus_test.go`, which exercises the individual
  GRES parsers (`ParseAllocatedGPUs`, `ParseIdleGPUs`, `ParseTotalGPUs`) across
  Slurm releases. `splitGPUViews` feeds those same parsers from the consolidated
  snapshot, so the version fixtures keep protecting the GRES parsing.

## `collector/node.go`

- `sinfo -h -N -O NodeList,AllocMem,Memory,CPUsState,StateLong,Partition`: per-node detail.
  - Test file: **`sinfo_mem.txt`**

## `collector/nodes.go`

- `sinfo -h -o "%D|%T|%b" -p <partition>`: node counts by state and feature set.
  - Test file: **`sinfo.txt`**
- `scontrol show nodes -o`: total node count.
- `sinfo -h -o "%R"`: partition list.

## `collector/partitions.go`

- `sinfo -h -o "%R,%C"`: CPU state per partition.
  - Test files: **`slurm-25.11.1-1/sinfo_partitions_cpu.txt`**
- `sinfo -h --Format=Nodes: ,Partition: ,Gres: ,GresUsed: --state=idle,allocated`: GPU state per partition.
  - Test files: **`slurm-25.11.1-1/sinfo_partitions_gpu.txt`**
- Pending and running job counts per partition are projected from the shared
  squeue snapshot (see above), filtering the consolidated output by state
  instead of issuing `squeue -o "%P" --states=PENDING` and `--states=RUNNING`
  separately (issue #144). Multi-partition jobs keep their comma-separated
  partition list, which is split across partitions.
  - Test file: **`squeue_jobs.txt`** (shared).

## `collector/queue.go`

- `squeue -h -o "%P|%T|%C|%r|%u" --states=all`: job states, cores, reason, user (pipe-delimited to safely handle commas in reason field). `--states=all` is what brings the terminal states into the output; it is dropped by `--no-collector.queue.terminal-states`.
  - Test file: **`squeue.txt`** — captured on the `scripts/testing` cluster
    (Slurm 25.11.2, 10 nodes) and covering eight states: RUNNING, PENDING,
    SUSPENDED, CANCELLED, COMPLETED, FAILED, TIMEOUT and NODE_FAIL. PREEMPTED
    needs `PreemptType` configured, COMPLETING lasts as long as an epilog and
    CONFIGURING as long as a node boots, so those three are covered by a
    hand-written input in `TestParseQueueMetricsUnreachableStates` instead.

## `collector/reservations.go`

- `scontrol show reservation`: active reservation details.
  - Test file: **`sreservations.txt`**

## `collector/reservation_nodes.go`

- `scontrol show nodes -o`: node states with reservation membership.
  - Test file: **`scontrol_nodes_reservation.txt`**

## `collector/scheduler.go`

- `sdiag`: `slurmctld` internal scheduler statistics.
  - Test file: **`sdiag.txt`**

## `collector/slurm_binary_info.go`

- `<binary> --version` for: `sinfo`, `squeue`, `sdiag`, `scontrol`, `sacct`, `sbatch`, `salloc`, `srun`.

## `collector/licenses.go`

- `scontrol show licenses -o`: license total/used/free/reserved counts.
  - Test file: **`slicense.txt`**

## `collector/sacct_efficiency.go`

- `sacct -P -n --starttime <window> --format JobID,User,Account,AllocCPUS,Elapsed,TotalCPU,CPUTime,MaxRSS,ReqMem --state COMPLETED,FAILED,TIMEOUT,CANCELLED`: job efficiency data.
  - Test file: **`sacct_efficiency.txt`**
  - Requires `JobAcctGatherType=jobacct_gather/linux` (or similar) in slurm.conf to populate TotalCPU/MaxRSS.
  - No `-X`: `MaxRSS` is a step-level field, empty on the allocation line, so the
    step lines (`<jobid>.batch`, `<jobid>.0`, …) are read and their peak `MaxRSS`
    is attributed back to the job by `JobID`.
  - The line **format** was captured from the test cluster (Slurm 25.11). The
    `MaxRSS` values on the step lines are representative rather than captured: the
    containerised cluster uses `proctrack/linuxproc`, which does not gather
    `MaxRSS`, so a real capture leaves that column empty. A cluster with a working
    `JobAcctGather` (`proctrack/cgroup`) populates it exactly in this shape.
  - Disabled by default (`--collector.sacct_efficiency` to enable).

## `collector/node_drain.go`

- `sinfo -h -N -o "%N|%E|%H|%T"`: node drain/down reason and timestamp.
  - No dedicated test file (data varies by cluster state, tested with inline fixtures).

## `collector/users.go`

- Projects the user view (`JobID|UserName|State|NumNodes|NumCPUs|tres-alloc`)
  from the shared squeue snapshot (see the top of this file): job/CPU/GPU counts
  by user.
  - Test file: **`squeue_tres_users.txt`** — still backs `ParseUsersMetrics`,
    which is unchanged; the projection re-emits exactly this layout.
  - Same `tres-alloc` rationale as accounts (issue #35).

---

## Versioned GPU test data

GPU sinfo output format varies between Slurm versions. Each subdirectory contains
`sinfo_gpus_allocated.txt`, `sinfo_gpus_idle.txt`, and `sinfo_gpus_total.txt`:

| Directory | Slurm version | Notes |
|-----------|---------------|-------|
| `slurm-20.11.8/` | 20.11.8 | Classic format |
| `slurm-21.08.5/` | 21.08.5 | IDX format |
| `slurm-23.11.10/` | 23.11.10 | |
| `slurm-23.11.10-2/` | 23.11.10 patch 2 | |
| `slurm-25.05/` | 25.05.x | `nvidia_gb200` GPU type; demonstrates column-overflow bug with 1056+ nodes and `--Format=Nodes: ,GresUsed:` |
| `slurm-25.11.1-1/` | 25.11.1-1 | Latest; also contains partition test data |
