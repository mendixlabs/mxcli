// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// ---------------------------------------------------------------------------
// Not connected
// ---------------------------------------------------------------------------

func TestAlterPage_NotConnected(t *testing.T) {
	mb := &mock.MockBackend{IsConnectedFunc: func() bool { return false }}
	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execAlterPage(ctx, &ast.AlterPageStmt{
		PageName: ast.QualifiedName{Module: "M", Name: "P"},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not connected")
}

// ---------------------------------------------------------------------------
// Page not found
// ---------------------------------------------------------------------------

func TestAlterPage_PageNotFound(t *testing.T) {
	mod := mkModule("MyModule")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return nil, nil },
	}
	h := mkHierarchy(mod)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execAlterPage(ctx, &ast.AlterPageStmt{
		PageName: ast.QualifiedName{Module: "MyModule", Name: "Missing"},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not found")
}

// ---------------------------------------------------------------------------
// Page happy path — SET property + Save
// ---------------------------------------------------------------------------

func TestAlterPage_SetProperty_Success(t *testing.T) {
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "TestPage")
	saved := false
	setPropCalled := false
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return &mock.MockPageMutator{
				SetWidgetPropertyFunc: func(widgetRef string, prop string, value any) error {
					setPropCalled = true
					if widgetRef != "myWidget" {
						t.Errorf("expected widgetRef myWidget, got %s", widgetRef)
					}
					if prop != "Caption" {
						t.Errorf("expected prop Caption, got %s", prop)
					}
					return nil
				},
				SaveFunc: func() error { saved = true; return nil },
			}, nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execAlterPage(ctx, &ast.AlterPageStmt{
		PageName: ast.QualifiedName{Module: "MyModule", Name: "TestPage"},
		Operations: []ast.AlterPageOperation{
			&ast.SetPropertyOp{
				Target:     ast.WidgetRef{Widget: "myWidget"},
				Properties: map[string]any{"Caption": "Hello"},
			},
		},
	}))
	if !setPropCalled {
		t.Error("expected SetWidgetProperty to be called")
	}
	if !saved {
		t.Error("expected Save to be called")
	}
	assertContainsStr(t, buf.String(), "Altered page")
	assertContainsStr(t, buf.String(), "MyModule.TestPage")
}

// Issue #661 — page-level SET of pop-up dimensions routes to the mutator with an
// empty widget ref (page-level), not a widget target.
func TestAlterPage_SetPopupDimensions_Issue661(t *testing.T) {
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "TestPage")
	saved := false
	gotProps := map[string]any{}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return &mock.MockPageMutator{
				SetWidgetPropertyFunc: func(widgetRef string, prop string, value any) error {
					if widgetRef != "" {
						t.Errorf("expected page-level set (empty widgetRef), got %q", widgetRef)
					}
					gotProps[prop] = value
					return nil
				},
				SaveFunc: func() error { saved = true; return nil },
			}, nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execAlterPage(ctx, &ast.AlterPageStmt{
		PageName: ast.QualifiedName{Module: "MyModule", Name: "TestPage"},
		Operations: []ast.AlterPageOperation{
			&ast.SetPropertyOp{
				Target: ast.WidgetRef{Widget: ""},
				Properties: map[string]any{
					"PopupWidth":     800,
					"PopupHeight":    480,
					"PopupResizable": true,
				},
			},
		},
	}))
	if !saved {
		t.Error("expected Save to be called")
	}
	if gotProps["PopupWidth"] != 800 || gotProps["PopupHeight"] != 480 || gotProps["PopupResizable"] != true {
		t.Errorf("unexpected props passed to mutator: %#v", gotProps)
	}
	assertContainsStr(t, buf.String(), "Altered page")
}

// ---------------------------------------------------------------------------
// Snippet happy path
// ---------------------------------------------------------------------------

func TestAlterPage_Snippet_Success(t *testing.T) {
	mod := mkModule("MyModule")
	snp := mkSnippet(mod.ID, "TestSnippet")
	saved := false
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListSnippetsFunc: func() ([]*pages.Snippet, error) {
			return []*pages.Snippet{snp}, nil
		},
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return &mock.MockPageMutator{
				SaveFunc: func() error { saved = true; return nil },
			}, nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, snp.ContainerID, mod.ID)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execAlterPage(ctx, &ast.AlterPageStmt{
		ContainerType: "snippet",
		PageName:      ast.QualifiedName{Module: "MyModule", Name: "TestSnippet"},
	}))
	if !saved {
		t.Error("expected Save to be called")
	}
	assertContainsStr(t, buf.String(), "Altered snippet")
}

