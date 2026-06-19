# Project name
PROJECT_NAME = slurm_exporter

# Go environment configuration
GO_INSTALLED_VERSION := $(shell go version 2>/dev/null | awk '{print $$3}' | sed 's/go//g')
GO_VERSION ?= $(if $(GO_INSTALLED_VERSION),$(GO_INSTALLED_VERSION),1.22.2)
OS ?= linux
ARCH ?= amd64
GOPATH := $(shell pwd)/go/modules
GOBIN := bin/$(PROJECT_NAME)
GOFILES := $(shell find . -name "*.go" -type f)
GO_URL := https://dl.google.com/go/go$(GO_VERSION).$(OS)-$(ARCH).tar.gz
GOPATH_ENV := GOPATH=$(GOPATH) PATH=$(shell pwd)/go/bin:$(PATH)

# Shell command for execution
SHELL := $(shell which bash) -eu -o pipefail

# Check if the installed Go version matches the required version
VERSION ?= $(shell git describe --tags --always --dirty --abbrev=7 || echo "untagged")
REVISION ?= $(shell git rev-parse HEAD)
BRANCH ?= $(shell git rev-parse --abbrev-ref HEAD)
BUILD_USER ?= $(shell git config user.name) <$(shell git config user.email)>
BUILD_DATE ?= $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

# LDFLAGS for injecting version information
LDFLAGS = \
	-X 'github.com/prometheus/common/version.Version=$(VERSION)' \
	-X 'github.com/prometheus/common/version.Revision=$(REVISION)' \
	-X 'github.com/prometheus/common/version.Branch=$(BRANCH)' \
	-X 'github.com/prometheus/common/version.BuildUser=$(BUILD_USER)' \
	-X 'github.com/prometheus/common/version.BuildDate=$(BUILD_DATE)'


.PHONY: all
all: setup build

# Target to install Go if not already installed or the wrong version is present
.PHONY: setup
setup:
	@if [ -z "$(GO_INSTALLED_VERSION)" ]; then \
		echo "Go is not installed. Installing Go $(GO_VERSION)..."; \
		wget $(GO_URL); \
		tar -xzvf go$(GO_VERSION).$(OS)-$(ARCH).tar.gz; \
		rm -f go$(GO_VERSION).$(OS)-$(ARCH).tar.gz; \
	elif [ "$(GO_INSTALLED_VERSION)" != "$(GO_VERSION)" ]; then \
		echo "Go version $(GO_INSTALLED_VERSION) is installed. Switching to version $(GO_VERSION)..."; \
		wget $(GO_URL); \
		tar -xzvf go$(GO_VERSION).$(OS)-$(ARCH).tar.gz; \
		rm -f go$(GO_VERSION).$(OS)-$(ARCH).tar.gz; \
	else \
		echo "Go version $(GO_VERSION) is already installed."; \
	fi

# Build target to compile the binary
.PHONY: build
build: $(GOBIN)

$(GOBIN): go/modules/pkg/mod
	@echo "Building $(GOBIN)"
	mkdir -p bin
	CGO_ENABLED=0 go build -v -ldflags "$(LDFLAGS)" -o $(GOBIN) ./cmd/slurm_exporter

# Target to download Go modules
go/modules/pkg/mod: go.mod
	@echo "Downloading Go modules"
	go mod download

# ─── Containerised tooling ────────────────────────────────────────────────────
# The check / report / lint / vet / race targets below run inside a single
# self-contained image (scripts/docker/tools/) that bundles Go, golangci-lint,
# gocyclo, misspell, and ineffassign. The only host requirement is Docker —
# no Go toolchain needed.

TOOLS_IMG     := slurm_exporter-tools:latest
TOOLS_CTX     := scripts/docker/tools
IN_TOOLS      := docker run --rm -v "$(CURDIR):/repo" -w /repo $(TOOLS_IMG)

# Build the tools image if missing or if its Dockerfile changed.
.PHONY: tools-image
tools-image:
	@if ! docker image inspect $(TOOLS_IMG) >/dev/null 2>&1 || \
	   [ $(TOOLS_CTX)/Dockerfile -nt /tmp/.$(TOOLS_IMG).stamp ]; then \
	  echo "Building $(TOOLS_IMG)..."; \
	  docker build -t $(TOOLS_IMG) $(TOOLS_CTX) && touch /tmp/.$(TOOLS_IMG).stamp; \
	fi

