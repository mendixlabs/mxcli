// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/pages"
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

	err := applyReplaceWidgetMutator(ctx, mutator, op, "MyModule", model.ID("mod-id"), nil)
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

// ALTER PAGE writes a text into the project's default language
// (pagemutator.setTextsTextTranslation reads model.AuthoringLanguage), so the
// language must be resolved before the first mutation — not left at whatever
// an earlier statement or the en_US default put there. A de_DE project whose
// tab caption had only en_US got its `set Caption` written to en_US, leaving
// the CE4899 it was meant to fix.
func TestAlterPage_ResolvesAuthoringLanguageBeforeWriting(t *testing.T) {
	prev := model.AuthoringLanguage()
	model.SetAuthoringLanguage("en_US")
	t.Cleanup(func() { model.SetAuthoringLanguage(prev) })

	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "TestPage")
	var langAtWrite string
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) {
			return &model.ProjectSettings{Language: &model.LanguageSettings{DefaultLanguageCode: "de_DE"}}, nil
		},
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return &mock.MockPageMutator{
				SetWidgetPropertyFunc: func(string, string, any) error {
					langAtWrite = model.AuthoringLanguage()
					return nil
				},
				SaveFunc: func() error { return nil },
			}, nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execAlterPage(ctx, &ast.AlterPageStmt{
		PageName: ast.QualifiedName{Module: "MyModule", Name: "TestPage"},
		Operations: []ast.AlterPageOperation{
			&ast.SetPropertyOp{
				Target:     ast.WidgetRef{Widget: "tabPage2"},
				Properties: map[string]any{"Caption": "Benutzer"},
			},
		},
	}))
	if langAtWrite != "de_DE" {
		t.Fatalf("authoring language during the write = %q, want the project's de_DE", langAtWrite)
	}
}

// ---------------------------------------------------------------------------
// ADD / DROP PARAMETERS (mendixlabs/mxcli#1234)
// ---------------------------------------------------------------------------

// alterParamsBackend is a project with MyModule.Customer, one page and one
// snippet, whose mutator records the parameter it is asked to add.
func alterParamsBackend(t *testing.T, added *[]backend.PageParameterSpec) (*ExecContext, *bytes.Buffer) {
	return alterParamsBackendURL(t, added, "")
}

func alterParamsBackendURL(t *testing.T, added *[]backend.PageParameterSpec, url string) (*ExecContext, *bytes.Buffer) {
	t.Helper()
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "TestPage")
	pg.URL = url
	snp := mkSnippet(mod.ID, "TestSnippet")
	ent := &domainmodel.Entity{BaseElement: model.BaseElement{ID: nextID("ent")}, Name: "Customer"}
	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm")},
		ContainerID: mod.ID,
		Entities:    []*domainmodel.Entity{ent},
	}
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc:      func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:        func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		ListSnippetsFunc:     func() ([]*pages.Snippet, error) { return []*pages.Snippet{snp}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return &mock.MockPageMutator{
				AddParameterFunc: func(p backend.PageParameterSpec) error {
					*added = append(*added, p)
					return nil
				},
				SaveFunc: func() error { return nil },
			}, nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)
	withContainer(h, snp.ContainerID, mod.ID)
	withContainer(h, dm.ID, mod.ID)
	return newMockCtx(t, withBackend(mb), withHierarchy(h))
}

func TestAlterPage_AddParameters_ResolvesTypes(t *testing.T) {
	var added []backend.PageParameterSpec
	ctx, buf := alterParamsBackend(t, &added)
	assertNoError(t, execAlterPage(ctx, &ast.AlterPageStmt{
		PageName: ast.QualifiedName{Module: "MyModule", Name: "TestPage"},
		Operations: []ast.AlterPageOperation{
			&ast.AddParameterOp{Parameter: ast.PageParameter{
				Name:       "Customer",
				EntityType: ast.QualifiedName{Module: "MyModule", Name: "Customer"},
				Type:       ast.DataType{Kind: ast.TypeEntity},
			}},
			&ast.AddParameterOp{Parameter: ast.PageParameter{Name: "Count", Type: ast.DataType{Kind: ast.TypeLong}}},
		},
	}))
	want := []backend.PageParameterSpec{
		{Name: "Customer", EntityName: "MyModule.Customer"},
		// Storage has no LongType; Integer/Long is one type (pageParamBSONType).
		{Name: "Count", PrimitiveType: "DataTypes$IntegerType"},
	}
	if len(added) != len(want) || added[0] != want[0] || added[1] != want[1] {
		t.Fatalf("added %+v, want %+v", added, want)
	}
	assertContainsStr(t, buf.String(), "Altered page")
}

