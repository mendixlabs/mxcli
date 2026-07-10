// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// comboboxWidgetID names the Mendix 11 association/enumeration selector. It needs
// one widget-specific quirk (enum-mode optionsSourceType inference) so it keeps a
// named constant; every other supported-widget decision is data-driven from
// widgets.def.json.
const comboboxWidgetID = "com.mendix.widget.web.combobox.Combobox"

// widgetsDefJSON is an auto-datasource HINT table for the built-in pluggable
// widgets, no longer a capability whitelist (Phase 1 of
// PROPOSAL_mcp_pluggable_widget_authoring.md). Acceptance is now registry-driven
// in mapCustomWidget; this table only supplies which property of a built-in widget
// is its DataSource so PropertyTypeIDs can report it and the engine's auto-
// datasource pass fires (DataGrid 2 reaches its datasource that way). A widget
// absent here is still authorable — it just can't use auto-datasource and must map
// its datasource explicitly via the .def.json (which `mxcli widget extract`
// produces). Embedded and consumed ONLY by this package so it cannot perturb the
// MPR datagrid path while that backend is being replaced.
//
//go:embed widgets.def.json
var widgetsDefJSON []byte

type mcpWidgetDef struct {
	WidgetID             string   `json:"widgetId"`
	DataSourceProperties []string `json:"dataSourceProperties"`
}

var mcpWidgetDefs = func() map[string]mcpWidgetDef {
	var doc struct {
		Widgets []mcpWidgetDef `json:"widgets"`
	}
	if err := json.Unmarshal(widgetsDefJSON, &doc); err != nil {
		panic("mcp: invalid widgets.def.json: " + err.Error())
	}
	m := make(map[string]mcpWidgetDef, len(doc.Widgets))
	for _, w := range doc.Widgets {
		m[w.WidgetID] = w
	}
	return m
}()

// mapCustomWidget emits the pg CustomWidgets$CustomWidget for a pluggable widget
// built this session. Only ComboBox is mapped so far; other pluggable widgets,
// or a ComboBox that relied on a property operation we don't translate, are
// rejected rather than written with missing properties.
func (b *Backend) mapCustomWidget(wd *pages.CustomWidget) (map[string]any, error) {
	cw := b.customWidgets[wd.ID]
	if cw == nil {
		return nil, fmt.Errorf("custom widget %q has no recorded properties (the MCP backend builds pluggable widgets via the widget engine)", wd.Name)
	}
	// Acceptance is registry-driven, not a private whitelist: the pluggable widget
	// engine only calls LoadWidgetTemplate for a widget it resolved a definition
	// for (project .mxcli/widgets → global → embedded), so any widget reaching here
	// is registered. We still refuse a widget that relied on a property operation
	// the MCP builder can't faithfully emit (recorded in `unsupported`) — loud
	// rejection over silent drop. Over pg_patch_page Studio Pro owns serialization
	// and expands every default, so no per-widget template is needed (Phase 1 of
	// PROPOSAL_mcp_pluggable_widget_authoring.md).
	if len(cw.unsupported) > 0 {
		return nil, fmt.Errorf("%s %q uses properties not yet supported by the MCP backend: %v", cw.widgetID, wd.Name, cw.unsupported)
	}
	// The combobox def.json enum mode maps only attributeEnumeration; the MPR
	// template carries optionsSourceType's default, which the MCP path lacks. pg
	// defaults an unset optionsSourceType to "association" and then prunes the
	// (now irrelevant) attributeEnumeration, so the enum binding is lost unless
	// we set the source type explicitly. Association mode already sets it via the
	// primitive mapping.
	if _, ok := cw.object["optionsSourceType"]; !ok {
		if _, isEnum := cw.object["attributeEnumeration"]; isEnum {
			cw.object["optionsSourceType"] = "enumeration"
		}
	}
	// Map Widgets-typed slots (e.g. Gallery `content`) recursively into the pg
	// object. Each child goes through the full widget mapper, so nested pluggable
	// widgets and conditional visibility work inside a slot.
	for _, key := range sortedKeys(cw.childSlots) {
		mapped, err := b.mapPageWidgets(cw.childSlots[key])
		if err != nil {
			return nil, fmt.Errorf("%s %q slot %q: %w", cw.widgetID, wd.Name, key, err)
		}
		cw.object[key] = mapped
	}
	return map[string]any{
		"$Type":      "CustomWidgets$CustomWidget",
		"name":       wd.Name,
		"appearance": pageAppearance(wd.Class, wd.Style),
		"widgetId":   cw.widgetID,
		"object":     cw.object,
	}, nil
}

