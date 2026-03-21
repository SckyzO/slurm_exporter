# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
