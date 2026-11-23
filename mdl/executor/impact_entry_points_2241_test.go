// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#2241 (mendixlabs/mxcli#1855): a page or microflow reachable only by
// its URL had no inbound edge, so `show impact` said nothing pointed at it and
// graph_dead_assets listed it as unused — the cue to delete something users
// open by its link.
func TestImpactNamesURLEntryPoints_2241(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	const setup = "mdl 1;\ncreate module E2241;\n" +
		"create page E2241.Reports (Title: 'Reports', Layout: Atlas_Core.Atlas_Default, Url: 'reports') {\n" +
		"  dynamictext t (Content: 'x')\n};\n" +
		"create page E2241.Orphan (Title: 'Orphan', Layout: Atlas_Core.Atlas_Default) {\n" +
		"  dynamictext t (Content: 'x')\n};\n" +
		"create microflow E2241.ACT_Export ()\nurl 'export'\nbegin\n  log info node 'E' 'x';\nend;\n"
	if err := agreeExec(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	if err := exec.Execute(&ast.RefreshCatalogStmt{Full: true, Force: true}); err != nil {
		t.Fatal(err)
	}
	ctx := exec.newExecContext(context.Background())
	for target, want := range map[string]string{
		"E2241.Reports":    "entry point: the page has URL 'reports'",
		"E2241.ACT_Export": "entry point: the microflow has URL 'export'",
	} {
		if got := noReferencesMessage(ctx, target); !strings.Contains(got, want) {
			t.Errorf("%s: %q, want it to say %q", target, got, want)
		}
	}
	// Control: a page with no URL gets the plain message.
	if got := noReferencesMessage(ctx, "E2241.Orphan"); strings.Contains(got, "entry point") {
		t.Errorf("E2241.Orphan has no URL, but: %q", got)
	}
	res, err := exec.catalog.Query("select QualifiedName from graph_dead_assets where ModuleName = 'E2241' order by 1")
	if err != nil {
		t.Fatal(err)
	}
	var dead []string
	for _, r := range res.Rows {
		dead = append(dead, refCell2529(r[0]))
	}
	if got := strings.Join(dead, ","); got != "E2241.Orphan" {
		t.Errorf("graph_dead_assets for E2241 = %q, want only E2241.Orphan (the URL entry points are used)", got)
	}
}
