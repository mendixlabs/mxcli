// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	genCa "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/codeactions"
	genJa "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/javaactions"
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
	"github.com/JordtenBulte-OLC/mxcli/sdk/javaactions"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// NOTE: the parameter/return TYPE elements (StringType, BasicParameterType,
// ConcreteEntityType, …) are emitted with the CodeActions$ storage prefix and so
// come from gen/codeactions (genCa). The gen/javaactions copies of those types
// emit a JavaActions$ prefix, which Studio Pro does not recognise. Only the
// JavaAction document and its JavaActionParameter come from gen/javaactions; the
// MicroflowActionInfo is post-patched as raw BSON (CodeActions$ shape — see
// patchMicroflowActionInfo / issue #656) rather than built from a gen element.

func init() {
	// A JavaAction always emits its Parameters and TypeParameters arrays (empty
	// arrays carry the marker 2), and a MicroflowActionInfo slot that is null when
	// the action isn't exposed as a microflow toolbox action — verified against
	// the legacy serializer.
	codec.RegisterTypeDefaults("JavaActions$JavaAction", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Parameters": 2, "TypeParameters": 2},
		NullFields:           []string{"MicroflowActionInfo"},
	})
	codec.RegisterListMarker("JavaActions$JavaActionParameter", 2)
	codec.RegisterListMarker("CodeActions$TypeParameter", 2)
}

// CreateJavaAction inserts a new JavaActions$JavaAction document unit (parameters,
// type parameters, return type, optional microflow-action info).
func (b *Backend) CreateJavaAction(ja *javaactions.JavaAction) error {
	if ja == nil {
		return fmt.Errorf("CreateJavaAction: nil java action")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateJavaAction: not connected for writing")
	}
	if ja.ID == "" {
		ja.ID = model.ID(mmpr.GenerateID())
	}
	g := javaActionToGen(ja)
	g.SetID(element.ID(ja.ID))
	contents, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		return fmt.Errorf("CreateJavaAction: encode: %w", err)
	}
	if contents, err = patchMicroflowActionInfo(contents, ja); err != nil {
		return fmt.Errorf("CreateJavaAction: patch action info: %w", err)
	}
	if err := b.writer.InsertUnit(string(ja.ID), string(ja.ContainerID), "Documents", "JavaActions$JavaAction", contents); err != nil {
		return fmt.Errorf("CreateJavaAction: insert: %w", err)
	}
	return nil
}

// UpdateJavaAction rewrites an existing java-action unit (CREATE OR REPLACE).
func (b *Backend) UpdateJavaAction(ja *javaactions.JavaAction) error {
	if ja == nil {
		return fmt.Errorf("UpdateJavaAction: nil java action")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateJavaAction: not connected for writing")
	}
	g := javaActionToGen(ja)
	g.SetID(element.ID(ja.ID))
	contents, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		return fmt.Errorf("UpdateJavaAction: encode: %w", err)
	}
	if contents, err = patchMicroflowActionInfo(contents, ja); err != nil {
		return fmt.Errorf("UpdateJavaAction: patch action info: %w", err)
	}
	if err := b.writer.UpdateRawUnit(string(ja.ID), contents); err != nil {
		return fmt.Errorf("UpdateJavaAction: update: %w", err)
	}
	return nil
}

// WriteJavaSourceFile writes the .java stub for a java action under
// javasource/<module>/actions/<Action>.java, using the shared generator so the
// output is byte-identical to the legacy engine.
func (b *Backend) WriteJavaSourceFile(moduleName, actionName string, javaCode string, params []*javaactions.JavaActionParameter, returnType javaactions.CodeActionReturnType, extraImports []string, extraCode string) error {
	if b.path == "" {
		return fmt.Errorf("WriteJavaSourceFile: no project path")
	}
	javaDir := filepath.Join(filepath.Dir(b.path), "javasource", strings.ToLower(moduleName), "actions")
	if err := os.MkdirAll(javaDir, 0o755); err != nil {
		return fmt.Errorf("WriteJavaSourceFile: create dir: %w", err)
	}
	source := javaactions.GenerateSource(moduleName, actionName, javaCode, params, returnType, extraImports, extraCode)
	if err := os.WriteFile(filepath.Join(javaDir, actionName+".java"), []byte(source), 0o644); err != nil {
		return fmt.Errorf("WriteJavaSourceFile: write: %w", err)
	}
	return nil
}

// DeleteJavaAction removes a JavaActions$JavaAction document unit. The generated
// .java source file (if any) is left in place, matching the legacy writer.
func (b *Backend) DeleteJavaAction(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteJavaAction: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

// RenameJavaSourceFile renames javasource/<module>/actions/<old>.java to <new>.java.
// A missing source file is not an error (the action may have no generated stub yet).
func (b *Backend) RenameJavaSourceFile(moduleName, oldName, newName string) error {
	if b.path == "" {
		return fmt.Errorf("RenameJavaSourceFile: no project path")
	}
	dir := filepath.Join(filepath.Dir(b.path), "javasource", strings.ToLower(moduleName), "actions")
	oldPath := filepath.Join(dir, oldName+".java")
	newPath := filepath.Join(dir, newName+".java")
	if err := os.Rename(oldPath, newPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("RenameJavaSourceFile: %w", err)
	}
	return nil
}

// DeleteJavaSourceFile removes javasource/<module>/actions/<action>.java. A missing
// file is not an error.
func (b *Backend) DeleteJavaSourceFile(moduleName, actionName string) error {
	if b.path == "" {
		return fmt.Errorf("DeleteJavaSourceFile: no project path")
	}
	filePath := filepath.Join(filepath.Dir(b.path), "javasource", strings.ToLower(moduleName), "actions", actionName+".java")
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("DeleteJavaSourceFile: %w", err)
	}
	return nil
}

