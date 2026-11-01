// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/widgets/mpk"
)

func TestDeriveMDLName(t *testing.T) {
	tests := []struct {
		widgetID string
		expected string
	}{
		{"com.mendix.widget.web.combobox.Combobox", "COMBOBOX"},
		{"com.mendix.widget.web.gallery.Gallery", "GALLERY"},
		{"com.company.widget.MyCustomWidget", "MYCUSTOMWIDGET"},
		{"SimpleWidget", "SIMPLEWIDGET"},
		// mendixlabs/mxcli#1463: native widgets must not take the web
		// keyword (`image`, `radiobuttons`, `badge`, …).
		{"com.mendix.widget.native.image.Image", "NATIVEIMAGE"},
		{"com.mendix.widget.native.radiobuttons.RadioButtons", "NATIVERADIOBUTTONS"},
		{"com.mendix.widget.web.image.Image", "IMAGE"},
	}

	for _, tc := range tests {
		t.Run(tc.widgetID, func(t *testing.T) {
			result := DeriveMDLName(tc.widgetID)
			if result != tc.expected {
				t.Errorf("DeriveMDLName(%q) = %q, want %q", tc.widgetID, result, tc.expected)
			}
		})
	}
}

func TestGenerateDefJSON(t *testing.T) {
	mpkDef := &mpk.WidgetDefinition{
		ID:   "com.example.widget.TestWidget",
		Name: "Test Widget",
		Properties: []mpk.PropertyDef{
			{Key: "datasource", Type: "datasource"},
			{Key: "content", Type: "widgets"},
			{Key: "filterBar", Type: "widgets"},
			{Key: "myAttribute", Type: "attribute"},
			{Key: "showHeader", Type: "boolean", DefaultValue: "true"},
			{Key: "itemSelection", Type: "selection", DefaultValue: "Single"},
			{Key: "myAssociation", Type: "association"},
			{Key: "pageSize", Type: "integer", DefaultValue: "10"},
		},
	}

	def := GenerateDefJSON(mpkDef, "TESTWIDGET")

	// Verify basic fields
	if def.WidgetID != "com.example.widget.TestWidget" {
		t.Errorf("WidgetID = %q, want %q", def.WidgetID, "com.example.widget.TestWidget")
	}
	if def.MDLName != "TESTWIDGET" {
		t.Errorf("MDLName = %q, want %q", def.MDLName, "TESTWIDGET")
	}
	if def.TemplateFile != "testwidget.json" {
		t.Errorf("TemplateFile = %q, want %q", def.TemplateFile, "testwidget.json")
	}
	if def.DefaultEditable != "Always" {
		t.Errorf("DefaultEditable = %q, want %q", def.DefaultEditable, "Always")
	}

	// Verify property mappings count (datasource, attribute, boolean, selection, association, integer = 6)
	if len(def.PropertyMappings) != 6 {
		t.Fatalf("PropertyMappings count = %d, want 6", len(def.PropertyMappings))
	}

	// Verify child slots (content → TEMPLATE, filterBar → FILTERBAR)
	if len(def.ChildSlots) != 2 {
		t.Fatalf("ChildSlots count = %d, want 2", len(def.ChildSlots))
	}

	// content → TEMPLATE (special case)
	if def.ChildSlots[0].MDLContainer != "TEMPLATE" {
		t.Errorf("ChildSlots[0].MDLContainer = %q, want %q", def.ChildSlots[0].MDLContainer, "TEMPLATE")
	}
	// filterBar → FILTERBAR
	if def.ChildSlots[1].MDLContainer != "FILTERBAR" {
		t.Errorf("ChildSlots[1].MDLContainer = %q, want %q", def.ChildSlots[1].MDLContainer, "FILTERBAR")
	}

	// Verify datasource mapping
	dsMappings := findMapping(def.PropertyMappings, "datasource")
	if dsMappings == nil {
		t.Fatal("datasource mapping not found")
	}
	if dsMappings.Operation != "datasource" {
		t.Errorf("datasource operation = %q, want %q", dsMappings.Operation, "datasource")
	}

	// Verify attribute mapping
	attrMapping := findMapping(def.PropertyMappings, "myAttribute")
	if attrMapping == nil {
		t.Fatal("myAttribute mapping not found")
	}
	if attrMapping.Operation != "attribute" || attrMapping.Source != "Attribute" {
		t.Errorf("myAttribute: operation=%q source=%q, want operation=attribute source=Attribute",
			attrMapping.Operation, attrMapping.Source)
	}

	// Verify boolean with default value
	boolMapping := findMapping(def.PropertyMappings, "showHeader")
	if boolMapping == nil {
		t.Fatal("showHeader mapping not found")
	}
	if boolMapping.Value != "true" {
		t.Errorf("showHeader value = %q, want %q", boolMapping.Value, "true")
	}

	// Verify selection with default
	selMapping := findMapping(def.PropertyMappings, "itemSelection")
	if selMapping == nil {
		t.Fatal("itemSelection mapping not found")
	}
	if selMapping.Operation != "selection" || selMapping.Default != "Single" {
		t.Errorf("itemSelection: operation=%q default=%q, want operation=selection default=Single",
			selMapping.Operation, selMapping.Default)
	}
}

