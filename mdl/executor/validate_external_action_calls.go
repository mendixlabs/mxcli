// SPDX-License-Identifier: Apache-2.0

// Reference validation for CALL EXTERNAL ACTION against the consumed OData
// service's cached $metadata.
//
// Mendix raises two errors on Microflows$CallExternalAction when the stored call
// stops matching the contract, and neither has anything to do with the domain
// model — both are defined on CallExternalAction.cs (Mendix 11.13,
// Mendix.Modeler.Texts.dll):
//
//	CE7252  ACTION_PARAMETERS_UNALIGNED
//	        "The parameters for remote action '{ACTION}' have changed."
//	CE7269  ACTION_RETURN_TYPE_UNALIGNED
//	        "The return type for remote action '{ACTION}' has changed."
//
// That is worth stating plainly, because the natural reading is the opposite:
// the errors mention a remote action, so re-running
// CREATE OR MODIFY EXTERNAL ENTITIES looks like the remedy. It never is — that
// statement writes entities, and these errors are raised by the microflow.
// mendixlabs/mxcli#1020 lost a debugging session to exactly that.
//
// Both are decidable here, against the same cached $metadata the writer reads,
// so they become an mxcli error naming the fix instead of a build-time surprise.
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// externalCall is one CALL EXTERNAL ACTION and the flow it was written in.
type externalCall struct {
	stmt *ast.CallExternalActionStmt
	flow string
}

// externalActionCallsIn collects every CALL EXTERNAL ACTION in a statement,
// including those nested inside loops and splits.
func externalActionCallsIn(stmt ast.Statement) []externalCall {
	var body []ast.MicroflowStatement
	var flow string
	switch s := stmt.(type) {
	case *ast.CreateMicroflowStmt:
		body, flow = s.Body, s.Name.String()
	case *ast.CreateNanoflowStmt:
		body, flow = s.Body, s.Name.String()
	default:
		return nil
	}

	var out []externalCall
	var walk func([]ast.MicroflowStatement)
	walk = func(stmts []ast.MicroflowStatement) {
		for _, s := range stmts {
			switch st := s.(type) {
			case *ast.CallExternalActionStmt:
				out = append(out, externalCall{stmt: st, flow: flow})
			case *ast.LoopStmt:
				walk(st.Body)
			case *ast.WhileStmt:
				walk(st.Body)
			case *ast.IfStmt:
				walk(st.ThenBody)
				walk(st.ElseBody)
			case *ast.EnumSplitStmt:
				for _, c := range st.Cases {
					walk(c.Body)
				}
				walk(st.ElseBody)
			case *ast.InheritanceSplitStmt:
				for _, c := range st.Cases {
					walk(c.Body)
				}
				walk(st.ElseBody)
			}
		}
	}
	walk(body)
	return out
}

// validateExternalActionCalls resolves every CALL EXTERNAL ACTION in the program
// against the cached contract.
func validateExternalActionCalls(ctx *ExecContext, prog *ast.Program) []error {
	if !ctx.Connected() {
		return nil
	}

	type located struct {
		call  externalCall
		index int
	}
	var calls []located
	for i, stmt := range prog.Statements {
		for _, c := range externalActionCallsIn(stmt) {
			calls = append(calls, located{call: c, index: i + 1})
		}
	}
	if len(calls) == 0 {
		return nil
	}

	services, err := ctx.Backend.ListConsumedODataServices()
	if err != nil {
		return nil
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil
	}

	var errs []error
	for _, c := range calls {
		if err := checkExternalActionCall(ctx, h, services, c.call); err != nil {
			errs = append(errs, fmt.Errorf("statement %d: %w", c.index, err))
		}
		for _, msg := range checkIncludedAssociations(ctx, h, c.call) {
			errs = append(errs, fmt.Errorf("statement %d: %s", c.index, msg))
		}
	}
	return errs
}

// checkIncludedAssociations resolves each `include associations ( … )` the
// call writes as exec's builder does (ako/mxcli#1914), so check refuses what
// exec would: a self-association, an association that does not touch the
// entity on its side, one whose side specializes both ends.
func checkIncludedAssociations(ctx *ExecContext, h *ContainerHierarchy, c externalCall) []string {
	stated := false
	for _, a := range c.stmt.Arguments {
		if a.IncludeAssociations != nil && len(a.IncludeAssociations.Associations) > 0 {
			stated = true
		}
	}
	if !stated {
		return nil
	}
	fb := &flowBuilder{backend: flowBuildBackend(ctx.Backend), hierarchy: h, resolveKnownOnly: true}
	kinds := fb.resolveExternalActionParameterKinds(c.stmt.ServiceName, c.stmt.ActionName)
	call := c.stmt.ServiceName.String() + "." + c.stmt.ActionName
	for _, a := range c.stmt.Arguments {
		if a.IncludeAssociations == nil {
			continue
		}
		fb.buildIncludedAssociations(call, a.Name, kinds[strings.ToLower(a.Name)].entity, a.IncludeAssociations.Associations)
	}
	return fb.GetErrors()
}

