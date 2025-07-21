# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2025-07-21

This release marks a major milestone, signifying a stable and feature-rich version of the Slurm Exporter. It includes a complete overhaul of the CI/CD pipeline, numerous new collectors, significant refactoring for better maintainability, and several important bug fixes.

### ✨ Features

- **New Collectors:**
  - `reservations`: Collects metrics about Slurm reservations.
  - `fairshare`: Gathers fairshare usage metrics.
  - `users`: Provides metrics on a per-user basis.
  - `accounts`: Adds metrics for Slurm accounts.
  - `slurm_info`: Exposes general information about the Slurm version.
  - `node`: Provides detailed per-node metrics including CPU and memory usage.
- **Collector Configuration:** Collectors can now be individually enabled or disabled via command-line flags (e.g., `--collector.reservations=false`).
- **Improved GPU Metrics:** GPU data collection is more robust and supports modern Slurm versions (`>=19.05`).
- **CPU Metrics:** Added metrics for pending CPUs per user and per account.
- **Enhanced Build Info:** Version details (commit, branch, build date) are now injected into the binary at build time.

### 🐛 Bug Fixes

- **GPU Parsing:** Fixed a regex issue for parsing GPU information when no specific GPU type is used.
- **Node Name Parsing:** Corrected an issue where long node names were truncated.
- **CI/CD:** Resolved multiple issues in the GoReleaser and GitHub Actions configurations to ensure reliable builds and releases.

### ♻️ Refactoring

- **Code Structure:** All collectors have been moved into a dedicated `collector` package for better organization.
- **Command Execution:** Centralized the execution of Slurm commands within the collectors, adding a configurable timeout for better resilience.
- **License Headers:** Consolidated and standardized license headers across the codebase.

### ⚙️ CI/CD

- **Major Overhaul:** The entire release process has been modernized. It now uses the latest versions of `goreleaser` and `golangci-lint`, and the GitHub Actions workflows have been simplified and made more reliable.

- **Snapshot Builds:** The CI/CD pipeline can now produce development "snapshot" builds for testing purposes.
- **Packaging:** Removed unsupported packaging formats (RPM, Snap) to focus on robust binary releases.

---

## [0.30]

### ✨ Features

- **New Metrics:**
  - `slurm_node_status`: Added a new metric to expose the status of each node individually.
  - `slurm_binary_info`: Added metrics exposing the version of the Slurm binaries.
- **Go Version:** Updated the project to use Go 1.20.

### ♻️ Refactoring

- Replaced the deprecated `io/ioutil` package with `io`.

### ⚙️ CI/CD

- Added a dedicated GitHub Actions workflow for releases.
- Updated Go version used in CI to 1.20.

---

## [0.21]

### ✨ Features

- **TLS & Basic Auth:** Added support for TLS and Basic Authentication via the Prometheus Exporter Toolkit.
- **GPU Metrics:** Updated GPU collection logic to be compatible with Slurm versions `19.05.0rc1` and newer by using the `GresUsed` format option.

### ⚙️ Build

- **CGO Disabled:** Builds are now produced with `CGO_ENABLED=0` for better portability.
- **Dependencies:** Updated Go module dependencies.
