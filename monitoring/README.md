# Monitoring assets

Everything you need to plug `slurm_exporter` into a Prometheus +
Alertmanager + Grafana stack, ready to drop into a running cluster
without having to read any code first.

```
monitoring/
├── grafana/
│   └── dashboards/         10 ready-to-import Grafana dashboards (+ screenshots)
└── prometheus/
    ├── alerts.yml          alerting rules (severity-based, site-neutral)
    └── rules.yml           recording rules (pre-computed expressions)
```

## Grafana dashboards

The ten JSON files under `grafana/dashboards/` are importable as-is via
the Grafana UI ("+ → Import → Upload JSON file"), the Grafana HTTP API, or
file-provisioning. The README in that folder documents each dashboard's
intent and the metrics it consumes.

Screenshots of every dashboard are under
`grafana/dashboards/screenshots/`.

## Prometheus rules

See [`prometheus/README.md`](prometheus/README.md) for the full inventory
of alerts and recording rules — threshold tables, calibration notes,
labelling conventions, and validation recipes.

### `alerts.yml`

A starter set of alerting rules tuned for medium-to-large HPC clusters.
Each alert carries:

- `severity: warning` or `critical` — Alertmanager routing.
- `component: hpc` — for filtering / namespacing.
- `summary` and `description` annotations.

Site-specific labels (`team`, `runbook_url`, `dashboard_url`, etc.) are
**intentionally not in this file**. They belong in your local override —
either by editing the file at deploy time, by Prometheus `external_labels`,
or by Alertmanager routing config. This keeps the file portable.

Alert thresholds (500 pending jobs, 5s scheduler cycle, etc.) are
reasonable defaults; tune them to your cluster size.

### `rules.yml`

Recording rules. Currently one rule:

- `cluster:slurm_job_failure_rate:ratio15m` — failed jobs over total
  submissions on a 15-minute rolling window, derived from the
  `slurm_scheduler_jobs_*` gauges.

The recording rule is required by the `SlurmJobFailureRateHigh` alert.

## Wiring it up in Prometheus

```yaml
# /etc/prometheus/prometheus.yml
scrape_configs:
  - job_name: slurm_exporter
    static_configs:
      - targets: ['slurmctld:9341']

rule_files:
  - /etc/prometheus/rules/slurm_alerts.yml
  - /etc/prometheus/rules/slurm_rules.yml
```

Place the two YAML files under `/etc/prometheus/rules/` (or your equivalent
path), reload Prometheus (`SIGHUP` or `/-/reload`), and check the **Status
→ Rules** page to confirm both groups (`slurm.alerts` and `slurm.rules`)
load without errors.

## What's not in this folder

- **Alertmanager configuration**: routing, silencing, notification
  receivers. Too site-specific — see the
  [Alertmanager docs](https://prometheus.io/docs/alerting/latest/configuration/).
- **Prometheus storage / retention / scrape interval**: same reason.
- **Grafana datasource configuration**: depends on your Grafana topology.
  For a quick-start setup, see `scripts/testing/monitoring/`.

If you want a full working end-to-end example (Prometheus + Grafana +
slurm_exporter wired up in Docker Compose), look at
[`scripts/testing/`](../scripts/testing/) — it builds the whole stack
locally on top of the
[giovtorres/slurm-docker-cluster](https://github.com/giovtorres/slurm-docker-cluster)
image and reuses the dashboards and Prometheus config from this directory.
