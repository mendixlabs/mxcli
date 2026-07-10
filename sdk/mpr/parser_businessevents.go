// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"

	"go.mongodb.org/mongo-driver/bson"
)

// parseBusinessEventService parses a BusinessEvents$BusinessEventService from BSON.
func (r *Reader) parseBusinessEventService(unitID, containerID string, contents []byte) (*model.BusinessEventService, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	svc := &model.BusinessEventService{}
	svc.ID = model.ID(unitID)
	svc.TypeName = "BusinessEvents$BusinessEventService"
	svc.ContainerID = model.ID(containerID)

	svc.Name = extractString(raw["Name"])
	svc.Documentation = extractString(raw["Documentation"])
	svc.Excluded = extractBool(raw["Excluded"], false)
	svc.ExportLevel = extractString(raw["ExportLevel"])
	svc.Document = extractString(raw["Document"])

	// Parse Definition (non-nil for service definitions)
	if defRaw, ok := raw["Definition"]; ok && defRaw != nil {
		if defMap := extractBsonMap(defRaw); defMap != nil {
			svc.Definition = parseBusinessEventDefinition(defMap)
		}
	}

	// Parse OperationImplementations
	opImpls := extractBsonArray(raw["OperationImplementations"])
	for _, oi := range opImpls {
		if oiMap := extractBsonMap(oi); oiMap != nil {
			svc.OperationImplementations = append(svc.OperationImplementations, parseServiceOperation(oiMap))
		}
	}

	return svc, nil
}

// parseBusinessEventDefinition parses a BusinessEvents$BusinessEventDefinition from a BSON map.
func parseBusinessEventDefinition(raw map[string]any) *model.BusinessEventDefinition {
	def := &model.BusinessEventDefinition{}
	def.ID = model.ID(extractBsonID(raw["$ID"]))
	def.TypeName = extractString(raw["$Type"])
	def.ServiceName = extractString(raw["ServiceName"])
	def.EventNamePrefix = extractString(raw["EventNamePrefix"])
	def.Description = extractString(raw["Description"])
	def.Summary = extractString(raw["Summary"])

	// Parse Channels
	channels := extractBsonArray(raw["Channels"])
	for _, ch := range channels {
		if chMap := extractBsonMap(ch); chMap != nil {
			def.Channels = append(def.Channels, parseBusinessEventChannel(chMap))
		}
	}

	return def
}

// parseBusinessEventChannel parses a BusinessEvents$Channel from a BSON map.
func parseBusinessEventChannel(raw map[string]any) *model.BusinessEventChannel {
	ch := &model.BusinessEventChannel{}
	ch.ID = model.ID(extractBsonID(raw["$ID"]))
	ch.TypeName = extractString(raw["$Type"])
	ch.ChannelName = extractString(raw["ChannelName"])
	ch.Description = extractString(raw["Description"])

	// Parse Messages
	messages := extractBsonArray(raw["Messages"])
	for _, msg := range messages {
		if msgMap := extractBsonMap(msg); msgMap != nil {
			ch.Messages = append(ch.Messages, parseBusinessEventMessage(msgMap))
		}
	}

	return ch
}

// parseBusinessEventMessage parses a BusinessEvents$Message from a BSON map.
func parseBusinessEventMessage(raw map[string]any) *model.BusinessEventMessage {
	msg := &model.BusinessEventMessage{}
	msg.ID = model.ID(extractBsonID(raw["$ID"]))
	msg.TypeName = extractString(raw["$Type"])
	msg.MessageName = extractString(raw["MessageName"])
	msg.Description = extractString(raw["Description"])
	msg.CanPublish = extractBool(raw["CanPublish"], false)
	msg.CanSubscribe = extractBool(raw["CanSubscribe"], false)

	// Parse Attributes
	attrs := extractBsonArray(raw["Attributes"])
	for _, a := range attrs {
		if aMap := extractBsonMap(a); aMap != nil {
			msg.Attributes = append(msg.Attributes, parseBusinessEventAttribute(aMap))
		}
	}

	return msg
}

// parseBusinessEventAttribute parses a BusinessEvents$MessageAttribute from a BSON map.
func parseBusinessEventAttribute(raw map[string]any) *model.BusinessEventAttribute {
	attr := &model.BusinessEventAttribute{}
	attr.ID = model.ID(extractBsonID(raw["$ID"]))
	attr.TypeName = extractString(raw["$Type"])
	attr.AttributeName = extractString(raw["AttributeName"])
	attr.Description = extractString(raw["Description"])

	// Parse AttributeType — extract kind from the nested $Type field
	// e.g., "DomainModels$LongAttributeType" → "Long"
	if atRaw := extractBsonMap(raw["AttributeType"]); atRaw != nil {
		typeName := extractString(atRaw["$Type"])
		attr.AttributeType = attributeTypeFromBsonType(typeName)
	}

	return attr
}

// attributeTypeFromBsonType converts a BSON $Type like "DomainModels$LongAttributeType" to "Long".
func attributeTypeFromBsonType(bsonType string) string {
	switch bsonType {
	case "DomainModels$LongAttributeType":
		return "Long"
	case "DomainModels$StringAttributeType":
		return "String"
	case "DomainModels$IntegerAttributeType":
		return "Integer"
	case "DomainModels$BooleanAttributeType":
		return "Boolean"
	case "DomainModels$DateTimeAttributeType":
		return "DateTime"
	case "DomainModels$DecimalAttributeType":
		return "Decimal"
	case "DomainModels$AutoNumberAttributeType":
		return "AutoNumber"
	case "DomainModels$BinaryAttributeType":
		return "Binary"
	default:
		return bsonType
	}
}

// parseServiceOperation parses a BusinessEvents$ServiceOperation from a BSON map.
func parseServiceOperation(raw map[string]any) *model.ServiceOperation {
	op := &model.ServiceOperation{}
	op.ID = model.ID(extractBsonID(raw["$ID"]))
	op.TypeName = extractString(raw["$Type"])
	op.MessageName = extractString(raw["MessageName"])
	op.Operation = extractString(raw["Operation"])
	op.Entity = extractString(raw["Entity"])
	op.Microflow = extractString(raw["Microflow"])
	return op
}
