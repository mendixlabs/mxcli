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

// SetSourceVariable stores the Forms$PageVariable an attribute or association
// property reads from — `$Param.Attr`, a page or snippet parameter — in the
// property's WidgetValue, where Studio Pro keeps it (TestApp
// WorkflowCommons.Snip_TaskDashboard_Header: SnippetParameter "DashboardContext"
// beside the AttributeRef / IndirectEntityRef). Without one the value is read
// from the enclosing object, as before.
func (ob *Builder) SetSourceVariable(propertyKey string, sv *pages.WidgetVariable) {
	if sv == nil || (sv.Widget == "" && sv.Variable == "") {
		return
	}
	ob.object = updateWidgetPropertyValue(ob.object, ob.propertyTypeIDs, propertyKey, func(val bson.D) bson.D {
		return setBSONField(val, "SourceVariable", PageVariableBSON(sv.Widget, sv.Variable, sv.Kind))
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

// SetImage points an image-typed property at an image collection entry, by its
// three-part qualified name (`Module.Collection.Image`).
//
// The Image widget's `imageObject` is the case this exists for: it holds the
// image the widget shows, and without it the default `ImageType: image` writes a
// model mxbuild refuses with "No image selected." It is also what a describe →
// exec copy of an Atlas layout needs in order to keep its brand image
// (mxcli-formula1 FINDINGS §142).
func (ob *Builder) SetImage(propertyKey string, imageQN string) {
	if imageQN == "" {
		return
	}
	ob.object = updateWidgetPropertyValue(ob.object, ob.propertyTypeIDs, propertyKey, func(val bson.D) bson.D {
		return setImageValue(val, imageQN)
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

// SetTextTemplateWithClientParams sets a text template from author-supplied
// parameters (MDL `contentparams:`), for a `{1}`-style template.
//
// SetTextTemplateWithParams derives parameters by matching `{AttrName}` against
// the entity context; that covers the named spelling but leaves the numeric one
// with no route, so a pluggable widget's `imageUrl: '{1}', contentparams: [...]`
// was stored with Parameters=[2] (empty) and mxbuild answered CE0720 "Place
// holder index 1 is greater than 0, the number of parameter(s)". (#928)
func (ob *Builder) SetTextTemplateWithClientParams(propertyKey string, text string, params []*pages.ClientTemplateParameter) {
	if text == "" {
		return
	}
	tmpl := BuildClientTemplateWithTextAndParams(text, params)
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
		case isUnsetRequiredTextTemplate(nestedEntry):
			prop = buildDefaultTextClientTemplateProperty(nestedEntry)
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
	itemKindDynamicText   objectListItemKind = "dynamictext"
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
	// A dynamic-text column (Show: dynamicText) has no attribute binding and no
	// content widgets, so it would otherwise fall through to itemKindDefault and
	// miss the tooltip empty-ClientTemplate convention Studio Pro applies to it
	// (CE0463, ledger #77). Detect it from the showContentAs primitive.
	if sca, ok := specByKey["showContentAs"]; ok && strings.EqualFold(sca.PrimitiveVal, "dynamicText") {
		return itemKindDynamicText
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
//	property        attribute column   dynamic-text column   custom-content column
//	tooltip         empty CT           empty CT              null
//	exportValue     null               null                 empty CT
//	dynamicText     null               (the cell template)  null
//
// The dynamic-text column matches the attribute column for tooltip/exportValue
// (verified against a Studio-Pro `mx update-widgets` reconciliation, ledger #77).
var emptyClientTemplateRules = map[string]map[string]map[objectListItemKind]map[string]bool{
	"com.mendix.widget.web.datagrid.Datagrid": {
		"columns": {
			itemKindAttribute: {
				"tooltip": true,
			},
			itemKindDynamicText: {
				"tooltip": true,
			},
			itemKindCustomContent: {
				"exportValue": true,
			},
		},
	},
}

// isUnsetRequiredTextTemplate reports whether an object-list item sub-property
// is a REQUIRED TextTemplate the author did not set (#891).
//
// This generalises emptyClientTemplateRules, which is a hardcoded table covering
// only DataGrid columns; every other object-list widget fell through it to a
// null. In a stock blank app an Accordion group (required `headerText`) and a
// Pop-up menu basic item (required `caption`) both raised CE0463, and thirteen
// shipped widgets declare object lists with texttemplate items.
//
// Required-ness comes from the widget's own PropertyTypes, so this needs no
// per-widget table and cannot go stale against a package upgrade. Note the
// widget XML schema defaults `required` to TRUE when the attribute is absent —
// exactly the Accordion's headerText — so a parser reading a missing attribute
// as false silently disables this (mpk.go encodes the default as
// `Required: p.Required != "false"`).
func isUnsetRequiredTextTemplate(e pages.PropertyTypeIDEntry) bool {
	return e.Required && strings.EqualFold(e.ValueType, "TextTemplate")
}

// buildDefaultTextClientTemplateProperty emits a required TextTemplate carrying
// the widget's shipped default text.
//
// Both weaker forms fail, which is why this is not simply the empty builder:
// TextTemplate=null is CE0463 "the definition of this widget has changed", and
// an EMPTY Forms$ClientTemplate is CE4899 "Property 'Groups/1/Text' is
// required". Populating from the ValueType's Translations is what
// `mx update-widgets` itself writes (the Accordion's headerText ships
// 'Header'/'Koptekst'). With no shipped translations there is nothing better to
// write than the empty template.
func buildDefaultTextClientTemplateProperty(entry pages.PropertyTypeIDEntry) bson.D {
	if len(entry.DefaultTranslations) == 0 {
		return buildEmptyClientTemplateProperty(entry)
	}
	value := createDefaultWidgetValue(entry)
	value = setBSONField(value, "TextTemplate", buildClientTemplateWithTranslations(entry.DefaultTranslations))
	return bson.D{
		{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: types.UUIDToBlob(entry.PropertyTypeID)},
		{Key: "Value", Value: value},
	}
}

// buildClientTemplateWithTranslations mirrors BuildEmptyClientTemplate but fills
// Template.Items with one Texts$Translation per shipped language. The Fallback
// stays empty and Parameters keeps marker 2 — the shape `mx update-widgets`
// produces.
func buildClientTemplateWithTranslations(translations []pages.PropertyTranslation) bson.D {
	items := bson.A{int32(3)}
	for _, t := range translations {
		items = append(items, bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Texts$Translation"},
			{Key: "LanguageCode", Value: t.LanguageCode},
			{Key: "Text", Value: t.Text},
		})
	}
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
			{Key: "Items", Value: items},
		}},
	}
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

// PrimitiveValues returns each property's current comparable value in the object
// being built, keyed as the widget declares it.
//
// Read before the property mappings are applied, this is the TEMPLATE's captured
// configuration — which is what an UNMAPPED property will actually be stored
// with, since nothing else ever touches one. That distinction is what a
// visibility rule has to be evaluated against: the declared default is what the
// property SHOULD hold, not what it will (see hiddenUnnamedProperties).
func (ob *Builder) PrimitiveValues() map[string]string {
	return primitiveValuesOf(ob.object, ob.propertyTypeIDs)
}

// ---------------------------------------------------------------------------
// Object list defaults
// ---------------------------------------------------------------------------

// EnsureRequiredObjectLists is intentionally a no-op: mxcli does not seed a
// placeholder row into an object list the author never wrote.
//
// It used to auto-populate any object list whose nested properties were all
// "simple" (nothing Attribute/Expression/TextTemplate/Widgets/DataSource).
// Studio Pro leaves such a list EMPTY, so the seeded row made the stored
// instance disagree with the installed .mpk — CE0463, "the definition of this
// widget has changed".
//
// Measured against the 31 shipped widget templates, the old heuristic fired on
// exactly two properties and helped neither:
//
//	barcodescanner  barcodeFormats  required, nested {Enumeration}
//	                -> seeded one row carrying the enum default AZTEC; three
//	                   Barcode Scanners on two pages each raised CE0463 and the
//	                   app would not deploy until Studio Pro's "Update widget"
//	                   deleted exactly that row.
//	htmlelement     events          optional, nested {Action,Boolean,Enumeration}
//	                -> seeded a phantom event handler Studio Pro never writes.
//
// The MCP write path (mdl/backend/mcp.(*mcpWidgetBuilder)) has always had this
// as a no-op, so removing the seeding also makes the two engines agree rather
// than making the model depend on which one authored it.
//
// This is NOT the sibling fix for issue #891. That one fills an *authored*
// object-list item's required TextTemplate with the widget's shipped
// translations, lives in buildObjectListItemBSON, and still applies — an absent
// `required` attribute in widget XML still means required (mpk.go:
// `Required: p.Required != "false"`). Nothing here changes that.
//
// The method is kept so backend.WidgetBuilder keeps its shape.
func (ob *Builder) EnsureRequiredObjectLists() {}

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
	ob.object = ApplyVisibilityRules(ob.object, ob.propertyTypeIDs, rules)
}

// ApplyVisibilityRules nulls the TextTemplate of any TextTemplate-typed property
// the rules mark as hidden under the object's current configuration, returning
// the updated object. It is the engine-agnostic form of ApplyPropertyVisibility
// so build paths that construct a widget object without a full Builder (e.g. the
// filter-widget build path) can apply the same #574 nulling. Non-TextTemplate
// properties are left untouched — only the populated-vs-null ClientTemplate
// choice triggers CE0463.
func ApplyVisibilityRules(object bson.D, propertyTypeIDs map[string]pages.PropertyTypeIDEntry, rules []types.WidgetVisibilityRule) bson.D {
	if len(rules) == 0 {
		return object
	}
	values := primitiveValuesOf(object, propertyTypeIDs)

	// Both directions, because null and empty are each invalid in the other's
	// state. #574 covered hidden→null; a VISIBLE TextTemplate left null is the
	// other half, and it is what made an authored ProgressCircle `showLabel:
	// true` fail CE0463: `labelText` is visible whenever `labelType` is "text"
	// (its default), and mxcli stored null where Mendix stores an empty
	// ClientTemplate. Confirmed by handing the failing project to Mendix's own
	// `mx update-widgets`, whose reconciliation writes exactly that template and
	// changes nothing else of substance (ledger #104 follow-on).
	//
	// Only properties the widget's schema declares CONDITIONAL are touched. A
	// TextTemplate with no rule is left exactly as it was: Studio Pro's
	// convention for an unset one is not uniform — a DataGrid custom-content
	// column stores null for `tooltip` and an empty ClientTemplate for
	// `exportValue` — so filling every unset template would trade this bug for
	// its mirror image. Those per-column conventions live in
	// emptyClientTemplateRules and are reached by a different path.
	hidden := make(map[string]bool, len(rules))
	conditional := make([]string, 0, len(rules))
	for _, rule := range rules {
		if rule.Nested() {
			// Item sub-property of an object list: evaluated per item below.
			continue
		}
		entry, ok := propertyTypeIDs[rule.PropertyKey]
		if !ok || entry.ValueType != "TextTemplate" {
			continue
		}
		if _, seen := hidden[rule.PropertyKey]; !seen {
			conditional = append(conditional, rule.PropertyKey)
			hidden[rule.PropertyKey] = false
		}
		// Several rules may govern one property; any one of them hiding it wins.
		// Within a rule, every term of its conjunction must hold — reading only
		// HiddenWhen would hide a template in configurations the editor shows.
		fires, determinable := rule.Fires(func(c types.WidgetVisibilityCondition) (string, bool) {
			v, ok := values[c.PropertyKey]
			return v, ok
		})
		if determinable && fires {
			hidden[rule.PropertyKey] = true
		}
	}
	// Sorted, because the object is serialized and map order is not stable.
	sort.Strings(conditional)

	for _, key := range conditional {
		isHidden := hidden[key]
		object = updateWidgetPropertyValue(object, propertyTypeIDs, key, func(val bson.D) bson.D {
			if isHidden {
				return setBSONField(val, "TextTemplate", nil)
			}
			// Never clobber real content — only fill in the absent template.
			if bsonFieldIsNil(val, "TextTemplate") {
				return setBSONField(val, "TextTemplate", BuildEmptyClientTemplate())
			}
			return val
		})
	}
	return applyNestedVisibilityRules(object, propertyTypeIDs, rules)
}

// applyNestedVisibilityRules nulls the TextTemplate of an object-list ITEM
// sub-property that a nested rule (ListPropertyKey set) hides under that
// item's own configuration.
//
// The File Uploader 2.5.0 `allowedFileFormats` item is the case that found it:
// `typeFormatDescription` is a REQUIRED TextTemplate hidden when
// `configMode = "simple"`. buildObjectListItemBSON fills unset required
// templates with the shipped default text (#891), which is right while the
// property is visible and CE0463 while it is hidden: Studio Pro stores null
// there, and `mx update-widgets` rewrites exactly that field and nothing else.
//
// Only the hidden direction is applied. A visible item template is already
// handled at item build time (#891 / emptyClientTemplateRules), and an
// indeterminable rule leaves the item untouched.
func applyNestedVisibilityRules(object bson.D, propertyTypeIDs map[string]pages.PropertyTypeIDEntry, rules []types.WidgetVisibilityRule) bson.D {
	byList := map[string][]types.WidgetVisibilityRule{}
	for _, rule := range rules {
		if rule.Nested() {
			byList[rule.ListPropertyKey] = append(byList[rule.ListPropertyKey], rule)
		}
	}
	if len(byList) == 0 {
		return object
	}
	listKeys := make([]string, 0, len(byList))
	for k := range byList {
		listKeys = append(listKeys, k)
	}
	sort.Strings(listKeys)
	for _, listKey := range listKeys {
		entry, ok := propertyTypeIDs[listKey]
		if !ok || len(entry.NestedPropertyIDs) == 0 {
			continue
		}
		listRules := byList[listKey]
		object = updateWidgetPropertyValue(object, propertyTypeIDs, listKey, func(val bson.D) bson.D {
			for i, elem := range val {
				if elem.Key != "Objects" {
					continue
				}
				arr, ok := elem.Value.(bson.A)
				if !ok {
					continue
				}
				out := make(bson.A, len(arr))
				for j, it := range arr {
					item, ok := it.(bson.D)
					if !ok {
						out[j] = it
						continue
					}
					values := primitiveValuesOf(item, entry.NestedPropertyIDs)
					for _, rule := range listRules {
						sub, ok := entry.NestedPropertyIDs[rule.PropertyKey]
						if !ok || sub.ValueType != "TextTemplate" {
							continue
						}
						fires, determinable := rule.Fires(func(c types.WidgetVisibilityCondition) (string, bool) {
							v, ok := values[c.PropertyKey]
							return v, ok
						})
						if determinable && fires {
							item = updateWidgetPropertyValue(item, entry.NestedPropertyIDs, rule.PropertyKey, func(v bson.D) bson.D {
								return setBSONField(v, "TextTemplate", nil)
							})
						}
					}
					out[j] = item
				}
				val[i] = bson.E{Key: "Objects", Value: out}
			}
			return val
		})
	}
	return object
}

// bsonFieldIsNil reports whether a field is absent or explicitly nil.
func bsonFieldIsNil(val bson.D, field string) bool {
	for _, elem := range val {
		if elem.Key == field {
			return elem.Value == nil
		}
	}
	return true
}

// primitiveValuesOf maps each known property key to its current comparable value
// string in the object (e.g. type → "expression", itemSelection → "Single").
// Properties absent from the object map to "".
func primitiveValuesOf(object bson.D, propertyTypeIDs map[string]pages.PropertyTypeIDEntry) map[string]string {
	rawByID := make(map[string]bson.D)
	for _, elem := range object {
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
			if val := widgetValueOfProperty(prop); val != nil {
				rawByID[id] = val
			}
		}
	}

	out := make(map[string]string, len(propertyTypeIDs))
	for key, entry := range propertyTypeIDs {
		if val, ok := rawByID[strings.ReplaceAll(entry.PropertyTypeID, "-", "")]; ok {
			out[key] = effectiveValue(val, entry.ValueType)
		}
	}
	return out
}

// effectiveValue reads the field of a WidgetValue that holds a property's
// primitive-comparable value, keyed by its ValueType: a Selection-typed property
// (e.g. DataGrid2 itemSelection = None/Single/Multi) stores its value in the
// `Selection` field, everything else in `PrimitiveValue`. editorConfig
// visibility conditions compare against this value, so reading the wrong field
// (always PrimitiveValue) made selection-conditioned rules evaluate against ""
// and mis-fire (issue #574: singleSelectionColumnLabel nulled under Selection:Single).
func effectiveValue(val bson.D, valueType string) string {
	field := "PrimitiveValue"
	if valueType == "Selection" {
		field = "Selection"
	}
	for _, ve := range val {
		if ve.Key == field {
			if s, ok := ve.Value.(string); ok {
				return s
			}
			return ""
		}
	}
	return ""
}

// widgetValueOfProperty returns a WidgetProperty's Value document, or nil.
func widgetValueOfProperty(prop bson.D) bson.D {
	for _, elem := range prop {
		if elem.Key == "Value" {
			if val, ok := elem.Value.(bson.D); ok {
				return val
			}
			return nil
		}
	}
	return nil
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

// setImageValue writes the image reference onto a WidgetValue's `Image` key.
//
// The key is REPLACED, never added: a value node that does not declare one
// belongs to a property that is not image-typed, and a key the widget's
// definition does not know about is the CE0463 shape. Measured on a Studio
// Pro-authored widget, the stored form is the bare qualified name —
// `MyFirstModule.Images._1` — with nothing else on the node changed.
func setImageValue(val bson.D, imageQN string) bson.D {
	if imageQN == "" {
		return val
	}
	result := make(bson.D, 0, len(val))
	for _, elem := range val {
		if elem.Key == "Image" {
			result = append(result, bson.E{Key: "Image", Value: imageQN})
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
				// The slot is nil, which is the NORMAL state of a conditional
				// template: #574 stores one as null while its condition hides it,
				// so a script that turns the condition on and writes the text in
				// the same statement finds nothing to edit. Keeping the nil here
				// dropped the text in silence — and left a widget whose own
				// enumeration says it has custom text with no text, which is a
				// build error (issue #254, Slider `tooltipType: customText`).
				//
				// Build the envelope, then write into it. Not a special case for
				// the visible/hidden question: ApplyVisibilityRules still runs
				// afterwards and nulls this again if the property turns out to be
				// hidden, so an authored text can never survive into a pruned slot.
				result = append(result, bson.E{
					Key:   "TextTemplate",
					Value: updateTemplateText(BuildEmptyClientTemplate(), text),
				})
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
								{Key: "LanguageCode", Value: model.AuthoringLanguage()},
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
				{Key: "LanguageCode", Value: model.AuthoringLanguage()},
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
				{Key: "LanguageCode", Value: model.AuthoringLanguage()},
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
		// Leave an unset String property empty (its schema default, "") — do NOT
		// manufacture a single space. Studio Pro stores "" here, and some widgets
		// (chart series `customSeriesOptions`, `customLayout`, `customConfigurations`)
		// feed the value straight to a client-side JSON.parse whose empty-guard is
		// `value !== "" ? value : "{}"`; a lone space passes that guard and makes
		// the widget throw `JSON.parse(" ")` → "Unexpected end of JSON input" at
		// render time (object-list-item path only; the top-level path already
		// leaves it empty).
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
					{Key: "LanguageCode", Value: model.AuthoringLanguage()},
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

	// Use the parameter's per-parameter formatting when present; a nil
	// FormattingInfo reproduces the previous hardcoded defaults, so every
	// unformatted column parameter is byte-identical to before. Mirrors
	// sdk/mpr/writer_widgets.go:serializeClientTemplateParameter so a
	// `format (...)` block authored on a DataGrid2 dynamic-text column param
	// reaches the runtime instead of being silently dropped (ledger #77).
	dateFormat, customDateFormat, enumFormat := "Date", "", "Text"
	decimalPrecision := int64(2)
	groupDigits := false
	if fi := param.FormattingInfo; fi != nil {
		if fi.DateFormat != "" {
			dateFormat = fi.DateFormat
		}
		customDateFormat = fi.CustomDateFormat
		if fi.EnumFormat != "" {
			enumFormat = fi.EnumFormat
		}
		decimalPrecision = int64(fi.DecimalPrecision)
		groupDigits = fi.GroupDigits
	}
	formattingInfo := bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Forms$FormattingInfo"},
		{Key: "CustomDateFormat", Value: customDateFormat},
		{Key: "DateFormat", Value: dateFormat},
		{Key: "DecimalPrecision", Value: decimalPrecision},
		{Key: "EnumFormat", Value: enumFormat},
		{Key: "GroupDigits", Value: groupDigits},
	}

	var sourceVariable any
	if param.SourceVariable != "" || param.SourceWidget != "" {
		sourceVariable = PageVariableBSON(param.SourceWidget, param.SourceVariable, param.SourceVariableKind)
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

// PageVariableBSON is a Forms$PageVariable. Studio Pro distinguishes three
// bindings by the slot the name fills:
//   - LocalVariable     → page-level Variables entry (kind "local")
//   - SnippetParameter  → snippet parameter            (kind "snippet")
//   - PageParameter     → page parameter               (kind "", the default)
//
// widget names a data view the binding reads through — `$dataView1.Attr`, the
// data view in Widget and its own variable in the slot (ako/mxcli#826). A
// binding read straight from a parameter has none.
func PageVariableBSON(widget, name, kind string) bson.D {
	local, page, snippet := "", "", ""
	switch kind {
	case "local":
		local = name
	case "snippet":
		snippet = name
	default:
		page = name
	}
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Forms$PageVariable"},
		{Key: "LocalVariable", Value: local},
		{Key: "PageParameter", Value: page},
		{Key: "SnippetParameter", Value: snippet},
		{Key: "SubKey", Value: ""},
		{Key: "UseAllPages", Value: false},
		{Key: "Widget", Value: widget},
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
