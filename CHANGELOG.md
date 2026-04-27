# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.8.1] - 2026-04-28

### 🐛 Bug Fixes

- **Dashboards — empty node states no longer break panels:** `count()` over an empty
  vector returns no samples (not `0`) in PromQL, so `count(stateA) + count(stateB)`
  silently returned "No data" whenever either side was empty. Six expressions in
  `slurm-overview` and `slurm-usage` (Active, Down+Drain, Node %, Avg Node %,
  Allocated+Completing) rewritten to use a single regex
  (`status=~"alloc.*|mix.*"`) — also avoids double-counting nodes that appear
  in multiple partitions. Plus 43 isolated `count(slurm_node_status{...})` panels
  now use `or vector(0)` so empty states render as `0` instead of "No data".

- **Multi-partition clusters — node state metrics double-counted:** nodes belonging
  to multiple partitions were counted once per partition. Fixed by adding
  `count by(node)` deduplication.

### 📋 Documentation

- `README.md` split into focused files under `docs/` (configuration, metrics, dashboards).
- `docs/configuration.md`: corrected collector flags and defaults.
- Full audit pass — missing flags, collectors, and metrics for v1.8 documented.
- Grafana dashboards renumbered for pyramid ordering in the dashboards UI.

### 🔧 Maintenance

- Bump `prometheus/exporter-toolkit` v0.15.1 → v0.16.0 (Go 1.26 support, dependency-only release, no breaking changes).
- Bump direct + indirect `golang.org/x/*` packages: crypto, net, sys, text, term, mod, tools.

---

## [1.8.0] - 2026-04-01

### ✨ Features

- **`sacct_efficiency` collector** (disabled by default — opt-in):
  - `slurm_job_cpu_efficiency_avg{account,user}` — avg(TotalCPU/CPUTime×100) over lookback window
  - `slurm_job_mem_efficiency_avg{account,user}` — avg(MaxRSS/ReqMem×100) over lookback window
  - `slurm_job_count_completed{account,user}` — jobs in lookback window
  - `slurm_job_cpu_hours_allocated{account,user}` — allocated CPU-hours in lookback window
  - `slurm_sacct_last_refresh_timestamp_seconds` — unix ts for staleness alerting
  - Background goroutine + RWMutex cache: Collect() is non-blocking, zero scrape timeout risk
  - Flags: `--collector.sacct_efficiency`, `--collector.sacct.interval=5m`, `--collector.sacct.lookback=1h`
  - Requires `JobAcctGatherType=jobacct_gather/linux|cgroup` in slurm.conf for CPU/mem data

- **sdiag job lifecycle counters** (zero extra RPC cost — already calling sdiag):
  - `slurm_scheduler_jobs_submitted_total`, `_started_total`, `_completed_total`, `_canceled_total`, `_failed_total`
  - Rate metric: `rate(slurm_scheduler_jobs_submitted_total[5m])` = scheduler throughput

- **`slurm_node_drain_reason_info{node,reason,since}`** — info-style metric for degraded nodes.
  Only emitted for drain/down nodes with an admin-set reason (not "none"/"not responding").
  Zero cardinality on healthy clusters.

- **New `slurm-exporter-perf` dashboard** (10th dashboard):
  Command duration p99/avg, call counts, error rates, scontrol cache age, sacct refresh age.
  Use to validate Axe 2 optimisations and detect slurmctld load.

### ⚡ Performance

- **sinfo: N per-partition calls → 1 global call** (`sinfo -h -o "%R|%D|%T|%b"`):
  Measured reduction: 112 → 10 calls per scrape window on a 4-partition cluster.
  On a 50-partition cluster: 50× less sinfo RPCs per scrape.

- **scontrol show nodes -o: 2 calls → 1 cached call**:
  nodes.go and reservation_nodes.go now share a `timedCache` (TTL=25s).
  `slurm_exporter_cache_age_seconds{cache="scontrol_nodes"}` reports freshness.

