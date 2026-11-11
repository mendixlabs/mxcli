// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#826. A binding read through a named data view — an input's
// `Attribute: $dataView1.Title`, a dynamic-text `{1} = $dataView1.Title` — is
// Studio Pro's Forms$PageVariable {Widget: dataView1, PageParameter: Task}: the
// data view in Widget, and beside it the data view's own variable. Given only
// {widget: dataView1} through its MCP server, Studio Pro 11.14 fills in the
// PageParameter itself. The builder had no Widget slot at all: an input dropped
// the binding (PedApp Administration.Account_Edit/Account_New), and a
// dynamic-text parameter named the data view as a page parameter.

func scopedPB(isSnippet bool) *pageBuilder {
	pb := templateBindingPB(isSnippet)
	pb.widgetScope = map[string]model.ID{}
	return pb
}

// buildInDataView builds `dataview dataView1 (DataSource: $Task) { child }` and
// returns the built child.
func buildInDataView(t *testing.T, pb *pageBuilder, child *ast.WidgetV3) pages.Widget {
	t.Helper()
	dv, err := pb.buildDataViewV3(&ast.WidgetV3{
		Name: "dataView1", Type: "dataview",
		Properties: map[string]any{"DataSource": &ast.DataSourceV3{Type: "parameter", Reference: "$Task"}},
		Children:   []*ast.WidgetV3{child},
	})
	if err != nil {
		t.Fatalf("buildDataViewV3: %v", err)
	}
	if len(dv.Widgets) != 1 {
		t.Fatalf("data view built %d children, want 1", len(dv.Widgets))
	}
	return dv.Widgets[0]
}

func TestInputAttributeThroughDataView(t *testing.T) {
	for _, tc := range []struct {
		name      string
		isSnippet bool
		attr      string
		want      *pages.WidgetVariable
	}{
		{"page: through the data view", false, "$dataView1.Title", &pages.WidgetVariable{Widget: "dataView1", Variable: "Task"}},
		{"snippet: through the data view", true, "$dataView1.Title", &pages.WidgetVariable{Widget: "dataView1", Variable: "Task", Kind: "snippet"}},
		// Control: a context-relative attribute keeps a null SourceVariable.
		{"page: context attribute", false, "Title", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := buildInDataView(t, scopedPB(tc.isSnippet), &ast.WidgetV3{
				Name: "tb", Type: "textbox", Properties: map[string]any{"Attribute": tc.attr},
			})
			tb, ok := w.(*pages.TextBox)
			if !ok {
				t.Fatalf("built %T, want *pages.TextBox", w)
			}
			if tb.AttributePath != "M.Job.Title" {
				t.Errorf("AttributePath = %q, want M.Job.Title", tb.AttributePath)
			}
			switch {
			case tc.want == nil && tb.SourceVariable != nil:
				t.Errorf("SourceVariable = %+v, want none", *tb.SourceVariable)
			case tc.want != nil && (tb.SourceVariable == nil || *tb.SourceVariable != *tc.want):
				t.Errorf("SourceVariable = %+v, want %+v", tb.SourceVariable, *tc.want)
			}
		})
	}
}

// `$name.Attr` on an input names a data view or a parameter (the parameter form
// is TestInputAttributeThroughParameter). A name that is neither is refused
// rather than written with no binding.
func TestInputAttributeThroughUnknownName(t *testing.T) {
	for _, attr := range []string{"$nope.Title"} {
		pb := scopedPB(false)
		dv := &ast.WidgetV3{
			Name: "dataView1", Type: "dataview",
			Properties: map[string]any{"DataSource": &ast.DataSourceV3{Type: "parameter", Reference: "$Task"}},
			Children:   []*ast.WidgetV3{{Name: "tb", Type: "textbox", Properties: map[string]any{"Attribute": attr}}},
		}
		_, err := pb.buildDataViewV3(dv)
		if err == nil || !strings.Contains(err.Error(), "data view") {
			t.Errorf("Attribute: %s: err = %v, want a refusal naming the data view form", attr, err)
		}
	}
}

