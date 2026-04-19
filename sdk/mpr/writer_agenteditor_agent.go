// SPDX-License-Identifier: Apache-2.0

// Package mpr - Writer for agent-editor Agent documents.
package mpr

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/agenteditor"
)

// CreateAgentEditorAgent writes an Agent document.
func (w *Writer) CreateAgentEditorAgent(a *agenteditor.Agent) error {
	if a == nil {
		return fmt.Errorf("agent is nil")
	}
	if a.Name == "" {
		return fmt.Errorf("agent name is required")
	}
	if a.ContainerID == "" {
		return fmt.Errorf("agent container ID is required")
	}
	if a.ID == "" {
		a.ID = model.ID(generateUUID())
	}

	// Ensure tool/KB entries have stable IDs.
	for i := range a.Tools {
		if a.Tools[i].ID == "" {
			a.Tools[i].ID = generateUUID()
		}
	}
	for i := range a.KBTools {
		if a.KBTools[i].ID == "" {
			a.KBTools[i].ID = generateUUID()
		}
	}

	contentsJSON, err := encodeAgentContents(a)
	if err != nil {
		return err
	}

	return w.writeCustomBlobDocument(customBlobInput{
		UnitID:             string(a.ID),
		ContainerID:        string(a.ContainerID),
		Name:               a.Name,
		Documentation:      a.Documentation,
		Excluded:           a.Excluded,
		ExportLevel:        a.ExportLevel,
		CustomDocumentType: agenteditor.CustomTypeAgent,
		ReadableTypeName:   agenteditor.ReadableAgent,
		ContentsJSON:       contentsJSON,
	})
}

// DeleteAgentEditorAgent removes an Agent by ID.
func (w *Writer) DeleteAgentEditorAgent(id string) error {
	return w.deleteUnit(id)
}

func encodeAgentContents(a *agenteditor.Agent) (string, error) {
	// Build the JSON shape matching what the agent editor extension produces.
	// Optional fields are omitted when empty/nil (omitempty).
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

	// Convert typed slices (ensure non-nil so JSON emits [] not null).
	tools := make([]toolEntry, 0, len(a.Tools))
	for _, t := range a.Tools {
		tools = append(tools, toolEntry{
			ID:          t.ID,
			Name:        t.Name,
			Description: t.Description,
			Enabled:     t.Enabled,
			ToolType:    t.ToolType,
			Document:    t.Document,
		})
	}
	kbTools := make([]kbToolEntry, 0, len(a.KBTools))
	for _, kb := range a.KBTools {
		kbTools = append(kbTools, kbToolEntry{
			ID:                   kb.ID,
			Name:                 kb.Name,
			Description:          kb.Description,
			Enabled:              kb.Enabled,
			ToolType:             kb.ToolType,
			Document:             kb.Document,
			CollectionIdentifier: kb.CollectionIdentifier,
			MaxResults:           kb.MaxResults,
		})
	}

	vars := a.Variables
	if vars == nil {
		vars = []agenteditor.AgentVar{}
	}

	payload := contentsShape{
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
	}

	return marshalCanonicalJSON(payload)
}
