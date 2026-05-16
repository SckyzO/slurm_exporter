# Running slurm_exporter in a container

This page covers running `slurm_exporter` as a Docker container, including the
constraints inherent to Slurm itself (MUNGE authentication, client/server
version compatibility) that you don't have when running the binary directly.

## TL;DR

```bash
docker run -d --name slurm_exporter \
  -p 9341:9341 \
  -v /etc/slurm:/etc/slurm:ro \
  -v /var/run/munge:/var/run/munge:ro \
  -v /etc/munge/munge.key:/etc/munge/munge.key:ro \
  ghcr.io/sckyzo/slurm_exporter:latest

curl -s http://localhost:9341/metrics | head
```

If that works, you're done. If it doesn't, read on.

## What the container needs from the host

The exporter shells out to the Slurm CLI tools (`sinfo`, `squeue`, `sdiag`,
`scontrol`, `sshare`, `sacct`) to collect metrics. Those tools live inside the
image, but they need three things from the cluster:

| Mount | Why |
|---|---|
| `/etc/slurm/slurm.conf` | Tells the Slurm CLI where to find slurmctld and how the cluster is configured. |
| `/var/run/munge/munge.socket.2` | Local socket of the MUNGE daemon that signs every Slurm RPC. The container talks to the host's munged through it. |
| `/etc/munge/munge.key` | The cluster-wide MUNGE shared key. Required if you run `munged` inside the container instead (advanced). |

If you skip any of these, the CLI tools fail with `slurm_load_partitions:
Unable to contact slurm controller` (slurm.conf missing) or `Could not
connect to munge socket` (munge socket missing).

## Slurm version compatibility

The image ships with **Slurm 23.11.x** (from Ubuntu 24.04). Slurm guarantees
client/server compatibility within a window of two major versions, which in
practice covers slurmctld 22.x through 25.x.

If your slurmctld is on a version outside that window — for example, an older
22.05 cluster or a very recent 25.11 deployment with new RPCs — rebuild the
runtime stage from a base image that matches your Slurm version. The cleanest
way is to clone the repo and edit the runtime `FROM` line, then `make
docker-build`.

For most production clusters running 23.x or 24.x, the published image works
out of the box.

## Compose

The compose file under `docker/docker-compose.yml` is a self-contained
example for the most common case: a node that already has a working
slurm-client + munged setup (slurmctld host, a login node, a dedicated
monitoring VM enrolled in the cluster).

```bash
docker compose -f docker/docker-compose.yml up -d
docker compose -f docker/docker-compose.yml logs -f slurm_exporter
```

To run against a locally-built image instead of the published one:

```bash
make docker-build               # builds slurm_exporter:dev
IMAGE=slurm_exporter:dev make docker-run
```

## Makefile targets

| Target | What it does |
|---|---|
| `make docker-build` | Builds the image locally as `slurm_exporter:dev`. Embeds the current `git describe` as the version. |
| `make docker-run` | Starts the compose stack (uses `IMAGE` env if set, otherwise pulls latest from GHCR). |
| `make docker-stop` | `docker compose down`. |
| `make docker-clean` | Removes the local `slurm_exporter:dev` image. |

These targets are for local iteration. The release image is built and
published automatically by GoReleaser on tag push — see the release process
doc.

## Deployment scenarios

### Scenario A — on the slurmctld host itself

Simplest case. The slurmctld machine already has slurm.conf, munged running,
and the MUNGE key in place.

```yaml
services:
  slurm_exporter:
    image: ghcr.io/sckyzo/slurm_exporter:latest
    network_mode: host          # exposes 9341 on the host
    volumes:
      - /etc/slurm:/etc/slurm:ro
      - /var/run/munge:/var/run/munge:ro
      - /etc/munge/munge.key:/etc/munge/munge.key:ro
    restart: unless-stopped
```

### Scenario B — a remote monitoring node

A node that isn't part of the cluster, but has the MUNGE key copied over and
its own munged running locally.

```yaml
services:
  slurm_exporter:
    image: ghcr.io/sckyzo/slurm_exporter:latest
    ports:
      - "9341:9341"
    volumes:
      # slurm.conf must point at the cluster's slurmctld via a hostname that
      # the container can resolve.
      - ./slurm.conf:/etc/slurm/slurm.conf:ro
      - /var/run/munge:/var/run/munge:ro
      - /etc/munge/munge.key:/etc/munge/munge.key:ro
```

### Scenario C — Kubernetes

Two viable patterns:

- **DaemonSet on slurmctld host(s)**: mount the host paths via `hostPath`
  volumes, requires the slurmctld to run on a known set of nodes.
- **Deployment with secret/configmap**: package slurm.conf as a `ConfigMap`
  and `munge.key` as a `Secret`, run an `initContainer` to launch munged.
  More portable but more moving parts.

A Helm chart is on the roadmap.

## Security posture

The published image:

- runs as a dedicated non-root user (`slurmexporter`, uid `9341`, gid
  `munge`)
- uses `read_only: true` filesystem with `tmpfs:/tmp` in the example compose
- drops all Linux capabilities, no `privileged`, no new privileges
- ships only the Slurm CLI tools (no shell beyond what slurm-client pulls in)

If you run from a less-trusted host, lock down the bind mounts to read-only
and keep the `cap_drop` / `no-new-privileges` settings from the example
compose.

## Prometheus scrape config

```yaml
scrape_configs:
  - job_name: slurm
    static_configs:
      - targets: ["slurm_exporter:9341"]
    scrape_interval: 30s
```

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| `Could not connect to munge socket` in exporter logs | The `/var/run/munge` mount is missing, or the host's munged isn't running. |
| `slurm_load_partitions: Unable to contact slurm controller` | Either slurm.conf is missing/wrong, or the container can't reach slurmctld on port 6817. |
| `error: Munge encode failed: Invalid credential` | The `munge.key` inside the container doesn't match the cluster's key. |
| `Unable to register: Zero Bytes were transmitted or received` | Slurm version mismatch between the container's slurm-client and the cluster's slurmctld. |
| Metrics endpoint returns 200 but most metrics are missing | The Slurm CLI tools work but slurmctld is rejecting some calls — check `--log.level=debug` for the actual error per collector. |

For everything else, run with `--log.level=debug` and look for the
`Failed to get <X> data` lines — they include the underlying Slurm error.
