// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/spf13/cobra"
)

// cmd_widget_sync.go is the CLI for reconciling stored widget instances against the
// widget packages installed in the project — the mxcli equivalent of Studio Pro's
// "Update all widgets" and `mx update-widgets`, without the latter's MPR v2 data loss.
//
// PARTIAL. On the reference fixture (Data Widgets 3.4 -> 3.11.3) this clears 7 of 40
// CE0463 while `mx update-widgets` clears all 40. What it does is faithful — the
// widget Type it writes is byte-identical to Mendix's own output — but a value-level
// difference not yet identified keeps DataGrid2 and Gallery instances erroring. The
// help text says so rather than implying the command finishes the job.
//
// Both engines are supported: the modelsdk and legacy backends produce structurally
// identical output (verified over ~31k paths on four widgets).

var widgetSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Reconcile stored widget instances against the installed widget packages",
	Long: `Compare every stored pluggable-widget instance against the .mpk currently
installed in the project, and reconcile the differences.

PARTIAL — this does not yet fully replace "Update all widgets". On the reference
fixture it clears 7 of 40 CE0463 errors; Mendix's own 'mx update-widgets' clears
all 40 but destroys the mprcontents/ folder on MPR v2 projects, which this does
not. Preview with --dry-run and verify with 'mx check' before relying on it.

Runs on either engine; both produce identical output.

Every unit is checked for duplicate GUIDs before it is written, and the run aborts
rather than persisting one: Mendix accepts a duplicate on load and on 'mx check',
then refuses to save the project.

mxcli writes a widget instance correctly for the package installed at authoring
time. When that package is later upgraded, the stored instances go stale and
Mendix reports CE0463 "the definition of this widget has changed" on each one.
This command finds those instances.

It reconciles SCHEMA — properties the package added, dropped, or redefined. It
never changes a property value you set, and it never touches a widget whose .mpk
is not installed.

Examples:
  mxcli widget sync -p app.mpr --dry-run
  mxcli widget sync -p app.mpr --dry-run --widget com.mendix.widget.web.datagrid.Datagrid
  mxcli widget sync -p app.mpr --dry-run --page MyModule.Overview

--page takes the qualified name; a bare name is accepted when only one module
has a page or snippet of that name.`,
	RunE: runWidgetSync,
}

func init() {
	widgetSyncCmd.Flags().StringP("project", "p", "", "Path to .mpr project file")
	widgetSyncCmd.Flags().Bool("dry-run", false, "Report what would change without writing")
	widgetSyncCmd.Flags().String("widget", "", "Only this widget type (full widget ID)")
	widgetSyncCmd.Flags().String("page", "", "Only this page or snippet (qualified name)")
	widgetSyncCmd.Flags().Bool("add-missing", false, "EXPERIMENTAL: also insert properties the package declares but the widget lacks (does not yet clear CE0463)")
	widgetSyncCmd.MarkFlagRequired("project")
	widgetCmd.AddCommand(widgetSyncCmd)
}

