// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#1302 (mendixlabs/mxcli#1311): the catalog had no row for a widget
// of a layout and no column for a page's or layout's own Appearance, so a lint
// rule "no inline styles" passed a page with Style 'color: red;' and a layout
// with a styled container — the places whose styling reaches every page.

// layoutBuilder is a full-mode Builder over one module "L" whose raw units are
// served from raws.
func layoutBuilder(t *testing.T, cat *Catalog, m *mock.MockBackend, raws map[model.ID]map[string]any) *Builder {
	t.Helper()
	const modID = model.ID("mod-l")
	m.GetRawUnitFunc = func(id model.ID) (map[string]any, error) { return raws[id], nil }
	m.GetNavigationFunc = func() (*types.NavigationDocument, error) { return &types.NavigationDocument{}, nil }
	tx, err := cat.CatalogDB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	return &Builder{
		catalog:  cat,
		reader:   m,
		snapshot: &Snapshot{ID: "snap"},
		hierarchy: &hierarchy{
			moduleIDs:       map[model.ID]bool{modID: true},
			moduleNames:     map[model.ID]string{modID: "L"},
			containerParent: map[model.ID]model.ID{},
			folderNames:     map[model.ID]string{},
		},
		tx:       wrapTx(tx),
		fullMode: true,
	}
}

func appearance(class, style, dyn string) map[string]any {
	return map[string]any{"$ID": "ap", "$Type": "Forms$Appearance", "Class": class, "Style": style, "DynamicClasses": dyn}
}

// The issue's repro layout: a scroll container whose center region holds a
// styled container around the Main placeholder, plus a button that is the only
// caller of a microflow.
func styledLayoutRaw() map[string]any {
	return map[string]any{
		"$ID": "lay", "$Type": "Forms$Layout", "Name": "Lay_Styled",
		"Appearance": appearance("layout-atlas", "min-height: 100%;", "'x'"),
		"Content": map[string]any{
			"$ID": "content", "$Type": "Forms$WebLayoutContent", "LayoutType": "Responsive",
			"Widgets": bsonList(wid("sc", "layoutContainer", "Forms$ScrollContainer", map[string]any{
				"CenterRegion": map[string]any{
					"$ID": "rc", "$Type": "Forms$ScrollContainerRegion",
					"Widgets": bsonList(wid("ctn", "ctnLayoutBanner", "Forms$DivContainer", map[string]any{
						"Appearance": appearance("banner", "padding: 4px;", ""),
						"Widgets": bsonList(
							wid("ph", "Main", "Forms$Placeholder", nil),
							wid("btn", "btnLogout", "Forms$ActionButton", map[string]any{
								"Action": map[string]any{"$ID": "act", "$Type": "Forms$MicroflowAction",
									"MicroflowSettings": map[string]any{"$ID": "ms", "$Type": "Forms$MicroflowSettings", "Microflow": "L.ACT_Logout"}},
							}),
						),
					})),
				},
			})),
		},
	}
}