func checkExternalActionCall(ctx *ExecContext, h *ContainerHierarchy, services []*model.ConsumedODataService, c externalCall) error {
	svcQN := c.stmt.ServiceName.String()

	var doc *types.EdmxDocument
	for _, svc := range services {
		modName := h.GetModuleName(h.FindModuleID(svc.ContainerID))
		if !strings.EqualFold(modName, c.stmt.ServiceName.Module) || !strings.EqualFold(svc.Name, c.stmt.ServiceName.Name) {
			continue
		}
		if svc.Metadata == "" {
			// No cached contract: nothing to resolve against. Refresh is a
			// separate operation, so this is not an error here.
			return nil
		}
		parsed, err := types.ParseEdmx(svc.Metadata)
		if err != nil {
			return nil
		}
		doc = parsed
		break
	}
	if doc == nil {
		// An unknown service is reported by the existing reference validation.
		return nil
	}

	var action *types.EdmAction
	var known []string
	for _, act := range doc.Actions {
		known = append(known, act.Name)
		if strings.EqualFold(act.Name, c.stmt.ActionName) {
			action = act
		}
	}
	if action == nil {
		sort.Strings(known)
		return mdlerrors.NewNotFoundMsg("external action", c.stmt.ActionName, fmt.Sprintf(
			"%s: external action %q does not exist in %s's cached contract.\n  Actions in this service: %s",
			c.flow, c.stmt.ActionName, svcQN, joinOrNone(known)))
	}

	if err := checkExternalActionParameters(c, action); err != nil {
		return err
	}
	// The types the contract gives those parameters, and the return type. A type
	// Mendix does not support on an external action is the case with no remedy
	// at all, and the one mendixlabs/mxcli#1089 was filed about: the statement
	// executed, the build failed with CE7252, and every MDL the reporter reached
	// for was aimed at a stored fingerprint that does not exist.
	if err := checkExternalActionTypes(c, action, doc, svcQN, func(remoteName string) string {
		return externalEntityFor(ctx, h, svcQN, remoteName)
	}); err != nil {
		return err
	}
	return checkExternalActionReturn(ctx, h, c, action, svcQN)
}

// checkExternalActionParameters compares the arguments written in the statement
// with the contract's parameter list. A mismatch is CE7252.
//
// A BOUND action's first parameter is its binding parameter, which Mendix
// supplies from the object the action is called on rather than from a mapping,
// so it is not something the statement names.
func checkExternalActionParameters(c externalCall, action *types.EdmAction) error {
	want := map[string]string{} // lower -> declared spelling
	var wantNames []string
	for i, p := range action.Parameters {
		if action.IsBound && i == 0 {
			continue
		}
		want[strings.ToLower(p.Name)] = p.Name
		wantNames = append(wantNames, p.Name)
	}

	got := map[string]bool{}
	var unknown []string
	for _, arg := range c.stmt.Arguments {
		declared, ok := want[strings.ToLower(arg.Name)]
		if !ok {
			unknown = append(unknown, arg.Name)
			continue
		}
		got[declared] = true
	}

	var missing []string
	for _, name := range wantNames {
		if !got[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 && len(unknown) == 0 {
		return nil
	}

	sort.Strings(missing)
	sort.Strings(unknown)
	var parts []string
	if len(unknown) > 0 {
		parts = append(parts, fmt.Sprintf("not declared by the action: %s", strings.Join(unknown, ", ")))
	}
	if len(missing) > 0 {
		parts = append(parts, fmt.Sprintf("declared but not supplied: %s", strings.Join(missing, ", ")))
	}
	return mdlerrors.NewValidation(fmt.Sprintf(
		"%s: the arguments to external action %q do not match the service contract (%s).\n"+
			"  The action declares: %s\n"+
			"  Mendix reports this as CE7252 \"The parameters for remote action '%s' have changed\"",
		c.flow, action.Name, strings.Join(parts, "; "),
		joinOrNone(wantNames), action.Name))
}

// checkExternalActionReturn reports an entity-typed return whose external entity
// has not been imported. Without it the writer cannot type the result variable,
// and Mendix reports CE7269.
//
// This is the one case where CREATE EXTERNAL ENTITIES really is the remedy — so
// the message says so, with the statement to run.
func checkExternalActionReturn(ctx *ExecContext, h *ContainerHierarchy, c externalCall, action *types.EdmAction, svcQN string) error {
	if edmReturnTypeToKind(action.ReturnType) != "" {
		return nil // primitive or void: always representable
	}
	typeName, isList := edmBareTypeName(action.ReturnType)
	if typeName == "" {
		return nil // a complex type mxcli does not model; not decidable here
	}
	if externalEntityFor(ctx, h, svcQN, typeName) != "" {
		return nil
	}

	shape := "an object"
	if isList {
		shape = "a list"
	}
	return mdlerrors.NewValidation(fmt.Sprintf(
		"%s: external action %q returns %s of %s, but no external entity has been imported "+
			"for that type, so the call's return type cannot be set.\n"+
			"  Mendix reports this as CE7269 \"The return type for remote action '%s' has changed\".\n"+
			"  Import it first: create or modify external entities from %s entities (%s)",
		c.flow, action.Name, shape, action.ReturnType, action.Name, svcQN, typeName))
}

// externalEntityFor finds the external entity imported from svcQN for the remote
// type remoteName, returning its qualified name. Mirrors the resolution the flow
// builder does at write time.
func externalEntityFor(ctx *ExecContext, h *ContainerHierarchy, svcQN, remoteName string) string {
	dms, err := projectDomainModels(ctx)
	if err != nil {
		return ""
	}
	for _, dm := range dms {
		modName := h.GetModuleName(h.FindModuleID(dm.ContainerID))
		for _, ent := range dm.Entities {
			if strings.EqualFold(ent.RemoteServiceName, svcQN) && strings.EqualFold(ent.RemoteEntityName, remoteName) {
				return modName + "." + ent.Name
			}
		}
	}
	return ""
}

func joinOrNone(names []string) string {
	if len(names) == 0 {
		return "(none)"
	}
	return strings.Join(names, ", ")
}
