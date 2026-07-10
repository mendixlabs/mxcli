// SPDX-License-Identifier: Apache-2.0

// Package mpr - Java action writer support.
package mpr

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/javaactions"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// emptyBinary is the BSON subtype-0 binary Studio Pro writes for an unset
// toolbox bitmap. It must never be BSON null: the MicroflowActionInfo *Data
// properties are mandatory binaries and a null crashes Studio Pro's UnitWriter
// on re-serialize (issue #656).
func bsonBinary(b []byte) primitive.Binary {
	if b == nil {
		b = []byte{}
	}
	return primitive.Binary{Subtype: 0x00, Data: b}
}

// microflowActionInfoBSON serializes a MicroflowActionInfo in the current
// metamodel shape: $Type CodeActions$MicroflowActionInfo, with all four icon/
// image bitmaps always present as (possibly empty) binaries and never null.
// The legacy JavaActions$ alias and the removed `Icon` key are not emitted.
func microflowActionInfoBSON(mai *javaactions.MicroflowActionInfo) bson.D {
	maiID := string(mai.ID)
	if maiID == "" {
		maiID = generateUUID()
	}
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(maiID)},
		{Key: "$Type", Value: "CodeActions$MicroflowActionInfo"},
		{Key: "Caption", Value: mai.Caption},
		{Key: "Category", Value: mai.Category},
		{Key: "IconData", Value: bsonBinary(mai.IconData)},
		{Key: "IconDataDark", Value: bsonBinary(mai.IconDataDark)},
		{Key: "ImageData", Value: bsonBinary(mai.ImageData)},
		{Key: "ImageDataDark", Value: bsonBinary(mai.ImageDataDark)},
	}
}

// CreateJavaAction creates a new Java action in the MPR.
func (w *Writer) CreateJavaAction(ja *javaactions.JavaAction) error {
	if ja.ID == "" {
		ja.ID = model.ID(generateUUID())
	}
	ja.TypeName = "JavaActions$JavaAction"

	contents, err := w.serializeJavaAction(ja)
	if err != nil {
		return fmt.Errorf("failed to serialize java action: %w", err)
	}

	return w.insertUnit(string(ja.ID), string(ja.ContainerID), "Documents", "JavaActions$JavaAction", contents)
}

// UpdateJavaAction updates an existing Java action.
func (w *Writer) UpdateJavaAction(ja *javaactions.JavaAction) error {
	contents, err := w.serializeJavaAction(ja)
	if err != nil {
		return fmt.Errorf("failed to serialize java action: %w", err)
	}

	return w.updateUnit(string(ja.ID), contents)
}

// DeleteJavaAction deletes a Java action by ID.
func (w *Writer) DeleteJavaAction(id model.ID) error {
	return w.deleteUnit(string(id))
}

// WriteJavaSourceFile writes the Java source file to the javasource directory.
// moduleName is the lowercase module name (e.g., "mymodule")
// actionName is the action name (e.g., "ValidateEmail")
// javaCode is the executeAction() body code
// params is the list of parameters with their types
// returnType is the return type (can be nil for void)
func (w *Writer) WriteJavaSourceFile(moduleName, actionName string, javaCode string, params []*javaactions.JavaActionParameter, returnType javaactions.CodeActionReturnType, extraImports []string, extraCode string) error {
	// Get project root directory (parent of .mpr file)
	projectRoot := filepath.Dir(w.reader.path)

	// Build the javasource path
	moduleNameLower := strings.ToLower(moduleName)
	javaDir := filepath.Join(projectRoot, "javasource", moduleNameLower, "actions")

	// Create directory if it doesn't exist
	if err := os.MkdirAll(javaDir, 0755); err != nil {
		return fmt.Errorf("failed to create javasource directory: %w", err)
	}

	// Generate Java source (shared with the modelsdk engine)
	source := javaactions.GenerateSource(moduleName, actionName, javaCode, params, returnType, extraImports, extraCode)

	// Write the file
	filePath := filepath.Join(javaDir, actionName+".java")
	if err := os.WriteFile(filePath, []byte(source), 0644); err != nil {
		return fmt.Errorf("failed to write Java source file: %w", err)
	}

	return nil
}

// DeleteJavaSourceFile removes the Java source file for a dropped Java action.
func (w *Writer) DeleteJavaSourceFile(moduleName, actionName string) error {
	projectRoot := filepath.Dir(w.reader.path)
	filePath := filepath.Join(projectRoot, "javasource", strings.ToLower(moduleName), "actions", actionName+".java")
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete Java source file: %w", err)
	}
	return nil
}

// RenameJavaSourceFile renames the Java source file when a Java action is renamed.
func (w *Writer) RenameJavaSourceFile(moduleName, oldName, newName string) error {
	projectRoot := filepath.Dir(w.reader.path)
	dir := filepath.Join(projectRoot, "javasource", strings.ToLower(moduleName), "actions")
	oldPath := filepath.Join(dir, oldName+".java")
	newPath := filepath.Join(dir, newName+".java")
	if err := os.Rename(oldPath, newPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to rename Java source file: %w", err)
	}
	return nil
}

