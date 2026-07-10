// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"

	"go.mongodb.org/mongo-driver/bson"
)

// CreateBusinessEventService creates a new business event service document.
func (w *Writer) CreateBusinessEventService(svc *model.BusinessEventService) error {
	if svc.ID == "" {
		svc.ID = model.ID(generateUUID())
	}
	svc.TypeName = "BusinessEvents$BusinessEventService"

	contents, err := w.serializeBusinessEventService(svc)
	if err != nil {
		return fmt.Errorf("failed to serialize business event service: %w", err)
	}

	return w.insertUnit(string(svc.ID), string(svc.ContainerID), "Documents", "BusinessEvents$BusinessEventService", contents)
}

// UpdateBusinessEventService updates an existing business event service.
func (w *Writer) UpdateBusinessEventService(svc *model.BusinessEventService) error {
	contents, err := w.serializeBusinessEventService(svc)
	if err != nil {
		return fmt.Errorf("failed to serialize business event service: %w", err)
	}

	return w.updateUnit(string(svc.ID), contents)
}

// DeleteBusinessEventService deletes a business event service by ID.
func (w *Writer) DeleteBusinessEventService(id model.ID) error {
	return w.deleteUnit(string(id))
}

// serializeBusinessEventService converts a BusinessEventService to BSON bytes.
func (w *Writer) serializeBusinessEventService(svc *model.BusinessEventService) ([]byte, error) {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(svc.ID))},
		{Key: "$Type", Value: "BusinessEvents$BusinessEventService"},
		{Key: "Name", Value: svc.Name},
		{Key: "Documentation", Value: svc.Documentation},
		{Key: "Excluded", Value: svc.Excluded},
		{Key: "ExportLevel", Value: svc.ExportLevel},
	}

	// Serialize Definition
	if svc.Definition != nil {
		doc = append(doc, bson.E{Key: "Definition", Value: serializeBusinessEventDefinition(svc.Definition)})
	} else {
		doc = append(doc, bson.E{Key: "Definition", Value: nil})
	}

	// Serialize OperationImplementations
	opImpls := bson.A{int32(2)} // versioned array prefix
	for _, op := range svc.OperationImplementations {
		opImpls = append(opImpls, serializeServiceOperation(op))
	}
	doc = append(doc, bson.E{Key: "OperationImplementations", Value: opImpls})

	// SourceApi is null for service definitions
	doc = append(doc, bson.E{Key: "SourceApi", Value: nil})

	return marshalUnitIDFirst(doc)
}

func serializeBusinessEventDefinition(def *model.BusinessEventDefinition) bson.D {
	id := string(def.ID)
	if id == "" {
		id = generateUUID()
	}

	// Serialize Channels
	channels := bson.A{int32(2)} // versioned array prefix
	for _, ch := range def.Channels {
		channels = append(channels, serializeBusinessEventChannel(ch))
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(id)},
		{Key: "$Type", Value: "BusinessEvents$BusinessEventDefinition"},
		{Key: "ServiceName", Value: def.ServiceName},
		{Key: "EventNamePrefix", Value: def.EventNamePrefix},
		{Key: "Description", Value: def.Description},
		{Key: "Summary", Value: def.Summary},
		{Key: "Channels", Value: channels},
	}
}

func serializeBusinessEventChannel(ch *model.BusinessEventChannel) bson.D {
	id := string(ch.ID)
	if id == "" {
		id = generateUUID()
	}

	// Serialize Messages
	messages := bson.A{int32(2)} // versioned array prefix
	for _, msg := range ch.Messages {
		messages = append(messages, serializeBusinessEventMessage(msg))
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(id)},
		{Key: "$Type", Value: "BusinessEvents$Channel"},
		{Key: "ChannelName", Value: ch.ChannelName},
		{Key: "Description", Value: ch.Description},
		{Key: "Messages", Value: messages},
	}
}

func serializeBusinessEventMessage(msg *model.BusinessEventMessage) bson.D {
	id := string(msg.ID)
	if id == "" {
		id = generateUUID()
	}

	// Serialize Attributes
	attrs := bson.A{int32(2)} // versioned array prefix
	for _, attr := range msg.Attributes {
		attrs = append(attrs, serializeBusinessEventAttribute(attr))
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(id)},
		{Key: "$Type", Value: "BusinessEvents$Message"},
		{Key: "MessageName", Value: msg.MessageName},
		{Key: "Description", Value: msg.Description},
		{Key: "CanPublish", Value: msg.CanPublish},
		{Key: "CanSubscribe", Value: msg.CanSubscribe},
		{Key: "Attributes", Value: attrs},
	}
}

func serializeBusinessEventAttribute(attr *model.BusinessEventAttribute) bson.D {
	id := string(attr.ID)
	if id == "" {
		id = generateUUID()
	}

	// Convert attribute type to BSON format: "Long" → {"$Type": "DomainModels$LongAttributeType", "$ID": ...}
	attrTypeDoc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: attributeTypeToBsonType(attr.AttributeType)},
	}
	// Date and DateTime both use DateTimeAttributeType; distinguish via LocalizeDate
	if attr.AttributeType == "DateTime" {
		attrTypeDoc = append(attrTypeDoc, bson.E{Key: "LocalizeDate", Value: true})
	} else if attr.AttributeType == "Date" {
		attrTypeDoc = append(attrTypeDoc, bson.E{Key: "LocalizeDate", Value: false})
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(id)},
		{Key: "$Type", Value: "BusinessEvents$MessageAttribute"},
		{Key: "AttributeName", Value: attr.AttributeName},
		{Key: "Description", Value: attr.Description},
		{Key: "AttributeType", Value: attrTypeDoc},
	}
}

// attributeTypeToBsonType converts a simple type name to a BSON $Type string.
func attributeTypeToBsonType(typeName string) string {
	switch typeName {
	case "Long":
		return "DomainModels$LongAttributeType"
	case "String":
		return "DomainModels$StringAttributeType"
	case "Integer":
		return "DomainModels$IntegerAttributeType"
	case "Boolean":
		return "DomainModels$BooleanAttributeType"
	case "DateTime", "Date":
		return "DomainModels$DateTimeAttributeType"
	case "Decimal":
		return "DomainModels$DecimalAttributeType"
	case "AutoNumber":
		return "DomainModels$AutoNumberAttributeType"
	case "Binary":
		return "DomainModels$BinaryAttributeType"
	default:
		return "DomainModels$StringAttributeType"
	}
}

func serializeServiceOperation(op *model.ServiceOperation) bson.D {
	id := string(op.ID)
	if id == "" {
		id = generateUUID()
	}
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(id)},
		{Key: "$Type", Value: "BusinessEvents$ServiceOperation"},
		{Key: "MessageName", Value: op.MessageName},
		{Key: "Operation", Value: op.Operation},
		{Key: "Entity", Value: op.Entity},
		{Key: "Microflow", Value: op.Microflow},
	}
}
