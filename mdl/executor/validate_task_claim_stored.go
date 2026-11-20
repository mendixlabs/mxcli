// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// storedTaskClaims reads stored microflows for MDL-WORKFLOW10: does a callee
// the script does not create claim the task it is passed (ako/mxcli#943)?
type storedTaskClaims struct {
	b backend.FullBackend
}

// TaskParameterClaims reports whether the stored microflow flow writes
// System.WorkflowUserTask_Assignees on its parameter param, and which calls it
// passes param on to. The stored flow is a graph, so every activity counts
// wherever it sits — a claim on any path is accepted, as for a script flow.
func (s *storedTaskClaims) TaskParameterClaims(flow, param string) (bool, [][2]string, bool) {
	objs, found := (&StoredCommitEvents{b: s.b}).flowObjects(false, flow)
	if !found {
		return false, nil, false
	}
	claims := false
	var calls [][2]string
	var walk func(*microflows.MicroflowObjectCollection)
	walk = func(oc *microflows.MicroflowObjectCollection) {
		if oc == nil {
			return
		}
		for _, o := range oc.Objects {
			switch a := o.(type) {
			case *microflows.ActionActivity:
				switch act := a.Action.(type) {
				case *microflows.ChangeObjectAction:
					if strings.EqualFold(act.ChangeVariable, param) && storedChangeClaims(act) {
						claims = true
					}
				case *microflows.MicroflowCallAction:
					if act.MicroflowCall == nil {
						continue
					}
					for _, m := range act.MicroflowCall.ParameterMappings {
						if m == nil || !strings.EqualFold(strings.TrimSpace(m.Argument), "$"+param) {
							continue
						}
						p := m.Parameter
						if i := strings.LastIndex(p, "."); i >= 0 {
							p = p[i+1:]
						}
						calls = append(calls, [2]string{act.MicroflowCall.Microflow, p})
					}
				}
			case *microflows.LoopedActivity:
				walk(a.ObjectCollection)
			}
		}
	}
	walk(objs)
	return claims, calls, true
}

// storedChangeClaims reports whether a stored Change activity writes the
// Assignees association.
func storedChangeClaims(act *microflows.ChangeObjectAction) bool {
	for _, c := range act.Changes {
		if c == nil {
			continue
		}
		for _, name := range []string{c.AssociationQualifiedName, c.AttributeQualifiedName} {
			if i := strings.LastIndex(name, "."); i >= 0 {
				name = name[i+1:]
			}
			if isClaimAssociation(name) {
				return true
			}
		}
	}
	return false
}
