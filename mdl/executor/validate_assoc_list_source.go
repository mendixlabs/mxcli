// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// MDL-ASSOCDS01 — a list widget over an association that yields one object.
//
// A list view, data grid or gallery whose data source is `$currentObject/M.A`
// needs the association path to produce a LIST. mxbuild refuses it otherwise:
// CE8812 "A grid association path must result in a list." `check` passed it,
// `exec` wrote it, and the build failed (ako/mxcli#969 item 3).
//
// Measured on mxbuild 11.14.0, one page per shape, a list view, a data grid and
// a gallery in each (all three behave identically):
//
//	Reference,    owner Default, from the FROM entity → CE8812
//	Reference,    owner Both,    from the FROM entity → CE8812
//	Reference,    owner Both,    from the TO entity   → CE8812
//	Reference,    owner Default, from the TO entity   → ok (the reverse is a list)
//	ReferenceSet, owner Default or Both, either end   → ok
//
// Only those shapes are judged. A context that is a specialization of an end,
// a self-association, a multi-hop path or an owner mxcli does not write is left
// alone rather than guessed at.

type assocShape struct {
	qn       string
	from, to string
	refSet   bool
	owner    string // "Default", "Both", or "" when not one of the measured two
	// view marks an association a view entity derives from its OQL (an
	// OqlViewAssociationSource): `from` is the view entity, and a page cannot
	// follow it from `to` (MDL-ASSOCDS02, validate_view_assoc_reverse.go).
	view bool
}

func astOwnerName(o ast.OwnerType) string {
	switch o {
	case ast.OwnerDefault:
		return "Default"
	case ast.OwnerBoth:
		return "Both"
	}
	return ""
}

func sdkOwnerName(o domainmodel.AssociationOwner) string {
	switch o {
	case domainmodel.AssociationOwnerDefault, "":
		return "Default"
	case domainmodel.AssociationOwnerBoth:
		return "Both"
	}
	return ""
}

// listWidgetTypes are the MDL widget keywords whose data source must be a list.
var listWidgetTypes = map[string]bool{
	"listview": true, "datagrid": true, "datagrid2": true, "gallery": true, "templategrid": true,
}

// checkAssociationShapes indexes the project's associations, overlaid with the
// script's, by lower-cased qualified name.
func checkAssociationShapes(ctx *ExecContext, sc *scriptContext) map[string]assocShape {
	return checkAssociationCatalog(ctx, sc).shapes
}

// checkAssociationCatalog is checkAssociationShapes with what could not be
// read, for a check that must tell "no such association" from "not looked".
func checkAssociationCatalog(ctx *ExecContext, sc *scriptContext) associationCatalog {
	cat := associationCatalog{shapes: map[string]assocShape{}, unnamed: map[string]bool{}}
	if ctx != nil && ctx.Backend != nil {
		if modules, err := getModulesFromCache(ctx); err == nil {
			if dms, err := projectDomainModels(ctx); err == nil {
				cat = projectAssociationCatalog(modules, dms)
			}
		}
	}
	if sc != nil {
		for k, v := range sc.associationShapes {
			cat.shapes[k] = v
		}
	}
	return cat
}

// checkAssocListSource reports MDL-ASSOCDS01 for one widget, given the data
// context it sits in.
func (v *attributeScopeValidator) checkAssocListSource(w *ast.WidgetV3, enclosing dataContext) {
	if v.assocs == nil || !listWidgetTypes[strings.ToLower(w.Type)] {
		return
	}
	ds := w.GetDataSource()
	if ds == nil || ds.Type != "association" || ds.Reference == "" {
		return
	}
	segs := strings.Split(ds.Reference, "/")
	if len(segs) > 2 {
		return
	}
	var ctxEntity string
	switch cv := strings.ToLower(ds.ContextVariable); cv {
	case "", "currentobject":
		if enclosing.unresolved || len(enclosing.entities) == 0 {
			return
		}
		ctxEntity = enclosing.entities[len(enclosing.entities)-1]
	default:
		qn, ok := v.pageParams[cv]
		if !ok {
			return
		}
		ctxEntity = qn
	}
	assoc := segs[0]
	if !strings.Contains(assoc, ".") {
		mod, _, _ := strings.Cut(ctxEntity, ".")
		assoc = mod + "." + assoc
	}
	shape, ok := v.assocs[strings.ToLower(assoc)]
	if !ok || shape.refSet || shape.owner == "" || strings.EqualFold(shape.from, shape.to) {
		return
	}
	var why string
	switch {
	case strings.EqualFold(ctxEntity, shape.from):
		why = fmt.Sprintf("%s is a Reference followed from its FROM entity %s, which yields one %s, not a list", shape.qn, shape.from, shape.to)
	case strings.EqualFold(ctxEntity, shape.to) && shape.owner == "Both":
		why = fmt.Sprintf("%s is a Reference with owner Both — one-to-one — so from %s it yields one %s, not a list", shape.qn, shape.to, shape.from)
	default:
		return
	}
	v.errs = append(v.errs, fmt.Sprintf(
		"%s data source %s: %s — mxbuild rejects this with CE8812 \"A grid association path must result in a list\". "+
			"Show the single object in a data view, or make the association a ReferenceSet [MDL-ASSOCDS01]",
		widgetLabel(w.Name, w.Type), dataSourceText(ds), why))
}

func dataSourceText(ds *ast.DataSourceV3) string {
	if ds.ContextVariable != "" {
		return "$" + ds.ContextVariable + "/" + ds.Reference
	}
	return "association " + ds.Reference
}