// sortedKeys returns a map's keys in sorted order, for deterministic output.
func sortedKeys(m map[string][]pages.Widget) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// mcpCustomWidget is the recorded high-level pg form of one pluggable widget:
// its widget id and the `object` property bag that pg_patch_page expands into a
// full widget (Studio Pro fills every default, so only the meaningful, MDL-
// derived properties need to be set — this is what sidesteps the CE0463
// template-mismatch class of bugs that the BSON writer path hits).
type mcpCustomWidget struct {
	widgetID    string
	object      map[string]any
	childSlots  map[string][]pages.Widget // Widgets-typed slots (e.g. Gallery `content`), mapped at emit time
	unsupported []string                  // property ops recorded for not-yet-supported widget shapes
}

// LoadWidgetTemplate returns an MCP-specific pluggable-widget builder. Unlike the
// MPR builder (which mutates an embedded BSON template), this one records the
// engine's semantic property operations into a high-level pg `object` map. No
// template BSON is needed — Studio Pro owns serialization over pg_patch_page —
// so the projectPath is ignored and the CE0463 template-mismatch class of bugs
// does not arise on the MCP path.
func (b *Backend) LoadWidgetTemplate(widgetID string, _ string) (backend.WidgetObjectBuilder, error) {
	return &mcpWidgetBuilder{backend: b, widgetID: widgetID, def: mcpWidgetDefs[widgetID], object: map[string]any{}}, nil
}

// mcpWidgetBuilder implements backend.WidgetObjectBuilder by translating each
// semantic Set* operation into the corresponding pg `object` field. Operations
// not needed by the supported widgets are recorded in `unsupported` so
// mapCustomWidget can refuse a widget whose shape we can't faithfully emit
// rather than silently dropping properties.
type mcpWidgetBuilder struct {
	backend     *Backend
	widgetID    string
	def         mcpWidgetDef
	object      map[string]any
	childSlots  map[string][]pages.Widget
	unsupported []string
}

var _ backend.WidgetObjectBuilder = (*mcpWidgetBuilder)(nil)

func (w *mcpWidgetBuilder) note(op string) { w.unsupported = append(w.unsupported, op) }

func (w *mcpWidgetBuilder) SetAttribute(propertyKey, attributePath string) {
	if attributePath == "" {
		return
	}
	w.object[propertyKey] = map[string]any{
		"$Type":     "DomainModels$AttributeRef",
		"attribute": attributePath,
	}
}

func (w *mcpWidgetBuilder) SetAssociation(propertyKey, assocPath, entityName string) {
	if assocPath == "" {
		return
	}
	w.object[propertyKey] = map[string]any{
		"$Type": "DomainModels$IndirectEntityRef",
		"steps": []any{map[string]any{
			"$Type":             "DomainModels$EntityRefStep",
			"association":       assocPath,
			"destinationEntity": entityName,
		}},
	}
}

func (w *mcpWidgetBuilder) SetPrimitive(propertyKey, value string) {
	w.object[propertyKey] = value
}

func (w *mcpWidgetBuilder) SetDataSource(propertyKey string, ds pages.DataSource) {
	if src := customWidgetXPathSource(ds); src != nil {
		w.object[propertyKey] = src
	}
}

// customWidgetXPathSource maps a database/entity data source onto a
// CustomWidgets$CustomWidgetXPathSource (used by DataGrid 2, Gallery, and the
// association combobox). A `sort by` clause becomes the grid sort bar.
// Microflow/other sources are not yet mapped.
func customWidgetXPathSource(ds pages.DataSource) map[string]any {
	var entity, constraint string
	var sorting []*pages.GridSort
	switch s := ds.(type) {
	case *pages.DatabaseSource:
		entity = s.EntityName
		constraint = s.XPathConstraint
		sorting = s.Sorting
	case *pages.DataViewSource:
		entity = s.EntityName
	}
	if entity == "" {
		return nil
	}
	src := map[string]any{
		"$Type":            "CustomWidgets$CustomWidgetXPathSource",
		"entityRef":        map[string]any{"$Type": "DomainModels$DirectEntityRef", "entity": entity},
		"forceFullObjects": false,
	}
	if constraint != "" {
		src["xPathConstraint"] = constraint
	}
	if len(sorting) > 0 {
		src["sortBar"] = gridSortBar(sorting)
	}
	return src
}