### 📊 New internal metrics

- `slurm_exporter_command_duration_seconds{command}` — histogram (11 buckets)
- `slurm_exporter_command_errors_total{command}` — error counter per CLI command
- `slurm_exporter_cache_age_seconds{cache}` — cache freshness gauge
- `slurm_sacct_last_refresh_timestamp_seconds` — sacct background refresh timestamp

### 🧪 Tests & Quality

- **Coverage: 57% → 81%** (+24 points):
  - gpus, nodes, scheduler, reservation_nodes, queue collectors fully covered
  - cache_test.go: 5 tests including concurrent access test
  - sacct_efficiency_test.go: 14 tests covering parsers, aggregation, collector lifecycle
  - node_drain_test.go: 6 tests
  - `test_data/sacct_efficiency.txt` fixture added
- **`CONTRIBUTING.md`**: full 10-step Definition of Done protocol for all PRs
- **Package comments** (`doc.go`): collector and cmd packages documented
- **`disabledByDefault` map** in main.go for future opt-in collectors

### 📋 Documentation

- `README.md`: 10 dashboards, new flags, new metrics documented
- `dashboards_grafana/README.md`: slurm-exporter-perf section added
- `test_data/readme.md`: sacct_efficiency and node_drain documented

---

## [1.7.1] - 2026-03-31

### 🐛 Bug Fixes

- **`slurm-accounting` dashboard — Active Users/Accounts "No data":** `count(metric > 0)` returns an empty result set in PromQL when no series match, causing stat panels to show "No data" instead of `0`. Fixed with `or vector(0)` fallback.
- **Dashboards — percentage formatting:** All `percent`/`percentunit` panels without explicit decimal settings now display 1 decimal place (e.g. `87.5%` instead of `87.54321%`). FairShare panels reduced from 3 to 1 decimal. 21 fixes across `slurm-accounting`, `slurm-all-metrics` and `slurm-usage` dashboards.
- **`accounts.go` / TRES GPU regex:** Extended char class from `[:/]` to `[:/=]` to handle the rare `gres/gpu=N` format (equals sign instead of colon).
- **`scheduler.go` — DBD Agent regex:** Tightened `^DBD Agent` to `^DBD Agent queue size` to prevent future sdiag fields from accidentally overwriting the queue size value.

### 📋 Documentation

- **`docs/audit-v1.7.md`:** Full audit report (395 lines) covering all 4 axes — command/format validation against Slurm 25.11, parser quality, missing metrics analysis (sacct efficiency, sstat, sinfo %E/%H), and PromQL review of all 9 dashboards. No breaking issues found. v1.8 backlog defined.
- Dashboard screenshots refreshed on 20-node live cluster with real user activity.

---

## [1.7.0] - 2026-03-30

### ✨ Features

