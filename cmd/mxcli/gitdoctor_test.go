// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// `mxcli git doctor` audits a repository the way Studio Pro needs it, and
// `mxcli git commit` does what Studio Pro's commit does: hashes, staging,
// the mx_metadata note. Both are factored as doctorRun / commitRun so the
// tests drive them without the cobra wiring.

func doctorEnv(t *testing.T, dir string) {
	t.Helper()
	requireGit(t)
	// The repository under test is the only git configuration these tests may
	// see. `git remote get-url` applies url.<base>.insteadOf, so a rewrite in
	// ~/.gitconfig (https -> git@, common on developer machines) or in the
	// GIT_CONFIG_COUNT/KEY/VALUE environment (git@ -> https, set by hosted
	// sandboxes) changed the URL the remote tests read back.
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_COUNT", "0")
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
}

func configLocal(t *testing.T, dir, key string) string {
	t.Helper()
	out, err := gitOut(dir, "config", "--local", key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// --- doctor: git local config ----------------------------------------------

func TestGitDoctor_ConfigMissingReported(t *testing.T) {
	dir := t.TempDir()
	doctorEnv(t, dir)
	gitIn(t, dir, "init", "-q", "-b", "main")

	var out bytes.Buffer
	err := doctorRun(dir, "", "", "", false, &out)
	if !errors.Is(err, errDoctorIssuesRemain) {
		t.Fatalf("want errDoctorIssuesRemain, got %v", err)
	}
	for _, key := range []string{"core.autocrlf", "mendix.commits-since-gc", "mendix.lineEndingResetDone"} {
		if !strings.Contains(out.String(), key) {
			t.Errorf("report must name %s:\n%s", key, out.String())
		}
	}
}

func TestGitDoctor_FixWritesConfigAndPasses(t *testing.T) {
	dir := t.TempDir()
	doctorEnv(t, dir)
	gitIn(t, dir, "init", "-q", "-b", "main")

	var out bytes.Buffer
	if err := doctorRun(dir, "", "", "", true, &out); err != nil {
		t.Fatalf("--fix must leave no issue: %v\n%s", err, out.String())
	}
	if got := configLocal(t, dir, "core.autocrlf"); !strings.EqualFold(got, "False") {
		t.Errorf("core.autocrlf = %q, want False", got)
	}
	if got := configLocal(t, dir, "mendix.commits-since-gc"); got != "0" {
		t.Errorf("mendix.commits-since-gc = %q, want 0", got)
	}
	if got := configLocal(t, dir, "mendix.lineEndingResetDone"); !strings.EqualFold(got, "True") {
		t.Errorf("mendix.lineEndingResetDone = %q, want True", got)
	}
	// A second run with nothing to fix stays clean.
	out.Reset()
	if err := doctorRun(dir, "", "", "", false, &out); err != nil {
		t.Fatalf("clean repo must pass: %v\n%s", err, out.String())
	}
	// A repo without a remote is reported, but not broken.
	if !strings.Contains(out.String(), "remote") {
		t.Errorf("report must mention the remote check:\n%s", out.String())
	}
}

// Measured on a real Studio Pro checkout (the mx-git-lab lab, S0): Studio Pro
// writes core.autocrlf=false in lowercase, counts commits-since-gc up between
// garbage collections (2 after two commits) and writes lineendingresetdone in
// lowercase. A healthy Studio Pro checkout must pass untouched.
func TestGitDoctor_StudioProValuesPass(t *testing.T) {
	dir := t.TempDir()
	doctorEnv(t, dir)
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "config", "--local", "core.autocrlf", "false")
	gitIn(t, dir, "config", "--local", "mendix.commits-since-gc", "2")
	gitIn(t, dir, "config", "--local", "mendix.lineendingresetdone", "true")

	var out bytes.Buffer
	if err := doctorRun(dir, "", "", "", false, &out); err != nil {
		t.Fatalf("a healthy Studio Pro checkout must pass: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), "mendix.commits-since-gc — ") {
		t.Errorf("the gc counter must not be judged:\n%s", out.String())
	}
}

// core.autocrlf=true is the one config value that damages a Mendix checkout:
// a checkout rewrites line endings inside .mxunit files on Windows.
func TestGitDoctor_AutoCRLFTrueIsFixed(t *testing.T) {
	dir := t.TempDir()
	doctorEnv(t, dir)
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "config", "--local", "core.autocrlf", "true")

	var out bytes.Buffer
	if err := doctorRun(dir, "", "", "", true, &out); err != nil {
		t.Fatalf("--fix must repair autocrlf=true: %v\n%s", err, out.String())
	}
	if got := configLocal(t, dir, "core.autocrlf"); !strings.EqualFold(got, "false") {
		t.Errorf("core.autocrlf = %q, want false", got)
	}
}

// --- doctor: remote URL scheme ----------------------------------------------

func TestGitDoctor_SSHRemoteRewrittenByFix(t *testing.T) {
	dir := t.TempDir()
	doctorEnv(t, dir)
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "remote", "add", "origin", "git@github.com:org/app.git")

	var out bytes.Buffer
	if err := doctorRun(dir, "", "", "", false, &out); !errors.Is(err, errDoctorIssuesRemain) {
		t.Fatalf("SSH remote must be an issue: %v", err)
	}
	if !strings.Contains(out.String(), "git@github.com:org/app.git") {
		t.Errorf("report must name the URL:\n%s", out.String())
	}

	out.Reset()
	if err := doctorRun(dir, "", "", "", true, &out); err != nil {
		t.Fatalf("--fix must repair the remote: %v\n%s", err, out.String())
	}
	got, err := gitOut(dir, "remote", "get-url", "origin")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got) != "https://github.com/org/app" {
		t.Errorf("remote url = %q, want https://github.com/org/app", got)
	}
}