// Issue #402 — visitor sets ContainerType to uppercase "SNIPPET"; executor
// must normalise before comparing so the snippet branch is taken.
func TestAlterPage_Snippet_UppercaseContainerType_Issue402(t *testing.T) {
	mod := mkModule("MyModule")
	snp := mkSnippet(mod.ID, "TestSnippet")
	saved := false
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListSnippetsFunc: func() ([]*pages.Snippet, error) {
			return []*pages.Snippet{snp}, nil
		},
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return &mock.MockPageMutator{
				SaveFunc: func() error { saved = true; return nil },
			}, nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, snp.ContainerID, mod.ID)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execAlterPage(ctx, &ast.AlterPageStmt{
		ContainerType: "SNIPPET", // uppercase as produced by the AST visitor
		PageName:      ast.QualifiedName{Module: "MyModule", Name: "TestSnippet"},
	}))
	if !saved {
		t.Error("expected Save to be called")
	}
	assertContainsStr(t, buf.String(), "Altered snippet")
}

// ---------------------------------------------------------------------------
// Open mutator error
// ---------------------------------------------------------------------------

func TestAlterPage_OpenMutatorError(t *testing.T) {
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "TestPage")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return nil, fmt.Errorf("lock error")
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execAlterPage(ctx, &ast.AlterPageStmt{
		PageName: ast.QualifiedName{Module: "MyModule", Name: "TestPage"},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "open page")
}

// ---------------------------------------------------------------------------
// Save error
// ---------------------------------------------------------------------------

func TestAlterPage_SaveError(t *testing.T) {
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "TestPage")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return &mock.MockPageMutator{
				SaveFunc: func() error { return fmt.Errorf("disk full") },
			}, nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execAlterPage(ctx, &ast.AlterPageStmt{
		PageName: ast.QualifiedName{Module: "MyModule", Name: "TestPage"},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "save")
}

// ---------------------------------------------------------------------------
// DROP widget via mutator
// ---------------------------------------------------------------------------

func TestAlterPage_DropWidget_Success(t *testing.T) {
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "TestPage")
	dropCalled := false
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return &mock.MockPageMutator{
				DropWidgetFunc: func(refs []backend.WidgetRef) error {
					dropCalled = true
					if len(refs) != 1 || refs[0].Widget != "oldWidget" {
						t.Errorf("unexpected refs: %v", refs)
					}
					return nil
				},
				SaveFunc: func() error { return nil },
			}, nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execAlterPage(ctx, &ast.AlterPageStmt{
		PageName: ast.QualifiedName{Module: "MyModule", Name: "TestPage"},
		Operations: []ast.AlterPageOperation{
			&ast.DropWidgetOp{
				Targets: []ast.WidgetRef{{Widget: "oldWidget"}},
			},
		},
	}))
	if !dropCalled {
		t.Error("expected DropWidget to be called")
	}
	assertContainsStr(t, buf.String(), "Altered page")
}

// ---------------------------------------------------------------------------
// ADD VARIABLE
// ---------------------------------------------------------------------------

func TestAlterPage_AddVariable_Success(t *testing.T) {
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "TestPage")
	addVarCalled := false
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return &mock.MockPageMutator{
				AddVariableFunc: func(name, dataType, defaultValue string) error {
					addVarCalled = true
					if name != "MyVar" || dataType != "String" || defaultValue != "hello" {
						t.Errorf("unexpected variable: %s %s %s", name, dataType, defaultValue)
					}
					return nil
				},
				SaveFunc: func() error { return nil },
			}, nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execAlterPage(ctx, &ast.AlterPageStmt{
		PageName: ast.QualifiedName{Module: "MyModule", Name: "TestPage"},
		Operations: []ast.AlterPageOperation{
			&ast.AddVariableOp{
				Variable: ast.PageVariable{Name: "MyVar", DataType: "String", DefaultValue: "hello"},
			},
		},
	}))
	if !addVarCalled {
		t.Error("expected AddVariable to be called")
	}
	assertContainsStr(t, buf.String(), "Altered page")
}

// ---------------------------------------------------------------------------
// SET Layout on snippet — unsupported
// ---------------------------------------------------------------------------

