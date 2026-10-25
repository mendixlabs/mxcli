// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// "Expose as microflow action" is a six-field sub-document — Caption, Category
// and four PNG bitmaps (icon and image, each with a dark-mode variant). MDL can
// only write the first two, so a rewrite that BUILDS the sub-document from the
// clause discards the other four.
//
// Measured on 11.13.0 before this fix: with the clause kept, a Studio
// Pro-authored icon and image came back as empty binaries; with the clause
// omitted, the whole sub-document was deleted and the action vanished from the
// toolbox. `mx check` reported 0 errors in both cases, because a Java action
// with no toolbox entry is perfectly valid — only Studio Pro can see the loss.
//
// This is the guard-don't-drop rule of ADR-0005: what MDL cannot express, a
// rewrite must carry, not silently rebuild.

// storedExposedAction is a Java action as Studio Pro left it: exposed, with an
// icon and an image set.
func storedExposedAction(mod *model.Module) *javaactions.JavaAction {
	ja := &javaactions.JavaAction{
		ContainerID: mod.ID,
		Name:        "JA_Ping",
		MicroflowActionInfo: &javaactions.MicroflowActionInfo{
			Caption:       "Ping",
			Category:      "My first module",
			IconData:      []byte("ICON-64x64-PNG-BYTES"),
			IconDataDark:  []byte("ICON-DARK-PNG-BYTES"),
			ImageData:     []byte("IMAGE-256x192-PNG-BYTES"),
			ImageDataDark: []byte("IMAGE-DARK-PNG-BYTES"),
		},
	}
	ja.ID = nextID("ja")
	return ja
}

