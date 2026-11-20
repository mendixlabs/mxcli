// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	modelsdk "github.com/mendixlabs/mxcli"
	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	mp "github.com/mendixlabs/mxcli/cmd/mxcli/marketplace"
	"github.com/mendixlabs/mxcli/internal/marketplace"
	"github.com/mendixlabs/mxcli/internal/zipentry"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/spf13/cobra"
)

var marketplaceInstallCmd = &cobra.Command{
	Use:   "install <content-id> | install --file <package.mpk>",
	Short: "Download and install a marketplace item into a project",
	Long: `Download a marketplace content version and install it into a project,
or install a .mpk you already have on disk with --file.

Install is type-aware:
  - Widget      copied into the project's widgets/ folder (overwrites on update)
  - Module      copied in with mxcli's own writer (new modules only)
  - other types downloaded to disk with import instructions

Updating a module that is already present is NOT done by install: it could
discard local edits and, for modules with persistent entities, change entity
IDs (which loses data). Install reports the module and names the command that
updates it — 'marketplace diff' to see what an update would touch, then
'marketplace update <content-id> -p <project.mpr> --to <version>', which carries
the entity identity and role grants across.

A module is installed by copying its units with mxcli's own writer rather than
by running 'mx module-import'. That preserves the project's storage format —
module-import rewrites MPR v2 as v1, collapsing mprcontents/ into a single
binary .mpr, one-way — and it works for theme modules, which module-import
refuses outright. Everything the package ships (widgets, themesource, ...) is
installed alongside the model.

--allow-format-change selects the legacy module-import path instead.

After a module or widget install, the repairs a headless install leaves for
Mendix's own tools run automatically — the widget resync (CE0463) and, for a
module, the design-property rename (CE6087), the same steps as 'mxcli fix
widgets' / 'mxcli fix design-properties', which keep MPR v2. When mx for the
project's version cannot be resolved, those commands are printed instead.

--file <package.mpk> installs a package from disk through the same writer, with
no marketplace lookup and no PAT: the kind (module or widget) is read from the
package's own package.xml. That is the route for a module distributed as a file
— an internal or company-standard module, a theme module, a package fetched
earlier with 'marketplace download', or any CI/air-gapped environment. A module
installed this way carries no marketplace version stamp, because it has none;
'marketplace update' and 'diff' will not claim it.`,
	Example: `  mxcli marketplace install 20 -p app.mpr
  mxcli marketplace install 2888 --version 7.0.3 -p app.mpr
  mxcli marketplace install --file ./CompanyTheme.mpk -p app.mpr`,
	Args: cobra.MaximumNArgs(1),
	RunE: runMarketplaceInstall,
	// A failed install/update/diff is a runtime failure, not a misuse of the
	// command: printing the full flag list on top of the error buries it.
	// SilenceErrors too, because main() already prints what Execute returns —
	// without it every refusal is printed twice.
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	marketplaceInstallCmd.Flags().String("profile", "default", "credential profile")
	marketplaceInstallCmd.Flags().StringP("project", "p", "", "path to the Mendix project (.mpr)")
	marketplaceInstallCmd.Flags().String("version", "", "version number to install (default: latest)")
	marketplaceInstallCmd.Flags().Bool("allow-format-change", false,
		"use the legacy 'mx module-import' path, which rewrites an MPR v2 project as v1 (one-way)")
	marketplaceInstallCmd.Flags().String("file", "",
		"install a .mpk from disk instead of marketplace content (no PAT, no lookup)")
	_ = marketplaceInstallCmd.MarkFlagRequired("project")

	marketplaceCmd.AddCommand(marketplaceInstallCmd)
}

