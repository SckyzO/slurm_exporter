# Prometheus rules for slurm_exporter

Alerting and recording rules ready to drop into a Prometheus deployment
scraping `slurm_exporter`. Site-neutral by design — no team labels, no
hardcoded runbook or dashboard URLs. Override at deploy time or via
Alertmanager routing.

## File index

| File | Purpose | Rule group |
|------|---------|------------|
| [`alerts.yml`](alerts.yml) | Alerting rules (node health, queue, scheduler, GPU) | `slurm.alerts` |
| [`rules.yml`](rules.yml) | Recording rules (pre-computed expressions used by some alerts) | `slurm.rules` |

Load both in Prometheus:

```yaml
# /etc/prometheus/prometheus.yml
rule_files:
  - /etc/prometheus/rules/slurm_alerts.yml
  - /etc/prometheus/rules/slurm_rules.yml
```

---

## `alerts.yml`

Exporter: [`slurm_exporter`](https://github.com/SckyzO/slurm_exporter)

| Alert | Warning Condition | Critical Condition | For |
|-------|-------------------|--------------------|-----|
| `SlurmNodeCritical` | — | status `down*` / `fail*` / `err*` | 2m |
| `SlurmNodeWarning` | status `drain*` / `maint*` | — | 5m |
| `SlurmPartitionNodesDown` | `slurm_nodes_down > 0` | `slurm_nodes_down > 5` | 10m |
| `SlurmJobsPendingHigh` | `slurm_jobs_pending > 500` | `slurm_jobs_pending > 1000` | 15m |
| `SlurmJobFailureRateHigh` | rate > 10% | rate > 25% | 15m |
| `SlurmSchedulerCycleSlow` | `last_cycle > 5s` | `last_cycle > 30s` | 5m |
| `SlurmSchedulerDBDQueueHigh` | `dbd_queue_size > 100` | — | 5m |
| `SlurmNoGPUsAvailable` | `slurm_gpus_idle == 0` (with GPUs configured) | + jobs pending | 30m |

`SlurmJobFailureRateHigh` depends on the `cluster:slurm_job_failure_rate:ratio15m`
recording rule from [`rules.yml`](rules.yml) — load both files together.

### ⚙️ Thresholds to calibrate

These defaults are reasonable for a medium-sized HPC cluster. Tune them
to your environment **before** wiring Alertmanager notifications.

- **`SlurmJobsPendingHigh`** (500 / 1000): scale with the number of nodes.
  On a 1000-node cluster, 500 pending jobs is normal; on a 50-node cluster
  it's a problem.
- **`SlurmSchedulerCycleSlow`** (5 s / 30 s): measure your `slurm_scheduler_last_cycle`
  at rest first. A healthy `slurmctld` on a quiet cluster typically reports
  cycles under 1 s.
- **`SlurmSchedulerDBDQueueHigh`** (100): if your DBD writes are typically
  flushed in seconds, even a depth of 20 is suspicious. Lower the threshold
  if you've never seen it climb in production.
- **`SlurmNoGPUsAvailable`** (`for: 30m`): the duration matters more than
  the threshold. Don't fire on every transient saturation.
- **`SlurmJobFailureRateHigh`** (10% / 25%): assumes a baseline of mostly
  successful jobs. Some testing clusters legitimately fail a lot — raise
  the thresholds accordingly or scope the alert to specific accounts.
- **`SlurmPartitionNodesDown`** critical (5): adjust for partition sizes;
  losing 5 of 8 nodes in a small partition is catastrophic, losing 5 of
  200 may be tolerable.

### 🚧 Optional alerts (not shipped)

These would be useful but require optional collectors or site-specific
configuration. They're documented here so you can add them locally:

- **`SlurmLicenseExhausted`** — alerts on `slurm_license_used / slurm_license_total`
  ratio. Requires the `licenses` collector (enabled by default) and that
  your Slurm controller actually tracks software licenses.

- **`SlurmLowCPUEfficiency` / `SlurmLowMemEfficiency`** — alerts on
  `slurm_job_cpu_efficiency_avg` / `slurm_job_mem_efficiency_avg`.
  Requires `--collector.sacct_efficiency` (disabled by default — opt-in
  because it queries SlurmDBD).

---

## `rules.yml`

| Recording rule | Definition |
|----------------|------------|
| `cluster:slurm_job_failure_rate:ratio15m` | `rate(slurm_scheduler_jobs_failed[15m]) / (rate(slurm_scheduler_jobs_submitted[15m]) > 0)` |

The numerator and denominator both come from `sdiag` and reset on every
`slurmctld` restart or `scontrol reconfigure` (they're gauges, not counters).
Prometheus' `rate()` will dip transiently at every reset; the `for: 15m`
on the matching alert smooths this out in practice.

---

## Conventions

### Labels

Only standard labels are present in the shipped rules:

| Label | Values | Purpose |
|-------|--------|---------|
| `severity` | `warning`, `critical` | Alertmanager routing |
| `component` | `hpc` | Filtering / namespacing |

Site-specific labels (`team`, `cluster`, `env`, `instance`) are **not**
set here — add them via:

- **Prometheus `external_labels`** (in `prometheus.yml`) for cluster-wide tags,
- **Alertmanager routing rules** for team / pager routing,
- **Manual edit** of the YAML if you really want them inline.

### Annotations

Each alert provides:

| Annotation | Content |
|------------|---------|
| `summary` | One-line headline, suitable for a Slack title or paging system |
| `description` | Full sentence with the affected resource and the observed value |

`runbook_url` and `dashboard_url` are **deliberately omitted**. Add them
via your local Alertmanager templating or a `sed`-time patch:

```bash
sed -i "s|runbook_url: ''|runbook_url: 'https://wiki.example.com/runbook/&'|" alerts.yml
```

### Recording rule naming

Format: `level:metric_name:operation`

Examples:
- `cluster:slurm_job_failure_rate:ratio15m` — cluster level, the failure
  ratio metric, computed over 15 minutes.
- `instance:slurm_node_cpu_utilization:percent` — instance level (if
  added later), CPU usage, expressed as percent.

---

## Validation

### Via `promtool` (if installed)

```bash
promtool check rules alerts.yml rules.yml
```

### Via Docker (no local install)

```bash
docker run --rm -v "$(pwd):/rules" \
  --entrypoint promtool prom/prometheus:latest \
  check rules /rules/alerts.yml /rules/rules.yml
```

Both should report `SUCCESS` with `15 rules found` (or current count).
