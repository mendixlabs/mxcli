// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// gitdoctor.go audits a repository for the state Studio Pro expects, and
// repairs it with --fix. Four things a commit made with plain git gets wrong:
//
//   - the local git config Studio Pro writes is missing (core.autocrlf=False
//     keeps a checkout from rewriting line endings inside .mxunit files on
//     Windows; mendix.commits-since-gc and mendix.lineEndingResetDone are
//     Studio Pro's own bookkeeping),
//   - the remote is SSH, which Studio Pro cannot use (HTTPS only),
//   - commits have no note under refs/notes/mx_metadata, so Studio Pro shows
//     "(unknown)" and cannot version-check the revision,
//   - a note is malformed, which Studio Pro cannot read at all.
//
// Real notes are never touched: a note is only written where none exists, or
// over one that is unreadable. Where the model at a commit cannot be read
// (MPR v1 history, a revision without the .mpr), a template note with a
// detected or --version ModelerVersion is written instead.

var errDoctorIssuesRemain = errors.New("issues remain (re-run with --fix to repair)")

// mxConfigKey is one entry of the local git config Studio Pro writes.
// fallback is what a missing key is backfilled with. presentValuePasses marks
// the keys whose value is Studio Pro's own bookkeeping — measured on a real
// checkout (the mx-git-lab lab, S0), commits-since-gc counts up between gc
// runs and lineEndingResetDone comes back in any case, so only a MISSING key
// is an issue there. core.autocrlf is judged: true (or input) makes a
// checkout rewrite line endings inside .mxunit files on Windows.
type mxConfigKey struct {
	key                string
	fallback           string
	presentValuePasses bool
}

var mxConfigKeys = []mxConfigKey{
	{"core.autocrlf", "False", false},
	{"mendix.commits-since-gc", "0", true},
	{"mendix.lineEndingResetDone", "True", true},
}

var (
	sshScpURLRe = regexp.MustCompile(`^git@([^:/]+):(.+?)(?:\.git)?$`)
	sshURLRe    = regexp.MustCompile(`^ssh://(?:[^@/]+@)?([^/:]+?)(:\d+)?/(.+?)(?:\.git)?$`)
	mxVersionRe = regexp.MustCompile(`^[0-9]+(\.[0-9]+)+$`)
)

