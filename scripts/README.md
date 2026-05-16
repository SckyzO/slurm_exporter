# Scripts

Project utilities, grouped by purpose.

```
scripts/
├── dashboards/         Grafana dashboard tooling (normalize, screenshots)
├── docker/tools/       Build context for the slurm_exporter-tools image
│                       used by make check / report / report-deps
└── testing/            Local test cluster (slurm-docker-cluster) + monitoring
```

## `dashboards/`

Idempotent transformations applied to the JSON files under
[`monitoring/grafana/dashboards/`](../monitoring/grafana/dashboards/) and the
screenshot helper. See [`dashboards/README.md`](dashboards/README.md) for the
full per-script doc.

| Script | Purpose |
|---|---|
| `add_drilldown_links.py` | Add the "Slurm Dashboards" dropdown to a dashboard's top bar. |
| `add_export_format.py`   | Add the `__inputs` / `__elements` / `__requires` sections that grafana.com's validator requires on upload. |
| `take_screenshots.sh`    | Grab a PNG of each dashboard via Playwright in a container. |

## `docker/tools/`

Container image used by the `make check`, `make report`, and `make report-deps`
targets in the root Makefile. Holds `golangci-lint`, `gocyclo`, `ineffassign`,
`misspell`, and the two reporting scripts (`goreport.sh`, `deps-report.sh`).
Built lazily by `make tools-image` — no host toolchain required for those
make targets, just Docker.

## `testing/`

Full local test cluster setup using
[giovtorres/slurm-docker-cluster](https://github.com/giovtorres/slurm-docker-cluster),
with Prometheus + Grafana wired up and the exporter deployed. See
[`testing/README.md`](testing/README.md) for the complete walkthrough.

```bash
cd scripts/testing
make setup     # one-shot: cluster + exporter + monitoring + dashboards
make workload  # submit test jobs
make status    # check cluster state
```

The `random_jobs.sh` job submission helper lives inside `testing/` because
it's only consumed by `make -C scripts/testing workload`.
