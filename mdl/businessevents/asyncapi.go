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

// UnsupportedAttributeTypeError reports a message attribute whose AsyncAPI
// rendering has not been measured against a Studio Pro export.
//
// Only Long is verified (#2384 §9.1): the fixture carries two Long attributes
// and nothing else. The rest are inferred from format strings in
// Mendix.Modeler.BusinessEvents.dll, and a contract that is subtly wrong is
// worse than one that was refused — the consuming app imports it, binds an
// entity to it, and finds out at deploy time.
type UnsupportedAttributeTypeError struct {
	Message   string
	Attribute string
	Type      string
}

func (e *UnsupportedAttributeTypeError) Error() string {
	return fmt.Sprintf(
		"cannot render attribute %s.%s of type %s: only Long is measured against a Studio Pro export (#2384 §9.1). "+
			"Export the contract from Studio Pro and author the consumer with ContractFile: instead",
		e.Message, e.Attribute, e.Type)
}

// asyncAPIType is the AsyncAPI schema type and format for a message attribute
// type, and whether that mapping has been measured.
func asyncAPIType(mendixType string) (schemaType, format string, measured bool) {
	if strings.EqualFold(mendixType, "long") {
		return "integer", "int64", true
	}
	return "", "", false
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
				schemaType, format, measured := asyncAPIType(attr.AttributeType)
				if !measured {
					return "", &UnsupportedAttributeTypeError{
						Message: msg.MessageName, Attribute: attr.AttributeName, Type: attr.AttributeType,
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
