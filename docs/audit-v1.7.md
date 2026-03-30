# Slurm Exporter — Audit Report v1.7.0

**Date:** 2026-03-31  
**Scope:** Axe 1 (command/format validity) + Axe 3 (parser quality/robustness)  
**Slurm version tested:** 25.11.2  
**Method:** context7 doc queries + live cluster validation

---

## Axe 1 — Command & Format Validation

### ✅ squeue formats — All valid in 25.11

| Used in | Format | Status | Notes |
|---------|--------|--------|-------|
| `accounts.go`, `users.go` | `%A\|%a\|%T\|%D\|%C\|%b` | ✅ Valid | JobID\|Account\|State\|NumNodes\|CPUs\|TRES |
| `queue.go` | `%P\|%T\|%C\|%r\|%u` | ✅ Valid | Partition\|State\|CPUs\|Reason\|User |
| `partitions.go` | `%P --states=PENDING/RUNNING` | ✅ Valid | — |

**New squeue fields available but unused:**

| Field | Specifier | Value for future use |
|-------|-----------|----------------------|
| Min memory requested | `%m` | e.g. `2G` — useful for memory demand metrics |
| Time limit | `%l` | e.g. `30:00` — useful for job efficiency |
| Expected start time | `%S` | ISO8601 — useful for wait time estimation |
| Eligible time | `%e` | Time job became eligible to run |
| Submit time | `%V` | Job submission timestamp |

---

### ✅ sinfo formats — All valid in 25.11

| Used in | Format | Status |
|---------|--------|--------|
| `cpus.go` | `%C` | ✅ Valid — `alloc/idle/other/total` |
| `nodes.go` | `%D\|%T\|%b` per partition | ✅ Valid |
| `nodes.go` | `%R` (partition list) | ✅ Valid |
| `partitions.go` | `%R,%C` | ✅ Valid |
| `gpus.go` | `--Format=Nodes:10 ,Gres:50 ,GresUsed:50` | ✅ Valid |
| `node.go` | `-N -O NodeList,AllocMem,Memory,CPUsState,StateLong,Partition` | ✅ Valid |

**New sinfo fields available but unused:**

| Field | Specifier | Value |
|-------|-----------|-------|
| Free memory (OS) | `%e` | Actual available RAM — detects memory pressure vs Slurm allocation |
| Reason (down/drain) | `%E` | Node unavailability reason string |
| Reason timestamp | `%H` | When the reason was set — ISO8601 |
| Total memory | `%m` | Available in node.go context |

> **Note:** `%E` and `%H` would enrich the "Problem Nodes" table in `slurm-nodes.json` significantly.

---

### ✅ sshare format — Valid, new fields available

Current: `Account,User,RawShares,NormShares,RawUsage,NormUsage,FairShare`

| New field | Description | Value |
|-----------|-------------|-------|
| `LevelFS` | FairShare at this level of the hierarchy | Shows per-level priority, not just leaf |
| `TRESRunMins` | TRES running minutes — breakdown by resource type | Multi-resource fairshare (CPU+GPU+mem) |

Live test confirms both fields work in 25.11: `LevelFS=1.200037`, `TRESRunMins=cpu=0,mem=0,...`

---

### ⚠️ sdiag — Significant gaps identified

14 fields currently parsed out of 35+ available.

**Missing fields by priority:**

🔴 **High value — actionable for alerting:**
| Field | Metric name | Why |
|-------|-------------|-----|
| `Jobs submitted` | `slurm_scheduler_jobs_submitted_total` | Rate → detects submission storms |
| `Jobs started` | `slurm_scheduler_jobs_started_total` | Rate → scheduler throughput |
| `Jobs completed` | `slurm_scheduler_jobs_completed_total` | Rate → cluster throughput |
| `Jobs canceled` | `slurm_scheduler_jobs_canceled_total` | Rate → user behavior anomaly |
| `Max cycle (main)` | `slurm_scheduler_max_cycle` | Latency spike detection |
| `Max cycle (bf)` | `slurm_scheduler_backfill_max_cycle` | BF latency spike detection |

🟠 **Medium value:**
| Field | Metric name | Why |
|-------|-------------|-----|
| `Total cycles (main)` | `slurm_scheduler_cycle_total` | Absolute cycle counter |
| `Last queue length` | `slurm_scheduler_last_queue_length` | Real-time queue pressure |
| `BF Last depth cycle` | `slurm_scheduler_backfill_last_depth` | BF effectiveness |
| `BF Queue length mean` | `slurm_scheduler_backfill_queue_mean` | BF pressure |
| `BF Table size` | `slurm_scheduler_backfill_table_size` | Memory usage in BF |

