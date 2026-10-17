// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	"github.com/spf13/cobra"
)

var marketplacePackageCmd = &cobra.Command{
	Use:   "package <module> -p <app.mpr>",
	Short: "Export a module of a project as a .mpk module package",
	Long: `Export a module as a .mpk module package, the reverse of
'marketplace install --file <package.mpk>'. The package is made by Mendix's own
'mx create-module-package', so Studio Pro, 'mx module-import' and mxcli can all
read it back.

  - Without --all-libs only the userlib JARs the module declares it needs (those
    with a '.<Module>.RequiredLib' file beside them) are bundled; with --all-libs
    every JAR in the app's userlib/ goes in (24 MB on an app with Business Events
    installed, against 12 KB for the module's own).
  - mx still exports the app's whole resources/ folder, whatever --all-libs says
    (measured: resources/EventLog/ of another module came along). mxcli opens the
    package and warns about any resources/, javasource/ or themesource/ file that
    belongs to another module, and prints the --exclude pattern that drops it.
  - --exclude takes ONE regular expression, matched against each file's path
    inside the package; join alternatives with '|'. Write the folder separator as
    '.' ('^resources.EventLog.'): a character class or a backslash makes mx print
    its usage and write nothing.

The mx must be the Mendix version of the project: a package made by another
version than the app it comes from is refused, naming both versions. mx is taken
from, in order, an mx on PATH, a Studio Pro install and the mxbuild cache, so a
different version can be found first; 'mxcli setup mxbuild --version <version>'
installs the right one. --mxbuild-path is the override: with it a mismatch is
allowed, with a warning on stderr.

mx writes into the project it is given (theme-cache/, deployment/sass/, the
module's themesource/ folder), so it runs on a temporary copy; the project on
disk is not changed. An output directory inside the project is left out of that
copy. A Studio Pro lock file beside the project does not stop the
export, which reads what is saved on disk.

The package is named <Module>.mpk and is written to --output-dir (default: the
current directory, created if missing), replacing an earlier one.`,
	Example: `  mxcli marketplace package MyModule -p app.mpr
  mxcli marketplace package MyModule -p app.mpr -o ./dist
  mxcli marketplace package MyModule -p app.mpr --exclude '^resources.EventLog.|^themesource.atlas_core.'
  mxcli marketplace package MyModule -p app.mpr --all-libs --json`,
	Args: cobra.ExactArgs(1),
	RunE: runMarketplacePackage,
	// Like install: a failed export is a runtime failure, not a misuse of the
	// command, and main() already prints the error.
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	marketplacePackageCmd.Flags().StringP("project", "p", "", "path to the Mendix project (.mpr)")
	marketplacePackageCmd.Flags().StringP("output-dir", "o", ".", "directory to write <Module>.mpk to (created if missing)")
	marketplacePackageCmd.Flags().Bool("all-libs", false,
		"bundle every userlib JAR of the app, not only those the module declares it requires")
	marketplacePackageCmd.Flags().String("exclude", "",
		"regular expression for files to leave out, one only (join alternatives with '|'); write the folder separator as '.'")
	marketplacePackageCmd.Flags().Bool("exclude-managed-dependencies", false,
		"leave out the module's managed dependencies (mx -m)")
	marketplacePackageCmd.Flags().String("mxbuild-path", "",
		"path to the Mendix installation to take mx from; it overrides the version check (a mismatch only warns)")
	marketplacePackageCmd.Flags().Bool("json", false, "emit {path, sizeBytes, files, foreignEntries} as JSON")
	_ = marketplacePackageCmd.MarkFlagRequired("project")

	marketplaceCmd.AddCommand(marketplacePackageCmd)
}