// doctorRun audits the repository at root and reports one line per finding.
// mprPath ("" when not given) lets --fix compute real notes from the model.
// With fix, repairs are applied. Returns errDoctorIssuesRemain when something
// is still wrong.
func doctorRun(root, mprPath, remoteOverride, version string, fix bool, out io.Writer) error {
	found, fixed := 0, 0

	fmt.Fprintln(out, "Git local config")
	for _, ck := range mxConfigKeys {
		key, want := ck.key, ck.fallback
		current := gitConfigLocal(root, key)
		switch {
		case current == "":
			found++
			fmt.Fprintf(out, "  [ ] %s — missing (expected %s)\n", key, want)
			if fix {
				if _, err := gitOut(root, "config", "--local", key, want); err != nil {
					return fmt.Errorf("set %s: %w", key, err)
				}
				fixed++
				fmt.Fprintf(out, "      set to %s\n", want)
			}
		case ck.presentValuePasses || strings.EqualFold(current, "false"):
			fmt.Fprintf(out, "  [x] %s = %s\n", key, current)
		default:
			found++
			fmt.Fprintf(out, "  [ ] %s = %s (expected %s)\n", key, current, want)
			if fix {
				if _, err := gitOut(root, "config", "--local", key, want); err != nil {
					return fmt.Errorf("set %s: %w", key, err)
				}
				fixed++
				fmt.Fprintf(out, "      set to %s\n", want)
			}
		}
	}

	fmt.Fprintln(out, "Remote")
	remote := detectDoctorRemote(root, remoteOverride)
	remoteNotes := "" // the remote's notes commit, when it has one and could be reached
	switch {
	case remote == "":
		fmt.Fprintln(out, "  [i] no remote configured; skipping remote checks")
	default:
		// The configured URL, not `git remote get-url`: that one applies
		// url.<base>.insteadOf, so a rewrite in the user's git config (https ->
		// git@ is a common one) made an HTTPS remote read as SSH, and --fix set
		// the configured URL on every run without changing what git uses.
		urlRaw, urlErr := gitOut(root, "config", "--get", "remote."+remote+".url")
		if urlErr != nil {
			found++
			fmt.Fprintf(out, "  [ ] remote %s does not exist (git remote -v lists the names)\n", remote)
			break
		}
		url := strings.TrimSpace(urlRaw)
		if host, p, ok := sshToHTTPS(url); ok {
			found++
			fmt.Fprintf(out, "  [ ] %s: %s — SSH; Studio Pro needs HTTPS\n", remote, url)
			if fix {
				https := "https://" + host + "/" + p
				if _, err := gitOut(root, "remote", "set-url", remote, https); err != nil {
					return fmt.Errorf("rewrite remote URL: %w", err)
				}
				fixed++
				fmt.Fprintf(out, "      rewritten to %s\n", https)
			}
			break
		}
		if strings.HasPrefix(url, "https://") {
			fmt.Fprintf(out, "  [x] %s: %s (HTTPS)\n", remote, url)
		} else {
			fmt.Fprintf(out, "  [i] %s: %s (not HTTPS; Studio Pro can only use HTTPS remotes)\n", remote, url)
		}
		if eff, err := gitOut(root, "remote", "get-url", remote); err == nil && strings.TrimSpace(eff) != url {
			fmt.Fprintf(out, "  [i] a url.<base>.insteadOf rule in your git config makes git use %s instead; --fix leaves the rule alone\n", strings.TrimSpace(eff))
		}
		// Absent or unreachable notes ref is a hint, not an issue.
		ls, err := gitOut(root, "ls-remote", remote, mxMetadataRef)
		switch {
		case err != nil:
			fmt.Fprintf(out, "  [i] could not reach %s to check %s\n", remote, mxMetadataRef)
		case strings.Contains(ls, mxMetadataRef):
			fmt.Fprintf(out, "  [x] %s has %s\n", remote, mxMetadataRef)
			if f := strings.Fields(ls); len(f) > 0 {
				remoteNotes = f[0]
			}
		default:
			fmt.Fprintf(out, "  [i] %s has no %s yet; push it together with the branch\n", remote, mxMetadataRef)
		}
	}

	fn, fx, err := doctorNotes(root, mprPath, version, remote, remoteNotes, fix, out)
	if err != nil {
		return err
	}
	found, fixed = found+fn, fixed+fx

	remaining := found - fixed
	if remaining < 0 {
		remaining = 0 // placeholder replacements were never counted as issues
	}
	switch {
	case remaining > 0:
		fmt.Fprintf(out, "\n%d issue(s) remain.\n", remaining)
		return errDoctorIssuesRemain
	case fixed > 0 && found > 0:
		fmt.Fprintf(out, "\nFixed %d issue(s).\n", fixed)
	case fixed > 0:
		fmt.Fprintf(out, "\nReplaced %d placeholder note(s).\n", fixed)
	default:
		fmt.Fprintln(out, "\nNothing to fix.")
	}
	return nil
}