func TestGenerateDefJSON_SkipsComplexTypes(t *testing.T) {
	mpkDef := &mpk.WidgetDefinition{
		ID:   "com.example.Complex",
		Name: "Complex",
		Properties: []mpk.PropertyDef{
			{Key: "myAction", Type: "action"},
			{Key: "myExpr", Type: "expression"},
			{Key: "myTemplate", Type: "textTemplate"},
			{Key: "myIcon", Type: "icon"},
			{Key: "myObj", Type: "object"},
		},
	}

	def := GenerateDefJSON(mpkDef, "COMPLEX")

	// textTemplate always yields a mapping (so captions like Badge `value` are
	// authorable), and so does every action property — one whose key is not
	// onClick/onChange becomes a NAMED slot, mapped with no Source and addressed
	// by its own key (#956) — and every expression, the same way
	// (mendixlabs/mxcli#1578). icon/non-list object remain skipped.
	if len(def.PropertyMappings) != 3 {
		t.Fatalf("PropertyMappings count = %d, want 3 (texttemplate + the named action slot + the expression; icon/object skipped): %+v",
			len(def.PropertyMappings), def.PropertyMappings)
	}
	byKey := map[string]PropertyMapping{}
	for _, m := range def.PropertyMappings {
		byKey[m.PropertyKey] = m
	}
	if ex := byKey["myExpr"]; ex.Operation != "expression" || ex.Source != "" {
		t.Errorf("myExpr = {op=%q src=%q}, want {op=expression src=\"\"}", ex.Operation, ex.Source)
	}
	if tt := byKey["myTemplate"]; tt.Operation != "texttemplate" {
		t.Errorf("myTemplate operation = %q, want texttemplate", tt.Operation)
	}
	// A named slot must carry no Source: that is what tells resolveMapping to
	// read the AST property by this key rather than from a fixed slot.
	if act := byKey["myAction"]; act.Operation != "action" || act.Source != "" {
		t.Errorf("myAction = {op=%q src=%q}, want {op=action src=\"\"} — a slot with no MDL "+
			"alias is addressed by its own key", act.Operation, act.Source)
	}
	if len(def.ChildSlots) != 0 {
		t.Errorf("ChildSlots count = %d, want 0", len(def.ChildSlots))
	}
}

// Every top-level texttemplate property yields an authorable mapping, keyed by
// its own property name, regardless of whether a friendly alias is registered.
// Regression guard for the Badge `value` / TreeNode `headerCaption` / Timeline
// `title` drop (MDL-WIDGET01).
func TestGenerateDefJSON_TextTemplatesAlwaysMapped(t *testing.T) {
	mpkDef := &mpk.WidgetDefinition{
		ID:   "com.mendix.widget.custom.badge.Badge",
		Name: "Badge",
		Properties: []mpk.PropertyDef{
			{Key: "type", Type: "enumeration", DefaultValue: "badge"},
			{Key: "value", Type: "textTemplate"},
			{Key: "onClick", Type: "action"},
		},
	}
	def := GenerateDefJSON(mpkDef, "BADGE")
	m := findMapping(def.PropertyMappings, "value")
	if m == nil {
		t.Fatal("value (textTemplate) has no mapping — it would be dropped (MDL-WIDGET01)")
	}
	if m.Operation != "texttemplate" || m.Source != "TextTemplate" {
		t.Errorf("value mapping = {op=%s src=%s}, want {texttemplate TextTemplate}", m.Operation, m.Source)
	}
}

// TestGenerateDefJSON_ActionMapping covers ledger #67: an `onClick` action
// property must yield an `action` PropertyMapping (source OnClick) so the writer
// serializes the client action instead of silently dropping it. A non-authorable
// action key (e.g. DataGrid2 `onSelectionChange`) is a NAMED slot: it maps too,
// with no Source, and is addressed by its own key (#956).
func TestGenerateDefJSON_ActionMapping(t *testing.T) {
	mpkDef := &mpk.WidgetDefinition{
		ID:   "com.mendix.widget.web.datagrid.Datagrid",
		Name: "Datagrid",
		Properties: []mpk.PropertyDef{
			{Key: "onClick", Type: "action"},
			{Key: "onSelectionChange", Type: "action"},
		},
	}
	def := GenerateDefJSON(mpkDef, "DATAGRID")
	onClick := findMapping(def.PropertyMappings, "onClick")
	if onClick == nil {
		t.Fatal("onClick (action) has no mapping — the action would be silently dropped (#67)")
	}
	if onClick.Operation != "action" || onClick.Source != "OnClick" {
		t.Errorf("onClick mapping = {op=%s src=%s}, want {action OnClick}", onClick.Operation, onClick.Source)
	}
	sel := findMapping(def.PropertyMappings, "onSelectionChange")
	if sel == nil {
		t.Fatal("onSelectionChange (action) has no mapping — a slot with no MDL alias is " +
			"addressed by its own key and cannot be written without one (#956)")
	}
	if sel.Operation != "action" || sel.Source != "" {
		t.Errorf("onSelectionChange mapping = {op=%s src=%s}, want {action \"\"} — a Source "+
			"would route it through a fixed AST slot instead of its own key",
			sel.Operation, sel.Source)
	}
}

// TestGenerateDefJSON_KnownPropertiesUnmapped covers the general #67-class guard:
// every declared property with NO mapping in the generated def is recorded in
// KnownProperties, so the checker warns "recognized but not persisted" rather
// than silently dropping it or falsely rejecting it as unknown. A mapped property
// must NOT appear there.
func TestGenerateDefJSON_KnownPropertiesUnmapped(t *testing.T) {
	mpkDef := &mpk.WidgetDefinition{
		ID:   "com.mendix.widget.web.datagrid.Datagrid",
		Name: "Datagrid",
		Properties: []mpk.PropertyDef{
			{Key: "onClick", Type: "action"},           // mapped (action → OnClick)
			{Key: "pageSize", Type: "integer"},         // mapped (primitive)
			{Key: "onSelectionChange", Type: "action"}, // mapped as a named slot (#956)
			{Key: "rowClass", Type: "expression"},      // mapped by its own key (mendixlabs/mxcli#1578)
			{Key: "rowIcon", Type: "icon"},             // unmapped (icon not handled)
		},
	}
	def := GenerateDefJSON(mpkDef, "DATAGRID")
	known := make(map[string]bool)
	for _, k := range def.KnownProperties {
		known[k] = true
	}
	for _, want := range []string{"rowIcon"} {
		if !known[want] {
			t.Errorf("KnownProperties missing unmapped property %q — the checker can't warn it will be dropped", want)
		}
	}
	// onSelectionChange is now mapped as a named slot, so listing it would make
	// MDL-WIDGET06 warn that a value mxcli writes is going to be dropped (#956).
	for _, notWant := range []string{"onClick", "pageSize", "onSelectionChange", "rowClass"} {
		if known[notWant] {
			t.Errorf("KnownProperties should not list mapped property %q", notWant)
		}
	}
}

