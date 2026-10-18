// SPDX-License-Identifier: Apache-2.0

package types

// PropertyTranslation is one widget-shipped translation of a property's default
// text, read from the template ValueType's Translations array.
type PropertyTranslation struct {
	LanguageCode string
	Text         string
}

// PropertyTypeIDEntry holds the IDs for a property type from a cloned pluggable widget template.
// This is an engine-internal struct used by WidgetObjectBuilder; it is not a BSON wire type.
type PropertyTypeIDEntry struct {
	PropertyTypeID string
	ValueTypeID    string
	DefaultValue   string // Default value from the template's ValueType
	ValueType      string // Type of value (Boolean, Integer, String, DataSource, etc.)
	Required       bool   // Whether this property is required
	// SelectionTypes lists the selection kinds a Selection-typed property offers,
	// as the widget declares them (<selectionTypes> in widget.xml, the template's
	// ValueType.SelectionTypes): None, Single and/or Multi. Empty for every other
	// type, and where the template carries none — a caller then falls back to the
	// full set rather than guessing a narrower one.
	SelectionTypes []string
	// DefaultTranslations are the widget-shipped <translations> for this property.
	// A REQUIRED TextTemplate the author leaves unset must be serialized WITH this
	// text: a null there is CE0463 "the definition of this widget has changed",
	// and an empty Forms$ClientTemplate is CE4899 "Property … is required" (#891).
	// This is what `mx update-widgets` itself writes.
	DefaultTranslations []PropertyTranslation
	DataSourceProperty  string // Non-empty when this attribute is linked to another DataSource property
	// IsLinked marks a datasource the PLATFORM wires from the containing widget
	// rather than one the developer sets: a Data Grid 2 column filter's
	// `linkedDs` is filled from the grid it sits in. Studio Pro stores it empty
	// on every such widget, and a value written there does not satisfy the
	// property — mxbuild still reports CE0642 for it. So it is not authorable,
	// and a .def.json mapping one is refused at build time rather than written.
	IsLinked bool
	// For object list properties (IsList=true with ObjectType), these hold nested IDs
	ObjectTypeID      string                         // ID of the nested ObjectType (for object lists like columns)
	NestedPropertyIDs map[string]PropertyTypeIDEntry // Property IDs within the nested ObjectType
	// NestedKeyOrder lists NestedPropertyIDs keys in template PropertyTypes order.
	// Studio Pro requires a WidgetObject's Properties to mirror the WidgetType's
	// PropertyTypes order or it raises CE0463 ("widget definition changed"); the
	// object-list item builder uses this instead of alphabetical map iteration.
	NestedKeyOrder []string
}
