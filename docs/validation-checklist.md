# Release Validation Checklist

> Back to [README](../README.md) · See also [release-process.md](release-process.md)

A step-by-step procedure to validate a release candidate of `slurm_exporter`
end-to-end against the Docker test cluster (`scripts/testing/`). Designed so
that a human or an AI agent can execute it without prior context.

**Each step has:**
- A **Command** — a copy-pasteable shell block.
- An **Expected** outcome — what the command should print or produce.
- An **If it fails** section — how to diagnose and what to fix before
  continuing.

**Pre-requisites**:
- The branch you want to validate is checked out and `make build` succeeds.
- Docker Engine is running.
- `~/Dev/work/apps_repo/orchestration-hpc/slurm-docker-cluster` exists, or
  another path that `scripts/testing/_lib.sh` auto-detects.
- The host has at least 4 GB free RAM and 5 GB free disk.

---

## Step 1 — Pre-flight static checks

Confirm the workspace is clean and the binary builds.

### Command

All `make vet`, `make lint`, `make test`, `make check`, `make race`, and
`make report` targets run inside a containerised toolchain
(`scripts/docker/tools/`). The only host requirement is Docker.

```bash
cd "$(git rev-parse --show-toplevel)"
git status --short
make check      # vet + lint + tests, all in container
make report     # offline goreportcard.com equivalent, in container
make build      # full ldflags build (produces bin/slurm_exporter)
bin/slurm_exporter --version
```

### Expected

- `git status --short` shows only intentional changes (and possibly
  `.claude/`, which is gitignored noise).
- `make check` exits 0 — `go vet`, `golangci-lint`, and all tests pass.
- `make report` exits 0 — average grade ≥ B (current master is A+ at
  98.76%). Any drop in grade vs. master is a release blocker.
- `make build` produces `bin/slurm_exporter` without warnings.
- `bin/slurm_exporter --version` prints a version string including the
  current branch name (e.g. `v1.8.1-NN-gXXXXXXX` for an unreleased branch).

### If it fails

- **`make check` errors**: fix before continuing. Don't validate a branch
  that doesn't pass static checks.
- **`make report` drops a grade**: read the per-check breakdown — most
  common cause is a new function above the gocyclo threshold (15).
  Either refactor or annotate with `//nolint:gocyclo` + rationale.
- **First `make` call is slow**: the tools image is being built. Subsequent
  runs reuse the cache and start in seconds.

---

## Step 2 — Bring up the Docker test cluster

Starts the Slurm cluster + Prometheus + Grafana + deploys the freshly-built
binary as a system process inside `slurmctld`.

### Command

```bash
make -C scripts/testing setup
```

### Expected

The script prints nine green checkmarks `[1/9]` through `[9/9]` and
finishes with `✓ Done. Run: make workload && make screenshots`.

Specifically:
- `✓ 10/10 nodes registered`
- `✓ Monitoring stack started (Prometheus + Grafana)`
- `✓ Exporter running on slurmctld:9341`
- `✓ 10 dashboards imported, 0 failed`

### If it fails

- Step `4/9` (node registration) timing out: the WSL2 / Docker network is
  slow. Wait a minute, then `make -C scripts/testing redeploy` and retry.
- `9/9` failed (exporter not reachable): the binary built with the wrong
  toolchain for the cluster image. Check `bin/slurm_exporter` runs locally
  first.
- Dashboard import fails: check `docker logs grafana`; usually a Grafana
  startup delay, retry `make -C scripts/testing redeploy-dashboards`.

---

## Step 3 — Restart the exporter with all collectors + debug logs

The default `make setup` runs the exporter with the minimum flags. For a
proper validation run, restart it with **every collector explicitly enabled**
(including the opt-in ones like `sacct_efficiency`) and `--log.level=debug`
so we can inspect every Slurm command it issues.

### Command

