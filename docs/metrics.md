# Metrics Reference

> Back to [README](../README.md) · See also: [metrics-examples.md](metrics-examples.md) for Prometheus text output examples

## 📊 Metrics

The exporter provides a wide range of metrics, each collected by a specific, toggleable collector.

> For full Prometheus text-format output examples per collector, see **[docs/metrics-examples.md](docs/metrics-examples.md)**.

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

Reports the calculated fairshare factor for each account.

- **Command:** `sshare -n -P -o "account,fairshare"`

| Metric | Description | Labels |
|---|---|---|
| `slurm_account_fairshare` | FairShare for account | `account` |

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

Provides internal performance metrics from the `slurmctld` daemon.

- **Command:** `sdiag`

| Metric | Description | Labels |
|---|---|---|
| `slurm_scheduler_threads` | Number of scheduler threads | (none) |
| `slurm_scheduler_queue_size` | Length of the scheduler queue | (none) |
| `slurm_scheduler_mean_cycle` | Scheduler mean cycle time (microseconds) | (none) |
| `slurm_rpc_stats` | RPC count statistic | `operation` |
| `slurm_user_rpc_stats` | RPC count statistic per user | `user` |
| `...` | (and many other backfill and RPC time metrics) | `operation` or `user` |

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

