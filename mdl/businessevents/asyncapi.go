// SPDX-License-Identifier: Apache-2.0

// Package businessevents renders the AsyncAPI contract of a published business
// event service — the document Studio Pro writes from its "Export AsyncAPI
// Document" button, and the only way a consuming app in another project can
// learn the channel id its subscriber must bind to.
//
// Nothing serves this document: a publisher stores an empty Document, so unlike
// a published OData or REST service's contract there is no running app to fetch
// it from (ako/mxcli#2384 §5.5). It has to be rendered.
//
// The output is compared byte for byte against Studio Pro's own export in the
// package's golden test. That is why this writes YAML by hand rather than
// through a marshaller: Studio Pro quotes `title: "X"` but not
// `operationId: x`, and single-quotes `$ref`, and no general marshaller
// reproduces that mix.
package businessevents

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/model"
)

// asyncAPIFixedTraits is the CloudEvents header trait and the Kafka operation
// trait, copied verbatim from a Studio Pro 11.16.1 export. They do not vary with
// the model — including the literal \r\n inside the summary strings, which are
// YAML escape sequences in the file and not raw carriage returns.
const asyncAPIFixedTraits = `  messageTraits:
    commonHeaders:
      name: cloudevents-headers
      summary: "\r\nMessage headers used by CloudEvents spec in binary content mode Kafka\r\nbinding (see\r\nhttps://github.com/cloudevents/spec/blob/v1.0.1/kafka-protocol-binding.md)"
      headers:
        type: object
        required:
        - ce_specversion
        - ce_id
        - ce-source
        - ce_type
        properties:
          ce_specversion:
            type: string
            enum:
            - 1.0
            description: "The version of the CloudEvents specification which the event uses."
          ce_id:
            type: string
            description: "Identifies the event."
            minLength: 1
          ce_source:
            type: string
            format: uri-reference
            description: "Identifies the context in which an event happened."
            minLength: 1
          ce_type:
            type: string
            description: "Describes the type of event related to the originating occurrence."
            minLength: 1
          ce_service:
            type: string
            description: "Describes the service name in which this event was defined."
          ce_subject:
            type: string
            description: "Describes the subject of the event in the context of the event\r\nproducer (identified by source)."
          ce_time:
            type: string
            format: date-time
            description: "Timestamp of when the occurrence happened. Must adhere to RFC\r\n3339."
          content-type:
            type: string
            enum:
            - application/json
  operationTraits:
    kafka:
      bindings:
        kafka:
          key:
            type: string
            description: "Event Id"
defaultContentType: application/json
`

// UnsupportedAttributeTypeError reports a message attribute Mendix puts no
// encoding for in an AsyncAPI contract. Reason says which of the two cases it
// is, because they need different fixes.
type UnsupportedAttributeTypeError struct {
	Message   string
	Attribute string
	Type      string
	// Reason is why there is no encoding, in a sentence the caller can print.
	Reason string
}

func (e *UnsupportedAttributeTypeError) Error() string {
	reason := e.Reason
	if reason == "" {
		reason = fmt.Sprintf("%s is not a Mendix attribute type", e.Type)
	}
	return fmt.Sprintf("cannot render attribute %s.%s: %s", e.Message, e.Attribute, reason)
}

// unsupportedReason explains why a type gets no encoding, or "" when it gets
// one. Both cases come from Mendix's own documentation, which is why they are
// worded as the product's limitation rather than as a gap in mxcli.
func unsupportedReason(mendixType string) string {
	switch strings.ToLower(mendixType) {
	case "autonumber", "binary", "hashedstring", "hashed string":
		return fmt.Sprintf("Mendix does not support %s for a business event attribute "+
			"(Business Events configuration, \"Attribute Types\"). Use a supported type: "+
			"Boolean, Decimal, Integer, Long, String or DateTime", mendixType)
	case "enumeration":
		return "an enumeration's items belong in the exported contract (\"The enumeration items are " +
			"stored in the exported AsyncAPI document\", Studio Pro 10.0+), and a business event message " +
			"attribute carries only a name, a type and a length — there is no enumeration for the renderer " +
			"to take them from. Export the contract from Studio Pro and author the consumer with ContractFile:"
	}
	return ""
}