func TestLayoutWidgetsAreIndexed_Issue1302(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	l := &pages.Layout{Name: "Lay_Styled", LayoutType: pages.LayoutTypeResponsive}
	l.ID, l.ContainerID = "lay", "mod-l"
	b := layoutBuilder(t, cat, &mock.MockBackend{
		ListLayoutsFunc: func() ([]*pages.Layout, error) { return []*pages.Layout{l}, nil },
	}, map[model.ID]map[string]any{"lay": styledLayoutRaw()})
	if err := b.buildLayouts(); err != nil {
		t.Fatalf("buildLayouts: %v", err)
	}
	if err := b.buildReferences(); err != nil {
		t.Fatalf("buildReferences: %v", err)
	}

	type row struct{ typ, containerType, container, parent, class, style string }
	got := map[string]row{}
	rows, err := b.tx.Query(`SELECT Name, WidgetType, ContainerType, ContainerQualifiedName,
		COALESCE(ParentWidgetId, ''), COALESCE(Class, ''), COALESCE(Style, '') FROM widgets_data`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name string
		var r row
		if err := rows.Scan(&name, &r.typ, &r.containerType, &r.container, &r.parent, &r.class, &r.style); err != nil {
			t.Fatal(err)
		}
		got[name] = r
	}
	rows.Close()
	banner, ok := got["ctnLayoutBanner"]
	if !ok {
		t.Fatalf("ctnLayoutBanner not indexed; widgets_data has %v", got)
	}
	if want := (row{"Forms$DivContainer", "LAYOUT", "L.Lay_Styled", "sc", "banner", "padding: 4px;"}); banner != want {
		t.Errorf("ctnLayoutBanner = %+v, want %+v", banner, want)
	}
	for _, name := range []string{"layoutContainer", "btnLogout"} {
		if got[name].containerType != "LAYOUT" {
			t.Errorf("%s: ContainerType = %q, want LAYOUT", name, got[name].containerType)
		}
	}

	// The layout's own Appearance.
	var class, style, dyn string
	if err := b.tx.QueryRow(`SELECT Class, Style, DynamicClasses FROM layouts_data WHERE Id = 'lay'`).
		Scan(&class, &style, &dyn); err != nil {
		t.Fatal(err)
	}
	if class != "layout-atlas" || style != "min-height: 100%;" || dyn != "'x'" {
		t.Errorf("layout appearance = %q / %q / %q", class, style, dyn)
	}

	// A microflow only a layout's button calls has a caller: the layout.
	var src, srcName string
	if err := b.tx.QueryRow(`SELECT SourceType, SourceName FROM refs WHERE TargetName = 'L.ACT_Logout'`).
		Scan(&src, &srcName); err != nil {
		t.Fatalf("no reference to L.ACT_Logout: %v", err)
	}
	if src != "LAYOUT" || srcName != "L.Lay_Styled" {
		t.Errorf("ref source = %s %s, want LAYOUT L.Lay_Styled", src, srcName)
	}
}

// A layout built on another layout keeps its widgets in the call's arguments,
// like a page; a native layout adds the header placeholder and the sidebar.
func TestExtractLayoutWidgets_NestedAndNative(t *testing.T) {
	nested := map[string]any{
		"Content": map[string]any{
			"$Type": "Forms$WebLayoutContent",
			"LayoutCall": map[string]any{
				"$Type": "Forms$LayoutCall", "Form": "Atlas_Core.Atlas_Default",
				"Arguments": bsonList(map[string]any{
					"$Type": "Forms$FormCallArgument", "Parameter": "Atlas_Core.Atlas_Default.Main",
					"Widgets": bsonList(wid("c1", "ctnInner", "Forms$DivContainer", nil)),
				}),
			},
		},
	}
	native := map[string]any{
		"Content": map[string]any{
			"$Type":                  "Forms$NativeLayoutContent",
			"Widgets":                bsonList(wid("n1", "ctnNative", "Forms$DivContainer", nil)),
			"RightHeaderPlaceholder": wid("n2", "HeaderRight", "Forms$Placeholder", nil),
			"Sidebar":                true,
			"SidebarWidgets":         bsonList(wid("n3", "txtSidebar", "Forms$DynamicText", nil)),
		},
	}
	for name, tc := range map[string]struct {
		raw  map[string]any
		want []string
	}{
		"nested": {nested, []string{"ctnInner"}},
		"native": {native, []string{"ctnNative", "HeaderRight", "txtSidebar"}},
	} {
		got := map[string]bool{}
		for _, w := range extractLayoutWidgets(tc.raw) {
			got[w.Name] = true
		}
		for _, w := range tc.want {
			if !got[w] {
				t.Errorf("%s: %s not extracted (got %v)", name, w, got)
			}
		}
	}
}

// The page's own Appearance: Class, Style and DynamicClasses.
func TestPageAppearanceColumns_Issue1302(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	pg := &pages.Page{Name: "Sale_Frame"}
	pg.ID, pg.ContainerID = "pg", "mod-l"
	b := layoutBuilder(t, cat, &mock.MockBackend{
		ListPagesFunc: func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
	}, map[model.ID]map[string]any{"pg": {
		"$Type":      "Forms$Page",
		"Appearance": appearance("frame", "color: red;", "if $x then 'a' else ''"),
	}})
	if err := b.buildPages(); err != nil {
		t.Fatalf("buildPages: %v", err)
	}
	var class, style, dyn string
	if err := b.tx.QueryRow(`SELECT Class, Style, DynamicClasses FROM pages_data WHERE Id = 'pg'`).
		Scan(&class, &style, &dyn); err != nil {
		t.Fatal(err)
	}
	if class != "frame" || style != "color: red;" || dyn != "if $x then 'a' else ''" {
		t.Errorf("page appearance = %q / %q / %q", class, style, dyn)
	}
}
