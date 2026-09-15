// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	modelsdk "github.com/mendixlabs/mxcli"
	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/executor"
)

// InstalledModule reports what the project records about a marketplace module:
// the version it was installed from, and the project's own Mendix version.
//
// The recorded AppStoreVersion is what makes "have I changed this?" answerable
// at all — it names the published package the installed copy started life as.
// A module with no recorded version was not installed from the marketplace (or
// was imported by hand), and cannot be compared against anything.
func InstalledModule(mprPath, moduleName string) (appStoreVersion, mendixVersion string, err error) {
	reader, err := modelsdk.Open(mprPath)
	if err != nil {
		return "", "", fmt.Errorf("open %s: %w", mprPath, err)
	}
	defer reader.Disconnect()

	mendixVersion, _ = reader.GetMendixVersion()

	mods, err := reader.ListModules()
	if err != nil {
		return "", "", fmt.Errorf("list modules: %w", err)
	}
	for _, m := range mods {
		if !strings.EqualFold(m.Name, moduleName) {
			continue
		}
		if m.AppStoreVersion == "" {
			return "", mendixVersion, fmt.Errorf(
				"module %q records no marketplace version, so there is no published package to compare it against",
				m.Name)
		}
		return m.AppStoreVersion, mendixVersion, nil
	}
	return "", mendixVersion, fmt.Errorf("module %q not found in %s", moduleName, filepath.Base(mprPath))
}

// ModuleForVersionIDs works out which module in a project came from a given
// piece of marketplace content, and which published version it is.
//
// The name is not derivable from the marketplace listing — content 23513 is
// listed as "Administration module" but installs a module called
// "Administration", and "Data Widgets" installs "DataWidgets". The package
// declares the name, but downloading a package to learn which module to compare
// puts the download before the decision of *which version* to download.
//
// The project answers both questions without a network call. Each installed
// module records `AppStoreGuid`, and that GUID is the marketplace **version**
// UUID: in a blank 11.12.1 project, Administration carries
// 2059615c-c6f1-4103-aedb-14820c077a1c, which is exactly content 23513's
// version 4.3.2, and DataWidgets carries content 116540's version 3.5.0.
//
// Matching on the GUID rather than the version *number* is what makes this
// sound. Version numbers collide across content — that same blank project has
// Atlas_Web_Content at 4.1.0 and Administration's content has also published a
// 4.1.0, so a number match picks two modules and cannot tell them apart. The
// returned version ID is then used to fetch the exact package, so the
// comparison never depends on parsing or matching a version string.
func ModuleForVersionIDs(mprPath string, versionIDs []string) (moduleName, versionID string, err error) {
	published := make(map[string]bool, len(versionIDs))
	for _, id := range versionIDs {
		if id != "" {
			published[id] = true
		}
	}

	reader, err := modelsdk.Open(mprPath)
	if err != nil {
		return "", "", fmt.Errorf("open %s: %w", mprPath, err)
	}
	defer reader.Disconnect()

	mods, err := reader.ListModules()
	if err != nil {
		return "", "", fmt.Errorf("list modules: %w", err)
	}

	type match struct{ name, id string }
	var matches []match
	for _, m := range mods {
		if published[m.AppStoreGuid] {
			matches = append(matches, match{m.Name, m.AppStoreGuid})
		}
	}
	switch len(matches) {
	case 1:
		return matches[0].name, matches[0].id, nil
	case 0:
		return "", "", fmt.Errorf("no module in %s was installed from this marketplace content",
			filepath.Base(mprPath))
	default:
		names := make([]string, 0, len(matches))
		for _, m := range matches {
			names = append(names, m.name)
		}
		sort.Strings(names)
		return "", "", fmt.Errorf("several modules record a version of this content (%s), so the right one cannot be identified",
			strings.Join(names, ", "))
	}
}

