// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	genCa "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/codeactions"
	genJa "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/javaactions"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/mprread"
	"github.com/JordtenBulte-OLC/mxcli/sdk/javaactions"
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
	}); ok && mai != nil {
		m := &javaactions.MicroflowActionInfo{Caption: mai.Caption(), Category: mai.Category()}
		m.ID = model.ID(mai.ID())
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
