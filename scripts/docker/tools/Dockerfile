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

FROM golang:1.26.10-alpine@sha256:a2012b4a1d1f9f7e9f44e0d39b805a901e856b1dd80b4e6ee4338db547050308

# shellcheck  → used by actionlint to lint `run:` shell blocks
# zizmor      → static analysis (security) for GitHub Actions workflows
# build-base  → C toolchain (gcc): `go test -race` requires cgo
RUN apk add --no-cache git bash curl ca-certificates findutils \
        shellcheck zizmor build-base

# Static-analysis tools used by `make report` (the same set goreportcard.com
# runs server-side), by `make check` (lint + vet + test + vuln + actionlint),
# and by `make vuln` / `make actionlint`.
RUN go install github.com/fzipp/gocyclo/cmd/gocyclo@latest \
 && go install github.com/client9/misspell/cmd/misspell@latest \
 && go install github.com/gordonklaus/ineffassign@latest \
 && go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest \
 && go install golang.org/x/vuln/cmd/govulncheck@latest \
 && go install github.com/rhysd/actionlint/cmd/actionlint@latest \
 && go install github.com/zricethezav/gitleaks/v8@latest \
 && go install github.com/google/osv-scanner/v2/cmd/osv-scanner@latest \
 && go install golang.org/x/tools/cmd/deadcode@latest \
 && go clean -cache -modcache -testcache

# All tools land in /go/bin which is already on $PATH.
WORKDIR /repo

# Default to a bash so callers can `docker run ... bash -c '...'`
ENTRYPOINT ["/bin/bash"]
