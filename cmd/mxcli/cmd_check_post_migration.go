// SPDX-License-Identifier: Apache-2.0

// Post-migration scan for legacy native widgets in a Mendix project.
//
// When a project is upgraded from Mendix 10.x to 11.x, Studio Pro does NOT
// auto-rewrite native-stack widgets (e.g. Forms$DataGrid) to their pluggable
// replacements. The author has to migrate them by hand. This scanner walks
// every page and snippet, looks for legacy widget types listed in
// executor.LegacyWidgets, and reports each occurrence with a hint to the
// recommended pluggable equivalent.

package main

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/JordtenBulte-OLC/mxcli/mdl/executor"
	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr"
)

// legacyHit records one legacy widget occurrence with enough context to
// produce a useful diagnostic.
type legacyHit struct {
	Module     string // qualified module name (may be empty if hierarchy resolution failed)
	Document   string // page or snippet name
	DocKind    string // "page" or "snippet"
	WidgetName string // widget instance name as authored in Studio Pro
	Entry      *executor.LegacyWidget
}

// scanLegacyWidgets opens the given .mpr, walks all pages and snippets, and
// returns a violation for every legacy native widget found that's deprecated
// on the project's Mendix version.
func scanLegacyWidgets(projectPath string) ([]linter.Violation, error) {
	reader, err := mpr.Open(projectPath)
	if err != nil {
		return nil, fmt.Errorf("opening project: %w", err)
	}
	defer reader.Close()

	mendixVersion, _ := reader.GetMendixVersion()

	hierarchy, err := executor.NewContainerHierarchy(reader)
	if err != nil {
		return nil, fmt.Errorf("building project hierarchy: %w", err)
	}

	var hits []legacyHit

	pgs, err := reader.ListPages()
	if err != nil {
		return nil, fmt.Errorf("listing pages: %w", err)
	}
	for _, pg := range pgs {
		module := hierarchy.GetModuleName(hierarchy.FindModuleID(pg.ContainerID))
		walkForLegacyWidgets(reflect.ValueOf(pg), mendixVersion, func(entry *executor.LegacyWidget, name string) {
			hits = append(hits, legacyHit{
				Module: module, Document: pg.Name, DocKind: "page",
				WidgetName: name, Entry: entry,
			})
		})
	}

	sns, err := reader.ListSnippets()
	if err != nil {
		return nil, fmt.Errorf("listing snippets: %w", err)
	}
	for _, sn := range sns {
		module := hierarchy.GetModuleName(hierarchy.FindModuleID(sn.ContainerID))
		walkForLegacyWidgets(reflect.ValueOf(sn), mendixVersion, func(entry *executor.LegacyWidget, name string) {
			hits = append(hits, legacyHit{
				Module: module, Document: sn.Name, DocKind: "snippet",
				WidgetName: name, Entry: entry,
			})
		})
	}

	sortHits(hits)
	return hitsToViolations(hits, mendixVersion), nil
}

// walkForLegacyWidgets recursively walks the reflect.Value of a parsed page
// or snippet. Any struct whose Go type name matches a known legacy widget
// triggers the callback.
//
// We match by `reflect.Type.Name()` (e.g. "DataGrid") rather than by type
// assertion against an interface, because the parsed page widgets in
// sdk/pages don't all implement a common Widget interface uniformly. Type
// names are stable enough — the catalog (executor.LegacyWidgets) is small
// and hand-maintained.
func walkForLegacyWidgets(v reflect.Value, version string, visit func(*executor.LegacyWidget, string)) {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Struct:
		if entry := executor.FindLegacyWidget(v.Type().Name()); entry != nil && entry.IsDeprecatedOnVersion(version) {
			visit(entry, widgetNameFrom(v))
		}
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			if !f.CanInterface() {
				continue
			}
			walkForLegacyWidgets(f, version, visit)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			walkForLegacyWidgets(v.Index(i), version, visit)
		}
	case reflect.Interface:
		if !v.IsNil() {
			walkForLegacyWidgets(v.Elem(), version, visit)
		}
	}
}

// widgetNameFrom reads the Name field from an embedded BaseWidget if present.
// Returns "" when no name is available.
func widgetNameFrom(v reflect.Value) string {
	// BaseWidget.Name is reachable via field path on every widget struct.
	if f := v.FieldByName("BaseWidget"); f.IsValid() && f.Kind() == reflect.Struct {
		if n := f.FieldByName("Name"); n.IsValid() && n.Kind() == reflect.String {
			return n.String()
		}
	}
	// Fallback: direct Name field (some non-widget types).
	if n := v.FieldByName("Name"); n.IsValid() && n.Kind() == reflect.String {
		return n.String()
	}
	return ""
}

// sortHits orders hits by module, document, widget for stable output.
func sortHits(hits []legacyHit) {
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Module != hits[j].Module {
			return hits[i].Module < hits[j].Module
		}
		if hits[i].Document != hits[j].Document {
			return hits[i].Document < hits[j].Document
		}
		return hits[i].WidgetName < hits[j].WidgetName
	})
}

// hitsToViolations converts legacy hits into linter violations with the
// MDL-WIDGET02 rule code.
func hitsToViolations(hits []legacyHit, version string) []linter.Violation {
	if len(hits) == 0 {
		return nil
	}
	out := make([]linter.Violation, 0, len(hits))
	for _, h := range hits {
		qualified := h.Document
		if h.Module != "" {
			qualified = h.Module + "." + h.Document
		}
		name := h.WidgetName
		if name == "" {
			name = "(unnamed)"
		}
		msg := fmt.Sprintf(
			"%s %s: widget `%s` uses deprecated native `%s` (deprecated from Mendix %s) — %s",
			h.DocKind, qualified, name, h.Entry.BSONType, h.Entry.DeprecatedFrom, h.Entry.Hint,
		)
		if version != "" {
			msg += fmt.Sprintf(" (project is on %s)", version)
		}
		out = append(out, linter.Violation{
			RuleID:   "MDL-WIDGET02",
			Severity: linter.SeverityWarning,
			Message:  msg,
		})
	}
	return out
}
