// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// mkSnippetWithParam creates a snippet that declares one entity-typed parameter.
func mkSnippetWithParam(containerID model.ID, snippetName, paramName string) *pages.Snippet {
	snp := mkSnippet(containerID, snippetName)
	snp.Parameters = []*pages.SnippetParameter{
		{
			BaseElement: model.BaseElement{ID: nextID("sp")},
			ContainerID: snp.ID,
			Name:        paramName,
			EntityName:  "Mod.Asset",
		},
	}
	return snp
}

// newPageBuilder returns a pageBuilder wired to the given mock backend and hierarchy.
func newPageBuilder(mb *mock.MockBackend, h *ContainerHierarchy, moduleName string) *pageBuilder {
	var modID model.ID
	for id, name := range h.moduleNames {
		if name == moduleName {
			modID = id
			break
		}
	}
	cache := &executorCache{hierarchy: h}
	return &pageBuilder{
		backend:          mb,
		moduleID:         modID,
		moduleName:       moduleName,
		widgetScope:      make(map[string]model.ID),
		paramScope:       make(map[string]model.ID),
		paramEntityNames: make(map[string]string),
		execCache:        cache,
		widgetBackend:    mb,
	}
}

// TestSnippetCall_MissingParam_ReturnsError verifies that placing a SNIPPETCALL
// without the required Params yields a validation error (issue #291 — guard).
func TestSnippetCall_MissingParam_ReturnsError(t *testing.T) {
	mod := mkModule("Mod")
	snp := mkSnippetWithParam(mod.ID, "MySnippet", "Asset")

	h := mkHierarchy(mod)
	withContainer(h, snp.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:  func() bool { return true },
		ListModulesFunc:  func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc:  func() ([]*types.FolderInfo, error) { return nil, nil },
		ListSnippetsFunc: func() ([]*pages.Snippet, error) { return []*pages.Snippet{snp}, nil },
	}

	pb := newPageBuilder(mb, h, "Mod")
	w := &ast.WidgetV3{
		Type:       "SNIPPETCALL",
		Name:       "sc1",
		Properties: map[string]any{"Snippet": "Mod.MySnippet"},
	}

	_, err := pb.buildSnippetCallV3(w)
	if err == nil {
		t.Fatal("expected validation error for missing snippet parameter, got nil")
	}
	if !strings.Contains(err.Error(), "Asset") {
		t.Errorf("error should mention missing parameter 'Asset', got: %v", err)
	}
}

// TestSnippetCall_WithParam_Succeeds verifies that a SNIPPETCALL with correct
// Params mapping passes validation and produces a SnippetCallWidget with mappings.
func TestSnippetCall_WithParam_Succeeds(t *testing.T) {
	mod := mkModule("Mod")
	snp := mkSnippetWithParam(mod.ID, "MySnippet", "Asset")

	h := mkHierarchy(mod)
	withContainer(h, snp.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:  func() bool { return true },
		ListModulesFunc:  func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc:  func() ([]*types.FolderInfo, error) { return nil, nil },
		ListSnippetsFunc: func() ([]*pages.Snippet, error) { return []*pages.Snippet{snp}, nil },
	}

	pb := newPageBuilder(mb, h, "Mod")
	w := &ast.WidgetV3{
		Type: "SNIPPETCALL",
		Name: "sc1",
		Properties: map[string]any{
			"Snippet": "Mod.MySnippet",
			"Params":  []ast.SnippetCallParam{{ParamName: "Asset", Variable: "$Asset"}},
		},
	}

	sc, err := pb.buildSnippetCallV3(w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sc.ParameterMappings) != 1 {
		t.Fatalf("ParameterMappings: want 1, got %d", len(sc.ParameterMappings))
	}
	if sc.ParameterMappings[0].ParamName != "Asset" {
		t.Errorf("ParamName: want Asset, got %q", sc.ParameterMappings[0].ParamName)
	}
	if sc.ParameterMappings[0].Argument != "$Asset" {
		t.Errorf("Argument: want $Asset, got %q", sc.ParameterMappings[0].Argument)
	}
}

// TestSnippetCall_NoParam_NoSnippetParams_Succeeds verifies that a parameterless
// snippet call against a snippet with no declared parameters works (no regression).
func TestSnippetCall_NoParam_NoSnippetParams_Succeeds(t *testing.T) {
	mod := mkModule("Mod")
	snp := mkSnippet(mod.ID, "Footer") // no parameters declared

	h := mkHierarchy(mod)
	withContainer(h, snp.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:  func() bool { return true },
		ListModulesFunc:  func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc:  func() ([]*types.FolderInfo, error) { return nil, nil },
		ListSnippetsFunc: func() ([]*pages.Snippet, error) { return []*pages.Snippet{snp}, nil },
	}

	pb := newPageBuilder(mb, h, "Mod")
	w := &ast.WidgetV3{
		Type:       "SNIPPETCALL",
		Name:       "sc1",
		Properties: map[string]any{"Snippet": "Mod.Footer"},
	}

	sc, err := pb.buildSnippetCallV3(w)
	if err != nil {
		t.Fatalf("unexpected error for parameterless snippet: %v", err)
	}
	if len(sc.ParameterMappings) != 0 {
		t.Errorf("expected empty ParameterMappings, got %d", len(sc.ParameterMappings))
	}
}

// TestSnippetCall_DollarPrefixParam_Succeeds verifies that param names with $
// prefix (as the user might write $Asset: $var) are matched correctly.
func TestSnippetCall_DollarPrefixParam_Succeeds(t *testing.T) {
	mod := mkModule("Mod")
	snp := mkSnippetWithParam(mod.ID, "MySnippet", "Asset")

	h := mkHierarchy(mod)
	withContainer(h, snp.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:  func() bool { return true },
		ListModulesFunc:  func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc:  func() ([]*types.FolderInfo, error) { return nil, nil },
		ListSnippetsFunc: func() ([]*pages.Snippet, error) { return []*pages.Snippet{snp}, nil },
	}

	pb := newPageBuilder(mb, h, "Mod")
	// User writes $Asset: $Asset (dollar-prefixed param name)
	w := &ast.WidgetV3{
		Type: "SNIPPETCALL",
		Name: "sc1",
		Properties: map[string]any{
			"Snippet": "Mod.MySnippet",
			"Params":  []ast.SnippetCallParam{{ParamName: "$Asset", Variable: "$Asset"}},
		},
	}

	sc, err := pb.buildSnippetCallV3(w)
	if err != nil {
		t.Fatalf("unexpected error with $-prefixed param name: %v", err)
	}
	if len(sc.ParameterMappings) != 1 {
		t.Fatalf("ParameterMappings: want 1, got %d", len(sc.ParameterMappings))
	}
	if sc.ParameterMappings[0].ParamName != "Asset" {
		t.Errorf("ParamName: want Asset (stripped), got %q", sc.ParameterMappings[0].ParamName)
	}
}
