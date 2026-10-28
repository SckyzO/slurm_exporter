# Paired capture, Slurm 25.11.2, node state flags

Thirteen commands taken at the same instant on the `scripts/testing` cluster,
with six state groups produced on purpose. It exists to pin the one thing a
single-state cluster cannot show: **`sinfo`'s display fields collapse a node's
base state and its flags into one word, and which word you get depends on the
combination.**

| nodes | `%T` prints | `StateComplete` | base |
|---|---|---|---|
| c[1-2] | `planned` | `idle+dynamic_norm+planned` | idle |
| c[9-10] | `fail` | `idle+dynamic_norm+fail` | idle |
| c[3,5,7] | `drained` | `idle+drain+dynamic_norm` | idle |
| c8 | `mixed` | `mixed+dynamic_norm` | mixed |
| c4 | `allocated` | `allocated+dynamic_norm` | allocated |
| c6 | `down` | `down+dynamic_norm` | down |

Three of the six groups carry a base state the printed word does not name, and
`dynamic_norm` is on all ten nodes while appearing nowhere in `%T`.

`filter_idle_allocated.txt` records what `--state=idle,allocated` selects on
this same instant: every node whose base state is `idle`, `allocated` or
`mixed`, flags included, and nothing whose base is `down`.

## Files

| file | command |
|---|---|
| `node_detail.txt` | `sinfo -h -N -O "NodeList: ,AllocMem: ,Memory: ,CPUsState: ,StateLong: ,Partition:"` |
| `drain_reason.txt` | `sinfo -h -N -o "%N\|%E\|%H\|%T"` |
| `scontrol_nodes.txt` | `scontrol show nodes -o` |
| `node_state.txt` | `sinfo -h -o "%N\|%T"` |
| `nodes_global.txt` | `sinfo -h -o "%R\|%D\|%T\|%b"` |
| `state_complete.txt` | `sinfo -a -h --Format="NodeList: ,StateLong: ,StateComplete: ,Nodes: ,Gres: ,GresUsed:"` |
| `gpus_total.txt` | `sinfo -a -h --Format="Nodes: ,Gres:"` |
| `gpus_allocated.txt` | `sinfo -a -h --Format="Nodes:10 ,GresUsed:" --state=allocated` |
| `gpus_idle.txt` | `sinfo -a -h --Format="Nodes: ,Gres: ,GresUsed:" --state=idle,allocated` |
| `gpus_snapshot_statelong.txt` | `sinfo -a -h --Format="Nodes: ,StateLong: ,Gres: ,GresUsed:"` |
| `gpus_snapshot_statecomplete.txt` | `sinfo -a -h --Format="Nodes: ,StateComplete: ,Gres: ,GresUsed:"` |
| `filter_idle_allocated.txt` | `sinfo -a -h --Format="StateComplete: ,NodeList: " --state=idle,allocated` |
| `VERSION` | `sinfo --version` and the capture timestamp |

## What it does not cover

- **No GRES.** This cluster has no GPUs, so every `Gres` column is `(null)` and
  the GPU arithmetic cannot be checked from here. The fake-GPU recipe
  (`slurmd -Z` with `File=` on device files created in the container) is needed
  for that.
- **No `not_responding`.** Producing the `*` suffix needs a stopped `slurmd`
  and `SlurmdTimeout` to elapse, 300 s on this cluster.
- `completing`, `reserved` and `maintenance` were not produced.

Separate from `slurm-25.11.2/`, which is a different instant captured for #254
and must not be overwritten.