// asyncAPIType is the AsyncAPI schema type and format Mendix writes for a
// message attribute type. ok is false for a type that gets no encoding;
// unsupportedReason says why.
//
// # Where this mapping comes from
//
// Three independent sources agree on these six, which are every type a business
// event attribute may have:
//
//  1. Mendix.Modeler.BusinessEvents.dll IS the generator — its string heap holds
//     every literal of the export (asyncapi 2.2.0, the channel and component
//     $refs, "Payload", the CloudEvents messageTraits, "Event Id") — and the only
//     JSON Schema values in it are `boolean`, `number`, `integer`, `string` and
//     the formats `int32`, `int64`, `binary`, `date-time`. So the generator
//     cannot emit anything outside that set.
//  2. Mendix's published REST JSON-schema reference gives Long as integer/int64,
//     Integer as integer/int32, Date and time as string/date-time and Decimal as
//     a plain number — a different feature, the same four pairings.
//  3. Measured on Mendix 11.16.1: a contract carrying string, date and decimal
//     payload fields passes mx check and the events round-trip through the
//     Business Events module intact.
//
// Long is measured directly as well, byte-for-byte against the Studio Pro export
// in testdata (#2384 §9.1), which is what the golden test pins.
//
// # What is deliberately absent
//
// AutoNumber, Binary and Hashed string are not here because Mendix does not
// support them for a business event attribute at all, and Enumeration because
// its items belong in the contract and the model cannot supply them — see
// unsupportedReason. An earlier draft of this function mapped all ten by
// elimination ("eight literals, ten types, nothing left over"); that argument
// was wrong, and rendering an enumeration as a bare string would have silently
// dropped its items — a contract that imports cleanly and is wrong, which is the
// failure refusing was there to prevent.
//
// Until this, only Long was accepted and the other nine were refused as
// "unmeasured", which made `export contract` unusable for a real service: one
// String field was enough to stop it.
func asyncAPIType(mendixType string) (schemaType, format string, ok bool) {
	switch strings.ToLower(mendixType) {
	case "boolean":
		return "boolean", "", true
	case "decimal":
		return "number", "", true
	case "integer":
		return "integer", "int32", true
	case "long":
		return "integer", "int64", true
	case "string":
		return "string", "", true
	case "datetime", "date":
		// mxcli's `Date` spelling stores as a DateTimeAttributeType — Mendix has
		// no date-only type (CLAUDE.md) — so it is a DateTime here too.
		return "string", "date-time", true
	}
	return "", "", false
}

// MendixAttributeTypesFor is asyncAPIType backwards: the Mendix attribute types
// a consumed entity may declare for a payload property of this JSON Schema type
// and format. ok is false for an encoding Mendix does not write.
//
// It returns a SET, not one type, because the two directions are not symmetric.
// Rendering has one answer per Mendix type; a contract being consumed is
// another app's, and more than one attribute type can legitimately hold a given
// encoding — an enumeration travels as its key, so a `string` property is as
// well served by an Enumeration attribute as by a String. A check that demanded
// the single type this mapping renders would flag correct models.
//
// The first entry is what Mendix itself renders for that encoding, so a caller
// wanting one name to suggest can take it.
func MendixAttributeTypesFor(schemaType, format string) ([]string, bool) {
	switch strings.ToLower(schemaType) {
	case "boolean":
		return []string{"Boolean"}, true
	case "number":
		return []string{"Decimal"}, true
	case "integer":
		switch strings.ToLower(format) {
		case "int32":
			return []string{"Integer"}, true
		case "int64":
			// AutoNumber is a Long at runtime; Mendix will not PUBLISH one, but
			// a consumed entity may store an int64 in either.
			return []string{"Long", "AutoNumber"}, true
		}
		// An integer with no format: either width holds it.
		return []string{"Long", "Integer", "AutoNumber"}, true
	case "string":
		switch strings.ToLower(format) {
		case "date-time":
			return []string{"DateTime"}, true
		case "binary":
			return []string{"Binary"}, true
		}
		return []string{"String", "Enumeration", "HashedString"}, true
	}
	return nil, false
}

