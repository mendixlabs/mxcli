// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	"github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/spf13/cobra"
)

// cmd_git_merge.go: `mxcli git merge <ref>` — merge a branch the way Studio
// Pro does. `git merge` runs first; when it stops on the model files (the
// .mpr is SQLite and every .mxunit is BSON, so any two-sided model change is
// a binary conflict), the three revisions are handed to `mx merge` — the same
// model-aware engine Studio Pro's own merge uses — as three .mpr paths over
// clean checkouts (base, ours at HEAD, theirs), never the working tree
// mid-merge, with the project's Mendix version's mx. The engine writes its
// result into MINE, which is copied back into the working tree. What the
// engine resolves is hash-checked, staged and committed as a normal
// two-parent merge commit with the mx_metadata note; what it reports as
// ManualResolutionRequired (exit 2) aborts the merge with the conflict list
// and a pointer to Studio Pro, leaving nothing behind; any other non-zero
// exit is the engine failing, reported as such, also with a clean abort.

var gitMergeCmd = &cobra.Command{
	Use:   "merge <ref>",
	Short: "Merge a branch with model-aware conflict resolution (mx merge)",
	Long: `Merge <ref> into the current branch, resolving model conflicts the way
Studio Pro does.

'git merge <ref>' runs first. A merge git resolves itself (disjoint files, a
fast-forward) is committed and annotated like 'mxcli git commit' does. When
git stops on the model files — the .mpr and the .mxunit units are binary, so
two sides changing the model always conflict — the base, ours (a clean
checkout of HEAD) and theirs revisions are checked out into temporary
worktrees and the three projects' .mpr files go to
'mx merge BASE MINE THEIRS' (Studio Pro's own model-aware merge, resolved
for the project's Mendix version). MINE is never the working tree mid-merge:
the engine chokes on the conflicted state. Its result is copied back:

  - everything the engine auto-resolves is copied back, hash-checked,
    staged and committed as the merge commit, with the mx_metadata note
    attached (this keeps both sides — the usual best outcome; the engine
    owns the .mpr, so a unit one side added while the other changed
    existing ones is kept, not dropped);
  - a conflict the engine reports as ManualResolutionRequired (exit 2) is
    never merged silently: the merge is aborted, and both sides' changes are
    printed for review — at property level, from 'mx diff' (e.g.
    "New type > Length: 100 → 150" on ours, "→ 200" on theirs). Then decide:
      --plan <file>  resolve per unit from the plan template written beside
                     the .mpr (<mpr>.merge-plan.json): set "take" to
                     "ours"/"theirs" per unit. The engine runs again and the
                     plan decides only the units it left open — everything
                     it merged stays merged, so the plan must name exactly
                     those units (a stale or hand-widened plan is refused).
                     Each planned unit is taken whole from its side, its
                     conflict marker cleared, and the .mpr index repaired.
                     Modified-on-both units only — added/deleted/moved
                     conflicts go to Studio Pro.
      --ours / --theirs   take one side of every conflicted unit
                     (deterministic; mx is not consulted again). The .mpr is
                     taken from that side too and the result is hash-checked:
                     when the sides' document sets differ (one added or
                     deleted a unit), the commit is refused — a whole side
                     would leave the other side's units orphaned on disk or
                     listed with no file — and the merge is aborted. Resolve
                     those in Studio Pro instead.
      Studio Pro (Version Control › Merge Changes Here) for mixed
                     within-unit resolutions — still the tool when a decision
                     must mix both sides inside one unit;
  - any other engine exit is mx failing, not a decision: it is reported and
    the merge is aborted cleanly;
  - a conflict in non-model files (text sources) is left in place for manual
    resolution, like plain git: resolve them, 'git add' them, and re-run
    this command — it finishes the merge (model conflicts included) and
    commits.

--ours/--theirs take one side of every conflicted unit — never a side of the
.mpr without the hash check, because the .mpr lists every document: a whole
side would drop the other side's added units and resurrect its deleted ones.

The merge is refused while Studio Pro has the project open (.mpr.lock).
MPR v2 projects only. The mx binary is resolved for the project's Mendix
version (PATH, the mxcli cache, or the Studio Pro installation).`,
	Example: `  mxcli git merge feature/orders -p app.mpr
  mxcli git merge origin/main -p app.mpr --push
  mxcli git merge feature/orders -p app.mpr --theirs   # after reviewing the report
  mxcli git merge feature/orders -p app.mpr --plan app.mpr.merge-plan.json`,
	Args:          cobra.ExactArgs(1),
	RunE:          runGitMerge,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	gitMergeCmd.Flags().StringP("project", "p", "", "path to the Mendix project (.mpr)")
	_ = gitMergeCmd.MarkFlagRequired("project")
	gitMergeCmd.Flags().StringP("message", "m", "", "merge commit message (default: git's)")
	gitMergeCmd.Flags().Bool("ours", false, "on a model conflict mx cannot resolve, take our side of every conflicted model file")
	gitMergeCmd.Flags().Bool("theirs", false, "on a model conflict mx cannot resolve, take their side of every conflicted model file")
	gitMergeCmd.Flags().String("plan", "", "resolve per unit from a plan file written by a previous run (<mpr>.merge-plan.json)")
	gitMergeCmd.Flags().Bool("push", false, "push the branch and the notes ref to their remote afterwards")
	gitCmd.AddCommand(gitMergeCmd)
}

