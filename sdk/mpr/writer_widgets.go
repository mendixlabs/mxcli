// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"strings"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"

	"go.mongodb.org/mongo-driver/bson"
)

// ============================================================================
// Widget Serialization — Dispatch
// ============================================================================

// serializeWidgetArray serializes a slice of widgets to a BSON array with version prefix.
// Mendix uses [3] for empty arrays, [2, item1, item2, ...] for non-empty arrays.
// Items go directly after the version marker, NOT nested in another array.
func serializeWidgetArray(widgets []pages.Widget) bson.A {
	arr := bson.A{int32(3)} // Start with empty marker
	hasItems := false
	for _, w := range widgets {
		if w != nil {
			if !hasItems {
				arr = bson.A{int32(2)} // First item: change to version 2
				hasItems = true
			}
			arr = append(arr, serializeWidget(w))
		}
	}
	return arr
}

// SerializeWidget serializes a single widget to BSON.
// This is the public entry point for widget serialization.
func SerializeWidget(w pages.Widget) bson.D {
	return serializeWidget(w)
}

// serializeWidget serializes a single widget to BSON.
func serializeWidget(w pages.Widget) bson.D {
	var doc bson.D
	switch widget := w.(type) {
	case *pages.Container:
		doc = serializeContainer(widget)
	case *pages.GroupBox:
		return serializeGroupBox(widget)
	case *pages.TabContainer:
		return serializeTabContainer(widget)
	case *pages.LayoutGrid:
		doc = serializeLayoutGrid(widget)
	case *pages.DynamicText:
		doc = serializeDynamicText(widget)
	case *pages.ActionButton:
		doc = serializeActionButton(widget)
	case *pages.Text:
		doc = serializeStaticText(widget)
	case *pages.Title:
		doc = serializeTitle(widget)
	case *pages.SnippetCallWidget:
		doc = serializeSnippetCall(widget)
	case *pages.Gallery:
		doc = serializeGallery(widget)
	case *pages.CustomWidget:
		doc = serializeCustomWidget(widget)
	case *pages.DataView:
		doc = serializeDataView(widget)
	case *pages.DataGrid:
		doc = serializeDataGrid(widget)
	case *pages.TextBox:
		doc = serializeTextBox(widget)
	case *pages.TextArea:
		doc = serializeTextArea(widget)
	case *pages.DatePicker:
		doc = serializeDatePicker(widget)
	case *pages.CheckBox:
		doc = serializeCheckBox(widget)
	case *pages.RadioButtons:
		doc = serializeRadioButtons(widget)
	case *pages.DropDown:
		doc = serializeDropDown(widget)
	case *pages.NavigationList:
		doc = serializeNavigationList(widget)
	case *pages.ListView:
		doc = serializeListView(widget)
	case *pages.StaticImage:
		doc = serializeStaticImage(widget)
	case *pages.DynamicImage:
		doc = serializeDynamicImage(widget)
	default:
		// Fallback for unknown widget types
		doc = bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(w.GetID()))},
			{Key: "$Type", Value: w.GetTypeName()},
			{Key: "Name", Value: w.GetName()},
		}
	}

	// Patch conditional settings from BaseWidget if set
	doc = patchConditionalSettings(doc, w)
	return doc
}

// patchConditionalSettings replaces nil ConditionalVisibilitySettings/ConditionalEditabilitySettings
// in the serialized BSON with actual values from the widget's BaseWidget fields.
func patchConditionalSettings(doc bson.D, w pages.Widget) bson.D {
	type baseWidgetGetter interface {
		GetBaseWidget() *pages.BaseWidget
	}
	bwg, ok := w.(baseWidgetGetter)
	if !ok {
		return doc
	}
	bw := bwg.GetBaseWidget()
	if bw.ConditionalVisibility == nil && bw.ConditionalEditability == nil {
		return doc
	}

	for i, elem := range doc {
		if elem.Key == "ConditionalVisibilitySettings" && bw.ConditionalVisibility != nil {
			doc[i].Value = serializeConditionalVisibility(bw.ConditionalVisibility)
		}
		if elem.Key == "ConditionalEditabilitySettings" && bw.ConditionalEditability != nil {
			doc[i].Value = serializeConditionalEditability(bw.ConditionalEditability)
		}
		if elem.Key == "Editable" && bw.ConditionalEditability != nil {
			doc[i].Value = "Conditional"
		}
	}
	return doc
}