// PackageProject builds a throwaway project containing nothing but the module
// from mpkPath, and returns the path to its .mpr.
//
// The project is created at mendixVersion — the consuming project's version, not
// the latest — so the imported module goes through the same conversion the
// installed copy went through. Comparing against a package converted to a
// different Mendix version would report the platform's own migrations as user
// edits.
//
// workDir must already exist and is written to freely; callers own its lifetime
// (t.TempDir in tests, an os.MkdirTemp the command removes in production).
// **Keep it short**: mx create-project extracts its template with .NET path
// handling and fails with PathTooLongException under a deep directory, so a
// nested scratch path breaks it where /tmp/xxxx works.
//
// newBackend is used to remove a module the template already ships (see below).
func PackageProject(ctx context.Context, mpkPath, mendixVersion, workDir string, newBackend func() backend.FullBackend) (string, error) {
	mxPath, err := docker.ResolveMxForVersion("", mendixVersion)
	if err != nil {
		return "", fmt.Errorf("locate mx for Mendix %s: %w\n"+
			"hint: run 'mxcli setup mxbuild --version %s'", mendixVersion, err, mendixVersion)
	}

	if err := blankProjectInto(ctx, mxPath, mendixVersion, workDir); err != nil {
		return "", err
	}

	mprPath, err := findScratchMpr(workDir, blankAppName)
	if err != nil {
		return "", err
	}

	// Verify the reference project really is at the requested version.
	//
	// This is not paranoia: ResolveMxForVersion falls back to any cached mxbuild
	// when the requested version is missing — asking for 11.6.6 on a machine
	// holding 11.12.1 returns 11.12.1 without complaint. mx create-project stamps
	// the project with the version of the binary that ran it, so the reference
	// would silently be built one or more Mendix versions away from the project
	// being compared, and every platform migration between them would surface as
	// a user edit. A tool whose false positives are indistinguishable from real
	// findings is worse than no tool, so this refuses instead of warning.
	if err := verifyProjectVersion(mprPath, mendixVersion); err != nil {
		return "", err
	}

	// A "blank" Mendix project is not empty: the template ships Administration,
	// Atlas_Core, Atlas_Web_Content and friends already installed. module-import
	// refuses a name that already exists ("Module 'X' already exist in the app",
	// exit 47) — and Administration is exactly the module the field report cares
	// about. So remove the template's copy first, then import the published one.
	//
	// mxcli's own DROP MODULE is used rather than a file edit because it also
	// unpicks the references the template set up (it reports e.g. "Removed
	// Administration.User from 1 user role(s)"); leaving those dangling would
	// give module-import an inconsistent model to write into.
	if name, err := ModuleNameInPackage(mpkPath); err == nil && name != "" {
		if err := dropModuleIfPresent(mprPath, name, newBackend); err != nil {
			return "", err
		}
	}

	// mx refuses a theme module (Atlas_Core, Atlas_Web_Content, Conversational
	// UI); makeImportable clears the flag that gates the refusal, on a copy.
	importable, err := makeImportable(mpkPath, workDir)
	if err != nil {
		return "", err
	}

	// Deliberately NOT guarded against the MPR v2→v1 collapse that
	// `marketplace install` refuses. This import rewrites the *reference*
	// project — a scratch copy in a temp directory that is read once and
	// deleted — so the format it ends up in does not matter. It is also
	// evidence for the comparison design: the reference lands as v1 while the
	// project under comparison stays v2, and the diff is still exact, because
	// DESCRIBE output does not depend on how the model is stored.
	imp := exec.CommandContext(ctx, mxPath, "module-import", importable, mprPath)
	docker.PrepareMxCommand(imp)
	if out, err := imp.CombinedOutput(); err != nil {
		return "", fmt.Errorf("mx module-import failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return mprPath, nil
}

// blankAppName is what every reference project is created as. It is fixed
// rather than per-run so a cached blank tree can be reused verbatim.
const blankAppName = "PackageRef"

// blankProjectInto puts a pristine blank Mendix project into workDir, from the
// cache when one is there and from `mx create-project` when not.
//
// `mx create-project` is the single most expensive step in building a reference
// (~12s of ~25s) and its result depends on nothing but the Mendix version, so a
// run updating six modules paid for twelve identical blank apps. The version
// stamp is verified BEFORE anything is cached, so a cache entry can never carry
// the wrong version — which matters more than the time, because comparing across
// versions reports Mendix's own conversions as user edits.
func blankProjectInto(ctx context.Context, mxPath, mendixVersion, workDir string) error {
	cacheDir, cerr := blankCacheDir(mendixVersion)
	if cerr == nil && cacheReady(cacheDir) {
		if err := copyTree(cacheDir, workDir); err == nil {
			return nil
		}
		// A cache entry that cannot be copied is not worth diagnosing: drop it and
		// build. Serving a partial project would surface as bogus diff findings.
		_ = os.RemoveAll(cacheDir)
	}

	create := exec.CommandContext(ctx, mxPath, "create-project", "--app-name", blankAppName)
	create.Dir = workDir
	docker.PrepareMxCommand(create)
	if out, err := create.CombinedOutput(); err != nil {
		return fmt.Errorf("mx create-project failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	if cerr != nil {
		return nil
	}

	// Verify before caching, not after serving. A blank app stamped with the
	// wrong version (ResolveMxForVersion falls back to any cached mxbuild) must
	// not become the entry every later run trusts.
	mprPath, err := findScratchMpr(workDir, blankAppName)
	if err != nil {
		return err
	}
	if err := verifyProjectVersion(mprPath, mendixVersion); err != nil {
		return err
	}
	cacheBlankProject(workDir, cacheDir)
	return nil
}

// cacheBlankProject copies a just-built blank project into the cache. Failure is
// silent by design: the caller already has a working project in workDir, and a
// cache that cannot be written is a slow tool, not a broken one.
//
// It copies rather than moves because workDir is what the caller goes on to use.
// The copy lands beside the cache entry and is renamed in, so a process killed
// mid-copy leaves a stray directory rather than a half-entry.
func cacheBlankProject(workDir, cacheDir string) {
	if refCacheDisabled() {
		return
	}
	staging, err := os.MkdirTemp(filepath.Dir(cacheDir), "building-")
	if err != nil {
		if err := os.MkdirAll(filepath.Dir(cacheDir), 0o755); err != nil {
			return
		}
		if staging, err = os.MkdirTemp(filepath.Dir(cacheDir), "building-"); err != nil {
			return
		}
	}
	if err := copyTree(workDir, staging); err != nil {
		_ = os.RemoveAll(staging)
		return
	}
	if err := publishToCache(staging, cacheDir); err != nil {
		_ = os.RemoveAll(staging)
	}
}

// A cache entry holds both halves of a reference, because both are needed and
// re-downloading one to serve the other would give back most of the saving:
//
//	<entry>/.mxcli-complete   written last; its absence means "rebuild"
//	<entry>/package.mpk       the published package
//	<entry>/project/          the reference project built from it
//
// The package is kept because `marketplace update` takes the module's bundled
// widgets from the .mpk rather than from the reference project, whose widgets/
// also holds the blank template's.
const (
	entryProject = "project"
	entryPackage = "package.mpk"
)

// CachedReference restores a previously built reference for this published
// version: the project tree into destDir, the package to mpkDest. It returns the
// path to the reference .mpr, or "" when there is no usable entry.
//
// Callers get a copy. PerformUpdate and SnapshotModule only read, but handing
// out the cached tree itself would let one careless writer poison every later
// run — the cost of being wrong here is silent, wrong diff findings.
func CachedReference(versionID, mendixVersion, destDir, mpkDest string) string {
	if versionID == "" {
		return ""
	}
	cacheDir, err := refCacheDir(versionID, mendixVersion)
	if err != nil || !cacheReady(cacheDir) {
		return ""
	}
	drop := func() string {
		// An entry that cannot be served is dropped rather than diagnosed: it will
		// be rebuilt on this very run, and a partial reference reads as local edits.
		_ = os.RemoveAll(cacheDir)
		return ""
	}

	if err := copyTree(filepath.Join(cacheDir, entryProject), destDir); err != nil {
		return drop()
	}
	if err := copyFile(filepath.Join(cacheDir, entryPackage), mpkDest, 0o644); err != nil {
		return drop()
	}
	mprPath, err := findScratchMpr(destDir, blankAppName)
	if err != nil {
		return drop()
	}
	// The stamp is re-checked on the way out, not only on the way in. An entry
	// written by an older mxcli, or before this guard was tightened, is dropped
	// rather than trusted — the comparison's whole validity rests on it.
	if err := verifyProjectVersion(mprPath, mendixVersion); err != nil {
		return drop()
	}
	touchEntry(cacheDir)
	return mprPath
}

// CacheReference stores a finished reference so the next `diff` or `update` of
// the same published version skips the download and both mx invocations.
//
// Failures are silent for the same reason as cacheBlankProject: the caller
// already has what it needs, and a cache that cannot be written makes the tool
// slow, not wrong.
func CacheReference(versionID, mendixVersion, refDir, mpkPath string) {
	if versionID == "" || refCacheDisabled() {
		return
	}
	cacheDir, err := refCacheDir(versionID, mendixVersion)
	if err != nil || os.MkdirAll(filepath.Dir(cacheDir), 0o755) != nil {
		return
	}
	staging, err := os.MkdirTemp(filepath.Dir(cacheDir), "building-")
	if err != nil {
		return
	}
	// Model only: see isModelFile for what is dropped, why nothing reads it, and
	// what that constrains.
	if err := copyTreeModelOnly(refDir, filepath.Join(staging, entryProject)); err != nil {
		_ = os.RemoveAll(staging)
		return
	}
	if err := copyFile(mpkPath, filepath.Join(staging, entryPackage), 0o644); err != nil {
		_ = os.RemoveAll(staging)
		return
	}
	if err := publishToCache(staging, cacheDir); err != nil {
		_ = os.RemoveAll(staging)
		return
	}
	// Prune after publishing, not before: the entry just written is the newest,
	// so it survives its own prune, and a bound of 1 still works.
	pruneRefCache(refCacheMaxEntries())
}

// findScratchMpr locates the project mx just created. The name follows
// --app-name, but that is not contractual, so fall back to whatever .mpr exists.
func findScratchMpr(workDir, appName string) (string, error) {
	named := filepath.Join(workDir, appName+".mpr")
	if _, err := os.Stat(named); err == nil {
		return named, nil
	}
	matches, _ := filepath.Glob(filepath.Join(workDir, "*.mpr"))
	if len(matches) > 0 {
		return matches[0], nil
	}
	// mx may nest the project one level down under the app name.
	matches, _ = filepath.Glob(filepath.Join(workDir, "*", "*.mpr"))
	if len(matches) > 0 {
		return matches[0], nil
	}
	return "", fmt.Errorf("mx create-project produced no .mpr under %s", workDir)
}

// verifyProjectVersion checks that a freshly created project is stamped with the
// version we asked for, and returns an actionable error when it is not.
//
// The stamped version is read from the project rather than inferred from the mx
// binary's path, because the path is a cache-layout detail while the stamp is
// what the comparison actually depends on.
func verifyProjectVersion(mprPath, want string) error {
	reader, err := modelsdk.Open(mprPath)
	if err != nil {
		return fmt.Errorf("open reference project: %w", err)
	}
	got, _ := reader.GetMendixVersion()
	_ = reader.Disconnect()

	if got == want {
		return nil
	}
	return fmt.Errorf(
		"reference project was built at Mendix %s but the project under comparison is %s.\n"+
			"Comparing across versions reports Mendix's own conversions as local edits, so this is refused.\n"+
			"hint: run 'mxcli setup mxbuild --version %s' to fetch the matching toolchain",
		orUnknown(got), want, want)
}

func orUnknown(v string) string {
	if v == "" {
		return "an unknown version"
	}
	return v
}

// ModuleNameInPackage reads the module name a .mpk declares. package.xml is the
// manifest mx itself reads; note it carries no version — the AppStoreVersion a
// project ends up with after `mx module-import` is the module's *internal*
// version (set by its author), not the marketplace release number. Importing
// Administration 4.3.2 stamps 2.0.1. That stamp never reaches DESCRIBE output,
// so it does not affect the comparison, but it does mean the reference project's
// recorded version is not evidence of which package was imported.
func ModuleNameInPackage(mpkPath string) (string, error) {
	zr, err := zip.OpenReader(mpkPath)
	if err != nil {
		return "", fmt.Errorf("open package %s: %w", filepath.Base(mpkPath), err)
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.Name != "package.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()

		var manifest struct {
			Module struct {
				Name string `xml:"name,attr"`
			} `xml:"modelerProject>module"`
		}
		if err := xml.NewDecoder(rc).Decode(&manifest); err != nil {
			return "", fmt.Errorf("parse package.xml: %w", err)
		}
		return manifest.Module.Name, nil
	}
	return "", fmt.Errorf("no package.xml in %s", filepath.Base(mpkPath))
}

// dropModuleIfPresent removes a module from the reference project when the
// template already provides it. A module that is not there is not an error.
func dropModuleIfPresent(mprPath, moduleName string, newBackend func() backend.FullBackend) error {
	var sink bytes.Buffer
	ex := executor.New(&sink)
	defer ex.Close()
	if newBackend != nil {
		ex.SetBackendFactory(newBackend)
	}
	if err := ex.Execute(&ast.ConnectStmt{Path: mprPath}); err != nil {
		return fmt.Errorf("connect reference project: %w", err)
	}

	mods, err := ex.Backend().ListModules()
	if err != nil {
		return fmt.Errorf("list reference modules: %w", err)
	}
	for _, m := range mods {
		if strings.EqualFold(m.Name, moduleName) {
			if err := ex.Execute(&ast.DropModuleStmt{Name: m.Name}); err != nil {
				return fmt.Errorf("remove the template's %s before importing the package: %w", m.Name, err)
			}
			return nil
		}
	}
	return nil
}
