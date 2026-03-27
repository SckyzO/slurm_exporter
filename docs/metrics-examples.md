# Metrics Output Examples

This document shows representative `/metrics` output for each collector.
Values are illustrative; actual numbers depend on your cluster.
All user names, account names, and partition names are generic.

---

## Exporter self-monitoring

These metrics are always present (one entry per enabled Slurm collector):

```
# HELP slurm_exporter_collector_duration_seconds Duration of the last scrape for the collector in seconds
# TYPE slurm_exporter_collector_duration_seconds gauge
slurm_exporter_collector_duration_seconds{collector="accounts"} 0.012
slurm_exporter_collector_duration_seconds{collector="gpus"} 0.034
slurm_exporter_collector_duration_seconds{collector="nodes"} 0.008
slurm_exporter_collector_duration_seconds{collector="queue"} 0.021
...

# HELP slurm_exporter_collector_success Whether the last scrape of the collector succeeded (1=success, 0=failure)
# TYPE slurm_exporter_collector_success gauge
slurm_exporter_collector_success{collector="accounts"} 1
slurm_exporter_collector_success{collector="gpus"} 1
slurm_exporter_collector_success{collector="nodes"} 1
slurm_exporter_collector_success{collector="queue"} 1
```

---

## `accounts` collector

Command: `squeue -a -r -h -o "%A|%a|%T|%D|%C|%b"`

```
# HELP slurm_account_cpus_running Running CPUs for account
# TYPE slurm_account_cpus_running gauge
slurm_account_cpus_running{account="hpc_team"} 1024
slurm_account_cpus_running{account="ml_lab"} 512

# HELP slurm_account_gpus_running Running GPUs for account
# TYPE slurm_account_gpus_running gauge
slurm_account_gpus_running{account="ml_lab"} 32

# HELP slurm_account_jobs_pending Pending jobs for account
# TYPE slurm_account_jobs_pending gauge
slurm_account_jobs_pending{account="hpc_team"} 14
slurm_account_jobs_pending{account="ml_lab"} 3

# HELP slurm_account_jobs_running Running jobs for account
# TYPE slurm_account_jobs_running gauge
slurm_account_jobs_running{account="hpc_team"} 42
slurm_account_jobs_running{account="ml_lab"} 8
```

---

## `cpus` collector

Command: `sinfo -h -o "%C"`

```
# HELP slurm_cpus_alloc Allocated CPUs
# TYPE slurm_cpus_alloc gauge
slurm_cpus_alloc 5725

# HELP slurm_cpus_idle Idle CPUs
# TYPE slurm_cpus_idle gauge
slurm_cpus_idle 877

# HELP slurm_cpus_other Mix CPUs
# TYPE slurm_cpus_other gauge
slurm_cpus_other 34

# HELP slurm_cpus_total Total CPUs
# TYPE slurm_cpus_total gauge
slurm_cpus_total 6636
```

---

## `fairshare` collector

Command: `sshare -a -P -n -o Account,User,RawShares,NormShares,RawUsage,NormUsage,FairShare`

```
# HELP slurm_account_fairshare FairShare factor for account
# TYPE slurm_account_fairshare gauge
slurm_account_fairshare{account="hpc_team"} 0.42
slurm_account_fairshare{account="ml_lab"} 0.78
slurm_account_fairshare{account="root"} 1

# HELP slurm_account_fairshare_norm_shares Normalized shares for account
# TYPE slurm_account_fairshare_norm_shares gauge
slurm_account_fairshare_norm_shares{account="hpc_team"} 0.3
slurm_account_fairshare_norm_shares{account="ml_lab"} 0.5

# HELP slurm_account_fairshare_norm_usage Normalized usage for account
# TYPE slurm_account_fairshare_norm_usage gauge
slurm_account_fairshare_norm_usage{account="hpc_team"} 0.4
slurm_account_fairshare_norm_usage{account="ml_lab"} 0.2

# HELP slurm_user_fairshare FairShare factor for user
# TYPE slurm_user_fairshare gauge
slurm_user_fairshare{account="hpc_team",user="alice"} 0.85
slurm_user_fairshare{account="ml_lab",user="bob"} 0.92

# HELP slurm_user_fairshare_raw_usage Raw usage for user
# TYPE slurm_user_fairshare_raw_usage gauge
slurm_user_fairshare_raw_usage{account="hpc_team",user="alice"} 1.08933138e+08
slurm_user_fairshare_raw_usage{account="ml_lab",user="bob"} 2.175631e+06
```

---

## `gpus` collector

Commands: `sinfo` with `--Format=Nodes:10 ,GresUsed:` / `Gres:` variations.

