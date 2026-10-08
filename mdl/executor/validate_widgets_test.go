// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// Issue #650 — MDL-WIDGET04 flags a dynamictext whose template references a {N}
// placeholder with no matching parameter binding (orphaned ClientTemplate).
func TestValidateDynamicTextPlaceholders(t *testing.T) {
	dt := func(props map[string]any) *ast.WidgetV3 {
		return &ast.WidgetV3{Type: "dynamictext", Name: "txt", Properties: props}
	}
	cases := []struct {
		name    string
		widget  *ast.WidgetV3
		wantBad bool
	}{
		{"orphan {1}", dt(map[string]any{"Content": "{1}"}), true},
		{"orphan {2} with one param", dt(map[string]any{
			"Content":       "Hi {1} {2}",
			"ContentParams": []ast.ParamAssignmentV3{{Value: "Name"}},
		}), true},
		{"bound via Attribute", dt(map[string]any{"Content": "{1}", "Attribute": "Title"}), false},
		{"bound via ContentParams", dt(map[string]any{
			"Content":       "{1}",
			"ContentParams": []ast.ParamAssignmentV3{{Value: "Title"}},
		}), false},
		{"static text, no placeholder", dt(map[string]any{"Content": "Hello"}), false},
		{"empty content (no AST placeholder)", dt(map[string]any{}), false},
		{"not a dynamictext", &ast.WidgetV3{Type: "textbox", Name: "tb", Properties: map[string]any{"Content": "{1}"}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := validateDynamicTextPlaceholders(c.widget, "page X")
			if c.wantBad && v == nil {
				t.Errorf("expected MDL-WIDGET04 violation, got none")
			}
			if !c.wantBad && v != nil {
				t.Errorf("unexpected violation: %s", v.Message)
			}
			if v != nil && v.RuleID != "MDL-WIDGET04" {
				t.Errorf("RuleID = %s, want MDL-WIDGET04", v.RuleID)
			}
			if c.wantBad && v != nil && !strings.Contains(v.Message, "orphaned placeholder") {
				t.Errorf("message lacks guidance: %s", v.Message)
			}
		})
	}
}

// TestValidateStaticWidgetUnknownProps covers MDL-WIDGET07: a property a core
// widget doesn't consume is warned (not errored) so the silent drop is visible.
func TestValidateStaticWidgetUnknownProps(t *testing.T) {
	dt := func(props map[string]any) *ast.WidgetV3 {
		return &ast.WidgetV3{Type: "dynamictext", Name: "txt", Properties: props}
	}
	cases := []struct {
		name      string
		widget    *ast.WidgetV3
		wantCount int
		wantHint  string // substring expected in the message, if any
	}{
		{"known props clean", dt(map[string]any{
			"Content": "hi", "Class": "c", "DynamicClasses": "x", "RenderMode": "H1",
		}), 0, ""},
		{"describe vocabulary clean (image units)", &ast.WidgetV3{
			Type: "image", Name: "img",
			Properties: map[string]any{"WidthUnit": "pixels", "Width": 36, "HeightUnit": "pixels", "Height": 36},
		}, 0, ""},
		{"lowercase keyword clean", dt(map[string]any{"content": "hi", "dynamicclasses": "x"}), 0, ""},
		{"unknown property warns", dt(map[string]any{"Content": "hi", "TotallyMadeUp": "x"}), 1, ""},
		{"typo suggests nearest", dt(map[string]any{"Contnet": "hi"}), 1, "did you mean `Content`"},
		// A data view's ShowFooter is read by the dataview builder and emitted by
		// describe page; it must not be reported as silently dropped.
		{"dataview showFooter clean", &ast.WidgetV3{
			Type: "dataview", Name: "dvOrder",
			Properties: map[string]any{"DataSource": &ast.DataSourceV3{Type: "parameter", Reference: "Order"}, "showFooter": true},
		}, 0, ""},
		{"dataview ShowFooter clean", &ast.WidgetV3{
			Type: "dataview", Name: "dvOrder",
			Properties: map[string]any{"ShowFooter": false},
		}, 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vs := validateStaticWidgetUnknownProps(c.widget, "page X")
			if len(vs) != c.wantCount {
				t.Fatalf("got %d violations, want %d: %+v", len(vs), c.wantCount, vs)
			}
			for _, v := range vs {
				if v.RuleID != "MDL-WIDGET07" {
					t.Errorf("RuleID = %s, want MDL-WIDGET07", v.RuleID)
				}
				if v.Severity != linter.SeverityWarning {
					t.Errorf("severity = %v, want warning (must not hard-fail check)", v.Severity)
				}
			}
			if c.wantHint != "" && (len(vs) == 0 || !strings.Contains(vs[0].Message, c.wantHint)) {
				t.Errorf("expected hint %q in message, got %+v", c.wantHint, vs)
			}
		})
	}
}

