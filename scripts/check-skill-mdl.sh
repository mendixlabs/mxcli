#!/usr/bin/env bash
# check-skill-mdl.sh — syntax-check the domain-model DDL blocks embedded in the
# user-facing skills AND the docs site, so invalid MDL can't drift into the docs.
# This is the class of drift seen in practice: associations documented with
# PARENT/CHILD or `[*] -> [1]`, enumerations with `= 'x'` or `as ( x: 'y' )`,
# ALTER ENTITY with `ADD (attr, ...)` instead of `ADD ATTRIBUTE attr: type`, etc.
#
# It recursively finds every *.md under the given directory, extracts each ```mdl
# and ```sql fenced block, and runs `mxcli check` (syntax only) on the individual
# statements whose first keyword is in scope — CREATE/ALTER/DROP of an entity,
# association, enumeration, or constant, PLUS complete CREATE/DROP of a page or
# snippet. Those are reliably complete and standalone; a bad statement inside a
# larger multi-document example is still caught (extraction is per-statement, not
# per-block). CREATE PAGE/SNIPPET is included because full page bodies are complete
# top-level statements — this is what catches page-action drift like
# `show_page X passing $obj` (the valid form is `show_page X(Param: $obj)`).
#
# Everything else is skipped on purpose, because skills legitimately show
# fragments that are NOT standalone top-level MDL: microflow activities
# (`change $x`, `show page X(...)`), ALTER PAGE operation fragments (`set ...`,
# `insert ...`), bare widget snippets, and the import-mapping / JSON-structure
# mini-DSL (`create entity X { ... }`). Blocks are also skipped when
# they are clearly illustrative: placeholders (`...`), syntax templates (` | `
# alternation, `<name>`, `[optional]`), brace-based mini-DSL (`{`/`}`), or a
# deliberately-wrong example (❌ / INCORRECT / WRONG / -- BAD).
#
# Usage: scripts/check-skill-mdl.sh [mxcli-binary] [docs-dir]
set -uo pipefail

MXCLI="${1:-bin/mxcli}"
SKILLS_DIR="${2:-.claude/skills/mendix}"

if [ ! -x "$MXCLI" ]; then
	echo "error: mxcli binary not found or not executable: $MXCLI (run 'make build')" >&2
	exit 2
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# A checkable statement's first keyword must be in scope. Two families:
#   DDL_RE  — the drift-prone domain model (entity / association / enumeration /
#             constant), for create / alter / drop.
#   PAGE_RE — complete CREATE / DROP of a page or snippet. A full `create page … { … }`
#             body is a standalone top-level statement, so it checks cleanly; this is
#             the family that catches page-action drift (e.g. `passing $obj`). ALTER
#             PAGE/SNIPPET is deliberately excluded — its blocks are operation
#             fragments (`set …`, `insert …`) that are not standalone.
# Out of scope entirely: security (`create module role`), microflows — fragment-heavy
# with their own syntax surface.
DDL_RE='^(create|alter|drop)( or (modify|replace))?( (persistent|non_persistent|view|external))? (entity|association|enumeration|constant)\b'
PAGE_RE='^(create|drop)( or (modify|replace))? (page|snippet)\b'

FAILED=0
CHECKED=0
SKIPPED=0

