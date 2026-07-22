# =============================================================================
# Makefile — Slurm Exporter Test Environment
#
# Manages a local Slurm cluster for testing the Prometheus Slurm Exporter.
# Configuration is in cluster.conf (override locally with cluster.local.conf).
#
# Prerequisites:
#   - Docker + Docker Compose v2
#   - A clone of https://github.com/giovtorres/slurm-docker-cluster
#   - Go toolchain (for make build)
#
# Quickstart:
#   git clone https://github.com/giovtorres/slurm-docker-cluster.git ~/slurm-docker-cluster
#   make setup
# =============================================================================

THIS_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
LIB      := bash $(THIS_DIR)_lib.sh

# ── Aliases so $(LIB) check-deps can be combined with other targets ───────────
.PHONY: help setup start stop clean status workload workload-gpu \
        cancel-all node-fail node-restore redeploy redeploy-dashboards \
        screenshots logs exporter-logs show-config check-deps

.DEFAULT_GOAL := help

help:
	@echo ""
	@echo "  Slurm Exporter — Test Environment"
	@echo "  ===================================="
	@echo ""
	@echo "  First time:"
	@echo "    make setup             Full setup: cluster + exporter + monitoring + dashboards"
	@echo ""
	@echo "  Daily use:"
	@echo "    make start             Start an already-configured cluster"
	@echo "    make stop              Stop cluster (data preserved)"
	@echo "    make status            Show nodes, jobs, exporter health"
	@echo "    make exporter-logs     Print the exporter log from the container"
	@echo "    make workload [N=30]   Submit random jobs"
	@echo "    make screenshots       Take Grafana dashboard screenshots"
	@echo ""
	@echo "  Simulation:"
	@echo "    make node-fail         Set 1-2 random nodes to down/drain"
	@echo "    make node-restore      Restore all degraded nodes"
	@echo "    make workload-gpu      Submit GPU jobs (requires GRES)"
	@echo "    make cancel-all        Cancel all running/pending jobs"
	@echo ""
	@echo "  Maintenance:"
	@echo "    make clean             Full teardown (removes volumes)"
	@echo "    make redeploy          Rebuild exporter + reimport dashboards"
	@echo "    make show-config       Print effective configuration"
	@echo "    make logs              Follow cluster logs"
	@echo ""
	@echo "  Configuration: scripts/testing/cluster.conf"
	@echo "  Override:      scripts/testing/cluster.local.conf (gitignored)"
	@echo ""

# ── Full setup ────────────────────────────────────────────────────────────────
setup:
	@$(LIB) check-deps
	@echo ""
	@$(LIB) pull-image
	@$(LIB) write-env
	@$(LIB) start-cluster
	@$(LIB) wait-ready
	@$(LIB) start-monitoring
	@$(LIB) wait-grafana
	@$(LIB) setup-accounting
	@$(LIB) setup-os-users
	@$(LIB) setup-partitions
	@$(LIB) build-exporter
	@$(LIB) deploy-exporter
	@$(LIB) import-dashboards
	@echo ""
	@echo "  ✓ Done. Run: make workload && make screenshots"
	@echo ""

# ── Start / Stop ──────────────────────────────────────────────────────────────
start:
	@$(LIB) start


stop:
	@$(LIB) stop
	@echo "  ✓ Cluster stopped (data preserved)"

clean:
	@$(LIB) clean

# ── Status ────────────────────────────────────────────────────────────────────
status:
	@echo "=== Nodes ==="
	@docker exec slurmctld sinfo 2>/dev/null || echo "  (not running)"
	@echo ""
	@echo "=== Jobs (top 20) ==="
	@docker exec slurmctld squeue --format="%.6i %.9P %.16j %.8u %.2t %.4C" 2>/dev/null | head -22 || true
	@echo ""
	@echo "=== Exporter ==="
	@docker exec slurmctld curl -s --max-time 5 http://localhost:9341/healthz 2>/dev/null || echo "  not running"
	@echo ""
	@bash -c 'source $(THIS_DIR)cluster.conf; \
	    [ -f "$(THIS_DIR)cluster.local.conf" ] && source "$(THIS_DIR)cluster.local.conf" || true; \
	    echo "  Grafana    → $${GRAFANA_URL}"; \
	    echo "  Prometheus → http://localhost:9090"'

exporter-logs:
	@$(LIB) exporter-logs

# ── Workload ──────────────────────────────────────────────────────────────────
N ?= 30

workload:
	@docker cp $(THIS_DIR)random_jobs.sh slurmctld:/tmp/random_jobs.sh
	@docker exec slurmctld bash /tmp/random_jobs.sh $(N)

workload-gpu:
	@$(LIB) workload-gpu


# scancel --state takes one value: passing it twice cancelled RUNNING only and
# left the pending jobs in place, silently (#167).
cancel-all:
	@docker exec slurmctld bash -c 'for s in PENDING RUNNING; do scancel --state=$$s 2>/dev/null || true; done'
	@echo "  ✓ All jobs cancelled"

# ── Node simulation ───────────────────────────────────────────────────────────
node-fail:
	@$(LIB) node-fail


node-restore:
	@$(LIB) node-restore


# ── Redeploy ──────────────────────────────────────────────────────────────────
redeploy:
	@$(LIB) build-exporter
	@$(LIB) deploy-exporter
	@$(LIB) import-dashboards

redeploy-dashboards:
	@$(LIB) import-dashboards

# ── Screenshots ───────────────────────────────────────────────────────────────
OUTPUT ?= /tmp/screenshots

screenshots:
	@bash -c 'source $(THIS_DIR)cluster.conf; \
	    [ -f "$(THIS_DIR)cluster.local.conf" ] && source "$(THIS_DIR)cluster.local.conf" || true; \
	    PLAYWRIGHT_IMAGE=$${PLAYWRIGHT_IMAGE} PLAYWRIGHT_VERSION=$${PLAYWRIGHT_VERSION} \
	    bash $(THIS_DIR)../dashboards/take_screenshots.sh $(OUTPUT)'

# ── Misc ──────────────────────────────────────────────────────────────────────
show-config:
	@$(LIB) show-config

logs:
	@$(LIB) logs

check-deps:
	@$(LIB) check-deps
