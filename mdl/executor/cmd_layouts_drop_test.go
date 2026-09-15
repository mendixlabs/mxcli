// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Layouts were the only document mxcli could create and alter but not delete
// (mendixlabs/mxcli#1063), which bit hardest when a layout mxcli had just
// written turned out not to build.
func TestExecDropLayout(t *testing.T) {
	ctx, mb, deleted := dropLayoutCtx(t, nil)
	err := execDropLayout(ctx, &ast.DropLayoutStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "App_Mine"},
	})
	if err != nil {
		t.Fatalf("execDropLayout: %v", err)
	}
	if *deleted != model.ID("lay-1") {
		t.Errorf("deleted %q, want lay-1", *deleted)
	}
	if out := ctx.Output.(*strings.Builder).String(); !strings.Contains(out, "Dropped layout MyModule.App_Mine") {
		t.Errorf("output %q does not confirm the drop", out)
	}
	_ = mb
}

// Pages still bound to the layout are WARNED about, not refused — mxcli's other
// DROPs do not refuse on dependents either, and drop-then-recreate under the
// same name is how a layout is corrected (verified end to end on 11.12.1: the
// pages go back to 0 errors). But the consequence of leaving it dropped is
// CE1613 on the PAGE, which names no layout at all, so the drop must not be
// silent.
func TestExecDropLayout_WarnsAboutPagesStillUsingIt(t *testing.T) {
	ctx, _, _ := dropLayoutCtx(t, map[model.ID]string{
		model.ID("pg-1"): "MyModule.App_Mine",
		model.ID("pg-2"): "MyModule.App_Mine",
		model.ID("pg-3"): "Atlas_Core.Atlas_Default", // on another layout: must not appear
	})
	if err := execDropLayout(ctx, &ast.DropLayoutStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "App_Mine"},
	}); err != nil {
		t.Fatalf("execDropLayout: %v", err)
	}

	out := ctx.Output.(*strings.Builder).String()
	if !strings.Contains(out, "Warning:") || !strings.Contains(out, "CE1613") {
		t.Errorf("output %q does not warn about the pages", out)
	}
	for _, want := range []string{"2 page(s)", "MyModule.Home", "MyModule.Detail"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q does not mention %q", out, want)
		}
	}
	// A page on a DIFFERENT layout is unaffected and naming it would make the
	// warning untrustworthy.
	if strings.Contains(out, "MyModule.Other") {
		t.Errorf("output %q names a page bound to another layout", out)
	}
	// It is still a drop, not a refusal.
	if !strings.Contains(out, "Dropped layout") {
		t.Errorf("output %q refused the drop instead of warning", out)
	}
}

// CONTROL: no dependents, no warning — otherwise the warning is noise that gets
// ignored on the run where it matters.
func TestExecDropLayout_QuietWhenNothingUsesIt(t *testing.T) {
	ctx, _, _ := dropLayoutCtx(t, nil)
	if err := execDropLayout(ctx, &ast.DropLayoutStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "App_Mine"},
	}); err != nil {
		t.Fatalf("execDropLayout: %v", err)
	}
	if out := ctx.Output.(*strings.Builder).String(); strings.Contains(out, "Warning:") {
		t.Errorf("an unused layout produced a warning: %q", out)
	}
}

func TestExecDropLayout_UnknownLayout(t *testing.T) {
	ctx, _, _ := dropLayoutCtx(t, nil)
	err := execDropLayout(ctx, &ast.DropLayoutStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "NoSuch"},
	})
	if err == nil || !strings.Contains(err.Error(), "NoSuch") {
		t.Fatalf("err = %v, want a not-found naming the layout", err)
	}
}

// dropLayoutCtx wires a project with one layout (MyModule.App_Mine) and three
// pages, each bound to whatever pageLayouts says.
func dropLayoutCtx(t *testing.T, pageLayouts map[model.ID]string) (*ExecContext, *mock.MockBackend, *model.ID) {
	t.Helper()
	mod := &model.Module{
		BaseElement: model.BaseElement{ID: model.ID("mod-own")},
		Name:        "MyModule",
	}
	lay := &pages.Layout{Name: "App_Mine"}
	lay.ID = model.ID("lay-1")
	lay.ContainerID = mod.ID

	mkPage := func(id, name string) *pages.Page {
		p := &pages.Page{Name: name}
		p.ID = model.ID(id)
		p.ContainerID = mod.ID
		return p
	}
	allPages := []*pages.Page{
		mkPage("pg-1", "Home"), mkPage("pg-2", "Detail"), mkPage("pg-3", "Other"),
	}

	var deleted model.ID
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListLayoutsFunc: func() ([]*pages.Layout, error) { return []*pages.Layout{lay}, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return allPages, nil },
		PageLayoutNameFunc: func(id model.ID) (string, error) {
			return pageLayouts[id], nil
		},
		DeleteLayoutFunc: func(id model.ID) error { deleted = id; return nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	ctx.Output = &strings.Builder{}
	return ctx, mb, &deleted
}
