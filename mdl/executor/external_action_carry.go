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
// associations belong to the entity it was ticked on.
func carryIncludedAssociations(rebuilt, stored *microflows.MicroflowObjectCollection, qualifiedName string) error {
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
		return mdlerrors.NewUnsupported(fmt.Sprintf(
			"microflow %s calls %s more than once with different associations to include, "+
				"and MDL cannot say which call has which — rewriting it would mix them up.\n"+
				"  Change the microflow in Studio Pro, or give the calls the same associations first.",
			qualifiedName, strings.Join(conflicts, ", ")))
	}
	if len(storedTrees) == 0 {
		return nil
	}
	used := map[string]bool{}
	for _, call := range externalActionCalls(rebuilt) {
		for _, pm := range call.ParameterMappings {
			key := externalActionKey(call) + "|" + strings.ToLower(pm.ParameterName)
			s, ok := storedTrees[key]
			if !ok || len(pm.IncludedAssociations) > 0 {
				continue
			}
			if s.entity != "" && pm.ParameterEntity != "" && !strings.EqualFold(s.entity, pm.ParameterEntity) {
				continue
			}
			// The first copy keeps the stored $IDs; a second call to the same
			// action gets fresh ones, since two elements of a unit cannot share one.
			pm.IncludedAssociations = cloneIncludedAssociations(s.tree, !used[key])
			used[key] = true
		}
	}
	return nil
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
