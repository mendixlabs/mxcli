// SPDX-License-Identifier: Apache-2.0

// Package mpr - Java action parsing.
package mpr

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/javaactions"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ReadJavaAction reads a Java action by its ID.
func (r *Reader) ReadJavaAction(id model.ID) (*javaactions.JavaAction, error) {
	units, err := r.listUnitsByType("JavaActions$JavaAction")
	if err != nil {
		return nil, err
	}

	for _, u := range units {
		if u.ID == string(id) {
			return r.parseJavaActionFull(u.ID, u.ContainerID, u.Contents)
		}
	}

	return nil, fmt.Errorf("java action not found: %s", id)
}

// ReadJavaActionByName reads a Java action by its qualified name (Module.ActionName).
func (r *Reader) ReadJavaActionByName(qualifiedName string) (*javaactions.JavaAction, error) {
	// First, list all Java actions
	units, err := r.listUnitsByType("JavaActions$JavaAction")
	if err != nil {
		return nil, err
	}

	// Build module and folder hierarchy
	modules, err := r.ListModules()
	if err != nil {
		return nil, err
	}
	moduleNames := make(map[model.ID]string)
	for _, m := range modules {
		moduleNames[m.ID] = m.Name
	}

	// Get all folders for hierarchy resolution
	folders, err := r.ListFolders()
	if err != nil {
		return nil, err
	}
	folderContainers := make(map[model.ID]model.ID)
	for _, f := range folders {
		folderContainers[f.ID] = f.ContainerID
	}

	for _, u := range units {
		contents, err := r.resolveContents(u.ID, u.Contents)
		if err != nil {
			continue
		}

		var raw map[string]any
		if err := bson.Unmarshal(contents, &raw); err != nil {
			continue
		}

		name := extractString(raw["Name"])

		// Find module name by walking up the container hierarchy
		modName := ""
		containerID := model.ID(u.ContainerID)
		for range 20 { // Max depth to prevent infinite loops
			if mn, ok := moduleNames[containerID]; ok {
				modName = mn
				break
			}
			// Check if container is a folder and get its parent
			if parent, ok := folderContainers[containerID]; ok {
				containerID = parent
			} else {
				break
			}
		}

		fullName := modName + "." + name
		if fullName == qualifiedName {
			return r.parseJavaActionFull(u.ID, u.ContainerID, contents)
		}
	}

	return nil, fmt.Errorf("java action not found: %s", qualifiedName)
}