- **Enhanced `fairshare` collector** (from community PR #6 by @franky920920, improved):
  - New per-user metrics: `slurm_user_fairshare{account,user}`, `slurm_user_fairshare_raw_shares`, `slurm_user_fairshare_norm_shares`, `slurm_user_fairshare_raw_usage_cpu_seconds`, `slurm_user_fairshare_norm_usage`
  - New per-account metrics: `slurm_account_fairshare_raw_shares`, `slurm_account_fairshare_norm_shares`, `slurm_account_fairshare_raw_usage_cpu_seconds`, `slurm_account_fairshare_norm_usage`
  - Enables answering "Why is this user's priority low?" directly in Grafana by comparing `norm_usage` vs `norm_shares`
  - `RawUsage` metric renamed to `raw_usage_cpu_seconds` for clarity (CPU-seconds, decay-weighted)

- **`--collector.fairshare.user-metrics` flag** (default `true`): Disable per-user fairshare metrics on clusters with many users to control cardinality. Each user generates 5 additional time series.

- **New `slurm-accounting` Grafana dashboard:** Dedicated HPC accounting dashboard with:
  - User FairShare summary table (FairShare factor, NormShares, NormUsage, Usage/Shares ratio, CPU-seconds)
  - Top consumers by running jobs, CPUs, and accounts
  - Priority ranking: users sorted by FairShare ascending (lowest priority first)
  - Account summary table with historical CPU usage
  - Usage trends: running jobs and CPUs per user and account over time
  - FairShare evolution timeseries per user and account
  - Filterable by `$account` and `$user` variables

- **`slurm-usage` dashboard updated** with two new user-level FairShare panels.

### 🧪 Tests & Quality

- **Coverage: 41% → 57%** — 6 new test files added:
  - `fairshare_test.go`: 15 tests — parser edge cases (empty account, parent skip, indented lines), Execute mock, full Collect/Describe coverage, deduplication guard, error handling, user-metrics flag
  - `users_test.go`: parser + collector tests (previously 0% coverage)
  - `status_test.go`: StatusTracker Add/Collect/Describe/panic-recovery (previously 0%)
  - `accounts_collector_test.go`, `licenses_collector_test.go`, `cpus_collector_test.go`: collector-level tests via Execute mock
  - `test_data/sshare_users.txt`: anonymized `sshare -a` fixture

- **Lint:** 0 issues (gofmt, goimports, golangci-lint v2)

---

## [1.6.0] - 2026-03-22

### ✨ Features

- **Global job metrics always present:** All `slurm_jobs_*` cluster-wide counters (`slurm_jobs_running`, `slurm_jobs_pending`, `slurm_jobs_completing`, `slurm_jobs_failed`, `slurm_jobs_timeout`, `slurm_jobs_cancelled`, `slurm_jobs_preempted`, `slurm_jobs_node_fail`, `slurm_jobs_suspended`, `slurm_jobs_cores_running`, `slurm_jobs_cores_pending`) are now **always emitted** — even when the cluster has zero jobs — so alerting rules never encounter missing time series.

### 🐛 Bug Fixes

- **StatusTracker deadlock on large clusters:** The previous implementation used a 512-slot intermediate channel between the inner collector and the Prometheus registry. On clusters with high metric cardinality (200+ nodes × partitions × metrics > 512), the inner collector blocked waiting for channel capacity while the goroutine draining the channel was waiting for it to finish — a classic deadlock. Fixed by writing directly to the Prometheus channel inside the collector goroutine, eliminating the intermediate buffer entirely.

---

## [1.5.0] - 2026-03-22

### ✨ Features

- **`--slurm.bin-path` flag:** Configure the directory where Slurm binaries (`sinfo`, `squeue`, `sdiag`, `scontrol`, `sshare`, etc.) are looked up. Defaults to empty (system `$PATH`). Required when running in environments where Slurm is not on `$PATH` (e.g. containers with host-mounted binaries, non-standard installations).

  Fatal startup validation: when `--slurm.bin-path` is set, the exporter checks that every required binary exists and is executable at boot. Missing or non-executable binaries are reported individually and the process exits with code 1 — fail fast with a clear message rather than silently returning empty metrics.

  ```bash
  ./slurm_exporter --slurm.bin-path=/opt/slurm/bin
  ```

- **`--collector.queue.user-label` flag** (default `true`): Disable the `user` label on all `slurm_queue_*` and `slurm_cores_*` metrics. When disabled, job counts are aggregated per partition only. On clusters with many users this dramatically reduces cardinality: 1 000 users × 10 partitions × 22 metrics = ~220 000 series → ~220 series.

- **Metrics output examples:** New [`docs/metrics-examples.md`](docs/metrics-examples.md) with representative Prometheus text-format output for all 14 collectors. Includes before/after comparisons for `--collector.nodes.feature-set`, `--collector.queue.user-label`, and `--web.disable-exporter-metrics`.

### ⚙️ Technical Improvements

- **CI upgraded to Node.js 24 actions** (ahead of the June 2, 2026 GitHub enforcement deadline):
  - `actions/checkout` v4 → v6
  - `actions/setup-go` v5 → v6
  - `goreleaser/goreleaser-action` v6 → v7
  - `golangci/golangci-lint-action` v8 → v9

- **`--slurm.bin-path` test coverage:** 5 tests covering custom path execution, missing binary, non-executable binary, and the skip-validation behaviour when path is empty (fake shell scripts in `t.TempDir()`).

- **Queue cardinality test coverage:** `TestPushAggregatedNVal` and `TestPushAggregatedNNVal` verify the aggregation logic for `--no-collector.queue.user-label`.

---

## [1.4.0] - 2026-03-21

### ✨ Features

- **GPU metrics per account and user:** New `slurm_account_gpus_running{account}` and `slurm_user_gpus_running{user}` metrics tracking running GPUs from the TRES field (`%b`) of `squeue`. Correctly multiplies per-node GPU count by the number of allocated nodes for multi-node jobs.
- **Reserved license metric:** New `slurm_license_reserved{license}` metric exposing the `Reserved` field from `scontrol show licenses`. The parser also now handles the complete real-world output format including `Remote`, `LastConsumed`, `LastDeficit`, and `LastUpdate` fields.
- **Reservation nodes collector:** New `reservation_nodes` collector providing per-reservation node state metrics from `scontrol show nodes -o`. Handles compound Slurm states (e.g. `ALLOCATED+MAINTENANCE+RESERVED`) by categorising on the primary state (token before the first `+`). Metrics: `slurm_reservation_nodes_{alloc,idle,mix,down,drain,planned,other,healthy}{reservation}`.
- **`--collector.nodes.feature-set` flag** (default `true`): Disable the `active_feature_set` label on `slurm_nodes_*` metrics to reduce cardinality on homogeneous clusters where feature sets add no monitoring value.
- **`--web.disable-exporter-metrics` flag** (default `false`): Exclude Go runtime and process metrics (`go_goroutines`, `process_cpu_seconds_total`, etc.) from `/metrics`. Useful when a dedicated Go runtime exporter is already scraping the host.

### 🐛 Bug Fixes

- **GPU sinfo column overflow:** `--Format=Nodes: ,GresUsed:` used only 1 character of padding between columns. On clusters with 1000+ node groups (e.g. `1056gpu:...`), the Nodes and GresUsed columns merged into a single unparseable token. Fixed by adding explicit column widths (`Nodes:10`, `Gres:50`, `GresUsed:50`) in `gpus.go` and `partitions.go`.
- **Queue parser truncation:** The squeue format changed from `%P,%T,%C,%r,%u` to `%P|%T|%C|%r|%u` (pipe delimiter). Pending reasons often contain commas (e.g. `Resources,Priority`) which silently truncated the reason field and shifted all subsequent fields.
- **StatusTracker panic on startup:** The previous `WrapWithStatus` approach registered one `StatusCollector` per Slurm collector. All instances tried to register the same `*prometheus.Desc` objects (different pointers, same fqName), causing a panic on boot. Replaced with a single `StatusTracker` that internally runs all collectors and emits health metrics from one canonical descriptor pair.

### ⚙️ Technical Improvements

- **`strings.SplitSeq` modernization:** Replaced `strings.Split` with `strings.SplitSeq` (Go 1.24+) in all parse functions that iterate over lines without needing a sorted or indexed slice (`accounts`, `users`, `fairshare`, `licenses`, `queue`, `reservation_nodes`). Avoids allocating the full intermediate `[]string` slice on every `Collect()` call.
- **Real-world test data:** All new parsers are backed by anonymised real-world `scontrol`/`squeue` output from production clusters (`slurm-25.05` with `nvidia_gb200` GPUs, `scontrol show nodes` with compound states and reservation fields).

---

## [1.3.0] - 2026-03-21

### ✨ Features

- **Custom Prometheus registry:** Replaced the default global registry with `prometheus.NewRegistry()`. Prevents metric pollution from third-party packages, makes the exposed metric set fully explicit, and enables OpenMetrics format.
- **OpenMetrics format:** `promhttp.HandlerFor` with `EnableOpenMetrics: true` — supports exemplars and newer Prometheus features.
- **GoCollector and ProcessCollector:** Go runtime and process metrics are now explicitly registered (`go_goroutines`, `go_gc_duration_seconds`, `process_cpu_seconds_total`, etc.).
- **`/healthz` endpoint:** Liveness probe returning `200 ok` without executing any Slurm commands. Allows Kubernetes and systemd to distinguish process liveness from Slurm reachability.
- **Per-collector health metrics:** `slurm_exporter_collector_success{collector}` (1=success, 0=panic) and `slurm_exporter_collector_duration_seconds{collector}` for independent alerting on each Slurm collector.

### 🐛 Bug Fixes

- **Nil pointer dereference in `ParsePartitionsMetrics` (issue #5):** When a partition appeared in GPU sinfo output but not in the CPU partition map, accessing the nil pointer caused a `SIGSEGV`. Fixed by initialising the partition entry before the GPU accumulation. Regression test `TestParsePartitionsMetricsGPUOnlyPartition` added.
- **`slurm_cores_suspended` never populated:** Copy-paste bug in `ParseQueueMetrics` incremented `qm.suspended` twice instead of populating `qm.c_suspended`. The `slurm_cores_suspended` metric was silently always zero.
- **Bounds checks:** Added `len(splitted) < 4` guard in `ParseCPUsMetrics` and `len(cpuInfo) < 4` guard in `ParseNodeMetrics` to prevent index-out-of-range panics on unexpected `sinfo` output.
- **Scheduler colon truncation:** `strings.Split(line, ":")` in `ParseSchedulerMetrics` truncated values containing colons (e.g. timestamps like `"Wed Apr 12 11:03:21"`). Fixed with `strings.SplitN(line, ":", 2)`.
- **Reservation timezone:** `time.Parse` used UTC silently. Switched to `time.ParseInLocation(slurmTimeLayout, value, time.Local)` so `StartTime`/`EndTime` Unix timestamps reflect the Slurm server's actual local timezone.

### ♻️ Refactoring

- **Data/Parse pattern enforced:** `ParseFairShareMetrics` and `ParseUsersMetrics` previously fetched data inside the parse function, making them untestable in isolation. Both now follow the standard `Data() → Parse() → GetMetrics()` pattern.
- **`ParsePartitionsMetrics` decomposed:** Extracted three focused helpers (`parsePartitionCPUs`, `parsePartitionGPUs`, `parsePartitionJobs`) to reduce cyclomatic complexity from 19 to 6.
- **Regexes pre-compiled:** All `regexp.MustCompile` calls in `accounts`, `users`, `nodes`, `reservations`, and `scheduler` collectors moved to package-level variables to avoid recompilation on every `Collect()` call.
- **camelCase rename (ST1003):** All unexported struct fields and local variables renamed from `snake_case` to `camelCase` throughout the `collector` package.
- **`slices` package:** Replaced `sort.Strings` + `RemoveDuplicates` with `slices.Sort` + `slices.Compact` (Go 1.21+) in `nodes.go` and `node.go`. `RemoveDuplicates` function removed.
- **`appendUnique` modernised:** Replaced manual loop with `slices.Contains`.

### ⚙️ Technical Improvements

- **Go 1.25 / toolchain 1.26.1:** Updated `go.mod` from `go 1.22` to `go 1.25.0` with `toolchain go1.26.1`.
- **All dependencies updated:** `prometheus/client_golang` v1.20.4 → v1.23.2, `prometheus/exporter-toolkit` v0.11.0 → v0.15.1, `prometheus/common` v0.60.0 → v0.67.5, `stretchr/testify` v1.9.0 → v1.11.1, and all transitive dependencies.
- **Slowloris mitigation:** Added `ReadHeaderTimeout: 5s` to `http.Server` (gosec G112).
- **golangci-lint v2 config:** Added `.golangci.yml` with `gosec`, `staticcheck`, `errcheck`, `govet`, `revive`, `gocritic`, `misspell`, `bodyclose`, `whitespace`.
- **CI updated:** Go version 1.22 → 1.25 in both workflows; golangci-lint `v1.59` → `latest` (v2.11.3).
- **Test coverage:** Added assertions to `cpus`, `queue`, `scheduler` tests; added `TestParseCPUsMetricsMalformed` (5 edge cases); added `TestParsePartitionsMetricsGPUOnlyPartition` regression test for issue #5.

---

## [1.2.1] - 2026-03-21

### 🐛 Bug Fixes

- **Nil pointer dereference in `ParsePartitionsMetrics` (issue #5):** Critical crash reproduced on Slurm 24.11.x (SUSE 15.6) and Slurm 25.11 (Ubuntu 24.04). When `sinfo --Format=Gres,GresUsed` returned a partition that was absent from the CPU `sinfo` output, accessing the nil map pointer caused a `SIGSEGV` at `partitions.go:117`. Fixed by initialising the map entry before access.
- **Bounds checks on `sinfo` CPU field:** Added `len(splitLine) < 2` and `len(statesSplit) < 4` guards in `ParsePartitionsMetrics` to handle truncated or malformed `sinfo` output without panicking.
- **Bounds checks on `squeue` fields:** Added `len(fields) < 4` guards in `ParseAccountsMetrics` and `ParseUsersMetrics` to handle incomplete squeue lines.
- **Bounds checks on `sshare` fields:** Added `len(fields) < 2` guard in `ParseFairShareMetrics`.
- **`slurm_cores_suspended` never populated:** Copy-paste bug: the second `qm.suspended.Incr(user, part, cores)` call should have been `qm.c_suspended.Incr(user, part, cores)`. The `slurm_cores_suspended` metric was silently always zero.

### ⚙️ Technical Improvements

- **Regression test:** Added `TestParsePartitionsMetricsGPUOnlyPartition` to prevent regressions of issue #5.
- **Merge of `fix/issue-5-crash-suse` branch:** The fix branch that was validated by users but never merged into `master` has been properly integrated.

---

## [1.2.0] - 2025-12-29

### ✨ Features

- **Licenses Collector:** Added a new collector to monitor license usage (`slurm_license_total`, `slurm_license_used`, `slurm_license_free`) via `scontrol show licenses`.
- **Enhanced Partition Metrics:** Added new metrics to the `partitions` collector:
  - `slurm_partition_jobs_running`: Number of running jobs per partition.
  - `slurm_partition_gpus_idle`: Number of idle GPUs per partition.
  - `slurm_partition_gpus_allocated`: Number of allocated GPUs per partition.

## [1.1.0] - 2025-08-07

This release focuses on major architectural improvements and modernization of the codebase. The project structure has been reorganized to follow Go best practices, and the logging system has been migrated from go-kit/log to the standard log/slog package for better performance and structured logging.

### 🏗️ Major Changes

- **Project Restructuring:** Moved main.go to `cmd/slurm_exporter/` directory following Go standards
- **Logging Migration:** Migrated from go-kit/log to log/slog for better performance and structured logging
- **Code Organization:** Reorganized code with `internal/logger/` and `internal/collector/` packages
- **Structured Logging:** Implemented structured logging system across all collectors

### 🔧 Improvements

- **Markdown Formatting:** Fixed markdown formatting issues in README.md (MD030/list-marker-space)
- **Code Formatting:** Improved code formatting and logger consistency
- **Default Settings:** Changed default log format from json to text for better readability
- **Project Visibility:** Added status badges to README for GitHub Actions, releases, and code quality
- **GoReleaser Configuration:** Fixed GoReleaser configuration for new project structure
- **Changelog Configuration:** Added explicit changelog configuration to GoReleaser

### 🐛 Bug Fixes

- **Test File Paths:** Fixed test file paths in all test files (corrected relative paths)
- **Build Configuration:** Fixed "build does not contain a main function" error in GoReleaser workflow
- **Tag Management:** Removed problematic `master` tag that was causing changelog generation issues

### ⚙️ Technical Improvements

- **Better Code Alignment:** Improved code alignment and organization throughout the project
- **Test Reliability:** All tests now pass successfully with correct file references
- **Build Process:** Ensured proper binary building after project restructuring

---

## [1.0.0] - 2025-07-21

This release marks a major milestone, signifying a stable and feature-rich version of the Slurm Exporter. It includes a complete overhaul of the CI/CD pipeline, numerous new collectors, significant refactoring for better maintainability, and several important bug fixes.

### ✨ Features

- **New Collectors:**
  - `reservations`: Collects metrics about Slurm reservations.
  - `fairshare`: Gathers fairshare usage metrics.
  - `users`: Provides metrics on a per-user basis.
  - `accounts`: Adds metrics for Slurm accounts.
  - `slurm_info`: Exposes general information about the Slurm version.
  - `node`: Provides detailed per-node metrics including CPU and memory usage.
- **Collector Configuration:** Collectors can now be individually enabled or disabled via command-line flags (e.g., `--collector.reservations=false`).
- **Improved GPU Metrics:** GPU data collection is more robust and supports modern Slurm versions (`>=19.05`).
- **CPU Metrics:** Added metrics for pending CPUs per user and per account.
- **Enhanced Build Info:** Version details (commit, branch, build date) are now injected into the binary at build time.

### 🐛 Bug Fixes

- **GPU Parsing:** Fixed a regex issue for parsing GPU information when no specific GPU type is used.
- **Node Name Parsing:** Corrected an issue where long node names were truncated.
- **CI/CD:** Resolved multiple issues in the GoReleaser and GitHub Actions configurations to ensure reliable builds and releases.

### ♻️ Refactoring

- **Code Structure:** All collectors have been moved into a dedicated `collector` package for better organization.
- **Command Execution:** Centralized the execution of Slurm commands within the collectors, adding a configurable timeout for better resilience.
- **License Headers:** Consolidated and standardized license headers across the codebase.

### ⚙️ CI/CD

- **Major Overhaul:** The entire release process has been modernized. It now uses the latest versions of `goreleaser` and `golangci-lint`, and the GitHub Actions workflows have been simplified and made more reliable.

- **Snapshot Builds:** The CI/CD pipeline can now produce development "snapshot" builds for testing purposes.
- **Packaging:** Removed unsupported packaging formats (RPM, Snap) to focus on robust binary releases.

---

## [0.30]

### ✨ Features

- **New Metrics:**
  - `slurm_node_status`: Added a new metric to expose the status of each node individually.
  - `slurm_binary_info`: Added metrics exposing the version of the Slurm binaries.
- **Go Version:** Updated the project to use Go 1.20.

### ♻️ Refactoring

- Replaced the deprecated `io/ioutil` package with `io`.

### ⚙️ CI/CD

- Added a dedicated GitHub Actions workflow for releases.
- Updated Go version used in CI to 1.20.

---

## [0.21]

### ✨ Features

- **TLS & Basic Auth:** Added support for TLS and Basic Authentication via the Prometheus Exporter Toolkit.
- **GPU Metrics:** Updated GPU collection logic to be compatible with Slurm versions `19.05.0rc1` and newer by using the `GresUsed` format option.

### ⚙️ Build

- **CGO Disabled:** Builds are now produced with `CGO_ENABLED=0` for better portability.
- **Dependencies:** Updated Go module dependencies.