// TestStaticWidgetKnownPropsCoverDescribe guards against describe-vocabulary
// drift: every property describe page can emit must be recognized, otherwise the
// describe→create roundtrip would self-warn. Representative sample of the
// describe emit set (native widgets + datagrid columns).
func TestStaticWidgetKnownPropsCoverDescribe(t *testing.T) {
	describeVocabulary := []string{
		"Action", "Alignment", "AlternativeText", "Attribute", "Attributes", "ButtonStyle",
		"Caption", "CaptionAttribute", "CaptionParams", "Class", "Collapsible", "ColumnWidth",
		"Content", "ContentParams", "DataSource", "DesignProperties", "DesktopColumns",
		"DisplayAs", "DynamicCellClass", "DynamicClasses", "Editable", "FilterType", "HeaderMode",
		"DefaultImage", "OnClickType",
		"Height", "HeightUnit", "Hidable", "Image", "ImageType", "ImageUrl", "Label", "LabelWidth",
		"OnClick", "PageSize", "Pagination", "PagingPosition", "PhoneColumns", "PhoneWidth",
		"ReadOnlyStyle", "RenderMode", "Responsive", "Selection", "ShowContentAs",
		"ShowFooter", "ShowPagingButtons", "Size", "Snippet", "Sortable", "Style", "TabletColumns",
		"TabletWidth", "Tooltip", "Visible", "Width", "WidthUnit",
	}
	for _, p := range describeVocabulary {
		if !isKnownStaticWidgetProp(p) {
			t.Errorf("describe emits %q but it is not in the MDL-WIDGET07 allow-list — the describe→create roundtrip would false-warn", p)
		}
	}
}

// Bug 3 — MDL-WIDGET09: a DataView cannot use a database data source either
// (a data view shows one object; database sources belong to list widgets).
// mxbuild rejects the legacy fallback with CE7007.
func TestValidateStaticWidget_DataViewDatabaseSource(t *testing.T) {
	dbDS := &ast.DataSourceV3{Type: "database", Reference: "M.Expense"}
	mfDS := &ast.DataSourceV3{Type: "microflow", Reference: "M.GetExpense"}

	cases := []struct {
		name   string
		widget *ast.WidgetV3
		want   bool // expect an MDL-WIDGET09 violation
	}{
		{"dataview + database → rejected", &ast.WidgetV3{Type: "dataview", Name: "dv", Properties: map[string]any{"DataSource": dbDS}}, true},
		{"dataview + microflow → ok", &ast.WidgetV3{Type: "dataview", Name: "dv", Properties: map[string]any{"DataSource": mfDS}}, false},
		{"listview + database → ok", &ast.WidgetV3{Type: "listview", Name: "lv", Properties: map[string]any{"DataSource": dbDS}}, false},
		{"datagrid + database → ok", &ast.WidgetV3{Type: "datagrid", Name: "dg", Properties: map[string]any{"DataSource": dbDS}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := false
			for _, v := range validateStaticWidget(c.widget, "page X") {
				if v.RuleID == "MDL-WIDGET09" {
					got = true
				}
			}
			if got != c.want {
				t.Errorf("MDL-WIDGET09 present = %v, want %v", got, c.want)
			}
		})
	}
}

