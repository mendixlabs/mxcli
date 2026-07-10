// SPDX-License-Identifier: Apache-2.0

package widgets

import (
	"encoding/json"
	"fmt"
	"sync/atomic"

	"github.com/JordtenBulte-OLC/mxcli/sdk/widgets/mpk"
)

// AugmentTemplate modifies a template's Type and Object in-place to match an .mpk definition.
// It adds PropertyTypes (in Type) and Properties (in Object) for keys present in .mpk but
// missing from the template, and removes those present in the template but missing from .mpk.
// Only regular properties are compared (not system properties like Label, Visibility, Editability).
func AugmentTemplate(tmpl *WidgetTemplate, def *mpk.WidgetDefinition) error {
	if tmpl == nil || def == nil {
		return nil
	}

	// Get PropertyTypes array from Type.ObjectType.PropertyTypes
	objType, ok := getMapField(tmpl.Type, "ObjectType")
	if !ok {
		return nil
	}
	propTypes, ok := getArrayField(objType, "PropertyTypes")
	if !ok {
		return nil
	}

	// Get Properties array from Object.Properties
	objProps, ok := getArrayField(tmpl.Object, "Properties")
	if !ok {
		return nil
	}

	// Build set of existing template property keys (non-system only)
	templateKeys := make(map[string]bool)
	// Also build a map of XML type -> exemplar index for cloning
	typeExemplars := make(map[string]int) // ValueType.Type -> index in propTypes
	systemKeys := def.SystemPropertyKeys()

	for i, pt := range propTypes {
		ptMap, ok := pt.(map[string]any)
		if !ok {
			continue
		}
		key, _ := ptMap["PropertyKey"].(string)
		if key == "" {
			continue
		}
		// Skip system properties
		if systemKeys[key] {
			continue
		}
		templateKeys[key] = true

		// Record exemplar for this value type
		vt, ok := getMapField(ptMap, "ValueType")
		if ok {
			vtType, _ := vt["Type"].(string)
			if vtType != "" {
				if _, exists := typeExemplars[vtType]; !exists {
					typeExemplars[vtType] = i
				}
			}
		}
	}

	// Determine mpk property keys (regular only)
	mpkKeys := def.PropertyKeys()

	// Find missing keys (in mpk but not in template)
	var missing []mpk.PropertyDef
	for _, p := range def.Properties {
		if !templateKeys[p.Key] {
			missing = append(missing, p)
		}
	}

	// Find stale keys (in template but not in mpk, excluding system props)
	var stale []string
	for key := range templateKeys {
		if !mpkKeys[key] && !systemKeys[key] {
			stale = append(stale, key)
		}
	}

	// Check if nested augmentation is needed (skip early return if so)
	hasNestedChildren := false
	for _, p := range def.Properties {
		if len(p.Children) > 0 {
			hasNestedChildren = true
			break
		}
	}

	// Nothing to add/remove at top level, and no nested children to process
	if len(missing) == 0 && len(stale) == 0 && !hasNestedChildren {
		return nil
	}

	// Remove stale properties
	if len(stale) > 0 {
		staleSet := make(map[string]bool, len(stale))
		for _, key := range stale {
			staleSet[key] = true
		}
		propTypes, objProps = removeProperties(propTypes, objProps, staleSet)
	}

	// Add missing properties
	for _, p := range missing {
		bsonType := xmlTypeToBSONType(p.Type)
		if bsonType == "" {
			continue // Unknown type, skip
		}

		// Find an exemplar of the same type to clone
		exemplarIdx, hasExemplar := typeExemplars[bsonType]
		var newPropType, newProp map[string]any
		if hasExemplar {
			var err error
			newPropType, newProp, err = clonePropertyPair(propTypes, objProps, exemplarIdx, p)
			if err != nil {
				return fmt.Errorf("augment %s: %w", tmpl.WidgetID, err)
			}
		}
		// Fall back to createPropertyPair if cloning failed (no exemplar or no matching property)
		if newPropType == nil || newProp == nil {
			newPropType, newProp = createPropertyPair(p, bsonType)
		}

		if newPropType != nil {
			propTypes = append(propTypes, newPropType)
		}
		if newProp != nil {
			objProps = append(objProps, newProp)
		}
	}

	// Write back top-level
	setArrayField(objType, "PropertyTypes", propTypes)
	setArrayField(tmpl.Object, "Properties", objProps)

	// Augment nested ObjectType properties (e.g., DataGrid2 column properties).
	// Top-level augmentation syncs the property list, but nested ObjectTypes inside
	// IsList Object properties also need syncing when the .mpk version differs
	// from the template version.
	for _, mpkProp := range def.Properties {
		if len(mpkProp.Children) == 0 {
			continue
		}
		if err := augmentNestedObjectType(propTypes, objProps, mpkProp); err != nil {
			return fmt.Errorf("augment nested %s: %w", mpkProp.Key, err)
		}
	}

	return nil
}

