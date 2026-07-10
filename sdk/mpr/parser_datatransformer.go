// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"

	"go.mongodb.org/mongo-driver/bson"
)

// parseDataTransformer parses a DataTransformers$DataTransformer from BSON.
func (r *Reader) parseDataTransformer(unitID, containerID string, contents []byte) (*model.DataTransformer, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	dt := &model.DataTransformer{}
	dt.ID = model.ID(unitID)
	dt.TypeName = "DataTransformers$DataTransformer"
	dt.ContainerID = model.ID(containerID)
	dt.Name = extractString(raw["Name"])
	dt.Excluded = extractBool(raw["Excluded"], false)

	// Parse Source
	if srcMap := extractBsonMap(raw["Source"]); srcMap != nil {
		srcType := extractString(srcMap["$Type"])
		switch srcType {
		case "DataTransformers$JsonSource":
			dt.SourceType = "JSON"
			dt.SourceJSON = extractString(srcMap["Content"])
		case "DataTransformers$XmlSource":
			dt.SourceType = "XML"
			dt.SourceJSON = extractString(srcMap["Content"])
		}
	}

	// Parse Steps
	steps := extractBsonArray(raw["Steps"])
	for _, step := range steps {
		stepMap, ok := step.(map[string]any)
		if !ok {
			continue
		}
		if extractString(stepMap["$Type"]) != "DataTransformers$Step" {
			continue
		}
		actionMap := extractBsonMap(stepMap["Action"])
		if actionMap == nil {
			continue
		}
		actionType := extractString(actionMap["$Type"])
		s := &model.DataTransformerStep{}
		switch actionType {
		case "DataTransformers$JsltAction":
			s.Technology = "JSLT"
			s.Expression = extractString(actionMap["Jslt"])
		case "DataTransformers$XsltAction":
			s.Technology = "XSLT"
			s.Expression = extractString(actionMap["Xslt"])
		default:
			s.Technology = actionType
		}
		dt.Steps = append(dt.Steps, s)
	}

	return dt, nil
}