func runGitMerge(cmd *cobra.Command, args []string) error {
	ref := args[0]
	message, _ := cmd.Flags().GetString("message")
	ours, _ := cmd.Flags().GetBool("ours")
	theirs, _ := cmd.Flags().GetBool("theirs")
	plan, _ := cmd.Flags().GetString("plan")
	push, _ := cmd.Flags().GetBool("push")
	project, _ := cmd.Flags().GetString("project")
	strategy := ""
	switch {
	case ours && theirs:
		return errors.New("--ours and --theirs cannot be combined")
	case ours && plan != "", theirs && plan != "":
		return errors.New("--plan cannot be combined with --ours/--theirs: the plan is the finer decision")
	case ours:
		strategy = "ours"
	case theirs:
		strategy = "theirs"
	}
	return mergeRun(project, ref, message, strategy, plan, push, mxRunners{merge: realMxMerge, diff: realMxDiff}, cmd.OutOrStdout())
}

// mxRunners are the mx invocations, injected for tests.
//
// merge runs the engine as `mx merge -v BASE MINE THEIRS` over the three
// revisions' project files — clean checkouts of base, ours (HEAD) and
// theirs — and writes its result into MINE. The -v conflict list (one JSON
// array on one stdout line) is what the review step reports from. Exit 0 is
// a resolved merge, 2 is a decision the engine could not make, anything else
// is the engine failing.
type mxMergeRunner func(baseMpr, mineMpr, theirsMpr string, shas [3]string) (int, string, error)
type mxDiffRunner func(base, mine, outJSON string) error

type mxRunners struct {
	merge mxMergeRunner
	diff  mxDiffRunner
}

