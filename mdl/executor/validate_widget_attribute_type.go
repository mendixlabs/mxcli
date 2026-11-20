// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// Two page errors `check --references` passed and only mxbuild reported, both
// about a BUILT-IN input widget (Forms$TextBox, Forms$DropDown, …) on a page:
//
//   - MDL-WIDGET39 / CE2421: an input widget bound to an attribute of a type it
//     does not take — a textbox on an enumeration. Also the bare-association
//     variant of the same mistake (`textbox (attribute: Order_Customer)`), which
//     the builder stores as an attribute that does not exist and mxbuild reports
//     as CE1613 — and which, measured, stops `mx check` from reporting anything
//     else in the project.
//   - MDL-WIDGET40 / CE0582: the classic drop-down on a project that uses the
//     React client.
//
// MDL-WIDGET39 also judges the pluggable combo box in enumeration mode, the one
// pluggable input whose attribute type mxbuild checks the same way (CE7247).
//
// Both are questions about the model (the attribute's type, the project's client
// setting), so they live in the --references tier with the pluggable-widget
// attribute-scope check, sharing its data-context walk. Without -p neither can
// run.

// The two rules' ids, named once so the rule-id registry tests see them.
const (
	widgetAttributeTypeRule = "MDL-WIDGET39" // CE2421 (and CE1613 for an association)
	reactDropDownRule       = "MDL-WIDGET40" // CE0582
)

// inputAttributeRule is what one built-in input widget accepts.
type inputAttributeRule struct {
	label   string          // mxbuild's name for the widget, as its messages print it
	accepts map[string]bool // attribute kinds (see attributeKind) it takes
	listed  string          // mxbuild's own list, verbatim from its message
	code    string          // the error mxbuild reports for a type outside accepts
}

// builtinInputAttributeRules is the measured matrix, not the Model SDK's: every
// (widget, attribute type) pair below was built on a fresh Mendix 11.14.0 app
// with mxbuild, and every pair not accepted here was CE2421.
//
// Only measured kinds are judged (measuredAttributeKinds). HashedString and Date
// are not among them: MDL cannot author either (`HashedString` in an entity
// declaration is stored as String(unlimited)), so no pair was built, and a
// widget's CE2421 list is what it offers, not proof of what it refuses.
var builtinInputAttributeRules = map[string]inputAttributeRule{
	"textbox": {"text box", kindSet("String", "HashedString", "Integer", "Long", "Decimal", "AutoNumber"),
		"Hashed string, Integer, Long, String, Decimal, AutoNumber", "CE2421"},
	"textarea":     {"text area", kindSet("String"), "String", "CE2421"},
	"datepicker":   {"date picker", kindSet("DateTime"), "Date and time", "CE2421"},
	"checkbox":     {"check box", kindSet("Boolean"), "Boolean", "CE2421"},
	"radiobuttons": {"radio buttons", kindSet("Boolean", "Enumeration"), "Boolean, Enumeration", "CE2421"},
	"dropdown":     {"drop-down", kindSet("Enumeration"), "Enumeration", "CE2421"},
}

// comboBoxEnumerationRule is the pluggable combo box with no options data
// source. Enumeration mode writes the attribute to `attributeEnumeration`, whose
// AllowedTypes is Enum alone. Measured on fresh 11.14.0 and 11.13.0 projects,
// one combo box per kind: every kind but Enumeration was CE7247 "Only attributes
// of type Enumeration are allowed here." Boolean is among them — the widget has
// a separate Boolean source (`attributeBoolean`), which MDL does not write.
var comboBoxEnumerationRule = inputAttributeRule{"combo box", kindSet("Enumeration"), "Enumeration", "CE7247"}

// comboBoxDatabaseRule is the combo box in database mode, which stores the
// chosen option's value on `databaseAttributeString` (mendixlabs/mxcli#1583).
// Measured on a fresh 11.12.5 project, Combo box 2.5.0, one combo box per
// kind: Boolean, DateTime, Decimal, AutoNumber and Binary were CE7247 "Only
// attributes of type String, Integer, Long, Enumeration are allowed here."
// Its attribute may also be written under that key.
var comboBoxDatabaseRule = inputAttributeRule{"combo box", kindSet("String", "Integer", "Long", "Enumeration"),
	"String, Integer, Long, Enumeration", "CE7247"}

