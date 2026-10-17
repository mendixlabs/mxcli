// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// describeMenu renders a standalone Menus$MenuDocument — the reusable menu a
// menu widget points at, as opposed to the menu embedded in a navigation
// profile. Atlas_Core ships Phone_Menu and Tablet_Menu.
//
// The entries are ordinary Menus$MenuItem elements, so the tree is rendered by
// printMenuMDL, the same renderer DESCRIBE NAVIGATION uses — and the same syntax
// CREATE MENU accepts, so the output round-trips.
func describeMenu(ctx *ExecContext, name ast.QualifiedName) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	md, err := ctx.Backend.GetMenuDocumentByQualifiedName(name.Module, name.Name)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return mdlerrors.NewNotFound("menu", name.String())
		}
		return mdlerrors.NewBackend("get menu", err)
	}

	if md.Documentation != "" {
		fmt.Fprintf(ctx.Output, "/**\n * %s\n */\n",
			strings.ReplaceAll(md.Documentation, "\n", "\n * "))
	}

	if md.Excluded {
		fmt.Fprintln(ctx.Output, "-- Excluded from the project")
	}

	// Output is re-executable: the item syntax is the same one CREATE MENU
	// accepts, so describe → exec → describe is a fixed point.
	fmt.Fprintf(ctx.Output, "create or modify menu %s.%s%s {\n", name.Module, md.Name, describeFolderClause(ctx, md.ContainerID))
	printMenuMDL(ctx, ctx.Output, md.Items, 1, "CREATE MENU")
	fmt.Fprintln(ctx.Output, "};")
	return nil
}

// execCreateMenu handles CREATE [OR MODIFY] MENU Module.Name { items }.
//
// Like CREATE NAVIGATION, the item list is the document's complete contents, so
// a modify replaces the items wholesale rather than merging. The existing
// document's $ID is reused on modify, so references to the menu survive and the
// unit is rewritten in place rather than replaced.
func execCreateMenu(ctx *ExecContext, s *ast.CreateMenuStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	mod, err := ctx.Backend.GetModuleByName(s.Name.Module)
	if err != nil || mod == nil {
		return mdlerrors.NewNotFound("module", s.Name.Module)
	}

	existing, _ := ctx.Backend.GetMenuDocumentByQualifiedName(s.Name.Module, s.Name.Name)
	if existing != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExists("menu", s.Name.String())
	}

	// Resolve the pages and flows its items name before anything is written, a
	// folder included. An excluded document is not validated by Mendix and keeps
	// its dangling targets, as an excluded page does.
	if existing == nil || !existing.Excluded {
		if err := validateNavigationTargetsForExec(ctx, s); err != nil {
			return err
		}
	}

	var existingContainer model.ID
	if existing != nil {
		existingContainer = existing.ContainerID
	}
	containerID, err := containerForDocument(ctx, mod.ID, s.Folder, existingContainer)
	if err != nil {
		return err
	}

	md := &types.MenuDocument{
		Name:          s.Name.Name,
		ContainerID:   containerID,
		Documentation: s.Documentation,
	}
	var storedItems []*types.NavMenuItem
	if existing != nil {
		storedItems = existing.Items
	}
	kept := map[string]string{}
	if md.Items, err = menuItemsFromAST(newMenuActionBuilder(ctx), s.Items, storedItems, "", kept); err != nil {
		return err
	}
	// A rewrite that carried no doc comment keeps the stored one (#1018).
	if existing != nil {
		md.Documentation = carriedDocumentation(s.DocumentationSet, s.Documentation, existing.Documentation)
	}

	if existing != nil {
		// Preserve the document's identity and the properties MDL does not
		// author, so a modify does not silently reset them.
		md.ID = existing.ID
		md.ExportLevel = existing.ExportLevel
		md.Excluded = existing.Excluded
		if md.Documentation == "" {
			md.Documentation = existing.Documentation
		}
		if err := ctx.Backend.UpdateMenuDocument(md); err != nil {
			return mdlerrors.NewBackend("update menu", err)
		}
		if _, err := applyDocumentFolder(ctx, md.ID, existingContainer, containerID); err != nil {
			return err
		}
		ctx.ReportMutation("Modified", "menu %s", s.Name.String())
		reportKeptMenuActions(ctx, kept)
		return nil
	}

	if err := ctx.Backend.CreateMenuDocument(md); err != nil {
		return mdlerrors.NewBackend("create menu", err)
	}
	fmt.Fprintf(ctx.Output, "Created menu %s\n", s.Name.String())
	return nil
}

// execDropMenu handles DROP MENU Module.Name.
func execDropMenu(ctx *ExecContext, s *ast.DropMenuStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	md, err := ctx.Backend.GetMenuDocumentByQualifiedName(s.Name.Module, s.Name.Name)
	if err != nil || md == nil {
		return mdlerrors.NewNotFound("menu", s.Name.String())
	}
	if err := ctx.Backend.DeleteMenuDocument(md.ID); err != nil {
		return mdlerrors.NewBackend("drop menu", err)
	}
	fmt.Fprintf(ctx.Output, "Dropped menu %s\n", s.Name.String())
	return nil
}

// menuItemsFromAST converts parsed menu items to the semantic model the menu
// document writer reads.
//
// stored is the document's current items at the same place in the tree, paired
// by caption as convertMenuItemDefs pairs a navigation menu's (ako/mxcli#980):
// an item that states no action keeps a stored action MDL cannot express, and a
// stated action is built by the page builder and carries the stored one, so the
// writer can keep what the script cannot say about an unchanged action.
func menuItemsFromAST(mb *menuActionBuilder, defs []ast.NavMenuItemDef, stored []*types.NavMenuItem, path string, kept map[string]string) ([]*types.NavMenuItem, error) {
	var out []*types.NavMenuItem
	used := make([]bool, len(stored))
	for _, d := range defs {
		match := pairStoredMenuItem(d.Caption, stored, used)
		itemPath := path + "'" + d.Caption + "'"
		item := &types.NavMenuItem{Caption: d.Caption, Icon: d.Icon, IconCode: d.IconCode}
		// The icon kind the script wrote: a glyph carries a code and no name,
		// an image points into an image collection. Reading only the name made
		// every icon a collection icon, and a glyph nothing at all.
		kind := d.IconKind
		if kind == types.MenuIconNone && d.Icon != "" {
			kind = types.MenuIconCollection
		}
		item.IconType = types.MenuIconStorageType(kind)
		if d.Page != nil {
			item.Page = d.Page.String()
			item.ActionType = "PageAction"
		} else if d.Microflow != nil {
			item.Microflow = d.Microflow.String()
			item.ActionType = "MicroflowAction"
		} else if d.SignOut {
			// Studio Pro stores a sign-out menu item as the same
			// Forms$SignOutClientAction a button carries (measured on
			// ako/TestApp), so it is an ActionType rather than a target.
			item.ActionType = "SignOutAction"
		} else {
			item.ActionType = "NoAction"
		}
		action, err := mb.build(d, itemPath)
		if err != nil {
			return nil, err
		}
		item.Action = action
		var storedSubs []*types.NavMenuItem
		if match != nil {
			storedSubs = match.Items
			if action != nil {
				item.StoredAction = match.StoredAction
			} else if keep, kind := keepStoredMenuAction(mb.execCtx(), d, match); keep {
				item.ActionType = match.ActionType
				item.StoredAction = match.StoredAction
				kept[itemPath] = kind
			}
		}
		if item.Items, err = menuItemsFromAST(mb, d.Items, storedSubs, itemPath+" > ", kept); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}