// mergeRun is the command without its wiring. runners are injected for tests.
//
// Conflict handling is a three-step flow a human stays in charge of:
//
//  1. `git merge` runs; a merge it resolves itself is annotated and done.
//  2. A model conflict goes to `mx merge` over the three revisions' .mpr
//     files (clean worktrees; the result is copied back from MINE) first —
//     what the engine auto-resolves keeps both sides, which is almost
//     always the best outcome — and is committed with the note.
//  3. What the engine reports as ManualResolutionRequired (exit 2) is never
//     merged silently: with no strategy the merge is aborted and both sides'
//     changes are printed for review; `--plan` then resolves the units the
//     engine left open, over the engine's own result, and
//     `--ours`/`--theirs` take one side of every conflicted unit, hash-gated
//     so a side that would drop or orphan documents is refused. Any other
//     engine exit is a failure, not a decision: reported, aborted cleanly.
func mergeRun(mprPath, ref, message, strategy, plan string, push bool, runners mxRunners, out io.Writer) error {
	switch strategy {
	case "", "ours", "theirs":
	default:
		return fmt.Errorf("unknown strategy %q (use --ours or --theirs)", strategy)
	}
	abs, err := filepath.Abs(mprPath)
	if err != nil {
		return err
	}
	if !mpr.AllowWritesWhileStudioProOpen && os.Getenv(mpr.AllowStudioProOpenEnv) != "1" {
		if lock, mod := mpr.StudioProLockFile(abs); lock != "" {
			return &mpr.StudioProOpenError{MprPath: abs, LockPath: lock, LockTime: mod}
		}
	}
	repo, err := openGitNoteRepo(mprPath)
	if err != nil {
		return err
	}
	defer repo.Close()
	root := repo.Root

	branch := strings.TrimSpace(gitOutOr(root, "symbolic-ref", "--quiet", "--short", "HEAD"))
	if branch == "" {
		return errors.New("detached HEAD: check out a branch before merging")
	}

	if plan != "" {
		return applyPlan(root, repo, abs, mprPath, ref, plan, message, branch, push, runners, out)
	}

	// A merge already in progress is resumed, not re-run: a previous run may
	// have stopped on text conflicts, which the human resolved and staged
	// since. Re-running git merge would only refuse ("not concluded").
	if _, mhErr := gitOut(root, "rev-parse", "--verify", "--quiet", "MERGE_HEAD"); mhErr != nil {
		mergeOut, mergeErr := gitOutIn(root, nil, "merge", "--no-edit", ref)
		if mergeErr == nil {
			if strings.Contains(mergeOut, "Already up to date") {
				fmt.Fprintln(out, "Already up to date.")
				return nil
			}
			head := strings.TrimSpace(gitOutOr(root, "rev-parse", "HEAD"))
			fmt.Fprintf(out, "Merged %s without model conflicts (%s).\n", ref, shortCommit(head))
			return finishMerge(out, repo, head, branch, push)
		}
		// A ref that does not resolve fails before any conflict exists, and
		// so does a merge git refuses to start (local changes it would
		// overwrite, no identity): git's reason is on stderr, which is the
		// error — stdout holds at most an "Updating a..b" line.
		if _, mhErr := gitOut(root, "rev-parse", "--verify", "--quiet", "MERGE_HEAD"); mhErr != nil {
			return fmt.Errorf("git merge %s: %v", ref, mergeErr)
		}
	}

	conflicted := nulFields(gitOutOr(root, "diff", "--name-only", "--diff-filter=U", "-z"))
	var model, other []string
	for _, p := range conflicted {
		if isModelPath(repo, p) {
			model = append(model, p)
		} else {
			other = append(other, p)
		}
	}
	if len(other) > 0 {
		fmt.Fprintln(out, "Non-model conflicts, resolve by hand, then 'git add' them and re-run this command to resolve the model and commit:")
		for _, p := range other {
			fmt.Fprintf(out, "  %s\n", p)
		}
		return errors.New("merge has non-model conflicts left for manual resolution")
	}

	baseSha := strings.TrimSpace(gitOutOr(root, "merge-base", "HEAD", "MERGE_HEAD"))
	oursSha := strings.TrimSpace(gitOutOr(root, "rev-parse", "HEAD"))
	theirsSha := strings.TrimSpace(gitOutOr(root, "rev-parse", "MERGE_HEAD"))

	// The human has decided: take one side of every conflicted model file. mx
	// is not consulted — `git checkout --ours/--theirs` reads the conflicted
	// stages directly. The .mpr is taken from the side too, and the result is
	// hash-checked before committing: a whole side includes the .mpr's list of
	// documents, so when the sides' document sets differ (one side added or
	// deleted a unit) committing would orphan the other side's units or list
	// units with no file. That refusal aborts the merge — it never commits a
	// half-project.
	if strategy != "" {
		side := "--" + strategy
		for _, p := range model {
			if _, err := gitOutIn(root, nil, "checkout", side, "--", p); err != nil {
				return abortMerge(root, fmt.Errorf("take the %s version of %s: %w", strategy, p, err))
			}
		}
		if err := repairOrRefuse(root, abs, out); err != nil {
			return err
		}
		return commitTheMerge(root, repo, model, message, branch, push, out)
	}

	// The engine first: what it auto-resolves keeps both sides, which is
	// almost always better than taking one side wholesale. The three
	// revisions are checked out as clean worktrees; MINE is the one at HEAD,
	// never the working tree mid-merge — the engine chokes on the conflicted
	// state (measured: it exits 129 when theirs deleted a document). Its
	// result lands in MINE and is copied back.
	revs, err := checkoutRevisions(root, ref, baseSha, oursSha, theirsSha)
	if err != nil {
		return err
	}
	tmp, baseDir, oursDir, theirsDir := revs.tmp, revs.baseDir, revs.oursDir, revs.theirsDir
	cleanWorktrees := revs.cleanup

	version := projectMendixVersion(mprPath)
	fmt.Fprintf(out, "Model files conflict (%s); merging the model with mx %s…\n",
		strings.Join(shortNames(model), ", "), version)
	code, mxOut, runErr := revs.runEngine(runners, repo, out)
	if runErr != nil {
		cleanWorktrees()
		return abortMerge(root, fmt.Errorf("mx merge failed to run: %w", runErr))
	}
	if code != 0 && code != 2 {
		// A crash, an unreadable base, anything but a merge decision: report
		// the engine as failing and leave nothing behind. Only exit 2 asks
		// for a decision.
		cleanWorktrees()
		return abortMerge(root, fmt.Errorf("mx merge failed (exit %d); the merge is aborted — if it persists, resolve in Studio Pro (Version Control › Merge Changes Here)", code))
	}
	if code == 2 {
		// The review step: what each side changed, at property level (mx
		// diff), plus a per-unit plan template. Then leave nothing behind —
		// the human decides. The unit list follows the engine's own conflict
		// list — git's can differ (a stale one-side index makes the engine
		// content-merge a unit git sees as changed). Without a parsable
		// list, the units the engine's result marks; without those, git's
		// conflicted units: err toward showing work rather than hiding it.
		// --plan reads the same set back, so the template it is given
		// matches what it checks the plan against.
		oursMpr := filepath.Join(oursDir, filepath.FromSlash(repo.MPRPath))
		manual := engineDecisions(mxOut, oursMpr)
		if len(manual) == 0 {
			manual = conflictedUnitIDs(model)
		}
		if runners.diff != nil {
			oursJSON := filepath.Join(tmp, "ours-diff.json")
			theirsJSON := filepath.Join(tmp, "theirs-diff.json")
			d1 := runners.diff(filepath.Join(baseDir, filepath.FromSlash(repo.MPRPath)), oursMpr, oursJSON)
			d2 := runners.diff(filepath.Join(baseDir, filepath.FromSlash(repo.MPRPath)), filepath.Join(theirsDir, filepath.FromSlash(repo.MPRPath)), theirsJSON)
			if d1 == nil && d2 == nil {
				fmt.Fprintln(out, propertyReport(oursJSON, theirsJSON, manual))
			}
		}
		if err := writePlanTemplate(mprPath, baseSha, oursSha, theirsSha, manual); err != nil {
			fmt.Fprintf(out, "  (could not write the plan template: %v)\n", err)
		} else {
			fmt.Fprintf(out, "Plan template: %s — set \"take\" per unit, re-run with --plan.\n", filepath.Base(mprPath)+".merge-plan.json")
		}
		cleanWorktrees()
		if _, abortErr := gitOutIn(root, nil, "merge", "--abort"); abortErr != nil {
			return fmt.Errorf("the model merge has conflicts and aborting it failed (%v); resolve in Studio Pro", abortErr)
		}
		fmt.Fprintln(out, "Merge aborted; the working tree is back to the pre-merge state.")
		return errors.New("the model merge needs a decision: re-run with --plan to resolve per unit, or resolve in Studio Pro (Version Control › Merge Changes Here)")
	}

	// Exit 0: the engine resolved the model into MINE, the clean worktree at
	// HEAD. Copy its result into the working tree — units the result dropped
	// go with it — gate the .mpr, and only then stage and commit: the gate
	// repairs hashes from the files, so staging must come after it, or the
	// commit would carry the pre-repair .mpr.
	if err := copyEngineResult(root, repo, oursDir); err != nil {
		cleanWorktrees()
		return abortMerge(root, fmt.Errorf("copy the engine's merge result: %w", err))
	}
	if err := repairOrRefuse(root, abs, out); err != nil {
		cleanWorktrees()
		return err
	}
	// Stage the whole model surface, not just the conflicted paths: a unit
	// the engine's result dropped was never conflicted, and its deletion
	// must be staged too.
	if _, err := gitOutIn(root, nil, "add", "-A", "--",
		filepath.FromSlash(repo.MPRPath), filepath.FromSlash(repo.Contents)); err != nil {
		cleanWorktrees()
		return abortMerge(root, fmt.Errorf("stage the resolved model files: %w", err))
	}
	if err := commitTheMerge(root, repo, nil, message, branch, push, out); err != nil {
		cleanWorktrees()
		return err
	}
	cleanWorktrees()
	return nil
}