func TestAlterPage_SetLayout_Snippet_Unsupported(t *testing.T) {
	mod := mkModule("MyModule")
	snp := mkSnippet(mod.ID, "TestSnippet")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListSnippetsFunc: func() ([]*pages.Snippet, error) {
			return []*pages.Snippet{snp}, nil
		},
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return &mock.MockPageMutator{}, nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, snp.ContainerID, mod.ID)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execAlterPage(ctx, &ast.AlterPageStmt{
		ContainerType: "snippet",
		PageName:      ast.QualifiedName{Module: "MyModule", Name: "TestSnippet"},
		Operations: []ast.AlterPageOperation{
			&ast.SetLayoutOp{
				NewLayout: ast.QualifiedName{Module: "M", Name: "L"},
			},
		},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not supported")
}

// ---------------------------------------------------------------------------
// REPLACE widget — same-name replacement must not false-positive on "duplicate"
// ---------------------------------------------------------------------------

// TestApplyReplaceWidgetMutator_SameNameAllowed verifies that replacing a widget
// with a new widget of the same name does not fail with "duplicate widget name".
// Before the fix, buildWidgetsFromAST received the full page WidgetScope (including
// the target widget), so registerWidgetName rejected the same-name replacement.
func TestApplyReplaceWidgetMutator_SameNameAllowed(t *testing.T) {
	existingID := model.ID("existing-id-123")
	replaceCalled := false

	mutator := &mock.MockPageMutator{
		FindWidgetFunc: func(name string) bool { return name == "myTitle" },
		WidgetScopeFunc: func() map[string]model.ID {
			return map[string]model.ID{"myTitle": existingID}
		},
		ParamScopeFunc: func() (map[string]model.ID, map[string]string) {
			return nil, nil
		},
		EnclosingEntityFunc: func(widgetRef string) string { return "" },
		ReplaceWidgetFunc: func(widgetRef, columnRef string, ws []pages.Widget) error {
			replaceCalled = true
			if widgetRef != "myTitle" || columnRef != "" {
				t.Errorf("unexpected refs: widgetRef=%q columnRef=%q", widgetRef, columnRef)
			}
			return nil
		},
		SaveFunc: func() error { return nil },
	}

	op := &ast.ReplaceWidgetOp{
		Target: ast.WidgetRef{Widget: "myTitle"},
		NewWidgets: []*ast.WidgetV3{
			{Name: "myTitle", Type: "title", Properties: map[string]any{"content": "New Title"}},
		},
	}

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return nil, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))

	err := applyReplaceWidgetMutator(ctx, mutator, op, "MyModule", model.ID("mod-id"))
	if err != nil {
		t.Errorf("same-name replacement should be allowed, got: %v", err)
	}
	if !replaceCalled {
		t.Error("expected ReplaceWidget to be called")
	}
}

// ---------------------------------------------------------------------------
// INSERT custom content column — uses InsertColumns (not InsertWidget)
// ---------------------------------------------------------------------------

// TestAlterPage_InsertCustomContentColumn verifies that INSERT BEFORE/AFTER a
// column ref with a column body routes to InsertColumns (which serializes as
// CustomWidgets$WidgetObject) rather than InsertWidget (which would emit
// Forms$* widget BSON and crash MxBuild with InvalidCastException).
func TestAlterPage_InsertCustomContentColumn(t *testing.T) {
	insertColumnsCalled := false
	insertWidgetCalled := false

	mutator := &mock.MockPageMutator{
		FindWidgetFunc: func(name string) bool { return false },
		WidgetScopeFunc: func() map[string]model.ID {
			return map[string]model.ID{"grid": model.ID("grid-id"), "colName": model.ID("col-id")}
		},
		ParamScopeFunc:      func() (map[string]model.ID, map[string]string) { return nil, nil },
		EnclosingEntityFunc: func(widgetRef string) string { return "MyModule.Customer" },
		InsertColumnsFunc: func(gridRef, afterColumnRef string, position backend.InsertPosition, columns []*backend.DataGridColumnSpec) error {
			insertColumnsCalled = true
			if gridRef != "grid" {
				t.Errorf("expected gridRef 'grid', got %q", gridRef)
			}
			if afterColumnRef != "colName" {
				t.Errorf("expected columnRef 'colName', got %q", afterColumnRef)
			}
			if !strings.EqualFold(string(position), "after") {
				t.Errorf("expected position 'after', got %q", position)
			}
			if len(columns) != 1 {
				t.Fatalf("expected 1 column, got %d", len(columns))
			}
			if len(columns[0].ChildWidgets) != 1 {
				t.Errorf("expected 1 child widget, got %d", len(columns[0].ChildWidgets))
			}
			return nil
		},
		InsertWidgetFunc: func(widgetRef, columnRef string, position backend.InsertPosition, widgets []pages.Widget) error {
			insertWidgetCalled = true
			return nil
		},
	}

	op := &ast.InsertWidgetOp{
		Position: "AFTER",
		Target:   ast.WidgetRef{Widget: "grid", Column: "colName"},
		Widgets: []*ast.WidgetV3{
			{
				Type: "column",
				Name: "colActions",
				Children: []*ast.WidgetV3{
					{Type: "actionbutton", Name: "btnEdit", Properties: map[string]any{"caption": "Edit"}},
				},
			},
		},
	}

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return nil, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))

	err := applyInsertWidgetMutator(ctx, mutator, op, "MyModule", model.ID("mod-id"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !insertColumnsCalled {
		t.Error("expected InsertColumns to be called for column INSERT")
	}
	if insertWidgetCalled {
		t.Error("InsertWidget must NOT be called for column INSERT")
	}
}