// ReadJavaSourceFile reads javasource/<module>/actions/<action>.java.
func (b *Backend) ReadJavaSourceFile(moduleName, actionName string) (string, error) {
	if b.path == "" {
		return "", fmt.Errorf("ReadJavaSourceFile: no project path")
	}
	filePath := filepath.Join(filepath.Dir(b.path), "javasource", strings.ToLower(moduleName), "actions", actionName+".java")
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("ReadJavaSourceFile: %w", err)
	}
	return string(content), nil
}

// javaActionToGen converts a model JavaAction to its gen element.
func javaActionToGen(ja *javaactions.JavaAction) *genJa.JavaAction {
	out := genJa.NewJavaAction()
	out.SetName(ja.Name)
	out.SetDocumentation(ja.Documentation)
	out.SetExcluded(ja.Excluded)
	exportLevel := ja.ExportLevel
	if exportLevel == "" {
		exportLevel = "Hidden"
	}
	out.SetExportLevel(exportLevel)
	out.SetActionDefaultReturnName(ja.ActionDefaultReturnName)

	for _, p := range ja.Parameters {
		gp := genJa.NewJavaActionParameter()
		if p.ID != "" {
			gp.SetID(element.ID(p.ID))
		}
		assignID(gp)
		gp.SetName(p.Name)
		gp.SetDescription(p.Description)
		gp.SetCategory(p.Category)
		gp.SetIsRequired(p.IsRequired)
		gp.SetParameterType(codeActionParamTypeToGen(p.ParameterType))
		out.AddParameters(gp)
	}

	for _, tp := range ja.TypeParameters {
		gtp := genCa.NewTypeParameter()
		if tp.ID != "" {
			gtp.SetID(element.ID(tp.ID))
		}
		assignID(gtp)
		gtp.SetName(tp.Name)
		out.AddTypeParameters(gtp)
	}

	// MicroflowActionInfo is intentionally left null here and injected after
	// encoding (see patchMicroflowActionInfo). The gen MicroflowActionInfo models
	// its icon/image bitmaps as Primitive[string], which serialises to BSON
	// strings — but Studio Pro stores them as mandatory binaries and crashes on a
	// null or wrong-typed value (#656). Post-patching lets us emit the correct
	// CodeActions$ shape with empty binaries without diverging the generated code.

	out.SetJavaReturnType(codeActionReturnTypeToGen(ja.ReturnType))
	return out
}

// microflowActionInfoBSON builds a MicroflowActionInfo sub-document in the
// current metamodel shape: $Type CodeActions$MicroflowActionInfo with all four
// icon/image bitmaps always present as (possibly empty) binaries, never null,
// and no obsolete Icon key. Mirrors the legacy writer's shape (issue #656).
func microflowActionInfoBSON(mai *javaactions.MicroflowActionInfo) bson.D {
	id := string(mai.ID)
	if id == "" {
		id = mmpr.GenerateID()
	}
	bin := func(b []byte) bson.Binary {
		if b == nil {
			b = []byte{}
		}
		return bson.Binary{Subtype: 0x00, Data: b}
	}
	return bson.D{
		{Key: "$ID", Value: mmpr.IDToBsonBinary(id)},
		{Key: "$Type", Value: "CodeActions$MicroflowActionInfo"},
		{Key: "Caption", Value: mai.Caption},
		{Key: "Category", Value: mai.Category},
		{Key: "IconData", Value: bin(mai.IconData)},
		{Key: "IconDataDark", Value: bin(mai.IconDataDark)},
		{Key: "ImageData", Value: bin(mai.ImageData)},
		{Key: "ImageDataDark", Value: bin(mai.ImageDataDark)},
	}
}

// patchMicroflowActionInfo replaces the encoded unit's null MicroflowActionInfo
// slot with the correct sub-document when the action is exposed as a toolbox
// item; a no-op otherwise. See issue #656.
func patchMicroflowActionInfo(contents []byte, ja *javaactions.JavaAction) ([]byte, error) {
	if ja.MicroflowActionInfo == nil {
		return contents, nil
	}
	return codec.PatchBSONField(contents, "MicroflowActionInfo", microflowActionInfoBSON(ja.MicroflowActionInfo))
}