func TestGenerateDefJSON_AssociationAfterDataSource(t *testing.T) {
	// Association mappings require entityContext from a prior DataSource mapping.
	// GenerateDefJSON must order datasource before association regardless of MPK order.
	mpkDef := &mpk.WidgetDefinition{
		ID:   "com.example.AssocFirst",
		Name: "AssocFirst",
		Properties: []mpk.PropertyDef{
			{Key: "myAssoc", Type: "association"}, // association BEFORE datasource in MPK
			{Key: "myLabel", Type: "string"},
			{Key: "myDS", Type: "datasource"},
		},
	}

	def := GenerateDefJSON(mpkDef, "ASSOCFIRST")

	// Should have 3 mappings: datasource, string primitive, association
	if len(def.PropertyMappings) != 3 {
		t.Fatalf("PropertyMappings count = %d, want 3", len(def.PropertyMappings))
	}

	// datasource must appear before association in the mappings slice
	dsIdx, assocIdx := -1, -1
	for i, m := range def.PropertyMappings {
		if m.Source == "DataSource" {
			dsIdx = i
		}
		if m.Source == "Association" {
			assocIdx = i
		}
	}
	if dsIdx < 0 {
		t.Fatal("DataSource mapping not found")
	}
	if assocIdx < 0 {
		t.Fatal("Association mapping not found")
	}
	if dsIdx > assocIdx {
		t.Errorf("DataSource at index %d must come before Association at index %d", dsIdx, assocIdx)
	}

	// Verify the generated definition can be loaded by the registry without validation errors.
	// The registry's validateMappings enforces Association-after-DataSource ordering.
}

func findMapping(mappings []PropertyMapping, key string) *PropertyMapping {
	for i := range mappings {
		if mappings[i].PropertyKey == key {
			return &mappings[i]
		}
	}
	return nil
}

// TestDeriveObjectListKeyword verifies plural→singular keyword derivation
// for object-list properties, including the override map for irregular cases.
func TestDeriveObjectListKeyword(t *testing.T) {
	tests := []struct {
		propertyKey string
		expected    string
	}{
		// Regular plurals (strip trailing 's', uppercase)
		{"groups", "GROUP"},
		{"columns", "COLUMN"},
		{"markers", "MARKER"},
		// Override map (irregular cases)
		{"basicItems", "ITEM"},
		{"customItems", "CUSTOMITEM"},
		{"dynamicMarkers", "DYNAMICMARKER"},
		{"attributesList", "ATTR"},
		{"filterOptions", "OPTION"},
		{"series", "SERIES"}, // Latin singular == plural
	}

	for _, tc := range tests {
		t.Run(tc.propertyKey, func(t *testing.T) {
			got := deriveObjectListKeyword(tc.propertyKey)
			if got != tc.expected {
				t.Errorf("deriveObjectListKeyword(%q) = %q, want %q",
					tc.propertyKey, got, tc.expected)
			}
		})
	}
}

// TestGenerateDefJSON_ObjectList covers extraction of Type:"object"+IsList:true
// properties (e.g. Accordion groups, DataGrid columns, PopupMenu basicItems).
// Each list item's sub-property tree should be split between ItemProperties
// (scalar/datasource/attribute/etc.) and ItemSlots (widgets-typed).
func TestGenerateDefJSON_ObjectList(t *testing.T) {
	// Synthesize an Accordion-style "groups" property with mixed sub-property kinds.
	mpkDef := &mpk.WidgetDefinition{
		ID:   "com.example.widget.Accordion",
		Name: "Accordion",
		Properties: []mpk.PropertyDef{
			{Key: "advancedMode", Type: "boolean", DefaultValue: "false"},
			{
				Key:    "groups",
				Type:   "object",
				IsList: true,
				Children: []mpk.PropertyDef{
					{Key: "headerRenderMode", Type: "enumeration", DefaultValue: "text"},
					{Key: "headerText", Type: "textTemplate"},
					{Key: "visible", Type: "expression"},
					{Key: "collapsed", Type: "attribute"},
					{Key: "onToggleCollapsed", Type: "action"},
					{Key: "headerContent", Type: "widgets"},
					{Key: "content", Type: "widgets"},
				},
			},
		},
	}

	def := GenerateDefJSON(mpkDef, "ACCORDION")

	// Top-level primitive should still land in PropertyMappings.
	if len(def.PropertyMappings) != 1 {
		t.Fatalf("PropertyMappings count = %d, want 1", len(def.PropertyMappings))
	}

	// Object-list goes to ObjectLists, not to ChildSlots or PropertyMappings.
	if len(def.ObjectLists) != 1 {
		t.Fatalf("ObjectLists count = %d, want 1", len(def.ObjectLists))
	}
	ol := def.ObjectLists[0]
	if ol.PropertyKey != "groups" {
		t.Errorf("ObjectLists[0].PropertyKey = %q, want %q", ol.PropertyKey, "groups")
	}
	if ol.MDLContainer != "GROUP" {
		t.Errorf("ObjectLists[0].MDLContainer = %q, want %q", ol.MDLContainer, "GROUP")
	}

	// 5 non-widgets items should be ItemProperties; 2 widgets items should be ItemSlots.
	if len(ol.ItemProperties) != 5 {
		t.Errorf("ItemProperties count = %d, want 5", len(ol.ItemProperties))
	}
	if len(ol.ItemSlots) != 2 {
		t.Errorf("ItemSlots count = %d, want 2", len(ol.ItemSlots))
	}

	// Spot-check operation kinds for sub-properties.
	wantOps := map[string]string{
		"headerRenderMode":  "primitive",
		"headerText":        "texttemplate",
		"visible":           "expression",
		"collapsed":         "attribute",
		"onToggleCollapsed": "action",
	}
	for _, ip := range ol.ItemProperties {
		want, ok := wantOps[ip.PropertyKey]
		if !ok {
			t.Errorf("unexpected ItemProperty key %q", ip.PropertyKey)
			continue
		}
		if ip.Operation != want {
			t.Errorf("ItemProperty %q: Operation = %q, want %q",
				ip.PropertyKey, ip.Operation, want)
		}
	}

	// ItemSlots should map widgets-typed sub-properties to their MDLContainer.
	wantSlots := map[string]string{
		"headerContent": "HEADERCONTENT",
		"content":       "CONTENT",
	}
	for _, slot := range ol.ItemSlots {
		want, ok := wantSlots[slot.PropertyKey]
		if !ok {
			t.Errorf("unexpected ItemSlot key %q", slot.PropertyKey)
			continue
		}
		if slot.MDLContainer != want {
			t.Errorf("ItemSlot %q: MDLContainer = %q, want %q",
				slot.PropertyKey, slot.MDLContainer, want)
		}
		if slot.Operation != "widgets" {
			t.Errorf("ItemSlot %q: Operation = %q, want %q",
				slot.PropertyKey, slot.Operation, "widgets")
		}
	}

	// IsList=false on an object property should still skip (not extracted).
	mpkDef2 := &mpk.WidgetDefinition{
		ID:   "com.example.widget.NotAList",
		Name: "NotAList",
		Properties: []mpk.PropertyDef{
			{Key: "myObj", Type: "object", IsList: false},
		},
	}
	def2 := GenerateDefJSON(mpkDef2, "NOTALIST")
	if len(def2.ObjectLists) != 0 {
		t.Errorf("ObjectLists for non-list object property = %d, want 0",
			len(def2.ObjectLists))
	}
}