// augmentNestedObjectType syncs nested ObjectType PropertyTypes for an Object-type property.
// When a .mpk defines children for a property (e.g., DataGrid2 "columns" has showContentAs,
// attribute, content, header, etc.), this function ensures the template's nested ObjectType
// has the same PropertyTypes as the .mpk, adding missing ones and removing stale ones.
func augmentNestedObjectType(propTypes []any, objProps []any, mpkProp mpk.PropertyDef) error {
	// Find the PropertyType matching this .mpk property
	var matchedPT map[string]any
	var matchedPTID string
	for _, pt := range propTypes {
		ptMap, ok := pt.(map[string]any)
		if !ok {
			continue
		}
		key, _ := ptMap["PropertyKey"].(string)
		if key == mpkProp.Key {
			matchedPT = ptMap
			matchedPTID, _ = ptMap["$ID"].(string)
			break
		}
	}
	if matchedPT == nil {
		return nil // PropertyType not found; top-level augmentation should have added it
	}

	// Navigate to ValueType.ObjectType.PropertyTypes
	vt, ok := getMapField(matchedPT, "ValueType")
	if !ok {
		return nil
	}
	nestedObjType, ok := getMapField(vt, "ObjectType")
	if !ok || nestedObjType == nil {
		return nil
	}
	nestedPropTypes, ok := getArrayField(nestedObjType, "PropertyTypes")
	if !ok {
		return nil
	}

	// Build set of existing nested property keys
	existingKeys := make(map[string]bool)
	nestedExemplars := make(map[string]int)
	for i, npt := range nestedPropTypes {
		nptMap, ok := npt.(map[string]any)
		if !ok {
			continue
		}
		key, _ := nptMap["PropertyKey"].(string)
		if key == "" {
			continue
		}
		existingKeys[key] = true

		// Record exemplar by value type
		nvt, ok := getMapField(nptMap, "ValueType")
		if ok {
			vtType, _ := nvt["Type"].(string)
			if vtType != "" {
				if _, exists := nestedExemplars[vtType]; !exists {
					nestedExemplars[vtType] = i
				}
			}
		}
	}

	// Build set of .mpk child keys
	mpkChildKeys := make(map[string]bool)
	for _, child := range mpkProp.Children {
		mpkChildKeys[child.Key] = true
	}

	// Find missing (in .mpk but not in template nested ObjectType)
	var missing []mpk.PropertyDef
	for _, child := range mpkProp.Children {
		if !existingKeys[child.Key] {
			missing = append(missing, child)
		}
	}

	// Find stale (in template but not in .mpk)
	var staleKeys []string
	for key := range existingKeys {
		if !mpkChildKeys[key] {
			staleKeys = append(staleKeys, key)
		}
	}

	if len(missing) == 0 && len(staleKeys) == 0 {
		return nil
	}

	// Also find the corresponding Object WidgetProperty and its nested WidgetObjects
	var nestedObjProps [][]any // one per WidgetObject in the Objects array
	var objPropContainers []map[string]any
	for _, prop := range objProps {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		tp, _ := propMap["TypePointer"].(string)
		if tp != matchedPTID {
			continue
		}
		// Navigate to Value.Objects
		val, ok := getMapField(propMap, "Value")
		if !ok {
			continue
		}
		objects, ok := getArrayField(val, "Objects")
		if !ok {
			continue
		}
		for _, obj := range objects {
			objMap, ok := obj.(map[string]any)
			if !ok {
				continue
			}
			props, ok := getArrayField(objMap, "Properties")
			if ok {
				nestedObjProps = append(nestedObjProps, props)
				objPropContainers = append(objPropContainers, objMap)
			}
		}
		break
	}

	// Remove stale nested PropertyTypes and Properties
	if len(staleKeys) > 0 {
		staleSet := make(map[string]bool, len(staleKeys))
		for _, key := range staleKeys {
			staleSet[key] = true
		}
		// Collect IDs of stale PropertyTypes
		staleIDs := make(map[string]bool)
		var filteredPropTypes []any
		for _, npt := range nestedPropTypes {
			nptMap, ok := npt.(map[string]any)
			if !ok {
				filteredPropTypes = append(filteredPropTypes, npt)
				continue
			}
			key, _ := nptMap["PropertyKey"].(string)
			if staleSet[key] {
				id, _ := nptMap["$ID"].(string)
				if id != "" {
					staleIDs[id] = true
				}
				continue
			}
			filteredPropTypes = append(filteredPropTypes, npt)
		}
		nestedPropTypes = filteredPropTypes

		// Remove matching Properties from each WidgetObject
		for i, nop := range nestedObjProps {
			var filtered []any
			for _, prop := range nop {
				propMap, ok := prop.(map[string]any)
				if !ok {
					filtered = append(filtered, prop)
					continue
				}
				tp, _ := propMap["TypePointer"].(string)
				if staleIDs[tp] {
					continue
				}
				filtered = append(filtered, prop)
			}
			nestedObjProps[i] = filtered
		}
	}

	// Add missing nested PropertyTypes and Properties
	for _, child := range missing {
		bsonType := xmlTypeToBSONType(child.Type)
		if bsonType == "" {
			continue
		}

		exemplarIdx, hasExemplar := nestedExemplars[bsonType]
		var newPropType, newProp map[string]any
		if hasExemplar && len(nestedObjProps) > 0 {
			var err error
			newPropType, newProp, err = clonePropertyPair(nestedPropTypes, nestedObjProps[0], exemplarIdx, child)
			if err != nil {
				return fmt.Errorf("clone nested property %q: %w", child.Key, err)
			}
		}
		if newPropType == nil || newProp == nil {
			newPropType, newProp = createPropertyPair(child, bsonType)
		}

		if newPropType != nil {
			nestedPropTypes = append(nestedPropTypes, newPropType)
		}
		if newProp != nil {
			for i := range nestedObjProps {
				nestedObjProps[i] = append(nestedObjProps[i], newProp)
			}
		}
	}

	// Write back nested PropertyTypes
	setArrayField(nestedObjType, "PropertyTypes", nestedPropTypes)

	// Write back nested Object Properties
	for i, container := range objPropContainers {
		if i < len(nestedObjProps) {
			setArrayField(container, "Properties", nestedObjProps[i])
		}
	}

	return nil
}