// mergeRevisions are clean worktrees of a merge's three revisions — the
// engine's BASE, MINE (ours, at HEAD) and THEIRS — under one temp folder.
type mergeRevisions struct {
	root, tmp                   string
	baseDir, oursDir, theirsDir string
	shas                        [3]string
}

// checkoutRevisions checks the merge's three revisions out as detached
// worktrees; cleanup removes them again.
func checkoutRevisions(root, ref, baseSha, oursSha, theirsSha string) (*mergeRevisions, error) {
	tmp, err := os.MkdirTemp("", "mxcli-gitmerge-")
	if err != nil {
		return nil, err
	}
	m := &mergeRevisions{
		root: root, tmp: tmp,
		baseDir:   filepath.Join(tmp, "base"),
		oursDir:   filepath.Join(tmp, "ours"),
		theirsDir: filepath.Join(tmp, "theirs"),
		shas:      [3]string{baseSha, oursSha, theirsSha},
	}
	if _, err := gitOutIn(root, nil, "worktree", "add", "--detach", m.baseDir, baseSha); err != nil {
		os.RemoveAll(tmp)
		return nil, fmt.Errorf("check out the merge base: %w", err)
	}
	if _, err := gitOutIn(root, nil, "worktree", "add", "--detach", m.oursDir, oursSha); err != nil {
		_, _ = gitOutIn(root, nil, "worktree", "remove", "--force", m.baseDir)
		os.RemoveAll(tmp)
		return nil, fmt.Errorf("check out HEAD: %w", err)
	}
	if _, err := gitOutIn(root, nil, "worktree", "add", "--detach", m.theirsDir, theirsSha); err != nil {
		_, _ = gitOutIn(root, nil, "worktree", "remove", "--force", m.baseDir)
		_, _ = gitOutIn(root, nil, "worktree", "remove", "--force", m.oursDir)
		os.RemoveAll(tmp)
		return nil, fmt.Errorf("check out %s: %w", ref, err)
	}
	return m, nil
}

func (m *mergeRevisions) cleanup() {
	_, _ = gitOutIn(m.root, nil, "worktree", "remove", "--force", m.baseDir)
	_, _ = gitOutIn(m.root, nil, "worktree", "remove", "--force", m.oursDir)
	_, _ = gitOutIn(m.root, nil, "worktree", "remove", "--force", m.theirsDir)
	_ = os.RemoveAll(m.tmp)
}

// runEngine runs the engine in file form — the three revisions' project
// files, MINE the clean worktree at HEAD — and echoes what it printed. Its
// result lands in MINE.
func (m *mergeRevisions) runEngine(runners mxRunners, repo *gitNoteRepo, out io.Writer) (int, string, error) {
	mprIn := func(dir string) string { return filepath.Join(dir, filepath.FromSlash(repo.MPRPath)) }
	code, mxOut, err := runners.merge(mprIn(m.baseDir), mprIn(m.oursDir), mprIn(m.theirsDir), m.shas)
	if s := strings.TrimSpace(mxOut); s != "" {
		fmt.Fprintln(out, s)
	}
	return code, mxOut, err
}

// engineDecisions is the set of units the engine left for a human after an
// exit 2: the ManualResolutionRequired entries of its -v list or, when it
// printed no list, the units its result (mineMpr) marks as conflicted.
func engineDecisions(mxOut, mineMpr string) []string {
	if ids := engineManualIDs(parseEngineConflicts(mxOut)); len(ids) > 0 {
		return ids
	}
	marks, err := conflictMarkers(mineMpr)
	if err != nil {
		return nil
	}
	var ids []string
	for _, c := range marks {
		ids = append(ids, c.UnitID)
	}
	return ids
}

// conflictMarkers lists the units a project's index marks as conflicted —
// what the engine writes for a decision it did not make, and what Studio Pro
// reports as CE0137 until it is gone.
func conflictMarkers(mprPath string) ([]mpr.UnitConflict, error) {
	r, err := mpr.Open(mprPath)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return r.UnitConflicts()
}

