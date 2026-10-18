// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// validateGalleryClickAmbiguity reports (MDL-WIDGET36 under mdl 1, the
// MDL-V1-GALLERYCLICK warning under mdl 0 — validate_widget_language.go) a Gallery whose row click
// cannot be told apart from selecting the row.
//
// The Gallery's own editor check (editorConfig.js, Gallery 3.4.0) makes it an
// error for a gallery to have a selection, an onClick and the single-click
// trigger at once — "The item click action is ambiguous. Change "On click
// trigger" to "Double click" or set "Selection" to "None"." — which `mx check`
// reports, so the build fails. mxcli's defaults are Selection Single and
// trigger single, so the plain `gallery g (…, onClick: …)` form hits it every
// time; before #842 it did not, only because the onClick was never written.
//
// The widget's condition is reproduced as it stands: selection not None, an
// onClick, trigger "single". Selection and trigger default to what mxcli writes
// when they are not given (gallery.def.json). Every gallery `check` passes this
// way fails `mx check`.
func validateGalleryClickAmbiguity(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if w == nil || !strings.EqualFold(w.Type, "gallery") || w.GetAction() == nil {
		return nil
	}
	selection := "Single"
	if v, ok := lookupProperty(w.Properties, "Selection"); ok {
		selection = stringifyAny(v)
	} else if v, ok := lookupProperty(w.Properties, "itemSelection"); ok {
		// The property's own key stores the same selection as the keyword; reading
		// only the keyword called a `itemSelection: None` gallery a selecting one.
		selection = stringifyAny(v)
	}
	trigger := "single"
	if v, ok := lookupProperty(w.Properties, "onClickTrigger"); ok {
		trigger = stringifyAny(v)
	}
	if strings.EqualFold(selection, "None") || !strings.EqualFold(trigger, "single") {
		return nil
	}
	return []linter.Violation{{
		RuleID:   galleryClickRefused.Code,
		Severity: linter.SeverityError,
		Message: fmt.Sprintf(
			"%s: %s has an onClick and a selection (%s) on the single-click trigger — the gallery "+
				"cannot tell a click from a selection, and the build fails with \"The item click action is ambiguous\"",
			locationPrefix, widgetLabel(w.Name, "gallery"), selection),
		Suggestion: "Add `Selection: None`, or `onClickTrigger: double` to run the action on a double click",
	}}
}
