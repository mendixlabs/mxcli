// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ReactUnsupportedWidgetRule reports the built-in widgets the React client does
// not support: the two legacy image widgets, the classic drop-down, the legacy
// data grid and the reference selector (ako/mxcli#564).
//
// # Why this is correctness, not style
//
// The React client arrived in Mendix 10.7, and mxbuild reports CE0582 on each
// of these widgets whenever it is enabled — measured on 11.12.1, where a page
// carrying one is a build ERROR:
//
//	[error] [CE0582] "Widget static image is not supported in React client.
//	                  Right-click this error to convert it to an alternative
//	                  widget type."   at Static image 'imgLogo'
//
// Studio Pro offers the conversion from that error's context menu. Which
// widgets are in the set is measured, not taken from the documentation (see
// reactUnsupportedWidgets).
//
// # Why mxcli writes them at all
//
// Because a project being converted UP already contains them, and mxcli's job
// there is to round-trip the model rather than quietly rewrite it. The image
// widgets and the drop-down are authorable and round-trip through DESCRIBE; the
// legacy data grid and the reference selector come from Studio Pro. What was
// missing was anyone saying so: `mxcli check` is silent on them, and the only
// signal was CE0582 at the far end of a build. The drop-down is the exception
// at author time — `check --references` refuses it in a script (MDL-WIDGET40)
// — but not for the drop-downs a project already holds, which only this rule
// reports.
//
// # Why the LINTER and not `mxcli check`
//
// `check` validates a script, and `describe page` -> `exec` of a legacy page is
// a legitimate, lossless operation that a warning would flag every time — a
// rule that fires on correct work is noise. `lint` audits the project, where
// "this page still holds a widget your client cannot render" is exactly the
// finding wanted, once.
//
// Native pages are out of scope too: the React client renders web pages only,
// and mxbuild reports no CE0582 on a page whose layout is native (measured,
// 11.13.0, ako/mxcli#953). Snippets are still reported — a snippet records no
// layout, and whether a native snippet builds a legacy image clean is unmeasured.
//
// Marketplace modules are already out of scope: ctx.Widgets() filters them
// (notPlatformModule excludes any module with a Source), which is what keeps the
// rule off the Studio Pro-authored static images a blank app inherits from
// FeedbackModule — content the reader cannot fix and an update would replace.
type ReactUnsupportedWidgetRule struct{}

// NewReactUnsupportedWidgetRule creates the MPR012 rule.
func NewReactUnsupportedWidgetRule() *ReactUnsupportedWidgetRule {
	return &ReactUnsupportedWidgetRule{}
}

func (r *ReactUnsupportedWidgetRule) ID() string                       { return "MPR012" }
func (r *ReactUnsupportedWidgetRule) Name() string                     { return "ReactUnsupportedWidget" }
func (r *ReactUnsupportedWidgetRule) Category() string                 { return "correctness" }
func (r *ReactUnsupportedWidgetRule) DefaultSeverity() linter.Severity { return linter.SeverityWarning }

// RequiredCatalogMode: ctx.Widgets() reads widgets_data, which only a full
// catalog build fills; under the default fast build the rule found nothing.
func (r *ReactUnsupportedWidgetRule) RequiredCatalogMode() linter.CatalogMode {
	return linter.CatalogFull
}

func (r *ReactUnsupportedWidgetRule) Description() string {
	return "Checks for built-in widgets the React client does not support — static/dynamic image, drop-down, legacy data grid, reference selector (CE0582)"
}

// ReactUnsupported describes one widget of the set, in the words the author
// will meet elsewhere: Mendix's own term (which CE0582 uses), the MDL keyword
// (empty for a widget MDL cannot write), and what to use instead.
type ReactUnsupported struct {
	MendixTerm  string
	MDLKeyword  string
	Replacement string
}

