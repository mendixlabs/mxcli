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
	"strings"

	modelsdk "github.com/mendixlabs/mxcli"
	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	mp "github.com/mendixlabs/mxcli/cmd/mxcli/marketplace"
	"github.com/mendixlabs/mxcli/internal/marketplace"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/spf13/cobra"
)

var marketplaceInstallCmd = &cobra.Command{
	Use:   "install <content-id>",
	Short: "Download and install a marketplace item into a project",
	Long: `Download a marketplace content version and install it into a project.

Install is type-aware:
  - Widget      copied into the project's widgets/ folder (overwrites on update)
  - Module      copied in with mxcli's own writer (new modules only)
  - other types downloaded to disk with import instructions

Updating a module that is already present is NOT done automatically: it could
discard local edits and, for modules with persistent entities, change entity
IDs (which loses data). Such updates are reported and left to Studio Pro.

A module is installed by copying its units with mxcli's own writer rather than
by running 'mx module-import'. That preserves the project's storage format —
module-import rewrites MPR v2 as v1, collapsing mprcontents/ into a single
binary .mpr, one-way — and it works for theme modules, which module-import
refuses outright. Everything the package ships (widgets, themesource, ...) is
installed alongside the model.

--allow-format-change selects the legacy module-import path instead.`,
	Example: `  mxcli marketplace install 20 -p app.mpr
  mxcli marketplace install 2888 --version 7.0.3 -p app.mpr`,
	Args: cobra.ExactArgs(1),
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
	_ = marketplaceInstallCmd.MarkFlagRequired("project")

	marketplaceCmd.AddCommand(marketplaceInstallCmd)
}

func runMarketplaceInstall(cmd *cobra.Command, args []string) error {
	contentID, err := parseContentID(args[0])
	if err != nil {
		return err
	}
	mprPath, _ := cmd.Flags().GetString("project")
	if _, err := os.Stat(mprPath); err != nil {
		return fmt.Errorf("project not found: %s", mprPath)
	}
	versionNumber, _ := cmd.Flags().GetString("version")
	allowFormatChange, _ := cmd.Flags().GetBool("allow-format-change")

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
		return installWidget(cmd.Context(), client, version, projDir, out)
	case "module":
		// Refuse a version the project's Mendix cannot import, before spending a
		// download and a reference build on it. Only modules go through
		// module-import, so only modules are gated here.
		if projectVer := mendixVersionOf(mprPath); projectVer != "" {
			if err := checkMendixCompatibility(version, verList.Items, projectVer, version.Name); err != nil {
				return err
			}
		}
		return installModule(cmd.Context(), client, version, mprPath, allowFormatChange, out)
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

// installWidget copies the widget .mpk into the project's widgets/ folder.
// An existing file with the same name is overwritten (the update path).
func installWidget(ctx context.Context, client *marketplace.Client, v *marketplace.Version, projDir string, out io.Writer) error {
	widgetsDir := filepath.Join(projDir, "widgets")
	if err := os.MkdirAll(widgetsDir, 0o755); err != nil {
		return fmt.Errorf("create widgets dir: %w", err)
	}
	dest, err := fetchMpkToFile(ctx, client, v, widgetsDir)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Installed widget %s into %s\n", v.VersionNumber, dest)
	fmt.Fprintln(out, "Run 'mxcli fix widgets -p <project.mpr>' (or reload in Studio Pro) to pick it up.")
	return nil
}

// installModule imports a module .mpk into the project, but only when the module
// is not already present. An existing module is reported, not modified.
func installModule(ctx context.Context, client *marketplace.Client, v *marketplace.Version, mprPath string, allowFormatChange bool, out io.Writer) error {
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
		// Postponed: do NOT auto-update modules — see the module-update memory.
		fmt.Fprintf(out, "Module %q is already installed (version %s).\n", moduleName, displayVer(installedVer))
		fmt.Fprintf(out, "Target version: %s.\n", v.VersionNumber)
		fmt.Fprintln(out, "In-place module updates are not applied automatically (they can discard local")
		fmt.Fprintln(out, "edits and change persistent-entity IDs, which loses data). Update via Studio Pro.")
		return nil
	}

	// Default path: copy the module in with mxcli's own writer, which preserves
	// the project's storage format and works for theme modules. --allow-format-change
	// selects the legacy `mx module-import`, which does neither.
	if !allowFormatChange {
		res, ierr := installByTransplant(ctx, mpkPath, mprPath, moduleName, mendixVer, v)
		if ierr != nil {
			return ierr
		}
		fmt.Fprintf(out, "Installed module %q version %s into %s\n",
			moduleName, v.VersionNumber, filepath.Base(mprPath))
		fmt.Fprintf(out, "  %d units copied, %d bundled file(s) installed.\n",
			res.UnitsCopied, len(res.FilesInstalled))
		reportSkippedFiles(out, res.FilesSkipped)
		reportSecurityReconcile(out, res)
		// A headless install leaves the model needing two repairs only Mendix's own
		// tools can make (CE0463, CE6087). 'mxcli fix' runs them without the v2 ->
		// v1 conversion the bare mx commands perform.
		fmt.Fprintln(out, "\n  Next, repair what a headless install leaves for Studio Pro to finish:")
		fmt.Fprintln(out, "      mxcli fix widgets -p <project.mpr>             # CE0463")
		fmt.Fprintln(out, "      mxcli fix design-properties -p <project.mpr>   # CE6087")
		fmt.Fprintln(out, "      mxcli docker check -p <project.mpr>")
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
	fmt.Fprintf(out, "Imported module %q version %s into %s\n", moduleName, v.VersionNumber, filepath.Base(mprPath))
	reportFormatChange(mprPath, out)
	return nil
}

// installByTransplant builds a reference project from the package and copies the
// module out of it, so the destination keeps its MPR format.
func installByTransplant(ctx context.Context, mpkPath, mprPath, moduleName, mendixVer string,
	v *marketplace.Version) (*mp.UpdateResult, error) {

	work, err := os.MkdirTemp("", "mxinstall")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)

	refDir := filepath.Join(work, "ref")
	if err := os.MkdirAll(refDir, 0o755); err != nil {
		return nil, err
	}
	refMpr, err := mp.PackageProject(ctx, mpkPath, mendixVer, refDir, newBackendFactory())
	if err != nil {
		return nil, fmt.Errorf("build a reference project from the package: %w", err)
	}
	return mp.PerformInstall(mprPath, refMpr, mpkPath, moduleName, v.VersionNumber, v.VersionID, newBackendFactory())
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

// moduleNameFromMpk reads package.xml from the .mpk and returns the module name
// (from <modelerProject><module> for modules, or <clientModule> for widgets).
func moduleNameFromMpk(mpkPath string) (string, error) {
	zr, err := zip.OpenReader(mpkPath)
	if err != nil {
		return "", err
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
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return "", err
		}
		var pkg mpkPackageXML
		if err := xml.Unmarshal(data, &pkg); err != nil {
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
	return "", fmt.Errorf("no package.xml in %s", filepath.Base(mpkPath))
}
