// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/javaactions"
)

// systemJavaParamDef defines a parameter of a System Java action.
type systemJavaParamDef struct {
	Name string
	Type string // "String", "Boolean", "Integer", "Long", "Decimal", "DateTime"
}

// systemJavaActionDef defines a Java action in the System module.
type systemJavaActionDef struct {
	Name          string
	Documentation string
	ReturnType    string // "Boolean", "String", "Integer", "Long", "Decimal", "DateTime", "Void"
	Parameters    []systemJavaParamDef
}

// systemJavaActions lists all Java actions in the System module.
// Extracted from Mendix Studio Pro 11.9.0 via mx dump-mpr --module-names=System --unit-type=JavaActions$JavaAction.
var systemJavaActions = []systemJavaActionDef{
	{
		Name:          "VerifyPassword",
		Documentation: "Verifies that the specified user name/password combination is valid.",
		ReturnType:    "Boolean",
		Parameters: []systemJavaParamDef{
			{Name: "userName", Type: "String"},
			{Name: "password", Type: "String"},
		},
	},
}

// BuildSystemJavaActions returns lightweight types.JavaAction entries for the System module.
// These are built-in Java actions not stored in the MPR SQLite database.
// Used by ListJavaActions() for executor reference validation.
func BuildSystemJavaActions() []*types.JavaAction {
	result := make([]*types.JavaAction, 0, len(systemJavaActions))
	for _, def := range systemJavaActions {
		ja := &types.JavaAction{
			ContainerID:   model.ID(SystemModuleID),
			Name:          def.Name,
			Documentation: def.Documentation,
		}
		ja.ID = model.ID(GenerateDeterministicID("System." + def.Name))
		result = append(result, ja)
	}
	return result
}

// BuildSystemJavaActionsFull returns fully-typed javaactions.JavaAction entries for the System module.
// These are built-in Java actions not stored in the MPR SQLite database.
// Used by ListJavaActionsFull() for catalog insertion.
func BuildSystemJavaActionsFull() []*javaactions.JavaAction {
	result := make([]*javaactions.JavaAction, 0, len(systemJavaActions))
	for _, def := range systemJavaActions {
		ja := &javaactions.JavaAction{
			ContainerID:   model.ID(SystemModuleID),
			Name:          def.Name,
			Documentation: def.Documentation,
			ExportLevel:   "Hidden",
		}
		ja.ID = model.ID(GenerateDeterministicID("System." + def.Name))
		ja.ReturnType = buildSystemReturnType(def.ReturnType)
		for _, p := range def.Parameters {
			param := &javaactions.JavaActionParameter{
				Name:       p.Name,
				IsRequired: true,
			}
			param.ID = model.ID(GenerateDeterministicID("System." + def.Name + "." + p.Name))
			param.ParameterType = buildSystemParamType(p.Type)
			ja.Parameters = append(ja.Parameters, param)
		}
		result = append(result, ja)
	}
	return result
}

func buildSystemReturnType(t string) javaactions.CodeActionReturnType {
	switch t {
	case "Boolean":
		return &javaactions.BooleanType{}
	case "String":
		return &javaactions.StringType{}
	case "Integer":
		return &javaactions.IntegerType{}
	case "Long":
		return &javaactions.LongType{}
	case "Decimal":
		return &javaactions.DecimalType{}
	case "DateTime":
		return &javaactions.DateTimeType{}
	default:
		return &javaactions.VoidType{}
	}
}

func buildSystemParamType(t string) javaactions.CodeActionParameterType {
	switch t {
	case "Boolean":
		return &javaactions.BooleanType{}
	case "Integer":
		return &javaactions.IntegerType{}
	case "Long":
		return &javaactions.LongType{}
	case "Decimal":
		return &javaactions.DecimalType{}
	case "DateTime":
		return &javaactions.DateTimeType{}
	default:
		return &javaactions.StringType{}
	}
}