// ReadJavaSourceFile reads the Java source file for a Java action.
func (w *Writer) ReadJavaSourceFile(moduleName, actionName string) (string, error) {
	projectRoot := filepath.Dir(w.reader.path)
	moduleNameLower := strings.ToLower(moduleName)
	filePath := filepath.Join(projectRoot, "javasource", moduleNameLower, "actions", actionName+".java")

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read Java source file: %w", err)
	}

	return string(content), nil
}

// serializeJavaAction serializes a Java action to BSON.
func (w *Writer) serializeJavaAction(ja *javaactions.JavaAction) ([]byte, error) {
	// Build parameters array (storageListType: 2)
	params := bson.A{int32(2)} // Array type marker for storageListType: 2
	for _, param := range ja.Parameters {
		paramDoc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(param.ID))},
			{Key: "$Type", Value: param.TypeName},
			{Key: "Category", Value: param.Category},
			{Key: "Description", Value: param.Description},
			{Key: "IsRequired", Value: param.IsRequired},
			{Key: "Name", Value: param.Name},
		}
		if param.ParameterType != nil {
			paramDoc = append(paramDoc, bson.E{Key: "ParameterType", Value: serializeParameterType(param.ParameterType)})
		}
		params = append(params, paramDoc)
	}

	// Build type parameters array (storageListType: 2)
	typeParams := bson.A{int32(2)} // Array type marker for storageListType: 2
	for _, tp := range ja.TypeParameters {
		tpID := string(tp.ID)
		if tpID == "" {
			tpID = generateUUID()
		}
		typeParams = append(typeParams, bson.D{
			{Key: "$ID", Value: idToBsonBinary(tpID)},
			{Key: "$Type", Value: "CodeActions$TypeParameter"},
			{Key: "Name", Value: tp.Name},
		})
	}

	// Build MicroflowActionInfo
	var maiValue any
	if ja.MicroflowActionInfo != nil {
		maiValue = microflowActionInfoBSON(ja.MicroflowActionInfo)
	}

	// Build main document
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(ja.ID))},
		{Key: "$Type", Value: "JavaActions$JavaAction"},
		{Key: "ActionDefaultReturnName", Value: stringOrDefault(ja.ActionDefaultReturnName, "")},
		{Key: "Documentation", Value: ja.Documentation},
		{Key: "Excluded", Value: ja.Excluded},
		{Key: "ExportLevel", Value: stringOrDefault(ja.ExportLevel, "Hidden")},
		{Key: "MicroflowActionInfo", Value: maiValue},
		{Key: "Name", Value: ja.Name},
		{Key: "Parameters", Value: params},
		{Key: "TypeParameters", Value: typeParams},
	}

	// Add return type
	if ja.ReturnType != nil {
		doc = append(doc, bson.E{Key: "JavaReturnType", Value: serializeReturnType(ja.ReturnType)})
	} else {
		// Default to void type
		doc = append(doc, bson.E{Key: "JavaReturnType", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "CodeActions$VoidType"},
		}})
	}

	return marshalUnitIDFirst(doc)
}

// serializeReturnType serializes a CodeActionReturnType to BSON.
func serializeReturnType(t javaactions.CodeActionReturnType) bson.D {
	if t == nil {
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "CodeActions$VoidType"},
		}
	}

	switch v := t.(type) {
	case *javaactions.VoidType:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(v.ID))},
			{Key: "$Type", Value: "CodeActions$VoidType"},
		}
	case *javaactions.BooleanType:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(v.ID))},
			{Key: "$Type", Value: "CodeActions$BooleanType"},
		}
	case *javaactions.IntegerType:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(v.ID))},
			{Key: "$Type", Value: "CodeActions$IntegerType"},
		}
	case *javaactions.LongType:
		// Mendix uses IntegerType for 64-bit integers (Long in Java)
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(v.ID))},
			{Key: "$Type", Value: "CodeActions$IntegerType"},
		}
	case *javaactions.DecimalType:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(v.ID))},
			{Key: "$Type", Value: "CodeActions$DecimalType"},
		}
	case *javaactions.StringType:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(v.ID))},
			{Key: "$Type", Value: "CodeActions$StringType"},
		}
	case *javaactions.DateTimeType:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(v.ID))},
			{Key: "$Type", Value: "CodeActions$DateTimeType"},
		}
	case *javaactions.EnumerationType:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(v.ID))},
			{Key: "$Type", Value: "CodeActions$EnumerationType"},
			{Key: "Enumeration", Value: v.Enumeration},
		}
	case *javaactions.EntityType:
		// Use ConcreteEntityType for return types
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(v.ID))},
			{Key: "$Type", Value: "CodeActions$ConcreteEntityType"},
			{Key: "Entity", Value: v.Entity},
		}
	case *javaactions.ListType:
		// ListType contains a Parameter which is a ConcreteEntityType
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(v.ID))},
			{Key: "$Type", Value: "CodeActions$ListType"},
			{Key: "Parameter", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "CodeActions$ConcreteEntityType"},
				{Key: "Entity", Value: v.Entity},
			}},
		}
	case *javaactions.TypeParameter:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(v.ID))},
			{Key: "$Type", Value: "CodeActions$ParameterizedEntityType"},
			{Key: "TypeParameterPointer", Value: idToBsonBinary(string(v.TypeParameterID))},
		}
	default:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "CodeActions$VoidType"},
		}
	}
}

