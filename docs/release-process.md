# Release Process

> Back to [README](../README.md)

This document describes the maintainer workflow for cutting a release of
`slurm_exporter`. It is the playbook for any future patch or minor release,
distilled from the v1.8.2 cycle.

The goal is to keep releases:

1. **Coherent** — every change in a release fits the release theme (patch =
   bug fixes only; minor = features and refactors; major = breaking changes).
2. **Reviewable** — atomic commits, conventional commit messages, BREAKING
   changes flagged explicitly.
3. **Trustworthy** — every fix lands with a non-regression test, every
   metric in `docs/metrics.md` matches what `/metrics` actually exposes.
4. **Respectful of contributors** — community PRs are credited via
   `Co-authored-by`, not silently absorbed.

---

## 1. Open the release branch

Always release from a dedicated branch — never commit straight to `master`.

```bash
git checkout master && git pull
git checkout -b fix/vX.Y.Z       # use feat/vX.Y for minor releases
```

Naming convention:

| Branch prefix | Used for |
|---|---|
| `fix/vX.Y.Z` | Patch release: bug fixes only |
| `feat/vX.Y` | Minor release: features + non-breaking improvements |
| `release/vX` | Major release: breaking changes |

---

## 2. Triage the backlog

Before opening the branch, list everything that *could* go in:

```bash
gh pr list  --repo $OWNER/slurm_exporter --state open
gh issue list --repo $OWNER/slurm_exporter --state open
```

For each item, decide:

- **In scope** for this release — note the PR/issue number in your scratchpad.
- **Out of scope** but actionable — comment on the issue/PR with the planned
  release (e.g. "tracked for v1.9").
- **Stale** — close with a polite explanation.

Aim for a release that ships **one theme** (e.g. v1.8.2 was "silent metric
loss in the node collector + community PR backlog"). Avoid mixing themes.

---

## 3. Integrate community PRs

For each PR you accept, follow the same loop:

### 3.1 Analyse

For every PR, run a four-step analysis:

1. **What it claims to fix** — read the issue + PR body.
2. **What the diff actually changes** — `gh pr diff <N>`.
3. **Whether it conflicts with the current branch** — does it touch lines
   you've already modified in this release?
4. **Whether it changes metric values, names, or labels** — anything visible
   to the user.

If any of these aren't clear, comment on the PR for clarification before
integrating.

### 3.2 Integrate locally with credit

We don't merge community PRs through the GitHub UI (this would scatter the
release across many PRs). Instead, integrate the diff locally in the release
branch with `Co-authored-by:` in the commit message:

```bash
# Apply the change manually (Edit / fix file by file)
git add <files>
git commit -m "$(cat <<'EOF'
fix(<collector>): <one-line summary> (#<issue>, #<pr>)

<longer explanation>

Refs: #<issue>
Co-authored-by: <author> <<author>@users.noreply.github.com>
EOF
)"
```

This preserves the contributor's authorship in GitHub's contributor page
without scattering the release across many merge commits.

### 3.3 One commit = one logical change

If a PR fixes two distinct bugs (e.g. PR #28 fixed both a regex hyphen issue
*and* a GRES separator issue), split it into two commits. Each commit must:

- Compile and pass `go test` on its own (so `git bisect` works).
- Have its own non-regression test if applicable.
- Carry the `Co-authored-by:` trailer.

### 3.4 Test of non-regression

Every code change must come with a test that:

- Fails before the fix (you should briefly verify this — write the test
  first, run it, see it fail).
- Passes after the fix.
- Uses the project's existing test fixtures in `test_data/` when possible.

If the PR doesn't come with a test, write one. If a fix is purely defensive
(e.g. log on empty parse), a unit test that exercises the empty path is
enough.

---

## 4. Defensive audit

For each bug you fix, ask: **is the same class of bug present anywhere
else in the codebase?**

Example (v1.8.2):
- Issue #10 was about `sinfo -O` using fixed-width columns in
  `internal/collector/node.go`. The defensive audit revealed the same
  pattern in `internal/collector/partitions.go` and
  `internal/collector/gpus.go`. All three were fixed in the same release.
