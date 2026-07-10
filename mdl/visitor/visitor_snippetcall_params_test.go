// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestSnippetCallParams_DollarPrefix(t *testing.T) {
	input := `CREATE PAGE Mod.MyPage (Layout: Atlas.Default) {
		SNIPPETCALL sc1 (Snippet: Mod.MySnippet, Params: {$Asset: $theAsset})
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("parse error: %v", e)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.CreatePageStmtV3)
	sc := findChildByName2(stmt.Widgets, "sc1")
	if sc == nil {
		t.Fatal("sc1 widget not found")
	}

	params := sc.GetSnippetParams()
	if len(params) != 1 {
		t.Fatalf("GetSnippetParams: want 1, got %d", len(params))
	}
	if params[0].ParamName != "$Asset" {
		t.Errorf("ParamName: want $Asset, got %q", params[0].ParamName)
	}
	if params[0].Variable != "$theAsset" {
		t.Errorf("Variable: want $theAsset, got %q", params[0].Variable)
	}
}

func TestSnippetCallParams_BareIdentifier(t *testing.T) {
	input := `CREATE PAGE Mod.MyPage (Layout: Atlas.Default) {
		SNIPPETCALL sc2 (Snippet: Mod.MySnippet, Params: {Asset: $theAsset})
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("parse error: %v", e)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.CreatePageStmtV3)
	sc := findChildByName2(stmt.Widgets, "sc2")
	if sc == nil {
		t.Fatal("sc2 widget not found")
	}

	params := sc.GetSnippetParams()
	if len(params) != 1 {
		t.Fatalf("GetSnippetParams: want 1, got %d", len(params))
	}
	if params[0].ParamName != "Asset" {
		t.Errorf("ParamName: want Asset, got %q", params[0].ParamName)
	}
	if params[0].Variable != "$theAsset" {
		t.Errorf("Variable: want $theAsset, got %q", params[0].Variable)
	}
}

func TestSnippetCallParams_MultipleParams(t *testing.T) {
	input := `CREATE PAGE Mod.MyPage (Layout: Atlas.Default) {
		SNIPPETCALL sc3 (Snippet: Mod.MySnippet, Params: {$Asset: $a, $Customer: $c})
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("parse error: %v", e)
		}
		t.FailNow()
	}

	stmt := prog.Statements[0].(*ast.CreatePageStmtV3)
	sc := findChildByName2(stmt.Widgets, "sc3")
	if sc == nil {
		t.Fatal("sc3 widget not found")
	}

	params := sc.GetSnippetParams()
	if len(params) != 2 {
		t.Fatalf("GetSnippetParams: want 2, got %d", len(params))
	}
}
