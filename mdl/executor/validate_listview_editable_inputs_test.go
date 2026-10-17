// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ako/mxcli#631.
//
// Reported with runtime evidence: "List views are written with `Editable:
// false` by mxcli, so every input inside a list view renders disabled — even
// with `editable: Always` on the text box, and even inside a nested data view."
// The BSON is valid, `mxcli check` was clean and the build succeeds; it only
// shows in the rendered app.
//
// false is Mendix's own default (mendixmodelsdk 4.115.0: Pages$ListView
// `editable` is a PrimitiveProperty defaulting to false, and
// _initializeDefaultProperties does not override it), so the writer is right to
// write it. Studio Pro agrees: in ako/TestApp (Mendix 11.14.0) 30 of 32 list
// views are stored false and hold no input, and the one with inputs was set to
// true by its author — the `control: editable true` case below is that page's
// shape. What was missing is a diagnostic for the combination that is never
// meant: inputs a list view will render read-only.
func TestMDLWIDGET31_ListViewInputsNotEditable(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		{
			// The issue's reproduction, verbatim.
			name: "issue repro: textbox editable Always in a default list view",
			src: `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  listview lv (datasource: database M.Thing) {
    textbox t (label: 'N', attribute: Name, editable: Always)
  }
}`,
			want: 1,
		},
		{
			name: "input inside a nested data view",
			src: `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  listview lv (datasource: database M.Thing) {
    dataview dv (datasource: $currentObject) {
      checkbox cb (label: 'A', attribute: Active)
    }
  }
}`,
			want: 1,
		},
		{
			name: "explicit editable false",
			src: `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  listview lv (datasource: database M.Thing, editable: false) {
    textbox t (label: 'N', attribute: Name)
  }
}`,
			want: 1,
		},
		{
			// A quoted 'true' is a string, and buildListViewV3 reads the
			// property with GetBoolProp — so it is written false, exactly as if
			// it were absent. The rule reports what the writer does.
			name: "quoted 'true' is written false",
			src: `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  listview lv (datasource: database M.Thing, editable: 'true') {
    textbox t (label: 'N', attribute: Name)
  }
}`,
			want: 1,
		},
		{
			name: "control: editable true",
			src: `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  listview lv (datasource: database M.Thing, editable: true) {
    textbox t (label: 'N', attribute: Name, editable: Always)
  }
}`,
			want: 0,
		},
		{
			name: "control: display-only content",
			src: `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  listview lv (datasource: database M.Thing) {
    dynamictext dt (content: '{1}', contentparams: [{1} = Name])
  }
}`,
			want: 0,
		},
		{
			name: "control: every input is explicitly Never",
			src: `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  listview lv (datasource: database M.Thing) {
    textbox t (label: 'N', attribute: Name, editable: Never)
  }
}`,
			want: 0,
		},
		{
			// `editable: false` and the quoted spellings are Never as well: the
			// author already made these inputs read-only, so there is nothing to warn about.
			name: "control: inputs marked editable false or 'Never'",
			src: `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  listview lv (datasource: database M.Thing) {
    textbox t (label: 'N', attribute: Name, editable: false)
    textbox u (label: 'M', attribute: Name, editable: 'Never')
  }
}`,
			want: 0,
		},
		{
			// A condition is not Never: the input can still be editable, so the list view still decides.
			name: "an input with a quoted condition is still warned about",
			src: `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  listview lv (datasource: database M.Thing) {
    textbox t (label: 'N', attribute: Name, editable: '$currentObject/Name = empty')
  }
}`,
			want: 1,
		},
		{
			// A nested list view's inputs are governed by the NESTED list
			// view's Editable, so the outer one is not the one to report.
			name: "nested editable list view owns its own inputs",
			src: `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  listview outer (datasource: database M.Thing) {
    listview inner (datasource: database M.Thing, editable: true) {
      textbox t (label: 'N', attribute: Name)
    }
  }
}`,
			want: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := widgetViolations(t, tc.src, "MDL-WIDGET31")
			if len(got) != tc.want {
				t.Fatalf("MDL-WIDGET31: got %d violation(s), want %d: %#v", len(got), tc.want, got)
			}
			if tc.want == 0 {
				return
			}
			msg := got[0].Message
			for _, s := range []string{"Editable", "read-only"} {
				if !strings.Contains(msg, s) {
					t.Errorf("message should mention %q: %s", s, msg)
				}
			}
			if !strings.Contains(got[0].Suggestion, "editable: true") {
				t.Errorf("suggestion should name the fix `editable: true`: %s", got[0].Suggestion)
			}
		})
	}
}
