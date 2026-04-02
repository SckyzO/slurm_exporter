# Configuration Reference

> Back to [README](../README.md)

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
| `--[no-]collector.<name>` | Enable or disable a collector (kingpin boolean flag). Most collectors default to enabled; `sacct_efficiency` defaults to disabled. | see below |
| `--collector.nodes.feature-set` | Include `active_feature_set` label in `slurm_nodes_*` metrics | `true` |
| `--collector.fairshare.user-metrics` | Collect per-user fairshare metrics (`slurm_user_fairshare_*`). Disable on clusters with many users to reduce cardinality. | `true` |
| `--web.disable-exporter-metrics` | Exclude Go runtime and process metrics from `/metrics` | `false` |

**Available collectors:**

| Collector | Default | Description |
|-----------|---------|-------------|
| `accounts` | enabled | Job stats by Slurm account |
| `cpus` | enabled | Cluster-wide CPU states |
| `drain_reason` | enabled | Node drain/down reason and timestamp |
| `fairshare` | enabled | Fairshare factor per account and user |
| `gpus` | enabled | Cluster-wide GPU states |
| `info` | enabled | Slurm binary versions |
| `licenses` | enabled | License counts |
| `node` | enabled | Per-node CPU and memory detail |
| `nodes` | enabled | Aggregated node states by partition |
| `partitions` | enabled | CPU states and jobs per partition |
| `queue` | enabled | Job states and core counts by user/partition |
| `reservation_nodes` | enabled | Node states per reservation |
| `reservations` | enabled | Active reservation details |
| `sacct_efficiency` | **disabled** | CPU/mem job efficiency via sacct (queries SlurmDBD) |
| `scheduler` | enabled | slurmctld internals and RPC stats |
| `users` | enabled | Job stats by user |

### Enabling and Disabling Collectors

Most collectors are **enabled** by default. The `sacct_efficiency` collector is **disabled** by default because it queries SlurmDBD and can be expensive — enable it explicitly with `--collector.sacct_efficiency`.

Use `--[no-]collector.<name>` (kingpin boolean syntax) to enable or disable individual collectors.

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

## 📈 Grafana Dashboards
