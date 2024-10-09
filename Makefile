# Project name
PROJECT_NAME = slurm_exporter

# Go environment configuration
GO_VERSION ?= 1.20
OS ?= linux
ARCH ?= amd64
GOPATH := $(shell pwd)/go/modules
GOBIN := bin/$(PROJECT_NAME)
GOFILES := $(wildcard *.go)
GO_URL := https://dl.google.com/go/go$(GO_VERSION).$(OS)-$(ARCH).tar.gz
GOPATH_ENV := GOPATH=$(GOPATH) PATH=$(shell pwd)/go/bin:$(PATH)

# Shell command for execution
SHELL := $(shell which bash) -eu -o pipefail

# Check if the installed Go version matches the required version
GO_INSTALLED_VERSION := $(shell go version 2>/dev/null | awk '{print $$3}' | sed 's/go//g')

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

$(GOBIN): go/modules/pkg/mod $(GOFILES)
	@echo "Building $(GOBIN)"
	mkdir -p bin
	CGO_ENABLED=0 go build -v -o $(GOBIN)

# Target to download Go modules
go/modules/pkg/mod: go.mod
	@echo "Downloading Go modules"
	go mod download

# Test target to run all tests
.PHONY: test
test: go/modules/pkg/mod $(GOFILES)
	@echo "Running tests"
	go test -v

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