func serializeConditionalVisibility(cvs *pages.ConditionalVisibilitySettings) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(cvs.ID))},
		{Key: "$Type", Value: "Forms$ConditionalVisibilitySettings"},
		// Attribute is a BY_NAME AttributeIdentifier; Studio Pro writes "" (not null)
		// when there is no attribute-based condition. 11.12's reader rejects the null.
		{Key: "Attribute", Value: ""},
		{Key: "Conditions", Value: bson.A{int32(3)}},
		{Key: "Expression", Value: cvs.Expression},
		{Key: "IgnoreSecurity", Value: false},
		{Key: "ModuleRoles", Value: bson.A{int32(3)}},
		{Key: "SourceVariable", Value: nil},
	}
}

func serializeConditionalEditability(ces *pages.ConditionalEditabilitySettings) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(ces.ID))},
		{Key: "$Type", Value: "Forms$ConditionalEditabilitySettings"},
		{Key: "Attribute", Value: ""}, // "" not null — see serializeConditionalVisibility
		{Key: "Conditions", Value: bson.A{int32(3)}},
		{Key: "Expression", Value: ces.Expression},
		{Key: "SourceVariable", Value: nil},
	}
}

// ============================================================================
// DataSource Serialization
// ============================================================================

// serializeDataSource serializes a datasource for DataView widgets (Forms$*Source types).
// NOTE: DataViews do not support database sources in Mendix. If a DatabaseSource is passed,
// it is serialized as a Forms$DataViewSource with entity reference as a best-effort fallback.
func serializeDataSource(ds pages.DataSource) bson.D {
	if ds == nil {
		return nil
	}

	switch d := ds.(type) {
	case *pages.DatabaseSource:
		// DataViews cannot have a database source in Mendix. Serialize as
		// Forms$DataViewSource with entity ref as the closest valid alternative.
		var entityRef any
		if d.EntityName != "" {
			entityRef = bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "DomainModels$DirectEntityRef"},
				{Key: "Entity", Value: d.EntityName},
			}
		}
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(d.ID))},
			{Key: "$Type", Value: "Forms$DataViewSource"},
			{Key: "EntityRef", Value: entityRef},
			{Key: "ForceFullObjects", Value: false},
			{Key: "SourceVariable", Value: nil},
		}
	case *pages.MicroflowSource:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(d.ID))},
			{Key: "$Type", Value: "Forms$MicroflowSource"},
			{Key: "MicroflowSettings", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Forms$MicroflowSettings"},
				{Key: "Asynchronous", Value: false},
				{Key: "ConfirmationInfo", Value: nil},
				{Key: "FormValidations", Value: "All"},
				{Key: "Microflow", Value: d.Microflow}, // Qualified name (e.g., "Module.MicroflowName")
				{Key: "ParameterMappings", Value: bson.A{int32(3)}},
				{Key: "ProgressBar", Value: "None"},
				{Key: "ProgressMessage", Value: nil},
			}},
		}
	case *pages.NanoflowSource:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(d.ID))},
			{Key: "$Type", Value: "Forms$NanoflowSource"},
			{Key: "NanoflowSettings", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Forms$NanoflowSettings"},
				{Key: "Nanoflow", Value: d.Nanoflow}, // Qualified name (e.g., "Module.NanoflowName")
				{Key: "ParameterMappings", Value: bson.A{int32(3)}},
			}},
		}
	default:
		return nil
	}
}

