// SPDX-License-Identifier: Apache-2.0

// Package widgetobj is the engine-agnostic pluggable-widget object builder: it
// fills a widget template's Object (raw BSON) with property values and emits a
// *pages.CustomWidget. Child content embedded in property values (widgets,
// actions, data sources) is serialized through the ChildSerializer the active
// engine supplies (legacy=sdk/mpr serializers, modelsdk=codec converters).
package widgetobj

import (
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ChildSerializer serializes child content embedded in a pluggable widget's
// property values to raw BSON. Each engine supplies its own implementation.
type ChildSerializer interface {
	SerializeWidget(w pages.Widget) bson.D
	SerializeClientAction(action pages.ClientAction) bson.D
	SerializeCustomWidgetDataSource(ds pages.DataSource) bson.D
}

// pkgChildSer is the active child serializer. SetChildSerializer (called by each
// engine's widget-building entry points) sets it; New also sets it. Widget
// building is synchronous and engines run sequentially (incl. compare mode), so a
// package-level hook is safe and avoids threading through every helper.
var pkgChildSer ChildSerializer

// SetChildSerializer registers the active engine's child serializer. Backends
// call this at widget-building entry points that don't go through New (e.g.
// filter-widget construction).
func SetChildSerializer(cs ChildSerializer) { pkgChildSer = cs }

// Builder implements backend.WidgetObjectBuilder.
type Builder struct {
	widgetID        string
	embeddedType    bson.D
	object          bson.D
	propertyTypeIDs map[string]pages.PropertyTypeIDEntry
	objectTypeID    string
}

var _ backend.WidgetObjectBuilder = (*Builder)(nil)

// New constructs a Builder for a loaded template and registers the engine's child
// serializer for this build.
func New(widgetID string, embeddedType, object bson.D, propertyTypeIDs map[string]pages.PropertyTypeIDEntry, objectTypeID string, cs ChildSerializer) *Builder {
	pkgChildSer = cs
	return &Builder{widgetID: widgetID, embeddedType: embeddedType, object: object, propertyTypeIDs: propertyTypeIDs, objectTypeID: objectTypeID}
}

// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// WidgetObjectBuilder — property operations
// ---------------------------------------------------------------------------

func (ob *Builder) SetAttribute(propertyKey string, attributePath string) {
	if attributePath == "" {
		return
	}
	ob.object = updateWidgetPropertyValue(ob.object, ob.propertyTypeIDs, propertyKey, func(val bson.D) bson.D {
		return setAttributeRef(val, attributePath)
	})
}

func (ob *Builder) SetAssociation(propertyKey string, assocPath string, entityName string) {
	if assocPath == "" {
		return
	}
	ob.object = updateWidgetPropertyValue(ob.object, ob.propertyTypeIDs, propertyKey, func(val bson.D) bson.D {
		return setAssociationRef(val, assocPath, entityName)
	})
}

func (ob *Builder) SetPrimitive(propertyKey string, value string) {
	if value == "" {
		return
	}
	ob.object = updateWidgetPropertyValue(ob.object, ob.propertyTypeIDs, propertyKey, func(val bson.D) bson.D {
		return setPrimitiveValue(val, value)
	})
}

func (ob *Builder) SetSelection(propertyKey string, value string) {
	if value == "" {
		return
	}
	canonical := canonicalSelectionValue(value)
	ob.object = updateWidgetPropertyValue(ob.object, ob.propertyTypeIDs, propertyKey, func(val bson.D) bson.D {
		result := make(bson.D, 0, len(val))
		for _, elem := range val {
			if elem.Key == "Selection" {
				result = append(result, bson.E{Key: "Selection", Value: canonical})
			} else {
				result = append(result, elem)
			}
		}
		return result
	})
}

// canonicalSelectionValue normalises a selection-enum value to the
// PascalCase form Studio Pro stores (Single / Multi / None). MDL accepts
// any case; lowercase passes mx check on some widgets but contributes to
// CE0463 widget-definition drift on others (gallery on Mendix 11.9).
// Unknown values pass through unchanged so the runtime/Studio Pro can
// surface them.
func canonicalSelectionValue(value string) string {
	switch strings.ToLower(value) {
	case "single":
		return "Single"
	case "multi", "multiple":
		return "Multi"
	case "none":
		return "None"
	}
	return value
}

func (ob *Builder) SetExpression(propertyKey string, value string) {
	if value == "" {
		return
	}
	ob.object = updateWidgetPropertyValue(ob.object, ob.propertyTypeIDs, propertyKey, func(val bson.D) bson.D {
		result := make(bson.D, 0, len(val))
		for _, elem := range val {
			if elem.Key == "Expression" {
				result = append(result, bson.E{Key: "Expression", Value: value})
			} else {
				result = append(result, elem)
			}
		}
		return result
	})
}

func (ob *Builder) SetDataSource(propertyKey string, ds pages.DataSource) {
	if ds == nil {
		return
	}
	ob.object = updateWidgetPropertyValue(ob.object, ob.propertyTypeIDs, propertyKey, func(val bson.D) bson.D {
		return setDataSource(val, ds)
	})
}

func (ob *Builder) SetChildWidgets(propertyKey string, children []pages.Widget) {
	if len(children) == 0 {
		return
	}
	ob.object = updateWidgetPropertyValue(ob.object, ob.propertyTypeIDs, propertyKey, func(val bson.D) bson.D {
		return setChildWidgets(val, children)
	})
}

func (ob *Builder) SetTextTemplate(propertyKey string, text string) {
	if text == "" {
		return
	}
	ob.object = updateWidgetPropertyValue(ob.object, ob.propertyTypeIDs, propertyKey, func(val bson.D) bson.D {
		return setTextTemplateValue(val, text)
	})
}

func (ob *Builder) SetTextTemplateWithParams(propertyKey string, text string, entityContext string) {
	if text == "" {
		return
	}
	tmpl := createClientTemplateBSONWithParams(text, entityContext)
	ob.object = updateWidgetPropertyValue(ob.object, ob.propertyTypeIDs, propertyKey, func(val bson.D) bson.D {
		result := make(bson.D, 0, len(val))
		for _, elem := range val {
			if elem.Key == "TextTemplate" {
				result = append(result, bson.E{Key: "TextTemplate", Value: tmpl})
			} else {
				result = append(result, elem)
			}
		}
		return result
	})
}

func (ob *Builder) SetAction(propertyKey string, action pages.ClientAction) {
	if action == nil {
		return
	}
	actionBSON := pkgChildSer.SerializeClientAction(action)
	ob.object = updateWidgetPropertyValue(ob.object, ob.propertyTypeIDs, propertyKey, func(val bson.D) bson.D {
		result := make(bson.D, 0, len(val))
		for _, elem := range val {
			if elem.Key == "Action" {
				result = append(result, bson.E{Key: "Action", Value: actionBSON})
			} else {
				result = append(result, elem)
			}
		}
		return result
	})
}

// SetObjectList sets a list of structured items on an object-list property
// (e.g. Accordion `groups`, PopupMenu `basicItems`). Each item is built from
// the template's nested PropertyTypeIDs, with spec values overlaid onto the
// matching sub-properties. Sub-properties not mentioned in the spec keep the
// template's default values (via createDefaultWidgetProperty).
//
// This is the generic implementation used by the pluggable widget engine for
// .def.json `objectLists` mappings. The DataGrid columns path keeps its own
// hand-coded builder (datagrid_builder.go) for backward compatibility.
func (ob *Builder) SetObjectList(propertyKey string, items []backend.ObjectListItemSpec) {
	if len(items) == 0 {
		return
	}
	entry, ok := ob.propertyTypeIDs[propertyKey]
	if !ok || entry.ObjectTypeID == "" || len(entry.NestedPropertyIDs) == 0 {
		return
	}

	objects := bson.A{int32(2)}
	for _, item := range items {
		objects = append(objects, buildObjectListItemBSON(ob.widgetID, propertyKey, entry, item))
	}

	ob.object = updateWidgetPropertyValue(ob.object, ob.propertyTypeIDs, propertyKey, func(val bson.D) bson.D {
		result := make(bson.D, 0, len(val))
		for _, elem := range val {
			if elem.Key == "Objects" {
				result = append(result, bson.E{Key: "Objects", Value: objects})
			} else {
				result = append(result, elem)
			}
		}
		return result
	})
}

// buildObjectListItemBSON constructs the BSON for one item of an object-list
// property. Walks the nested PropertyTypeIDs, applies spec overrides where
// the spec mentions a sub-property, and falls back to template defaults
// otherwise.
//
// widgetID and listPropertyKey identify the parent widget and the object-list
// property — used to look up widget-specific default-emission conventions
// (e.g. DataGrid column tooltip → empty ClientTemplate when the column is
// attribute-bound, matching the keyword path's c3d61af1 behavior).
func buildObjectListItemBSON(widgetID, listPropertyKey string, parentEntry pages.PropertyTypeIDEntry, item backend.ObjectListItemSpec) bson.D {
	// Index spec scalar properties by key for fast lookup.
	specByKey := make(map[string]backend.ObjectListItemProperty, len(item.Properties))
	for _, p := range item.Properties {
		specByKey[p.PropertyKey] = p
	}

	itemKind := detectObjectListItemKind(specByKey, item.ChildWidgets)

	propsArr := bson.A{int32(2)}
	// Use template PropertyTypes order when available — Studio Pro expects
	// the WidgetObject.Properties array to mirror the WidgetType.PropertyTypes
	// order or it flags CE0463. Fall back to alphabetical for templates that
	// didn't capture the order (older callers, custom widgets without nested
	// schema).
	nestedKeys := parentEntry.NestedKeyOrder
	if len(nestedKeys) == 0 {
		nestedKeys = make([]string, 0, len(parentEntry.NestedPropertyIDs))
		for k := range parentEntry.NestedPropertyIDs {
			nestedKeys = append(nestedKeys, k)
		}
		sort.Strings(nestedKeys)
	}

	for _, k := range nestedKeys {
		nestedEntry := parentEntry.NestedPropertyIDs[k]
		spec, hasSpec := specByKey[k]
		childWidgets := item.ChildWidgets[k]

		var prop bson.D
		switch {
		case hasSpec:
			prop = buildItemSubProperty(nestedEntry, spec)
		case len(childWidgets) > 0:
			prop = buildItemChildWidgetsProperty(nestedEntry, childWidgets)
		case shouldEmitEmptyClientTemplate(widgetID, listPropertyKey, k, itemKind):
			prop = buildEmptyClientTemplateProperty(nestedEntry)
		default:
			prop = createDefaultWidgetProperty(nestedEntry)
		}
		propsArr = append(propsArr, prop)
	}

	return bson.D{
		{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
		{Key: "$Type", Value: "CustomWidgets$WidgetObject"},
		{Key: "TypePointer", Value: types.UUIDToBlob(parentEntry.ObjectTypeID)},
		{Key: "Properties", Value: propsArr},
	}
}

// objectListItemKind classifies an item by what kind of content fills its
// primary slot — relevant for widgets whose unset-property conventions vary
// by item kind. For DataGrid columns: a column with `Attribute:` set and no
// child widgets is "attribute"; a column with child widgets in the
// dynamicText / content slot is "customcontent".
type objectListItemKind string

const (
	itemKindAttribute     objectListItemKind = "attribute"
	itemKindCustomContent objectListItemKind = "customcontent"
	itemKindDefault       objectListItemKind = ""
)

// detectObjectListItemKind inspects the spec and child widgets of an item
// to classify it. Mirrors the keyword path's `hasCustomContent` heuristic in
// datagrid_builder.go.
//
// Custom-content kind requires widgets in the *content* slot specifically —
// sidecar slots like `filter` (DataGrid column filter widget) don't make
// the column a custom-content cell.
func detectObjectListItemKind(specByKey map[string]backend.ObjectListItemProperty, childWidgets map[string][]pages.Widget) objectListItemKind {
	if len(childWidgets["content"]) > 0 {
		return itemKindCustomContent
	}
	if attr, ok := specByKey["attribute"]; ok && attr.AttributePath != "" {
		return itemKindAttribute
	}
	return itemKindDefault
}

// emptyClientTemplateRules maps (widgetID, listPropertyKey, itemKind,
// propertyKey) tuples to "emit empty ClientTemplate" for unset TextTemplate
// properties whose Studio Pro convention is an empty Forms$ClientTemplate
// rather than null.
//
// Source: c3d61af1 in datagrid_builder.go — Studio Pro's per-column-kind
// convention for DataGrid columns (verified against Cars_Overview):
//
//	property        attribute column   custom-content column
//	tooltip         empty CT           null
//	exportValue     null               empty CT
//	dynamicText     null               null
var emptyClientTemplateRules = map[string]map[string]map[objectListItemKind]map[string]bool{
	"com.mendix.widget.web.datagrid.Datagrid": {
		"columns": {
			itemKindAttribute: {
				"tooltip": true,
			},
			itemKindCustomContent: {
				"exportValue": true,
			},
		},
	},
}

// shouldEmitEmptyClientTemplate returns true when an unset TextTemplate-typed
// property should be serialized as an empty Forms$ClientTemplate (Items=[3]
// in both Fallback and Template, no Translation entries) instead of
// TextTemplate=null.
func shouldEmitEmptyClientTemplate(widgetID, listPropertyKey, propertyKey string, kind objectListItemKind) bool {
	return emptyClientTemplateRules[widgetID][listPropertyKey][kind][propertyKey]
}

// buildEmptyClientTemplateProperty emits a WidgetProperty whose Value
// carries an empty ClientTemplate (Items=[3] markers only, no Translations).
// Used for column properties where Studio Pro stores an empty ClientTemplate
// rather than null when the property is unset — see shouldEmitEmptyClientTemplate.
func buildEmptyClientTemplateProperty(entry pages.PropertyTypeIDEntry) bson.D {
	value := createDefaultWidgetValue(entry)
	value = setBSONField(value, "TextTemplate", BuildEmptyClientTemplate())
	return bson.D{
		{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: types.UUIDToBlob(entry.PropertyTypeID)},
		{Key: "Value", Value: value},
	}
}

// buildItemSubProperty builds one sub-property (a scalar value: primitive,
// attribute, expression, etc.) of an object-list item, starting from the
// template default value and overlaying the spec.
func buildItemSubProperty(entry pages.PropertyTypeIDEntry, spec backend.ObjectListItemProperty) bson.D {
	value := createDefaultWidgetValue(entry)
	value = overlayItemValue(value, entry, spec)
	return bson.D{
		{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: types.UUIDToBlob(entry.PropertyTypeID)},
		{Key: "Value", Value: value},
	}
}

// overlayItemValue mutates the template-default WidgetValue BSON to apply the
// spec's value, dispatched by Operation. Returns the updated value.
func overlayItemValue(value bson.D, entry pages.PropertyTypeIDEntry, spec backend.ObjectListItemProperty) bson.D {
	switch spec.Operation {
	case "primitive":
		value = setBSONField(value, "PrimitiveValue", spec.PrimitiveVal)
	case "attribute":
		if spec.AttributePath != "" {
			value = setAttributeRefField(value, spec.AttributePath, spec.AttributeRefSteps)
		}
	case "expression":
		value = setBSONField(value, "Expression", spec.Expression)
		value = setBSONField(value, "PrimitiveValue", "")
	case "texttemplate":
		// A VISIBLE-but-unset texttemplate sub-property must serialize as an
		// empty Forms$ClientTemplate, not null, or Studio Pro flags CE0463
		// (e.g. a chart series' staticName, or a static·/dynamic· name whose
		// datasource is configured). EmptyTemplate is set by the executor from
		// the widget.xml dataSource-binding visibility rule. Chart series (9a).
		if spec.TextTemplate == "" && spec.EmptyTemplate {
			value = setBSONField(value, "TextTemplate", BuildEmptyClientTemplate())
			break
		}
		// Skip when the spec carries no text — leave the value's existing
		// TextTemplate untouched (null, set by createDefaultWidgetValue).
		// Inserting a placeholder ClientTemplate here causes Studio Pro
		// CE0463 on object-list items where the field is conditionally
		// unset (e.g., Accordion headerText when HeaderRenderMode is custom).
		if spec.TextTemplate != "" {
			var tmpl bson.D
			if len(spec.Parameters) > 0 {
				// Caller resolved CaptionParams / ContentParams into
				// ClientTemplateParameter[] — serialize them alongside the
				// text. Mirrors the keyword path's
				// BuildClientTemplateWithTextAndParams output.
				tmpl = BuildClientTemplateWithTextAndParams(spec.TextTemplate, spec.Parameters)
			} else {
				tmpl = createClientTemplateBSONWithParams(spec.TextTemplate, spec.EntityContext)
			}
			value = setBSONField(value, "TextTemplate", tmpl)
		}
	case "datasource":
		if spec.DataSource != nil {
			value = setDataSource(value, spec.DataSource)
		}
	case "action":
		if spec.Action != nil {
			actionBSON := pkgChildSer.SerializeClientAction(spec.Action)
			value = setBSONField(value, "Action", actionBSON)
		}
	}
	return value
}

// buildItemChildWidgetsProperty builds an item sub-property whose value type is
// Widgets — populates the Widgets array with serialized child widgets.
func buildItemChildWidgetsProperty(entry pages.PropertyTypeIDEntry, children []pages.Widget) bson.D {
	value := createDefaultWidgetValue(entry)
	widgetsArr := bson.A{int32(2)}
	for _, w := range children {
		widgetsArr = append(widgetsArr, pkgChildSer.SerializeWidget(w))
	}
	value = setBSONField(value, "Widgets", widgetsArr)
	return bson.D{
		{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: types.UUIDToBlob(entry.PropertyTypeID)},
		{Key: "Value", Value: value},
	}
}

// setBSONField returns a copy of the bson.D with the named key's value
// replaced. If the key is absent, the original is returned unchanged.
func setBSONField(doc bson.D, key string, val any) bson.D {
	result := make(bson.D, len(doc))
	for i, elem := range doc {
		if elem.Key == key {
			result[i] = bson.E{Key: key, Value: val}
		} else {
			result[i] = elem
		}
	}
	return result
}

// setAttributeRefField sets the AttributeRef field on a WidgetValue BSON to
// reference the given fully-qualified attribute path (Module.Entity.Attr).
//
// The BSON $Type is DomainModels$AttributeRef — CustomWidgets$AttributeRef
// is not a registered Mendix type and triggers TypeCacheUnknownTypeException
// when Studio Pro or mx update-widgets loads the project (issue #64).
func setAttributeRefField(value bson.D, attributePath string, steps []pages.AttributeRefStep) bson.D {
	if strings.Count(attributePath, ".") < 2 {
		return setBSONField(value, "AttributeRef", nil)
	}
	attrRef := bson.D{
		{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
		{Key: "$Type", Value: "DomainModels$AttributeRef"},
		{Key: "Attribute", Value: attributePath},
		{Key: "EntityRef", Value: attributeEntityRefBSON(steps)},
	}
	return setBSONField(value, "AttributeRef", attrRef)
}

// attributeEntityRefBSON builds the DomainModels$IndirectEntityRef that navigates
// an attribute over associations (one EntityRefStep per hop), or nil for an
// own-entity attribute. Mirrors the codec-form attributeRefWithStepsToGen and the
// legacy serializeAssociationSource EntityRef structure.
func attributeEntityRefBSON(steps []pages.AttributeRefStep) any {
	if len(steps) == 0 {
		return nil
	}
	stepArr := bson.A{int32(2)}
	for _, s := range steps {
		stepArr = append(stepArr, bson.D{
			{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
			{Key: "$Type", Value: "DomainModels$EntityRefStep"},
			{Key: "Association", Value: s.Association},
			{Key: "DestinationEntity", Value: s.DestinationEntity},
		})
	}
	return bson.D{
		{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
		{Key: "$Type", Value: "DomainModels$IndirectEntityRef"},
		{Key: "Steps", Value: stepArr},
	}
}

func (ob *Builder) SetAttributeObjects(propertyKey string, attributePaths []string) {
	if len(attributePaths) == 0 {
		return
	}

	entry, ok := ob.propertyTypeIDs[propertyKey]
	if !ok || entry.ObjectTypeID == "" {
		return
	}

	nestedEntry, ok := entry.NestedPropertyIDs["attribute"]
	if !ok {
		return
	}

	ob.object = updateWidgetPropertyValue(ob.object, ob.propertyTypeIDs, propertyKey, func(val bson.D) bson.D {
		objects := make([]any, 0, len(attributePaths)+1)
		objects = append(objects, int32(2)) // BSON array version marker

		for _, attrPath := range attributePaths {
			attrObj, err := CreateAttributeObject(attrPath, entry.ObjectTypeID, nestedEntry.PropertyTypeID, nestedEntry.ValueTypeID)
			if err != nil {
				// TODO(shared-types): propagate error instead of logging — requires interface change.
				log.Printf("warning: skipping attribute %s: %v", attrPath, err)
				continue
			}
			objects = append(objects, attrObj)
		}

		result := make(bson.D, 0, len(val))
		for _, elem := range val {
			if elem.Key == "Objects" {
				result = append(result, bson.E{Key: "Objects", Value: bson.A(objects)})
			} else {
				result = append(result, elem)
			}
		}
		return result
	})

	// A filter widget (DatagridTextFilter etc.) with an explicit `attributes`
	// list must select attrChoice="linked" ("Custom"). The "auto" default fits
	// only column-bound filters that carry no attributes and bind to the grid
	// column; Studio Pro 11.10+ flags attrChoice="auto" alongside a populated
	// attributes list as definition drift (CE0463). The two enum values are
	// "auto" and "linked".
	if _, ok := ob.propertyTypeIDs["attrChoice"]; ok {
		ob.object = updateWidgetPropertyValue(ob.object, ob.propertyTypeIDs, "attrChoice", func(val bson.D) bson.D {
			return setPrimitiveValue(val, "linked")
		})
	}
}

// ---------------------------------------------------------------------------
// Template metadata
// ---------------------------------------------------------------------------

func (ob *Builder) PropertyTypeIDs() map[string]pages.PropertyTypeIDEntry {
	return ob.propertyTypeIDs
}

// ---------------------------------------------------------------------------
// Object list defaults
// ---------------------------------------------------------------------------

func (ob *Builder) EnsureRequiredObjectLists() {
	ob.object = ensureRequiredObjectLists(ob.object, ob.propertyTypeIDs)
}

// ---------------------------------------------------------------------------
// Property visibility (#574)
// ---------------------------------------------------------------------------

// ApplyPropertyVisibility nulls the TextTemplate of any TextTemplate-typed
// property the rules mark as hidden under the widget's current configuration.
// Non-TextTemplate properties are left untouched: only the populated-vs-null
// ClientTemplate choice triggers CE0463, so Phase 1 scopes the action to
// TextTemplate. The widget's current primitive values (read from the assembled
// object) drive rule evaluation, so a rule keyed on e.g. `type` sees the value
// just set from MDL.
func (ob *Builder) ApplyPropertyVisibility(rules []types.WidgetVisibilityRule) {
	if len(rules) == 0 {
		return
	}
	values := ob.currentPrimitiveValues()
	for _, rule := range rules {
		if !rule.HiddenWhen.Hidden(values) {
			continue
		}
		entry, ok := ob.propertyTypeIDs[rule.PropertyKey]
		if !ok || entry.ValueType != "TextTemplate" {
			continue
		}
		ob.object = updateWidgetPropertyValue(ob.object, ob.propertyTypeIDs, rule.PropertyKey, func(val bson.D) bson.D {
			return setBSONField(val, "TextTemplate", nil)
		})
	}
}

// currentPrimitiveValues maps each known property key to its current
// PrimitiveValue string in the assembled object (e.g. type → "expression",
// customVisualization → "true"). Properties absent from the object map to "".
func (ob *Builder) currentPrimitiveValues() map[string]string {
	byID := make(map[string]string)
	for _, elem := range ob.object {
		if elem.Key != "Properties" {
			continue
		}
		arr, ok := elem.Value.(bson.A)
		if !ok {
			continue
		}
		for _, item := range arr {
			prop, ok := item.(bson.D)
			if !ok {
				continue
			}
			id := propertyTypePointerID(prop)
			if id == "" {
				continue
			}
			byID[id] = primitiveValueOfProperty(prop)
		}
	}

	out := make(map[string]string, len(ob.propertyTypeIDs))
	for key, entry := range ob.propertyTypeIDs {
		if v, ok := byID[strings.ReplaceAll(entry.PropertyTypeID, "-", "")]; ok {
			out[key] = v
		}
	}
	return out
}

// propertyTypePointerID returns a WidgetProperty's TypePointer as a normalized
// (dash-stripped) UUID hex string, or "" when absent.
func propertyTypePointerID(prop bson.D) string {
	for _, elem := range prop {
		if elem.Key != "TypePointer" {
			continue
		}
		switch v := elem.Value.(type) {
		case primitive.Binary:
			return strings.ReplaceAll(types.BlobToUUID(v.Data), "-", "")
		case []byte:
			return strings.ReplaceAll(types.BlobToUUID(v), "-", "")
		}
	}
	return ""
}

// primitiveValueOfProperty reads Value.PrimitiveValue from a WidgetProperty as
// a string, or "" when missing/non-string.
func primitiveValueOfProperty(prop bson.D) string {
	for _, elem := range prop {
		if elem.Key != "Value" {
			continue
		}
		val, ok := elem.Value.(bson.D)
		if !ok {
			return ""
		}
		for _, ve := range val {
			if ve.Key == "PrimitiveValue" {
				if s, ok := ve.Value.(string); ok {
					return s
				}
				return ""
			}
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Gallery-specific
// ---------------------------------------------------------------------------

func (ob *Builder) CloneGallerySelectionProperty(propertyKey string, selectionMode string) {
	propEntry, ok := ob.propertyTypeIDs[propertyKey]
	if !ok {
		return
	}

	// Work at the Properties array level: find the property, clone it with new
	// IDs and updated Selection, then append.
	result := make(bson.D, 0, len(ob.object))
	for _, elem := range ob.object {
		if elem.Key == "Properties" {
			if arr, ok := elem.Value.(bson.A); ok {
				newArr := make(bson.A, len(arr))
				copy(newArr, arr)
				// Find the matching property and clone it
				for _, item := range arr {
					if prop, ok := item.(bson.D); ok {
						if matchesTypePointer(prop, propEntry.PropertyTypeID) {
							cloned := buildGallerySelectionProperty(prop, selectionMode)
							newArr = append(newArr, cloned)
							break
						}
					}
				}
				result = append(result, bson.E{Key: "Properties", Value: newArr})
				continue
			}
		}
		result = append(result, elem)
	}
	ob.object = result
}

// ---------------------------------------------------------------------------
// Finalize
// ---------------------------------------------------------------------------

func (ob *Builder) Finalize(id model.ID, name string, label string, editable string) *pages.CustomWidget {
	return &pages.CustomWidget{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       id,
				TypeName: "CustomWidgets$CustomWidget",
			},
			Name: name,
		},
		Label:             label,
		Editable:          editable,
		RawType:           ob.embeddedType,
		RawObject:         ob.object,
		PropertyTypeIDMap: ob.propertyTypeIDs,
		ObjectTypeID:      ob.objectTypeID,
	}
}

// ===========================================================================
// Package-level helpers (moved from executor)
// ===========================================================================

// ---------------------------------------------------------------------------
// Property update core
// ---------------------------------------------------------------------------

// updateWidgetPropertyValue finds and updates a specific property value in a WidgetObject.
func updateWidgetPropertyValue(obj bson.D, propTypeIDs map[string]pages.PropertyTypeIDEntry, propertyKey string, updateFn func(bson.D) bson.D) bson.D {
	propEntry, ok := propTypeIDs[propertyKey]
	if !ok {
		return obj
	}

	result := make(bson.D, 0, len(obj))
	for _, elem := range obj {
		if elem.Key == "Properties" {
			if arr, ok := elem.Value.(bson.A); ok {
				result = append(result, bson.E{Key: "Properties", Value: updatePropertyInArray(arr, propEntry.PropertyTypeID, updateFn)})
				continue
			}
		}
		result = append(result, elem)
	}
	return result
}

// updatePropertyInArray finds a property by TypePointer and updates its value.
func updatePropertyInArray(arr bson.A, propertyTypeID string, updateFn func(bson.D) bson.D) bson.A {
	result := make(bson.A, len(arr))
	matched := false
	for i, item := range arr {
		if prop, ok := item.(bson.D); ok {
			if matchesTypePointer(prop, propertyTypeID) {
				result[i] = updatePropertyValue(prop, updateFn)
				matched = true
			} else {
				result[i] = item
			}
		} else {
			result[i] = item
		}
	}
	if !matched {
		// TODO(shared-types): propagate warning instead of logging — requires interface change.
		log.Printf("warning: updatePropertyInArray: no match for TypePointer %s in %d properties", propertyTypeID, len(arr)-1)
	}
	return result
}

// matchesTypePointer checks if a WidgetProperty has the given TypePointer.
func matchesTypePointer(prop bson.D, propertyTypeID string) bool {
	normalizedTarget := strings.ReplaceAll(propertyTypeID, "-", "")
	for _, elem := range prop {
		if elem.Key == "TypePointer" {
			switch v := elem.Value.(type) {
			case primitive.Binary:
				propID := strings.ReplaceAll(types.BlobToUUID(v.Data), "-", "")
				return propID == normalizedTarget
			case []byte:
				propID := strings.ReplaceAll(types.BlobToUUID(v), "-", "")
				if propID == normalizedTarget {
					return true
				}
				rawHex := fmt.Sprintf("%x", v)
				return rawHex == normalizedTarget
			}
		}
	}
	return false
}

// updatePropertyValue updates the Value field in a WidgetProperty.
func updatePropertyValue(prop bson.D, updateFn func(bson.D) bson.D) bson.D {
	result := make(bson.D, 0, len(prop))
	for _, elem := range prop {
		if elem.Key == "Value" {
			if val, ok := elem.Value.(bson.D); ok {
				result = append(result, bson.E{Key: "Value", Value: updateFn(val)})
			} else {
				result = append(result, elem)
			}
		} else {
			result = append(result, elem)
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// Value setters
// ---------------------------------------------------------------------------

func setPrimitiveValue(val bson.D, value string) bson.D {
	result := make(bson.D, 0, len(val))
	for _, elem := range val {
		if elem.Key == "PrimitiveValue" {
			result = append(result, bson.E{Key: "PrimitiveValue", Value: value})
		} else {
			result = append(result, elem)
		}
	}
	return result
}

func setDataSource(val bson.D, ds pages.DataSource) bson.D {
	result := make(bson.D, 0, len(val))
	for _, elem := range val {
		if elem.Key == "DataSource" {
			result = append(result, bson.E{Key: "DataSource", Value: pkgChildSer.SerializeCustomWidgetDataSource(ds)})
		} else {
			result = append(result, elem)
		}
	}
	return result
}

func setAssociationRef(val bson.D, assocPath string, entityName string) bson.D {
	result := make(bson.D, 0, len(val))
	for _, elem := range val {
		if elem.Key == "EntityRef" && entityName != "" {
			result = append(result, bson.E{Key: "EntityRef", Value: bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "DomainModels$IndirectEntityRef"},
				{Key: "Steps", Value: bson.A{
					int32(2),
					bson.D{
						{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
						{Key: "$Type", Value: "DomainModels$EntityRefStep"},
						{Key: "Association", Value: assocPath},
						{Key: "DestinationEntity", Value: entityName},
					},
				}},
			}})
		} else {
			result = append(result, elem)
		}
	}
	return result
}

func setAttributeRef(val bson.D, attrPath string) bson.D {
	result := make(bson.D, 0, len(val))
	for _, elem := range val {
		if elem.Key == "AttributeRef" {
			if strings.Count(attrPath, ".") >= 2 {
				result = append(result, bson.E{Key: "AttributeRef", Value: bson.D{
					{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
					{Key: "$Type", Value: "DomainModels$AttributeRef"},
					{Key: "Attribute", Value: attrPath},
					{Key: "EntityRef", Value: nil},
				}})
			} else {
				result = append(result, bson.E{Key: "AttributeRef", Value: nil})
			}
		} else {
			result = append(result, elem)
		}
	}
	return result
}

func setChildWidgets(val bson.D, children []pages.Widget) bson.D {
	widgetsArr := bson.A{int32(2)}
	for _, w := range children {
		widgetsArr = append(widgetsArr, pkgChildSer.SerializeWidget(w))
	}

	result := make(bson.D, 0, len(val))
	for _, elem := range val {
		if elem.Key == "Widgets" {
			result = append(result, bson.E{Key: "Widgets", Value: widgetsArr})
		} else {
			result = append(result, elem)
		}
	}
	return result
}

func setTextTemplateValue(val bson.D, text string) bson.D {
	result := make(bson.D, 0, len(val))
	for _, elem := range val {
		if elem.Key == "TextTemplate" {
			if tmpl, ok := elem.Value.(bson.D); ok && tmpl != nil {
				result = append(result, bson.E{Key: "TextTemplate", Value: updateTemplateText(tmpl, text)})
			} else {
				result = append(result, elem)
			}
		} else {
			result = append(result, elem)
		}
	}
	return result
}

func updateTemplateText(tmpl bson.D, text string) bson.D {
	result := make(bson.D, 0, len(tmpl))
	for _, elem := range tmpl {
		if elem.Key == "Template" {
			if template, ok := elem.Value.(bson.D); ok {
				updated := make(bson.D, 0, len(template))
				for _, tElem := range template {
					if tElem.Key == "Items" {
						updated = append(updated, bson.E{Key: "Items", Value: bson.A{
							int32(3),
							bson.D{
								{Key: "$ID", Value: bsonutil.IDToBsonBinary(types.GenerateID())},
								{Key: "$Type", Value: "Texts$Translation"},
								{Key: "LanguageCode", Value: "en_US"},
								{Key: "Text", Value: text},
							},
						}})
					} else {
						updated = append(updated, tElem)
					}
				}
				result = append(result, bson.E{Key: "Template", Value: updated})
			} else {
				result = append(result, elem)
			}
		} else {
			result = append(result, elem)
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// Template helpers
// ---------------------------------------------------------------------------

func createClientTemplateBSONWithParams(text string, entityContext string) bson.D {
	re := regexp.MustCompile(`\{([A-Za-z][A-Za-z0-9_]*)\}`)
	matches := re.FindAllStringSubmatchIndex(text, -1)

	if len(matches) == 0 {
		return createDefaultClientTemplateBSON(text)
	}

	// Collect attribute names (skip numeric placeholders)
	var attrNames []string
	for i := 0; i < len(matches); i++ {
		match := matches[i]
		attrName := text[match[2]:match[3]]
		if _, err := fmt.Sscanf(attrName, "%d", new(int)); err == nil {
			continue
		}
		attrNames = append(attrNames, attrName)
	}

	paramText := re.ReplaceAllStringFunc(text, func(s string) string {
		name := s[1 : len(s)-1]
		if _, err := fmt.Sscanf(name, "%d", new(int)); err == nil {
			return s
		}
		for i, an := range attrNames {
			if an == name {
				return fmt.Sprintf("{%d}", i+1)
			}
		}
		return s
	})

	// Build parameters BSON
	params := bson.A{int32(2)}
	for _, attrName := range attrNames {
		attrPath := attrName
		if entityContext != "" && !strings.Contains(attrName, ".") {
			attrPath = entityContext + "." + attrName
		}
		params = append(params, bson.D{
			{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
			{Key: "$Type", Value: "Forms$ClientTemplateParameter"},
			{Key: "AttributeRef", Value: bson.D{
				{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
				{Key: "$Type", Value: "DomainModels$AttributeRef"},
				{Key: "Attribute", Value: attrPath},
				{Key: "EntityRef", Value: nil},
			}},
			{Key: "Expression", Value: ""},
			{Key: "FormattingInfo", Value: bson.D{
				{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
				{Key: "$Type", Value: "Forms$FormattingInfo"},
				{Key: "CustomDateFormat", Value: ""},
				{Key: "DateFormat", Value: "Date"},
				{Key: "DecimalPrecision", Value: int64(2)},
				{Key: "EnumFormat", Value: "Text"},
				{Key: "GroupDigits", Value: false},
			}},
			{Key: "SourceVariable", Value: nil},
		})
	}

	// Studio Pro convention: caption-style ClientTemplates carry the text
	// in Template and leave Fallback as an empty Items array (count marker
	// only). The engine path now mirrors the keyword path's
	// BuildClientTemplateWithTextAndParams output in datagrid_builder.go.
	emptyText := func() bson.D {
		return bson.D{
			{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3)}},
		}
	}
	populatedText := func(t string) bson.D {
		return bson.D{
			{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3), bson.D{
				{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
				{Key: "$Type", Value: "Texts$Translation"},
				{Key: "LanguageCode", Value: "en_US"},
				{Key: "Text", Value: t},
			}}},
		}
	}

	return bson.D{
		{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
		{Key: "$Type", Value: "Forms$ClientTemplate"},
		{Key: "Fallback", Value: emptyText()},
		{Key: "Parameters", Value: params},
		{Key: "Template", Value: populatedText(paramText)},
	}
}

func createDefaultClientTemplateBSON(text string) bson.D {
	emptyText := func() bson.D {
		return bson.D{
			{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3)}},
		}
	}
	populatedText := func(t string) bson.D {
		return bson.D{
			{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3), bson.D{
				{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
				{Key: "$Type", Value: "Texts$Translation"},
				{Key: "LanguageCode", Value: "en_US"},
				{Key: "Text", Value: t},
			}}},
		}
	}
	return bson.D{
		{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
		{Key: "$Type", Value: "Forms$ClientTemplate"},
		{Key: "Fallback", Value: emptyText()},
		{Key: "Parameters", Value: bson.A{int32(2)}},
		{Key: "Template", Value: populatedText(text)},
	}
}

// ---------------------------------------------------------------------------
// Property type ID conversion
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Default object lists
// ---------------------------------------------------------------------------

func ensureRequiredObjectLists(obj bson.D, propertyTypeIDs map[string]pages.PropertyTypeIDEntry) bson.D {
	// Sort keys for deterministic BSON output.
	keys := make([]string, 0, len(propertyTypeIDs))
	for k := range propertyTypeIDs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, propKey := range keys {
		entry := propertyTypeIDs[propKey]
		if entry.ObjectTypeID == "" || len(entry.NestedPropertyIDs) == 0 {
			continue
		}
		if !entry.Required {
			hasNestedDS := false
			for _, nested := range entry.NestedPropertyIDs {
				if nested.ValueType == "DataSource" {
					hasNestedDS = true
					break
				}
			}
			if hasNestedDS {
				continue
			}
		}
		// Skip auto-populate when any nested property has a complex ValueType
		// (Attribute / Expression / TextTemplate / Widgets / DataSource).
		// Complex types have no sensible empty default — Studio Pro flags an
		// auto-generated entry with empty Expression/Attribute as CE0463/CE0566.
		// This also avoids over-populating mode-dependent required lists such as
		// Combobox optionsSourceStaticDataSource (only used when source=static).
		hasComplexNested := false
		for _, nested := range entry.NestedPropertyIDs {
			switch nested.ValueType {
			case "Attribute", "Expression", "TextTemplate", "Widgets", "DataSource":
				hasComplexNested = true
			}
			if hasComplexNested {
				break
			}
		}
		if hasComplexNested {
			continue
		}
		obj = updateWidgetPropertyValue(obj, propertyTypeIDs, propKey, func(val bson.D) bson.D {
			for _, elem := range val {
				if elem.Key == "Objects" {
					if arr, ok := elem.Value.(bson.A); ok && len(arr) <= 1 {
						defaultObj := createDefaultWidgetObject(entry.ObjectTypeID, entry.NestedPropertyIDs)
						newArr := bson.A{int32(2), defaultObj}
						result := make(bson.D, 0, len(val))
						for _, e := range val {
							if e.Key == "Objects" {
								result = append(result, bson.E{Key: "Objects", Value: newArr})
							} else {
								result = append(result, e)
							}
						}
						return result
					}
				}
			}
			return val
		})
	}
	return obj
}

func createDefaultWidgetObject(objectTypeID string, nestedProps map[string]pages.PropertyTypeIDEntry) bson.D {
	propsArr := bson.A{int32(2)}
	// Sort keys for deterministic BSON output.
	nestedKeys := make([]string, 0, len(nestedProps))
	for k := range nestedProps {
		nestedKeys = append(nestedKeys, k)
	}
	sort.Strings(nestedKeys)
	for _, k := range nestedKeys {
		entry := nestedProps[k]
		prop := createDefaultWidgetProperty(entry)
		propsArr = append(propsArr, prop)
	}
	return bson.D{
		{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
		{Key: "$Type", Value: "CustomWidgets$WidgetObject"},
		{Key: "TypePointer", Value: types.UUIDToBlob(objectTypeID)},
		{Key: "Properties", Value: propsArr},
	}
}

func createDefaultWidgetProperty(entry pages.PropertyTypeIDEntry) bson.D {
	return bson.D{
		{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: types.UUIDToBlob(entry.PropertyTypeID)},
		{Key: "Value", Value: createDefaultWidgetValue(entry)},
	}
}

func createDefaultWidgetValue(entry pages.PropertyTypeIDEntry) bson.D {
	primitiveVal := entry.DefaultValue
	expressionVal := ""
	var textTemplate any

	switch entry.ValueType {
	case "Expression":
		expressionVal = primitiveVal
		primitiveVal = ""
	case "TextTemplate":
		// When the property has no schema default text, leave TextTemplate
		// as null rather than manufacturing a single-space ClientTemplate.
		// Studio Pro rejects the latter with CE0463 on object-list items
		// (e.g., Accordion `headerText` when HeaderRenderMode is "custom").
		if primitiveVal != "" {
			textTemplate = createDefaultClientTemplateBSON(primitiveVal)
		}
	case "String":
		if primitiveVal == "" {
			primitiveVal = " "
		}
	}

	return bson.D{
		{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
		{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
		{Key: "Action", Value: bson.D{
			{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
			{Key: "$Type", Value: "Forms$NoAction"},
			{Key: "DisabledDuringExecution", Value: true},
		}},
		{Key: "AttributeRef", Value: nil},
		{Key: "DataSource", Value: nil},
		{Key: "EntityRef", Value: nil},
		{Key: "Expression", Value: expressionVal},
		{Key: "Form", Value: ""},
		{Key: "Icon", Value: nil},
		{Key: "Image", Value: ""},
		{Key: "Microflow", Value: ""},
		{Key: "Nanoflow", Value: ""},
		{Key: "Objects", Value: bson.A{int32(2)}},
		{Key: "PrimitiveValue", Value: primitiveVal},
		{Key: "Selection", Value: "None"},
		{Key: "SourceVariable", Value: nil},
		{Key: "TextTemplate", Value: textTemplate},
		{Key: "TranslatableValue", Value: nil},
		{Key: "TypePointer", Value: types.UUIDToBlob(entry.ValueTypeID)},
		{Key: "Widgets", Value: bson.A{int32(2)}},
		{Key: "XPathConstraint", Value: ""},
	}
}

// ---------------------------------------------------------------------------
// Gallery cloning
// ---------------------------------------------------------------------------

func buildGallerySelectionProperty(propMap bson.D, selectionMode string) bson.D {
	result := make(bson.D, 0, len(propMap))

	for _, elem := range propMap {
		if elem.Key == "$ID" {
			result = append(result, bson.E{Key: "$ID", Value: bsonutil.NewIDBsonBinary()})
		} else if elem.Key == "Value" {
			if valueMap, ok := elem.Value.(bson.D); ok {
				result = append(result, bson.E{Key: "Value", Value: cloneGallerySelectionValue(valueMap, selectionMode)})
			} else {
				result = append(result, elem)
			}
		} else {
			result = append(result, elem)
		}
	}

	return result
}

func cloneGallerySelectionValue(valueMap bson.D, selectionMode string) bson.D {
	result := make(bson.D, 0, len(valueMap))

	for _, elem := range valueMap {
		if elem.Key == "$ID" {
			result = append(result, bson.E{Key: "$ID", Value: bsonutil.NewIDBsonBinary()})
		} else if elem.Key == "Selection" {
			result = append(result, bson.E{Key: "Selection", Value: selectionMode})
		} else if elem.Key == "Action" {
			if actionMap, ok := elem.Value.(bson.D); ok {
				result = append(result, bson.E{Key: "Action", Value: cloneActionWithNewID(actionMap)})
			} else {
				result = append(result, elem)
			}
		} else {
			result = append(result, elem)
		}
	}

	return result
}

func cloneActionWithNewID(actionMap bson.D) bson.D {
	result := make(bson.D, 0, len(actionMap))

	for _, elem := range actionMap {
		if elem.Key == "$ID" {
			result = append(result, bson.E{Key: "$ID", Value: bsonutil.NewIDBsonBinary()})
		} else {
			result = append(result, elem)
		}
	}

	return result
}

// ---------------------------------------------------------------------------
// Attribute object creation
// ---------------------------------------------------------------------------

func CreateAttributeObject(attributePath string, objectTypeID, propertyTypeID, valueTypeID string) (bson.D, error) {
	if strings.Count(attributePath, ".") < 2 {
		return nil, mdlerrors.NewValidationf("invalid attribute path %q: expected Module.Entity.Attribute format", attributePath)
	}
	return bson.D{
		{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
		{Key: "$Type", Value: "CustomWidgets$WidgetObject"},
		{Key: "Properties", Value: bson.A{
			int32(2),
			bson.D{
				{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
				{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
				{Key: "TypePointer", Value: types.UUIDToBlob(propertyTypeID)},
				{Key: "Value", Value: bson.D{
					{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
					{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
					{Key: "Action", Value: bson.D{
						{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
						{Key: "$Type", Value: "Forms$NoAction"},
						{Key: "DisabledDuringExecution", Value: true},
					}},
					{Key: "AttributeRef", Value: bson.D{
						{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
						{Key: "$Type", Value: "DomainModels$AttributeRef"},
						{Key: "Attribute", Value: attributePath},
						{Key: "EntityRef", Value: nil},
					}},
					{Key: "DataSource", Value: nil},
					{Key: "EntityRef", Value: nil},
					{Key: "Expression", Value: ""},
					{Key: "Form", Value: ""},
					{Key: "Icon", Value: nil},
					{Key: "Image", Value: ""},
					{Key: "Microflow", Value: ""},
					{Key: "Nanoflow", Value: ""},
					{Key: "Objects", Value: bson.A{int32(2)}},
					{Key: "PrimitiveValue", Value: ""},
					{Key: "Selection", Value: "None"},
					{Key: "SourceVariable", Value: nil},
					{Key: "TextTemplate", Value: nil},
					{Key: "TranslatableValue", Value: nil},
					{Key: "TypePointer", Value: types.UUIDToBlob(valueTypeID)},
					{Key: "Widgets", Value: bson.A{int32(2)}},
					{Key: "XPathConstraint", Value: ""},
				}},
			},
		}},
		{Key: "TypePointer", Value: types.UUIDToBlob(objectTypeID)},
	}, nil
}

// --- moved from datagrid_builder.go (shared pure helpers) ---

// BuildClientTemplateWithTextAndParams builds a Forms$ClientTemplate with the
// given Template text and an optional list of ClientTemplateParameters.
// Mirrors sdk/mpr/writer_widgets.go:serializeClientTemplate for the templated
// column header / dynamicText paths.
func BuildClientTemplateWithTextAndParams(text string, params []*pages.ClientTemplateParameter) bson.D {
	parametersArr := bson.A{int32(2)} // empty array marker; populated below if params exist
	if len(params) > 0 {
		for _, p := range params {
			parametersArr = append(parametersArr, SerializeColumnClientTemplateParameter(p))
		}
	}
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Forms$ClientTemplate"},
		{Key: "Fallback", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3)}},
		}},
		{Key: "Parameters", Value: parametersArr},
		{Key: "Template", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{
				int32(3),
				bson.D{
					{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
					{Key: "$Type", Value: "Texts$Translation"},
					{Key: "LanguageCode", Value: "en_US"},
					{Key: "Text", Value: text},
				},
			}},
		}},
	}
}

// SerializeColumnClientTemplateParameter serializes a ClientTemplateParameter
// for embedding inside a column TextTemplate. Mirrors the structure used by
// sdk/mpr/writer_widgets.go:serializeClientTemplateParameter (Forms$FormattingInfo
// schema-aligned to avoid CE0463).
func SerializeColumnClientTemplateParameter(param *pages.ClientTemplateParameter) bson.D {
	paramID := bsonutil.NewIDBsonBinary()
	if param.ID != "" {
		paramID = bsonutil.IDToBsonBinary(string(param.ID))
	}

	var attrRefBSON any
	if param.AttributeRef != "" {
		attrRefBSON = bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "DomainModels$AttributeRef"},
			{Key: "Attribute", Value: param.AttributeRef},
			{Key: "EntityRef", Value: nil},
		}
	}

	formattingInfo := bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Forms$FormattingInfo"},
		{Key: "CustomDateFormat", Value: ""},
		{Key: "DateFormat", Value: "Date"},
		{Key: "DecimalPrecision", Value: int64(2)},
		{Key: "EnumFormat", Value: "Text"},
		{Key: "GroupDigits", Value: false},
	}

	var sourceVariable any
	if param.SourceVariable != "" {
		// Studio Pro distinguishes between three Forms$PageVariable bindings:
		//   - LocalVariable     → page-level Variables entry (Kind="local")
		//   - SnippetParameter  → snippet parameter            (Kind="snippet")
		//   - PageParameter     → page parameter               (Kind="" default)
		// Emitting a $localVar reference as a literal Expression causes Studio
		// Pro to interpret the value as an entity attribute path.
		fields := bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$PageVariable"},
		}
		switch param.SourceVariableKind {
		case "local":
			fields = append(fields,
				bson.E{Key: "LocalVariable", Value: param.SourceVariable},
				bson.E{Key: "PageParameter", Value: ""},
				bson.E{Key: "SnippetParameter", Value: ""},
			)
		case "snippet":
			fields = append(fields,
				bson.E{Key: "LocalVariable", Value: ""},
				bson.E{Key: "PageParameter", Value: ""},
				bson.E{Key: "SnippetParameter", Value: param.SourceVariable},
			)
		default:
			fields = append(fields,
				bson.E{Key: "LocalVariable", Value: ""},
				bson.E{Key: "PageParameter", Value: param.SourceVariable},
				bson.E{Key: "SnippetParameter", Value: ""},
			)
		}
		fields = append(fields,
			bson.E{Key: "SubKey", Value: ""},
			bson.E{Key: "UseAllPages", Value: false},
			bson.E{Key: "Widget", Value: ""},
		)
		sourceVariable = fields
	}

	return bson.D{
		{Key: "$ID", Value: paramID},
		{Key: "$Type", Value: "Forms$ClientTemplateParameter"},
		{Key: "AttributeRef", Value: attrRefBSON},
		{Key: "Expression", Value: param.Expression},
		{Key: "FormattingInfo", Value: formattingInfo},
		{Key: "SourceVariable", Value: sourceVariable},
	}
}

func BuildEmptyClientTemplate() bson.D {
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Forms$ClientTemplate"},
		{Key: "Fallback", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3)}},
		}},
		{Key: "Parameters", Value: bson.A{int32(2)}},
		{Key: "Template", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3)}},
		}},
	}
}
