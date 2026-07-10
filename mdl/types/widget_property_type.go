// SPDX-License-Identifier: Apache-2.0

package types

// PropertyTypeIDEntry holds the IDs for a property type from a cloned pluggable widget template.
// This is an engine-internal struct used by WidgetObjectBuilder; it is not a BSON wire type.
type PropertyTypeIDEntry struct {
	PropertyTypeID     string
	ValueTypeID        string
	DefaultValue       string // Default value from the template's ValueType
	ValueType          string // Type of value (Boolean, Integer, String, DataSource, etc.)
	Required           bool   // Whether this property is required
	DataSourceProperty string // Non-empty when this attribute is linked to another DataSource property
	// For object list properties (IsList=true with ObjectType), these hold nested IDs
	ObjectTypeID      string                         // ID of the nested ObjectType (for object lists like columns)
	NestedPropertyIDs map[string]PropertyTypeIDEntry // Property IDs within the nested ObjectType
	// NestedKeyOrder lists NestedPropertyIDs keys in template PropertyTypes order.
	// Studio Pro requires a WidgetObject's Properties to mirror the WidgetType's
	// PropertyTypes order or it raises CE0463 ("widget definition changed"); the
	// object-list item builder uses this instead of alphabetical map iteration.
	NestedKeyOrder []string
}
