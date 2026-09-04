# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

Targeting **2.0.0**.

A dozen metrics in the 1.8 line published a number that was wrong rather than
absent. `slurm_jobs_failed` said no job had ever failed on a cluster that had
just failed nineteen. `slurm_reservation_start_time_seconds` published January
of year 1 as a date. `slurm_exporter_collector_success` stayed at 1 through the
outage it exists to signal. A node the controller had downed for silence
produced no drain series at all. None of those look like a fault from the
outside, which is why they lasted as long as they did.

This release fixes them. Three of the fixes change what an existing series
does, which is what makes it a major, and the major in turn moves the Go module
path to `/v2`. The section below says exactly what moves and how to keep the old
behaviour where one exists.

The rest is the machinery that stops the same class of bug coming back: the
commands the exporter runs are now declared in code and verified against the
collectors by CI, the fixtures behind them are captured by a generated script
rather than by hand, and each release is validated against both ends of the
supported Slurm window.

### ⚠️ Breaking changes

- **The Go module path is now `github.com/sckyzo/slurm_exporter/v2` (#238):**
  Go requires the major version in the import path from v2 onward, and this
  module has a `go.mod` at every tag, so the `+incompatible` escape hatch that
  lets some older projects tag a v2 without it does not apply here.

  **Operator-visible impact:** the suffix is not optional and its absence is
  silent. `go install github.com/sckyzo/slurm_exporter/cmd/slurm_exporter@latest`
  keeps resolving to **v1.8.5** — the module proxy already serves this path and
  will not offer a v2 tag under it, so the command succeeds and installs the
  previous major. Install 2.0 with the suffix:

  ```
  go install github.com/sckyzo/slurm_exporter/v2/cmd/slurm_exporter@latest
  ```

  Anyone importing the packages updates their import lines the same way. Nobody
  installing a binary from the release archives, a Docker image, or a distro
  package is affected: the artefact names and the image tags do not carry the
  suffix.

- **`slurm_jobs_*` terminal counters stop being constant zeros (#161):**
  `squeue` reports pending and running jobs when it is not told which states
  to view, so nine of the eleven states the queue parser understands never
  reached it. `slurm_jobs_failed`, `_timeout`, `_cancelled`, `_preempted`,
  `_node_fail` and `_completed` held a flat zero, and the "Terminal Job States
  Over Time" panel drew a line along the bottom of the chart. `QueueData` now
  passes `--states=all`.

  **Operator-visible impact:** those six counters, and their per-user and
  per-partition twins in `slurm_queue_*` and `slurm_cores_*` that existed for
  no label combination at all until now, start carrying values. The window is
  a sliding one governed by `MinJobAge` in `slurm.conf` (300 s by default), so
  a burst of failures appears in full and falls back to zero once slurmctld
  forgets the jobs. An alert written as `slurm_jobs_failed > 0` against the old
  behaviour will fire continuously and has to move to a rate or a ratio. No
  shipped rule is affected: `SlurmJobFailureRateHigh` reads
  `cluster:slurm_job_failure_rate:ratio15m`, which `rules.yml` computes from
  the sdiag counters. Pass `--no-collector.queue.terminal-states` to restore
  the previous query.

- **`slurm_node_drain_reason_info` loses its `since` label (#157):** the label
  carried the drain timestamp, so draining the same node twice produced two
  label sets. Prometheus saw a new series and orphaned the old one, so the
  series count grew with how often operators drain nodes over the life of the
  TSDB rather than with how many nodes are drained. A site that drains
  routinely for maintenance paid for it continuously.

  **Operator-visible impact:** the timestamp moves to its own gauge, keyed by
  node:

  ```
  slurm_node_drain_reason_info{node="c1",reason="disk failure"} 1
  slurm_node_drain_since_timestamp_seconds{node="c1"} 1.7847e+09
  ```

  Queries that select or display `since` must read the new gauge and join on
  `node`. The metric name, its value of 1, and its `node` and `reason` labels
  are unchanged, so queries that ignored `since` keep working.

- **Nodes downed for not responding now produce a drain series (#198):**
  `slurm_node_drain_reason_info` dropped any row whose reason was
  `not responding`, alongside the genuinely empty `none` and `unknown`. But
  slurmctld writes that reason itself when it loses contact with a node, so a
  node the controller had downed produced no series at all. The metric stayed
  silent about exactly the outage it exists to describe. v1.8 excluded the
  reason on the grounds that Slurm sets it rather than an admin; the right test
  is whether the reason carries information, and this one is the only place the
  difference between an unreachable node and an admin-drained one shows up.

  **Operator-visible impact:** those nodes start producing a series where they
  produced none, and anything counting the metric will see them. Empty reasons
  stay filtered, so cardinality is still bounded by the node count.

### ✨ Features

- **Per-node GRES totals and usage (#29):** every GPU metric in the exporter
  was an aggregate. `slurm_gpus_*` covers the whole cluster with no labels at
  all, `slurm_partition_gpus_*` stops at the partition, and nothing anywhere
  carried a model. "Which of my forty nodes has a free GPU" and "are the H100s
  saturated while the A100s idle" had no answer, on the one class of cluster
  where both are the daily question. `slurm_node_gres_total` and
  `slurm_node_gres_used` report what Slurm configures and allocates per node,
  keyed by a `gres_type` label that is `gpu` for an untyped resource and
  `gpu:A100` when a model is set. Non-GPU resources such as `shard` come
  through the same path. It reads columns of the `sinfo` call the node
  collector already makes, so no extra RPC reaches slurmctld. Behind
  `--collector.node.gres`, default on; a node with no GRES publishes nothing,
  so a CPU-only cluster pays nothing for the default. Contributed by
  @ncreddine.

- **The code is the single source of truth for Slurm commands (#195):** the
  list of commands the exporter runs lived in three places with nothing
  checking them against each other: the `*Data()` functions, the prose in
  `test_data/readme.md`, and the memory of whoever last captured a fixture.
  They drifted: the readme documented a `sinfo` format `node.go` had already
  abandoned, pointed at a fixture deleted two releases earlier, and omitted the
  `--endtime` that `sacct` requires. A contributor followed it and had to redo
  the work. `CommandRegistry` declares each invocation once, and a contract test
  invokes every entry through its real `*Data()` path behind a stubbed
  `Execute`, so the table is a verified mirror rather than a fourth copy.
  Changing a command without updating the entry fails CI, as does a declared
  fixture that is not on disk, or a file under `test_data/` that no entry
  claims. That last check is what six live regression fixtures were missing.

- **`test_data/readme.md` and `scripts/capture.sh` are generated from the
  registry (#195):** capturing a fixture by hand meant knowing which commands
  to run, remembering to anonymise, and getting the anonymisation right, which
  is three chances to produce a capture that looks fine and encodes something
  wrong. The capture script now runs exactly the commands the exporter runs and
  cannot drift from them; every one is read-only, because no write command
  exists in the registry to generate. Both artefacts are checked for staleness
  in CI.

- **Format-drift reporting across the support window (#195):**
  `tools/fixture-diff` compares two versioned fixture directories by shape
  rather than by value, so a cluster having more nodes than another is not
  reported as a format change while a new column or a changed separator is.
  Running it across the window found one real drift: `SuspendTime` is new in
  `scontrol show nodes -o` on 26.05.

- **The test cluster builds unpublished Slurm majors from source (#189):** the
  release gate asks for validation against both ends of the supported window,
  and upstream does not tag every major the window covers. The harness now
  resolves its image cheapest-first (reuse local, pull published tag, build
  from source), so both ends can be exercised locally. Validated live on 26.05.2
  and 25.05.8: 9/9 setup, 15/15 collectors at `success=1`, correct
  `slurm_info` version, workload metrics populated, no ERROR or WARN.

- **Fake GPU workers for the test cluster:** dynamic `slurmd -Z` registration
  with fabricated device files lets the CPU-only test cluster advertise two
  distinct GPU models, which is what the per-node GRES work needed to be tested
  against something other than a hand-written fixture. `make gpu-workers`,
  sized by `GPU_N`.

### 🐛 Bug Fixes

- **`slurm_exporter_collector_success` reported 1 during an outage (#138):**
  `StatusTracker` only lowered the gauge inside its `recover()` block, and no
  collector panics when a Slurm command fails: every one logs and returns. So
  with slurmctld unreachable, or `sinfo` exceeding `--command.timeout`, the
  collector's series vanished from `/metrics` while the health gauge kept
  saying everything was fine.

- **Jobs submitted to several partitions were counted nowhere (#153) or under
  a partition that does not exist (#175):** `sbatch -p debug,high` is reported
  by `squeue` as `debug,high`. The partitions collector looked that raw string
  up against normalised map keys and matched nothing, dropping the job; the
  queue collector passed it through as a label, so
  `sum by(partition) (slurm_queue_pending)` gained a row for a partition no
  cluster has while undercounting the two real ones. Both now split the field
  and count the job in each partition it is queued in.

- **Reservation timestamps published year 1 (#159):** the `scontrol` parse
  error was discarded, leaving the zero `time.Time`, and the collector
  published it unconditionally as `-62135596800`. Nothing about that value
  looks like an error: no threshold rejects it, no subtraction fails on it, and
  nothing was logged. It is now omitted, which makes the metric absent rather
  than wrong.

- **`SLURM_TIME_FORMAT` in the exporter's environment broke every timestamp
  (#160):** Slurm renders dates according to that variable and the parsers read
  one layout. `Execute` never set `cmd.Env`, so a value inherited from the
  service environment reached `sinfo`, `squeue`, `scontrol`, `sdiag`, `sshare`
  and `sacct` alike. It is now pinned to `standard` on every call. This is the
  root cause the collector-side fixes in #153 and #159 were treating.

- **Drained nodes inside a reservation were counted healthy (#178):**
  `scontrol` emits compound states as `BASE+FLAG`, and the parser only read the
  segment before the first `+`, so `slurm_reservation_nodes_drain` stayed at 0
  no matter how many nodes in a maintenance reservation were drained. `DRAIN`
  and `PLANNED` are now counted orthogonally on top of the base bucket.

- **Memory efficiency was a constant 0 (#180):** four independent defects in
  `sacct_efficiency`, among them jobs with no recorded `MaxRSS` entering the
  average at 0% instead of being left out, and `T`/`P` unit suffixes going
  unhandled with the `strconv` error discarded, so `1.50T` became 0.

- **SIGTERM and SIGINT were ignored (#179):** the signal context added for #18
  was wired only into the `sacct_efficiency` collector. `web.ListenAndServe`
  blocked for the life of the process, nothing called `server.Shutdown()`, and
  the graceful-shutdown block sat after the listen call where it could never be
  reached. The listener now runs in a goroutine while `runServer` selects over
  its result and `ctx.Done()`.

- **Every path that was not a route answered 200 (#233):** the landing page was
  registered with `http.HandleFunc("/")`, and `/` in an `http.ServeMux` claims
  everything no other pattern does. A Prometheus server pointed at `/metric`,
  or at any typo, was answered 200 with an HTML page: the target scraped as
  `up`, and the mistake surfaced hours later as a parse error in Prometheus
  rather than immediately as a 404 from the exporter. The page now uses
  `exporter-toolkit`'s `web.NewLandingPage`, which also sets the `Content-Type`
  the hand-written page never sent and links `/healthz` alongside `/metrics`.
  Profiling links are disabled, since the exporter does not import
  `net/http/pprof` and advertising them would point at a 404.

- **Release image build (#109):** the refresh binary is now staged at both the
  `dockers_v2` `$TARGETPLATFORM` path and the older flat one, so either
  `Dockerfile` COPY layout finds it.

- **`make` invoked the host `go` on every target (#134),** which broke the
  containerised "no host toolchain needed" contract the Makefile advertises.

### ⚡ Performance

- **One `squeue` snapshot shared across three collectors (#144):** the
  accounts, users and partitions collectors each dumped the full job queue from
  slurmctld on every scrape, up to five separate calls run serially. They now
  read one per-scrape cached snapshot and project the exact column layout each
  parser already consumes, so no metric value changes.

- **Binary versions resolved once instead of on every scrape (#149):** Slurm
  binary versions cannot change while the process runs, yet the info collector
  re-forked `<binary> --version` every time: six required forks plus up to
  three optional, with `sinfo` forked twice. Now probed once behind a
  `sync.Once`.

- **All GPU metrics derived from one `sinfo` snapshot (#181):** three
  per-scrape calls (total, allocated, idle) replaced by a single consolidated
  query, projected into the three layouts the separate calls returned so the
  per-version fixture matrix keeps protecting GRES parsing unchanged.

- **GRES parsing unified into one helper and one regexp (#150):** the
  cluster-wide and per-node views can no longer disagree about what a GRES
  string means. Two bugs the shared parser had to get right, both found against
  real captures rather than reasoned about: splitting on every comma breaks
  `gpu:NVlink_A100_40GB:2(IDX:0,3)` into fragments that parse as nothing and
  silently drop that node's GPUs, so the split now tracks parenthesis depth;
  and an unanchored token pattern latched onto `shard:h10`, the tail of
  `shard:h100:2` cut off by a fixed-width column, and published a phantom
  model with a count of 0. The `(null)` path no longer allocates.

### 🧪 Tests & Quality

- **The metric surface is now a generated golden, checked in CI (#236):** the
  release checklist asked for a scrape diffed against `docs/metrics.md`, a
  comparison that cannot see a metric the cluster had no reason to emit — and
  it never did. Eight `slurm_queue_*` and eight `slurm_cores_*` terminal-state
  metrics sat behind a single `...` row in the document, invisible to a reader
  searching for `slurm_queue_failed` and invisible to the diff, because a test
  cluster with no failed jobs publishes none of them. `docs/metric-surface.md`
  is generated from every collector's `Describe()` output — 209 declarations
  across 24 builds, including both settings of each cardinality flag — so a
  deleted metric and a metric with no data are no longer the same observation.
  A companion test compares `docs/metrics.md` against that surface in both
  directions, so a metric can no longer be documented without existing, or
  exist without being documented.
- **Fixtures renamed after the registry entry they back (#195)** and the
  per-version discovery made honest (#177), so a directory that exists but
  covers nothing can no longer read as coverage.
- Real `squeue` (#174) and multi-model GPU captures from the test cluster, a
  per-version GPU matrix asserting exact counts (#176), and the hand-written
  `State=DRAIN+RESERVED` fixture (a shape Slurm never emits) replaced with a
  real one.
- **The `unused` linter was never enabled (#151):** golangci-lint v2 merged
  gosimple and stylecheck into staticcheck, but not `unused`, and with
  `default: none` it had no gate at all. The `deadcode` target in `make check`
  reports unreachable functions, not unreferenced package-level variables or
  write-only struct fields, so that whole class went unchecked.
- Nine fixes to the integration harness in `scripts/testing/`, each of which
  had been quietly making a Definition-of-Done step unprovable: exporter output
  not captured so the log check read nothing (#163), unbounded health checks
  that hung on a mute port (#165), generated jobs exceeding the partition
  `MaxTime` (#169) or the smallest node's memory (#173), pending jobs surviving
  cleanup (#170), job output blocking job start (#172), a stale provisioned
  dashboard masking the real ones (#156), a floating MariaDB tag aborting
  slurmdbd (#187), and orphan recipe fragments in the Makefile (#166).

### 📋 Documentation

- **Non-regression is a release step, not an intention (#235):** the process
  asked for it in prose and never said what would satisfy it, so it was
  discharged by whatever had been run. It is now step 4 of the Definition of
  Done and its own section in the release process, with a table mapping each
  kind of change to the net that catches it, and a ranked list of acceptable
  measurements for the changes no test can express — a documented version
  number among them.
- **The 1.8 support window is stated in `SECURITY.md` (#215):** 1.8 is the last
  release of the v1 line and lives on `release-1.8`. It receives every security
  fix until 2.0.0 ships, then critical severity only, for six months from the
  2.0.0 release date. The backport route onto that branch is documented in the
  release process.
- **P0 to P3 issue triage scale documented (#152)** and a registry entry now
  required for every new collector (#195).
- **The release Slurm-version window follows Slurm's own policy (#189):**
  newest major plus the previous three, re-derived at each release, with each
  release validated on both ends of it.
- **What the two `sacct` flags cost SlurmDBD:** `sacct` never runs on the
  scrape path, so scrape frequency is irrelevant to the load. What governs it
  is the ratio of `--collector.sacct.lookback` to `--collector.sacct.interval`.
  Each refresh queries the whole window, so at the defaults every job is read
  about twelve times before ageing out.
- Four comments contradicted by the code corrected (#135), restatement noise
  dropped (#136), and the "not maintained for 25.11+" stance removed: this
  project is maintained, and the native OpenMetrics endpoint in Slurm 25.11+
  exposes far fewer metrics than it does.

### 🔧 Maintenance

- Go toolchain 1.26.4 → 1.26.5 (#124); `govulncheck` pinned to v1.6.0 (#125).
- `golang.org/x/text` 0.37.0 → 0.39.0 for CVE-2026-56852, which was gating
  every image-building PR; `golang.org/x/crypto` 0.51.0 → 0.53.0, starting with
  #110.
- `prometheus/common` 0.68.1 → 0.70.0, `prometheus/exporter-toolkit` 0.16.0 →
  0.17.1, plus the transitive set.
- **CI runs on release branches (#237):** the workflow triggered on `master`
  only, so `release-1.8` — the branch the support window commits to
  backporting onto — had no lint or test gate at all. The same PR stopped a
  Go Report Card badge refresh from failing a tag: the POST is a cosmetic
  side-effect and now tolerates a non-2xx.
- Five unused compatibility shims removed from the logger (#137); the Makefile
  Go-version fallback derived from `go.mod` (#114).
- The usual run of pinned action and base-image digest bumps.

## [1.8.5] - 2026-09-04

A security-only release. No collector, parser, metric, label or dashboard
changes: the exporter behaves exactly as v1.8.4 did. The binary is what moves.

### 🛡️ Security

- **Go toolchain 1.26.4 → 1.26.8:** `govulncheck` run against v1.8.4 as shipped
  reported five standard-library vulnerabilities reachable by symbol, three of
  them on the listener the exporter exists to run:

  | Advisory | Package | Fixed in |
  |---|---|---|
  | GO-2026-6091 | `html/template` | 1.26.6 |
  | GO-2026-6090 | `crypto/tls` | 1.26.6 |
  | GO-2026-6089 | `net/http` | 1.26.6 |
  | GO-2026-5972 | `encoding/asn1` | 1.26.6 |
  | GO-2026-5856 | `crypto/tls` | 1.26.5 |

  Trivy independently reported 8 HIGH against `stdlib v1.26.4`. Both gates are
  clear on 1.26.8, which is the current patch of the line v1.8 already used, so
  nothing changes but the standard library.

- **`golang.org/x/crypto` v0.51.0 → v0.56.0** for CVE-2026-56854, which Trivy
  rates CRITICAL. It is not reachable from this code — the module is pinned
  because `exporter-toolkit`'s basic-auth password hashing links
  `bcrypt`/`blowfish` — but Trivy gates image publishing at module-version
  granularity, so the dependency moves anyway. `go mod tidy` carried
  `x/sys` 0.45.0 → 0.47.0 and `x/text` 0.37.0 → 0.41.0 with it.

### 📋 Documentation

- **`SECURITY.md` now states the 1.8 support window.** It previously said the
  project was not actively maintained for Slurm 25.11+ and pointed readers at a
  different repository. That stance has been reversed: `slurm_exporter` is
  maintained. 1.8 is the last of the v1 line and keeps receiving every security
  fix until 2.0.0 ships, then critical severity only, for six months from the
  2.0.0 release date.


### ✅ Validation

Validated on the Slurm 25.11.2 integration cluster with the v1.8.5 binary
deployed: 16/16 collectors at `success=1`, 507 series under workload, zero
`ERROR` or `WARN` across the run, and every documented metric either exposed or
absent for a reason confirmed on the cluster (no licenses, no reservations, no
drained nodes, no GPUs, no suspended jobs).

Because three of the five advisories are on the TLS and HTTP paths, which the
standard checklist never exercised, this release was additionally validated
with `--web.config.file`: TLS handshake with HTTP/2, a scrape returning 200
over HTTPS, bcrypt basic auth accepting the right password and answering 401 to
a wrong one and to none, and a plaintext request to the TLS port rejected.

## [1.8.4] - 2026-06-19

A supply-chain and CI-hardening release. One operator-visible label fix
(#69); no breaking changes. The bulk is defensive tooling, SHA/digest
pinning, and a documentation overhaul.

### 🐛 Bug Fixes

- **`node` collector — default-partition `*` stripped from `slurm_node_*`
  labels (#69):** `sinfo`'s `%P` field marks the default partition with a
  trailing `*`, so `slurm_node_*` series for default-partition nodes were
  labelled `partition="gpu*"` instead of `partition="gpu"` — splitting the
  series and breaking label joins with every other collector. `node.go` now
  applies the same `TrimRight(…, "*")` that `nodes.go` already did, with a
  regression test and fixture.

  **Operator-visible impact:** `slurm_node_*` series for default-partition
  nodes change label value (`partition="<name>*"` → `"<name>"`). Update any
  alert or dashboard selector that matched the trailing asterisk.

### 🛡️ Defensive hardening

- **Go toolchain bumped to 1.26.4 (#71):** picks up the stdlib fixes for
  GO-2026-5039 and GO-2026-5037.
- **OpenSSF Scorecard workflow (#83)** and **govulncheck call-graph scan
  (#75)** added to CI, surfacing supply-chain and reachable-vulnerability
  findings on every push.
- **Third-party GitHub Actions pinned by commit SHA (#77)** and **Docker base
  images pinned by digest (#92)** — closes Scorecard `PinnedDependencies`.
- **Workflow write permissions scoped to the job (#90)** — closes Scorecard
  `TokenPermissions`.
- **Release signing migrated to cosign v3 / cosign-installer v4 (#104).**
- **`SECURITY.md` added and linked from the README (#64),** with a summary of
  security practices (#88).
- Toolchain gains `gitleaks` (secret scan) and `osv-scanner` (#85), and
  `make check` is gated on `zizmor` (Actions static analysis) (#80).

### ✨ Features

- **Lint + test on every PR** as the Definition-of-Done gate (#94).
- **`CODEOWNERS`** added (#78).
- **Scheduled image maintenance:** weekly rebuild of the last two stable image
  lines (#57) and a monthly cron pruning dated immutable tags (#58).
- **Docker Hub description synced** from `docker/README.md` (#51).

### ⚙️ CI/CD

- Migrated the GoReleaser Docker config to the `dockers_v2` layout (#50); copy
  the binary from `$TARGETPLATFORM` for that layout (#107); pinned GoReleaser
  to 2.15.4 ahead of the 2.16 layout change (#106).
- Built `refresh` images per-arch so arm64 ships an arm64 binary (#63); skip
  refresh tags without a single-stage Dockerfile (#62).
- Dropped a redundant tag fetch from `dev-release` (#65); defined dev-release
  build vars and removed the legacy `COSIGN_EXPERIMENTAL` (#87).
- Added `deadcode` (`make deadcode`) (#98) and
  `govulncheck`/`actionlint`/`shellcheck`/`zizmor` to the toolchain plus a
  `make race` fix (#73). Bumped `docker/setup-{qemu,buildx}-action` to v4 (#52).

### ♻️ Refactoring

- Removed the unreachable single-partition path in the `nodes` collector
  (#100).

### 🧪 Tests & Quality

- Removed orphaned legacy GPU fixtures (Slurm 17.11.2) (#102); fixed a broken
  leftover shell fragment in `make stop` (#96).

### 📋 Documentation

- Restructured the main README to surface full project capability (#54), a
  Docker Hub best-practices README (#53), folded Quick Start into Get Started
  (#55), and enriched the PR template with a Make-targets section (#56).
- Aligned `metrics.md` scheduler RPC label and fairshare descriptions with the
  collector Help strings, aligned README/CONTRIBUTING collector and dashboard
  counts, and bumped the documented Go toolchain to 1.26.4 (#105).

### 🔧 Maintenance

- Dependency bumps: `github.com/prometheus/common` (#67, #68),
  `docker/build-push-action` 6 → 7 (#60), `peter-evans/dockerhub-description`
  4 → 5 (#59).

## [1.8.3] - 2026-05-16

### 🐛 Bug Fixes

- **`accounts` and `users` collectors — `slurm_account_gpus_running` and
  `slurm_user_gpus_running` silently drop jobs submitted with `--gpus` or
  `--gpus-per-node` (issue #35, reported by @ncreddine):**
  Both collectors parsed the GRES count from `squeue -o "%b"`
  (`tres-per-node`). That field reflects only resources requested via
  `--gres` and returns `N/A` for the newer `--gpus[-per-node]` flags,
  causing `parseGPUsFromTRES("N/A")` to fall to 0. Accounts and users
  whose entire GPU footprint came through `--gpus[-per-node]` were
  invisible on those two metrics while `slurm_account_cpus_running` and
  `slurm_account_jobs_running` reported correctly.

  Switched both collectors to `squeue -O ...,tres-alloc`, which is the
  effective allocation total across all submission flags (documented
  identically on Slurm 20.11 → 25.05). The per-node multiplication
  (`gpus_per_node * num_nodes`) was removed since `tres-alloc` already
  reports the total. Regression coverage in `accounts_test.go` and
  `users_test.go` includes `--gpus`, `--gpus-per-node`, `--gres=gpu:N`,
  and typed-GPU variants (`gres/gpu:a100=N`).

  **Operator-visible impact:** on clusters where users submit GPU jobs
  with `--gpus[-per-node]`, `slurm_account_gpus_running` and
  `slurm_user_gpus_running` will step up at the next scrape after upgrade.
  Values for jobs using `--gres` are unchanged. Update any saturation
  alerts that were calibrated against the previously-undercounted
  values.

### ✨ Features

- **Docker image — standard and minimal variants (PR #43, #48):**
  First-class container support. Two flavors published to
  `ghcr.io/sckyzo/slurm_exporter` and `docker.io/sckyzo/slurm-exporter`,
  both as multi-arch manifests (linux/amd64 + linux/arm64):

  | Variant | Base | Size | Use when |
  | --- | --- | --- | --- |
  | Standard (`:1.8.3`, `:latest`) | Ubuntu 26.04 + slurm-client 25.11 | ~160 MB | Cluster runs Slurm 23.x — 26.x packaged from a distro |
  | Minimal (`:1.8.3-minimal`, `:latest-minimal`) | distroless/cc-debian12 + libmunge only | ~36 MB | Cluster runs Slurm built from source / OHPC; bring your own slurm-client via `--slurm.bin-path` |

  Tag matrix per release: `:vX.Y.Z`, `:X.Y`, `:X`, `:latest` (and their
  `-minimal` counterparts). Pre-release tags push only the pinned
  version and don't touch the floating pointers. End-to-end smoke-tested
  against Slurm 25.11.2.

  Compose examples and full deployment scenarios documented in
  `docker/README.md`. Local iteration via `make docker-build`,
  `make docker-run`, `make docker-build-minimal`, etc.

- **Dependabot + `make report-deps` (PR #44):**
  Weekly automated dependency PRs across four ecosystems (gomod,
  github-actions, two Dockerfiles). Related deps grouped:
  `golang.org/x/*`, `github.com/prometheus/*`, `github.com/stretchr/*`
  each bundle into one PR.

  Complementary `make report-deps` Makefile target prints a tabular
  snapshot of every Go module — direct deps with their current version,
  indirect deps with available bumps, color-coded patch / minor / major
  classification. Runs in the containerized toolchain, no host Go
  required.

### 🔧 Improvements

- **Go Report Card score: 100% across every check (PRs #38–#41):**
  Refactored the five functions that were above the `gocyclo` threshold
  of 15 — `parseReservations` (16 → 5), `TestParsePartitionsMetricsWithRealOutput`
  (18 → 6), `ParseNodesMetrics` and `ParseNodesMetricsGlobal` (17/18 →
  4/5 via shared helpers), `ParseSchedulerMetrics` (21 → 8 by splitting
  the field switch into four domain helpers). No behavior change — same
  state regexes, same default buckets, same field semantics. `make
  report` now scores 100% on every check (gofmt, go vet, gocyclo,
  ineffassign, misspell, license).

- **Go toolchain aligned on 1.26 (PR #42):**
  `go.mod` minimum bumped from 1.25.0 to 1.26.0 (toolchain already
  pinned at go1.26.1). CI workflows (`release.yml`, `dev-release.yml`)
  align on Go 1.26. The `make check`/`make report` tools image was
  already on `golang:1.26-alpine`.

  Same PR refreshed six indirect deps to their latest patch/minor:
  `golang.org/x/crypto`, `x/net`, `x/sys`, `x/text`,
  `github.com/mdlayher/socket`, `github.com/klauspost/compress`.

- **Standard image: Slurm 25.11 + Ubuntu 26.04 base (PR #46):**
  Bumped the standard variant base from Ubuntu 24.04 (Slurm 23.11) to
  Ubuntu 26.04 (Slurm 25.11.2) and glibc 2.39 → 2.43. The slurmctld
  compatibility window extends from 22-25 to 23-26. End-to-end smoke
  test against the local Slurm 25.11.2 cluster: 417 `slurm_*` series
  emitted (vs 184 baseline-only previously), zero RPC errors —
  resolves the version-mismatch caveat from the initial Docker work.

- **Minimal image: Debian 13 builder + libmunge2 0.5.16 (PR #45):**
  Bumped the libmunge extractor stage to Debian 13 slim. Distroless
  runtime is unchanged, image size stays at 36.5 MB.

- **Scripts directory regrouped by purpose (PR #47):**
  Moved dashboard tooling (Python helpers + screenshot grabber) under
  `scripts/dashboards/` with its own README, and `random_jobs.sh`
  under `scripts/testing/` (its only consumer). Top-level
  `scripts/dashboards_add_*.py` and `scripts/take_screenshots.sh`
  paths are gone. References updated in the testing Makefile and the
  monitoring docs. New `scripts/README.md` describes the
  three-folder layout (`dashboards/`, `docker/tools/`, `testing/`).

- **`Common Pitfalls` section in `CONTRIBUTING.md` (PR #36):**
  Documents the `squeue -O field:` / `sinfo --Format` truncation gotcha
  (20-char default width that actually truncates the trailing field) —
  same class of bug as issue #10 on sinfo and #35 on squeue. Includes a
  worked example showing the wrong vs right form. Saves the next
  contributor half a debugging session.

## [1.8.2] - 2026-05-11

### ⚠️ Breaking Changes

- **`scheduler` collector — `slurm_scheduler_jobs_*_total` renamed and
  retyped (issue #22, PR #23 by @UeliDeSchwert):**
  The five sdiag-derived counters introduced in v1.8.0 were declared as
  `prometheus.CounterValue` but `sdiag` resets these values to zero on
  every `slurmctld` restart or `scontrol reconfigure`. A Counter that
  decreases violates the Prometheus data model and breaks `rate()` /
  `increase()` at the reset boundary. The `_total` suffix is also
  reserved for Counters by Prometheus naming conventions.

  | Old (Counter) | New (Gauge) |
  | --- | --- |
  | `slurm_scheduler_jobs_submitted_total` | `slurm_scheduler_jobs_submitted` |
  | `slurm_scheduler_jobs_started_total`   | `slurm_scheduler_jobs_started` |
  | `slurm_scheduler_jobs_completed_total` | `slurm_scheduler_jobs_completed` |
  | `slurm_scheduler_jobs_canceled_total`  | `slurm_scheduler_jobs_canceled` |
  | `slurm_scheduler_jobs_failed_total`    | `slurm_scheduler_jobs_failed` |

  **Migration:**
  - Replace the metric names in any external dashboards / recording rules /
    alerts.
  - Drop any `rate()` or `increase()` wrappers — these were already
    producing incorrect results across slurmctld restarts. Use the raw
    Gauge value (cumulative since last reset) or `deriv()` for a
    short-window throughput estimate.
  - Help text on each metric documents the reset behavior.

  Rationale for shipping this in a patch release: the metrics were only
  introduced six weeks ago (v1.8.0, 2026-04-02), shipped in two releases
  (v1.8.0 and v1.8.1), are not referenced in any in-repo dashboard, and
  had a real correctness bug under any non-trivial cluster reconfigure.
  The disruption window is small and the longer the broken Counter ships,
  the more downstream consumers we'd break later.

  The `dashboards_grafana/05-slurm-scheduler.json` dashboard ships in this
  release with a new **"Job Lifecycle (since slurmctld start)"** row
  exposing the five renamed metrics — the first in-repo visualisation
  of these counters.

### 🐛 Bug Fixes

- **`node` collector — long node names silently dropped (issue #10):**
  `sinfo -O "NodeList,..."` uses fixed-width columns (default 20 chars for
  `NodeList`). On clusters with node hostnames longer than 20 characters, the
  `NodeList` column collided with `AllocMem`, leaving lines with only 5
  whitespace-separated tokens instead of 6. The parser silently skipped them
  (`if len(node) < 6 { continue }`), causing entire nodes to disappear from the
  metrics map — and the collector reported success (no error, non-zero
  `slurm_exporter_collector_duration_seconds`) while exposing zero
  `slurm_node_*` series. Fixed by switching to variable-width columns
  (`NodeList: ,AllocMem: ,...`); the trailing `:` instructs `sinfo` to size
  each column to its value. The parser itself is unchanged.
  Regression was introduced when `slurm_node_status` was added; the original
  2022 fix for the same class of bug (commit `77080e0`) was inadvertently
  reverted at that point.

- **`reservations` collector — phantom row when no reservations defined
  (issue #26):**
  `parseReservations` processed every non-empty record from
  `scontrol show reservation`, including the literal `"No reservations in
  the system"` line that scontrol emits on an empty cluster. With no
  key=value to parse, every field stayed at its zero value and the
  record was still appended — producing a phantom series:

  ```
  slurm_reservation_info{reservation_name="",...} 1
  slurm_reservation_start_time_seconds{reservation_name=""} -6.21355968e+10
  slurm_reservation_end_time_seconds{reservation_name=""} -6.21355968e+10
  ```

  The `-6.21e+10` timestamp is `time.Time{}.Unix() = -62135596800`
  (year 0001), which Grafana renders as `1968-01-12 20:06:43` on the
  reservations dashboard.

  Fixed by skipping records that didn't yield a `ReservationName`. Empty
  clusters now produce zero `slurm_reservation_*` series; dashboards
  show "No data" instead of a fake 1968 reservation. Non-regression test
  added with a `sreservations_empty.txt` fixture.
- **`scheduler` collector — RPC usernames with hyphens silently truncated
  (PR #28 by @ncreddine):**
  `schedulerRPCLineRe` used the character class `[A-Za-z0-9_]*` for the
  username capture group, which silently dropped the hyphen. Usernames
  like `alice-21` were truncated to `alice`, collapsing every per-user RPC
  stat onto the prefix and hiding per-user breakdowns in
  `slurm_user_rpc_stats_*`. Extended the class to `[A-Za-z0-9_-]*`.
  Table-driven non-regression test added.
- **`accounts` collector — `gres:gpu:N` (colon separator) not parsed
  (PR #28 by @ncreddine):**
  `tresGPURe` matched only the slash form `gres/gpu:N` from `squeue %b`
  output. Some Slurm versions emit `gres:gpu:N` (colon prefix) which fell
  through to a count of 0, undercounting `slurm_account_cores_gpu` and
  `slurm_user_cores_gpu` on those clusters. Broadened the prefix to
  `gres[:/]gpu`. Existing slash-form tests still pass; four colon-form
  cases added.
- **Startup fails if `sbatch`/`salloc`/`srun` are absent (issue #24, PR #25 by
  @UeliDeSchwert):**
  `ValidateBinaries()` required `sbatch`, `salloc`, and `srun` in addition to
  the Slurm monitoring tools actually used by the exporter. These three
  job-submission binaries are never invoked by any collector and are often
  absent on read-only monitoring containers or minimal Slurm client
  installations, causing the exporter to refuse to start with
  `--slurm.bin-path` set. They are now removed from the required list.
  Companion follow-up below restores informational visibility.

### ✨ Improvements

- **`slurm_info` collector — expose job submission tool versions when
  available:**
  Following the issue #24 fix that dropped `sbatch`/`salloc`/`srun` from the
  strict startup validation, they are now reintroduced in the `slurm_info`
  collector as **silent optionals**: emitted only when present on the host,
  with no log entry or metric when absent. Lookup uses `os.Stat` against
  `--slurm.bin-path` (or `exec.LookPath` against `$PATH` when empty),
  avoiding any subprocess spawn. Required binaries continue to emit a
  `slurm_info{binary="X",version="not_found"}` series with value `0` when
  missing, so operators can still alert on their absence.

- **`partitions` collector — default partition `*` suffix not stripped
  (issue #20, PR #21 by @UeliDeSchwert):**
  Slurm appends `*` to the default partition name in `sinfo` output
  (e.g. `compute*`). The `nodes` collector already strips this suffix
  (`nodes.go:169`), but `partitions.go` did not, producing
  `slurm_partition_cpus_*` and `slurm_partition_gpus_*` with
  `partition="compute*"` while every other metric used `partition="compute"`.
  PromQL joins on the partition label silently returned no data for the
  default partition. Fixed by applying the same `strings.TrimRight(..., "*")`
  in both the CPU path and the GPU path; two unit tests verify the
  asterisk-suffixed input is stored under the bare key.
- **`queue` collector — same `*` suffix bug, defensive companion fix:**
  `squeue -o "%P"` emits `compute*` for the default partition on some
  Slurm versions; the queue collector now applies the same
  `TrimRight(..., "*")` so `slurm_queue_*` and `slurm_cores_*` labels
  stay aligned with the partitions and nodes collectors.
  Non-regression test added.
- **`sacct_efficiency` collector — graceful shutdown on SIGTERM/SIGINT
  (issue #18, PR #19 by @UeliDeSchwert):**
  The background refresh goroutine was started with `context.Background()`,
  which is never cancelled. On SIGTERM/SIGINT, the HTTP server stopped but
  the goroutine — possibly mid-`sacct` invocation — was only terminated
  when the OS killed the process. Now wired through `signal.NotifyContext`,
  so the context is cancelled cleanly on signal. The main loop also waits
  up to 5 seconds for the goroutine to exit (via the new `Done()` channel)
  before returning, so any in-flight `sacct` call has a chance to complete.
  Non-regression test added (`TestSacctEfficiencyCollector_DoneClosesOnCancel`).
- **`gpus` collector — `slurm_gpus_other` can be negative on busy clusters
  (issue #16, PR #17 by @UeliDeSchwert):**
  `other` is computed as `total − allocated − idle`, where each value comes
  from a separate `sinfo` invocation. Cluster state can change between the
  three calls, transiently producing `alloc + idle > total` and a negative
  gauge — which Grafana renders incorrectly. Clamped to zero with a Debug
  log when the clamp triggers (useful for diagnosing suspected miscounting
  without spamming production logs, since the race is common on loaded
  clusters). A follow-up issue tracks the proper fix: consolidate the three
  `sinfo` calls into one to eliminate the race at the source.
- **`sacct_efficiency` collector — memory efficiency average understated
  (issue #14, PR #15 by @UeliDeSchwert):**
  `slurm_job_mem_efficiency_avg` accumulated the per-job ratio only when
  `ReqMemMB > 0` (correct) but divided by `JobCount` — the total number of
  jobs, including those without memory requests. On a cluster where half the
  jobs are submitted without `--mem`, the reported average was half the real
  value. Same structural pattern fixed for `slurm_job_cpu_efficiency_avg`
  (lower impact in practice). Fixed by adding `CPUJobCount` and `MemJobCount`
  to the aggregates struct and dividing by the per-metric counter. Affected
  sites will see both averages rise to their correct value after upgrade.
  Non-regression test added.
- **`queue` collector — `slurm_queue_suspended` and `slurm_cores_suspended`
  never emitted (issue #12, PR #13 by @UeliDeSchwert):**
  Both metrics were declared, described, and populated by `ParseQueueMetrics`,
  but `Collect()` was missing the `PushMetric` / `pushAggregatedNVal` calls
  for them — every scrape silently dropped these series. The global
  `slurm_jobs_suspended` gauge was unaffected. Fixed by adding the four
  missing calls (two in the per-user branch, two in the aggregated branch).
  Non-regression test added.
- **`partitions` collector — multi-type GPU undercount:**
  `parseGpuCount()` in `partitions.go` used `FindStringSubmatch` (singular),
  returning only the first `gpu:*:N` match in a GRES string. On nodes
  exposing multiple GPU types (`gpu:A100:4,gpu:H100:2`),
  `slurm_partition_gpus_allocated` and `slurm_partition_gpus_idle` were
  silently undercounted (returned 4 instead of 6). Fixed by iterating over
  comma-separated GRES sub-specs and accumulating, matching the
  long-correct behavior of `gpus.go::parseGPUCount`. Cluster-wide
  `slurm_gpus_*` was not affected. Affected sites will see
  `slurm_partition_gpus_*` values increase to their real count after
  upgrade.

### 🛡️ Defensive hardening

- **`partitions` collector — fixed-width truncation of GRES strings:**
  `sinfo --Format=...Gres:50,GresUsed:50` truncates rich GRES specs on busy
  GPU nodes (multi-type GPUs, MIG slices) at 50 chars, producing wrong GPU
  counts in `slurm_partition_gpus_*`. Same class of bug as the `node`
  collector issue. Switched to variable-width (`Gres: ,GresUsed:`).
- **`gpus` collector — fixed-width truncation of GRES strings:**
  Same fix applied to `IdleGPUsData()` and `TotalGPUsData()`. `AllocatedGPUsData()`
  was already correct.
- **Empty-parse warning logs:** `node` and `partitions` collectors now emit a
  warning when the parser returns zero entries despite the underlying command
  succeeding. This makes the failure mode from issue #10 fail loudly instead
  of silently — operators see the warning instead of staring at "No data"
  dashboards with no clue why.
- **Data race in `sacct_efficiency` test fixed; `Done()` channel added.**
  `TestSacctEfficiencyCollector_ErrorKeepsPreviousCache` had two races caught
  by `go test -race`: an unprotected `callCount++` in the mock closure, and
  the test's `defer Execute = oldExecute` racing with the background refresh
  goroutine still reading `Execute`. Counter now uses `atomic.Int64`, and
  `SacctEfficiencyCollector` exposes a new `Done() <-chan struct{}` channel
  that closes when the background goroutine exits — letting tests
  synchronise teardown deterministically. Production behavior unchanged.

### 📊 Dashboard impact

No dashboard JSON changes — metric names, labels, and types are unchanged.
However, **clusters previously affected by silent truncation will see metric
values increase** as the missing series reappear:

- `slurm_node_*` series for nodes with hostnames > 20 chars will now be exposed
  (previously absent), so `count`/`sum` queries over them will rise to their
  real values.
- `slurm_partition_*` series for partitions with names > 30 chars will now
  appear under their full name; series previously stored under a truncated
  partition name will disappear.
- `slurm_gpus_*` and `slurm_partition_gpus_*` will reflect the true GPU
  inventory on nodes with rich GRES specs (multi-type GPU, MIG).

The `or vector(0)` guards added to dashboards in v1.8.1 remain valid (they
protect against legitimately empty states) and require no rework.

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
