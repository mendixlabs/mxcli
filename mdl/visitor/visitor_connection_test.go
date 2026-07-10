// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestConnect_Local(t *testing.T) {
	input := `CONNECT LOCAL '/path/to/project';`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.ConnectStmt)
	if !ok {
		t.Fatalf("Expected ConnectStmt, got %T", prog.Statements[0])
	}
	if stmt.Path != "/path/to/project" {
		t.Errorf("Expected /path/to/project, got %q", stmt.Path)
	}
}

// TestConnect_WindowsPathRaw documents the bug from issue #644: interpolating a
// raw Windows path into CONNECT LOCAL '<path>' mangles it, because the MDL lexer
// interprets `\t`/`\n`/`\r` and drops escape backslashes.
func TestConnect_WindowsPathRaw(t *testing.T) {
	const winPath = `C:\temp\App.mpr`
	prog, errs := Build("CONNECT LOCAL '" + winPath + "'")
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.ConnectStmt)
	// The raw interpolation corrupts the path (\t -> tab). This is the bug.
	if stmt.Path == winPath {
		t.Fatalf("expected raw interpolation to corrupt the path, but it survived")
	}
}

// TestQuoteString_RoundTrip is the fix for #644: QuoteString(path) embedded in a
// CONNECT LOCAL literal must parse back to the original path unchanged.
func TestQuoteString_RoundTrip(t *testing.T) {
	paths := []string{
		`C:\Users\me\app\App.mpr`,
		`C:\temp\App.mpr`,    // \t
		`C:\new\App.mpr`,     // \n
		`C:\reports\App.mpr`, // \r
		`D:\a\b\c.mpr`,
		`\\server\share\App.mpr`, // UNC
		`/home/user/app.mpr`,     // unix, unchanged
		`C:\It's a\trap.mpr`,     // apostrophe + \t
	}
	for _, p := range paths {
		// Unit level: QuoteString is the exact inverse of unquoteString.
		if got := unquoteString("'" + QuoteString(p) + "'"); got != p {
			t.Errorf("unquoteString(QuoteString(%q)) = %q, want %q", p, got, p)
		}
		// Integration level: through the real parser via CONNECT LOCAL.
		prog, errs := Build("CONNECT LOCAL '" + QuoteString(p) + "'")
		if len(errs) > 0 {
			t.Errorf("parse errors for %q: %v", p, errs)
			continue
		}
		if stmt, ok := prog.Statements[0].(*ast.ConnectStmt); !ok || stmt.Path != p {
			t.Errorf("CONNECT LOCAL round-trip for %q: got %+v", p, prog.Statements[0])
		}
	}
}

func TestDisconnect(t *testing.T) {
	input := `DISCONNECT;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	_, ok := prog.Statements[0].(*ast.DisconnectStmt)
	if !ok {
		t.Fatalf("Expected DisconnectStmt, got %T", prog.Statements[0])
	}
}

func TestStatus_BareKeyword(t *testing.T) {
	prog, errs := Build(`STATUS;`)
	if len(errs) > 0 {
		t.Fatalf("Parse errors for bare STATUS: %v", errs)
	}
	if _, ok := prog.Statements[0].(*ast.StatusStmt); !ok {
		t.Fatalf("Expected StatusStmt, got %T", prog.Statements[0])
	}
}

func TestStatus_ShowStatus(t *testing.T) {
	prog, errs := Build(`SHOW STATUS;`)
	if len(errs) > 0 {
		t.Fatalf("Parse errors for SHOW STATUS: %v", errs)
	}
	if _, ok := prog.Statements[0].(*ast.StatusStmt); !ok {
		t.Fatalf("Expected StatusStmt for SHOW STATUS, got %T", prog.Statements[0])
	}
}

func TestStatus_ShowCatalogStatusUnaffected(t *testing.T) {
	prog, errs := Build(`SHOW CATALOG STATUS;`)
	if len(errs) > 0 {
		t.Fatalf("Parse errors for SHOW CATALOG STATUS: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.ShowStmt)
	if !ok {
		t.Fatalf("Expected ShowStmt, got %T", prog.Statements[0])
	}
	if stmt.ObjectType != ast.ShowCatalogStatus {
		t.Errorf("Expected ShowCatalogStatus, got %v", stmt.ObjectType)
	}
}
