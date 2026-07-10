// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"encoding/json"
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
	"github.com/JordtenBulte-OLC/mxcli/sdk/agenteditor"
)

// customBlobDocType is the BSON $Type of the generic wrapper the agent-editor
// extension (Mendix 11.9+) stores all of its documents in. The document kind is
// discriminated by CustomDocumentType; the payload is a JSON string in Contents.
const customBlobDocType = "CustomBlobDocuments$CustomBlobDocument"

// customBlobDocToGen builds the CustomBlobDocument envelope (Contents JSON +
// metadata) as a codec element. Mirrors sdk/mpr.writeCustomBlobDocument.
func customBlobDocToGen(unitID, name, documentation, exportLevel string, excluded bool, customType, readableType, contentsJSON string) element.Element {
	g := newElem(customBlobDocType, unitID)
	addStr(g, "Contents", contentsJSON)
	addStr(g, "CustomDocumentType", customType)
	addStr(g, "Documentation", documentation)
	addBool(g, "Excluded", excluded)
	addStr(g, "ExportLevel", orDefault(exportLevel, "Hidden"))
	meta := newElem("CustomBlobDocuments$CustomBlobDocumentMetadata", "")
	addStr(meta, "CreatedByExtension", agenteditor.CreatedByExtensionID)
	addStr(meta, "ReadableTypeName", readableType)
	addPart(g, "Metadata", meta)
	addStr(g, "Name", name)
	return g
}

// writeCustomBlob encodes the envelope and inserts (create) or rewrites (update)
// the unit.
func (b *Backend) writeCustomBlob(unitID, containerID, name, documentation, exportLevel string, excluded bool, customType, readableType, contentsJSON string, update bool) error {
	el := customBlobDocToGen(unitID, name, documentation, exportLevel, excluded, customType, readableType, contentsJSON)
	raw, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		return fmt.Errorf("encode custom blob: %w", err)
	}
	if update {
		return b.writer.UpdateRawUnit(unitID, raw)
	}
	return b.writer.InsertUnit(unitID, containerID, "Documents", customBlobDocType, raw)
}

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

func (b *Backend) CreateAgentEditorModel(m *agenteditor.Model) error {
	if m == nil {
		return fmt.Errorf("CreateAgentEditorModel: nil model")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateAgentEditorModel: not connected for writing")
	}
	if m.Name == "" {
		return fmt.Errorf("CreateAgentEditorModel: name is required")
	}
	if m.ContainerID == "" {
		return fmt.Errorf("CreateAgentEditorModel: container ID is required")
	}
	if m.Provider == "" {
		m.Provider = "MxCloudGenAI"
	}
	if m.ID == "" {
		m.ID = model.ID(mmpr.GenerateID())
	}
	contents, err := encodeAgentEditorModelContents(m)
	if err != nil {
		return err
	}
	return b.writeCustomBlob(string(m.ID), string(m.ContainerID), m.Name, m.Documentation, m.ExportLevel, m.Excluded, agenteditor.CustomTypeModel, agenteditor.ReadableModel, contents, false)
}

func (b *Backend) UpdateAgentEditorModel(m *agenteditor.Model) error {
	if m == nil {
		return fmt.Errorf("UpdateAgentEditorModel: nil model")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateAgentEditorModel: not connected for writing")
	}
	if m.Provider == "" {
		m.Provider = "MxCloudGenAI"
	}
	contents, err := encodeAgentEditorModelContents(m)
	if err != nil {
		return err
	}
	return b.writeCustomBlob(string(m.ID), string(m.ContainerID), m.Name, m.Documentation, m.ExportLevel, m.Excluded, agenteditor.CustomTypeModel, agenteditor.ReadableModel, contents, true)
}

func (b *Backend) DeleteAgentEditorModel(id string) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteAgentEditorModel: not connected for writing")
	}
	return b.writer.DeleteUnit(id)
}

// ---------------------------------------------------------------------------
// Knowledge base
// ---------------------------------------------------------------------------

func (b *Backend) CreateAgentEditorKnowledgeBase(k *agenteditor.KnowledgeBase) error {
	if k == nil {
		return fmt.Errorf("CreateAgentEditorKnowledgeBase: nil knowledge base")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateAgentEditorKnowledgeBase: not connected for writing")
	}
	if k.Name == "" {
		return fmt.Errorf("CreateAgentEditorKnowledgeBase: name is required")
	}
	if k.ContainerID == "" {
		return fmt.Errorf("CreateAgentEditorKnowledgeBase: container ID is required")
	}
	if k.ID == "" {
		k.ID = model.ID(mmpr.GenerateID())
	}
	contents, err := encodeKnowledgeBaseContents(k)
	if err != nil {
		return err
	}
	return b.writeCustomBlob(string(k.ID), string(k.ContainerID), k.Name, k.Documentation, k.ExportLevel, k.Excluded, agenteditor.CustomTypeKnowledgeBase, agenteditor.ReadableKnowledgeBase, contents, false)
}