# Test target to run all tests (in container).
.PHONY: test
test: tools-image
	@echo "Running tests (containerised)"
	@$(IN_TOOLS) -c 'go test -v ./...'

# Tests with the race detector (in container). Useful to catch concurrency bugs
# in collectors with background goroutines (e.g. sacct_efficiency).
# CGO_ENABLED=1 is required: the race detector is implemented in C. The tools
# image bundles build-base (gcc) for this.
.PHONY: race
race: tools-image
	@echo "Running tests with race detector (containerised)"
	@$(IN_TOOLS) -c 'CGO_ENABLED=1 go test -race -count=1 ./...'

# go vet (in container).
.PHONY: vet
vet: tools-image
	@echo "Running go vet (containerised)"
	@$(IN_TOOLS) -c 'go vet ./...'

# golangci-lint, same tool as CI (in container).
.PHONY: lint
lint: tools-image
	@echo "Running golangci-lint (containerised)"
	@$(IN_TOOLS) -c 'golangci-lint run ./...'

# govulncheck — Go call-graph vulnerability scanner (in container).
# Catches reachable stdlib / dependency CVEs that image scanners (Trivy) and
# module-list scanners (osv-scanner) miss. Needs network to fetch the vuln DB.
.PHONY: vuln
vuln: tools-image
	@echo "Running govulncheck (containerised)"
	@$(IN_TOOLS) -c 'govulncheck ./...'

# actionlint — GitHub Actions workflow linter (in container). Auto-discovers
# .github/workflows/ and runs shellcheck on every `run:` block (shellcheck is
# bundled in the tools image).
.PHONY: actionlint
actionlint: tools-image
	@echo "Running actionlint (containerised)"
	@$(IN_TOOLS) -c 'actionlint -color'

# zizmor — static analysis (security) for GitHub Actions (in container).
# --offline keeps it deterministic (no GitHub API calls).
.PHONY: zizmor
zizmor: tools-image
	@echo "Running zizmor (containerised)"
	@$(IN_TOOLS) -c 'zizmor --offline .'

# gitleaks — secret scanner (in container). Scans the working tree. Kept out of
# `check` (it's a prevention tool, not a build gate); run it before committing.
.PHONY: secrets
secrets: tools-image
	@echo "Running gitleaks secret scan (containerised)"
	@$(IN_TOOLS) -c 'gitleaks dir . --no-banner --redact'

# osv-scanner — dependency vulnerability scan against the OSV database (in
# container). Complements govulncheck (call graph) with the OSV feed. Needs
# network, so it's a separate target rather than part of `check`.
.PHONY: osv
osv: tools-image
	@echo "Running osv-scanner (containerised)"
	@$(IN_TOOLS) -c 'osv-scanner scan source --lockfile go.mod'

# deadcode — unreachable Go functions (reachability from main + tests). Fails if
# any dead code is found. Not yet part of `check`: there is known dead code to
# remove first; it joins `check` once the codebase is clean.
.PHONY: deadcode
deadcode: tools-image
	@echo "Running deadcode (containerised)"
	@$(IN_TOOLS) -c 'out=$$(deadcode -test ./...); if [ -n "$$out" ]; then echo "$$out"; echo "dead code found"; exit 1; fi; echo "no dead code"'

# Full pre-commit / pre-release verification — mirrors what CI runs.
.PHONY: check
check: vet lint test vuln actionlint zizmor

# Offline equivalent of the goreportcard.com checks (in container).
# Runs gofmt -s, go vet, gocyclo, ineffassign, misspell, and a LICENSE check,
# then prints a per-check score and an overall grade. Exits non-zero below B
# so CI / pre-commit hooks can gate on it.
.PHONY: report
report: tools-image
	@$(IN_TOOLS) -c '$(TOOLS_CTX)/goreport.sh'

# Reports the state of Go module dependencies in a tabular form: which direct
# deps are up to date, which indirect ones have an upgrade available, and
# whether each pending bump is patch / minor / major. Read-only; never runs
# `go get` automatically — that's left for `go get -u ./... && go mod tidy`.
.PHONY: report-deps
report-deps: tools-image
	@$(IN_TOOLS) -c '$(TOOLS_CTX)/deps-report.sh'

# Run the built binary
.PHONY: run
run: $(GOBIN)
	$(GOBIN)

# ─── Docker images (local debug) ─────────────────────────────────────────────
# Two variants:
#   - standard  (Dockerfile)         bundles slurm-client 23.11 from Ubuntu
#   - minimal   (Dockerfile.minimal) ships only the exporter, mount your own
# Release images are built and published by GoReleaser on tag push; these
# targets only exist for local iteration.