func TestGitDoctor_SSHURLFormRewrittenByFix(t *testing.T) {
	dir := t.TempDir()
	doctorEnv(t, dir)
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "remote", "add", "origin", "ssh://git@github.com/org/app.git")

	var out bytes.Buffer
	if err := doctorRun(dir, "", "", "", true, &out); err != nil {
		t.Fatalf("--fix must repair ssh:// remotes: %v\n%s", err, out.String())
	}
	got, err := gitOut(dir, "remote", "get-url", "origin")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got) != "https://github.com/org/app" {
		t.Errorf("remote url = %q, want https://github.com/org/app", got)
	}
}

// A url.<base>.insteadOf rule rewrites the URL every git command uses, not the
// URL the repository is configured with. The doctor judged the rewritten one:
// with the common https -> git@ rule in ~/.gitconfig an HTTPS remote read as
// SSH, and --fix "repaired" the configured URL on every run.
func TestGitDoctor_InsteadOfRewriteIsNotAnSSHRemote(t *testing.T) {
	dir := t.TempDir()
	doctorEnv(t, dir)
	t.Setenv("GIT_SSH_COMMAND", "false") // the rewritten URL is SSH: never reach the network
	gitIn(t, dir, "init", "-q", "-b", "main")
	for _, kv := range mxConfigKeys {
		gitIn(t, dir, "config", "--local", kv.key, kv.fallback)
	}
	gitIn(t, dir, "config", "--local", "url.git@git.example.com:.insteadOf", "https://git.example.com/")
	gitIn(t, dir, "remote", "add", "origin", "https://git.example.com/org/app.git")

	var out bytes.Buffer
	if err := doctorRun(dir, "", "", "", false, &out); err != nil {
		t.Errorf("a configured HTTPS remote is not an issue, whatever a rewrite makes of it: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "insteadOf") {
		t.Errorf("the report must explain the rewrite:\n%s", out.String())
	}

	out.Reset()
	if err := doctorRun(dir, "", "", "", true, &out); err != nil {
		t.Errorf("--fix: %v\n%s", err, out.String())
	}
	if got := configLocal(t, dir, "remote.origin.url"); got != "https://git.example.com/org/app.git" {
		t.Errorf("--fix changed the configured URL to %q:\n%s", got, out.String())
	}
}

// The configured URL is what --fix rewrites, so an SSH remote is still found
// when a rule rewrites it to HTTPS (as hosted sandboxes do).
func TestGitDoctor_ConfiguredSSHRemoteFoundBehindRewrite(t *testing.T) {
	dir := t.TempDir()
	doctorEnv(t, dir)
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "config", "--local", "url.https://git.example.com/.insteadOf", "git@git.example.com:")
	gitIn(t, dir, "remote", "add", "origin", "git@git.example.com:org/app.git")

	var out bytes.Buffer
	if err := doctorRun(dir, "", "", "", true, &out); err != nil {
		t.Fatalf("--fix: %v\n%s", err, out.String())
	}
	if got := configLocal(t, dir, "remote.origin.url"); got != "https://git.example.com/org/app" {
		t.Errorf("configured URL = %q, want https://git.example.com/org/app\n%s", got, out.String())
	}
}

