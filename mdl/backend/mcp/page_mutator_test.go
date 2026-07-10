// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

func newTestMutator() *mcpPageMutator {
	content := map[string]any{
		"layout":     "Atlas_Core.Atlas_Default",
		"parameters": []any{map[string]any{"name": "Acct", "entity": "Sales.Account"}},
		"widgets": []any{
			map[string]any{"$Type": "Pages$Content", "slot": "Main", "widgets": []any{
				map[string]any{
					"$Type":      "Pages$DataView",
					"name":       "dv",
					"dataSource": map[string]any{"$Type": "Pages$DataViewSource", "entityRef": map[string]any{"entity": "Sales.Order"}},
					"widgets": []any{
						map[string]any{"$Type": "Pages$DynamicText", "name": "t1"},
					},
				},
			}},
		},
	}
	return &mcpPageMutator{backend: &Backend{}, moduleName: "Sales", pageName: "P", content: content}
}

func dvChildNames(m *mcpPageMutator) []string {
	_, _, _, dv, _ := findWidget(m.content, "dv")
	arr, _ := dv["widgets"].([]any)
	out := make([]string, 0, len(arr))
	for _, w := range arr {
		wm, _ := w.(map[string]any)
		if n, _ := wm["name"].(string); n != "" {
			out = append(out, n)
		}
	}
	return out
}

func dynText(name string) *pages.DynamicText {
	dt := &pages.DynamicText{Content: &pages.ClientTemplate{Template: &model.Text{Translations: map[string]string{"en_US": name}}}}
	dt.Name = name
	return dt
}

func TestPageMutator_InsertAfterBefore(t *testing.T) {
	m := newTestMutator()
	// The executor passes the AST token "AFTER"/"BEFORE" (uppercase).
	if err := m.InsertWidget("t1", "", backend.InsertPosition("AFTER"), []pages.Widget{dynText("t2")}); err != nil {
		t.Fatal(err)
	}
	if got := dvChildNames(m); len(got) != 2 || got[0] != "t1" || got[1] != "t2" {
		t.Fatalf("after AFTER insert: %v (want [t1 t2])", got)
	}
	if err := m.InsertWidget("t1", "", backend.InsertPosition("BEFORE"), []pages.Widget{dynText("t0")}); err != nil {
		t.Fatal(err)
	}
	if got := dvChildNames(m); len(got) != 3 || got[0] != "t0" || got[1] != "t1" || got[2] != "t2" {
		t.Fatalf("after BEFORE insert: %v (want [t0 t1 t2])", got)
	}
}

func TestPageMutator_ReplaceAndDrop(t *testing.T) {
	m := newTestMutator()
	_ = m.InsertWidget("t1", "", backend.InsertPosition("AFTER"), []pages.Widget{dynText("t2")})
	if err := m.ReplaceWidget("t2", "", []pages.Widget{dynText("t2b")}); err != nil {
		t.Fatal(err)
	}
	if got := dvChildNames(m); len(got) != 2 || got[1] != "t2b" {
		t.Fatalf("after replace: %v (want [t1 t2b])", got)
	}
	if err := m.DropWidget([]backend.WidgetRef{{Widget: "t1"}}); err != nil {
		t.Fatal(err)
	}
	if got := dvChildNames(m); len(got) != 1 || got[0] != "t2b" {
		t.Fatalf("after drop: %v (want [t2b])", got)
	}
}

func TestPageMutator_Introspection(t *testing.T) {
	m := newTestMutator()
	if !m.FindWidget("t1") || m.FindWidget("nope") {
		t.Fatal("FindWidget")
	}
	if scope := m.WidgetScope(); scope["dv"] == "" || scope["t1"] == "" {
		t.Fatalf("WidgetScope: %v", scope)
	}
	ids, ents := m.ParamScope()
	if ids["Acct"] == "" || ents["Acct"] != "Sales.Account" {
		t.Fatalf("ParamScope: %v / %v", ids, ents)
	}
	// t1 is inside dv, whose source entity is Sales.Order.
	if e := m.EnclosingEntity("t1"); e != "Sales.Order" {
		t.Fatalf("EnclosingEntity(t1) = %q, want Sales.Order", e)
	}
	if e := m.EnclosingEntityForChildren("dv"); e != "Sales.Order" {
		t.Fatalf("EnclosingEntityForChildren(dv) = %q, want Sales.Order", e)
	}
}