DOCKER_IMAGE         ?= slurm_exporter
DOCKER_TAG           ?= dev
DOCKER_REF           := $(DOCKER_IMAGE):$(DOCKER_TAG)
DOCKER_REF_MINIMAL   := $(DOCKER_IMAGE):$(DOCKER_TAG)-minimal

# Build args shared by both variants.
DOCKER_BUILD_ARGS = \
	--build-arg VERSION=$$(git describe --tags --dirty 2>/dev/null || echo dev) \
	--build-arg COMMIT=$$(git rev-parse HEAD) \
	--build-arg BRANCH=$$(git rev-parse --abbrev-ref HEAD) \
	--build-arg BUILD_USER="$$(git config user.email)" \
	--build-arg BUILD_DATE=$$(date -u +%Y-%m-%dT%H:%M:%SZ)

# Both image variants are single-stage and expect ./slurm_exporter to be
# present in the build context. We compile the binary first (mirrors what
# GoReleaser does for releases), copy it next to the Dockerfile, build, and
# clean up — never commit ./slurm_exporter to git (it's in .gitignore as
# the binary at repo root).
#
# The same ldflags that go into bin/slurm_exporter for `make build` go
# into ./slurm_exporter for `make docker-build`, so `docker run … --version`
# reports the right metadata. The Dockerfile's build-args populate the
# OCI labels on top — same information, different surface.

# ldflags pulled from the build target so both paths stay in sync.
DOCKER_GO_LDFLAGS = -s -w \
	-X github.com/prometheus/common/version.Version=$$(git describe --tags --dirty 2>/dev/null || echo dev) \
	-X github.com/prometheus/common/version.Revision=$$(git rev-parse HEAD) \
	-X github.com/prometheus/common/version.Branch=$$(git rev-parse --abbrev-ref HEAD) \
	-X github.com/prometheus/common/version.BuildUser=$$(git config user.email) \
	-X github.com/prometheus/common/version.BuildDate=$$(date -u +%Y-%m-%dT%H:%M:%SZ)

.PHONY: docker-build
docker-build:
	@echo "Building $(DOCKER_REF)"
	CGO_ENABLED=0 GOOS=linux go build -ldflags "$(DOCKER_GO_LDFLAGS)" -o ./slurm_exporter ./cmd/slurm_exporter
	docker build $(DOCKER_BUILD_ARGS) -t $(DOCKER_REF) .
	@rm -f ./slurm_exporter
	@echo "✓ $(DOCKER_REF)"

.PHONY: docker-build-minimal
docker-build-minimal:
	@echo "Building $(DOCKER_REF_MINIMAL)"
	CGO_ENABLED=0 GOOS=linux go build -ldflags "$(DOCKER_GO_LDFLAGS)" -o ./slurm_exporter ./cmd/slurm_exporter
	docker build $(DOCKER_BUILD_ARGS) -f Dockerfile.minimal -t $(DOCKER_REF_MINIMAL) .
	@rm -f ./slurm_exporter
	@echo "✓ $(DOCKER_REF_MINIMAL)"

.PHONY: docker-build-all
docker-build-all: docker-build docker-build-minimal

.PHONY: docker-run
docker-run:
	@echo "Starting compose stack (override IMAGE=$(DOCKER_REF))"
	IMAGE=$(DOCKER_REF) docker compose -f docker/docker-compose.yml up -d
	@echo "✓ Metrics at http://localhost:9341/metrics"

.PHONY: docker-run-minimal
docker-run-minimal:
	@echo "Starting minimal compose stack (override IMAGE=$(DOCKER_REF_MINIMAL))"
	IMAGE=$(DOCKER_REF_MINIMAL) docker compose -f docker/docker-compose.minimal.yml up -d
	@echo "✓ Metrics at http://localhost:9341/metrics"

.PHONY: docker-stop
docker-stop:
	-docker compose -f docker/docker-compose.yml down
	-docker compose -f docker/docker-compose.minimal.yml down

.PHONY: docker-clean
docker-clean:
	-docker rmi $(DOCKER_REF) $(DOCKER_REF_MINIMAL) 2>/dev/null

# Clean up the build artifacts
.PHONY: clean
clean:
	@echo "Cleaning up"
	go clean -modcache
	rm -fr bin/ go/
