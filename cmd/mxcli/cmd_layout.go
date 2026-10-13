// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/dmlayout"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/spf13/cobra"
)

// cmd_layout.go arranges a module's domain model from its association graph.
//
// Positions are presentation, and MDL treats them that way: `@Position` exists
// but almost no script writes one, so entities take the default slot. The
// default is deliberately dumb — it cannot be anything else, because the first
// entity of a script is placed before the last one is known. Laying the model
// out properly needs the whole graph, which only exists once the script has run,
// so it is a separate operation rather than a side effect of CREATE.
//
// It is opt-in for a second reason: a domain model somebody arranged by hand in
// Studio Pro is not improved by being rearranged. Nothing here runs unless
// asked, --dry-run shows the moves first, and Marketplace modules are skipped
// outright — mxcli does not rearrange a module the next update replaces.

var (
	layoutModules            []string
	layoutDryRun             bool
	layoutIncludeMarketplace bool
	layoutOnlyOverlapping    bool
)

var layoutCmd = &cobra.Command{
	Use:   "layout",
	Short: "Arrange a module's domain model from its association graph",
	Long: `Arrange the entities of a domain model so related ones sit together.

Entities are layered on the association graph: an entity that references nothing
else in the module is a lookup and goes on the left, and everything else sits
one column past the furthest thing it references. Association lines then mostly
run one way instead of crossing the diagram. Entities with no association at all
— non-persistent helpers, mostly — go in a band underneath rather than being
mixed in with the lookups.

Positions are a function of the model alone, so running this twice in a row
changes nothing the second time and re-running after adding an entity moves only
what the new relationships require.

This REPLACES the positions of every entity in the modules it touches, including
any you arranged by hand. Use --dry-run to see the moves first. Marketplace
modules and System are never touched.

With --only-overlapping nothing is re-arranged. The command finds the entities
whose boxes collide (a box is estimated from the entity's name and attribute
count, as in the full layout; a domain-model annotation counts as an obstacle),
keeps the one that comes first in the model, and moves each other colliding
entity to the nearest free spot. Every entity that collides with nothing keeps
its position exactly, so a diagram arranged by hand is repaired, not replaced.
Running it again finds nothing to move. This is the fix for the default grid,
where an entity with many attributes is taller than a grid row and runs into
the one below.

To re-arrange microflows and nanoflows, use 'mxcli layout flows'.`,
	Example: `  mxcli layout -p app.mpr
  mxcli layout -p app.mpr --module CapTrack
  mxcli layout -p app.mpr --dry-run
  mxcli layout -p app.mpr --module Sales --only-overlapping --dry-run`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runLayout(cmd)
	},
}

func init() {
	layoutCmd.Flags().StringSliceVar(&layoutModules, "module", nil,
		"module to lay out (repeatable; default: every module the project owns)")
	layoutCmd.Flags().BoolVar(&layoutDryRun, "dry-run", false,
		"report the moves without writing")
	layoutCmd.Flags().BoolVar(&layoutIncludeMarketplace, "include-marketplace", false,
		"also lay out Marketplace modules (a module update replaces them, so this is normally pointless)")
	layoutCmd.Flags().BoolVar(&layoutOnlyOverlapping, "only-overlapping", false,
		"move only entities whose boxes collide with another entity or annotation (to the nearest free spot); all other positions are kept")
	rootCmd.AddCommand(layoutCmd)
}

func runLayout(cmd *cobra.Command) error {
	projectPath, _ := cmd.Flags().GetString("project")
	if projectPath == "" {
		return fmt.Errorf("no project given: pass -p <project.mpr>")
	}
	if _, err := os.Stat(projectPath); err != nil {
		return fmt.Errorf("project not found: %s", projectPath)
	}

	b := newBackendFactory()()
	if err := b.Connect(projectPath); err != nil {
		return err
	}
	defer func() { _ = b.Disconnect() }()

	modules, err := b.ListModules()
	if err != nil {
		return err
	}

	// Keyed lower-cased because Mendix resolves module names case-insensitively,
	// but the value keeps the spelling the author used so a "not found" quotes
	// their typo back rather than a normalised version of it.
	wanted := map[string]string{}
	for _, m := range layoutModules {
		if t := strings.TrimSpace(m); t != "" {
			wanted[strings.ToLower(t)] = t
		}
	}

	targets, err := layoutTargets(modules, wanted)
	if err != nil {
		return err
	}

	movedTotal, unchangedTotal := 0, 0
	for _, m := range targets {
		dm, err := b.GetDomainModel(m.ID)
		if err != nil {
			return fmt.Errorf("read domain model of %s: %w", m.Name, err)
		}
		if dm == nil || len(dm.Entities) == 0 {
			continue
		}

		update := func(dm *domainmodel.DomainModel) error {
			if err := b.UpdateDomainModel(dm); err != nil {
				return fmt.Errorf("write domain model of %s: %w", m.Name, err)
			}
			return nil
		}
		moved, err := layoutModule(cmd.OutOrStdout(), m.Name, dm, layoutDryRun, layoutOnlyOverlapping, update)
		if err != nil {
			return err
		}
		if moved == 0 {
			unchangedTotal += len(dm.Entities)
		}
		movedTotal += moved
	}

	switch {
	case movedTotal == 0 && unchangedTotal == 0:
		fmt.Fprintln(cmd.OutOrStdout(), "Nothing to lay out.")
	case layoutDryRun && (movedTotal > 0 || !layoutOnlyOverlapping):
		fmt.Fprintf(cmd.OutOrStdout(), "Dry run: %d entities would move. Re-run without --dry-run to apply.\n", movedTotal)
	}
	return nil
}

