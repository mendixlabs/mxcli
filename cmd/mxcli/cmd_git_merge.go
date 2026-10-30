// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
// model-aware engine Studio Pro's own merge uses, over the base and theirs
// checkouts and the working tree — with the project's Mendix version's mx.
// What the engine resolves is staged, hash-checked and committed as a normal
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
two sides changing the model always conflict — the base and theirs revisions
are checked out into temporary worktrees and the three project folders go to
'mx merge BASE MINE THEIRS' (Studio Pro's own model-aware merge, resolved for
the project's Mendix version):

  - everything the engine auto-resolves is staged, hash-checked and committed
    as the merge commit, with the mx_metadata note attached (this keeps both
    sides — the usual best outcome; the engine owns the .mpr, so a unit one
    side added while the other changed existing ones is kept, not dropped);
  - a conflict the engine reports as ManualResolutionRequired (exit 2) is
    never merged silently: the merge is aborted, and both sides' changes are
    printed for review — at property level, from 'mx diff' (e.g.
    "New type > Length: 100 → 150" on ours, "→ 200" on theirs). Then decide:
      --plan <file>  resolve per unit from the plan template written beside
                     the .mpr (<mpr>.merge-plan.json): set "take" to
                     "ours"/"theirs" per unit; the .mpr index is repaired
                     from the mixed files. Modified-on-both units only —
                     added/deleted/moved conflicts go to Studio Pro.
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
// merge runs the engine over the three PROJECT FOLDERS — the base checkout,
// the working tree, the theirs checkout — and writes its result into the
// working tree, App.mpr included. Exit 0 is a resolved merge, 2 is a decision
// the engine could not make (its per-unit list is on stdout), anything else
// is the engine failing.
type mxMergeRunner func(baseDir, mineDir, theirsDir string, shas [3]string) (int, string, error)
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
//  2. A model conflict goes to `mx merge` over the three project folders
//     first — what the engine auto-resolves keeps both sides, which is almost
//     always the best outcome — and is committed with the note.
//  3. What the engine reports as ManualResolutionRequired (exit 2) is never
//     merged silently: with no strategy the merge is aborted and both sides'
//     changes are printed for review; `--plan` then resolves per unit, and
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
		return applyPlan(root, repo, abs, mprPath, ref, plan, message, branch, push, out)
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
		// A ref that does not resolve fails before any conflict exists.
		if _, mhErr := gitOut(root, "rev-parse", "--verify", "--quiet", "MERGE_HEAD"); mhErr != nil {
			return fmt.Errorf("git merge %s: %s", ref, firstLineOf(mergeOut))
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
	// almost always better than taking one side wholesale. The base and
	// theirs revisions are checked out as folders; MINE is the working tree.
	// The engine writes its result into the working tree and owns the .mpr —
	// its unit list carries both sides' added and deleted documents, so a
	// unit one side added while the other changed existing ones is kept, not
	// dropped.
	tmp, err := os.MkdirTemp("", "mxcli-gitmerge-")
	if err != nil {
		return err
	}
	baseDir := filepath.Join(tmp, "base")
	theirsDir := filepath.Join(tmp, "theirs")
	if _, err := gitOutIn(root, nil, "worktree", "add", "--detach", baseDir, baseSha); err != nil {
		os.RemoveAll(tmp)
		return fmt.Errorf("check out the merge base: %w", err)
	}
	if _, err := gitOutIn(root, nil, "worktree", "add", "--detach", theirsDir, theirsSha); err != nil {
		_, _ = gitOutIn(root, nil, "worktree", "remove", "--force", baseDir)
		os.RemoveAll(tmp)
		return fmt.Errorf("check out %s: %w", ref, err)
	}
	cleanWorktrees := func() {
		_, _ = gitOutIn(root, nil, "worktree", "remove", "--force", baseDir)
		_, _ = gitOutIn(root, nil, "worktree", "remove", "--force", theirsDir)
		_ = os.RemoveAll(tmp)
	}

	version := projectMendixVersion(mprPath)
	fmt.Fprintf(out, "Model files conflict (%s); merging the model with mx %s…\n",
		strings.Join(shortNames(model), ", "), version)
	// Folder mode: the engine takes the three project folders and writes its
	// result into MINE — the working tree.
	projRel := filepath.FromSlash(filepath.Dir(repo.MPRPath)) // "." at the repo root
	code, mxOut, runErr := runners.merge(
		filepath.Join(baseDir, projRel),
		filepath.Dir(abs),
		filepath.Join(theirsDir, projRel),
		[3]string{baseSha, oursSha, theirsSha},
	)
	if s := strings.TrimSpace(mxOut); s != "" {
		fmt.Fprintln(out, s)
	}
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
		// the human decides.
		if runners.diff != nil {
			oursJSON := filepath.Join(tmp, "ours-diff.json")
			theirsJSON := filepath.Join(tmp, "theirs-diff.json")
			d1 := runners.diff(filepath.Join(baseDir, filepath.FromSlash(repo.MPRPath)), abs, oursJSON)
			d2 := runners.diff(filepath.Join(baseDir, filepath.FromSlash(repo.MPRPath)), filepath.Join(theirsDir, filepath.FromSlash(repo.MPRPath)), theirsJSON)
			if d1 == nil && d2 == nil {
				fmt.Fprintln(out, propertyReport(oursJSON, theirsJSON, conflictedUnitIDs(model)))
			}
		}
		if err := writePlanTemplate(mprPath, baseSha, oursSha, theirsSha, model); err != nil {
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

	// Exit 0: the engine resolved the model into the working tree, .mpr
	// included — both sides' document sets are in it. Stage, gate, commit.
	args := []string{"add", "--"}
	args = append(args, model...)
	if _, err := gitOutIn(root, nil, args...); err != nil {
		cleanWorktrees()
		return abortMerge(root, fmt.Errorf("stage the resolved model files: %w", err))
	}
	if err := repairOrRefuse(root, abs, out); err != nil {
		cleanWorktrees()
		return err
	}
	if err := commitTheMerge(root, repo, nil, message, branch, push, out); err != nil {
		cleanWorktrees()
		return err
	}
	cleanWorktrees()
	return nil
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

// realMxMerge shells out to `mx merge BASE MINE THEIRS` over the three
// project folders, mx resolved for the project's Mendix version.
func realMxMerge(baseDir, mineDir, theirsDir string, shas [3]string) (int, string, error) {
	version := ""
	if matches, _ := filepath.Glob(filepath.Join(mineDir, "*.mpr")); len(matches) > 0 {
		version = projectMendixVersion(matches[0])
	}
	mxPath, err := docker.ResolveMxForVersion("", version)
	if err != nil {
		return 1, "", fmt.Errorf("no mx for Mendix %s: %w (model conflicts need it)", version, err)
	}
	cmd := exec.Command(mxPath, "merge", baseDir, mineDir, theirsDir)
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

func writePlanTemplate(mprPath, baseSha, oursSha, theirsSha string, model []string) error {
	tmpl := mergePlanTemplate{Base: baseSha, Ours: oursSha, Theirs: theirsSha}
	for _, id := range conflictedUnitIDs(model) {
		tmpl.Units = append(tmpl.Units, planUnit{ID: id, Take: "ours"})
	}
	b, err := json.MarshalIndent(tmpl, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(mprPath+".merge-plan.json", b, 0o644)
}

// applyPlan re-runs the merge unless one is already in progress (it must
// conflict the same way), checks the plan against those revisions, resolves
// per unit, repairs the .mpr index from the mixed files and commits — with
// the note. Every refusal after the merge started aborts it: a half-done
// merge makes Studio Pro re-run it (and choke) on every open.
func applyPlan(root string, repo *gitNoteRepo, mprAbs, mprPath, ref, planPath, message, branch string, push bool, out io.Writer) error {
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
		mergeOut, mergeErr := gitOutIn(root, nil, "merge", "--no-edit", ref)
		if mergeErr == nil {
			return errors.New("the merge no longer conflicts — the plan is unnecessary; nothing was done")
		}
		if _, mhErr := gitOut(root, "rev-parse", "--verify", "--quiet", "MERGE_HEAD"); mhErr != nil {
			return fmt.Errorf("git merge %s: %s", ref, firstLineOf(mergeOut))
		}
	}
	baseSha := strings.TrimSpace(gitOutOr(root, "merge-base", "HEAD", "MERGE_HEAD"))
	oursSha := strings.TrimSpace(gitOutOr(root, "rev-parse", "HEAD"))
	theirsSha := strings.TrimSpace(gitOutOr(root, "rev-parse", "MERGE_HEAD"))
	if plan.Base != baseSha || plan.Ours != oursSha || plan.Theirs != theirsSha {
		return abortMerge(root, errors.New("stale plan: the merge revisions moved since it was written; delete it and re-run the merge to regenerate"))
	}

	conflicted := nulFields(gitOutOr(root, "diff", "--name-only", "--diff-filter=U", "-z"))
	var model []string
	for _, p := range conflicted {
		if isModelPath(repo, p) {
			model = append(model, p)
		}
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

	// Ours everywhere, then the planned units from theirs.
	for _, p := range model {
		if _, err := gitOutIn(root, nil, "checkout", "--ours", "--", p); err != nil {
			return abortMerge(root, fmt.Errorf("take the ours version of %s: %w", p, err))
		}
	}
	for _, id := range conflictedUnitIDs(model) {
		if take[id] != "theirs" {
			continue
		}
		p := path.Join(repo.Contents, id[:2], id[2:4], id+".mxunit")
		if _, err := gitOutIn(root, nil, "checkout", "--theirs", "--", p); err != nil {
			return abortMerge(root, fmt.Errorf("take the theirs version of %s: %w", p, err))
		}
		fmt.Fprintf(out, "  %s: theirs\n", shortCommit(id))
	}
	// The .mpr index is ours; the mixed unit files have different bytes, so
	// the hashes are rewritten from the files — files are the truth. The
	// gate refuses (and aborts) when the sides' document sets differ: a unit
	// theirs added is not in ours' .mpr, and committing would orphan it.
	if err := repairOrRefuse(root, mprPath, out); err != nil {
		return err
	}

	args := []string{"add", "--"}
	args = append(args, model...)
	if _, err := gitOutIn(root, nil, args...); err != nil {
		return abortMerge(root, fmt.Errorf("stage the resolved model files: %w", err))
	}
	return commitTheMerge(root, repo, nil, message, branch, push, out)
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
