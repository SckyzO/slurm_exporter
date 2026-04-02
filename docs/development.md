# Development Guide

> Back to [README](../README.md)

## 🛠️ Development

This project requires access to a node with the Slurm CLI (`sinfo`, `squeue`, `sdiag`, etc.).

### Prerequisites

- [Go](https://golang.org/dl/) (version 1.25 or higher, toolchain 1.26.1 recommended)
- Slurm CLI tools available in your `$PATH`

### Building from Source

1. Clone this repository:
   ```bash
   git clone https://github.com/sckyzo/slurm_exporter.git
   cd slurm_exporter
   ```

2. Build the exporter binary:
   ```bash
   make build
   ```

   The binary will be available in `bin/slurm_exporter`.

### Running Tests

To run all tests:

```bash
make test
```

### Development Commands

**Run the linter:**
```bash
golangci-lint run ./...
```

**Clean build artifacts:**
```bash
make clean
```

**Run the exporter locally:**
```bash
bin/slurm_exporter --web.listen-address=:8080
```

**Query metrics:**
```bash
curl http://localhost:8080/metrics
```

**Liveness probe:**
```bash
curl http://localhost:8080/healthz
# returns: ok
```

**Advanced build options:**
You can override the Go version and architecture via environment variables:
```bash
make build GO_VERSION=1.22.2 OS=linux ARCH=amd64
```

---


## 🧪 Test Cluster

A complete local Slurm test cluster can be set up for integration testing.
See [`scripts/testing/README.md`](../scripts/testing/README.md) for full instructions.

```bash
cd scripts/testing
make setup    # Start cluster, deploy exporter + dashboards
make workload  # Submit test jobs
make stop      # Tear down
```
