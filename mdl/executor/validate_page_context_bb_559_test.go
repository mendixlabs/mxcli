// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// cardBlockCtx is a connected context whose project holds one building block,
// MyModule.Card, made of a container and a dynamictext (bbRawWidgets).
func cardBlockCtx(t *testing.T) *ExecContext {
	t.Helper()
	mod := mkModule("MyModule")
	bb := mkBuildingBlock(mod.ID, "Card")
	h := mkHierarchy(mod)
	withContainer(h, bb.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListBuildingBlocksFunc: func() ([]*pages.BuildingBlock, error) {
			return []*pages.BuildingBlock{bb}, nil
		},
		GetRawUnitFunc: func(id model.ID) (map[string]any, error) {
			return bbRawWidgets(), nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx
}

func pageContextErrorsOf(t *testing.T, ctx *ExecContext, src string) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	s, ok := prog.Statements[0].(*ast.CreatePageStmtV3)
	if !ok {
		t.Fatalf("expected CreatePageStmtV3, got %T", prog.Statements[0])
	}
	return validatePageContextTree(ctx, s.Parameters, allPageWidgets(s))
}

// TestCheckPrefixedBuildingBlocksAreNotDuplicates is ako/mxcli#559: placing a
// block several times, each with its own `as <prefix>`, was refused by
// `check --references` as
//
//	duplicate widget name 'Atlas_Web_Content.Card' (used 4 times)
//
// — the STATEMENT counted as a widget named after the block, before
// expansion — while exec expands and prefixes, and mx check reports no CE0495.
func TestCheckPrefixedBuildingBlocksAreNotDuplicates(t *testing.T) {
	src := `create page MyModule.Dash (Title: 'Dash', Layout: Atlas_Core.Atlas_Default) {
		use building block MyModule.Card as a_
		use building block MyModule.Card as b_
		use building block MyModule.Card as c_
		use building block MyModule.Card as d_
	};`
	for _, e := range pageContextErrorsOf(t, cardBlockCtx(t), src) {
		if strings.Contains(e, "CE0495") {
			t.Errorf("prefixed copies reported as duplicates: %s", e)
		}
	}
}

// TestCheckUnprefixedBuildingBlockTwiceIsDuplicate is the other half: the same
// block placed twice WITHOUT a prefix really does collide — exec refuses it
// with "duplicate widget name 'container'" — so check must still say so, by
// the widget names the expansion produces rather than the block's name.
func TestCheckUnprefixedBuildingBlockTwiceIsDuplicate(t *testing.T) {
	src := `create page MyModule.Dash (Title: 'Dash', Layout: Atlas_Core.Atlas_Default) {
		use building block MyModule.Card
		use building block MyModule.Card
	};`
	errs := pageContextErrorsOf(t, cardBlockCtx(t), src)
	joined := strings.Join(errs, "\n")
	if !strings.Contains(joined, "duplicate widget name 'container'") {
		t.Errorf("unprefixed double placement not reported by its widget names; got:\n%s", joined)
	}
	if strings.Contains(joined, "MyModule.Card") {
		t.Errorf("the block's own name was counted as a widget name:\n%s", joined)
	}
}

// TestCheckPrefixedBlockNameCollidesWithPageWidget: a prefixed copy whose
// expanded name equals a widget the page declares itself is a real CE0495.
func TestCheckPrefixedBlockNameCollidesWithPageWidget(t *testing.T) {
	src := `create page MyModule.Dash (Title: 'Dash', Layout: Atlas_Core.Atlas_Default) {
		container a_container
		use building block MyModule.Card as a_
	};`
	joined := strings.Join(pageContextErrorsOf(t, cardBlockCtx(t), src), "\n")
	if !strings.Contains(joined, "duplicate widget name 'a_container'") {
		t.Errorf("collision with the page's own widget not reported; got:\n%s", joined)
	}
}

// TestCheckBuildingBlockWithoutProjectIsNotAWidgetName: with nothing to expand
// against (no project, an unknown block), the reference is not a widget and
// its name must not be counted.
func TestCheckBuildingBlockWithoutProjectIsNotAWidgetName(t *testing.T) {
	src := `create page MyModule.Dash (Title: 'Dash', Layout: Atlas_Core.Atlas_Default) {
		use building block Atlas_Web_Content.Card as a_
		use building block Atlas_Web_Content.Card as b_
	};`
	for _, e := range pageContextErrorsOf(t, nil, src) {
		if strings.Contains(e, "CE0495") {
			t.Errorf("unexpanded block reference counted as a widget: %s", e)
		}
	}
}
