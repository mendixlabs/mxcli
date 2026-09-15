// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genCa "github.com/mendixlabs/mxcli/modelsdk/gen/codeactions"
	genJa "github.com/mendixlabs/mxcli/modelsdk/gen/javaactions"
	"github.com/mendixlabs/mxcli/modelsdk/meta"
	"github.com/mendixlabs/mxcli/modelsdk/mprread"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// ReadJavaActionByName returns the fully-parsed Java action (parameters, types,
// return type, microflow-action info) for a qualified name. Used by DESCRIBE JAVA
// ACTION and by the microflow builder to resolve a java-action call's parameter
// types. Inverse of javaActionToGen.
func (b *Backend) ReadJavaActionByName(qualifiedName string) (*javaactions.JavaAction, error) {
	dot := strings.LastIndex(qualifiedName, ".")
	if dot < 0 {
		return nil, fmt.Errorf("invalid java action name: %s", qualifiedName)
	}
	moduleName, actionName := qualifiedName[:dot], qualifiedName[dot+1:]
	mod, err := b.GetModuleByName(moduleName)
	if err != nil || mod == nil {
		return nil, fmt.Errorf("java action not found: %s", qualifiedName)
	}
	containers := b.containerSetForModule(string(mod.ID)) // module + nested folders
	units, err := mprread.ListUnitsWithContainer[*genJa.JavaAction](b.reader)
	if err != nil {
		return nil, err
	}
	for _, u := range units {
		if containers[string(u.ContainerID)] && u.Element.Name() == actionName {
			return javaActionFromGen(u.Element, u.ContainerID), nil
		}
	}
	return nil, fmt.Errorf("java action not found: %s", qualifiedName)
}

// ListJavaActionsFull reads every Java action unit into the fully-parsed semantic
// form (parameters, types, return type) — what the catalog's java-actions builder
// and SHOW JAVA ACTIONS need. (The virtual System-module java actions that the
// legacy reader appends are not yet modelled in modelsdk/meta, so they're omitted;
// the catalog simply indexes fewer rows, it does not error.)
func (b *Backend) ListJavaActionsFull() ([]*javaactions.JavaAction, error) {
	units, err := mprread.ListUnitsWithContainer[*genJa.JavaAction](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*javaactions.JavaAction, 0, len(units))
	for _, u := range units {
		out = append(out, javaActionFromGen(u.Element, u.ContainerID))
	}
	// Same reason as ListJavaActions: the System module's are platform built-ins
	// with no stored unit, so they have to be synthesized or they vanish.
	out = append(out, meta.BuildSystemJavaActionsFull()...)
	return out, nil
}

// javaActionFromGen converts a gen JavaAction to the semantic type.
func javaActionFromGen(g *genJa.JavaAction, containerID model.ID) *javaactions.JavaAction {
	out := &javaactions.JavaAction{
		ContainerID:             containerID,
		Name:                    g.Name(),
		Documentation:           g.Documentation(),
		Excluded:                g.Excluded(),
		ExportLevel:             g.ExportLevel(),
		ActionDefaultReturnName: g.ActionDefaultReturnName(),
	}
	out.ID = model.ID(g.ID())
	for _, el := range g.ParametersItems() {
		p, ok := el.(*genJa.JavaActionParameter)
		if !ok {
			continue
		}
		jp := &javaactions.JavaActionParameter{
			Name:          p.Name(),
			Description:   p.Description(),
			Category:      p.Category(),
			IsRequired:    p.IsRequired(),
			ParameterType: codeActionParamTypeFromGen(p.ParameterType()),
		}
		jp.ID = model.ID(p.ID())
		out.Parameters = append(out.Parameters, jp)
	}
	for _, el := range g.TypeParametersItems() {
		tp, ok := el.(*genCa.TypeParameter)
		if !ok {
			continue
		}
		d := &javaactions.TypeParameterDef{Name: tp.Name()}
		d.ID = model.ID(tp.ID())
		out.TypeParameters = append(out.TypeParameters, d)
	}
	// The MicroflowActionInfo sub-document may decode as either gen type: the
	// legacy JavaActions$ shape or the current CodeActions$ shape we now write
	// (#656). Both expose Caption/Category/ID, so read via a shared interface.
	if mai, ok := g.MicroflowActionInfo().(interface {
		Caption() string
		Category() string
		ID() element.ID
		Raw() bson.Raw
	}); ok && mai != nil {
		m := &javaactions.MicroflowActionInfo{Caption: mai.Caption(), Category: mai.Category()}
		m.ID = model.ID(mai.ID())
		readActionInfoBitmaps(mai.Raw(), m)
		out.MicroflowActionInfo = m
	}
	out.ReturnType = codeActionReturnTypeFromGen(g.JavaReturnType())
	return out
}

// codeActionParamTypeFromGen converts a gen parameter-type element to the semantic
// type (inverse of codeActionParamTypeToGen).
func codeActionParamTypeFromGen(el element.Element) javaactions.CodeActionParameterType {
	switch t := el.(type) {
	case *genCa.BasicParameterType:
		return codeActionBasicFromGen(t.Type())
	case *genCa.StringTemplateParameterType:
		s := &javaactions.StringTemplateParameterType{Grammar: t.Grammar()}
		s.ID = model.ID(t.ID())
		return s
	case *genCa.EntityTypeParameterType:
		e := &javaactions.EntityTypeParameterType{TypeParameterID: model.ID(t.TypeParameterRefID())}
		e.ID = model.ID(t.ID())
		return e
	default:
		return codeActionBasicFromGen(el)
	}
}

