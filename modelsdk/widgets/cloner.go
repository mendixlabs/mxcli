// SPDX-License-Identifier: Apache-2.0

package widgets

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/widgets/mpk"
)

// PropertyCloner handles deep cloning of widget property+propertyType pairs
// with ID regeneration.
type PropertyCloner struct {
	idGenerator func() string
}

// NewPropertyCloner creates a PropertyCloner with the given ID generator.
func NewPropertyCloner(idGen func() string) *PropertyCloner {
	return &PropertyCloner{idGenerator: idGen}
}

// ClonePair deep-clones an existing PropertyType/Property pair and updates keys/IDs.
// It finds the exemplar PropertyType at exemplarIdx in propTypes, clones it,
// and finds the corresponding Property in objProps via TypePointer matching.
func (c *PropertyCloner) ClonePair(propTypes []any, objProps []any, exemplarIdx int, p mpk.PropertyDef) (map[string]any, map[string]any, error) {
	exemplar, ok := propTypes[exemplarIdx].(map[string]any)
	if !ok {
		return nil, nil, nil
	}

	// Deep-clone the PropertyType
	newPT, err := deepCloneMap(exemplar)
	if err != nil {
		return nil, nil, fmt.Errorf("clone property type %q: %w", p.Key, err)
	}
	newPTID := c.idGenerator()
	newPT["$ID"] = newPTID
	newPT["PropertyKey"] = p.Key
	newPT["Caption"] = p.Caption
	newPT["Description"] = p.Description
	newPT["Category"] = p.Category

	// Update the ValueType ID and set defaults
	var newVTID string
	if vt, ok := getMapField(newPT, "ValueType"); ok {
		// Regenerate nested $ID fields FIRST (EnumerationValues, ObjectType, etc.)
		// so they get unique placeholders without overwriting the IDs we set below.
		c.RegenerateIDs(vt)

		// Now set the top-level VT $ID -- this must happen AFTER RegenerateIDs
		// because RegenerateIDs replaces ALL $ID fields including this one.
		// The Property's Value.TypePointer will reference this ID, so it must match.
		newVTID = c.idGenerator()
		vt["$ID"] = newVTID

		// Set default value for enumeration/boolean types
		if p.DefaultValue != "" {
			vt["DefaultValue"] = p.DefaultValue
		}

		// Update Required flag
		vt["Required"] = p.Required

		// Update IsList
		vt["IsList"] = p.IsList

		// Update DataSourceProperty
		if p.DataSource != "" {
			vt["DataSourceProperty"] = p.DataSource
		} else {
			vt["DataSourceProperty"] = ""
		}

		// Clear enumeration values for non-enumeration types or set empty
		vtType, _ := vt["Type"].(string)
		if vtType != "Enumeration" {
			vt["EnumerationValues"] = []any{float64(2)}
		}

		// Clear ObjectType for non-object types; build nested ObjectType for object types with children
		if vtType != "Object" {
			vt["ObjectType"] = nil
		} else if len(p.Children) > 0 {
			vt["ObjectType"] = buildNestedObjectType(p.Children)
		}

		// Clear ReturnType for non-expression types
		if vtType != "Expression" {
			vt["ReturnType"] = nil
		}
	}

	// Find the corresponding Property in objProps that uses the exemplar's TypePointer
	exemplarID, _ := exemplar["$ID"].(string)
	var exemplarProp map[string]any
	for _, prop := range objProps {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		tp, _ := propMap["TypePointer"].(string)
		if tp == exemplarID {
			exemplarProp = propMap
			break
		}
	}

	if exemplarProp == nil {
		return newPT, nil, nil
	}

	// Deep-clone the Property
	newProp, err := deepCloneMap(exemplarProp)
	if err != nil {
		return nil, nil, fmt.Errorf("clone property %q: %w", p.Key, err)
	}
	newProp["$ID"] = c.idGenerator()
	newProp["TypePointer"] = newPTID

	// Update Value.TypePointer to reference the new ValueType ID
	if val, ok := getMapField(newProp, "Value"); ok {
		val["$ID"] = c.idGenerator()
		if newVTID != "" {
			val["TypePointer"] = newVTID
		}

		// Reset the value to default for the type
		resetPropertyValue(val, p)

		// Regenerate action ID
		if action, ok := getMapField(val, "Action"); ok {
			action["$ID"] = c.idGenerator()
		}

		// Regenerate TextTemplate IDs if present
		if tt, ok := getMapField(val, "TextTemplate"); ok {
			c.RegenerateIDs(tt)
		}
	}

	return newPT, newProp, nil
}

// RegenerateIDs walks a structure and replaces all $ID fields with new IDs
// from the cloner's ID generator.
func (c *PropertyCloner) RegenerateIDs(m map[string]any) {
	for key, val := range m {
		if key == "$ID" {
			m[key] = c.idGenerator()
			continue
		}
		switch v := val.(type) {
		case map[string]any:
			c.RegenerateIDs(v)
		case []any:
			for _, item := range v {
				if nested, ok := item.(map[string]any); ok {
					c.RegenerateIDs(nested)
				}
			}
		}
	}
}
