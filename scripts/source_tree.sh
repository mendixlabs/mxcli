#!/bin/bash
# Source code tree view sorted by dependency order
# Shows all Go and TypeScript source files with line counts and visual bars
#
# Directories are sorted by dependency depth: packages with no internal
# dependencies appear first (tier 0), packages that depend on them next
# (tier 1), and so on up to the CLI entry point at the top.
#
# Options:
#   --cover   Run tests with coverage before displaying (slow)
#
# If coverage.out exists in the project root, coverage bars are shown
# automatically. Use --cover to generate fresh coverage data.

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_ROOT"

MODULE_PATH=$(go list -m 2>/dev/null)

# --- Parse arguments ---
COVER_FLAG=false
for arg in "$@"; do
    case "$arg" in
        --cover) COVER_FLAG=true ;;
    esac
done

# --- Colors & symbols ---
if [ -t 1 ]; then
    BOLD='\033[1m'
    DIM='\033[2m'
    CYAN='\033[36m'
    GREEN='\033[32m'
    YELLOW='\033[33m'
    ORANGE='\033[38;5;208m'
    RED='\033[31m'
    WHITE='\033[37m'
    RESET='\033[0m'
else
    BOLD='' DIM='' CYAN='' GREEN='' YELLOW='' ORANGE='' RED='' WHITE='' RESET=''
fi

TMPWORK=$(mktemp -d)
trap "rm -rf $TMPWORK" EXIT

# --- Step 1: Collect all source file line counts ---

find . -name "*.go" \
    -not -path "./vendor/*" \
    -not -path "./.git/*" \
    -not -path "./generated/*" \
    -not -path "./libs/*" \
    -not -path "./reference/*" \
    -not -path "*/parser/*.go" \
    -not -path "*/node_modules/*" \
    -type f -exec wc -l {} \; 2>/dev/null | \
    awk '{gsub("^\\./", "", $2); print $1 "\t" $2}' > "$TMPWORK/files.txt"

find ./vscode-mdl -name "*.ts" \
    -not -path "*/node_modules/*" \
    -not -path "*/out/*" \
    -not -path "*/.vscode-test/*" \
    -type f -exec wc -l {} \; 2>/dev/null | \
    awk '{gsub("^\\./", "", $2); print $1 "\t" $2}' >> "$TMPWORK/files.txt"

# --- Step 1.5: Coverage data ---

if [ "$COVER_FLAG" = true ]; then
    echo -e "${DIM}Running tests with coverage (this may take a while)...${RESET}" >&2
    CGO_ENABLED=0 go test -coverprofile="$PROJECT_ROOT/coverage.out" ./... 2>/dev/null || true
fi

HAS_COVERAGE=false
if [ -f "$PROJECT_ROOT/coverage.out" ]; then
    HAS_COVERAGE=true
    # Parse coverage profile into per-file data:
    #   pct <TAB> total_stmts <TAB> covered_stmts <TAB> filepath
    awk -v mod="$MODULE_PATH/" '
    /^mode:/ { next }
    {
        # Format: file:startLine.startCol,endLine.endCol numStmts count
        idx = match($1, /:[0-9]/)
        if (idx == 0) next
        file = substr($1, 1, idx-1)
        sub(mod, "", file)
        stmts = $2 + 0
        count = $3 + 0
        total[file] += stmts
        if (count > 0) covered[file] += stmts
    }
    END {
        for (f in total) {
            pct = (total[f] > 0) ? int(covered[f] * 100 / total[f]) : 0
            print pct "\t" total[f] "\t" covered[f]+0 "\t" f
        }
    }' "$PROJECT_ROOT/coverage.out" > "$TMPWORK/coverage.txt"
fi

# --- Step 1.6: Recent commit change data ---

HAS_COMMITS=false
COMMIT_COUNT=0

