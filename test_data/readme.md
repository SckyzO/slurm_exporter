# Slurm Commands Used in slurm_exporter

This file documents all Slurm shell commands executed by `slurm_exporter` and the test
data files that correspond to each command's output.

All test data files are anonymized (real cluster node names, user names, account names
and reservation names have been replaced with generic equivalents).

## `collector/accounts.go`

- `squeue -a -r -h -o "%A|%a|%T|%D|%C|%b"`: job/CPU/GPU counts by account.
  - Test file: **`squeue_tres.txt`**

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
- `sinfo -h --Format=Nodes:10 ,Partition:30 ,Gres:50 ,GresUsed:50 --state=idle,allocated`: GPU state per partition.
  - Test files: **`slurm-25.11.1-1/sinfo_partitions_gpu.txt`**
- `squeue -a -r -h -o "%P" --states=PENDING`: pending jobs per partition.
  - Test files: **`slurm-25.11.1-1/squeue_partitions_pending_job.txt`**
- `squeue -a -r -h -o "%P" --states=RUNNING`: running jobs per partition.
  - Test files: **`slurm-25.11.1-1/squeue_partitions_running_job.txt`**

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

## `collector/users.go`

- `squeue -a -r -h -o "%A|%u|%T|%D|%C|%b"`: job/CPU/GPU counts by user.
  - Test file: **`squeue_tres.txt`** (same format as accounts)

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