// reactUnsupportedWidgets is deliberately a deny-list of measured storage
// names. An allow-list would make every unrecognised widget a violation, and
// the widgets it must never fire on are the pluggable replacements.
//
// Measured with mxbuild on projects set to the React client:
//   - 11.12.1: static image; 11.14.0: drop-down (MDL-WIDGET40);
//   - 11.16.0 (FactoryManagement): data grid, reference selector, and the
//     reference selector with a select page — the same Forms$ReferenceSelector;
//     static and dynamic image again.
//
// And measured silent, so left out: navigation tree, menu bar, simple menu bar,
// file manager, scroll container (11.16.0); image uploader (11.13.0); text box,
// text area, check box, date picker, radio buttons, list view, group box,
// navigation list (11.15.0). The template grid and the reference set selectors
// were on no web page available to measure, so they are not claimed either way.
//
// DocumentTemplates$StaticImageViewer is a different type in a different
// document kind and is not covered: document templates are not pages, the React
// client does not render them, and CE0582 does not mention them.
var reactUnsupportedWidgets = map[string]ReactUnsupported{
	"Forms$StaticImageViewer": {"static image", "staticimage",
		"Replace `staticimage` with the pluggable `image` widget (same `Image:` reference)"},
	"Forms$ImageViewer": {"dynamic image", "dynamicimage",
		"Replace `dynamicimage` with the pluggable `image` widget (same `Image:` reference)"},
	"Forms$DropDown": {"drop-down", "dropdown",
		"Replace `dropdown` with `combobox` over the same attribute"},
	"Forms$DataGrid": {"data grid", "",
		"Replace the legacy data grid with Data grid 2 (`datagrid`)"},
	"Forms$ReferenceSelector": {"reference selector", "",
		"Replace the reference selector with `combobox` over the same association"},
}

// ReactUnsupportedWidget reports whether widgetType is one the React client
// does not support, and how to name and replace it.
func ReactUnsupportedWidget(widgetType string) (ReactUnsupported, bool) {
	got, ok := reactUnsupportedWidgets[widgetType]
	return got, ok
}

// Check reports one violation per React-unsupported widget found.
func (r *ReactUnsupportedWidgetRule) Check(ctx *linter.LintContext) []linter.Violation {
	var violations []linter.Violation
	native := ctx.NativePages()
	classic := runsClassicClient(ctx)

	for w := range ctx.Widgets() {
		if ctx.IsExcluded(w.ModuleName) {
			continue
		}
		legacy, ok := ReactUnsupportedWidget(w.WidgetType)
		if !ok {
			continue
		}
		// A native page is not rendered by the React client; mxbuild 11.13.0
		// builds a static image on a NativePhone_Default page clean (#953).
		if w.ContainerType != "SNIPPET" && native[w.ContainerQualifiedName] {
			continue
		}

		docType := "page"
		if w.ContainerType == "SNIPPET" {
			docType = "snippet"
		}

		severity := r.DefaultSeverity()
		message := fmt.Sprintf(
			"%s '%s' in %s is not supported by the React client, which this app uses — mxbuild reports CE0582",
			legacy.MendixTerm, w.Name, w.ContainerQualifiedName)
		if classic {
			severity = linter.SeverityInfo
			message = fmt.Sprintf(
				"%s '%s' in %s will need converting before the app switches to the React client (it builds on the client this app uses now)",
				legacy.MendixTerm, w.Name, w.ContainerQualifiedName)
		}
		violations = append(violations, linter.Violation{
			RuleID:   r.ID(),
			Severity: severity,
			Message:  message,
			Location: linter.Location{
				Module:       w.ModuleName,
				DocumentType: docType,
				DocumentName: docNameFromQualified(w.ContainerQualifiedName),
				DocumentID:   w.ContainerID,
			},
			Suggestion: legacy.Replacement + ", or convert it in Studio Pro from the CE0582 error's context menu",
		})
	}

	return violations
}

// runsClassicClient reports whether the project's Web UI setting is anything
// but the React client outright. A Mendix 11 app can still run the classic
// client, and there these widgets build clean: 57 warnings that each said
// "mxbuild reports CE0582" on an app at 0 errors (mendixlabs/mxcli#1226).
// Migration mode is not the React client either — measured on 11.14.0, the same
// CE0582 on a classic drop-down is an error under "Yes" and clean under "No"
// and "MigrationMode" (usesReactClient in mdl/executor).
//
// Settings that cannot be read keep the warning: lint always has them, so an
// unreadable value means something is wrong, not that the app is classic.
func runsClassicClient(ctx *linter.LintContext) bool {
	reader := ctx.Reader()
	if reader == nil {
		return false
	}
	ps, err := reader.GetProjectSettings()
	if err != nil || ps == nil || ps.WebUI == nil || ps.WebUI.UseOptimizedClient == "" {
		return false
	}
	return ps.WebUI.UseOptimizedClient != "Yes"
}
