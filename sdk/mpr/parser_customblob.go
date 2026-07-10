// SPDX-License-Identifier: Apache-2.0

// Package mpr - Parsing of CustomBlobDocuments$CustomBlobDocument units.
//
// The agent-editor Studio Pro extension (Mendix 11.9+) stores all of its
// documents — Agent, Model, Knowledge Base, Consumed MCP Service — as
// generic CustomBlobDocument units. They share the same BSON wrapper and
// are discriminated by the CustomDocumentType field. The actual document
// payload lives in a JSON string in the Contents field.
//
// This file provides the generic wrapper decode plus type-specific
// decoders for each inner JSON schema.
package mpr

import (
	"encoding/json"
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/agenteditor"

	"go.mongodb.org/mongo-driver/bson"
)

// customBlobDocType is the BSON $Type of the wrapper.
const customBlobDocType = "CustomBlobDocuments$CustomBlobDocument"

// rawCustomBlobDoc is the decoded BSON wrapper (fields we care about).
type rawCustomBlobDoc struct {
	Name               string
	Documentation      string
	Excluded           bool
	ExportLevel        string
	CustomDocumentType string
	Contents           string // JSON payload
}

// parseCustomBlobWrapper decodes the outer CustomBlobDocument BSON wrapper.
// Returns a rawCustomBlobDoc or an error.
func parseCustomBlobWrapper(contents []byte) (*rawCustomBlobDoc, error) {
	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal CustomBlobDocument BSON: %w", err)
	}

	out := &rawCustomBlobDoc{}
	if v, ok := raw["Name"].(string); ok {
		out.Name = v
	}
	if v, ok := raw["Documentation"].(string); ok {
		out.Documentation = v
	}
	if v, ok := raw["Excluded"].(bool); ok {
		out.Excluded = v
	}
	if v, ok := raw["ExportLevel"].(string); ok {
		out.ExportLevel = v
	}
	if v, ok := raw["CustomDocumentType"].(string); ok {
		out.CustomDocumentType = v
	}
	if v, ok := raw["Contents"].(string); ok {
		out.Contents = v
	}
	return out, nil
}

// parseAgentEditorModel parses a CustomBlobDocument with
// CustomDocumentType == "agenteditor.model" into an agenteditor.Model.
func (r *Reader) parseAgentEditorModel(unitID, containerID string, contents []byte) (*agenteditor.Model, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	wrap, err := parseCustomBlobWrapper(contents)
	if err != nil {
		return nil, err
	}
	if wrap.CustomDocumentType != agenteditor.CustomTypeModel {
		return nil, fmt.Errorf("unit %s is not an agent-editor model (CustomDocumentType=%q)",
			unitID, wrap.CustomDocumentType)
	}

	m := &agenteditor.Model{}
	m.ID = model.ID(unitID)
	m.TypeName = customBlobDocType
	m.ContainerID = model.ID(containerID)
	m.Name = wrap.Name
	m.Documentation = wrap.Documentation
	m.Excluded = wrap.Excluded
	m.ExportLevel = wrap.ExportLevel

	// Decode the Contents JSON payload.
	if wrap.Contents != "" {
		var payload struct {
			Type           string `json:"type"`
			Name           string `json:"name"`
			DisplayName    string `json:"displayName"`
			Provider       string `json:"provider"`
			ProviderFields struct {
				Environment  string                   `json:"environment"`
				DeepLinkURL  string                   `json:"deepLinkURL"`
				KeyID        string                   `json:"keyId"`
				KeyName      string                   `json:"keyName"`
				ResourceName string                   `json:"resourceName"`
				Key          *agenteditor.ConstantRef `json:"key"`
			} `json:"providerFields"`
		}
		if err := json.Unmarshal([]byte(wrap.Contents), &payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal agent-editor Model Contents JSON: %w", err)
		}

		m.Type = payload.Type
		m.InnerName = payload.Name
		m.DisplayName = payload.DisplayName
		m.Provider = payload.Provider
		m.Environment = payload.ProviderFields.Environment
		m.DeepLinkURL = payload.ProviderFields.DeepLinkURL
		m.KeyID = payload.ProviderFields.KeyID
		m.KeyName = payload.ProviderFields.KeyName
		m.ResourceName = payload.ProviderFields.ResourceName
		m.Key = payload.ProviderFields.Key
	}

	return m, nil
}

// parseAgentEditorKnowledgeBase parses a CustomBlobDocument with
// CustomDocumentType == "agenteditor.knowledgebase" into an
// agenteditor.KnowledgeBase.
func (r *Reader) parseAgentEditorKnowledgeBase(unitID, containerID string, contents []byte) (*agenteditor.KnowledgeBase, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	wrap, err := parseCustomBlobWrapper(contents)
	if err != nil {
		return nil, err
	}
	if wrap.CustomDocumentType != agenteditor.CustomTypeKnowledgeBase {
		return nil, fmt.Errorf("unit %s is not an agent-editor knowledge base (CustomDocumentType=%q)",
			unitID, wrap.CustomDocumentType)
	}

	k := &agenteditor.KnowledgeBase{}
	k.ID = model.ID(unitID)
	k.TypeName = customBlobDocType
	k.ContainerID = model.ID(containerID)
	k.Name = wrap.Name
	k.Documentation = wrap.Documentation
	k.Excluded = wrap.Excluded
	k.ExportLevel = wrap.ExportLevel

	if wrap.Contents != "" {
		var payload struct {
			Name           string `json:"name"`
			Provider       string `json:"provider"`
			ProviderFields struct {
				Environment      string                   `json:"environment"`
				DeepLinkURL      string                   `json:"deepLinkURL"`
				KeyID            string                   `json:"keyId"`
				KeyName          string                   `json:"keyName"`
				ModelDisplayName string                   `json:"modelDisplayName"`
				ModelName        string                   `json:"modelName"`
				Key              *agenteditor.ConstantRef `json:"key"`
			} `json:"providerFields"`
		}
		if err := json.Unmarshal([]byte(wrap.Contents), &payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal agent-editor KnowledgeBase Contents JSON: %w", err)
		}
		k.Provider = payload.Provider
		k.Environment = payload.ProviderFields.Environment
		k.DeepLinkURL = payload.ProviderFields.DeepLinkURL
		k.KeyID = payload.ProviderFields.KeyID
		k.KeyName = payload.ProviderFields.KeyName
		k.ModelDisplayName = payload.ProviderFields.ModelDisplayName
		k.ModelName = payload.ProviderFields.ModelName
		k.Key = payload.ProviderFields.Key
	}

	return k, nil
}

