#!/usr/bin/env bash
# mxcli-batch.sh -- run mxcli across a fleet of .mpr projects in three modes.
#
# Modes:
#   calibrate  Raw, unscored per-category penalty/rate data (same as the
#              original calibrate.sh) -- for deriving/tuning
#              normalizationBaseline values. Writes ONE file
#              (default: calibration.json) directly into <projects-dir>.
#
#   summary    The ACTUAL scored report (mxcli's real Score/Categories,
#              using whatever normalizationBaseline is currently built
#              into the binary) for every project, combined into ONE
#              json file (default: summary.json) directly into
#              <projects-dir>. Suitable as-is for visualization (e.g. the
#              per-app-per-category heatmap).
#
#   full       Same combined summary.json as above, PLUS the complete,
#              unmodified mxcli report for each individual project saved
#              as "<app-name>-report.json". All of these land together in
#              one output FOLDER (default name: mxcli-reports) inside
#              <projects-dir>.
#
# Expects a directory containing one subfolder per project, each subfolder
# containing exactly one .mpr file, e.g.:
#   MAIN/
#     GBS - Human in the Loop (HITL)-main/CANValidator.mpr
#     TOPS-main/SomeApp.mpr
#     ...
#
# Usage: ./mxcli-batch.sh <calibrate|summary|full> <mxcli-binary> <projects-dir> [output-name] [--force-init] [--exclude-marketplace] [-e MODULE ...]
#
# --force-init: by default, a project's .claude/lint-rules/ is only
# (re)created via `mxcli init` if it's missing or empty -- an already-
# initialized project is left alone, even if the rules embedded in the
# mxcli binary have since changed (e.g. after editing a .star rule and
# rebuilding). Pass --force-init to always wipe and re-run `mxcli init`
# for every project, guaranteeing everyone is on the latest rules
# currently embedded in $MXCLI.
#
# --exclude-marketplace: forwarded to every `mxcli report` call, excluding
# all Marketplace-sourced modules (and System) from calibration/summary/full
# output alike.
#
# -e/--exclude MODULE: forwarded to every `mxcli report` call, may be
# repeated. Combine with --exclude-marketplace to also drop specific local
# modules (e.g. internal shared libraries) from the score.
set -euo pipefail

FORCE_INIT=false
EXCLUDE_MARKETPLACE=false
EXCLUDE_MODULES=()
POSITIONAL=()
while [ $# -gt 0 ]; do
	case "$1" in
		--force-init)
			FORCE_INIT=true
			shift
			;;
		--exclude-marketplace)
			EXCLUDE_MARKETPLACE=true
			shift
			;;
		-e|--exclude)
			EXCLUDE_MODULES+=("$2")
			shift 2
			;;
		*)
			POSITIONAL+=("$1")
			shift
			;;
	esac
done
set -- "${POSITIONAL[@]+"${POSITIONAL[@]}"}"

MODE="${1:?Usage: mxcli-batch.sh <calibrate|summary|full> <mxcli-binary> <projects-dir> [output-name] [--force-init]}"
MXCLI="${2:?Usage: mxcli-batch.sh <calibrate|summary|full> <mxcli-binary> <projects-dir> [output-name] [--force-init]}"
PROJECTS_DIR="${3:?Usage: mxcli-batch.sh <calibrate|summary|full> <mxcli-binary> <projects-dir> [output-name] [--force-init]}"
PROJECTS_DIR="${PROJECTS_DIR%/}"

case "$MODE" in
	calibrate) DEFAULT_OUT="calibration.json" ;;
	summary)   DEFAULT_OUT="summary.json" ;;
	full)      DEFAULT_OUT="mxcli-reports" ;;
	*)
		echo "Unknown mode '$MODE'. Use one of: calibrate, summary, full." >&2
		exit 1
		;;
esac
OUT_NAME="${4:-$DEFAULT_OUT}"

EXCLUDE_FLAGS=()
if [ "$EXCLUDE_MARKETPLACE" = true ]; then
	EXCLUDE_FLAGS+=(--exclude-marketplace)
fi
for m in "${EXCLUDE_MODULES[@]:-}"; do
	[ -n "$m" ] && EXCLUDE_FLAGS+=(--exclude "$m")
done