// removeProperties removes PropertyTypes and their corresponding Properties by PropertyKey.
func removeProperties(propTypes []any, objProps []any, staleKeys map[string]bool) ([]any, []any) {
	// Collect IDs of PropertyTypes to remove
	removeIDs := make(map[string]bool)
	var newPropTypes []any
	for _, pt := range propTypes {
		ptMap, ok := pt.(map[string]any)
		if !ok {
			newPropTypes = append(newPropTypes, pt) // Keep markers (e.g., float64(2))
			continue
		}
		key, _ := ptMap["PropertyKey"].(string)
		if staleKeys[key] {
			id, _ := ptMap["$ID"].(string)
			if id != "" {
				removeIDs[id] = true
			}
			continue // Skip this PropertyType
		}
		newPropTypes = append(newPropTypes, pt)
	}

	// Remove corresponding Properties whose TypePointer matches a removed PropertyType
	var newObjProps []any
	for _, prop := range objProps {
		propMap, ok := prop.(map[string]any)
		if !ok {
			newObjProps = append(newObjProps, prop) // Keep markers
			continue
		}
		tp, _ := propMap["TypePointer"].(string)
		if removeIDs[tp] {
			continue // Remove this property
		}
		newObjProps = append(newObjProps, prop)
	}

	return newPropTypes, newObjProps
}

