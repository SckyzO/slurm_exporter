# Test Cluster — Setup & Usage

Scripts to spin up a local Slurm cluster pre-configured for testing the
Prometheus Slurm Exporter and its Grafana dashboards.

The cluster is based on **[giovtorres/slurm-docker-cluster](https://github.com/giovtorres/slurm-docker-cluster)**.
The monitoring stack (Prometheus + Grafana) is provided here and deployed
automatically by `make setup`.

## Prerequisites

### 1. slurm-docker-cluster

Clone the official repository:

```bash
git clone https://github.com/giovtorres/slurm-docker-cluster.git ~/slurm-docker-cluster
```

The Makefile auto-detects this path. If you clone it elsewhere, set it in
`cluster.local.conf` (see [Configuration](#configuration)).

> **No build needed.** Pre-built images are pulled automatically from Docker Hub.

### 2. Go toolchain

Required to build the exporter binary. Go 1.25+ — see the
[main README](../../README.md) for details.

That's all. Docker and Docker Compose v2 are assumed to be installed.

---

## Quick Start

```bash
# From the repo root:
make -C scripts/testing setup

# Or from this directory:
cd scripts/testing
make setup
```

`make setup` handles **everything** automatically:

| Step | What happens |
|------|-------------|
| 1 | Pull `giovtorres/slurm-docker-cluster` Docker image from Docker Hub |
| 2 | Write `.env` with the configured node count |
| 3 | Start the cluster (`NODES` CPU workers) |
| 4 | Copy monitoring files + start Prometheus and Grafana |
| 5 | Create Slurm accounts (`hpc_team`, `ml_group`, `physics`, `bio`) |
| 6 | Create Slurm users (`alice`, `bob`, `carol`, `dave`, `eve`, `frank`) |
| 7 | Create OS users in slurmctld + all worker containers |
| 8 | Create extra partitions (`debug`, `high`) |
| 9 | Build and deploy the exporter |
| 10 | Import all 8 Grafana dashboards via API |

**Endpoints when running:**

| Service | URL |
|---------|-----|
| Grafana | http://localhost:3000 (admin / admin) |
| Prometheus | http://localhost:9090 |
| Exporter | http://localhost:9341/metrics |

---

## All Targets

```
make setup                Full one-shot setup (first time)
make start                Start an already-configured cluster
make stop                 Stop cluster (data preserved)
make clean                Full teardown (removes Docker volumes)
make status               Show nodes, jobs, exporter health
make workload [N=30]      Submit N random jobs
make screenshots          Take Grafana dashboard screenshots
make node-fail            Set 1-2 random nodes to down/drain
make node-restore         Restore all degraded nodes
make workload-gpu         Submit GPU jobs (requires GRES)
make gpu-workers          Add 10 fake GPU nodes, two models, no hardware
make gpu-workers-down     Remove them, node records included
make cancel-all           Cancel all running/pending jobs
make redeploy             Rebuild exporter + reimport dashboards
make redeploy-dashboards  Reimport dashboards only
make show-config          Print effective configuration
make logs                 Follow slurmctld + slurmdbd logs
make exporter-logs        Print the exporter's own log
```

### Reading the exporter log

The exporter runs inside `slurmctld` with its output captured in
`/tmp/slurm_exporter.log`; `docker logs slurmctld` shows the container's main
process, not the exporter. Step 7 of the
[Definition of Done](../../CONTRIBUTING.md#definition-of-done) reads that file:

```bash
docker exec slurmctld curl -s http://localhost:9341/metrics > /dev/null
make exporter-logs | grep -E 'level=(ERROR|WARN)'
```

A collector whose command succeeds but whose output makes no sense still
reports `slurm_exporter_collector_success` = 1 — the warning in this log is the
only signal. `make redeploy` truncates the file, so it always belongs to the
binary currently under test.

---

## Configuration

Settings are in `cluster.conf`. Create `cluster.local.conf` next to it
for machine-specific overrides — it is gitignored.

```bash
# Example cluster.local.conf
CLUSTER_DIR=/opt/my-slurm-cluster
NODES=20
```

### Key settings

| Variable | Default | Description |
|----------|---------|-------------|
| `CLUSTER_DIR` | auto-detect | Path to slurm-docker-cluster clone |
| `SLURM_VERSION` | `25.11.2` | Slurm version (must be on Docker Hub) |
| `NODES` | `10` | Number of CPU compute nodes |
| `GRAFANA_URL` | `http://localhost:3000` | Grafana endpoint |
| `ACCOUNTS` | hpc_team ml_group physics bio | Slurm accounts (`name:desc:org`) |
| `USERS` | alice bob carol dave eve frank | Test users (`name:uid:account`) |
| `PARTITIONS` | debug high | Extra partitions (`name:nodes:maxtime_min:priority:default`) |
| `PLAYWRIGHT_IMAGE` | `mcr.microsoft.com/playwright:v1.58.2-noble` | Playwright Docker image |
| `PLAYWRIGHT_VERSION` | `1.58.2` | npm playwright version |

### Auto-detection of CLUSTER_DIR

`_lib.sh` looks for the cluster in these locations (in order):

1. `../../../orchestration-hpc/slurm-docker-cluster` *(relative to repo root)*
2. `~/slurm-docker-cluster`
3. `~/dev/slurm-docker-cluster`
4. `~/projects/slurm-docker-cluster`

If none found, set it explicitly in `cluster.local.conf`.

### Adding accounts or users

Edit `cluster.conf`, then apply without restarting:

```bash
bash _lib.sh setup-accounting
bash _lib.sh setup-os-users
```

### Adding partitions

```bash
# cluster.conf — format: NAME:NODES:MAXTIME_MIN:PRIORITY:DEFAULT
PARTITIONS="debug:c[1-2]:30:1:NO high:c[1-3]:0:100:NO bigmem:c[4-6]:240:50:NO"
```

Apply: `bash _lib.sh setup-partitions`

---

## Monitoring

The `monitoring/` directory in this folder contains all files needed to run
Prometheus and Grafana alongside the cluster. `make setup` copies them
automatically into the cluster directory before starting:

```
monitoring/
├── docker-compose.monitoring.yml   # Prometheus + Grafana services
├── prometheus.yml                  # Scrape config → slurmctld:9341
└── grafana/provisioning/
    ├── datasources/prometheus.yml  # Grafana datasource (auto-configured)
    └── dashboards/dashboards.yml   # Dashboard provisioning config
```

Dashboards are imported via the Grafana API by `make setup` (and `make redeploy-dashboards`),
not through file provisioning — so they always reflect the latest JSON files.

---

## GPU nodes, without a GPU

```bash
make gpu-workers            # 10 nodes: 4 model_a, 4 model_b, 2 carrying both
make gpu-workers GPU_N=4    # fewer
make gpu-workers-down       # remove them
make workload-gpu           # submit jobs that request GPUs
```

No NVIDIA hardware, no `nvidia-container-toolkit`, no change to the cluster
clone. A dynamic `slurmd` (`-Z`) declares its own resources when it registers,
and Slurm treats a GRES backed by an ordinary device node exactly like one
backed by a real card.

The layout is deliberate. Homogeneous nodes are what a site actually buys, so
eight of the ten carry a single model; mixed-model nodes produce the GRES string
that breaks naive parsers (`gpu:model_a:2,gpu:model_b:2` with a separate index
list per model), so two of them carry both. One cluster covers the common case
and the awkward one, and gives per-model aggregates that mean something:

```
gpu:model_a   total=20  used=8
gpu:model_b   total=20  used=2
```

That is the question `slurm_node_gres_*` exists to answer — one model saturated
while the other idles — and neither `slurm_gpus_*` (no labels) nor
`slurm_partition_gpus_*` (stops at the partition) can express it.

Four details are load-bearing, each of them a failure that was hit while
building this:

- **`--privileged --cgroupns private`**, or slurmd exits on `Unable to
  initialize cgroup plugin` before it ever registers.
- **`File=` is mandatory for a gpu-typed GRES**, one device per card. Without it
  slurmd logs `Ignoring file-less GPU ... from final GRES list`, the resource
  never reaches the usable list, and the node lands in `inval`.
- **`/etc/slurm` is shared with the whole cluster.** Each fake node writes its
  own `gres.conf` under a private `SLURM_CONF`; editing the shared volume would
  reconfigure every other node and outlive the run.
- **slurmctld remembers dynamic nodes.** `make gpu-workers-down` deletes the
  node records as well as the containers, discovering them from `sinfo` rather
  than assuming a range, so shrinking the cluster does not leave ghost nodes
  still advertising GPUs.

Captures taken this way live under `test_data/slurm-<version>/` like any other
fixture, and are real `sinfo` output rather than hand-written lines.

---

## Typical Workflow

```bash
# First time
make setup

# Generate activity
make workload N=40

# Simulate node failures for dashboard testing
make node-fail
sleep 120
make node-restore

# Take screenshots of all dashboards
make screenshots OUTPUT=~/screenshots

# After code changes
make redeploy

# End of session
make stop       # keeps volumes — resume with make start

# Full reset
make clean
```