// TestValidateStaticWidget_UnconsumableConditional — MDL-WIDGET19 is the safety
// net asked for in issue #852: the grammar fix stops `Visible: [trim(…)]` from
// falling through, but any FUTURE conditional expression the visitor cannot turn
// into VisibleIf/EditableIf would land in the plain Visible/Editable slot as a
// non-string, non-bool value, which the builder drops without a word. A dropped
// Visible reads as "always visible", so the failure is invisible until someone
// looks at the running app.
//
// Recognized static forms (bool, expression string) must NOT be flagged — they
// are consumed by pages.StaticVisibleExpression.
func TestValidateStaticWidget_UnconsumableConditional(t *testing.T) {
	cases := []struct {
		name   string
		widget *ast.WidgetV3
		want   bool // expect an MDL-WIDGET19 violation
	}{
		{
			"unparsed bracket residue → flagged",
			&ast.WidgetV3{Type: "dynamictext", Name: "a1", Properties: map[string]any{
				"Visible": []any{"trim($currentObject/Slug)", "!=", "''"},
			}},
			true,
		},
		{
			"unparsed Editable residue → flagged",
			&ast.WidgetV3{Type: "textbox", Name: "b1", Properties: map[string]any{
				"Editable": []any{"length($currentObject/Slug)", ">", "0"},
			}},
			true,
		},
		{
			"routed to VisibleIf → not flagged",
			&ast.WidgetV3{Type: "dynamictext", Name: "a2", Properties: map[string]any{
				"VisibleIf": "trim($currentObject/Slug) != ''",
			}},
			false,
		},
		{
			"static bool → not flagged",
			&ast.WidgetV3{Type: "dynamictext", Name: "a3", Properties: map[string]any{"Visible": false}},
			false,
		},
		{
			"expression string → not flagged",
			&ast.WidgetV3{Type: "dynamictext", Name: "a4", Properties: map[string]any{
				"Visible": "$currentObject/Slug != ''",
			}},
			false,
		},
		{
			"no visibility property → not flagged",
			&ast.WidgetV3{Type: "dynamictext", Name: "a5", Properties: map[string]any{"Content": "x"}},
			false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := false
			for _, v := range validateStaticWidget(c.widget, "page X") {
				if v.RuleID == "MDL-WIDGET19" {
					got = true
				}
			}
			if got != c.want {
				t.Errorf("MDL-WIDGET19 present = %v, want %v", got, c.want)
			}
		})
	}
}

// TestValidateWidgetExpressionAssociations — MDL-WIDGET13 flags an association
// step inside an expression-typed widget property (DynamicClasses/VisibleIf/
// EditableIf). Such expressions fail the build with CE0117; a data binding on the
// same widget may traverse the same association legitimately (not flagged here).
func TestValidateWidgetExpressionAssociations(t *testing.T) {
	cases := []struct {
		name   string
		widget *ast.WidgetV3
		want   bool // expect an MDL-WIDGET13 violation
	}{
		{"dynamicclasses traversing association → rejected",
			&ast.WidgetV3{Type: "dynamictext", Name: "rowBadge", Properties: map[string]any{
				"DynamicClasses": "'fl-tint-' + $currentObject/Feedline.Article_Source/Slug"}}, true},
		{"visibleif traversing association → rejected",
			&ast.WidgetV3{Type: "container", Name: "box", Properties: map[string]any{
				"VisibleIf": "$currentObject/Feedline.Article_Source/Active"}}, true},
		{"editableif traversing association → rejected",
			&ast.WidgetV3{Type: "textbox", Name: "tb", Properties: map[string]any{
				"EditableIf": "$currentObject/Feedline.Article_Source/Editable"}}, true},
		{"dynamicclasses on a plain attribute → ok",
			&ast.WidgetV3{Type: "dynamictext", Name: "rowBadge", Properties: map[string]any{
				"DynamicClasses": "'fl-tint-' + $currentObject/SourceSlug"}}, false},
		{"visibleif comparing an enum literal → ok (not a traversal)",
			&ast.WidgetV3{Type: "container", Name: "box", Properties: map[string]any{
				"VisibleIf": "$currentObject/Status = Feedline.State.Open"}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := false
			for _, v := range validateStaticWidget(c.widget, "page X") {
				if v.RuleID == "MDL-WIDGET13" {
					got = true
				}
			}
			if got != c.want {
				t.Errorf("MDL-WIDGET13 present = %v, want %v", got, c.want)
			}
		})
	}
}

