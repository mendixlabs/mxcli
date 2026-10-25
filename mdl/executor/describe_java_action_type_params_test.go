// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1404: a Java action parameter typed by a type parameter that
// no `entity <T>` parameter declares — GenAICommons'
// ChunkCollection_AddKnowledgeBaseChunk, `MxObject: "Entity"`. MDL declared a
// type parameter only through an entity selector, so describe printed the
// reference with no declaration and exec wrote it as the entity `.Entity`
// (CE1613 "The selected entity '.Entity' no longer exists"), while check passed.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// describeJA describes one mock Java action and returns the output.
func describeJA(t *testing.T, ja *javaactions.JavaAction) string {
	t.Helper()
	mod := mkModule("GenAICommons")
	ja.BaseElement = model.BaseElement{ID: nextID("ja")}
	ja.ContainerID = mod.ID
	mb := &mock.MockBackend{
		IsConnectedFunc:          func() bool { return true },
		ReadJavaActionByNameFunc: func(string) (*javaactions.JavaAction, error) { return ja, nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeJavaAction(ctx, ast.QualifiedName{Module: "GenAICommons", Name: ja.Name}))
	return buf.String()
}

// createJACtx wires an empty project for a new Java action and captures what
// exec hands to CreateJavaAction.
func createJACtx(t *testing.T) (*ExecContext, **javaactions.JavaAction) {
	t.Helper()
	mod := mkModule("GenAICommons")
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
	return ctx, &captured
}

// The reported action: describe declares the type parameter in the header, and
// exec of that output binds MxObject to it rather than to an entity `.Entity`.
func TestJavaAction_UndeclaredTypeParameterRoundTrips(t *testing.T) {
	tp := &types.TypeParameterDef{BaseElement: model.BaseElement{ID: nextID("tp")}, Name: "Entity"}
	out := describeJA(t, &javaactions.JavaAction{
		Name:           "ChunkCollection_AddKnowledgeBaseChunk",
		TypeParameters: []*types.TypeParameterDef{tp},
		Parameters: []*javaactions.JavaActionParameter{
			{Name: "HumanReadableID", IsRequired: true, ParameterType: &javaactions.StringType{}},
			{Name: "MxObject", ParameterType: &javaactions.TypeParameter{TypeParameterID: tp.ID, TypeParameter: "Entity"}},
		},
		ReturnType: &javaactions.BooleanType{},
	})
	if strings.Contains(out, "NOT re-executable") {
		t.Fatalf("an expressible action is marked NOT re-executable:\n%s", out)
	}
	stmt := findStmt[*ast.CreateJavaActionStmt](t, reparse(t, out), out)
	if strings.Join(stmt.TypeParameters, ",") != "Entity" {
		t.Fatalf("re-parsed type parameters %q, want [Entity]\n%s", stmt.TypeParameters, out)
	}

	ctx, captured := createJACtx(t)
	assertNoError(t, execCreateJavaAction(ctx, stmt))
	got := *captured
	if got == nil || len(got.TypeParameters) != 1 {
		t.Fatalf("type parameters not written: %+v", got)
	}
	p, ok := got.Parameters[1].ParameterType.(*javaactions.TypeParameter)
	if !ok || p.TypeParameterID != got.TypeParameters[0].ID {
		t.Errorf("MxObject written as %#v, want type parameter %s", got.Parameters[1].ParameterType, got.TypeParameters[0].ID)
	}
}

// The stored order is part of the action: when the entity selectors would
// declare the type parameters in another order, the header restates them all.
func TestJavaAction_TypeParameterOrderIsKept(t *testing.T) {
	a := &types.TypeParameterDef{BaseElement: model.BaseElement{ID: nextID("tp")}, Name: "A"}
	b := &types.TypeParameterDef{BaseElement: model.BaseElement{ID: nextID("tp")}, Name: "B"}
	out := describeJA(t, &javaactions.JavaAction{
		Name:           "Pair",
		TypeParameters: []*types.TypeParameterDef{b, a},
		Parameters: []*javaactions.JavaActionParameter{
			{Name: "SelA", ParameterType: &javaactions.EntityTypeParameterType{TypeParameterID: a.ID, TypeParameterName: "A"}},
			{Name: "SelB", ParameterType: &javaactions.EntityTypeParameterType{TypeParameterID: b.ID, TypeParameterName: "B"}},
		},
		ReturnType: &javaactions.BooleanType{},
	})
	stmt := findStmt[*ast.CreateJavaActionStmt](t, reparse(t, out), out)
	if strings.Join(stmt.TypeParameters, ",") != "B,A" {
		t.Errorf("re-parsed type parameters %q, want [B A]\n%s", stmt.TypeParameters, out)
	}
}

// An entity selector bound to no type parameter has no MDL form (it printed
// `entity <>`, which does not parse): commented out, the script still parses
// and re-running it leaves the action alone.
func TestJavaAction_UnboundEntitySelectorIsCommentedOut(t *testing.T) {
	out := describeJA(t, &javaactions.JavaAction{
		Name: "Refresh",
		Parameters: []*javaactions.JavaActionParameter{
			{Name: "EntityToRefresh", ParameterType: &javaactions.EntityTypeParameterType{}},
		},
		ReturnType: &javaactions.BooleanType{},
	})
	for _, s := range reparse(t, out).Statements {
		if _, ok := s.(*ast.CreateJavaActionStmt); ok {
			t.Errorf("an inexpressible action printed a live statement:\n%s", out)
		}
	}
	if !strings.Contains(out, "-- NOT re-executable:") {
		t.Errorf("no NOT re-executable note:\n%s", out)
	}
}

// The reduced repro: `MxObject: "Entity"` with nothing declaring Entity. It is
// neither a qualified entity nor a declared type parameter, so check and exec
// refuse it rather than write the entity `.Entity`.
func TestJavaAction_UndeclaredUnqualifiedTypeIsRefused(t *testing.T) {
	src := "create or modify java action GenAICommons.JA_A(\n  MxObject: \"Entity\"\n) returns Boolean\nas $$\nreturn true;\n$$;"
	prog := reparse(t, src)
	stmt := findStmt[*ast.CreateJavaActionStmt](t, prog, src)

	var errs []linter.Violation
	for _, v := range ValidateProgram(prog, "") {
		if v.Severity == linter.SeverityError && v.RuleID == "MDL-JA01" {
			errs = append(errs, v)
		}
	}
	if len(errs) != 1 {
		t.Errorf("check: got %d MDL-JA01 errors, want 1: %+v", len(errs), errs)
	}

	ctx, captured := createJACtx(t)
	if err := execCreateJavaAction(ctx, stmt); err == nil {
		t.Errorf("exec accepted it and wrote %#v", (*captured).Parameters[0].ParameterType)
	}

	// Control: declared in the header, the same parameter is accepted.
	ok := "create or modify java action GenAICommons.JA_A<\"Entity\">(\n  MxObject: \"Entity\"\n) returns Boolean\nas $$\nreturn true;\n$$;"
	okProg := reparse(t, ok)
	for _, v := range ValidateProgram(okProg, "") {
		if v.RuleID == "MDL-JA01" {
			t.Errorf("declared type parameter reported: %+v", v)
		}
	}
	ctx2, _ := createJACtx(t)
	assertNoError(t, execCreateJavaAction(ctx2, findStmt[*ast.CreateJavaActionStmt](t, okProg, ok)))
}
