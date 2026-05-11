# Metrics Reference

> Back to [README](../README.md) · See also: [metrics-examples.md](metrics-examples.md) for Prometheus text output examples

## 📊 Metrics

The exporter provides a wide range of metrics, each collected by a specific, toggleable collector.

> For full Prometheus text-format output examples per collector, see **[docs/metrics-examples.md](metrics-examples.md)**.

### `accounts` Collector

Provides job statistics aggregated by Slurm account.

- **Command:** `squeue -a -r -h -o "%A|%a|%T|%C"`

| Metric | Description | Labels |
|---|---|---|
| `slurm_account_jobs_pending` | Pending jobs for account | `account` |
| `slurm_account_jobs_running` | Running jobs for account | `account` |
| `slurm_account_cpus_running` | Running CPUs for account | `account` |
| `slurm_account_gpus_running` | Running GPUs for account (from TRES) | `account` |
| `slurm_account_jobs_suspended` | Suspended jobs for account | `account` |

### `cpus` Collector

Provides global statistics on CPU states for the entire cluster.

- **Command:** `sinfo -h -o "%C"`

| Metric | Description | Labels |
|---|---|---|
| `slurm_cpus_alloc` | Allocated CPUs | (none) |
| `slurm_cpus_idle` | Idle CPUs | (none) |
| `slurm_cpus_other` | Mix CPUs | (none) |
| `slurm_cpus_total` | Total CPUs | (none) |

### `fairshare` Collector

Reports the calculated fairshare factor and underlying share/usage components,
per account and (when enabled via `--collector.fairshare.user-metrics`, default on) per user.

- **Command:** `sshare -a -P -n -o "Account,User,RawShares,NormShares,RawUsage,NormUsage,FairShare"`

| Metric | Description | Labels |
|---|---|---|
| `slurm_account_fairshare` | FairShare factor for account | `account` |
| `slurm_account_fairshare_norm_shares` | Normalised share allocation | `account` |
| `slurm_account_fairshare_norm_usage` | Normalised usage over the decay window | `account` |
| `slurm_account_fairshare_raw_shares` | Raw share allocation | `account` |
| `slurm_account_fairshare_raw_usage_cpu_seconds` | Raw CPU-seconds consumed | `account` |
| `slurm_user_fairshare` | FairShare factor for user | `user`, `account` |
| `slurm_user_fairshare_norm_shares` | Normalised share allocation per user | `user`, `account` |
| `slurm_user_fairshare_norm_usage` | Normalised usage per user | `user`, `account` |
| `slurm_user_fairshare_raw_shares` | Raw share allocation per user | `user`, `account` |
| `slurm_user_fairshare_raw_usage_cpu_seconds` | Raw CPU-seconds consumed per user | `user`, `account` |

User-level metrics can be disabled on clusters with many users to reduce cardinality
via `--collector.fairshare.user-metrics=false`.

### `gpus` Collector

Provides global statistics on GPU states for the entire cluster.

> ⚠️ **Note:** This collector is enabled by default. Disable it with `--no-collector.gpus` if not needed.

- **Command:** `sinfo` (with various formats)

| Metric | Description | Labels |
|---|---|---|
| `slurm_gpus_alloc` | Allocated GPUs | (none) |
| `slurm_gpus_idle` | Idle GPUs | (none) |
| `slurm_gpus_other` | Other GPUs | (none) |
| `slurm_gpus_total` | Total GPUs | (none) |
| `slurm_gpus_utilization` | Total GPU utilization | (none) |

### `info` Collector

Exposes the version of Slurm and the availability of different Slurm binaries.

- **Command:** `<binary> --version`

| Metric | Description | Labels |
|---|---|---|
| `slurm_info` | Information on Slurm version and binaries | `type`, `binary`, `version` |

### `licenses` Collector

Provides metrics on license counts and usage.

- **Command:** `scontrol show licenses -o`

| Metric | Description | Labels |
|---|---|---|
| `slurm_license_total` | Total count for license | `license` |
| `slurm_license_used` | Used count for license | `license` |
| `slurm_license_free` | Free count for license | `license` |
| `slurm_license_reserved` | Reserved count for license | `license` |

### `node` Collector

Provides detailed, per-node metrics for CPU and memory usage.

- **Command:** `sinfo -h -N -O "NodeList,AllocMem,Memory,CPUsState,StateLong,Partition"`

