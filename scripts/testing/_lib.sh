#!/usr/bin/env bash
# =============================================================================
# _lib.sh — Internal helper library for the test cluster Makefile.
# Reads cluster.conf (and optionally cluster.local.conf), provides functions
# called by each Makefile target.
#
# DO NOT call directly. Use: make <target>
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# ── Load configuration ────────────────────────────────────────────────────────
source "$SCRIPT_DIR/cluster.conf"
[ -f "$SCRIPT_DIR/cluster.local.conf" ] && source "$SCRIPT_DIR/cluster.local.conf"

# ── Auto-detect CLUSTER_DIR ───────────────────────────────────────────────────
if [ -z "${CLUSTER_DIR:-}" ]; then
    for candidate in \
        "$REPO_ROOT/../../../orchestration-hpc/slurm-docker-cluster" \
        "$HOME/slurm-docker-cluster" \
        "$HOME/dev/slurm-docker-cluster" \
        "$HOME/projects/slurm-docker-cluster"; do
        if [ -f "$candidate/docker-compose.yml" ]; then
            CLUSTER_DIR="$(realpath "$candidate")"
            break
        fi
    done
fi

# ── Colors ────────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
info()    { echo -e "${CYAN}  $*${NC}"; }
ok()      { echo -e "${GREEN}  ✓ $*${NC}"; }
warn()    { echo -e "${YELLOW}  ⚠ $*${NC}"; }
die()     { echo -e "${RED}  ✗ $*${NC}" >&2; exit 1; }
step()    { echo -e "${CYAN}[$1]${NC} $2"; }

# ── Helpers ───────────────────────────────────────────────────────────────────
slurm() { docker exec slurmctld "$@" 2>/dev/null; }

get_user_names() {
    echo "$USERS" | tr ' ' '\n' | grep -v '^$' | cut -d: -f1
}
get_user_uid()     { echo "$USERS"   | tr ' ' '\n' | grep "^$1:" | cut -d: -f2; }
get_user_account() { echo "$USERS"   | tr ' ' '\n' | grep "^$1:" | cut -d: -f3; }
get_account_desc() { echo "$ACCOUNTS"| tr ' ' '\n' | grep "^$1:" | cut -d: -f2; }
get_account_org()  { echo "$ACCOUNTS"| tr ' ' '\n' | grep "^$1:" | cut -d: -f3; }
get_account_names(){ echo "$ACCOUNTS"| tr ' ' '\n' | grep -v '^$' | cut -d: -f1; }

# ── check-deps ────────────────────────────────────────────────────────────────
cmd_check_deps() {
    command -v docker  >/dev/null 2>&1 || die "docker not found"
    command -v curl    >/dev/null 2>&1 || die "curl not found"
    command -v python3 >/dev/null 2>&1 || die "python3 not found"
    if [ -z "${CLUSTER_DIR:-}" ] || [ ! -f "${CLUSTER_DIR}/docker-compose.yml" ]; then
        echo ""
        die "slurm-docker-cluster not found.

  Clone it:
    git clone https://github.com/giovtorres/slurm-docker-cluster.git ~/slurm-docker-cluster

  Or set CLUSTER_DIR in scripts/testing/cluster.local.conf:
    echo 'CLUSTER_DIR=/path/to/slurm-docker-cluster' > scripts/testing/cluster.local.conf"
    fi
    ok "Dependencies OK  (cluster: $CLUSTER_DIR)"
}

# ── pull-image ────────────────────────────────────────────────────────────────
cmd_pull_image() {
    step "1/9" "Pulling Docker image giovtorres/slurm-docker-cluster:${SLURM_VERSION}..."
    docker pull "giovtorres/slurm-docker-cluster:${SLURM_VERSION}" 2>&1 | grep -E 'Pull|Status|already' || true
    docker tag "giovtorres/slurm-docker-cluster:${SLURM_VERSION}" "slurm-docker-cluster:${SLURM_VERSION}" 2>/dev/null || true
    ok "Image ready"
}

# ── write-env ─────────────────────────────────────────────────────────────────
cmd_write_env() {
    step "2/9" "Writing .env (${NODES} nodes, Slurm ${SLURM_VERSION})..."
    cat > "$CLUSTER_DIR/.env" << ENVEOF
COMPOSE_PROJECT_NAME=slurm
SLURM_VERSION=${SLURM_VERSION}
CPU_WORKER_COUNT=${NODES}
MYSQL_USER=slurm
MYSQL_PASSWORD=password
MYSQL_DATABASE=slurm_acct_db
SSH_ENABLE=false
ENVEOF
    ok ".env written"
}

# ── start-cluster ─────────────────────────────────────────────────────────────
cmd_start_cluster() {
    step "3/9" "Starting cluster..."
    cd "$CLUSTER_DIR"
    docker compose up -d --scale "cpu-worker=${NODES}"
    ok "Containers started"
}