```
# HELP slurm_gpus_alloc Allocated GPUs
# TYPE slurm_gpus_alloc gauge
slurm_gpus_alloc 78

# HELP slurm_gpus_idle Idle GPUs
# TYPE slurm_gpus_idle gauge
slurm_gpus_idle 24

# HELP slurm_gpus_other Other GPUs
# TYPE slurm_gpus_other gauge
slurm_gpus_other 52

# HELP slurm_gpus_total Total GPUs
# TYPE slurm_gpus_total gauge
slurm_gpus_total 154

# HELP slurm_gpus_utilization Total GPU utilization
# TYPE slurm_gpus_utilization gauge
slurm_gpus_utilization 0.506
```

---

## `info` collector

Command: `<binary> --version` for sinfo, squeue, sdiag, scontrol, sacct, sbatch, salloc, srun.

```
# HELP slurm_info Information on Slurm version and binaries
# TYPE slurm_info gauge
slurm_info{binary="",type="general",version="25.05.3"} 1
slurm_info{binary="sacct",type="binary",version="25.05.3"} 1
slurm_info{binary="salloc",type="binary",version="25.05.3"} 1
slurm_info{binary="sbatch",type="binary",version="25.05.3"} 1
slurm_info{binary="scontrol",type="binary",version="25.05.3"} 1
slurm_info{binary="sdiag",type="binary",version="25.05.3"} 1
slurm_info{binary="sinfo",type="binary",version="25.05.3"} 1
slurm_info{binary="squeue",type="binary",version="25.05.3"} 1
slurm_info{binary="srun",type="binary",version="25.05.3"} 1
```

---

## `licenses` collector

Command: `scontrol show licenses -o`

```
# HELP slurm_license_free Free count for license
# TYPE slurm_license_free gauge
slurm_license_free{license="ansys@flex"} 80
slurm_license_free{license="fluent@flex"} 20

# HELP slurm_license_reserved Reserved count for license
# TYPE slurm_license_reserved gauge
slurm_license_reserved{license="ansys@flex"} 0
slurm_license_reserved{license="fluent@flex"} 5

# HELP slurm_license_total Total count for license
# TYPE slurm_license_total gauge
slurm_license_total{license="ansys@flex"} 100
slurm_license_total{license="fluent@flex"} 30

# HELP slurm_license_used Used count for license
# TYPE slurm_license_used gauge
slurm_license_used{license="ansys@flex"} 20
slurm_license_used{license="fluent@flex"} 10
```

---

## `node` collector

Command: `sinfo -h -N -O NodeList,AllocMem,Memory,CPUsState,StateLong,Partition`

```
# HELP slurm_node_cpu_alloc Allocated CPUs per node
# TYPE slurm_node_cpu_alloc gauge
slurm_node_cpu_alloc{node="gpu-node-01",partition="gpu",status="mixed"} 16
slurm_node_cpu_alloc{node="gpu-node-02",partition="gpu",status="allocated"} 32

# HELP slurm_node_mem_alloc Allocated memory per node
# TYPE slurm_node_mem_alloc gauge
slurm_node_mem_alloc{node="gpu-node-01",partition="gpu",status="mixed"} 163840
slurm_node_mem_alloc{node="gpu-node-02",partition="gpu",status="allocated"} 327680

# HELP slurm_node_status Node Status with partition
# TYPE slurm_node_status gauge
slurm_node_status{node="gpu-node-01",partition="gpu",status="mixed"} 1
slurm_node_status{node="gpu-node-02",partition="gpu",status="allocated"} 1
```

---

## `nodes` collector

### With `--collector.nodes.feature-set` (default)

```
# HELP slurm_nodes_alloc Allocated nodes
# TYPE slurm_nodes_alloc gauge
slurm_nodes_alloc{active_feature_set="a100,nvlink",partition="gpu"} 10
slurm_nodes_alloc{active_feature_set="null",partition="cpu"} 42

# HELP slurm_nodes_idle Idle nodes
# TYPE slurm_nodes_idle gauge
slurm_nodes_idle{active_feature_set="a100,nvlink",partition="gpu"} 4
slurm_nodes_idle{active_feature_set="null",partition="cpu"} 18

# HELP slurm_nodes_total Total number of nodes
# TYPE slurm_nodes_total gauge
slurm_nodes_total 128
```

### With `--no-collector.nodes.feature-set`

The `active_feature_set` label is dropped; counts are aggregated per partition:

```
# HELP slurm_nodes_alloc Allocated nodes
# TYPE slurm_nodes_alloc gauge
slurm_nodes_alloc{partition="gpu"} 10
slurm_nodes_alloc{partition="cpu"} 42

# HELP slurm_nodes_idle Idle nodes
# TYPE slurm_nodes_idle gauge
slurm_nodes_idle{partition="gpu"} 4
slurm_nodes_idle{partition="cpu"} 18
```

---

## `partitions` collector

