// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/javaactions"
)

func TestShowJavaActions_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	ja := &types.JavaAction{
		BaseElement: model.BaseElement{ID: nextID("ja")},
		ContainerID: mod.ID,
		Name:        "DoSomething",
	}

	h := mkHierarchy(mod)
	withContainer(h, ja.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:     func() bool { return true },
		ListJavaActionsFunc: func() ([]*types.JavaAction, error) { return []*types.JavaAction{ja}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listJavaActions(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "Qualified Name")
	assertContainsStr(t, out, "MyModule.DoSomething")
}

func TestDescribeJavaAction_Mock(t *testing.T) {
	mod := mkModule("MyModule")

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ReadJavaActionByNameFunc: func(qn string) (*javaactions.JavaAction, error) {
			return &javaactions.JavaAction{
				BaseElement: model.BaseElement{ID: nextID("ja")},
				ContainerID: mod.ID,
				Name:        "DoSomething",
			}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeJavaAction(ctx, ast.QualifiedName{Module: "MyModule", Name: "DoSomething"}))

	out := buf.String()
	assertContainsStr(t, out, "create java action")
}

// TestDescribeJavaAction_AlwaysEmitsBody is a regression test for issue #637:
// the CREATE JAVA ACTION grammar requires an `as $$ ... $$;` body, but DESCRIBE
// only emitted one when the .java source was readable. Add-on/Marketplace actions
// ship without source on disk (here the mock MprPath resolves to nothing), so the
// output must still include a placeholder body terminated by `$$;` to re-parse.
func TestDescribeJavaAction_AlwaysEmitsBody(t *testing.T) {
	mod := mkModule("MyModule")

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ReadJavaActionByNameFunc: func(qn string) (*javaactions.JavaAction, error) {
			return &javaactions.JavaAction{
				BaseElement: model.BaseElement{ID: nextID("ja")},
				ContainerID: mod.ID,
				Name:        "SourcelessAction",
			}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeJavaAction(ctx, ast.QualifiedName{Module: "MyModule", Name: "SourcelessAction"}))

	out := buf.String()
	assertContainsStr(t, out, "as $$")
	assertContainsStr(t, out, "$$;")
}

// NOTE: listJavaActions has no explicit not-connected guard. It calls
// getHierarchy (which returns nil when disconnected) and is intended to
// be reached through execShow, which enforces a connected backend first.
// A nil hierarchy is only harmless when the backend returns no Java
// actions; if Java actions are returned while disconnected, dereferencing
// the nil hierarchy would panic.

func TestShowJavaActions_BackendError(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListJavaActionsFunc: func() ([]*types.JavaAction, error) {
			return nil, fmt.Errorf("backend unavailable")
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertError(t, listJavaActions(ctx, ""))
}

func TestDescribeJavaAction_NotFound(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ReadJavaActionByNameFunc: func(qn string) (*javaactions.JavaAction, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	assertError(t, describeJavaAction(ctx, ast.QualifiedName{Module: "MyModule", Name: "Missing"}))
}

func TestShowJavaActions_FilterByModule(t *testing.T) {
	mod1 := mkModule("Alpha")
	mod2 := mkModule("Beta")
	ja1 := &types.JavaAction{
		BaseElement: model.BaseElement{ID: nextID("ja")},
		ContainerID: mod1.ID,
		Name:        "ActionA",
	}
	ja2 := &types.JavaAction{
		BaseElement: model.BaseElement{ID: nextID("ja")},
		ContainerID: mod2.ID,
		Name:        "ActionB",
	}

	h := mkHierarchy(mod1, mod2)
	withContainer(h, ja1.ContainerID, mod1.ID)
	withContainer(h, ja2.ContainerID, mod2.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:     func() bool { return true },
		ListJavaActionsFunc: func() ([]*types.JavaAction, error) { return []*types.JavaAction{ja1, ja2}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listJavaActions(ctx, "Alpha"))

	out := buf.String()
	assertContainsStr(t, out, "Alpha.ActionA")
	assertNotContainsStr(t, out, "Beta.ActionB")
}

func TestShowJavaActions_JSON(t *testing.T) {
	mod := mkModule("MyModule")
	ja := &types.JavaAction{
		BaseElement: model.BaseElement{ID: nextID("ja")},
		ContainerID: mod.ID,
		Name:        "DoSomething",
	}

	h := mkHierarchy(mod)
	withContainer(h, ja.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:     func() bool { return true },
		ListJavaActionsFunc: func() ([]*types.JavaAction, error) { return []*types.JavaAction{ja}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h), withFormat(FormatJSON))
	assertNoError(t, listJavaActions(ctx, ""))
	assertValidJSON(t, buf.String())
}
