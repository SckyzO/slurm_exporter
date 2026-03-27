# Prometheus Slurm Exporter 🚀

[![Release](https://github.com/sckyzo/slurm_exporter/actions/workflows/release.yml/badge.svg)](https://github.com/sckyzo/slurm_exporter/actions/workflows/release.yml)
[![Create Dev Release](https://github.com/sckyzo/slurm_exporter/actions/workflows/dev-release.yml/badge.svg)](https://github.com/sckyzo/slurm_exporter/actions/workflows/dev-release.yml)
[![GitHub release (latest by date)](https://img.shields.io/github/v/release/sckyzo/slurm_exporter)](https://github.com/sckyzo/slurm_exporter/releases/latest)
[![Go Report Card](https://goreportcard.com/badge/github.com/sckyzo/slurm_exporter)](https://goreportcard.com/report/github.com/sckyzo/slurm_exporter)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)

Prometheus collector and exporter for metrics extracted from the [Slurm](https://slurm.schedmd.com/overview.html) resource scheduling system.

> [!WARNING]
> This repository will no longer be actively maintained starting with Slurm version 25.11, as Slurm natively integrates support for OpenMetrics metrics for Prometheus. Please consider migrating to:
> [Slurm 25.11 metrics plugin](https://slurm.schedmd.com/metrics.html)
>
>
> I developed a new Slurm exporter to simplify and improve the usage of Slurm metrics:
> https://github.com/sckyzo/slurm_prometheus_exporter/
>
> ✨ Features:
> - ✅ Export Native OpenMetrics from Slurm (version 25.11+)
> - ✅ Support for multiple endpoints (jobs, jobs-users-accts, nodes, partitions, scheduler)
> - ✅ Basic Authentication and SSL/TLS support
> - ✅ Customizable global labels for all metrics
> - ✅ Easy configuration with YAML
> - ✅ Built with Clean Architecture principles
> - ✅ Comprehensive error handling and logging

## 📋 Table of Contents

- [Prometheus Slurm Exporter 🚀](#prometheus-slurm-exporter-)
  - [✨ Features](#-features)
  - [📦 Installation](#-installation)
    - [1. From Pre-compiled Releases](#1-from-pre-compiled-releases)
    - [2. From Source](#2-from-source)
  - [⚙️ Usage](#️-usage)
    - [Command-Line Options](#command-line-options)
    - [Enabling and Disabling Collectors](#enabling-and-disabling-collectors)
  - [🛠️ Development](#️-development)
    - [Prerequisites](#prerequisites)
    - [Building from Source](#building-from-source)
    - [Running Tests](#running-tests)
    - [Development Commands](#development-commands)
  - [📊 Metrics](#-metrics)
    - [`accounts` Collector](#accounts-collector)
    - [`cpus` Collector](#cpus-collector)
    - [`fairshare` Collector](#fairshare-collector)
    - [`gpus` Collector](#gpus-collector)
    - [`info` Collector](#info-collector)
    - [`node` Collector](#node-collector)
    - [`nodes` Collector](#nodes-collector)
    - [`partitions` Collector](#partitions-collector)
    - [`queue` Collector](#queue-collector)
    - [`reservations` Collector](#reservations-collector)
    - [`scheduler` Collector](#scheduler-collector)
    - [`users` Collector](#users-collector)
  - [📡 Prometheus Configuration](#-prometheus-configuration)
    - [Performance Considerations](#performance-considerations)
  - [📈 Grafana Dashboard](#-grafana-dashboard)
  - [📜 License](#-license)
  - [🍴 About this fork](#-about-this-fork)

---

## ✨ Features

- ✅ Exports a wide range of metrics from Slurm, including nodes, partitions, jobs, CPUs, and GPUs.
- ✅ All metric collectors are optional and can be enabled/disabled via flags.
- ✅ Supports TLS and Basic Authentication for secure connections.
- ✅ OpenMetrics format supported (exemplars, newer Prometheus features).
- ✅ Per-collector health metrics (`slurm_exporter_collector_success`, `slurm_exporter_collector_duration_seconds`).
- ✅ Liveness probe at `/healthz` for orchestrators (Kubernetes, systemd).
- ✅ GPU metrics per account and user (`slurm_account_gpus_running`, `slurm_user_gpus_running`).
- ✅ Per-reservation node state metrics (`slurm_reservation_nodes_*`).
- ✅ Ready-to-use Grafana dashboard.

---

## 📦 Installation

There are two recommended ways to install the Slurm Exporter.

### 1. From Pre-compiled Releases

This is the easiest method for most users.

1. Download the latest release for your OS and architecture from the [GitHub Releases](https://github.com/sckyzo/slurm_exporter/releases) page. 📥
2. Place the `slurm_exporter` binary in a suitable location on a node with Slurm CLI access, such as `/usr/local/bin/`.
3. Ensure the binary is executable:

   ```bash
   chmod +x /usr/local/bin/slurm_exporter
   ```

4. (Optional) To run the exporter as a service, you can adapt the example Systemd unit file provided in this repository at [systemd/slurm_exporter.service](systemd/slurm_exporter.service).
   - Copy it to `/etc/systemd/system/slurm_exporter.service` and customize it for your environment (especially the `ExecStart` path).
   - Reload the Systemd daemon, then enable and start the service:

     ```bash
     sudo systemctl daemon-reload
     sudo systemctl enable slurm_exporter
     sudo systemctl start slurm_exporter
     ```

### 2. From Source

If you want to build the exporter yourself, you can do so using the provided Makefile. 👩‍💻

1. Clone the repository:

   ```bash
   git clone https://github.com/sckyzo/slurm_exporter.git
   cd slurm_exporter
   ```

2. Build the binary:

   ```bash
   make build
   ```

3. The new binary will be available at `bin/slurm_exporter`. You can then copy it to a location like `/usr/local/bin/` and set up the Systemd service as described in the section above.

---

## ⚙️ Usage

The exporter can be configured using command-line flags.

**Basic execution:**

```bash
./slurm_exporter --web.listen-address=":9341"
```

**Using a configuration file for web settings (TLS/Basic Auth):**

```bash
./slurm_exporter --web.config.file=/path/to/web-config.yml
```

For details on the `web-config.yml` format, see the [Exporter Toolkit documentation](https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md).

**View help and all available options:**

```bash
./slurm_exporter --help
```

### Command-Line Options

| Flag | Description | Default |
|------|-------------|---------|
| `--web.listen-address` | Address to listen on for web interface and telemetry | `:9341` |
| `--web.config.file` | Path to configuration file for TLS/Basic Auth | (none) |
| `--command.timeout` | Timeout for executing Slurm commands | `5s` |
| `--log.level` | Log level: `debug`, `info`, `warn`, `error` | `info` |
| `--log.format` | Log format: `json`, `text` | `text` |
| `--collector.<name>` | Enable the specified collector | `true` (all enabled by default) |
| `--no-collector.<name>` | Disable the specified collector | (none) |
| `--collector.nodes.feature-set` | Include `active_feature_set` label in `slurm_nodes_*` metrics | `true` |
| `--web.disable-exporter-metrics` | Exclude Go runtime and process metrics from `/metrics` | `false` |

**Available collectors:** `accounts`, `cpus`, `fairshare`, `gpus`, `info`, `node`, `nodes`, `partitions`, `queue`, `reservations`, `reservation_nodes`, `scheduler`, `users`, `licenses`

### Enabling and Disabling Collectors

By default, all collectors are **enabled**.

You can control which collectors are active using the `--collector.<name>` and `--no-collector.<name>` flags.

**Example: Disable the `scheduler` and `partitions` collectors**

```bash
./slurm_exporter --no-collector.scheduler --no-collector.partitions
```

**Example: Disable the `gpus` collector**

```bash
./slurm_exporter --no-collector.gpus
```

**Example: Run only the `nodes` and `cpus` collectors**

This requires disabling all other collectors individually.

```bash
./slurm_exporter \
  --no-collector.accounts \
  --no-collector.fairshare \
  --no-collector.gpus \
  --no-collector.node \
  --no-collector.partitions \
  --no-collector.queue \
  --no-collector.reservations \
  --no-collector.scheduler \
  --no-collector.info \
  --no-collector.users
```

**Example: Custom timeout and logging**

```bash
./slurm_exporter \
  --command.timeout=10s \
  --log.level=debug \
  --log.format=json
```

---

## 🛠️ Development

This project requires access to a node with the Slurm CLI (`sinfo`, `squeue`, `sdiag`, etc.).

### Prerequisites

- [Go](https://golang.org/dl/) (version 1.25 or higher, toolchain 1.26.1 recommended)
- Slurm CLI tools available in your `$PATH`

### Building from Source

1. Clone this repository:
   ```bash
   git clone https://github.com/sckyzo/slurm_exporter.git
   cd slurm_exporter
   ```

2. Build the exporter binary:
   ```bash
   make build
   ```

   The binary will be available in `bin/slurm_exporter`.

### Running Tests

To run all tests:

```bash
make test
```

### Development Commands

**Run the linter:**
```bash
golangci-lint run ./...
```

**Clean build artifacts:**
```bash
make clean
```

**Run the exporter locally:**
```bash
bin/slurm_exporter --web.listen-address=:8080
```

**Query metrics:**
```bash
curl http://localhost:8080/metrics
```

**Liveness probe:**
```bash
curl http://localhost:8080/healthz
# returns: ok
```

**Advanced build options:**
You can override the Go version and architecture via environment variables:
```bash
make build GO_VERSION=1.22.2 OS=linux ARCH=amd64
```

---

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

## 📡 Prometheus Configuration

```yaml
scrape_configs:
  - job_name: 'slurm_exporter'
    scrape_interval: 30s
    scrape_timeout: 30s
    static_configs:
      - targets: ['slurm_host.fqdn:9341']
```

- **scrape_interval**: A 30s interval is recommended to avoid overloading the Slurm master with frequent command executions.
- **scrape_timeout**: Should be equal to or less than the `scrape_interval` to prevent `context_deadline_exceeded` errors.

Check config:

```bash
promtool check-config prometheus.yml
```

### Internal Exporter Metrics

Each collector emits two self-monitoring metrics:

| Metric | Description | Labels |
|---|---|---|
| `slurm_exporter_collector_success` | `1` if last scrape succeeded, `0` if the collector panicked | `collector` |
| `slurm_exporter_collector_duration_seconds` | Wall time of the last `Collect()` call | `collector` |

These allow per-collector alerting independently of the global Prometheus `scrape_error`.

---

### Performance Considerations

- **Command Timeout**: The default timeout is 5 seconds. Increase it if Slurm commands take longer in your environment:
  
  ```bash
  ./slurm_exporter --command.timeout=10s
  ```

- **Scrape Interval**: Use at least 30 seconds to avoid overloading the Slurm controller with frequent command executions.

- **Collector Selection**: Disable unused collectors to reduce load and improve performance:
  
  ```bash
  ./slurm_exporter --no-collector.fairshare --no-collector.reservations
  ```

---

## 📈 Grafana Dashboard

A [Grafana dashboard](https://grafana.com/dashboards/4323) is available:

![Node Status](images/Node_Status.png)
![Job Status](images/Job_Status.png)
![Scheduler Info](images/Scheduler_Info.png)

---

## 📜 License

This project is licensed under the GNU General Public License, version 3 or later.

[![Buy Me a Coffee](https://storage.ko-fi.com/cdn/kofi6.png?v=6)](https://ko-fi.com/C0C514I8WG)

---

## 🍴 About this fork

This project is a **fork** of [cea-hpc/slurm_exporter](https://github.com/cea-hpc/slurm_exporter),
which itself is a fork of [vpenso/prometheus-slurm-exporter](https://github.com/vpenso/prometheus-slurm-exporter) (now apparently unmaintained).

Feel free to contribute or open issues!
