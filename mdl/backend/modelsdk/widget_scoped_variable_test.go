// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#826. Three bindings the writer put in the wrong Forms$PageVariable
// slot, or dropped:
//
//   - an input widget inside a data view whose attribute is read through that
//     data view: Studio Pro stores {Widget: dataView1, PageParameter: Account}
//     (PedApp Administration.Account_Edit/Account_New, TestApp
//     WorkflowComment_Edit_Admin). The writer had no field for it and wrote null;
//   - a dynamic-text parameter `{1} = $dataView1.Name`: the widget name went
//     into PageParameter, a page parameter that does not exist. Given only
//     {widget: dataView1}, Studio Pro 11.14's own model fills in the data
//     view's page parameter beside it — measured through its MCP server;
//   - a snippet call passing a page `Variables:` entry: written to PageParameter.

func pageVariableSlots(t *testing.T, pv any) map[string]any {
	t.Helper()
	d, ok := pv.(bson.D)
	if !ok {
		t.Fatalf("SourceVariable = %#v, want a Forms$PageVariable document", pv)
	}
	out := map[string]any{}
	for _, k := range []string{"Widget", "PageParameter", "SnippetParameter", "LocalVariable"} {
		out[k] = docGet(d, k)
	}
	return out
}

func TestInputWidgetScopedSourceVariable(t *testing.T) {
	for _, tc := range []struct {
		name string
		w    func(sv *pages.WidgetVariable) pages.Widget
	}{
		{"textbox", func(sv *pages.WidgetVariable) pages.Widget {
			return &pages.TextBox{BaseWidget: pages.BaseWidget{Name: "tb"}, AttributePath: "M.Account.FullName", SourceVariable: sv}
		}},
		{"textarea", func(sv *pages.WidgetVariable) pages.Widget {
			return &pages.TextArea{BaseWidget: pages.BaseWidget{Name: "ta"}, AttributePath: "M.Account.FullName", SourceVariable: sv}
		}},
		{"checkbox", func(sv *pages.WidgetVariable) pages.Widget {
			return &pages.CheckBox{BaseWidget: pages.BaseWidget{Name: "cb"}, AttributePath: "M.Account.Active", SourceVariable: sv}
		}},
		{"datepicker", func(sv *pages.WidgetVariable) pages.Widget {
			return &pages.DatePicker{BaseWidget: pages.BaseWidget{Name: "dp"}, AttributePath: "M.Account.Born", SourceVariable: sv}
		}},
		{"dropdown", func(sv *pages.WidgetVariable) pages.Widget {
			return &pages.DropDown{BaseWidget: pages.BaseWidget{Name: "dd"}, AttributePath: "M.Account.Kind", SourceVariable: sv}
		}},
		{"radiobuttons", func(sv *pages.WidgetVariable) pages.Widget {
			return &pages.RadioButtons{BaseWidget: pages.BaseWidget{Name: "rb"}, AttributePath: "M.Account.Kind", SourceVariable: sv}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := encodeWidget(t, tc.w(&pages.WidgetVariable{Widget: "dataView1", Variable: "Account"}))
			got := pageVariableSlots(t, docGet(doc, "SourceVariable"))
			want := map[string]any{"Widget": "dataView1", "PageParameter": "Account", "SnippetParameter": "", "LocalVariable": ""}
			for k, v := range want {
				if got[k] != v {
					t.Errorf("SourceVariable.%s = %v, want %q", k, got[k], v)
				}
			}
			// Control: no widget-scoped variable keeps the null Studio Pro stores
			// on an input bound to the enclosing context.
			if sv := docGet(encodeWidget(t, tc.w(nil)), "SourceVariable"); sv != nil {
				t.Errorf("without a SourceVariable the widget wrote %#v, want null", sv)
			}
		})
	}
}

