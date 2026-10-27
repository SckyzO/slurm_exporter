# Image with every CI / report tool baked in.
#
# Used by `make check` and `make report` so contributors can run them
# without installing Go, golangci-lint, gocyclo, etc. on their host —
# only Docker is required.
#
# Built locally on first use via `make tools-image`; rebuilds are
# incremental thanks to the layer cache.
#
# The Go version below is not free to move on its own. `make check` lints and
# tests inside this image while CI does it with setup-go, so if the two drift
# a contributor can be told their branch is clean by a Go that is not the one
# gating the merge. It tracks the `go-version` pinned in .github/workflows/
# and the `toolchain` directive in go.mod; all three change together or none
# do. Dependabot is told to leave this image alone in .github/dependabot.yml.

FROM golang:1.26.8-alpine@sha256:ce864e7223ac17b1775e6fd0b4c0db580c2eb50e7953a427916379e4b92a1628

# shellcheck  → used by actionlint to lint `run:` shell blocks
# zizmor      → static analysis (security) for GitHub Actions workflows
# build-base  → C toolchain (gcc): `go test -race` requires cgo
RUN apk add --no-cache git bash curl ca-certificates findutils \
        shellcheck zizmor build-base

# Static-analysis tools used by `make report` (the same set goreportcard.com
# runs server-side), by `make check` (lint + vet + test + vuln + actionlint),
# and by `make vuln` / `make actionlint`.
#
# Every version is pinned, and that is not tidiness. `@latest` resolves at
# build time against the Go above, which is itself pinned to match CI, so any
# tool that raises its own `go` directive past that line stops installing and
# takes the whole image — and therefore `make check`, `make test` and
# `make lint` — down with it, on every branch at once. osv-scanner v2.7.0 did
# exactly that: it requires go >= 1.27.0, past the line pinned above, and because
# CI runs these tools through setup-go rather than this image, nothing went
# red to say so.
#
# govulncheck tracks the version pinned in .github/workflows/govulncheck.yml,
# for the same reason the Go version tracks setup-go: `make vuln` must not
# report a different set of findings than the job that gates the merge.
#
# Dependabot does not read `go install` lines, so these are bumped by hand.
# To re-resolve a ceiling, read the tool's `go` directive before bumping:
#     curl -s https://proxy.golang.org/<module>/@v/<version>.mod | grep '^go '
RUN go install github.com/fzipp/gocyclo/cmd/gocyclo@v0.6.0 \
 && go install github.com/client9/misspell/cmd/misspell@v0.3.4 \
 && go install github.com/gordonklaus/ineffassign@v0.2.0 \
 && go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 \
 && go install golang.org/x/vuln/cmd/govulncheck@v1.6.0 \
 && go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 \
 && go install github.com/zricethezav/gitleaks/v8@v8.30.1 \
 && go install github.com/google/osv-scanner/v2/cmd/osv-scanner@v2.5.1 \
 && go install golang.org/x/tools/cmd/deadcode@v0.52.0 \
 && go clean -cache -modcache -testcache

# All tools land in /go/bin which is already on $PATH.
WORKDIR /repo

# Default to a bash so callers can `docker run ... bash -c '...'`
ENTRYPOINT ["/bin/bash"]