// gridSortBar maps a data source's `sort by` clause onto a Pages$GridSortBar.
func gridSortBar(sorting []*pages.GridSort) map[string]any {
	items := make([]any, 0, len(sorting))
	for _, s := range sorting {
		if s.AttributePath == "" {
			continue
		}
		dir := "Ascending"
		if strings.HasPrefix(strings.ToLower(string(s.Direction)), "desc") {
			dir = "Descending"
		}
		items = append(items, map[string]any{
			"$Type":         "Pages$GridSortItem",
			"attributeRef":  map[string]any{"$Type": "DomainModels$AttributeRef", "attribute": s.AttributePath},
			"sortDirection": dir,
		})
	}
	return map[string]any{"$Type": "Pages$GridSortBar", "sortItems": items}
}

// SetSelection sets a selection-typed property (e.g. a DataGrid's itemSelection).
// In pg these are plain string enums ("None" / "Single" / "Multi"), so the value
// is stored directly. (The MPR path's richer multi-selection cloning is not
// needed — Studio Pro expands the rest on pg_patch_page.)
func (w *mcpWidgetBuilder) SetSelection(propertyKey, value string) {
	if value != "" {
		w.object[propertyKey] = value
	}
}

// SetExpression stores an expression-typed property. In a pg custom-widget
// `object` an expression is a plain string (verified live on Studio Pro 11.12:
// ComboBox customEditabilityExpression round-trip, ped_check_errors clean —
// Phase 2 of PROPOSAL_mcp_pluggable_widget_authoring.md).
func (w *mcpWidgetBuilder) SetExpression(propertyKey, value string) {
	if value == "" {
		return
	}
	w.object[propertyKey] = value
}

// SetChildWidgets stores a Widgets-typed slot (e.g. a Gallery's `content`
// template, emptyPlaceholder, filtersPlaceholder). The child widgets are mapped
// to their pg forms lazily in mapCustomWidget, where errors can be surfaced.
func (w *mcpWidgetBuilder) SetChildWidgets(propertyKey string, children []pages.Widget) {
	if len(children) == 0 {
		return
	}
	if w.childSlots == nil {
		w.childSlots = make(map[string][]pages.Widget)
	}
	w.childSlots[propertyKey] = children
}

// SetTextTemplate stores a text-template property. pg keys these with a `ct:`
// prefix and accepts a plain string, which Studio Pro expands into a full
// Pages$ClientTemplate (verified live on 11.12: Image ct:imageUrl /
// ct:alternativeText, ped_check_errors clean).
func (w *mcpWidgetBuilder) SetTextTemplate(propertyKey, text string) {
	if text == "" {
		return
	}
	w.object["ct:"+propertyKey] = text
}

// templateAttrPlaceholderRe matches `{AttrName}` placeholders in a text
// template. Numeric placeholders (`{1}`) don't match and pass through verbatim.
var templateAttrPlaceholderRe = regexp.MustCompile(`\{([A-Za-z][A-Za-z0-9_]*)\}`)