// RenderAsyncAPI renders a published business event service's contract.
//
// It refuses a service with no definition: a consumed one already stores its
// contract, and rendering a second from nothing would produce a document whose
// channel ids match no producer.
func RenderAsyncAPI(svc *model.BusinessEventService) (string, error) {
	if svc == nil || svc.Definition == nil {
		return "", fmt.Errorf("no definition to render: only a published business event service has one (a consumed service already stores its contract)")
	}
	def := svc.Definition

	var b strings.Builder
	b.WriteString("asyncapi: 2.2.0\n")
	b.WriteString("info:\n")
	fmt.Fprintf(&b, "  title: %s\n", yamlQuoted(def.ServiceName))
	// Studio Pro writes a fixed document version; the model carries none.
	b.WriteString("  version: \"1.0.0\"\n")
	fmt.Fprintf(&b, "  description: %s\n", yamlQuoted(def.Description))
	fmt.Fprintf(&b, "  x-summary: %s\n", yamlQuoted(def.Summary))
	b.WriteString("  x-mendix-generated: true\n")

	b.WriteString("channels:\n")
	for _, ch := range def.Channels {
		fmt.Fprintf(&b, "  %s:\n", ch.ChannelName)
		for _, msg := range ch.Messages {
			// AsyncAPI 2's direction is the OTHER app's: a message this service
			// publishes is one others `subscribe` to, which is what
			// Message.CanSubscribe records and what Studio Pro exports.
			if !msg.CanSubscribe {
				return "", fmt.Errorf("message %s is not one other apps subscribe to; only that direction is measured against a Studio Pro export (#2384 §9.1)", msg.MessageName)
			}
			b.WriteString("    subscribe:\n")
			fmt.Fprintf(&b, "      operationId: receive%sEvents\n", msg.MessageName)
			b.WriteString("      traits:\n")
			b.WriteString("      - $ref: '#/components/operationTraits/kafka'\n")
			b.WriteString("      message:\n")
			fmt.Fprintf(&b, "        $ref: '#/components/messages/%s'\n", msg.MessageName)
		}
	}

	b.WriteString("components:\n")
	b.WriteString("  messages:\n")
	for _, ch := range def.Channels {
		for _, msg := range ch.Messages {
			fmt.Fprintf(&b, "    %s:\n", msg.MessageName)
			fmt.Fprintf(&b, "      name: %s\n", msg.MessageName)
			fmt.Fprintf(&b, "      title: %s event\n", msg.MessageName)
			fmt.Fprintf(&b, "      description: %s\n", yamlQuoted(msg.Description))
			b.WriteString("      contentType: application/json\n")
			b.WriteString("      traits:\n")
			b.WriteString("      - $ref: '#/components/messageTraits/commonHeaders'\n")
			b.WriteString("      payload:\n")
			fmt.Fprintf(&b, "        $ref: '#/components/schemas/%sPayload'\n", msg.MessageName)
		}
	}

	b.WriteString("  schemas:\n")
	for _, ch := range def.Channels {
		for _, msg := range ch.Messages {
			fmt.Fprintf(&b, "    %sPayload:\n", msg.MessageName)
			b.WriteString("      type: object\n")
			b.WriteString("      properties:\n")
			for _, attr := range msg.Attributes {
				schemaType, format, ok := asyncAPIType(attr.AttributeType)
				if !ok {
					return "", &UnsupportedAttributeTypeError{
						Message: msg.MessageName, Attribute: attr.AttributeName, Type: attr.AttributeType,
						Reason: unsupportedReason(attr.AttributeType),
					}
				}
				fmt.Fprintf(&b, "        %s:\n", attr.AttributeName)
				fmt.Fprintf(&b, "          type: %s\n", schemaType)
				if format != "" {
					fmt.Fprintf(&b, "          format: %s\n", format)
				}
			}
		}
	}

	b.WriteString(asyncAPIFixedTraits)
	return b.String(), nil
}

// yamlQuoted renders a value the way Studio Pro writes these fields: always
// double-quoted, with backslashes and quotes escaped. An empty value is "",
// which is what the fixture carries for an unset description.
func yamlQuoted(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`)
	return `"` + r.Replace(s) + `"`
}
