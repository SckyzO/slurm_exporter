# syntax=docker/dockerfile:1.7
#
# slurm_exporter — Prometheus exporter for the Slurm workload manager.
#
# Two stages:
#   1. builder    Go 1.26-alpine, produces a static slurm_exporter binary.
#   2. runtime    Ubuntu 24.04, ships the binary plus the Slurm CLI tools
#                 (sinfo, squeue, sdiag, scontrol, sshare, sacct) that the
#                 exporter shells out to.
#
# The runtime image needs three things from the host to talk to slurmctld:
#   - /etc/slurm/slurm.conf            (slurmctld endpoint and cluster config)
#   - /var/run/munge/munge.socket.2    (authentication daemon socket)
#   - /etc/munge/munge.key             (cluster-wide MUNGE shared key)
#
# See docker/README.md for compose examples and the troubleshooting guide.

FROM golang:1.26-alpine AS builder

WORKDIR /src

# Cache module downloads in a separate layer.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=
ARG BRANCH=
ARG BUILD_USER=docker
ARG BUILD_DATE=

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w \
        -X github.com/prometheus/common/version.Version=${VERSION} \
        -X github.com/prometheus/common/version.Revision=${COMMIT} \
        -X github.com/prometheus/common/version.Branch=${BRANCH} \
        -X github.com/prometheus/common/version.BuildUser=${BUILD_USER} \
        -X github.com/prometheus/common/version.BuildDate=${BUILD_DATE}" \
    -o /out/slurm_exporter ./cmd/slurm_exporter

# ---

FROM ubuntu:24.04

# slurm-client provides sinfo/squeue/sdiag/scontrol/sshare/sacct.
# Ubuntu 24.04 ships Slurm 23.11.x, compatible with slurmctld 22.x → 25.x in
# practice. For clusters running an older or much newer slurmctld, rebuild
# this stage from a base that matches your version (see docker/README.md).
RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        slurm-client \
        munge \
        ca-certificates \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Dedicated unprivileged user, member of the munge group so the container
# can write into the munge socket inherited from the host.
RUN useradd --system --no-create-home --shell /usr/sbin/nologin \
        --uid 9341 --gid munge slurmexporter

COPY --from=builder /out/slurm_exporter /usr/local/bin/slurm_exporter

USER slurmexporter

EXPOSE 9341

LABEL org.opencontainers.image.source="https://github.com/SckyzO/slurm_exporter" \
      org.opencontainers.image.description="Prometheus exporter for the Slurm workload manager" \
      org.opencontainers.image.licenses="GPL-3.0-only" \
      org.opencontainers.image.documentation="https://github.com/SckyzO/slurm_exporter/blob/master/docker/README.md"

ENTRYPOINT ["/usr/local/bin/slurm_exporter"]
CMD ["--web.listen-address=:9341"]
