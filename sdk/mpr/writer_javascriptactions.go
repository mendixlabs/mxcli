// SPDX-License-Identifier: Apache-2.0

// Package mpr - JavaScript action writer support.
package mpr

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/javaactions"
	"go.mongodb.org/mongo-driver/bson"
)

// CreateJavaScriptAction serializes and inserts a new JavaScript action unit.
func (w *Writer) CreateJavaScriptAction(jsa *JavaScriptAction) error {
	if jsa.ID == "" {
		jsa.ID = model.ID(generateUUID())
	}
	jsa.TypeName = "JavaScriptActions$JavaScriptAction"

	contents, err := w.serializeJavaScriptAction(jsa)
	if err != nil {
		return fmt.Errorf("failed to serialize javascript action: %w", err)
	}
	return w.insertUnit(string(jsa.ID), string(jsa.ContainerID), "Documents", "JavaScriptActions$JavaScriptAction", contents)
}

// UpdateJavaScriptAction rewrites an existing JavaScript action unit.
func (w *Writer) UpdateJavaScriptAction(jsa *JavaScriptAction) error {
	jsa.TypeName = "JavaScriptActions$JavaScriptAction"
	contents, err := w.serializeJavaScriptAction(jsa)
	if err != nil {
		return fmt.Errorf("failed to serialize javascript action: %w", err)
	}
	return w.updateUnit(string(jsa.ID), contents)
}

// DeleteJavaScriptAction removes a JavaScript action unit.
func (w *Writer) DeleteJavaScriptAction(id model.ID) error {
	return w.deleteUnit(string(id))
}

// serializeJavaScriptAction serializes a JavaScript action to BSON. The shape
// mirrors a Java action (shared parameter/return-type/MicroflowActionInfo
// serialization) with $Type JavaScriptActions$JavaScriptAction, JavaScript
// parameter $Types, and an added Platform field.
func (w *Writer) serializeJavaScriptAction(jsa *JavaScriptAction) ([]byte, error) {
	params := bson.A{int32(2)} // typed-array marker
	for _, param := range jsa.Parameters {
		paramType := param.TypeName
		if paramType == "" {
			paramType = "JavaScriptActions$JavaScriptActionParameter"
		}
		paramDoc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(param.ID))},
			{Key: "$Type", Value: paramType},
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

	typeParams := bson.A{int32(2)}
	for _, tp := range jsa.TypeParameters {
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

	var maiValue any
	if jsa.MicroflowActionInfo != nil {
		maiValue = microflowActionInfoBSON(jsa.MicroflowActionInfo)
	}

	var returnType bson.D
	if jsa.ReturnType != nil {
		returnType = serializeReturnType(jsa.ReturnType)
	} else {
		returnType = bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "CodeActions$VoidType"},
		}
	}

	platform := jsa.Platform
	if platform == "" {
		platform = "Web"
	}

	// Key order follows what Studio Pro writes (alphabetical).
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(jsa.ID))},
		{Key: "$Type", Value: "JavaScriptActions$JavaScriptAction"},
		{Key: "ActionDefaultReturnName", Value: stringOrDefault(jsa.ActionDefaultReturnName, "ReturnValueName")},
		{Key: "Documentation", Value: jsa.Documentation},
		{Key: "Excluded", Value: jsa.Excluded},
		{Key: "ExportLevel", Value: stringOrDefault(jsa.ExportLevel, "Hidden")},
		{Key: "JavaReturnType", Value: returnType},
		{Key: "MicroflowActionInfo", Value: maiValue},
		{Key: "Name", Value: jsa.Name},
		{Key: "Parameters", Value: params},
		{Key: "Platform", Value: platform},
		{Key: "TypeParameters", Value: typeParams},
	}

	return marshalUnitIDFirst(doc)
}

// jsActionSourceDir returns javascriptsource/<module>/actions, using the original
// module-name casing Studio Pro writes (unlike javasource, which is lowercased).
func (w *Writer) jsActionSourceDir(moduleName string) string {
	return filepath.Join(filepath.Dir(w.reader.path), "javascriptsource", moduleName, "actions")
}

// WriteJavaScriptSourceFile writes javascriptsource/<module>/actions/<action>.js.
func (w *Writer) WriteJavaScriptSourceFile(moduleName, actionName string, jsCode string, params []*javaactions.JavaActionParameter, returnType javaactions.CodeActionReturnType) error {
	dir := w.jsActionSourceDir(moduleName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create javascriptsource directory: %w", err)
	}
	source := javaactions.GenerateJavaScriptSource(actionName, jsCode, params, returnType)
	if err := os.WriteFile(filepath.Join(dir, actionName+".js"), []byte(source), 0o644); err != nil {
		return fmt.Errorf("failed to write JavaScript source file: %w", err)
	}
	return nil
}

// DeleteJavaScriptSourceFile removes the .js file for a dropped JavaScript action.
func (w *Writer) DeleteJavaScriptSourceFile(moduleName, actionName string) error {
	filePath := filepath.Join(w.jsActionSourceDir(moduleName), actionName+".js")
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete JavaScript source file: %w", err)
	}
	return nil
}

// RenameJavaScriptSourceFile renames the .js file when a JavaScript action is renamed.
func (w *Writer) RenameJavaScriptSourceFile(moduleName, oldName, newName string) error {
	dir := w.jsActionSourceDir(moduleName)
	oldPath := filepath.Join(dir, oldName+".js")
	newPath := filepath.Join(dir, newName+".js")
	if err := os.Rename(oldPath, newPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to rename JavaScript source file: %w", err)
	}
	return nil
}
