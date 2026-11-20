// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// carryIncludedAssociations copies each stored "call external action"
// parameter's Associations to include onto the rewritten call, because MDL has
// no syntax for them: a CREATE OR MODIFY that restated the call wrote it with
// none, and the OData request then silently left out the associated objects
// (#1914). check, mxbuild and the build all pass either way.
//
// A call is matched on its service and action, then each mapping on its
// parameter name. The stored order of a microflow's objects is Studio Pro's,
// not the statement's, so position cannot pair two calls to the same action.
// When those calls hold the same tree it does not matter which gets which;
// when they differ, the rewrite is refused rather than guessed.
//
// A tree is not carried onto a parameter that now binds another entity: its
// associations belong to the entity it was ticked on. Nor onto a mapping whose
// statement writes `include associations ( … )` (stated), which says the tree
// itself — `()` included, which clears it.
//
// Each carry is returned, so exec can say that a statement left the clause out
// and kept the stored tree: deleting the clause removes nothing, and the
// warning is what tells the author so.
func carryIncludedAssociations(rebuilt, stored *microflows.MicroflowObjectCollection, qualifiedName string,
	stated map[*microflows.ExternalActionParameterMapping]bool) ([]carriedIncludes, error) {
	type slot struct {
		entity string
		tree   []*microflows.IncludedAssociation
		shape  string
	}
	storedTrees := map[string]*slot{} // service.action|parameter
	var conflicts []string
	for _, call := range externalActionCalls(stored) {
		for _, pm := range call.ParameterMappings {
			if len(pm.IncludedAssociations) == 0 {
				continue
			}
			key := externalActionKey(call) + "|" + strings.ToLower(pm.ParameterName)
			shape := includedAssociationsShape(pm.IncludedAssociations)
			if prev, ok := storedTrees[key]; ok {
				if prev.shape != shape || !strings.EqualFold(prev.entity, pm.ParameterEntity) {
					conflicts = append(conflicts, call.ConsumedODataService+"."+call.Name+"("+pm.ParameterName+")")
				}
				continue
			}
			storedTrees[key] = &slot{entity: pm.ParameterEntity, tree: pm.IncludedAssociations, shape: shape}
		}
	}
	if len(conflicts) > 0 {
		sort.Strings(conflicts)
		return nil, mdlerrors.NewUnsupported(fmt.Sprintf(
			"microflow %s calls %s more than once with different associations to include, "+
				"and MDL cannot say which call has which — rewriting it would mix them up.\n"+
				"  Change the microflow in Studio Pro, or give the calls the same associations first.",
			qualifiedName, strings.Join(conflicts, ", ")))
	}
	if len(storedTrees) == 0 {
		return nil, nil
	}
	var carried []carriedIncludes
	used := map[string]bool{}
	for _, call := range externalActionCalls(rebuilt) {
		for _, pm := range call.ParameterMappings {
			key := externalActionKey(call) + "|" + strings.ToLower(pm.ParameterName)
			s, ok := storedTrees[key]
			if !ok || len(pm.IncludedAssociations) > 0 || stated[pm] {
				continue
			}
			if s.entity != "" && pm.ParameterEntity != "" && !strings.EqualFold(s.entity, pm.ParameterEntity) {
				continue
			}
			// The first copy keeps the stored $IDs; a second call to the same
			// action gets fresh ones, since two elements of a unit cannot share one.
			pm.IncludedAssociations = cloneIncludedAssociations(s.tree, !used[key])
			if !used[key] {
				carried = append(carried, carriedIncludes{
					call:  call.ConsumedODataService + "." + call.Name,
					param: pm.ParameterName,
					tree:  formatIncludedAssociations(s.tree),
				})
			}
			used[key] = true
		}
	}
	return carried, nil
}

// carriedIncludes is one argument that kept its stored Associations to include
// because its statement did not write the clause.
type carriedIncludes struct {
	call, param string
	tree        string // as the clause writes it: "Odata.Quantity (Odata.Unit)"
}

// carriedIncludesWarning is what exec prints for a carry (ako/mxcli#1914).
func carriedIncludesWarning(flow string, c carriedIncludes) string {
	return fmt.Sprintf("Warning: %s: call external action %s(%s) keeps its stored associations to include (%s), "+
		"because the statement does not write `include associations`. State them — `%s = … include associations (%s)` — "+
		"or write `include associations ()` to remove them [%s]",
		flow, c.call, c.param, c.tree, c.param, c.tree, carriedIncludesRule)
}

// carriedIncludesRule is the rule id of the omitted-clause warning.
const carriedIncludesRule = "MDL-ODATA10"

// formatIncludedAssociations writes a tree as the clause's contents.
func formatIncludedAssociations(in []*microflows.IncludedAssociation) string {
	parts := make([]string, 0, len(in))
	for _, ia := range in {
		s := ia.Association
		if len(ia.IncludedAssociations) > 0 {
			s += " (" + formatIncludedAssociations(ia.IncludedAssociations) + ")"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ", ")
}

// externalActionCalls returns every CallExternalAction in a flow, loop bodies
// included.
func externalActionCalls(oc *microflows.MicroflowObjectCollection) []*microflows.CallExternalAction {
	if oc == nil {
		return nil
	}
	var out []*microflows.CallExternalAction
	for _, obj := range oc.Objects {
		switch o := obj.(type) {
		case *microflows.ActionActivity:
			if c, ok := o.Action.(*microflows.CallExternalAction); ok {
				out = append(out, c)
			}
		case *microflows.LoopedActivity:
			out = append(out, externalActionCalls(o.ObjectCollection)...)
		}
	}
	return out
}

func externalActionKey(c *microflows.CallExternalAction) string {
	return strings.ToLower(c.ConsumedODataService + "." + c.Name)
}

// includedAssociationsShape is a tree's content without its $IDs, for telling
// two stored trees apart.
func includedAssociationsShape(in []*microflows.IncludedAssociation) string {
	var b strings.Builder
	for _, ia := range in {
		fmt.Fprintf(&b, "(%s %t %s)", ia.Association, ia.IsParent, includedAssociationsShape(ia.IncludedAssociations))
	}
	return b.String()
}

func cloneIncludedAssociations(in []*microflows.IncludedAssociation, keepIDs bool) []*microflows.IncludedAssociation {
	out := make([]*microflows.IncludedAssociation, 0, len(in))
	for _, ia := range in {
		c := &microflows.IncludedAssociation{
			Association:          ia.Association,
			IsParent:             ia.IsParent,
			IncludedAssociations: cloneIncludedAssociations(ia.IncludedAssociations, keepIDs),
		}
		if keepIDs {
			c.ID = ia.ID
		}
		out = append(out, c)
	}
	return out
}
