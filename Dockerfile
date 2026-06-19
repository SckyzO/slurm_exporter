# syntax=docker/dockerfile:1.7
#
# slurm_exporter — Prometheus exporter for the Slurm workload manager.
#
# Single-stage runtime image. The slurm_exporter binary is expected to be
# present in the build context next to this Dockerfile. That's how GoReleaser
# drives Docker builds: it cross-compiles the binary first, then writes it to
# a temporary build context alongside the Dockerfile before running
# `docker build`. The Makefile's docker-build target follows the same
# convention (see `make docker-build`).
#
# The runtime needs three things from the host to talk to slurmctld:
#   - /etc/slurm/slurm.conf            (slurmctld endpoint and cluster config)
#   - /var/run/munge/munge.socket.2    (authentication daemon socket)
#   - /etc/munge/munge.key             (cluster-wide MUNGE shared key)
#
# See docker/README.md for compose examples and troubleshooting.

FROM ubuntu:26.04@sha256:e153663f92c94118ff22a5dc397b59b351ffd695480566debb5850e017e5937a

# slurm-client provides sinfo/squeue/sdiag/scontrol/sshare/sacct.
# Ubuntu 26.04 ships Slurm 25.11.x, compatible with slurmctld 23.x → 26.x in
# practice. For clusters running a much older or much newer slurmctld,
# rebuild this stage from a base that matches your version (see
# docker/README.md).
RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        slurm-client \
        munge \
        ca-certificates \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/* \
    && rm -f /usr/bin/pebble
# Ubuntu 26.04 ships an unmanaged /usr/bin/pebble (Canonical's init system)
# that we never use. It still embeds an older Go stdlib that Trivy flags as
# HIGH CVEs. Removing it shrinks the image by ~10 MB and clears the noise.

# Dedicated unprivileged user, member of the munge group so the container
# can write into the munge socket inherited from the host.
RUN useradd --system --no-create-home --shell /usr/sbin/nologin \
        --uid 9341 --gid munge slurmexporter

COPY slurm_exporter /usr/local/bin/slurm_exporter

USER slurmexporter

EXPOSE 9341

ARG VERSION=dev
ARG COMMIT=
ARG BUILD_DATE=

LABEL org.opencontainers.image.source="https://github.com/SckyzO/slurm_exporter" \
      org.opencontainers.image.title="slurm_exporter" \
      org.opencontainers.image.description="Prometheus exporter for the Slurm workload manager" \
      org.opencontainers.image.licenses="GPL-3.0-only" \
      org.opencontainers.image.documentation="https://github.com/SckyzO/slurm_exporter/blob/master/docker/README.md" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}"

ENTRYPOINT ["/usr/local/bin/slurm_exporter"]
CMD ["--web.listen-address=:9341"]
