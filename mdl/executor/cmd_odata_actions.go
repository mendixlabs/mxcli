// SPDX-License-Identifier: Apache-2.0

// PUBLISH MICROFLOW — an OData action.
//
// Mendix exposes a published microflow in $metadata as an ActionImport, so a
// client can POST arguments to it. Without it, a parameterised resource has to
// be modelled as an entity set that carries its own arguments as attributes and
// echoes them back on every row, because Mendix validates $filter against the
// published metadata before the read microflow ever runs (mxcli-formula1 §47).
//
// Parameter data types and the return type are read off the microflow rather
// than authored, so the two cannot drift — the same thing Studio Pro does.
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// astMicroflowDefsToModel resolves each PUBLISH MICROFLOW block against the
// project's microflows and converts it to the semantic model.
func astMicroflowDefsToModel(ctx *ExecContext, defs []*ast.PublishedMicroflowDef) ([]*model.PublishedMicroflow, error) {
	if len(defs) == 0 {
		return nil, nil
	}
	byName, err := microflowsByQualifiedName(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*model.PublishedMicroflow, 0, len(defs))
	for _, def := range defs {
		qn := def.Microflow.String()
		mf, ok := byName[strings.ToLower(qn)]
		if !ok {
			return nil, mdlerrors.NewNotFoundMsg("microflow", qn,
				fmt.Sprintf("cannot publish %s as an OData action: microflow not found", qn))
		}
		pm, err := publishedMicroflowFor(def, mf, qn)
		if err != nil {
			return nil, err
		}
		out = append(out, pm)
	}
	return out, nil
}

// publishedMicroflowFor builds one published microflow from its definition and
// the microflow it names.
func publishedMicroflowFor(def *ast.PublishedMicroflowDef, mf *microflows.Microflow, qn string) (*model.PublishedMicroflow, error) {
	exposed := def.ExposedName
	if exposed == "" {
		exposed = mf.Name
	}
	pm := &model.PublishedMicroflow{Microflow: qn, ExposedName: exposed}
	pm.ReturnTypeKind, pm.ReturnTypeRef = dataTypeKindRef(mf.ReturnType)

	// Index the microflow's own parameters so an authored name can be checked
	// against them — a typo would otherwise publish a parameter Mendix cannot
	// bind, and the failure would surface as a build error far from the cause.
	params := map[string]*microflows.MicroflowParameter{}
	for _, p := range mf.Parameters {
		params[strings.ToLower(p.Name)] = p
	}

	selected := def.Parameters
	if len(selected) == 0 {
		// No selection: publish every parameter under its own name, in the
		// microflow's declared order.
		for _, p := range mf.Parameters {
			selected = append(selected, &ast.PublishedParamDef{Name: p.Name})
		}
	}
	for _, sel := range selected {
		p, ok := params[strings.ToLower(sel.Name)]
		if !ok {
			return nil, mdlerrors.NewNotFoundMsg("microflow parameter", sel.Name,
				fmt.Sprintf("%s has no parameter %q (it has: %s)",
					qn, sel.Name, microflowParamNames(mf)))
		}
		exposedParam := sel.ExposedName
		if exposedParam == "" {
			exposedParam = p.Name
		}
		mp := &model.PublishedMicroflowParameter{
			// Module.Microflow.Param — the same by-name shape the published-REST
			// writer uses for this reference.
			MicroflowParameter: qn + "." + p.Name,
			ExposedName:        exposedParam,
			CanBeEmpty:         sel.CanBeEmpty,
		}
		mp.DataTypeKind, mp.DataTypeRef = dataTypeKindRef(p.Type)
		pm.Parameters = append(pm.Parameters, mp)
	}
	return pm, nil
}

// dataTypeKindRef splits a microflow data type into a kind and, for the three
// kinds that name something, its qualified name. Kept apart because
// "Module.X" alone cannot say whether X is an entity or an enumeration.
func dataTypeKindRef(dt microflows.DataType) (kind, ref string) {
	switch t := dt.(type) {
	case nil:
		return "", ""
	case *microflows.ObjectType:
		return "Object", t.EntityQualifiedName
	case *microflows.ListType:
		return "List", t.EntityQualifiedName
	case *microflows.EnumerationType:
		return "Enumeration", t.EnumerationQualifiedName
	default:
		return dt.GetTypeName(), ""
	}
}

// microflowParamNames lists a microflow's parameter names for an error message.
func microflowParamNames(mf *microflows.Microflow) string {
	if len(mf.Parameters) == 0 {
		return "none"
	}
	names := make([]string, 0, len(mf.Parameters))
	for _, p := range mf.Parameters {
		names = append(names, p.Name)
	}
	return strings.Join(names, ", ")
}

// microflowsByQualifiedName indexes the project's microflows, lower-cased.
func microflowsByQualifiedName(ctx *ExecContext) (map[string]*microflows.Microflow, error) {
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil, mdlerrors.NewBackend("resolve module hierarchy", err)
	}
	mfs, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return nil, mdlerrors.NewBackend("list microflows", err)
	}
	out := make(map[string]*microflows.Microflow, len(mfs))
	for _, mf := range liveFirst(mfs, microflowExcluded) {
		key := strings.ToLower(h.GetQualifiedName(mf.ContainerID, mf.Name))
		if _, seen := out[key]; !seen { // live before an excluded twin (#1928)
			out[key] = mf
		}
	}
	return out, nil
}
