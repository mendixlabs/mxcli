// SPDX-License-Identifier: Apache-2.0

// A selection-typed widget property — a Data grid 2's or Gallery's
// `itemSelection` — and the two ways MDL lets a script name it.
//
// The mapped keyword, `Selection: Single`, is the older spelling; the property's
// own key, `itemSelection: Single`, is how every other widget property is
// addressed and what DESCRIBE emits for a widget whose keyword is not mapped.
// Both name ONE property, and that property keeps its value in the WidgetValue's
// `Selection` field — not in `PrimitiveValue`, where a plain scalar lives.
//
// Before, only the keyword was read as a selection. The key fell through to the
// explicit-property pass, which wrote it as a scalar: `PrimitiveValue 'Single'`,
// `Selection 'None'`, a grid with no selection, and nothing reported — check,
// exec and mx check were all clean. The pass that decides which names the mapping
// pass already handled compared them against the mappings' SOURCE keywords, so the
// property's KEY matched nothing.
//
// This file is the one place that reads either spelling, validates the value
// against what the widget declares, and decides what happens when both are given.
// The engine (resolveMapping, the explicit pass) and `check`
// (validateSelectionProperties) both call it, so they cannot disagree.

package executor

import (
	"fmt"
	"strings"
	"sync"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
	mwidgets "github.com/mendixlabs/mxcli/modelsdk/widgets"
)

// selectionValueRule is the rule ID for a selection property given a value the
// widget does not offer, or two spellings that disagree. Not gated on the
// language version: neither input ever produced what the script said, so there
// is no earlier meaning to preserve (compare refuseUndeclaredMappedKey, which is
// ungated for the same reason).
const selectionValueRule = "MDL-WIDGET50"

// allSelectionTypes is the set a selection property falls back to when the
// widget's template names none. It is the whole of the Mendix enumeration, so
// the fallback never refuses a value the widget could take.
var allSelectionTypes = []string{"None", "Single", "Multi"}

// selectionProblem is a selection value `check` and `exec` both refuse.
type selectionProblem struct{ msg string }

func (p *selectionProblem) Error() string { return p.msg }

// execError is the refusal exec makes. check reports the rule ID itself, so the
// message carries none; exec has no such wrapper and names it here.
func (p *selectionProblem) execError() error {
	return mdlerrors.NewValidation(p.msg + " [" + selectionValueRule + "]")
}

// selectionAuthored reads what the script wrote for a selection property: under
// the mapped keyword (the mapping's Source), and under the property's own key or
// one of its aliases. Either is "" when not written.
func selectionAuthored(w *ast.WidgetV3, m PropertyMapping) (mapped, byKey string) {
	if m.Source != "" {
		if v, ok := lookupProperty(w.Properties, m.Source); ok {
			mapped = stringifyAny(v)
		}
	}
	for _, name := range append([]string{m.PropertyKey}, m.MdlAliases...) {
		if name == "" || strings.EqualFold(name, m.Source) {
			continue
		}
		if v, ok := lookupProperty(w.Properties, name); ok {
			if s := stringifyAny(v); s != "" {
				return mapped, s
			}
		}
	}
	return mapped, ""
}

// resolveSelectionValue returns the selection a script gives a property, in the
// canonical spelling (None, Single or Multi), or "" when the script gives none.
//
// allowed is what the widget declares for the property (empty: all three).
// keywordShared says the mapped keyword feeds more than one selection property,
// so it names none of them in particular and cannot contradict a key.
//
// Precedence, when both spellings are given: they must agree. A disagreement is
// refused rather than resolved, because either choice is a guess and "the last
// pass wins" would make the stored value depend on the order of two passes.
func resolveSelectionValue(w *ast.WidgetV3, label string, m PropertyMapping, allowed []string, keywordShared bool) (string, *selectionProblem) {
	if len(allowed) == 0 {
		allowed = allSelectionTypes
	}
	mapped, byKey := selectionAuthored(w, m)
	canon := func(spelling, raw string) (string, *selectionProblem) {
		if raw == "" {
			return "", nil
		}
		c := canonicalSelection(raw)
		for _, a := range allowed {
			if strings.EqualFold(c, a) {
				return a, nil
			}
		}
		return "", &selectionProblem{fmt.Sprintf(
			"%s: `%s: %s` is not a selection this widget offers — valid values are %s",
			label, spelling, raw, strings.Join(allowed, ", "))}
	}
	mappedVal, p := canon(m.Source, mapped)
	if p != nil {
		return "", p
	}
	keyVal, p := canon(m.PropertyKey, byKey)
	if p != nil {
		return "", p
	}
	if mappedVal != "" && keyVal != "" && mappedVal != keyVal && !keywordShared {
		return "", &selectionProblem{fmt.Sprintf(
			"%s: `%s: %s` and `%s: %s` both set the selection and disagree — keep one of them",
			label, m.Source, mapped, m.PropertyKey, byKey)}
	}
	if keyVal != "" {
		return keyVal, nil
	}
	return mappedVal, nil
}

