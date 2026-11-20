// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// A tab page is not a widget: Forms$TabPage holds a Name, a Caption that is a
// plain Texts$Text (no ClientTemplate, so no parameters), RefreshOnShow,
// ConditionalVisibilitySettings, Widgets and a Badge — and no Appearance, so
// no Class, Style, DynamicClasses or DesignProperties. The page-widget
// vocabulary MDL-WIDGET07 checks against is a union across widget types, so
// `CaptionParams:` or `Class:` on a tab page passed it, and buildTabPageV3,
// which reads only the caption, visibility and children, dropped them
// (mendixlabs/mxcli#1465). The refusal cannot be false: the metamodel has
// nowhere to put them.
//
// It is a new rejection, so per ADR-0011 it applies under `mdl 1;` only; a
// headerless script keeps the old outcome — the property is dropped — and is
// told so with MDL-V1-TABPAGEPROP.
var tabPagePropDropped = langver.Change{
	Code:  "MDL-V1-TABPAGEPROP",
	Since: langver.V1,
	Old:   "A property a tab page cannot store (CaptionParams, Class, Style, …) is accepted and dropped on write",
	New:   "a refusal (MDL-WIDGET44), with nothing written",
}

// tabPageStoredProps are the keys buildTabPageV3 writes. `visibleroles` is the
// roles of `Visible: roles (…)`, applied by childVisibility
// (mendixlabs/mxcli#1460): it was missing here, so check refused — under mdl 1 —
// or warned "not written" — headerless — about a property that is written, and
// that MxBuild accepts for a role of the tab page's own module. The badge keys
// are not listed because they are not page-widget properties at all
// (isTabPageBadgeProp), so tabPageDroppedProps never reaches them.
var tabPageStoredProps = map[string]bool{
	"caption": true, "visible": true, "visibleif": true, "visiblewhen": true,
	"visibleroles": true,
}

// tabPageDroppedProps lists, sorted, the keys on a tab page that the page
// vocabulary accepts and the tab page cannot store. Unrecognised keys are left
// to MDL-WIDGET07 and editability to MDL-WIDGET20, which already report them.
func tabPageDroppedProps(w *ast.WidgetV3) []string {
	if w == nil || !strings.EqualFold(w.Type, "tabpage") {
		return nil
	}
	var out []string
	for _, key := range sortedPropertyKeys(w) {
		l := strings.ToLower(key)
		if tabPageStoredProps[l] || l == "editable" || l == "editableif" || !isKnownStaticWidgetProp(key) {
			continue
		}
		out = append(out, key)
	}
	return out
}

func tabPageDroppedMessage(w *ast.WidgetV3, keys []string) string {
	return fmt.Sprintf("tabpage %s: %s cannot be stored on a tab page — Forms$TabPage has a plain-text "+
		"Caption (no parameters) and no Appearance; it stores only Caption and Visible",
		w.Name, "`"+strings.Join(keys, "`, `")+"`")
}

const tabPageDroppedSuggestion = "Put the parameterised text, class or style on a widget inside the tab page " +
	"(a `dynamictext` or a `container`), or remove the property"

// validateTabPageProps is the check-time half (MDL-V1-TABPAGEPROP, gated by
// GateWidgetViolations to MDL-WIDGET44 under mdl 1).
func validateTabPageProps(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	keys := tabPageDroppedProps(w)
	if len(keys) == 0 {
		return nil
	}
	return []linter.Violation{{
		RuleID:     tabPagePropDropped.Code,
		Severity:   linter.SeverityError,
		Message:    fmt.Sprintf("%s: %s", locationPrefix, tabPageDroppedMessage(w, keys)),
		Suggestion: tabPageDroppedSuggestion,
	}}
}

// refuseTabPageDroppedProps is the exec-time half, for `exec --no-check`: an
// error under mdl 1, a warning under mdl 0 (the properties are then dropped,
// as they always were).
func refuseTabPageDroppedProps(ctx *ExecContext, w *ast.WidgetV3) error {
	keys := tabPageDroppedProps(w)
	if len(keys) == 0 {
		return nil
	}
	version := langver.V0
	if ctx != nil {
		version = ctx.LanguageVersion
	}
	if tabPagePropDropped.Applies(version) {
		return mdlerrors.NewValidationf("%s, so nothing was written [MDL-WIDGET44]", tabPageDroppedMessage(w, keys))
	}
	if ctx != nil {
		fmt.Fprintf(ctx.progress(), "Warning [%s]: %s; not written. %s\n",
			tabPagePropDropped.Code, tabPageDroppedMessage(w, keys), tabPagePropDropped.Warning(version))
	}
	return nil
}
