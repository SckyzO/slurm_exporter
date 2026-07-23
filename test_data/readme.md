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

- `sinfo -a -h --Format=Nodes:10 ,GresUsed: --state=allocated`: allocated GPUs.
  - Test files: **`slurm-*/sinfo_gpus_allocated.txt`**
- `sinfo -a -h --Format=Nodes:10 ,Gres:50 ,GresUsed:50 --state=idle,allocated`: idle + allocated GPUs.
  - Test files: **`slurm-*/sinfo_gpus_idle.txt`**
- `sinfo -a -h --Format=Nodes:10 ,Gres:50`: total GPUs.
  - Test files: **`slurm-*/sinfo_gpus_total.txt`**

## `collector/node.go`

- `scontrol show nodes -o`: per-node memory, CPU counts, state flags and
  partition membership. Shared with `nodes.go` and `reservation_nodes.go`
  through `scontrolNodesCache`, so one call serves every collector per scrape.
  - Test file: **`slurm-25.11.2/scontrol_nodes.txt`**
- `sinfo -h -o "%N|%T"`: the node state string, one row per (partition, state)
  group with the node list collapsed into a hostlist expression.
  - Test file: **`slurm-25.11.2/node_state.txt`**

  This pair replaced `sinfo -h -N -O "NodeList: ,AllocMem: ,Memory: ,CPUsState: ,StateLong: ,Partition:"`
  in #254: asking sinfo for the partition column of every node cost one to two
  minutes on a large multi-partition cluster, past `--command.timeout`, so no
  `slurm_node_*` series was exposed at all. The old command's output is kept as
  **`slurm-25.11.2/node_detail.txt`** and **`sinfo_mem.txt`**, which is what
  `ParseNodeMetrics` still parses — not in production, but as the reference the
  equivalence test compares the new path against.

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

- `squeue -h -o "%P|%T|%C|%r|%u"`: job states, cores, reason, user (pipe-delimited to safely handle commas in reason field).
  - Test file: **`squeue.txt`**

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

- `sacct -X -P -n --starttime <window> --format User,Account,AllocCPUS,Elapsed,TotalCPU,CPUTime,MaxRSS,ReqMem --state COMPLETED,FAILED,TIMEOUT,CANCELLED`: job efficiency data.
  - Test file: **`sacct_efficiency.txt`**
  - Requires `JobAcctGatherType=jobacct_gather/linux` (or similar) in slurm.conf to populate TotalCPU/MaxRSS.
  - Disabled by default (`--collector.sacct_efficiency` to enable).

## `collector/node_drain.go`

- `scontrol show nodes -o`: the drain/down reason and its timestamp, read from
  `Reason=`. Served from the same cache as `node.go`, so this collector adds no
  call of its own.
  - Test file: **`slurm-25.11.2/scontrol_nodes.txt`**
- `sinfo -h -o "%N|%T"`: the state string the reason is filtered on.
  - Test file: **`slurm-25.11.2/node_state.txt`**

  Replaced `sinfo -h -N -o "%N|%E|%H|%T"` in #254, which ran at about 4 s
  against a 5 s default timeout and was killed on some scrapes. Its output is
  kept as **`slurm-25.11.2/drain_reason.txt`** for the equivalence test.

### `slurm-25.11.2/` — the paired capture

The four files above were captured in one run on the `scripts/testing` cluster
(Slurm 25.11.2, 10 nodes, partitions `cpu`/`debug`/`gpu`/`high`), with the node
states driven on purpose so the comparison has something to compare:

| Node | State | What it covers |
|---|---|---|
| `c1`, `c2` | `mixed` | free CPUs count as idle on a healthy node; both sit in three partitions |
| `c3` | `draining` (`MIXED+DRAIN`) | free CPUs of a *busy* degraded node count as other, not idle — the case #255 listed as unproven |
| `c4` | `idle` | the baseline |
| `c5` | `drained` | a plain `Reason=` with its `[user@timestamp]` suffix |
| `c6` | `down` | a reason containing brackets (`NHC: check_hw_eth [eth0] failed`) |
| `c7` | `drained$` | a node in a maintenance reservation, so `ReservationName=` follows `Reason=`, and the `$` state suffix |
| `c8` | `idle*` (`IDLE+NOT_RESPONDING`) | an unreachable node before SlurmdTimeout: free CPUs count as **idle**, and the `*` suffix survives (#243) |
| `c9` | `fail` (`IDLE+FAIL`) | a failed node's free CPUs count as **idle** |
| `c10` | `failing` (`MIXED+FAIL`) | the same on a busy node, and a reason on a node neither path reports (the drain filter matches only down/drain) |

`c8`, `c9` and `c10` are the reason this capture was taken twice. #254 proposed
treating `FAIL`, `INVALID_REG` and `NOT_RESPONDING` as states that move a node's
free CPUs to `other`. Two of the three are wrong, and this capture is what
proves it: with the rule as the issue wrote it, the equivalence test fails on
exactly these three nodes.

Two notes for whoever recaptures this:

- **Row order in `node_detail.txt` is not stable.** Two consecutive runs of
  `sinfo -N` list a multi-partition node's rows in a different order. The
  content is identical; the parser sorts its input, and the equivalence test
  compares partitions as sets. A reordering is not a regression.
- `CPUEfctv` equals `CPUTot` on every node here, so this capture cannot say
  whether sinfo's CPU total follows one or the other when `CoreSpecCount` is
  set. That gap stays open in #255.
- `c8` is only `NOT_RESPONDING` while its worker container is stopped and
  `SlurmdTimeout` (300s here) has not yet expired; it reached that state about
  90s after the container was stopped. Recapturing it means stopping a worker
  and watching for the state rather than sleeping a fixed amount.

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
