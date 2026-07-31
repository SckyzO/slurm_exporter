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