// SetTextTemplateWithParams stores a text-template property whose text may
// embed `{AttrName}` placeholders. Placeholders become `{1}`-style parameter
// references backed by Pages$ClientTemplateParameter attributeRefs, resolved
// against the page's entity context — the same semantics as the MPR builder
// (widgetobj.createClientTemplateBSONWithParams). The parameterised
// Pages$ClientTemplate shape was verified live on 11.12 (attributeRef
// persisted, ped_check_errors clean).
func (w *mcpWidgetBuilder) SetTextTemplateWithParams(propertyKey, text, entityContext string) {
	if text == "" {
		return
	}
	var attrNames []string
	paramText := templateAttrPlaceholderRe.ReplaceAllStringFunc(text, func(s string) string {
		name := s[1 : len(s)-1]
		for i, an := range attrNames {
			if an == name {
				return fmt.Sprintf("{%d}", i+1)
			}
		}
		attrNames = append(attrNames, name)
		return fmt.Sprintf("{%d}", len(attrNames))
	})
	if len(attrNames) == 0 {
		w.object["ct:"+propertyKey] = text
		return
	}
	params := make([]any, 0, len(attrNames))
	for _, attrName := range attrNames {
		attrPath := attrName
		if entityContext != "" && !strings.Contains(attrName, ".") {
			attrPath = entityContext + "." + attrName
		}
		params = append(params, map[string]any{
			"$Type":        "Pages$ClientTemplateParameter",
			"attributeRef": map[string]any{"$Type": "DomainModels$AttributeRef", "attribute": attrPath},
			"formattingInfo": map[string]any{
				"$Type":            "Pages$FormattingInfo",
				"decimalPrecision": 2,
				"groupDigits":      false,
				"enumFormat":       "Text",
				"dateFormat":       "Date",
				"customDateFormat": "",
			},
		})
	}
	w.object["ct:"+propertyKey] = map[string]any{
		"$Type":      "Pages$ClientTemplate",
		"t:template": paramText,
		"parameters": params,
		"t:fallback": "",
	}
}

// SetAction stores an action-typed property using the documented
// Pages$*ClientAction shapes. One deviation from the native LightPage widget
// constructors: inside a custom widget's `object`, MicroflowClientAction must
// nest the microflow reference in microflowSettings — pg silently drops a flat
// `microflow` key there (verified live on 11.12: flat key → "Select a
// microflow" consistency error; nested → clean). Actions with parameter
// mappings need PageVariable-shaped values and fully-qualified parameter names
// whose pg forms are not yet pinned live, so they are rejected loudly rather
// than emitted with the mappings dropped.
func (w *mcpWidgetBuilder) SetAction(propertyKey string, action pages.ClientAction) {
	mapped, err := customWidgetClientAction(action)
	if err != nil {
		w.note(fmt.Sprintf("action:%s (%v)", propertyKey, err))
		return
	}
	w.object[propertyKey] = mapped
}

// customWidgetClientAction maps a semantic client action onto the pg shape a
// custom widget's `object` accepts. Kept separate from mapClientAction (the
// native-widget mapper) because the accepted shapes differ — see SetAction.
func customWidgetClientAction(a pages.ClientAction) (map[string]any, error) {
	switch act := a.(type) {
	case nil, *pages.NoClientAction:
		return map[string]any{"$Type": "Pages$NoClientAction"}, nil
	case *pages.MicroflowClientAction:
		if len(act.ParameterMappings) > 0 {
			return nil, fmt.Errorf("microflow action parameter mappings are not yet supported over MCP")
		}
		return map[string]any{
			"$Type": "Pages$MicroflowClientAction",
			"microflowSettings": map[string]any{
				"$Type":             "Pages$MicroflowSettings",
				"microflow":         act.MicroflowName,
				"parameterMappings": []any{},
				"progressBar":       "None",
				"asynchronous":      false,
				"formValidations":   "All",
			},
			"disabledDuringExecution": true,
		}, nil
	case *pages.PageClientAction:
		if len(act.ParameterMappings) > 0 {
			return nil, fmt.Errorf("page action parameter mappings are not yet supported over MCP")
		}
		return map[string]any{
			"$Type": "Pages$PageClientAction",
			"pageSettings": map[string]any{
				"$Type":             "Pages$PageSettings",
				"page":              act.PageName,
				"parameterMappings": []any{},
			},
			"disabledDuringExecution": true,
		}, nil
	default:
		return nil, fmt.Errorf("client action %T is not yet supported over MCP", a)
	}
}

// SetAttributeObjects is a deliberate no-op. The only widgets that use it are the
// DataGrid column filters, whose def.json always sets attrChoice="auto" — under
// which Studio Pro auto-binds the filter to the column's attribute and *ignores*
// the attributes list (in fact pg rejects a non-empty attributes list when
// attrChoice is "auto", silently dropping the widget). So emitting the derived
// column attribute here would be both redundant and harmful; auto-bind is the
// correct, faithful result. (Revisit if a filter ever exposes attrChoice="linked"
// with an explicit Attributes property.)
func (w *mcpWidgetBuilder) SetAttributeObjects(_ string, _ []string) {}
func (w *mcpWidgetBuilder) CloneGallerySelectionProperty(propertyKey, _ string) {
	w.note("gallerySelection:" + propertyKey)
}

