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
.PHONY: race
race: tools-image
	@echo "Running tests with race detector (containerised)"
	@$(IN_TOOLS) -c 'go test -race -count=1 ./...'

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

# Full pre-commit / pre-release verification — mirrors what CI runs.
.PHONY: check
check: vet lint test

# Offline equivalent of the goreportcard.com checks (in container).
# Runs gofmt -s, go vet, gocyclo, ineffassign, misspell, and a LICENSE check,
# then prints a per-check score and an overall grade. Exits non-zero below B
# so CI / pre-commit hooks can gate on it.
.PHONY: report
report: tools-image
	@$(IN_TOOLS) -c '$(TOOLS_CTX)/goreport.sh'

# Run the built binary
.PHONY: run
run: $(GOBIN)
	$(GOBIN)

# Clean up the build artifacts
.PHONY: clean
clean:
	@echo "Cleaning up"
	go clean -modcache
	rm -fr bin/ go/