// inputAttributeRuleFor is the rule w's attribute is judged by. A combo box
// is judged in the mode exec builds it in (comboBoxMode): the association mode
// binds an association, not an attribute, so it has none.
func inputAttributeRuleFor(w *ast.WidgetV3) (rule inputAttributeRule, isComboBox, ok bool) {
	if w.TypeIsGeneric {
		return inputAttributeRule{}, false, false
	}
	if strings.EqualFold(w.Type, "combobox") {
		if comboBoxHasOptionsDataSource(w) {
			return inputAttributeRule{}, true, false
		}
		if comboBoxMode(w) == "database" {
			return comboBoxDatabaseRule, true, true
		}
		return comboBoxEnumerationRule, true, true
	}
	rule, ok = builtinInputAttributeRules[strings.ToLower(w.Type)]
	return rule, false, ok
}

// measuredAttributeKinds are the kinds the matrix was measured against. A kind
// outside it is not judged — the check may be quieter than mxbuild, never louder.
var measuredAttributeKinds = kindSet("String", "Integer", "Long", "Decimal",
	"AutoNumber", "Boolean", "DateTime", "Enumeration", "Binary")

func kindSet(kinds ...string) map[string]bool {
	m := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		m[k] = true
	}
	return m
}

// attributeKind names a stored attribute type; "" for one this check does not know.
func attributeKind(t domainmodel.AttributeType) string {
	switch t.(type) {
	case *domainmodel.StringAttributeType:
		return "String"
	case *domainmodel.HashedStringAttributeType:
		return "HashedString"
	case *domainmodel.IntegerAttributeType:
		return "Integer"
	case *domainmodel.LongAttributeType:
		return "Long"
	case *domainmodel.DecimalAttributeType:
		return "Decimal"
	case *domainmodel.AutoNumberAttributeType:
		return "AutoNumber"
	case *domainmodel.BooleanAttributeType:
		return "Boolean"
	case *domainmodel.DateTimeAttributeType:
		return "DateTime"
	case *domainmodel.DateAttributeType:
		return "Date"
	case *domainmodel.EnumerationAttributeType:
		return "Enumeration"
	case *domainmodel.BinaryAttributeType:
		return "Binary"
	}
	return ""
}

// astAttributeKind names a script-declared attribute type, as attributeKind
// does for a stored one. The audit pseudo-types are system members, not
// attributes, and are left out: the builder does not store them as one.
func astAttributeKind(t ast.DataType) string {
	switch t.Kind {
	case ast.TypeString:
		return "String"
	case ast.TypeInteger:
		return "Integer"
	case ast.TypeLong:
		return "Long"
	case ast.TypeDecimal:
		return "Decimal"
	case ast.TypeBoolean:
		return "Boolean"
	case ast.TypeDateTime:
		return "DateTime"
	case ast.TypeDate:
		return "Date"
	case ast.TypeAutoNumber:
		return "AutoNumber"
	case ast.TypeBinary:
		return "Binary"
	case ast.TypeEnumeration:
		return "Enumeration"
	}
	return ""
}

// memberTypeIndex answers "what type is this attribute, and where is it
// declared" and "where does this association lead" over the project's domain
// models, overlaid with what the script declares.
type memberTypeIndex struct {
	attrs   map[string]map[string]indexedAttr // entity QN -> lower attr name -> attr
	parents map[string]string                 // entity QN -> generalization QN
	assocs  map[string][2]string              // lower association QN -> {from, to}
}

type indexedAttr struct {
	name string
	kind string
}

// lookup finds attr on entityQN or the generalization that declares it. known
// is false when the chain runs through an entity the index does not hold.
func (ix memberTypeIndex) lookup(entityQN, attr string) (declaring string, a indexedAttr, found, known bool) {
	lower := strings.ToLower(attr)
	seen := map[string]bool{}
	for cur := entityQN; cur != ""; cur = ix.parents[cur] {
		if seen[cur] {
			break
		}
		seen[cur] = true
		attrs, ok := ix.attrs[cur]
		if !ok {
			return "", indexedAttr{}, false, false
		}
		if hit, ok := attrs[lower]; ok {
			return cur, hit, true, true
		}
	}
	return "", indexedAttr{}, false, true
}

// chain is entityQN followed by its generalizations, as far as the index knows.
func (ix memberTypeIndex) chain(entityQN string) []string {
	var out []string
	seen := map[string]bool{}
	for cur := entityQN; cur != "" && !seen[cur]; cur = ix.parents[cur] {
		seen[cur] = true
		out = append(out, cur)
	}
	return out
}