func runMarketplacePackage(cmd *cobra.Command, args []string) error {
	var opts packageOptions
	opts.Module = args[0]
	opts.ProjectPath, _ = cmd.Flags().GetString("project")
	opts.OutputDir, _ = cmd.Flags().GetString("output-dir")
	opts.AllLibs, _ = cmd.Flags().GetBool("all-libs")
	opts.Exclude, _ = cmd.Flags().GetString("exclude")
	opts.ExcludeManaged, _ = cmd.Flags().GetBool("exclude-managed-dependencies")
	opts.MxBuildPath, _ = cmd.Flags().GetString("mxbuild-path")
	opts.JSON, _ = cmd.Flags().GetBool("json")
	_, err := runPackage(cmd.Context(), opts, cmd.OutOrStdout(), cmd.ErrOrStderr())
	return err
}

type packageOptions struct {
	ProjectPath    string
	Module         string
	OutputDir      string
	AllLibs        bool
	Exclude        string
	ExcludeManaged bool
	MxBuildPath    string
	JSON           bool
}

// packageResult is what the export produced. ForeignEntries is never nil, so the
// JSON form carries an empty array rather than null.
type packageResult struct {
	Path           string   `json:"path"`
	SizeBytes      int64    `json:"sizeBytes"`
	Files          int      `json:"files"`
	ForeignEntries []string `json:"foreignEntries"`
}

// The variables below are seams: tests replace them, because they cannot run
// Studio Pro's mx.

// packageProjectInfo reports the project's Mendix version and its module names.
var packageProjectInfo = func(mprPath string) (version string, modules []string, err error) {
	reader, err := openProjectReadOnly(mprPath)
	if err != nil {
		return "", nil, err
	}
	defer reader.Disconnect()
	version, _ = reader.GetMendixVersion()
	mods, err := reader.ListModules()
	if err != nil {
		return "", nil, err
	}
	for _, m := range mods {
		modules = append(modules, m.Name)
	}
	return version, modules, nil
}

var resolveMxForPackage = docker.ResolveMxForVersion

// mxBinaryVersion reports the Mendix version of an mx binary, "" when unknown.
var mxBinaryVersion = docker.MxBinaryVersion

// mxCreateModulePackageCmd runs mx with args and writes what it prints (stdout
// and stderr together; mx reports errors on stdout) to out. It returns mx's exit
// code; err is set when mx could not be started or ctx ended before it did (mx
// is then killed).
var mxCreateModulePackageCmd = func(ctx context.Context, mxPath string, args []string, out io.Writer) (exitCode int, err error) {
	c := exec.CommandContext(ctx, mxPath, args...)
	c.Stdout = out
	c.Stderr = out
	docker.PrepareMxCommand(c)
	runErr := c.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return -1, ctxErr
	}
	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
		return 0, nil
	case errors.As(runErr, &exitErr):
		return exitErr.ExitCode(), nil
	}
	return -1, runErr
}

// packageStagingPrefix names the directory inside the output directory that mx
// writes into. Exporting to a fresh directory, then moving the package out, is
// how a package left over from an earlier run is told apart from the one this
// run made, and guards against an mx that exits 0 without writing a package (it
// exits 1 for a pattern it cannot parse, measured on 11.12.5).
const packageStagingPrefix = ".mxcli-package-"