# ── wait-ready ────────────────────────────────────────────────────────────────
cmd_wait_ready() {
    step "4/9" "Waiting for slurmctld and node registration..."
    for i in $(seq 1 40); do
        docker exec slurmctld scontrol ping >/dev/null 2>&1 && break
        [ "$i" -eq 40 ] && die "Timeout: slurmctld not ready"
        printf "."; sleep 3
    done; echo ""
    for i in $(seq 1 40); do
        REG=$(docker exec slurmctld sinfo --noheader --format="%D" 2>/dev/null \
              | awk '{s+=$1} END{print s+0}')
        [ "$REG" -ge "$NODES" ] && ok "$REG/$NODES nodes registered" && return 0
        [ "$i" -eq 40 ] && warn "Only $REG/$NODES nodes after 120s — continuing" && return 0
        printf "."; sleep 3
    done
}

# ── start-monitoring ──────────────────────────────────────────────────────────
cmd_start_monitoring() {
    step "5/9" "Starting Prometheus + Grafana..."

    # Copy our monitoring files into the cluster directory if not already there
    if [ -d "$SCRIPT_DIR/monitoring" ]; then
        cp "$SCRIPT_DIR/monitoring/docker-compose.monitoring.yml" "$CLUSTER_DIR/docker-compose.monitoring.yml"
        cp "$SCRIPT_DIR/monitoring/prometheus.yml"                "$CLUSTER_DIR/monitoring/prometheus.yml" 2>/dev/null ||         { mkdir -p "$CLUSTER_DIR/monitoring"; cp "$SCRIPT_DIR/monitoring/prometheus.yml" "$CLUSTER_DIR/monitoring/prometheus.yml"; }
        mkdir -p "$CLUSTER_DIR/monitoring/grafana/provisioning/datasources"                  "$CLUSTER_DIR/monitoring/grafana/provisioning/dashboards"
        cp "$SCRIPT_DIR/monitoring/grafana/provisioning/datasources/prometheus.yml"            "$CLUSTER_DIR/monitoring/grafana/provisioning/datasources/prometheus.yml"
        cp "$SCRIPT_DIR/monitoring/grafana/provisioning/dashboards/dashboards.yml"            "$CLUSTER_DIR/monitoring/grafana/provisioning/dashboards/dashboards.yml"
    fi

    if [ -f "$CLUSTER_DIR/docker-compose.monitoring.yml" ]; then
        cd "$CLUSTER_DIR"
        docker compose -f docker-compose.monitoring.yml up -d
        ok "Monitoring stack started (Prometheus + Grafana)"
    else
        warn "docker-compose.monitoring.yml not found — skipping monitoring"
    fi
}

# ── wait-grafana ──────────────────────────────────────────────────────────────
cmd_wait_grafana() {
    docker ps --format '{{.Names}}' | grep -q grafana || return 0
    for i in $(seq 1 20); do
        curl -s "${GRAFANA_URL}/api/health" 2>/dev/null | grep -q '"database": "ok"' && \
            ok "Grafana ready" && return 0
        [ "$i" -eq 20 ] && warn "Grafana not ready after 60s" && return 0
        printf "."; sleep 3
    done
}

# ── setup-accounting ──────────────────────────────────────────────────────────
cmd_setup_accounting() {
    step "6/9" "Setting up Slurm accounts and users..."

    # Create accounts
    for account in $(get_account_names); do
        desc=$(get_account_desc "$account")
        org=$(get_account_org "$account")
        docker exec slurmctld bash -c \
            "sacctmgr -i show account $account >/dev/null 2>&1 || \
             sacctmgr -i add account $account description='$desc' organization=$org" 2>/dev/null || true
    done

    # Create users
    for user in $(get_user_names); do
        acct=$(get_user_account "$user")
        docker exec slurmctld bash -c \
            "sacctmgr -i show user $user >/dev/null 2>&1 || \
             sacctmgr -i add user $user account=$acct" 2>/dev/null || true
    done

    local accounts_str=$(get_account_names | tr '\n' ' ')
    local users_str=$(get_user_names | tr '\n' ' ')
    ok "Accounts: $accounts_str"
    ok "Users:    $users_str"
}

# ── setup-os-users ────────────────────────────────────────────────────────────
cmd_setup_os_users() {
    step "7/9" "Creating OS users in all containers..."

    _create_in_container() {
        local container="$1"
        for user in $(get_user_names); do
            local uid=$(get_user_uid "$user")
            docker exec "$container" bash -c \
                "useradd -m -s /bin/bash -u $uid $user 2>/dev/null || true" 2>/dev/null || true
        done
    }

    _create_in_container slurmctld &

    for worker in $(docker ps --format '{{.Names}}' | grep slurm-cpu-worker || true); do
        _create_in_container "$worker" &
    done
    wait

    ok "OS users created in slurmctld + all workers"
}

