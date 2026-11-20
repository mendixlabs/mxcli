// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// A stored activity MDL has no statement for is read as an UnsupportedAction and
// described as a `-- Unsupported action: <type>` comment, which parses to
// nothing. The splice path (create or modify of an existing flow) keeps it,
// because it does not rewrite it. Every path that builds the flow from the
// statement instead — a drop followed by a create, the route MDL-V1-REBUILD
// itself recommends, or a whole-flow rebuild — writes the flow without it, and
// reports success: the model is valid without the activity, so nothing fails at
// build time either. mendixlabs/mxcli#1735 lost a Send external object that way,
// and with it every save of the external entity.
//
// These guards refuse instead, naming what would be lost.

// unsupportedActionTypes lists the storage types of the activities in a stored
// flow body that MDL cannot express, loop bodies included, sorted and deduped.
func unsupportedActionTypes(oc *microflows.MicroflowObjectCollection) []string {
	seen := map[string]bool{}
	var walk func(*microflows.MicroflowObjectCollection)
	walk = func(c *microflows.MicroflowObjectCollection) {
		if c == nil {
			return
		}
		for _, obj := range c.Objects {
			switch o := obj.(type) {
			case *microflows.ActionActivity:
				if u, ok := o.Action.(*microflows.UnsupportedAction); ok && u != nil {
					seen[u.StorageType] = true
				}
			case *microflows.LoopedActivity:
				walk(o.ObjectCollection)
			}
		}
	}
	walk(oc)
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// unsupportedActionLossError is the refusal both guards give.
func unsupportedActionLossError(kind, qn, how string, types []string) error {
	return mdlerrors.NewUnsupported(fmt.Sprintf(
		"%s %s holds activities MDL cannot express (%s); %s would write the %s without them, and nothing "+
			"would report the loss. Change the %s with `create or modify` (which keeps them) or `alter %s %s { … }`, "+
			"or remove them in Studio Pro first if losing them is intended.",
		kind, qn, strings.Join(types, ", "), how, kind, kind, kind, qn))
}

// storedFlowUnsupportedActions returns the unexpressible activities of the
// stored microflow or nanoflow qn, or nil when there is none or it cannot be read.
func storedFlowUnsupportedActions(ctx *ExecContext, kind, qn string) []string {
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil
	}
	switch kind {
	case "microflow":
		if mf := findMicroflowByQualifiedName(ctx, qn); mf != nil {
			return unsupportedActionTypes(mf.ObjectCollection)
		}
	case "nanoflow":
		nfs, err := projectNanoflows(ctx)
		if err != nil {
			return nil
		}
		for _, nf := range nfs {
			if !nf.Excluded && h.GetQualifiedName(nf.ContainerID, nf.Name) == qn {
				return unsupportedActionTypes(nf.ObjectCollection)
			}
		}
	}
	return nil
}

// validateDropRecreateKeepsActivities refuses a script that drops a flow and
// creates it again when the stored flow holds an activity MDL cannot express.
// It runs before anything is written (check -p and exec's pre-flight), because
// once the drop has run there is nothing left to keep.
func validateDropRecreateKeepsActivities(ctx *ExecContext, prog *ast.Program) []error {
	if prog == nil || !ctx.Connected() {
		return nil
	}
	dropped := map[string]bool{} // "kind\x00qn" dropped earlier in the script
	var errs []error
	for _, stmt := range prog.Statements {
		var kind, qn string
		switch s := stmt.(type) {
		case *ast.DropMicroflowStmt:
			dropped["microflow\x00"+s.Name.String()] = true
			continue
		case *ast.DropNanoflowStmt:
			dropped["nanoflow\x00"+s.Name.String()] = true
			continue
		case *ast.CreateMicroflowStmt:
			kind, qn = "microflow", s.Name.String()
		case *ast.CreateNanoflowStmt:
			kind, qn = "nanoflow", s.Name.String()
		default:
			continue
		}
		if !dropped[kind+"\x00"+qn] {
			continue
		}
		delete(dropped, kind+"\x00"+qn)
		if types := storedFlowUnsupportedActions(ctx, kind, qn); len(types) > 0 {
			errs = append(errs, unsupportedActionLossError(kind, qn, "dropping it and creating it again", types))
		}
	}
	return errs
}
