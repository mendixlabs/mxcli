// SPDX-License-Identifier: Apache-2.0

package meta

// The System module's built-in Java actions.
//
// These are NOT stored in the .mpr — Mendix ships them with the platform — so a
// reader that only decodes stored units reports them as absent. The legacy
// sdk/mpr reader synthesized them and the codec backend did not, which made
// `project-tree` lose System.VerifyPassword the moment it moved onto the
// backend (docs/plans/2026-09-14-retire-legacy-engine.md, Phase 4a).
//
// They live here rather than in mdl/types because the fully-typed builder needs
// sdk/javaactions, and sdk/javaactions imports mdl/types — the other direction
// would be a cycle. modelsdk/meta already owns the virtual System module's
// entities and associations, so the Java actions belong beside them.

import (
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// SystemJavaParamDef defines a parameter of a System Java action.
type SystemJavaParamDef struct {
	Name string
	Type string // "String", "Boolean", "Integer", "Long", "Decimal", "DateTime"
}

// SystemJavaActionDef defines a Java action in the System module.
type SystemJavaActionDef struct {
	Name          string
	Documentation string
	ReturnType    string // "Boolean", "String", "Integer", "Long", "Decimal", "DateTime", "Void"
	Parameters    []SystemJavaParamDef
}

// SystemJavaActions lists all Java actions in the System module.
// Extracted from Mendix Studio Pro 11.9.0 via
// `mx dump-mpr --module-names=System --unit-type=JavaActions$JavaAction`.
var SystemJavaActions = []SystemJavaActionDef{
	{
		Name:          "VerifyPassword",
		Documentation: "Verifies that the specified user name/password combination is valid.",
		ReturnType:    "Boolean",
		Parameters: []SystemJavaParamDef{
			{Name: "userName", Type: "String"},
			{Name: "password", Type: "String"},
		},
	},
}

// BuildSystemJavaActions returns lightweight types.JavaAction entries.
//
// IDs are deterministic, so two readers describing the same project agree about
// a System action's identity even though neither read it from storage.
func BuildSystemJavaActions() []*types.JavaAction {
	result := make([]*types.JavaAction, 0, len(SystemJavaActions))
	for _, def := range SystemJavaActions {
		ja := &types.JavaAction{
			ContainerID:   model.ID(SystemModuleID),
			Name:          def.Name,
			Documentation: def.Documentation,
		}
		ja.ID = model.ID(types.GenerateDeterministicID("System." + def.Name))
		result = append(result, ja)
	}
	return result
}

// BuildSystemJavaActionsFull returns fully-typed entries, for catalog insertion.
func BuildSystemJavaActionsFull() []*javaactions.JavaAction {
	result := make([]*javaactions.JavaAction, 0, len(SystemJavaActions))
	for _, def := range SystemJavaActions {
		ja := &javaactions.JavaAction{
			ContainerID:   model.ID(SystemModuleID),
			Name:          def.Name,
			Documentation: def.Documentation,
			ExportLevel:   "Hidden",
		}
		ja.ID = model.ID(types.GenerateDeterministicID("System." + def.Name))
		ja.ReturnType = buildSystemReturnType(def.ReturnType)
		for _, p := range def.Parameters {
			param := &javaactions.JavaActionParameter{
				Name:       p.Name,
				IsRequired: true,
			}
			param.ID = model.ID(types.GenerateDeterministicID("System." + def.Name + "." + p.Name))
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
