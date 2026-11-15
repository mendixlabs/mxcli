// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// A page or snippet attribute path written fully qualified, the way a
// microflow writes `$X/Mod.Assoc/Mod.Entity/Attr` (follow-up to
// ako/mxcli#2252). Exec reads such a path with resolveAssociationPathFrom and
// stores the short form describe prints: a leading step naming the entity the
// path is read from is dropped, and so is an entity step that names the entity
// the hop before it already arrives at. check said nothing about either, so
// check and exec disagreed on what the statement means. Now:
//
//   - MDL-ATTRPATH01 (warning): the path resolves, and a shorter one resolves
//     to the identical binding — check names the form exec stores. The short
//     form is found by asking exec's own resolver, not by a second copy of its
//     rules: an entity step is dropped only when the resolution without it is
//     the same final attribute and the same association steps, so a real
//     downcast (`Assoc/Mod.Specialization/Attr`) is kept.
//   - MDL-ATTRPATH02 (error): `Mod.Entity/Attr`, the entity the path is read
//     from (or a generalization of it) and then the attribute. Exec drops the
//     leading entity only when an association follows it, so this one was
//     written as one flat attribute name and the widget left unbound —
//     measured on mxbuild 11.16.0: CE0402 on a text's parameter, CE7005 on a
//     text box, "An attribute is required" on a Data grid 2 column.
//
// Judged: an input's or column's `Attribute:`, and the attribute-path values
// of ContentParams / CaptionParams, context-relative or rooted at a page or
// snippet parameter. A context this walk cannot resolve, or a path naming
// what only the script creates (the resolver reads the stored model), is left
// unjudged: check stays quieter than exec, never louder.
const (
	qualifiedPathStoredRule = "MDL-ATTRPATH01"
	qualifiedPathFlatRule   = "MDL-ATTRPATH02"
)

func (v *attributeScopeValidator) checkQualifiedAttributePaths(w *ast.WidgetV3, enclosing dataContext) {
	v.checkQualifiedAttributePathsIn(w, enclosing, true)
}

// checkQualifiedAttributePathsIn is checkQualifiedAttributePaths; rooted false
// leaves `$Param/…` values alone, for a second pass over widgets the scope walk
// has already judged those on.
func (v *attributeScopeValidator) checkQualifiedAttributePathsIn(w *ast.WidgetV3, enclosing dataContext, rooted bool) {
	if v.pb == nil {
		return
	}
	context := ""
	if !enclosing.unresolved && len(enclosing.entities) > 0 {
		context = enclosing.entities[len(enclosing.entities)-1]
	}
	if attr := strings.TrimSpace(w.GetAttribute()); attr != "" && !strings.HasPrefix(attr, "$") {
		v.judgeQualifiedPath(w, "", attr, attr, context)
	}
	for _, set := range []struct {
		key    string
		params []ast.ParamAssignmentV3
	}{{"ContentParams", w.GetContentParams()}, {"CaptionParams", w.GetCaptionParams()}} {
		for _, p := range set.params {
			raw, ok := p.Value.(string)
			if !ok {
				continue
			}
			raw = strings.TrimSpace(raw)
			where := fmt.Sprintf("%s {%d}", set.key, p.Index)
			// Rooted first: isTemplateExpression reads `$Root/…` as an
			// expression, and only the document's builder knows its roots.
			if root, path, isRooted := rootedAssociationPath(raw); isRooted {
				if entity := v.paramEntity(root); rooted && entity != "" {
					v.judgeQualifiedPath(w, where, raw, path, entity)
				}
				continue
			}
			if isTemplateExpression(raw) {
				continue
			}
			path := strings.TrimPrefix(raw, "$currentObject/")
			if !strings.HasPrefix(path, "$") {
				v.judgeQualifiedPath(w, where, raw, path, context)
			}
		}
	}
}

// checkDataWidgetItemPaths judges what a Data grid 2 or a gallery binds per
// item, against the entity its datasource supplies. The scope walk leaves the
// children of a pluggable data widget unresolved — it does not model the
// widget's slots — which left a grid column, the shape ako/mxcli#2252 was
// reported on, unjudged. Here only the slots that render the item are read: a
// grid's columns (and their params), a gallery's content; filters, control
// bars and empty placeholders are skipped, and so is anything inside a data
// container of its own.
func (v *attributeScopeValidator) checkDataWidgetItemPaths(w *ast.WidgetV3) {
	var kind string
	switch {
	case strings.EqualFold(w.Type, "datagrid"):
		kind = "datagrid"
	case strings.EqualFold(w.Type, "gallery"):
		kind = "gallery"
	default:
		return
	}
	entity := v.dataSourceEntity(w.GetDataSource())
	if entity == "" {
		return
	}
	item := dataContext{}.with(entity)
	var visit func(c *ast.WidgetV3)
	visit = func(c *ast.WidgetV3) {
		t := strings.ToLower(c.Type)
		if t == "controlbar" || t == "emptyplaceholder" || t == "filter" || strings.HasSuffix(t, "filter") ||
			t == "dropdownsort" || c.GetDataSource() != nil {
			return
		}
		v.checkQualifiedAttributePathsIn(c, item, false)
		for _, g := range c.Children {
			visit(g)
		}
	}
	for _, c := range w.Children {
		if kind == "datagrid" && !strings.EqualFold(c.Type, "column") {
			continue
		}
		visit(c)
	}
}

