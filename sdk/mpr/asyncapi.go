// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// AsyncAPIDocument represents a parsed AsyncAPI 2.x document.
type AsyncAPIDocument struct {
	Version     string // AsyncAPI version (e.g. "2.2.0")
	Title       string // Service title
	DocVersion  string // Document version
	Description string
	Channels    []*AsyncAPIChannel // Resolved channels
	Messages    []*AsyncAPIMessage // Resolved messages (from components)
}

// AsyncAPIChannel represents a channel in the AsyncAPI document.
type AsyncAPIChannel struct {
	Name          string // Channel ID/name
	OperationType string // "subscribe" or "publish"
	OperationID   string // e.g. "receiveOrderChangedEventEvents"
	MessageRef    string // Resolved message name
}

// AsyncAPIMessage represents a message type.
type AsyncAPIMessage struct {
	Name        string
	Title       string
	Description string
	ContentType string
	Properties  []*AsyncAPIProperty // Resolved from payload schema
}

// AsyncAPIProperty represents a property in a message payload schema.
type AsyncAPIProperty struct {
	Name   string
	Type   string // "string", "integer", "number", "boolean", "array", "object"
	Format string // "int64", "int32", "date-time", "uri-reference", etc.
}

// ParseAsyncAPI parses an AsyncAPI YAML string into an AsyncAPIDocument.
func ParseAsyncAPI(yamlStr string) (*AsyncAPIDocument, error) {
	if yamlStr == "" {
		return nil, fmt.Errorf("empty AsyncAPI document")
	}

	var raw yamlAsyncAPI
	if err := yaml.Unmarshal([]byte(yamlStr), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse AsyncAPI YAML: %w", err)
	}

	doc := &AsyncAPIDocument{
		Version:     raw.AsyncAPI,
		Title:       raw.Info.Title,
		DocVersion:  raw.Info.Version,
		Description: raw.Info.Description,
	}

	// Resolve messages from components
	for name, msg := range raw.Components.Messages {
		resolved := &AsyncAPIMessage{
			Name:        name,
			Title:       msg.Title,
			Description: msg.Description,
			ContentType: msg.ContentType,
		}

		// Resolve payload schema (follow $ref if present)
		schemaName := ""
		if msg.Payload.Ref != "" {
			schemaName = refName(msg.Payload.Ref)
		}

		if schemaName != "" {
			if schema, ok := raw.Components.Schemas[schemaName]; ok {
				resolved.Properties = resolveSchemaProperties(schema)
			}
		} else if msg.Payload.Properties != nil {
			// Inline schema
			resolved.Properties = resolveSchemaProperties(msg.Payload)
		}

		doc.Messages = append(doc.Messages, resolved)
	}

	// Resolve channels
	for channelName, channel := range raw.Channels {
		if channel.Subscribe != nil {
			msgName := ""
			if channel.Subscribe.Message.Ref != "" {
				msgName = refName(channel.Subscribe.Message.Ref)
			}
			doc.Channels = append(doc.Channels, &AsyncAPIChannel{
				Name:          channelName,
				OperationType: "subscribe",
				OperationID:   channel.Subscribe.OperationID,
				MessageRef:    msgName,
			})
		}
		if channel.Publish != nil {
			msgName := ""
			if channel.Publish.Message.Ref != "" {
				msgName = refName(channel.Publish.Message.Ref)
			}
			doc.Channels = append(doc.Channels, &AsyncAPIChannel{
				Name:          channelName,
				OperationType: "publish",
				OperationID:   channel.Publish.OperationID,
				MessageRef:    msgName,
			})
		}
	}

	return doc, nil
}

// FindMessage looks up a message by name.
func (d *AsyncAPIDocument) FindMessage(name string) *AsyncAPIMessage {
	for _, m := range d.Messages {
		if strings.EqualFold(m.Name, name) {
			return m
		}
	}
	return nil
}

// refName extracts the last segment from a $ref like "#/components/messages/OrderChangedEvent".
func refName(ref string) string {
	if idx := strings.LastIndex(ref, "/"); idx >= 0 {
		return ref[idx+1:]
	}
	return ref
}

func resolveSchemaProperties(schema yamlSchema) []*AsyncAPIProperty {
	var props []*AsyncAPIProperty
	for name, prop := range schema.Properties {
		props = append(props, &AsyncAPIProperty{
			Name:   name,
			Type:   prop.Type,
			Format: prop.Format,
		})
	}
	return props
}

// ============================================================================
// YAML deserialization types (internal)
// ============================================================================

type yamlAsyncAPI struct {
	AsyncAPI   string                 `yaml:"asyncapi"`
	Info       yamlInfo               `yaml:"info"`
	Channels   map[string]yamlChannel `yaml:"channels"`
	Components yamlComponents         `yaml:"components"`
}

type yamlInfo struct {
	Title       string `yaml:"title"`
	Version     string `yaml:"version"`
	Description string `yaml:"description"`
}

type yamlChannel struct {
	Subscribe *yamlOperation `yaml:"subscribe"`
	Publish   *yamlOperation `yaml:"publish"`
}

type yamlOperation struct {
	OperationID string  `yaml:"operationId"`
	Message     yamlRef `yaml:"message"`
}

type yamlRef struct {
	Ref string `yaml:"$ref"`
}

type yamlComponents struct {
	Messages map[string]yamlMessage `yaml:"messages"`
	Schemas  map[string]yamlSchema  `yaml:"schemas"`
}

type yamlMessage struct {
	Name        string     `yaml:"name"`
	Title       string     `yaml:"title"`
	Description string     `yaml:"description"`
	ContentType string     `yaml:"contentType"`
	Payload     yamlSchema `yaml:"payload"`
}

type yamlSchema struct {
	Ref        string                        `yaml:"$ref"`
	Type       string                        `yaml:"type"`
	Properties map[string]yamlSchemaProperty `yaml:"properties"`
}

type yamlSchemaProperty struct {
	Type   string `yaml:"type"`
	Format string `yaml:"format"`
}
