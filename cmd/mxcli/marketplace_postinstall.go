// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// marketplace_postinstall.go runs the repairs a headless install leaves for
// Mendix's own tools, as part of `marketplace install` (mendixlabs/mxcli#1527).
//
// A module or widget copied in without Studio Pro leaves stored widget instances
// older than the installed widget packages (CE0463), and a module's design
// properties may use names an older Atlas spelled differently (CE6087). Only
// `mx update-widgets` / `mx rename-design-properties` fix those, and both
// collapse MPR v2 to v1 when run bare. These are the same calls `mxcli fix
// widgets` / `mxcli fix design-properties` make: docker.RunToolPreservingFormat,
// which keeps the storage format.
//
// install used to print the two commands as a "Next" step; the field report was
// 41 units of CE0463 left behind. When mx cannot be resolved the commands are
// still printed — as a warning, never a failure, since the install itself
// succeeded.

// postInstallRepair is one Mendix repair tool and the mxcli command that runs it.
type postInstallRepair struct {
	subcommand string // mx subcommand
	what       string // what it updates, for output
	fixCmd     string // the manual equivalent
	clears     string // the error it clears
}

var (
	repairWidgets = postInstallRepair{"update-widgets", "widget definitions", "mxcli fix widgets", "CE0463"}
	repairDesign  = postInstallRepair{"rename-design-properties", "design properties", "mxcli fix design-properties", "CE6087"}

	// moduleRepairs run after a module install; widgetRepairs after a widget
	// package lands in widgets/ (a widget ships no design properties to rename).
	moduleRepairs = []postInstallRepair{repairWidgets, repairDesign}
	widgetRepairs = []postInstallRepair{repairWidgets}
)

// Seams for tests: resolving mx, running a tool, and reading the project's
// Mendix version.
var (
	postInstallResolveMx = func(mendixVer string) (string, error) {
		return docker.ResolveMxForVersion("", mendixVer)
	}
	postInstallRunTool       = docker.RunToolPreservingFormat
	postInstallMendixVersion = mendixVersionOf
)

// runPostInstallRepairs runs each repair on the project. A failure is reported
// with its manual command and the next repair still runs.
func runPostInstallRepairs(out, stderr io.Writer, mprPath, mendixVer string, repairs []postInstallRepair) {
	if mendixVer == "" {
		fmt.Fprintln(out, "\n  Warning: could not read the project's Mendix version, so the post-install repairs did not run.")
		printManualRepairs(out, mprPath, repairs)
		return
	}
	mxPath, err := postInstallResolveMx(mendixVer)
	if err != nil {
		fmt.Fprintf(out, "\n  Warning: no mx for Mendix %s (%v), so the post-install repairs did not run.\n", mendixVer, err)
		fmt.Fprintf(out, "  Run 'mxcli setup mxbuild -p %s', then:\n", mprPath)
		printManualRepairs(out, mprPath, repairs)
		return
	}

	fmt.Fprintln(out, "\n  Repairing what a headless install leaves for Studio Pro to finish:")
	var failed []postInstallRepair
	for _, r := range repairs {
		res, err := postInstallRunTool(mxPath, mprPath, r.subcommand, out, stderr)
		if err != nil {
			fmt.Fprintf(out, "  Warning: updating %s (%s) failed: %v\n", r.what, r.clears, err)
			failed = append(failed, r)
			continue
		}
		fmt.Fprintf(out, "    %s (%s): %s\n", r.what, r.clears, repairSummary(res))
	}
	if len(failed) > 0 {
		fmt.Fprintln(out, "  Retry the failed repair(s):")
		printManualRepairs(out, mprPath, failed)
	}
	fmt.Fprintf(out, "\n  Next: mxcli docker check -p %s\n", mprPath)
}

func repairSummary(res *docker.HarvestResult) string {
	if res == nil || !res.Harvested {
		return "updated (MPR v1, written natively)"
	}
	s := fmt.Sprintf("%d unit(s) changed, %d .mxunit file(s) before and after", res.UnitsWritten, res.StorageFiles)
	if !res.StillV2 || res.StorageFiles == 0 {
		s += " — WARNING: the project is no longer MPR v2; restore it from version control and report this"
	}
	return s
}

func printManualRepairs(out io.Writer, mprPath string, repairs []postInstallRepair) {
	for _, r := range repairs {
		fmt.Fprintf(out, "      %s -p %s   # %s\n", r.fixCmd, mprPath, r.clears)
	}
	fmt.Fprintf(out, "      mxcli docker check -p %s\n", mprPath)
}

// repairReference runs on a freshly built reference project the repairs a
// module install runs on the app, so `marketplace diff` compares like with
// like. Without them every snippet whose stored widgets the repair rewrites
// read as a local edit of an untouched module (ako/mxcli#2367). A failure is
// an error: a half-repaired reference reports false edits, and must not be
// cached.
func repairReference(mprPath, mendixVer string) error {
	mxPath, err := postInstallResolveMx(mendixVer)
	if err != nil {
		return fmt.Errorf("locate mx for Mendix %s to repair the reference: %w", mendixVer, err)
	}
	for _, r := range moduleRepairs {
		if _, err := postInstallRunTool(mxPath, mprPath, r.subcommand, io.Discard, io.Discard); err != nil {
			return fmt.Errorf("repair the reference project (%s, as install does on the app): %w", r.subcommand, err)
		}
	}
	return nil
}
