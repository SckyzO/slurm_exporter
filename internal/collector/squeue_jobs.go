package collector

import (
	"strings"

	"github.com/sckyzo/slurm_exporter/internal/logger"
)

// squeueJobsColumns is the consolidated squeue layout that feeds the accounts,
// users and partitions collectors from a single controller query (issue #144).
// It is the union of the columns those collectors read individually. The
// trailing colon on every field forces variable-width columns; without it
// squeue caps a field at 20 characters and silently drops the tail (the reason
// AccountsData needed it on tres-alloc, and issue #10).
//
// Field order, referenced by the projections below:
//
//	0 JobID  1 Account  2 UserName  3 Partition  4 State  5 NumNodes  6 NumCPUs  7 tres-alloc
const squeueJobsColumns = "JobID:|,Account:|,UserName:|,Partition:|,State:|,NumNodes:|,NumCPUs:|,tres-alloc:"

// SqueueJobsData returns one shared squeue snapshot of the whole job queue,
// cached for the scrape. Before issue #144 the accounts, users and partitions
// collectors issued up to five separate full-queue dumps to slurmctld every
// scrape; they now all read this single snapshot.
//
// The -a -r flags and the default state set match what those collectors
// requested individually, so every metric they derive is unchanged — the
// per-collector projections below re-emit exactly the column layout each parser
// already consumes. queue.go is deliberately NOT a consumer: it omits -a/-r and
// toggles --states=all, and folding it in here would change how job arrays and
// hidden-partition jobs are counted.
func SqueueJobsData(log *logger.Logger) ([]byte, error) {
	out, err := squeueJobsCache.GetOrFetch(func() ([]byte, error) {
		return Execute(log, "squeue", []string{"-a", "-r", "-h", "-O", squeueJobsColumns})
	})
	updateCacheAge()
	return out, err
}

// squeueJobsFields splits one squeueJobsColumns line into its trimmed fields,
// or returns nil when the line is not a full data row.
func squeueJobsFields(line string) []string {
	if !strings.Contains(line, "|") {
		return nil
	}
	fields := strings.SplitN(line, "|", 8)
	if len(fields) < 8 {
		return nil
	}
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}
	return fields
}

// projectAccountsView re-emits the shared snapshot in the exact layout
// ParseAccountsMetrics consumes: "JobID|Account|State|NumNodes|NumCPUs|tres-alloc".
func projectAccountsView(data []byte) []byte {
	return projectNamedJobView(data, 1)
}

// projectUsersView re-emits the shared snapshot in the exact layout
// ParseUsersMetrics consumes: "JobID|UserName|State|NumNodes|NumCPUs|tres-alloc".
func projectUsersView(data []byte) []byte {
	return projectNamedJobView(data, 2)
}

// projectNamedJobView builds the six-column view shared by the accounts and
// users parsers, selecting either the Account (nameIdx 1) or UserName (nameIdx 2)
// column for the grouping field.
func projectNamedJobView(data []byte, nameIdx int) []byte {
	var b strings.Builder
	for line := range strings.SplitSeq(string(data), "\n") {
		f := squeueJobsFields(line)
		if f == nil {
			continue
		}
		// JobID|<name>|State|NumNodes|NumCPUs|tres-alloc
		b.WriteString(strings.Join([]string{f[0], f[nameIdx], f[4], f[5], f[6], f[7]}, "|"))
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

// projectPartitionView re-emits the Partition column for every job in the given
// state, one per line — the same shape parsePartitionJobs expects from the old
// `squeue -o "%P" --states=<state>` calls. Multi-partition jobs keep their
// comma-separated list, which squeuePartitions splits.
func projectPartitionView(data []byte, state string) []byte {
	var b strings.Builder
	for line := range strings.SplitSeq(string(data), "\n") {
		f := squeueJobsFields(line)
		if f == nil {
			continue
		}
		if f[4] != state {
			continue
		}
		b.WriteString(f[3])
		b.WriteByte('\n')
	}
	return []byte(b.String())
}
