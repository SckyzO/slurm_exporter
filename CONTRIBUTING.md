# Contributing to slurm_exporter

Thank you for your interest in contributing!

---

## Development Setup

### Prerequisites

- Go 1.25+ (toolchain 1.26.1 recommended)
- Docker + Docker Compose v2
- [golangci-lint](https://golangci-lint.run/) v2.11.3+
- A clone of [giovtorres/slurm-docker-cluster](https://github.com/giovtorres/slurm-docker-cluster) for integration tests

### Quick start

```bash
git clone https://github.com/SckyzO/slurm_exporter.git
cd slurm_exporter
make build
make test
golangci-lint run ./...
```

---

## Definition of Done

Every feature, bug fix, or refactoring **must** pass all steps below before being merged.
This applies equally to PRs from contributors and to internal development.

### Step-by-step protocol

```
1. make build
   → Binary compiles without errors or warnings

2. make test
   → All tests pass
   → Coverage must not decrease vs the previous commit
   → Run with: go test -count=1 -coverprofile=coverage.out ./...
               go tool cover -func=coverage.out | grep total

3. golangci-lint run ./...
   → 0 issues

4. make -C scripts/testing setup
   → Test cluster starts cleanly (10 nodes by default)
   → All 9 Grafana dashboards imported successfully
   → slurm_exporter running on slurmctld:9341

5. make -C scripts/testing workload N=20
   → Jobs submitted across alice/bob/carol/dave/eve/frank
   → Running jobs visible in: docker exec slurmctld squeue --state=RUNNING

6. Prometheus validation (wait one scrape cycle ~35s)
   → For each new metric, verify it exists and has a sensible value:
     curl -s http://localhost:9090/api/v1/query?query=<new_metric> | python3 -m json.tool
   → Must return at least one series with a non-NaN value

7. Log check
   → No unexpected ERROR or WARN in exporter logs:
     docker exec slurmctld curl -s http://localhost:9341/metrics > /dev/null
     docker exec slurmctld cat /var/log/slurm/slurmctld.log | grep -c ERROR
   → slurmctld must not show increased error rate

8. Dashboard validation (if dashboards were modified)
   → make -C scripts/testing screenshots OUTPUT=/tmp/pr-screenshots
   → All panels show data, no "No data" on panels that should have values
   → No PromQL errors visible

9. make -C scripts/testing stop
   → Cluster stops cleanly, no dangling containers

10. act push (CI simulation)
    → act push --workflows .github/workflows/release.yml --job test \
        --platform ubuntu-latest=catthehacker/ubuntu:act-latest --no-cache-server
    → All CI jobs pass
```

### Shortcut for quick checks

For small changes (docs, comments, minor fixes):

```bash
make build && make test && golangci-lint run ./...
```

Integration tests (steps 4-9) are required for any change to:
- Collector code (`internal/collector/`)
- Main entrypoint (`cmd/slurm_exporter/`)
- Grafana dashboards (`dashboards_grafana/`)
- Test cluster scripts (`scripts/testing/`)

---

## Code Conventions

### Naming (Go idioms)

- **Initialisms stay all-caps**: `GPU`, `CPU`, `RPC`, `TRES`, `DBD`, `URL`, `ID`
- **Unexported**: camelCase — `parseGPUFromTRES`, `numCPU`
- **Exported**: PascalCase with full caps initialism — `NewGPUCollector`, `ParseCPUMetrics`
- **Avoid plural on initialisms**: `NewCPUCollector` not `NewCPUsCollector`

### New collectors

Every new collector must have:

1. A `*Data(logger)` function calling `Execute()`
2. A `Parse*()` function with pure logic (no I/O — testable without cluster)
3. A `*Collector` struct implementing `prometheus.Collector`
4. A `New*Collector()` constructor
5. A `test_data/<command_output>.txt` fixture with anonymized real output
6. A `*_test.go` file covering:
   - The parser with at least: happy path, empty input, malformed lines, edge cases
   - The collector via `Execute` mock using the test_data fixture
   - `Describe()` descriptor count
   - Error handling (Execute returns error → no panic, no metrics)

### New Grafana dashboards

- Use `${datasource}` template variable (never hardcoded UID)
- Add `or vector(0)` on `count(metric > 0)` stat panels
- Use `clamp_min(denominator, 1)` on any division
- Set `decimals: 1` on all `percent`/`percentunit` fields
- Include a link to the GitHub repo in `links[]`
- Add the dashboard to `dashboards_grafana/README.md`

### Commit messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(collector): add sacct_efficiency collector with background refresh
fix(dashboard): fix Active Users no data with or vector(0)
docs: update CONTRIBUTING.md with Definition of Done
chore: bump CHANGELOG for v1.8.0
```

---

## Test Data

All fixtures in `test_data/` must be anonymized:
- Real cluster names → `cluster1`, `cluster2`
- Real usernames → `user1`, `user2`, `alice`, `bob`
- Real account names → `account_a`, `hpc_team`, `ml_group`
- Real node names → `c1`, `c2`, `a001`, `b001`

See `test_data/readme.md` for the mapping of commands to fixture files.

---

## Performance Considerations

Before adding a new Slurm command call, check:

1. **Can it be merged** with an existing call? (different format on same command)
2. **Is it cacheable?** (scontrol output rarely changes between scrapes)
3. **What is the cardinality?** (per-job metrics on 100k jobs = problem)
4. **Should it be opt-in?** (expensive commands like `sacct` must default to disabled)

The target: total scrape time < 5s on a 10 000-node cluster at 30s interval.

---

## Releasing

Releases are automated via GoReleaser on tag push. To prepare a release:

```bash
# Ensure clean state
make test && golangci-lint run ./...

# Update CHANGELOG.md with the new version section

# Commit and tag
git add CHANGELOG.md
git commit -m "chore: bump CHANGELOG for vX.Y.Z"
git tag -a vX.Y.Z -m "vX.Y.Z — short description"
git push origin master && git push origin vX.Y.Z
```

The CI pipeline runs `test → lint → release` automatically.
