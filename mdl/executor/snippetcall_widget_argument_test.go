// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#1968 (mendixlabs/mxcli#1746): `snippetcall … Params: (Item =
// $dvOuter)` passes a data view's object by naming the data view. It was
// classified as a page parameter and written in PageParameter, a page
// parameter the page does not have: mx check CE0115. A document widget's
// name is the Widget slot's, as it is for a flow call's argument (#1560).
func TestSnippetCall_DataViewNameIsAWidgetArgument(t *testing.T) {
	mod := mkModule("Mod")
	snp := mkSnippetWithParam(mod.ID, "MySnippet", "Asset")
	h := mkHierarchy(mod)
	withContainer(h, snp.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:  func() bool { return true },
		ListModulesFunc:  func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc:  func() ([]*types.FolderInfo, error) { return nil, nil },
		ListSnippetsFunc: func() ([]*pages.Snippet, error) { return []*pages.Snippet{snp}, nil },
	}
	build := func(arg string) pages.SnippetParamMapping {
		t.Helper()
		pb := newPageBuilder(mb, h, "Mod")
		pb.isDocumentWidget = func(name string) bool { return name == "dvOuter" }
		pb.paramScope["Asset"] = "p1"
		sc, err := pb.buildSnippetCallV3(&ast.WidgetV3{Type: "SNIPPETCALL", Name: "sc1", Properties: map[string]any{
			"Snippet": "Mod.MySnippet",
			"Params":  []ast.SnippetCallParam{{ParamName: "Asset", Variable: arg}},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if len(sc.ParameterMappings) != 1 {
			t.Fatalf("mappings = %d, want 1", len(sc.ParameterMappings))
		}
		return sc.ParameterMappings[0]
	}
	if m := build("$dvOuter"); !m.IsWidget || m.IsSnippetParameter || m.IsLocalVariable {
		t.Errorf("$dvOuter: %+v, want a widget argument", m)
	}
	// Control: a page parameter of that page stays one.
	if m := build("$Asset"); m.IsWidget {
		t.Errorf("$Asset: %+v, want a page parameter", m)
	}
}

// describe reads the Widget slot back as `$dvOuter`, so describe -> exec
// round-trips the call; it read PageParameter / SnippetParameter /
// LocalVariable only and printed the argument as absent.
func TestSnippetCall_DescribeReadsTheWidgetSlot(t *testing.T) {
	call := func(variable map[string]any) map[string]any {
		return map[string]any{"FormCall": map[string]any{"ParameterMappings": []any{
			int32(2),
			map[string]any{"Parameter": "ModD.SN_Item.Item", "Variable": variable},
		}}}
	}
	if got := extractSnippetCallParams(call(map[string]any{"Widget": "dvOuter", "PageParameter": ""})); got != "Item = $dvOuter" {
		t.Errorf("widget slot: %q, want Item = $dvOuter", got)
	}
	if got := extractSnippetCallParams(call(map[string]any{"PageParameter": "Note"})); got != "Item = $Note" {
		t.Errorf("control, page parameter: %q", got)
	}
}