# ── setup-partitions ──────────────────────────────────────────────────────────
cmd_setup_partitions() {
    step "8/9" "Creating extra partitions..."

    for entry in $PARTITIONS; do
        name=$(echo "$entry"     | cut -d: -f1)
        nodes=$(echo "$entry"    | cut -d: -f2)
        maxtime=$(echo "$entry"  | cut -d: -f3)
        priority=$(echo "$entry" | cut -d: -f4)
        default=$(echo "$entry"  | cut -d: -f5)
        time_str=$([ "$maxtime" = "0" ] && echo "INFINITE" || echo "$maxtime")

        docker exec slurmctld bash -c \
            "scontrol show partition $name >/dev/null 2>&1 || \
             scontrol create PartitionName=$name Nodes=$nodes MaxTime=$time_str \
                 Priority=$priority Default=$default State=UP" 2>/dev/null || true
    done

    local partitions=$(docker exec slurmctld sinfo --format='%P' --noheader 2>/dev/null | sort | tr '\n' ' ')
    ok "Partitions: $partitions"
}

# ── build-exporter ────────────────────────────────────────────────────────────
cmd_build_exporter() {
    step "9/9" "Building slurm_exporter..."
    make -C "$REPO_ROOT" build 2>&1 | grep -E 'Building|error:|warning:' || true
    [ -f "$REPO_ROOT/bin/slurm_exporter" ] || die "Binary not found after build"
    ok "Binary: $REPO_ROOT/bin/slurm_exporter"
}

# ── deploy-exporter ───────────────────────────────────────────────────────────
cmd_deploy_exporter() {
    info "Deploying exporter to slurmctld..."
    docker cp "$REPO_ROOT/bin/slurm_exporter" slurmctld:/usr/local/bin/slurm_exporter
    docker exec slurmctld chmod +x /usr/local/bin/slurm_exporter
    docker exec slurmctld bash -c "killall slurm_exporter 2>/dev/null || true"
    sleep 1
    docker exec -d slurmctld /usr/local/bin/slurm_exporter \
        --web.listen-address=:9341 --log.level=info --command.timeout=10s
    sleep 2
    docker exec slurmctld curl -s http://localhost:9341/healthz 2>/dev/null | grep -q "ok" && \
        ok "Exporter running on slurmctld:9341" || \
        warn "Exporter health check failed"
}

# ── import-dashboards ─────────────────────────────────────────────────────────
cmd_import_dashboards() {
    docker ps --format '{{.Names}}' | grep -q grafana || {
        warn "Grafana not running — skipping dashboard import"
        return 0
    }
    info "Importing Grafana dashboards..."
    local ok_count=0 fail_count=0
    local creds
    creds=$(echo -n "${GRAFANA_USER}:${GRAFANA_PASS}" | base64)

    for f in "$REPO_ROOT/dashboards_grafana/"*.json; do
        [ -f "$f" ] || continue
        result=$(python3 - "$f" "$GRAFANA_URL" "$creds" << 'PY'
import json, sys, urllib.request, urllib.error
f, url, creds = sys.argv[1], sys.argv[2] + "/api/dashboards/db", sys.argv[3]
d = json.load(open(f))
payload = json.dumps({"dashboard": d, "overwrite": True, "folderId": 0}).encode()
req = urllib.request.Request(url, data=payload,
    headers={"Content-Type": "application/json", "Authorization": "Basic " + creds})
try:
    r = json.loads(urllib.request.urlopen(req).read())
    print(r.get("status", "error"))
except urllib.error.URLError as e:
    print("error: " + str(e))
PY
        )
        [ "$result" = "success" ] && ok_count=$((ok_count+1)) || fail_count=$((fail_count+1))
    done
    ok "$ok_count dashboards imported, $fail_count failed"
}



# ── node-fail ─────────────────────────────────────────────────────────────────
cmd_node_fail() {
    info "Setting 1-2 random nodes to down/drain..."
    docker exec -i slurmctld bash <<'ENDBASH' 2>/dev/null
NODES=$(sinfo --noheader --format="%n %T" | awk '$2~/idle|mixed/{print $1}' | shuf 2>/dev/null | head -2)
if [ -z "$NODES" ]; then echo "  No idle/mixed nodes available"; exit 0; fi
I=0
for node in $NODES; do
    if [ $((I%2)) -eq 0 ]; then
        scontrol update NodeName=$node State=DRAIN Reason=test-drain && echo "  -> $node: drain"
    else
        scontrol update NodeName=$node State=DOWN Reason=test-down && echo "  -> $node: down"
    fi
    I=$((I+1))
done
ENDBASH
}

