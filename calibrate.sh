#!/usr/bin/env bash
# calibrate.sh -- run `mxcli report --raw` across a fleet of .mpr files,
# auto-initializing any project missing .claude/lint-rules first, and
# collect per-category penalty/rate data plus a cross-project summary.
#
# Expects a directory containing one subfolder per project, each subfolder
# containing exactly one .mpr file, e.g.:
#   MAIN/
#     GBS - Human in the Loop (HITL)-main/CANValidator.mpr
#     TOPS-main/SomeApp.mpr
#     ...
#
# Output is written inside <projects-dir> itself (default filename
# calibration.json), not wherever the script happens to be run from.
#
# Usage: ./calibrate.sh <mxcli-binary> <projects-dir> [output-filename]
set -euo pipefail

MXCLI="${1:?Usage: calibrate.sh <path-to-mxcli-binary> <projects-dir> [output-filename]}"
PROJECTS_DIR="${2:?Usage: calibrate.sh <path-to-mxcli-binary> <projects-dir> [output-filename]}"
OUT_NAME="${3:-calibration.json}"

# Strip any trailing slash so path joins below don't produce a double "//".
PROJECTS_DIR="${PROJECTS_DIR%/}"
OUT_FILE="$PROJECTS_DIR/$OUT_NAME"

# Entries that mxcli init (and mxcli itself, during report/lint runs) can
# scaffold into a project directory. These are calibration/AI-tooling
# artifacts, not part of the Mendix app -- make sure they never get
# committed to that project's own git repo.
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

# Idempotently ensures a project's .gitignore contains the entries above,
# without duplicating any that are already present (exact-line match).
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
			echo "# mxcli / AI-assisted tooling (added by calibrate.sh)"
			printf '%s\n' "${to_add[@]}"
		} >> "$gitignore_file"
		echo "  Updated .gitignore in '$project_name' (+${#to_add[@]} entries)" >&2
	fi
}

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

# Each successfully-reported project's raw JSON gets appended as one line
# here (JSON Lines), rather than hand-building a JSON array with manual
# comma bookkeeping -- simpler and harder to get subtly wrong.
COLLECTED="$TMP_DIR/collected.jsonl"
: > "$COLLECTED"

count=0
skipped=0

# One project per subfolder; find its .mpr file (should be exactly one).
for project_dir in "$PROJECTS_DIR"/*/; do
	project_name=$(basename "$project_dir")

	# Find the .mpr file inside this project folder (non-recursive, in case
	# of nested backup copies -- adjust -maxdepth if yours are deeper).
	mpr_file=$(find "$project_dir" -maxdepth 1 -iname "*.mpr" | head -1)

	if [ -z "$mpr_file" ]; then
		echo "  skipped '$project_name' (no .mpr found)" >&2
		skipped=$((skipped + 1))
		continue
	fi

	# If this project has never had mxcli init run against it, its Starlark
	# rules (CONV001-010, CONV015-017) won't be loaded, so it'll silently
	# report on fewer categories than an initialized project -- skewing any
	# cross-project comparison. Initialize on the fly so every project
	# contributes the same rule coverage to the calibration set.
	lint_rules_dir="${project_dir}.claude/lint-rules"
	if [ ! -d "$lint_rules_dir" ] || [ -z "$(ls -A "$lint_rules_dir" 2>/dev/null)" ]; then
		echo "  '$project_name' has no .claude/lint-rules -- running mxcli init..." >&2
		if ! "$MXCLI" init "$project_dir" >"$TMP_DIR/${project_name}_init.log" 2>&1; then
			echo "  skipped '$project_name' (mxcli init failed -- see below)" >&2
			sed 's/^/    /' "$TMP_DIR/${project_name}_init.log" >&2 || true
			skipped=$((skipped + 1))
			continue
		fi
	fi

	# Run regardless of whether init just ran above -- covers projects that
	# were already initialized previously (manually, or by an earlier run
	# of this script before this .gitignore step existed).
	ensure_gitignore "$project_dir" "$project_name"

	echo "Assessing '$project_name'..." >&2
	safe_name=$(echo "$project_name" | tr -c '[:alnum:]_-' '_')
	out_json="$TMP_DIR/$safe_name.json"

	if ! "$MXCLI" report -p "$mpr_file" --raw --format json -o "$out_json" 2>"$TMP_DIR/$safe_name.err"; then
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

	# Re-serialize compactly onto one line and append.
	python3 -c "import json,sys; print(json.dumps(json.load(open(sys.argv[1]))))" "$out_json" >> "$COLLECTED"
	count=$((count + 1))
done

echo "" >&2
echo "Collected $count project(s), skipped $skipped. Building summary..." >&2

# Post-process: read the JSON-Lines file, compute per-category averages
# across all projects, and write { summary, projectCount, projects } to
# OUT_FILE. This is pure post-processing of mxcli's own --raw output --
# no mxcli code involved, so no rebuild is needed to change this shape.
python3 - "$COLLECTED" "$OUT_FILE" << 'PYEOF'
import json, sys
from collections import defaultdict

collected_path, out_path = sys.argv[1], sys.argv[2]

projects = []
with open(collected_path) as f:
	for line in f:
		line = line.strip()
		if line:
			projects.append(json.loads(line))

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

output = {
	"summary": summary,
	"projectCount": len(projects),
	"projects": projects,
}

with open(out_path, "w") as f:
	json.dump(output, f, indent=2)

print(f"Wrote {out_path}", file=sys.stderr)
PYEOF

echo "Done: $count project(s) collected, $skipped skipped -> $OUT_FILE"
