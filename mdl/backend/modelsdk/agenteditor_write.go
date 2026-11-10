// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/agenteditor"
)

// customBlobDocType is the BSON $Type of the generic wrapper the agent-editor
// extension (Mendix 11.9+) stores all of its documents in. The document kind is
// discriminated by CustomDocumentType; the payload is a JSON string in Contents.
const customBlobDocType = "CustomBlobDocuments$CustomBlobDocument"

// customBlobDocToGen builds the CustomBlobDocument envelope (Contents JSON +
// metadata) as a codec element. Mirrors sdk/mpr.writeCustomBlobDocument.
func customBlobDocToGen(unitID, name, documentation, exportLevel string, excluded bool, customType, readableType, contentsJSON, createdBy string) element.Element {
	g := newElem(customBlobDocType, unitID)
	addStr(g, "Contents", contentsJSON)
	addStr(g, "CustomDocumentType", customType)
	addStr(g, "Documentation", documentation)
	addBool(g, "Excluded", excluded)
	addStr(g, "ExportLevel", orDefault(exportLevel, "Hidden"))
	meta := newElem("CustomBlobDocuments$CustomBlobDocumentMetadata", "")
	addStr(meta, "CreatedByExtension", orDefault(createdBy, agenteditor.CreatedByExtensionID))
	addStr(meta, "ReadableTypeName", readableType)
	addPart(g, "Metadata", meta)
	addStr(g, "Name", name)
	return g
}