Commands: `sinfo -h -o "%R,%C"` and `squeue --states=PENDING/RUNNING`

```
# HELP slurm_partition_cpus_allocated Allocated CPUs for partition
# TYPE slurm_partition_cpus_allocated gauge
slurm_partition_cpus_allocated{partition="cpu"} 20756
slurm_partition_cpus_allocated{partition="gpu"} 478

# HELP slurm_partition_gpus_allocated Allocated GPUs for partition
# TYPE slurm_partition_gpus_allocated gauge
slurm_partition_gpus_allocated{partition="gpu"} 36

# HELP slurm_partition_gpus_idle Idle GPUs for partition
# TYPE slurm_partition_gpus_idle gauge
slurm_partition_gpus_idle{partition="gpu"} 8

# HELP slurm_partition_jobs_pending Pending jobs for partition
# TYPE slurm_partition_jobs_pending gauge
slurm_partition_jobs_pending{partition="cpu"} 24
slurm_partition_jobs_pending{partition="gpu"} 7

# HELP slurm_partition_jobs_running Running jobs for partition
# TYPE slurm_partition_jobs_running gauge
slurm_partition_jobs_running{partition="cpu"} 312
slurm_partition_jobs_running{partition="gpu"} 18
```

---

## `queue` collector

Command: `squeue -h -o "%P|%T|%C|%r|%u"`

### With `--collector.queue.user-label` (default)

```
# HELP slurm_queue_pending Pending jobs in queue
# TYPE slurm_queue_pending gauge
slurm_queue_pending{partition="gpu",reason="Resources",user="alice"} 3
slurm_queue_pending{partition="gpu",reason="Priority",user="bob"} 1

# HELP slurm_queue_running Running jobs in the cluster
# TYPE slurm_queue_running gauge
slurm_queue_running{partition="gpu",user="alice"} 4
slurm_queue_running{partition="cpu",user="bob"} 12

# HELP slurm_cores_running Running cores in the cluster
# TYPE slurm_cores_running gauge
slurm_cores_running{partition="gpu",user="alice"} 128
slurm_cores_running{partition="cpu",user="bob"} 384
```

### With `--no-collector.queue.user-label`

The `user` label is dropped; counts are aggregated per partition.
Cardinality: ~220 series instead of potentially 220,000 on large clusters.

```
# HELP slurm_queue_pending Pending jobs in queue
# TYPE slurm_queue_pending gauge
slurm_queue_pending{partition="gpu",reason="Resources"} 47
slurm_queue_pending{partition="gpu",reason="Priority"} 12

# HELP slurm_queue_running Running jobs in the cluster
# TYPE slurm_queue_running gauge
slurm_queue_running{partition="gpu"} 38
slurm_queue_running{partition="cpu"} 274
```

### Global totals (always present, both modes)

These metrics have no labels and are always emitted even at 0.
Use them for alerting on cluster state without risking "No Data" in PromQL.

```
# HELP slurm_jobs_pending Total pending jobs in the cluster
# TYPE slurm_jobs_pending gauge
slurm_jobs_pending 59

# HELP slurm_jobs_running Total running jobs in the cluster
# TYPE slurm_jobs_running gauge
slurm_jobs_running 312

# HELP slurm_jobs_failed Total failed jobs in the cluster
# TYPE slurm_jobs_failed gauge
slurm_jobs_failed 0

# HELP slurm_jobs_cores_running Total cores used by running jobs
# TYPE slurm_jobs_cores_running gauge
slurm_jobs_cores_running 3744

# HELP slurm_jobs_cores_pending Total cores requested by pending jobs
# TYPE slurm_jobs_cores_pending gauge
slurm_jobs_cores_pending 708
```

**PromQL example — alert when cluster has been idle for 10 minutes:**
```promql
slurm_jobs_running == 0
```
This works reliably because `slurm_jobs_running` is always present.
With `sum(slurm_queue_running)`, PromQL returns "No Data" when there are no jobs.

---

## `reservation_nodes` collector

Command: `scontrol show nodes -o`

Compound states (e.g. `ALLOCATED+MAINTENANCE+RESERVED`) are categorised
by the primary state (token before the first `+`).

```
# HELP slurm_reservation_nodes_alloc Allocated nodes in reservation
# TYPE slurm_reservation_nodes_alloc gauge
slurm_reservation_nodes_alloc{reservation="maintenance-2026"} 1

# HELP slurm_reservation_nodes_healthy Healthy nodes in reservation (alloc+idle+mix+planned)
# TYPE slurm_reservation_nodes_healthy gauge
slurm_reservation_nodes_healthy{reservation="maintenance-2026"} 5
slurm_reservation_nodes_healthy{reservation="prod"} 102

# HELP slurm_reservation_nodes_idle Idle nodes in reservation
# TYPE slurm_reservation_nodes_idle gauge
slurm_reservation_nodes_idle{reservation="maintenance-2026"} 4
slurm_reservation_nodes_idle{reservation="prod"} 12

# HELP slurm_reservation_nodes_down Down nodes in reservation
# TYPE slurm_reservation_nodes_down gauge
slurm_reservation_nodes_down{reservation="prod"} 2
```

