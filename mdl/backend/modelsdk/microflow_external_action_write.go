// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func init() {
	// ParameterMappings is a mandatory typed array (marker 3) that Studio Pro
	// always emits, even empty (e.g. an external Action with no parameters like
	// TripPin ResetDataSource). Register the marker so the empty case still
	// serializes the array rather than omitting it.
	codec.RegisterTypeDefaults("Microflows$CallExternalAction", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"ParameterMappings": 3},
	})
}

// callExternalActionToGen builds a Microflows$CallExternalAction
// ("call external action" against a consumed OData service). Mirrors the legacy
// serializer field-for-field; see sdk/mpr/writer_microflow_actions.go. #680/odata.
func callExternalActionToGen(a *microflows.CallExternalAction) element.Element {
	g := newElem("Microflows$CallExternalAction", string(a.ID))
	addStr(g, "ErrorHandlingType", orDefault(string(a.ErrorHandlingType), "Rollback"))
	addStr(g, "ConsumedODataService", a.ConsumedODataService)
	addStr(g, "Name", a.Name)
	addStr(g, "VariableName", a.ResultVariableName)
	// VariableDataType pins the action's return type so Mendix doesn't raise
	// CE7269 ("return type has changed"); the executor resolves the kind from the
	// consumed service's cached $metadata. Omitted for void/unknown.
	if a.ResultDataType != "" {
		addPart(g, "VariableDataType", externalActionReturnTypeToGen(a.ResultDataType))
	}
	mappings := make([]element.Element, 0, len(a.ParameterMappings))
	for _, pm := range a.ParameterMappings {
		m := newElem("Microflows$ExternalActionParameterMapping", string(pm.ID))
		addStr(m, "ParameterName", pm.ParameterName)
		addStr(m, "Argument", pm.Argument)
		addBool(m, "CanBeEmpty", pm.CanBeEmpty)
		mappings = append(mappings, m)
	}
	if len(mappings) > 0 {
		addPartList(g, "ParameterMappings", mappings)
	}
	return g
}

// externalActionReturnTypeToGen maps a Mendix kind name to the DataTypes$ element
// stored in CallExternalAction.VariableDataType. Mirrors
// sdk/mpr.serializeExternalActionReturnType.
func externalActionReturnTypeToGen(kind string) element.Element {
	t := "DataTypes$VoidType"
	switch kind {
	case "Boolean":
		t = "DataTypes$BooleanType"
	case "String":
		t = "DataTypes$StringType"
	case "Integer", "Long":
		t = "DataTypes$IntegerType"
	case "Decimal", "Float":
		t = "DataTypes$DecimalType"
	case "DateTime":
		t = "DataTypes$DateTimeType"
	case "Binary":
		t = "DataTypes$BinaryType"
	}
	return newElem(t, "")
}
