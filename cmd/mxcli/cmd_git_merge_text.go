// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Text resolutions across the ManualResolutionRequired abort
// (mendixlabs/mxcli#1751).
//
// A merge that conflicts on both a non-model file and the model goes in
// steps: the run stops on the text conflict, the human resolves and stages
// it, the re-run reaches the engine, and the engine needs a decision (exit 2)
// — which aborts the merge, and `git merge --abort` resets the index with the
// human's resolution in it. The --plan run then re-merged, met the same text
// conflict unresolved, wrote the model anyway and failed at the commit:
// half done, the resolution gone.
//
// So the abort records each staged non-model path in the plan template — its
// index blob, which git keeps in the object store after the abort — and the
// --plan run puts it back on a path that conflicts again. And before the plan
// touches the model, any non-model conflict still open stops it, as it stops
// the plain run, with the merge left in progress for the human.

// textResolution is one staged non-model path at the abort: the index blob it
// had, or Deleted when the resolution removed it.
type textResolution struct {
	Path    string `json:"path"`
	Blob    string `json:"blob,omitempty"`
	Deleted bool   `json:"deleted,omitempty"`
}

// stagedTextResolutions lists the non-model paths the index changes against
// HEAD — the human's resolutions, and the side's clean changes, which restoring
// writes back unchanged.
func stagedTextResolutions(root string, repo *gitNoteRepo, mprPath string) []textResolution {
	plan := filepath.ToSlash(filepath.Base(mprPath) + ".merge-plan.json")
	var out []textResolution
	for _, p := range nulFields(gitOutOr(root, "diff", "--cached", "--name-only", "-z")) {
		if isModelPath(repo, p) || strings.HasSuffix(filepath.ToSlash(p), plan) {
			continue
		}
		blob, err := gitOut(root, "rev-parse", "--verify", "--quiet", ":0:"+p)
		if blob = strings.TrimSpace(blob); err != nil || blob == "" {
			out = append(out, textResolution{Path: p, Deleted: true})
			continue
		}
		out = append(out, textResolution{Path: p, Blob: blob})
	}
	return out
}

// restoreTextResolutions puts each recorded resolution back on a path that
// conflicts again, and stages it. A path that merged cleanly this time is left
// as the merge made it. It reports the paths it restored.
func restoreTextResolutions(root string, saved []textResolution) ([]string, error) {
	if len(saved) == 0 {
		return nil, nil
	}
	unmerged := map[string]bool{}
	for _, p := range nulFields(gitOutOr(root, "diff", "--name-only", "--diff-filter=U", "-z")) {
		unmerged[p] = true
	}
	var restored []string
	for _, r := range saved {
		if !unmerged[r.Path] {
			continue
		}
		if r.Deleted {
			if _, err := gitOut(root, "rm", "-q", "--", r.Path); err != nil {
				return restored, fmt.Errorf("restore the resolution of %s (deleted): %w", r.Path, err)
			}
			restored = append(restored, r.Path)
			continue
		}
		content, err := gitOut(root, "cat-file", "blob", r.Blob)
		if err != nil {
			return restored, fmt.Errorf("restore the resolution of %s: blob %s is gone (%v); resolve it again", r.Path, shortCommit(r.Blob), err)
		}
		dst := filepath.Join(root, filepath.FromSlash(r.Path))
		if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
			return restored, err
		}
		if _, err := gitOut(root, "add", "--", r.Path); err != nil {
			return restored, fmt.Errorf("stage the resolution of %s: %w", r.Path, err)
		}
		restored = append(restored, r.Path)
	}
	return restored, nil
}

// openTextConflicts lists the non-model paths still unmerged.
func openTextConflicts(root string, repo *gitNoteRepo) []string {
	var other []string
	for _, p := range nulFields(gitOutOr(root, "diff", "--name-only", "--diff-filter=U", "-z")) {
		if !isModelPath(repo, p) {
			other = append(other, p)
		}
	}
	return other
}

// reportTextConflicts prints the plain run's "resolve by hand" list and
// returns its error; the merge is left in progress.
func reportTextConflicts(out io.Writer, other []string) error {
	fmt.Fprintln(out, "Non-model conflicts, resolve by hand, then 'git add' them and re-run this command to resolve the model and commit:")
	for _, p := range other {
		fmt.Fprintf(out, "  %s\n", p)
	}
	return decisionNeeded(errors.New("merge has non-model conflicts left for manual resolution"))
}
