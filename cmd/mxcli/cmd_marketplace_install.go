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

	modelsdk "github.com/JordtenBulte-OLC/mxcli"
	"github.com/JordtenBulte-OLC/mxcli/cmd/mxcli/docker"
	"github.com/JordtenBulte-OLC/mxcli/internal/marketplace"
	"github.com/spf13/cobra"
)

var marketplaceInstallCmd = &cobra.Command{
	Use:   "install <content-id>",
	Short: "Download and install a marketplace item into a project",
	Long: `Download a marketplace content version and install it into a project.

Install is type-aware:
  - Widget      copied into the project's widgets/ folder (overwrites on update)
  - Module      imported via 'mx module-import' (new modules only)
  - other types downloaded to disk with import instructions

Updating a module that is already present is NOT done automatically: it could
discard local edits and, for modules with persistent entities, change entity
IDs (which loses data). Such updates are reported and left to Studio Pro.`,
	Example: `  mxcli marketplace install 20 -p app.mpr
  mxcli marketplace install 2888 --version 7.0.3 -p app.mpr`,
	Args: cobra.ExactArgs(1),
	RunE: runMarketplaceInstall,
}

func init() {
	marketplaceInstallCmd.Flags().String("profile", "default", "credential profile")
	marketplaceInstallCmd.Flags().StringP("project", "p", "", "path to the Mendix project (.mpr)")
	marketplaceInstallCmd.Flags().String("version", "", "version number to install (default: latest)")
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
		return installModule(cmd.Context(), client, version, mprPath, out)
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
	fmt.Fprintln(out, "Reload the project in Studio Pro (or run 'mx update-widgets') to pick it up.")
	return nil
}

// installModule imports a module .mpk into the project, but only when the module
// is not already present. An existing module is reported, not modified.
func installModule(ctx context.Context, client *marketplace.Client, v *marketplace.Version, mprPath string, out io.Writer) error {
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
	_ = reader.Close()

	if existing {
		// Postponed: do NOT auto-update modules — see the module-update memory.
		fmt.Fprintf(out, "Module %q is already installed (version %s).\n", moduleName, displayVer(installedVer))
		fmt.Fprintf(out, "Target version: %s.\n", v.VersionNumber)
		fmt.Fprintln(out, "In-place module updates are not applied automatically (they can discard local")
		fmt.Fprintln(out, "edits and change persistent-entity IDs, which loses data). Update via Studio Pro.")
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
	return nil
}

// findModule reports whether a marketplace-sourced module of the given name is
// present, and its installed AppStore version.
func findModule(reader *modelsdk.Reader, name string) (found bool, appStoreVersion string) {
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
