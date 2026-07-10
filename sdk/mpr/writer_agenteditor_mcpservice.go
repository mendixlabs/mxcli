// SPDX-License-Identifier: Apache-2.0

// Package mpr - Writer for agent-editor Consumed MCP Service documents.
package mpr

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/agenteditor"
)

// CreateAgentEditorConsumedMCPService writes a Consumed MCP Service document.
func (w *Writer) CreateAgentEditorConsumedMCPService(c *agenteditor.ConsumedMCPService) error {
	if c == nil {
		return fmt.Errorf("consumed MCP service is nil")
	}
	if c.Name == "" {
		return fmt.Errorf("consumed MCP service name is required")
	}
	if c.ContainerID == "" {
		return fmt.Errorf("consumed MCP service container ID is required")
	}
	if c.ID == "" {
		c.ID = model.ID(generateUUID())
	}

	contentsJSON, err := encodeConsumedMCPServiceContents(c)
	if err != nil {
		return err
	}

	return w.writeCustomBlobDocument(customBlobInput{
		UnitID:             string(c.ID),
		ContainerID:        string(c.ContainerID),
		Name:               c.Name,
		Documentation:      c.Documentation,
		Excluded:           c.Excluded,
		ExportLevel:        c.ExportLevel,
		CustomDocumentType: agenteditor.CustomTypeConsumedMCPService,
		ReadableTypeName:   agenteditor.ReadableConsumedMCPService,
		ContentsJSON:       contentsJSON,
	})
}

// UpdateAgentEditorConsumedMCPService replaces an existing Consumed MCP Service document, preserving its UUID.
func (w *Writer) UpdateAgentEditorConsumedMCPService(c *agenteditor.ConsumedMCPService) error {
	if c == nil {
		return fmt.Errorf("consumed MCP service is nil")
	}

	contentsJSON, err := encodeConsumedMCPServiceContents(c)
	if err != nil {
		return err
	}

	return w.updateCustomBlobDocument(customBlobInput{
		UnitID:             string(c.ID),
		ContainerID:        string(c.ContainerID),
		Name:               c.Name,
		Documentation:      c.Documentation,
		Excluded:           c.Excluded,
		ExportLevel:        c.ExportLevel,
		CustomDocumentType: agenteditor.CustomTypeConsumedMCPService,
		ReadableTypeName:   agenteditor.ReadableConsumedMCPService,
		ContentsJSON:       contentsJSON,
	})
}

// DeleteAgentEditorConsumedMCPService removes a Consumed MCP Service by ID.
func (w *Writer) DeleteAgentEditorConsumedMCPService(id string) error {
	return w.deleteUnit(id)
}

func encodeConsumedMCPServiceContents(c *agenteditor.ConsumedMCPService) (string, error) {
	type contentsShape struct {
		ProtocolVersion          string `json:"protocolVersion"`
		Documentation            string `json:"documentation"`
		Version                  string `json:"version"`
		ConnectionTimeoutSeconds int    `json:"connectionTimeoutSeconds"`
	}
	payload := contentsShape{
		ProtocolVersion:          c.ProtocolVersion,
		Documentation:            c.InnerDocumentation,
		Version:                  c.Version,
		ConnectionTimeoutSeconds: c.ConnectionTimeoutSeconds,
	}
	return marshalCanonicalJSON(payload)
}
