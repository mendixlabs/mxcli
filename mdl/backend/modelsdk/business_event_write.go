// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
)

func init() {
	// A business event service always serializes SourceApi as null and (when the
	// service has no definition) Definition as null. Its OperationImplementations
	// list and every nested list (Channels/Messages/Attributes) use marker 2.
	codec.RegisterTypeDefaults("BusinessEvents$BusinessEventService", codec.TypeDefaults{
		NullFields:           []string{"SourceApi", "Definition"},
		MandatoryListMarkers: map[string]int32{"OperationImplementations": 2},
	})
	codec.RegisterTypeDefaults("BusinessEvents$BusinessEventDefinition", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Channels": 2},
	})
	codec.RegisterTypeDefaults("BusinessEvents$Channel", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Messages": 2},
	})
	codec.RegisterTypeDefaults("BusinessEvents$Message", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Attributes": 2},
	})
	codec.RegisterListMarker("BusinessEvents$ServiceOperation", 2)
	codec.RegisterListMarker("BusinessEvents$Channel", 2)
	codec.RegisterListMarker("BusinessEvents$Message", 2)
	codec.RegisterListMarker("BusinessEvents$MessageAttribute", 2)
}

// CreateBusinessEventService inserts a new BusinessEvents$BusinessEventService
// document (its AsyncAPI-style definition: channels → messages → attributes, plus
// service-operation implementations). Mirrors the legacy serializer.
func (b *Backend) CreateBusinessEventService(svc *model.BusinessEventService) error {
	if svc == nil {
		return fmt.Errorf("CreateBusinessEventService: nil service")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateBusinessEventService: not connected for writing")
	}
	if svc.ID == "" {
		svc.ID = model.ID(mmpr.GenerateID())
	}
	contents, err := (&codec.Encoder{}).Encode(businessEventServiceToGen(svc))
	if err != nil {
		return fmt.Errorf("CreateBusinessEventService: encode: %w", err)
	}
	return b.writer.InsertUnit(string(svc.ID), string(svc.ContainerID), "Documents", "BusinessEvents$BusinessEventService", contents)
}

// UpdateBusinessEventService rewrites an existing service in place (CREATE OR MODIFY).
func (b *Backend) UpdateBusinessEventService(svc *model.BusinessEventService) error {
	if svc == nil {
		return fmt.Errorf("UpdateBusinessEventService: nil service")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateBusinessEventService: not connected for writing")
	}
	contents, err := (&codec.Encoder{}).Encode(businessEventServiceToGen(svc))
	if err != nil {
		return fmt.Errorf("UpdateBusinessEventService: encode: %w", err)
	}
	return b.writer.UpdateRawUnit(string(svc.ID), contents)
}

// DeleteBusinessEventService removes a business event service unit by ID.
func (b *Backend) DeleteBusinessEventService(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteBusinessEventService: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

func businessEventServiceToGen(svc *model.BusinessEventService) element.Element {
	g := newElem("BusinessEvents$BusinessEventService", string(svc.ID))
	addStr(g, "Name", svc.Name)
	addStr(g, "Documentation", svc.Documentation)
	addBool(g, "Excluded", svc.Excluded)
	addStr(g, "ExportLevel", orDefault(svc.ExportLevel, "Hidden"))
	if svc.Definition != nil {
		addPart(g, "Definition", businessEventDefinitionToGen(svc.Definition))
	}
	ops := make([]element.Element, 0, len(svc.OperationImplementations))
	for _, op := range svc.OperationImplementations {
		o := newElem("BusinessEvents$ServiceOperation", string(op.ID))
		addStr(o, "MessageName", op.MessageName)
		addStr(o, "Operation", op.Operation)
		addStr(o, "Entity", op.Entity)
		addStr(o, "Microflow", op.Microflow)
		ops = append(ops, o)
	}
	addPartList(g, "OperationImplementations", ops)
	return g
}

func businessEventDefinitionToGen(def *model.BusinessEventDefinition) element.Element {
	d := newElem("BusinessEvents$BusinessEventDefinition", string(def.ID))
	addStr(d, "ServiceName", def.ServiceName)
	addStr(d, "EventNamePrefix", def.EventNamePrefix)
	addStr(d, "Description", def.Description)
	addStr(d, "Summary", def.Summary)
	channels := make([]element.Element, 0, len(def.Channels))
	for _, ch := range def.Channels {
		c := newElem("BusinessEvents$Channel", string(ch.ID))
		addStr(c, "ChannelName", ch.ChannelName)
		addStr(c, "Description", ch.Description)
		messages := make([]element.Element, 0, len(ch.Messages))
		for _, msg := range ch.Messages {
			m := newElem("BusinessEvents$Message", string(msg.ID))
			addStr(m, "MessageName", msg.MessageName)
			addStr(m, "Description", msg.Description)
			addBool(m, "CanPublish", msg.CanPublish)
			addBool(m, "CanSubscribe", msg.CanSubscribe)
			attrs := make([]element.Element, 0, len(msg.Attributes))
			for _, attr := range msg.Attributes {
				attrs = append(attrs, businessEventAttributeToGen(attr))
			}
			addPartList(m, "Attributes", attrs)
			messages = append(messages, m)
		}
		addPartList(c, "Messages", messages)
		channels = append(channels, c)
	}
	addPartList(d, "Channels", channels)
	return d
}

func businessEventAttributeToGen(attr *model.BusinessEventAttribute) element.Element {
	a := newElem("BusinessEvents$MessageAttribute", string(attr.ID))
	addStr(a, "AttributeName", attr.AttributeName)
	addStr(a, "Description", attr.Description)
	// AttributeType is a DomainModels$*AttributeType sub-object; Date/DateTime both
	// map to DateTimeAttributeType, distinguished by LocalizeDate.
	at := newElem(businessEventAttrTypeName(attr.AttributeType), "")
	switch attr.AttributeType {
	case "DateTime":
		addBool(at, "LocalizeDate", true)
	case "Date":
		addBool(at, "LocalizeDate", false)
	}
	addPart(a, "AttributeType", at)
	return a
}

// businessEventAttrTypeName maps a simple type name to its DomainModels$ storage $Type.
func businessEventAttrTypeName(typeName string) string {
	switch typeName {
	case "Long":
		return "DomainModels$LongAttributeType"
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