// writeCustomBlob encodes the envelope and inserts (create) or rewrites (update)
// the unit. A rewrite writes onto the stored Contents and keeps the stored
// creating extension (an imported agent says extension/agent-importer).
func (b *Backend) writeCustomBlob(unitID, containerID, name, documentation, exportLevel string, excluded bool, customType, readableType, contentsJSON string, update bool) error {
	createdBy := ""
	if update {
		stored := b.readStoredCustomBlob(unitID)
		var err error
		if contentsJSON, err = carryStoredContents(stored.contents, contentsJSON, contentsSpecFor(customType)); err != nil {
			return err
		}
		createdBy = stored.createdBy
	}
	el := customBlobDocToGen(unitID, name, documentation, exportLevel, excluded, customType, readableType, contentsJSON, createdBy)
	raw, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		return fmt.Errorf("encode custom blob: %w", err)
	}
	if update {
		// MDL has no export-level spelling for these documents (#816).
		if raw, err = b.keepStoredExportLevel(unitID, raw); err != nil {
			return err
		}
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
// Published MCP service
// ---------------------------------------------------------------------------

func (b *Backend) CreateAgentEditorPublishedMCPService(p *agenteditor.PublishedMCPService) error {
	if p == nil {
		return fmt.Errorf("CreateAgentEditorPublishedMCPService: nil service")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateAgentEditorPublishedMCPService: not connected for writing")
	}
	if p.Name == "" {
		return fmt.Errorf("CreateAgentEditorPublishedMCPService: name is required")
	}
	if p.ContainerID == "" {
		return fmt.Errorf("CreateAgentEditorPublishedMCPService: container ID is required")
	}
	if p.ID == "" {
		p.ID = model.ID(mmpr.GenerateID())
	}
	contents, err := encodePublishedMCPServiceContents(p)
	if err != nil {
		return err
	}
	// Only Agent Editor 2.6+ has this document, and 2.6 indents Contents.
	if contents, err = formatLikeStored([]byte(contents), "{\n"); err != nil {
		return err
	}
	return b.writeCustomBlob(string(p.ID), string(p.ContainerID), p.Name, p.Documentation, p.ExportLevel, p.Excluded, agenteditor.CustomTypePublishedMCPService, agenteditor.ReadablePublishedMCPService, contents, false)
}

func (b *Backend) UpdateAgentEditorPublishedMCPService(p *agenteditor.PublishedMCPService) error {
	if p == nil {
		return fmt.Errorf("UpdateAgentEditorPublishedMCPService: nil service")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateAgentEditorPublishedMCPService: not connected for writing")
	}
	contents, err := encodePublishedMCPServiceContents(p)
	if err != nil {
		return err
	}
	return b.writeCustomBlob(string(p.ID), string(p.ContainerID), p.Name, p.Documentation, p.ExportLevel, p.Excluded, agenteditor.CustomTypePublishedMCPService, agenteditor.ReadablePublishedMCPService, contents, true)
}

func (b *Backend) DeleteAgentEditorPublishedMCPService(id string) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteAgentEditorPublishedMCPService: not connected for writing")
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
		Name:     k.InnerName,
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

// encodeConsumedMCPServiceContents writes endpoint first and the credentials
// microflow last, as most Studio Pro 11.16 documents hold them (FactoryManagement);
// a stored document keeps its own order through the carry.
func encodeConsumedMCPServiceContents(c *agenteditor.ConsumedMCPService) (string, error) {
	type contentsShape struct {
		Endpoint                 *agenteditor.DocRef `json:"endpoint,omitempty"`
		ProtocolVersion          string              `json:"protocolVersion"`
		Documentation            string              `json:"documentation"`
		Version                  string              `json:"version"`
		ConnectionTimeoutSeconds int                 `json:"connectionTimeoutSeconds"`
		AuthenticationMicroflow  *agenteditor.DocRef `json:"authenticationMicroflow,omitempty"`
	}
	return marshalJSON(contentsShape{
		Endpoint:                 c.Endpoint,
		AuthenticationMicroflow:  c.AuthenticationMicroflow,
		ProtocolVersion:          c.ProtocolVersion,
		Documentation:            c.InnerDocumentation,
		Version:                  c.Version,
		ConnectionTimeoutSeconds: c.ConnectionTimeoutSeconds,
	})
}

// encodePublishedMCPServiceContents writes the keys in the order Agent Editor
// 2.6 saves them: authenticationMicroflow last, after documentation, when it is
// set. authorizationServerUrl and scopes, which the Studio Pro 11.16 sample does
// not hold, follow it; their place is inferred from the runtime reader.
func encodePublishedMCPServiceContents(p *agenteditor.PublishedMCPService) (string, error) {
	// Agent Editor writes a tool's sourceMicroflow qualifiedName first, and
	// the authenticationMicroflow documentId first (as agenteditor.DocRef).
	type microflowRef struct {
		QualifiedName string `json:"qualifiedName"`
		DocumentID    string `json:"documentId"`
	}
	ref := func(r *agenteditor.DocRef) *microflowRef {
		if r == nil {
			return nil
		}
		return &microflowRef{QualifiedName: r.QualifiedName, DocumentID: r.DocumentID}
	}
	type toolEntry struct {
		UUID            string        `json:"uuid"`
		Name            string        `json:"name"`
		Description     string        `json:"description"`
		SourceMicroflow *microflowRef `json:"sourceMicroflow,omitempty"`
		Title           string        `json:"title"`
		Schema          string        `json:"schema,omitempty"`
	}
	type contentsShape struct {
		ServerName              string              `json:"serverName"`
		EndpointPath            string              `json:"endpointPath"`
		Version                 string              `json:"version"`
		ProtocolVersion         string              `json:"protocolVersion"`
		AuthenticationEnabled   bool                `json:"authenticationEnabled"`
		Tools                   []toolEntry         `json:"tools"`
		Documentation           string              `json:"documentation"`
		AuthenticationMicroflow *agenteditor.DocRef `json:"authenticationMicroflow,omitempty"`
		AuthorizationServerURL  string              `json:"authorizationServerUrl,omitempty"`
		Scopes                  string              `json:"scopes,omitempty"`
	}
	tools := make([]toolEntry, 0, len(p.Tools))
	for _, t := range p.Tools {
		tools = append(tools, toolEntry{UUID: t.UUID, Name: t.Name, Description: t.Description, SourceMicroflow: ref(t.SourceMicroflow), Title: t.Title, Schema: t.Schema})
	}
	return marshalJSON(contentsShape{
		ServerName:              p.ServerName,
		EndpointPath:            p.EndpointPath,
		Version:                 p.Version,
		ProtocolVersion:         p.ProtocolVersion,
		AuthenticationEnabled:   p.AuthenticationEnabled,
		AuthenticationMicroflow: p.AuthenticationMicroflow,
		AuthorizationServerURL:  p.AuthorizationServerURL,
		Scopes:                  p.Scopes,
		Tools:                   tools,
		Documentation:           p.InnerDocumentation,
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
		// Written only when set, so a document without them stays without.
		UserAccessApproval string  `json:"userAccessApproval,omitempty"`
		DisplayTitle       *string `json:"displayTitle,omitempty"`
		DisplayDescription *string `json:"displayDescription,omitempty"`
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
		MinSimilarity        *float64            `json:"minSimilarity,omitempty"`
		IsVisible            *bool               `json:"isVisible,omitempty"`
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
		// After model, before entity: where Studio Pro puts it.
		SelectedModel *agenteditor.SelectedModel `json:"selectedModel,omitempty"`
		Entity        *agenteditor.DocRef        `json:"entity,omitempty"`
		MaxTokens     *int                       `json:"maxTokens,omitempty"`
		ToolChoice    string                     `json:"toolChoice,omitempty"`
		Temperature   *float64                   `json:"temperature,omitempty"`
		TopP          *float64                   `json:"topP,omitempty"`
		EnableBackend *bool                      `json:"enableBackendStreaming,omitempty"`
		EnableUI      *bool                      `json:"enableUIStreaming,omitempty"`
	}

	tools := make([]toolEntry, 0, len(a.Tools))
	for _, t := range a.Tools {
		tools = append(tools, toolEntry{ID: t.ID, Name: t.Name, Description: t.Description, Enabled: t.Enabled, ToolType: t.ToolType, Document: t.Document,
			UserAccessApproval: t.UserAccessApproval, DisplayTitle: t.DisplayTitle, DisplayDescription: t.DisplayDescription})
	}
	kbTools := make([]kbToolEntry, 0, len(a.KBTools))
	for _, kb := range a.KBTools {
		kbTools = append(kbTools, kbToolEntry{ID: kb.ID, Name: kb.Name, Description: kb.Description, Enabled: kb.Enabled, ToolType: kb.ToolType, Document: kb.Document, CollectionIdentifier: kb.CollectionIdentifier, MaxResults: kb.MaxResults,
			MinSimilarity: kb.MinSimilarity, IsVisible: kb.IsVisible})
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
		SelectedModel:      a.SelectedModel,
		Entity:             a.Entity,
		MaxTokens:          a.MaxTokens,
		ToolChoice:         a.ToolChoice,
		Temperature:        a.Temperature,
		TopP:               a.TopP,
		EnableBackend:      a.EnableBackendStreaming,
		EnableUI:           a.EnableUIStreaming,
	})
}

func marshalJSON(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	// Agent Editor stores <, > and & as themselves (prompts are full of them).
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return string(bytes.TrimRight(buf.Bytes(), "\n")), nil
}

// formatLikeStored lays raw out the way stored is: Agent Editor 2.6 saves
// Contents indented by two spaces, earlier versions and the agent importer
// compact. A rewrite in the other layout differed from the stored bytes, so
// every restatement of an unchanged document wrote it.
func formatLikeStored(raw []byte, stored string) (string, error) {
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return "", err
	}
	if !strings.HasPrefix(stored, "{\n") {
		return compact.String(), nil
	}
	var out bytes.Buffer
	if err := json.Indent(&out, compact.Bytes(), "", "  "); err != nil {
		return "", err
	}
	return out.String(), nil
}