// TestValidateTemplateParamExpressions — MDL-WIDGET14 is retired: an expression
// in a contentparams/captionparams slot is stored as the parameter's
// Expression (ako/mxcli#823 option C), so no value is refused. Before, the
// function call and the arithmetic below were errors. (ledger finding #26)
func TestValidateTemplateParamExpressions(t *testing.T) {
	cp := func(vals ...any) *ast.WidgetV3 {
		params := make([]ast.ParamAssignmentV3, len(vals))
		for i, v := range vals {
			params[i] = ast.ParamAssignmentV3{Index: i + 1, Value: v}
		}
		return &ast.WidgetV3{Type: "dynamictext", Name: "d", Properties: map[string]any{"ContentParams": params}}
	}
	for _, w := range []*ast.WidgetV3{
		cp("formatDateTime($currentObject/LastImport,'d MMM yyyy')"),
		cp("$currentObject/Qty * $currentObject/Price"),
		cp("$currentObject/Name"),
		cp("'literal text'"),
	} {
		for _, v := range validateStaticWidget(w, "page X") {
			if v.RuleID == "MDL-WIDGET14" {
				t.Errorf("MDL-WIDGET14 reported: %s", v.Message)
			}
		}
	}
}

// TestValidateConsecutiveDynamicText — MDL-WIDGET15 (info) flags two or more
// adjacent dynamictext siblings, which Mendix renders inline (concatenated with
// no separator) regardless of RenderMode. (ledger finding #27)
func TestValidateConsecutiveDynamicText(t *testing.T) {
	dt := func(name string) *ast.WidgetV3 { return &ast.WidgetV3{Type: "dynamictext", Name: name} }
	dtRM := func(name, rm string) *ast.WidgetV3 {
		return &ast.WidgetV3{Type: "dynamictext", Name: name, Properties: map[string]any{"RenderMode": rm}}
	}
	dtProp := func(name, key, val string) *ast.WidgetV3 {
		return &ast.WidgetV3{Type: "dynamictext", Name: name, Properties: map[string]any{key: val}}
	}
	tb := func(name string) *ast.WidgetV3 { return &ast.WidgetV3{Type: "textbox", Name: name} }
	cases := []struct {
		name     string
		siblings []*ast.WidgetV3
		want     bool
	}{
		{"two adjacent dynamictexts", []*ast.WidgetV3{dt("a"), dt("b")}, true},
		{"three adjacent (warns once)", []*ast.WidgetV3{dt("a"), dt("b"), dt("c")}, true},
		{"explicit Text render mode", []*ast.WidgetV3{dtRM("a", "Text"), dtRM("b", "Text")}, true},
		{"separated by another widget", []*ast.WidgetV3{dt("a"), tb("x"), dt("b")}, false},
		{"single dynamictext", []*ast.WidgetV3{dt("a")}, false},
		{"no dynamictext", []*ast.WidgetV3{tb("x"), tb("y")}, false},
		// Only headings (H1–H6) are block-level. Paragraph renders inline (<span>)
		// and fuses, so it IS flagged (#29 corrected treating it as block-level).
		{"two paragraphs fuse", []*ast.WidgetV3{dtRM("p1", "Paragraph"), dtRM("p2", "Paragraph")}, true},
		{"paragraph then text fuse", []*ast.WidgetV3{dtRM("p", "Paragraph"), dt("t")}, true},
		// Headings render block-level, so a heading + subtitle does not concatenate.
		{"heading then subtitle", []*ast.WidgetV3{dtRM("h", "H2"), dt("sub")}, false},
		{"two headings", []*ast.WidgetV3{dtRM("h1", "H2"), dtRM("h2", "H3")}, false},
		{"heading breaks a run of inlines", []*ast.WidgetV3{dt("a"), dtRM("h", "H2"), dt("b")}, false},
		// ako/mxcli#943: a text with its own class or style is laid out by the
		// author's SCSS, and BankV1's report pages drew ~35 notes for them.
		{"each with its own class", []*ast.WidgetV3{dtProp("a", "Class", "lbl"), dtProp("b", "Class", "val")}, false},
		{"each with its own style", []*ast.WidgetV3{dtProp("a", "Style", "display:block"), dtProp("b", "Style", "display:block")}, false},
		{"a styled text breaks the run", []*ast.WidgetV3{dt("a"), dtProp("b", "Class", "val"), dt("c")}, false},
		{"two classless after a styled one", []*ast.WidgetV3{dtProp("a", "Class", "lbl"), dt("b"), dt("c")}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := len(validateConsecutiveDynamicText(c.siblings, "page X")) > 0
			if got != c.want {
				t.Errorf("MDL-WIDGET15 present = %v, want %v", got, c.want)
			}
		})
	}
}