// copyEngineResult copies the engine's merged model files — the .mpr and
// every .mxunit under mprcontents — from the MINE worktree into the working
// tree, removing working-tree units the result no longer has (a deletion the
// engine resolved). Non-model files are untouched, so a human's staged text
// resolutions survive as they are.
func copyEngineResult(root string, repo *gitNoteRepo, mineDir string) error {
	srcContents := filepath.Join(mineDir, filepath.FromSlash(repo.Contents))
	dstContents := filepath.Join(root, filepath.FromSlash(repo.Contents))
	if err := copyFile(
		filepath.Join(mineDir, filepath.FromSlash(repo.MPRPath)),
		filepath.Join(root, filepath.FromSlash(repo.MPRPath)),
	); err != nil {
		return err
	}
	present := map[string]bool{}
	err := filepath.WalkDir(srcContents, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".mxunit") {
			return nil
		}
		rel, err := filepath.Rel(srcContents, p)
		if err != nil {
			return err
		}
		present[filepath.ToSlash(rel)] = true
		return copyFile(p, filepath.Join(dstContents, rel))
	})
	if err != nil {
		return err
	}
	return filepath.WalkDir(dstContents, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".mxunit") {
			return nil
		}
		rel, err := filepath.Rel(dstContents, p)
		if err != nil {
			return err
		}
		if !present[filepath.ToSlash(rel)] {
			return os.Remove(p)
		}
		return nil
	})
}

func copyFile(src, dst string) error {
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, raw, 0o644)
}

// abortMerge backs a failed resolution out entirely — working tree, index and
// MERGE_HEAD return to the pre-merge state — and returns the original error.
// Every refusal on this command ends here: a half-done merge makes Studio Pro
// re-run it (and choke) every time the project opens.
func abortMerge(root string, err error) error {
	if _, abortErr := gitOutIn(root, nil, "merge", "--abort"); abortErr != nil {
		return fmt.Errorf("%w (aborting the merge failed too: %v — resolve it in Studio Pro)", err, abortErr)
	}
	return err
}

// repairOrRefuse is the hash gate every side- or plan-based resolution ends
// with: ContentsHash values are repaired from the files on disk (the files
// are the truth once units have been taken from both sides), and a MISSING or
// ORPHAN entry — a unit the .mpr lists with no file behind it, or a file no
// .mpr entry lists — refuses the commit: committing it would drop or orphan
// documents. The refusal aborts the merge.
func repairOrRefuse(root, mprPath string, out io.Writer) error {
	err := fixHashes(mprPath, true, out)
	if err == nil {
		return nil
	}
	if !errors.Is(err, errHashIssuesRemain) {
		return abortMerge(root, err)
	}
	return abortMerge(root, errors.New("refusing to commit: the ContentsHash index has MISSING or ORPHAN entries (see above) — the sides' document sets differ, and committing would drop or orphan units; resolve in Studio Pro (Version Control › Merge Changes Here)"))
}

// commitTheMerge commits the resolved merge (MERGE_HEAD is present), writes
// the note and pushes. stagedPaths, when given, are staged first.
func commitTheMerge(root string, repo *gitNoteRepo, stagedPaths []string, message, branch string, push bool, out io.Writer) error {
	if stagedPaths != nil {
		args := []string{"add", "--"}
		args = append(args, stagedPaths...)
		if _, err := gitOutIn(root, nil, args...); err != nil {
			return fmt.Errorf("stage the resolved model files: %w", err)
		}
	}
	commitArgs := []string{"commit", "--no-edit"}
	if message != "" {
		commitArgs = []string{"commit", "-m", message}
	}
	if _, err := gitOutIn(root, nil, commitArgs...); err != nil {
		return fmt.Errorf("commit the merge: %w", err)
	}

	head := strings.TrimSpace(gitOutOr(root, "rev-parse", "HEAD"))
	fmt.Fprintf(out, "Model merge committed as %s.\n", shortCommit(head))
	return finishMerge(out, repo, head, branch, push)
}

// finishMerge attaches the mx_metadata note to the finished merge and prints
// (or, with push, runs) the branch-and-notes push.
func finishMerge(out io.Writer, repo *gitNoteRepo, head, branch string, push bool) error {
	if err := annotateCommits(out, repo, []string{head}, branch, true, false); err != nil {
		return fmt.Errorf("committed %s, but attaching the note failed (%v) — run 'mxcli git note -p … --write' to attach it", shortCommit(head), err)
	}
	return pushBranchAndNotes(out, repo.Root, branch, push)
}

func pushBranchAndNotes(out io.Writer, root, branch string, push bool) error {
	if push {
		if branch == "" {
			return errors.New("detached HEAD; cannot push — check out a branch and push manually")
		}
		if _, err := gitOutIn(root, nil, "push", "origin", branch, mxMetadataRef); err != nil {
			return fmt.Errorf("push %s and %s: %w", branch, mxMetadataRef, err)
		}
		fmt.Fprintf(out, "Pushed %s and %s to origin.\n", branch, mxMetadataRef)
		return nil
	}
	if branch != "" {
		fmt.Fprintf(out, "Push with: git push origin %s %s\n", branch, mxMetadataRef)
	}
	return nil
}

// isModelPath reports whether a repo-relative path is part of the model:
// the .mpr itself or an .mxunit under mprcontents.
func isModelPath(repo *gitNoteRepo, rel string) bool {
	f := filepath.ToSlash(rel)
	if f == repo.MPRPath {
		return true
	}
	return strings.HasPrefix(f, repo.Contents+"/") && strings.HasSuffix(f, ".mxunit")
}

// projectMendixVersion reads the project's _ProductVersion for the mx
// resolution; "" on any error (ResolveMxForVersion then falls back to PATH).
func projectMendixVersion(mprPath string) string {
	r, err := mpr.Open(mprPath)
	if err != nil {
		return ""
	}
	defer r.Close()
	v, err := r.GetMendixVersion()
	if err != nil {
		return ""
	}
	return v
}