// selectionMappingsOf lists the selection mappings of a definition — top level
// and in every mode, one per property key.
func selectionMappingsOf(def *WidgetDefinition) []PropertyMapping {
	if def == nil {
		return nil
	}
	var out []PropertyMapping
	seen := map[string]bool{}
	add := func(ms []PropertyMapping) {
		for _, m := range ms {
			if m.Operation == "selection" && !seen[m.PropertyKey] {
				seen[m.PropertyKey] = true
				out = append(out, m)
			}
		}
	}
	add(def.PropertyMappings)
	for _, mode := range def.Modes {
		add(mode.PropertyMappings)
	}
	return out
}

// selectionKeywordShared reports whether the mapped keyword feeds several of the
// definition's selection properties.
func selectionKeywordShared(def *WidgetDefinition, source string) bool {
	if source == "" {
		return false
	}
	n := 0
	for _, m := range selectionMappingsOf(def) {
		if m.Source == source {
			n++
		}
	}
	return n > 1
}

var templateSelectionTypesCache sync.Map // widgetID → map[propertyKey][]string

// templateSelectionTypes returns, per property key, the selection kinds the
// widget's embedded template declares for its selection-typed properties. It is
// the check-time twin of PropertyTypeIDEntry.SelectionTypes, which the builder
// reads from the same template; empty when the widget has no embedded template.
func templateSelectionTypes(widgetID string) map[string][]string {
	if v, ok := templateSelectionTypesCache.Load(widgetID); ok {
		return v.(map[string][]string)
	}
	out := map[string][]string{}
	if tmpl, err := mwidgets.GetTemplate(widgetID); err == nil && tmpl != nil {
		objType, _ := tmpl.Type["ObjectType"].(map[string]any)
		pts, _ := objType["PropertyTypes"].([]any)
		for _, pt := range pts {
			m, ok := pt.(map[string]any)
			if !ok {
				continue // leading array marker
			}
			vt, _ := m["ValueType"].(map[string]any)
			if vt == nil || asString(vt["Type"]) != "Selection" {
				continue
			}
			out[asString(m["PropertyKey"])] = mwidgets.SelectionTypesOf(vt)
		}
	}
	templateSelectionTypesCache.Store(widgetID, out)
	return out
}

// validateSelectionProperties refuses, at check time, a selection property given
// a value its widget does not offer, or given under both spellings with two
// different values — the same refusals the engine makes on exec.
//
// It covers the definition's selection mappings and any selection-typed property
// of the template no mapping reaches (those are written by the explicit-property
// pass, which applies the same rule).
func validateSelectionProperties(w *ast.WidgetV3, def *WidgetDefinition, locationPrefix string) []linter.Violation {
	if w == nil || def == nil {
		return nil
	}
	types := templateSelectionTypes(def.WidgetID)
	mappings := selectionMappingsOf(def)
	covered := map[string]bool{}
	for _, m := range mappings {
		covered[m.PropertyKey] = true
	}
	for key := range types {
		if !covered[key] {
			mappings = append(mappings, PropertyMapping{PropertyKey: key, Operation: "selection"})
		}
	}
	var out []linter.Violation
	for _, m := range mappings {
		_, p := resolveSelectionValue(w, widgetLabel(w.Name, def.MDLName), m, types[m.PropertyKey],
			selectionKeywordShared(def, m.Source))
		if p == nil {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:     selectionValueRule,
			Severity:   linter.SeverityError,
			Message:    locationPrefix + ": " + p.msg,
			Suggestion: "Write the selection once — `itemSelection: Single` or `Selection: Single` — with None, Single or Multi.",
		})
	}
	return out
}