- Issue #20 was about partition `*` suffix not being stripped in
  `partitions.go`. The same parsing pattern existed in `queue.go` and got
  the same defensive fix.

Search patterns:

```bash
# Find all sinfo --Format / -O sites
grep -rn '"-O"\|"--Format=\|"-o "' internal/collector/

# Find all partition / user / job state parsing
grep -rn 'fields\[\|splitLine\[' internal/collector/
```

Defensive fixes ship as separate commits with `(defensive)` in the
summary, no `Co-authored-by:`.

---

## 5. Validate continuously

**Every command below runs inside a containerised toolchain** — Docker is
the only requirement on the developer machine, the result is identical on
every host. The image is defined in [`scripts/docker/tools/`](../scripts/docker/tools/)
and built lazily on first use.

After every commit, run:

```bash
make check    # vet + golangci-lint + tests (containerised)
```

**Before tagging, both of these must be green — release blockers:**

```bash
make check    # exit 0 required
make report   # exit 0 → grade ≥ B; aim for A or A+
```

`make report` is the offline equivalent of
[goreportcard.com](https://goreportcard.com): runs `gofmt -s`, `go vet`,
`gocyclo`, `ineffassign`, `misspell`, and a `LICENSE` check, prints a
per-check score, and assigns a global grade. It exits non-zero below
grade B so it can gate CI / pre-commit. The score matches what
goreportcard.com would publish.

Also run at least once before tagging:

```bash
make race     # race detector (containerised)
make build    # full ldflags build (native)
```

If `make race` fails on a pre-existing test (not something you introduced),
fix it in this release if cheap, otherwise file a follow-up issue.

If `make report` drops a grade vs. master (e.g. a new function above
gocyclo threshold), either refactor before tagging or annotate explicitly
with `//nolint:gocyclo` and a rationale comment.

---

## 6. End-to-end test on a live cluster

Spin up the test cluster and validate the actual `/metrics` output, with
every collector enabled, debug logs captured, and release-specific
assertions made explicit.

The detailed step-by-step playbook is in
**[`docs/validation-checklist.md`](validation-checklist.md)** — 11 steps,
copy-pasteable commands, expected outputs, and "if it fails" diagnostics
for each. Designed so a human or an AI agent can execute the validation
end-to-end without prior context.

Short version of what that checklist covers:

```bash
make -C scripts/testing setup     # Step 2 — bring up cluster + deploy binary

# Step 3 — restart exporter with ALL collectors and debug logs
# (see the checklist for the full command)

# Step 4-5 — verify scrape returns 200 and every collector success=1
# Step 6 — inspect the exact Slurm commands logged
# Step 7 — submit workload, re-scrape
# Step 8 — diff /metrics ↔ docs/metrics.md
# Step 9 — release-specific assertions (template in the checklist)
# Step 10 — visual Grafana dashboard pass
```

### Spot-check the release theme

For v1.8.2 we explicitly verified:

- The fix actually fires under realistic conditions (e.g. parse a fixture
  that previously dropped a node, confirm the node now appears).
- Renamed metrics use the new names (e.g. `slurm_scheduler_jobs_submitted`
  without `_total`).
- Dashboards in `monitoring/grafana/dashboards/` still render — `make redeploy-dashboards`
  reimports them and Grafana is reachable at `http://localhost:3000`.

### Diff exposed metrics against docs

```bash
# Names emitted by the live exporter
grep -E '^slurm_' /tmp/metrics.txt | sed 's/[{ ].*//' | LC_ALL=C sort -u > /tmp/exposed.txt

# Names documented in docs/metrics.md
grep -oE '`slurm_[a-z_]+`' docs/metrics.md | tr -d '`' | LC_ALL=C sort -u > /tmp/doc.txt

# Gaps in both directions
comm -23 /tmp/doc.txt /tmp/exposed.txt   # documented but not exposed
comm -13 /tmp/doc.txt /tmp/exposed.txt   # exposed but not documented
```

Treat "exposed but not documented" entries as bugs to fix in this release
(unless they're histogram `_bucket/_count/_sum` suffixes, which are implicit).

"Documented but not exposed" entries are usually contextual (e.g. account-level
metrics need running jobs, sacct_efficiency is opt-in). Verify each before
declaring the doc clean.

> The full diff workflow is automated and explained in the
> [validation checklist](validation-checklist.md#step-8--diff-exposed-metrics-against-docsmetricsmd).

---

## 7. Update documentation

For every change that affects user-visible behavior:

| File | Update when |
|---|---|
| `CHANGELOG.md` | Always. New version section with Breaking Changes / Bug Fixes / Improvements / Dashboard impact sub-sections |
| `docs/metrics.md` | Any metric added, renamed, removed, or label-changed |
| `docs/metrics-examples.md` | Any new metric in a section where a representative sample would help users |
| `docs/configuration.md` | Any new flag, default change, or behavior toggle |
| `README.md` | Headline features only |
| `monitoring/grafana/dashboards/*.json` | Any rename, drop of a metric that's actually used in a panel, or addition of a high-value metric that deserves its own panel |
| Companion `monitoring-stacks/alerts/slurm.yml` | Any metric rename, drop, or new opt-in collector that recording rules / alerts depend on |

For breaking changes, include a **migration table** in CHANGELOG:

```markdown
| Old | New |
| --- | --- |
| `slurm_foo_bar_total` (Counter) | `slurm_foo_bar` (Gauge) |
```

---

## 8. Push and open the PR

```bash
git push -u origin fix/vX.Y.Z
gh pr create --base master --head fix/vX.Y.Z \
  --title "fix(vX.Y.Z): <theme> + N community PRs" \
  --body-file /tmp/pr_body.md
```

PR body structure (use the v1.8.2 PR as the canonical template):

```
## Summary
1-3 bullets.

## ⚠️ Breaking change          (omit if none)
Migration table + rationale (esp. how recent the affected metric is).

## Bug fixes
### <Theme of this release>
### Integrated community PRs   (table: PR | issue | author | subject)
### Other hardening

## Dashboard impact
Effects users will see on values, not on JSON structure (unless JSON changed).

## Test plan
Checklist already ticked by the time the PR opens.

## Follow-ups (next release)
Bullet list with issue numbers.
```

Self-review the diff on GitHub before requesting outside review.

---

## 9. Post-merge cleanup

Once the PR is merged to `master`:

### Close integrated community PRs

```bash
for pr in <list of integrated PRs>; do
  gh pr close "$pr" --repo $OWNER/slurm_exporter \
    --comment "Integrated into vX.Y.Z (see release notes). \
Thanks for the contribution — your authorship is preserved via Co-authored-by in the commit. 🙏"
done
```

### Respond to issues

For issues that were fixed: post a short comment pointing to the release and
close.

For issues that are *acknowledged but planned for the next release*: leave
them open and comment with the planned milestone and what the user can do
in the meantime.

### Test on a real platform before the final tag

The Docker test cluster (`scripts/testing`) catches structural bugs, but it
runs on short hostnames (`c1`–`c10`), no real workload variability, and a
clean Slurm install. **A real cluster will surface things the test cluster
can't** — long node names, multi-type GPUs, slurmctld restarts, sustained
load, version-specific output quirks. Before tagging the final release,
run the binary on an actual cluster.

Two approaches depending on risk:

#### A. Release candidate tag (recommended for minor/major and breaking changes)

For releases that rename metrics, change types, or touch the scheduler
(v1.8.2 was one of these), tag a release candidate first. CI will publish
it as a GitHub **pre-release** thanks to the `prerelease: auto` setting in
`.goreleaser.yaml`, so users won't grab it by accident from
`go install @latest` or release-assets automation.

```bash
git checkout master && git pull
git tag -a vX.Y.Z-rc1 -m "vX.Y.Z release candidate 1"
git push origin vX.Y.Z-rc1
# → CI publishes a GitHub pre-release with the rc1 binary
```

Deploy the rc1 binary to a staging or production cluster, leave it running
through at least one full work cycle (a few hours, ideally overnight), and
watch for:

- Unexpected dips or spikes in any metric series.
- New error logs from the exporter.
- Dashboard panels suddenly showing "No data" where they used to.
- `slurm_exporter_collector_duration_seconds` getting much longer.

If you find an issue: fix on a hotfix branch, merge, then `vX.Y.Z-rc2`.
Repeat as needed. Once stable, ship the final tag:

```bash
git tag -a vX.Y.Z -m "vX.Y.Z — <headline>"
git push origin vX.Y.Z
```

#### B. Local build + manual deploy (for trivial patches)

For a patch that only touches docs, tests, or a clearly isolated bug, the
RC dance is overkill. Build the binary from merged `master` and ship it
manually to a staging cluster:

```bash
git checkout master && git pull
make build

scp bin/slurm_exporter staging-cluster:/usr/local/bin/slurm_exporter.next
ssh staging-cluster "
  /usr/local/bin/slurm_exporter.next --version &&
  systemctl stop slurm_exporter &&
  mv /usr/local/bin/slurm_exporter.next /usr/local/bin/slurm_exporter &&
  systemctl start slurm_exporter
"
```

Watch Grafana and `/metrics` for the same signals as approach A, for a
shorter window (~30 min) before tagging.

#### Decision matrix

| Change type | Approach |
|---|---|
| Breaking change (metric rename/retyped, label change) | **A — RC tag**, deploy to staging, full overnight |
| New collector / new exposed metric | **A — RC tag**, deploy to staging, a few hours |
| Bug fix to existing collector | **B — local build**, 30 min validation |
| Docs / test-only patch | None required — `make check` is enough |

### Tag the final release

Once you're confident on a real cluster:

```bash
git tag -a vX.Y.Z -m "vX.Y.Z — <short headline>"
git push origin vX.Y.Z
```

CI (`.github/workflows/release.yml`) picks up the tag, runs
`lint → test → goreleaser release`, and publishes the binary + the
GitHub release page.

---

## 10. Handle PRs that don't ship in this release

When you receive a PR that has the right idea but doesn't fit the current
release (wrong scope, missing tests, conflicts with in-flight work), the
right answer is **not** to leave it rotting.

Comment on the PR with:

1. **What's good** — what the PR gets right.
2. **What blocks merging today** — concrete blockers with reasons (not
   opinions).
3. **What the plan is** — which release you'll integrate it in, what you'll
   adapt or add (tests, flags, dashboard panel).
4. **A concrete signal of intent** — e.g. "I'll adapt and ship in v1.9 this
   month. You'll be credited via Co-authored-by."
5. **Concrete preview** — when relevant, show an example of the metrics
   that would be exposed, or a snippet of dashboard PromQL.

Close with the maintainer formula:
> I'm not closing the PR — keeping it open as the reference discussion while
> I adapt. Thanks again, nice contribution.

The PRs #29 / issue #27 responses from the v1.8.2 cycle are good templates.

---

## Reference: what v1.8.2 looked like

For context, v1.8.2 (the release that codified this process) was made of:

- 1 issue-driven bug fix (#10, silent metric loss on long node names).
- 3 defensive fixes (same bug class on partitions/gpus, parseGpuCount
  multi-type, queue partition `*` strip).
- 8 integrated community PRs (#12/#13, #14/#15, #16/#17, #18/#19, #20/#21,
  #22/#23 — breaking —, #24/#25, and PR #28).
- 1 issue-driven bug fix (#26, reservation phantom row).
- 2 follow-up responses on issues planned for v1.9 (#27, PR #29).
- 4 docs updates (metrics, metrics-examples, CHANGELOG, this file).
- 25 commits, all conventional, all atomic.

Time from branch creation to PR open: ~1 working day. Plan around that
order of magnitude for similar releases.