| Metric | Description | Labels |
|---|---|---|
| `slurm_node_cpu_alloc` | Allocated CPUs per node | `node`, `status`, `partition` |
| `slurm_node_cpu_idle` | Idle CPUs per node | `node`, `status`, `partition` |
| `slurm_node_cpu_other` | Other CPUs per node | `node`, `status`, `partition` |
| `slurm_node_cpu_total` | Total CPUs per node | `node`, `status`, `partition` |
| `slurm_node_mem_alloc` | Allocated memory per node | `node`, `status`, `partition` |
| `slurm_node_mem_total` | Total memory per node | `node`, `status`, `partition` |
| `slurm_node_status` | Node Status with partition (1 if up) | `node`, `status`, `partition` |

### `nodes` Collector

Provides aggregated metrics on node states for the cluster.

- **Commands:** `sinfo -h -o "%D|%T|%b"`, `scontrol show nodes -o`

| Metric | Description | Labels |
|---|---|---|
| `slurm_nodes_alloc` | Allocated nodes | `partition`, `active_feature_set` |
| `slurm_nodes_comp` | Completing nodes | `partition`, `active_feature_set` |
| `slurm_nodes_down` | Down nodes | `partition`, `active_feature_set` |
| `slurm_nodes_drain` | Drain nodes | `partition`, `active_feature_set` |
| `slurm_nodes_err` | Error nodes | `partition`, `active_feature_set` |
| `slurm_nodes_fail` | Fail nodes | `partition`, `active_feature_set` |
| `slurm_nodes_idle` | Idle nodes | `partition`, `active_feature_set` |
| `slurm_nodes_inval` | Inval nodes | `partition`, `active_feature_set` |
| `slurm_nodes_maint` | Maint nodes | `partition`, `active_feature_set` |
| `slurm_nodes_mix` | Mix nodes | `partition`, `active_feature_set` |
| `slurm_nodes_resv` | Reserved nodes | `partition`, `active_feature_set` |
| `slurm_nodes_other` | Nodes reported with an unknown state | `partition`, `active_feature_set` |
| `slurm_nodes_planned` | Planned nodes | `partition`, `active_feature_set` |
| `slurm_nodes_total` | Total number of nodes | (none) |

### `partitions` Collector

Provides metrics on CPU usage and pending jobs for each partition.

- **Commands:** `sinfo -h -o "%R,%C"`, `squeue -a -r -h -o "%P" --states=PENDING`

| Metric                           | Description | Labels |
|----------------------------------|---|---|
| `slurm_partition_cpus_allocated` | Allocated CPUs for partition | `partition` |
| `slurm_partition_cpus_idle`      | Idle CPUs for partition | `partition` |
| `slurm_partition_cpus_other`     | Other CPUs for partition | `partition` |
| `slurm_partition_cpus_total`     | Total CPUs for partition | `partition` |
| `slurm_partition_jobs_pending`   | Pending jobs for partition | `partition` |
| `slurm_partition_jobs_running`   | Running jobs for partition | `partition` |
| `slurm_partition_gpus_idle`      | Idle GPUs for partition | `partition` |
| `slurm_partition_gpus_allocated` | Allocated GPUs for partition | `partition` |

### `queue` Collector

Provides detailed metrics on job states and resource usage.

- **Command:** `squeue -h -o "%P|%T|%C|%r|%u"`

**Per-user/partition metrics** — only emitted when jobs exist in that state:

| Metric | Description | Labels |
|---|---|---|
| `slurm_queue_pending` | Pending jobs | `user`, `partition`, `reason` |
| `slurm_queue_running` | Running jobs | `user`, `partition` |
| `slurm_queue_suspended` | Suspended jobs | `user`, `partition` |
| `slurm_cores_pending` | Pending cores | `user`, `partition`, `reason` |
| `slurm_cores_running` | Running cores | `user`, `partition` |
| `slurm_cores_suspended` | Suspended cores | `user`, `partition` |
| `...` | (cancelled, completing, completed, configuring, failed, timeout, preempted, node_fail) | `user`, `partition` |

**Global totals** — always emitted even at 0, useful for alerting on empty cluster:

