// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ALTER STYLING was the one statement whose entire job is writing design
// properties, and the one statement MDL-WIDGET11 never looked at.
//
// ValidateDesignPropertiesForStatement switches on CreatePageStmtV3,
// CreateSnippetStmtV3 and AlterPageStmt (its INSERT / REPLACE trees).
// *ast.AlterStylingStmt is not among them, so an unsupported key was silent
// until mxbuild:
//
//	alter styling on page … widget lvThings set ( 'Remove empty text': on );
//	  mxcli check --references -> Check passed!
//	  mxcli exec               -> Updated styling on widget "lvThings"
//	  mxcli docker check       -> [CE6083] "Design property Remove empty text is
//	                              not supported by your theme."  1 error
//
// Measured on a blank 11.12.2 project; the same statement with 'Row size' =
// 'Small' is 0 errors (ako/mxcli#509).
//
// # Two passes: any type without the document, the widget's own type with it
//
// The other three statements CARRY the widget, so the validator reads its type
// off the AST and checks the key against that type's properties.
// AlterStylingStmt carries only a widget NAME (mdl/ast/ast_styling.go) — it
// names a widget already stored, whose $Type lives only in the document.
//
// This pass (validateAlterStylingDesignProps) runs with no project at all —
// `mxcli check script.mdl`, the corpus default — so it asks the question that
// IS answerable without the document: does ANY widget type in the theme declare
// this key? A key no type declares cannot be right for this widget either
// (#509's reported case).
//
// The reference pass then opens the document (validateAlterStylingStoredTypes,
// the pageProbe capability, opened as validateAlterSetProperties opens it) and
// judges every key the first pass let through against the widget's OWN type:
// the stored $Type → theme group mapping is the one ALTER PAGE SET's routing
// already uses (designPropsKeyOfStored), and the group's keys come from
// GetPropertiesForWidget, so the `Widget` group's inherited keys are included
// rather than re-derived. No third resolver (ako/mxcli#613). That is what
// catches a key declared for some OTHER type — `Row size`, a ListView key, on a
// DynamicText is CE6083, and the any-type question passed it.
//
// It has to be the reference pass, not a project opened from this one: the
// validators' own read-only connection (openProjectForValidation) refuses
// OpenPageForMutation — "not connected for writing" — which a mock backend
// does not, so only a run against a real project shows it. Splitting by key
// (the second pass reports only keys the first accepted) is what keeps one key
// from being reported twice.
//
// Where the stored type cannot be established — the widget is not stored, its
// type has no theme group, or the script itself creates or grows the document
// — the second pass is silent and the first pass's answer stands.
// Under-reporting, never over-reporting, is the only safe direction for a check
// that cannot see what it is judging.
//
// mdlKeywordToDesignPropsKey and bsonTypeToDesignPropsKey remain two tables;
// this consumes the second and unifies neither (out of scope for #613).

// validateAlterStylingDesignProps reports (MDL-WIDGET11) an ALTER STYLING design
// property key that no widget type in the project's theme declares.
//
// A warning, never an error, matching the authoring paths: a newer theme may add
// keys the snapshot lacks, and blocking a script that would have worked is worse
// than the gap it replaces.
//
// reg is nil, or empty, when the project has no themesource defining design
// properties. There is then nothing to be undeclared relative to, and the honest
// answer is silence — otherwise every key in every script becomes a warning.
func validateAlterStylingDesignProps(prog *ast.Program, reg *ThemeRegistry) []linter.Violation {
	if prog == nil || reg == nil || len(reg.WidgetProperties) == 0 {
		return nil
	}
	declared := allDeclaredDesignPropertyNames(reg)
	if len(declared) == 0 {
		return nil
	}

	var out []linter.Violation
	for _, stmt := range prog.Statements {
		s, ok := stmt.(*ast.AlterStylingStmt)
		if !ok {
			continue
		}
		label := alterStylingLabel(s)
		for _, a := range s.Assignments {
			// CLASS / STYLE / DYNAMICCLASSES are appearance fields, not design
			// properties — the visitor marks them, so this does not have to
			// re-derive which names are which.
			if a.IsCSS || a.Property == "" {
				continue
			}
			if declaredHas(declared, a.Property) {
				// Declared, but possibly in a shape ALTER STYLING cannot write:
				// a multi-select property is a compound, and a StylingAssignment
				// carries one flat value (ako/mxcli#511).
				if multi := multiSelectPropertyNamed(reg, a.Property); multi != nil {
					out = append(out, linter.Violation{
						RuleID:   "MDL-WIDGET12",
						Severity: linter.SeverityWarning,
						Message: fmt.Sprintf("%s: design property %q on %q takes a SET of options, "+
							"and ALTER STYLING can only write one value — mxbuild refuses the result "+
							"with CE6084", label, a.Property, s.WidgetName),
						Location: linter.Location{DocumentType: "page", DocumentName: s.ContainerName.String()},
						Suggestion: fmt.Sprintf("Set it inline instead: `DesignProperties: ('%s': ('%s': on))` "+
							"on the widget in CREATE PAGE, or in an ALTER PAGE REPLACE.",
							a.Property, firstOptionName(multi, a.Value)),
					})
				}
				continue
			}
			// Declared after all — under an OLD name the theme renamed. mxbuild
			// answers that with CE6087, not CE6083, and the fix is the current
			// name, not a spelling near-miss.
			if r := renamedAnywhere(reg, a.Property, a.Value); r != nil {
				out = append(out, linter.Violation{
					RuleID:   "MDL-WIDGET11",
					Severity: linter.SeverityWarning,
					Message: fmt.Sprintf("%s: sets design property %q on %q, which the theme renamed to %q "+
						"— mxbuild reports the old name as CE6087 (\"Design properties have been renamed "+
						"in your theme\")", label, a.Property, s.WidgetName, r.NewKey),
					Location:   linter.Location{DocumentType: "page", DocumentName: s.ContainerName.String()},
					Suggestion: renamedStylingSuggestion(r, a.Value),
				})
				continue
			}
			out = append(out, linter.Violation{
				RuleID:   "MDL-WIDGET11",
				Severity: linter.SeverityError, // CE6083, as for CREATE PAGE (ako/mxcli#2236)
				Message: fmt.Sprintf("%s: sets design property %q on %q, which no widget type "+
					"in this project's theme declares — mxbuild reports this as CE6083",
					label, a.Property, s.WidgetName),
				Location:   linter.Location{DocumentType: "page", DocumentName: s.ContainerName.String()},
				Suggestion: undeclaredDesignPropSuggestion(declared, a.Property, s.WidgetName),
			})
		}
	}
	return out
}

