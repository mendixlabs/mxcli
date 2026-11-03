// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mendixlabs/mxcli/cmd/mxcli/marketplace"
	"github.com/mendixlabs/mxcli/internal/auth"
	mp "github.com/mendixlabs/mxcli/internal/marketplace"
	"github.com/spf13/cobra"
)

var marketplaceDiffCmd = &cobra.Command{
	Use:   "diff <content-id> -p <project.mpr>",
	Short: "Report what has been changed locally in an installed marketplace module",
	Long: `Compare an installed marketplace module against the published package it was
installed from, and report which elements have been modified locally.

This answers the question that decides whether a module can safely be updated:
has anyone edited it since it was installed? Studio Pro's Marketplace update
does not ask — it replaces the module and discards local edits silently.

How it works: the project records which version each marketplace module came
from. That version's package is downloaded and imported into a throwaway
reference project built at the project's own Mendix version, then every element
of the module is described on both sides and the descriptions are compared.

Comparison is on DESCRIBE output rather than raw storage, because an untouched
module differs from its own published package in thousands of BSON paths — the
installed copy carries subtrees the package does not.

An element that cannot be described is reported as unknown, never as unchanged.
"Nothing changed" and "we could not tell" are different answers, and only the
first of them makes an upgrade safe.

Requires the mxbuild toolchain for the project's Mendix version:
  mxcli setup mxbuild -p <project.mpr>`,
	Example: `  # What have I changed in this module since installing it?
  mxcli marketplace diff 23513 -p app.mpr

  # What would upgrading to 4.5.0 touch, and does it collide with my edits?
  mxcli marketplace diff 23513 -p app.mpr --to 4.5.0

  # Fail a build when a marketplace module has been edited
  mxcli marketplace diff 23513 -p app.mpr --json`,
	Args: cobra.ExactArgs(1),
	RunE: runMarketplaceDiff,
	// A failed install/update/diff is a runtime failure, not a misuse of the
	// command: printing the full flag list on top of the error buries it.
	// SilenceErrors too, because main() already prints what Execute returns —
	// without it every refusal is printed twice.
	SilenceUsage:  true,
	SilenceErrors: true,
}

func runMarketplaceDiff(cmd *cobra.Command, args []string) error {
	contentID, err := parseContentID(args[0])
	if err != nil {
		return err
	}
	mprPath, _ := cmd.Flags().GetString("project")
	if mprPath == "" {
		return fmt.Errorf("-p/--project is required: the installed module is what gets compared")
	}
	moduleName, _ := cmd.Flags().GetString("module")
	target, _ := cmd.Flags().GetString("to")
	asJSON, _ := cmd.Flags().GetBool("json")

	ctx := cmd.Context()
	client, err := newMarketplaceClient(ctx, cmd)
	if err != nil {
		return err
	}

	versions, err := client.Versions(ctx, contentID)
	if err != nil {
		return err
	}

	work, err := marketplace.NewWorkDir("mxd")
	if err != nil {
		return fmt.Errorf("create work directory: %w", err)
	}
	defer os.RemoveAll(work)

	// Which module in this project came from this content, and which published
	// version is it? The project records the marketplace version UUID per module,
	// which answers both exactly. --module skips the question.
	var baseVersionID string
	if moduleName == "" {
		moduleName, baseVersionID, err = marketplace.ModuleForVersionIDs(mprPath, versionIDs(versions.Items))
		if err != nil {
			return fmt.Errorf("%w\nhint: pass --module with the module's name in the project", err)
		}
	}

	installedVersion, mendixVersion, err := marketplace.InstalledModule(mprPath, moduleName)
	if err != nil {
		return err
	}

	base, err := pickVersion(versions.Items, baseVersionID, installedVersion)
	if err != nil {
		return err
	}
	baseRef, basePkgModule, _, err := referenceFor(ctx, client, base, mendixVersion, work, "b")
	if err != nil {
		return err
	}

	installed, err := marketplace.SnapshotModule(mprPath, moduleName, newBackendFactory())
	if err != nil {
		return fmt.Errorf("read %s from the project: %w", moduleName, err)
	}
	// The reference side is read under the name the *package* declares: a module
	// renamed between versions would otherwise read as empty and every element
	// would be reported as locally added.
	published, err := marketplace.SnapshotModule(baseRef, basePkgModule, newBackendFactory())
	if err != nil {
		return fmt.Errorf("read %s from the published package: %w", basePkgModule, err)
	}

	drift := marketplace.Compare(installed, published)
	result := marketplace.NewDiffResult(moduleName, installedVersion, mendixVersion, drift)

	if target != "" {
		targetVersion, err := pickVersion(versions.Items, "", target)
		if err != nil {
			return err
		}
		targetRef, targetPkgModule, _, err := referenceFor(ctx, client, targetVersion, mendixVersion, work, "t")
		if err != nil {
			return err
		}
		targetSnap, err := marketplace.SnapshotModule(targetRef, targetPkgModule, newBackendFactory())
		if err != nil {
			return fmt.Errorf("read %s from the target package: %w", targetPkgModule, err)
		}
		result.TargetVersion = target
		result.Upgrade = marketplace.UpgradeImpactOf(drift, marketplace.Compare(published, targetSnap))
	}

	if asJSON {
		return result.WriteJSON(cmd.OutOrStdout())
	}
	return result.WriteText(cmd.OutOrStdout())
}

