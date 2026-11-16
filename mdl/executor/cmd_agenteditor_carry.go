// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/modelsdk/meta"

	"github.com/mendixlabs/mxcli/sdk/agenteditor"
)

// carryAgentIdentity gives a restated agent the identities its stored twin
// holds and the statement cannot spell: each tool's and knowledge base tool's
// id, and the entity reference's documentId. A setting the statement leaves
// out (a tool's UserAccessApproval and display text, a knowledge base tool's
// MinSimilarity and IsVisible, the agent's streaming switches) keeps its
// stored value, so a script written before they had MDL does not reset them.
//
// The ids are what the write path pairs a restated tool with its stored entry
// by, so they decide whether the tool keeps what Agent Editor stored on it
// (its approval, display title and description). A tool pairs with the
// stored tool of the same type and name, or for an MCP tool the same service;
// each stored tool pairs once.
func carryAgentIdentity(a, stored *agenteditor.Agent) {
	used := map[int]bool{}
	for i := range a.Tools {
		for j, st := range stored.Tools {
			if !used[j] && sameAgentTool(a.Tools[i], st) {
				used[j] = true
				a.Tools[i].ID = st.ID
				keepToolUserSettings(&a.Tools[i], st)
				if isMCPTool(st) {
					// describe prints an MCP tool by its service; the stored
					// entry's own name is Agent Editor's.
					a.Tools[i].Name = st.Name
				}
				break
			}
		}
	}
	usedKB := map[int]bool{}
	for i := range a.KBTools {
		for j, st := range stored.KBTools {
			if !usedKB[j] && st.Name == a.KBTools[i].Name {
				usedKB[j] = true
				a.KBTools[i].ID = st.ID
				if a.KBTools[i].MinSimilarity == nil {
					a.KBTools[i].MinSimilarity = st.MinSimilarity
				}
				if a.KBTools[i].IsVisible == nil {
					a.KBTools[i].IsVisible = st.IsVisible
				}
				break
			}
		}
	}
	a.Entity = carryEntityRef(a.Entity, stored.Entity)
	if a.EnableBackendStreaming == nil {
		a.EnableBackendStreaming = stored.EnableBackendStreaming
	}
	if a.EnableUIStreaming == nil {
		a.EnableUIStreaming = stored.EnableUIStreaming
	}
}

func keepToolUserSettings(t *agenteditor.AgentTool, stored agenteditor.AgentTool) {
	if t.UserAccessApproval == "" {
		t.UserAccessApproval = stored.UserAccessApproval
	}
	if t.DisplayTitle == nil {
		t.DisplayTitle = stored.DisplayTitle
	}
	if t.DisplayDescription == nil {
		t.DisplayDescription = stored.DisplayDescription
	}
}

// agentEntityRef is the reference an agent's Entity setting writes: the
// entity's qualified name and, as its documentId, the entity element's $ID —
// what Studio Pro stores (ako/mxcli#2289, measured on AgentCommons' example
// agents). An entity the project does not contain is refused, as a model that
// is not there is. System entities are the exception: the System module is not
// in the project, so its $ID comes from meta.SystemEntityID — measured from
// built models, and System.Language's is the 83d81a7b-… a Studio Pro save
// stores (ako/mxcli#2292). A System entity mxbuild gives a per-load $ID has no
// fixed one to write, so its reference keeps only its name (carryEntityRef then
// keeps a stored id) rather than an invented one.
func agentEntityRef(ctx *ExecContext, verb string, agentName string, qn ast.QualifiedName) (*agenteditor.DocRef, error) {
	ref := &agenteditor.DocRef{QualifiedName: qn.String()}
	if strings.EqualFold(qn.Module, "System") {
		if id, ok := meta.SystemEntityID(qn.Name); ok {
			ref.DocumentID = id
		}
		return ref, nil
	}
	e, ok := lookupEntity(ctx, qn.String())
	if !ok || e.ID == "" {
		return nil, fmt.Errorf("%s %s: entity not found: %s", verb, agentName, qn)
	}
	ref.DocumentID = string(e.ID)
	return ref, nil
}

// agentModelRef is the reference an agent's Model setting writes: the model
// document's unit id and its qualified name. A model the project does not
// contain is refused. check --references calls it too (ako/mxcli#2293), so
// both refuse the same reference with the same message.
func agentModelRef(ctx *ExecContext, verb string, agentName string, qn ast.QualifiedName) (*agenteditor.DocRef, error) {
	m := findAgentEditorModel(ctx, qn.Module, qn.Name)
	if m == nil {
		return nil, fmt.Errorf("%s %s: model not found: %s", verb, agentName, qn)
	}
	return &agenteditor.DocRef{DocumentID: string(m.ID), QualifiedName: qn.String()}, nil
}

// carryEntityRef keeps the stored entity reference's documentId when the
// statement names the same entity and mxcli could not resolve one itself
// (a System entity whose $ID mxbuild mints per load): writing it empty wiped
// what Studio Pro stored.
func carryEntityRef(ref, stored *agenteditor.DocRef) *agenteditor.DocRef {
	if ref == nil || stored == nil || ref.DocumentID != "" || ref.QualifiedName != stored.QualifiedName {
		return ref
	}
	return &agenteditor.DocRef{DocumentID: stored.DocumentID, QualifiedName: ref.QualifiedName}
}

func isMCPTool(t agenteditor.AgentTool) bool { return strings.EqualFold(t.ToolType, "MCP") }

func sameAgentTool(t, stored agenteditor.AgentTool) bool {
	if !strings.EqualFold(t.ToolType, stored.ToolType) {
		return false
	}
	if isMCPTool(t) {
		return t.Document != nil && stored.Document != nil && t.Document.QualifiedName == stored.Document.QualifiedName
	}
	return t.Name == stored.Name
}

// selectedModelFrom is the agent's selectedModel after a statement (#2031).
// id is the SelectedModel key and name SelectedModelName, nil when the
// statement leaves them out. Left out, the stored value stays; an empty id
// clears it. A name left out is the stored one while the id is the same, and
// otherwise the id: the stored display name belongs to the stored model, and
// the runtime reads only the id.
func selectedModelFrom(id, name *string, stored *agenteditor.SelectedModel) *agenteditor.SelectedModel {
	if id == nil {
		if stored == nil {
			return nil
		}
		sm := *stored
		if name != nil {
			sm.Name = *name
		}
		return &sm
	}
	if *id == "" {
		return nil
	}
	sm := agenteditor.SelectedModel{ID: *id, Name: *id}
	switch {
	case name != nil:
		sm.Name = *name
	case stored != nil && stored.ID == *id:
		sm.Name = stored.Name
	}
	return &sm
}