func runMarketplaceInstall(cmd *cobra.Command, args []string) error {
	mprPath, _ := cmd.Flags().GetString("project")
	if _, err := os.Stat(mprPath); err != nil {
		return fmt.Errorf("project not found: %s", mprPath)
	}
	versionNumber, _ := cmd.Flags().GetString("version")
	allowFormatChange, _ := cmd.Flags().GetBool("allow-format-change")

	// A package on disk: no marketplace client is constructed at all, so no PAT
	// is needed and nothing is fetched. Everything after the download step is
	// shared with the online path.
	if filePath, _ := cmd.Flags().GetString("file"); filePath != "" {
		if len(args) != 0 {
			return fmt.Errorf("give either a content id or --file, not both")
		}
		if versionNumber != "" {
			return fmt.Errorf("--version selects a marketplace release; it does not apply to --file")
		}
		return installFromFile(cmd.Context(), filePath, mprPath, allowFormatChange, cmd.OutOrStdout())
	}
	if len(args) != 1 {
		return fmt.Errorf("a content id is required (or --file <package.mpk> for a package on disk)")
	}
	contentID, err := parseContentID(args[0])
	if err != nil {
		return err
	}

	client, err := newMarketplaceClient(cmd.Context(), cmd)
	if err != nil {
		return err
	}

	content, err := client.Get(cmd.Context(), contentID)
	if err != nil {
		return err
	}
	verList, err := client.Versions(cmd.Context(), contentID)
	if err != nil {
		return err
	}
	version, err := resolveVersion(verList.Items, versionNumber)
	if err != nil {
		return err
	}
	if version.DownloadURL == "" {
		return fmt.Errorf("version %s exposes no download URL", version.VersionNumber)
	}

	projDir := filepath.Dir(mprPath)
	out := cmd.OutOrStdout()

	switch strings.ToLower(content.Type) {
	case "widget":
		return installWidget(cmd.Context(), client, version, mprPath, out)
	case "module":
		// Refuse a version the project's Mendix cannot import, before spending a
		// download and a reference build on it. Only modules go through
		// module-import, so only modules are gated here.
		if projectVer := mendixVersionOf(mprPath); projectVer != "" {
			if err := checkMendixCompatibility(version, verList.Items, projectVer, version.Name); err != nil {
				return err
			}
		}
		return installModule(cmd.Context(), client, contentID, version, mprPath, allowFormatChange, out)
	default:
		// Theme / Starter App / Sample / unknown: download + instruct rather
		// than guess a placement we can't verify.
		path, err := fetchMpkToFile(cmd.Context(), client, version, projDir)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Downloaded %s content to %s\n", content.Type, path)
		fmt.Fprintf(out, "This content type is not auto-installed. Import it via Studio Pro.\n")
		return nil
	}
}

// installFromFile installs a package that is already on disk. The package's
// own package.xml says what it is: a module (installed through the transplant
// writer, exactly as an online install would be) or a widget (placed under
// widgets/). Anything else is refused rather than guessed at.
func installFromFile(ctx context.Context, mpkPath, mprPath string, allowFormatChange bool, out io.Writer) error {
	if _, err := os.Stat(mpkPath); err != nil {
		return fmt.Errorf("package not found: %s", mpkPath)
	}
	pkg, err := readPackageXML(mpkPath)
	if err != nil {
		return fmt.Errorf("inspect package: %w", err)
	}
	switch {
	case pkg.ModelerProject.Module.Name != "":
		return installModuleFromFile(ctx, mpkPath, mprPath, allowFormatChange, 0, "", "",
			"from "+filepath.Base(mpkPath), out)
	case pkg.ClientModule.Name != "":
		return placeWidgetFile(mpkPath, mprPath, out)
	}
	return fmt.Errorf("%s is neither a module nor a widget package: its package.xml names no module and no clientModule",
		filepath.Base(mpkPath))
}

// installWidget copies the widget .mpk into the project's widgets/ folder.
// An existing file with the same name is overwritten (the update path).
func installWidget(ctx context.Context, client *marketplace.Client, v *marketplace.Version, mprPath string, out io.Writer) error {
	widgetsDir := filepath.Join(filepath.Dir(mprPath), "widgets")
	if err := os.MkdirAll(widgetsDir, 0o755); err != nil {
		return fmt.Errorf("create widgets dir: %w", err)
	}
	dest, err := fetchMpkToFile(ctx, client, v, widgetsDir)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Installed widget %s into %s\n", v.VersionNumber, dest)
	resyncInstalledWidget(out, mprPath)
	return nil
}

// placeWidgetFile is installWidget for a package already on disk: the same
// destination and the same overwrite-on-update rule, without the download.
func placeWidgetFile(mpkPath, mprPath string, out io.Writer) error {
	widgetsDir := filepath.Join(filepath.Dir(mprPath), "widgets")
	if err := os.MkdirAll(widgetsDir, 0o755); err != nil {
		return fmt.Errorf("create widgets dir: %w", err)
	}
	body, err := os.ReadFile(mpkPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", mpkPath, err)
	}
	dest := filepath.Join(widgetsDir, filepath.Base(mpkPath))
	if err := os.WriteFile(dest, body, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", dest, err)
	}
	fmt.Fprintf(out, "Installed widget %s into %s\n", filepath.Base(mpkPath), dest)
	resyncInstalledWidget(out, mprPath)
	return nil
}

