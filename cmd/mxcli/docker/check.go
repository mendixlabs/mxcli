// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// CheckOptions configures the mx check command.
type CheckOptions struct {
	// ProjectPath is the path to the .mpr file.
	ProjectPath string

	// MxBuildPath is an explicit path to the mxbuild executable (used to find mx).
	MxBuildPath string

	// By default the project is checked as stored, and when that finds CE0463
	// ("the definition of this widget has changed") the same temporary copy is
	// re-checked after 'mx update-widgets' to label each CE0463 as a stale
	// widget definition or as one that persists (ako/mxcli#568, #646). The
	// verdict is always the stored project's.

	// SkipUpdateWidgets checks the project as stored and never runs
	// 'mx update-widgets', not even for the re-check (--no-update-widgets).
	// The verdict is the default's; only the labels are missing.
	SkipUpdateWidgets bool

	// UpdateWidgetsFirst runs 'mx update-widgets' on the temporary copy before
	// the check and checks only that widget-normalised copy (--update-widgets).
	// It was the default until #646: it hides every CE0463 a resync clears,
	// which the stored project still has and which fails MxBuild and
	// `mxcli run --local`.
	UpdateWidgetsFirst bool

	// Stdout for output messages.
	Stdout io.Writer

	// Stderr for error output.
	Stderr io.Writer
}

// The two labels a CE0463 of the stored project gets from the re-check.
const (
	staleCE0463Label      = "stale widget definition — `mxcli fix widgets` fixes it"
	persistingCE0463Label = "persists after update-widgets"
)

// copyFile copies a single file from src to dst, preserving the source file mode.
func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// resolveMxForCheck and mxCheckCmd are seams for tests, which substitute stubs
// that behave like the real tools without needing mx.
var resolveMxForCheck = ResolveMxForVersion

var mxCheckCmd = func(mxPath, mprPath string, args []string, w, stderr io.Writer) error {
	cmd := exec.Command(mxPath, append([]string{"check", mprPath}, args...)...)
	cmd.Stdout = w
	cmd.Stderr = stderr
	PrepareMxCommand(cmd)
	return cmd.Run()
}