if git rev-parse --is-inside-work-tree &>/dev/null; then
    git log --format='%h' -5 2>/dev/null > "$TMPWORK/commit_hashes.txt"
    COMMIT_COUNT=$(wc -l < "$TMPWORK/commit_hashes.txt" | tr -d ' ')
    if [ "$COMMIT_COUNT" -gt 0 ]; then
        HAS_COMMITS=true
        # Collect per-file change counts for each commit
        : > "$TMPWORK/commits.txt"
        while IFS= read -r hash; do
            git diff --numstat --no-renames "${hash}~1..${hash}" 2>/dev/null | \
            while IFS=$'\t' read -r added deleted filepath; do
                # Skip binary files (numstat shows - for binary)
                [ "$added" = "-" ] && continue
                changed=$(( added + deleted ))
                printf '%s\t%s\t%d\n' "$hash" "$filepath" "$changed" >> "$TMPWORK/commits.txt"
            done
        done < "$TMPWORK/commit_hashes.txt"
    fi
fi

# --- Step 2: Compute dependency depth via go list + topological sort ---

go list -f '{{.ImportPath}}{{range .Imports}} {{.}}{{end}}' ./... 2>/dev/null | \
awk -v mod="$MODULE_PATH" '
{
    pkg = $1
    sub("^" mod "/?", "", pkg)
    if (pkg == "") pkg = "."
    packages[pkg] = 1

    for (i = 2; i <= NF; i++) {
        dep = $i
        if (index(dep, mod) == 1) {
            sub("^" mod "/?", "", dep)
            if (dep == "") dep = "."
            ndeps[pkg]++
            dep_arr[pkg, ndeps[pkg]] = dep
        }
    }
}
END {
    for (p in packages) depth[p] = 0

    for (iter = 0; iter < 30; iter++) {
        changed = 0
        for (p in packages) {
            for (i = 1; i <= ndeps[p]; i++) {
                d = dep_arr[p, i]
                if (d in packages && depth[d] + 1 > depth[p]) {
                    depth[p] = depth[d] + 1
                    changed = 1
                }
            }
        }
        if (!changed) break
    }

    for (p in packages) print depth[p] "\t" p
}' > "$TMPWORK/depths.txt"

# Add TypeScript directories at the highest tier (depends on the Go CLI)
max_depth=$(awk '{if ($1 > m) m=$1} END {print m+0}' "$TMPWORK/depths.txt")
awk -F'\t' '$2 ~ /\.ts$/ {n=split($2,a,"/"); d=""; for(i=1;i<n;i++){if(i>1)d=d"/"; d=d a[i]}; print d}' \
    "$TMPWORK/files.txt" | sort -u | while read -r dir; do
    printf '%s\t%s\n' "$((max_depth + 1))" "$dir" >> "$TMPWORK/depths.txt"
done

# --- Step 3: Find max line count for bar scaling ---

max_lines=$(awk -F'\t' '{if ($1 > m) m=$1} END {print m+0}' "$TMPWORK/files.txt")
BAR_MAX=40
COV_BAR_MAX=10

bar_color() {
    local lines=$1
    if [ "$lines" -ge 1500 ]; then
        echo "$RED"
    elif [ "$lines" -ge 1000 ]; then
        echo "$ORANGE"
    elif [ "$lines" -ge 500 ]; then
        echo "$YELLOW"
    else
        echo "$GREEN"
    fi
}

make_bar() {
    local lines=$1
    local len=$(( lines * BAR_MAX / max_lines ))
    local bar=""
    local display_len=0
    for ((i=0; i<len; i++)); do bar="${bar}█"; done
    display_len=$len
    # Add a half block if we'd round up
    local remainder=$(( (lines * BAR_MAX * 2 / max_lines) - len * 2 ))
    if [ "$remainder" -gt 0 ] && [ "$len" -lt "$BAR_MAX" ]; then
        bar="${bar}▌"
        display_len=$((display_len + 1))
    fi
    # Pad to fixed width for column alignment
    local pad=$((BAR_MAX - display_len))
    for ((i=0; i<pad; i++)); do bar="${bar} "; done
    echo "$bar"
}

cov_bar_color() {
    local pct=$1
    if [ "$pct" -ge 80 ]; then
        echo "$GREEN"
    elif [ "$pct" -ge 60 ]; then
        echo "$YELLOW"
    elif [ "$pct" -ge 30 ]; then
        echo "$ORANGE"
    else
        echo "$RED"
    fi
}

commit_change_color() {
    local changed=$1
    if [ "$changed" -ge 500 ]; then
        echo "$RED"
    elif [ "$changed" -ge 200 ]; then
        echo "$ORANGE"
    elif [ "$changed" -ge 50 ]; then
        echo "$YELLOW"
    else
        echo "$GREEN"
    fi
}

