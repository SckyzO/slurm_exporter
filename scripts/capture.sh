#!/bin/sh
# GENERATED FILE. DO NOT EDIT.
#
# Source of truth: internal/collector/command_registry.go
# Regenerate with:  make generate
#
# Collect Slurm fixtures for slurm_exporter, anonymised on this machine.
#
# What it does, and what it deliberately does not:
#
#   * Runs only the commands the exporter itself runs. The list is generated
#     from the collector registry, so it cannot include a command the exporter
#     does not use, and every one of them is read-only. There is no write
#     command in the registry, so none can appear here.
#   * Anonymises before writing anything. Node, user, account, reservation and
#     GRES-model names are replaced on this machine; the tarball never contains
#     them. That is what makes it safe to run on a production cluster and hand
#     the result to someone else.
#   * Records provenance: Slurm version, date, and the exit status of every
#     command. A partial capture is visible rather than silent.
#   * Leaves sacct alone unless asked. It queries SlurmDBD and is expensive on
#     a busy cluster, so it is behind --with-sacct and bounded to one hour.
#
# Usage:  ./capture.sh [-o OUTDIR] [--with-sacct]

set -eu

OUTDIR=${OUTDIR:-slurm-fixtures-$(date +%Y%m%d-%H%M%S)}
WITH_SACCT=0

while [ $# -gt 0 ]; do
    case $1 in
        -o|--out)     OUTDIR=$2; shift 2 ;;
        --with-sacct) WITH_SACCT=1; shift ;;
        -h|--help)    sed -n '2,25p' "$0"; exit 0 ;;
        *)            echo "unknown option: $1" >&2; exit 2 ;;
    esac
done

command -v sinfo >/dev/null 2>&1 || { echo 'sinfo not found in PATH' >&2; exit 1; }
mkdir -p "$OUTDIR"

# The exporter pins this on every command it runs (issue #158). Slurm renders
# every timestamp according to it, and a capture taken under a different
# setting comes back in a layout the parsers reject — so the fixture would
# encode a format the exporter never sees.
SLURM_TIME_FORMAT=standard
export SLURM_TIME_FORMAT

MAP="$OUTDIR/.anonymize.map"
AWK="$OUTDIR/.anonymize.awk"
PROV="$OUTDIR/provenance.txt"

# ── Anonymisation map ────────────────────────────────────────────────────────
#
# Built by asking Slurm what exists, so only real names are rewritten and no
# token is rewritten by resemblance. Node names map by alphabetic prefix, which
# covers both the expanded form (c1) and the compressed hostlists Slurm writes
# elsewhere (c[1-10,14]) without parsing range syntax.

build_map() {
    : > "$MAP"

    # The replacement must not end in a digit: a node prefix is immediately
    # followed by the node number, so mapping c -> n1 turns c1 into n11, which
    # reads as node eleven. Letters keep the numbering intact: c1 -> na1.
    sinfo -h -N -o '%N' 2>/dev/null | sed 's/[0-9].*$//' | sort -u | grep -v '^$' |
        awk '{ printf "prefix\t%s\tn%c\n", $1, 96 + NR }' >> "$MAP"

    { squeue -a -h -o '%u' 2>/dev/null; sshare -a -P -n -o User 2>/dev/null; } |
        tr -d ' ' | sort -u | grep -v '^$' |
        awk '{ printf "word\t%s\tuser%d\n", $1, NR }' >> "$MAP"

    sshare -a -P -n -o Account 2>/dev/null | tr -d ' ' | sort -u | grep -v '^$' |
        awk '{ printf "word\t%s\taccount%d\n", $1, NR }' >> "$MAP"

    scontrol show reservation 2>/dev/null |
        sed -n 's/.*ReservationName=\([^ ]*\).*/\1/p' | sort -u | grep -v '^$' |
        awk '{ printf "word\t%s\tresv%d\n", $1, NR }' >> "$MAP"

    # GRES models: the part between the type and the count in gpu:<model>:<n>.
    # A model name identifies the hardware a site bought, which is site data.
    sinfo -h -N -O 'Gres: ,GresUsed:' 2>/dev/null |
        tr ' ,' '\n\n' | sed -n 's/^[a-z]*:\([A-Za-z][A-Za-z0-9_.-]*\):[0-9].*/\1/p' |
        sort -u | grep -v '^$' |
        awk '{ printf "word\t%s\tmodel_%c\n", $1, 96 + NR }' >> "$MAP"

    printf 'entities to rewrite: %s\n' "$(wc -l < "$MAP" | tr -d ' ')"
}

