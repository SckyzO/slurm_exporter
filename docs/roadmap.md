# Roadmap

> Back to [README](../README.md)

What's planned, in roughly the order we expect to ship it. This is a living
document — items that land in a release move to the [CHANGELOG](../CHANGELOG.md);
new items get appended here as they crystallise.

Sources for items below: open GitHub issues, follow-up commitments made
in PR/issue comments, and internal observations during recent releases.

---

## 2.0

> Tracked in [#61](https://github.com/SckyzO/slurm_exporter/issues/61) — the
> authoritative scope and per-PR breakdown for this milestone. Each item below
> maps to one atomic PR against `master`. 2.0 is cut once the public-feature
> checklist there is complete; internal-hygiene items are welcome but slide to
> a later release if they are not ready in time.
>
> There is no 1.9. 1.8 is the last of the v1 line — it lives on `release-1.8`
> under the support window in [SECURITY.md](../SECURITY.md) — and everything
> since is 2.0, whose Go module path is `github.com/sckyzo/slurm_exporter/v2`.

### Commitments made publicly

- **Per-state job counts in `sacct_efficiency`** *(answers [#27](https://github.com/SckyzO/slurm_exporter/issues/27))*
  Extend the optional `sacct_efficiency` collector to expose
  `slurm_job_count_failed`, `_timeout`, `_preempted`, `_node_fail`,
  `_cancelled` per `account` + `user`, over the existing
  `--collector.sacct.lookback` window. Reuses the single `sacct` call
  already made for efficiency stats — no extra load on Slurm.

  `--collector.queue.terminal-states` already answers the *"my failure
  counters read zero"* half of #27 from `squeue`, but only within `MinJobAge`.
  This item is what makes the same question answerable over hours.

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

- **Constructor refactor — `context.Context` first** *(tracked in [#61](https://github.com/SckyzO/slurm_exporter/issues/61))*
  All 17 constructors still take `*logger.Logger` first, so the one collector
  that needs a context gets it through the `nil`-then-override pattern
  `main.go` still carries a comment about. Non-blocking by #61's own criteria:
  it changes no metric and no flag.

---

## After 2.0 (uncommitted, open-ended)

- **Rework the "Terminal Job States Over Time" panel** on
  `monitoring/grafana/dashboards/04-slurm-usage.json` once
  `sacct_efficiency` exposes the per-state counts (see 2.0 above). Today
  the panel uses queue-collector metrics that only carry values inside
  `MinJobAge`, so anything older than a few minutes reads as zero.

---

## Requested, not yet scheduled

Open feature requests that are not committed to a milestone yet — surfaced here
so they are visible during 2.0 planning.

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
   (e.g. *"I'll ship X in v2.1"*).
2. A draft issue exists in `tmp/` (gitignored scratch) that captures the
   problem and the proposed direction, waiting for a GitHub issue.
3. A change came up during a release validation pass and is too large to
   sneak into the patch.

Items leave when they ship — they go into `CHANGELOG.md` and are
removed from the roadmap on the same commit.
