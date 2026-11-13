// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mendixlabs/mxcli/cmd/mxcli/marketplace"
	"github.com/mendixlabs/mxcli/internal/auth"
	"github.com/spf13/cobra"
)

var marketplaceUpdateCmd = &cobra.Command{
	Use:   "update <content-id> -p <project.mpr> --to <version>",
	Short: "Replace an installed marketplace module with a newer version",
	Long: `Replace an installed marketplace module with another published version,
preserving the two things a plain replace destroys.

The first is element identity. The runtime keys entities and their attributes on
the model's GUID, so a module whose documents are replaced without carrying the
old GUIDs is a different module as far as the database is concerned, and its
tables are dropped on the next deploy. Studio Pro's own update transplants them;
so does this.

The second is access. A user role's grant of a module role lives in the project's
security document rather than in the module, so removing the module takes the
grants with it and putting the module back does not return them.

Local edits are NOT preserved, and by default the update refuses when it finds
any. Use --save-edits to write them out as re-executable MDL first, and --force
to proceed. Studio Pro discards them without asking; this at least tells you what
they were.

Associations that OTHER modules own and that point into this module are removed
with it, because they reach entities the update deletes. They count as local work
here too: the update refuses over them, and --save-edits writes each one as
association-<Module>_<Name>.mdl to replay afterwards.

This does not roll back. Work on a copy or have the project in version control:
if a step fails partway, the module has already been removed.`,
	Example: `  # What would this update touch, and have I edited any of it?
  mxcli marketplace diff 23513 -p app.mpr --to 4.5.0

  # Park local edits, then update over them
  mxcli marketplace update 23513 -p app.mpr --to 4.5.0 --save-edits ./local-edits
  mxcli marketplace update 23513 -p app.mpr --to 4.5.0 --force
  mxcli exec ./local-edits/entity-Account.mdl -p app.mpr`,
	Args: cobra.ExactArgs(1),
	RunE: runMarketplaceUpdate,
	// A failed install/update/diff is a runtime failure, not a misuse of the
	// command: printing the full flag list on top of the error buries it.
	// SilenceErrors too, because main() already prints what Execute returns —
	// without it every refusal is printed twice.
	SilenceUsage:  true,
	SilenceErrors: true,
}

