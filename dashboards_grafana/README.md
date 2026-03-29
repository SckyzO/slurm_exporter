# Grafana Dashboards

Eight ready-to-use Grafana dashboards for the Prometheus Slurm Exporter.
All dashboards are compatible with **Grafana 12+** and use a `$datasource`
template variable for portability.

## Table of Contents

- [Import](#import)
- [Dashboards](#dashboards)
  - [Cluster Overview](#1-cluster-overview)
  - [Jobs & Queue](#2-jobs--queue)
  - [Node Detail](#3-node-detail)
  - [Cluster Usage Statistics](#4-cluster-usage-statistics)
  - [Scheduler](#5-scheduler)
  - [Exporter Health](#6-exporter-health)
  - [Reservations & Licenses](#7-reservations--licenses)
  - [All Metrics Reference](#8-all-metrics-reference)
- [Generating Screenshots](#generating-screenshots)

---

## Import

**Option 1 — Grafana UI:** Go to **Dashboards → Import**, paste a JSON file or upload it directly.

**Option 2 — Provisioning** (recommended for permanent setup):

```bash
cp dashboards_grafana/*.json /etc/grafana/provisioning/dashboards/
# Reload Grafana or wait for the provisioning interval (default: 30s)
```

**Option 3 — API** (batch import):

```bash
for f in dashboards_grafana/*.json; do
  curl -s -X POST http://admin:password@grafana-host:3000/api/dashboards/db \
    -H "Content-Type: application/json" \
    -d "{\"dashboard\": $(cat "$f"), \"overwrite\": true, \"folderId\": 0}"
done
```

---

## Dashboards

### 1. Cluster Overview

**File:** `slurm-overview.json` | **UID:** `slurm-overview`

Global cluster snapshot: node states, CPU/GPU utilization gauges, job totals,
per-partition table (CPU allocation, running/pending jobs), and job state history.

| Panel | Type | Description |
|-------|------|-------------|
| Total Nodes / Active / Running / Pending / CPU % / Down+Drain | stat | Quick health indicators |
| CPU Utilization | gauge | `slurm_cpus_alloc / slurm_cpus_total * 100` |
| Node Utilization | gauge | `(alloc + mix) / total * 100` |
| Jobs Over Time | timeseries | Running / Pending / Completing trend |
| Node State Breakdown | bargauge | Idle / Mixed / Alloc / Drain / Down / Maint |
| Node States Over Time | timeseries | Stacked node state history |
| CPUs Over Time | timeseries | Allocated / Idle / Total |
| Partitions | table | CPU Alloc / Total / % / Running Jobs / Pending Jobs per partition |
| Job States Over Time | timeseries | Running / Pending / Completing / Failed / Suspended |
| Cores Requested Over Time | timeseries | Cores running vs pending |
| Collector Status | stat | OK/FAIL per collector |
| Collector Duration | timeseries | Scrape duration history |

![Cluster Overview — top](screenshots/overview-1.png)
![Cluster Overview — node states](screenshots/overview-2.png)
![Cluster Overview — jobs & partitions](screenshots/overview-3.png)

---

### 2. Jobs & Queue

**File:** `slurm-jobs.json` | **UID:** `slurm-jobs`

Detailed job queue monitoring: global totals, top users, pending reasons,
per-partition breakdown, and job state timeline.

| Panel | Type | Description |
|-------|------|-------------|
| Running / Pending / Cores / Failed / Timeout | stat | Cluster-wide instant counters |
| All Job States | timeseries | All `slurm_jobs_*` globals over time |
| Cores Running & Pending | timeseries | CPU core demand trend |
| Top Users — Running Jobs | bargauge | `topk(15, slurm_user_jobs_running)` |
| Top Users — Running CPUs | bargauge | `topk(15, slurm_user_cpus_running)` |
| Top Accounts — Running CPUs | bargauge | `topk(15, slurm_account_cpus_running)` |
| Pending Jobs by Reason | bargauge | `sum by(reason) (slurm_queue_pending)` |
| Queue State by Partition | timeseries | Running / Pending per partition |
| Running vs Pending Timeline | state-timeline | Visual job activity history |

![Jobs & Queue — top](screenshots/jobs-1.png)
![Jobs & Queue — by user](screenshots/jobs-2.png)
![Jobs & Queue — timeline](screenshots/jobs-3.png)

---

### 3. Node Detail

**File:** `slurm-nodes.json` | **UID:** `slurm-nodes`

Per-node CPU and memory utilization. **Designed to scale from small clusters
to 100k+ nodes** via a `$partition` filter variable.

| Panel | Type | Description |
|-------|------|-------------|
| Total / Idle / Mixed / Alloc / Down / Drain | stat | Cluster-wide node counts |
| Node States per Partition | barchart | Stacked distribution by partition |
| Partition Summary | table | Alloc / Idle / Mix / Down / Drain per partition |
| Down & Drain Nodes | table | **Always scalable** — only shows degraded nodes |
| Node CPU & Memory Detail | table | Per-node: CPU alloc/total/%, Mem alloc/total/% — filtered by `$partition` |
| CPU Allocated per Node (top 10) | timeseries | Most loaded nodes over time |
| Memory Allocated per Node (top 10) | timeseries | Top memory consumers |

> **Scale note:** The per-node detail table is filtered by the `$partition` variable.
> On clusters with 100k+ nodes, always select a specific partition to limit results.
> The partition summary and "Down & Drain Nodes" panels are always O(partitions).

![Node Detail — summary & distribution](screenshots/nodes-1.png)
![Node Detail — problem nodes & per-node table](screenshots/nodes-2.png)
![Node Detail — timeseries](screenshots/nodes-3.png)

---

### 4. Cluster Usage Statistics

**File:** `slurm-usage.json` | **UID:** `slurm-usage`

Comprehensive utilization metrics: CPU/memory/GPU gauges, per-user and
per-account breakdowns, fairshare, scheduler health, and trend timeseries.

| Panel | Type | Description |
|-------|------|-------------|
| CPU / Node / Memory / GPU Utilization | gauge | Current utilization percentages |
| Avg CPU Util (period) | stat | `avg_over_time(cpu_util[1h:5m])` |
| Max CPU Util (period) | stat | `max_over_time(cpu_util[1h:5m])` |
| Total CPUs / Nodes / GPUs / Running Jobs | stat | Cluster capacity snapshot |
| CPU Utilization % Over Time | timeseries | Instantaneous + rolling average |
| Memory Utilization % Over Time | timeseries | `sum(node_mem_alloc) / sum(node_mem_total)` |
| GPU Utilization & Allocation % | timeseries | GPU util and alloc trend |
| Node States Over Time | timeseries | Idle / Mix / Alloc / Down / Drain |
| FairShare per Account | timeseries | Fairshare factor by account |
| Running / Pending Jobs per User & Account | timeseries | Per-user and per-account breakdown |
| CPUs Allocated per User & Account | timeseries | CPU demand by user/account |
| GPU Running per Account & User | timeseries | GPU job distribution |
| CPU Allocation (total) | timeseries | Alloc / Idle / Total CPUs |
| CPUs Allocated per Partition | timeseries | Per-partition CPU usage |
| GPU States Over Time | timeseries | Total / Alloc / Idle GPU trend |
| Scheduler Stats | stat + timeseries | Threads, queue size, cycle time, backfill |

![Cluster Usage — gauges & stats](screenshots/usage-1.png)
![Cluster Usage — timeseries utilization](screenshots/usage-2.png)
![Cluster Usage — per-user breakdown](screenshots/usage-3.png)

---

### 5. Scheduler

**File:** `slurm-scheduler.json` | **UID:** `slurm-scheduler`

Deep-dive into `slurmctld` internals: main scheduler and backfill cycle times,
RPC statistics, queue sizes, and thread counts.

| Panel | Type | Description |
|-------|------|-------------|
| Scheduler Threads | stat | Active scheduler threads |
| Queue Size | stat | Jobs in scheduler queue |
| Mean Cycle / Last Cycle | stat | Scheduler cycle times (µs) |
| DBD Queue | gauge | Slurm accounting daemon queue depth |
| Cycles/min | stat | Scheduler frequency |
| Scheduler Cycles (µs) | timeseries | Last vs mean cycle time |
| Backfill Cycles (µs) | timeseries | Backfill last vs mean |
| RPC Stats — Count | bargauge | Top RPCs by count |
| RPC Stats — Time | bargauge | Top RPCs by total time |
| RPC Calls Over Time | timeseries | RPC call frequency |
| User RPC Stats | table | Per-user RPC counts |

![Scheduler — overview](screenshots/scheduler-1.png)
![Scheduler — RPC stats](screenshots/scheduler-2.png)

---

### 6. Exporter Health

**File:** `slurm-health.json` | **UID:** `slurm-health`

Monitors the health of the exporter itself: collector success/failure,
scrape duration history, and Slurm binary availability.

| Panel | Type | Description |
|-------|------|-------------|
| Collector Status | stat | OK / FAIL per collector — background color alert |
| Collectors Healthy % | gauge | `sum(success) / count(success) * 100` |
| Collector Health Timeline | state-timeline | Visual OK/FAIL history per collector |
| Scrape Duration Status History | status-history | Duration colored by threshold (<1s green, 1-5s yellow, >5s red) |
| Last Scrape Duration | bargauge | Current duration per collector, sorted slowest-first |
| Duration Over Time | timeseries | Collector duration trend for detecting degradation |
| Slurm Version | stat | `slurm_info{type="general"}` — active Slurm version |
| Slurm Binaries | table | Availability check of each Slurm binary (sacct, sbatch, sinfo, …) |

![Exporter Health — status & timeline](screenshots/health-1.png)
![Exporter Health — duration & binaries](screenshots/health-2.png)

---

### 7. Reservations & Licenses

**File:** `slurm-reservations.json` | **UID:** `slurm-reservations`

Active Slurm reservations, per-reservation node states, and license usage.
License panels show "No data" when no licenses are configured — this is expected.

| Panel | Type | Description |
|-------|------|-------------|
| Active Reservations | stat | Count of current reservations |
| Reservation Info | table | Name / State / Users / Nodes / Partition / Flags |
| Reservation Timeline | timeseries | Start/end times as gauge |
| Nodes per Reservation | bargauge | Node counts by reservation |
| Node States per Reservation | timeseries | Alloc / Idle / Mix / Down / Drain per reservation |
| Reservation Nodes Healthy % | gauge | `(alloc+idle+mix+planned) / total_in_reservation` |
| License Usage | timeseries | Total / Used / Free / Reserved per license |
| License Utilization % | bargauge | `used / total * 100` per license |

> **Note:** License panels (`slurm_license_*`) only show data when Slurm licenses
> are configured (`scontrol show licenses`).

![Reservations & Licenses](screenshots/reservations-1.png)

---

### 8. All Metrics Reference

**File:** `slurm-all-metrics.json` | **UID:** `slurm-all-metrics`

An exhaustive reference dashboard showing **every metric** exported by the
Slurm Exporter, organized by collector. Useful for:
- Discovering available metrics
- Debugging queries
- Validating that all collectors are working

108 panels covering all 14 collectors:
`accounts`, `cpus`, `fairshare`, `gpus`, `info`, `licenses`, `node`, `nodes`,
`partitions`, `queue`, `reservation_nodes`, `reservations`, `scheduler`, `users`

> This dashboard is intentionally dense. Use the other focused dashboards for
> daily monitoring. This one is a reference/debug tool.

---

## Generating Screenshots

Screenshots are generated using Playwright in a Docker container:

```bash
# Prerequisite: Grafana running on localhost:3000
./scripts/take_screenshots.sh /tmp/screenshots
```

Or manually:

```bash
GSESSION=$(curl -s -D - -X POST http://localhost:3000/login \
  -H "Content-Type: application/json" \
  -d '{"user":"admin","password":"admin"}' \
  | grep 'grafana_session=' | grep -v expiry \
  | sed 's/.*grafana_session=\([^;]*\).*/\1/' | tr -d '\r\n')

docker run --rm --network slurm_slurm-network \
  -e "GSESSION=$GSESSION" -e "LANG=en_US.UTF-8" \
  -v /tmp/screenshots:/screenshots -w /work \
  mcr.microsoft.com/playwright:v1.51.1-noble \
  bash -c 'npm i -y playwright@1.51.1 > /dev/null && node script.js'
```

See [`scripts/take_screenshots.sh`](../scripts/take_screenshots.sh) for the full script.