// --- doctor: mx_metadata notes ----------------------------------------------

// doctorNoteRepo makes a two-commit repository holding an MPR v2 project:
// the base commit, then a page retitle. Returns dir and the .mpr path.
//
// The fixture's git is hermetic: no system or global config leaks in from the
// machine this runs on (runner images, developer setups and CI images differ),
// bytes are committed exactly as written, and no repository above the temp
// dir is discovered. Without this, git-merge fixtures see a different merge
// on each machine and fail with the merge's own evidence thrown away.
func doctorNoteRepo(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	gitIn(t, dir, "init", "-q", "-b", "feature")
	gitIn(t, dir, "config", "core.autocrlf", "false")
	units := baseUnits(t)
	writeProject(t, dir, "11.13.0", units)
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "base")
	units[tPage] = tUnit{tFolder, unitDoc(t, "Forms$Page", "Home", bson.E{Key: "Title", Value: "Welcome"})}
	writeProject(t, dir, "11.13.0", units)
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "retitle home")
	return dir, filepath.Join(dir, "App.mpr")
}

func noteOn(t *testing.T, dir, rev string) string {
	t.Helper()
	out, err := gitOut(dir, "notes", "--ref="+mxMetadataRef, "show", rev)
	if err != nil {
		return ""
	}
	return out
}