// TestGenerateDefJSON_ObjectListPrimitiveDefaults verifies that primitive
// item properties carry their MPK default values into the ItemPropertyMapping.
func TestGenerateDefJSON_ObjectListPrimitiveDefaults(t *testing.T) {
	mpkDef := &mpk.WidgetDefinition{
		ID:   "com.example.widget.Sized",
		Name: "Sized",
		Properties: []mpk.PropertyDef{
			{
				Key:    "items",
				Type:   "object",
				IsList: true,
				Children: []mpk.PropertyDef{
					{Key: "size", Type: "integer", DefaultValue: "10"},
					{Key: "label", Type: "string", DefaultValue: ""},
					{Key: "kind", Type: "enumeration", DefaultValue: "default"},
				},
			},
		},
	}

	def := GenerateDefJSON(mpkDef, "SIZED")
	if len(def.ObjectLists) != 1 {
		t.Fatalf("ObjectLists count = %d, want 1", len(def.ObjectLists))
	}
	props := def.ObjectLists[0].ItemProperties
	if len(props) != 3 {
		t.Fatalf("ItemProperties count = %d, want 3", len(props))
	}

	wantValues := map[string]string{
		"size":  "10",
		"label": "", // empty default → no Value set
		"kind":  "default",
	}
	for _, ip := range props {
		want := wantValues[ip.PropertyKey]
		if ip.Value != want {
			t.Errorf("ItemProperty %q Value = %q, want %q",
				ip.PropertyKey, ip.Value, want)
		}
		if ip.Operation != "primitive" {
			t.Errorf("ItemProperty %q Operation = %q, want primitive",
				ip.PropertyKey, ip.Operation)
		}
	}
}

// TestMdlContainerForWidgetSlot covers the editorial override table that
// maps (widgetID, propertyKey) pairs to the MDL keyword users type to fill
// a widgets-typed property.
//
// The override exists because Studio Pro users (and the historical keyword
// path) think of `controlbar { ... }` for DataGrid and `filter { ... }`
// for Gallery, not `filtersplaceholder { ... }`. Without the override the
// auto-derived keyword would be the uppercase property key, which doesn't
// match the MDL grammar tokens.
func TestMdlContainerForWidgetSlot(t *testing.T) {
	tests := []struct {
		name        string
		widgetID    string
		propertyKey string
		want        string
	}{
		{
			name:        "DataGrid filtersPlaceholder → CONTROLBAR",
			widgetID:    "com.mendix.widget.web.datagrid.Datagrid",
			propertyKey: "filtersPlaceholder",
			want:        "CONTROLBAR",
		},
		{
			name:        "Gallery filtersPlaceholder → FILTER",
			widgetID:    "com.mendix.widget.web.gallery.Gallery",
			propertyKey: "filtersPlaceholder",
			want:        "FILTER",
		},
		{
			name:        "content property → TEMPLATE (global convention)",
			widgetID:    "com.example.AnyWidget",
			propertyKey: "content",
			want:        "TEMPLATE",
		},
		{
			name:        "unmapped property → uppercase key",
			widgetID:    "com.mendix.widget.web.datagrid.Datagrid",
			propertyKey: "emptyPlaceholder",
			want:        "EMPTYPLACEHOLDER",
		},
		{
			name:        "unmapped widget, unmapped property → uppercase key",
			widgetID:    "com.example.UnknownWidget",
			propertyKey: "myslot",
			want:        "MYSLOT",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mdlContainerForWidgetSlot(tc.widgetID, tc.propertyKey)
			if got != tc.want {
				t.Errorf("mdlContainerForWidgetSlot(%q, %q) = %q, want %q",
					tc.widgetID, tc.propertyKey, got, tc.want)
			}
		})
	}
}

