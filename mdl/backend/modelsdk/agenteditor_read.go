// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"encoding/json"
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/agenteditor"
	"go.mongodb.org/mongo-driver/bson"
)

// rawCustomBlob is the decoded CustomBlobDocument wrapper (the fields the
// agent-editor doc types care about). Contents holds the per-type JSON payload.
type rawCustomBlob struct {
	Name               string
	Documentation      string
	Excluded           bool
	ExportLevel        string
	CustomDocumentType string
	Contents           string
}

// parseCustomBlobWrapper decodes the outer CustomBlobDocument BSON wrapper.
func parseCustomBlobWrapper(contents []byte) (*rawCustomBlob, error) {
	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal CustomBlobDocument BSON: %w", err)
	}
	out := &rawCustomBlob{}
	out.Name, _ = raw["Name"].(string)
	out.Documentation, _ = raw["Documentation"].(string)
	out.Excluded, _ = raw["Excluded"].(bool)
	out.ExportLevel, _ = raw["ExportLevel"].(string)
	out.CustomDocumentType, _ = raw["CustomDocumentType"].(string)
	out.Contents, _ = raw["Contents"].(string)
	return out, nil
}

// listCustomBlobs returns every CustomBlobDocument unit whose CustomDocumentType
// matches the given kind, paired with its unit/container ID and parsed wrapper.
func (b *Backend) listCustomBlobs(customType string) ([]customBlobUnit, error) {
	units, err := b.reader.ListRawUnitsByType(customBlobDocType)
	if err != nil {
		return nil, err
	}
	var out []customBlobUnit
	for _, u := range units {
		wrap, err := parseCustomBlobWrapper(u.Contents)
		if err != nil {
			return nil, err
		}
		if wrap.CustomDocumentType != customType {
			continue
		}
		out = append(out, customBlobUnit{unitID: string(u.ID), containerID: string(u.ContainerID), wrap: wrap})
	}
	return out, nil
}

type customBlobUnit struct {
	unitID      string
	containerID string
	wrap        *rawCustomBlob
}

