# Slurm Commands Used in slurm_exporter

This file documents all the Slurm shell commands executed by the `slurm_exporter` application to collect metrics. The commands are grouped by the collector that executes them.

## `collector/accounts.go`

- `squeue -a -r -h -o %A|%a|%T|%C`: Retrieves job and CPU count information, aggregated by account.

## `collector/cpus.go`

- `sinfo -h -o %C`: Retrieves the state of CPUs (allocated/idle/other/total) for the entire cluster.

## `collector/fairshare.go`

- `sshare -n -P -o account,fairshare`: Retrieves fair share information by account.

## `collector/gpus.go`

- `sinfo -a -h --Format=Nodes:,GresUsed: --state=allocated`: Retrieves allocated GPUs.
- `sinfo -a -h --Format=Nodes:,Gres:,GresUsed: --state=idle,allocated`: Retrieves idle and allocated GPUs to calculate the idle count.
- `sinfo -a -h --Format=Nodes:,Gres:`: Retrieves the total number of GPUs.

## `collector/node.go`

- `sinfo -h -N -O NodeList,AllocMem,Memory,CPUsState,StateLong,Partition`: Retrieves detailed information for each node, including memory usage, CPU state, and partition.

## `collector/nodes.go`

- `sinfo -h -o %D|%T -p <partition>`: Retrieves the number of nodes by state for a given partition.
- `scontrol show nodes -o`: Retrieves detailed information for all nodes to get a total count.
- `sinfo -h -o %R`: Retrieves the list of all unique partitions.

## `collector/partitions.go`

- `sinfo -h -o %R,%C`: Retrieves the CPU state (alloc/idle/other/total) for each partition.
- `squeue -a -r -h -o %P --states=PENDING`: Retrieves the list of pending jobs to count them per partition.

## `collector/queue.go`

- `squeue -h -o %P,%T,%C,%r,%u`: Retrieves detailed information about jobs in the queue (partition, state, cores, reason, user).

## `collector/reservations.go`

- `scontrol show reservation`: Retrieves detailed information about all active reservations.

## `collector/scheduler.go`

- `sdiag`: Retrieves statistics from the Slurm scheduler (`slurmctld`).

## `collector/slurm_binary_info.go`

- `sinfo --version`: Checks the version of `sinfo`.
- `squeue --version`: Checks the version of `squeue`.
- `sdiag --version`: Checks the version of `sdiag`.
- `scontrol --version`: Checks the version of `scontrol`.
- `sacct --version`: Checks the version of `sacct`.
- `sbatch --version`: Checks the version of `sbatch`.
- `salloc --version`: Checks the version of `salloc`.
- `srun --version`: Checks the version of `srun`.

## `collector/users.go`

- `squeue -a -r -h -o %A|%u|%T|%C`: Retrieves job and CPU count information, aggregated by user.
