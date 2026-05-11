#!/usr/bin/env bash
# Offline equivalent of goreportcard.com checks. Designed to run *inside*
# the slurm_exporter-tools container, where every required binary is
# present and the repository is mounted at /repo.
#
# Reproduces the six checks goreportcard.com performs:
#   gofmt -s, go vet, gocyclo, ineffassign, misspell, license presence
#
# Score = files_ok / total_files per check; grade matches goreportcard.com:
#   A+ ≥ 95   A ≥ 90   B ≥ 80   C ≥ 70   D ≥ 60   F < 60
# Exits non-zero if grade is below B so it can gate a Makefile target.

set -u

C_RESET=$'\033[0m'
C_GREEN=$'\033[32m'
C_YELLOW=$'\033[33m'
C_RED=$'\033[31m'
C_BOLD=$'\033[1m'

mapfile -t GO_FILES < <(find . -type f -name '*.go' \
    -not -path './go/*' \
    -not -path './bin/*' \
    -not -path './dist/*')
TOTAL=${#GO_FILES[@]}
if [ "$TOTAL" -eq 0 ]; then
    echo "no Go source files found" >&2
    exit 1
fi

pct_color() {
    local pct=$1
    if [ "$pct" -ge 9000 ]; then echo "$C_GREEN"
    elif [ "$pct" -ge 8000 ]; then echo "$C_YELLOW"
    else echo "$C_RED"
    fi
}

# Each check writes its bad-file count to stdout.
count_gofmt() {
    gofmt -s -l "${GO_FILES[@]}" 2>/dev/null | awk 'END{print NR}'
}
count_govet() {
    go vet ./... 2>&1 | grep -E '^[^ ]+\.go:[0-9]+:' | awk -F: '{print $1}' | sort -u | wc -l
}
count_gocyclo() {
    gocyclo -over 15 . 2>/dev/null \
        | awk '{print $NF}' \
        | awk -F: '{print $1}' \
        | sort -u \
        | wc -l
}
count_ineffassign() {
    ineffassign ./... 2>&1 | grep -oE '[^ ]+\.go' | sort -u | wc -l
}
count_misspell() {
    misspell "${GO_FILES[@]}" 2>/dev/null \
        | grep -oE '^[^:]+\.go' \
        | sort -u \
        | wc -l
}
count_license() {
    if [ -f LICENSE ] || [ -f LICENSE.md ] || [ -f LICENSE.txt ]; then
        echo 0
    else
        echo "$TOTAL"
    fi
}

# Run a check and record its score.
declare -A SCORE
declare -A FAILED

run_check() {
    local name="$1"
    local description="$2"
    local count_fn="$3"
    local bad
    bad=$($count_fn 2>/dev/null | tr -cd "[:digit:]")
    bad=${bad:-0}
    local ok=$((TOTAL - bad))
    [ "$ok" -lt 0 ] && ok=0
    local pct=$(( ok * 10000 / TOTAL ))
    SCORE["$name"]=$pct
    FAILED["$name"]=$bad
    local pct_disp
    pct_disp=$(printf '%d.%02d' $((pct/100)) $((pct%100)))
    printf '%-14s %s%6s%%%s   %3d/%3d files OK   %s\n' \
        "$name" "$(pct_color $pct)" "$pct_disp" "$C_RESET" \
        "$ok" "$TOTAL" "$description"
}

echo "${C_BOLD}Go Report Card — offline ($TOTAL Go files)${C_RESET}"
echo
run_check "gofmt"       "Standard formatting (gofmt -s)"   count_gofmt
run_check "go vet"      "Standard static checks"           count_govet
run_check "gocyclo"     "Cyclomatic complexity > 15"       count_gocyclo
run_check "ineffassign" "Ineffectual assignments"          count_ineffassign
run_check "misspell"    "Common English misspellings"      count_misspell
run_check "license"     "LICENSE file present"             count_license

total=0
for v in "${SCORE[@]}"; do total=$((total + v)); done
avg=$(( total / 6 ))
avg_disp=$(printf '%d.%02d' $((avg/100)) $((avg%100)))

grade="F"
if [ "$avg" -ge 9500 ]; then grade="A+"
elif [ "$avg" -ge 9000 ]; then grade="A"
elif [ "$avg" -ge 8000 ]; then grade="B"
elif [ "$avg" -ge 7000 ]; then grade="C"
elif [ "$avg" -ge 6000 ]; then grade="D"
fi

echo
printf '%-14s %s%6s%%%s   %sgrade: %s%s\n' \
    "average" "$(pct_color $avg)" "$avg_disp" "$C_RESET" \
    "$C_BOLD" "$grade" "$C_RESET"

# Detail what's flagged for the most useful checks.
gocyclo_fail=${FAILED[gocyclo]:-0}
if [ "$gocyclo_fail" -gt 0 ]; then
    echo
    echo "${C_BOLD}gocyclo details (functions > 15):${C_RESET}"
    gocyclo -over 15 . 2>/dev/null | sed 's/^/  /'
fi
misspell_fail=${FAILED[misspell]:-0}
if [ "$misspell_fail" -gt 0 ]; then
    echo
    echo "${C_BOLD}misspell details:${C_RESET}"
    misspell "${GO_FILES[@]}" 2>/dev/null | head -20 | sed 's/^/  /'
fi
gofmt_fail=${FAILED[gofmt]:-0}
if [ "$gofmt_fail" -gt 0 ]; then
    echo
    echo "${C_BOLD}gofmt details (files needing gofmt -s):${C_RESET}"
    gofmt -s -l "${GO_FILES[@]}" 2>/dev/null | sed 's/^/  /'
fi

# Gate: fail if grade is below B.
[ "$avg" -ge 8000 ]
