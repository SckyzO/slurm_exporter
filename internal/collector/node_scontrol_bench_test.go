package collector

import (
	"fmt"
	"strings"
	"testing"
)

/*
The node parse path runs on every scrape over the largest output the exporter
reads, so its cost is worth keeping an eye on. The capture in
test_data/slurm-25.11.2/ has ten nodes; these benchmarks extrapolate the same
line shape to the size the cluster in #254 actually has.

What they caught: the first version of ParseScontrolNodes searched the line
once per field with anchored regexes, which cost about 340 ms at 5,884 nodes
against 14 ms for the sinfo output it replaced. Reading each line in a single
pass brought it to about 17 ms — below the old path, on much more verbose
input.
*/

// syntheticCluster builds scontrol and grouped-sinfo output for n nodes in
// four partitions, in the shape the real captures have — including the OS=
// field, which is most of the line the parser has to scan past.
func syntheticCluster(n int) (scontrolOut, stateOut, oldSinfoOut []byte) {
	var sc, old strings.Builder
	for i := 1; i <= n; i++ {
		name := fmt.Sprintf("na%04d", i)
		alloc := (i % 5) * 8
		fmt.Fprintf(&sc,
			"NodeName=%s Arch=x86_64 CoresPerSocket=32  CPUAlloc=%d CPUEfctv=40 CPUTot=40 CPULoad=8.62 "+
				"AvailableFeatures=cpu ActiveFeatures=cpu Gres=(null) NodeAddr=10.1.2.3 NodeHostName=%s "+
				"Version=25.05.9 OS=Linux 6.18.33.2-generic #1 SMP PREEMPT_DYNAMIC Thu Jun 18 21:54:43 UTC 2026  "+
				"RealMemory=192000 AllocMem=%d FreeMem=406 Sockets=1 Boards=1 State=%s ThreadsPerCore=2 "+
				"TmpDisk=0 Weight=1 Owner=N/A MCS_label=N/A Partitions=pa,pb,pc,pd  BootTime=2026-07-24T16:41:28 "+
				"SlurmdStartTime=2026-07-31T03:59:08 LastBusyTime=2026-07-31T03:59:08 ResumeAfterTime=None "+
				"CfgTRES=cpu=40,mem=192000M,billing=40 AllocTRES=cpu=%d,mem=8G CurrentWatts=0 AveWatts=0\n",
			name, alloc, name, alloc*1024, stateFor(i), alloc)
		for _, p := range []string{"pa", "pb", "pc", "pd"} {
			fmt.Fprintf(&old, "%s %d 192000 %d/%d/0/40 %s %s\n",
				name, alloc*1024, alloc, 40-alloc, sinfoStateFor(i), p)
		}
	}
	// The grouped call collapses everything into a handful of rows.
	state := "na[0001-" + fmt.Sprintf("%04d", n) + "]|mixed\n"
	return []byte(sc.String()), []byte(state), []byte(old.String())
}

func stateFor(i int) string {
	if i%97 == 0 {
		return "IDLE+DRAIN"
	}
	return "MIXED"
}

func sinfoStateFor(i int) string {
	if i%97 == 0 {
		return "drained"
	}
	return "mixed"
}

func BenchmarkNodeParsePathAtScale(b *testing.B) {
	for _, n := range []int{10, 1000, 5884} {
		sc, st, old := syntheticCluster(n)
		b.Run(fmt.Sprintf("n=%d/new_scontrol+state", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				BuildNodeMetrics(sc, st)
			}
		})
		b.Run(fmt.Sprintf("n=%d/old_sinfo_N", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				ParseNodeMetrics(old)
			}
		})
	}
}

func BenchmarkExpandHostListAtScale(b *testing.B) {
	expr := "na[0001-5884]"
	b.ReportAllocs()
	for b.Loop() {
		expandHostList(expr)
	}
}
