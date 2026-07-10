// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
)

func TestShowVersion_Mock(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ProjectVersionFunc: func() *types.ProjectVersion {
			return &types.ProjectVersion{
				ProductVersion: "10.18.0",
				BuildVersion:   "10.18.0.12345",
				FormatVersion:  2,
				SchemaHash:     "abc123def456",
			}
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, listVersion(ctx))

	out := buf.String()
	assertContainsStr(t, out, "Mendix Version")
	assertContainsStr(t, out, "10.18.0")
	assertContainsStr(t, out, "Build Version")
	assertContainsStr(t, out, "MPR Format")
	assertContainsStr(t, out, "Schema Hash")
	assertContainsStr(t, out, "abc123def456")
}

func TestShowVersion_NotConnected(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	assertError(t, listVersion(ctx))
}

func TestShowVersion_NoSchemaHash(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ProjectVersionFunc: func() *types.ProjectVersion {
			return &types.ProjectVersion{
				ProductVersion: "9.24.0",
				BuildVersion:   "9.24.0.5678",
				FormatVersion:  1,
				SchemaHash:     "",
			}
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, listVersion(ctx))

	out := buf.String()
	assertContainsStr(t, out, "9.24.0")
	assertNotContainsStr(t, out, "Schema Hash")
}

func TestHelp_Mock(t *testing.T) {
	ctx, buf := newMockCtx(t)
	assertNoError(t, execHelp(ctx, &ast.HelpStmt{}))

	out := buf.String()
	assertContainsStr(t, out, "MDL Commands")
	assertContainsStr(t, out, "connect local")
}

// ---------------------------------------------------------------------------
// SET — format key updates ctx.Format
// ---------------------------------------------------------------------------

func TestExecSet_FormatKey_UpdatesCtxFormat(t *testing.T) {
	ctx, buf := newMockCtx(t) // starts as FormatTable (default)
	if ctx.Format != FormatTable {
		t.Fatalf("expected default FormatTable, got %q", ctx.Format)
	}
	err := execSet(ctx, &ast.SetStmt{Key: "format", Value: "json"})
	assertNoError(t, err)
	if ctx.Format != FormatJSON {
		t.Errorf("expected ctx.Format = %q after SET format=json, got %q", FormatJSON, ctx.Format)
	}
	assertContainsStr(t, buf.String(), "Set format = json")
}

func TestExecSet_FormatKey_BareIdentifier(t *testing.T) {
	ctx, _ := newMockCtx(t)
	assertNoError(t, execSet(ctx, &ast.SetStmt{Key: "format", Value: "table"}))
	if ctx.Format != FormatTable {
		t.Errorf("expected FormatTable, got %q", ctx.Format)
	}
}

func TestExecSet_UnknownKey_StoredOnly(t *testing.T) {
	ctx, _ := newMockCtx(t)
	assertNoError(t, execSet(ctx, &ast.SetStmt{Key: "mykey", Value: "myval"}))
	if ctx.Settings["mykey"] != "myval" {
		t.Errorf("expected Settings[mykey]=myval, got %v", ctx.Settings["mykey"])
	}
}

// ---------------------------------------------------------------------------
// EXECUTE SCRIPT — depth limit
// ---------------------------------------------------------------------------

func TestExecuteScript_DepthLimitExceeded(t *testing.T) {
	ctx, _ := newMockCtx(t)
	ctx.ScriptDepth = maxScriptDepth
	err := execExecuteScript(ctx, &ast.ExecuteScriptStmt{Path: "/some/script.mdl"})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "maximum script nesting depth")
}