func runPackage(ctx context.Context, opts packageOptions, stdout, stderr io.Writer) (*packageResult, error) {
	if _, err := os.Stat(opts.ProjectPath); err != nil {
		return nil, fmt.Errorf("project not found: %s", opts.ProjectPath)
	}
	projectAbs, err := filepath.Abs(opts.ProjectPath)
	if err != nil {
		return nil, err
	}
	progress := stdout
	if opts.JSON {
		progress = io.Discard // stdout carries one JSON document and nothing else
	}

	version, modules, err := packageProjectInfo(opts.ProjectPath)
	if err != nil {
		return nil, fmt.Errorf("open project: %w", err)
	}
	module, ok := findModuleName(modules, opts.Module)
	if !ok {
		sorted := append([]string(nil), modules...)
		sort.Strings(sorted)
		return nil, fmt.Errorf("module %q not found in %s\nmodules in the project: %s",
			opts.Module, filepath.Base(opts.ProjectPath), strings.Join(sorted, ", "))
	}

	if opts.MxBuildPath != "" {
		// ResolveMxForVersion quietly falls back to an mx on PATH when this path is
		// bad, which would turn a typo into an unchecked export.
		if _, err := os.Stat(opts.MxBuildPath); err != nil {
			return nil, fmt.Errorf("--mxbuild-path %s: %w", opts.MxBuildPath, err)
		}
	}
	mxPath, err := resolveMxForPackage(opts.MxBuildPath, version)
	if err != nil {
		return nil, fmt.Errorf("locate mx for Mendix %s: %w\nhint: run 'mxcli setup mxbuild -p %s'", version, err, opts.ProjectPath)
	}
	mxVersion, err := checkMxVersion(mxPath, version, opts.MxBuildPath != "", stderr)
	if err != nil {
		return nil, err
	}

	outDir, err := filepath.Abs(opts.OutputDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// mx writes into the project it is given, so it runs on a copy; see the
	// command's Long text. The staging directory is made after the copy, and an
	// output directory inside the project is left out of it, so neither the staging
	// directory nor earlier packages are copied.
	var finalMpk, mxOutput string
	var exitCode int
	err = docker.WithProjectCopy(projectAbs, func(workMpr string) error {
		staging, err := os.MkdirTemp(outDir, packageStagingPrefix+"*")
		if err != nil {
			return fmt.Errorf("create staging directory in %s: %w", outDir, err)
		}
		defer os.RemoveAll(staging)

		fmt.Fprintf(progress, "Exporting module %s from %s (Mendix %s) with %s...\n",
			module, filepath.Base(projectAbs), version, mxDescription(mxPath, mxVersion))
		var out bytes.Buffer
		exitCode, err = mxCreateModulePackageCmd(ctx, mxPath, mxArgs(opts, staging, workMpr, module), &out)
		if err != nil {
			return fmt.Errorf("run %s: %w", mxPath, err)
		}
		mxOutput = strings.TrimSpace(nameUserPaths(out.String(), filepath.Dir(workMpr), filepath.Dir(projectAbs), staging, outDir))
		if exitCode != 0 {
			return nil
		}
		found, _ := filepath.Glob(filepath.Join(staging, "*.mpk"))
		if len(found) != 1 {
			return fmt.Errorf("mx exited 0 but wrote no .mpk%s", withMxOutput(mxOutput))
		}
		dest := filepath.Join(outDir, filepath.Base(found[0]))
		if err := os.Rename(found[0], dest); err != nil {
			return fmt.Errorf("move the package to %s: %w", dest, err)
		}
		finalMpk = dest
		if mxOutput != "" {
			// mx's warnings on a successful export; stderr, so --json stays clean.
			fmt.Fprintln(stderr, mxOutput)
		}
		return nil
	}, outDir)
	if err != nil {
		return nil, err
	}
	if exitCode != 0 {
		return nil, mxExitError(exitCode, module, mxOutput)
	}

	names, err := readPackageEntries(finalMpk)
	if err != nil {
		return nil, fmt.Errorf("read the package %s: %w", finalMpk, err)
	}
	foreign, own := classifyPackageEntries(names, module)
	info, err := os.Stat(finalMpk)
	if err != nil {
		return nil, err
	}
	res := &packageResult{Path: finalMpk, SizeBytes: info.Size(), Files: len(names), ForeignEntries: foreign}

	if len(foreign) > 0 {
		warnForeignEntries(stderr, module, foreign, own, opts.Exclude)
	}
	if opts.JSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return res, enc.Encode(res)
	}
	fmt.Fprintf(stdout, "Exported module %s\n  path:  %s\n  size:  %s (%d bytes)\n  files: %d\n",
		module, res.Path, formatBytes(res.SizeBytes), res.SizeBytes, res.Files)
	return res, nil
}

