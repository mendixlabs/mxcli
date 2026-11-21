// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#2461: widget placements mxbuild rejects in a layout passed check
// and exec. Every case is a shape measured with mx check 11.12.6; the error
// it produced is in the comment. The controls are shapes the same run built
// with 0 errors.
func layoutWidgetRules2461(t *testing.T, src string) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs, src)
	}
	var rules []string
	for _, s := range prog.Statements {
		for _, v := range validateLayoutWidgets(s) {
			rules = append(rules, v.RuleID)
		}
	}
	sort.Strings(rules)
	return rules
}

const toggleRegion2461 = `region left (Size: 260, SizeMode: 'Pixels', ToggleMode: 'SlideOverContent') { dynamictext t2 (Content: 'menu') }
    region center { placeholder Main }`

func TestLayoutWidgetPlacement_2461(t *testing.T) {
	cases := map[string]struct{ src, want string }{
		// CE6086 "Data views are not allowed on web layouts." — the report.
		"dataview in responsive layout": {`create layout M.L (layouttype: 'Responsive') {
  dataview dv (DataSource: microflow M.DS) { dynamictext t1 (Content: 'x') }
  placeholder Main
};`, "MDL-LAYOUT01"},
		// Same, nested in a container.
		"dataview nested in tablet layout": {`create layout M.L (layouttype: 'Tablet') {
  container c1 { dataview dv (DataSource: microflow M.DS) { dynamictext t1 (Content: 'x') } }
  placeholder Main
};`, "MDL-LAYOUT01"},
		// CE6086 "Data views are not allowed on native layouts."
		"dataview in native layout": {`create layout M.L (layouttype: 'Default') {
  dataview dv (DataSource: microflow M.DS) { dynamictext t1 (Content: 'x') }
  placeholder Main
};`, "MDL-LAYOUT01"},
		// CE0611 — the report.
		"toggling region without toggle": {`create layout M.L (layouttype: 'Responsive') {
  scrollcontainer layoutContainer {
    ` + toggleRegion2461 + `
  }
};`, "MDL-LAYOUT02"},
		// CE0611 + CE0628 on the snippet's toggle: a called snippet's toggle
		// does not count.
		"toggle only inside a called snippet": {`create layout M.L (layouttype: 'Responsive') {
  snippetcall sc1 (Snippet: M.SnToggle)
  scrollcontainer layoutContainer {
    ` + toggleRegion2461 + `
  }
};`, "MDL-LAYOUT02"},
		// CE0628: a container around the scroll container.
		"toggle with container on top": {`create layout M.L (layouttype: 'Responsive') {
  container hdr { sidebartoggle tg (Caption: 'Menu') }
  scrollcontainer layoutContainer {
    ` + toggleRegion2461 + `
  }
};`, "MDL-LAYOUT03"},
		// CE0628: the toggle beside the scroll container.
		"toggle beside scroll container": {`create layout M.L (layouttype: 'Responsive') {
  scrollcontainer layoutContainer {
    ` + toggleRegion2461 + `
  }
  sidebartoggle tg (Caption: 'Menu')
};`, "MDL-LAYOUT03"},
		// CE0628: no region toggles.
		"toggle with no toggling region": {`create layout M.L (layouttype: 'Responsive') {
  scrollcontainer layoutContainer {
    region top { sidebartoggle tg (Caption: 'Menu') }
    region center { placeholder Main }
  }
};`, "MDL-LAYOUT03"},
		// CE6086 "Sidebar toggles are not allowed on web pages."
		"toggle on a page": {`create page M.P (Layout: M.L) {
  placeholder Main { sidebartoggle tg (Caption: 'Menu') }
};`, "MDL-LAYOUT04"},
		// CE6086 "Sidebar toggles are not allowed on web snippets."
		"toggle in a snippet": {`create snippet M.S {
  sidebartoggle tg (Caption: 'Menu')
};`, "MDL-LAYOUT04"},

		// Controls, 0 errors in the same measurement.
		"toggle inside the scroll container": {`create layout M.L (layouttype: 'Responsive') {
  scrollcontainer layoutContainer {
    region top { sidebartoggle tg (Caption: 'Menu') }
    ` + toggleRegion2461 + `
  }
};`, ""},
		"phone layout with toggle": {`create layout M.L (layouttype: 'Phone') {
  scrollcontainer layoutContainer {
    region top { sidebartoggle tg (Caption: 'Menu') }
    region left (ToggleMode: 'PushContentAside') { dynamictext t2 (Content: 'menu') }
    region center { placeholder Main }
  }
};`, ""},
		"no toggle and no toggling region": {`create layout M.L (layouttype: 'Responsive') {
  scrollcontainer layoutContainer {
    region left (ToggleMode: 'None') { dynamictext t2 (Content: 'menu') }
    region center { placeholder Main }
  }
};`, ""},
		"page with a data view": {`create page M.P (Layout: M.L) {
  placeholder Main { dataview dv (DataSource: microflow M.DS) { dynamictext t1 (Content: 'x') } }
};`, ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got := strings.Join(layoutWidgetRules2461(t, c.src), ",")
			if got != c.want {
				t.Errorf("rules = %q, want %q", got, c.want)
			}
		})
	}
}

// The message names the mxbuild code and the platform the layout is for.
func TestLayoutDataViewMessageNamesPlatform_2461(t *testing.T) {
	for typ, want := range map[string]string{"Responsive": "web layouts", "Default": "native layouts"} {
		prog, errs := visitor.Build(`create layout M.L (layouttype: '` + typ + `') {
  dataview dv (DataSource: microflow M.DS) { dynamictext t1 (Content: 'x') }
  placeholder Main
};`)
		if len(errs) > 0 {
			t.Fatal(errs)
		}
		vs := validateLayoutWidgets(prog.Statements[0])
		if len(vs) != 1 || !strings.Contains(vs[0].Message, "CE6086") || !strings.Contains(vs[0].Message, want) {
			t.Errorf("%s: %+v, want CE6086 on %s", typ, vs, want)
		}
	}
}
