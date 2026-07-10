// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// forwardRefCtx returns an ExecContext connected to a project with no existing
// pages, so every page reference must resolve within the script.
func forwardRefCtx(t *testing.T) *ExecContext {
	t.Helper()
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return nil, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	return ctx
}

func TestValidateForwardPageRefs_ForwardFails(t *testing.T) {
	src := `create page M.Overview (title: 'O', layout: Atlas_Core.Atlas_Default) {
  actionbutton b (caption: 'D', action: show_page M.Detail)
}
create page M.Detail (title: 'D', layout: Atlas_Core.Atlas_Default) {
  dynamictext dt (content: 'x')
}`
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	got := validateForwardPageRefs(forwardRefCtx(t), prog)
	if len(got) == 0 {
		t.Fatal("expected a forward-reference error, got none")
	}
	if !strings.Contains(got[0].Error(), "M.Detail") || !strings.Contains(got[0].Error(), "before it is created") {
		t.Errorf("unexpected error: %v", got[0])
	}
}

// TestValidateForwardPageRefs_DuplicateRefsReportedOnce guards against emitting
// the same diagnostic twice when one page references the same forward page from
// more than one widget (e.g. an overview page with both a New button and a
// per-row Edit button pointing at the edit page).
func TestValidateForwardPageRefs_DuplicateRefsReportedOnce(t *testing.T) {
	src := `create page M.Overview (title: 'O', layout: Atlas_Core.Atlas_Default) {
  actionbutton newBtn (caption: 'New', action: show_page M.NewEdit)
  actionbutton editBtn (caption: 'Edit', action: show_page M.NewEdit)
}
create page M.NewEdit (title: 'E', layout: Atlas_Core.Atlas_Default) {
  dynamictext dt (content: 'x')
}`
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	got := validateForwardPageRefs(forwardRefCtx(t), prog)
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 forward-reference error, got %d: %v", len(got), got)
	}
	if !strings.Contains(got[0].Error(), "M.NewEdit") || !strings.Contains(got[0].Error(), "before it is created") {
		t.Errorf("unexpected error: %v", got[0])
	}
}

func TestValidateForwardPageRefs_CorrectOrderPasses(t *testing.T) {
	src := `create page M.Detail (title: 'D', layout: Atlas_Core.Atlas_Default) {
  dynamictext dt (content: 'x')
}
create page M.Overview (title: 'O', layout: Atlas_Core.Atlas_Default) {
  actionbutton b (caption: 'D', action: show_page M.Detail)
}`
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	if got := validateForwardPageRefs(forwardRefCtx(t), prog); len(got) != 0 {
		t.Errorf("expected no errors for correct ordering, got: %v", got)
	}
}
