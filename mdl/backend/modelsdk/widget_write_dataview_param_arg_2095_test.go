// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#2095: a flow argument naming a data view that shows a page or
// snippet parameter is the pair Widget + PageParameter / SnippetParameter, the
// shape the reporter hand-patched to mx check 0 errors. Widget alone is CE0115.
func TestFlowCallWritesDataViewParameterPair(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		param, slot           string
		wantPage, wantSnippet string
	}{
		{"page parameter", "Item", "", "Item", ""},
		{"snippet parameter", "Item", "snippet", "", "Item"},
		{"no parameter (control)", "", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mf, err := clientActionToGen(&pages.MicroflowClientAction{
				BaseElement:   model.BaseElement{ID: "a"},
				MicroflowName: "ModV.ACT_Open",
				ParameterMappings: []*pages.MicroflowParameterMapping{{
					ParameterName: "Item", Variable: "$dvOuter", VariableKind: "widget",
					WidgetParameter: tc.param, WidgetParameterKind: tc.slot,
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			nf, err := clientActionToGen(&pages.NanoflowClientAction{
				BaseElement:  model.BaseElement{ID: "b"},
				NanoflowName: "ModV.NF_Open",
				ParameterMappings: []*pages.NanoflowParameterMapping{{
					ParameterName: "Item", Variable: "$dvOuter", VariableKind: "widget",
					WidgetParameter: tc.param, WidgetParameterKind: tc.slot,
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			mfm := mf.(*genPg.MicroflowClientAction).MicroflowSettings().(*genPg.MicroflowSettings).ParameterMappingsItems()[0].(*genPg.MicroflowParameterMapping)
			nfm := nf.(*genPg.CallNanoflowClientAction).ParameterMappingsItems()[0].(*genPg.NanoflowParameterMapping)
			for kind, v := range map[string]any{"microflow": mfm.Variable(), "nanoflow": nfm.Variable()} {
				pv, ok := v.(*genPg.PageVariable)
				if !ok {
					t.Fatalf("%s: Variable = %T", kind, v)
				}
				if pv.WidgetQualifiedName() != "dvOuter" || pv.PageParameterQualifiedName() != tc.wantPage ||
					pv.SnippetParameterQualifiedName() != tc.wantSnippet {
					t.Errorf("%s: PageVariable widget=%q page=%q snippet=%q, want dvOuter/%q/%q", kind,
						pv.WidgetQualifiedName(), pv.PageParameterQualifiedName(), pv.SnippetParameterQualifiedName(),
						tc.wantPage, tc.wantSnippet)
				}
			}
		})
	}
}
