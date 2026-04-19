// SPDX-License-Identifier: Apache-2.0

package types

import (
	"fmt"
	"sort"
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

	// Resolve messages from components (sorted for deterministic output)
	messageNames := make([]string, 0, len(raw.Components.Messages))
	for name := range raw.Components.Messages {
		messageNames = append(messageNames, name)
	}
	sort.Strings(messageNames)
	for _, name := range messageNames {
		msg := raw.Components.Messages[name]
		resolved := &AsyncAPIMessage{
			Name:        name,
			Title:       msg.Title,
			Description: msg.Description,
			ContentType: msg.ContentType,
		}

		// Resolve payload schema (follow $ref if present)
		schemaName := ""
		if msg.Payload.Ref != "" {
			schemaName = asyncRefName(msg.Payload.Ref)
		}

		if schemaName != "" {
			if schema, ok := raw.Components.Schemas[schemaName]; ok {
				resolved.Properties = resolveAsyncSchemaProperties(schema)
			}
		} else if msg.Payload.Properties != nil {
			// Inline schema
			resolved.Properties = resolveAsyncSchemaProperties(msg.Payload)
		}

		doc.Messages = append(doc.Messages, resolved)
	}

	// Resolve channels (sorted for deterministic output)
	channelNames := make([]string, 0, len(raw.Channels))
	for name := range raw.Channels {
		channelNames = append(channelNames, name)
	}
	sort.Strings(channelNames)
	for _, channelName := range channelNames {
		channel := raw.Channels[channelName]
		if channel.Subscribe != nil {
			msgName := ""
			if channel.Subscribe.Message.Ref != "" {
				msgName = asyncRefName(channel.Subscribe.Message.Ref)
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
				msgName = asyncRefName(channel.Publish.Message.Ref)
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

// asyncRefName extracts the last segment from a $ref like "#/components/messages/OrderChangedEvent".
func asyncRefName(ref string) string {
	if idx := strings.LastIndex(ref, "/"); idx >= 0 {
		return ref[idx+1:]
	}
	return ref
}

func resolveAsyncSchemaProperties(schema yamlAsyncSchema) []*AsyncAPIProperty {
	// Sort property names for deterministic output
	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}
	sort.Strings(names)

	props := make([]*AsyncAPIProperty, 0, len(names))
	for _, name := range names {
		prop := schema.Properties[name]
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
	AsyncAPI   string                      `yaml:"asyncapi"`
	Info       yamlAsyncInfo               `yaml:"info"`
	Channels   map[string]yamlAsyncChannel `yaml:"channels"`
	Components yamlAsyncComponents         `yaml:"components"`
}

type yamlAsyncInfo struct {
	Title       string `yaml:"title"`
	Version     string `yaml:"version"`
	Description string `yaml:"description"`
}

type yamlAsyncChannel struct {
	Subscribe *yamlAsyncOperation `yaml:"subscribe"`
	Publish   *yamlAsyncOperation `yaml:"publish"`
}

type yamlAsyncOperation struct {
	OperationID string       `yaml:"operationId"`
	Message     yamlAsyncRef `yaml:"message"`
}

type yamlAsyncRef struct {
	Ref string `yaml:"$ref"`
}

type yamlAsyncComponents struct {
	Messages map[string]yamlAsyncMessage `yaml:"messages"`
	Schemas  map[string]yamlAsyncSchema  `yaml:"schemas"`
}

type yamlAsyncMessage struct {
	Name        string          `yaml:"name"`
	Title       string          `yaml:"title"`
	Description string          `yaml:"description"`
	ContentType string          `yaml:"contentType"`
	Payload     yamlAsyncSchema `yaml:"payload"`
}

type yamlAsyncSchema struct {
	Ref        string                             `yaml:"$ref"`
	Type       string                             `yaml:"type"`
	Properties map[string]yamlAsyncSchemaProperty `yaml:"properties"`
}

type yamlAsyncSchemaProperty struct {
	Type   string `yaml:"type"`
	Format string `yaml:"format"`
}
