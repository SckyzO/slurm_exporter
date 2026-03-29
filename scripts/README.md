# Scripts

| Script | Description |
|--------|-------------|
| `random_jobs.sh [N]` | Submit N random Slurm jobs across multiple users, accounts and partitions. Must be run inside `slurmctld` (or via `docker exec slurmctld bash random_jobs.sh`). |
| `take_screenshots.sh [dir]` | Take screenshots of all 8 Grafana dashboards via Playwright in Docker. Requires Grafana on `localhost:3000`. |

## Test cluster

See [`testing/`](testing/) for the full test cluster setup using
[giovtorres/slurm-docker-cluster](https://github.com/giovtorres/slurm-docker-cluster).

```bash
cd scripts/testing
make setup     # one-shot: cluster + exporter + monitoring + dashboards
make workload  # submit test jobs
make status    # check cluster state
```
