<!--
Thanks for contributing! Please fill in the sections below.
For larger changes, please open an issue first to discuss the approach —
it saves rebase work for both sides.

Commit message style: Conventional Commits (feat, fix, chore, docs,
refactor, test, ci…). See git log on master for examples.
-->

## Summary

<!-- 1-3 bullets describing what this PR changes and why. -->

## Test plan

<!-- Tick what applies. Skip what doesn't. -->

- [ ] `make check` (containerised vet + lint + tests) passes
- [ ] `make report` stays at grade A+ (offline goreportcard)
- [ ] New behavior covered by a unit test
- [ ] `make race` passes if the change touches concurrent code
- [ ] `make docker-build && make docker-build-minimal` succeed if the change touches Docker
- [ ] `docs/metrics.md` updated if a metric was added / renamed / removed
- [ ] `CHANGELOG.md` updated if the change is user-visible
- [ ] Manual validation on a real cluster, if relevant (see [`docs/validation-checklist.md`](../docs/validation-checklist.md))

The Trivy scan workflow runs automatically on this PR if it touches `Dockerfile*`, `go.mod`, or `go.sum`. Wait for it to go green before requesting review.

## Related issue

<!-- `Fixes #N` if this closes an issue, or `Refs: #N` for a partial fix. -->

## Notes for reviewers

<!-- Anything special: caveats, things you considered and discarded, areas
     you'd like extra eyes on. If your PR touches `squeue -O` / `sinfo
     --Format`, read the "Common Pitfalls" section of CONTRIBUTING.md
     first — it'll save you a debug session. Optional. -->