func TestPageMutator_SetLayoutAndUnsupported(t *testing.T) {
	m := newTestMutator()
	if err := m.SetLayout("Atlas_Core.Atlas_TopBar", nil); err != nil {
		t.Fatal(err)
	}
	if m.content["layout"] != "Atlas_Core.Atlas_TopBar" {
		t.Fatalf("layout: %v", m.content["layout"])
	}
	// Known properties map to pg keys (case-insensitive); unknown ones are rejected.
	if err := m.SetWidgetProperty("t1", "Class", "hl"); err != nil {
		t.Fatalf("SetWidgetProperty Class: %v", err)
	}
	if _, _, _, w, _ := findWidget(m.content, "t1"); w["appearance"].(map[string]any)["class"] != "hl" {
		t.Fatalf("Class not applied: %+v", w["appearance"])
	}
	if err := m.SetWidgetProperty("t1", "caption", "Hello"); err != nil {
		t.Fatalf("SetWidgetProperty caption (lowercase): %v", err)
	}
	if _, _, _, w, _ := findWidget(m.content, "t1"); w["ct:caption"] != "Hello" {
		t.Fatalf("caption not applied: %+v", w["ct:caption"])
	}
	if err := m.SetWidgetProperty("t1", "Bogus", "x"); err == nil {
		t.Error("unknown SET property should be rejected")
	}
	if err := m.AddVariable("v", "String", ""); err == nil {
		t.Error("AddVariable should be rejected")
	}
}

// Page-level SET Title (empty widgetRef) maps onto the pg LightPage's top-level
// "title" key — the same key CreatePage writes and pg_read_page returns.
func TestPageMutator_PageLevelSet_Title(t *testing.T) {
	m := newTestMutator()
	if err := m.SetWidgetProperty("", "Title", "Edit Expense"); err != nil {
		t.Fatalf("SET Title: %v", err)
	}
	if m.content["title"] != "Edit Expense" {
		t.Fatalf("title = %v, want \"Edit Expense\"", m.content["title"])
	}
	// Lowercase from the executor must work too.
	if err := m.SetWidgetProperty("", "title", "Lower"); err != nil {
		t.Fatalf("SET title (lowercase): %v", err)
	}
	if m.content["title"] != "Lower" {
		t.Fatalf("title = %v, want \"Lower\"", m.content["title"])
	}
	// A non-string Title is rejected (no silent coercion).
	if err := m.SetWidgetProperty("", "Title", 42); err == nil {
		t.Error("non-string Title should be rejected")
	}
}

// Url and pop-up settings are not part of the pg LightPage shape (its schema is
// additionalProperties:false), so they must reject clearly — before Save(), so
// they never poison the content tree — rather than report the misleading
// "widget \"\" not found" (issue #661).
func TestPageMutator_PageLevelSet_Unsupported(t *testing.T) {
	for _, prop := range []string{"Url", "PopupWidth", "PopupHeight", "PopupResizable", "PopupCloseAction"} {
		m := newTestMutator()
		err := m.SetWidgetProperty("", prop, 800)
		if err == nil {
			t.Fatalf("%s: expected rejection, got nil", prop)
		}
		if got := err.Error(); !contains(got, "page-level property") || !contains(got, "MCP backend") {
			t.Errorf("%s: unclear error %q", prop, got)
		}
		// The unsupported key must NOT have leaked into the content tree.
		if _, leaked := m.content[strings.ToLower(prop)]; leaked {
			t.Errorf("%s: rejected property leaked into content tree", prop)
		}
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