// TestConsecutiveDynamicTextHasALocation: MDL-WIDGET15 printed under
// "(no module)" with a blank "at", so a report of 35 could not be traced to a
// page without reading every message (ako/mxcli#943).
func TestConsecutiveDynamicTextHasALocation(t *testing.T) {
	prog, errs := visitor.Build(`create page Shop.Report (title: 'R', layout: Atlas_Core.Atlas_Default) {
  dynamictext txtA (content: 'a')
  dynamictext txtB (content: 'b')
}`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	registry := LoadWidgetRegistry("")
	var found bool
	for _, v := range ValidateWidgetPropertiesForStatement(prog.Statements[0], registry) {
		if v.RuleID != "MDL-WIDGET15" {
			continue
		}
		found = true
		want := linter.Location{Module: "Shop", DocumentType: "page", DocumentName: "Report"}
		if v.Location != want {
			t.Errorf("location = %+v, want %+v", v.Location, want)
		}
		if !strings.Contains(v.Message, "txtA") || !strings.Contains(v.Message, "txtB") {
			t.Errorf("the message does not name the widgets: %s", v.Message)
		}
	}
	if !found {
		t.Fatal("no MDL-WIDGET15 for two adjacent classless texts")
	}
}

// TestValidateObjectListItemEnums — MDL-WIDGET08 flags an object-list item's
// enumeration sub-property whose value isn't a declared member key (e.g. a Maps
// marker LocationType outside {address, latlng}). Studio Pro silently defaults
// an invalid value, so this class of typo otherwise fails quietly at build.
func TestValidateObjectListItemEnums(t *testing.T) {
	mapping := &ObjectListMapping{
		MDLContainer: "DYNAMICMARKER",
		ItemProperties: []ItemPropertyMapping{
			{PropertyKey: "locationType", Operation: "primitive", EnumValues: []string{"address", "latlng"}},
			{PropertyKey: "markerStyleDynamic", Operation: "primitive", EnumValues: []string{"default", "custom"}},
			{PropertyKey: "title", Operation: "attribute"}, // no enum → never flagged
		},
	}
	marker := func(props map[string]any) *ast.WidgetV3 {
		return &ast.WidgetV3{Type: "dynamicmarker", Name: "m1", Properties: props}
	}
	cases := []struct {
		name    string
		widget  *ast.WidgetV3
		wantBad bool
	}{
		{"invalid locationType", marker(map[string]any{"LocationType": "coordinates"}), true},
		{"valid latlng", marker(map[string]any{"LocationType": "latlng"}), false},
		{"valid address (case-insensitive key match)", marker(map[string]any{"locationType": "address"}), false},
		{"unset enum is fine", marker(map[string]any{"Title": "Name"}), false},
		{"non-string value ignored", marker(map[string]any{"LocationType": 3}), false},
		{"second enum prop invalid", marker(map[string]any{"MarkerStyleDynamic": "sparkly"}), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vs := validateObjectListItemEnums(c.widget, mapping, "page X")
			got := len(vs) > 0
			if got != c.wantBad {
				t.Errorf("got %d violations (bad=%v), want bad=%v: %+v", len(vs), got, c.wantBad, vs)
			}
			for _, v := range vs {
				if v.RuleID != "MDL-WIDGET08" {
					t.Errorf("expected MDL-WIDGET08, got %s", v.RuleID)
				}
			}
		})
	}
}

