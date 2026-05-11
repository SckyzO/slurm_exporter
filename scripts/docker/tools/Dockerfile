# Image with every CI / report tool baked in.
#
# Used by `make check` and `make report` so contributors can run them
# without installing Go, golangci-lint, gocyclo, etc. on their host —
# only Docker is required.
#
# Built locally on first use via `make tools-image`; rebuilds are
# incremental thanks to the layer cache.

FROM golang:1.26-alpine

RUN apk add --no-cache git bash curl ca-certificates findutils

# Static-analysis tools used by `make report` (the same set goreportcard.com
# runs server-side) and by `make check` (lint + vet + test).
RUN go install github.com/fzipp/gocyclo/cmd/gocyclo@latest \
 && go install github.com/client9/misspell/cmd/misspell@latest \
 && go install github.com/gordonklaus/ineffassign@latest \
 && go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest \
 && go clean -cache -modcache -testcache

# All tools land in /go/bin which is already on $PATH.
WORKDIR /repo

# Default to a bash so callers can `docker run ... bash -c '...'`
ENTRYPOINT ["/bin/bash"]
