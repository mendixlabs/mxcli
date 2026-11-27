// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
	mwidgets "github.com/mendixlabs/mxcli/modelsdk/widgets"
	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

// widgetPlatformRule warns about a pluggable widget on a page of the other
// platform.
const widgetPlatformRule = "MDL-WIDGET55"

// ValidateWidgetPlatforms warns (MDL-WIDGET55) about a pluggable widget whose
// package declares a platform other than its page's: a web widget
// (`combobox`, `datagrid`, `image`, …) on a native page, or a native one
// (`nativeslider`, `nativegallery`, …) on a web page. The client of the other
// platform does not render it.
//
// Measured on PedApp (11.13.0) with Native Mobile Resources 12.6.0: both
// directions passed check and exec. A web widget on a native page is an error:
// mx check 11.12.6 fails `image`, `accordion` and `gallery` there with CE6089
// "Widget 'X' is only supported on web documents." (ako/mxcli#2619). A native
// widget on a web page stays a warning: what mxbuild reports for it is not
// measured.
//
// A page is judged only when its layout's platform is known — created by the
// script, or held by the project — and a widget only when its platform is:
// from its embedded template, or the project's installed package.
func ValidateWidgetPlatforms(prog *ast.Program, projectPath string, project func() backend.FullBackend) []linter.Violation {
	return validateWidgetPlatforms(prog, LoadWidgetRegistry(projectPath), projectPath, project)
}

func validateWidgetPlatforms(prog *ast.Program, registry *WidgetRegistry, projectPath string, project func() backend.FullBackend) []linter.Violation {
	if registry == nil {
		return nil
	}
	layoutPlatform := layoutPlatformResolver(prog, project)
	platforms := map[string]string{}
	platformOf := func(widgetID string) string {
		p, ok := platforms[widgetID]
		if !ok {
			p = pluggableWidgetPlatform(widgetID, projectPath)
			platforms[widgetID] = p
		}
		return p
	}

	var out []linter.Violation
	for _, stmt := range prog.Statements {
		page, ok := stmt.(*ast.CreatePageStmtV3)
		if !ok || page.Layout == "" {
			continue
		}
		native, known := layoutPlatform(page.Layout)
		if !known {
			continue
		}
		pagePlatform, otherPlatform := "Web", "Native"
		if native {
			pagePlatform, otherPlatform = "Native", "Web"
		}
		forEachWidget(page, func(w *ast.WidgetV3, where string) {
			def := lookupWidgetDef(w, registry)
			if def == nil {
				return
			}
			if !strings.EqualFold(platformOf(def.WidgetID), otherPlatform) {
				return
			}
			v := linter.Violation{
				RuleID:   widgetPlatformRule,
				Severity: linter.SeverityWarning,
				Message: fmt.Sprintf("page %s: `%s %s` is a %s widget (%s) on a %s page, which the %s client does not render",
					page.Name.String(), strings.ToLower(w.Type), w.Name, strings.ToLower(otherPlatform), def.WidgetID,
					strings.ToLower(pagePlatform), strings.ToLower(pagePlatform)),
				Suggestion: platformSuggestion(native),
			}
			if native {
				v.Severity = linter.SeverityError
				v.Message += ", so mxbuild fails it with CE6089"
			}
			out = append(out, v)
		})
	}
	return out
}

func platformSuggestion(nativePage bool) string {
	if nativePage {
		return "use the native widget instead (`widget init -p app.mpr` lists the installed `native…` keywords; " +
			"`staticimage` / `dynamicimage` for an image), or put the widget on a web page"
	}
	return "use the web widget instead, or put the widget on a native page"
}

// pluggableWidgetPlatform is the platform a pluggable widget's package
// declares — Web, Native or All — read from its embedded template, else the
// project's installed .mpk; "" when neither knows the widget.
func pluggableWidgetPlatform(widgetID, projectPath string) string {
	if tmpl, err := mwidgets.GetTemplate(widgetID); err == nil && tmpl != nil {
		if p, ok := tmpl.Type["SupportedPlatform"].(string); ok && p != "" {
			return p
		}
		return "Web" // an absent supportedPlatform is Web
	}
	if projectPath == "" {
		return ""
	}
	mpkPath, err := mpk.FindMPK(filepath.Dir(projectPath), widgetID)
	if err != nil || mpkPath == "" {
		return ""
	}
	def, err := mpk.ParseMPKForWidget(mpkPath, widgetID)
	if err != nil || def == nil {
		return ""
	}
	return def.SupportedPlatform
}

// refuseWebWidgetsOnNativeDocument refuses the web pluggable widgets an ALTER
// would insert into a native page, snippet or layout: mxbuild fails each with
// CE6089 (ako/mxcli#2619). Check cannot see the stored document's platform, so
// exec asks the mutator, as the theme routing does.
func refuseWebWidgetsOnNativeDocument(ctx *ExecContext, mutator backend.PageMutator, widgets []*ast.WidgetV3) error {
	r, ok := mutator.(documentPlatformReporter)
	if !ok {
		return nil
	}
	native, layout := r.DocumentPlatform()
	if !native && layout != "" {
		native = layoutNamedIsNative(ctx, layout)
	}
	if !native {
		return nil
	}
	registry := ctx.GetWidgetRegistry()
	if registry == nil {
		return nil
	}
	var refused error
	var walk func([]*ast.WidgetV3)
	walk = func(ws []*ast.WidgetV3) {
		for _, w := range ws {
			if w == nil || refused != nil {
				continue
			}
			if def := lookupWidgetDef(w, registry); def != nil &&
				strings.EqualFold(pluggableWidgetPlatform(def.WidgetID, ctx.MprPath), "Web") {
				refused = mdlerrors.NewValidation(fmt.Sprintf(
					"`%s %s` is a web widget (%s) and this document is native, so mxbuild fails it with CE6089 [%s]; %s",
					strings.ToLower(w.Type), w.Name, def.WidgetID, widgetPlatformRule, platformSuggestion(true)))
				return
			}
			walk(w.Children)
		}
	}
	walk(widgets)
	return refused
}