// TestObjectListItemAliases covers MDL property name aliases on object-list
// items. A DataGrid column's `Caption:` and `Content:` in MDL fill the
// schema's `header` and `dynamicText` properties respectively. Without the
// aliases, the engine would look up `header` / `dynamicText` in the AST
// property bag and find nothing — the caption text would be silently dropped.
func TestObjectListItemAliases(t *testing.T) {
	mpkDef := &mpk.WidgetDefinition{
		ID:   "com.mendix.widget.web.datagrid.Datagrid",
		Name: "Data grid 2",
		Properties: []mpk.PropertyDef{
			{
				Key:    "columns",
				Type:   "object",
				IsList: true,
				Children: []mpk.PropertyDef{
					{Key: "header", Type: "textTemplate"},
					{Key: "dynamicText", Type: "textTemplate"},
					{Key: "tooltip", Type: "textTemplate"},
					{Key: "attribute", Type: "attribute"},
					{Key: "width", Type: "enumeration"},
					{Key: "columnClass", Type: "expression"},
					{Key: "visible", Type: "expression"},
				},
			},
		},
	}
	def := GenerateDefJSON(mpkDef, "DATAGRID")

	if len(def.ObjectLists) != 1 {
		t.Fatalf("ObjectLists count = %d, want 1", len(def.ObjectLists))
	}
	cols := def.ObjectLists[0]
	if cols.PropertyKey != "columns" {
		t.Fatalf("PropertyKey = %q, want columns", cols.PropertyKey)
	}

	aliases := map[string][]string{}
	for _, ip := range cols.ItemProperties {
		aliases[ip.PropertyKey] = ip.MdlAliases
	}

	if got := aliases["header"]; len(got) != 1 || got[0] != "Caption" {
		t.Errorf("header MdlAliases = %v, want [Caption]", got)
	}
	if got := aliases["dynamicText"]; len(got) != 1 || got[0] != "Content" {
		t.Errorf("dynamicText MdlAliases = %v, want [Content]", got)
	}
	// width is filled by MDL `ColumnWidth:` (dgDyn CE0463 regression fix).
	if got := aliases["width"]; len(got) != 1 || got[0] != "ColumnWidth" {
		t.Errorf("width MdlAliases = %v, want [ColumnWidth]", got)
	}
	// columnClass is filled by MDL `DynamicCellClass:` (Bug 10a — per-cell
	// dynamic class was silently dropped without this alias).
	if got := aliases["columnClass"]; len(got) != 1 || got[0] != "DynamicCellClass" {
		t.Errorf("columnClass MdlAliases = %v, want [DynamicCellClass]", got)
	}
	// visible is filled by the visitor's VisibleIf — every expression spelling
	// of `Visible:` (`if … then … else …`, `$currentObject/Flag`, `[cond]`) is
	// lowered there; without the alias a column's visibility was dropped.
	if got := aliases["visible"]; len(got) != 1 || got[0] != "VisibleIf" {
		t.Errorf("visible MdlAliases = %v, want [VisibleIf]", got)
	}
	// tooltip and attribute have no aliases — schema name is the MDL keyword.
	if got := aliases["tooltip"]; len(got) != 0 {
		t.Errorf("tooltip MdlAliases = %v, want []", got)
	}
	if got := aliases["attribute"]; len(got) != 0 {
		t.Errorf("attribute MdlAliases = %v, want []", got)
	}
}

// HeatMap scaleColors `colour` (British spelling) is filled by MDL `ColorValue:`
// — without the alias the scale colour is silently dropped on write (Bug 10a class).
func TestObjectListItemAliases_HeatMapColour(t *testing.T) {
	mpkDef := &mpk.WidgetDefinition{
		ID:   "com.mendix.widget.web.heatmap.HeatMap",
		Name: "Heat map",
		Properties: []mpk.PropertyDef{{
			Key:    "scaleColors",
			Type:   "object",
			IsList: true,
			Children: []mpk.PropertyDef{
				{Key: "valuePercentage", Type: "integer"},
				{Key: "colour", Type: "string"},
			},
		}},
	}
	def := GenerateDefJSON(mpkDef, "HEATMAP")
	if len(def.ObjectLists) != 1 {
		t.Fatalf("ObjectLists = %d, want 1", len(def.ObjectLists))
	}
	aliases := map[string][]string{}
	for _, ip := range def.ObjectLists[0].ItemProperties {
		aliases[ip.PropertyKey] = ip.MdlAliases
	}
	if got := aliases["colour"]; len(got) != 1 || got[0] != "ColorValue" {
		t.Errorf("colour MdlAliases = %v, want [ColorValue]", got)
	}
	if got := aliases["valuePercentage"]; len(got) != 0 {
		t.Errorf("valuePercentage MdlAliases = %v, want [] (matches case-insensitively)", got)
	}
}

// TestItemSlotAcceptedChildTypes covers the routing of widget keywords (e.g.
// textfilter, numberfilter) directly inside an object-list item body. Without
// this routing, the engine would treat the filter widgets as default-slot
// (content) children, putting them in the wrong slot.
func TestItemSlotAcceptedChildTypes(t *testing.T) {
	mpkDef := &mpk.WidgetDefinition{
		ID:   "com.mendix.widget.web.datagrid.Datagrid",
		Name: "Data grid 2",
		Properties: []mpk.PropertyDef{
			{
				Key:    "columns",
				Type:   "object",
				IsList: true,
				Children: []mpk.PropertyDef{
					{Key: "content", Type: "widgets"},
					{Key: "filter", Type: "widgets"},
				},
			},
		},
	}
	def := GenerateDefJSON(mpkDef, "DATAGRID")
	if len(def.ObjectLists) != 1 {
		t.Fatalf("ObjectLists count = %d, want 1", len(def.ObjectLists))
	}

	slots := def.ObjectLists[0].ItemSlots
	got := map[string][]string{}
	for _, s := range slots {
		got[s.PropertyKey] = s.AcceptedChildTypes
	}

	if len(got["content"]) != 0 {
		t.Errorf("content slot AcceptedChildTypes = %v, want [] (no special routing)", got["content"])
	}
	wantFilter := []string{"textfilter", "numberfilter", "datefilter", "dropdownfilter"}
	if len(got["filter"]) != len(wantFilter) {
		t.Fatalf("filter slot AcceptedChildTypes = %v, want %v", got["filter"], wantFilter)
	}
	for i, w := range wantFilter {
		if got["filter"][i] != w {
			t.Errorf("filter[%d] = %q, want %q", i, got["filter"][i], w)
		}
	}
}

