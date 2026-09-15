// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"os"

	modelsdk "github.com/mendixlabs/mxcli"
	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	"github.com/spf13/cobra"
)

// cmd_fix.go exposes Mendix's own model-fixing tools without their side effect.
//
// `mx rename-design-properties` and `mx update-widgets` each fix something only
// Mendix can fix, and each rewrites an MPR v2 project as v1 while doing it. The
// commands here run the tool, harvest its output, and put it back into the v2
// project through mxcli's writer — see docker.RunToolPreservingFormat.
//
// Both errors these clear are the normal aftermath of a headless module install,
// not defects: CE0463 because the project has not resynced widget definitions,
// CE6087 because a module's design properties were renamed in a newer Atlas.

var fixCmd = &cobra.Command{
	Use:   "fix",
	Short: "Apply Mendix's model-fixing tools while preserving the MPR v2 storage format",
	Long: `Run Mendix's own model-fixing tools and keep the project's storage format.

'mx update-widgets' and 'mx rename-design-properties' both do work that only
Mendix can do, and both rewrite an MPR v2 project into the single-file v1 format
as a side effect — measured on 11.12.1, rename-design-properties turned 1,866
.mxunit files into 0 and a 69 KB index into 39 MB.

These subcommands run the tool, read its result back out, restore the v2
storage, and write the changed units into it with mxcli's own writer. The fix
persists; the format survives. An MPR v1 project is passed straight through.`,
}

var fixDesignPropertiesCmd = &cobra.Command{
	Use:   "design-properties",
	Short: "Update renamed design properties (clears CE6087) without collapsing MPR v2",
	Long: `Apply 'mx rename-design-properties' to the project, preserving MPR v2.

CE6087 "Design properties have been renamed in your theme and need to be
updated" appears when a module references design properties an older Atlas
spelled differently. It is the normal aftermath of installing a module that
ships its own design properties, and Mendix's rename tool is the only thing that
fixes it.

Unlike the widget resync, this fix has to persist, so it cannot be run under a
snapshot-and-restore: the restore would undo the renames. The renamed units are
read back out of the converted file and written into the restored v2 project
instead.`,
	Example: `  mxcli fix design-properties -p app.mpr`,
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runFixTool(cmd, "rename-design-properties", "design properties")
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

var fixWidgetsCmd = &cobra.Command{
	Use:   "widgets",
	Short: "Resync widget definitions (clears CE0463) without collapsing MPR v2",
	Long: `Apply 'mx update-widgets' to the project, preserving MPR v2.

CE0463 "The definition of this widget has changed" is what a project reports
when its stored widget instances are older than the widget packages installed
beside them — the normal state after any headless module or widget install.

'mxcli docker check' already runs this step, but under a snapshot that is
restored afterwards, so the check passes and the stored model stays stale. This
persists the resync instead, which is what Studio Pro's "Update all widgets"
does. It is also more complete than 'mxcli widget sync', which reconciles widget
schemas itself and clears only part of the same errors.`,
	Example: `  mxcli fix widgets -p app.mpr`,
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runFixTool(cmd, "update-widgets", "widget definitions")
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	for _, c := range []*cobra.Command{fixDesignPropertiesCmd, fixWidgetsCmd} {
		c.Flags().StringP("project", "p", "", "path to the Mendix project (.mpr)")
		_ = c.MarkFlagRequired("project")
		c.Flags().String("mx", "", "path to the mx binary (default: resolved from the project's Mendix version)")
		fixCmd.AddCommand(c)
	}
	rootCmd.AddCommand(fixCmd)
}

func runFixTool(cmd *cobra.Command, subcommand, what string) error {
	mprPath, _ := cmd.Flags().GetString("project")
	if _, err := os.Stat(mprPath); err != nil {
		return fmt.Errorf("project not found: %s", mprPath)
	}
	mxOverride, _ := cmd.Flags().GetString("mx")
	out := cmd.OutOrStdout()

	reader, err := modelsdk.Open(mprPath)
	if err != nil {
		return fmt.Errorf("open project: %w", err)
	}
	mendixVer, _ := reader.GetMendixVersion()
	_ = reader.Disconnect()

	mxPath, err := docker.ResolveMxForVersion(mxOverride, mendixVer)
	if err != nil {
		return fmt.Errorf("locate mx for Mendix %s: %w\nhint: run 'mxcli setup mxbuild -p %s'", mendixVer, err, mprPath)
	}

	fmt.Fprintf(out, "Updating %s in %s (Mendix %s)...\n", what, mprPath, mendixVer)
	res, err := docker.RunToolPreservingFormat(mxPath, mprPath, subcommand, out, cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	reportFix(out, what, res)
	return nil
}

func reportFix(out io.Writer, what string, res *docker.HarvestResult) {
	if !res.Harvested {
		fmt.Fprintf(out, "\nUpdated %s. The project is MPR v1, which this tool writes natively.\n", what)
		return
	}

	fmt.Fprintf(out, "\nUpdated %s: %d unit(s) changed.\n", what, res.UnitsWritten)
	if res.UnitsWritten == 0 {
		fmt.Fprintln(out, "  Nothing to update — the model was already in sync.")
	}
	// Print the storage numbers unconditionally. The failure this mechanism
	// exists to prevent shows up here as a zero, and a success message that does
	// not carry its own evidence is how the collapse shipped in the first place.
	fmt.Fprintf(out, "  Storage: %d .mxunit file(s), unchanged from %d before (MPR v2 preserved).\n",
		res.StorageFiles, res.StorageFilesBefore)

	if !res.StillV2 || res.StorageFiles == 0 {
		fmt.Fprintln(out, "\n  WARNING: the project is no longer in the MPR v2 layout. Restore it from")
		fmt.Fprintln(out, "  version control and report this — the storage format should have survived.")
	}
	for _, a := range res.Added {
		fmt.Fprintf(out, "  Note: the tool added a unit this did not carry over: %s\n", a)
	}
	for _, r := range res.Removed {
		fmt.Fprintf(out, "  Note: the tool removed a unit this did not remove: %s\n", r)
	}
	fmt.Fprintln(out, "\n  Next: 'mxcli docker check -p <project.mpr>'")
}
