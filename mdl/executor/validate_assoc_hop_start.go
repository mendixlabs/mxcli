// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// MDL-ASSOCDS03 — a data source over associations whose first association does
// not touch the entity it starts from.
//
// `$Order/M.Account_Flag/M.Flag` names an association between Account and Flag
// from an Order. Every other check passes it — the association exists and Flag
// is one of its ends — so exec wrote an EntityRefStep that cannot be followed
// from the context object (ako/mxcli#834). exec now refuses it
// (resolveAssociationHops); this reports it at check time, from the same facts.
//
// Only what can be proved is reported: the association must be known (project
// or script), and every entity on the context entity's generalization chain
// must be loaded — an entity outside them (the System module's, say) could
// inherit from an end, so it ends the check silently.
func (v *attributeScopeValidator) checkAssocHopStart(w *ast.WidgetV3, enclosing dataContext) {
	if v.assocs == nil {
		return
	}
	ds := w.GetDataSource()
	if ds == nil {
		return
	}
	var path string
	switch {
	case ds.Type == "association":
		path = ds.Reference
	case ds.Type == "database" && ds.AssociationPath != "":
		path = ds.AssociationPath
	default:
		return
	}
	start := v.dataSourceContextEntity(ds, enclosing)
	if start == "" || path == "" {
		return
	}
	first, _, _ := strings.Cut(path, "/")
	qn := strings.TrimSpace(first)
	if qn == "" {
		return
	}
	if !strings.Contains(qn, ".") {
		mod, _, _ := strings.Cut(start, ".")
		qn = mod + "." + qn
	}
	shape, ok := v.assocs[strings.ToLower(qn)]
	if !ok || !v.provablyOffAssociation(start, shape) {
		return
	}
	v.errs = append(v.errs, fmt.Sprintf(
		"%s data source %s: %s joins %s and %s, and does not start at %s — "+
			"the step it writes cannot be followed from the context object. Follow an association of %s first [MDL-ASSOCDS03]",
		widgetLabel(w.Name, w.Type), dataSourceDisplay(ds), shape.qn, shape.from, shape.to, start, start))
}

// provablyOffAssociation reports whether entity is known to be neither end of
// the association nor a specialization of one: every entity on its
// generalization chain is in the script or the loaded domain models, and none
// of them is an end.
func (v *attributeScopeValidator) provablyOffAssociation(entity string, shape assocShape) bool {
	seen := map[string]bool{}
	for e := entity; e != ""; {
		key := strings.ToLower(e)
		if seen[key] {
			return false
		}
		seen[key] = true
		if strings.EqualFold(e, shape.from) || strings.EqualFold(e, shape.to) {
			return false
		}
		if gen, ok := v.generalizations[key]; ok {
			e = gen
			continue
		}
		if v.pb == nil {
			return false
		}
		gen, known := v.pb.entityGeneralization(e)
		if !known {
			return false
		}
		e = gen
	}
	return true
}