# --- Step 4: Render the tree ---

echo -e "${BOLD}Source Code Tree${RESET} ${DIM}(sorted by dependency depth — leaf packages first)${RESET}"
echo -e "  ${DIM}Lines:${RESET}    ${GREEN}██${RESET} <500  ${YELLOW}██${RESET} 500-999  ${ORANGE}██${RESET} 1000-1499  ${RED}██${RESET} 1500+"
if [ "$HAS_COVERAGE" = true ]; then
    echo -e "  ${DIM}Coverage:${RESET} ${GREEN}██${RESET} 80%+  ${YELLOW}██${RESET} 60-79%  ${ORANGE}██${RESET} 30-59%  ${RED}██${RESET} <30%"
fi
if [ "$HAS_COMMITS" = true ]; then
    echo -e "  ${DIM}Commits:${RESET}  ${GREEN}██${RESET} <50   ${YELLOW}██${RESET} 50-199  ${ORANGE}██${RESET} 200-499  ${RED}██${RESET} 500+"
    # Print commit hash header aligned with file columns
    # 2 (indent) + 4 (connector) + 36 (filename) + 5 (lines) + 2 (gap) + BAR_MAX (bar)
    pad_width=$((2 + 4 + 36 + 5 + 2 + BAR_MAX))
    if [ "$HAS_COVERAGE" = true ]; then
        # coverage: 2 (gap) + 4 (pct) + 1 (space) + COV_BAR_MAX (bar)
        pad_width=$((pad_width + 2 + 4 + 1 + COV_BAR_MAX))
    fi
    printf "%${pad_width}s" ""
    printf "  "
    while IFS= read -r ch_hash; do
        printf "${DIM}%6s${RESET}" "$(echo "$ch_hash" | cut -c1-4)"
    done < "$TMPWORK/commit_hashes.txt"
    printf "\n"
fi
echo ""

prev_tier=-1

sort -t$'\t' -k1,1n -k2,2 "$TMPWORK/depths.txt" | while IFS=$'\t' read -r tier dir; do
    # Determine directory prefix for matching files
    if [ "$dir" = "." ]; then
        # Root package: files with no slash
        files=$(awk -F'\t' '$2 !~ /\// {print $1 "\t" $2}' "$TMPWORK/files.txt" | sort -t$'\t' -k1,1rn)
    else
        # Files directly in this directory (not subdirectories)
        files=$(awk -F'\t' -v pfx="${dir}/" '$2 ~ "^"pfx && $2 !~ "^"pfx".+/" {print $1 "\t" $2}' "$TMPWORK/files.txt" | sort -t$'\t' -k1,1rn)
    fi

    [ -z "$files" ] && continue

    # Tier separator
    if [ "$tier" -ne "$prev_tier" ]; then
        if [ "$prev_tier" -ne -1 ]; then
            echo ""
        fi
        printf "${DIM}──── tier %d " "$tier"
        printf '─%.0s' $(seq 1 60)
        printf "${RESET}\n"
        prev_tier=$tier
    fi

    # Directory totals
    dir_total=$(echo "$files" | awk -F'\t' '{s+=$1} END {print s}')
    file_count=$(echo "$files" | wc -l)

    if [ "$dir" = "." ]; then
        display_dir="(root)"
    else
        display_dir="${dir}/"
    fi

    printf "${BOLD}${CYAN}%-48s${RESET} ${YELLOW}%6d${RESET} lines  ${DIM}(%d files)${RESET}\n" \
        "$display_dir" "$dir_total" "$file_count"

    # Print files with tree connectors and bars
    n=$(echo "$files" | wc -l)
    i=0
    echo "$files" | while IFS=$'\t' read -r lines filepath; do
        i=$((i + 1))
        filename=$(basename "$filepath")

        if [ "$i" -eq "$n" ]; then
            connector="└── "
        else
            connector="├── "
        fi

        bar=$(make_bar "$lines")
        color=$(bar_color "$lines")

        # Print base columns (connector, filename, line count, size bar)
        printf "  ${DIM}%s${RESET}%-36s${WHITE}%5d${RESET}  ${color}%s${RESET}" \
            "$connector" "$filename" "$lines" "$bar"

        # Append coverage bar if data is available
        if [ "$HAS_COVERAGE" = true ]; then
            cov_pct=$(awk -F'\t' -v f="$filepath" '$4 == f {print $1; exit}' "$TMPWORK/coverage.txt" 2>/dev/null)
            if [ -n "$cov_pct" ]; then
                cov_clr=$(cov_bar_color "$cov_pct")
                filled_len=$(( cov_pct * COV_BAR_MAX / 100 ))
                cov_filled=""
                cov_empty=""
                for ((j=0; j<filled_len; j++)); do cov_filled="${cov_filled}█"; done
                for ((j=filled_len; j<COV_BAR_MAX; j++)); do cov_empty="${cov_empty}░"; done
                printf "  ${cov_clr}%3d%% %s${DIM}%s${RESET}" "$cov_pct" "$cov_filled" "$cov_empty"
            else
                # Pad to keep commit columns aligned
                printf "  %*s" $((4 + 1 + COV_BAR_MAX)) ""
            fi
        fi

        # Append recent commit change columns
        if [ "$HAS_COMMITS" = true ]; then
            printf "  "
            while IFS= read -r ch_hash; do
                ch_count=$(awk -F'\t' -v h="$ch_hash" -v f="$filepath" '$1 == h && $2 == f {print $3; exit}' "$TMPWORK/commits.txt" 2>/dev/null)
                if [ -n "$ch_count" ] && [ "$ch_count" -gt 0 ]; then
                    ch_clr=$(commit_change_color "$ch_count")
                    printf "${ch_clr}%6d${RESET}" "$ch_count"
                else
                    printf "${DIM}%6s${RESET}" "·"
                fi
            done < "$TMPWORK/commit_hashes.txt"
        fi

        printf "\n"
    done
