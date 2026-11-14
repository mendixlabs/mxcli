// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// An audit field declared with a pseudo-type — `ChangedDate: AutoChangedDate`,
// `Owner: AutoOwner` — is not an attribute of that name: the entity stores the
// system member (changedDate, createdDate, System.owner, System.changedBy). An
// attribute path (contentparams) resolves the declared name, but a widget
// EXPRESSION does not: `Visible: $currentObject/ChangedDate != empty` passed
// check and exec, and mx check reported CE0117 "Error(s) in expression"
// (ako/mxcli#2253). Measured with mx check on 11.12.6, one container per
// spelling: the four declared names fail, the four system spellings build.

var currentObjectMemberRe = regexp.MustCompile(`\$currentObject/([A-Za-z_][A-Za-z0-9_]*)\b`)

// describedSystemMembers are the names describe prints for a stored entity's
// audit fields, which a stored entity keeps no name for, with the spelling an
// expression needs.
var describedSystemMembers = map[string]string{
	"CreatedDate": "createdDate",
	"ChangedDate": "changedDate",
	"Owner":       "System.owner",
	"ChangedBy":   "System.changedBy",
}

// exprMemberSpelling is how an expression names a pseudo-type's system member:
// the date attributes by their own name, the user associations qualified.
func exprMemberSpelling(k ast.DataTypeKind) string {
	switch k {
	case ast.TypeAutoOwner, ast.TypeAutoChangedBy:
		return "System." + systemMemberOfPseudoType(k)
	}
	return systemMemberOfPseudoType(k)
}

// exprSystemMemberErrors reports each `$currentObject/<name>` in a widget's
// expression properties that names an audit field by its declared name.
// decl is the script's declaration of entity, nil when the entity is stored;
// storedAttrs are a stored entity's attribute names, lower-cased, nil when
// they are unknown — then nothing is judged.
func exprSystemMemberErrors(where string, props map[string]any, entity string, decl *ast.CreateEntityStmt, storedAttrs map[string]bool) []string {
	wrong := map[string]string{} // name written -> spelling to use
	switch {
	case decl != nil:
		for _, a := range decl.Attributes {
			if isAutoSystemMemberType(a.Type.Kind) {
				wrong[a.Name] = exprMemberSpelling(a.Type.Kind)
			}
		}
	case storedAttrs != nil:
		for name, spelling := range describedSystemMembers {
			if !storedAttrs[strings.ToLower(name)] {
				wrong[name] = spelling
			}
		}
	default:
		return nil
	}
	if len(wrong) == 0 {
		return nil
	}
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []string
	for _, key := range keys {
		expr, ok := props[key].(string)
		if !ok || !strings.Contains(expr, "$currentObject/") {
			continue
		}
		for _, m := range currentObjectMemberRe.FindAllStringSubmatch(expr, -1) {
			fix, bad := wrong[m[1]]
			if !bad || m[1] == fix {
				continue
			}
			out = append(out, fmt.Sprintf(
				"%s: %s uses $currentObject/%s — on %s that is the system member, which an expression spells "+
					"$currentObject/%s; mxbuild reports CE0117 \"Error(s) in expression\" for the declared name",
				where, key, m[1], entity, fix))
		}
	}
	return out
}

// checkExprSystemMembers is exprSystemMemberErrors over one widget in its data
// context.
func (v *attributeScopeValidator) checkExprSystemMembers(w *ast.WidgetV3, enclosing dataContext) {
	if enclosing.unresolved || len(enclosing.entities) == 0 || len(w.Properties) == 0 {
		return
	}
	entity := enclosing.entities[len(enclosing.entities)-1]
	var decl *ast.CreateEntityStmt
	var stored map[string]bool
	if v.decls != nil {
		decl = v.decls[entity]
	}
	if decl == nil {
		if attrs, ok := v.index.owners[entity]; ok {
			stored = attrs
		}
	}
	where := fmt.Sprintf("%s `%s`", w.Type, w.Name)
	v.errs = append(v.errs, exprSystemMemberErrors(where, w.Properties, entity, decl, stored)...)
}
