# Running slurm_exporter in a container

This page covers running `slurm_exporter` as a Docker container, including the
constraints inherent to Slurm itself (MUNGE authentication, client/server
version compatibility) that you don't have when running the binary directly.

## Two image variants

The project publishes two images. Pick the one that matches your cluster:

| Variant | Tag | Bundled slurm-client | When to use it |
|---|---|---|---|
| **Standard** | `:vX.Y.Z` / `:latest` | Yes, Slurm 23.11.x (Ubuntu 24.04 repos) | Cluster running Slurm 22.x — 24.x packaged from your distro. Just works. |
| **Minimal** | `:vX.Y.Z-minimal` / `:latest-minimal` | No | Cluster running a Slurm version outside the 22–24 window, OR Slurm built from source / OHPC with custom plugins. You mount the cluster's Slurm install into the container. |

Both variants ship `munge` (daemon + library) and run as a non-root user:
- **Standard** runs as `slurmexporter` (uid `9341`, gid `munge`).
- **Minimal** runs as `nonroot` (uid `65532`, the standard distroless user).

The host's `munge.socket.2` is typically created with mode `0777`, so any
unprivileged uid in the container can use it. If your cluster runs munged
with a stricter umask, mount the socket through with explicit perms or
adjust the container user.

## TL;DR — Standard variant

```bash
docker run -d --name slurm_exporter \
  -p 9341:9341 \
  -v /etc/slurm:/etc/slurm:ro \
  -v /var/run/munge:/var/run/munge:ro \
  -v /etc/munge/munge.key:/etc/munge/munge.key:ro \
  ghcr.io/sckyzo/slurm_exporter:latest

curl -s http://localhost:9341/metrics | head
```

## TL;DR — Minimal variant (BYO Slurm install)

```bash
docker run -d --name slurm_exporter \
  -p 9341:9341 \
  -v /opt/slurm:/opt/slurm:ro \
  -v /etc/slurm:/etc/slurm:ro \
  -v /var/run/munge:/var/run/munge:ro \
  -v /etc/munge/munge.key:/etc/munge/munge.key:ro \
  -e LD_LIBRARY_PATH=/opt/slurm/lib \
  ghcr.io/sckyzo/slurm_exporter:latest-minimal \
  --slurm.bin-path=/opt/slurm/bin
```

Adjust `/opt/slurm` to wherever your cluster's Slurm prefix lives. The image
runtime is Ubuntu 24.04 with glibc 2.39, which is forward-compatible with
binaries built against RHEL 8 (glibc 2.28), RHEL 9 / Rocky 9 (glibc 2.34),
and Debian 12 (glibc 2.36). If your build environment is more recent than
that, you'll need to rebuild the runtime stage from a matching base.

If that works, you're done. If it doesn't, read on.

## What the container needs from the host

The exporter shells out to the Slurm CLI tools (`sinfo`, `squeue`, `sdiag`,
`scontrol`, `sshare`, `sacct`) to collect metrics. Those tools live inside the
image, but they need three things from the cluster:

| Mount | Why |
|---|---|
| `/etc/slurm/slurm.conf` | Tells the Slurm CLI where to find slurmctld and how the cluster is configured. |
| `/var/run/munge/munge.socket.2` | Local socket of the MUNGE daemon that signs every Slurm RPC. The container talks to the host's munged through it. |
| `/etc/munge/munge.key` | The cluster-wide MUNGE shared key. Read by the CLI tools when contacting munged. |

If you skip any of these, the CLI tools fail with `slurm_load_partitions:
Unable to contact slurm controller` (slurm.conf missing) or `Could not
connect to munge socket` (munge socket missing).

The image ships `libmunge.so.2` so the Slurm CLI tools (either bundled in
the standard variant or host-mounted in the minimal variant) find their
munge dependency at runtime. The `munged` daemon itself is **not** in the
image — both variants are designed to use the host's munged through the
mounted socket.

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

### Path overrides (env vars)

The compose paths default to a standard Linux Slurm install. If your
distribution puts things elsewhere, override them at run time or in a
`.env` file next to the compose:

| Variable | Default | Override when… |
|---|---|---|
| `SLURM_CONF_DIR` | `/etc/slurm` | older Debian/Ubuntu use `/etc/slurm-llnl`, source-installed or OHPC clusters often use `/opt/slurm/etc` |
| `MUNGE_RUN_DIR` | `/var/run/munge` | some systemd-based distros use `/run/munge` |
| `MUNGE_KEY` | `/etc/munge/munge.key` | non-default install location or a key staged separately |
| `IMAGE` | `ghcr.io/sckyzo/slurm_exporter:latest` | testing a locally-built tag |
| `HOST_PORT` | `9341` | host port already taken by another exporter |

Example for an older Debian cluster running munged via `/run/munge`:

```bash
SLURM_CONF_DIR=/etc/slurm-llnl \
MUNGE_RUN_DIR=/run/munge \
  docker compose -f docker/docker-compose.yml up -d
```

### Running a locally-built image

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
  and `munge.key` as a `Secret`, plus a `munged` sidecar container (separate
  image, since our image doesn't ship the daemon) that exposes its socket
  via an `emptyDir` volume shared with the exporter container. More portable
  but more moving parts.

A Helm chart is on the roadmap.

## Image freshness & retention

Container images go stale: a base image like `ubuntu:24.04` receives security
patches continuously (glibc, openssl, etc.), and an image we build today
freezes those packages at their current version. To stay safe, the project
publishes refreshed images on the following cadence:

| Event | What happens | Tag(s) updated |
|---|---|---|
| Release tag pushed (`vX.Y.Z`) | Full build + publish via GoReleaser | `:vX.Y.Z`, `:vX.Y`, `:vX`, `:latest` (and `-minimal` counterparts) |
| Weekly cron (Monday, 04:00 UTC) | Rebuild of the two latest stable lines with the up-to-date base image | `:vX.Y.Z` re-pushed (new digest, same tag), plus a dated immutable tag `:vX.Y.Z-YYYYMMDD` |

The dated `-YYYYMMDD` tags are **immutable** — once published, their digest
never changes. Use them in GitOps when you need bit-for-bit determinism. The
unsuffixed `:vX.Y.Z` tag is a **moving alias** to the latest build of that
version: pull it when you want the freshest security patches applied to a
specific exporter version.

### Retention policy

Tags accumulate over time. The project cleans them up monthly so the registry
stays readable:

| Tag class | Example | Retention |
|---|---|---|
| Semver release | `:v1.8.3`, `:v1.8.3-minimal` | Kept forever |
| Floating aliases | `:latest`, `:latest-minimal`, `:v1.8`, `:v1` | Kept forever (always overwritten) |
| Dated immutable | `:v1.8.3-20260516` | Last 10 per version kept; older ones pruned |

That gives roughly two and a half months of rollback per version with weekly
rebuilds — enough for forensic investigation, short of being a long-term
archive. If you need to pin a build older than that, mirror the digest into
your own registry.

### Verifying what you're running

Every published image carries OCI annotations that let you inspect what it
contains and when it was built:

```bash
docker inspect ghcr.io/sckyzo/slurm_exporter:v1.8.3 \
  --format '{{json .Config.Labels}}' | jq
```

Look for `org.opencontainers.image.created` (build timestamp),
`org.opencontainers.image.revision` (git commit), and
`org.opencontainers.image.version` (exporter version).

## Security posture

Both variants:

- run as a dedicated non-root user (standard: `slurmexporter` uid `9341`;
  minimal: `nonroot` uid `65532`)
- expose a read-only filesystem with `tmpfs:/tmp` in the example compose
- drop all Linux capabilities, no `privileged`, no new privileges

The **minimal** variant runs on `gcr.io/distroless/cc-debian12:nonroot` — no
shell, no package manager, no userland beyond what the dynamic loader and
libstdc++ need. Smallest attack surface possible while still supporting
dynamically-linked Slurm binaries mounted from the host.

The **standard** variant runs on Ubuntu 24.04. The slurm-client install
pulls in standard utilities (bash, coreutils, etc.) — necessary to make
the bundled `sinfo` / `squeue` etc. usable, but a larger surface than
distroless. Pick the minimal variant when threat-modeling matters more
than the convenience of bundled binaries.

Lock down bind mounts to read-only and keep the `cap_drop` /
`no-new-privileges` settings from the example compose regardless of variant.

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