| Metric | Description | Labels |
|---|---|---|
| `slurm_jobs_pending` | Total pending jobs cluster-wide | (none) |
| `slurm_jobs_running` | Total running jobs cluster-wide | (none) |
| `slurm_jobs_suspended` | Total suspended jobs | (none) |
| `slurm_jobs_completing` | Total completing jobs | (none) |
| `slurm_jobs_completed` | Total completed jobs | (none) |
| `slurm_jobs_configuring` | Total configuring jobs | (none) |
| `slurm_jobs_failed` | Total failed jobs | (none) |
| `slurm_jobs_timeout` | Total timed-out jobs | (none) |
| `slurm_jobs_preempted` | Total preempted jobs | (none) |
| `slurm_jobs_node_fail` | Total jobs stopped by node fail | (none) |
| `slurm_jobs_cancelled` | Total cancelled jobs | (none) |
| `slurm_jobs_cores_running` | Total cores used by running jobs | (none) |
| `slurm_jobs_cores_pending` | Total cores requested by pending jobs | (none) |

### `reservations` Collector

Provides metrics about active Slurm reservations.

> **Note:** `start_time` and `end_time` are parsed in the server's local timezone (`time.Local`).

- **Command:** `scontrol show reservation`

| Metric | Description | Labels |
|---|---|---|
| `slurm_reservation_info` | A metric with a constant '1' value labeled by reservation details | `reservation_name`, `state`, `users`, `nodes`, `partition`, `flags` |
| `slurm_reservation_start_time_seconds` | The start time of the reservation in seconds since the Unix epoch | `reservation_name` |
| `slurm_reservation_end_time_seconds` | The end time of the reservation in seconds since the Unix epoch | `reservation_name` |
| `slurm_reservation_node_count` | The number of nodes allocated to the reservation | `reservation_name` |
| `slurm_reservation_core_count` | The number of cores allocated to the reservation | `reservation_name` |

### `reservation_nodes` Collector

Provides per-reservation node state metrics, parsed from `scontrol show nodes -o`.
Compound node states (e.g. `ALLOCATED+MAINTENANCE+RESERVED`) are categorized by
primary state (token before the first `+`).

- **Command:** `scontrol show nodes -o`

| Metric | Description | Labels |
|---|---|---|
| `slurm_reservation_nodes_alloc` | Allocated nodes in reservation | `reservation` |
| `slurm_reservation_nodes_idle` | Idle nodes in reservation | `reservation` |
| `slurm_reservation_nodes_mix` | Mixed nodes in reservation | `reservation` |
| `slurm_reservation_nodes_down` | Down nodes in reservation | `reservation` |
| `slurm_reservation_nodes_drain` | Drained nodes in reservation | `reservation` |
| `slurm_reservation_nodes_planned` | Planned nodes in reservation | `reservation` |
| `slurm_reservation_nodes_other` | Nodes in other states | `reservation` |
| `slurm_reservation_nodes_healthy` | Healthy nodes (alloc+idle+mix+planned) | `reservation` |

---

### `scheduler` Collector

Provides internal performance metrics from the `slurmctld` daemon, parsed from
`sdiag` output.

- **Command:** `sdiag`

**Scheduler health (gauges, always emitted):**

| Metric | Description | Labels |
|---|---|---|
| `slurm_scheduler_threads` | Number of scheduler threads | (none) |
| `slurm_scheduler_queue_size` | Length of the scheduler queue | (none) |
| `slurm_scheduler_dbd_queue_size` | Pending entries in the SlurmDBD agent queue | (none) |
| `slurm_scheduler_last_cycle` | Last scheduler cycle duration (µs) | (none) |
| `slurm_scheduler_mean_cycle` | Scheduler mean cycle duration (µs) | (none) |
| `slurm_scheduler_cycle_per_minute` | Scheduler cycles per minute | (none) |
| `slurm_scheduler_backfill_last_cycle` | Last backfill cycle duration (µs) | (none) |
| `slurm_scheduler_backfill_mean_cycle` | Mean backfill cycle duration (µs) | (none) |
| `slurm_scheduler_backfill_depth_mean` | Mean backfill depth | (none) |

**Backfill activity (gauges that reset on `slurmctld` restart):**

| Metric | Description | Labels |
|---|---|---|
| `slurm_scheduler_backfilled_jobs_since_start_total` | Jobs backfilled since `slurmctld` start | (none) |
| `slurm_scheduler_backfilled_jobs_since_cycle_total` | Jobs backfilled since last stats cycle | (none) |
| `slurm_scheduler_backfilled_heterogeneous_total` | Heterogeneous job components backfilled | (none) |

**Job lifecycle counts (gauges that reset on `slurmctld` restart or `scontrol reconfigure`):**

