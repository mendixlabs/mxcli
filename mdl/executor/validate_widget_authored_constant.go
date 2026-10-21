// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

// authoredConstantUndeclared is the rule ID for a value the script writes, by the
// property's key, over a definition constant the installed widget has no property
// for.
const authoredConstantUndeclared = "MDL-WIDGET49"

// validateAuthoredConstantUndeclared is `check`'s twin of the build-time refusal
// in refuseUndeclaredMappedKey: a mapping that carries a constant (`value` in the
// definition) is overridden when the script names the property by its key, and
// when the installed widget declares no such property the builder skips it —
// `gallery g (…, advanced: true)` on a Gallery 3.x, which has no `advanced`,
// checked clean and then failed exec (or, before the refusal, stored nothing).
//
// An authored value equal to the constant is left alone, as the builder leaves
// it: it drops nothing the definition would not have written anyway.
//
// The widget's declared keys come from the project's installed package when
// there is one, and otherwise from the embedded template the builder starts
// from; with neither, nothing is known and nothing is reported.
func validateAuthoredConstantUndeclared(w *ast.WidgetV3, def *WidgetDefinition, projectPath, locationPrefix string) []linter.Violation {
	if w == nil || def == nil {
		return nil
	}
	var declared map[string]bool
	var out []linter.Violation
	seen := map[string]bool{}
	check := func(ms []PropertyMapping) {
		for _, m := range ms {
			if m.Value == "" || m.Operation == "selection" || seen[m.PropertyKey] {
				continue
			}
			raw, ok := lookupProperty(w.Properties, m.PropertyKey)
			if !ok {
				continue
			}
			authored := stringifyAny(raw)
			if authored == "" || authored == m.Value {
				continue
			}
			if declared == nil {
				declared = declaredWidgetKeys(projectPath, def.WidgetID)
			}
			if len(declared) == 0 || declared[strings.ToLower(m.PropertyKey)] {
				continue
			}
			seen[m.PropertyKey] = true
			out = append(out, linter.Violation{
				RuleID:   authoredConstantUndeclared,
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"%s: %s has no property `%s` in the installed widget — its definition maps it to the constant %q, "+
						"and the value `%s` written here would be dropped",
					locationPrefix, widgetLabel(w.Name, def.MDLName), m.PropertyKey, m.Value, authored),
				Suggestion: "Remove the property, or update the widget package if a newer version declares it.",
			})
		}
	}
	check(def.PropertyMappings)
	for _, mode := range def.Modes {
		check(mode.PropertyMappings)
	}
	return out
}

var (
	declaredWidgetKeysMu    sync.Mutex
	declaredWidgetKeysCache = map[string]map[string]bool{}
)

// declaredWidgetKeys returns the lower-cased top-level property keys a widget
// declares: the project's installed package first, then the embedded template.
// Empty when neither is known.
func declaredWidgetKeys(projectPath, widgetID string) map[string]bool {
	cacheKey := projectPath + "\x00" + widgetID
	declaredWidgetKeysMu.Lock()
	if d, ok := declaredWidgetKeysCache[cacheKey]; ok {
		declaredWidgetKeysMu.Unlock()
		return d
	}
	declaredWidgetKeysMu.Unlock()

	out := map[string]bool{}
	if projectPath != "" {
		projectDir := projectPath
		if strings.EqualFold(filepath.Ext(projectDir), ".mpr") {
			projectDir = filepath.Dir(projectDir)
		}
		if mpkPath, err := mpk.FindMPK(projectDir, widgetID); err == nil && mpkPath != "" {
			if wd, err := mpk.ParseMPKForWidget(mpkPath, widgetID); err == nil && wd != nil {
				for _, p := range wd.Properties {
					out[strings.ToLower(p.Key)] = true
				}
			}
		}
	}
	if len(out) == 0 {
		for k := range widgetPropertyTypes(widgetID) {
			out[k] = true
		}
	}

	declaredWidgetKeysMu.Lock()
	declaredWidgetKeysCache[cacheKey] = out
	declaredWidgetKeysMu.Unlock()
	return out
}