// exposeRewriteCtx wires a backend holding storedExposedAction and captures what
// the rewrite hands to UpdateJavaAction.
func exposeRewriteCtx(t *testing.T, stored *javaactions.JavaAction, mod *model.Module) (*ExecContext, **javaactions.JavaAction) {
	t.Helper()
	var captured *javaactions.JavaAction

	h := mkHierarchy(mod)
	withContainer(h, stored.ContainerID, mod.ID)

	light := &types.JavaAction{
		BaseElement: model.BaseElement{ID: stored.ID},
		ContainerID: stored.ContainerID,
		Name:        stored.Name,
	}

	mb := &mock.MockBackend{
		IsConnectedFunc:          func() bool { return true },
		ListModulesFunc:          func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListJavaActionsFunc:      func() ([]*types.JavaAction, error) { return []*types.JavaAction{light}, nil },
		ReadJavaActionByNameFunc: func(string) (*javaactions.JavaAction, error) { return stored, nil },
		UpdateJavaActionFunc: func(ja *javaactions.JavaAction) error {
			captured = ja
			return nil
		},
		WriteJavaSourceFileFunc: func(string, string, string, []*javaactions.JavaActionParameter,
			javaactions.CodeActionReturnType, []string, string, string) error {
			return nil
		},
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, &captured
}

func rewriteStmt(exposed bool) *ast.CreateJavaActionStmt {
	s := &ast.CreateJavaActionStmt{
		Name:           ast.QualifiedName{Module: "MyModule", Name: "JA_Ping"},
		JavaCode:       `return "pong";`,
		CreateOrModify: true,
	}
	if exposed {
		s.ExposedCaption = "Ping"
		s.ExposedCategory = "My first module"
	}
	return s
}

// The clause is kept, so Caption and Category come from MDL — but the four
// bitmaps MDL cannot name must survive.
func TestJavaActionRewriteKeepsIconAndImage(t *testing.T) {
	mod := mkModule("MyModule")
	stored := storedExposedAction(mod)
	ctx, captured := exposeRewriteCtx(t, stored, mod)

	assertNoError(t, execCreateJavaAction(ctx, rewriteStmt(true)))

	got := *captured
	if got == nil {
		t.Fatal("UpdateJavaAction was never called")
	}
	mai := got.MicroflowActionInfo
	if mai == nil {
		t.Fatal("MicroflowActionInfo is nil — the exposure was dropped by a rewrite that kept the clause")
	}
	if mai.Caption != "Ping" || mai.Category != "My first module" {
		t.Errorf("Caption/Category = %q/%q, want Ping/My first module", mai.Caption, mai.Category)
	}
	for _, tc := range []struct {
		field string
		got   []byte
		want  string
	}{
		{"IconData", mai.IconData, "ICON-64x64-PNG-BYTES"},
		{"IconDataDark", mai.IconDataDark, "ICON-DARK-PNG-BYTES"},
		{"ImageData", mai.ImageData, "IMAGE-256x192-PNG-BYTES"},
		{"ImageDataDark", mai.ImageDataDark, "IMAGE-DARK-PNG-BYTES"},
	} {
		if !bytes.Equal(tc.got, []byte(tc.want)) {
			t.Errorf("%s = %q, want %q — MDL cannot express a bitmap, so a rewrite must carry it",
				tc.field, tc.got, tc.want)
		}
	}
}

// The clause is omitted. MDL saying nothing about the exposure is not MDL asking
// for it to be removed: the whole sub-document is carried, bitmaps included.
func TestJavaActionRewriteWithoutClauseKeepsExposure(t *testing.T) {
	mod := mkModule("MyModule")
	stored := storedExposedAction(mod)
	ctx, captured := exposeRewriteCtx(t, stored, mod)

	assertNoError(t, execCreateJavaAction(ctx, rewriteStmt(false)))

	got := *captured
	if got == nil {
		t.Fatal("UpdateJavaAction was never called")
	}
	mai := got.MicroflowActionInfo
	if mai == nil {
		t.Fatal("MicroflowActionInfo is nil: omitting the clause deleted the toolbox entry — " +
			"caption, category, icon and image all lost, and mx check reports 0 errors")
	}
	if mai.Caption != "Ping" || mai.Category != "My first module" {
		t.Errorf("Caption/Category = %q/%q, want the stored Ping/My first module", mai.Caption, mai.Category)
	}
	if !bytes.Equal(mai.IconData, []byte("ICON-64x64-PNG-BYTES")) {
		t.Errorf("IconData = %q, want the stored bytes", mai.IconData)
	}
}

// The control: an action that was never exposed does not become exposed, and a
// rewrite that names no clause leaves it alone. Without this, "preserve" could
// be implemented as "always write something" and the tests above would still
// pass.
func TestJavaActionRewriteLeavesUnexposedActionAlone(t *testing.T) {
	mod := mkModule("MyModule")
	stored := &javaactions.JavaAction{ContainerID: mod.ID, Name: "JA_Ping"}
	stored.ID = nextID("ja")
	ctx, captured := exposeRewriteCtx(t, stored, mod)

	assertNoError(t, execCreateJavaAction(ctx, rewriteStmt(false)))

	got := *captured
	if got == nil {
		t.Fatal("UpdateJavaAction was never called")
	}
	if got.MicroflowActionInfo != nil {
		t.Errorf("MicroflowActionInfo = %+v, want nil — an unexposed action must not gain a toolbox entry",
			got.MicroflowActionInfo)
	}
}

// A clause on an action that was not exposed before still exposes it: preserving
// must not turn into ignoring.
func TestJavaActionRewriteExposesPreviouslyUnexposedAction(t *testing.T) {
	mod := mkModule("MyModule")
	stored := &javaactions.JavaAction{ContainerID: mod.ID, Name: "JA_Ping"}
	stored.ID = nextID("ja")
	ctx, captured := exposeRewriteCtx(t, stored, mod)

	assertNoError(t, execCreateJavaAction(ctx, rewriteStmt(true)))

	got := *captured
	if got == nil {
		t.Fatal("UpdateJavaAction was never called")
	}
	if got.MicroflowActionInfo == nil || got.MicroflowActionInfo.Caption != "Ping" {
		t.Fatalf("MicroflowActionInfo = %+v, want Caption Ping", got.MicroflowActionInfo)
	}
}
