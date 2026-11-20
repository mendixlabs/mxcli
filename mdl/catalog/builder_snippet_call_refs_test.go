// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#2017: a Forms$SnippetCall stores its snippet under "Form", the key
// a page action uses for its page, so every snippet call was recorded as a
// `show_page` edge to a PAGE. No refs row ever had TargetType SNIPPET: on
// FactoryManagement (11.16) 311 snippet calls read as page openings, and a
// query for "what uses this snippet" by type found nothing.

func snippetCall(id, name, snippet string) map[string]any {
	return wid(id, name, "Forms$SnippetCallWidget", map[string]any{
		"FormCall": map[string]any{"$ID": id + "-fc", "$Type": "Forms$SnippetCall", "Form": snippet},
	})
}

func TestSnippetCallIsASnippetEdge_Issue2017(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()

	// The report's case: a layout placing the tab bar in its bottom region.
	l := &pages.Layout{Name: "Lay_App", LayoutType: pages.LayoutTypeResponsive}
	l.ID, l.ContainerID = "lay", "mod-l"
	layoutRaw := map[string]any{
		"$ID": "lay", "$Type": "Forms$Layout", "Name": "Lay_App",
		"Content": map[string]any{
			"$ID": "content", "$Type": "Forms$WebLayoutContent", "LayoutType": "Responsive",
			"Widgets": bsonList(wid("ctn", "ctnBottom", "Forms$DivContainer", map[string]any{
				"Widgets": bsonList(snippetCall("sc1", "snippetCall1", "L.SNIP_Tabbar")),
			})),
		},
	}
	// A page that both calls a snippet and has a button opening a page — the
	// control: the real page opening must stay a show_page edge.
	pg := &pages.Page{Name: "Home"}
	pg.ID, pg.ContainerID = "pg", "mod-l"
	pageRaw := map[string]any{
		"$Type": "Forms$Page",
		"FormCall": map[string]any{
			"$Type": "Forms$LayoutCall", "Form": "L.Lay_App",
			"Arguments": bsonList(map[string]any{
				"$Type": "Forms$FormCallArgument",
				"Widgets": bsonList(
					snippetCall("sc2", "snippetCall2", "L.SNIP_Header"),
					wid("btn", "btnOpen", "Forms$ActionButton", map[string]any{
						"Action": map[string]any{"$ID": "act", "$Type": "Forms$FormAction",
							"FormSettings": map[string]any{"$ID": "fs", "$Type": "Forms$FormSettings", "Form": "L.Details"}},
					}),
				),
			}),
		},
	}

	b := layoutBuilder(t, cat, &mock.MockBackend{
		ListLayoutsFunc: func() ([]*pages.Layout, error) { return []*pages.Layout{l}, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
	}, map[model.ID]map[string]any{"lay": layoutRaw, "pg": pageRaw})
	if err := b.buildLayouts(); err != nil {
		t.Fatalf("buildLayouts: %v", err)
	}
	if err := b.buildPages(); err != nil {
		t.Fatalf("buildPages: %v", err)
	}
	if err := b.buildReferences(); err != nil {
		t.Fatalf("buildReferences: %v", err)
	}

	type edge struct{ sourceType, source, kind, targetType string }
	got := map[string][]edge{}
	rows, err := b.tx.Query(`SELECT TargetName, SourceType, SourceName, RefKind, TargetType FROM refs`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var target string
		var e edge
		if err := rows.Scan(&target, &e.sourceType, &e.source, &e.kind, &e.targetType); err != nil {
			t.Fatal(err)
		}
		got[target] = append(got[target], e)
	}
	rows.Close()

	for target, want := range map[string]edge{
		"L.SNIP_Tabbar": {"LAYOUT", "L.Lay_App", RefKindSnippetCall, RefObjectSnippet},
		"L.SNIP_Header": {"PAGE", "L.Home", RefKindSnippetCall, RefObjectSnippet},
		"L.Details":     {"PAGE", "L.Home", RefKindShowPage, RefObjectPage}, // control
	} {
		if len(got[target]) != 1 || got[target][0] != want {
			t.Errorf("refs to %s = %+v, want exactly [%+v]", target, got[target], want)
		}
	}

	// The widget row carries the snippet in its own column, not as a page.
	var pageRef, snippetRef string
	if err := b.tx.QueryRow(`SELECT COALESCE(PageRef, ''), COALESCE(SnippetRef, '') FROM widgets_data WHERE Name = 'snippetCall1'`).
		Scan(&pageRef, &snippetRef); err != nil {
		t.Fatal(err)
	}
	if pageRef != "" || snippetRef != "L.SNIP_Tabbar" {
		t.Errorf("snippetCall1: PageRef = %q, SnippetRef = %q; want \"\", \"L.SNIP_Tabbar\"", pageRef, snippetRef)
	}
}