// resyncInstalledWidget runs the widget resync a new or updated widget package
// needs before the project's stored instances match it (CE0463) — what
// 'mxcli fix widgets' does. Without mx it prints that command instead.
func resyncInstalledWidget(out io.Writer, mprPath string) {
	runPostInstallRepairs(out, out, mprPath, postInstallMendixVersion(mprPath), widgetRepairs)
}

// installModule imports a module .mpk into the project, but only when the module
// is not already present. An existing module is reported, not modified.
func installModule(ctx context.Context, client *marketplace.Client, contentID int, v *marketplace.Version, mprPath string, allowFormatChange bool, out io.Writer) error {
	// Download to a temp .mpk so we can inspect it and hand it to mx.
	tmpDir, err := os.MkdirTemp("", "mxcli-install-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	mpkPath, err := fetchMpkToFile(ctx, client, v, tmpDir)
	if err != nil {
		return err
	}
	return installModuleFromFile(ctx, mpkPath, mprPath, allowFormatChange, contentID, v.VersionNumber, v.VersionID,
		"version "+v.VersionNumber, out)
}

// installModuleFromFile is the part of a module install that starts once the
// package is on disk — shared by the online path (after its download) and by
// --file (which has no download). versionNumber and versionID are the
// marketplace identity to record on the module; both are empty for a package
// from disk, which has none, and so is contentID (0). label is how the package
// is named in output.
func installModuleFromFile(ctx context.Context, mpkPath, mprPath string, allowFormatChange bool,
	contentID int, versionNumber, versionID, label string, out io.Writer) error {

	moduleName, err := moduleNameFromMpk(mpkPath)
	if err != nil {
		return fmt.Errorf("inspect package: %w", err)
	}

	// Is the module already in the project?
	reader, err := modelsdk.Open(mprPath)
	if err != nil {
		return fmt.Errorf("open project: %w", err)
	}
	existing, installedVer := findModule(reader, moduleName)
	mendixVer, _ := reader.GetMendixVersion()
	_ = reader.Disconnect()

	if existing {
		// Install never updates implicitly: an update is its own command.
		reportModuleAlreadyInstalled(out, moduleName, installedVer, label, contentID, versionNumber, mprPath)
		return nil
	}

	// Default path: copy the module in with mxcli's own writer, which preserves
	// the project's storage format and works for theme modules. --allow-format-change
	// selects the legacy `mx module-import`, which does neither.
	if !allowFormatChange {
		res, ierr := installByTransplant(ctx, mpkPath, mprPath, moduleName, mendixVer, versionNumber, versionID)
		if ierr != nil {
			return ierr
		}
		reportTransplantInstall(out, mprPath, mendixVer, moduleName, label, res)
		return nil
	}

	mxPath, err := docker.ResolveMxForVersion("", mendixVer)
	if err != nil {
		return fmt.Errorf("locate mx for Mendix %s: %w\nhint: run 'mxcli setup mxbuild -p %s'", mendixVer, err, mprPath)
	}

	c := exec.CommandContext(ctx, mxPath, "module-import", mpkPath, mprPath)
	docker.PrepareMxCommand(c)
	combined, runErr := c.CombinedOutput()
	if runErr != nil {
		return fmt.Errorf("mx module-import failed: %w\n%s", runErr, strings.TrimSpace(string(combined)))
	}
	fmt.Fprintf(out, "Imported module %q %s into %s\n", moduleName, label, filepath.Base(mprPath))
	reportFormatChange(mprPath, out)
	syncInstalledJarDependencies(out, mprPath)
	return nil
}

// reportTransplantInstall reports a module copied in by the transplant writer,
// then runs the repairs a headless install leaves (CE0463, CE6087) — the same
// calls 'mxcli fix widgets' / 'mxcli fix design-properties' make, which keep the
// MPR v2 storage format (mendixlabs/mxcli#1527). Without mx it prints them.
func reportTransplantInstall(out io.Writer, mprPath, mendixVer, moduleName, label string, res *mp.UpdateResult) {
	fmt.Fprintf(out, "Installed module %q %s into %s\n",
		moduleName, label, filepath.Base(mprPath))
	fmt.Fprintf(out, "  %d units copied, %d bundled file(s) installed.\n",
		res.UnitsCopied, len(res.FilesInstalled))
	reportAddOnModules(out, res.FilesInstalled)
	reportSkippedFiles(out, res.FilesSkipped)
	reportSecurityReconcile(out, res)
	// Before the repairs, whose output ends with the Next step.
	syncInstalledJarDependencies(out, mprPath)
	runPostInstallRepairs(out, out, mprPath, mendixVer, moduleRepairs)
}

// installByTransplant builds a reference project from the package and copies the
// module out of it, so the destination keeps its MPR format.
func installByTransplant(ctx context.Context, mpkPath, mprPath, moduleName, mendixVer string,
	versionNumber, versionID string) (*mp.UpdateResult, error) {

	work, err := mp.NewWorkDir("mxi")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)

	refDir := filepath.Join(work, "r")
	if err := os.MkdirAll(refDir, 0o755); err != nil {
		return nil, err
	}
	refMpr, err := mp.PackageProject(ctx, mpkPath, mendixVer, refDir, newBackendFactory())
	if err != nil {
		return nil, fmt.Errorf("build a reference project from the package: %w", err)
	}
	return mp.PerformInstall(mprPath, refMpr, mpkPath, moduleName, versionNumber, versionID, newBackendFactory())
}

