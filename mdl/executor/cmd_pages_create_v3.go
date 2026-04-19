// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// ============================================================================
// V3 Page Creation
// ============================================================================

// execCreatePageV3 handles CREATE PAGE statement with V3 syntax.
func execCreatePageV3(ctx *ExecContext, s *ast.CreatePageStmtV3) error {
	e := ctx.executor
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	// Version pre-check: page parameters require 11.0+
	if len(s.Parameters) > 0 {
		if err := checkFeature(ctx, "pages", "page_parameters",
			"CREATE PAGE with parameters",
			"pass data via a non-persistent entity or microflow parameter instead"); err != nil {
			return err
		}
	}

	// Find or auto-create module
	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return mdlerrors.NewBackend(fmt.Sprintf("find module %s", s.Name.Module), err)
	}
	moduleID := module.ID

	// Check if page already exists - collect ALL duplicates
	existingPages, _ := ctx.Backend.ListPages()
	var pagesToDelete []model.ID
	for _, p := range existingPages {
		modID := getModuleID(ctx, p.ContainerID)
		modName := getModuleName(ctx, modID)
		if modName == s.Name.Module && p.Name == s.Name.Name {
			if !s.IsReplace && !s.IsModify && len(pagesToDelete) == 0 {
				return mdlerrors.NewAlreadyExists("page", s.Name.String())
			}
			pagesToDelete = append(pagesToDelete, p.ID)
		}
	}

	// Build the page BEFORE deleting the old one (atomic: if build fails, old page is preserved)
	pb := &pageBuilder{
		backend:          ctx.Backend,
		moduleID:         moduleID,
		moduleName:       s.Name.Module,
		widgetScope:      make(map[string]model.ID),
		paramScope:       make(map[string]model.ID),
		paramEntityNames: make(map[string]string),
		execCache:        ctx.Cache,
		fragments:        ctx.Fragments,
		themeRegistry:    ctx.GetThemeRegistry(),
		widgetBackend:    ctx.Backend,
	}

	page, err := pb.buildPageV3(s)
	if err != nil {
		return mdlerrors.NewBackend("build page", err)
	}

	// Replace or create the page in the MPR
	if len(pagesToDelete) > 0 {
		// Reuse first existing page's UUID to avoid git delete+add (which crashes Studio Pro RevStatusCache)
		page.ID = pagesToDelete[0]
		if err := ctx.Backend.UpdatePage(page); err != nil {
			return mdlerrors.NewBackend("update page", err)
		}
		// Delete any additional duplicates
		for _, id := range pagesToDelete[1:] {
			if err := ctx.Backend.DeletePage(id); err != nil {
				return mdlerrors.NewBackend("delete duplicate page", err)
			}
		}
	} else {
		if err := ctx.Backend.CreatePage(page); err != nil {
			return mdlerrors.NewBackend("create page", err)
		}
	}

	// Track the created page so it can be resolved by subsequent page references
	e.trackCreatedPage(s.Name.Module, s.Name.Name, page.ID, moduleID)

	// Invalidate hierarchy cache so the new page's container is visible
	invalidateHierarchy(ctx)

	fmt.Fprintf(ctx.Output, "Created page %s\n", s.Name.String())
	return nil
}

// execCreateSnippetV3 handles CREATE SNIPPET statement with V3 syntax.
func execCreateSnippetV3(ctx *ExecContext, s *ast.CreateSnippetStmtV3) error {
	e := ctx.executor
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	// Find or auto-create module
	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return mdlerrors.NewBackend(fmt.Sprintf("find module %s", s.Name.Module), err)
	}
	moduleID := module.ID

	// Check if snippet already exists - collect ALL duplicates
	existingSnippets, _ := ctx.Backend.ListSnippets()
	var snippetsToDelete []model.ID
	for _, snip := range existingSnippets {
		modID := getModuleID(ctx, snip.ContainerID)
		modName := getModuleName(ctx, modID)
		if modName == s.Name.Module && snip.Name == s.Name.Name {
			if !s.IsReplace && !s.IsModify && len(snippetsToDelete) == 0 {
				return mdlerrors.NewAlreadyExists("snippet", s.Name.String())
			}
			snippetsToDelete = append(snippetsToDelete, snip.ID)
		}
	}

	// Build the snippet BEFORE deleting the old one (atomic: if build fails, old snippet is preserved)
	pb := &pageBuilder{
		backend:          ctx.Backend,
		moduleID:         moduleID,
		moduleName:       s.Name.Module,
		widgetScope:      make(map[string]model.ID),
		paramScope:       make(map[string]model.ID),
		paramEntityNames: make(map[string]string),
		execCache:        ctx.Cache,
		fragments:        ctx.Fragments,
		themeRegistry:    ctx.GetThemeRegistry(),
		widgetBackend:    ctx.Backend,
	}

	snippet, err := pb.buildSnippetV3(s)
	if err != nil {
		return mdlerrors.NewBackend("build snippet", err)
	}

	// Delete old snippets only after successful build
	for _, id := range snippetsToDelete {
		if err := ctx.Backend.DeleteSnippet(id); err != nil {
			return mdlerrors.NewBackend("delete existing snippet", err)
		}
	}

	// Create the snippet in the MPR
	if err := ctx.Backend.CreateSnippet(snippet); err != nil {
		return mdlerrors.NewBackend("create snippet", err)
	}

	// Track the created snippet so it can be resolved by subsequent snippet references
	e.trackCreatedSnippet(s.Name.Module, s.Name.Name, snippet.ID, moduleID)

	// Invalidate hierarchy cache so the new snippet's container is visible
	invalidateHierarchy(ctx)

	fmt.Fprintf(ctx.Output, "Created snippet %s\n", s.Name.String())
	return nil
}
