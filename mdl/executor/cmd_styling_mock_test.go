// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// ---------------------------------------------------------------------------
// execShowDesignProperties
// ---------------------------------------------------------------------------

func TestShowDesignProperties_NotConnected(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execShowDesignProperties(ctx, &ast.ShowDesignPropertiesStmt{})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not connected")
}

func TestShowDesignProperties_NoMprPath(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	ctx.MprPath = ""
	err := execShowDesignProperties(ctx, &ast.ShowDesignPropertiesStmt{})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "project path")
}

// NOTE: execShowDesignProperties happy path requires loadThemeRegistry which
// reads design-properties.json from the filesystem. Would need a temp dir with
// a valid theme structure to test. Tracked separately.

// ---------------------------------------------------------------------------
// execDescribeStyling
// ---------------------------------------------------------------------------

func TestDescribeStyling_NotConnected(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execDescribeStyling(ctx, &ast.DescribeStylingStmt{
		ContainerType: "page",
		ContainerName: ast.QualifiedName{Module: "Mod", Name: "Home"},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not connected")
}

// NOTE: execDescribeStyling happy path calls getPageWidgetsFromRaw /
// getSnippetWidgetsFromRaw which use ctx.Backend.GetRawUnit for BSON parsing.
// MockBackend has GetRawUnitFunc but producing valid BSON test data for the
// page widget walker is non-trivial. Tracked separately.

// ---------------------------------------------------------------------------
// execAlterStyling
// ---------------------------------------------------------------------------

func TestAlterStyling_NotConnected(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execAlterStyling(ctx, &ast.AlterStylingStmt{
		ContainerType: "page",
		ContainerName: ast.QualifiedName{Module: "Mod", Name: "Home"},
		WidgetName:    "container1",
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not connected")
}

// stylingMutatorBackend wires a mock backend whose OpenPageForMutation returns
// the supplied mutator, with a page "MyModule.Home" resolvable.
func stylingMutatorBackend(t *testing.T, mut *mock.MockPageMutator) (*ExecContext, *mock.MockBackend, func() string) {
	t.Helper()
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "Home")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return mut, nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, mb, buf.String
}

// Issue #631 — visitor sets ContainerType to uppercase "PAGE"; the executor must
// normalise before comparing, then go through the page mutator (not the old
// reflection walker) so it works on builder-created pages too.
func TestAlterStyling_UppercaseContainerType_SetClass_Issue631(t *testing.T) {
	saved := false
	var gotProp, gotValue string
	mut := &mock.MockPageMutator{
		FindWidgetFunc: func(name string) bool { return name == "ctnHeader" },
		SetWidgetPropertyFunc: func(widgetRef, prop string, value any) error {
			gotProp = prop
			gotValue, _ = value.(string)
			return nil
		},
		SaveFunc: func() error { saved = true; return nil },
	}
	ctx, _, out := stylingMutatorBackend(t, mut)

	assertNoError(t, execAlterStyling(ctx, &ast.AlterStylingStmt{
		ContainerType: "PAGE", // uppercase, as produced by the visitor
		ContainerName: ast.QualifiedName{Module: "MyModule", Name: "Home"},
		WidgetName:    "ctnHeader",
		Assignments: []ast.StylingAssignment{
			{Property: "Class", Value: "card card-bordered", IsCSS: true},
		},
	}))

	if gotProp != "Class" || gotValue != "card card-bordered" {
		t.Errorf("expected Class='card card-bordered', got %s=%q", gotProp, gotValue)
	}
	if !saved {
		t.Error("expected Save to be called")
	}
	assertContainsStr(t, out(), "Updated styling on widget \"ctnHeader\"")
}

func TestAlterStyling_DesignProperties_ToggleAndOption(t *testing.T) {
	type call struct {
		key, valueType, option string
	}
	var calls []call
	mut := &mock.MockPageMutator{
		FindWidgetFunc: func(name string) bool { return true },
		SetDesignPropertyFunc: func(widgetRef, key, valueType, option string) error {
			calls = append(calls, call{key, valueType, option})
			return nil
		},
		SaveFunc: func() error { return nil },
	}
	ctx, _, _ := stylingMutatorBackend(t, mut)

	assertNoError(t, execAlterStyling(ctx, &ast.AlterStylingStmt{
		ContainerType: "PAGE",
		ContainerName: ast.QualifiedName{Module: "MyModule", Name: "Home"},
		WidgetName:    "ctnHeader",
		Assignments: []ast.StylingAssignment{
			{Property: "Full width", IsToggle: true, ToggleOn: true},
			{Property: "Spacing bottom", Value: "Large"},
		},
	}))

	if len(calls) != 2 {
		t.Fatalf("expected 2 SetDesignProperty calls, got %d", len(calls))
	}
	if calls[0] != (call{"Full width", "toggle", ""}) {
		t.Errorf("toggle call wrong: %+v", calls[0])
	}
	if calls[1] != (call{"Spacing bottom", "option", "Large"}) {
		t.Errorf("option call wrong: %+v", calls[1])
	}
}

func TestAlterStyling_ToggleOff_RemovesDesignProperty(t *testing.T) {
	var removed string
	mut := &mock.MockPageMutator{
		FindWidgetFunc:           func(name string) bool { return true },
		RemoveDesignPropertyFunc: func(widgetRef, key string) error { removed = key; return nil },
		SaveFunc:                 func() error { return nil },
	}
	ctx, _, _ := stylingMutatorBackend(t, mut)

	assertNoError(t, execAlterStyling(ctx, &ast.AlterStylingStmt{
		ContainerType: "PAGE",
		ContainerName: ast.QualifiedName{Module: "MyModule", Name: "Home"},
		WidgetName:    "ctnHeader",
		Assignments: []ast.StylingAssignment{
			{Property: "Full width", IsToggle: true, ToggleOn: false},
		},
	}))

	if removed != "Full width" {
		t.Errorf("expected RemoveDesignProperty('Full width'), got %q", removed)
	}
}

func TestAlterStyling_ClearDesignProperties(t *testing.T) {
	cleared := false
	mut := &mock.MockPageMutator{
		FindWidgetFunc:            func(name string) bool { return true },
		ClearDesignPropertiesFunc: func(widgetRef string) error { cleared = true; return nil },
		SaveFunc:                  func() error { return nil },
	}
	ctx, _, _ := stylingMutatorBackend(t, mut)

	assertNoError(t, execAlterStyling(ctx, &ast.AlterStylingStmt{
		ContainerType:    "PAGE",
		ContainerName:    ast.QualifiedName{Module: "MyModule", Name: "Home"},
		WidgetName:       "ctnHeader",
		ClearDesignProps: true,
	}))

	if !cleared {
		t.Error("expected ClearDesignProperties to be called")
	}
}

// Issue #631 follow-up: a quoted design-property key named 'Style' must be
// treated as a design property (e.g. Button Style = 'Icon button'), NOT routed
// to the CSS Style widget property. IsCSS is false for quoted keys.
func TestAlterStyling_QuotedStyleIsDesignProperty(t *testing.T) {
	var dpKey, dpVal string
	cssCalled := false
	mut := &mock.MockPageMutator{
		FindWidgetFunc:        func(name string) bool { return true },
		SetWidgetPropertyFunc: func(widgetRef, prop string, value any) error { cssCalled = true; return nil },
		SetDesignPropertyFunc: func(widgetRef, key, valueType, option string) error { dpKey = key; dpVal = option; return nil },
		SaveFunc:              func() error { return nil },
	}
	ctx, _, _ := stylingMutatorBackend(t, mut)

	assertNoError(t, execAlterStyling(ctx, &ast.AlterStylingStmt{
		ContainerType: "PAGE",
		ContainerName: ast.QualifiedName{Module: "MyModule", Name: "Home"},
		WidgetName:    "btnSave",
		Assignments: []ast.StylingAssignment{
			{Property: "Style", Value: "Icon button", IsCSS: false}, // quoted 'Style' design property
		},
	}))

	if cssCalled {
		t.Error("quoted 'Style' must NOT be routed to the CSS Style property")
	}
	if dpKey != "Style" || dpVal != "Icon button" {
		t.Errorf("expected design property Style='Icon button', got %s=%q", dpKey, dpVal)
	}
}

func TestAlterStyling_WidgetNotFound(t *testing.T) {
	mut := &mock.MockPageMutator{
		FindWidgetFunc: func(name string) bool { return false },
	}
	ctx, _, _ := stylingMutatorBackend(t, mut)

	err := execAlterStyling(ctx, &ast.AlterStylingStmt{
		ContainerType: "PAGE",
		ContainerName: ast.QualifiedName{Module: "MyModule", Name: "Home"},
		WidgetName:    "ghost",
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not found")
}