🟡 **New in 23.11+ — not yet parsed:**
| Field | Why |
|-------|-----|
| `Main scheduler exit reasons` (`end of queue`, `hit default_queue_depth`, ...) | Diagnose why scheduler stops early |
| `Backfill exit reasons` (`end of queue`, `hit bf_max_job_start`, ...) | Diagnose BF early termination |
| `Pending RPC statistics` | New section since 25.x |
| `Gettimeofday latency` | System health indicator |

---

### ✅ scontrol — Valid

`scontrol show nodes -o` and `scontrol show reservation` formats confirmed valid in 25.11.

---

## Axe 3 — Parser Quality & Robustness

### ✅ `queue.go` — pipe delimiter safe

`strings.SplitN(line, "|", 5)` correctly limits splits so that a reason field
containing `|` would not cause misparse. Confirmed: Slurm standard reasons
(`PartitionConfig`, `Priority`, `Resources`, `None`) do not contain `|`.

However: **`%r` (reason) CAN contain `:` characters** (e.g. `PartitionTimeLimit`,
`Dependency:jobid`). The current `SplitN(..., 5)` handles this correctly since
reason is field 4 (index 3) and user is field 5 (index 4) — safe.

### ✅ `gpus.go` — TRES regex robust

Regex `gres/gpu[^,\s]*[:/](\d+)` correctly handles all observed formats:
- `gres/gpu:4` ✅
- `gres/gpu:a100:2` ✅
- `gres/gpu:nvidia_gb200:8` ✅
- `gres/gpu:v100:4,gres/nic:2` ✅ (picks first GPU, ignores NIC)
- `N/A` → 0 ✅
- `gres/gpu=2` → 0 ⚠️ (`=` sign format: rare but theoretically possible)

> **Minor gap:** `gres/gpu=2` (equals sign) is not matched. In practice Slurm
> `%b` always uses `:` for TRES counts, but defensive coverage would be:  
> `gres/gpu[^,\s]*[:/=](\d+)`

### ✅ `nodes.go` — Node states comprehensive

All standard Slurm 25.11 node states are covered:

| State | Covered | Notes |
|-------|---------|-------|
| alloc, idle, mix, down, drain | ✅ | Core states |
| comp (completing) | ✅ | |
| resv (reserved) | ✅ | |
| maint (maintenance) | ✅ | |
| planned | ✅ | |
| fail, inval, err | ✅ via `other` | Mapped to `slurm_nodes_other` |
| POWER_DOWN, POWERING_UP | ⚠️ | New cloud/power states → fall to `other` |
| DYNAMIC, FUTURE, CLOUD | ⚠️ | Fall to `other` — correct but not explicit |
| UNKNOWN, NOT_RESPONDING | ⚠️ | Fall to `other` |

Cloud/power node states are correctly caught by the `other` bucket. No bug,
but worth documenting explicitly.

### ✅ `scheduler.go` — sdiag parsing robust

- `SplitN(line, ":", 2)` correctly handles timestamps like
  `Last cycle when: Wed Apr 12 11:03:21 2017` — verified.
- `lastCycleCount`/`meanCycleCount` counters correctly handle the fact that
  `Last cycle` and `Mean cycle` appear twice (main + backfill).

**Bug candidate:** The `schedulerPatternDBD` regex `^DBD Agent` matches
`DBD Agent queue size`. Verified working, but the regex is fragile —
`DBD Agent count` or `DBD Agent thread count` would also match and overwrite
the queue size value if they appear before it. Currently safe because Slurm
outputs them in a fixed order, but worth making more specific:
`^DBD Agent queue size`.

### ✅ `node.go` — sinfo -N -O format

`NodeList,AllocMem,Memory,CPUsState,StateLong,Partition` confirmed valid.
`StateLong` correctly returns `mixed`, `idle`, `draining`, etc. (not truncated).

One observation: nodes in multiple partitions appear multiple times.
The collector correctly uses a map keyed by node name — last partition wins.
This is documented behavior but could surprise users on overlapping partitions.

### ✅ `fairshare.go` — sshare parsing robust

`strings.TrimSpace()` on all fields handles indented sub-account lines correctly.
`parent` RawShares correctly skipped. Empty Account guard in place.

---

## Summary

### No breaking issues found

All command formats used by the exporter are valid in Slurm 25.11.
No regressions from 23.x to 25.11 detected.

### Issues to fix (minor)

| # | Severity | File | Issue | Fix |
|---|----------|------|-------|-----|
| 1 | 🟡 Low | `gpus.go` | `gres/gpu=2` not matched by TRES regex | Add `=` to char class: `[:/=]` |
| 2 | 🟡 Low | `scheduler.go` | `^DBD Agent` regex too broad | Use `^DBD Agent queue size` |

### Opportunities (Axe 2 candidates)