# ── node-restore ──────────────────────────────────────────────────────────────
cmd_node_restore() {
    info "Restoring all degraded nodes..."
    docker exec -i slurmctld bash <<'ENDBASH' 2>/dev/null
sinfo --noheader --format="%n %T" | awk '$2~/down|drain/{print $1}' \
| while read n; do
    scontrol update NodeName=$n State=RESUME && echo "  -> $n: resumed"
done
echo "  Done"
ENDBASH
}

# ── workload-gpu ──────────────────────────────────────────────────────────────
cmd_workload_gpu() {
    GPU_NODES=$(docker exec slurmctld sinfo --format="%G" --noheader 2>/dev/null | grep -c 'gpu:' || echo 0)
    if [ "$GPU_NODES" -eq 0 ]; then
        warn "No GPU GRES configured. Add GresTypes=gpu to slurm.conf and configure gres.conf."
        return 0
    fi
    info "Submitting GPU jobs..."
    for user in $(get_user_names | head -6); do
        acct=$(get_user_account "$user")
        docker exec slurmctld sbatch --uid="$user" --account="$acct"             --partition=cpu --ntasks=1 --cpus-per-task=4 --mem=8192M             --time=120 --gres=gpu:1 --job-name="gpu-$user"             --output=/dev/null --wrap="sleep 7200" 2>/dev/null && printf "." || printf "x"
    done
    echo " GPU jobs submitted"
}


# ── stop ──────────────────────────────────────────────────────────────────────
cmd_stop() {
    info "Stopping cluster (data preserved)..."
    docker exec slurmctld bash -c "killall slurm_exporter 2>/dev/null || true" 2>/dev/null || true
    if [ -f "$CLUSTER_DIR/docker-compose.monitoring.yml" ]; then
        cd "$CLUSTER_DIR" && docker compose -f docker-compose.monitoring.yml down 2>/dev/null || true
    fi
    cd "$CLUSTER_DIR" && docker compose down
    ok "Cluster stopped"
}

# ── clean ──────────────────────────────────────────────────────────────────────
cmd_clean() {
    warn "Removing all containers and volumes (irreversible)..."
    docker exec slurmctld bash -c "killall slurm_exporter 2>/dev/null || true" 2>/dev/null || true
    if [ -f "$CLUSTER_DIR/docker-compose.monitoring.yml" ]; then
        cd "$CLUSTER_DIR" && docker compose -f docker-compose.monitoring.yml down -v 2>/dev/null || true
    fi
    cd "$CLUSTER_DIR" && docker compose down -v
    ok "Full clean complete"
}

# ── start (just cluster + monitoring + exporter) ───────────────────────────────
cmd_start() {
    cmd_check_deps
    cd "$CLUSTER_DIR" && docker compose up -d --scale "cpu-worker=${NODES}"
    cmd_wait_ready
    cmd_start_monitoring
    cmd_deploy_exporter
    ok "Cluster started"
}

# ── logs ──────────────────────────────────────────────────────────────────────
cmd_logs() {
    cd "$CLUSTER_DIR" && docker compose logs -f slurmctld slurmdbd
}

# ── Dispatch ──────────────────────────────────────────────────────────────────
CMD="${1:-help}"
case "$CMD" in
    check-deps)         cmd_check_deps ;;
    pull-image)         cmd_pull_image ;;
    write-env)          cmd_write_env ;;
    start-cluster)      cmd_start_cluster ;;
    wait-ready)         cmd_wait_ready ;;
    start-monitoring)   cmd_start_monitoring ;;
    wait-grafana)       cmd_wait_grafana ;;
    setup-accounting)   cmd_setup_accounting ;;
    setup-os-users)     cmd_setup_os_users ;;
    setup-partitions)   cmd_setup_partitions ;;
    build-exporter)     cmd_build_exporter ;;
    deploy-exporter)    cmd_deploy_exporter ;;
    import-dashboards)  cmd_import_dashboards ;;
    show-config)
        echo "CLUSTER_DIR    = $CLUSTER_DIR"
        echo "SLURM_VERSION  = $SLURM_VERSION"
        echo "NODES          = $NODES"
        echo "GRAFANA_URL    = $GRAFANA_URL"
        echo "ACCOUNTS       = $(get_account_names | tr '\n' ' ')"
        echo "USERS          = $(get_user_names | tr '\n' ' ')"
        echo "PARTITIONS     = $PARTITIONS"
        echo "PLAYWRIGHT     = $PLAYWRIGHT_VERSION"
        ;;
    node-fail)          cmd_node_fail ;;
    node-restore)       cmd_node_restore ;;
    workload-gpu)       cmd_workload_gpu ;;
    stop)               cmd_stop ;;
    clean)              cmd_clean ;;
    start)              cmd_start ;;
    logs)               cmd_logs ;;
    *) echo "Unknown command: $CMD"; exit 1 ;;
esac
