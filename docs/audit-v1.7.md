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
