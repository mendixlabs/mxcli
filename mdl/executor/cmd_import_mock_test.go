// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
)

// ---------------------------------------------------------------------------
// execImport
//
// NOTE: execImport depends on sqllib.Connection, ensureSQLManager,
// getOrAutoConnect, sqllib.ExecuteImport, and resolveImportLinks (which uses
// sqllib.LookupAssociationInfo with a live DB connection). The current mock
// infrastructure does not provide SQL connection/manager mocks, so only the
// not-connected guard can be exercised here. Expanding coverage requires
// building sqllib mock infrastructure (tracked separately).
// ---------------------------------------------------------------------------

func TestImport_NotConnected(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execImport(ctx, &ast.ImportStmt{
		SourceAlias:  "mydb",
		Query:        "SELECT * FROM users",
		TargetEntity: "MyModule.User",
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not connected")
}