// follow navigates the association named seg from fromQN, returning its
// qualified name and the entity at the other end. An unqualified name is tried
// in the module of the entity and of each generalization, which is where the
// page builder looks for it too.
func (ix memberTypeIndex) follow(seg, fromQN string) (assocQN, target string, ok bool) {
	chain := ix.chain(fromQN)
	var candidates []string
	if strings.Contains(seg, ".") {
		candidates = []string{seg}
	} else {
		for _, e := range chain {
			if m := qualifiedNameModule(e); m != "" {
				candidates = append(candidates, m+"."+seg)
			}
		}
	}
	for _, qn := range candidates {
		ends, found := ix.assocs[strings.ToLower(qn)]
		if !found {
			continue
		}
		for _, e := range chain {
			switch e {
			case ends[0]:
				return qn, ends[1], true
			case ends[1]:
				return qn, ends[0], true
			}
		}
	}
	return "", "", false
}

// checkMemberTypeIndex builds the index from the project and the script.
func checkMemberTypeIndex(ctx *ExecContext, sc *scriptContext) memberTypeIndex {
	ix := memberTypeIndex{
		attrs:   map[string]map[string]indexedAttr{},
		parents: map[string]string{},
		assocs:  map[string][2]string{},
	}
	// No project (a script checked on its own): the index is the script's.
	if ctx == nil || ctx.Backend == nil {
		return overlayScriptMembers(ix, sc)
	}
	if modules, err := getModulesFromCache(ctx); err == nil {
		moduleNames := make(map[model.ID]string, len(modules))
		for _, m := range modules {
			moduleNames[m.ID] = m.Name
		}
		if dms, err := projectDomainModels(ctx); err == nil {
			byID := map[model.ID]string{}
			for _, dm := range dms {
				mod := moduleNames[dm.ContainerID]
				if mod == "" {
					continue
				}
				for _, ent := range dm.Entities {
					qn := mod + "." + ent.Name
					byID[ent.ID] = qn
					attrs := make(map[string]indexedAttr, len(ent.Attributes))
					for _, a := range ent.Attributes {
						attrs[strings.ToLower(a.Name)] = indexedAttr{name: a.Name, kind: attributeKind(a.Type)}
					}
					ix.attrs[qn] = attrs
					if ent.GeneralizationRef != "" {
						ix.parents[qn] = ent.GeneralizationRef
					}
				}
			}
			for _, dm := range dms {
				mod := moduleNames[dm.ContainerID]
				if mod == "" {
					continue
				}
				for _, a := range dm.Associations {
					if from, to := byID[a.ParentID], byID[a.ChildID]; from != "" && to != "" {
						ix.assocs[strings.ToLower(mod+"."+a.Name)] = [2]string{from, to}
					}
				}
				for _, a := range dm.CrossAssociations {
					if from := byID[a.ParentID]; from != "" && a.ChildRef != "" {
						ix.assocs[strings.ToLower(mod+"."+a.Name)] = [2]string{from, a.ChildRef}
					}
				}
			}
		}
	}
	return overlayScriptMembers(ix, sc)
}

// overlayScriptMembers lays what the script declares over the project's index.
func overlayScriptMembers(ix memberTypeIndex, sc *scriptContext) memberTypeIndex {
	if sc == nil {
		return ix
	}
	for qn, decl := range sc.entityDecls {
		attrs := map[string]indexedAttr{}
		if decl.CreateOrModify {
			// Modify keeps what it does not name.
			for k, v := range ix.attrs[qn] {
				attrs[k] = v
			}
		}
		for _, a := range decl.Attributes {
			attrs[strings.ToLower(a.Name)] = indexedAttr{name: a.Name, kind: astAttributeKind(a.Type)}
		}
		ix.attrs[qn] = attrs
		delete(ix.parents, qn)
		if gen := sc.entityGeneralizations[strings.ToLower(qn)]; gen != "" {
			ix.parents[qn] = gen
		}
	}
	for qn := range sc.alteredEntities {
		// An ALTER ENTITY can add, drop or retype a member; what it leaves is not
		// worked out here, so the entity's members go unjudged.
		delete(ix.attrs, qn)
	}
	for qn, ends := range sc.associationEnds {
		ix.assocs[qn] = ends
	}
	return ix
}