# ── Embedded anonymiser ──────────────────────────────────────────────────────
write_awk() {
    cat > "$AWK" <<'ANONEOF'
# anonymize.awk — rewrite site-identifying names in captured Slurm output.
#
# Runs on the cluster, before anything is written to the tarball, so raw names
# never leave the machine. POSIX awk only: a login node has the Slurm client and
# little else, and requiring a Go toolchain there would defeat the point.
#
# The rule that makes this safe: it replaces names it was *told about*, read
# from a mapping file built by querying Slurm itself. It never guesses from
# shape. A pattern-matching anonymiser eventually eats "(null)", "N/A",
# "drained*", a GRES model or a timestamp, and the damage is silent — the
# fixture still looks plausible, and the expected values derived from it are
# wrong forever.
#
# Node names are mapped by their alphabetic prefix rather than whole. Slurm
# writes hostlists compressed ("kairosgh[0-3]", "c[1-10,14]") and expanded
# ("kairosgh0") in different commands, and mapping the prefix handles both
# without having to parse range syntax: kairosgh[0-3] becomes g[0-3], keeping
# the structure a parser under test needs to see.
#
# Usage:  awk -v mapfile=<map> -f anonymize.awk <capture>
#
# The map is one "kind<TAB>from<TAB>to" per line:
#   prefix   kairosgh    g          node-name prefixes, applied first
#   word     alice       user1      whole words: users, accounts, reservations
#   word     gh200       model_a    GRES models

BEGIN {
    FS = "\t"
    npfx = 0
    nword = 0
    while ((getline line < mapfile) > 0) {
        n = split(line, f, "\t")
        if (n < 3 || f[1] ~ /^#/) continue
        if (f[1] == "prefix") { pfx_from[++npfx] = f[2]; pfx_to[npfx] = f[3] }
        else if (f[1] == "word") { word_from[++nword] = f[2]; word_to[nword] = f[3] }
    }
    close(mapfile)
}

# is_word_char reports whether c can appear inside a Slurm name. Underscore and
# hyphen count: "tesla_v100-sxm3-32gb" is one name, and matching "v100" inside
# it would corrupt a GRES model rather than anonymise a node.
function is_word_char(c) {
    return c ~ /[A-Za-z0-9_.-]/
}

# replace_word swaps every standalone occurrence of `from` with `to`. Standalone
# means not glued to another name character on either side, so mapping node "c1"
# leaves "c10" and "gpu:c1model" untouched. Done by hand rather than with gsub
# because the replacement text may contain characters gsub treats specially.
function replace_word(s, from, to,   out, i, before, after, flen) {
    out = ""
    flen = length(from)
    while ((i = index(s, from)) > 0) {
        before = (i == 1) ? "" : substr(s, i - 1, 1)
        after  = substr(s, i + flen, 1)
        if ((before == "" || !is_word_char(before)) &&
            (after  == "" || !is_word_char(after))) {
            out = out substr(s, 1, i - 1) to
        } else {
            out = out substr(s, 1, i + flen - 1)
        }
        s = substr(s, i + flen)
    }
    return out s
}

# replace_prefix swaps a node-name prefix wherever it starts a name. The prefix
# must be followed by a digit or "[", which is what distinguishes the node
# "kairosgh0" and the hostlist "kairosgh[0-3]" from an unrelated word that
# merely starts with the same letters.
function replace_prefix(s, from, to,   out, i, before, after, flen) {
    out = ""
    flen = length(from)
    while ((i = index(s, from)) > 0) {
        before = (i == 1) ? "" : substr(s, i - 1, 1)
        after  = substr(s, i + flen, 1)
        if ((before == "" || !is_word_char(before)) && after ~ /[0-9[]/) {
            out = out substr(s, 1, i - 1) to
        } else {
            out = out substr(s, 1, i + flen - 1)
        }
        s = substr(s, i + flen)
    }
    return out s
}

{
    line = $0
    # Prefixes first: a node prefix can be a substring of nothing else, while a
    # word mapping applied first could split a hostname and leave the prefix
    # rule unable to see it.
    for (i = 1; i <= npfx; i++)  line = replace_prefix(line, pfx_from[i], pfx_to[i])
    for (i = 1; i <= nword; i++) line = replace_word(line, word_from[i], word_to[i])
    print line
}
ANONEOF
}

# ── Capture ──────────────────────────────────────────────────────────────────

# run_step executes one command, anonymises its output and records whether it
# worked. A command that fails writes an empty file and a FAILED line rather
# than aborting: a partial capture is still useful, as long as it says so.
run_step() {
    name=$1; shift
    if "$@" > "$OUTDIR/.raw" 2>"$OUTDIR/.err"; then
        status=ok
    else
        status=FAILED
    fi
    awk -v mapfile="$MAP" -f "$AWK" < "$OUTDIR/.raw" > "$OUTDIR/$name.txt"
    printf '%-22s %-8s %s\n' "$name" "$status" "$*" >> "$PROV"
    [ "$status" = ok ] || printf '    stderr: %s\n' "$(head -1 "$OUTDIR/.err")" >> "$PROV"
}

write_awk
build_map

{
    echo 'slurm_exporter fixture capture'
    echo "date:    $(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo "slurm:   $(sinfo --version 2>/dev/null)"
    echo "host:    $(hostname | cksum | cut -d' ' -f1)  (hashed)"
    echo "sacct:   $([ "$WITH_SACCT" = 1 ] && echo included || echo skipped)"
    echo
    echo 'command                status   invocation'
} > "$PROV"

run_step squeue_jobs            squeue '-a' '-r' '-h' '-O' 'JobID:|,Account:|,UserName:|,Partition:|,State:|,NumNodes:|,NumCPUs:|,tres-alloc:'
run_step queue_all_states       squeue '-h' '-o' '%P|%T|%C|%r|%u' '--states=all'
run_step queue_default_states   squeue '-h' '-o' '%P|%T|%C|%r|%u'
run_step cpus                   sinfo '-h' '-o' '%C'
run_step fairshare              sshare '-a' '-P' '-n' '-o' 'Account,User,RawShares,NormShares,RawUsage,NormUsage,FairShare'
run_step gpus_snapshot          sinfo '-a' '-h' '--Format=Nodes: ,StateLong: ,Gres: ,GresUsed:'
run_step node_detail            sinfo '-h' '-N' '-O' 'NodeList: ,AllocMem: ,Memory: ,CPUsState: ,StateLong: ,Partition: ,Gres: ,GresUsed:'
run_step nodes_global           sinfo '-h' '-o' '%R|%D|%T|%b'
run_step scontrol_nodes         scontrol 'show' 'nodes' '-o'
run_step partitions_cpu         sinfo '-h' '-o' '%R,%C'
run_step partitions_gpu         sinfo '-h' '--Format=Nodes: ,Partition: ,Gres: ,GresUsed:' '--state=idle,allocated'
run_step drain_reason           sinfo '-h' '-N' '-o' '%N|%E|%H|%T'
run_step reservations           scontrol 'show' 'reservation'
run_step licenses               scontrol 'show' 'licenses' '-o'
run_step scheduler              sdiag

if [ "$WITH_SACCT" = 1 ]; then
    run_step sacct_efficiency       sacct '-P' '-n' '--starttime' "$(date -d "-1 hour" +%Y-%m-%dT%H:%M:%S)" '--endtime' "$(date +%Y-%m-%dT%H:%M:%S)" '--format' 'JobID,User,Account,AllocCPUS,Elapsed,TotalCPU,CPUTime,MaxRSS,ReqMem' '--state' 'COMPLETED,FAILED,TIMEOUT,CANCELLED'
fi

rm -f "$OUTDIR/.raw" "$OUTDIR/.err" "$AWK"

# The mapping stays on the cluster. It is the one file that could undo the
# anonymisation, so it never goes in the tarball.
mv "$MAP" "./$(basename "$OUTDIR").map" 2>/dev/null || rm -f "$MAP"

tar czf "$OUTDIR.tar.gz" "$OUTDIR"
echo
echo "captured: $OUTDIR.tar.gz"
echo "mapping kept locally, not in the tarball: $(basename "$OUTDIR").map"
cat "$PROV"