| Priority | Command | New data | Impact |
|----------|---------|----------|--------|
| 🔴 High | `sdiag` | `Jobs submitted/started/completed/canceled` counters | Scheduler throughput alerting |
| 🔴 High | `sdiag` | `Max cycle` (main + backfill) | Latency spike detection |
| 🟠 Medium | `sinfo %E %H` | Node reason + timestamp | Enrich Problem Nodes dashboard |
| 🟠 Medium | `sshare LevelFS` | Per-level fairshare | Hierarchical fairshare analysis |
| 🟡 Low | `squeue %m %l` | Memory/timelimit requested | Job efficiency preparation |
| 🟡 Low | `sdiag` | BF exit reasons (23.11+) | Backfill diagnosis |

---

*Next: Axe 2 (missing metrics — sstat, sacct efficiency) + Axe 4 (dashboard PromQL review)*

---

## Axe 2 — Missing Metrics (High Value)

### `sstat` — Real-time job resource usage

`sstat` provides live CPU/memory/I/O stats for **running** job steps.

**Available fields (Slurm 25.11):**

| Field | Description | Unit |
|-------|-------------|------|
| `AveCPU` | Average CPU time per task | HH:MM:SS |
| `AveRSS` | Average resident set size | KB |
| `AveVMSize` | Average virtual memory size | KB |
| `AveDiskRead` | Average bytes read from disk | bytes |
| `AveDiskWrite` | Average bytes written to disk | bytes |
| `MaxRSS` | Peak RSS across all tasks | KB |
| `ConsumedEnergyRaw` | Energy consumed (if IPMI/cgroup) | joules |
| `TRESUsageInTot` | TRES usage totals (CPU, mem, GPU) | various |
| `NTasks` | Number of tasks | — |

**Assessment:** `sstat` requires a call per job (or batch of job IDs via `-j id1,id2,...`).
The two-step approach (squeue → sstat) is valid but adds significant cost on large clusters.
`sleep`-based jobs return empty steps (no step registered), so sstat only works for real
computation jobs with `srun` steps.

> **Verdict: Medium priority.** Useful for `slurm_job_cpu_efficiency` metric but requires
> careful design to avoid O(running_jobs) cost. Better suited for a separate, opt-in
> collector with configurable sampling: `--collector.sstat.enabled=false` (default).

---

### `sacct` — Job efficiency metrics

`sacct` exposes post-completion job accounting. Key efficiency fields:

| Field | Description | Derivation |
|-------|-------------|-----------|
| `TotalCPU` | Actual CPU time used (user+system) | direct |
| `CPUTime` | Allocated CPU time (AllocCPUS × Elapsed) | direct |
| `CPUTimeRAW` | CPUTime in seconds | direct |
| **CPU Efficiency** | `TotalCPU / CPUTime × 100` | **computed** |
| `MaxRSS` | Peak memory per job | direct |
| `ReqMem` | Requested memory | direct |
| **Mem Efficiency** | `MaxRSS / ReqMem × 100` | **computed** |
| `ConsumedEnergyRaw` | Energy in joules | direct |
| `Elapsed` | Wall-clock time | direct |
| `TimelimitRaw` | Requested time limit in seconds | direct |

**Confirmed working in 25.11:** `sacct -X -P -n --format=JobID,User,Account,AllocCPUS,Elapsed,TotalCPU,CPUTime,State`

**Practical constraint:** sacct hits the SlurmDBD. The performance depends on the
accounting window. Using `--state=COMPLETED` with a rolling window (e.g., last 24h)
is manageable. Aggregating by user/account (not per-job) reduces cardinality drastically.

**Proposed metrics:**

```
slurm_job_cpu_efficiency_avg{account, user}   # avg(TotalCPU/CPUTime) last 24h
slurm_job_mem_efficiency_avg{account, user}   # avg(MaxRSS/ReqMem) last 24h
slurm_job_count_completed{account, user}      # jobs completed in window
```

> **Verdict: High priority for v1.8.** This is the most actionable new collector —
> "your jobs are only using 12% of the CPUs you requested" is directly addressable.
> Requires a new `sacct_efficiency` collector with a configurable lookback window
> (`--collector.sacct.lookback=24h`) and aggregation by account/user.

---

### `sinfo %E %H` — Node drain/down reason and timestamp

Both fields confirmed working in 25.11:
```
%E → "audit-test disk-slow"
%H → "2026-03-31T22:11:03"
```

**Proposed improvement:** Enrich the "Problem Nodes" table in `slurm-nodes.json`
and the "Down & Drain Nodes" panel with reason and timestamp.

Currently: `slurm_node_status{node, partition, status}` — no reason field.

Option A: Add a new metric `slurm_node_drain_reason_info{node, partition, reason}` (gauge=1, info-style).
Option B: Add `reason` label to `slurm_node_status` — but this creates cardinality issues
          if reasons change frequently (free-text field).