// checkInputBinding judges one input widget's attribute — a built-in input, or
// a combo box in enumeration mode — against the entity its enclosing data
// container supplies.
func (v *attributeScopeValidator) checkInputBinding(w *ast.WidgetV3, enclosing dataContext) {
	rule, isComboBox, ok := inputAttributeRuleFor(w)
	if !ok {
		return
	}
	attr := w.GetAttribute()
	if attr == "" && isComboBox && comboBoxMode(w) == "database" {
		attr = w.GetStringProp("databaseAttributeString")
	}
	attr = strings.TrimPrefix(attr, "$currentObject/")
	if attr == "" || strings.HasPrefix(attr, "$") || enclosing.unresolved || len(enclosing.entities) == 0 {
		return
	}
	entity := enclosing.entities[len(enclosing.entities)-1]
	segs := strings.Split(attr, "/")
	for _, seg := range segs[:len(segs)-1] {
		_, next, ok := v.types.follow(seg, entity)
		if !ok {
			return // a hop this index cannot resolve: not judged
		}
		entity = next
	}
	name := segs[len(segs)-1]
	if strings.Contains(name, ".") {
		return
	}
	declaring, a, found, known := v.types.lookup(entity, name)
	if !known {
		return
	}
	if !found {
		// The CE1613 below was measured for the built-in inputs only; a combo
		// box over a bare association is left unjudged rather than guessed at.
		if len(segs) == 1 && !isComboBox {
			if assocQN, _, isAssoc := v.types.follow(name, entity); isAssoc {
				v.errs = append(v.errs, fmt.Sprintf(
					"widget `%s` (%s): `%s` is the association %s, not an attribute — an input widget binds an "+
						"attribute, so the page would name the attribute `%s.%s`, which does not exist, and mxbuild "+
						"reports CE1613 \"The selected attribute … no longer exists\" (which also stops it reporting "+
						"anything else). Bind an attribute over it (`%s/<Attribute>`), or select the associated object "+
						"with a `combobox` over the association ["+widgetAttributeTypeRule+"]",
					w.Name, strings.ToLower(w.Type), name, assocQN, entity, name, name))
				return
			}
		}
		if !isComboBox {
			v.reportMissingAttribute(w, "", attr, entity, name)
		}
		return
	}
	if a.kind == "" || !measuredAttributeKinds[a.kind] || rule.accepts[a.kind] {
		return
	}
	msg := fmt.Sprintf(
		"widget `%s` (%s) is bound to %s.%s, a%s %s attribute; the %s widget takes only attributes of type %s — "+
			"mxbuild reports %s \"Only attributes of type %s are allowed here.\"",
		w.Name, strings.ToLower(w.Type), declaring, a.name, anPrefix(a.kind), a.kind, rule.label, rule.listed, rule.code, rule.listed)
	if alt := widgetsAccepting(a.kind); alt != "" {
		msg += " For " + a.kind + ", use " + alt + "."
	}
	v.errs = append(v.errs, msg+" ["+widgetAttributeTypeRule+"]")
}

// auditMembers are the system members an entity's audit settings add —
// bindable by these names, and never in its attribute list.
var auditMembers = kindSet("createddate", "changeddate", "owner", "changedby")

// reportMissingAttribute is CE1613 for a binding to an attribute the entity
// in context does not have (ako/mxcli#555). Exec wrote the name as given and
// mxbuild refused it; check had resolved neither an input widget's Attribute
// nor a template parameter. where names the template parameter ("" for the
// widget's own Attribute).
func (v *attributeScopeValidator) reportMissingAttribute(w *ast.WidgetV3, where, path, entity, name string) {
	if auditMembers[strings.ToLower(name)] {
		return
	}
	var have []string
	for _, e := range v.types.chain(entity) {
		for _, a := range v.types.attrs[e] {
			have = append(have, a.name)
		}
	}
	sort.Strings(have)
	const max = 12
	list := strings.Join(have, ", ")
	if len(have) > max {
		list = fmt.Sprintf("%s and %d more", strings.Join(have[:max], ", "), len(have)-max)
	}
	if list == "" {
		list = "none"
	}
	subject := fmt.Sprintf("widget `%s` (%s)", w.Name, strings.ToLower(w.Type))
	if where != "" {
		subject += " " + where
	}
	v.errs = append(v.errs, fmt.Sprintf(
		"%s binds `%s`, but %s has no attribute %s (attributes: %s) — mxbuild reports CE1613 "+
			"\"The selected attribute '%s.%s' no longer exists\"",
		subject, path, entity, name, list, entity, name))
}

