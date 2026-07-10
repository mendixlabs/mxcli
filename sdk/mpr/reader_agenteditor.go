// SPDX-License-Identifier: Apache-2.0

// Package mpr - Reader methods for agent-editor CustomBlobDocuments.
//
// Covers the four document types created by the Studio Pro Agent Editor
// extension: Agent, Model, Knowledge Base, Consumed MCP Service. Each
// shares the outer CustomBlobDocument BSON wrapper and is discriminated
// by CustomDocumentType. This file currently implements Model only; the
// other three will follow the same pattern.
package mpr

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/sdk/agenteditor"
)

// ListAgentEditorModels returns all agent-editor Model documents in the
// project (CustomDocumentType == "agenteditor.model").
func (r *Reader) ListAgentEditorModels() ([]*agenteditor.Model, error) {
	units, err := r.listUnitsByType(customBlobDocType)
	if err != nil {
		return nil, err
	}

	var result []*agenteditor.Model
	for _, u := range units {
		wrap, err := parseCustomBlobWrapper(u.Contents)
		if err != nil {
			// Skip units we can't decode; log to error list if useful later.
			continue
		}
		if wrap.CustomDocumentType != agenteditor.CustomTypeModel {
			continue
		}
		m, err := r.parseAgentEditorModel(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse agent-editor model %s: %w", u.ID, err)
		}
		result = append(result, m)
	}
	return result, nil
}

// ListAgentEditorKnowledgeBases returns all agent-editor Knowledge Base
// documents in the project (CustomDocumentType == "agenteditor.knowledgebase").
func (r *Reader) ListAgentEditorKnowledgeBases() ([]*agenteditor.KnowledgeBase, error) {
	units, err := r.listUnitsByType(customBlobDocType)
	if err != nil {
		return nil, err
	}

	var result []*agenteditor.KnowledgeBase
	for _, u := range units {
		wrap, err := parseCustomBlobWrapper(u.Contents)
		if err != nil {
			continue
		}
		if wrap.CustomDocumentType != agenteditor.CustomTypeKnowledgeBase {
			continue
		}
		kb, err := r.parseAgentEditorKnowledgeBase(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse agent-editor knowledge base %s: %w", u.ID, err)
		}
		result = append(result, kb)
	}
	return result, nil
}

// ListAgentEditorConsumedMCPServices returns all agent-editor Consumed MCP
// Service documents in the project (CustomDocumentType ==
// "agenteditor.consumedMCPService").
func (r *Reader) ListAgentEditorConsumedMCPServices() ([]*agenteditor.ConsumedMCPService, error) {
	units, err := r.listUnitsByType(customBlobDocType)
	if err != nil {
		return nil, err
	}

	var result []*agenteditor.ConsumedMCPService
	for _, u := range units {
		wrap, err := parseCustomBlobWrapper(u.Contents)
		if err != nil {
			continue
		}
		if wrap.CustomDocumentType != agenteditor.CustomTypeConsumedMCPService {
			continue
		}
		c, err := r.parseAgentEditorConsumedMCPService(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse agent-editor consumed MCP service %s: %w", u.ID, err)
		}
		result = append(result, c)
	}
	return result, nil
}

// ListAgentEditorAgents returns all agent-editor Agent documents in the
// project (CustomDocumentType == "agenteditor.agent").
func (r *Reader) ListAgentEditorAgents() ([]*agenteditor.Agent, error) {
	units, err := r.listUnitsByType(customBlobDocType)
	if err != nil {
		return nil, err
	}

	var result []*agenteditor.Agent
	for _, u := range units {
		wrap, err := parseCustomBlobWrapper(u.Contents)
		if err != nil {
			continue
		}
		if wrap.CustomDocumentType != agenteditor.CustomTypeAgent {
			continue
		}
		a, err := r.parseAgentEditorAgent(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse agent-editor agent %s: %w", u.ID, err)
		}
		result = append(result, a)
	}
	return result, nil
}