func runWidgetSync(cmd *cobra.Command, args []string) error {
	projectPath, _ := cmd.Flags().GetString("project")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	widgetID, _ := cmd.Flags().GetString("widget")
	page, _ := cmd.Flags().GetString("page")
	// Past flag parsing, an error is about the project, not the invocation:
	// the usage text would bury it.
	cmd.SilenceUsage = true

	exec, logger := newLoggedExecutor("subcommand")
	defer logger.Close()
	defer exec.Close()

	prog, errs := visitor.Build(fmt.Sprintf("CONNECT LOCAL '%s';", visitor.QuoteString(projectPath)))
	if len(errs) > 0 {
		return fmt.Errorf("connect: %v", errs[0])
	}
	exec.SetQuiet(true)
	if err := exec.ExecuteProgram(prog); err != nil {
		return fmt.Errorf("connect to %s: %w", projectPath, err)
	}

	addMissing, _ := cmd.Flags().GetBool("add-missing")
	opts := executor.SyncOptions{WidgetID: widgetID, Container: page, AddMissing: addMissing}

	if dryRun {
		plan, err := executor.PlanWidgetSync(exec.Backend(), projectPath, opts)
		if err != nil {
			return err
		}
		renderSyncPlan(os.Stdout, plan, true)
		return nil
	}

	res, plan, err := executor.ApplyWidgetSync(exec.Backend(), projectPath, opts)
	if err != nil {
		return err
	}
	renderSyncPlan(os.Stdout, plan, false)
	fmt.Printf("\nApplied: %d property change(s) on %d widget(s) in %d unit(s).\n",
		res.PropertiesFixed, res.WidgetsChanged, res.UnitsChanged)
	if n := len(res.Skipped); n > 0 {
		fmt.Printf("Not applied: %d add(s) — see `mxcli widget sync --help`.\n", n)
	}
	return nil
}

// renderSyncPlan prints the plan. Every property is named rather than counted:
// removing a property discards its stored value, and that is exactly what a user
// needs to see before it happens. dryRun says whether the plan is only a
// preview: an applying run printed "nothing was written" right before
// reporting what it wrote (ako/mxcli#1997).
func renderSyncPlan(out io.Writer, plan *executor.SyncPlan, dryRun bool) {

	if len(plan.Unresolved) > 0 {
		fmt.Fprintln(out, "Widgets used in the model with no installed .mpk (skipped, never modified):")
		sort.Strings(plan.Unresolved)
		for _, id := range plan.Unresolved {
			fmt.Fprintf(out, "  %s\n", id)
		}
		fmt.Fprintln(out)
	}

	if plan.Empty() {
		// Say what was compared, not what was concluded. This message used to
		// read "Every stored widget instance already matches its installed
		// package", which a reporting project got on a widget `mx check` was
		// reporting CE0463 on at that moment (mxcli-ledger §142) — and the two
		// were both right, about different things. The comparison is over the
		// stored SCHEMA (each property's declared type attributes); the value in
		// the widget's Object is not looked at, and the CE0463 there was a value.
		fmt.Fprintln(out, "No schema drift: every stored widget's property declarations match its installed package.")
		fmt.Fprintln(out, "  This compares the stored property SCHEMA, not the values in each widget — a widget can")
		fmt.Fprintln(out, "  still fail CE0463 on a value the package would not accept. Run 'mxcli fix widgets' for")
		fmt.Fprintln(out, "  the full update.")
		return
	}

	container := ""
	for _, w := range plan.Widgets {
		if w.Container != container {
			container = w.Container
			fmt.Fprintf(out, "\n%s\n", container)
		}
		fmt.Fprintf(out, "  %s  (%s %s)\n", w.Widget, shortName(w.WidgetID), w.PackageVer)
		for _, c := range w.Changes {
			fmt.Fprintf(out, "    %s %-24s %s\n", changeMarker(c.Kind), c.Key, c.Detail)
		}
	}

	fmt.Fprintf(out, "\n%d widget instance(s), %d property change(s) across %d container(s).\n",
		len(plan.Widgets), plan.TotalChanges(), countContainers(plan))
	if dryRun {
		fmt.Fprintln(out, "Read-only: nothing was written.")
	}
}

func changeMarker(k executor.SyncChangeKind) string {
	switch k {
	case executor.SyncRemove:
		return "-"
	case executor.SyncAdd:
		return "+"
	default:
		return "~"
	}
}

func countContainers(plan *executor.SyncPlan) int {
	seen := map[string]bool{}
	for _, w := range plan.Widgets {
		seen[w.Container] = true
	}
	return len(seen)
}

func shortName(widgetID string) string {
	for i := len(widgetID) - 1; i >= 0; i-- {
		if widgetID[i] == '.' {
			return widgetID[i+1:]
		}
	}
	return widgetID
}
