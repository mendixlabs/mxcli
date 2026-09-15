// SPDX-License-Identifier: Apache-2.0

// Package executor - DROP LAYOUT command
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// execDropLayout handles DROP LAYOUT statements.
//
// Layouts were the only document mxcli could create and alter but not delete
// (mendixlabs/mxcli#1063): every other doctype — pages, snippets, microflows,
// entities, queues, rules, folders — has a DROP. The backend could already do
// it (DeleteLayout is what CREATE OR REPLACE LAYOUT uses for the replace half),
// so only the verb was missing, and its absence was worst exactly when it was
// needed: a layout written by mistake failed the build and could not be removed
// headlessly.
//
// # Pages that still use it are WARNED about, not refused
//
// This follows what the rest of mxcli's DROPs do — none of them refuse on
// dependents — and matches how a layout is normally replaced: drop, then create
// the corrected one under the same name, with the pages still bound by qualified
// name and rebinding to the new document. Refusing would break that, and the
// order in a script is the author's business.
//
// The warning matters because of what the pages then do. Measured on 11.12.1,
// dropping a layout one page was bound to:
//
//	[error] [CE1613] "The selected layout 'MyFirstModule.App_Mine' no longer
//	                  exists." at Page 'MyFirstModule.Home_Web'
//	[error] [CE1613] "The selected placeholder 'MyFirstModule.App_Mine.Main' no
//	                  longer exists." at Content for placeholder 'Main'
//
// Both name the PAGE end, never the layout that was removed — so without this
// warning the drop is silent and its consequence surfaces a build later,
// attributed to a document the author did not touch.
func execDropLayout(ctx *ExecContext, s *ast.DropLayoutStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	layouts, err := ctx.Backend.ListLayouts()
	if err != nil {
		return mdlerrors.NewBackend("list layouts", err)
	}

	for _, l := range layouts {
		modID := getModuleID(ctx, l.ContainerID)
		if getModuleName(ctx, modID) != s.Name.Module || l.Name != s.Name.Name {
			continue
		}

		// Collected BEFORE the delete: afterwards the layout is gone and the
		// pages' references resolve to nothing, so there would be no way to
		// name them.
		if users := pagesUsingLayout(ctx, s.Name.String()); len(users) > 0 {
			fmt.Fprintf(ctx.Output,
				"Warning: %d page(s) still use layout %s and will fail to build (CE1613) "+
					"until they are repointed: %s\n",
				len(users), s.Name.String(), joinUpTo(users, 10))
			fmt.Fprintf(ctx.Output,
				"  Repoint them with `alter pages set layout = <Module.Layout> where layout = %s`, "+
					"or re-create %s.\n", s.Name.String(), s.Name.String())
		}

		if err := ctx.Backend.DeleteLayout(l.ID); err != nil {
			return mdlerrors.NewBackend("delete layout", err)
		}
		invalidateHierarchy(ctx)
		fmt.Fprintf(ctx.Output, "Dropped layout %s\n", s.Name.String())
		return nil
	}

	return mdlerrors.NewNotFound("layout", s.Name.String())
}

// pagesUsingLayout returns the qualified names of pages bound to the layout,
// sorted. A page that cannot be read is skipped rather than guessed at: the
// warning exists to be trusted, and naming a page that is not affected is worse
// than the silence this replaces.
func pagesUsingLayout(ctx *ExecContext, layoutQN string) []string {
	allPages, err := ctx.Backend.ListPages()
	if err != nil {
		return nil
	}
	var out []string
	for _, p := range allPages {
		name, err := ctx.Backend.PageLayoutName(p.ID)
		if err != nil || name != layoutQN {
			continue
		}
		modName := getModuleName(ctx, getModuleID(ctx, p.ContainerID))
		out = append(out, modName+"."+p.Name)
	}
	sort.Strings(out)
	return out
}

// joinUpTo keeps the warning readable when a layout is used by the whole app —
// repointing is a bulk operation, so the full list is noise past the first few.
func joinUpTo(names []string, max int) string {
	if len(names) <= max {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(names[:max], ", "), len(names)-max)
}