func (b *Backend) UpdateAgentEditorKnowledgeBase(k *agenteditor.KnowledgeBase) error {
	if k == nil {
		return fmt.Errorf("UpdateAgentEditorKnowledgeBase: nil knowledge base")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateAgentEditorKnowledgeBase: not connected for writing")
	}
	contents, err := encodeKnowledgeBaseContents(k)
	if err != nil {
		return err
	}
	return b.writeCustomBlob(string(k.ID), string(k.ContainerID), k.Name, k.Documentation, k.ExportLevel, k.Excluded, agenteditor.CustomTypeKnowledgeBase, agenteditor.ReadableKnowledgeBase, contents, true)
}

func (b *Backend) DeleteAgentEditorKnowledgeBase(id string) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteAgentEditorKnowledgeBase: not connected for writing")
	}
	return b.writer.DeleteUnit(id)
}

// ---------------------------------------------------------------------------
// Consumed MCP service
// ---------------------------------------------------------------------------

func (b *Backend) CreateAgentEditorConsumedMCPService(c *agenteditor.ConsumedMCPService) error {
	if c == nil {
		return fmt.Errorf("CreateAgentEditorConsumedMCPService: nil service")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateAgentEditorConsumedMCPService: not connected for writing")
	}
	if c.Name == "" {
		return fmt.Errorf("CreateAgentEditorConsumedMCPService: name is required")
	}
	if c.ContainerID == "" {
		return fmt.Errorf("CreateAgentEditorConsumedMCPService: container ID is required")
	}
	if c.ID == "" {
		c.ID = model.ID(mmpr.GenerateID())
	}
	contents, err := encodeConsumedMCPServiceContents(c)
	if err != nil {
		return err
	}
	return b.writeCustomBlob(string(c.ID), string(c.ContainerID), c.Name, c.Documentation, c.ExportLevel, c.Excluded, agenteditor.CustomTypeConsumedMCPService, agenteditor.ReadableConsumedMCPService, contents, false)
}

func (b *Backend) UpdateAgentEditorConsumedMCPService(c *agenteditor.ConsumedMCPService) error {
	if c == nil {
		return fmt.Errorf("UpdateAgentEditorConsumedMCPService: nil service")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateAgentEditorConsumedMCPService: not connected for writing")
	}
	contents, err := encodeConsumedMCPServiceContents(c)
	if err != nil {
		return err
	}
	return b.writeCustomBlob(string(c.ID), string(c.ContainerID), c.Name, c.Documentation, c.ExportLevel, c.Excluded, agenteditor.CustomTypeConsumedMCPService, agenteditor.ReadableConsumedMCPService, contents, true)
}

func (b *Backend) DeleteAgentEditorConsumedMCPService(id string) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteAgentEditorConsumedMCPService: not connected for writing")
	}
	return b.writer.DeleteUnit(id)
}

// ---------------------------------------------------------------------------
// Agent
// ---------------------------------------------------------------------------

func (b *Backend) CreateAgentEditorAgent(a *agenteditor.Agent) error {
	if a == nil {
		return fmt.Errorf("CreateAgentEditorAgent: nil agent")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateAgentEditorAgent: not connected for writing")
	}
	if a.Name == "" {
		return fmt.Errorf("CreateAgentEditorAgent: name is required")
	}
	if a.ContainerID == "" {
		return fmt.Errorf("CreateAgentEditorAgent: container ID is required")
	}
	if a.ID == "" {
		a.ID = model.ID(mmpr.GenerateID())
	}
	contents, err := encodeAgentContents(a)
	if err != nil {
		return err
	}
	return b.writeCustomBlob(string(a.ID), string(a.ContainerID), a.Name, a.Documentation, a.ExportLevel, a.Excluded, agenteditor.CustomTypeAgent, agenteditor.ReadableAgent, contents, false)
}

func (b *Backend) UpdateAgentEditorAgent(a *agenteditor.Agent) error {
	if a == nil {
		return fmt.Errorf("UpdateAgentEditorAgent: nil agent")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateAgentEditorAgent: not connected for writing")
	}
	contents, err := encodeAgentContents(a)
	if err != nil {
		return err
	}
	return b.writeCustomBlob(string(a.ID), string(a.ContainerID), a.Name, a.Documentation, a.ExportLevel, a.Excluded, agenteditor.CustomTypeAgent, agenteditor.ReadableAgent, contents, true)
}