// SerializeCustomWidgetDataSource serializes a datasource for custom widgets.
// Exported for use by page builders.
func SerializeCustomWidgetDataSource(ds pages.DataSource) bson.D {
	if ds == nil {
		return nil
	}

	switch d := ds.(type) {
	case *pages.DatabaseSource:
		// EntityRef needs to be serialized with the entity qualified name
		var entityRef any
		if d.EntityName != "" {
			entityRef = bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "DomainModels$DirectEntityRef"},
				{Key: "Entity", Value: d.EntityName},
			}
		}

		// Build SortItems array from Sorting field
		sortItems := bson.A{int32(2)} // Version marker for non-empty array
		for _, sort := range d.Sorting {
			sortItem := bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Forms$GridSortItem"},
				{Key: "AttributeRef", Value: bson.D{
					{Key: "$ID", Value: idToBsonBinary(generateUUID())},
					{Key: "$Type", Value: "DomainModels$AttributeRef"},
					{Key: "Attribute", Value: sort.AttributePath},
					{Key: "EntityRef", Value: nil},
				}},
				// Forms$GridSortItem stores its direction under SortDirection, NOT
				// SortOrder (that key belongs to Microflows$SortItem / document
				// templates). Studio Pro ignores the misnamed field → sort silently
				// reverts to ascending. Bug 8.
				{Key: "SortDirection", Value: string(sort.Direction)},
			}
			sortItems = append(sortItems, sortItem)
		}

		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(d.ID))},
			{Key: "$Type", Value: "CustomWidgets$CustomWidgetXPathSource"},
			{Key: "EntityRef", Value: entityRef},
			{Key: "ForceFullObjects", Value: false},
			{Key: "SortBar", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Forms$GridSortBar"},
				{Key: "SortItems", Value: sortItems},
			}},
			{Key: "SourceVariable", Value: nil},
			{Key: "XPathConstraint", Value: d.XPathConstraint},
		}
	case *pages.MicroflowSource:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(d.ID))},
			{Key: "$Type", Value: "Forms$MicroflowSource"},
			{Key: "MicroflowSettings", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Forms$MicroflowSettings"},
				{Key: "Asynchronous", Value: false},
				{Key: "ConfirmationInfo", Value: nil},
				{Key: "FormValidations", Value: "All"},
				{Key: "Microflow", Value: d.Microflow},
				{Key: "ParameterMappings", Value: bson.A{int32(3)}},
				{Key: "ProgressBar", Value: "None"},
				{Key: "ProgressMessage", Value: nil},
			}},
		}
	case *pages.NanoflowSource:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(d.ID))},
			{Key: "$Type", Value: "Forms$NanoflowSource"},
			{Key: "NanoflowSettings", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Forms$NanoflowSettings"},
				{Key: "Nanoflow", Value: d.Nanoflow},
				{Key: "ParameterMappings", Value: bson.A{int32(3)}},
			}},
		}
	case *pages.AssociationSource:
		return serializeAssociationSource(d)
	default:
		return nil
	}
}

// serializeAssociationSource builds a Forms$AssociationSource BSON document.
// EntityPath is "Module.Assoc" or "Module.Assoc/Module.DestEntity".
// When DestinationEntity is omitted, it's left empty — Studio Pro will resolve it.
func serializeAssociationSource(d *pages.AssociationSource) bson.D {
	parts := strings.Split(d.EntityPath, "/")
	association := parts[0]
	destEntity := ""
	if len(parts) >= 2 {
		destEntity = parts[1]
	}

	step := bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "DomainModels$EntityRefStep"},
		{Key: "Association", Value: association},
		{Key: "DestinationEntity", Value: destEntity},
	}

	entityRef := bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "DomainModels$IndirectEntityRef"},
		{Key: "Steps", Value: bson.A{int32(2), step}},
	}

	var sourceVar any
	if d.ContextVariable != "" {
		sourceVar = bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Forms$PageVariable"},
			{Key: "LocalVariable", Value: ""},
			{Key: "PageParameter", Value: d.ContextVariable},
			{Key: "SnippetParameter", Value: ""},
			{Key: "SubKey", Value: ""},
			{Key: "UseAllPages", Value: false},
			{Key: "Widget", Value: ""},
		}
	}

	id := string(d.ID)
	if id == "" {
		id = generateUUID()
	}
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(id)},
		{Key: "$Type", Value: "Forms$AssociationSource"},
		{Key: "EntityRef", Value: entityRef},
		{Key: "ForceFullObjects", Value: false},
		{Key: "SourceVariable", Value: sourceVar},
	}
}