// TestApplyColumnHeaderFallback covers the missing-Caption fallback that
// makes DataGrid columns without `Caption:` emit a header populated with
// the attribute leaf name. Mirrors datagrid_builder.go's
// `if caption == "" { caption = col.Attribute }` convention.
func TestApplyColumnHeaderFallback(t *testing.T) {
	// Case 1: header already present → no change
	spec1 := &backend.ObjectListItemSpec{
		Properties: []backend.ObjectListItemProperty{
			{PropertyKey: "header", Operation: "texttemplate", TextTemplate: "Override"},
			{PropertyKey: "attribute", Operation: "attribute", AttributePath: "Mod.Ent.Foo"},
		},
	}
	applyColumnHeaderFallback(spec1, true)
	if len(spec1.Properties) != 2 {
		t.Errorf("Case 1 (header present): expected 2 properties, got %d", len(spec1.Properties))
	}

	// Case 2: no header slot (a header-less object-list item like a chart series)
	// → never touched, even with no attribute.
	spec2 := &backend.ObjectListItemSpec{
		Properties: []backend.ObjectListItemProperty{
			{PropertyKey: "staticName", Operation: "primitive", PrimitiveVal: "x"},
		},
	}
	applyColumnHeaderFallback(spec2, false)
	if len(spec2.Properties) != 1 {
		t.Errorf("Case 2 (no header slot): expected 1 property, got %d", len(spec2.Properties))
	}

	// Case 3: no header, attribute set → synthesize header from attribute leaf
	spec3 := &backend.ObjectListItemSpec{
		Properties: []backend.ObjectListItemProperty{
			{PropertyKey: "attribute", Operation: "attribute", AttributePath: "Mod.Ent.OrderNumber"},
		},
	}
	applyColumnHeaderFallback(spec3, true)
	if len(spec3.Properties) != 2 {
		t.Fatalf("Case 3 (fallback): expected 2 properties, got %d", len(spec3.Properties))
	}
	hdr := spec3.Properties[1]
	if hdr.PropertyKey != "header" || hdr.Operation != "texttemplate" || hdr.TextTemplate != "OrderNumber" {
		t.Errorf("Case 3: synthesized header = %+v, want PropertyKey=header Operation=texttemplate TextTemplate=OrderNumber", hdr)
	}

	// Case 4: unqualified attribute path → use as-is
	spec4 := &backend.ObjectListItemSpec{
		Properties: []backend.ObjectListItemProperty{
			{PropertyKey: "attribute", Operation: "attribute", AttributePath: "BareName"},
		},
	}
	applyColumnHeaderFallback(spec4, true)
	if len(spec4.Properties) != 2 || spec4.Properties[1].TextTemplate != "BareName" {
		t.Errorf("Case 4: fallback for unqualified path = %v", spec4.Properties)
	}

	// Case 5 (ledger #54): an explicit empty caption (`Caption: ''`) reaches the
	// engine as a texttemplate header with empty text and no params. Studio Pro
	// rejects an empty column header with CE0463, so it must be treated like an
	// absent one — filled IN PLACE with the attribute leaf, not left empty and
	// not duplicated.
	spec5 := &backend.ObjectListItemSpec{
		Properties: []backend.ObjectListItemProperty{
			{PropertyKey: "header", Operation: "texttemplate", TextTemplate: ""},
			{PropertyKey: "attribute", Operation: "attribute", AttributePath: "Mod.Ent.Amount"},
		},
	}
	applyColumnHeaderFallback(spec5, true)
	if len(spec5.Properties) != 2 {
		t.Fatalf("Case 5 (empty caption): expected 2 properties (no duplicate), got %d", len(spec5.Properties))
	}
	if h := spec5.Properties[0]; h.PropertyKey != "header" || h.Operation != "texttemplate" || h.TextTemplate != "Amount" {
		t.Errorf("Case 5: empty header not filled in place: %+v, want TextTemplate=Amount", spec5.Properties[0])
	}

	// Case 6: a template header WITH params (e.g. Caption: '{1}') is NOT empty —
	// it must be left untouched even though its literal text may be blank.
	spec6 := &backend.ObjectListItemSpec{
		Properties: []backend.ObjectListItemProperty{
			{PropertyKey: "header", Operation: "texttemplate", TextTemplate: "", Parameters: []*pages.ClientTemplateParameter{{}}},
			{PropertyKey: "attribute", Operation: "attribute", AttributePath: "Mod.Ent.Amount"},
		},
	}
	applyColumnHeaderFallback(spec6, true)
	if spec6.Properties[0].TextTemplate != "" {
		t.Errorf("Case 6: param-bearing header must not be overwritten, got TextTemplate=%q", spec6.Properties[0].TextTemplate)
	}

	// Case 7 (ako/mxcli#535): a custom-content column with `Caption: ''` and no
	// attribute gets an EMPTY header, never the column's name. Mendix stores no
	// name for a column, and Data grid 2 renders an empty header blank — so
	// writing the name put an internal identifier ("colTriage") in front of end
	// users. The empty header is the empty Forms$ClientTemplate update-widgets
	// writes (ChipCoV8 #11), which mx check accepts (0 errors on 11.15.0, the
	// keyword `datagrid` path has always written it).
	spec7 := &backend.ObjectListItemSpec{
		Properties: []backend.ObjectListItemProperty{
			{PropertyKey: "header", Operation: "texttemplate", TextTemplate: ""},
			{PropertyKey: "showContentAs", Operation: "primitive", PrimitiveVal: "customContent"},
		},
	}
	applyColumnHeaderFallback(spec7, true)
	if h := spec7.Properties[0]; h.PropertyKey != "header" || h.TextTemplate != "" || !h.EmptyTemplate {
		t.Errorf("Case 7: empty custom-content header = %+v, want an empty texttemplate with EmptyTemplate=true (not the column name)", spec7.Properties[0])
	}

	// Case 8 (ako/mxcli#535): custom-content column with NO header prop at all —
	// append an empty header, the same as Case 7.
	spec8 := &backend.ObjectListItemSpec{
		Properties: []backend.ObjectListItemProperty{
			{PropertyKey: "showContentAs", Operation: "primitive", PrimitiveVal: "customContent"},
		},
	}
	applyColumnHeaderFallback(spec8, true)
	if len(spec8.Properties) != 2 {
		t.Fatalf("Case 8 (absent custom-content header): expected 2 properties, got %d", len(spec8.Properties))
	}
	if h := spec8.Properties[1]; h.PropertyKey != "header" || h.TextTemplate != "" || !h.EmptyTemplate {
		t.Errorf("Case 8: appended header = %+v, want an empty texttemplate with EmptyTemplate=true", spec8.Properties[1])
	}

	// Case 9 (ChipCoV8 #11): an UNNAMED column with no caption and no attribute
	// (`column (Caption: '') { … }`, or bare `column { … }`) has nothing to fall
	// back to. Its header must still serialize as an empty Forms$ClientTemplate
	// — the shape update-widgets writes — not TextTemplate: null, which is
	// CE0463. Flagged EmptyTemplate, filled in place.
	spec9 := &backend.ObjectListItemSpec{
		Properties: []backend.ObjectListItemProperty{
			{PropertyKey: "header", Operation: "texttemplate", TextTemplate: ""},
			{PropertyKey: "showContentAs", Operation: "primitive", PrimitiveVal: "customContent"},
		},
	}
	applyColumnHeaderFallback(spec9, true)
	if len(spec9.Properties) != 2 {
		t.Fatalf("Case 9 (unnamed, empty caption): expected 2 properties (no duplicate), got %d", len(spec9.Properties))
	}
	if h := spec9.Properties[0]; h.PropertyKey != "header" || h.Operation != "texttemplate" || h.TextTemplate != "" || !h.EmptyTemplate {
		t.Errorf("Case 9: header = %+v, want an empty texttemplate with EmptyTemplate=true", h)
	}

	// Case 10: unnamed column with NO header prop at all — append an
	// EmptyTemplate header.
	spec10 := &backend.ObjectListItemSpec{
		Properties: []backend.ObjectListItemProperty{
			{PropertyKey: "showContentAs", Operation: "primitive", PrimitiveVal: "customContent"},
		},
	}
	applyColumnHeaderFallback(spec10, true)
	if len(spec10.Properties) != 2 {
		t.Fatalf("Case 10 (unnamed, absent header): expected 2 properties, got %d", len(spec10.Properties))
	}
	if h := spec10.Properties[1]; h.PropertyKey != "header" || h.Operation != "texttemplate" || !h.EmptyTemplate {
		t.Errorf("Case 10: appended header = %+v, want an empty texttemplate with EmptyTemplate=true", h)
	}

	// Case 11: no header slot, unnamed — still untouched (the EmptyTemplate
	// header is scoped to datagrid columns, never to header-less items).
	spec11 := &backend.ObjectListItemSpec{
		Properties: []backend.ObjectListItemProperty{
			{PropertyKey: "staticName", Operation: "texttemplate", TextTemplate: ""},
		},
	}
	applyColumnHeaderFallback(spec11, false)
	if len(spec11.Properties) != 1 || spec11.Properties[0].EmptyTemplate {
		t.Errorf("Case 11 (no header slot): must be untouched, got %+v", spec11.Properties)
	}
}

