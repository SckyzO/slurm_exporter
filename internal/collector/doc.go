// Package collector implements Prometheus collectors for the Slurm
// workload manager. Each collector executes one or more Slurm CLI
// commands via the Execute function and exposes the parsed output
// as Prometheus metrics.
//
// All collectors follow the same pattern:
//   - A *Data(logger) function calls Execute() to run a Slurm command
//   - A Parse*() function parses the raw bytes into a metrics struct
//   - A *Collector struct implements prometheus.Collector
//   - A New*Collector() constructor wires everything together
//
// Collectors are registered in cmd/slurm_exporter/main.go and wrapped
// in a StatusTracker that emits per-collector health metrics.
package collector
