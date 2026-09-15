// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

// The three reads the MCP backend used to take from its own sdk/mpr reader.
//
// MCP opens the local project read-only and sends writes to Studio Pro, so it
// needs a semantic reader — and it held a concrete `*mpr.Reader` for one because
// the codec backend did not offer these three. That is the whole reason they sat
// on FullBackend with no caller through a backend value
// (unimplemented_reachability_test.go names MCP for each). Implementing them is
// what lets MCP compose this backend instead, which is step 2 of Phase 3 in
// docs/plans/2026-09-14-retire-legacy-engine.md.
//
// Each is a re-keying or a widening of a read this package already does, not new
// decoding: the conversions (domainModelFromGen, workflowFromGen,
// navProfileFromGen) are shared with their siblings, so a fix to any of them
// reaches all callers rather than one.

import (
	"fmt"

	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	genNav "github.com/mendixlabs/mxcli/modelsdk/gen/navigation"
	genWf "github.com/mendixlabs/mxcli/modelsdk/gen/workflows"
	"github.com/mendixlabs/mxcli/modelsdk/mprread"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// GetDomainModelByID reads a domain model by its own unit id.
//
// GetDomainModel keys by MODULE id; this keys by the document's. Both exist
// because a caller that already holds a domain model's id should not have to
// find its module first — which is exactly what MCP does.
//
// The System module's virtual domain model is deliberately NOT served here,
// unlike in GetDomainModel: it is injected rather than stored, so it has no unit
// id to look up and any id that reached this function came from a real document.
func (b *Backend) GetDomainModelByID(id model.ID) (*domainmodel.DomainModel, error) {
	units, err := mprread.ListUnitsWithContainer[*genDm.DomainModel](b.reader)
	if err != nil {
		return nil, err
	}
	for _, u := range units {
		if model.ID(u.Element.ID()) == id {
			dm := domainModelFromGen(u.Element, u.ContainerID)
			b.populateViewEntityOql([]*domainmodel.DomainModel{dm})
			return dm, nil
		}
	}
	// Match the legacy contract: a miss is an error, not (nil, nil). Callers
	// branch on the error; a nil document reads as "exists but is empty".
	return nil, fmt.Errorf("domain model not found: %s", id)
}

// GetWorkflow reads one workflow by unit id.
//
// Shares workflowFromGen with ListWorkflows rather than decoding again, so the
// two cannot disagree about what a workflow is — the drift that makes a
// single-item read return something subtly different from the same item in a
// listing.
func (b *Backend) GetWorkflow(id model.ID) (*workflows.Workflow, error) {
	units, err := mprread.ListUnitsWithContainer[*genWf.Workflow](b.reader)
	if err != nil {
		return nil, err
	}
	for _, u := range units {
		if model.ID(u.Element.ID()) == id {
			return workflowFromGen(u.Element, model.ID(u.ContainerID)), nil
		}
	}
	return nil, fmt.Errorf("workflow not found: %s", id)
}

// ListNavigationDocuments returns every navigation document in the project.
//
// A project has exactly one in every version shipped so far, which is why
// GetNavigation takes units[0] — but the model permits a list, and MCP asks for
// the list. Returning all of them keeps this honest about the storage rather
// than about the convention; GetNavigation keeps its first-one shortcut, since
// changing that is a separate decision with its own callers.
func (b *Backend) ListNavigationDocuments() ([]*types.NavigationDocument, error) {
	units, err := mprread.ListUnitsWithContainer[*genNav.NavigationDocument](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*types.NavigationDocument, 0, len(units))
	for _, u := range units {
		g := u.Element
		nav := &types.NavigationDocument{ContainerID: model.ID(u.ContainerID)}
		nav.ID = model.ID(g.ID())
		nav.TypeName = "Navigation$NavigationDocument"
		for _, profEl := range g.ProfilesItems() {
			if p := navProfileFromGen(profEl); p != nil {
				nav.Profiles = append(nav.Profiles, p)
			}
		}
		out = append(out, nav)
	}
	return out, nil
}