# calibrate/summary write a single file directly into PROJECTS_DIR.
# full writes a whole folder of files into PROJECTS_DIR.
if [ "$MODE" = "full" ]; then
	REPORTS_DIR="$PROJECTS_DIR/$OUT_NAME"
	mkdir -p "$REPORTS_DIR"
	OUT_FILE="$REPORTS_DIR/summary.json"
else
	OUT_FILE="$PROJECTS_DIR/$OUT_NAME"
fi

# Entries that mxcli init (and mxcli itself, during report/lint runs) can
# scaffold into a project directory. These are tooling artifacts, not part
# of the Mendix app -- make sure they never get committed to that
# project's own git repo.
GITIGNORE_ENTRIES=(
	"/.ai-context"
	"/.claude"
	"/.mxcli"
	"/.playwright"
	"AGENTS.md"
	"CLAUDE.md"
	"/.local"
	"/.devcontainer"
	"/node_modules"
	"/javascriptsource/**/android/"
	"/*.launch"
)

ensure_gitignore() {
	local project_dir="$1"
	local project_name="$2"
	local gitignore_file="${project_dir}.gitignore"

	touch "$gitignore_file"

	local to_add=()
	for entry in "${GITIGNORE_ENTRIES[@]}"; do
		if ! grep -qxF -- "$entry" "$gitignore_file"; then
			to_add+=("$entry")
		fi
	done

	if [ ${#to_add[@]} -gt 0 ]; then
		{
			echo ""
			echo "# mxcli / AI-assisted tooling (added by mxcli-batch.sh)"
			printf '%s\n' "${to_add[@]}"
		} >> "$gitignore_file"
		echo "  Updated .gitignore in '$project_name' (+${#to_add[@]} entries)" >&2
	fi
}

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

# Each successfully-processed project's report JSON gets appended as one
# line here (JSON Lines) -- avoids fragile manual array/comma bookkeeping.
COLLECTED="$TMP_DIR/collected.jsonl"
: > "$COLLECTED"

count=0
skipped=0

for project_dir in "$PROJECTS_DIR"/*/; do
	project_name=$(basename "$project_dir")

	# In full mode, the output folder itself lives inside PROJECTS_DIR and
	# would otherwise get picked up by this same glob as if it were a
	# project -- skip it silently, it's not a project.
	if [ "$MODE" = "full" ] && [ "${project_dir%/}" = "${REPORTS_DIR%/}" ]; then
		continue
	fi

	mpr_file=$(find "$project_dir" -maxdepth 1 -iname "*.mpr" | head -1)
	if [ -z "$mpr_file" ]; then
		echo "  skipped '$project_name' (no .mpr found)" >&2
		skipped=$((skipped + 1))
		continue
	fi

	# Auto-initialize any project missing Starlark lint rules, so every
	# project contributes the same rule coverage regardless of mode.
	# --force-init bypasses the "already has rules" check entirely and
	# wipes the directory first, so a stale copy of an old .star rule
	# can never survive a forced re-init.
	lint_rules_dir="${project_dir}.claude/lint-rules"
	run_init=false
	if [ "$FORCE_INIT" = true ]; then
		run_init=true
		echo "  '$project_name': --force-init set, refreshing lint rules..." >&2
		rm -rf "$lint_rules_dir"
	elif [ ! -d "$lint_rules_dir" ] || [ -z "$(ls -A "$lint_rules_dir" 2>/dev/null)" ]; then
		run_init=true
		echo "  '$project_name' has no .claude/lint-rules -- running mxcli init..." >&2
	fi

	if [ "$run_init" = true ]; then
		if ! "$MXCLI" init "$project_dir" >"$TMP_DIR/${project_name}_init.log" 2>&1; then
			echo "  skipped '$project_name' (mxcli init failed -- see below)" >&2
			sed 's/^/    /' "$TMP_DIR/${project_name}_init.log" >&2 || true
			skipped=$((skipped + 1))
			continue
		fi
	fi
	ensure_gitignore "$project_dir" "$project_name"

	echo "Assessing '$project_name'..." >&2
	safe_name=$(echo "$project_name" | tr -c '[:alnum:]_-' '_')
	out_json="$TMP_DIR/$safe_name.json"

	if [ "$MODE" = "calibrate" ]; then
		REPORT_FLAGS=(--raw --format json)
	else
		# summary/full: the ACTUAL scored report, not raw calibration data.
		REPORT_FLAGS=(--format json)
	fi
	REPORT_FLAGS+=("${EXCLUDE_FLAGS[@]}")

	if ! "$MXCLI" report -p "$mpr_file" "${REPORT_FLAGS[@]}" -o "$out_json" 2>"$TMP_DIR/$safe_name.err"; then
		echo "  skipped '$project_name' (report failed -- see below)" >&2
		sed 's/^/    /' "$TMP_DIR/$safe_name.err" >&2 || true
		skipped=$((skipped + 1))
		continue
	fi

	if [ ! -s "$out_json" ]; then
		echo "  skipped '$project_name' (mxcli exited 0 but wrote no output)" >&2
		skipped=$((skipped + 1))
		continue
	fi

	# full mode: keep the complete, unmodified per-app report alongside
	# the eventual summary, named "<app-name>-report.json".
	if [ "$MODE" = "full" ]; then
		cp "$out_json" "$REPORTS_DIR/${project_name}-report.json"
	fi

	python3 -c "import json,sys; print(json.dumps(json.load(open(sys.argv[1]))))" "$out_json" >> "$COLLECTED"
	count=$((count + 1))
done

echo "" >&2
echo "Collected $count project(s), skipped $skipped. Building output..." >&2

# Post-process the collected JSON Lines into the final output shape.
# calibrate keeps its own per-category summary/average math (unscored
# rates); summary/full pass mxcli's own already-scored report objects
# through unchanged -- no re-scoring happens in this script.
python3 - "$COLLECTED" "$OUT_FILE" "$MODE" << 'PYEOF'
import json, sys
from collections import defaultdict

collected_path, out_path, mode = sys.argv[1], sys.argv[2], sys.argv[3]

projects = []
with open(collected_path) as f:
	for line in f:
		line = line.strip()
		if line:
			projects.append(json.loads(line))

if mode == "calibrate":
	sums = defaultdict(lambda: {
		"n": 0, "elementsChecked": 0, "errors": 0, "warnings": 0,
		"infos": 0, "penalty": 0.0, "rate": 0.0,
	})
	for proj in projects:
		for cat in proj.get("categories", []):
			name = cat.get("name", "Unknown")
			s = sums[name]
			s["n"] += 1
			s["elementsChecked"] += cat.get("elementsChecked", 0)
			s["errors"] += cat.get("errors", 0)
			s["warnings"] += cat.get("warnings", 0)
			s["infos"] += cat.get("infos", 0)
			s["penalty"] += cat.get("penalty", 0.0)
			s["rate"] += cat.get("rate", 0.0)

	summary = {}
	for name, s in sorted(sums.items()):
		n = s["n"] or 1
		summary[name] = {
			"projectsWithData": s["n"],
			"avgElementsChecked": round(s["elementsChecked"] / n, 1),
			"avgErrors": round(s["errors"] / n, 2),
			"avgWarnings": round(s["warnings"] / n, 2),
			"avgInfos": round(s["infos"] / n, 2),
			"avgPenalty": round(s["penalty"] / n, 3),
			"avgRate": round(s["rate"] / n, 4),
		}

	output = {"summary": summary, "projectCount": len(projects), "projects": projects}

else:
	# summary/full: mxcli's own scored Report objects, combined as-is.
	# Also compute a simple across-fleet average per category score, since
	# that's the one useful cross-project rollup that doesn't require
	# re-deriving anything mxcli already scored.
	cat_scores = defaultdict(list)
	for proj in projects:
		for cat in proj.get("categories", []):
			cat_scores[cat.get("name", "Unknown")].append(cat.get("score", 0))

	summary = {
		name: {"projectsWithData": len(scores), "avgScore": round(sum(scores) / len(scores), 1)}
		for name, scores in sorted(cat_scores.items())
	}

	output = {"summary": summary, "projectCount": len(projects), "apps": projects}

with open(out_path, "w") as f:
	json.dump(output, f, indent=2)

print(f"Wrote {out_path}", file=sys.stderr)
PYEOF

if [ "$MODE" = "full" ]; then
	echo "Done: $count project(s) collected, $skipped skipped -> $REPORTS_DIR/ (summary.json + per-app reports)"
else
	echo "Done: $count project(s) collected, $skipped skipped -> $OUT_FILE"
fi
