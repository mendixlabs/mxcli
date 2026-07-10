// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

func TestShowPages_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "Home")

	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listPages(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "MyModule.Home")
	assertContainsStr(t, out, "(1 pages)")
}

func TestShowPages_Mock_FilterByModule(t *testing.T) {
	mod1 := mkModule("Sales")
	mod2 := mkModule("HR")
	pg1 := mkPage(mod1.ID, "OrderList")
	pg2 := mkPage(mod2.ID, "EmployeeList")

	h := mkHierarchy(mod1, mod2)
	withContainer(h, pg1.ContainerID, mod1.ID)
	withContainer(h, pg2.ContainerID, mod2.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg1, pg2}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listPages(ctx, "HR"))

	out := buf.String()
	assertNotContainsStr(t, out, "Sales.OrderList")
	assertContainsStr(t, out, "HR.EmployeeList")
}

func TestShowSnippets_Mock_FilterByModule(t *testing.T) {
	mod1 := mkModule("Sales")
	mod2 := mkModule("HR")
	snp1 := mkSnippet(mod1.ID, "OrderHeader")
	snp2 := mkSnippet(mod2.ID, "EmployeeCard")

	h := mkHierarchy(mod1, mod2)
	withContainer(h, snp1.ContainerID, mod1.ID)
	withContainer(h, snp2.ContainerID, mod2.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:  func() bool { return true },
		ListSnippetsFunc: func() ([]*pages.Snippet, error) { return []*pages.Snippet{snp1, snp2}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listSnippets(ctx, "HR"))

	out := buf.String()
	assertNotContainsStr(t, out, "Sales.OrderHeader")
	assertContainsStr(t, out, "HR.EmployeeCard")
}

func TestShowLayouts_Mock_FilterByModule(t *testing.T) {
	mod1 := mkModule("Sales")
	mod2 := mkModule("HR")
	lay1 := mkLayout(mod1.ID, "SalesLayout")
	lay2 := mkLayout(mod2.ID, "HRLayout")

	h := mkHierarchy(mod1, mod2)
	withContainer(h, lay1.ContainerID, mod1.ID)
	withContainer(h, lay2.ContainerID, mod2.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListLayoutsFunc: func() ([]*pages.Layout, error) { return []*pages.Layout{lay1, lay2}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listLayouts(ctx, "HR"))

	out := buf.String()
	assertNotContainsStr(t, out, "Sales.SalesLayout")
	assertContainsStr(t, out, "HR.HRLayout")
}

func TestDescribePage_Mock_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{}, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertError(t, describePage(ctx, ast.QualifiedName{Module: "MyModule", Name: "NonExistent"}))
}

func TestDescribeSnippet_Mock_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc:  func() bool { return true },
		ListSnippetsFunc: func() ([]*pages.Snippet, error) { return []*pages.Snippet{}, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertError(t, describeSnippet(ctx, ast.QualifiedName{Module: "MyModule", Name: "NonExistent"}))
}

func TestDescribeLayout_Mock_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListLayoutsFunc: func() ([]*pages.Layout, error) { return []*pages.Layout{}, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertError(t, describeLayout(ctx, ast.QualifiedName{Module: "MyModule", Name: "NonExistent"}))
}

func TestShowSnippets_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	snp := mkSnippet(mod.ID, "Header")

	h := mkHierarchy(mod)
	withContainer(h, snp.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:  func() bool { return true },
		ListSnippetsFunc: func() ([]*pages.Snippet, error) { return []*pages.Snippet{snp}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listSnippets(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "MyModule.Header")
	assertContainsStr(t, out, "(1 snippets)")
}

func TestShowLayouts_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	lay := mkLayout(mod.ID, "Atlas_Default")

	h := mkHierarchy(mod)
	withContainer(h, lay.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListLayoutsFunc: func() ([]*pages.Layout, error) { return []*pages.Layout{lay}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listLayouts(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "MyModule.Atlas_Default")
	assertContainsStr(t, out, "(1 layouts)")
}