while IFS= read -r md; do
	[ -e "$md" ] || continue
	# Split the file into ```mdl / ```sql blocks, one file per block.
	rm -f "$WORK"/block_* 2>/dev/null || true
	awk -v dir="$WORK" '
		/^```(mdl|sql)[ \t]*$/ { inb=1; n++; fn=sprintf("%s/block_%03d.mdl", dir, n); next }
		inb && /^```/ { inb=0; next }
		inb { print > fn }
	' "$md"

	for blk in "$WORK"/block_*.mdl; do
		[ -e "$blk" ] || continue

		# Block-level skip: templates / placeholders, an explicit `-- check-skip`
		# opt-out, or a block that deliberately demonstrates a PARSE ERROR. Checked
		# on the whole block because these markers live in comments (stripped before
		# splitting). Note: "❌ INCORRECT" examples are NOT skipped — those are almost
		# always syntactically valid but semantically wrong (e.g. a reversed
		# association direction), so they pass a syntax check and their statements
		# should still be validated. Only genuinely syntax-broken demos are skipped.
		if grep -qE '\.\.\.| \| |<[a-zA-Z]|[$][{]' "$blk" ||
			grep -qiE 'check-skip|parse error' "$blk"; then
			SKIPPED=$((SKIPPED + 1)); continue
		fi

		# Split the block into individual statements so a bad statement inside a larger
		# multi-document example is still caught. DDL and page/snippet statements
		# terminate differently: DDL ends at a top-level `;`, but a `create page … { … }`
		# body ends at its matching `}` with NO `;`. The splitter scans character by
		# character tracking paren depth, brace depth, and quoted strings, so it can
		# tell a page BODY brace (at paren depth 0) from an inline map brace such as
		# `params: { $x: M.E }` (inside the header parens) or `params: {C: $c}` in a
		# widget arg. A statement flushes when its body brace closes, on a top-level `;`,
		# or on a standalone `/` separator — so a following statement (e.g. a
		# `create … navigation`) is never concatenated onto a page.
		rm -f "$WORK"/stmt_* 2>/dev/null || true
		awk -v dir="$WORK" '
			BEGIN { q = sprintf("%c", 39) }                   # single quote
			function flush(   fn) {
				if (buf ~ /[^[:space:]]/) {
					i++
					fn = sprintf("%s/stmt_%03d.mdl", dir, i)
					printf "%s\n", buf > fn
					close(fn)
				}
				buf = ""; bdepth = 0; pdepth = 0; bopened = 0; instr = 0
			}
			{
				line = $0
				sub(/[[:space:]]*--.*/, "", line)             # strip line comment
				if (line ~ /^[[:space:]]*\/[[:space:]]*$/) { flush(); next }  # `/` separator
				buf = buf line "\n"
				n = length(line)
				for (k = 1; k <= n; k++) {
					ch = substr(line, k, 1)
					if (instr) { if (ch == q) instr = 0; continue }  # skip string body
					if (ch == q) { instr = 1; continue }
					if (ch == "(") pdepth++
					else if (ch == ")") { if (pdepth > 0) pdepth-- }
					else if (pdepth == 0) {
						if (ch == "{") { bdepth++; bopened = 1 }
						else if (ch == "}") { if (bdepth > 0) bdepth-- }
						else if (ch == ";" && bdepth == 0) { flush(); break }
					}
				}
				if (bopened && bdepth == 0 && pdepth == 0) flush()   # body closed
			}
			END { flush() }' "$blk"

		for stmt in "$WORK"/stmt_*.mdl; do
			[ -e "$stmt" ] || continue
			first="$(grep -vE '^[[:space:]]*$' "$stmt" | head -1 | sed -E 's/^[[:space:]]+//' | tr 'A-Z' 'a-z')"
			# Only in-scope statements: domain-model DDL, or complete CREATE/DROP of a
			# page or snippet.
			printf '%s' "$first" | grep -qE "$DDL_RE|$PAGE_RE" || { SKIPPED=$((SKIPPED + 1)); continue; }
			# Skip the JSON-structure mini-DSL (`create entity X (NON_PERSISTENT)`).
			grep -qE 'NON_PERSISTENT\)' "$stmt" && { SKIPPED=$((SKIPPED + 1)); continue; }

			CHECKED=$((CHECKED + 1))
			out="$("$MXCLI" check "$stmt" 2>&1)"
			# Fail only on SYNTAX/parse errors — the drift this guard exists to
			# catch. Semantic validation (reserved attribute names, cross-references,
			# …) is context-dependent and noisy for isolated snippets, so it is not
			# enforced here.
			if printf '%s' "$out" | grep -qiE 'Syntax errors found|mismatched input|no viable alternative|extraneous input|expecting'; then
				FAILED=1
				echo "FAIL: $md — $(grep -vE '^[[:space:]]*$' "$stmt" | head -1 | sed -E 's/^[[:space:]]+//')"
				printf '%s\n' "$out" | grep -iE 'mismatched|expecting|no viable|extraneous' | grep -iv 'vibe' | sed 's/^/    /' | head -2
			fi
		done
	done
done < <(find "$SKILLS_DIR" -name '*.md' | sort)

echo "---"
echo "MDL blocks: checked $CHECKED, skipped $SKIPPED (illustrative/templates)"
if [ "$FAILED" -ne 0 ]; then
	echo "Some MDL blocks have invalid syntax (see above)."
	exit 1
fi
echo "All checkable MDL blocks pass 'mxcli check'."
