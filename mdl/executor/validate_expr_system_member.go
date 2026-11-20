// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
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
	wrong := auditFieldSpellings(decl, storedAttrs)
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

// auditFieldSpellings maps each name an expression must not use for an audit
// field to the spelling it needs: the script's declared pseudo-type names, or
// describe's four names on a stored entity that has no attribute of that name.
// Empty when neither is known.
func auditFieldSpellings(decl *ast.CreateEntityStmt, storedAttrs map[string]bool) map[string]string {
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
	}
	return wrong
}

// widgetExpressionTexts are a widget's client-side expression properties, with
// the name each is reported under: the ones MDL-WIDGET13 reads, the quoted
// `Editable: '<expr>'` included.
func widgetExpressionTexts(w *ast.WidgetV3) [][2]string {
	var out [][2]string
	for _, key := range expressionWidgetProps {
		expr, ok := lookupWidgetProp(w, key)
		shown := key
		if key == "EditableIf" && (!ok || expr == "") {
			if e, quoted := editableCondition(w); quoted {
				expr, ok, shown = e, true, "Editable"
			}
		}
		if ok && strings.TrimSpace(expr) != "" {
			out = append(out, [2]string{shown, expr})
		}
	}
	return out
}

// checkExprSystemMembers judges the members one widget's expressions name, in
// its data context (ako/mxcli#2253, ako/mxcli#2258).
//
// First an audit field's declared name (exprSystemMemberErrors). Then every
// `$currentObject/<name>` and `$<pageParam>/<name>`: an attribute the entity
// does not have, or an association written by its bare name, is CE0117. A
// widget expression cannot follow an association at all — measured on 11.12.6,
// bare, qualified and missing associations are each CE0117 — and the qualified
// forms are MDL-WIDGET13's, so only the first step is judged here. A variable
// whose entity is not known, or a context this walk cannot resolve, is left to
// mxbuild.
func (v *attributeScopeValidator) checkExprSystemMembers(w *ast.WidgetV3, enclosing dataContext) {
	if len(w.Properties) == 0 {
		return
	}
	exprs := widgetExpressionTexts(w)
	if len(exprs) == 0 {
		return
	}
	where := fmt.Sprintf("%s `%s`", w.Type, w.Name)
	current := ""
	if !enclosing.unresolved && len(enclosing.entities) > 0 {
		current = enclosing.entities[len(enclosing.entities)-1]
	}

	reported := map[string]bool{} // entity + "/" + member, so each is told once
	if current != "" {
		var decl *ast.CreateEntityStmt
		var stored map[string]bool
		if v.decls != nil {
			decl = v.decls[current]
		}
		if decl == nil {
			if attrs, ok := v.index.owners[current]; ok {
				stored = attrs
			}
		}
		v.errs = append(v.errs, exprSystemMemberErrors(where, w.Properties, current, decl, stored)...)
		// An audit field's declared name is told above, with its spelling.
		for name := range auditFieldSpellings(decl, stored) {
			reported[current+"/"+name] = true
		}
	}
	if v.members == nil {
		return
	}
	for _, kv := range exprs {
		e, ok := visitor.ParseExpressionText(kv[1])
		if !ok {
			continue
		}
		for _, p := range collectAttributePaths(e) {
			name := strings.TrimPrefix(p.Variable, "$")
			entity := ""
			if strings.EqualFold(name, "currentObject") {
				entity = current
			} else {
				entity = v.paramEntity(name)
			}
			steps := expressionPathSteps(p)
			if entity == "" || len(steps) == 0 || strings.Contains(steps[0], ".") {
				continue
			}
			step := steps[0]
			key := entity + "/" + step
			if reported[key] {
				continue
			}
			res, kind := v.members.member(entity, step, true)
			var msg string
			switch {
			case res == memberMissing:
				msg = fmt.Sprintf("%s: %s uses $%s/%s — %s has no attribute %q%s; mxbuild rejects the expression "+
					"with CE0117 \"Error(s) in expression.\"%s",
					where, kv[0], name, step, entity, step, flowSystemMemberHint(step), v.members.nearMembers(entity))
			case res == memberFound && kind == memberIsAssociation:
				msg = fmt.Sprintf("%s: %s uses $%s/%s — %s is an association, and a widget expression cannot "+
					"follow associations (CE0117). Bind the value through a data binding (contentparams, attribute) "+
					"or a nested data view over the association instead",
					where, kv[0], name, step, step)
			default:
				continue
			}
			reported[key] = true
			v.errs = append(v.errs, msg)
		}
	}
}