func (b *Backend) DeleteAgentEditorAgent(id string) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteAgentEditorAgent: not connected for writing")
	}
	return b.writer.DeleteUnit(id)
}

// ---------------------------------------------------------------------------
// Contents JSON encoders — mirror sdk/mpr field-for-field (key order matters:
// Studio Pro emits the providerFields nested object in this exact order).
// ---------------------------------------------------------------------------

func encodeAgentEditorModelContents(m *agenteditor.Model) (string, error) {
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
	return marshalJSON(contentsShape{
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
	})
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
	return marshalJSON(contentsShape{
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
	})
}

func encodeConsumedMCPServiceContents(c *agenteditor.ConsumedMCPService) (string, error) {
	type contentsShape struct {
		ProtocolVersion          string `json:"protocolVersion"`
		Documentation            string `json:"documentation"`
		Version                  string `json:"version"`
		ConnectionTimeoutSeconds int    `json:"connectionTimeoutSeconds"`
	}
	return marshalJSON(contentsShape{
		ProtocolVersion:          c.ProtocolVersion,
		Documentation:            c.InnerDocumentation,
		Version:                  c.Version,
		ConnectionTimeoutSeconds: c.ConnectionTimeoutSeconds,
	})
}

func encodeAgentContents(a *agenteditor.Agent) (string, error) {
	type toolEntry struct {
		ID          string              `json:"id"`
		Name        string              `json:"name"`
		Description string              `json:"description"`
		Enabled     bool                `json:"enabled"`
		ToolType    string              `json:"toolType"`
		Document    *agenteditor.DocRef `json:"document,omitempty"`
	}
	type kbToolEntry struct {
		ID                   string              `json:"id"`
		Name                 string              `json:"name"`
		Description          string              `json:"description"`
		Enabled              bool                `json:"enabled"`
		ToolType             string              `json:"toolType"`
		Document             *agenteditor.DocRef `json:"document,omitempty"`
		CollectionIdentifier string              `json:"collectionIdentifier,omitempty"`
		MaxResults           int                 `json:"maxResults,omitempty"`
	}
	type contentsShape struct {
		Description        string                 `json:"description"`
		SystemPrompt       string                 `json:"systemPrompt"`
		UserPrompt         string                 `json:"userPrompt"`
		UsageType          string                 `json:"usageType"`
		Variables          []agenteditor.AgentVar `json:"variables"`
		Tools              []toolEntry            `json:"tools"`
		KnowledgebaseTools []kbToolEntry          `json:"knowledgebaseTools"`
		Model              *agenteditor.DocRef    `json:"model,omitempty"`
		Entity             *agenteditor.DocRef    `json:"entity,omitempty"`
		MaxTokens          *int                   `json:"maxTokens,omitempty"`
		ToolChoice         string                 `json:"toolChoice,omitempty"`
		Temperature        *float64               `json:"temperature,omitempty"`
		TopP               *float64               `json:"topP,omitempty"`
	}

	tools := make([]toolEntry, 0, len(a.Tools))
	for _, t := range a.Tools {
		tools = append(tools, toolEntry{ID: t.ID, Name: t.Name, Description: t.Description, Enabled: t.Enabled, ToolType: t.ToolType, Document: t.Document})
	}
	kbTools := make([]kbToolEntry, 0, len(a.KBTools))
	for _, kb := range a.KBTools {
		kbTools = append(kbTools, kbToolEntry{ID: kb.ID, Name: kb.Name, Description: kb.Description, Enabled: kb.Enabled, ToolType: kb.ToolType, Document: kb.Document, CollectionIdentifier: kb.CollectionIdentifier, MaxResults: kb.MaxResults})
	}
	vars := a.Variables
	if vars == nil {
		vars = []agenteditor.AgentVar{}
	}
	return marshalJSON(contentsShape{
		Description:        a.Description,
		SystemPrompt:       a.SystemPrompt,
		UserPrompt:         a.UserPrompt,
		UsageType:          a.UsageType,
		Variables:          vars,
		Tools:              tools,
		KnowledgebaseTools: kbTools,
		Model:              a.Model,
		Entity:             a.Entity,
		MaxTokens:          a.MaxTokens,
		ToolChoice:         a.ToolChoice,
		Temperature:        a.Temperature,
		TopP:               a.TopP,
	})
}

func marshalJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
