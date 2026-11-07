// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"path/filepath"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ako/mxcli#1302 (mendixlabs/mxcli#1311): a rule "no inline styles" could not
// see the page's own Style or any widget of a layout. The fixture is the
// issue's repro: page Sale_Frame (class 'frame', style 'color: red;') on layout
// Lay_Styled, whose container ctnLayoutBanner is styled.
func appearanceFixture(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatalf("NewFromFile: %v", err)
	}
	t.Cleanup(func() { cat.Close() })
	db := cat.CatalogDB()
	for _, q := range []string{
		`INSERT INTO modules_data (Id, Name, Source, ProjectId, SnapshotId) VALUES ('m', 'MyFirstModule', '', 'p', 's')`,
		`INSERT INTO pages_data (Id, Name, QualifiedName, ModuleName, Folder, LayoutRef, Class, Style, DynamicClasses, ProjectId, SnapshotId)
			VALUES ('pg', 'Sale_Frame', 'MyFirstModule.Sale_Frame', 'MyFirstModule', '', 'MyFirstModule.Lay_Styled',
			'frame', 'color: red;', '', 'p', 's')`,
		`INSERT INTO layouts_data (Id, Name, QualifiedName, ModuleName, Folder, LayoutType, Platform, Description,
			Class, Style, DynamicClasses, ProjectId, SnapshotId)
			VALUES ('lay', 'Lay_Styled', 'MyFirstModule.Lay_Styled', 'MyFirstModule', '', 'Responsive', 'Web', '',
			'layout-atlas', '', '', 'p', 's')`,
		`INSERT INTO widgets_data (Id, Name, WidgetType, ContainerId, ContainerQualifiedName, ContainerType, ModuleName,
			ParentWidgetId, Depth, Class, Style, ProjectId, SnapshotId)
			VALUES ('ctn', 'ctnLayoutBanner', 'Forms$DivContainer', 'lay', 'MyFirstModule.Lay_Styled', 'LAYOUT',
			'MyFirstModule', 'sc', 1, 'banner', 'padding: 4px;', 'p', 's')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	return cat
}

func TestStarlarkSeesPageAndLayoutAppearance_Issue1302(t *testing.T) {
	ctx := linter.NewLintContext(appearanceFixture(t), &minimalReader{})
	vs, _ := runSrc(t, ctx, `
def check():
    out = []
    for p in pages():
        out.append(violation(message="page %s class=%s style=%s dyn=%s" % (p.qualified_name, p.class_name, p.style, p.dynamic_classes)))
    for l in layouts():
        out.append(violation(message="layout %s class=%s style=%s dyn=%s" % (l.qualified_name, l.class_name, l.style, l.dynamic_classes)))
    for w in widgets():
        if w.container_type == "LAYOUT":
            out.append(violation(message="widget %s in %s class=%s style=%s" % (w.name, w.container_qualified_name, w.class_name, w.style)))
    return out
`)
	want := "page MyFirstModule.Sale_Frame class=frame style=color: red; dyn=\n" +
		"layout MyFirstModule.Lay_Styled class=layout-atlas style= dyn=\n" +
		"widget ctnLayoutBanner in MyFirstModule.Lay_Styled class=banner style=padding: 4px;"
	if got := messages(vs); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// The appearance columns are written by a full build only, so reading one
// must raise the rule's catalog mode — under a fast build every page would
// read as unstyled and the rule would pass clean.
func TestAppearanceFieldsRequireFullCatalog(t *testing.T) {
	ctx := linter.NewLintContext(appearanceFixture(t), &minimalReader{})
	for _, src := range []string{
		"def check():\n    return [violation(message=p.style) for p in pages() if p.style]\n",
		"def check():\n    return [violation(message=l.class_name) for l in layouts() if l.class_name]\n",
		"def check():\n    return [violation(message=p.dynamic_classes) for p in pages() if p.dynamic_classes]\n",
	} {
		_, r := runSrc(t, ctx, src)
		if got := r.RequiredCatalogMode(); got != linter.CatalogFull {
			t.Errorf("RequiredCatalogMode = %v, want full, for:\n%s", got, src)
		}
	}
}