// serializeParameterType serializes a CodeActionParameterType to BSON.
// Parameter types are wrapped in BasicParameterType with a nested Type property.
func serializeParameterType(t javaactions.CodeActionParameterType) bson.D {
	if t == nil {
		// Default to String type wrapped in BasicParameterType
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "CodeActions$BasicParameterType"},
			{Key: "Type", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "CodeActions$StringType"},
			}},
		}
	}

	// Special case for StringTemplateParameterType - not wrapped in BasicParameterType
	if v, ok := t.(*javaactions.StringTemplateParameterType); ok {
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(v.ID))},
			{Key: "$Type", Value: "CodeActions$StringTemplateParameterType"},
			{Key: "Grammar", Value: v.Grammar},
		}
	}

	// Special case for EntityTypeParameterType - not wrapped in BasicParameterType
	if v, ok := t.(*javaactions.EntityTypeParameterType); ok {
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(v.ID))},
			{Key: "$Type", Value: "CodeActions$EntityTypeParameterType"},
			{Key: "TypeParameterPointer", Value: idToBsonBinary(string(v.TypeParameterID))},
		}
	}

	// Special case for TypeParameter (ParameterizedEntityType) - wrapped in BasicParameterType
	if v, ok := t.(*javaactions.TypeParameter); ok {
		innerType := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(v.ID))},
			{Key: "$Type", Value: "CodeActions$ParameterizedEntityType"},
			{Key: "TypeParameterPointer", Value: idToBsonBinary(string(v.TypeParameterID))},
		}
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "CodeActions$BasicParameterType"},
			{Key: "Type", Value: innerType},
		}
	}

	// All other types are wrapped in BasicParameterType
	innerType := serializeInnerType(t)
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "CodeActions$BasicParameterType"},
		{Key: "Type", Value: innerType},
	}
}

// serializeInnerType serializes the inner type for BasicParameterType.
func serializeInnerType(t javaactions.CodeActionParameterType) bson.D {
	switch v := t.(type) {
	case *javaactions.BooleanType:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(v.ID))},
			{Key: "$Type", Value: "CodeActions$BooleanType"},
		}
	case *javaactions.IntegerType:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(v.ID))},
			{Key: "$Type", Value: "CodeActions$IntegerType"},
		}
	case *javaactions.LongType:
		// Mendix uses IntegerType for 64-bit integers (Long in Java)
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(v.ID))},
			{Key: "$Type", Value: "CodeActions$IntegerType"},
		}
	case *javaactions.DecimalType:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(v.ID))},
			{Key: "$Type", Value: "CodeActions$DecimalType"},
		}
	case *javaactions.StringType:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(v.ID))},
			{Key: "$Type", Value: "CodeActions$StringType"},
		}
	case *javaactions.DateTimeType:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(v.ID))},
			{Key: "$Type", Value: "CodeActions$DateTimeType"},
		}
	case *javaactions.EnumerationType:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(v.ID))},
			{Key: "$Type", Value: "CodeActions$EnumerationType"},
			{Key: "Enumeration", Value: v.Enumeration},
		}
	case *javaactions.EntityType:
		// Use ConcreteEntityType for entity parameters
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(v.ID))},
			{Key: "$Type", Value: "CodeActions$ConcreteEntityType"},
			{Key: "Entity", Value: v.Entity},
		}
	case *javaactions.ListType:
		// ListType contains a Parameter which is a ConcreteEntityType
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(v.ID))},
			{Key: "$Type", Value: "CodeActions$ListType"},
			{Key: "Parameter", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "CodeActions$ConcreteEntityType"},
				{Key: "Entity", Value: v.Entity},
			}},
		}
	case *javaactions.TypeParameter:
		// ParameterizedEntityType - references a type parameter by ID
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(v.ID))},
			{Key: "$Type", Value: "CodeActions$ParameterizedEntityType"},
			{Key: "TypeParameterPointer", Value: idToBsonBinary(string(v.TypeParameterID))},
		}
	default:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "CodeActions$StringType"},
		}
	}
}