func runMarketplaceUpdate(cmd *cobra.Command, args []string) error {
	contentID, err := parseContentID(args[0])
	if err != nil {
		return err
	}
	mprPath, _ := cmd.Flags().GetString("project")
	if mprPath == "" {
		return fmt.Errorf("-p/--project is required")
	}
	target, _ := cmd.Flags().GetString("to")
	if target == "" {
		return fmt.Errorf("--to <version> is required: an update needs a version to move to")
	}
	moduleName, _ := cmd.Flags().GetString("module")
	saveEdits, _ := cmd.Flags().GetString("save-edits")
	force, _ := cmd.Flags().GetBool("force")
	noBaseline, _ := cmd.Flags().GetBool("no-baseline")

	ctx := cmd.Context()
	out := cmd.OutOrStdout()
	client, err := newMarketplaceClient(ctx, cmd)
	if err != nil {
		return err
	}
	versions, err := client.Versions(ctx, contentID)
	if err != nil {
		return err
	}

	work, err := marketplace.NewWorkDir("mxu")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	var installedVersionID string
	if moduleName == "" {
		moduleName, installedVersionID, err = marketplace.ModuleForVersionIDs(mprPath, versionIDs(versions.Items))
		if err != nil {
			return fmt.Errorf("%w\nhint: pass --module with the module's name in the project", err)
		}
	}
	installedVersion, mendixVersion, err := marketplace.InstalledModule(mprPath, moduleName)
	if err != nil {
		return err
	}
	if installedVersion == target {
		fmt.Fprintf(out, "%s is already at %s; nothing to do.\n", moduleName, target)
		return nil
	}

	// Refuse a target the project's Mendix cannot import, before building two
	// reference projects to discover it from `mx module-import`'s exit code.
	if tv, verr := pickVersion(versions.Items, "", target); verr == nil {
		if err := checkMendixCompatibility(tv, versions.Items, mendixVersion, moduleName); err != nil {
			return err
		}
	}

	// Has anyone edited this module? Answering needs the version it was installed
	// from, built as a reference exactly as `marketplace diff` does — which is
	// impossible when that version has been unpublished. A blank 11.13 app ships
	// NanoflowCommons 6.0.0 and the 6.x line now starts at 6.1.1, so the module
	// that most needs updating is the one whose baseline cannot be built.
	var drift *marketplace.Report
	if noBaseline {
		fmt.Fprintf(out, "\n--no-baseline: skipping the local-edit check for %s.\n", moduleName)
		fmt.Fprintln(out, "  Any local edits to this module will be discarded without being named.")
	} else {
		base, berr := pickVersion(versions.Items, installedVersionID, installedVersion)
		if berr != nil {
			return fmt.Errorf("%w\n"+
				"  The installed version is the baseline for \"has anyone edited this?\", so it has to be\n"+
				"  downloadable. It is not, and --force does not help: there is nothing to compare against.\n"+
				"  hint: re-run with --no-baseline to update without that check (local edits are lost silently)", berr)
		}
		baseRef, basePkgModule, _, rerr := referenceFor(ctx, client, base, mendixVersion, work, "b")
		if rerr != nil {
			return rerr
		}
		installed, ierr := marketplace.SnapshotModule(mprPath, moduleName, newBackendFactory())
		if ierr != nil {
			return fmt.Errorf("read %s from the project: %w", moduleName, ierr)
		}
		published, perr := marketplace.SnapshotModule(baseRef, basePkgModule, newBackendFactory())
		if perr != nil {
			return fmt.Errorf("read %s from its published package: %w", basePkgModule, perr)
		}
		drift = marketplace.Compare(installed, published)

		if saveEdits != "" {
			written, unsaved, serr := marketplace.SaveEdits(saveEdits, drift)
			if serr != nil {
				return serr
			}
			reportSavedEdits(out, saveEdits, written, unsaved)
		}

		if gerr := gateOnLocalEdits(out, drift, force, saveEdits); gerr != nil {
			return gerr
		}
	}

	// Build the version being moved to, and replace.
	targetVersion, err := pickVersion(versions.Items, "", target)
	if err != nil {
		return err
	}
	// The bundled files and widgets are taken from the package rather than from
	// the reference project, whose widgets/ also holds the blank template's. Its
	// path is the one referenceFor downloaded to: this used to be spelled out
	// here as target.mpk while the download was t.mpk, so every update dropped
	// the module and then failed to open the package (ako/mxcli#1934).
	targetRef, _, targetMpk, err := referenceFor(ctx, client, targetVersion, mendixVersion, work, "t")
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "\nUpdating %s %s → %s...\n", moduleName, installedVersion, target)
	res, err := marketplace.PerformUpdate(mprPath, targetRef, targetMpk, moduleName, installedVersion, target,
		targetVersion.VersionID, newBackendFactory())
	if err != nil {
		if errors.Is(err, marketplace.ErrUpdateNotStarted) {
			return err
		}
		return fmt.Errorf("%w\n\nThe project may be mid-update — %s could be missing. Restore from version control",
			err, moduleName)
	}
	if drift != nil {
		res.DependentsRemoved = drift.DependentNames()
	}
	reportUpdate(out, res)
	// A new version can declare new managed Java dependencies (ako/mxcli#2028).
	syncInstalledJarDependencies(out, mprPath)
	return nil
}

// gateOnLocalEdits refuses an update that would destroy local work, unless the
// user has said to proceed. The refusal names the elements rather than just
// their count: "3 elements were modified" is not enough to decide with.
func gateOnLocalEdits(out io.Writer, drift *marketplace.Report, force bool, saveEdits string) error {
	result := marketplace.NewDiffResult("", "", "", drift)
	edited := append(append([]string{}, result.Modified...), result.OnlyInstalled...)
	// Associations other modules own that reach into this one. Not edits to the
	// module, but the update deletes them just the same (mendixlabs/mxcli#1502).
	edited = append(edited, result.Dependents...)

	if len(edited) == 0 {
		if !result.Verified {
			// Unknown elements are not proof of safety. Say so, and let --force
			// carry the decision, rather than reporting a clean bill of health.
			if !force {
				return fmt.Errorf("%d element(s) could not be read, so it cannot be shown that the module is unedited.\n"+
					"Re-run with --force to update anyway", len(result.Unknown))
			}
			fmt.Fprintf(out, "Note: %d element(s) could not be read; proceeding under --force.\n", len(result.Unknown))
		}
		return nil
	}

	if !force {
		msg := fmt.Sprintf("refusing to update: %d element(s) have been changed locally and the update would discard them:\n", len(edited))
		if len(result.Dependents) > 0 {
			msg = fmt.Sprintf("refusing to update: the update would discard %d local element(s) "+
				"(%d association(s) in other modules reach into %s and are removed with it):\n",
				len(edited), len(result.Dependents), drift.Module)
		}
		for _, e := range edited {
			msg += "    " + e + "\n"
		}
		if saveEdits == "" {
			msg += "\n  - Save them first:  --save-edits <dir>\n  - Then update:      --force"
		} else {
			msg += "\n  Saved above. Re-run with --force to proceed."
		}
		return fmt.Errorf("%s", msg)
	}

	fmt.Fprintf(out, "Proceeding under --force; %d locally changed element(s) will be replaced:\n", len(edited))
	for _, e := range edited {
		fmt.Fprintf(out, "    %s\n", e)
	}
	return nil
}