// codeActionBasicFromGen converts the inner type of a BasicParameterType (or a
// directly-typed element) to the semantic parameter type.
func codeActionBasicFromGen(el element.Element) javaactions.CodeActionParameterType {
	switch t := el.(type) {
	// A microflow-typed parameter (a java action that takes a microflow to call
	// back into — MCPServer.AddTool's ExecutingMicroflow, and every "register a
	// handler" action). Missing here, it fell through to the default and read
	// back as a String, so the microflow builder authored a
	// BasicCodeActionParameterValue holding the microflow's name as a literal
	// and `mx check` reported CE0115. mxcli-chat FINDINGS §36.
	case *genJa.MicroflowJavaActionParameterType:
		m := &javaactions.MicroflowType{}
		m.ID = model.ID(t.ID())
		return m
	case *genJa.MicroflowParameterType:
		m := &javaactions.MicroflowType{}
		m.ID = model.ID(t.ID())
		return m
	case *genCa.EnumerationType:
		return &javaactions.EnumerationType{Enumeration: t.EnumerationQualifiedName()}
	case *genCa.ConcreteEntityType:
		return &javaactions.EntityType{Entity: t.EntityQualifiedName()}
	case *genCa.ListType:
		return &javaactions.ListType{Entity: listElementEntity(t)}
	case *genCa.ParameterizedEntityType:
		return &javaactions.TypeParameter{TypeParameterID: model.ID(t.TypeParameterRefID())}
	case *genCa.BooleanType:
		return &javaactions.BooleanType{}
	case *genCa.IntegerType:
		return &javaactions.IntegerType{}
	case *genCa.DecimalType, *genCa.FloatType:
		return &javaactions.DecimalType{}
	case *genCa.DateTimeType:
		return &javaactions.DateTimeType{}
	default:
		return &javaactions.StringType{}
	}
}

// codeActionReturnTypeFromGen converts a gen return-type element to the semantic
// return type (nil for void; inverse of codeActionReturnTypeToGen).
func codeActionReturnTypeFromGen(el element.Element) javaactions.CodeActionReturnType {
	switch t := el.(type) {
	case nil, *genCa.VoidType:
		return nil
	case *genCa.EnumerationType:
		return &javaactions.EnumerationType{Enumeration: t.EnumerationQualifiedName()}
	case *genCa.ConcreteEntityType:
		return &javaactions.EntityType{Entity: t.EntityQualifiedName()}
	case *genCa.ListType:
		return &javaactions.ListType{Entity: listElementEntity(t)}
	case *genCa.ParameterizedEntityType:
		return &javaactions.TypeParameter{TypeParameterID: model.ID(t.TypeParameterRefID())}
	case *genCa.BooleanType:
		return &javaactions.BooleanType{}
	case *genCa.IntegerType:
		return &javaactions.IntegerType{}
	case *genCa.DecimalType, *genCa.FloatType:
		return &javaactions.DecimalType{}
	case *genCa.DateTimeType:
		return &javaactions.DateTimeType{}
	default:
		return &javaactions.StringType{}
	}
}

// listElementEntity extracts the entity qualified name from a gen ListType's
// element parameter (a ConcreteEntityType).
func listElementEntity(l *genCa.ListType) string {
	if ce, ok := l.Parameter().(*genCa.ConcreteEntityType); ok {
		return ce.EntityQualifiedName()
	}
	return ""
}

// readActionInfoBitmaps fills the four toolbox bitmaps from the sub-document's
// raw BSON.
//
// They cannot come from the gen accessors: gen declares IconData, IconDataDark,
// ImageData and ImageDataDark as `string`, and Mendix stores them as BSON
// *binary*, so gen's DecodeString returns "" for every real icon — the same
// class of gen/storage mismatch CLAUDE.md records for scheduled events and
// regular expressions. The write side already goes to raw BSON for exactly this
// reason (microflowActionInfoBSON, #656); the read side did not, which is what
// let a rewrite carry an icon it had never actually loaded.
//
// An absent or non-binary field yields nil, which the writer emits as an empty
// binary — never BSON null, which crashes Studio Pro's UnitWriter.
func readActionInfoBitmaps(raw bson.Raw, m *javaactions.MicroflowActionInfo) {
	if raw == nil || m == nil {
		return
	}
	get := func(key string) []byte {
		val, err := raw.LookupErr(key)
		if err != nil {
			return nil
		}
		if _, data, ok := val.BinaryOK(); ok {
			return data
		}
		return nil
	}
	m.IconData = get("IconData")
	m.IconDataDark = get("IconDataDark")
	m.ImageData = get("ImageData")
	m.ImageDataDark = get("ImageDataDark")
}

// actionInfoFromGen converts a toolbox-entry element to the semantic type,
// bitmaps included. Shared by the Java-action and microflow readers; nil in,
// nil out.
func actionInfoFromGen(el element.Element) *javaactions.MicroflowActionInfo {
	info, ok := el.(interface {
		Caption() string
		Category() string
		ID() element.ID
		Raw() bson.Raw
	})
	if !ok || info == nil {
		return nil
	}
	m := &javaactions.MicroflowActionInfo{Caption: info.Caption(), Category: info.Category()}
	m.ID = model.ID(info.ID())
	readActionInfoBitmaps(info.Raw(), m)
	return m
}