// codeActionParamTypeToGen converts a parameter type. Most types are wrapped in a
// BasicParameterType; StringTemplate and EntityTypeParameter are emitted directly
// (mirrors the legacy serializeParameterType).
func codeActionParamTypeToGen(t javaactions.CodeActionParameterType) element.Element {
	switch v := t.(type) {
	case nil:
		b := genCa.NewBasicParameterType()
		assignID(b)
		b.SetType(newPrimitiveCAType("StringType"))
		return b
	case *javaactions.StringTemplateParameterType:
		s := genCa.NewStringTemplateParameterType()
		if v.ID != "" {
			s.SetID(element.ID(v.ID))
		}
		assignID(s)
		s.SetGrammar(v.Grammar)
		return s
	case *javaactions.EntityTypeParameterType:
		e := genCa.NewEntityTypeParameterType()
		if v.ID != "" {
			e.SetID(element.ID(v.ID))
		}
		assignID(e)
		e.SetTypeParameterID(element.ID(v.TypeParameterID))
		return e
	default:
		b := genCa.NewBasicParameterType()
		assignID(b)
		b.SetType(codeActionInnerTypeToGen(t))
		return b
	}
}

// codeActionInnerTypeToGen converts the inner type carried by a BasicParameterType.
func codeActionInnerTypeToGen(t javaactions.CodeActionParameterType) element.Element {
	switch v := t.(type) {
	case *javaactions.EnumerationType:
		e := genCa.NewEnumerationType()
		assignID(e)
		e.SetEnumerationQualifiedName(v.Enumeration)
		return e
	case *javaactions.EntityType:
		e := genCa.NewConcreteEntityType()
		assignID(e)
		e.SetEntityQualifiedName(v.Entity)
		return e
	case *javaactions.ListType:
		l := genCa.NewListType()
		assignID(l)
		ce := genCa.NewConcreteEntityType()
		assignID(ce)
		ce.SetEntityQualifiedName(v.Entity)
		l.SetParameter(ce)
		return l
	case *javaactions.TypeParameter:
		p := genCa.NewParameterizedEntityType()
		assignID(p)
		p.SetTypeParameterID(element.ID(v.TypeParameterID))
		return p
	default:
		return newPrimitiveCAType(primitiveCATypeName(t))
	}
}

// codeActionReturnTypeToGen converts a return type (void when nil).
func codeActionReturnTypeToGen(t javaactions.CodeActionReturnType) element.Element {
	switch v := t.(type) {
	case nil:
		return newPrimitiveCAType("VoidType")
	case *javaactions.EnumerationType:
		e := genCa.NewEnumerationType()
		assignID(e)
		e.SetEnumerationQualifiedName(v.Enumeration)
		return e
	case *javaactions.EntityType:
		e := genCa.NewConcreteEntityType()
		assignID(e)
		e.SetEntityQualifiedName(v.Entity)
		return e
	case *javaactions.ListType:
		l := genCa.NewListType()
		assignID(l)
		ce := genCa.NewConcreteEntityType()
		assignID(ce)
		ce.SetEntityQualifiedName(v.Entity)
		l.SetParameter(ce)
		return l
	case *javaactions.TypeParameter:
		p := genCa.NewParameterizedEntityType()
		assignID(p)
		p.SetTypeParameterID(element.ID(v.TypeParameterID))
		return p
	default:
		return newPrimitiveCAType(primitiveCAReturnTypeName(t))
	}
}

// newPrimitiveCAType builds a bare CodeActions primitive type element by kind (the
// element carries only $ID + $Type).
func newPrimitiveCAType(kind string) element.Element {
	var e element.Element
	switch kind {
	case "BooleanType":
		e = genCa.NewBooleanType()
	case "IntegerType":
		e = genCa.NewIntegerType()
	case "DecimalType":
		e = genCa.NewDecimalType()
	case "FloatType":
		e = genCa.NewFloatType()
	case "DateTimeType":
		e = genCa.NewDateTimeType()
	case "VoidType":
		e = genCa.NewVoidType()
	default:
		e = genCa.NewStringType()
	}
	assignID(e)
	return e
}

// primitiveCATypeName maps a model parameter type to its gen primitive kind.
// Long maps to IntegerType (Mendix has no distinct 64-bit type), matching legacy.
func primitiveCATypeName(t javaactions.CodeActionParameterType) string {
	switch t.(type) {
	case *javaactions.BooleanType:
		return "BooleanType"
	case *javaactions.IntegerType, *javaactions.LongType:
		return "IntegerType"
	case *javaactions.DecimalType:
		return "DecimalType"
	case *javaactions.DateTimeType:
		return "DateTimeType"
	case *javaactions.StringType:
		return "StringType"
	default:
		return "StringType"
	}
}

// primitiveCAReturnTypeName maps a model return type to its gen primitive kind.
func primitiveCAReturnTypeName(t javaactions.CodeActionReturnType) string {
	switch t.(type) {
	case *javaactions.VoidType:
		return "VoidType"
	case *javaactions.BooleanType:
		return "BooleanType"
	case *javaactions.IntegerType, *javaactions.LongType:
		return "IntegerType"
	case *javaactions.DecimalType:
		return "DecimalType"
	case *javaactions.DateTimeType:
		return "DateTimeType"
	case *javaactions.StringType:
		return "StringType"
	default:
		return "VoidType"
	}
}