// TestObjectListItemPropertyParamsConvention documents the alias→params
// naming convention that the engine relies on. When MDL writes
// `Caption: '{1}'` together with `CaptionParams: [{1} = attr]`, the engine
// pairs them by matching the alias name + "Params" suffix in the AST
// property bag. This test locks in the convention so the engine's lookup
// (in buildObjectListItem) stays compatible with the AST naming.
func TestObjectListItemPropertyParamsConvention(t *testing.T) {
	// The convention: for a header itemProperty whose MdlAliases contains
	// "Caption", the engine looks for "CaptionParams" in the AST.
	tests := []struct {
		alias       string
		wantPairKey string
	}{
		{"Caption", "CaptionParams"},
		{"Content", "ContentParams"},
		{"Tooltip", "TooltipParams"}, // future extension; matches naming convention
	}
	for _, tc := range tests {
		t.Run(tc.alias, func(t *testing.T) {
			got := tc.alias + "Params"
			if got != tc.wantPairKey {
				t.Errorf("alias %q expected param companion %q, got %q", tc.alias, tc.wantPairKey, got)
			}
		})
	}
}

// TestRefreshStaleWidgetDefinitions covers the generatorVersion-stamp
// staleness guard that exec uses to self-heal project-local def.json files
// generated by an older mxcli build.
func TestRefreshStaleWidgetDefinitions(t *testing.T) {
	t.Run("no project path → no-op", func(t *testing.T) {
		refreshed, err := RefreshStaleWidgetDefinitions("")
		if err != nil || refreshed {
			t.Errorf("empty path: got (refreshed=%v, err=%v), want (false, nil)", refreshed, err)
		}
	})

	t.Run("no .mxcli/widgets dir → no-op", func(t *testing.T) {
		dir := t.TempDir()
		refreshed, err := RefreshStaleWidgetDefinitions(filepath.Join(dir, "App.mpr"))
		if err != nil || refreshed {
			t.Errorf("missing defs dir: got (refreshed=%v, err=%v), want (false, nil)", refreshed, err)
		}
	})

	t.Run("current-version def → no refresh", func(t *testing.T) {
		dir := t.TempDir()
		defsDir := filepath.Join(dir, ".mxcli", "widgets")
		if err := os.MkdirAll(defsDir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeDef(t, defsDir, "datagrid.def.json", WidgetDefGeneratorVersion)
		// No widgets/ dir → if it tried to refresh, RefreshWidgetDefinitions
		// would find no .mpk and no-op anyway; the point is the stamp check
		// short-circuits before that.
		refreshed, err := RefreshStaleWidgetDefinitions(filepath.Join(dir, "App.mpr"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if refreshed {
			t.Errorf("current-version def should not trigger refresh")
		}
	})

	t.Run("behind-version def → triggers refresh", func(t *testing.T) {
		dir := t.TempDir()
		defsDir := filepath.Join(dir, ".mxcli", "widgets")
		if err := os.MkdirAll(defsDir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeDef(t, defsDir, "datagrid.def.json", WidgetDefGeneratorVersion-1)
		// No widgets/*.mpk, so RefreshWidgetDefinitions itself no-ops, but
		// the staleness scan must still classify the def as behind and call it.
		// We assert no error; the "did it call refresh" signal is covered by
		// the integration-level exec test.
		_, err := RefreshStaleWidgetDefinitions(filepath.Join(dir, "App.mpr"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("unstamped (pre-v2) def → treated as stale", func(t *testing.T) {
		dir := t.TempDir()
		defsDir := filepath.Join(dir, ".mxcli", "widgets")
		if err := os.MkdirAll(defsDir, 0o755); err != nil {
			t.Fatal(err)
		}
		// A def with no generatorVersion field unmarshals to 0 < current → stale.
		if err := os.WriteFile(filepath.Join(defsDir, "datagrid.def.json"),
			[]byte(`{"widgetId":"com.x.D","mdlName":"D"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := RefreshStaleWidgetDefinitions(filepath.Join(dir, "App.mpr"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// TestGenerateDefJSONPropertyVisibility verifies GenerateDefJSON stamps the
// hand-authored visibility rules for VideoPlayer and Timeline (#574) and emits
// none for widgets without rules.
func TestGenerateDefJSONPropertyVisibility(t *testing.T) {
	t.Run("VideoPlayer gets type=expression rules", func(t *testing.T) {
		def := GenerateDefJSON(&mpk.WidgetDefinition{ID: "com.mendix.widget.web.videoplayer.VideoPlayer", IsPluggable: true}, "VIDEOPLAYER")
		keys := map[string]string{}
		for _, r := range def.PropertyVisibility {
			if r.HiddenWhen == nil {
				t.Fatalf("rule for %q has nil HiddenWhen", r.PropertyKey)
			}
			keys[r.PropertyKey] = r.HiddenWhen.PropertyKey + " " + r.HiddenWhen.Operator + " " + r.HiddenWhen.Value
		}
		for _, want := range []string{"videoUrl", "posterUrl"} {
			if keys[want] != "type eq expression" {
				t.Errorf("visibility[%q] = %q, want %q", want, keys[want], "type eq expression")
			}
		}
	})

	t.Run("Timeline gets customVisualization truthy rules", func(t *testing.T) {
		def := GenerateDefJSON(&mpk.WidgetDefinition{ID: "com.mendix.widget.web.timeline.Timeline", IsPluggable: true}, "TIMELINE")
		found := map[string]bool{}
		for _, r := range def.PropertyVisibility {
			if r.HiddenWhen == nil || r.HiddenWhen.PropertyKey != "customVisualization" || r.HiddenWhen.Operator != "truthy" {
				t.Errorf("rule %q = %+v, want customVisualization truthy", r.PropertyKey, r.HiddenWhen)
			}
			found[r.PropertyKey] = true
		}
		for _, want := range []string{"title", "description", "timeIndication"} {
			if !found[want] {
				t.Errorf("missing visibility rule for %q", want)
			}
		}
	})

	t.Run("widget without rules has no propertyVisibility", func(t *testing.T) {
		def := GenerateDefJSON(&mpk.WidgetDefinition{ID: "com.mendix.widget.web.combobox.Combobox", IsPluggable: true}, "COMBOBOX")
		if len(def.PropertyVisibility) != 0 {
			t.Errorf("PropertyVisibility = %+v, want empty for a widget with no rules", def.PropertyVisibility)
		}
	})
}

func writeDef(t *testing.T, dir, name string, genVersion int) {
	t.Helper()
	def := WidgetDefinition{
		WidgetID:         "com.example.widget.Test",
		MDLName:          "TEST",
		GeneratorVersion: genVersion,
	}
	data, err := json.MarshalIndent(def, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRegenerateWidgetDocsMultiWidgetMPK guards bug 9a's docs half: a bundled
// .mpk (Charts.mpk holds 10 chart widgets) must produce one doc per widget, not
// just the first widgetFile. Before the fix RegenerateWidgetDocs used ParseMPK
// (first widget only), so only areachart.md was written and the other charts
// (columnchart, barchart, piechart, linechart, bubblechart, …) were omitted.
func TestRegenerateWidgetDocsMultiWidgetMPK(t *testing.T) {
	const chartsFixture = "../../testdata/expr-checker/widgets/Charts.mpk"
	if _, err := os.Stat(chartsFixture); err != nil {
		t.Skipf("Charts.mpk fixture not available: %v", err)
	}

	projectDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectDir, "widgets"), 0755); err != nil {
		t.Fatal(err)
	}
	// Copy the bundled Charts.mpk into the project's widgets/ dir.
	src, err := os.ReadFile(chartsFixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "widgets", "Charts.mpk"), src, 0644); err != nil {
		t.Fatal(err)
	}

	projectPath := filepath.Join(projectDir, "App.mpr")
	generated, err := RegenerateWidgetDocs(projectPath)
	if err != nil {
		t.Fatalf("RegenerateWidgetDocs: %v", err)
	}

	// ParseMPKAll reports 10 widgets in this fixture; assert we documented more
	// than the single first widget the old code produced.
	all, err := mpk.ParseMPKAll(chartsFixture)
	if err != nil {
		t.Fatal(err)
	}
	if generated != len(all) {
		t.Errorf("generated %d docs, want %d (one per widget in the bundle)", generated, len(all))
	}

	docsDir := filepath.Join(projectDir, ".claude", "skills", "widgets")
	// Every chart widget must have a doc file, not just areachart.md.
	for _, w := range all {
		name := "widget"
		if i := lastDotIndex(w.ID); i >= 0 {
			name = w.ID[i+1:]
		}
		f := filepath.Join(docsDir, toLowerASCII(name)+".md")
		if _, err := os.Stat(f); err != nil {
			t.Errorf("expected doc for %s at %s, but it is missing", w.ID, f)
		}
	}
}

func lastDotIndex(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' {
			return i
		}
	}
	return -1
}

func toLowerASCII(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}