func TestAlterPage_AddParameters_UnknownEntityRefused(t *testing.T) {
	var added []backend.PageParameterSpec
	ctx, _ := alterParamsBackend(t, &added)
	err := execAlterPage(ctx, &ast.AlterPageStmt{
		PageName: ast.QualifiedName{Module: "MyModule", Name: "TestPage"},
		Operations: []ast.AlterPageOperation{
			&ast.AddParameterOp{Parameter: ast.PageParameter{
				Name:       "Order",
				EntityType: ast.QualifiedName{Module: "MyModule", Name: "Order"},
				Type:       ast.DataType{Kind: ast.TypeEntity},
			}},
		},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "MyModule.Order")
	if len(added) != 0 {
		t.Errorf("a parameter of an unknown entity reached the mutator: %+v", added)
	}
}

// A snippet parameter must be an entity (CE0046), as on CREATE SNIPPET.
// `alter snippet … add parameters $Label: String` is the 11.17 feature: below
// it mxbuild rejects the parameter with CE0046 (measured on 11.13.0 and
// 11.16.0), from it the same bytes check clean (11.17.0). The refusal names
// the floor and the CE; the 11.17 run is the control that it is the version
// deciding, not the primitive.
func TestAlterSnippet_AddPrimitiveParameterGatedAt1117(t *testing.T) {
	for _, c := range []struct {
		version string
		minor   int
		refused bool
	}{{"11.16.0", 16, true}, {"11.17.0", 17, false}} {
		t.Run(c.version, func(t *testing.T) {
			var added []backend.PageParameterSpec
			ctx, _ := alterParamsBackend(t, &added)
			v := &types.ProjectVersion{ProductVersion: c.version, MajorVersion: 11, MinorVersion: c.minor}
			ctx.Backend.(*mock.MockBackend).ProjectVersionFunc = func() *types.ProjectVersion { return v }
			err := execAlterPage(ctx, &ast.AlterPageStmt{
				ContainerType: "snippet",
				PageName:      ast.QualifiedName{Module: "MyModule", Name: "TestSnippet"},
				Operations: []ast.AlterPageOperation{
					&ast.AddParameterOp{Parameter: ast.PageParameter{Name: "Label", Type: ast.DataType{Kind: ast.TypeString}}},
				},
			})
			if c.refused {
				assertError(t, err)
				assertContainsStr(t, err.Error(), "CE0046")
				assertContainsStr(t, err.Error(), "11.17")
				if len(added) != 0 {
					t.Errorf("a primitive snippet parameter reached the mutator below 11.17: %+v", added)
				}
				return
			}
			assertNoError(t, err)
			if len(added) != 1 || added[0].PrimitiveType != "DataTypes$StringType" {
				t.Errorf("added %+v, want one $Label typed DataTypes$StringType", added)
			}
		})
	}
}

// A page with a URL needs a `{Name}` segment per parameter or mxbuild fails it
// with CE5601 (measured on 11.12.1 with this exact add). The URL the statement
// ends with is what counts.
func TestAlterPage_AddParameters_URLSegment(t *testing.T) {
	addCount := &ast.AddParameterOp{Parameter: ast.PageParameter{Name: "Count", Type: ast.DataType{Kind: ast.TypeInteger}}}

	var added []backend.PageParameterSpec
	ctx, _ := alterParamsBackendURL(t, &added, "test/{Customer}")
	err := execAlterPage(ctx, &ast.AlterPageStmt{
		PageName:   ast.QualifiedName{Module: "MyModule", Name: "TestPage"},
		Operations: []ast.AlterPageOperation{addCount},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "CE5601")
	assertContainsStr(t, err.Error(), "test/{Customer}/{Count}")
	if len(added) != 0 {
		t.Errorf("refused statement still reached the mutator: %+v", added)
	}

	// Control: the same add with the URL extended in the same statement.
	added = nil
	ctx, _ = alterParamsBackendURL(t, &added, "test/{Customer}")
	assertNoError(t, execAlterPage(ctx, &ast.AlterPageStmt{
		PageName: ast.QualifiedName{Module: "MyModule", Name: "TestPage"},
		Operations: []ast.AlterPageOperation{
			&ast.SetPropertyOp{Properties: map[string]any{"Url": "test/{Customer}/{Count}"}},
			addCount,
		},
	}))
	if len(added) != 1 {
		t.Errorf("added %+v, want the Count parameter", added)
	}
}
