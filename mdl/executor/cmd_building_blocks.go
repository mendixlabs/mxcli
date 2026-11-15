// SPDX-License-Identifier: Apache-2.0

// Package executor - Building block commands (SHOW/DESCRIBE BUILDING BLOCKS).
// Building blocks are read-only: DESCRIBE output is informational, not a
// round-trippable CREATE statement.
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/doccomment"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// listBuildingBlocks handles SHOW BUILDING BLOCKS command.
func listBuildingBlocks(ctx *ExecContext, moduleName string) error {
	// Get hierarchy for module/folder resolution
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Get all building blocks
	blocks, err := ctx.Backend.ListBuildingBlocks()
	if err != nil {
		return mdlerrors.NewBackend("list building blocks", err)
	}

	// Collect rows
	type row struct {
		qualifiedName string
		module        string
		name          string
		displayName   string
		platform      string
		category      string
	}
	var rows []row

	for _, bb := range blocks {
		modID := h.FindModuleID(bb.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName == "" || modName == moduleName {
			qualifiedName := modName + "." + bb.Name
			rows = append(rows, row{qualifiedName, modName, bb.Name, bb.DisplayName, bb.Platform, bb.TemplateCategory})
		}
	}

	// Sort by qualified name
	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].qualifiedName) < strings.ToLower(rows[j].qualifiedName)
	})

	result := &TableResult{
		Columns: []string{"Qualified Name", "Module", "Name", "Display Name", "Platform", "Category"},
		Summary: fmt.Sprintf("(%d building blocks)", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.qualifiedName, r.module, r.name, r.displayName, r.platform, r.category})
	}
	return writeResult(ctx, result)
}

// describeBuildingBlock handles DESCRIBE BUILDING BLOCK command. Building blocks
// are read-only, so the output is informational (a comment header plus the widget
// tree), not a re-executable CREATE statement.
func describeBuildingBlock(ctx *ExecContext, name ast.QualifiedName) error {
	// Get hierarchy for module/folder resolution
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find the building block
	allBlocks, err := ctx.Backend.ListBuildingBlocks()
	if err != nil {
		return mdlerrors.NewBackend("list building blocks", err)
	}

	var found *pages.BuildingBlock
	for _, bb := range allBlocks {
		modID := h.FindModuleID(bb.ContainerID)
		modName := h.GetModuleName(modID)
		if bb.Name == name.Name && (name.Module == "" || modName == name.Module) {
			found = bb
			break
		}
	}

	if found == nil {
		return mdlerrors.NewNotFound("building block", name.String())
	}

	// Get module name for the building block
	modID := h.FindModuleID(found.ContainerID)
	modName := h.GetModuleName(modID)

	// Output documentation if present
	if found.Documentation != "" {
		lines := strings.Split(doccomment.Escape(found.Documentation), "\n")
		fmt.Fprint(ctx.Output, "/**\n")
		for _, line := range lines {
			fmt.Fprintf(ctx.Output, " * %s\n", line)
		}
		fmt.Fprint(ctx.Output, " */\n")
	}

	// Output informational header (building blocks cannot be authored via MDL).
	fmt.Fprintf(ctx.Output, "-- Building Block: %s.%s\n", modName, found.Name)
	if found.DisplayName != "" {
		fmt.Fprintf(ctx.Output, "-- Display Name: %s\n", found.DisplayName)
	}
	if found.Platform != "" {
		fmt.Fprintf(ctx.Output, "-- Platform: %s\n", found.Platform)
	}
	if found.TemplateCategory != "" {
		fmt.Fprintf(ctx.Output, "-- Category: %s\n", found.TemplateCategory)
	}
	fmt.Fprintf(ctx.Output, "-- Building blocks are read-only; they cannot be created via MDL.\n")

	// Output widgets from raw building block data
	rawWidgets := getBuildingBlockWidgetsFromRaw(ctx, found.ID)
	if len(rawWidgets) > 0 {
		fmt.Fprint(ctx.Output, "{\n")
		for _, w := range rawWidgets {
			outputWidgetMDLV3(ctx, w, 1)
		}
		fmt.Fprint(ctx.Output, "}")
	} else {
		fmt.Fprint(ctx.Output, "{\n}")
	}

	fmt.Fprint(ctx.Output, "\n")
	return nil
}

// getBuildingBlockWidgetsFromRaw extracts widgets from raw building block BSON.
// Building blocks store their widget tree as a top-level "Widgets" array (same
// shape as a Studio Pro snippet).
func getBuildingBlockWidgetsFromRaw(ctx *ExecContext, blockID model.ID) []rawWidget {
	// Get raw building block data
	rawData, err := ctx.Backend.GetRawUnit(blockID)
	if err != nil {
		return nil
	}

	// Handle both formats:
	// - Studio Pro uses "Widgets" (plural): a top-level array of widgets
	// - fallback "Widget" (singular): a single container whose "Widgets" field holds children
	var widgetsArray []any
	if wa := getBsonArrayElements(rawData["Widgets"]); wa != nil {
		widgetsArray = wa
	} else if widgetContainer, ok := rawData["Widget"].(map[string]any); ok {
		widgetsArray = getBsonArrayElements(widgetContainer["Widgets"])
	}
	if widgetsArray == nil {
		return nil
	}

	var result []rawWidget
	for _, w := range widgetsArray {
		if wMap, ok := w.(map[string]any); ok {
			result = append(result, parseRawWidget(ctx, wMap)...)
		}
	}
	return result
}