> **Verdict: Medium priority.** Option A (info metric) is the right design.
> Low cardinality, no churn. Very useful for the Problem Nodes dashboard panel.

---

### `sshare LevelFS + TRESRunMins`

- `LevelFS`: FairShare computed at each level of the account hierarchy (not just leaf).
  Useful for understanding fairshare in multi-tenant, hierarchical account structures.
- `TRESRunMins`: Running minutes by TRES type (cpu, mem, gres/gpu, ...).
  More granular than RawUsage — shows GPU-minutes separately.

> **Verdict: Low priority.** LevelFS is niche (multi-tier HPC centers only).
> TRESRunMins becomes interesting once GPU accounting is common.

---

### `sdiag` — Jobs submitted/started/completed counters

From the Axe 1 audit, these counters exist and are not collected:
- `Jobs submitted`, `Jobs started`, `Jobs completed`, `Jobs canceled`

These are **rate metrics** (counters since last stats reset) — suitable as Prometheus counters.
Rate = `increase(metric[5m])` in Grafana.

> **Verdict: High priority.** Easy to add (already in sdiag output, parser just needs
> 4 new regex patterns + 4 new prometheus.CounterValue metrics).

---

## Axe 4 — Dashboard PromQL Review

### Confirmed issues

| # | Dashboard | Panel | Issue | Severity |
|---|-----------|-------|-------|----------|
| 1 | `slurm-all-metrics.json` | P8, P17 | `slurm_cpus_alloc / slurm_cpus_total` — theoretical div0 if cluster unreachable | 🟡 Low (safe in practice) |
| 2 | `slurm-usage.json` | P1, P10, P12, P20 | Same as above — already has `$instance` filter | 🟡 Low |
| 3 | `slurm-overview.json` | P5, P10 | `slurm_cpus_alloc / slurm_cpus_total` — same | 🟡 Low |
| 4 | All GPU panels | multiple | `slurm_gpus_alloc / slurm_gpus_total` → ✅ **already fixed** with `clamp_min` | ✅ Fixed |
| 5 | `slurm-accounting.json` | P4, P5 | `count(... > 0)` → ✅ **already fixed** with `or vector(0)` | ✅ Fixed |

### No real issues found

After detailed analysis:

- **All `slurm_queue_*` usages are correct**: `sum by(reason)`, `sum by(partition)` aggregation
  properly used in jobs/all-metrics dashboards. Flagged "issues" were scanner false positives.
- **Memory expressions are correct**: `slurm_node_mem_alloc / 1024` for GB columns,
  `mem_alloc / mem_total * 100` for % columns. No unit errors.
- **No deprecated metrics** found anywhere.
- **topk() calls** are all legitimate and scoped (bargauge top-N, filtered by partition in nodes).
- **All avg_over_time() subqueries** already use `[Nh:5m]` notation — fixed previously.
- **Label mismatches** flagged by scanner are false positives: `slurm_cpus_alloc{instance,job}`
  and `slurm_cpus_total{instance,job}` have identical label sets — division works correctly.

### Minor observation

`slurm_partition_cpus_allocated / slurm_partition_cpus_total` in `slurm-overview.json`
(Partitions table) could theoretically return NaN for a GPU-only partition with 0 CPUs.
Confirmed: `partition=gpu` returns `NaN` in our test cluster.

**Fix:**

```promql
# Replace in slurm-overview.json P31 refId=C
sum by(partition) (slurm_partition_cpus_allocated) / 
  clamp_min(sum by(partition) (slurm_partition_cpus_total), 1) * 100
```

---

## Updated Backlog for v1.8

### Implement (confirmed value + feasible)

| Priority | Feature | Effort |
|----------|---------|--------|
| 🔴 High | `sdiag` jobs counters (submitted/started/completed/canceled) as `CounterValue` | Low — 4 regex + 4 descriptors |
| 🔴 High | `sacct_efficiency` collector — CPU/mem efficiency aggregated by user+account | Medium — new collector, rolling window |
| 🟠 Medium | `slurm_node_drain_reason_info{node,partition,reason}` via `sinfo %N %E %H` | Low — new metric in node.go |
| 🟠 Medium | Fix `slurm_partition_cpus_total=0` NaN in overview Partitions table | Trivial — add clamp_min |

### Discuss first

| Feature | Why discuss |
|---------|-------------|
| `sstat` collector | Cost model on large clusters — needs opt-in by default |
| `sshare LevelFS` | Only relevant for multi-tier account hierarchies |
| `sshare TRESRunMins` | Wait until GPU accounting is more common in user base |

---

*Audit complete. See also: [Axe 1 & 3 findings above](#axe-1--command--format-validation)*