// pickVersion selects the published version to compare against.
//
// The version ID is preferred when the project supplied one: it names the exact
// release, while a version *number* has to be matched as a string and is not
// unique across content. The number is the fallback for --to and for --module,
// where no ID is available.
func pickVersion(versions []mp.Version, versionID, versionNumber string) (*mp.Version, error) {
	if versionID != "" {
		for i := range versions {
			if versions[i].VersionID == versionID {
				return &versions[i], nil
			}
		}
		return nil, fmt.Errorf("the version this module was installed from (%s) is no longer published for this content",
			orNoVersion(versionNumber, versionID))
	}
	return resolveVersion(versions, versionNumber)
}

func orNoVersion(number, id string) string {
	if number != "" {
		return number
	}
	return id
}

// downloadVersion fetches one version's .mpk into the work directory.
func downloadVersion(ctx context.Context, client *mp.Client, v *mp.Version, work, slot string) (string, error) {
	if v.DownloadURL == "" {
		return "", fmt.Errorf("version %s exposes no download URL", v.VersionNumber)
	}

	mpkPath := filepath.Join(work, slot+".mpk")
	f, err := os.Create(mpkPath)
	if err != nil {
		return "", fmt.Errorf("create %s: %w", mpkPath, err)
	}
	_, derr := client.Download(ctx, v, f)
	cerr := f.Close()
	if derr != nil {
		return "", fmt.Errorf("download version %s: %w", v.VersionNumber, derr)
	}
	if cerr != nil {
		return "", cerr
	}
	return mpkPath, nil
}

// referenceFor downloads one version's package and builds a reference project
// from it. Each version gets its own subdirectory so --to can hold two. It also
// returns the module name the package declares, which is what the reference
// project's copy is called, and where the package itself is: a caller that
// needs the package derives no path of its own, which is how `marketplace
// update` came to look for target.mpk while the package was t.mpk and fail
// after dropping the module (ako/mxcli#1934).
func referenceFor(ctx context.Context, client *mp.Client, v *mp.Version,
	mendixVersion, work, slot string) (mprPath, pkgModule, mpkPath string, err error) {

	// The slot is one letter on purpose: mx writes the whole reference project
	// beneath refDir, and every character here is one less for the package's own
	// paths (see marketplace.NewWorkDir).
	refDir := filepath.Join(work, slot)
	if err := os.MkdirAll(refDir, 0o755); err != nil {
		return "", "", "", err
	}
	mpkPath = filepath.Join(work, slot+".mpk")

	// A reference is expensive (a download plus two ~10s mx invocations) and
	// immutable, so the same published version is never built twice: `diff`
	// followed by `update` needs the same base, and so does re-running either.
	if cached := marketplace.CachedReference(v.VersionID, mendixVersion, refDir, mpkPath); cached != "" {
		if pkgModule, err = marketplace.ModuleNameInPackage(mpkPath); err == nil {
			return cached, pkgModule, mpkPath, nil
		}
		// The package came back unreadable, so the entry is not trustworthy.
		// Fall through and rebuild rather than failing the command.
		if err := os.RemoveAll(refDir); err == nil {
			_ = os.MkdirAll(refDir, 0o755)
		}
	}

	if mpkPath, err = downloadVersion(ctx, client, v, work, slot); err != nil {
		return "", "", "", err
	}
	pkgModule, err = marketplace.ModuleNameInPackage(mpkPath)
	if err != nil {
		return "", "", "", err
	}
	mprPath, err = marketplace.PackageProject(ctx, mpkPath, mendixVersion, refDir, newBackendFactory())
	if err != nil {
		return "", "", "", err
	}
	marketplace.CacheReference(v.VersionID, mendixVersion, refDir, mpkPath)
	return mprPath, pkgModule, mpkPath, nil
}

// versionIDs projects the marketplace's version list to bare version UUIDs.
func versionIDs(versions []mp.Version) []string {
	out := make([]string, 0, len(versions))
	for _, v := range versions {
		out = append(out, v.VersionID)
	}
	return out
}

func init() {
	marketplaceDiffCmd.Flags().StringP("project", "p", "", "path to the Mendix project (.mpr)")
	marketplaceDiffCmd.Flags().String("module", "", "module name in the project, when it differs from the marketplace listing")
	marketplaceDiffCmd.Flags().String("to", "", "also report what upgrading to this version would touch")
	marketplaceDiffCmd.Flags().String("profile", auth.ProfileDefault, "credential profile")
	marketplaceDiffCmd.Flags().Bool("json", false, "emit JSON instead of text")

	marketplaceCmd.AddCommand(marketplaceDiffCmd)
}
