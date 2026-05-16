#!/usr/bin/env bash
# Reports the state of Go module dependencies in a tabular form.
# Lists direct dependencies first, then indirect ones, marking each with
# its current version, the latest available version (if any), and the
# nature of the upgrade (patch / minor / major).
#
# Designed to run inside the slurm_exporter-tools container so it doesn't
# depend on the host's Go installation.

set -u

C_RESET=$'\033[0m'
C_BOLD=$'\033[1m'
C_GREEN=$'\033[32m'
C_YELLOW=$'\033[33m'
C_RED=$'\033[31m'
C_DIM=$'\033[2m'

# Pull the full state once — direct + indirect + available updates.
ALL=$(go list -m -u -mod=mod all 2>/dev/null) || {
    echo "go list failed — are we inside a Go module?" >&2
    exit 1
}

# Read the direct deps straight from go.mod so we don't have to
# heuristically tell them apart from indirects in the `go list` output.
mapfile -t DIRECT < <(awk '
    /^require \(/  { in_require = 1; next }
    /^\)/          { in_require = 0; next }
    in_require && !/\/\/ indirect/ && NF >= 2 { print $1 }
    /^require [^(]/ && !/\/\/ indirect/ && NF >= 3 { print $2 }
' go.mod | sort -u)

# Classify an upgrade as patch / minor / major.
classify_bump() {
    local current=$1 available=$2
    current=${current#v}
    available=${available#v}
    local c_major c_minor c_patch a_major a_minor a_patch
    IFS=. read -r c_major c_minor c_patch _ <<< "$current"
    IFS=. read -r a_major a_minor a_patch _ <<< "$available"
    if [ "${c_major:-0}" != "${a_major:-0}" ]; then
        echo "major"
    elif [ "${c_minor:-0}" != "${a_minor:-0}" ]; then
        echo "minor"
    else
        echo "patch"
    fi
}

# Format one module line for the table.
emit_row() {
    local module=$1
    local line
    line=$(printf '%s\n' "$ALL" | awk -v m="$module" '$1 == m { print; exit }')
    if [ -z "$line" ]; then
        return
    fi

    local current available
    current=$(echo "$line" | awk '{ print $2 }')

    if echo "$line" | grep -q '\['; then
        available=$(echo "$line" | sed -n 's/.*\[\([^]]*\)\].*/\1/p')
        local bump
        bump=$(classify_bump "$current" "$available")
        local color
        case "$bump" in
            patch) color=$C_GREEN  ;;
            minor) color=$C_YELLOW ;;
            major) color=$C_RED    ;;
        esac
        printf "  %-50s %-18s %s->%s %-15s %s[%s]%s\n" \
            "$module" "$current" "$C_DIM" "$C_RESET" "$available" "$color" "$bump" "$C_RESET"
    else
        printf "  %-50s %-18s %s(up to date)%s\n" \
            "$module" "$current" "$C_DIM" "$C_RESET"
    fi
}

# Modules to show as "indirect with updates" — only the ones that actually
# have an upgrade available, otherwise the indirect list is too noisy.
mapfile -t INDIRECT_WITH_UPDATES < <(
    printf '%s\n' "$ALL" \
        | awk '/\[/ { print $1 }' \
        | grep -vxFf <(printf '%s\n' "${DIRECT[@]}") \
        | sort -u
)

count_direct=${#DIRECT[@]}
count_indirect_updates=${#INDIRECT_WITH_UPDATES[@]}
direct_with_updates=0
for module in "${DIRECT[@]}"; do
    if printf '%s\n' "$ALL" | awk -v m="$module" '$1 == m && /\[/ { exit 0 } END { exit 1 }'; then
        direct_with_updates=$((direct_with_updates + 1))
    fi
done

echo "${C_BOLD}Dependency update report${C_RESET}"
echo

printf "%sDirect dependencies%s (%d total, %d with updates)\n" \
    "$C_BOLD" "$C_RESET" "$count_direct" "$direct_with_updates"
printf "  %-50s %-18s %-18s %s\n" "MODULE" "CURRENT" "AVAILABLE" "BUMP"
for module in "${DIRECT[@]}"; do
    emit_row "$module"
done

echo
if [ "$count_indirect_updates" -eq 0 ]; then
    echo "${C_DIM}Indirect dependencies: all up to date${C_RESET}"
else
    printf "%sIndirect dependencies with updates%s (%d)\n" \
        "$C_BOLD" "$C_RESET" "$count_indirect_updates"
    printf "  %-50s %-18s %-18s %s\n" "MODULE" "CURRENT" "AVAILABLE" "BUMP"
    for module in "${INDIRECT_WITH_UPDATES[@]}"; do
        emit_row "$module"
    done
fi

echo
total_updates=$((direct_with_updates + count_indirect_updates))
if [ "$total_updates" -eq 0 ]; then
    printf "${C_GREEN}${C_BOLD}Everything is up to date.${C_RESET}\n"
else
    printf '%s%s%d update(s) available.%s Run %sgo get -u ./...%s then %sgo mod tidy%s.\n' \
        "$C_YELLOW" "$C_BOLD" "$total_updates" "$C_RESET" \
        "$C_BOLD" "$C_RESET" "$C_BOLD" "$C_RESET"
fi