// realMxMerge shells out to `mx merge -v BASE MINE THEIRS` over the three
// revisions' project files, mx resolved for the project's Mendix version;
// -v makes the engine print its per-unit conflict list.
func realMxMerge(baseMpr, mineMpr, theirsMpr string, shas [3]string) (int, string, error) {
	version := projectMendixVersion(mineMpr)
	mxPath, err := docker.ResolveMxForVersion("", version)
	if err != nil {
		return 1, "", fmt.Errorf("no mx for Mendix %s: %w (model conflicts need it)", version, err)
	}
	cmd := exec.Command(mxPath, "merge", "-v", baseMpr, mineMpr, theirsMpr)
	docker.PrepareMxCommand(cmd)
	b, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else {
			return 1, string(b), err
		}
	}
	return code, string(b), nil
}

func shortNames(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		out = append(out, filepath.Base(p))
	}
	return out
}

// conflictedUnitIDs extracts the unit ids of conflicted .mxunit paths.
func conflictedUnitIDs(model []string) []string {
	var ids []string
	for _, p := range model {
		if strings.HasSuffix(p, ".mxunit") {
			ids = append(ids, strings.TrimSuffix(filepath.Base(p), ".mxunit"))
		}
	}
	return ids
}

// engineUnitConflict is one entry of the engine's per-unit conflict list,
// printed only with `-v` as one JSON array on one stdout line. Only
// ManualResolutionDetails "ManualResolutionRequired" needs a decision;
// "AutoResolved" entries are already merged.
type engineUnitConflict struct {
	UnitID                    string  `json:"UnitId"`
	UnitName                  *string `json:"UnitName"`
	ModuleName                string  `json:"ModuleName"`
	ContainmentName           string  `json:"ContainmentName"`
	Type                      string  `json:"Type"`
	TreeConflict              string  `json:"TreeConflict"`
	ConflictResolutionDetails string  `json:"ConflictResolutionDetails"`
}

// parseEngineConflicts finds the engine's conflict list in its output: the
// last line that parses as a JSON array of objects. nil when the engine
// printed none — the caller falls back to git's conflicted-file list.
func parseEngineConflicts(mxOut string) []engineUnitConflict {
	var found []engineUnitConflict
	for _, line := range strings.Split(mxOut, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[") {
			continue
		}
		var cs []engineUnitConflict
		if err := json.Unmarshal([]byte(line), &cs); err == nil && len(cs) > 0 {
			found = cs
		}
	}
	return found
}

// engineManualIDs keeps the entries that need a human decision; entries
// without a unit id say nothing decidable and are skipped.
func engineManualIDs(cs []engineUnitConflict) []string {
	var ids []string
	for _, c := range cs {
		if c.ConflictResolutionDetails == "ManualResolutionRequired" && c.UnitID != "" {
			ids = append(ids, c.UnitID)
		}
	}
	return ids
}

// --- property-level conflict report and the per-unit plan -------------------

type mxDiffChange struct {
	Change            string         `json:"change"`
	ChangeDescription string         `json:"changeDescription"`
	Property          string         `json:"property"`
	BaseValue         string         `json:"baseValue"`
	MineValue         string         `json:"mineValue"`
	ChangeDetails     []mxDiffChange `json:"changeDetails"`
}

type mxDiffUnit struct {
	ID            string         `json:"id"`
	Type          string         `json:"type"`
	Change        string         `json:"change"`
	ChangeDetails []mxDiffChange `json:"changeDetails"`
}

type mxDiffFile struct {
	UnitDifferences []mxDiffUnit `json:"unitDifferences"`
}

