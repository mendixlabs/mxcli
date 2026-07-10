// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func init() {
	// A REST operation call's path/query parameter mapping lists use marker 3
	// (the empty form defaults to 3 already). OutputVariable, BodyVariable, and
	// BaseUrlParameterMapping serialize as BSON null when unset.
	codec.RegisterListMarker("Microflows$ParameterMapping", 3)
	codec.RegisterListMarker("Microflows$QueryParameterMapping", 3)
	codec.RegisterTypeDefaults("Microflows$RestOperationCallAction", codec.TypeDefaults{
		NullFields:           []string{"OutputVariable", "BodyVariable", "BaseUrlParameterMapping"},
		MandatoryListMarkers: map[string]int32{"ParameterMappings": 3, "QueryParameterMappings": 3},
	})
}

// restOperationCallActionToGen builds a Microflows$RestOperationCallAction
// ("call rest operation" on a consumed REST service). Mirrors
// serializeRestOperationCallAction field-for-field.
func restOperationCallActionToGen(a *microflows.RestOperationCallAction) element.Element {
	g := newElem("Microflows$RestOperationCallAction", string(a.ID))
	addStr(g, "Operation", a.Operation)

	if a.OutputVariable != nil {
		ov := newElem("Microflows$OutputVariable", string(a.OutputVariable.ID))
		addStr(ov, "VariableName", a.OutputVariable.VariableName)
		addPart(g, "OutputVariable", ov)
	}
	if a.BodyVariable != nil {
		bv := newElem("Microflows$BodyVariable", string(a.BodyVariable.ID))
		addStr(bv, "VariableName", a.BodyVariable.VariableName)
		addPart(g, "BodyVariable", bv)
	}
	// BaseUrlParameterMapping: null (via NullFields).

	params := make([]element.Element, 0, len(a.ParameterMappings))
	for _, pm := range a.ParameterMappings {
		p := newElem("Microflows$ParameterMapping", "")
		addStr(p, "Parameter", pm.Parameter)
		addStr(p, "Value", pm.Value)
		params = append(params, p)
	}
	if len(params) > 0 {
		addPartList(g, "ParameterMappings", params)
	}

	queries := make([]element.Element, 0, len(a.QueryParameterMappings))
	for _, qm := range a.QueryParameterMappings {
		q := newElem("Microflows$QueryParameterMapping", "")
		addStr(q, "QueryParameter", qm.Parameter)
		addStr(q, "Value", qm.Value)
		addStr(q, "Included", qm.Included)
		queries = append(queries, q)
	}
	if len(queries) > 0 {
		addPartList(g, "QueryParameterMappings", queries)
	}
	return g
}
