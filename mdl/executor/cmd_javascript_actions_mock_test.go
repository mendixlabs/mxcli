// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

func TestShowJavaScriptActions_Mock(t *testing.T) {
	mod := mkModule("WebMod")
	jsa := &types.JavaScriptAction{
		BaseElement: model.BaseElement{ID: nextID("jsa")},
		ContainerID: mod.ID,
		Name:        "ShowAlert",
		Platform:    "Web",
	}

	h := mkHierarchy(mod)
	withContainer(h, jsa.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:           func() bool { return true },
		ListJavaScriptActionsFunc: func() ([]*types.JavaScriptAction, error) { return []*types.JavaScriptAction{jsa}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listJavaScriptActions(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "Qualified Name")
	assertContainsStr(t, out, "WebMod.ShowAlert")
}

func TestDescribeJavaScriptAction_Mock(t *testing.T) {
	mod := mkModule("WebMod")

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ReadJavaScriptActionByNameFunc: func(qn string) (*types.JavaScriptAction, error) {
			return &types.JavaScriptAction{
				BaseElement: model.BaseElement{ID: nextID("jsa")},
				ContainerID: mod.ID,
				Name:        "ShowAlert",
				Platform:    "Web",
			}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeJavaScriptAction(ctx, ast.QualifiedName{Module: "WebMod", Name: "ShowAlert"}))

	out := buf.String()
	assertContainsStr(t, out, "create or modify javascript action")
}

func TestDescribeJavaScriptAction_NotFound(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ReadJavaScriptActionByNameFunc: func(qn string) (*types.JavaScriptAction, error) {
			return nil, fmt.Errorf("not found: %s", qn)
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	assertError(t, describeJavaScriptAction(ctx, ast.QualifiedName{Module: "X", Name: "NoSuch"}))
}

func TestShowJavaScriptActions_FilterByModule(t *testing.T) {
	mod := mkModule("WebMod")
	jsa := &types.JavaScriptAction{
		BaseElement: model.BaseElement{ID: nextID("jsa")},
		ContainerID: mod.ID,
		Name:        "ShowAlert",
		Platform:    "Web",
	}

	h := mkHierarchy(mod)
	withContainer(h, jsa.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:           func() bool { return true },
		ListJavaScriptActionsFunc: func() ([]*types.JavaScriptAction, error) { return []*types.JavaScriptAction{jsa}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listJavaScriptActions(ctx, "WebMod"))
	assertContainsStr(t, buf.String(), "WebMod.ShowAlert")
}

func TestCreateJavaScriptAction_Mock(t *testing.T) {
	mod := mkModule("WebMod")
	h := mkHierarchy(mod)

	var captured *types.JavaScriptAction
	var wroteSource string
	mb := &mock.MockBackend{
		IsConnectedFunc:           func() bool { return true },
		ListModulesFunc:           func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListJavaScriptActionsFunc: func() ([]*types.JavaScriptAction, error) { return nil, nil },
		CreateJavaScriptActionFunc: func(jsa *types.JavaScriptAction) error {
			captured = jsa
			return nil
		},
		WriteJavaScriptSourceFileFunc: func(moduleName, actionName, jsCode string, _ []*types.JavaActionParameter, _ types.CodeActionReturnType) error {
			wroteSource = moduleName + "/" + actionName + ":" + jsCode
			return nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execCreateJavaScriptAction(ctx, &ast.CreateJavaScriptActionStmt{
		Name:            ast.QualifiedName{Module: "WebMod", Name: "DoThing"},
		Parameters:      []ast.JavaActionParam{{Name: "Input", Type: ast.DataType{Kind: ast.TypeString}, IsRequired: true}},
		ReturnType:      ast.DataType{Kind: ast.TypeBoolean},
		ExposedCaption:  "Do Thing",
		ExposedCategory: "Demo",
		Platform:        "Native",
		JavaScriptCode:  "return Promise.resolve(true);",
	})
	assertNoError(t, err)

	if captured == nil {
		t.Fatal("CreateJavaScriptAction was not called")
	}
	if captured.Name != "DoThing" || captured.ContainerID != mod.ID {
		t.Errorf("action = %+v", captured)
	}
	if captured.Platform != "Native" {
		t.Errorf("platform = %q, want Native", captured.Platform)
	}
	if captured.TypeName != "JavaScriptActions$JavaScriptAction" {
		t.Errorf("typeName = %q", captured.TypeName)
	}
	if len(captured.Parameters) != 1 || captured.Parameters[0].TypeName != "JavaScriptActions$JavaScriptActionParameter" {
		t.Errorf("params = %+v", captured.Parameters)
	}
	if captured.MicroflowActionInfo == nil || captured.MicroflowActionInfo.Caption != "Do Thing" {
		t.Errorf("MAI = %+v", captured.MicroflowActionInfo)
	}
	if wroteSource != "WebMod/DoThing:return Promise.resolve(true);" {
		t.Errorf("source write = %q", wroteSource)
	}
	assertContainsStr(t, buf.String(), "Created javascript action: WebMod.DoThing")
}

func TestCreateJavaScriptAction_DuplicateRejected(t *testing.T) {
	mod := mkModule("WebMod")
	h := mkHierarchy(mod)
	existing := &types.JavaScriptAction{
		BaseElement: model.BaseElement{ID: nextID("jsa")},
		ContainerID: mod.ID,
		Name:        "Dup",
	}
	withContainer(h, existing.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:           func() bool { return true },
		ListModulesFunc:           func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListJavaScriptActionsFunc: func() ([]*types.JavaScriptAction, error) { return []*types.JavaScriptAction{existing}, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execCreateJavaScriptAction(ctx, &ast.CreateJavaScriptActionStmt{
		Name:       ast.QualifiedName{Module: "WebMod", Name: "Dup"},
		ReturnType: ast.DataType{Kind: ast.TypeBoolean},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "already exists")
}

// With describe emitting `create or modify javascript action`, its output now
// re-executes (#731). Where describe could not read the .js source it prints a
// placeholder body; that is not code, and writing it would replace the real
// source (or add a stub file to a project that ships none). The Java twin is
// TestJavaActionBody_PlaceholderIsNotWritten.
func TestJavaScriptActionBody_PlaceholderIsNotWritten(t *testing.T) {
	mod := mkModule("WebMod")
	h := mkHierarchy(mod)
	wrote := false
	mb := &mock.MockBackend{
		IsConnectedFunc:            func() bool { return true },
		ListModulesFunc:            func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListJavaScriptActionsFunc:  func() ([]*types.JavaScriptAction, error) { return nil, nil },
		CreateJavaScriptActionFunc: func(*types.JavaScriptAction) error { return nil },
		WriteJavaScriptSourceFileFunc: func(string, string, string, []*types.JavaActionParameter, types.CodeActionReturnType) error {
			wrote = true
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	stmt := func(code string) *ast.CreateJavaScriptActionStmt {
		return &ast.CreateJavaScriptActionStmt{
			Name:           ast.QualifiedName{Module: "WebMod", Name: "DoThing"},
			ReturnType:     ast.DataType{Kind: ast.TypeBoolean},
			JavaScriptCode: code,
		}
	}

	assertNoError(t, execCreateJavaScriptAction(ctx, stmt("\n"+jsSourceOmittedBody+"\n")))
	if wrote {
		t.Error("the DESCRIBE placeholder body was written as the action's JavaScript source")
	}

	// Control: an authored body is still written.
	assertNoError(t, execCreateJavaScriptAction(ctx, stmt("return Promise.resolve(true);")))
	if !wrote {
		t.Error("an authored body was not written — the guard is too broad")
	}
}

// A parameter's Description and Category have no MDL spelling — describe
// prints the description as a `--` comment — so `create or modify` carries them
// from the stored parameter of the same name, as the Java twin does. Dropping
// them made running unchanged describe output rewrite the action (#731).
func TestJavaScriptActionRewrite_CarriesParameterDescription(t *testing.T) {
	mod := mkModule("WebMod")
	h := mkHierarchy(mod)
	stored := &types.JavaScriptAction{
		BaseElement: model.BaseElement{ID: nextID("jsa")},
		ContainerID: mod.ID,
		Name:        "Wait",
		Parameters: []*types.JavaActionParameter{{
			Name:          "Delay",
			Description:   "The number of milliseconds to wait.",
			Category:      "Timing",
			IsRequired:    true,
			ParameterType: &types.IntegerType{},
		}},
	}
	withContainer(h, stored.ContainerID, mod.ID)
	var captured *types.JavaScriptAction
	mb := &mock.MockBackend{
		IsConnectedFunc:           func() bool { return true },
		ListModulesFunc:           func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListJavaScriptActionsFunc: func() ([]*types.JavaScriptAction, error) { return []*types.JavaScriptAction{stored}, nil },
		ReadJavaScriptActionByNameFunc: func(string) (*types.JavaScriptAction, error) {
			return stored, nil
		},
		UpdateJavaScriptActionFunc: func(jsa *types.JavaScriptAction) error {
			captured = jsa
			return nil
		},
		WriteJavaScriptSourceFileFunc: func(string, string, string, []*types.JavaActionParameter, types.CodeActionReturnType) error {
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execCreateJavaScriptAction(ctx, &ast.CreateJavaScriptActionStmt{
		Name:           ast.QualifiedName{Module: "WebMod", Name: "Wait"},
		CreateOrModify: true,
		Parameters: []ast.JavaActionParam{
			{Name: "Delay", Type: ast.DataType{Kind: ast.TypeInteger}, IsRequired: true},
			{Name: "Fresh", Type: ast.DataType{Kind: ast.TypeString}},
		},
		ReturnType:     ast.DataType{Kind: ast.TypeVoid},
		JavaScriptCode: "\n" + jsSourceOmittedBody + "\n",
	}))
	if captured == nil || len(captured.Parameters) != 2 {
		t.Fatalf("UpdateJavaScriptAction not called with two parameters: %+v", captured)
	}
	if p := captured.Parameters[0]; p.Description != "The number of milliseconds to wait." || p.Category != "Timing" {
		t.Errorf("Delay: description %q, category %q — not carried from the stored parameter", p.Description, p.Category)
	}
	// Control: a parameter with no stored twin gets none.
	if p := captured.Parameters[1]; p.Description != "" || p.Category != "" {
		t.Errorf("Fresh: description %q, category %q — a new parameter inherited a description", p.Description, p.Category)
	}
}

// Only the identifiers in the generated .js are lower-cased, the way mxbuild
// regenerates them. The parameter names in the model are what the author
// declared, so describe prints them unchanged and re-running its output is
// stable.
func TestCreateJavaScriptAction_ModelKeepsDeclaredParameterCase(t *testing.T) {
	mod := mkModule("WebMod")
	h := mkHierarchy(mod)

	var created *types.JavaScriptAction
	var written []*types.JavaActionParameter
	mb := &mock.MockBackend{
		IsConnectedFunc:           func() bool { return true },
		ListModulesFunc:           func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListJavaScriptActionsFunc: func() ([]*types.JavaScriptAction, error) { return nil, nil },
		CreateJavaScriptActionFunc: func(jsa *types.JavaScriptAction) error {
			created = jsa
			return nil
		},
		WriteJavaScriptSourceFileFunc: func(_, _, _ string, params []*types.JavaActionParameter, _ types.CodeActionReturnType) error {
			written = params
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execCreateJavaScriptAction(ctx, &ast.CreateJavaScriptActionStmt{
		Name: ast.QualifiedName{Module: "WebMod", Name: "JS_Open"},
		Parameters: []ast.JavaActionParam{
			{Name: "Url", Type: ast.DataType{Kind: ast.TypeString}},
			{Name: "ReadOnly", Type: ast.DataType{Kind: ast.TypeBoolean}},
		},
		ReturnType:     ast.DataType{Kind: ast.TypeString},
		JavaScriptCode: "return url;",
	}))

	if created == nil || len(created.Parameters) != 2 {
		t.Fatalf("action = %+v", created)
	}
	for i, want := range []string{"Url", "ReadOnly"} {
		if created.Parameters[i].Name != want {
			t.Errorf("model parameter %d = %q, want %q", i, created.Parameters[i].Name, want)
		}
		if len(written) == 2 && written[i].Name != want {
			t.Errorf("parameter %d handed to the .js writer = %q, want the model name %q", i, written[i].Name, want)
		}
	}
}