---

## `reservations` collector

Command: `scontrol show reservation`

```
# HELP slurm_reservation_core_count The number of cores allocated to the reservation
# TYPE slurm_reservation_core_count gauge
slurm_reservation_core_count{reservation_name="prod"} 25152

# HELP slurm_reservation_end_time_seconds The end time of the reservation in seconds since the Unix epoch
# TYPE slurm_reservation_end_time_seconds gauge
slurm_reservation_end_time_seconds{reservation_name="prod"} 1.75477992e+09

# HELP slurm_reservation_info A metric with a constant '1' value labeled by reservation details
# TYPE slurm_reservation_info gauge
slurm_reservation_info{flags="SPEC_NODES,ALL_NODES",nodes="node[001-102]",partition="",reservation_name="prod",state="ACTIVE",users="admin"} 1

# HELP slurm_reservation_node_count The number of nodes allocated to the reservation
# TYPE slurm_reservation_node_count gauge
slurm_reservation_node_count{reservation_name="prod"} 102
```

---

## `scheduler` collector

Command: `sdiag`

```
# HELP slurm_scheduler_backfill_last_cycle Last backfill cycle time in microseconds reported by sdiag
# TYPE slurm_scheduler_backfill_last_cycle gauge
slurm_scheduler_backfill_last_cycle 1.94289e+06

# HELP slurm_scheduler_backfilled_jobs_since_start_total Jobs started via backfilling since last Slurm start
# TYPE slurm_scheduler_backfilled_jobs_since_start_total gauge
slurm_scheduler_backfilled_jobs_since_start_total 111544

# HELP slurm_scheduler_cycle_per_minute Number of scheduler cycles per minute reported by sdiag
# TYPE slurm_scheduler_cycle_per_minute gauge
slurm_scheduler_cycle_per_minute 63

# HELP slurm_scheduler_last_cycle Last scheduler cycle time in microseconds reported by sdiag
# TYPE slurm_scheduler_last_cycle gauge
slurm_scheduler_last_cycle 97209

# HELP slurm_scheduler_queue_size Length of the scheduler queue reported by sdiag
# TYPE slurm_scheduler_queue_size gauge
slurm_scheduler_queue_size 0

# HELP slurm_rpc_stats RPC call count by operation, reported by sdiag
# TYPE slurm_rpc_stats gauge
slurm_rpc_stats{operation="REQUEST_NODE_INFO"} 4320
slurm_rpc_stats{operation="REQUEST_JOB_INFO"} 8640
```

---

## `users` collector

Command: `squeue -a -r -h -o "%A|%u|%T|%D|%C|%b"`

```
# HELP slurm_user_cpus_running Running CPUs for user
# TYPE slurm_user_cpus_running gauge
slurm_user_cpus_running{user="alice"} 288
slurm_user_cpus_running{user="bob"} 96

# HELP slurm_user_gpus_running Running GPUs for user
# TYPE slurm_user_gpus_running gauge
slurm_user_gpus_running{user="alice"} 8

# HELP slurm_user_jobs_pending Pending jobs for user
# TYPE slurm_user_jobs_pending gauge
slurm_user_jobs_pending{user="alice"} 5
slurm_user_jobs_pending{user="bob"} 2

# HELP slurm_user_jobs_running Running jobs for user
# TYPE slurm_user_jobs_running gauge
slurm_user_jobs_running{user="alice"} 3
slurm_user_jobs_running{user="bob"} 1
```

---

## `--web.disable-exporter-metrics`

### Default (Go runtime metrics present)

```
# HELP go_goroutines Number of goroutines that currently exist.
# TYPE go_goroutines gauge
go_goroutines 12

# HELP process_cpu_seconds_total Total user and system CPU time spent in seconds.
# TYPE process_cpu_seconds_total counter
process_cpu_seconds_total 0.43

# HELP go_gc_duration_seconds A summary of the pause duration of garbage collection cycles.
# TYPE go_gc_duration_seconds summary
go_gc_duration_seconds{quantile="0"} 3.42e-05
...
```

### With `--web.disable-exporter-metrics`

All `go_*` and `process_*` metrics are absent from `/metrics`. Only `build_info` and Slurm metrics are present:

```
# HELP go_build_info Build information about the main Go module.
# TYPE go_build_info gauge
go_build_info{checksum="",path="github.com/sckyzo/slurm_exporter",version="(devel)"} 1

# HELP slurm_cpus_alloc Allocated CPUs
...
```
