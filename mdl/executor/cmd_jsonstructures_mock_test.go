// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

func TestShowJsonStructures_Mock(t *testing.T) {
	mod := mkModule("OrderMgmt")
	js := &types.JsonStructure{
		BaseElement: model.BaseElement{ID: nextID("js")},
		ContainerID: mod.ID,
		Name:        "OrderSchema",
	}

	h := mkHierarchy(mod)
	withContainer(h, js.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		ListJsonStructuresFunc: func() ([]*types.JsonStructure, error) { return []*types.JsonStructure{js}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listJsonStructures(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "json Structure")
	assertContainsStr(t, out, "OrderMgmt.OrderSchema")
}

func TestShowJsonStructures_FilterByModule(t *testing.T) {
	mod1 := mkModule("OrderMgmt")
	mod2 := mkModule("Other")
	js1 := &types.JsonStructure{
		BaseElement: model.BaseElement{ID: nextID("js")},
		ContainerID: mod1.ID,
		Name:        "OrderSchema",
	}
	js2 := &types.JsonStructure{
		BaseElement: model.BaseElement{ID: nextID("js")},
		ContainerID: mod2.ID,
		Name:        "OtherSchema",
	}

	h := mkHierarchy(mod1, mod2)
	withContainer(h, js1.ContainerID, mod1.ID)
	withContainer(h, js2.ContainerID, mod2.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		ListJsonStructuresFunc: func() ([]*types.JsonStructure, error) { return []*types.JsonStructure{js1, js2}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listJsonStructures(ctx, "OrderMgmt"))

	out := buf.String()
	assertContainsStr(t, out, "OrderMgmt.OrderSchema")
	assertNotContainsStr(t, out, "Other.OtherSchema")
}

func TestDescribeJsonStructure_Mock(t *testing.T) {
	mod := mkModule("OrderMgmt")
	js := &types.JsonStructure{
		BaseElement: model.BaseElement{ID: nextID("js")},
		ContainerID: mod.ID,
		Name:        "OrderSchema",
	}

	h := mkHierarchy(mod)
	withContainer(h, js.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		ListJsonStructuresFunc: func() ([]*types.JsonStructure, error) { return []*types.JsonStructure{js}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeJsonStructure(ctx, ast.QualifiedName{Module: "OrderMgmt", Name: "OrderSchema"}))
	assertContainsStr(t, buf.String(), "create or modify json structure")
}

// TestDescribeJsonStructure_DocumentationWithCommentEnd: R9 made the doc
// comment the one spelling describe emits, but a text with `*/` in it ends a
// doc comment early. Describe keeps the `comment '…'` clause for it, so the
// statement parses and the documentation reads back unchanged. The control is
// an ordinary text.
func TestDescribeJsonStructure_DocumentationWithCommentEnd(t *testing.T) {
	for _, doc := range []string{"ends */ here", "plain text"} {
		mod := mkModule("OrderMgmt")
		js := &types.JsonStructure{
			BaseElement:   model.BaseElement{ID: nextID("js")},
			ContainerID:   mod.ID,
			Name:          "OrderSchema",
			Documentation: doc,
			JsonSnippet:   `{"a":1}`,
		}
		h := mkHierarchy(mod)
		withContainer(h, js.ContainerID, mod.ID)
		mb := &mock.MockBackend{
			IsConnectedFunc:        func() bool { return true },
			ListJsonStructuresFunc: func() ([]*types.JsonStructure, error) { return []*types.JsonStructure{js}, nil },
		}
		ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
		assertNoError(t, describeJsonStructure(ctx, ast.QualifiedName{Module: "OrderMgmt", Name: "OrderSchema"}))
		prog, errs := visitor.Build(buf.String())
		if len(errs) > 0 {
			t.Fatalf("describe of documentation %q does not parse: %v\n%s", doc, errs, buf.String())
		}
		var got *ast.CreateJsonStructureStmt
		for _, s := range prog.Statements {
			if cs, ok := s.(*ast.CreateJsonStructureStmt); ok {
				got = cs
			}
		}
		if got == nil || got.Documentation != doc {
			t.Errorf("documentation %q did not read back; got %+v from:\n%s", doc, got, buf.String())
		}
	}
}

func TestDescribeJsonStructure_NotFound(t *testing.T) {
	mod := mkModule("OrderMgmt")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		ListJsonStructuresFunc: func() ([]*types.JsonStructure, error) { return nil, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertError(t, describeJsonStructure(ctx, ast.QualifiedName{Module: "OrderMgmt", Name: "NoSuch"}))
}

// TestDescribeJsonStructure_SnippetContainingDollarDollar (ako/mxcli#1003): a
// snippet holding `$$` — a Teamcenter UID such as
// "SR::N::BOMLine..10.7$$Otklm9GclKA" — ended describe's `$$ … $$` literal
// early, and `mxcli check` on the output reported `token recognition error at:
// '$$`. The controls are the same snippet without the `$$`, which keeps the
// dollar-quoted form, and one ending in a single `$`.
func TestDescribeJsonStructure_SnippetContainingDollarDollar(t *testing.T) {
	for _, snippet := range []string{
		`{"uid": "SR::N::BOMLine..10.7$$Otklm9GclKA", "name": "it's"}`,
		`{"uid": "SR::N::BOMLine..10.7Otklm9GclKA", "name": "it's"}`,
		`{"path": "C:\\temp$$x", "price": "5$"}`,
	} {
		mod := mkModule("Tc")
		js := &types.JsonStructure{
			BaseElement: model.BaseElement{ID: nextID("js")},
			ContainerID: mod.ID,
			Name:        "BomLine",
			JsonSnippet: snippet,
		}
		h := mkHierarchy(mod)
		withContainer(h, js.ContainerID, mod.ID)
		mb := &mock.MockBackend{
			IsConnectedFunc:        func() bool { return true },
			ListJsonStructuresFunc: func() ([]*types.JsonStructure, error) { return []*types.JsonStructure{js}, nil },
		}
		ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
		assertNoError(t, describeJsonStructure(ctx, ast.QualifiedName{Module: "Tc", Name: "BomLine"}))
		prog, errs := visitor.Build(describedSource(ctx, buf.String()))
		if len(errs) > 0 {
			t.Fatalf("describe of snippet %q does not parse: %v\n%s", snippet, errs, buf.String())
		}
		var got *ast.CreateJsonStructureStmt
		for _, s := range prog.Statements {
			if cs, ok := s.(*ast.CreateJsonStructureStmt); ok {
				got = cs
			}
		}
		want := types.PrettyPrintJSON(snippet)
		if got == nil || got.JsonSnippet != want {
			t.Errorf("snippet %q did not read back; got %+v from:\n%s", snippet, got, buf.String())
		}
	}
}
