// SPDX-License-Identifier: Apache-2.0

// Package mpr - Writer for agent-editor Knowledge Base documents.
package mpr

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/agenteditor"
)

// CreateAgentEditorKnowledgeBase writes a Knowledge Base document.
func (w *Writer) CreateAgentEditorKnowledgeBase(k *agenteditor.KnowledgeBase) error {
	if k == nil {
		return fmt.Errorf("knowledge base is nil")
	}
	if k.Name == "" {
		return fmt.Errorf("knowledge base name is required")
	}
	if k.ContainerID == "" {
		return fmt.Errorf("knowledge base container ID is required")
	}
	if k.Provider == "" {
		k.Provider = "MxCloudGenAI"
	}
	if k.ID == "" {
		k.ID = model.ID(generateUUID())
	}

	contentsJSON, err := encodeKnowledgeBaseContents(k)
	if err != nil {
		return err
	}

	return w.writeCustomBlobDocument(customBlobInput{
		UnitID:             string(k.ID),
		ContainerID:        string(k.ContainerID),
		Name:               k.Name,
		Documentation:      k.Documentation,
		Excluded:           k.Excluded,
		ExportLevel:        k.ExportLevel,
		CustomDocumentType: agenteditor.CustomTypeKnowledgeBase,
		ReadableTypeName:   agenteditor.ReadableKnowledgeBase,
		ContentsJSON:       contentsJSON,
	})
}

// UpdateAgentEditorKnowledgeBase replaces an existing Knowledge Base document, preserving its UUID.
func (w *Writer) UpdateAgentEditorKnowledgeBase(k *agenteditor.KnowledgeBase) error {
	if k == nil {
		return fmt.Errorf("knowledge base is nil")
	}
	if k.Provider == "" {
		k.Provider = "MxCloudGenAI"
	}

	contentsJSON, err := encodeKnowledgeBaseContents(k)
	if err != nil {
		return err
	}

	return w.updateCustomBlobDocument(customBlobInput{
		UnitID:             string(k.ID),
		ContainerID:        string(k.ContainerID),
		Name:               k.Name,
		Documentation:      k.Documentation,
		Excluded:           k.Excluded,
		ExportLevel:        k.ExportLevel,
		CustomDocumentType: agenteditor.CustomTypeKnowledgeBase,
		ReadableTypeName:   agenteditor.ReadableKnowledgeBase,
		ContentsJSON:       contentsJSON,
	})
}

// DeleteAgentEditorKnowledgeBase removes a Knowledge Base by ID.
func (w *Writer) DeleteAgentEditorKnowledgeBase(id string) error {
	return w.deleteUnit(id)
}

func encodeKnowledgeBaseContents(k *agenteditor.KnowledgeBase) (string, error) {
	type providerFields struct {
		Environment      string                   `json:"environment"`
		DeepLinkURL      string                   `json:"deepLinkURL"`
		KeyID            string                   `json:"keyId"`
		KeyName          string                   `json:"keyName"`
		ModelDisplayName string                   `json:"modelDisplayName"`
		ModelName        string                   `json:"modelName"`
		Key              *agenteditor.ConstantRef `json:"key,omitempty"`
	}
	type contentsShape struct {
		Name           string         `json:"name"`
		Provider       string         `json:"provider"`
		ProviderFields providerFields `json:"providerFields"`
	}
	payload := contentsShape{
		Name:     "",
		Provider: k.Provider,
		ProviderFields: providerFields{
			Environment:      k.Environment,
			DeepLinkURL:      k.DeepLinkURL,
			KeyID:            k.KeyID,
			KeyName:          k.KeyName,
			ModelDisplayName: k.ModelDisplayName,
			ModelName:        k.ModelName,
			Key:              k.Key,
		},
	}
	return marshalCanonicalJSON(payload)
}
