# Rework `mxcli git merge` per ako's review (PR #1674)

Decisions (user, 2026-10-30): switch the engine call to `mx merge` (folder mode);
keep `--ours`/`--theirs` but limited to conflicted units with a hash gate; one
push after the rework passes `make ci-local`.

## Review claims → verified

1. --ours/--theirs take App.mpr wholesale → orphan/missing documents. TRUE (strategy path, cmd_git_merge.go).
2. --plan takes ours' App.mpr → refuses on theirs' adds; error paths leave MERGE_HEAD. TRUE (applyPlan; abort only on stale).
3. Every non-zero mx exit treated as decision-needed. TRUE (code != 0 → plan + decision error; 129 = crash).
4. mx git-merge broken on Linux ("Git not found", 129). Reviewer-measured; repo finding corroborates mx merge works (0/2/129).
5. Should-fix: upstream merge conflicts (DONE, 2ef7e1068); merge-plan.json not junk-pathed; mixed-conflict advice; PrepareMxCommand nit.

## Changes

### A. Engine interface (cmd_git_merge.go)
- `mxMergeRunner func(baseDir, mineDir, theirsDir string, shas [3]string) (int, string, error)` — folders.
- realMxMerge: `mx merge <baseDir> <mineDir> <theirsDir>` + docker.PrepareMxCommand (11.10 LD_PRELOAD nit).
- baseDir/theirsDir: temp worktrees (unchanged); project dir inside = filepath.Join(dir, projDirRel).
- mineDir = the working project dir (filepath.Dir(abs mpr)).

### B. Exit-code honesty
- 0 → stage model paths → fixHashes gate (MISSING/ORPHAN → refuse + git merge --abort) → commitTheMerge.
- 2 → property report (mx diff) + plan template (units = engine JSON's manual units; fallback git's conflicted ids) + git merge --abort + decision error.
- other (129…) → "mx merge failed (exit N)" + git merge --abort. Never a plan, never a decision suggestion.

### C. --ours/--theirs limited
- No engine call (unchanged). Conflicted UNITS only from git's list (skip the .mpr).
- App.mpr takes the flag's side; then fixHashes gate: MISSING/ORPHAN → refuse + git merge --abort (loud, Studio Pro pointer). No silent drops.

### D. --plan aborts everywhere
- invalid JSON / invalid take / not-modified-on-both / fixHashes refusal → git merge --abort before returning.
- stale-plan path already aborts (keep).

### E. Re-entry
- At entry (non-plan): if MERGE_HEAD already exists, skip `git merge` and go straight to conflict classification. Supports: mixed conflict → resolve text files → `git add` → re-run command finishes the merge.

### F. Mixed-conflict advice
- Early-return kept, reworded: resolve the text files, `git add` them, then re-run `mxcli git merge <ref>` (it resumes, resolves the model, commits).

### G. merge-plan.json is junk
- isJunkPath: `<anything>.merge-plan.json` never staged by `mxcli git commit`.

### H. Words
- Long help, CHANGELOG git-merge entry, docs-site outside-studio-pro.md, teamserver-git SKILL.md "aborts the merge cleanly" — match the new reality.

### I. Tests (gitmerge_test.go)
- Runner fixtures → folder form (assert dirs end in the project folder, contain App.mpr).
- exit 129 → no plan text, MERGE_HEAD gone, tree restored, error names mx.
- --theirs with ours-added unit → refuse + abort (no ORPHAN commit, MERGE_HEAD gone).
- --plan error paths → MERGE_HEAD gone.
- re-entry: staged text resolution + MERGE_HEAD → finishes.
- isJunkPath: x.merge-plan.json → junk.

## Gate
- go vet ./cmd/mxcli/; focused go test; then make ci-local; then one push.