func reportSavedEdits(out io.Writer, dir string, written, unsaved []string) {
	if len(written) > 0 {
		fmt.Fprintf(out, "Saved %d locally changed element(s) to %s:\n", len(written), dir)
		for _, f := range written {
			fmt.Fprintf(out, "    %s\n", f)
		}
		fmt.Fprintln(out, "  Replay after the update with 'mxcli exec <file> -p <project.mpr>'.")
		fmt.Fprintln(out, "  These are resulting states, not diffs: an edit that REMOVED something")
		fmt.Fprintln(out, "  will not be restored by replaying them.")
	}
	if len(unsaved) > 0 {
		fmt.Fprintf(out, "\n  Could not be saved (%d) — these edits are not recoverable this way:\n", len(unsaved))
		for _, u := range unsaved {
			fmt.Fprintf(out, "    %s\n", u)
		}
	}
}

// reportSecurityReconcile says what the transplant's access rules needed.
//
// Both halves are worth printing. A count means the package shipped rules that
// did not cover every member of their entities — the model Mendix rejects with
// CE0066 — and that mxcli has just repaired it, which is a change to the module
// the operator will see in `diff-local` and should not have to explain to
// themselves. A failure means CE0066 is still there, so it names the one command
// that clears it rather than leaving the error to be discovered by `mx check`
// (mendixlabs/mxcli#1085).
func reportSecurityReconcile(out io.Writer, r *marketplace.UpdateResult) {
	if r.SecurityErr != nil {
		fmt.Fprintf(out, "\n  Entity access rules could not be reconciled: %v\n", r.SecurityErr)
		fmt.Fprintln(out, "  Mendix reports this as CE0066 \"Entity access is out of date\". Repair it with:")
		fmt.Fprintf(out, "      mxcli -p <project.mpr> -c \"update security %s\"\n", r.Module)
		return
	}
	if r.RulesReconciled > 0 {
		fmt.Fprintf(out, "  %d entity access rule(s) reconciled — the package's rules did not cover every\n",
			r.RulesReconciled)
		fmt.Fprintln(out, "  member of their entities, which Mendix reports as CE0066.")
	}
}

func reportUpdate(out io.Writer, r *marketplace.UpdateResult) {
	fmt.Fprintf(out, "\n%s updated %s → %s\n", r.Module, r.FromVersion, r.ToVersion)
	fmt.Fprintf(out, "  %d units copied, %d element identities preserved, %d role grant(s) restored.\n",
		r.UnitsCopied, r.IdentitiesKept, r.GrantsRestored)
	if len(r.FilesInstalled) > 0 {
		fmt.Fprintf(out, "  %d bundled file(s) replaced (widgets, themesource, ...).\n", len(r.FilesInstalled))
	}
	reportAddOnModules(out, r.FilesInstalled)
	reportSkippedFiles(out, r.FilesSkipped)

	if len(r.IdentitiesLost) > 0 {
		fmt.Fprintf(out, "\n  Removed in %s (%d) — their database columns or tables will go on the next deploy:\n",
			r.ToVersion, len(r.IdentitiesLost))
		for _, e := range r.IdentitiesLost {
			fmt.Fprintf(out, "    %s\n", e)
		}
	}
	if len(r.GrantsDropped) > 0 {
		fmt.Fprintf(out, "\n  Role grants that could not be restored (%d) — those users lose that access:\n",
			len(r.GrantsDropped))
		for _, g := range r.GrantsDropped {
			fmt.Fprintf(out, "    %s\n", g)
		}
	}
	if len(r.DependentsRemoved) > 0 {
		fmt.Fprintf(out, "\n  Removed from other modules (%d) — they reached into %s; replay them from --save-edits:\n",
			len(r.DependentsRemoved), r.Module)
		for _, d := range r.DependentsRemoved {
			fmt.Fprintf(out, "    %s\n", d)
		}
	}
	reportSecurityReconcile(out, r)
	// A newer module's pages reference widget definitions the project has not
	// resynced, so `mx check` reports CE0463 until it is told to. Measured on
	// Administration 4.3.2 → 4.5.0: 11 CE0463 errors, and 0 after the resync.
	// Saying so here is the difference between a two-command fix and a day in
	// diagnose-ce0463.md, which is where that error normally leads.
	//
	// The resync is named as `mxcli fix widgets`, not as bare `mx update-widgets`:
	// the latter rewrites an MPR v2 project as v1 (measured on 11.12.1 — 200
	// .mxunit files to 0, a 69 KB index to 14 MB), while fix widgets writes the
	// result back as v2 (#808). docker check then checks the project as stored.
	fmt.Fprintln(out, "\n  Next, repair what a headless update leaves behind (expected, not a fault):")
	fmt.Fprintln(out, "      mxcli fix widgets -p <project.mpr>             # CE0463")
	fmt.Fprintln(out, "      mxcli fix design-properties -p <project.mpr>   # CE6087")
	fmt.Fprintln(out, "\n  Then validate and review:")
	fmt.Fprintln(out, "      mxcli docker check -p <project.mpr>")
	fmt.Fprintln(out, "      mxcli diff-local -p <project.mpr>")
}

