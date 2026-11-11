// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ako/mxcli#2106: Studio Pro stores a system member a specialization inherits
// against the entity that stores it. ConversationalUI.Span_ToolCallUserConfirmation_SlideIn
// binds createdDate in a GenAICommons.ToolSpan context and stores
// 'GenAICommons.Span.createdDate' (Span has "Store created date"); the rebuild
// wrote 'GenAICommons.ToolSpan.createdDate'.
func TestResolveAttributePath_InheritedSystemMember(t *testing.T) {
	const mod = model.ID("mod-genai")
	pb := &pageBuilder{
		entityContext: "GenAICommons.ToolSpan",
		execCache: &executorCache{
			hierarchy: &ContainerHierarchy{moduleNames: map[model.ID]string{mod: "GenAICommons"}},
			domainModels: []*domainmodel.DomainModel{{
				ContainerID: mod,
				Entities: []*domainmodel.Entity{
					{BaseElement: model.BaseElement{ID: "e-span"}, Name: "Span", HasCreatedDate: true, HasOwner: true,
						Attributes: []*domainmodel.Attribute{{Name: "Output"}}},
					{BaseElement: model.BaseElement{ID: "e-tool"}, Name: "ToolSpan", GeneralizationRef: "GenAICommons.Span",
						Attributes: []*domainmodel.Attribute{{Name: "ToolName"}}},
				},
			}},
		},
	}
	for attr, want := range map[string]string{
		"createdDate": "GenAICommons.Span.createdDate",
		"owner":       "GenAICommons.Span.owner",
		// Controls: an inherited attribute and an own one, as before.
		"Output":   "GenAICommons.Span.Output",
		"ToolName": "GenAICommons.ToolSpan.ToolName",
		// A system member no entity in the chain stores keeps the context.
		"changedDate": "GenAICommons.ToolSpan.changedDate",
	} {
		if got := pb.resolveAttributePath(attr); got != want {
			t.Errorf("%s: resolved %q, want %q", attr, got, want)
		}
	}
}