// ============================================================================
// Reference Serialization
// ============================================================================

// serializeAttributeRef serializes an attribute reference for input widgets.
// The attrPath MUST be a fully qualified name (Module.Entity.Attribute) with at least 2 dots.
// If the path is not fully qualified, returns nil to avoid Mendix resolution errors.
func serializeAttributeRef(attrPath string) any {
	if attrPath == "" {
		return nil
	}
	// Attribute path must be fully qualified: Module.Entity.Attribute (at least 2 dots)
	dotCount := strings.Count(attrPath, ".")
	if dotCount < 2 {
		// Not fully qualified - cannot serialize as Mendix won't be able to resolve it
		return nil
	}
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "DomainModels$AttributeRef"},
		{Key: "Attribute", Value: attrPath},
		{Key: "EntityRef", Value: nil},
	}
}

// serializeEntityRef serializes an entity reference.
func serializeEntityRef(entityPath string) any {
	if entityPath == "" {
		return nil
	}
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "DomainModels$DirectEntityRef"},
		{Key: "Entity", Value: entityPath},
	}
}

// ============================================================================
// Appearance Serialization
// ============================================================================

// serializeAppearance creates a standard Appearance object for widgets.
func serializeAppearance(class, style, dynamicClasses string, designProps []pages.DesignPropertyValue) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Forms$Appearance"},
		{Key: "Class", Value: class},
		{Key: "DesignProperties", Value: serializeDesignProperties(designProps)},
		{Key: "DynamicClasses", Value: dynamicClasses},
		{Key: "Style", Value: style},
	}
}

// serializeDesignProperties serializes design property values to a BSON array.
// Both empty and non-empty use version marker int64(3).
func serializeDesignProperties(props []pages.DesignPropertyValue) bson.A {
	if len(props) == 0 {
		return bson.A{int32(3)}
	}

	arr := bson.A{int32(3)}
	for _, p := range props {
		var valueBson bson.D
		switch p.ValueType {
		case "toggle":
			valueBson = bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Forms$ToggleDesignPropertyValue"},
			}
		case "option":
			valueBson = bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Forms$OptionDesignPropertyValue"},
				{Key: "Option", Value: p.Option},
			}
		case "custom":
			// ToggleButtonGroup and ColorPicker properties use CustomDesignPropertyValue
			valueBson = bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Forms$CustomDesignPropertyValue"},
				{Key: "Value", Value: p.Option},
			}
		default:
			continue
		}
		arr = append(arr, bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Forms$DesignPropertyValue"},
			{Key: "Key", Value: p.Key},
			{Key: "Value", Value: valueBson},
		})
	}
	return arr
}

// ============================================================================
// Input Widget Helpers
// ============================================================================

// serializeWidgetValidation creates the required WidgetValidation object for input widgets.
func serializeWidgetValidation() bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Forms$WidgetValidation"},
		{Key: "Expression", Value: ""},
		{Key: "Message", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3)}},
		}},
	}
}

// serializeFormattingInfo creates a default FormattingInfo object for input widgets.
func serializeFormattingInfo() bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Forms$FormattingInfo"},
		{Key: "CustomDateFormat", Value: ""},
		{Key: "DateFormat", Value: "Date"},
		{Key: "DecimalPrecision", Value: int64(2)},
		{Key: "EnumFormat", Value: "Text"},
		{Key: "GroupDigits", Value: false},
	}
}

// ============================================================================
// Text/Template Helpers
// ============================================================================

// serializeEmptyText creates an empty Texts$Text object.
// Required for properties like CounterMessage that cannot be null.
func serializeEmptyText() bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Texts$Text"},
		{Key: "Items", Value: bson.A{int32(3)}},
	}
}

// serializeEmptyPlaceholderTemplate creates an empty ClientTemplate for placeholder text.
func serializeEmptyPlaceholderTemplate() bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Forms$ClientTemplate"},
		{Key: "Fallback", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3)}},
		}},
		{Key: "Parameters", Value: bson.A{int32(3)}},
		{Key: "Template", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3)}},
		}},
	}
}

