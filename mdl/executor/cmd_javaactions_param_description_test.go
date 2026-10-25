// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// mendixlabs/mxcli#1588. `create java action` had no way to set a parameter's
// description or the return value's name. `describe` printed both as `--`
// comments (`Input: String not null  -- The text to convert.` and `-- return
// NAME: 'UpperText'`), which read like syntax: written in a script they passed
// check and exec and were dropped, the description left empty and the return
// name the default `ReturnValueName`.
//
// They are now spelled the way the language already spells the same two things
// elsewhere — a parameter's description is a `/** … */` doc comment before it,
// as on an entity attribute, and the return value's name is `returns T as
// $Name`, as on a microflow — and describe prints those forms, so a describe →
// exec round trip carries them. Both forms are shared with JavaScript actions.

const toUpperScript = `create java action MyModule.ToUpper(
    /** The text to convert. */
    Input: String not null,
    /**
     * How to convert it.
     * Two lines, both kept.
     */
    Mode: String
) returns String as $UpperText
as $$
return Input.toUpperCase();
$$;`

func parseOne[T ast.Statement](t *testing.T, src string) T {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs, src)
	}
	return findStmt[T](t, prog, src)
}

func TestCreateJavaAction_WritesStatedParameterDescriptionAndReturnName(t *testing.T) {
	s := parseOne[*ast.CreateJavaActionStmt](t, toUpperScript)

	mod := mkModule("MyModule")
	var captured *javaactions.JavaAction
	mb := &mock.MockBackend{
		IsConnectedFunc:     func() bool { return true },
		ListModulesFunc:     func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListJavaActionsFunc: func() ([]*types.JavaAction, error) { return nil, nil },
		CreateJavaActionFunc: func(ja *javaactions.JavaAction) error {
			captured = ja
			return nil
		},
		WriteJavaSourceFileFunc: func(string, string, string, []*javaactions.JavaActionParameter,
			javaactions.CodeActionReturnType, []string, string, string) error {
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	assertNoError(t, execCreateJavaAction(ctx, s))

	if captured == nil || len(captured.Parameters) != 2 {
		t.Fatalf("CreateJavaAction not called with two parameters: %+v", captured)
	}
	if got := captured.Parameters[0].Description; got != "The text to convert." {
		t.Errorf("Input description = %q, want %q", got, "The text to convert.")
	}
	if got, want := captured.Parameters[1].Description, "How to convert it.\nTwo lines, both kept."; got != want {
		t.Errorf("Mode description = %q, want %q", got, want)
	}
	if captured.ActionDefaultReturnName != "UpperText" {
		t.Errorf("return value name = %q, want UpperText — the default is ReturnValueName, "+
			"which is what the report saw", captured.ActionDefaultReturnName)
	}
}

// A stated value replaces the stored one; an unstated one is still carried
// (TestJavaActionRewriteCarriesParameterDescriptionAndCategory), so the
// existing preserve rule and the new spelling do not fight.
func TestJavaActionRewrite_StatedDescriptionAndReturnNameReplaceStored(t *testing.T) {
	mod := mkModule("MyModule")
	stored := storedExposedAction(mod)
	stored.ActionDefaultReturnName = "Old"
	stored.Parameters = []*javaactions.JavaActionParameter{
		{Name: "Input", Description: "old text", Category: "Kept"},
		{Name: "Other", Description: "kept, the statement does not say"},
	}
	ctx, captured := exposeRewriteCtx(t, stored, mod)

	s := rewriteStmt(false)
	s.ReturnType = ast.DataType{Kind: ast.TypeString}
	s.ReturnName = "New"
	s.Parameters = []ast.JavaActionParam{
		{Name: "Input", Type: ast.DataType{Kind: ast.TypeString}, Description: "new text", DescriptionSet: true},
		{Name: "Other", Type: ast.DataType{Kind: ast.TypeString}},
	}
	assertNoError(t, execCreateJavaAction(ctx, s))

	got := *captured
	if got == nil {
		t.Fatal("UpdateJavaAction was never called")
	}
	if p := got.Parameters[0]; p.Description != "new text" || p.Category != "Kept" {
		t.Errorf("Input: description %q, category %q — want the stated description and the carried category", p.Description, p.Category)
	}
	if p := got.Parameters[1]; p.Description != "kept, the statement does not say" {
		t.Errorf("Other: description %q — an unstated description must be carried", p.Description)
	}
	if got.ActionDefaultReturnName != "New" {
		t.Errorf("return value name = %q, want the stated New", got.ActionDefaultReturnName)
	}
}

// Describe prints the forms create reads, so its output re-executes into the
// same values — including a description of several lines, of which the `--`
// form kept only the first.
func TestDescribeJavaAction_DescriptionAndReturnNameRoundTrip(t *testing.T) {
	mod := mkModule("MyModule")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ReadJavaActionByNameFunc: func(string) (*javaactions.JavaAction, error) {
			return &javaactions.JavaAction{
				BaseElement:             model.BaseElement{ID: nextID("ja")},
				ContainerID:             mod.ID,
				Name:                    "ToUpper",
				ActionDefaultReturnName: "UpperText",
				ReturnType:              &types.StringType{},
				Parameters:              describedParams(),
			}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeJavaAction(ctx, ast.QualifiedName{Module: "MyModule", Name: "ToUpper"}))
	out := buf.String()
	stmt := findStmt[*ast.CreateJavaActionStmt](t, reparse(t, out), out)
	assertDescribedForms(t, out, stmt.Parameters, stmt.ReturnName)
}

func TestCreateJavaScriptAction_WritesStatedParameterDescriptionAndReturnName(t *testing.T) {
	s := parseOne[*ast.CreateJavaScriptActionStmt](t,
		strings.Replace(toUpperScript, "java action", "javascript action", 1))

	mod := mkModule("MyModule")
	var captured *types.JavaScriptAction
	mb := &mock.MockBackend{
		IsConnectedFunc:           func() bool { return true },
		ListModulesFunc:           func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListJavaScriptActionsFunc: func() ([]*types.JavaScriptAction, error) { return nil, nil },
		CreateJavaScriptActionFunc: func(jsa *types.JavaScriptAction) error {
			captured = jsa
			return nil
		},
		WriteJavaScriptSourceFileFunc: func(string, string, string, []*types.JavaActionParameter, types.CodeActionReturnType) error {
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	assertNoError(t, execCreateJavaScriptAction(ctx, s))

	if captured == nil || len(captured.Parameters) != 2 {
		t.Fatalf("CreateJavaScriptAction not called with two parameters: %+v", captured)
	}
	if got := captured.Parameters[0].Description; got != "The text to convert." {
		t.Errorf("Input description = %q", got)
	}
	if got, want := captured.Parameters[1].Description, "How to convert it.\nTwo lines, both kept."; got != want {
		t.Errorf("Mode description = %q, want %q", got, want)
	}
	if captured.ActionDefaultReturnName != "UpperText" {
		t.Errorf("return value name = %q, want UpperText", captured.ActionDefaultReturnName)
	}
}

func TestDescribeJavaScriptAction_DescriptionAndReturnNameRoundTrip(t *testing.T) {
	mod := mkModule("WebMod")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ReadJavaScriptActionByNameFunc: func(string) (*types.JavaScriptAction, error) {
			return &types.JavaScriptAction{
				BaseElement:             model.BaseElement{ID: nextID("jsa")},
				ContainerID:             mod.ID,
				Name:                    "ToUpper",
				Platform:                "Web",
				ActionDefaultReturnName: "UpperText",
				ReturnType:              &types.StringType{},
				Parameters:              describedParams(),
			}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeJavaScriptAction(ctx, ast.QualifiedName{Module: "WebMod", Name: "ToUpper"}))
	out := buf.String()
	stmt := findStmt[*ast.CreateJavaScriptActionStmt](t, reparse(t, out), out)
	assertDescribedForms(t, out, stmt.Parameters, stmt.ReturnName)
}

// describedParams covers the description shapes describe has to print: one
// line, several lines (CRLF, as Studio Pro stores it), a line that starts with
// `*` (which the doc-comment gutter would eat inline), one holding `*/` (which
// would end the doc comment early, so it cannot be one), and none.
func describedParams() []*types.JavaActionParameter {
	return []*types.JavaActionParameter{
		{Name: "Input", Description: "The text to convert.", IsRequired: true, ParameterType: &types.StringType{}},
		{Name: "Mode", Description: "How to convert it.\r\nTwo lines, both kept.", ParameterType: &types.StringType{}},
		{Name: "Bullet", Description: "* a bullet", ParameterType: &types.StringType{}},
		{Name: "Glob", Description: "Files matching /tmp/*/ or */logs", ParameterType: &types.StringType{}},
		{Name: "Plain", ParameterType: &types.StringType{}},
	}
}

func assertDescribedForms(t *testing.T, out string, params []ast.JavaActionParam, returnName string) {
	t.Helper()
	if strings.Contains(out, "-- return NAME") || strings.Contains(out, "  -- The text") {
		t.Errorf("describe still prints a value as a `--` comment, which create drops:\n%s", out)
	}
	want := []struct {
		name, desc string
		set        bool
	}{
		{"Input", "The text to convert.", true},
		{"Mode", "How to convert it.\nTwo lines, both kept.", true},
		{"Bullet", "* a bullet", true},
		// Not stated, so a rewrite carries the stored description — and the
		// output still parses, which a doc comment holding `*/` would not.
		{"Glob", "", false},
		{"Plain", "", false},
	}
	if len(params) != len(want) {
		t.Fatalf("re-parsed %d parameters, want %d\n%s", len(params), len(want), out)
	}
	for i, w := range want {
		p := params[i]
		if p.Name != w.name || p.Description != w.desc || p.DescriptionSet != w.set {
			t.Errorf("parameter %d re-parsed as %q description %q (set %v), want %q %q (set %v)\n%s",
				i, p.Name, p.Description, p.DescriptionSet, w.name, w.desc, w.set, out)
		}
	}
	if returnName != "UpperText" {
		t.Errorf("return value name re-parsed as %q, want UpperText\n%s", returnName, out)
	}
}