// parseJavaActionFull parses a Java action with full details.
func (r *Reader) parseJavaActionFull(unitID, containerID string, contents []byte) (*javaactions.JavaAction, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	ja := &javaactions.JavaAction{}
	ja.ID = model.ID(unitID)
	ja.TypeName = "JavaActions$JavaAction"
	ja.ContainerID = model.ID(containerID)

	// Basic fields
	ja.Name = extractString(raw["Name"])
	ja.Documentation = extractString(raw["Documentation"])
	ja.Excluded = extractBool(raw["Excluded"], false)
	ja.ExportLevel = extractString(raw["ExportLevel"])
	ja.ActionDefaultReturnName = extractString(raw["ActionDefaultReturnName"])

	// Parse return type - handle both map and primitive.D
	switch rt := raw["JavaReturnType"].(type) {
	case map[string]any:
		ja.ReturnType = parseCodeActionReturnType(rt)
	case primitive.D:
		ja.ReturnType = parseCodeActionReturnType(primitiveToMap(rt))
	}

	// Parse parameters - handle both map and primitive.D and primitive.A
	switch params := raw["Parameters"].(type) {
	case []any:
		for _, p := range params {
			pMap := toMap(p)
			if pMap != nil {
				param := parseJavaActionParameter(pMap)
				if param != nil {
					ja.Parameters = append(ja.Parameters, param)
				}
			}
		}
	case primitive.A:
		for _, p := range params {
			pMap := toMap(p)
			if pMap != nil {
				param := parseJavaActionParameter(pMap)
				if param != nil {
					ja.Parameters = append(ja.Parameters, param)
				}
			}
		}
	}

	// Parse type parameters (generics) - preserve IDs for BY_ID references
	switch typeParams := raw["TypeParameters"].(type) {
	case []any:
		for _, tp := range typeParams {
			tpMap := toMap(tp)
			if tpMap != nil {
				if name := extractString(tpMap["Name"]); name != "" {
					tpDef := &javaactions.TypeParameterDef{
						BaseElement: model.BaseElement{ID: model.ID(extractBsonID(tpMap["$ID"]))},
						Name:        name,
					}
					ja.TypeParameters = append(ja.TypeParameters, tpDef)
				}
			}
		}
	case primitive.A:
		for _, tp := range typeParams {
			tpMap := toMap(tp)
			if tpMap != nil {
				if name := extractString(tpMap["Name"]); name != "" {
					tpDef := &javaactions.TypeParameterDef{
						BaseElement: model.BaseElement{ID: model.ID(extractBsonID(tpMap["$ID"]))},
						Name:        name,
					}
					ja.TypeParameters = append(ja.TypeParameters, tpDef)
				}
			}
		}
	}

	// Parse MicroflowActionInfo
	if mai := toMap(raw["MicroflowActionInfo"]); mai != nil {
		ja.MicroflowActionInfo = parseMicroflowActionInfo(mai)
	}

	// Resolve type parameter names for EntityTypeParameterType and ParameterizedEntityType parameters
	for _, param := range ja.Parameters {
		switch pt := param.ParameterType.(type) {
		case *javaactions.EntityTypeParameterType:
			pt.TypeParameterName = ja.FindTypeParameterName(pt.TypeParameterID)
		case *javaactions.TypeParameter:
			if pt.TypeParameterID != "" && pt.TypeParameter == "" {
				pt.TypeParameter = ja.FindTypeParameterName(pt.TypeParameterID)
			}
		}
	}

	// Resolve type parameter name for return type if it's a ParameterizedEntityType
	if tp, ok := ja.ReturnType.(*javaactions.TypeParameter); ok {
		if tp.TypeParameterID != "" && tp.TypeParameter == "" {
			tp.TypeParameter = ja.FindTypeParameterName(tp.TypeParameterID)
		}
	}

	return ja, nil
}

