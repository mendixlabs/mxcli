// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/mprread"
)

// DESCRIBE PAGE loaded every page with the page's local name, in whatever
// module, and then picked the one in the module asked for. Names like
// Overview, NewEdit and Home repeat across the modules of a project, and a
// rewrite describes the stored page up to three times, so a script that
// re-runs its pages paid for every namesake each time (ako/mxcli#2160).
//
// Describing a page that has namesakes in four other modules must load no
// more units than describing one that has none.
func TestDescribePage_LoadsNoNamesakesFromOtherModules(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	var sb strings.Builder
	sb.WriteString("mdl 1;\n")
	page := func(module, name string) {
		fmt.Fprintf(&sb, "create module if not exists %s;\n", module)
		fmt.Fprintf(&sb, "create or modify page %s.%s (title: 'P', layout: Atlas_Core.Atlas_Default) {\n  layoutgrid g { row { column (desktopwidth: 12) { dynamictext t (content: 'x') } } }\n};\n", module, name)
	}
	page("Namesake1", "Lonely")
	for i := 1; i <= 5; i++ {
		page(fmt.Sprintf("Namesake%d", i), "Shared")
	}
	if err := runScript(t, exec, sb.String()); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	loadsFor := func(name string) int64 {
		// Once before counting, so both are measured warm.
		if err := agreeExec(t, exec, "describe page "+name+";"); err != nil {
			t.Fatalf("describe %s: %v", name, err)
		}
		before := mprread.DecodeCacheHits() + mprread.UnitsDecoded()
		out.Reset()
		if err := agreeExec(t, exec, "describe page "+name+";"); err != nil {
			t.Fatalf("describe %s: %v", name, err)
		}
		if !strings.Contains(out.String(), "create or modify page "+name) {
			t.Fatalf("describe %s printed something else:\n%s", name, out.String())
		}
		return mprread.DecodeCacheHits() + mprread.UnitsDecoded() - before
	}
	lonely, shared := loadsFor("Namesake1.Lonely"), loadsFor("Namesake1.Shared")
	t.Logf("units loaded: %d describing a page with no namesake, %d describing one with four", lonely, shared)
	if lonely == 0 {
		t.Fatal("control: describing a page loaded no unit, so the count is not measuring it")
	}
	if shared > lonely {
		t.Errorf("describing a page with four namesakes loaded %d units against %d without: the namesakes are loaded", shared, lonely)
	}
}
