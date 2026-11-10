// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// TestWidgets_ProjectsMicroflowNanoflowRef guards findings #35: CATALOG.WIDGETS
// records the action/datasource flow of a widget, but the linter's Widget
// projection dropped MicroflowRef/NanoflowRef, so a custom rule could not detect
// a microflow-datasource ListView (no database pushdown). The fields must now be
// carried through from the catalog into the Widget struct.
func TestWidgets_ProjectsMicroflowNanoflowRef(t *testing.T) {
	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatalf("NewFromFile: %v", err)
	}
	defer cat.Close()
	db := cat.CatalogDB()

	if _, err := db.Exec(
		`INSERT INTO modules_data (Id, Name, ProjectId, SnapshotId) VALUES (?,?,?,?)`,
		"mod-1", "Sales", "default", "s1",
	); err != nil {
		t.Fatalf("insert module: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO widgets_data
			(Id, Name, WidgetType, ContainerId, ContainerQualifiedName, ContainerType,
			 ModuleName, EntityRef, AttributeRef, MicroflowRef, NanoflowRef, ProjectId, SnapshotId)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"w-1", "lvOrders", "listview", "c-1", "Sales.Order_Overview", "page",
		"Sales", "Sales.Order", "", "Sales.DS_Orders", "", "default", "s1",
	); err != nil {
		t.Fatalf("insert widget: %v", err)
	}

	ctx := linter.NewLintContext(cat, nil)
	var found *linter.Widget
	for w := range ctx.Widgets() {
		if w.ID == "w-1" {
			ww := w
			found = &ww
			break
		}
	}
	if found == nil {
		t.Fatal("widget w-1 not returned by ctx.Widgets()")
	}
	if found.MicroflowRef != "Sales.DS_Orders" {
		t.Errorf("MicroflowRef = %q, want %q", found.MicroflowRef, "Sales.DS_Orders")
	}
	if found.NanoflowRef != "" {
		t.Errorf("NanoflowRef = %q, want empty", found.NanoflowRef)
	}
}

// TestWidgets_TreeAppearanceActionReachStarlark guards mendixlabs/mxcli#1268:
// widgets_data records each widget's parent, depth, appearance and primary
// action, and widgets() must hand every one of them to a Starlark rule under
// its documented snake_case name.
func TestWidgets_TreeAppearanceActionReachStarlark(t *testing.T) {
	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatalf("NewFromFile: %v", err)
	}
	defer cat.Close()
	db := cat.CatalogDB()
	if _, err := db.Exec(
		`INSERT INTO modules_data (Id, Name, ProjectId, SnapshotId) VALUES (?,?,?,?)`,
		"mod-1", "Sales", "default", "s1",
	); err != nil {
		t.Fatalf("insert module: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO widgets_data
			(Id, Name, WidgetType, ContainerId, ContainerQualifiedName, ContainerType,
			 ModuleName, PageRef, ParentWidgetId, Depth, Class, Style, DynamicClasses,
			 ActionType, HasConfirmation, ProjectId, SnapshotId)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"w-2", "btnDelete", "Forms$ActionButton", "c-1", "Sales.Order_Overview", "PAGE",
		"Sales", "Sales.Order_Edit", "w-1", 3, "btn-danger", "color: red;", "'x'",
		"Forms$MicroflowAction", 1, "default", "s1",
	); err != nil {
		t.Fatalf("insert widget: %v", err)
	}

	vs, _ := runSrc(t, linter.NewLintContext(cat, nil), `
def check():
    out = []
    for w in widgets():
        out.append(violation(message = "|".join([w.parent_widget_id, str(w.depth),
            w.class_name, w.style, w.dynamic_classes, w.action_type,
            str(w.has_confirmation), w.page_ref])))
    return out
`)
	if len(vs) != 1 {
		t.Fatalf("got %d violations, want 1", len(vs))
	}
	want := "w-1|3|btn-danger|color: red;|'x'|Forms$MicroflowAction|True|Sales.Order_Edit"
	if vs[0].Message != want {
		t.Errorf("projection = %q\nwant         %q", vs[0].Message, want)
	}
}

// ako/mxcli#1303: with two buttons on a page a rule could not tell which
// caption belonged to the one without a confirmation: widgets() had no caption,
// and the strings() caption row's element_id is the button's
// Forms$ClientTemplate, not the button. A row now carries caption and tooltip
// in the default language, and each strings() row the widget_id, so the
// Dutch caption is found by the widget's id.
func TestWidgets_CaptionTooltipAndTheirTranslations(t *testing.T) {
	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatalf("NewFromFile: %v", err)
	}
	defer cat.Close()
	db := cat.CatalogDB()
	for _, q := range []string{
		`INSERT INTO modules_data (Id, Name, ProjectId, SnapshotId) VALUES ('mod-1', 'Sales', 'default', 's1')`,
		`INSERT INTO widgets_data (Id, Name, WidgetType, ContainerId, ContainerQualifiedName, ContainerType,
			ModuleName, ActionType, HasConfirmation, Caption, Tooltip, ProjectId, SnapshotId) VALUES
			('b1', 'btnDelete', 'Forms$ActionButton', 'p', 'Sales.Sale_Edit', 'PAGE', 'Sales', 'Forms$MicroflowAction', 0, 'Delete sale', 'Deletes it', 'default', 's1'),
			('b2', 'btnRemove', 'Forms$ActionButton', 'p', 'Sales.Sale_Edit', 'PAGE', 'Sales', 'Forms$MicroflowAction', 1, 'Remove', '', 'default', 's1')`,
		`INSERT INTO strings (QualifiedName, ObjectType, StringValue, StringContext, Language, ElementId, ModuleName, WidgetId) VALUES
			('Sales.Sale_Edit', 'PAGE', 'Verkoop verwijderen', 'Forms$ClientTemplate.Template', 'nl_NL', 'tpl1', 'Sales', 'b1'),
			('Sales.Sale_Edit', 'PAGE', 'Weghalen', 'Forms$ClientTemplate.Template', 'nl_NL', 'tpl2', 'Sales', 'b2'),
			('Sales.Sale_Edit', 'PAGE', 'Deze verkoop verwijderen?', 'Forms$ConfirmationInfo.Question', 'nl_NL', 'conf2', 'Sales', 'b2')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	vs, _ := runSrc(t, linter.NewLintContext(cat, nil), `
def check():
    nl = {}
    for s in strings(language = "nl_NL"):
        nl.setdefault(s.widget_id, []).append(s.value)
    out = []
    for w in widgets():
        out.append(violation(message = "%s|%s|%s|%s|%s" % (w.name, w.caption, w.tooltip, w.has_confirmation, ",".join(sorted(nl.get(w.id, []))))))
    return out
`)
	var got []string
	for _, v := range vs {
		got = append(got, v.Message)
	}
	want := "btnDelete|Delete sale|Deletes it|False|Verkoop verwijderen;" +
		"btnRemove|Remove||True|Deze verkoop verwijderen?,Weghalen"
	if strings.Join(got, ";") != want {
		t.Errorf("got  %q\nwant %q", strings.Join(got, ";"), want)
	}
}
