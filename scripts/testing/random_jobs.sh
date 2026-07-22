#!/bin/bash
# Emulate cluster activity by submitting random Slurm jobs.
# Useful for testing dashboards and the exporter.
#
# Prerequisites: users alice, bob, carol, dave, eve, frank must exist as OS users
# and Slurm accounts (hpc_team, ml_group, physics, bio) must be configured.
#
# Usage (on slurmctld host): ./scripts/testing/random_jobs.sh [nb_jobs]
# Usage (via docker):        docker exec slurmctld bash /path/to/random_jobs.sh [nb_jobs]
# Or simply: make -C scripts/testing workload N=30

NB=${1:-30}
USERS=(alice bob carol dave eve frank)
PARTITIONS=(cpu debug high)
ACCOUNTS=(hpc_team ml_group physics bio)
DURATIONS=(1800 3600 7200 14400)
CPU_COUNTS=(1 2 4 8)
MEM_PER_CPU=2048

# Slurm accepts a job whose time limit exceeds its partition's cap, then leaves
# it PENDING with reason PartitionTimeLimit for as long as the cluster lives.
# Read each partition's MaxTime once and clamp against it, so that changing a
# partition definition in cluster.conf cannot strand the workload again (#146).

# "UNLIMITED" or "[days-]hours:minutes:seconds" -> minutes, 0 meaning no cap.
to_minutes() {
    local raw="$1" days=0 h m s
    case "$raw" in
        UNLIMITED|INFINITE|"") echo 0; return ;;
        *-*) days=${raw%%-*}; raw=${raw#*-} ;;
    esac
    IFS=: read -r h m s <<< "$raw"
    if [ -z "$s" ]; then
        echo "  ! unexpected MaxTime format '$1' — not clamping this partition" >&2
        echo 0; return
    fi
    echo $((10#$days * 1440 + 10#$h * 60 + 10#$m))
}

declare -A PART_MAX_MINUTES
for p in "${PARTITIONS[@]}"; do
    PART_MAX_MINUTES[$p]=$(to_minutes \
        "$(scontrol show partition "$p" 2>/dev/null | tr ' ' '\n' | sed -n 's/^MaxTime=//p')")
done

echo "Submitting $NB random jobs..."
count=0
clamped=0
for i in $(seq 1 $NB); do
    USER=${USERS[$RANDOM % ${#USERS[@]}]}
    ACCOUNT=${ACCOUNTS[$RANDOM % ${#ACCOUNTS[@]}]}
    PARTITION=${PARTITIONS[$RANDOM % ${#PARTITIONS[@]}]}
    DURATION=${DURATIONS[$RANDOM % ${#DURATIONS[@]}]}
    NCPUS=${CPU_COUNTS[$RANDOM % ${#CPU_COUNTS[@]}]}
    MEM=$(($NCPUS * $MEM_PER_CPU))
    JOBNAME="rand-$(date +%s%N 2>/dev/null | tail -c5 || date +%s | tail -c5)"

    MINUTES=$(($DURATION / 60))
    MAX_MINUTES=${PART_MAX_MINUTES[$PARTITION]:-0}
    if [ "$MAX_MINUTES" -gt 0 ] && [ "$MINUTES" -gt "$MAX_MINUTES" ]; then
        MINUTES=$MAX_MINUTES
        DURATION=$(($MINUTES * 60))
        clamped=$(($clamped + 1))
    fi

    if sbatch \
        --account="$ACCOUNT" \
        --partition="$PARTITION" \
        --ntasks=1 \
        --cpus-per-task="$NCPUS" \
        --mem="${MEM}M" \
        --time="$MINUTES" \
        --job-name="$JOBNAME" \
        --wrap="sleep $DURATION" \
        --uid="$USER" 2>/dev/null; then
        count=$(($count + 1))
        echo -n "."
    else
        echo -n "x"
    fi
done

echo ""
echo "Submitted: $count / $NB jobs"
[ "$clamped" -gt 0 ] && echo "Clamped:   $clamped job(s) to their partition's MaxTime"
echo ""
squeue --format="%.6i %.9P %.14j %.8u %.2t %.4C %R" | head -50