// Check runs 'mx check' on the project to validate it before building.
//
// It checks the project as stored: a CE0463 there fails MxBuild and
// `mxcli run --local`, so the check must report it (ako/mxcli#568, #646). When
// it finds one, it re-checks the copy after `mx update-widgets` and says which
// CE0463s a widget resync (`mxcli fix widgets`) clears; the verdict stays the
// stored project's. opts.UpdateWidgetsFirst restores the old behaviour of
// checking only the normalised copy.
//
// It never modifies the project. `mx update-widgets` rewrites the model (and
// turns an MPRv2 project into MPRv1), and `mx check` itself writes theme-cache/
// and deployment/sass/ — so both run on a temporary copy, and what mx prints is
// reported against the project's own paths (ako/mxcli#951).
func Check(opts CheckOptions) error {
	if opts.SkipUpdateWidgets && opts.UpdateWidgetsFirst {
		return fmt.Errorf("--no-update-widgets and --update-widgets cannot be combined: " +
			"the first checks the project as stored, the second only a widget-normalised copy")
	}
	w := opts.Stdout
	if w == nil {
		w = os.Stdout
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	// Resolve mx binary
	projectVersion := ""
	if opts.ProjectPath != "" {
		if reader, err := openReadOnly(opts.ProjectPath); err == nil {
			projectVersion = reader.ProjectVersion().ProductVersion
			reader.Disconnect()
		}
	}

	mxPath, err := resolveMxForCheck(opts.MxBuildPath, projectVersion)
	if err != nil {
		return fmt.Errorf("%w\n  docker check does not download MxBuild; 'mxcli setup mxbuild -p %s' sets up the project's version", err, opts.ProjectPath)
	}
	fmt.Fprintf(w, "Using mx: %s\n", mxPath)

	projectPath := opts.ProjectPath
	if abs, err := filepath.Abs(projectPath); err == nil {
		projectPath = abs
	}
	workMpr, cleanup, err := copyProjectToTemp(projectPath)
	if err != nil {
		return fmt.Errorf("copy the project to a temporary directory for checking: %w\n"+
			"  mx writes into the project it checks, so docker check never runs it on the original;\n"+
			"  set TMPDIR to a disk with room for the project", err)
	}
	defer cleanup()
	out := newPathRewriter(w, filepath.Dir(workMpr), filepath.Dir(projectPath))
	errOut := newPathRewriter(stderr, filepath.Dir(workMpr), filepath.Dir(projectPath))
	defer out.Flush()
	defer errOut.Flush()
	fmt.Fprintln(w, "Checking a temporary copy (mx writes into the project it checks; the project on disk is not changed).")

	// --update-widgets: normalise pluggable widget definitions first and check
	// only the normalised copy, as docker check did by default before #646. On
	// the copy, so neither the model nor its storage format changes.
	normalised := false
	if opts.UpdateWidgetsFirst {
		fmt.Fprintln(w, "Normalising widget definitions on the temporary copy (--update-widgets)...")
		if err := updateWidgetsCmd(mxPath, workMpr, out, errOut); err != nil {
			out.Flush()
			fmt.Fprintf(w, "Warning: update-widgets failed (continuing): %v\n", err)
		} else {
			normalised = true
		}
	}

	// Run mx check. Its output is kept as well as shown, to find the CE0463s.
	fmt.Fprintf(w, "Checking project %s...\n", opts.ProjectPath)
	var checkOut, checkErrOut bytes.Buffer
	checkErr := mxCheckCmd(mxPath, workMpr, nil, io.MultiWriter(out, &checkOut), io.MultiWriter(errOut, &checkErrOut))
	out.Flush()
	errOut.Flush()

	switch {
	case normalised:
		// #568 / #646: the normalisation can hide a CE0463 the stored project
		// still has, which then fails `run --local` and MxBuild.
		fmt.Fprintln(w, "Note: widget definitions were normalised on a temporary copy before checking")
		fmt.Fprintln(w, "  (--update-widgets), so a CE0463 the stored project still has is not reported here;")
		fmt.Fprintln(w, "  it fails MxBuild and `mxcli run --local`. Without --update-widgets docker check")
		fmt.Fprintln(w, "  checks the project as stored; `mxcli fix widgets` applies the normalisation to it.")
	case opts.UpdateWidgetsFirst:
		// mendixlabs/mxcli#1629: update-widgets refuses a project of another
		// Mendix version than mx's own (exit status 4) where mx check accepts it.
		fmt.Fprintln(w, "Note: widget definitions were not normalised (update-widgets failed above), so this")
		fmt.Fprintln(w, "  checked the project as stored, and a CE0463 may be one normalisation would clear.")
		fmt.Fprintln(w, versionMismatchHint)
	case checkErr != nil && !opts.SkipUpdateWidgets:
		if stored := ce0463Lines(checkOut.String() + "\n" + checkErrOut.String()); len(stored) > 0 {
			recheckCE0463(w, out, mxPath, workMpr, opts.ProjectPath, stored, checkOut.String())
		}
	}
	if checkErr != nil {
		return fmt.Errorf("project check failed: %w", checkErr)
	}

	fmt.Fprintln(w, "Project check passed.")
	return nil
}

// versionMismatchHint explains the commonest update-widgets failure
// (mendixlabs/mxcli#1629).
const versionMismatchHint = "  If mx said \"Project is in different Mendix version\", update-widgets needs an mx of\n" +
	"  the project's exact version, which mx check does not: `mxcli setup mxbuild -p\n" +
	"  <app>.mpr` fetches it, and --mxbuild-path ~/.mxcli/mxbuild/<version> selects it."

// recheckCE0463 runs `mx update-widgets` on the copy the stored project was just
// checked on, checks it again, and labels each CE0463 of the stored project: a
// stale widget definition, which the resync clears and `mxcli fix widgets`
// applies to the project, or one that persists after it. The re-check's own
// output is not shown — the stored project's result is the one that counts.
//
// mx check's text output does not name a CE0463's page, so two same-named
// widgets on different pages print the same line; the lines are compared as
// counts, and a line that is partly cleared says how many of it were.
func recheckCE0463(w io.Writer, out *pathRewriter, mxPath, workMpr, projectPath string, stored []string, storedOutput string) {
	fmt.Fprintf(w, "\nThe stored project has %d CE0463 %s. Re-checking the temporary copy after\n",
		len(stored), plural(len(stored), "error", "errors"))
	fmt.Fprintln(w, "  `mx update-widgets` to tell stale widget definitions from other causes...")

	var uwOut bytes.Buffer
	if err := updateWidgetsCmd(mxPath, workMpr, &uwOut, &uwOut); err != nil {
		// Its output says why (mendixlabs/mxcli#1629: another patch version).
		io.Copy(out, &uwOut)
		out.Flush()
		fmt.Fprintf(w, "The re-check could not tell which CE0463s are stale widget definitions: update-widgets failed: %v\n", err)
		fmt.Fprintln(w, versionMismatchHint)
		return
	}
	var after bytes.Buffer
	_ = mxCheckCmd(mxPath, workMpr, nil, &after, &after) // non-zero whenever errors remain
	afterTotal, ok := appErrorCount(after.String())
	if !ok {
		fmt.Fprintln(w, "The re-check could not tell which CE0463s are stale widget definitions: mx check did")
		fmt.Fprintln(w, "  not complete on the copy after update-widgets.")
		return
	}

	remaining := map[string]int{}
	for _, l := range ce0463Lines(after.String()) {
		remaining[l]++
	}
	total := map[string]int{}
	var order []string
	for _, l := range stored {
		if total[l] == 0 {
			order = append(order, l)
		}
		total[l]++
	}
	stale, persisting := 0, 0
	fmt.Fprintln(w, "CE0463 in the stored project:")
	for _, l := range order {
		n := total[l]
		p := min(remaining[l], n)
		s := n - p
		stale += s
		persisting += p
		at := ce0463Location(l)
		if s > 0 {
			fmt.Fprintf(w, "  at %s%s: %s\n", at, countOf(s, n), staleCE0463Label)
		}
		if p > 0 {
			fmt.Fprintf(w, "  at %s%s: %s\n", at, countOf(p, n), persistingCE0463Label)
		}
	}
	switch {
	case persisting == 0 && stale == 1:
		fmt.Fprintln(w, "It is a stale widget definition. This updates it in the project, as Studio Pro's")
		fmt.Fprintln(w, "  \"Update all widgets\" does:")
	case persisting == 0:
		fmt.Fprintf(w, "All %d are stale widget definitions. This updates them in the project, as Studio\n", stale)
		fmt.Fprintln(w, "  Pro's \"Update all widgets\" does:")
	case stale == 0 && persisting == 1:
		fmt.Fprintln(w, "It is not a stale widget definition: it persists after update-widgets, so a widget")
		fmt.Fprintln(w, "  resync (`mxcli fix widgets`) does not clear it.")
	case stale == 0:
		fmt.Fprintf(w, "None of the %d is a stale widget definition: all persist after update-widgets, so a\n", persisting)
		fmt.Fprintln(w, "  widget resync (`mxcli fix widgets`) does not clear them.")
	default:
		fmt.Fprintf(w, "%d of %d are stale widget definitions; %d persist after update-widgets, which a\n",
			stale, stale+persisting, persisting)
		fmt.Fprintln(w, "  widget resync does not clear. This updates the stale ones in the project:")
	}
	if stale > 0 {
		fmt.Fprintf(w, "    mxcli fix widgets -p %s\n", projectPath)
	}
	if storedTotal, ok := appErrorCount(storedOutput); ok {
		fmt.Fprintf(w, "As stored: %d %s. ", storedTotal, plural(storedTotal, "error", "errors"))
	}
	fmt.Fprintf(w, "After update-widgets: %d %s.\n", afterTotal, plural(afterTotal, "error", "errors"))
}

// countOf prints "(k of n)" when only part of n identical lines share a label.
func countOf(k, n int) string {
	switch {
	case n == 1:
		return ""
	case k == n:
		return fmt.Sprintf(" (%d×)", n)
	default:
		return fmt.Sprintf(" (%d of %d)", k, n)
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// ce0463Lines returns mx check's CE0463 lines, trimmed, in the order printed:
//
//	[error] [CE0463] "The definition of this widget has changed. ..." at Data grid 2 'dgRequests'
func ce0463Lines(output string) []string {
	var lines []string
	for _, l := range strings.Split(output, "\n") {
		if l = strings.TrimSpace(l); strings.Contains(l, "[CE0463]") {
			lines = append(lines, l)
		}
	}
	return lines
}

// ce0463Location is the widget a CE0463 line names ("Data grid 2 'dgRequests'"),
// or the whole line when it names none.
func ce0463Location(line string) string {
	if i := strings.LastIndex(line, `" at `); i >= 0 {
		return line[i+len(`" at `):]
	}
	return line
}

var appErrorCountRe = regexp.MustCompile(`The app contains: (\d+) errors?\.`)

// appErrorCount reads mx check's closing "The app contains: N errors." line.
func appErrorCount(output string) (int, bool) {
	m := appErrorCountRe.FindStringSubmatch(output)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

// MxCheckOnCopy runs `mx check <project> args...` on a temporary copy of the
// project, so the check cannot write into it, and returns mx's error (a non-zero
// exit when the project has errors). Output naming the copy is rewritten to name
// the project. It does not run update-widgets: it checks the project as stored.
//
// Every caller that runs a plain `mx check` should go through this: mx check
// writes theme-cache/ and deployment/sass/ into the project it is given — the TUI
// checker and the eval runner each did, on every run (ako/mxcli#961).
func MxCheckOnCopy(mxPath, mprPath string, args []string, stdout, stderr io.Writer) error {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	projectPath, err := filepath.Abs(mprPath)
	if err != nil {
		return err
	}
	workMpr, cleanup, err := copyProjectToTemp(projectPath)
	if err != nil {
		return fmt.Errorf("copy the project to a temporary directory for mx check: %w", err)
	}
	defer cleanup()
	out := newPathRewriter(stdout, filepath.Dir(workMpr), filepath.Dir(projectPath))
	errOut := newPathRewriter(stderr, filepath.Dir(workMpr), filepath.Dir(projectPath))
	defer out.Flush()
	defer errOut.Flush()
	return mxCheckCmd(mxPath, workMpr, args, out, errOut)
}

// mxBinaryName returns the platform-specific mx binary name.
func mxBinaryName() string {
	if runtime.GOOS == "windows" {
		return "mx.exe"
	}
	return "mx"
}

func mxBinaryNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"mx.exe", "mx"}
	}
	return []string{"mx"}
}

// ResolveMx finds the mx executable.
// Priority: derive from mxbuild path > PATH lookup.
func ResolveMx(mxbuildPath string) (string, error) {
	return ResolveMxForVersion(mxbuildPath, "")
}

// ResolveMxForVersion finds the mx executable, preferring the project's exact
// Mendix version when multiple local installations or cached downloads exist.
func ResolveMxForVersion(mxbuildPath, preferredVersion string) (string, error) {
	if mxbuildPath != "" {
		// Resolve mxbuild first to handle directory paths
		resolvedMxBuild, err := resolveMxBuild(mxbuildPath, preferredVersion)
		if err == nil {
			// Look for mx in the same directory as mxbuild
			mxDir := filepath.Dir(resolvedMxBuild)
			candidate := filepath.Join(mxDir, mxBinaryName())
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}

			// Try deriving mx name from mxbuild name (e.g. mxbuild11.6.3 -> mx11.6.3)
			mxbuildBase := filepath.Base(resolvedMxBuild)
			suffix := strings.TrimPrefix(mxbuildBase, "mxbuild")
			if runtime.GOOS == "windows" {
				suffix = strings.TrimPrefix(mxbuildBase, "mxbuild")
				suffix = strings.TrimSuffix(suffix, ".exe")
				candidate = filepath.Join(mxDir, "mx"+suffix+".exe")
			} else {
				candidate = filepath.Join(mxDir, "mx"+suffix)
			}
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
		}
	}

	// Try PATH
	if p, err := exec.LookPath("mx"); err == nil {
		return p, nil
	}

	if preferredVersion != "" {
		if studioProDir := ResolveStudioProDir(preferredVersion); studioProDir != "" {
			for _, name := range mxBinaryNames() {
				candidate := filepath.Join(studioProDir, "modeler", name)
				if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
					return candidate, nil
				}
			}
		}
	}

	// Try OS-specific known locations (Studio Pro on Windows) before cached downloads.
	if matches := globVersionedMatches(mendixSearchPaths(mxBinaryName())); len(matches) > 0 {
		if exact := exactVersionedPath(matches, preferredVersion); exact != "" {
			return exact, nil
		}
		if newest := NewestVersionedPath(matches); newest != "" {
			return newest, nil
		}
	}

	if preferredVersion != "" {
		if p := CachedMxPath(preferredVersion); p != "" {
			return p, nil
		}
	}
	if p := AnyCachedMxPath(); p != "" {
		return p, nil
	}

	return "", fmt.Errorf("mx not found; specify --mxbuild-path pointing to Mendix installation directory")
}

