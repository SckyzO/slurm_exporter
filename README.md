# Prometheus Slurm Exporter 🚀

[![Release](https://github.com/sckyzo/slurm_exporter/actions/workflows/release.yml/badge.svg)](https://github.com/sckyzo/slurm_exporter/actions/workflows/release.yml)
[![Create Dev Release](https://github.com/sckyzo/slurm_exporter/actions/workflows/dev-release.yml/badge.svg)](https://github.com/sckyzo/slurm_exporter/actions/workflows/dev-release.yml)
[![GitHub release (latest by date)](https://img.shields.io/github/v/release/sckyzo/slurm_exporter)](https://github.com/sckyzo/slurm_exporter/releases/latest)
[![Go Report Card](https://goreportcard.com/badge/github.com/sckyzo/slurm_exporter)](https://goreportcard.com/report/github.com/sckyzo/slurm_exporter)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)

> 📸 **[View Dashboard Screenshots](#-screenshots)**

Prometheus collector and exporter for metrics extracted from the [Slurm](https://slurm.schedmd.com/overview.html) resource scheduling system.

> [!NOTE]
> Looking for a next-generation Slurm exporter with native OpenMetrics support (Slurm 25.11+)?
> Check out my new project: **[sckyzo/slurm_prometheus_exporter](https://github.com/sckyzo/slurm_prometheus_exporter/)**
>
> ✨ Features: Native OpenMetrics · Multiple endpoints · Basic Auth & TLS · Global labels · YAML config · Clean Architecture

## 📋 Table of Contents

- [✨ Features](#-features)
- [📦 Installation](#-installation)
- [⚙️ Configuration](docs/configuration.md) *(flags, collectors, Prometheus)*
- [📊 Metrics Reference](docs/metrics.md) *(all 14 collectors)*
- [🛠️ Development](docs/development.md) *(build, test, lint)*
- [📈 Grafana Dashboards](#-grafana-dashboards)
- [📸 Screenshots](#-screenshots)
- [📜 License](#-license)

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

## 📈 Grafana Dashboards

Ten ready-to-use Grafana dashboards are provided in the [`dashboards_grafana/`](dashboards_grafana/) directory.
All dashboards use a `$datasource` template variable and are compatible with Grafana 12+.

| Dashboard | UID | Description |
|-----------|-----|-------------|
| **Cluster Overview** | `slurm-overview` | Global cluster health: CPU/GPU utilization, node states, job totals, partition summary |
| **Jobs & Queue** | `slurm-jobs` | Job queue details by user, account, partition — pending reasons, top users |
| **Node Detail** | `slurm-nodes` | Per-node CPU & memory table (filtered by partition), scalable to 100k+ nodes |
| **Scheduler** | `slurm-scheduler` | slurmctld internals: cycle time, backfill, RPC statistics |
| **Reservations & Licenses** | `slurm-reservations` | Active reservations, node states per reservation, license usage |
| **Exporter Health** | `slurm-health` | Collector OK/FAIL status, scrape duration history, Slurm binary versions |
| **Cluster Usage Statistics** | `slurm-usage` | CPU/GPU utilization gauges, fairshare per account, top users by CPU |
| **All Metrics Reference** | `slurm-all-metrics` | Exhaustive reference panel for every exported metric |
| **Accounting** | `slurm-accounting` | User/account consumption, FairShare analysis, top consumers, priority diagnostics |
| **Exporter Performance** | `slurm-exporter-perf` | Command durations, cache freshness, error rates, scrape health (new in v1.8.0) |

### Import to Grafana

**Option 1 — Copy JSON files** to your Grafana provisioning directory:

```bash
cp dashboards_grafana/*.json /etc/grafana/provisioning/dashboards/
```

**Option 2 — Import via API:**

```bash
for f in dashboards_grafana/*.json; do
  curl -s -X POST http://admin:password@grafana-host:3000/api/dashboards/db \
    -H "Content-Type: application/json" \
    -d "{\"dashboard\": $(cat $f), \"overwrite\": true, \"folderId\": 0}"
done
```

> **Scale note (Node Detail dashboard):** The per-node table is filtered by the `$partition` variable.
> On clusters with 100k+ nodes, always select a specific partition to avoid loading excessive data.
> The partition summary and problem nodes panels are always scalable regardless of cluster size.

---

## 📸 Screenshots

> Screenshots taken on a 20-node test cluster (alice/bob/carol/dave/eve/frank, multiple accounts and partitions).
> Click any thumbnail to open the full-size image. See [`dashboards_grafana/README.md`](dashboards_grafana/README.md) for the full dashboard documentation.

<table>
<tr>
<td align="center" width="33%">

**Cluster Overview**<br>
<a href="dashboards_grafana/screenshots/overview-1.png">
  <img src="dashboards_grafana/screenshots/overview-1.png" width="100%" alt="Cluster Overview">
</a>

</td>
<td align="center" width="33%">

**Jobs & Queue**<br>
<a href="dashboards_grafana/screenshots/jobs-1.png">
  <img src="dashboards_grafana/screenshots/jobs-1.png" width="100%" alt="Jobs & Queue">
</a>

</td>
<td align="center" width="33%">

**Node Detail** *(scalable 100k+ nodes)*<br>
<a href="dashboards_grafana/screenshots/nodes-1.png">
  <img src="dashboards_grafana/screenshots/nodes-1.png" width="100%" alt="Node Detail">
</a>

</td>
</tr>
<tr>
<td align="center" width="33%">

**Cluster Usage Statistics**<br>
<a href="dashboards_grafana/screenshots/usage-1.png">
  <img src="dashboards_grafana/screenshots/usage-1.png" width="100%" alt="Cluster Usage Statistics">
</a>

</td>
<td align="center" width="33%">

**Scheduler**<br>
<a href="dashboards_grafana/screenshots/scheduler-1.png">
  <img src="dashboards_grafana/screenshots/scheduler-1.png" width="100%" alt="Scheduler">
</a>

</td>
<td align="center" width="33%">

**Exporter Health**<br>
<a href="dashboards_grafana/screenshots/health-1.png">
  <img src="dashboards_grafana/screenshots/health-1.png" width="100%" alt="Exporter Health">
</a>

</td>
</tr>
<tr>
<td align="center" width="33%">

**Reservations & Licenses**<br>
<a href="dashboards_grafana/screenshots/reservations-1.png">
  <img src="dashboards_grafana/screenshots/reservations-1.png" width="100%" alt="Reservations & Licenses">
</a>

</td>
<td align="center" width="33%">

**Accounting** *(new in v1.7.0)*<br>
<a href="dashboards_grafana/screenshots/accounting-1.png">
  <img src="dashboards_grafana/screenshots/accounting-1.png" width="100%" alt="Accounting">
</a>

</td>
<td align="center" width="33%">

**Exporter Performance** *(new in v1.8.0)*<br>
<a href="dashboards_grafana/screenshots/exporter-perf-1.png">
  <img src="dashboards_grafana/screenshots/exporter-perf-1.png" width="100%" alt="Exporter Performance">
</a>

</td>
</tr>
<tr>
<td align="center" colspan="3">

*All 10 dashboards documented in [`dashboards_grafana/README.md`](dashboards_grafana/README.md)*

</td>
</tr>
</table>

## 📜 License

This project is licensed under the GNU General Public License, version 3 or later.

[![Buy Me a Coffee](https://storage.ko-fi.com/cdn/kofi6.png?v=6)](https://ko-fi.com/C0C514I8WG)

---

## 🍴 About this fork

This project is a **fork** of [cea-hpc/slurm_exporter](https://github.com/cea-hpc/slurm_exporter),
which itself is a fork of [vpenso/prometheus-slurm-exporter](https://github.com/vpenso/prometheus-slurm-exporter) (now apparently unmaintained).

Feel free to contribute or open issues!