// mxArgs builds the argument list of 'mx create-module-package': options first,
// then the project and the module as the positional arguments mx expects.
func mxArgs(opts packageOptions, packageDir, targetMpr, module string) []string {
	args := []string{"create-module-package"}
	if !opts.AllLibs {
		args = append(args, "-l")
	}
	if opts.Exclude != "" {
		args = append(args, "-e", opts.Exclude)
	}
	if opts.ExcludeManaged {
		args = append(args, "-m")
	}
	return append(args, "-d", packageDir, targetMpr, module)
}

// checkMxVersion compares the Mendix version of the mx that was found with the
// project's and returns the mx's version ("" when unknown). A mismatch, or an mx
// whose version cannot be told, is refused: the package would be made by another
// Mendix version than the app it comes from. override is true when the user named
// the mx with --mxbuild-path; then the same findings are only warned about on
// stderr (which --json leaves alone).
func checkMxVersion(mxPath, projectVersion string, override bool, stderr io.Writer) (string, error) {
	got := mxBinaryVersion(mxPath)
	if projectVersion == "" {
		fmt.Fprintf(stderr, "warning: the project's Mendix version is unknown, so the version of %s was not checked\n", mxPath)
		return got, nil
	}
	if docker.SameMendixRelease(got, projectVersion) {
		return got, nil
	}
	var problem string
	if got == "" {
		problem = fmt.Sprintf("cannot tell which Mendix version %s is, and the project is Mendix %s", mxPath, projectVersion)
	} else {
		problem = fmt.Sprintf("the mx found, %s, is Mendix %s, but the project is Mendix %s", mxPath, got, projectVersion)
	}
	if override {
		fmt.Fprintf(stderr, "warning: %s; going on because --mxbuild-path was given\n", problem)
		return got, nil
	}
	return "", fmt.Errorf("%s.\nA package made by another Mendix version than the app it comes from is refused.\n"+
		"hint: run 'mxcli setup mxbuild --version %s' to install the matching toolchain (an mx on PATH is used before it,\n"+
		"so take that one off PATH); to use this mx on purpose, pass --mxbuild-path", problem, projectVersion)
}

// mxDescription names the mx for the progress line: its path, and its version when known.
func mxDescription(mxPath, mxVersion string) string {
	if mxVersion == "" {
		return mxPath
	}
	return fmt.Sprintf("%s (Mendix %s)", mxPath, mxVersion)
}

// findModuleName matches a module name exactly, then case-insensitively, and
// returns the project's own spelling, which is what mx wants.
func findModuleName(modules []string, name string) (string, bool) {
	for _, m := range modules {
		if m == name {
			return m, true
		}
	}
	for _, m := range modules {
		if strings.EqualFold(m, name) {
			return m, true
		}
	}
	return "", false
}

// nameUserPaths rewrites mx's output to name the files the user has: the
// temporary copy becomes the project, the staging directory the output directory.
func nameUserPaths(text, workDir, projectDir, staging, outDir string) string {
	return strings.NewReplacer(workDir, projectDir, staging, outDir).Replace(text)
}

func withMxOutput(output string) string {
	if output == "" {
		return ""
	}
	return "\nmx printed:\n" + output
}

// mxExitError turns the documented exit codes of 'mx create-module-package'
// into messages that say what to change.
func mxExitError(code int, module, output string) error {
	var msg string
	switch code {
	case 1:
		msg = "mx could not use the project file (exit 1): it is not a usable Mendix project, or an option was rejected (a --exclude pattern mx cannot parse fails this way)"
	case 2:
		msg = "mx could not determine the output folder (exit 2)"
	case 3:
		msg = fmt.Sprintf("mx rejected the module name %q (exit 3)", module)
	case 4:
		msg = "mx reported an unknown export error (exit 4)"
	default:
		msg = fmt.Sprintf("mx create-module-package failed (exit status %d)", code)
	}
	return errors.New(msg + withMxOutput(output))
}

// packageModuleRoots are the package folders that hold one sub-folder per
// module. Only resources/ was observed leaking another module's files into a
// package (11.12.5); javasource/ and themesource/ are checked as well, in case an
// mx version exports them. Mendix writes those two folder names in lower case.
var packageModuleRoots = map[string]bool{"resources": true, "javasource": true, "themesource": true}