// doctorNotes reports (and with fix, repairs) missing and malformed notes over
// the whole current history. It returns how many it found and how many it fixed.
//
// remoteNotes is the remote's notes commit ("" when it has none or could not
// be reached). Those are the notes Studio Pro reads: its fetch force-replaces
// the local ref with them. A local ref that does not contain that commit — a
// fresh clone, since git clone never fetches refs/notes/* — cannot be judged:
// every commit the remote annotates would look unannotated, and a note written
// over it could not be pushed (non-fast-forward). So the commits are not
// judged until the ref is fetched; --fix fetches it, as a fast-forward only,
// which never drops a note that exists only locally.
func doctorNotes(root, mprPath, version, remote, remoteNotes string, fix bool, out io.Writer) (found, fixed int, err error) {
	fmt.Fprintln(out, "mx_metadata notes")
	if _, err := gitOut(root, "rev-parse", "--verify", "--quiet", "HEAD"); err != nil {
		fmt.Fprintln(out, "  [i] no commits yet")
		return 0, 0, nil
	}
	if remoteNotes != "" && !hasNotesCommit(root, remoteNotes) {
		found++
		fmt.Fprintf(out, "  [ ] %s from %s is not fetched; the notes cannot be judged without it\n", mxMetadataRef, remote)
		if !fix {
			fmt.Fprintf(out, "      git fetch %s %s:%s\n", remote, mxMetadataRef, mxMetadataRef)
			return found, 0, nil
		}
		if _, err := gitOut(root, "fetch", "--quiet", remote, mxMetadataRef+":"+mxMetadataRef); err != nil {
			fmt.Fprintf(out, "      could not fetch it as a fast-forward (%v); if the local ref has notes the remote lacks, push or reconcile them first\n", err)
			return found, 0, nil
		}
		fixed++
		fmt.Fprintf(out, "      fetched from %s\n", remote)
	}
	revs := strings.Fields(gitOutOr(root, "rev-list", "--reverse", "HEAD"))
	noted := map[string]bool{}
	for _, line := range strings.Split(strings.TrimRight(gitOutOr(root, "notes", "--ref="+mxMetadataRef, "list"), "\n"), "\n") {
		if f := strings.Fields(line); len(f) == 2 {
			noted[f[1]] = true
		}
	}

	var missing, malformed, placeholders []string
	for _, c := range revs {
		if !noted[c] {
			missing = append(missing, c)
			continue
		}
		text := gitNoteText(root, c)
		var n struct {
			HasModelerVersion *bool `json:"HasModelerVersion"`
		}
		if json.Unmarshal([]byte(text), &n) != nil {
			malformed = append(malformed, c)
			continue
		}
		if n.HasModelerVersion != nil && !*n.HasModelerVersion {
			placeholders = append(placeholders, c)
		}
	}
	for _, c := range missing {
		fmt.Fprintf(out, "  [ ] %s without a note\n", shortCommit(c))
	}
	for _, c := range malformed {
		fmt.Fprintf(out, "  [ ] malformed note on %s\n", shortCommit(c))
	}
	if len(placeholders) > 0 {
		fmt.Fprintf(out, "  [i] %d placeholder note(s) Studio Pro back-filled\n", len(placeholders))
	}
	found = len(missing) + len(malformed)
	if !fix || found+len(placeholders) == 0 {
		return found, 0, nil
	}

	// An explicit --version must look like one; a typo would be stamped into
	// every template note in history.
	if version != "" && !mxVersionRe.MatchString(version) {
		return found, 0, fmt.Errorf("--version %q is not a Mendix version (digits and dots, e.g. 10.21.0 or 11.13.0.12345) — pass --version or fix it", version)
	}
	var repo *gitNoteRepo
	if mprPath != "" {
		// Refuse a path that does not exist before anything else: a snapshot
		// of a revision without the .mpr reads as an empty model, and a
		// computed note from it would carry no version.
		if _, err := os.Stat(mprPath); err != nil {
			return found, 0, fmt.Errorf("project not found: %s", mprPath)
		}
		r, err := openGitNoteRepo(mprPath)
		if err != nil {
			return found, 0, err
		}
		defer r.Close()
		repo = r
	}
	tmplVersion := version
	if tmplVersion == "" {
		tmplVersion, _ = newestNotedVersion(root)
	}
	branch := strings.TrimSpace(gitOutOr(root, "symbolic-ref", "--quiet", "--short", "HEAD"))

	// Oldest first, every commit once: a missing note is computed, a malformed
	// or placeholder note is replaced — from the model where it can be read,
	// from the template otherwise.
	unfixable := 0
	for _, c := range revs {
		replace := false
		for _, l := range malformed {
			replace = replace || l == c
		}
		for _, l := range placeholders {
			replace = replace || l == c
		}
		if noted[c] && !replace {
			continue // a real note: never touched
		}
		text, computed, err := noteForCommit(repo, tmplVersion, c, branch)
		if err != nil {
			unfixable++
			fmt.Fprintf(out, "  [ ] %s: %v\n", shortCommit(c), err)
			continue
		}
		if err := writeNoteRaw(root, c, text); err != nil {
			unfixable++
			fmt.Fprintf(out, "  [ ] %s: write note: %v\n", shortCommit(c), err)
			continue
		}
		kind := "template note"
		if computed {
			kind = "computed note"
		}
		fmt.Fprintf(out, "  [x] %s: wrote %s\n", shortCommit(c), kind)
		fixed++
	}
	if unfixable > 0 && tmplVersion == "" {
		return found, fixed, fmt.Errorf("%d commit(s) still without a note: no ModelerVersion is known — pass --version", unfixable)
	}
	return found, fixed, nil
}

// --- helpers -----------------------------------------------------------------