```bash
docker exec slurmctld bash -c '
  pkill -9 -f slurm_exporter 2>/dev/null
  sleep 1
  rm -f /tmp/exporter.log
  nohup /usr/local/bin/slurm_exporter \
    --web.listen-address=:9341 \
    --log.level=debug \
    --command.timeout=10s \
    --collector.accounts \
    --collector.cpus \
    --collector.drain_reason \
    --collector.fairshare \
    --collector.fairshare.user-metrics \
    --collector.gpus \
    --collector.info \
    --collector.licenses \
    --collector.node \
    --collector.nodes \
    --collector.nodes.feature-set \
    --collector.partitions \
    --collector.queue \
    --collector.queue.user-label \
    --collector.reservations \
    --collector.sacct_efficiency \
    --collector.sacct.interval=30s \
    --collector.sacct.lookback=1h \
    --collector.scheduler \
    --collector.users \
    > /tmp/exporter.log 2>&1 &
  sleep 3
  curl -s http://localhost:9341/healthz
'
```

### Expected

- Final output line is `ok` (from `/healthz`).
- The first 20 lines of `/tmp/exporter.log` contain `level=INFO msg="Collector enabled"` for every collector you passed (16 of them).
- One `level=INFO msg="Starting Slurm Exporter server..."` line.
- One `level=INFO msg="Listening on" address=[::]:9341` line.
- **No `level=ERROR` or `level=WARN` entries** in the startup phase.

### If it fails

- `address already in use`: another exporter instance is alive. Re-run the
  `pkill` and check `docker exec slurmctld ps aux | grep slurm_exporter`.
- `Collector enabled` missing for one collector: a flag name changed in the
  binary. Compare with `slurm_exporter --help` output.
- Warnings during startup: capture them — they often signal a real bug
  introduced by the branch.

---

## Step 4 — Verify a full scrape with no errors

A single `/metrics` request must succeed without producing any error log
entry.

### Command

```bash
docker exec slurmctld bash -c '
  curl -s -o /tmp/scrape.txt -w "HTTP %{http_code} in %{time_total}s\n" \
    http://localhost:9341/metrics
'
docker exec slurmctld bash -c '
  echo "=== Metric series count ==="
  grep -cE "^slurm_" /tmp/scrape.txt
  echo "=== Errors / warnings in logs ==="
  grep -cE "level=ERROR|level=WARN" /tmp/exporter.log
'
```

### Expected

- HTTP 200, scrape duration < 1 second on a quiet cluster.
- Metric series count ≥ 400 (with all collectors enabled and after `make
  workload`, expect 500–600; without workload, 400–500).
- **0 errors and 0 warnings** in the log file.

### If it fails

- HTTP 500 / 503: the exporter crashed mid-scrape. Read the last 50 lines
  of `/tmp/exporter.log` for the panic trace.
- Scrape duration > 5s: a collector is timing out. Inspect
  `slurm_exporter_collector_duration_seconds` after the scrape to find
  which one.
- Any `level=ERROR`: real problem to investigate before tagging.

---

## Step 5 — Validate all collectors succeeded

Each collector exposes a success gauge. They must all be `1`.

### Command

```bash
docker exec slurmctld bash -c '
  grep -E "^slurm_exporter_collector_success" /tmp/scrape.txt | sort
'
```

### Expected

16 lines, all ending with ` 1`:

```
slurm_exporter_collector_success{collector="accounts"} 1
slurm_exporter_collector_success{collector="cpus"} 1
slurm_exporter_collector_success{collector="drain_reason"} 1
slurm_exporter_collector_success{collector="fairshare"} 1
slurm_exporter_collector_success{collector="gpus"} 1
slurm_exporter_collector_success{collector="info"} 1
slurm_exporter_collector_success{collector="licenses"} 1
slurm_exporter_collector_success{collector="node"} 1
slurm_exporter_collector_success{collector="nodes"} 1
slurm_exporter_collector_success{collector="partitions"} 1
slurm_exporter_collector_success{collector="queue"} 1
slurm_exporter_collector_success{collector="reservation_nodes"} 1
slurm_exporter_collector_success{collector="reservations"} 1
slurm_exporter_collector_success{collector="sacct_efficiency"} 1
slurm_exporter_collector_success{collector="scheduler"} 1
slurm_exporter_collector_success{collector="users"} 1
```

### If it fails

- Any `... 0` value: that collector failed silently. Find its error in
  `/tmp/exporter.log` and fix it before tagging.