// parseAgentEditorConsumedMCPService parses a CustomBlobDocument with
// CustomDocumentType == "agenteditor.consumedMCPService" into an
// agenteditor.ConsumedMCPService.
func (r *Reader) parseAgentEditorConsumedMCPService(unitID, containerID string, contents []byte) (*agenteditor.ConsumedMCPService, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	wrap, err := parseCustomBlobWrapper(contents)
	if err != nil {
		return nil, err
	}
	if wrap.CustomDocumentType != agenteditor.CustomTypeConsumedMCPService {
		return nil, fmt.Errorf("unit %s is not an agent-editor consumed MCP service (CustomDocumentType=%q)",
			unitID, wrap.CustomDocumentType)
	}

	c := &agenteditor.ConsumedMCPService{}
	c.ID = model.ID(unitID)
	c.TypeName = customBlobDocType
	c.ContainerID = model.ID(containerID)
	c.Name = wrap.Name
	c.Documentation = wrap.Documentation
	c.Excluded = wrap.Excluded
	c.ExportLevel = wrap.ExportLevel

	if wrap.Contents != "" {
		var payload struct {
			ProtocolVersion          string `json:"protocolVersion"`
			Documentation            string `json:"documentation"`
			Version                  string `json:"version"`
			ConnectionTimeoutSeconds int    `json:"connectionTimeoutSeconds"`
		}
		if err := json.Unmarshal([]byte(wrap.Contents), &payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal agent-editor ConsumedMCPService Contents JSON: %w", err)
		}
		c.ProtocolVersion = payload.ProtocolVersion
		c.InnerDocumentation = payload.Documentation
		c.Version = payload.Version
		c.ConnectionTimeoutSeconds = payload.ConnectionTimeoutSeconds
	}

	return c, nil
}

// parseAgentEditorAgent parses a CustomBlobDocument with
// CustomDocumentType == "agenteditor.agent" into an agenteditor.Agent.
func (r *Reader) parseAgentEditorAgent(unitID, containerID string, contents []byte) (*agenteditor.Agent, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	wrap, err := parseCustomBlobWrapper(contents)
	if err != nil {
		return nil, err
	}
	if wrap.CustomDocumentType != agenteditor.CustomTypeAgent {
		return nil, fmt.Errorf("unit %s is not an agent-editor agent (CustomDocumentType=%q)",
			unitID, wrap.CustomDocumentType)
	}

	a := &agenteditor.Agent{}
	a.ID = model.ID(unitID)
	a.TypeName = customBlobDocType
	a.ContainerID = model.ID(containerID)
	a.Name = wrap.Name
	a.Documentation = wrap.Documentation
	a.Excluded = wrap.Excluded
	a.ExportLevel = wrap.ExportLevel

	if wrap.Contents != "" {
		// Decode the fields we know about; unknown fields are ignored so
		// the parser stays forward-compatible with editor updates.
		var payload struct {
			Description        string                    `json:"description"`
			SystemPrompt       string                    `json:"systemPrompt"`
			UserPrompt         string                    `json:"userPrompt"`
			UsageType          string                    `json:"usageType"`
			Variables          []agenteditor.AgentVar    `json:"variables"`
			Tools              []agenteditor.AgentTool   `json:"tools"`
			KnowledgebaseTools []agenteditor.AgentKBTool `json:"knowledgebaseTools"`
			Model              *agenteditor.DocRef       `json:"model"`
			Entity             *agenteditor.DocRef       `json:"entity"`
			MaxTokens          *int                      `json:"maxTokens"`
			ToolChoice         string                    `json:"toolChoice"`
			Temperature        *float64                  `json:"temperature"`
			TopP               *float64                  `json:"topP"`
		}
		if err := json.Unmarshal([]byte(wrap.Contents), &payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal agent-editor Agent Contents JSON: %w", err)
		}
		a.Description = payload.Description
		a.SystemPrompt = payload.SystemPrompt
		a.UserPrompt = payload.UserPrompt
		a.UsageType = payload.UsageType
		a.Variables = payload.Variables
		a.Tools = payload.Tools
		a.KBTools = payload.KnowledgebaseTools
		a.Model = payload.Model
		a.Entity = payload.Entity
		a.MaxTokens = payload.MaxTokens
		a.ToolChoice = payload.ToolChoice
		a.Temperature = payload.Temperature
		a.TopP = payload.TopP
	}

	return a, nil
}