func TestGitDoctor_MissingAndMalformedNotesReportedAndFixed(t *testing.T) {
	dir, mprPath := doctorNoteRepo(t)
	doctorEnv(t, dir)
	gitIn(t, dir, "notes", "--ref="+mxMetadataRef, "add", "-f", "-m", "not json", "HEAD~1")

	var out bytes.Buffer
	if err := doctorRun(dir, mprPath, "", "", false, &out); !errors.Is(err, errDoctorIssuesRemain) {
		t.Fatalf("missing and malformed notes must be issues: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "malformed") || !strings.Contains(out.String(), "without a note") {
		t.Errorf("report must name both findings:\n%s", out.String())
	}

	out.Reset()
	if err := doctorRun(dir, mprPath, "", "", true, &out); err != nil {
		t.Fatalf("--fix must repair the notes: %v\n%s", err, out.String())
	}
	var head, base mxNote
	if err := json.Unmarshal([]byte(noteOn(t, dir, "HEAD")), &head); err != nil {
		t.Fatalf("HEAD note is not valid JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(noteOn(t, dir, "HEAD~1")), &base); err != nil {
		t.Fatalf("HEAD~1 note (was malformed) is not valid JSON: %v", err)
	}
	if !head.HasModelerVersion || head.ModelerVersion != "11.13.0" {
		t.Errorf("HEAD note header: %+v", head)
	}
	if len(head.ModelChanges) != 1 || head.ModelChanges[0].UnitID != tPage {
		t.Errorf("HEAD note must list the retitled page: %+v", head.ModelChanges)
	}
	if len(base.ModelChanges) == 0 {
		t.Errorf("base commit note must list the added units: %+v", base)
	}
}

func TestGitDoctor_FixKeepsRealNotes(t *testing.T) {
	dir, mprPath := doctorNoteRepo(t)
	doctorEnv(t, dir)
	gitIn(t, dir, "notes", "--ref="+mxMetadataRef, "add", "-f", "-m", realStudioProNote, "HEAD")

	var out bytes.Buffer
	doctorRun(dir, mprPath, "", "", true, &out) // config findings remain; the note must not
	if got := strings.TrimSpace(noteOn(t, dir, "HEAD")); got != realStudioProNote {
		t.Fatalf("real note was changed:\n got %s\nwant %s", got, realStudioProNote)
	}
}

// A fresh clone has no refs/notes/* (git clone never fetches them), while the
// remote carries the notes Studio Pro wrote. Judged against the empty local
// ref, every commit looked unannotated, and --fix wrote notes over the
// remote's that it then refused to accept (non-fast-forward).
func TestGitDoctor_UnfetchedNotesAreNotMissing(t *testing.T) {
	src, _ := doctorNoteRepo(t)
	doctorEnv(t, src)
	gitIn(t, src, "notes", "--ref="+mxMetadataRef, "add", "-f", "-m", realStudioProNote, "HEAD")
	remote := t.TempDir()
	gitIn(t, remote, "init", "-q", "--bare")
	gitIn(t, src, "remote", "add", "origin", remote)
	gitIn(t, src, "push", "-q", "origin", "feature", mxMetadataRef)

	clone := filepath.Join(t.TempDir(), "clone")
	gitIn(t, filepath.Dir(clone), "clone", "-q", "-b", "feature", remote, clone)
	for _, kv := range mxConfigKeys {
		gitIn(t, clone, "config", "--local", kv.key, kv.fallback)
	}
	mprPath := filepath.Join(clone, "App.mpr")
	head := strings.TrimSpace(gitOutOr(clone, "rev-parse", "HEAD"))

	var out bytes.Buffer
	err := doctorRun(clone, mprPath, "", "", false, &out)
	if strings.Contains(out.String(), shortCommit(head)+" without a note") {
		t.Errorf("HEAD has a note on the remote; an unfetched notes ref must not report it missing:\n%s", out.String())
	}
	if !errors.Is(err, errDoctorIssuesRemain) || !strings.Contains(out.String(), "git fetch origin "+mxMetadataRef) {
		t.Errorf("the unfetched notes ref must be the finding, with the command that fetches it: %v\n%s", err, out.String())
	}

	out.Reset()
	if err := doctorRun(clone, mprPath, "", "", true, &out); err != nil {
		t.Fatalf("--fix must fetch the notes and annotate the rest: %v\n%s", err, out.String())
	}
	if got := strings.TrimSpace(noteOn(t, clone, "HEAD")); got != realStudioProNote {
		t.Fatalf("--fix replaced the remote's note on HEAD:\n got %s\nwant %s\n%s", got, realStudioProNote, out.String())
	}
	if _, err := gitOut(clone, "push", "origin", "feature", mxMetadataRef); err != nil {
		t.Errorf("the notes --fix leaves must push as a fast-forward: %v\n%s", err, out.String())
	}
}

func TestGitDoctor_TemplateWithoutProjectUsesVersionFlag(t *testing.T) {
	dir := t.TempDir()
	doctorEnv(t, dir)
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "commit", "-q", "--allow-empty", "-m", "docs")

	// Without --version there is nothing to derive a ModelerVersion from.
	var out bytes.Buffer
	err := doctorRun(dir, "", "", "", true, &out)
	if err == nil || !strings.Contains(err.Error(), "--version") {
		t.Fatalf("want an error naming --version, got %v", err)
	}

	out.Reset()
	if err := doctorRun(dir, "", "", "10.6.0", true, &out); err != nil {
		t.Fatalf("--fix --version must fill the notes: %v\n%s", err, out.String())
	}
	var n mxNote
	if err := json.Unmarshal([]byte(noteOn(t, dir, "HEAD")), &n); err != nil {
		t.Fatal(err)
	}
	if n.ModelerVersion != "10.6.0" || n.BranchName != "" || !n.HasModelerVersion || len(n.ModelChanges) != 0 {
		t.Errorf("template note: %+v", n)
	}
}

func TestGitDoctor_TemplateDetectsVersionFromHistory(t *testing.T) {
	dir, _ := doctorNoteRepo(t)
	doctorEnv(t, dir)
	gitIn(t, dir, "notes", "--ref="+mxMetadataRef, "add", "-f", "-m", realStudioProNote, "HEAD~1")

	// No -p: the note cannot be computed from the model, so the template is
	// written with the version detected from the newest valid note.
	var out bytes.Buffer
	if err := doctorRun(dir, "", "", "", true, &out); err != nil {
		t.Fatalf("--fix: %v\n%s", err, out.String())
	}
	var n mxNote
	if err := json.Unmarshal([]byte(noteOn(t, dir, "HEAD")), &n); err != nil {
		t.Fatal(err)
	}
	if n.ModelerVersion != "11.13.0" {
		t.Errorf("template ModelerVersion = %q, want 11.13.0 from the newest valid note", n.ModelerVersion)
	}
}

// --- git commit ---------------------------------------------------------------

func commitSubject(t *testing.T, dir string) string {
	t.Helper()
	out, err := gitOut(dir, "show", "-s", "--format=%s", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out)
}

func committedFiles(t *testing.T, dir string) string {
	t.Helper()
	out, err := gitOut(dir, "show", "--name-only", "--format=", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestGitCommit_HappyPathWritesNoteAndSkipsJunk(t *testing.T) {
	dir, mprPath := doctorNoteRepo(t)
	doctorEnv(t, dir)
	// A real source change, plus one junk file that must never be committed.
	units := baseUnits(t)
	units[tPage] = tUnit{tFolder, unitDoc(t, "Forms$Page", "Home", bson.E{Key: "Title", Value: "Again"})}
	writeProject(t, dir, "11.13.0", units)
	if err := os.WriteFile(filepath.Join(dir, "project-settings.user.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := commitRun(mprPath, "retitle again", false, false, &out); err != nil {
		t.Fatalf("commitRun: %v\n%s", err, out.String())
	}
	if subj := commitSubject(t, dir); subj != "retitle again" {
		t.Errorf("HEAD subject = %q", subj)
	}
	files := committedFiles(t, dir)
	if strings.Contains(files, "project-settings.user.json") {
		t.Errorf("user settings must not be committed:\n%s", files)
	}
	var n mxNote
	if err := json.Unmarshal([]byte(noteOn(t, dir, "HEAD")), &n); err != nil {
		t.Fatalf("HEAD has no valid note: %v", err)
	}
	if n.BranchName != "feature" || !n.HasModelerVersion {
		t.Errorf("note header: %+v", n)
	}
	// The junk file is still on disk, untracked.
	if _, err := os.Stat(filepath.Join(dir, "project-settings.user.json")); err != nil {
		t.Error("junk file was deleted")
	}
	if !strings.Contains(out.String(), "git push origin feature "+mxMetadataRef) {
		t.Errorf("must tell how to push branch and notes:\n%s", out.String())
	}
}

func TestGitCommit_RepairsHashDrift(t *testing.T) {
	dir, mprPath := doctorNoteRepo(t)
	doctorEnv(t, dir)
	// Rewrite a unit file behind the index's back: the classic git checkout drift.
	page := filepath.Join(dir, "mprcontents", tPage[:2], tPage[2:4], tPage+".mxunit")
	raw, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw[:len(raw)-2], raw[len(raw)-2]+1, raw[len(raw)-1])
	if err := os.WriteFile(page, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := commitRun(mprPath, "drift", false, false, &out); err != nil {
		t.Fatalf("commitRun must repair mismatched hashes and commit: %v\n%s", err, out.String())
	}
	var out2 bytes.Buffer
	if err := fixHashes(mprPath, false, &out2); err != nil {
		t.Errorf("hashes still drift after commit: %v\n%s", err, out2.String())
	}
	if subj := commitSubject(t, dir); subj != "drift" {
		t.Errorf("HEAD subject = %q", subj)
	}
}

func TestGitCommit_RefusedWhileStudioProOpen(t *testing.T) {
	dir, mprPath := doctorNoteRepo(t)
	doctorEnv(t, dir)
	lock := mprPath + ".lock"
	if err := os.WriteFile(lock, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := commitRun(mprPath, "nope", false, false, &out); err == nil {
		t.Fatal("commit must be refused while the lock file exists")
	}
	if subj := commitSubject(t, dir); subj != "retitle home" {
		t.Errorf("a commit was made anyway: %q", subj)
	}
}

// --dry-run staged everything to report it and then reset the whole index, so
// whatever the user had staged before running it was unstaged.
func TestGitCommit_DryRunKeepsTheUsersStaging(t *testing.T) {
	dir, mprPath := doctorNoteRepo(t)
	doctorEnv(t, dir)
	for _, name := range []string{"staged.txt", "unstaged.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitIn(t, dir, "add", "staged.txt")

	var out bytes.Buffer
	if err := commitRun(mprPath, "dry", false, true, &out); err != nil {
		t.Fatalf("dry run failed: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "unstaged.txt") {
		t.Errorf("the dry run must still report every file it would commit:\n%s", out.String())
	}
	staged, err := gitOut(dir, "diff", "--cached", "--name-only")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(staged); got != "staged.txt" {
		t.Errorf("staged after --dry-run = %q, want exactly what the user had staged (staged.txt)", got)
	}
}

func TestGitCommit_DryRunCommitsNothing(t *testing.T) {
	dir, mprPath := doctorNoteRepo(t)
	doctorEnv(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, "theme"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "theme", "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := commitSubject(t, dir)

	var out bytes.Buffer
	if err := commitRun(mprPath, "dry", false, true, &out); err != nil {
		t.Fatalf("dry run failed: %v\n%s", err, out.String())
	}
	if got := commitSubject(t, dir); got != before {
		t.Errorf("dry run committed: %q", got)
	}
	staged, err := gitOut(dir, "diff", "--cached", "--name-only")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(staged) != "" {
		t.Errorf("dry run left changes staged:\n%s", staged)
	}
}

func TestGitCommit_PushesBranchAndNotes(t *testing.T) {
	dir, mprPath := doctorNoteRepo(t)
	doctorEnv(t, dir)
	units := baseUnits(t)
	units[tPage] = tUnit{tFolder, unitDoc(t, "Forms$Page", "Home", bson.E{Key: "Title", Value: "Ship"})}
	writeProject(t, dir, "11.13.0", units)
	remote := t.TempDir()
	gitIn(t, remote, "init", "-q", "--bare")
	gitIn(t, dir, "remote", "add", "origin", remote)

	var out bytes.Buffer
	if err := commitRun(mprPath, "push me", true, false, &out); err != nil {
		t.Fatalf("commitRun --push: %v\n%s", err, out.String())
	}
	ls, err := gitOut(dir, "ls-remote", "origin")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ls, "refs/heads/feature") {
		t.Errorf("branch was not pushed:\n%s", ls)
	}
	if !strings.Contains(ls, mxMetadataRef) {
		t.Errorf("notes ref was not pushed:\n%s", ls)
	}
}

func TestGitCommit_NothingToCommit(t *testing.T) {
	dir, mprPath := doctorNoteRepo(t)
	doctorEnv(t, dir)

	var out bytes.Buffer
	if err := commitRun(mprPath, "empty", false, false, &out); err != nil {
		t.Fatalf("nothing to commit must not be an error: %v", err)
	}
	if !strings.Contains(out.String(), "nothing to commit") {
		t.Errorf("must say so:\n%s", out.String())
	}
}

// gitInE runs git and lets it fail, for the commands whose failure is the
// fixture (a merge that conflicts).
func gitInE(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	// The same fixed identity gitIn commits with: `git merge` needs one even
	// when it stops on a conflict, and a host whose name gives git nothing to
	// guess from (a bare "vm", no domain) refused the fixture's merge.
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	b, err := cmd.CombinedOutput()
	return string(b), err
}

// A staged path with spaces must reach the commit whole, and junk under a
// spaced path must still be left out.
func TestGitCommit_SpacedPathsStagedWholeAndJunkLeft(t *testing.T) {
	dir, mprPath := doctorNoteRepo(t)
	doctorEnv(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, "my docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "my docs", "read me.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "deployment"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deployment", "old log.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := commitRun(mprPath, "spacey", false, false, &out); err != nil {
		t.Fatalf("commitRun: %v\n%s", err, out.String())
	}
	files := committedFiles(t, dir)
	if !strings.Contains(files, "my docs/read me.txt") {
		t.Errorf("spaced path not committed whole:\n%s", files)
	}
	if strings.Contains(files, "deployment/old log.txt") {
		t.Errorf("junk was committed:\n%s", files)
	}
}

// writeFiles creates each repo-relative path with its parent folders.
func writeFiles(t *testing.T, dir string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(p), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// committedSet is the set of paths HEAD changed.
func committedSet(t *testing.T, dir string) map[string]bool {
	t.Helper()
	set := map[string]bool{}
	for _, f := range strings.Split(strings.TrimSpace(committedFiles(t, dir)), "\n") {
		set[f] = true
	}
	return set
}

// The never-commit folders are the project's own, beside the .mpr, where
// Studio Pro's .gitignore anchors them (/deployment/). Matched at any depth, a
// user folder of the same name was silently left out of the commit.
func TestGitCommit_NestedFolderNamedLikeJunkIsCommitted(t *testing.T) {
	dir, mprPath := doctorNoteRepo(t)
	doctorEnv(t, dir)
	writeFiles(t, dir, "resources/deployment/endpoints.xml", "javascriptsource/theme-cache/x.js", "deployment/run.log", "theme-cache/a.css")

	var out bytes.Buffer
	if err := commitRun(mprPath, "nested", false, false, &out); err != nil {
		t.Fatalf("commitRun: %v\n%s", err, out.String())
	}
	files := committedSet(t, dir)
	for _, keep := range []string{"resources/deployment/endpoints.xml", "javascriptsource/theme-cache/x.js"} {
		if !files[keep] {
			t.Errorf("%s is not the project's own folder and must be committed:\n%s", keep, out.String())
		}
	}
	for _, junk := range []string{"deployment/run.log", "theme-cache/a.css"} {
		if files[junk] {
			t.Errorf("%s is the project's own and must be left out", junk)
		}
	}
}

// With the project in a subfolder of the repository, its folders are under
// that subfolder; the same names elsewhere in the repository are not its own.
func TestGitCommit_JunkAnchoredAtProjectInSubfolder(t *testing.T) {
	root := t.TempDir()
	doctorEnv(t, root)
	gitIn(t, root, "init", "-q", "-b", "feature")
	app := filepath.Join(root, "app")
	if err := os.MkdirAll(app, 0o755); err != nil {
		t.Fatal(err)
	}
	writeProject(t, app, "11.13.0", baseUnits(t))
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "base")
	writeFiles(t, root, "app/deployment/run.log", "app/resources/deployment/endpoints.xml", "tools/deployment/deploy.sh")

	var out bytes.Buffer
	if err := commitRun(filepath.Join(app, "App.mpr"), "sub", false, false, &out); err != nil {
		t.Fatalf("commitRun: %v\n%s", err, out.String())
	}
	files := committedSet(t, root)
	if files["app/deployment/run.log"] {
		t.Error("app/deployment/ is the project's own and must be left out")
	}
	for _, keep := range []string{"app/resources/deployment/endpoints.xml", "tools/deployment/deploy.sh"} {
		if !files[keep] {
			t.Errorf("%s must be committed:\n%s", keep, out.String())
		}
	}
}

// An unresolved merge must refuse the commit: `git add -A` would otherwise
// stage the conflict markers and commit them.
func TestGitCommit_RefusedDuringUnresolvedMerge(t *testing.T) {
	dir, mprPath := doctorNoteRepo(t)
	doctorEnv(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("feature side\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "notes.txt")
	gitIn(t, dir, "commit", "-q", "-m", "notes feature")
	gitIn(t, dir, "switch", "-q", "-c", "other", "HEAD~1")
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("other side\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "notes.txt")
	gitIn(t, dir, "commit", "-q", "-m", "notes other")
	gitIn(t, dir, "switch", "-q", "feature")
	if _, err := gitInE(t, dir, "merge", "other"); err == nil {
		t.Fatal("fixture: the merge was expected to conflict")
	}

	var out bytes.Buffer
	if err := commitRun(mprPath, "conflicted", false, false, &out); err == nil {
		t.Fatal("commit must be refused during an unresolved merge")
	}
	if subj := commitSubject(t, dir); subj != "notes feature" {
		t.Errorf("a commit was made anyway: %q", subj)
	}
}

// The lock guard honours the same override every other mxcli write honours,
// for the crash-left-a-lock case.
func TestGitCommit_LockOverrideEnvCommits(t *testing.T) {
	dir, mprPath := doctorNoteRepo(t)
	doctorEnv(t, dir)
	if err := os.WriteFile(mprPath+".lock", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	units := baseUnits(t)
	units[tPage] = tUnit{tFolder, unitDoc(t, "Forms$Page", "Home", bson.E{Key: "Title", Value: "Crashed"})}
	writeProject(t, dir, "11.13.0", units)
	t.Setenv("MXCLI_ALLOW_STUDIO_PRO_OPEN", "1")

	var out bytes.Buffer
	if err := commitRun(mprPath, "override", false, false, &out); err != nil {
		t.Fatalf("commitRun with the override env: %v\n%s", err, out.String())
	}
	if subj := commitSubject(t, dir); subj != "override" {
		t.Errorf("HEAD subject = %q", subj)
	}
}

// A dry run writes nothing — not even the ContentsHash repair.
func TestGitCommit_DryRunLeavesHashDriftUntouched(t *testing.T) {
	dir, mprPath := doctorNoteRepo(t)
	doctorEnv(t, dir)
	page := filepath.Join(dir, "mprcontents", tPage[:2], tPage[2:4], tPage+".mxunit")
	raw, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw[:len(raw)-2], raw[len(raw)-2]+1, raw[len(raw)-1])
	if err := os.WriteFile(page, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := commitRun(mprPath, "dry over drift", false, true, &out); err != nil {
		t.Fatalf("dry run failed: %v\n%s", err, out.String())
	}
	var out2 bytes.Buffer
	if err := fixHashes(mprPath, false, &out2); !errors.Is(err, errHashIssuesRemain) {
		t.Errorf("the dry run repaired the hashes (err %v):\n%s", err, out2.String())
	}
}

// The staged-files summary must list what the commit will hold, not the junk
// that was just unstaged (measured on the mx-git-lab lab: the list opened with
// 10 .mxcli/ paths while the commit held 2 model files).
func TestGitCommit_StagedListOmitsJunk(t *testing.T) {
	dir, mprPath := doctorNoteRepo(t)
	doctorEnv(t, dir)
	units := baseUnits(t)
	units[tPage] = tUnit{tFolder, unitDoc(t, "Forms$Page", "Home", bson.E{Key: "Title", Value: "List"})}
	writeProject(t, dir, "11.13.0", units)
	if err := os.WriteFile(filepath.Join(dir, "project-settings.user.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := commitRun(mprPath, "clean list", false, false, &out); err != nil {
		t.Fatalf("commitRun: %v\n%s", err, out.String())
	}
	section := out.String()[strings.Index(out.String(), "Staged"):]
	if strings.Contains(section, "project-settings.user.json") {
		t.Errorf("the staged list names junk that was left out:\n%s", section)
	}
	if !strings.Contains(section, "App.mpr") {
		t.Errorf("the staged list must name the model files:\n%s", section)
	}
}

func TestGitCommit_DryRunWithPushRefused(t *testing.T) {
	dir, mprPath := doctorNoteRepo(t)
	doctorEnv(t, dir)

	var out bytes.Buffer
	if err := commitRun(mprPath, "both", true, true, &out); err == nil {
		t.Fatal("--dry-run with --push must be refused")
	}
}

// A misspelled -p must be refused, not turned into a "real" note with an
// empty ModelerVersion that no later run would ever touch again.
func TestGitDoctor_BadProjectPathRefused(t *testing.T) {
	dir, mprPath := doctorNoteRepo(t)
	doctorEnv(t, dir)

	var out bytes.Buffer
	err := doctorRun(dir, mprPath+"-typo", "", "10.6.0", true, &out)
	if err == nil || !strings.Contains(err.Error(), "project not found") {
		t.Fatalf("want a project-not-found error, got %v", err)
	}
	if note := noteOn(t, dir, "HEAD"); strings.Contains(note, `"ModelerVersion":""`) {
		t.Fatalf("a note with an empty version was written:\n%s", note)
	}
}

func TestGitDoctor_VersionFlagValidated(t *testing.T) {
	dir := t.TempDir()
	doctorEnv(t, dir)
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "commit", "-q", "--allow-empty", "-m", "docs")

	var out bytes.Buffer
	err := doctorRun(dir, "", "", "banana", true, &out)
	if err == nil || !strings.Contains(err.Error(), "--version") {
		t.Fatalf("want a --version format error, got %v", err)
	}
}