// readPackageEntries lists the files in the package, folder entries left out,
// with '/' as the separator.
func readPackageEntries(mpkPath string) ([]string, error) {
	zr, err := zip.OpenReader(mpkPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	var names []string
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, `\`, "/")
		if strings.HasSuffix(name, "/") {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

// classifyPackageEntries splits entries into those under another module's
// folder — resources/<X>/…, javasource/<x>/…, themesource/<x>/… with X not the
// exported module, compared case-insensitively — and everything else. Both are
// sorted; foreign is never nil.
func classifyPackageEntries(names []string, module string) (foreign, own []string) {
	foreign = []string{}
	for _, name := range names {
		parts := strings.Split(name, "/")
		if len(parts) >= 3 && packageModuleRoots[parts[0]] && !strings.EqualFold(parts[1], module) {
			foreign = append(foreign, name)
		} else {
			own = append(own, name)
		}
	}
	sort.Strings(foreign)
	sort.Strings(own)
	return foreign, own
}

var nonWordChar = regexp.MustCompile(`[^A-Za-z0-9_]`)

// excludePatternFor builds the one --exclude expression that drops the foreign
// entries, in the form mx accepts everywhere: anchored, with '.' for the folder
// separator and for any character that is not a letter, digit or underscore —
// never a character class or a backslash. Because '.' matches any character, a
// folder whose pattern would also match one of the module's own files is left
// out of the pattern and returned in ambiguous instead.
func excludePatternFor(foreign, own []string) (pattern string, ambiguous []string) {
	groups := map[string][]string{} // "resources/EventLog" -> its entries
	for _, name := range foreign {
		parts := strings.Split(name, "/")
		key := parts[0] + "/" + parts[1]
		groups[key] = append(groups[key], name)
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var patterns []string
	for _, key := range keys {
		p := "^" + nonWordChar.ReplaceAllString(key, ".") + "."
		re := regexp.MustCompile(p)
		clash := false
		for _, o := range own {
			if re.MatchString(o) {
				clash = true
				break
			}
		}
		if clash {
			ambiguous = append(ambiguous, groups[key]...)
			continue
		}
		patterns = append(patterns, p)
	}
	return strings.Join(patterns, "|"), ambiguous
}

// shellQuote quotes s as one word for a POSIX shell: in single quotes, with each
// single quote inside closing the quote, adding an escaped quote, and reopening it.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// warnForeignEntries tells the user the package holds files of other modules and
// how to leave them out. It is a warning, not an error: the package is valid,
// just larger than the module.
func warnForeignEntries(w io.Writer, module string, foreign, own []string, existingExclude string) {
	const maxListed = 20
	fmt.Fprintf(w, "warning: the package holds %d file(s) that belong to other modules, not to %s:\n", len(foreign), module)
	for i, name := range foreign {
		if i == maxListed {
			fmt.Fprintf(w, "  ... and %d more\n", len(foreign)-maxListed)
			break
		}
		fmt.Fprintf(w, "  %s\n", name)
	}
	pattern, ambiguous := excludePatternFor(foreign, own)
	if pattern != "" {
		if existingExclude != "" {
			pattern = existingExclude + "|" + pattern
			fmt.Fprintln(w, "To leave them out, export again with this --exclude (yours first):")
		} else {
			fmt.Fprintln(w, "To leave them out, export again with:")
		}
		fmt.Fprintf(w, "  --exclude %s\n", shellQuote(pattern))
	}
	if len(ambiguous) > 0 {
		fmt.Fprintf(w, "%d of them (e.g. %s) cannot be left out by a pattern that is safe for %s:\n"+
			"  it would also match files of the module itself. Remove them from the project's\n"+
			"  resources/, javasource/ or themesource/ folder instead.\n", len(ambiguous), ambiguous[0], module)
	}
}
