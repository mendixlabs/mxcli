// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
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

// jsActionSourceDir returns javascriptsource/<module>/actions with the module
// name LOWERCASED, which is where Mendix looks: a blank Mendix 11 app ships
// javascriptsource/nanoflowcommons/, /datawidgets/ and /webactions/ for modules
// named NanoflowCommons, DataWidgets and WebActions.
//
// Writing the original casing instead is silent and total: mxbuild finds no
// source at the path it reads, generates a stub whose body throws
// "JavaScript action was not implemented", and bundles that. The action parses,
// passes `mxcli check` and builds cleanly, then throws when a user clicks it.
// Only reproduces on a case-sensitive filesystem — on macOS and Windows the two
// spellings are the same directory, which is why it went unnoticed.
func (b *Backend) jsActionSourceDir(moduleName string) string {
	return filepath.Join(filepath.Dir(b.path), "javascriptsource", strings.ToLower(moduleName), "actions")
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
	path := filepath.Join(dir, actionName+".js")
	// Regenerate the way Studio Pro does: the existing file's import list and
	// EXTRA CODE section are retained, since MDL cannot express either.
	// Keeping only the user code deleted hand-restored helpers the user code
	// calls (mendixlabs/mxcli#1429; the Java twin is ako/mxcli#705).
	existing, _ := os.ReadFile(path)
	imports, extraCode := javaactions.JavaScriptRetainedSections(string(existing))
	// The file as mxbuild writes it, the action's documentation and its
	// enumerations' values included, so a build leaves it alone and a re-run
	// writes nothing (ako/mxcli#2357). The action's unit is written before its
	// file, so its documentation is read back from it.
	doc := ""
	if b.reader != nil {
		if a, err := b.ReadJavaScriptActionByName(moduleName + "." + actionName); err == nil && a != nil {
			doc = a.Documentation
		}
	}
	source := javaactions.JSSource{
		Action: actionName, Documentation: doc, UserCode: jsCode, Params: params, ReturnType: returnType,
		Imports: imports, ExtraCode: extraCode, EnumValues: b.enumerationValueNames,
	}.Generate()
	changed, err := javaactions.WriteSourceIfChanged(path, source)
	b.noteFileWrite(changed)
	if err != nil {
		return fmt.Errorf("WriteJavaScriptSourceFile: write: %w", err)
	}
	if changed {
		b.noteSourceFile(path, "", false, javaScriptActionBody)
	}
	return nil
}

// DeleteJavaScriptSourceFile removes the .js file for a dropped JS action.
func (b *Backend) DeleteJavaScriptSourceFile(moduleName, actionName string) error {
	if b.path == "" {
		return fmt.Errorf("DeleteJavaScriptSourceFile: no project path")
	}
	filePath := filepath.Join(b.jsActionSourceDir(moduleName), actionName+".js")
	switch err := os.Remove(filePath); {
	case err == nil:
		b.noteSourceFile(filePath, "", true, javaScriptActionBody)
	case !os.IsNotExist(err):
		return fmt.Errorf("DeleteJavaScriptSourceFile: %w", err)
	}
	return nil
}

// MoveJavaScriptSourceFile moves javascriptsource/<from>/actions/<action>.js into
// <to>'s actions directory when the action changes module, for the reason
// MoveJavaSourceFile gives: mxbuild looks for the source under the action's
// module and bundles a stub that throws when it finds none. The source names no
// module, so it moves as it is.
func (b *Backend) MoveJavaScriptSourceFile(fromModule, toModule, actionName string) (string, error) {
	if b.path == "" {
		return "", fmt.Errorf("MoveJavaScriptSourceFile: no project path")
	}
	moved, _, err := b.moveSourceFile(b.jsActionSourceDir(fromModule), b.jsActionSourceDir(toModule), actionName+".js", javaScriptActionBody)
	return moved, err
}

// RenameJavaScriptSourceFile renames the .js file when a JS action is renamed.
func (b *Backend) RenameJavaScriptSourceFile(moduleName, oldName, newName string) error {
	if b.path == "" {
		return fmt.Errorf("RenameJavaScriptSourceFile: no project path")
	}
	dir := b.jsActionSourceDir(moduleName)
	oldPath, newPath := filepath.Join(dir, oldName+".js"), filepath.Join(dir, newName+".js")
	switch err := os.Rename(oldPath, newPath); {
	case err == nil:
		b.noteSourceFile(newPath, oldPath, false, javaScriptActionBody)
	case !os.IsNotExist(err):
		return fmt.Errorf("RenameJavaScriptSourceFile: %w", err)
	}
	return nil
}

// enumerationValueNames is the value names of the enumeration qualifiedName,
// in order, or nil when it cannot be read.
func (b *Backend) enumerationValueNames(qualifiedName string) []string {
	if b.reader == nil {
		return nil
	}
	info, err := b.reader.GetRawUnitByName("enumeration", qualifiedName)
	if err != nil || info == nil {
		return nil
	}
	var doc bson.D
	if bson.Unmarshal(info.Contents, &doc) != nil {
		return nil
	}
	var names []string
	for _, e := range doc {
		if e.Key != "Values" {
			continue
		}
		list, _ := e.Value.(bson.A)
		for _, v := range list {
			vd, ok := v.(bson.D)
			if !ok {
				continue
			}
			for _, f := range vd {
				if f.Key == "Name" {
					if n, ok := f.Value.(string); ok {
						names = append(names, n)
					}
				}
			}
		}
	}
	return names
}