// TestValidateWidgetVisibility checks the #574 config-aware property check
// (MDL-WIDGET10): a property the user sets that the widget hides under the
// current configuration is flagged, while the same property under a config that
// shows it is not. Uses a DataGrid-like def with a selection-mapped condition.
func TestValidateWidgetVisibility(t *testing.T) {
	def := &WidgetDefinition{
		WidgetID: "com.mendix.widget.web.datagrid.Datagrid",
		MDLName:  "DATAGRID",
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "itemSelection", Source: "Selection", Operation: "selection"},
		},
		PropertyVisibility: []types.WidgetVisibilityRule{
			{PropertyKey: "clearSelectionButtonLabel", HiddenWhen: &types.WidgetVisibilityCondition{
				PropertyKey: "itemSelection", Operator: "ne", Value: "Multi",
			}},
		},
	}
	registry := &WidgetRegistry{byMDLName: map[string]*WidgetDefinition{"DATAGRID": def}}

	widget := func(selection string) *ast.WidgetV3 {
		return &ast.WidgetV3{Type: "datagrid", Name: "dg", Properties: map[string]any{
			"Selection":                 selection,
			"ClearSelectionButtonLabel": "Clear it",
		}}
	}

	// None → clearSelectionButtonLabel hidden → warn.
	if v := validateWidgetVisibility(widget("None"), registry, "page P"); len(v) != 1 || v[0].RuleID != "MDL-WIDGET10" {
		t.Errorf("Selection:None → got %d violations %+v, want 1 MDL-WIDGET10", len(v), v)
	}
	// Multi → visible → no warning.
	if v := validateWidgetVisibility(widget("Multi"), registry, "page P"); len(v) != 0 {
		t.Errorf("Selection:Multi → got %d violations %+v, want 0 (property is visible)", len(v), v)
	}
	// Property not set → no warning even under hiding config.
	notSet := &ast.WidgetV3{Type: "datagrid", Name: "dg", Properties: map[string]any{"Selection": "None"}}
	if v := validateWidgetVisibility(notSet, registry, "page P"); len(v) != 0 {
		t.Errorf("unset property → got %d violations, want 0", len(v))
	}
}

// TestValidateComboBoxAssociation guards traceops #23 (the MDL-WIDGET16 check):
// a combobox that binds an association needs an options datasource; without one
// Mendix drops the binding and fails the build with CE0642.
func TestValidateComboBoxAssociation(t *testing.T) {
	dbDS := &ast.DataSourceV3{Type: "database", Reference: "M.Customer"}
	cases := []struct {
		name   string
		widget *ast.WidgetV3
		want   bool // expect MDL-WIDGET16
	}{
		{"association, no datasource → flagged", &ast.WidgetV3{Type: "combobox", Name: "cb", Properties: map[string]any{"Association": "M.Order_Customer"}}, true},
		{"association + datasource → ok", &ast.WidgetV3{Type: "combobox", Name: "cb", Properties: map[string]any{"Association": "M.Order_Customer", "DataSource": dbDS}}, false},
		{"enumeration attribute only → ok", &ast.WidgetV3{Type: "combobox", Name: "cb", Properties: map[string]any{"Attribute": "Status"}}, false},
		{"not a combobox → ignored", &ast.WidgetV3{Type: "textbox", Name: "tb", Properties: map[string]any{"Association": "M.Order_Customer"}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := false
			for _, v := range validateComboBoxAssociation(c.widget, "page X") {
				if v.RuleID == "MDL-WIDGET16" {
					got = true
				}
			}
			if got != c.want {
				t.Errorf("MDL-WIDGET16 present = %v, want %v", got, c.want)
			}
		})
	}
}