// clonePropertyPair deep-clones an existing PropertyType/Property pair and updates keys/IDs.
func clonePropertyPair(propTypes []any, objProps []any, exemplarIdx int, p mpk.PropertyDef) (map[string]any, map[string]any, error) {
	exemplar, ok := propTypes[exemplarIdx].(map[string]any)
	if !ok {
		return nil, nil, nil
	}

	// Deep-clone the PropertyType
	newPT, err := deepCloneMap(exemplar)
	if err != nil {
		return nil, nil, fmt.Errorf("clone property type %q: %w", p.Key, err)
	}
	newPTID := placeholderID()
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
		regenerateNestedIDs(vt)

		// Now set the top-level VT $ID — this must happen AFTER regenerateNestedIDs
		// because regenerateNestedIDs replaces ALL $ID fields including this one.
		// The Property's Value.TypePointer will reference this ID, so it must match.
		newVTID = placeholderID()
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
	newProp["$ID"] = placeholderID()
	newProp["TypePointer"] = newPTID

	// Update Value.TypePointer to reference the new ValueType ID
	if val, ok := getMapField(newProp, "Value"); ok {
		val["$ID"] = placeholderID()
		if newVTID != "" {
			val["TypePointer"] = newVTID
		}

		// Reset the value to default for the type
		resetPropertyValue(val, p)

		// Regenerate action ID
		if action, ok := getMapField(val, "Action"); ok {
			action["$ID"] = placeholderID()
		}

		// Regenerate TextTemplate IDs if present
		if tt, ok := getMapField(val, "TextTemplate"); ok {
			regenerateNestedIDs(tt)
		}
	}

	return newPT, newProp, nil
}

// createPropertyPair creates a new PropertyType/Property pair from scratch.
func createPropertyPair(p mpk.PropertyDef, bsonType string) (map[string]any, map[string]any) {
	ptID := placeholderID()
	vtID := placeholderID()

	// Create PropertyType
	pt := map[string]any{
		"$ID":         ptID,
		"$Type":       "CustomWidgets$WidgetPropertyType",
		"Caption":     p.Caption,
		"Category":    p.Category,
		"Description": p.Description,
		"IsDefault":   false,
		"PropertyKey": p.Key,
		"ValueType":   createDefaultValueType(vtID, bsonType, p),
	}

	// Create Property (WidgetProperty with WidgetValue)
	prop := map[string]any{
		"$ID":         placeholderID(),
		"$Type":       "CustomWidgets$WidgetProperty",
		"TypePointer": ptID,
		"Value":       createDefaultWidgetValue(vtID, bsonType, p),
	}

	return pt, prop
}

// createDefaultValueType creates a default ValueType structure for a given BSON type.
func createDefaultValueType(vtID string, bsonType string, p mpk.PropertyDef) map[string]any {
	vt := map[string]any{
		"$ID":                         vtID,
		"$Type":                       "CustomWidgets$WidgetValueType",
		"ActionVariables":             []any{float64(2)},
		"AllowNonPersistableEntities": false,
		"AllowUpload":                 false,
		"AllowedTypes":                []any{float64(1)},
		"AssociationTypes":            []any{float64(1)},
		"DataSourceProperty":          "",
		"DefaultType":                 "None",
		"DefaultValue":                p.DefaultValue,
		"EntityProperty":              "",
		"EnumerationValues":           []any{float64(2)},
		"IsLinked":                    false,
		"IsList":                      p.IsList,
		"IsMetaData":                  false,
		"IsPath":                      "No",
		"Multiline":                   false,
		"ObjectType":                  nil,
		"OnChangeProperty":            "",
		"ParameterIsList":             false,
		"PathType":                    "None",
		"Required":                    p.Required,
		"ReturnType":                  nil,
		"SelectableObjectsProperty":   "",
		"SelectionTypes":              []any{float64(1)},
		"SetLabel":                    false,
		"Translations":                []any{float64(2)},
		"Type":                        bsonType,
	}

	if p.DataSource != "" {
		vt["DataSourceProperty"] = p.DataSource
	}

	// Build nested ObjectType for object-type properties with children
	if bsonType == "Object" && len(p.Children) > 0 {
		vt["ObjectType"] = buildNestedObjectType(p.Children)
	}

	return vt
}

