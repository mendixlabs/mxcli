// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"log"

	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/bson"
)

// PropertyMapping maps an MDL source (attribute, association, literal, etc.)
// to a pluggable widget property key via a named operation.
type PropertyMapping struct {
	PropertyKey string `json:"propertyKey"`
	Source      string `json:"source,omitempty"`
	Value       string `json:"value,omitempty"`
	Operation   string `json:"operation"`
	Default     string `json:"default,omitempty"`
}

// OperationFunc updates a template object's property identified by propertyKey.
// It receives the current object BSON, the property type ID map, the target key,
// and the build context containing resolved values.
type OperationFunc func(obj bson.D, propTypeIDs map[string]pages.PropertyTypeIDEntry, propertyKey string, ctx *BuildContext) bson.D

// OperationRegistry maps operation names to their implementations.
type OperationRegistry struct {
	operations map[string]OperationFunc
}

// NewOperationRegistry creates a registry pre-loaded with the built-in operations.
func NewOperationRegistry() *OperationRegistry {
	reg := &OperationRegistry{
		operations: make(map[string]OperationFunc),
	}
	reg.Register("attribute", opAttribute)
	reg.Register("association", opAssociation)
	reg.Register("primitive", opPrimitive)
	reg.Register("selection", opSelection)
	reg.Register("datasource", opDatasource)
	reg.Register("widgets", opWidgets)
	reg.Register("texttemplate", opTextTemplate)
	reg.Register("action", opAction)
	reg.Register("attributeObjects", opAttributeObjects)
	return reg
}

// Register adds or replaces an operation by name.
func (r *OperationRegistry) Register(name string, fn OperationFunc) {
	r.operations[name] = fn
}

// Lookup returns the operation function for the given name, or nil if not found.
func (r *OperationRegistry) Lookup(name string) OperationFunc {
	return r.operations[name]
}

// Has returns true if the named operation is registered.
func (r *OperationRegistry) Has(name string) bool {
	_, ok := r.operations[name]
	return ok
}

// =============================================================================
// Built-in Operations
// =============================================================================

// opAttribute sets an attribute reference on a widget property.
func opAttribute(obj bson.D, propTypeIDs map[string]pages.PropertyTypeIDEntry, propertyKey string, ctx *BuildContext) bson.D {
	if ctx.AttributePath == "" {
		return obj
	}
	return updateWidgetPropertyValue(obj, propTypeIDs, propertyKey, func(val bson.D) bson.D {
		return setAttributeRef(val, ctx.AttributePath)
	})
}

// opAssociation sets an association reference (AttributeRef + EntityRef) on a widget property.
func opAssociation(obj bson.D, propTypeIDs map[string]pages.PropertyTypeIDEntry, propertyKey string, ctx *BuildContext) bson.D {
	if ctx.AssocPath == "" {
		return obj
	}
	return updateWidgetPropertyValue(obj, propTypeIDs, propertyKey, func(val bson.D) bson.D {
		return setAssociationRef(val, ctx.AssocPath, ctx.EntityName)
	})
}

// opPrimitive sets a primitive string value on a widget property.
func opPrimitive(obj bson.D, propTypeIDs map[string]pages.PropertyTypeIDEntry, propertyKey string, ctx *BuildContext) bson.D {
	if ctx.PrimitiveVal == "" {
		return obj
	}
	return updateWidgetPropertyValue(obj, propTypeIDs, propertyKey, func(val bson.D) bson.D {
		return setPrimitiveValue(val, ctx.PrimitiveVal)
	})
}

// opSelection sets a selection mode on a widget property, updating the Selection field
// inside the WidgetValue (which requires a deeper update than opPrimitive's PrimitiveValue).
func opSelection(obj bson.D, propTypeIDs map[string]pages.PropertyTypeIDEntry, propertyKey string, ctx *BuildContext) bson.D {
	if ctx.PrimitiveVal == "" {
		return obj
	}
	return updateWidgetPropertyValue(obj, propTypeIDs, propertyKey, func(val bson.D) bson.D {
		result := make(bson.D, 0, len(val))
		for _, elem := range val {
			if elem.Key == "Selection" {
				result = append(result, bson.E{Key: "Selection", Value: ctx.PrimitiveVal})
			} else {
				result = append(result, elem)
			}
		}
		return result
	})
}

// opExpression sets an expression string on a widget property.
func opExpression(obj bson.D, propTypeIDs map[string]pages.PropertyTypeIDEntry, propertyKey string, ctx *BuildContext) bson.D {
	if ctx.PrimitiveVal == "" {
		return obj
	}
	return updateWidgetPropertyValue(obj, propTypeIDs, propertyKey, func(val bson.D) bson.D {
		result := make(bson.D, 0, len(val))
		for _, elem := range val {
			if elem.Key == "Expression" {
				result = append(result, bson.E{Key: "Expression", Value: ctx.PrimitiveVal})
			} else {
				result = append(result, elem)
			}
		}
		return result
	})
}