func (b *Backend) ListAgentEditorModels() ([]*agenteditor.Model, error) {
	units, err := b.listCustomBlobs(agenteditor.CustomTypeModel)
	if err != nil {
		return nil, err
	}
	out := make([]*agenteditor.Model, 0, len(units))
	for _, u := range units {
		m := &agenteditor.Model{}
		m.ID = model.ID(u.unitID)
		m.TypeName = customBlobDocType
		m.ContainerID = model.ID(u.containerID)
		m.Name = u.wrap.Name
		m.Documentation = u.wrap.Documentation
		m.Excluded = u.wrap.Excluded
		m.ExportLevel = u.wrap.ExportLevel
		if u.wrap.Contents != "" {
			var p struct {
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
			if err := json.Unmarshal([]byte(u.wrap.Contents), &p); err != nil {
				return nil, fmt.Errorf("unmarshal Model Contents: %w", err)
			}
			m.Type = p.Type
			m.InnerName = p.Name
			m.DisplayName = p.DisplayName
			m.Provider = p.Provider
			m.Environment = p.ProviderFields.Environment
			m.DeepLinkURL = p.ProviderFields.DeepLinkURL
			m.KeyID = p.ProviderFields.KeyID
			m.KeyName = p.ProviderFields.KeyName
			m.ResourceName = p.ProviderFields.ResourceName
			m.Key = p.ProviderFields.Key
		}
		out = append(out, m)
	}
	return out, nil
}

func (b *Backend) ListAgentEditorKnowledgeBases() ([]*agenteditor.KnowledgeBase, error) {
	units, err := b.listCustomBlobs(agenteditor.CustomTypeKnowledgeBase)
	if err != nil {
		return nil, err
	}
	out := make([]*agenteditor.KnowledgeBase, 0, len(units))
	for _, u := range units {
		k := &agenteditor.KnowledgeBase{}
		k.ID = model.ID(u.unitID)
		k.TypeName = customBlobDocType
		k.ContainerID = model.ID(u.containerID)
		k.Name = u.wrap.Name
		k.Documentation = u.wrap.Documentation
		k.Excluded = u.wrap.Excluded
		k.ExportLevel = u.wrap.ExportLevel
		if u.wrap.Contents != "" {
			var p struct {
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
			if err := json.Unmarshal([]byte(u.wrap.Contents), &p); err != nil {
				return nil, fmt.Errorf("unmarshal KnowledgeBase Contents: %w", err)
			}
			k.Provider = p.Provider
			k.Environment = p.ProviderFields.Environment
			k.DeepLinkURL = p.ProviderFields.DeepLinkURL
			k.KeyID = p.ProviderFields.KeyID
			k.KeyName = p.ProviderFields.KeyName
			k.ModelDisplayName = p.ProviderFields.ModelDisplayName
			k.ModelName = p.ProviderFields.ModelName
			k.Key = p.ProviderFields.Key
		}
		out = append(out, k)
	}
	return out, nil
}

func (b *Backend) ListAgentEditorConsumedMCPServices() ([]*agenteditor.ConsumedMCPService, error) {
	units, err := b.listCustomBlobs(agenteditor.CustomTypeConsumedMCPService)
	if err != nil {
		return nil, err
	}
	out := make([]*agenteditor.ConsumedMCPService, 0, len(units))
	for _, u := range units {
		c := &agenteditor.ConsumedMCPService{}
		c.ID = model.ID(u.unitID)
		c.TypeName = customBlobDocType
		c.ContainerID = model.ID(u.containerID)
		c.Name = u.wrap.Name
		c.Documentation = u.wrap.Documentation
		c.Excluded = u.wrap.Excluded
		c.ExportLevel = u.wrap.ExportLevel
		if u.wrap.Contents != "" {
			var p struct {
				ProtocolVersion          string `json:"protocolVersion"`
				Documentation            string `json:"documentation"`
				Version                  string `json:"version"`
				ConnectionTimeoutSeconds int    `json:"connectionTimeoutSeconds"`
			}
			if err := json.Unmarshal([]byte(u.wrap.Contents), &p); err != nil {
				return nil, fmt.Errorf("unmarshal ConsumedMCPService Contents: %w", err)
			}
			c.ProtocolVersion = p.ProtocolVersion
			c.InnerDocumentation = p.Documentation
			c.Version = p.Version
			c.ConnectionTimeoutSeconds = p.ConnectionTimeoutSeconds
		}
		out = append(out, c)
	}
	return out, nil
}

func (b *Backend) ListAgentEditorAgents() ([]*agenteditor.Agent, error) {
	units, err := b.listCustomBlobs(agenteditor.CustomTypeAgent)
	if err != nil {
		return nil, err
	}
	out := make([]*agenteditor.Agent, 0, len(units))
	for _, u := range units {
		a := &agenteditor.Agent{}
		a.ID = model.ID(u.unitID)
		a.TypeName = customBlobDocType
		a.ContainerID = model.ID(u.containerID)
		a.Name = u.wrap.Name
		a.Documentation = u.wrap.Documentation
		a.Excluded = u.wrap.Excluded
		a.ExportLevel = u.wrap.ExportLevel
		if u.wrap.Contents != "" {
			var p struct {
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
			if err := json.Unmarshal([]byte(u.wrap.Contents), &p); err != nil {
				return nil, fmt.Errorf("unmarshal Agent Contents: %w", err)
			}
			a.Description = p.Description
			a.SystemPrompt = p.SystemPrompt
			a.UserPrompt = p.UserPrompt
			a.UsageType = p.UsageType
			a.Variables = p.Variables
			a.Tools = p.Tools
			a.KBTools = p.KnowledgebaseTools
			a.Model = p.Model
			a.Entity = p.Entity
			a.MaxTokens = p.MaxTokens
			a.ToolChoice = p.ToolChoice
			a.Temperature = p.Temperature
			a.TopP = p.TopP
		}
		out = append(out, a)
	}
	return out, nil
}
