// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// CE6206: "Attribute paths with multiple steps cannot be used on pages that are
// accessible through an offline-based navigation."
//
// An offline navigation profile restricts every page it can reach: an attribute
// may be bound across at most ONE association hop. Measured in Studio Pro 11.14
// on a page carrying both, where only the second is flagged:
//
//	MaintenanceRequest_Asset → AssetName               1 step   accepted
//	MaintenanceRequest_Asset → Asset_Site → SiteName   2 steps  CE6206
//
// So the rule is two-or-more hops, not "any indirect reference".
//
// This bites at a distance. The pages were valid; ADDING the offline profile is
// what invalidated them, and mxcli writes multi-step paths happily — verified:
// a datagrid column bound `Req_Asset/Asset_Site/SiteName` stores an
// IndirectEntityRef with two steps and `mx check` reports 0 errors, because that
// project had no offline profile.
//
// WHAT THIS RULE DOES NOT DO is decide whether a page is reachable from the
// offline profile. Studio Pro walks the whole page graph — home pages, menu
// items, then every page those pages open. mxcli would have to build that graph
// (the catalog has the edges, but `check` does not build the catalog), so the
// diagnostic is a WARNING that names the profile and states the condition,
// rather than an error asserting a reachability it has not established. Being
// wrong in the confident direction is how a checker teaches people to ignore it.

// offlineProfileNames returns the offline navigation profiles in the project.
// Empty when the project has none, which is the overwhelmingly common case and
// makes the whole rule inert for one navigation read.
func offlineProfileNames(nav *types.NavigationDocument) []string {
	if nav == nil {
		return nil
	}
	var names []string
	for _, p := range nav.Profiles {
		if p == nil {
			continue
		}
		// Every offline kind Mendix defines ends in "Offline":
		// ResponsiveOffline, TabletOffline, PhoneOffline, HybridOffline, …
		if strings.HasSuffix(p.Kind, "Offline") {
			names = append(names, p.Name)
		}
	}
	sort.Strings(names)
	return names
}

// attributePathSteps counts the association hops in an MDL attribute path.
// `Name` is 0, `Assoc/Name` is 1, `Assoc/Assoc2/Name` is 2 — the last segment is
// the attribute, every one before it is a hop.
func attributePathSteps(path string) int {
	p := strings.TrimSpace(path)
	if p == "" {
		return 0
	}
	return strings.Count(p, "/")
}

// multiStepBindingsInWidget collects the attribute paths a widget binds across
// two or more association hops.
//
// Only ATTRIBUTE bindings are considered. An XPath constraint also contains
// slashes (`[Mod.Assoc/Mod.Entity/Attr = 1]`) and is not an attribute path, so
// scanning every property that looks path-shaped would report constraints that
// offline navigation permits.
func multiStepBindingsInWidget(w *ast.WidgetV3) []string {
	if w == nil {
		return nil
	}
	var found []string
	consider := func(p string) {
		if attributePathSteps(p) >= 2 {
			found = append(found, p)
		}
	}
	consider(w.GetAttribute())
	consider(w.GetAttr())
	for _, a := range w.GetAttributes() {
		consider(a)
	}
	// A dynamic text binds its attributes through template parameters, which is
	// how the reference case failed: `Text 'txtAssetSite'` was a content
	// parameter, not an `Attribute:`.
	for _, ps := range [][]ast.ParamAssignmentV3{w.GetContentParams(), w.GetCaptionParams()} {
		for _, p := range ps {
			if s, ok := p.Value.(string); ok {
				consider(s)
			}
		}
	}
	return found
}

// ValidateOfflineAttributePaths reports multi-step attribute paths in the pages
// and snippets a script authors, when the project has an offline navigation
// profile (MDL-OFFLINE01). Needs --project: without one there is no way to know
// whether an offline profile exists, and the rule stays silent rather than
// warning every project in case.
func ValidateOfflineAttributePaths(prog *ast.Program, projectPath string) []linter.Violation {
	if prog == nil || projectPath == "" {
		return nil
	}
	profiles := offlineProfilesIn(projectPath)
	if len(profiles) == 0 {
		return nil
	}

	var out []linter.Violation
	for _, stmt := range prog.Statements {
		var label string
		var widgets []*ast.WidgetV3
		switch s := stmt.(type) {
		case *ast.CreatePageStmtV3:
			label, widgets = "page "+s.Name.String(), s.Widgets
		case *ast.CreateSnippetStmtV3:
			label, widgets = "snippet "+s.Name.String(), s.Widgets
		case *ast.AlterPageStmt:
			label = "alter " + s.PageName.String()
			for _, op := range s.Operations {
				switch o := op.(type) {
				case *ast.InsertWidgetOp:
					widgets = append(widgets, o.Widgets...)
				case *ast.ReplaceWidgetOp:
					widgets = append(widgets, o.NewWidgets...)
				}
			}
		default:
			continue
		}
		if v := offlinePathViolations(label, widgets, profiles); v != nil {
			out = append(out, v...)
		}
	}
	return out
}

// offlineProfilesIn reads the project's offline navigation profiles. A project
// that cannot be opened reports none, so an unreadable model silences the rule
// instead of failing the check on something it could not inspect.
func offlineProfilesIn(projectPath string) []string {
	reader := openProjectForValidation(projectPath)
	if reader == nil {
		return nil
	}
	defer func() { _ = reader.Disconnect() }()
	nav, err := reader.GetNavigation()
	if err != nil {
		return nil
	}
	return offlineProfileNames(nav)
}

// offlinePathViolations builds the diagnostic for one document.
func offlinePathViolations(docLabel string, widgets []*ast.WidgetV3, profiles []string) []linter.Violation {
	var paths []string
	var walk func(ws []*ast.WidgetV3)
	walk = func(ws []*ast.WidgetV3) {
		for _, w := range ws {
			if w == nil {
				continue
			}
			paths = append(paths, multiStepBindingsInWidget(w)...)
			walk(w.Children)
		}
	}
	walk(widgets)
	if len(paths) == 0 {
		return nil
	}
	sort.Strings(paths)
	paths = dedupeStrings(paths)

	return []linter.Violation{{
		RuleID:   "MDL-OFFLINE01",
		Severity: linter.SeverityWarning,
		Message: fmt.Sprintf(
			"%s: binds an attribute across more than one association (%s), and this project has "+
				"an offline navigation profile (%s). Mendix rejects a multi-step attribute path with "+
				"CE6206 on any page reachable from an offline profile — one hop is allowed, two are not.",
			docLabel, strings.Join(paths, ", "), strings.Join(profiles, ", ")),
		Suggestion: "If this page is reachable from that profile, bind one hop and carry the rest — " +
			"add an attribute to the intermediate entity and keep it in step, or keep the page off the " +
			"offline profile. mxcli does not compute reachability, so this is a warning: Studio Pro decides.",
	}}
}
