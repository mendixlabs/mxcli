// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"

	"go.mongodb.org/mongo-driver/bson"
)

func (r *Reader) parseEnumeration(unitID, containerID string, contents []byte) (*model.Enumeration, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	enum := &model.Enumeration{}
	enum.ID = model.ID(unitID)
	enum.TypeName = "Enumerations$Enumeration"
	enum.ContainerID = model.ID(containerID)

	if name, ok := raw["Name"].(string); ok {
		enum.Name = name
	}
	if doc, ok := raw["Documentation"].(string); ok {
		enum.Documentation = doc
	}

	// Parse values - array may start with a version number, skip non-map elements
	if values, ok := raw["Values"].(bson.A); ok {
		for _, v := range values {
			if valMap, ok := v.(map[string]any); ok {
				value := parseEnumerationValue(valMap)
				enum.Values = append(enum.Values, value)
			}
		}
	}

	return enum, nil
}

func parseEnumerationValue(raw map[string]any) model.EnumerationValue {
	value := model.EnumerationValue{}

	if name, ok := raw["Name"].(string); ok {
		value.Name = name
	}
	if caption, ok := raw["Caption"].(map[string]any); ok {
		value.Caption = parseTextMap(caption)
	}

	return value
}

// parseTextMap parses a Text from map[string]interface{}
func parseTextMap(raw map[string]any) *model.Text {
	text := &model.Text{
		Translations: make(map[string]string),
	}

	if items, ok := raw["Items"].(bson.A); ok {
		for _, item := range items {
			if transMap, ok := item.(map[string]any); ok {
				langCode, _ := transMap["LanguageCode"].(string)
				textVal, _ := transMap["Text"].(string)
				if langCode != "" {
					text.Translations[langCode] = textVal
				}
			}
		}
	}

	return text
}

// parseConstant parses constant contents from BSON.
func (r *Reader) parseConstant(unitID, containerID string, contents []byte) (*model.Constant, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	constant := &model.Constant{}
	constant.ID = model.ID(unitID)
	constant.TypeName = "Constants$Constant"
	constant.ContainerID = model.ID(containerID)

	if name, ok := raw["Name"].(string); ok {
		constant.Name = name
	}
	if doc, ok := raw["Documentation"].(string); ok {
		constant.Documentation = doc
	}
	// Parse Type as a nested BSON object containing $Type field
	if typeObj, ok := raw["Type"].(map[string]any); ok {
		constant.Type = parseConstantDataType(typeObj)
	}
	if defaultValue, ok := raw["DefaultValue"].(string); ok {
		constant.DefaultValue = defaultValue
	}
	if exposed, ok := raw["ExposedToClient"].(bool); ok {
		constant.ExposedToClient = exposed
	}
	if excluded, ok := raw["Excluded"].(bool); ok {
		constant.Excluded = excluded
	}
	if exportLevel, ok := raw["ExportLevel"].(string); ok {
		constant.ExportLevel = exportLevel
	}

	return constant, nil
}

// parseConstantDataType extracts the data type from a constant's Type field.
func parseConstantDataType(typeObj map[string]any) model.ConstantDataType {
	dt := model.ConstantDataType{}
	typeName, _ := typeObj["$Type"].(string)

	switch typeName {
	case "DataTypes$StringType":
		dt.Kind = "String"
	case "DataTypes$IntegerType":
		dt.Kind = "Integer"
	case "DataTypes$LongType":
		dt.Kind = "Long"
	case "DataTypes$DecimalType":
		dt.Kind = "Decimal"
	case "DataTypes$BooleanType":
		dt.Kind = "Boolean"
	case "DataTypes$DateTimeType":
		dt.Kind = "DateTime"
	case "DataTypes$BinaryType":
		dt.Kind = "Binary"
	case "DataTypes$FloatType":
		dt.Kind = "Float"
	case "DataTypes$EnumerationType":
		dt.Kind = "Enumeration"
		// Enumeration reference can be string (qualified name) or binary ID
		if enumRef, ok := typeObj["Enumeration"].(string); ok {
			dt.EnumRef = enumRef
		}
	case "DataTypes$ObjectType":
		dt.Kind = "Object"
		if entityRef, ok := typeObj["Entity"].(string); ok {
			dt.EntityRef = entityRef
		}
	case "DataTypes$ListType":
		dt.Kind = "List"
		if entityRef, ok := typeObj["Entity"].(string); ok {
			dt.EntityRef = entityRef
		}
	default:
		dt.Kind = "Unknown"
	}

	return dt
}

// parseScheduledEvent parses scheduled event contents from BSON.
func (r *Reader) parseScheduledEvent(unitID, containerID string, contents []byte) (*model.ScheduledEvent, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	event := &model.ScheduledEvent{}
	event.ID = model.ID(unitID)
	event.TypeName = "ScheduledEvents$ScheduledEvent"
	event.ContainerID = model.ID(containerID)

	if name, ok := raw["Name"].(string); ok {
		event.Name = name
	}
	if doc, ok := raw["Documentation"].(string); ok {
		event.Documentation = doc
	}
	if mfID, ok := raw["Microflow"].(string); ok {
		event.MicroflowID = model.ID(mfID)
	}
	if enabled, ok := raw["Enabled"].(bool); ok {
		event.Enabled = enabled
	}
	// Issue #585: Studio Pro stores Interval as BSON int64; extractInt
	// also accepts int32/int/float64 emitted by other writers.
	if _, ok := raw["Interval"]; ok {
		event.Interval = extractInt(raw["Interval"])
	}
	if intervalType, ok := raw["IntervalType"].(string); ok {
		event.IntervalType = intervalType
	}

	return event, nil
}

// resolveContents handles MPR v2 external file references.