// SetObjectList translates an object-list property (e.g. a DataGrid's `columns`)
// into a list of pg CustomWidgets$WidgetObject items. The shared engine has
// already resolved each item's properties (attribute paths, header templates,
// primitives), so the translation is generic: the operation kind determines the
// pg shape, and text-template properties take pg's `ct:` prefix. Items that need
// shapes not yet supported (custom-content child widgets, parameterised header
// templates) are recorded as unsupported so the widget is rejected rather than
// written with missing content.
func (w *mcpWidgetBuilder) SetObjectList(propertyKey string, items []backend.ObjectListItemSpec) {
	list := make([]any, 0, len(items))
	for _, it := range items {
		obj := map[string]any{"$Type": "CustomWidgets$WidgetObject"}
		for _, p := range it.Properties {
			switch p.Operation {
			case "attribute":
				if p.AttributePath != "" {
					obj[p.PropertyKey] = map[string]any{"$Type": "DomainModels$AttributeRef", "attribute": p.AttributePath}
				}
			case "texttemplate":
				if len(p.Parameters) > 0 {
					w.note(fmt.Sprintf("%s[].%s (template params)", propertyKey, p.PropertyKey))
					continue
				}
				// pg expects ct:-prefixed ClientTemplate keys and wraps a plain string.
				obj["ct:"+p.PropertyKey] = p.TextTemplate
			case "primitive":
				obj[p.PropertyKey] = p.PrimitiveVal
			case "expression":
				obj[p.PropertyKey] = p.Expression
			case "datasource":
				if src := customWidgetXPathSource(p.DataSource); src != nil {
					obj[p.PropertyKey] = src
				}
			default:
				w.note(fmt.Sprintf("%s[].%s (%s)", propertyKey, p.PropertyKey, p.Operation))
			}
		}
		// Widgets-typed sub-slots of the item (e.g. a column's `filter` widget, or
		// custom-content cell `content` widgets) are mapped recursively. The child
		// widgets were already built and registered by the engine before this call.
		for _, slot := range sortedKeys(it.ChildWidgets) {
			mapped, err := w.backend.mapPageWidgets(it.ChildWidgets[slot])
			if err != nil {
				w.note(fmt.Sprintf("%s[].%s: %v", propertyKey, slot, err))
				continue
			}
			obj[slot] = mapped
		}
		list = append(list, obj)
	}
	w.object[propertyKey] = list
}

// PropertyTypeIDs reports the widget's DataSource-typed properties from
// widgets.def.json. The shared engine's auto-datasource pass reads this to route
// the AST data source through SetDataSource (the MCP path has no template, so
// this is the only metadata it needs). Other property types are not needed —
// Studio Pro expands every default on pg_patch_page.
func (w *mcpWidgetBuilder) PropertyTypeIDs() map[string]pages.PropertyTypeIDEntry {
	out := make(map[string]pages.PropertyTypeIDEntry, len(w.def.DataSourceProperties))
	for _, key := range w.def.DataSourceProperties {
		out[key] = pages.PropertyTypeIDEntry{ValueType: "DataSource"}
	}
	return out
}

func (w *mcpWidgetBuilder) EnsureRequiredObjectLists()                             {}
func (w *mcpWidgetBuilder) ApplyPropertyVisibility(_ []types.WidgetVisibilityRule) {}

// Finalize builds the CustomWidget shell and registers its recorded pg object on
// the backend, keyed by the widget's ID so mapPageWidget can find it.
func (w *mcpWidgetBuilder) Finalize(id model.ID, name, label, editable string) *pages.CustomWidget {
	if w.backend.customWidgets == nil {
		w.backend.customWidgets = make(map[model.ID]*mcpCustomWidget)
	}
	w.backend.customWidgets[id] = &mcpCustomWidget{
		widgetID:    w.widgetID,
		object:      w.object,
		childSlots:  w.childSlots,
		unsupported: w.unsupported,
	}
	return &pages.CustomWidget{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{ID: id, TypeName: "CustomWidgets$CustomWidget"},
			Name:        name,
		},
		Label:    label,
		Editable: editable,
	}
}
