// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/sdk/agenteditor"
)

// carryAgentIdentity gives a restated agent the identities its stored twin
// holds and the statement cannot spell: each tool's and knowledge base tool's
// id, and the entity reference's documentId.
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
				break
			}
		}
	}
	a.Entity = carryEntityRef(a.Entity, stored.Entity)
}

// carryEntityRef keeps the stored entity reference's documentId when the
// statement names the same entity. mxcli cannot compute it, and writing it
// empty wiped what Studio Pro stored.
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
