// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// resolveColumnAttribute binds `set (Attribute: …) on <grid> column(…)` as
// CREATE binds a column's `Attribute:` (ako/mxcli#1644), so the mutator only
// writes it — the split #1641 made for a pluggable widget's attribute.
//
// A column attribute is linked to the grid's data source: a bare name is
// qualified against the grid's entity (an inherited member by its declaring
// one), `Module.Entity.Attr` is kept but refused when the entity is not the
// grid's or a generalization of it (CE7247), `Assoc/…/Attr` gets the hops CREATE
// writes for a multi-hop column, and `$Param.Attr` is refused — a column reads
// the grid's items, not a parameter.
func resolveColumnAttribute(ctx *ExecContext, mutator backend.PageMutator, target ast.WidgetRef, value any,
	moduleName string, moduleID model.ID) (backend.AttributeBinding, error) {
	where := fmt.Sprintf("column `%s` attribute", target.Name())
	if expr, ok := value.(*ast.WidgetExpressionV3); ok {
		return backend.AttributeBinding{}, mdlerrors.NewValidationf(
			"%s: `%s` names a parameter's attribute, but a column's attribute is read from each of the grid's "+
				"items — bind an attribute of the grid's entity (`Attr`, or `Assoc/Attr` over an association)",
			where, expr.Text)
	}
	attr, ok := value.(string)
	if !ok || strings.TrimSpace(attr) == "" {
		return backend.AttributeBinding{}, mdlerrors.NewValidationf(
			"%s takes an attribute name, `Module.Entity.Attr` or `Assoc/Attr`", where)
	}
	entity := mutator.EnclosingEntityForChildren(target.Widget)
	if entity == "" {
		mf, nf := mutator.EnclosingDataSourceFlow(target.Widget, true)
		entity = resolveDataSourceFlowEntity(ctx, moduleName, moduleID, mf, nf)
	}
	pb := newAlterPageBuilder(ctx, moduleName, moduleID, entity, mutator)
	if strings.Contains(attr, "/") {
		if entity == "" {
			return backend.AttributeBinding{}, mdlerrors.NewValidationf(
				"%s: the grid's data source names no entity, so the path `%s` cannot be followed", where, attr)
		}
		final, steps, ok := pb.resolveAssociationAttributePathForEntity(attr, entity)
		if !ok {
			return backend.AttributeBinding{}, mdlerrors.NewValidationf(
				"%s: `%s` does not resolve from %s — each step must be an association of the entity before it, "+
					"and the last an attribute", where, attr, entity)
		}
		return backend.AttributeBinding{Path: final, Steps: steps}, nil
	}
	outer := mutator.EnclosingEntity(target.Widget)
	ds := map[string]string{}
	if entity != "" {
		ds["datasource"] = entity
	}
	bs := resolveBindingScope("datasource", true, outer, ds, "")
	ix := pb.attributeIndex()
	if err := misboundAttributeError(where, attr, bs, outer, ds, ix); err != nil {
		return backend.AttributeBinding{}, err
	}
	if strings.Contains(attr, ".") {
		if err := foreignQualifiedAttributeError(where, attr, bs, ix); err != nil {
			return backend.AttributeBinding{}, err
		}
		return backend.AttributeBinding{Path: attr}, nil
	}
	if entity == "" {
		return backend.AttributeBinding{}, mdlerrors.NewValidationf(
			"%s: the grid's data source names no entity to qualify `%s` with — write `Module.Entity.%s`",
			where, attr, attr)
	}
	if err := pb.rejectAssociationAsAttribute(attr, entity, where); err != nil {
		return backend.AttributeBinding{}, err
	}
	return backend.AttributeBinding{Path: pb.resolveAttributePathForEntity(attr, entity)}, nil
}
