Prometheus exporter for the [Slurm workload manager](https://slurm.schedmd.com/),
shipped as two purpose-built container images.

[![Release](https://img.shields.io/github/v/release/SckyzO/slurm_exporter?label=release)](https://github.com/SckyzO/slurm_exporter/releases)
[![Build](https://img.shields.io/github/actions/workflow/status/SckyzO/slurm_exporter/release.yml?label=build)](https://github.com/SckyzO/slurm_exporter/actions/workflows/release.yml)
[![Pulls](https://img.shields.io/docker/pulls/sckyzo/slurm-exporter)](https://hub.docker.com/r/sckyzo/slurm-exporter)
[![Image size — standard](https://img.shields.io/docker/image-size/sckyzo/slurm-exporter/latest?label=size%20%28standard%29)](https://hub.docker.com/r/sckyzo/slurm-exporter/tags)
[![Image size — minimal](https://img.shields.io/docker/image-size/sckyzo/slurm-exporter/latest-minimal?label=size%20%28minimal%29)](https://hub.docker.com/r/sckyzo/slurm-exporter/tags)
[![License](https://img.shields.io/github/license/SckyzO/slurm_exporter)](https://github.com/SckyzO/slurm_exporter/blob/master/LICENSE)

## Tags & variants

Two image flavors, both published as **multi-arch manifests** (linux/amd64 +
linux/arm64) to **two registries**:

| Use case | Tag pattern | Base | When |
|---|---|---|---|
| **Standard** | `:vX.Y.Z`, `:X.Y`, `:X`, `:latest` | Ubuntu 26.04 + slurm-client 25.11 | Cluster runs Slurm 23.x — 26.x packaged from a distro. Just works. |
| **Minimal** | `:vX.Y.Z-minimal`, `:X.Y-minimal`, `:X-minimal`, `:latest-minimal` | distroless/cc-debian12 + libmunge | Slurm built from source / OHPC / outside the 23-26 window. Mount your own slurm-client via `--slurm.bin-path`. |

Available at:

- `docker.io/sckyzo/slurm-exporter`
- `ghcr.io/sckyzo/slurm_exporter` (mirror)

Pre-release tags (`vX.Y.Z-rc1` etc.) push only the pinned version and never
overwrite the floating tags.

## Quick start

```bash
docker run -d --name slurm_exporter \
  -p 9341:9341 \
  -v /etc/slurm:/etc/slurm:ro \
  -v /var/run/munge:/var/run/munge:ro \
  -v /etc/munge/munge.key:/etc/munge/munge.key:ro \
  sckyzo/slurm-exporter:latest

curl -s http://localhost:9341/metrics | head
```

If that returns metrics, you're done. If it doesn't, read on.

## What this image does

The exporter shells out to the Slurm CLI tools (`sinfo`, `squeue`, `sdiag`,
`scontrol`, `sshare`, `sacct`) and exposes their output as Prometheus
metrics on `/metrics`. It needs three things from the host to talk to
slurmctld:

| Mount | Why |
|---|---|
| `/etc/slurm/slurm.conf` | Tells the Slurm CLI where to find slurmctld. |
| `/var/run/munge/munge.socket.2` | Local socket of the MUNGE daemon that signs every Slurm RPC. |
| `/etc/munge/munge.key` | Cluster-wide MUNGE shared key. |

Without any one of them, the CLI fails with either
`slurm_load_partitions: Unable to contact slurm controller` (slurm.conf
missing) or `Could not connect to munge socket` (socket missing).

The image ships `libmunge.so.2` so host-mounted binaries find their munge
dependency at runtime; the `munged` daemon itself is **not** in the image
— both variants use the host's munged through the mounted socket.

## Standard vs minimal — which one ?

Pick **standard** (`:latest`) if your cluster runs Slurm 23.x → 26.x from a
distro package. The image bundles slurm-client matching that range and
just works out of the box.

Pick **minimal** (`:latest-minimal`) if any of the following apply:

- Your cluster runs a Slurm version outside the 23-26 window.
- Slurm is built from source / OHPC with custom plugins (PMIx, custom
  job_submit, SPANK hooks…).
- You need the smallest possible attack surface (distroless: no shell, no
  package manager, no userland beyond the dynamic loader and libstdc++).

The minimal variant needs the host's Slurm install mounted in. Quick
example:

```bash
docker run -d --name slurm_exporter \
  -p 9341:9341 \
  -v /opt/slurm:/opt/slurm:ro \
  -v /etc/slurm:/etc/slurm:ro \
  -v /var/run/munge:/var/run/munge:ro \
  -v /etc/munge/munge.key:/etc/munge/munge.key:ro \
  -e LD_LIBRARY_PATH=/opt/slurm/lib \
  sckyzo/slurm-exporter:latest-minimal \
  --slurm.bin-path=/opt/slurm/bin
```

The runtime is Ubuntu 26.04 with glibc 2.43 (standard) or distroless
cc-debian12 with the same glibc family (minimal) — both forward-compatible
with binaries built against RHEL 8 (glibc 2.28), RHEL 9 / Rocky 9 (2.34),
Debian 12 (2.36), and Debian 13 (2.41).

## Compose

A complete compose file ships in the repo under
[`docker/docker-compose.yml`](https://github.com/SckyzO/slurm_exporter/blob/master/docker/docker-compose.yml).
Compact form:

```yaml
services:
  slurm_exporter:
    image: sckyzo/slurm-exporter:latest
    container_name: slurm_exporter
    restart: unless-stopped
    ports:
      - "9341:9341"
    volumes:
      - /etc/slurm:/etc/slurm:ro
      - /var/run/munge:/var/run/munge:ro
      - /etc/munge/munge.key:/etc/munge/munge.key:ro
    security_opt:
      - no-new-privileges:true
    cap_drop: [ALL]
    read_only: true
    tmpfs: [/tmp]
```

### Path overrides

The compose paths default to a standard Linux Slurm install. Override at
run-time or in a `.env` next to the compose:

| Variable | Default | Override when… |
|---|---|---|
| `SLURM_CONF_DIR` | `/etc/slurm` | older Debian/Ubuntu use `/etc/slurm-llnl`, source-installed often use `/opt/slurm/etc` |
| `MUNGE_RUN_DIR` | `/var/run/munge` | some systemd-based distros use `/run/munge` |
| `MUNGE_KEY` | `/etc/munge/munge.key` | non-default install location |
| `IMAGE` | `ghcr.io/sckyzo/slurm_exporter:latest` | testing a locally-built tag |
| `HOST_PORT` | `9341` | host port already taken |

## Prometheus scrape config

```yaml
scrape_configs:
  - job_name: slurm
    static_configs:
      - targets: ["slurm_exporter:9341"]
    scrape_interval: 30s
```

## Supply chain

Every published artifact ships with three signals consumers can verify:

### Image signatures (cosign / Sigstore keyless)

Every manifest is signed by the GitHub Actions workflow that built it,
attested by the runner's OIDC token. No keys on either side.

```bash
cosign verify sckyzo/slurm-exporter:latest \
  --certificate-identity-regexp 'https://github.com/SckyzO/slurm_exporter/.github/workflows/release.yml@.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

The release checksums file is signed the same way (`*.pem` + `*.sig`
alongside `slurm_exporter_checksums.txt` on the GitHub release).

### Software Bill of Materials

Each release archive ships with a **CycloneDX SBOM** (`*.sbom.json`)
listing every Go module compiled into the binary plus their versions and
PURLs. Suitable for Dependency-Track, Anchore Enterprise, and similar.

```bash
gh release download v1.8.3 -p '*sbom.json'
jq '.components[] | {name, version, purl}' slurm_exporter-1.8.3-linux-amd64.tar.gz.sbom.json
```

### Vulnerability scanning

Both images are scanned with [Trivy](https://github.com/aquasecurity/trivy)
on every PR that touches the Dockerfiles or Go dependencies, and weekly
against the published images. The PR scan blocks the merge on HIGH/CRITICAL
CVEs that have a fix upstream. Workflow:
[`.github/workflows/trivy-scan.yml`](https://github.com/SckyzO/slurm_exporter/blob/master/.github/workflows/trivy-scan.yml).

## Security posture

Both variants:

- run as a dedicated non-root user (standard: `slurmexporter` uid `9341`,
  gid `munge` ; minimal: `nonroot` uid `65532`)
- expose a read-only filesystem with `tmpfs:/tmp` in the example compose
- drop all Linux capabilities, no `privileged`, no new privileges

The minimal variant runs on `gcr.io/distroless/cc-debian12:nonroot` — no
shell, no package manager. The standard variant runs on Ubuntu 26.04 with
the slurm-client utilities available — convenient but a larger surface.
Pick minimal if threat-modeling matters more than the convenience of
bundled binaries.

## Image freshness & retention

| Event | What happens |
|---|---|
| Release tag pushed (`vX.Y.Z`) | Full GoReleaser run: build, push to both registries, sign, generate SBOM, attach to GitHub release. |
| Weekly cron (Monday 04:00 UTC) | Two latest stable lines rebuilt against the up-to-date base image. Same tag is re-pushed with a fresh digest, plus a dated immutable tag `:vX.Y.Z-YYYYMMDD`. |
| Monthly cleanup | Dated immutable tags older than the last ten per version are pruned. Semver tags and floating aliases are kept forever. |

Pin `:vX.Y.Z-YYYYMMDD` for bit-for-bit GitOps determinism. Use the
unsuffixed `:vX.Y.Z` (or `:latest`) when you want the freshest base-image
patches applied to that version.

## Verifying what you're running

OCI labels carry the build metadata:

```bash
docker inspect sckyzo/slurm-exporter:latest \
  --format '{{json .Config.Labels}}' | jq
```

Look for `org.opencontainers.image.created` (build timestamp),
`org.opencontainers.image.revision` (git commit), and
`org.opencontainers.image.version`.

## Deployment scenarios

The repo's [`docker/README.md`](https://github.com/SckyzO/slurm_exporter/blob/master/docker/README.md)
covers the three patterns in detail:

- **Scenario A** — Exporter on the slurmctld host itself (simplest, host
  networking, just mount the three paths above).
- **Scenario B** — Remote monitoring node with its own munged + a copy of
  the cluster's munge.key.
- **Scenario C** — Kubernetes (DaemonSet on slurmctld nodes, or Deployment
  with ConfigMap/Secret + a munged sidecar from a separate image).

A Helm chart is on the roadmap.

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| `Could not connect to munge socket` | `/var/run/munge` not mounted, or host munged isn't running. |
| `slurm_load_partitions: Unable to contact slurm controller` | slurm.conf missing/wrong, or no network route to slurmctld on `:6817`. |
| `error: Munge encode failed: Invalid credential` | `munge.key` in the container doesn't match the cluster's. |
| `Unable to register: Zero Bytes were transmitted or received` | Slurm version mismatch — try the variant that matches your cluster. |
| `/metrics` returns 200 but most metrics are empty | CLI tools work but slurmctld rejects some calls. Run with `--log.level=debug`. |

For everything else, enable debug logging (`--log.level=debug`) and look
at the `Failed to get <X> data` lines — they include the underlying Slurm
error.

## Links

- **Source code & full documentation**: [github.com/SckyzO/slurm_exporter](https://github.com/SckyzO/slurm_exporter)
- **Report a bug / request a feature**: [issue tracker](https://github.com/SckyzO/slurm_exporter/issues)
- **Release notes**: [CHANGELOG.md](https://github.com/SckyzO/slurm_exporter/blob/master/CHANGELOG.md)
- **GHCR mirror**: [ghcr.io/sckyzo/slurm_exporter](https://github.com/users/SckyzO/packages/container/package/slurm_exporter)

## License

GPL-3.0 — see [LICENSE](https://github.com/SckyzO/slurm_exporter/blob/master/LICENSE).
