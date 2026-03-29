#!/bin/bash
# Emulate cluster activity by submitting random Slurm jobs.
# Useful for testing dashboards and the exporter.
#
# Prerequisites: users alice, bob, carol, dave, eve, frank must exist as OS users
# and Slurm accounts (hpc_team, ml_group, physics, bio) must be configured.
#
# Usage (on slurmctld host): ./scripts/random_jobs.sh [nb_jobs]
# Usage (via docker):        docker exec slurmctld bash /path/to/random_jobs.sh [nb_jobs]

NB=${1:-30}
USERS=(alice bob carol dave eve frank)
PARTITIONS=(cpu debug high)
ACCOUNTS=(hpc_team ml_group physics bio)
DURATIONS=(1800 3600 7200 14400)
CPU_COUNTS=(1 2 4 8)
MEM_PER_CPU=2048

echo "Submitting $NB random jobs..."
count=0
for i in $(seq 1 $NB); do
    USER=${USERS[$RANDOM % ${#USERS[@]}]}
    ACCOUNT=${ACCOUNTS[$RANDOM % ${#ACCOUNTS[@]}]}
    PARTITION=${PARTITIONS[$RANDOM % ${#PARTITIONS[@]}]}
    DURATION=${DURATIONS[$RANDOM % ${#DURATIONS[@]}]}
    NCPUS=${CPU_COUNTS[$RANDOM % ${#CPU_COUNTS[@]}]}
    MEM=$(($NCPUS * $MEM_PER_CPU))
    JOBNAME="rand-$(date +%s%N 2>/dev/null | tail -c5 || date +%s | tail -c5)"

    if sbatch \
        --account="$ACCOUNT" \
        --partition="$PARTITION" \
        --ntasks=1 \
        --cpus-per-task="$NCPUS" \
        --mem="${MEM}M" \
        --time="$(($DURATION/60))" \
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
echo ""
squeue --format="%.6i %.9P %.14j %.8u %.2t %.4C %R" | head -50
