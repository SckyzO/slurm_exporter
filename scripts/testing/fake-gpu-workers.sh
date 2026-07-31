#!/usr/bin/env bash
#
# Fake GPU workers for the integration cluster.
#
# The upstream clone can start GPU workers, but only against real NVIDIA
# hardware: its entrypoint counts /dev/nvidia* and gres.conf uses AutoDetect=nvml.
# That left every GPU-shaped capture depending on a production cluster, which
# also happened to be single-model — so the shape that matters most to the
# per-node GRES metrics, several GPU models on one cluster, could not be produced
# at all.
#
# It can be. A dynamic slurmd (-Z) declares its own resources at registration,
# and Slurm treats a GRES backed by an ordinary device node exactly like one
# backed by a real card. Nothing here needs a GPU, nvidia-container-toolkit, or
# a change to the clone's tracked files.
#
# Four things have to be right, each of them learned the hard way:
#
#   1. --privileged and --cgroupns private, or slurmd dies on "Unable to
#      initialize cgroup plugin" before it ever registers.
#   2. File= is mandatory for a gpu-typed GRES. Without it slurmd logs
#      "Ignoring file-less GPU ... from final GRES list", the resource never
#      reaches the usable list, and the node lands in inval.
#   3. /etc/slurm is a volume shared with the whole cluster and carries its own
#      gres.conf (AutoDetect=nvml, Type=nvidia). The node needs a private
#      SLURM_CONF instead; editing the shared volume would reconfigure every
#      other node.
#   4. slurmctld remembers a dynamic node across restarts, so changing a node's
#      GRES means deleting it first. Cleanup below does that.
#
# Layout: homogeneous nodes are what a real site buys, mixed-model nodes are what
# breaks parsers. This makes both, so one cluster covers the common case and the
# awkward one.

# Sourced by _lib.sh before its command dispatch. Not executable on its own: it
# relies on _lib.sh for cluster.conf, the colour helpers and slurm().

GPU_NETWORK="${COMPOSE_PROJECT_NAME:-slurm}_slurm-network"
GPU_IMAGE="slurm-docker-cluster:${SLURM_VERSION}"
GPU_NAME_PREFIX="slurm-fake-gpu"

# Node layout: <count>:<gres spec>:<gres.conf lines, ; separated>
# GPUs per node is 4 throughout, so a mixed node splits its four devices two and
# two rather than pretending to hold more cards than its homogeneous neighbours.
GPU_MODEL_A="${GPU_MODEL_A:-model_a}"
GPU_MODEL_B="${GPU_MODEL_B:-model_b}"

# ── Helpers ───────────────────────────────────────────────────────────────────

# gpu_node_spec prints "gres_spec|gres_conf" for the Nth node of a 4+4+2 layout.
gpu_node_spec() {
    local n=$1
    if [ "$n" -le 4 ]; then
        printf '%s|%s' \
            "gpu:${GPU_MODEL_A}:4" \
            "Name=gpu Type=${GPU_MODEL_A} File=/dev/nvidia[0-3]"
    elif [ "$n" -le 8 ]; then
        printf '%s|%s' \
            "gpu:${GPU_MODEL_B}:4" \
            "Name=gpu Type=${GPU_MODEL_B} File=/dev/nvidia[0-3]"
    else
        printf '%s|%s' \
            "gpu:${GPU_MODEL_A}:2,gpu:${GPU_MODEL_B}:2" \
            "Name=gpu Type=${GPU_MODEL_A} File=/dev/nvidia[0-1]
Name=gpu Type=${GPU_MODEL_B} File=/dev/nvidia[2-3]"
    fi
}

gpu_container_names() {
    docker ps -a --filter "name=^${GPU_NAME_PREFIX}-" --format '{{.Names}}' 2>/dev/null || true
}

# ── Commands ──────────────────────────────────────────────────────────────────