// localMxForVersion returns a locally available mx binary that is EXACTLY the
// requested version, or "" if there is none.
//
// Unlike ResolveMxForVersion it never substitutes a different version. That
// substitution is reasonable when the project already exists and its version is a
// preference; it is wrong when the version IS the request — see
// ResolveMxForNewProject.
//
// Looks in the same places, minus the "any version will do" fallbacks: an
// exact-version Studio Pro install, an exact-version binary in the OS-specific
// install locations, then the exact-version download cache. PATH is deliberately
// excluded: an `mx` on PATH carries no version guarantee.
func localMxForVersion(version string) string {
	if version == "" {
		return ""
	}
	if studioProDir := ResolveStudioProDir(version); studioProDir != "" {
		for _, name := range mxBinaryNames() {
			candidate := filepath.Join(studioProDir, "modeler", name)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
		}
	}
	if matches := globVersionedMatches(mendixSearchPaths(mxBinaryName())); len(matches) > 0 {
		if exact := exactVersionedPath(matches, version); exact != "" {
			return exact
		}
	}
	return CachedMxPath(version)
}

// ResolveMxForNewProject finds the mx binary for use by mxcli new.
//
// The requested version is not a preference here, it is the definition of the
// output: `mx create-project` stamps the new project with the version of the binary
// that created it. So only an EXACT match may be reused; anything else is
// downloaded. Delegating to ResolveMxForVersion was wrong, because its last resort
// is AnyCachedMxPath() — with only 11.6.6 cached, `mxcli new --version 11.12.2`
// printed "Resolving MxBuild 11.12.2..." and then produced an 11.6.6 project, with
// no warning that the requested version had been ignored (mendixlabs/mxcli#808 era
// finding; the CDN had the version all along).
//
// On Windows and macOS an exact-version Studio Pro is still preferred, so those
// platforms do not download a Linux CDN binary that cannot execute.
func ResolveMxForNewProject(version string, progressWriter io.Writer) (string, error) {
	if mxPath := localMxForVersion(version); mxPath != "" {
		return mxPath, nil
	}
	// Not available locally at the requested version: download it (works on Linux;
	// on macOS/Windows this is only reached when Studio Pro is not installed).
	mxbuildPath, err := DownloadMxBuild(version, progressWriter)
	if err != nil {
		return "", err
	}
	return ResolveMx(mxbuildPath)
}

func CachedMxPath(version string) string {
	cacheDir, err := MxBuildCacheDir(version)
	if err != nil {
		return ""
	}
	return cachedBinaryPath(cacheDir, mxBinaryNames())
}

func AnyCachedMxPath() string {
	return anyCachedBinaryPath(mxBinaryNames())
}

// LocalMxForVersion is localMxForVersion: an installed or cached mx of exactly
// version, or "".
func LocalMxForVersion(version string) string { return localMxForVersion(version) }
