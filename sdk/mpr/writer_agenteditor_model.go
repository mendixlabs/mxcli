// SPDX-License-Identifier: Apache-2.0

// Package mpr - Writer for agent-editor Model documents.
package mpr

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/agenteditor"
)

// CreateAgentEditorModel writes a Model document to the project. The
// Model.ContainerID must be set (the module/folder to place it in).
// The Model.ID is auto-generated if empty.
//
// The Contents JSON shape mirrors what Studio Pro's agent-editor
// extension produces — see PROPOSAL_agent_document_support.md.
func (w *Writer) CreateAgentEditorModel(m *agenteditor.Model) error {
	if m == nil {
		return fmt.Errorf("model is nil")
	}
	if m.Name == "" {
		return fmt.Errorf("model name is required")
	}
	if m.ContainerID == "" {
		return fmt.Errorf("model container ID is required")
	}
	if m.Provider == "" {
		// Only one provider is currently supported by the agent editor.
		m.Provider = "MxCloudGenAI"
	}
	if m.ID == "" {
		m.ID = model.ID(generateUUID())
	}

	contentsJSON, err := encodeAgentEditorModelContents(m)
	if err != nil {
		return err
	}

	return w.writeCustomBlobDocument(customBlobInput{
		UnitID:             string(m.ID),
		ContainerID:        string(m.ContainerID),
		Name:               m.Name,
		Documentation:      m.Documentation,
		Excluded:           m.Excluded,
		ExportLevel:        m.ExportLevel,
		CustomDocumentType: agenteditor.CustomTypeModel,
		ReadableTypeName:   agenteditor.ReadableModel,
		ContentsJSON:       contentsJSON,
	})
}

// UpdateAgentEditorModel replaces an existing Model document, preserving its UUID.
func (w *Writer) UpdateAgentEditorModel(m *agenteditor.Model) error {
	if m == nil {
		return fmt.Errorf("model is nil")
	}
	if m.Provider == "" {
		m.Provider = "MxCloudGenAI"
	}

	contentsJSON, err := encodeAgentEditorModelContents(m)
	if err != nil {
		return err
	}

	return w.updateCustomBlobDocument(customBlobInput{
		UnitID:             string(m.ID),
		ContainerID:        string(m.ContainerID),
		Name:               m.Name,
		Documentation:      m.Documentation,
		Excluded:           m.Excluded,
		ExportLevel:        m.ExportLevel,
		CustomDocumentType: agenteditor.CustomTypeModel,
		ReadableTypeName:   agenteditor.ReadableModel,
		ContentsJSON:       contentsJSON,
	})
}

// DeleteAgentEditorModel removes a Model document by ID.
func (w *Writer) DeleteAgentEditorModel(id string) error {
	return w.deleteUnit(id)
}

// encodeAgentEditorModelContents produces the JSON payload stored in
// the Contents field of a Model CustomBlobDocument.
func encodeAgentEditorModelContents(m *agenteditor.Model) (string, error) {
	// Provider-specific fields are nested under providerFields. Keys are
	// emitted in the same order Studio Pro uses.
	type providerFields struct {
		Environment  string                   `json:"environment"`
		DeepLinkURL  string                   `json:"deepLinkURL"`
		KeyID        string                   `json:"keyId"`
		KeyName      string                   `json:"keyName"`
		ResourceName string                   `json:"resourceName"`
		Key          *agenteditor.ConstantRef `json:"key,omitempty"`
	}
	type contentsShape struct {
		Type           string         `json:"type"`
		Name           string         `json:"name"`
		DisplayName    string         `json:"displayName"`
		Provider       string         `json:"provider"`
		ProviderFields providerFields `json:"providerFields"`
	}

	payload := contentsShape{
		Type:        m.Type,
		Name:        m.InnerName,
		DisplayName: m.DisplayName,
		Provider:    m.Provider,
		ProviderFields: providerFields{
			Environment:  m.Environment,
			DeepLinkURL:  m.DeepLinkURL,
			KeyID:        m.KeyID,
			KeyName:      m.KeyName,
			ResourceName: m.ResourceName,
			Key:          m.Key,
		},
	}

	return marshalCanonicalJSON(payload)
}