func init() {
	marketplaceUpdateCmd.Flags().StringP("project", "p", "", "path to the Mendix project (.mpr)")
	marketplaceUpdateCmd.Flags().String("to", "", "version to update to (required)")
	marketplaceUpdateCmd.Flags().String("module", "", "module name in the project, when it cannot be identified automatically")
	marketplaceUpdateCmd.Flags().String("save-edits", "", "write locally changed elements to this directory as re-executable MDL")
	marketplaceUpdateCmd.Flags().Bool("force", false, "update even though local edits will be discarded")
	marketplaceUpdateCmd.Flags().Bool("no-baseline", false,
		"update without checking for local edits, when the installed version is no longer published")
	marketplaceUpdateCmd.Flags().String("profile", auth.ProfileDefault, "credential profile")

	marketplaceCmd.AddCommand(marketplaceUpdateCmd)
}

// reportAddOnModules names the add-on module packages (modules/<Name>.mxmodule)
// among the files an install or update wrote. They have no unit in the .mpr, so
// `show modules` lists them only through the file, and the file is what Studio
// Pro loads the add-on's extension from (ako/mxcli#2205). An add-on the previous
// version shipped and this one does not is left in place, like every other
// bundled file.
func reportAddOnModules(out io.Writer, written []string) {
	for _, p := range marketplace.AddOnModulePackages(written) {
		fmt.Fprintf(out, "  Add-on module package written: %s\n", filepath.ToSlash(p))
	}
}

// reportSkippedFiles names the bundled files that were deliberately not written.
//
// A module package pins its own copy of every widget its pages use, and
// different modules pin different versions of the same widget — so installing
// modules in one order and then another used to roll widgets backwards with no
// sign that anything had happened (an older widget is not a check error). These
// lines are what makes that visible.
//
// The package's own metadata (License.txt, manifest.json, ...) is left out of
// every install, so it gets one line rather than one per file.
func reportSkippedFiles(out io.Writer, skipped []marketplace.SkippedFile) {
	if len(skipped) == 0 {
		return
	}
	var kept, dupes []marketplace.SkippedFile
	var metadata []string
	for _, s := range skipped {
		switch {
		case s.Kept != "":
			kept = append(kept, s)
		case s.Reason == marketplace.ReasonNotAProjectFile:
			metadata = append(metadata, s.Path)
		default:
			dupes = append(dupes, s)
		}
	}
	if len(metadata) > 0 {
		fmt.Fprintf(out, "  Left out %d package file(s) package.xml does not list as project files: %s.\n",
			len(metadata), strings.Join(metadata, ", "))
	}
	if len(kept) > 0 {
		fmt.Fprintf(out, "\n  Kept %d newer widget(s) the package would have rolled back:\n", len(kept))
		for _, s := range kept {
			fmt.Fprintf(out, "    %s — kept %s, package ships %s\n", s.Path, s.Kept, s.Offered)
		}
	}
	for _, s := range dupes {
		fmt.Fprintf(out, "  Skipped %s (%s).\n", s.Path, s.Reason)
	}
}