// createDefaultWidgetValue creates a default WidgetValue for a given BSON type.
func createDefaultWidgetValue(vtID string, bsonType string, p mpk.PropertyDef) map[string]any {
	val := map[string]any{
		"$ID":               placeholderID(),
		"$Type":             "CustomWidgets$WidgetValue",
		"Action":            createDefaultNoAction(),
		"AttributeRef":      nil,
		"DataSource":        nil,
		"EntityRef":         nil,
		"Expression":        "",
		"Form":              "",
		"Icon":              nil,
		"Image":             "",
		"Microflow":         "",
		"Nanoflow":          "",
		"Objects":           []any{float64(2)},
		"PrimitiveValue":    "",
		"Selection":         "None",
		"SourceVariable":    nil,
		"TextTemplate":      nil,
		"TranslatableValue": nil,
		"TypePointer":       vtID,
		"Widgets":           []any{float64(2)},
		"XPathConstraint":   "",
	}

	// Set type-specific defaults
	switch bsonType {
	case "Boolean":
		if p.DefaultValue != "" {
			val["PrimitiveValue"] = p.DefaultValue
		} else {
			val["PrimitiveValue"] = "false"
		}
	case "Integer":
		if p.DefaultValue != "" {
			val["PrimitiveValue"] = p.DefaultValue
		} else {
			val["PrimitiveValue"] = "0"
		}
	case "Enumeration":
		if p.DefaultValue != "" {
			val["PrimitiveValue"] = p.DefaultValue
		}
	case "TextTemplate":
		val["TextTemplate"] = createDefaultClientTemplate()
	}

	return val
}

// createDefaultNoAction creates a default Forms$NoAction structure.
func createDefaultNoAction() map[string]any {
	return map[string]any{
		"$ID":                     placeholderID(),
		"$Type":                   "Forms$NoAction",
		"DisabledDuringExecution": true,
	}
}

// createDefaultClientTemplate creates a default Forms$ClientTemplate structure.
func createDefaultClientTemplate() map[string]any {
	return map[string]any{
		"$ID":   placeholderID(),
		"$Type": "Forms$ClientTemplate",
		"Fallback": map[string]any{
			"$ID":   placeholderID(),
			"$Type": "Texts$Text",
			"Items": []any{float64(3)},
		},
		"Parameters": []any{float64(2)},
		"Template": map[string]any{
			"$ID":   placeholderID(),
			"$Type": "Texts$Text",
			"Items": []any{float64(3)},
		},
	}
}

// resetPropertyValue resets a WidgetValue to defaults for the given property type.
func resetPropertyValue(val map[string]any, p mpk.PropertyDef) {
	bsonType := xmlTypeToBSONType(p.Type)

	// Reset all value fields to defaults
	val["AttributeRef"] = nil
	val["DataSource"] = nil
	val["EntityRef"] = nil
	val["Expression"] = ""
	val["Form"] = ""
	val["Icon"] = nil
	val["Image"] = ""
	val["Microflow"] = ""
	val["Nanoflow"] = ""
	val["Objects"] = []any{float64(2)}
	val["PrimitiveValue"] = ""
	val["Selection"] = "None"
	val["SourceVariable"] = nil
	val["TextTemplate"] = nil
	val["TranslatableValue"] = nil
	val["Widgets"] = []any{float64(2)}
	val["XPathConstraint"] = ""

	// Set type-specific defaults
	switch bsonType {
	case "Boolean":
		if p.DefaultValue != "" {
			val["PrimitiveValue"] = p.DefaultValue
		} else {
			val["PrimitiveValue"] = "false"
		}
	case "Integer":
		if p.DefaultValue != "" {
			val["PrimitiveValue"] = p.DefaultValue
		} else {
			val["PrimitiveValue"] = "0"
		}
	case "Enumeration":
		if p.DefaultValue != "" {
			val["PrimitiveValue"] = p.DefaultValue
		}
	case "TextTemplate":
		val["TextTemplate"] = createDefaultClientTemplate()
	}
}

