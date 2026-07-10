// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

func TestShowDatabaseConnections_Mock(t *testing.T) {
	mod := mkModule("DataMod")
	conn := &model.DatabaseConnection{
		BaseElement:  model.BaseElement{ID: nextID("dbc")},
		ContainerID:  mod.ID,
		Name:         "MyDB",
		DatabaseType: "PostgreSQL",
	}

	h := mkHierarchy(mod)
	withContainer(h, conn.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:             func() bool { return true },
		ListDatabaseConnectionsFunc: func() ([]*model.DatabaseConnection, error) { return []*model.DatabaseConnection{conn}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listDatabaseConnections(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "Qualified Name")
	assertContainsStr(t, out, "DataMod.MyDB")
}

func TestShowDatabaseConnections_FilterByModule(t *testing.T) {
	mod1 := mkModule("DataMod")
	mod2 := mkModule("Other")
	conn1 := &model.DatabaseConnection{
		BaseElement:  model.BaseElement{ID: nextID("dbc")},
		ContainerID:  mod1.ID,
		Name:         "MyDB",
		DatabaseType: "PostgreSQL",
	}
	conn2 := &model.DatabaseConnection{
		BaseElement:  model.BaseElement{ID: nextID("dbc")},
		ContainerID:  mod2.ID,
		Name:         "OtherDB",
		DatabaseType: "MySQL",
	}

	h := mkHierarchy(mod1, mod2)
	withContainer(h, conn1.ContainerID, mod1.ID)
	withContainer(h, conn2.ContainerID, mod2.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:             func() bool { return true },
		ListDatabaseConnectionsFunc: func() ([]*model.DatabaseConnection, error) { return []*model.DatabaseConnection{conn1, conn2}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listDatabaseConnections(ctx, "DataMod"))

	out := buf.String()
	assertContainsStr(t, out, "DataMod.MyDB")
	assertNotContainsStr(t, out, "Other.OtherDB")
}

func TestDescribeDatabaseConnection_NotFound(t *testing.T) {
	mod := mkModule("DataMod")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc:             func() bool { return true },
		ListDatabaseConnectionsFunc: func() ([]*model.DatabaseConnection, error) { return nil, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertError(t, describeDatabaseConnection(ctx, ast.QualifiedName{Module: "DataMod", Name: "NoSuch"}))
}

func TestDescribeDatabaseConnection_Mock(t *testing.T) {
	mod := mkModule("DataMod")
	conn := &model.DatabaseConnection{
		BaseElement:  model.BaseElement{ID: nextID("dbc")},
		ContainerID:  mod.ID,
		Name:         "MyDB",
		DatabaseType: "PostgreSQL",
	}

	h := mkHierarchy(mod)
	withContainer(h, conn.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:             func() bool { return true },
		ListDatabaseConnectionsFunc: func() ([]*model.DatabaseConnection, error) { return []*model.DatabaseConnection{conn}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeDatabaseConnection(ctx, ast.QualifiedName{Module: "DataMod", Name: "MyDB"}))

	out := buf.String()
	assertContainsStr(t, out, "create database connection")
}
