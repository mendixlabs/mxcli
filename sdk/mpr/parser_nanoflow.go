// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"

	"go.mongodb.org/mongo-driver/bson"
)

func (r *Reader) parseNanoflow(unitID, containerID string, contents []byte) (*microflows.Nanoflow, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	nf := &microflows.Nanoflow{}
	nf.ID = model.ID(unitID)
	nf.TypeName = "Microflows$Nanoflow"
	nf.ContainerID = model.ID(containerID)

	if name, ok := raw["Name"].(string); ok {
		nf.Name = name
	}
	if doc, ok := raw["Documentation"].(string); ok {
		nf.Documentation = doc
	}
	if markAsUsed, ok := raw["MarkAsUsed"].(bool); ok {
		nf.MarkAsUsed = markAsUsed
	}
	if excluded, ok := raw["Excluded"].(bool); ok {
		nf.Excluded = excluded
	}

	// Parse AllowedModuleRoles
	for _, role := range extractBsonArray(raw["AllowedModuleRoles"]) {
		if roleID, ok := role.(string); ok {
			nf.AllowedModuleRoles = append(nf.AllowedModuleRoles, model.ID(roleID))
		}
	}

	// Parse parameters (same format variants as microflows)
	var paramsArray any
	if mpc, ok := raw["MicroflowParameterCollection"]; ok {
		if mpcMap := extractBsonMap(mpc); mpcMap != nil {
			paramsArray = mpcMap["Parameters"]
		}
	} else {
		paramKey := "MicroflowParameters"
		if _, ok := raw[paramKey]; !ok {
			paramKey = "Parameters"
		}
		paramsArray = raw[paramKey]
	}
	for _, p := range extractBsonSlice(paramsArray) {
		if paramMap := extractBsonMap(p); paramMap != nil {
			param := parseMicroflowParameter(paramMap)
			nf.Parameters = append(nf.Parameters, param)
		}
	}

	// Parse return type (uses same BSON key as microflows)
	if rt, ok := raw["MicroflowReturnType"].(map[string]any); ok {
		nf.ReturnType = parseMicroflowDataType(rt)
	}

	// Parse object collection (activities)
	if oc := extractBsonMap(raw["ObjectCollection"]); oc != nil {
		nf.ObjectCollection = parseMicroflowObjectCollection(oc)
	}

	// Also extract parameters from ObjectCollection.Objects (modern format)
	if len(nf.Parameters) == 0 {
		if ocRaw := extractBsonMap(raw["ObjectCollection"]); ocRaw != nil {
			for _, obj := range extractBsonSlice(ocRaw["Objects"]) {
				if objMap := extractBsonMap(obj); objMap != nil {
					if typeName, _ := objMap["$Type"].(string); typeName == "Microflows$MicroflowParameter" {
						param := parseMicroflowParameter(objMap)
						nf.Parameters = append(nf.Parameters, param)
					}
				}
			}
		}
	}

	// Parse Flows array (SequenceFlows and AnnotationFlows at root level)
	if flowsRaw := raw["Flows"]; flowsRaw != nil {
		if nf.ObjectCollection == nil {
			nf.ObjectCollection = &microflows.MicroflowObjectCollection{}
		}
		for _, f := range extractBsonSlice(flowsRaw) {
			if flowMap := extractBsonMap(f); flowMap != nil {
				typeName, _ := flowMap["$Type"].(string)
				switch typeName {
				case "Microflows$AnnotationFlow":
					if af := parseAnnotationFlow(flowMap); af != nil {
						nf.ObjectCollection.AnnotationFlows = append(nf.ObjectCollection.AnnotationFlows, af)
					}
				default:
					if flow := parseSequenceFlow(flowMap); flow != nil {
						nf.ObjectCollection.Flows = append(nf.ObjectCollection.Flows, flow)
					}
				}
			}
		}
	}

	return nf, nil
}

// parsePage parses page contents from BSON.