done

# --- Step 5: Summary ---

echo ""
echo ""

total_lines=$(awk -F'\t' '{s+=$1} END {print s}' "$TMPWORK/files.txt")
total_files=$(wc -l < "$TMPWORK/files.txt")
go_lines=$(awk -F'\t' '$2 ~ /\.go$/ {s+=$1} END {print s+0}' "$TMPWORK/files.txt")
go_count=$(awk -F'\t' '$2 ~ /\.go$/' "$TMPWORK/files.txt" | wc -l)
ts_lines=$(awk -F'\t' '$2 ~ /\.ts$/ {s+=$1} END {print s+0}' "$TMPWORK/files.txt")
ts_count=$(awk -F'\t' '$2 ~ /\.ts$/' "$TMPWORK/files.txt" | wc -l)

# Count unique directories
dir_count=$(awk -F'\t' '{n=split($2,a,"/"); d=""; for(i=1;i<n;i++){if(i>1)d=d"/"; d=d a[i]}; print d}' "$TMPWORK/files.txt" | sort -u | wc -l)

echo -e "${BOLD}Summary${RESET}"
printf "  %-14s ${YELLOW}%6d${RESET} lines   %3d files   %2d packages\n" "Go" "$go_lines" "$go_count" "$dir_count"
printf "  %-14s ${YELLOW}%6d${RESET} lines   %3d files\n" "TypeScript" "$ts_lines" "$ts_count"
echo -e "  ${DIM}──────────────────────────────────────────${RESET}"
printf "  ${BOLD}%-14s ${YELLOW}%6d${RESET} lines   %3d files${RESET}\n" "Total" "$total_lines" "$total_files"

if [ "$HAS_COVERAGE" = true ]; then
    cov_total=$(awk -F'\t' '{s+=$2} END {print s+0}' "$TMPWORK/coverage.txt")
    cov_covered=$(awk -F'\t' '{s+=$3} END {print s+0}' "$TMPWORK/coverage.txt")
    if [ "$cov_total" -gt 0 ]; then
        cov_pct=$((cov_covered * 100 / cov_total))
    else
        cov_pct=0
    fi
    cov_clr=$(cov_bar_color "$cov_pct")
    cov_files=$(wc -l < "$TMPWORK/coverage.txt")
    echo ""
    printf "  ${BOLD}Coverage${RESET}       ${cov_clr}%3d%%${RESET}          %3d files with data\n" "$cov_pct" "$cov_files"
fi