| Metric | Description | Labels |
|---|---|---|
| `slurm_scheduler_jobs_submitted` | Jobs submitted to the scheduler since last stats reset | (none) |
| `slurm_scheduler_jobs_started` | Jobs started (dispatched) since last stats reset | (none) |
| `slurm_scheduler_jobs_completed` | Jobs completed since last stats reset | (none) |
| `slurm_scheduler_jobs_canceled` | Jobs canceled since last stats reset | (none) |
| `slurm_scheduler_jobs_failed` | Jobs failed since last stats reset | (none) |

> **Note:** these five counters were renamed in v1.8.2 to drop the `_total`
> suffix and switched from Counter to Gauge, because `sdiag` resets them on
> every `slurmctld` restart or `scontrol reconfigure`. A Counter that decreases
> breaks `rate()` and `increase()`. Use `delta()` over a time window, or
> consume the raw value as a cumulative since-last-reset gauge.

**RPC statistics (cluster-wide and per user):**

| Metric | Description | Labels |
|---|---|---|
| `slurm_rpc_stats` | RPC call count by message type | `operation` |
| `slurm_rpc_stats_avg_time` | Average RPC time (µs) by message type | `operation` |
| `slurm_rpc_stats_total_time` | Total cumulative RPC time (µs) by message type | `operation` |
| `slurm_user_rpc_stats` | RPC call count per user | `user` |
| `slurm_user_rpc_stats_avg_time` | Average RPC time (µs) per user | `user` |
| `slurm_user_rpc_stats_total_time` | Total cumulative RPC time (µs) per user | `user` |

### `users` Collector

Provides job statistics aggregated by user.

- **Command:** `squeue -a -r -h -o "%A|%u|%T|%C"`

| Metric | Description | Labels |
|---|---|---|
| `slurm_user_jobs_pending` | Pending jobs for user | `user` |
| `slurm_user_jobs_running` | Running jobs for user | `user` |
| `slurm_user_cpus_running` | Running CPUs for user | `user` |
| `slurm_user_gpus_running` | Running GPUs for user (from TRES) | `user` |
| `slurm_user_jobs_suspended` | Suspended jobs for user | `user` |

---

---

### `drain_reason` Collector

Provides the drain/down reason for degraded nodes. Only emits metrics when
nodes are in `drain` or `down` state with an admin-set reason.
Zero cardinality overhead on healthy clusters.

- **Command:** `sinfo -h -N -o "%N|%E|%H|%T"`

| Metric | Description | Labels |
|---|---|---|
| `slurm_node_drain_reason_info` | Always 1 — use labels for the reason and timestamp | `node`, `reason`, `since` |

---

### `sacct_efficiency` Collector

Aggregated job efficiency metrics from `sacct`. **Disabled by default.**
Enable with `--collector.sacct_efficiency`.
Requires `JobAcctGatherType=jobacct_gather/linux|cgroup` in `slurm.conf`.

- **Command:** `sacct -X -P -n --starttime <lookback> --format User,Account,AllocCPUS,Elapsed,TotalCPU,CPUTime,MaxRSS,ReqMem --state COMPLETED,FAILED,TIMEOUT,CANCELLED`

| Metric | Description | Labels |
|---|---|---|
| `slurm_job_cpu_efficiency_avg` | Avg CPU efficiency (TotalCPU/CPUTime×100) over lookback window | `account`, `user` |
| `slurm_job_mem_efficiency_avg` | Avg memory efficiency (MaxRSS/ReqMem×100) over lookback window | `account`, `user` |
| `slurm_job_count_completed` | Jobs completed in lookback window | `account`, `user` |
| `slurm_job_cpu_hours_allocated` | Allocated CPU-hours in lookback window | `account`, `user` |
| `slurm_sacct_last_refresh_timestamp_seconds` | Unix timestamp of last sacct refresh | (none) |

---

### Internal Exporter Metrics

Self-monitoring metrics exposed by the exporter itself.

| Metric | Type | Description | Labels |
|---|---|---|---|
| `slurm_exporter_command_duration_seconds` | histogram | Duration of each Slurm CLI command | `command` |
| `slurm_exporter_command_errors_total` | counter | CLI command execution errors | `command` |
| `slurm_exporter_cache_age_seconds` | gauge | Age of internal caches (scontrol) | `cache` |
| `slurm_exporter_collector_success` | gauge | 1=OK, 0=FAIL per collector | `collector` |
| `slurm_exporter_collector_duration_seconds` | gauge | Last scrape duration per collector | `collector` |