// judgeQualifiedPath judges one path read from start. written is the value as
// the script wrote it (with any `$Root/` prefix), path the part the resolver
// reads.
func (v *attributeScopeValidator) judgeQualifiedPath(w *ast.WidgetV3, where, written, path, start string) {
	if start == "" || !strings.Contains(path, "/") {
		return
	}
	segs := strings.Split(path, "/")
	qualified := false
	for _, s := range segs[:len(segs)-1] {
		if strings.Count(s, ".") == 1 {
			qualified = true
		}
	}
	if !qualified {
		return // the short form: nothing to say
	}
	subject := widgetLabel(w.Name, w.Type)
	if where != "" {
		subject += " " + where
	}
	finalQN, steps, ok := v.pb.resolveAssociationPathFrom(path, start)
	if !ok {
		v.judgeLeadingEntityAttribute(subject, written, segs, start)
		return
	}
	short := v.shortestEquivalentPath(segs, start, finalQN, steps)
	if short == path {
		return
	}
	stored := short
	if prefix := strings.TrimSuffix(written, path); prefix != written {
		stored = prefix + short
	}
	v.warns = append(v.warns, fmt.Sprintf(
		"%s binds `%s`, which repeats an entity the path is already at; exec stores it as `%s`, "+
			"so write that instead [%s]",
		subject, written, stored, qualifiedPathStoredRule))
}

// shortestEquivalentPath drops each qualified segment whose absence the
// resolver does not notice: the same final attribute, the same steps.
func (v *attributeScopeValidator) shortestEquivalentPath(segs []string, start, finalQN string, steps []pages.AttributeRefStep) string {
	cur := append([]string(nil), segs...)
	for i := 0; i < len(cur)-1; {
		if strings.Count(cur[i], ".") != 1 {
			i++
			continue
		}
		trial := append(append([]string(nil), cur[:i]...), cur[i+1:]...)
		if len(trial) >= 2 {
			if q, s, ok := v.pb.resolveAssociationPathFrom(strings.Join(trial, "/"), start); ok && q == finalQN && sameAttributeSteps(s, steps) {
				cur = trial
				continue
			}
		}
		i++
	}
	return strings.Join(cur, "/")
}

func sameAttributeSteps(a, b []pages.AttributeRefStep) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// judgeLeadingEntityAttribute refuses `Mod.Entity/Attr` read from Mod.Entity
// or a specialization of it — the shape exec does not normalise.
func (v *attributeScopeValidator) judgeLeadingEntityAttribute(subject, written string, segs []string, start string) {
	if len(segs) != 2 || strings.Count(segs[0], ".") != 1 || segs[1] == "" || strings.Contains(segs[1], ".") {
		return
	}
	lead := segs[0]
	if _, _, isAssoc := v.types.follow(lead, start); isAssoc {
		return
	}
	if !v.isOrSpecializes(start, lead) {
		return
	}
	v.errs = append(v.errs, leadingEntityAttributeMessage(subject, written, lead, segs[1]))
}

// leadingEntityAttributeMessage is MDL-ATTRPATH02's refusal of `lead/attr`,
// lead being the entity the path is read from; ALTER … SET on a pluggable
// widget refuses the same shape in the same words (ako/mxcli#2283).
func leadingEntityAttributeMessage(subject, written, lead, attr string) string {
	return fmt.Sprintf(
		"%s binds `%s`, which starts with %s, the entity it is read from. Exec drops that step only when an "+
			"association follows it, so this path would be written as one flat attribute name and the widget "+
			"left unbound — mxbuild reports CE0402 / CE7005 / \"An attribute is required\". Name the attribute "+
			"alone: write `%s` [%s]",
		subject, written, lead, attr, qualifiedPathFlatRule)
}

// isOrSpecializes reports whether entity is base or provably descends from it,
// through the script's EXTENDS targets ahead of the project's.
func (v *attributeScopeValidator) isOrSpecializes(entity, base string) bool {
	seen := map[string]bool{}
	for e := entity; e != ""; {
		key := strings.ToLower(e)
		if seen[key] {
			return false
		}
		seen[key] = true
		if strings.EqualFold(e, base) {
			return true
		}
		if gen, ok := v.generalizations[key]; ok {
			e = gen
			continue
		}
		gen, known := v.pb.entityGeneralization(e)
		if !known {
			return false
		}
		e = gen
	}
	return false
}