- Missing line for a collector: the flag wasn't picked up. Re-check the
  command in Step 3.

---

## Step 6 — Inspect Slurm commands the exporter actually ran

This is the moment to confirm any release-specific change is wired through.
For v1.8.2 we expected `sinfo -O` to use variable-width column specs (`:` at
the end of each field).

### Command

```bash
docker exec slurmctld bash -c '
  echo "=== sinfo / squeue / sacct calls in the last scrape ==="
  grep -E "Executing command" /tmp/exporter.log \
    | awk -F"command=" "{print \$2}" \
    | awk -F" elapsed_ms=" "{print \$1}" \
    | sort -u
'
```

### Expected

A unique list of `command=X args="..."` lines. **Inspect each one** for:
- The argument format matches what `internal/collector/*.go` documents in
  its function comments.
- No suspicious tokens (e.g. unescaped quotes, missing `:` on variable-width
  fields, fixed widths where they shouldn't be).

For v1.8.2 the line we wanted to see was:

```
command=sinfo args="-h -N -O NodeList: ,AllocMem: ,Memory: ,CPUsState: ,StateLong: ,Partition:"
```

### If it fails

- A command argument doesn't match what the code says: regression in the
  release branch. Check the matching `*Data()` function in
  `internal/collector/`.
- Unexpected commands appear: a new collector ran that wasn't supposed to,
  or one ran twice. Check `collector.SetCommandTimeout` and the constructor
  chain in `cmd/slurm_exporter/main.go`.

---

## Step 7 — Generate a realistic workload

Empty-cluster metrics don't exercise the per-user / per-account / queue
collectors. Submit a workload, then re-scrape.

### Command

```bash
make -C scripts/testing workload N=30

# Resume any nodes stuck in DOWN (common after a fresh container restart)
docker exec slurmctld scontrol update NodeName=ALL State=RESUME 2>/dev/null

# Wait for the scheduler to dispatch
sleep 15

# Refresh metrics
docker exec slurmctld curl -s http://localhost:9341/metrics > /tmp/scrape_workload.txt
docker exec slurmctld bash -c 'wc -l /tmp/scrape_workload.txt'
```

### Expected

- `wc -l` reports > 800 lines (more series under load).
- Job-related metrics are non-zero somewhere:

  ```bash
  grep -E "^slurm_jobs_(pending|running|completed) " /tmp/scrape_workload.txt
  ```

  At least one of `pending` / `running` should be > 0 right after submission;
  `completed` may grow on a second scrape a few seconds later.

### If it fails

- All jobs stay PD with reason `(ReqNodeNotAvail)`: nodes are DOWN. Run the
  `scontrol update State=RESUME` command above and wait another 15s.
- `make workload` errors: missing OS users. Re-run
  `bash scripts/testing/_lib.sh setup-os-users`.

---

## Step 8 — Diff exposed metrics against `docs/metrics.md`

Catch undocumented or stale metrics. The exporter is the source of truth.

### Command

```bash
docker exec slurmctld cat /tmp/scrape_workload.txt > /tmp/scrape.txt

# Names emitted by the live exporter
grep -E '^slurm_' /tmp/scrape.txt | sed 's/[{ ].*//' | LC_ALL=C sort -u > /tmp/exposed.txt

# Names documented in docs/metrics.md
grep -oE '`slurm_[a-z_]+`' docs/metrics.md | tr -d '`' | LC_ALL=C sort -u > /tmp/doc.txt

echo "=== Documented but NOT exposed ==="
comm -23 /tmp/doc.txt /tmp/exposed.txt

echo
echo "=== Exposed but NOT documented ==="
comm -13 /tmp/doc.txt /tmp/exposed.txt
```

### Expected

- **Documented but not exposed** is small and *contextual* — every entry has
  a defensible reason for not being emitted on this cluster (e.g. no GPU
  jobs, no reservations, sacct_efficiency disabled by default in the doc
  example, no DRAIN nodes). Investigate each; none should be a permanent
  ghost.

- **Exposed but not documented** is **empty** or contains only histogram
  suffixes (`_bucket`, `_count`, `_sum`) which are implicit from the parent
  metric name.

### If it fails

- Any new exposed-but-not-documented name (other than histogram suffixes):
  open `docs/metrics.md`, find the relevant `### Collector` section, add
  a row in its metrics table. Commit before tagging.

---

## Step 9 — Release-specific assertions

Replace this section's contents for each release. The pattern is the same:

1. Identify the bug or feature changing in this release.
2. Find a metric or log signal that proves the fix is wired.
3. Assert it explicitly with grep.

### Example: assertions for v1.8.2

```bash
echo "=== Issue #10 fix: variable-width sinfo -O ==="
grep -E '"-h -N -O NodeList: ' /tmp/exporter.log | head -1
# Expected: a line appears

echo "=== Issue #26 fix: no phantom reservation row ==="
grep '^slurm_reservation' /tmp/scrape.txt | grep 'reservation_name=""' || echo "no phantom row ✓"
# Expected: no phantom row ✓

echo "=== Issue #20 fix: default partition asterisk stripped ==="
grep -E '^slurm_partition_cpus_total\{' /tmp/scrape.txt | grep -v '"cpu\*"' | head
# Expected: lines show partition="cpu" without the asterisk

echo "=== BREAKING #23 fix: scheduler counters renamed (no _total suffix) ==="
grep -E '^slurm_scheduler_jobs_(submitted|started|completed|canceled|failed) ' /tmp/scrape.txt | head
# Expected: exactly five lines, none with _total suffix
```

### Expected

Each assertion prints the expected signal. Any failure here is **a release
blocker** — the fix didn't make it into the binary.

### If it fails

- The grep returns nothing where it should return something: re-check the
  binary version (`bin/slurm_exporter --version`) matches the branch.
  Re-run `make build` and `make -C scripts/testing redeploy`.

---

## Step 10 — Visual inspection of Grafana dashboards

Final pass: pages must render.

### Command

Open in browser (or via Playwright/Chromium for the AI agent):

```
http://localhost:3000  (login admin / admin)
```

Walk through every dashboard in `monitoring/grafana/dashboards/`. For each:
- No "No data" on panels that should have data.
- No PromQL parse errors (red banner).
- Time ranges show plausible values.

For automated screenshots:

```bash
make -C scripts/testing screenshots OUTPUT=/tmp/dashboard-screenshots
```

### Expected

- All 10 dashboards open.
- Panels populated (at minimum: scheduler health metrics, partition counts,
  node states).
- The new "Job Lifecycle (since slurmctld start)" row on
  `05-slurm-scheduler.json` shows non-zero values after Step 7's workload.

### If it fails

- Dashboard says "No data" on a panel: open the panel's edit view, copy the
  expression, run it in Prometheus directly (http://localhost:9090). If
  Prometheus also returns nothing, the metric was renamed or removed in
  this branch — update the dashboard JSON or the metric name.

---

## Step 11 — Tear down

When done, free the resources.

### Command

```bash
make -C scripts/testing stop      # keeps volumes — fast resume with `make start`
# or
make -C scripts/testing clean     # full teardown including Docker volumes
```

---

## Summary checklist (for quick reference)

Tick each as you go:

- [ ] Step 1 — `make check`, `make report` (grade ≥ B), `make build` all green
- [ ] Step 2 — `make setup` completes 9/9
- [ ] Step 3 — Exporter restarted with all collectors + debug
- [ ] Step 4 — `/metrics` returns 200, 0 errors/warnings in log
- [ ] Step 5 — All 16 collectors report `success = 1`
- [ ] Step 6 — Slurm commands logged match expected formats for the branch
- [ ] Step 7 — Workload submitted, jobs run, queue/cores/user metrics populated
- [ ] Step 8 — `docs/metrics.md` ↔ `/metrics` diff is clean
- [ ] Step 9 — Release-specific assertions pass
- [ ] Step 10 — All Grafana dashboards render with data
- [ ] Step 11 — Cluster teardown

Once every box is ticked, the release is ready for the **real-platform
validation** step described in
[release-process.md § Test on a real platform before the final tag](release-process.md#test-on-a-real-platform-before-the-final-tag).