// propertyReport renders the two mx diff files as one property-level report
// over the conflicted units: what each side changed.
func propertyReport(oursJSON, theirsJSON string, unitIDs []string) string {
	want := map[string]bool{}
	for _, id := range unitIDs {
		want[id] = true
	}
	lines := map[string][]string{}
	for _, f := range []struct{ side, path string }{{"Yours", oursJSON}, {"Theirs", theirsJSON}} {
		raw, err := os.ReadFile(f.path)
		if err != nil {
			lines[f.side] = append(lines[f.side], fmt.Sprintf("  (diff unavailable: %v)", err))
			continue
		}
		var d mxDiffFile
		if err := json.Unmarshal(bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf")), &d); err != nil {
			lines[f.side] = append(lines[f.side], fmt.Sprintf("  (diff unreadable: %v)", err))
			continue
		}
		var side []string
		for _, u := range d.UnitDifferences {
			if !want[u.ID] {
				continue
			}
			side = append(side, flattenChanges(u.ChangeDetails, "  ")...)
		}
		if len(side) == 0 {
			side = append(side, "  (no change details)")
		}
		lines[f.side] = side
	}
	var b strings.Builder
	b.WriteString("\nProperty-level changes on the conflicted units:\n")
	for _, side := range []string{"Yours", "Theirs"} {
		fmt.Fprintf(&b, "%s:\n", side)
		for _, l := range lines[side] {
			b.WriteString(l)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// flattenChanges walks mx diff's change tree and yields one line per leaf:
// the description, and for property leaves the value movement.
func flattenChanges(changes []mxDiffChange, indent string) []string {
	var out []string
	for _, c := range changes {
		if c.Property != "" {
			out = append(out, fmt.Sprintf("%s%s: %s → %s", indent, c.Property, c.BaseValue, c.MineValue))
			continue
		}
		desc := c.ChangeDescription
		if desc == "" {
			desc = c.Change
		}
		out = append(out, fmt.Sprintf("%s%s", indent, desc))
		out = append(out, flattenChanges(c.ChangeDetails, indent+"    ")...)
	}
	return out
}

// mergePlanTemplate is the file a ManualResolutionRequired run writes beside
// the .mpr; the human sets "take" per unit and re-runs with --plan.
type mergePlanTemplate struct {
	Base   string     `json:"base"`
	Ours   string     `json:"ours"`
	Theirs string     `json:"theirs"`
	Units  []planUnit `json:"units"`
}

type planUnit struct {
	ID   string `json:"id"`
	Take string `json:"take"`
}

func writePlanTemplate(mprPath, baseSha, oursSha, theirsSha string, unitIDs []string) error {
	tmpl := mergePlanTemplate{Base: baseSha, Ours: oursSha, Theirs: theirsSha}
	for _, id := range unitIDs {
		tmpl.Units = append(tmpl.Units, planUnit{ID: id, Take: "ours"})
	}
	b, err := json.MarshalIndent(tmpl, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(mprPath+".merge-plan.json", b, 0o644)
}

// applyPlan re-runs the merge unless one is already in progress (it must
// conflict the same way), checks the plan against those revisions, runs the
// engine again and resolves over its result: everything the engine merged
// stays merged, and each unit it left open takes the plan's side — whole,
// from that side's commit — with its conflict marker cleared. The .mpr index
// is repaired from the files and the merge committed with the note. Every
// refusal after the merge started aborts it: a half-done merge makes Studio
// Pro re-run it (and choke) on every open.
//
// The plan must name exactly the units the engine left open. Resolving from
// git's two sides instead — ours for every conflicted file, theirs for the
// planned units — took a unit the engine had merged (an entity each side
// added to one domain model) from ours, dropping theirs' change to it
// without the plan ever naming it.
func applyPlan(root string, repo *gitNoteRepo, mprAbs, mprPath, ref, planPath, message, branch string, push bool, runners mxRunners, out io.Writer) error {
	raw, err := os.ReadFile(planPath)
	if err != nil {
		return fmt.Errorf("read plan: %w", err)
	}
	var plan struct {
		Base   string     `json:"base"`
		Ours   string     `json:"ours"`
		Theirs string     `json:"theirs"`
		Units  []planUnit `json:"units"`
	}
	if err := json.Unmarshal(raw, &plan); err != nil {
		return fmt.Errorf("plan %s is not valid JSON: %w", planPath, err)
	}

	// An unfinished merge (text conflicts resolved and staged, say) is
	// resumed; otherwise the merge runs and must conflict the same way.
	if _, mhErr := gitOut(root, "rev-parse", "--verify", "--quiet", "MERGE_HEAD"); mhErr != nil {
		_, mergeErr := gitOutIn(root, nil, "merge", "--no-edit", ref)
		if mergeErr == nil {
			return errors.New("the merge no longer conflicts — the plan is unnecessary; nothing was done")
		}
		if _, mhErr := gitOut(root, "rev-parse", "--verify", "--quiet", "MERGE_HEAD"); mhErr != nil {
			return fmt.Errorf("git merge %s: %v", ref, mergeErr)
		}
	}
	baseSha := strings.TrimSpace(gitOutOr(root, "merge-base", "HEAD", "MERGE_HEAD"))
	oursSha := strings.TrimSpace(gitOutOr(root, "rev-parse", "HEAD"))
	theirsSha := strings.TrimSpace(gitOutOr(root, "rev-parse", "MERGE_HEAD"))
	if plan.Base != baseSha || plan.Ours != oursSha || plan.Theirs != theirsSha {
		return abortMerge(root, errors.New("stale plan: the merge revisions moved since it was written; delete it and re-run the merge to regenerate"))
	}

	// Every planned unit must be modified on both sides; anything else
	// (added on one side, deleted, moved) only Studio Pro can decide.
	baseSnap, err := repo.snapshot(baseSha)
	if err != nil {
		return abortMerge(root, err)
	}
	oursSnap, err := repo.snapshot(oursSha)
	if err != nil {
		return abortMerge(root, err)
	}
	theirsSnap, err := repo.snapshot(theirsSha)
	if err != nil {
		return abortMerge(root, err)
	}
	oursStatus := changeStatuses(computeModelChanges(baseSnap, oursSnap, nil))
	theirsStatus := changeStatuses(computeModelChanges(baseSnap, theirsSnap, nil))
	take := map[string]string{}
	for _, u := range plan.Units {
		if u.Take != "ours" && u.Take != "theirs" {
			return abortMerge(root, fmt.Errorf("plan: unit %s has take %q (ours|theirs)", shortCommit(u.ID), u.Take))
		}
		if oursStatus[u.ID] != "Modified" || theirsStatus[u.ID] != "Modified" {
			return abortMerge(root, fmt.Errorf("plan: unit %s is %s on ours and %s on theirs — only modified-on-both conflicts can be planned; resolve it in Studio Pro", shortCommit(u.ID), statusOrNone(oursStatus[u.ID]), statusOrNone(theirsStatus[u.ID])))
		}
		take[u.ID] = u.Take
	}

	// The engine again, over clean worktrees as in the merge itself: the plan
	// decides only what it leaves open.
	revs, err := checkoutRevisions(root, ref, baseSha, oursSha, theirsSha)
	if err != nil {
		return abortMerge(root, err)
	}
	defer revs.cleanup()
	fmt.Fprintf(out, "Merging the model with mx %s; the plan decides what the engine leaves open…\n", projectMendixVersion(mprPath))
	code, mxOut, runErr := revs.runEngine(runners, repo, out)
	switch {
	case runErr != nil:
		return abortMerge(root, fmt.Errorf("mx merge failed to run: %w", runErr))
	case code == 0:
		return abortMerge(root, errors.New("the engine now merges the model without a decision — the plan is unnecessary; re-run without --plan"))
	case code != 2:
		return abortMerge(root, fmt.Errorf("mx merge failed (exit %d); the merge is aborted — if it persists, resolve in Studio Pro (Version Control › Merge Changes Here)", code))
	}
	open := engineDecisions(mxOut, filepath.Join(revs.oursDir, filepath.FromSlash(repo.MPRPath)))
	if len(open) == 0 {
		return abortMerge(root, errors.New("the engine stopped on a decision but named no unit; resolve in Studio Pro (Version Control › Merge Changes Here)"))
	}
	isOpen := map[string]bool{}
	for _, id := range open {
		isOpen[id] = true
		if _, planned := take[id]; !planned {
			return abortMerge(root, fmt.Errorf("plan: unit %s is left for a decision by the engine and the plan does not decide it — delete the plan and re-run without --plan to regenerate it", shortCommit(id)))
		}
	}
	for _, u := range plan.Units {
		if !isOpen[u.ID] {
			return abortMerge(root, fmt.Errorf("plan: unit %s is merged without a decision by the engine — remove it from the plan: taking a side there drops the other side's change to it", shortCommit(u.ID)))
		}
	}

	// The engine's result first, then each planned unit whole from its
	// side's commit, its marker cleared. The hashes are rewritten from the
	// files — files are the truth — and the gate refuses (and aborts) a
	// result whose index and files disagree on which units exist.
	if err := copyEngineResult(root, repo, revs.oursDir); err != nil {
		return abortMerge(root, fmt.Errorf("copy the engine's merge result: %w", err))
	}
	var decided []string
	for _, u := range plan.Units {
		sha := oursSha
		if u.Take == "theirs" {
			sha = theirsSha
		}
		p := path.Join(repo.Contents, u.ID[:2], u.ID[2:4], u.ID+".mxunit")
		if _, err := gitOutIn(root, nil, "checkout", sha, "--", p); err != nil {
			return abortMerge(root, fmt.Errorf("take the %s version of %s: %w", u.Take, p, err))
		}
		decided = append(decided, u.ID)
		fmt.Fprintf(out, "  %s: %s\n", shortCommit(u.ID), u.Take)
	}
	if err := clearDecidedMarkers(mprAbs, decided); err != nil {
		return abortMerge(root, err)
	}
	if err := repairOrRefuse(root, mprPath, out); err != nil {
		return err
	}
	// Nothing may be committed still marked: Studio Pro would report the
	// project as mid-conflict (CE0137) on every open.
	marks, err := conflictMarkers(mprAbs)
	if err != nil {
		return abortMerge(root, fmt.Errorf("read the merge conflict markers: %w", err))
	}
	if len(marks) > 0 {
		var ids []string
		for _, c := range marks {
			ids = append(ids, shortCommit(c.UnitID))
		}
		return abortMerge(root, fmt.Errorf("refusing to commit: the index still marks unit(s) %s as conflicted; resolve in Studio Pro (Version Control › Merge Changes Here)", strings.Join(ids, ", ")))
	}

	if _, err := gitOutIn(root, nil, "add", "-A", "--",
		filepath.FromSlash(repo.MPRPath), filepath.FromSlash(repo.Contents)); err != nil {
		return abortMerge(root, fmt.Errorf("stage the resolved model files: %w", err))
	}
	return commitTheMerge(root, repo, nil, message, branch, push, out)
}

// clearDecidedMarkers records the planned units as decided in the index: their
// files now hold the chosen side, so the engine's marker on them goes.
func clearDecidedMarkers(mprPath string, ids []string) error {
	w, err := mpr.NewWriter(mprPath)
	if err != nil {
		return fmt.Errorf("open project: %w", err)
	}
	defer w.Close()
	if _, err := w.ClearContentsConflicts(ids); err != nil {
		return fmt.Errorf("clear the decided units' conflict markers: %w", err)
	}
	return nil
}

func changeStatuses(changes []mxModelChange) map[string]string {
	out := map[string]string{}
	for _, c := range changes {
		out[c.UnitID] = c.Status
	}
	return out
}

func statusOrNone(s string) string {
	if s == "" {
		return "unchanged"
	}
	return s
}

// realMxDiff shells out to `mx diff BASE MINE OUTPUT` (resolved like the merge).
func realMxDiff(base, mine, outJSON string) error {
	version := projectMendixVersion(mine)
	mxPath, err := docker.ResolveMxForVersion("", version)
	if err != nil {
		return fmt.Errorf("no mx for Mendix %s: %w", version, err)
	}
	cmd := exec.Command(mxPath, "diff", base, mine, outJSON)
	docker.PrepareMxCommand(cmd)
	if b, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mx diff: %v: %s", err, strings.TrimSpace(string(b)))
	}
	return nil
}
