# Roadmap

> Back to [README](../README.md)

What's planned, in roughly the order we expect to ship it. This is a living
document — items that land in a release move to the [CHANGELOG](../CHANGELOG.md);
new items get appended here as they crystallise.

Sources for items below: open GitHub issues, follow-up commitments made
in PR/issue comments, and internal observations during recent releases.

---

## v1.9

> Tracked in [#61](https://github.com/SckyzO/slurm_exporter/issues/61) — the
> authoritative scope and per-PR breakdown for this milestone. Each item below
> maps to one atomic PR against `master`. v1.9 is cut once the public-feature
> checklist there is complete; internal-hygiene items are welcome but slide to
> v1.9.1 if they are not ready in time.

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

- **Dashboard uniformity — `$instance`** *(prerequisite for multi-cluster, tracked in [#61](https://github.com/SckyzO/slurm_exporter/issues/61))*
  Only `04-slurm-usage.json` currently carries an `$instance` template
  variable; the other 9 dashboards expose just a `${datasource}` picker and
  query bare metric names. Add a consistent `$instance` variable to all 10 so
  they behave uniformly — and to give the `$cluster` work below one place to
  hook into.

- **Multi-cluster dashboards** *(promised in the [issue #10 close-out](https://github.com/SckyzO/slurm_exporter/issues/10#issuecomment-4422385540))*
  Add a `$cluster` template variable to all 10 in-repo dashboards (none has
  one today). Default `allValue: ".*"` so single-cluster users see no change.
  Document the `external_labels: {cluster: ...}` Prometheus pattern and the
  Thanos / Mimir / Cortex equivalents.

### Internal hygiene (welcome but not promised)

- Convert `tmp/issue_collector_constructor_context.md` into a GitHub
  issue and ship the refactor (constructor signature gets `context.Context`
  as first parameter, eliminating the `nil`+override pattern in `main.go`).
- Convert `tmp/issue_gpus_single_sinfo.md` into a GitHub issue and
  consolidate the three `sinfo` calls in `internal/collector/gpus.go`
  into a single atomic snapshot. The v1.8.2 clamp on `slurm_gpus_other`
  becomes redundant once this lands.
- **`Makefile` container-first cleanup** *([#114](https://github.com/SckyzO/slurm_exporter/issues/114))*
  The "Docker-only, no host Go toolchain" contract only half-holds — `build`,
  `setup`, `run`, `clean` still use the host `go`. Also a stale `GO_VERSION`
  fallback (`1.22.2` at `Makefile:6`, vs `go 1.26.0` in `go.mod`) and a stale
  slurm-client comment (`23.11` at `Makefile:186`, vs the `25.11` the
  Dockerfile actually ships). Either containerise `build` or soften the claim,
  and derive the Go version from a single source. Splittable: the
  comment/version fixes can land ahead of the larger containerise-`build`
  change.

---

## v2.0 (uncommitted, open-ended)

- **Refondre le panel "Terminal Job States Over Time"** on
  `monitoring/grafana/dashboards/04-slurm-usage.json` once
  `sacct_efficiency` exposes the per-state counts (see v1.9). Today
  the panel uses queue-collector metrics that stay at zero because
  `squeue` doesn't surface terminal states.

---

## Requested, not yet scheduled

Open feature requests that are not committed to a milestone yet — surfaced here
so they are visible during v1.9 planning.

- **Job wait-time metrics** *([#118](https://github.com/SckyzO/slurm_exporter/issues/118))*
  Median / histogram wait times (submit → start) broken down by cluster,
  partition, account and user, for capacity planning and user-experience
  trends. Feasible from `squeue -O submittime,starttime` (running) or
  `sacct -X -o submit,start` (running + completed). Needs a cardinality and
  collector-cost decision (histogram buckets, whether it rides on the opt-in
  `sacct_efficiency` path) before it can be scoped into a release.

---

## Long-term, undecided

- **Posture toward Slurm 25.11+** — *decided: keep evolving.* Slurm 25.11
  ships a native OpenMetrics endpoint, but it exposes far fewer metrics than
  this exporter (per-user RPC stats, fairshare sub-metrics, the dashboard
  suite, etc.), so it does not replace it for most deployments. Decision:
  `slurm_exporter` stays actively maintained and keeps gaining features; the
  earlier freeze/deprecation wording has been removed from the README and
  `SECURITY.md`. The separate, from-scratch
  [sckyzo/slurm_prometheus_exporter](https://github.com/sckyzo/slurm_prometheus_exporter/)
  wraps the native endpoint — a different tool with a different scope, not a
  successor to this one.

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