// serializeLabelTemplate creates a standard label template for input widgets.
func serializeLabelTemplate(label string) bson.D {
	if label == "" {
		return nil
	}
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Forms$ClientTemplate"},
		{Key: "Fallback", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3)}},
		}},
		{Key: "Parameters", Value: bson.A{int32(3)}},
		{Key: "Template", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3), bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Texts$Translation"},
				{Key: "LanguageCode", Value: "en_US"},
				{Key: "Text", Value: label},
			}}},
		}},
	}
}

// serializeClientTemplate serializes a ClientTemplate with parameters.
func serializeClientTemplate(ct *pages.ClientTemplate, fallbackText *model.Text, defaultText string) bson.D {
	captionID := generateUUID()
	captionTransID := generateUUID()
	captionText := defaultText

	// Get text from ClientTemplate or fallback Text
	if ct != nil && ct.Template != nil {
		for _, text := range ct.Template.Translations {
			captionText = text
			break
		}
	} else if fallbackText != nil {
		for _, text := range fallbackText.Translations {
			captionText = text
			break
		}
	}

	// Build the template document
	// Mendix uses [3] as version marker, followed by array items
	template := bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Texts$Text"},
		{Key: "Items", Value: bson.A{int32(3), bson.D{
			{Key: "$ID", Value: idToBsonBinary(captionTransID)},
			{Key: "$Type", Value: "Texts$Translation"},
			{Key: "LanguageCode", Value: "en_US"},
			{Key: "Text", Value: captionText},
		}}},
	}

	// Build Fallback as a Texts$Text object (not a string)
	fallback := bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Texts$Text"},
		{Key: "Items", Value: bson.A{int32(3)}}, // Empty fallback
	}

	// Build parameters array - use [3] for empty, [2, items...] for non-empty
	params := bson.A{int32(3)} // Empty array with version marker 3
	if ct != nil && len(ct.Parameters) > 0 {
		params = bson.A{int32(2)} // Non-empty array uses version marker 2
		for _, param := range ct.Parameters {
			params = append(params, serializeClientTemplateParameter(param))
		}
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(captionID)},
		{Key: "$Type", Value: "Forms$ClientTemplate"},
		{Key: "Fallback", Value: fallback}, // Must be Fallback object, not FallbackValue string
		{Key: "Parameters", Value: params},
		{Key: "Template", Value: template},
	}
}

// serializeClientTemplateParameter serializes a ClientTemplateParameter.
func serializeClientTemplateParameter(param *pages.ClientTemplateParameter) bson.D {
	paramID := generateUUID()
	if param.ID != "" {
		paramID = string(param.ID)
	}

	// Build AttributeRef if present - use serializeAttributeRef for validation
	attrRef := serializeAttributeRef(param.AttributeRef)

	// Build FormattingInfo — schema-aligned with Forms$FormattingInfo
	// reflection (CustomDateFormat / DateFormat / DecimalPrecision /
	// EnumFormat / GroupDigits). Writing TimeFormat here triggers Studio
	// Pro CE0463 "widget definition changed" on pluggable widgets that
	// embed this struct (e.g. Gallery / DataGrid2 column captions).
	formattingInfo := bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Forms$FormattingInfo"},
		{Key: "CustomDateFormat", Value: ""},
		{Key: "DateFormat", Value: "Date"},
		{Key: "DecimalPrecision", Value: int64(2)},
		{Key: "EnumFormat", Value: "Text"},
		{Key: "GroupDigits", Value: false},
	}

	// Build SourceVariable if present (references a page/snippet parameter)
	var sourceVariable any
	if param.SourceVariable != "" {
		sourceVariable = bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Forms$PageVariable"},
			{Key: "PageParameter", Value: param.SourceVariable},
			{Key: "UseAllPages", Value: false},
			{Key: "Widget", Value: ""},
		}
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(paramID)},
		{Key: "$Type", Value: "Forms$ClientTemplateParameter"},
		{Key: "AttributeRef", Value: attrRef},
		{Key: "Expression", Value: param.Expression},
		{Key: "FormattingInfo", Value: formattingInfo},
		{Key: "SourceVariable", Value: sourceVariable},
	}
}