// xmlTypeToBSONType maps XML property type to BSON ValueType.Type.
func xmlTypeToBSONType(xmlType string) string {
	switch mpk.NormalizeType(xmlType) {
	case "attribute":
		return "Attribute"
	case "expression":
		return "Expression"
	case "textTemplate":
		return "TextTemplate"
	case "widgets":
		return "Widgets"
	case "enumeration":
		return "Enumeration"
	case "boolean":
		return "Boolean"
	case "integer":
		return "Integer"
	case "datasource":
		return "DataSource"
	case "action":
		return "Action"
	case "selection":
		return "Selection"
	case "association":
		return "Association"
	case "object":
		return "Object"
	case "string":
		return "String"
	case "decimal":
		return "Decimal"
	case "icon":
		return "Icon"
	case "image":
		return "Image"
	case "file":
		return "File"
	default:
		return ""
	}
}

// buildNestedObjectType creates a WidgetObjectType with PropertyTypes for nested children
// of an object-type property. This is needed for properties like filterList and sortList
// that contain sub-properties (e.g., filter, attribute, caption).
func buildNestedObjectType(children []mpk.PropertyDef) map[string]any {
	propTypes := []any{float64(2)} // version marker

	for _, child := range children {
		childBsonType := xmlTypeToBSONType(child.Type)
		if childBsonType == "" {
			continue
		}

		childVTID := placeholderID()
		childPT := map[string]any{
			"$ID":         placeholderID(),
			"$Type":       "CustomWidgets$WidgetPropertyType",
			"Caption":     child.Caption,
			"Category":    "General",
			"Description": child.Description,
			"IsDefault":   false,
			"PropertyKey": child.Key,
			"ValueType":   createDefaultValueType(childVTID, childBsonType, child),
		}

		propTypes = append(propTypes, childPT)
	}

	return map[string]any{
		"$ID":           placeholderID(),
		"$Type":         "CustomWidgets$WidgetObjectType",
		"PropertyTypes": propTypes,
	}
}

// --- Helpers ---

// placeholderCounter generates sequential placeholder IDs (atomic for concurrent safety).
var placeholderCounter atomic.Uint32

// placeholderID generates a placeholder hex ID. These will be remapped by collectIDs
// in GetTemplateFullBSON, so exact values don't matter — they just need to be unique
// 32-char hex strings.
func placeholderID() string {
	n := placeholderCounter.Add(1)
	return fmt.Sprintf("aa000000000000000000000000%06x", n)
}

// ResetPlaceholderCounter resets the counter (for testing).
func ResetPlaceholderCounter() {
	placeholderCounter.Store(0)
}

// getMapField gets a nested map field from a JSON map.
func getMapField(m map[string]any, key string) (map[string]any, bool) {
	val, ok := m[key]
	if !ok {
		return nil, false
	}
	nested, ok := val.(map[string]any)
	return nested, ok
}

// getArrayField gets an array field from a JSON map.
func getArrayField(m map[string]any, key string) ([]any, bool) {
	val, ok := m[key]
	if !ok {
		return nil, false
	}
	arr, ok := val.([]any)
	return arr, ok
}

// setArrayField sets an array field in a JSON map.
func setArrayField(m map[string]any, key string, arr []any) {
	m[key] = arr
}

// deepCloneMap deep-clones a map[string]interface{} via JSON round-trip.
func deepCloneMap(m map[string]any) (map[string]any, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("deep clone marshal: %w", err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("deep clone unmarshal: %w", err)
	}
	return result, nil
}