// parseCodeActionReturnType parses a Java action return type.
func parseCodeActionReturnType(raw map[string]any) javaactions.CodeActionReturnType {
	if raw == nil {
		return nil
	}

	typeName := extractString(raw["$Type"])
	switch typeName {
	case "CodeActions$VoidType":
		return &javaactions.VoidType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
	case "CodeActions$BooleanType":
		return &javaactions.BooleanType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
	case "CodeActions$IntegerType":
		return &javaactions.IntegerType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
	case "CodeActions$LongType":
		return &javaactions.LongType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
	case "CodeActions$DecimalType":
		return &javaactions.DecimalType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
	case "CodeActions$StringType":
		return &javaactions.StringType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
	case "CodeActions$DateTimeType":
		return &javaactions.DateTimeType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
	case "CodeActions$EntityType", "CodeActions$ConcreteEntityType":
		et := &javaactions.EntityType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
		et.Entity = extractString(raw["Entity"])
		return et
	case "CodeActions$ListType":
		lt := &javaactions.ListType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
		// ListType can have Entity directly or Parameter containing ConcreteEntityType
		if entity := extractString(raw["Entity"]); entity != "" {
			lt.Entity = entity
		} else if param := toMap(raw["Parameter"]); param != nil {
			lt.Entity = extractString(param["Entity"])
		}
		return lt
	case "CodeActions$FileDocumentType":
		return &javaactions.FileDocumentType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
	case "CodeActions$EnumerationType":
		et := &javaactions.EnumerationType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
		et.Enumeration = extractString(raw["Enumeration"])
		return et
	case "CodeActions$TypeParameter":
		tp := &javaactions.TypeParameter{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
		tp.TypeParameter = extractString(raw["TypeParameter"])
		return tp
	case "CodeActions$ParameterizedEntityType":
		// Return type referencing a type parameter (e.g., returns the entity passed as type param)
		tp := &javaactions.TypeParameter{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
		// ParameterizedEntityType stores the type parameter as a binary ID pointer
		id := extractBsonID(raw["TypeParameterPointer"])
		if id == "" {
			id = extractBsonID(raw["TypeParameter"])
		}
		tp.TypeParameterID = model.ID(id)
		return tp
	}

	// Unknown type - return nil
	return nil
}

// parseJavaActionParameter parses a Java action parameter.
func parseJavaActionParameter(raw map[string]any) *javaactions.JavaActionParameter {
	if raw == nil {
		return nil
	}

	// Skip array markers (items without $ID)
	if raw["$ID"] == nil {
		return nil
	}

	param := &javaactions.JavaActionParameter{}
	param.ID = model.ID(extractBsonID(raw["$ID"]))
	param.TypeName = extractString(raw["$Type"])
	param.Name = extractString(raw["Name"])
	param.Description = extractString(raw["Description"])
	param.Category = extractString(raw["Category"])
	param.IsRequired = extractBool(raw["IsRequired"], false)

	// Parse parameter type - handle both map and primitive.D
	switch pt := raw["ParameterType"].(type) {
	case map[string]any:
		param.ParameterType = parseCodeActionParameterType(pt)
	case primitive.D:
		param.ParameterType = parseCodeActionParameterType(primitiveToMap(pt))
	}

	return param
}

// parseCodeActionParameterType parses a Java action parameter type.
func parseCodeActionParameterType(raw map[string]any) javaactions.CodeActionParameterType {
	if raw == nil {
		return nil
	}

	typeName := extractString(raw["$Type"])
	switch typeName {
	case "CodeActions$BasicParameterType":
		// BasicParameterType wraps the actual type in a "Type" property
		innerType := toMap(raw["Type"])
		if innerType != nil {
			return parseInnerParameterType(innerType)
		}
		return nil
	case "CodeActions$BooleanType":
		return &javaactions.BooleanType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
	case "CodeActions$IntegerType":
		return &javaactions.IntegerType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
	case "CodeActions$LongType":
		return &javaactions.LongType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
	case "CodeActions$DecimalType":
		return &javaactions.DecimalType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
	case "CodeActions$StringType":
		return &javaactions.StringType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
	case "CodeActions$DateTimeType":
		return &javaactions.DateTimeType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
	case "CodeActions$EntityType", "CodeActions$ConcreteEntityType":
		et := &javaactions.EntityType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
		et.Entity = extractString(raw["Entity"])
		return et
	case "CodeActions$ListType":
		lt := &javaactions.ListType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
		// ListType can have Entity directly or Parameter containing ConcreteEntityType
		if entity := extractString(raw["Entity"]); entity != "" {
			lt.Entity = entity
		} else if param := toMap(raw["Parameter"]); param != nil {
			lt.Entity = extractString(param["Entity"])
		}
		return lt
	case "CodeActions$StringTemplateParameterType":
		st := &javaactions.StringTemplateParameterType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
		st.Grammar = extractString(raw["Grammar"])
		return st
	case "CodeActions$FileDocumentType":
		return &javaactions.FileDocumentType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
	case "CodeActions$EnumerationType":
		et := &javaactions.EnumerationType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
		et.Enumeration = extractString(raw["Enumeration"])
		return et
	case "CodeActions$MicroflowType", "JavaActions$MicroflowJavaActionParameterType":
		return &javaactions.MicroflowType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
	case "CodeActions$TypeParameter":
		tp := &javaactions.TypeParameter{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
		tp.TypeParameter = extractString(raw["TypeParameter"])
		return tp
	case "CodeActions$EntityTypeParameterType":
		etpt := &javaactions.EntityTypeParameterType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
		// Studio Pro uses "TypeParameterPointer"; fall back to "TypeParameter" for backward compat
		id := extractBsonID(raw["TypeParameterPointer"])
		if id == "" {
			id = extractBsonID(raw["TypeParameter"])
		}
		etpt.TypeParameterID = model.ID(id)
		return etpt
	case "JavaScriptActions$NanoflowJavaScriptActionParameterType":
		return &javaactions.NanoflowType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
	}

	// Unknown type - return nil
	return nil
}

// parseInnerParameterType parses the inner type from BasicParameterType.
func parseInnerParameterType(raw map[string]any) javaactions.CodeActionParameterType {
	if raw == nil {
		return nil
	}

	typeName := extractString(raw["$Type"])
	switch typeName {
	case "CodeActions$BooleanType":
		return &javaactions.BooleanType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
	case "CodeActions$IntegerType":
		return &javaactions.IntegerType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
	case "CodeActions$DecimalType":
		return &javaactions.DecimalType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
	case "CodeActions$StringType":
		return &javaactions.StringType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
	case "CodeActions$DateTimeType":
		return &javaactions.DateTimeType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
	case "CodeActions$MicroflowType", "JavaActions$MicroflowJavaActionParameterType":
		return &javaactions.MicroflowType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
	case "CodeActions$ConcreteEntityType", "CodeActions$EntityType":
		et := &javaactions.EntityType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
		et.Entity = extractString(raw["Entity"])
		return et
	case "CodeActions$ListType":
		lt := &javaactions.ListType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
		// ListType contains Parameter with ConcreteEntityType
		if param := toMap(raw["Parameter"]); param != nil {
			lt.Entity = extractString(param["Entity"])
		} else if entity := extractString(raw["Entity"]); entity != "" {
			lt.Entity = entity
		}
		return lt
	case "CodeActions$EntityTypeParameterType":
		etpt := &javaactions.EntityTypeParameterType{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
		// Studio Pro uses "TypeParameterPointer"; fall back to "TypeParameter" for backward compat
		id := extractBsonID(raw["TypeParameterPointer"])
		if id == "" {
			id = extractBsonID(raw["TypeParameter"])
		}
		etpt.TypeParameterID = model.ID(id)
		return etpt
	case "CodeActions$ParameterizedEntityType":
		tp := &javaactions.TypeParameter{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(raw["$ID"]))},
		}
		// ParameterizedEntityType stores the type parameter as a binary ID pointer
		id := extractBsonID(raw["TypeParameterPointer"])
		if id == "" {
			id = extractBsonID(raw["TypeParameter"])
		}
		tp.TypeParameterID = model.ID(id)
		return tp
	}

	return nil
}

// ListJavaActionsFull returns all Java actions with full details, including virtual System module actions.
func (r *Reader) ListJavaActionsFull() ([]*javaactions.JavaAction, error) {
	units, err := r.listUnitsByType("JavaActions$JavaAction")
	if err != nil {
		return nil, err
	}

	var result []*javaactions.JavaAction
	for _, u := range units {
		ja, err := r.parseJavaActionFull(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse java action %s: %w", u.ID, err)
		}
		result = append(result, ja)
	}

	// Append virtual System module Java actions (not stored in the MPR database)
	result = append(result, BuildSystemJavaActionsFull()...)

	return result, nil
}

// toMap converts various BSON types to map[string]interface{}.
func toMap(v any) map[string]any {
	if v == nil {
		return nil
	}
	switch m := v.(type) {
	case map[string]any:
		return m
	case primitive.D:
		return primitiveToMap(m)
	default:
		return nil
	}
}

// primitiveToMap converts primitive.D to map[string]interface{}.
func primitiveToMap(d primitive.D) map[string]any {
	result := make(map[string]any)
	for _, e := range d {
		result[e.Key] = e.Value
	}
	return result
}

// extractBinary returns the bytes of a BSON binary value, or nil when the field
// is absent, BSON null, or some other type. This tolerates the legacy
// MicroflowActionInfo shape (null/absent ImageData) on read so already-corrupted
// units can still be loaded and repaired. See issue #656.
func extractBinary(v any) []byte {
	if b, ok := v.(primitive.Binary); ok {
		return b.Data
	}
	return nil
}

// parseMicroflowActionInfo reads a MicroflowActionInfo sub-document. It accepts
// both the current CodeActions$ binary shape and the legacy JavaActions$ shape
// (an `Icon` string and a null/absent `ImageData`), reading the four icon/image
// bitmaps as binaries and silently dropping the obsolete `Icon` key. Shared by
// the Java- and JavaScript-action parsers. See issue #656.
func parseMicroflowActionInfo(mai map[string]any) *javaactions.MicroflowActionInfo {
	return &javaactions.MicroflowActionInfo{
		BaseElement:   model.BaseElement{ID: model.ID(extractBsonID(mai["$ID"]))},
		Caption:       extractString(mai["Caption"]),
		Category:      extractString(mai["Category"]),
		IconData:      extractBinary(mai["IconData"]),
		IconDataDark:  extractBinary(mai["IconDataDark"]),
		ImageData:     extractBinary(mai["ImageData"]),
		ImageDataDark: extractBinary(mai["ImageDataDark"]),
	}
}