// layoutModule positions the entities of one domain model, prints what it did
// and, unless dryRun, hands the changed model to update. It returns how many
// entities moved.
//
// The full layout (Plan) positions every entity from the association graph.
// With onlyOverlapping it instead moves only the entities whose estimated boxes
// collide (dmlayout.ResolveOverlaps) and leaves every other Location untouched.
func layoutModule(out io.Writer, module string, dm *domainmodel.DomainModel, dryRun, onlyOverlapping bool,
	update func(*domainmodel.DomainModel) error) (int, error) {
	to := map[model.ID]model.Point{}
	if onlyOverlapping {
		plan := dmlayout.ResolveOverlaps(dm)
		for _, p := range plan.Pairs {
			with := module + "." + p.With
			if p.WithAnnotation {
				with = fmt.Sprintf("annotation %q", p.With)
			}
			fmt.Fprintf(out, "  %s.%s overlaps %s\n", module, p.Entity, with)
		}
		for _, mv := range plan.Moves {
			to[mv.ID] = mv.To
		}
	} else {
		to = dmlayout.Plan(dm)
	}

	moved := 0
	var moves []string
	for _, e := range dm.Entities {
		p, ok := to[e.ID]
		if !ok || p == e.Location {
			continue
		}
		moves = append(moves, fmt.Sprintf("  %s.%s  (%d, %d) -> (%d, %d)",
			module, e.Name, e.Location.X, e.Location.Y, p.X, p.Y))
		if !dryRun {
			e.Location = p
		}
		moved++
	}
	sort.Strings(moves)

	if moved == 0 {
		if onlyOverlapping {
			fmt.Fprintf(out, "%s: no overlapping entities (%d entities)\n", module, len(dm.Entities))
		} else {
			fmt.Fprintf(out, "%s: already laid out (%d entities)\n", module, len(dm.Entities))
		}
		return 0, nil
	}
	for _, line := range moves {
		fmt.Fprintln(out, line)
	}
	if dryRun {
		fmt.Fprintf(out, "%s: %d of %d entities would move\n", module, moved, len(dm.Entities))
		return moved, nil
	}
	if err := update(dm); err != nil {
		return 0, err
	}
	fmt.Fprintf(out, "%s: moved %d of %d entities\n", module, moved, len(dm.Entities))
	return moved, nil
}

// layoutTargets picks the modules to touch.
//
// A Marketplace module is excluded by default for the same reason CREATE LAYOUT
// refuses to write into one: an update replaces the module wholesale, so any
// arrangement is thrown away. System is excluded because it is not the user's to
// arrange. A --module the project does not have is an error rather than a silent
// no-op — a typo there would otherwise report success having done nothing.
func layoutTargets(modules []*model.Module, wanted map[string]string) ([]*model.Module, error) {
	var out []*model.Module
	matched := map[string]bool{}
	for _, m := range modules {
		if m == nil {
			continue
		}
		if _, ok := wanted[strings.ToLower(m.Name)]; len(wanted) > 0 && !ok {
			continue
		}
		matched[strings.ToLower(m.Name)] = true
		if m.Name == "System" {
			if len(wanted) > 0 {
				return nil, fmt.Errorf("the System module cannot be laid out")
			}
			continue
		}
		fromMarketplace := m.FromAppStore || strings.TrimSpace(m.AppStoreGuid) != ""
		if fromMarketplace && !layoutIncludeMarketplace {
			if len(wanted) > 0 {
				return nil, fmt.Errorf("%s comes from the Marketplace, and a module update replaces it — "+
					"pass --include-marketplace to lay it out anyway", m.Name)
			}
			continue
		}
		out = append(out, m)
	}

	var missing []string
	for key, spelled := range wanted {
		if !matched[key] {
			missing = append(missing, spelled)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("module not found: %s", strings.Join(missing, ", "))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
