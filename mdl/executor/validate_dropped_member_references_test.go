// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
)

// Dropping an attribute, an entity or an association that a microflow, nanoflow,
// page or snippet still uses. Measured on MxBuild 11.16.1, one project per case
// (CE1613 is an unresolved reference and hides every other error):
//
//	drop attribute used in a microflow `change`      CE1613 on the microflow
//	  ... in a page / snippet / nanoflow binding     CE1613 on each
//	  ... only in a retrieve's XPath                 CE0161 on the microflow
//	drop entity used as a microflow parameter        CE1613
//	drop association used by a retrieve or a page    CE1613
//	drop entity that only an association points at   clean (the association goes with it)
//	drop the attribute, then add it again            clean
//
// check passed every rejected case, exec applied it with at most a "may still
// reference" warning, and only `mx check` said so afterwards.
func droppedRefsCtx(t *testing.T, edges ...[4]string) *ExecContext {
	t.Helper()
	cat, err := catalog.New()
	if err != nil {
		t.Fatalf("catalog.New: %v", err)
	}
	t.Cleanup(func() { cat.Close() })
	for _, e := range edges { // sourceType, sourceName, targetName, refKind
		if _, err := cat.CatalogDB().Exec(
			`INSERT INTO refs (SourceType, SourceId, SourceName, TargetType, TargetId, TargetName, RefKind, ProjectId, SnapshotId)
			 VALUES (?, '', ?, 'ATTRIBUTE', '', ?, ?, 'p', 's')`, e[0], e[1], e[2], e[3]); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return &ExecContext{Catalog: cat, Output: &bytes.Buffer{}}
}

func TestCheckDroppedMemberReferences(t *testing.T) {
	edges := [][4]string{
		{"MICROFLOW", "Shop.SUB_Recalc", "Shop.Product.Code", "member"},
		{"SNIPPET", "Shop.SNIP_Card", "Shop.Product.Code", "member"},
		{"PAGE", "Shop.Pg", "Shop.Product.Code", "member"},
		{"MICROFLOW", "Shop.ByCode", "Shop.Product.Code", "xpath"},
		{"MICROFLOW", "Shop.TakesProduct", "Shop.Product", "parameter"},
		{"ASSOCIATION", "Shop.Line_Product", "Shop.Product", "associate"},
		{"MICROFLOW", "Shop.Walk", "Shop.Line_Product", "retrieve"},
		{"PAGE", "Shop.LinePg", "Shop.Line_Product", "member"},
		{"MICROFLOW", "Shop.Untouched", "Shop.Other.Name", "member"},
	}
	cases := []struct {
		name, script string
		want         []string // sources the violation names; nil = none
	}{
		{"attribute used by a microflow, a snippet, a page and an XPath",
			`alter entity Shop.Product drop attribute "Code";`,
			[]string{"Shop.SUB_Recalc", "Shop.SNIP_Card", "Shop.Pg", "Shop.ByCode"}},
		{"attribute nothing uses (control)", `alter entity Shop.Product drop attribute "Name";`, nil},
		{"another entity's attribute (control)", `alter entity Shop.Other drop attribute "Zip";`, nil},
		{"the attribute name in another case",
			`alter entity Shop.Product drop attribute "code";`,
			[]string{"Shop.SUB_Recalc", "Shop.SNIP_Card", "Shop.Pg", "Shop.ByCode"}},
		{"entity used as a parameter, its own association left out",
			`drop entity Shop.Product;`,
			[]string{"Shop.SUB_Recalc", "Shop.SNIP_Card", "Shop.Pg", "Shop.ByCode", "Shop.TakesProduct"}},
		{"association used by a retrieve and a page",
			`drop association Shop.Line_Product;`, []string{"Shop.Walk", "Shop.LinePg"}},
		{"attribute dropped and added again (control)",
			"alter entity Shop.Product drop attribute \"Code\";\nalter entity Shop.Product add attribute \"Code\": string(20);", nil},
		{"every user dropped in the same script (control)",
			"drop microflow Shop.SUB_Recalc;\ndrop microflow Shop.ByCode;\ndrop snippet Shop.SNIP_Card;\ndrop page Shop.Pg;\nalter entity Shop.Product drop attribute \"Code\";", nil},
		{"one user rewritten, the rest still reported",
			"drop microflow Shop.SUB_Recalc;\ndrop microflow Shop.ByCode;\ndrop snippet Shop.SNIP_Card;\nalter entity Shop.Product drop attribute \"Code\";",
			[]string{"Shop.Pg"}},
		{"entity dropped and created again (control)",
			"drop entity Shop.Product;\ncreate persistent entity Shop.Product (Code: string(20));", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := droppedRefsCtx(t, edges...)
			got := CheckDroppedMemberReferences(ctx, mustBuild(t, "mdl 1;\n"+c.script))
			if len(c.want) == 0 {
				if len(got) != 0 {
					t.Fatalf("want none, got %v: %+v", rulesOf(got), got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("want one violation, got %v", rulesOf(got))
			}
			v := got[0]
			if v.RuleID != droppedMemberRefRule {
				t.Errorf("rule = %s", v.RuleID)
			}
			for _, src := range c.want {
				if !strings.Contains(v.Message, src) {
					t.Errorf("message does not name %s: %s", src, v.Message)
				}
			}
			if strings.Contains(v.Message, "Shop.Untouched") {
				t.Errorf("message names a document that does not use the target: %s", v.Message)
			}
			if strings.HasPrefix(c.script, "drop entity") && strings.Contains(v.Message, "Shop.Line_Product") {
				t.Errorf("the association that goes with the entity is not a user to fix: %s", v.Message)
			}
		})
	}
}

// A source of a type the build was not measured on (a mapping, a workflow) is
// not named: the claim is "mxbuild reports CE1613 on it".
func TestCheckDroppedMemberReferences_UnmeasuredSourceTypes(t *testing.T) {
	ctx := droppedRefsCtx(t, [4]string{"IMPORT_MAPPING", "Shop.IM", "Shop.Product.Code", "member"})
	if got := CheckDroppedMemberReferences(ctx, mustBuild(t, "mdl 1;\nalter entity Shop.Product drop attribute \"Code\";")); len(got) != 0 {
		t.Errorf("a mapping is not a measured source: want none, got %+v", got)
	}
}

// Without a catalog nothing is judged.
func TestCheckDroppedMemberReferences_NoCatalog(t *testing.T) {
	if got := CheckDroppedMemberReferences(&ExecContext{}, mustBuild(t, "mdl 1;\ndrop entity Shop.Product;")); len(got) != 0 {
		t.Errorf("no catalog: want none, got %v", got)
	}
}