// checkTemplateParams judges the attribute bindings among a widget's template
// parameters (ContentParams, CaptionParams) the way checkInputBinding judges an
// input's Attribute: against the entity the enclosing data container supplies,
// the context exec qualifies a bare name with (ako/mxcli#555). An expression,
// a $variable path, or a context this walk cannot resolve is not judged.
func (v *attributeScopeValidator) checkTemplateParams(w *ast.WidgetV3, enclosing dataContext) {
	if enclosing.unresolved || len(enclosing.entities) == 0 {
		return
	}
	for _, set := range []struct {
		key    string
		params []ast.ParamAssignmentV3
	}{{"ContentParams", w.GetContentParams()}, {"CaptionParams", w.GetCaptionParams()}} {
		for _, p := range set.params {
			raw, ok := p.Value.(string)
			if !ok || isTemplateExpression(raw) {
				continue
			}
			path := strings.TrimPrefix(strings.TrimSpace(raw), "$currentObject/")
			if path == "" || strings.HasPrefix(path, "$") {
				continue
			}
			entity := enclosing.entities[len(enclosing.entities)-1]
			segs := strings.Split(path, "/")
			resolved := true
			for _, seg := range segs[:len(segs)-1] {
				_, next, ok := v.types.follow(seg, entity)
				if !ok {
					resolved = false
					break
				}
				entity = next
			}
			name := segs[len(segs)-1]
			if !resolved || name == "" || strings.Contains(name, ".") {
				continue
			}
			if _, _, found, known := v.types.lookup(entity, name); !known || found {
				continue
			}
			if _, _, isAssoc := v.types.follow(name, entity); isAssoc {
				continue
			}
			v.reportMissingAttribute(w, fmt.Sprintf("%s {%d}", set.key, p.Index), raw, entity, name)
		}
	}
}

func anPrefix(kind string) string {
	if strings.ContainsAny(kind[:1], "AEIOU") {
		return "n"
	}
	return ""
}

// widgetsAccepting lists the input widgets that take kind, for the message.
// The classic drop-down is left out (MDL-WIDGET40 on the React client); the
// pluggable combo box is offered for an enumeration in its place.
func widgetsAccepting(kind string) string {
	var out []string
	for k, r := range builtinInputAttributeRules {
		if k != "dropdown" && r.accepts[kind] {
			out = append(out, "`"+k+"`")
		}
	}
	if kind == "Enumeration" {
		out = append(out, "`combobox`")
	}
	sort.Strings(out)
	return strings.Join(out, " or ")
}

// usesReactClient reports whether the project's Web UI setting is the React
// client outright. MigrationMode is NOT the React client for this purpose:
// measured on 11.14.0, a classic drop-down is CE0582 under "Yes" and builds
// clean under both "No" and "MigrationMode". A setting that cannot be read is
// treated as not React — the check stays quieter than mxbuild, never louder.
func usesReactClient(ctx *ExecContext) bool {
	if ctx == nil || ctx.Backend == nil {
		return false
	}
	ps, err := ctx.Backend.GetProjectSettings()
	if err != nil || ps == nil || ps.WebUI == nil {
		return false
	}
	return ps.WebUI.UseOptimizedClient == "Yes"
}

// layoutIsNative reports whether the named layout (Module.Name) is a native
// one. A page on it is not rendered by the React client, so the React-only
// CE0582 does not apply: measured on 11.13.0, a classic drop-down on a
// NativePhone_Default page builds clean (ako/mxcli#953). An empty or
// unresolvable name — a snippet, a layout this script creates — reads as web,
// which keeps the check exactly as loud as it was.
func layoutIsNative(ctx *ExecContext, layout string) bool {
	if ctx == nil || ctx.Backend == nil || layout == "" {
		return false
	}
	h, err := getHierarchy(ctx)
	if err != nil || h == nil {
		return false
	}
	layouts, err := ctx.Backend.ListLayouts()
	if err != nil {
		return false
	}
	for _, l := range layouts {
		if strings.EqualFold(h.GetQualifiedName(l.ContainerID, l.Name), layout) {
			return l.Native
		}
	}
	return false
}

// checkReactUnsupported refuses the classic drop-down on a React-client project
// (MDL-WIDGET40).
//
// Unlike MPR012 (legacy image widgets), which stays a lint rule so that
// describe → exec of a page holding one does not warn, this is an error mxbuild
// raises: a React-client project with a Forms$DropDown does not build, whether
// the page came from a script or from describe. Reporting it before the write
// costs a round-trip of a describe'd legacy page nothing it did not already have.
func (v *attributeScopeValidator) checkReactUnsupported(w *ast.WidgetV3) {
	if !v.reactClient || w.TypeIsGeneric || !strings.EqualFold(w.Type, "dropdown") {
		return
	}
	v.errs = append(v.errs, fmt.Sprintf(
		"widget `%s`: the classic drop-down (Forms$DropDown) is not supported by the React client this project "+
			"uses (Web UI setting OptimizedClient: Yes) — mxbuild reports CE0582 \"Widget drop-down is not "+
			"supported in React client.\" Use `combobox` over the same attribute ["+reactDropDownRule+"]", w.Name))
}
