// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#833: `{1} = $dv1.Name` naming a data view that does not enclose
// the widget passed check and exec, was written as SourceVariable
// { Widget: dv1 } and mx check reported CE7001 "Widget should be placed inside
// Data view 'dv1' to use it as a source widget." at Text 't2'. A data view's
// name is a source only for the widgets inside it.
func TestTemplateParamNamingNonEnclosingDataViewIsRefused(t *testing.T) {
	text := func(name, value string) *ast.WidgetV3 {
		return &ast.WidgetV3{Type: "dynamictext", Name: name, Properties: map[string]any{
			"Content":       "{1}",
			"ContentParams": []ast.ParamAssignmentV3{{Index: 1, Value: value}},
		}}
	}
	dv := func(name string, children ...*ast.WidgetV3) *ast.WidgetV3 {
		return &ast.WidgetV3{Type: "dataview", Name: name, Children: children}
	}
	widgets := []*ast.WidgetV3{
		dv("dv1", text("inside", "$dv1.Name")),            // control: encloses
		dv("dv2", text("t2", "$dv1.Name")),                // sibling: CE7001
		dv("dv3", dv("dv4", text("nested", "$dv3.Name"))), // control: an outer data view
	}
	got := validateNamedObjectBindings(widgets, []string{"Item"}, "page M.P")
	if len(got) != 1 {
		t.Fatalf("%d violations, want 1 (t2 only): %+v", len(got), got)
	}
	if got[0].RuleID != "MDL-WIDGET34" || !strings.Contains(got[0].Message, "`t2`") || !strings.Contains(got[0].Message, "CE7001") {
		t.Errorf("violation = %+v", got[0])
	}
}

// ako/mxcli#833: a page variable passed to a snippet parameter passed check and
// exec and was CE0115 "The arguments that are passed to snippet 'M833c.SN' do
// not match the expected parameters and need to be refreshed." Page variables
// are primitive, snippet parameters objects: it can never match.
func TestSnippetCallPassingPageVariableIsRefused(t *testing.T) {
	call := func(arg string) *ast.WidgetV3 {
		return &ast.WidgetV3{Type: "snippetcall", Name: "sc1", Properties: map[string]any{
			"Snippet": "M.SN",
			"Params":  []ast.SnippetCallParam{{ParamName: "Item", Variable: arg}},
		}}
	}
	vars := []ast.PageVariable{{Name: "Flag"}}
	if got := validatePageVariableBindings([]*ast.WidgetV3{call("$Flag")}, vars, "page M.P"); len(got) != 1 ||
		!strings.Contains(got[0].Message, "CE0115") {
		t.Errorf("$Flag: %+v, want one CE0115 refusal", got)
	}
	// Control: a parameter is passed as before.
	if got := validatePageVariableBindings([]*ast.WidgetV3{call("$Item")}, vars, "page M.P"); len(got) != 0 {
		t.Errorf("$Item: %+v, want none", got)
	}

	// The builder refuses it too (exec --no-check).
	mod := mkModule("Mod")
	snp := mkSnippetWithParam(mod.ID, "SN", "Item")
	h := mkHierarchy(mod)
	withContainer(h, snp.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:  func() bool { return true },
		ListModulesFunc:  func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc:  func() ([]*types.FolderInfo, error) { return nil, nil },
		ListSnippetsFunc: func() ([]*pages.Snippet, error) { return []*pages.Snippet{snp}, nil },
	}
	pb := newPageBuilder(mb, h, "Mod")
	pb.localVariables = map[string]bool{"Flag": true}
	_, err := pb.buildSnippetCallV3(&ast.WidgetV3{Type: "SNIPPETCALL", Name: "sc1", Properties: map[string]any{
		"Snippet": "Mod.SN",
		"Params":  []ast.SnippetCallParam{{ParamName: "Item", Variable: "$Flag"}},
	}})
	if err == nil || !strings.Contains(err.Error(), "CE0115") {
		t.Errorf("builder: err = %v, want the CE0115 refusal", err)
	}
}
