// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
	"github.com/JordtenBulte-OLC/mxcli/sdk/javaactions"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// A JavaScript action is structurally identical to a Java action (same
// JavaReturnType/Parameters/TypeParameters/MicroflowActionInfo storage keys),
// differing only in the document and parameter $Type names plus a Platform
// field. The gen JavaScriptAction binds the wrong storage keys
// (ActionReturnType/ActionParameters/ModelerActionInfo) and lacks Platform, so
// rather than use it we encode the action through the working Java gen path and
// rewrite the two JavaActions$ $Type names to their JavaScriptActions$
// equivalents, then inject Platform and the MicroflowActionInfo (#656 shape).

// CreateJavaScriptAction inserts a new JavaScriptActions$JavaScriptAction unit.
func (b *Backend) CreateJavaScriptAction(jsa *types.JavaScriptAction) error {
	if jsa == nil {
		return fmt.Errorf("CreateJavaScriptAction: nil javascript action")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateJavaScriptAction: not connected for writing")
	}
	if jsa.ID == "" {
		jsa.ID = model.ID(mmpr.GenerateID())
	}
	contents, err := encodeJavaScriptAction(jsa)
	if err != nil {
		return err
	}
	if err := b.writer.InsertUnit(string(jsa.ID), string(jsa.ContainerID), "Documents", "JavaScriptActions$JavaScriptAction", contents); err != nil {
		return fmt.Errorf("CreateJavaScriptAction: insert: %w", err)
	}
	return nil
}

// UpdateJavaScriptAction rewrites an existing JavaScript action unit.
func (b *Backend) UpdateJavaScriptAction(jsa *types.JavaScriptAction) error {
	if jsa == nil {
		return fmt.Errorf("UpdateJavaScriptAction: nil javascript action")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateJavaScriptAction: not connected for writing")
	}
	contents, err := encodeJavaScriptAction(jsa)
	if err != nil {
		return err
	}
	if err := b.writer.UpdateRawUnit(string(jsa.ID), contents); err != nil {
		return fmt.Errorf("UpdateJavaScriptAction: update: %w", err)
	}
	return nil
}

// DeleteJavaScriptAction removes a JavaScript action unit.
func (b *Backend) DeleteJavaScriptAction(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteJavaScriptAction: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

// encodeJavaScriptAction encodes a JS action by reusing the Java gen path and
// rewriting it into the JavaScriptActions$ shape.
func encodeJavaScriptAction(jsa *types.JavaScriptAction) ([]byte, error) {
	g := javaActionToGen(jsActionAsJavaAction(jsa))
	g.SetID(element.ID(jsa.ID))
	contents, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		return nil, fmt.Errorf("encodeJavaScriptAction: encode: %w", err)
	}

	var doc bson.D
	if err := bson.Unmarshal(contents, &doc); err != nil {
		return nil, fmt.Errorf("encodeJavaScriptAction: unmarshal: %w", err)
	}
	rewriteJavaToJavaScriptTypes(doc)

	platform := jsa.Platform
	if platform == "" {
		platform = "Web"
	}
	doc = codec.SetBSONDocField(doc, "Platform", platform)

	if jsa.MicroflowActionInfo != nil {
		doc = codec.SetBSONDocField(doc, "MicroflowActionInfo", microflowActionInfoBSON(jsa.MicroflowActionInfo))
	}

	return bson.Marshal(doc)
}

// jsActionAsJavaAction copies a JS action's shared fields onto a JavaAction so
// the Java gen encoder can serialize it. Platform/$Type are handled separately.
func jsActionAsJavaAction(jsa *types.JavaScriptAction) *javaactions.JavaAction {
	return &javaactions.JavaAction{
		BaseElement:             model.BaseElement{ID: jsa.ID},
		ContainerID:             jsa.ContainerID,
		Name:                    jsa.Name,
		Documentation:           jsa.Documentation,
		Excluded:                jsa.Excluded,
		ExportLevel:             jsa.ExportLevel,
		ActionDefaultReturnName: jsa.ActionDefaultReturnName,
		ReturnType:              jsa.ReturnType,
		Parameters:              jsa.Parameters,
		TypeParameters:          jsa.TypeParameters,
		// MicroflowActionInfo is injected post-encode (binary-safe), like java_write.
	}
}

// rewriteJavaToJavaScriptTypes recursively rewrites the document and parameter
// $Type names from their JavaActions$ form to the JavaScriptActions$ form. Inner
// CodeActions$ types (parameter/return types, MicroflowActionInfo) are untouched.
func rewriteJavaToJavaScriptTypes(v any) {
	switch t := v.(type) {
	case bson.D:
		for i := range t {
			if t[i].Key == "$Type" {
				if s, ok := t[i].Value.(string); ok {
					switch s {
					case "JavaActions$JavaAction":
						t[i].Value = "JavaScriptActions$JavaScriptAction"
					case "JavaActions$JavaActionParameter":
						t[i].Value = "JavaScriptActions$JavaScriptActionParameter"
					}
				}
				continue
			}
			rewriteJavaToJavaScriptTypes(t[i].Value)
		}
	case bson.A:
		for i := range t {
			rewriteJavaToJavaScriptTypes(t[i])
		}
	}
}

// jsActionSourceDir returns javascriptsource/<module>/actions using the original
// module-name casing Studio Pro writes.
func (b *Backend) jsActionSourceDir(moduleName string) string {
	return filepath.Join(filepath.Dir(b.path), "javascriptsource", moduleName, "actions")
}

// WriteJavaScriptSourceFile writes javascriptsource/<module>/actions/<action>.js.
func (b *Backend) WriteJavaScriptSourceFile(moduleName, actionName string, jsCode string, params []*types.JavaActionParameter, returnType types.CodeActionReturnType) error {
	if b.path == "" {
		return fmt.Errorf("WriteJavaScriptSourceFile: no project path")
	}
	dir := b.jsActionSourceDir(moduleName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("WriteJavaScriptSourceFile: create dir: %w", err)
	}
	source := javaactions.GenerateJavaScriptSource(actionName, jsCode, params, returnType)
	if err := os.WriteFile(filepath.Join(dir, actionName+".js"), []byte(source), 0o644); err != nil {
		return fmt.Errorf("WriteJavaScriptSourceFile: write: %w", err)
	}
	return nil
}

// DeleteJavaScriptSourceFile removes the .js file for a dropped JS action.
func (b *Backend) DeleteJavaScriptSourceFile(moduleName, actionName string) error {
	if b.path == "" {
		return fmt.Errorf("DeleteJavaScriptSourceFile: no project path")
	}
	filePath := filepath.Join(b.jsActionSourceDir(moduleName), actionName+".js")
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("DeleteJavaScriptSourceFile: %w", err)
	}
	return nil
}

// RenameJavaScriptSourceFile renames the .js file when a JS action is renamed.
func (b *Backend) RenameJavaScriptSourceFile(moduleName, oldName, newName string) error {
	if b.path == "" {
		return fmt.Errorf("RenameJavaScriptSourceFile: no project path")
	}
	dir := b.jsActionSourceDir(moduleName)
	if err := os.Rename(filepath.Join(dir, oldName+".js"), filepath.Join(dir, newName+".js")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("RenameJavaScriptSourceFile: %w", err)
	}
	return nil
}
