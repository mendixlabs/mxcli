// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
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
	assertContainsStr(t, out, "create javascript action")
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