// allDeclaredDesignPropertyNames is every design-property name the project's
// theme declares, across every widget type.
//
// Built from the registry's own groups rather than from one type's, which is
// what makes an INHERITED key (`Align self`, declared under `Widget` and applying
// to every widget) resolve here. Reading a single type's list is how a design
// property that works gets reported as unknown.
func allDeclaredDesignPropertyNames(reg *ThemeRegistry) map[string]string {
	out := map[string]string{}
	for _, props := range reg.WidgetProperties {
		for i := range props {
			if props[i].Name != "" {
				out[strings.ToLower(props[i].Name)] = props[i].Name
			}
		}
	}
	return out
}

func declaredHas(declared map[string]string, key string) bool {
	_, ok := declared[strings.ToLower(key)]
	return ok
}

// undeclaredDesignPropSuggestion points at the widget-scoped command rather than
// listing every key in the theme: the full list spans every widget type and is
// not the set valid HERE, so printing it would answer a question nobody asked.
// A case-only miss is called out first — design-property keys are case-sensitive,
// and that is the commonest real mistake.
func undeclaredDesignPropSuggestion(declared map[string]string, key, widgetName string) string {
	if exact, ok := declared[strings.ToLower(key)]; ok && exact != key {
		return fmt.Sprintf("Design-property keys are case-sensitive — did you mean %q?", exact)
	}
	names := make([]string, 0, len(declared))
	for _, n := range declared {
		names = append(names, n)
	}
	sort.Strings(names)
	if near := nearestKey(key, names); near != "" {
		return fmt.Sprintf("Did you mean %q? Run `mxcli show design properties for <widget type>` "+
			"to list the keys valid on %s.", near, widgetName)
	}
	return fmt.Sprintf("Run `mxcli show design properties for <widget type>` to list the keys "+
		"valid on %s, or `mxcli show design properties` for every type.", widgetName)
}

// multiSelectPropertyNamed returns the theme's declaration of key when it is
// multi-select, else nil. Searched across every widget group for the same reason
// the key check is: this pass cannot tell which widget the statement names.
func multiSelectPropertyNamed(reg *ThemeRegistry, key string) *ThemeProperty {
	if reg == nil {
		return nil
	}
	for _, props := range reg.WidgetProperties {
		for i := range props {
			if strings.EqualFold(props[i].Name, key) && props[i].MultiSelect {
				return &props[i]
			}
		}
	}
	return nil
}

// firstOptionName picks the option to show in the suggested spelling: the one the
// author wrote when it is a real option, else the property's first declared one,
// so the example is always something that would actually work.
func firstOptionName(p *ThemeProperty, authored string) string {
	for _, o := range p.Options {
		if strings.EqualFold(o.Name, authored) {
			return o.Name
		}
	}
	if len(p.Options) > 0 {
		return p.Options[0].Name
	}
	return authored
}

// renamedAnywhere finds key among the old names of any widget type's design
// properties. ALTER STYLING carries only a widget NAME, so — as for the
// declared check above — it asks the whole theme rather than one type. Groups
// are visited in a fixed order so the answer does not depend on map order.
func renamedAnywhere(reg *ThemeRegistry, key, value string) *designPropRename {
	groups := make([]string, 0, len(reg.WidgetProperties))
	for g := range reg.WidgetProperties {
		groups = append(groups, g)
	}
	sort.Strings(groups)
	for _, g := range groups {
		if r := findRenamedThemeProp(reg.WidgetProperties[g], key, value); r != nil {
			return r
		}
	}
	return nil
}

// renamedStylingSuggestion turns a rename into what to write INSTEAD in an
// ALTER STYLING script. A flat replacement ('Key': 'Option' or 'Key': on) is
// one assignment; a compound one — a Spacing side, or a multi-select option —
// is a value ALTER STYLING cannot write (one flat value per assignment, the
// MDL-WIDGET12 limit), so it points at the inline form.
func renamedStylingSuggestion(r *designPropRename, value string) string {
	if r.Replacement == "" {
		return renamedDesignPropSuggestion(r, value)
	}
	if strings.Contains(r.Replacement, "[") {
		return fmt.Sprintf("ALTER STYLING cannot write its current form, which is a compound: set "+
			"`DesignProperties: (%s)` on the widget in CREATE PAGE, or in an ALTER PAGE REPLACE.", r.Replacement)
	}
	// 'Key': 'Value'  →  set ( 'Key': 'Value' ), the canonical list (R3)
	if strings.Contains(r.Replacement, "': ") {
		return fmt.Sprintf("Write it as `set ( %s )`.", r.Replacement)
	}
	return renamedDesignPropSuggestion(r, value)
}