func gitConfigLocal(root, key string) string {
	out, err := gitOut(root, "config", "--local", key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func gitOutOr(root string, args ...string) string {
	out, _ := gitOut(root, args...)
	return out
}

func shortCommit(c string) string {
	if len(c) > 10 {
		return c[:10]
	}
	return c
}

// detectDoctorRemote picks the remote the way Studio Pro would use it: the
// override, else the current branch's tracking remote, else origin, else the
// first remote. "" when there is none.
func detectDoctorRemote(root, override string) string {
	if override != "" {
		return override
	}
	if up, err := gitOut(root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"); err == nil {
		if i := strings.Index(strings.TrimSpace(up), "/"); i > 0 {
			return strings.TrimSpace(up)[:i]
		}
	}
	remotes := strings.Fields(gitOutOr(root, "remote"))
	for _, r := range remotes {
		if r == "origin" {
			return r
		}
	}
	if len(remotes) > 0 {
		return remotes[0]
	}
	return ""
}

// sshToHTTPS rewrites the two SSH spellings git uses to an HTTPS URL. ok is
// false for everything else, including ssh:// URLs with a port (an HTTPS port
// cannot be derived).
func sshToHTTPS(url string) (host, p string, ok bool) {
	if m := sshScpURLRe.FindStringSubmatch(url); m != nil {
		return m[1], m[2], true
	}
	if m := sshURLRe.FindStringSubmatch(url); m != nil {
		if m[2] != "" {
			return "", "", false
		}
		return m[1], m[3], true
	}
	return "", "", false
}

// hasNotesCommit reports whether the local notes ref is, or contains, the
// notes commit c. False when the ref is absent or c is not in its history
// (including when c's object was never fetched).
func hasNotesCommit(root, c string) bool {
	local, err := gitOut(root, "rev-parse", "--verify", "--quiet", mxMetadataRef)
	if err != nil {
		return false
	}
	if strings.TrimSpace(local) == c {
		return true
	}
	_, err = gitOut(root, "merge-base", "--is-ancestor", c, strings.TrimSpace(local))
	return err == nil
}

func gitNoteText(root, c string) string {
	out, err := gitOut(root, "notes", "--ref="+mxMetadataRef, "show", c)
	if err != nil {
		return ""
	}
	return out
}

// newestNotedVersion scans the history newest first and returns the
// ModelerVersion of the first readable note, the way Studio Pro's own
// version column would read it.
func newestNotedVersion(root string) (string, error) {
	for _, c := range strings.Fields(gitOutOr(root, "rev-list", "HEAD")) {
		text := gitNoteText(root, c)
		if text == "" {
			continue
		}
		var n struct {
			ModelerVersion    string `json:"ModelerVersion"`
			HasModelerVersion bool   `json:"HasModelerVersion"`
		}
		if json.Unmarshal([]byte(text), &n) != nil {
			continue
		}
		if n.HasModelerVersion && n.ModelerVersion != "" && n.ModelerVersion != "(unknown)" {
			return n.ModelerVersion, nil
		}
	}
	return "", errors.New("no readable note in history")
}

// noteForCommit builds the note text for a commit: computed from the model
// when repo is set and the revision reads, the template otherwise. A computed
// note without a ModelerVersion is a snapshot of a revision without a readable
// .mpr — treated as unreadable, because a "real" note with an empty version is
// one no later run would ever touch again.
func noteForCommit(repo *gitNoteRepo, tmplVersion, c, branch string) (text string, computed bool, err error) {
	if repo != nil {
		if n, err := repo.buildNote(c, branch); err == nil && n.ModelerVersion != "" {
			return n.marshal(), true, nil
		}
	}
	if tmplVersion == "" {
		return "", false, errors.New("model unreadable and no template version known")
	}
	return templateNote(tmplVersion), false, nil
}

// templateNote is the note of last resort: structurally exactly what Studio
// Pro writes, with the model changes it cannot know left empty.
func templateNote(v string) string {
	return mxNote{
		BranchName:        "",
		ModelerVersion:    v,
		ModelChanges:      []mxModelChange{},
		RelatedStories:    []string{},
		SolutionVersion:   "",
		MPRFormatVersion:  "Version2",
		HasModelerVersion: true,
	}.marshal()
}

// writeNoteRaw attaches text to c, replacing whatever is there (-f is what
// makes a malformed note repairable; missing notes have nothing to lose).
func writeNoteRaw(root, c, text string) error {
	_, err := gitOutIn(root, []byte(text), "notes", "--ref="+mxMetadataRef, "add", "-f", "-F", "-", c)
	return err
}