// isMPRv2 reports whether the project at mprPath uses the MPR v2 storage format:
// a small .mpr holding metadata beside an mprcontents/ tree of one .mxunit file
// per document. The presence of that directory is how the readers themselves
// decide, so this asks the same question they do rather than inventing a second
// definition.
func isMPRv2(mprPath string) bool {
	stat, err := os.Stat(filepath.Join(filepath.Dir(mprPath), "mprcontents"))
	return err == nil && stat.IsDir()
}

// checkStorageFormatPreserved refuses to import into an MPR v2 project.
//
// `mx module-import` rewrites a v2 project as v1: measured on a blank Mendix
// 11.12.1 app, one import turned a 69 KB .mpr plus 341 .mxunit files into a
// single 14 MB SQLite blob with no mprcontents/ and no _Transaction table, and
// the same was observed independently on 11.13.0. It is a silent, one-way
// conversion — `mx convert` targets Mendix versions rather than storage formats,
// and there is no documented way back.
//
// That is not a cosmetic difference. The v2 layout is what makes the model
// diffable and mergeable per document: it is what `mxcli diff-local` reads, and
// what makes an idempotent re-run observable as "no files changed" (ADR-0008).
// Collapsing it destroys the reviewable history of a repository, and the damage
// lands on the user's real project rather than on a scratch copy.
//
// So this refuses by default rather than warning. The escape hatch exists
// because the conversion is a legitimate choice for someone who does not keep
// the model in git — but it has to be chosen, not discovered afterwards.
func checkStorageFormatPreserved(mprPath string, allowed bool) error {
	if allowed || !isMPRv2(mprPath) {
		return nil
	}
	return fmt.Errorf(
		"refusing to import: %s uses the MPR v2 storage format, and 'mx module-import' would rewrite it as v1.\n"+
			"That collapses mprcontents/ into a single binary .mpr, which loses the per-document files\n"+
			"'mxcli diff-local' and git-based review depend on. The conversion is one-way.\n"+
			"\n"+
			"  - Import the module in Studio Pro, which preserves the format; or\n"+
			"  - pass --allow-format-change to accept the conversion to MPR v1.",
		filepath.Base(mprPath))
}

// reportFormatChange states plainly what happened when the user opted in. An
// accepted conversion is still worth naming — the project on disk no longer has
// the shape its tooling expects.
func reportFormatChange(mprPath string, out io.Writer) {
	if isMPRv2(mprPath) {
		return
	}
	fmt.Fprintln(out, "\nNote: the project is now MPR v1 — mprcontents/ is gone and the model is a single")
	fmt.Fprintln(out, "binary .mpr. 'mxcli diff-local' and per-document git review no longer apply.")
}

