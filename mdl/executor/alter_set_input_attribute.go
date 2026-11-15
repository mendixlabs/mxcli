// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// resolveInputSetAttribute resolves `alter page … set (attribute: …)` on a
// built-in input widget (text box, text area, date picker, drop-down, check
// box, radio buttons) by the rule `create page` uses — resolveInputAttribute,
// against the entity of the data container the widget sits in — and returns
// the binding the mutator writes as the AttributeRef.
//
// Before ako/mxcli#2278 the value reached the mutator as written: `Assoc/Attr`
// and a bare `Attr` stored a null AttributeRef (the input unbound), and the
// fully qualified `Mod.Assoc/Mod.Entity/Attr` was stored verbatim as one flat
// attribute name. A path create page cannot resolve either is refused here,
// as a Data grid 2 column's is (resolveColumnAttribute), rather than written
// unbound; check dry-runs this same function, so the two agree.
func resolveInputSetAttribute(ctx *ExecContext, mutator backend.PageMutator, widgetRef, attr string,
	moduleName string, moduleID model.ID) (backend.AttributeBinding, error) {
	where := fmt.Sprintf("widget `%s` attribute", widgetRef)
	attr = strings.TrimSpace(attr)
	if attr == "" {
		return backend.AttributeBinding{}, mdlerrors.NewValidationf(
			"%s takes an attribute name, `Module.Entity.Attr` or `Assoc/Attr`", where)
	}
	path := strings.TrimPrefix(attr, "$currentObject/")
	if strings.HasPrefix(path, "$") {
		return backend.AttributeBinding{}, mdlerrors.NewUnsupported(fmt.Sprintf(
			"%s: alter page set cannot bind `%s`, which reads a parameter or a data view by name — use "+
				"`replace <widget> with …`, which rebuilds the widget through the CREATE PAGE path", where, attr))
	}
	entity := mutator.EnclosingEntity(widgetRef)
	if entity == "" {
		mf, nf := mutator.EnclosingDataSourceFlow(widgetRef, false)
		entity = resolveDataSourceFlowEntity(ctx, moduleName, moduleID, mf, nf)
	}
	pb := newAlterPageBuilder(ctx, moduleName, moduleID, entity, mutator)
	if !strings.Contains(path, "/") {
		return backend.AttributeBinding{Path: pb.resolveAttributePath(path)}, nil
	}
	if entity == "" {
		return backend.AttributeBinding{}, mdlerrors.NewValidationf(
			"%s: the widget's data container names no entity, so the path `%s` cannot be followed", where, attr)
	}
	if _, _, ok := pb.resolveAssociationAttributePath(path); !ok {
		segs := strings.Split(path, "/")
		if len(segs) == 2 && strings.Count(segs[0], ".") == 1 && segs[1] != "" &&
			pb.entityIsOrDescendsFrom(entity, segs[0]) {
			// The shape check refuses in create page too (MDL-ATTRPATH02).
			return backend.AttributeBinding{}, mdlerrors.NewValidationf(
				"%s: `%s` starts with %s, the entity it is read from, and then names the attribute — not "+
					"a path. Name the attribute alone: write `%s` [%s]",
				where, attr, segs[0], segs[1], qualifiedPathFlatRule)
		}
		return backend.AttributeBinding{}, mdlerrors.NewValidationf(
			"%s: `%s` does not resolve from %s — each step must be an association of the entity before it, "+
				"and the last an attribute", where, attr, entity)
	}
	final, steps := pb.resolveInputAttribute(path)
	return backend.AttributeBinding{Path: final, Steps: steps}, nil
}