func TestTemplateParameterWidgetSlot(t *testing.T) {
	encode := func(p *pages.ClientTemplateParameter) bson.D {
		t.Helper()
		out, err := (&codec.Encoder{}).Encode(clientTemplateParameterToGen(p))
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		var doc bson.D
		if err := bson.Unmarshal(out, &doc); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return doc
	}
	for _, tc := range []struct {
		name string
		p    pages.ClientTemplateParameter
		want map[string]any
	}{
		{"data view over a page parameter",
			pages.ClientTemplateParameter{AttributeRef: "M.Car.Brand", SourceWidget: "dataView1", SourceVariable: "Car"},
			map[string]any{"Widget": "dataView1", "PageParameter": "Car", "SnippetParameter": ""}},
		{"data view over a snippet parameter",
			pages.ClientTemplateParameter{AttributeRef: "M.Car.Brand", SourceWidget: "dataView1", SourceVariable: "Car", SourceVariableKind: "snippet"},
			map[string]any{"Widget": "dataView1", "PageParameter": "", "SnippetParameter": "Car"}},
		{"data view with no variable source",
			pages.ClientTemplateParameter{AttributeRef: "M.Car.Brand", SourceWidget: "dataView1"},
			map[string]any{"Widget": "dataView1", "PageParameter": "", "SnippetParameter": ""}},
		// Control: a page parameter binding is unchanged.
		{"page parameter",
			pages.ClientTemplateParameter{AttributeRef: "M.Car.Brand", SourceVariable: "Car"},
			map[string]any{"Widget": "", "PageParameter": "Car", "SnippetParameter": ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := pageVariableSlots(t, docGet(encode(&tc.p), "SourceVariable"))
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("SourceVariable.%s = %v, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestSnippetCallMappingLocalVariableSlot(t *testing.T) {
	sc := &pages.SnippetCallWidget{
		SnippetName: "M.Snip",
		ParameterMappings: []pages.SnippetParamMapping{
			{ParamName: "Label", Argument: "$Caption", IsLocalVariable: true},
			{ParamName: "Car", Argument: "$Car"},
		},
	}
	sc.Name = "sc1"
	call, _ := docGet(encodeWidget(t, sc), "FormCall").(bson.D)
	mappings, _ := docGet(call, "ParameterMappings").(bson.A)
	if len(mappings) != 3 {
		t.Fatalf("ParameterMappings = %#v, want [marker, 2 mappings]", mappings)
	}
	for i, want := range []map[string]any{
		{"LocalVariable": "Caption", "PageParameter": ""},
		{"LocalVariable": "", "PageParameter": "Car"}, // control
	} {
		m, _ := mappings[i+1].(bson.D)
		got := pageVariableSlots(t, docGet(m, "Variable"))
		for k, v := range want {
			if got[k] != v {
				t.Errorf("mapping %d Variable.%s = %v, want %q", i+1, k, got[k], v)
			}
		}
	}
}

// ako/mxcli#1968: a data view's object passed by naming the data view is the
// Widget slot alone; in PageParameter it named a page parameter that does not
// exist, and mx check 11.14 reported CE0115.
func TestSnippetCallMappingWidgetSlot(t *testing.T) {
	sc := &pages.SnippetCallWidget{
		SnippetName: "M.Snip",
		ParameterMappings: []pages.SnippetParamMapping{
			{ParamName: "Item", Argument: "$dvOuter", IsWidget: true},
			{ParamName: "Car", Argument: "$Car"},
		},
	}
	sc.Name = "sc1"
	call, _ := docGet(encodeWidget(t, sc), "FormCall").(bson.D)
	mappings, _ := docGet(call, "ParameterMappings").(bson.A)
	if len(mappings) != 3 {
		t.Fatalf("ParameterMappings = %#v, want [marker, 2 mappings]", mappings)
	}
	for i, want := range []map[string]any{
		{"Widget": "dvOuter", "PageParameter": "", "SnippetParameter": "", "LocalVariable": ""},
		{"Widget": "", "PageParameter": "Car"}, // control
	} {
		m, _ := mappings[i+1].(bson.D)
		got := pageVariableSlots(t, docGet(m, "Variable"))
		for k, v := range want {
			if got[k] != v {
				t.Errorf("mapping %d Variable.%s = %v, want %q", i+1, k, got[k], v)
			}
		}
	}
}