// findModule reports whether a marketplace-sourced module of the given name is
// present, and its installed AppStore version.
func findModule(reader backend.FullBackend, name string) (found bool, appStoreVersion string) {
	mods, err := reader.ListModules()
	if err != nil {
		return false, ""
	}
	for _, m := range mods {
		if strings.EqualFold(m.Name, name) {
			return true, m.AppStoreVersion
		}
	}
	return false, ""
}

func displayVer(v string) string {
	if v == "" {
		return "unknown"
	}
	return v
}

// fetchMpkToFile downloads the version's .mpk into destDir under its CDN
// filename, atomically (temp file + rename). Returns the path written.
func fetchMpkToFile(ctx context.Context, client *marketplace.Client, v *marketplace.Version, destDir string) (string, error) {
	tmp, err := os.CreateTemp(destDir, ".mxcli-download-*.part")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed

	filename, derr := client.Download(ctx, v, tmp)
	closeErr := tmp.Close()
	if derr != nil {
		return "", derr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if filename == "" {
		filename = fmt.Sprintf("content-%s.mpk", v.VersionNumber)
	}
	finalPath := filepath.Join(destDir, filename)
	if err := os.Rename(tmpName, finalPath); err != nil {
		return "", fmt.Errorf("write %s: %w", finalPath, err)
	}
	return finalPath, nil
}

// mpkPackageXML is the minimal shape of an .mpk package.xml needed to read the
// contained module (or client-module/widget) name. Namespaces are ignored —
// Go's xml decoder matches on local element name.
type mpkPackageXML struct {
	ClientModule struct {
		Name string `xml:"name,attr"`
	} `xml:"clientModule"`
	ModelerProject struct {
		Module struct {
			Name string `xml:"name,attr"`
		} `xml:"module"`
	} `xml:"modelerProject"`
}

// readPackageXML returns the parsed package.xml of a .mpk. It is the one place
// that knows where a package describes itself, so both "what is its name" and
// "what kind of package is it" read from here.
func readPackageXML(mpkPath string) (mpkPackageXML, error) {
	var pkg mpkPackageXML
	zr, err := zip.OpenReader(mpkPath)
	if err != nil {
		return pkg, err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != "package.xml" {
			continue
		}
		rc, err := zipentry.Open(f)
		if err != nil {
			return pkg, err
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return pkg, err
		}
		if err := xml.Unmarshal(data, &pkg); err != nil {
			return pkg, err
		}
		return pkg, nil
	}
	return pkg, fmt.Errorf("no package.xml in %s", filepath.Base(mpkPath))
}

// moduleNameFromMpk reads package.xml from the .mpk and returns the module name
// (from <modelerProject><module> for modules, or <clientModule> for widgets).
func moduleNameFromMpk(mpkPath string) (string, error) {
	pkg, err := readPackageXML(mpkPath)
	if err != nil {
		return "", err
	}
	if pkg.ModelerProject.Module.Name != "" {
		return pkg.ModelerProject.Module.Name, nil
	}
	if pkg.ClientModule.Name != "" {
		return pkg.ClientModule.Name, nil
	}
	return "", fmt.Errorf("package.xml has no module name")
}

// reportModuleAlreadyInstalled says why install left a present module alone and
// names the command that updates it (mendixlabs/mxcli#1442). It used to send
// the user to Studio Pro, although 'marketplace update' does the update
// headlessly and carries the GUIDs and role grants a plain replace would lose.
// contentID and versionNumber are 0/"" for a --file install, which has neither;
// the command is then named with placeholders.
func reportModuleAlreadyInstalled(out io.Writer, moduleName, installedVer, label string, contentID int, versionNumber, mprPath string) {
	id, to := "<content-id>", "<version>"
	if contentID > 0 {
		id = strconv.Itoa(contentID)
	}
	if versionNumber != "" {
		to = versionNumber
	}
	fmt.Fprintf(out, "Module %q is already installed (version %s).\n", moduleName, displayVer(installedVer))
	fmt.Fprintf(out, "Target: %s.\n", label)
	fmt.Fprintln(out, "It is not updated by install (an update can discard local edits). To update it,")
	fmt.Fprintln(out, "review what would change, then apply it with entity identity and role grants preserved:")
	fmt.Fprintf(out, "  mxcli marketplace diff %s -p %s --to %s\n", id, mprPath, to)
	fmt.Fprintf(out, "  mxcli marketplace update %s -p %s --to %s\n", id, mprPath, to)
}
