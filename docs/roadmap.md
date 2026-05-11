# Roadmap

> Back to [README](../README.md)

What's planned, in roughly the order we expect to ship it. This is a living
document — items that land in a release move to the [CHANGELOG](../CHANGELOG.md);
new items get appended here as they crystallise.

Sources for items below: open GitHub issues, follow-up commitments made
in PR/issue comments, and internal observations during recent releases.

---

## v1.9

### Commitments made publicly

- **Per-state job counts in `sacct_efficiency`** *(answers [#27](https://github.com/SckyzO/slurm_exporter/issues/27))*
  Extend the optional `sacct_efficiency` collector to expose
  `slurm_job_count_failed`, `_timeout`, `_preempted`, `_node_fail`,
  `_cancelled` per `account` + `user`, over the existing
  `--collector.sacct.lookback` window. Reuses the single `sacct` call
  already made for efficiency stats — no extra load on Slurm.

- **Per-node GRES metrics** *(adapts [PR #29](https://github.com/SckyzO/slurm_exporter/pull/29) from @ncreddine)*
  Land `slurm_node_gres_total{node, partition, status, gres_type}` and
  `slurm_node_gres_used{...}`. Adapt to the variable-width `sinfo -O`
  format introduced in v1.8.2 (the original PR uses fixed widths and
  would regress issue #10). Add a `--collector.node.gres` flag and a
  `--collector.node.gres-types` filter for cardinality control on
  multi-type / MIG clusters. Includes a new dashboard panel.

- **Multi-cluster dashboards** *(promised in the [issue #10 close-out](https://github.com/SckyzO/slurm_exporter/issues/10#issuecomment-4422385540))*
  Add a `$cluster` template variable to the in-repo dashboards. Default
  `allValue: ".*"` so single-cluster users see no change. Document the
  Prometheus relabel patterns (single Prometheus, Thanos/Mimir/Cortex,
  federation).

### Internal hygiene (welcome but not promised)

- Convert `tmp/issue_collector_constructor_context.md` into a GitHub
  issue and ship the refactor (constructor signature gets `context.Context`
  as first parameter, eliminating the `nil`+override pattern in `main.go`).
- Convert `tmp/issue_gpus_single_sinfo.md` into a GitHub issue and
  consolidate the three `sinfo` calls in `internal/collector/gpus.go`
  into a single atomic snapshot. The v1.8.2 clamp on `slurm_gpus_other`
  becomes redundant once this lands.

---

## v2.0 (uncommitted, open-ended)

- **Refondre le panel "Terminal Job States Over Time"** on
  `monitoring/grafana/dashboards/04-slurm-usage.json` once
  `sacct_efficiency` exposes the per-state counts (see v1.9). Today
  the panel uses queue-collector metrics that stay at zero because
  `squeue` doesn't surface terminal states.

---

## Long-term, undecided

- **Posture toward Slurm 25.11+**. Slurm 25.11 ships a native OpenMetrics
  endpoint, which makes part of this exporter redundant for new
  deployments. We need to decide whether to:
  - position `slurm_exporter` as a back-compat tool for older Slurm
    versions and gradually freeze new feature work,
  - keep building on top of it because it offers metrics and dashboards
    the native endpoint doesn't (per-user RPC stats, fairshare
    sub-metrics, the dashboard suite, etc.),
  - or evolve into a complement to the native endpoint, scraping it and
    exposing higher-level / cross-cutting metrics.

  The README currently mentions a freeze stance; that note pre-dates the
  current backlog of contributions and probably needs revisiting once
  we've seen how the v1.8.x line is used in the wild.

---

## How items land here

A new item is added to this roadmap when **any** of the following is
true:

1. A maintainer publicly commits to it in a PR or issue comment
   (e.g. *"I'll ship X in v1.9"*).
2. A draft issue exists in `tmp/` (gitignored scratch) that captures the
   problem and the proposed direction, waiting for a GitHub issue.
3. A change came up during a release validation pass and is too large to
   sneak into the patch.

Items leave when they ship — they go into `CHANGELOG.md` and are
removed from the roadmap on the same commit.