cmd_gpu_workers_up() {
    local count="${1:-10}"

    docker network inspect "$GPU_NETWORK" >/dev/null 2>&1 ||
        die "network $GPU_NETWORK not found — start the cluster first (make setup)"
    docker image inspect "$GPU_IMAGE" >/dev/null 2>&1 ||
        die "image $GPU_IMAGE not found — run make setup first"

    step "1/3" "Removing any previous fake GPU workers..."
    cmd_gpu_workers_down >/dev/null

    step "2/3" "Starting $count fake GPU workers..."
    local i spec gres gres_conf name
    for i in $(seq 1 "$count"); do
        spec="$(gpu_node_spec "$i")"
        gres="${spec%%|*}"
        gres_conf="${spec#*|}"
        name="${GPU_NAME_PREFIX}-${i}"

        docker run -d --name "$name" --hostname "g${i}" \
            --network "$GPU_NETWORK" --privileged --cgroupns private \
            -v "${COMPOSE_PROJECT_NAME:-slurm}_etc_slurm:/etc/slurm:ro" \
            -v "${COMPOSE_PROJECT_NAME:-slurm}_etc_munge:/etc/munge" \
            -v "${COMPOSE_PROJECT_NAME:-slurm}_slurm_jobdir:/data" \
            -v "${COMPOSE_PROJECT_NAME:-slurm}_var_log_slurm:/var/log/slurm" \
            --entrypoint sh "$GPU_IMAGE" -c "
                set -e
                # Character devices under major 195, the NVIDIA major. mknod can
                # fail on some storage drivers; a plain file satisfies Slurm's
                # File= just as well, it only checks the path resolves.
                for d in 0 1 2 3; do
                    mknod /dev/nvidia\$d c 195 \$d 2>/dev/null || touch /dev/nvidia\$d
                done
                mkdir -p /tmp/sc && cp /etc/slurm/*.conf /tmp/sc/ 2>/dev/null || true
                cat > /tmp/sc/gres.conf <<'GRESEOF'
${gres_conf}
GRESEOF
                until 2>/dev/null >/dev/tcp/slurmctld/6817; do sleep 2; done
                /usr/sbin/munged --force >/dev/null 2>&1 || true
                sleep 2
                export SLURM_CONF=/tmp/sc/slurm.conf
                exec /usr/sbin/slurmd -Z -D --conf \"Feature=gpu Gres=${gres}\"
            " >/dev/null || die "failed to start $name"
    done

    step "3/3" "Waiting for registration..."
    local waited=0 registered=0
    while [ "$waited" -lt 90 ]; do
        registered=$(slurm sinfo -h -N -o "%N" 2>/dev/null | grep -c '^g[0-9]' || true)
        [ "$registered" -ge "$count" ] && break
        sleep 3
        waited=$((waited + 3))
    done

    # A node that ever registered without usable GRES stays drained with
    # "gres/gpu count reported lower than configured". Resuming is safe here:
    # these nodes are disposable and were just created.
    for i in $(seq 1 "$count"); do
        slurm scontrol update "NodeName=g${i}" State=RESUME >/dev/null 2>&1 || true
    done
    sleep 3

    if [ "$registered" -lt "$count" ]; then
        warn "only $registered/$count GPU nodes registered — check: docker logs ${GPU_NAME_PREFIX}-1"
    else
        ok "$count fake GPU nodes registered (${GPU_MODEL_A} / ${GPU_MODEL_B})"
    fi
    slurm sinfo -h -N -o "%N %T %G" | grep '^g[0-9]' || true
}

cmd_gpu_workers_down() {
    local names
    names="$(gpu_container_names)"
    if [ -n "$names" ]; then
        # shellcheck disable=SC2086
        docker rm -f $names >/dev/null 2>&1 || true
    fi
    # Delete the dynamic node records too: slurmctld keeps them, and a stale
    # record with the old GRES would shadow a node relaunched with a new layout
    # — a ghost node that still reports GPUs nothing is running on.
    #
    # The list is discovered rather than assumed. A fixed range only cleans up
    # what the current layout would have created, so shrinking the cluster left
    # the extra nodes behind, still advertising their GRES.
    local node
    for node in $(slurm sinfo -h -N -o "%N" 2>/dev/null | grep -E '^g[0-9]+$' | sort -u); do
        slurm scontrol delete "NodeName=${node}" >/dev/null 2>&1 || true
    done
    ok "fake GPU workers removed"
}