// regenerateNestedIDs walks a structure and replaces all $ID fields with new placeholders.
func regenerateNestedIDs(m map[string]any) {
	for key, val := range m {
		if key == "$ID" {
			m[key] = placeholderID()
			continue
		}
		switch v := val.(type) {
		case map[string]any:
			regenerateNestedIDs(v)
		case []any:
			for _, item := range v {
				if nested, ok := item.(map[string]any); ok {
					regenerateNestedIDs(nested)
				}
			}
		}
	}
}

// deepCloneTemplate deep-clones a WidgetTemplate so augmentation doesn't mutate the cache.
func deepCloneTemplate(tmpl *WidgetTemplate) (*WidgetTemplate, error) {
	clone := &WidgetTemplate{
		WidgetID:      tmpl.WidgetID,
		Name:          tmpl.Name,
		Version:       tmpl.Version,
		ExtractedFrom: tmpl.ExtractedFrom,
	}

	if tmpl.Type != nil {
		var err error
		clone.Type, err = deepCloneMap(tmpl.Type)
		if err != nil {
			return nil, fmt.Errorf("clone template type %s: %w", tmpl.WidgetID, err)
		}
	}
	if tmpl.Object != nil {
		var err error
		clone.Object, err = deepCloneMap(tmpl.Object)
		if err != nil {
			return nil, fmt.Errorf("clone template object %s: %w", tmpl.WidgetID, err)
		}
	}

	return clone, nil
}

// collectNestedPropertyTypeIDs extracts PropertyKey→$ID mappings from a ValueType's ObjectType.
func collectNestedPropertyTypeIDs(vt map[string]any) map[string]string {
	result := make(map[string]string)
	objType, ok := getMapField(vt, "ObjectType")
	if !ok {
		return result
	}
	propTypes, ok := getArrayField(objType, "PropertyTypes")
	if !ok {
		return result
	}
	for _, pt := range propTypes {
		ptMap, ok := pt.(map[string]any)
		if !ok {
			continue
		}
		key, _ := ptMap["PropertyKey"].(string)
		id, _ := ptMap["$ID"].(string)
		if key != "" && id != "" {
			result[key] = id
		}
	}
	return result
}

// collectNestedPropertyTypeIDsByKey extracts PropertyKey→$ID from a rebuilt ObjectType map.
func collectNestedPropertyTypeIDsByKey(objType map[string]any) map[string]string {
	result := make(map[string]string)
	propTypes, ok := getArrayField(objType, "PropertyTypes")
	if !ok {
		return result
	}
	for _, pt := range propTypes {
		ptMap, ok := pt.(map[string]any)
		if !ok {
			continue
		}
		key, _ := ptMap["PropertyKey"].(string)
		id, _ := ptMap["$ID"].(string)
		if key != "" && id != "" {
			result[key] = id
		}
	}
	return result
}

// remapObjectTypePointers walks the Object Properties array and updates TypePointers
// that reference old PropertyType IDs from a rebuilt ObjectType.
func remapObjectTypePointers(objProps []any, idRemap map[string]string) {
	if len(idRemap) == 0 {
		return
	}
	for _, prop := range objProps {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		// Check Value.Objects for nested WidgetObjects with TypePointers
		val, ok := getMapField(propMap, "Value")
		if !ok {
			continue
		}
		objects, ok := getArrayField(val, "Objects")
		if !ok {
			continue
		}
		for _, obj := range objects {
			objMap, ok := obj.(map[string]any)
			if !ok {
				continue
			}
			// Remap the object's nested properties' TypePointers
			nestedProps, ok := getArrayField(objMap, "Properties")
			if !ok {
				continue
			}
			for _, nestedProp := range nestedProps {
				npMap, ok := nestedProp.(map[string]any)
				if !ok {
					continue
				}
				if tp, ok := npMap["TypePointer"].(string); ok {
					if newTP, exists := idRemap[tp]; exists {
						npMap["TypePointer"] = newTP
					}
				}
				// Also remap Value.TypePointer (references ValueType $ID)
				if nestedVal, ok := getMapField(npMap, "Value"); ok {
					if tp, ok := nestedVal["TypePointer"].(string); ok {
						if newTP, exists := idRemap[tp]; exists {
							nestedVal["TypePointer"] = newTP
						}
					}
				}
			}
		}
	}
}