// opDatasource sets a data source on a widget property.
func opDatasource(obj bson.D, propTypeIDs map[string]pages.PropertyTypeIDEntry, propertyKey string, ctx *BuildContext) bson.D {
	if ctx.DataSource == nil {
		return obj
	}
	return updateWidgetPropertyValue(obj, propTypeIDs, propertyKey, func(val bson.D) bson.D {
		return setDataSource(val, ctx.DataSource)
	})
}

// opWidgets replaces the Widgets array in a widget property value with child widgets.
func opWidgets(obj bson.D, propTypeIDs map[string]pages.PropertyTypeIDEntry, propertyKey string, ctx *BuildContext) bson.D {
	if len(ctx.ChildWidgets) == 0 {
		return obj
	}
	result := updateWidgetPropertyValue(obj, propTypeIDs, propertyKey, func(val bson.D) bson.D {
		return setChildWidgets(val, ctx.ChildWidgets)
	})
	return result
}

// setChildWidgets replaces the Widgets field in a WidgetValue with the given child widgets.
func setChildWidgets(val bson.D, childWidgets []bson.D) bson.D {
	widgetsArr := bson.A{int32(2)} // version marker
	for _, w := range childWidgets {
		widgetsArr = append(widgetsArr, w)
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

// opTextTemplate sets a text template value on a widget property.
// It replaces the Template.Items in the TextTemplate with a single text item.
func opTextTemplate(obj bson.D, propTypeIDs map[string]pages.PropertyTypeIDEntry, propertyKey string, ctx *BuildContext) bson.D {
	if ctx.PrimitiveVal == "" {
		return obj
	}
	return updateWidgetPropertyValue(obj, propTypeIDs, propertyKey, func(val bson.D) bson.D {
		return setTextTemplateValue(val, ctx.PrimitiveVal)
	})
}

// setTextTemplateValue sets the text content in a TextTemplate WidgetValue field.
func setTextTemplateValue(val bson.D, text string) bson.D {
	result := make(bson.D, 0, len(val))
	for _, elem := range val {
		if elem.Key == "TextTemplate" {
			if tmpl, ok := elem.Value.(bson.D); ok && tmpl != nil {
				result = append(result, bson.E{Key: "TextTemplate", Value: updateTemplateText(tmpl, text)})
			} else {
				// TextTemplate was null in the template — skip.
				// Creating a TextTemplate from null triggers CE0463 because Studio Pro
				// detects the structural change. The template must be extracted from a
				// widget that already has this property configured in Studio Pro.
				result = append(result, elem)
			}
		} else {
			result = append(result, elem)
		}
	}
	return result
}

// updateTemplateText updates the Template.Items in a Forms$ClientTemplate with a text value.
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

// opAction sets a client action on a widget property.
func opAction(obj bson.D, propTypeIDs map[string]pages.PropertyTypeIDEntry, propertyKey string, ctx *BuildContext) bson.D {
	if ctx.ActionBSON == nil {
		return obj
	}
	return updateWidgetPropertyValue(obj, propTypeIDs, propertyKey, func(val bson.D) bson.D {
		result := make(bson.D, 0, len(val))
		for _, elem := range val {
			if elem.Key == "Action" {
				result = append(result, bson.E{Key: "Action", Value: ctx.ActionBSON})
			} else {
				result = append(result, elem)
			}
		}
		return result
	})
}

// opAttributeObjects populates the Objects array in an "attributes" property
// with attribute reference objects. Used by filter widgets (TEXTFILTER, etc.).
func opAttributeObjects(obj bson.D, propTypeIDs map[string]pages.PropertyTypeIDEntry, propertyKey string, ctx *BuildContext) bson.D {
	if len(ctx.AttributePaths) == 0 {
		return obj
	}

	entry, ok := propTypeIDs[propertyKey]
	if !ok || entry.ObjectTypeID == "" {
		return obj
	}

	// Get nested "attribute" property IDs from the PropertyTypeIDEntry
	nestedEntry, ok := entry.NestedPropertyIDs["attribute"]
	if !ok {
		return obj
	}

	return updateWidgetPropertyValue(obj, propTypeIDs, propertyKey, func(val bson.D) bson.D {
		objects := make([]any, 0, len(ctx.AttributePaths)+1)
		objects = append(objects, int32(2)) // BSON array version marker

		for _, attrPath := range ctx.AttributePaths {
			attrObj, err := ctx.pageBuilder.createAttributeObject(attrPath, entry.ObjectTypeID, nestedEntry.PropertyTypeID, nestedEntry.ValueTypeID)
			if err != nil {
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
}