func TestTemplateParameterThroughDataView(t *testing.T) {
	for _, tc := range []struct {
		name       string
		isSnippet  bool
		value      string
		wantWidget string
		wantVar    string
		wantKind   string
	}{
		{"page: through the data view", false, "$dataView1.Title", "dataView1", "Task", ""},
		{"snippet: through the data view", true, "$dataView1.Title", "dataView1", "Task", "snippet"},
		// Control: a parameter binding is unchanged.
		{"page parameter", false, "$Task.Title", "", "Task", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := buildInDataView(t, scopedPB(tc.isSnippet), &ast.WidgetV3{
				Name: "t1", Type: "dynamictext", Properties: map[string]any{
					"Content":       "{1}",
					"ContentParams": []ast.ParamAssignmentV3{{Index: 1, Value: tc.value}},
				},
			})
			dt, ok := w.(*pages.DynamicText)
			if !ok || dt.Content == nil || len(dt.Content.Parameters) != 1 {
				t.Fatalf("built %#v, want a dynamic text with one parameter", w)
			}
			p := dt.Content.Parameters[0]
			if p.SourceWidget != tc.wantWidget || p.SourceVariable != tc.wantVar || p.SourceVariableKind != tc.wantKind {
				t.Errorf("%s: (Widget, Variable, Kind) = (%q, %q, %q), want (%q, %q, %q)", tc.value,
					p.SourceWidget, p.SourceVariable, p.SourceVariableKind, tc.wantWidget, tc.wantVar, tc.wantKind)
			}
			if p.AttributeRef != "M.Job.Title" {
				t.Errorf("AttributeRef = %q, want M.Job.Title", p.AttributeRef)
			}
		})
	}
}

// A page `Variables:` entry passed to a snippet call is refused: page
// variables are primitive and a snippet parameter is an object, so the call is
// mx check CE0115 in either slot — measured with the LocalVariable slot this
// test used to pin (ako/mxcli#833). A page parameter is passed as before.
func TestSnippetCallLocalVariableArgument(t *testing.T) {
	for _, tc := range []struct {
		arg     string
		refused bool
	}{
		{"$Caption", true},
		{"$Task", false}, // control: a page parameter
	} {
		pb := scopedPB(false)
		pb.localVariables = map[string]bool{"Caption": true}
		pb.backend = &mock.MockBackend{ListSnippetsFunc: func() ([]*pages.Snippet, error) {
			return []*pages.Snippet{{Name: "Snip", Parameters: []*pages.SnippetParameter{{Name: "P"}}}}, nil
		}}
		sc := &pages.SnippetCallWidget{}
		err := pb.buildSnippetCallParams(sc, "M.Snip", []ast.SnippetCallParam{{ParamName: "P", Variable: tc.arg}})
		if tc.refused {
			if err == nil {
				t.Errorf("%s: built %+v, want the CE0115 refusal", tc.arg, sc.ParameterMappings)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", tc.arg, err)
		}
		if len(sc.ParameterMappings) != 1 || sc.ParameterMappings[0].IsLocalVariable {
			t.Errorf("%s: %+v, want one page-parameter mapping", tc.arg, sc.ParameterMappings)
		}
	}
}

// A data view is a source only for the widgets inside it. Reading through one
// that is closed — a sibling built earlier on the same page — was accepted and
// written, and mx check 11.13.0 rejected the page: CE7001 "Widget should be
// placed inside Data view 'dv1' to use it as a source widget."
func TestInputAttributeThroughClosedDataView(t *testing.T) {
	pb := scopedPB(false)
	dataView := func(name string, children ...*ast.WidgetV3) *ast.WidgetV3 {
		return &ast.WidgetV3{
			Name: name, Type: "dataview",
			Properties: map[string]any{"DataSource": &ast.DataSourceV3{Type: "parameter", Reference: "$Task"}},
			Children:   children,
		}
	}
	input := func(name, attr string) *ast.WidgetV3 {
		return &ast.WidgetV3{Name: name, Type: "textbox", Properties: map[string]any{"Attribute": attr}}
	}
	if _, err := pb.buildDataViewV3(dataView("dv1", input("tb1", "Title"))); err != nil {
		t.Fatalf("dv1: %v", err)
	}
	_, err := pb.buildDataViewV3(dataView("dv2", input("tb2", "$dv1.Title")))
	if err == nil || !strings.Contains(err.Error(), "data view") {
		t.Errorf("reading through a closed data view: err = %v, want a refusal", err)
	}
	// Control: an enclosing data view, one level out, still resolves.
	if _, err := pb.buildDataViewV3(dataView("dv3", dataView("dv4", input("tb3", "$dv3.Title")))); err != nil {
		t.Errorf("reading through an enclosing data view: %v", err)
	}
}
