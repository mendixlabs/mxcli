// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// LoadThemeRegistryForProject loads the design-property registry for a project
// given the path to its .mpr file (the theme lives in <projectDir>/themesource).
// Returns nil when there is no usable metadata (no project, no themesource, or an
// empty registry) so callers can skip validation cleanly. The LSP loads this once
// per session to keep file I/O out of the per-keystroke path.
func LoadThemeRegistryForProject(mprPath string) *ThemeRegistry {
	if mprPath == "" {
		return nil
	}
	reg, err := loadThemeRegistry(filepath.Dir(mprPath))
	if err != nil || reg == nil || (len(reg.WidgetProperties) == 0 && len(reg.NativeWidgetProperties) == 0) {
		return nil
	}
	return reg
}

// ValidateDesignProperties validates the design properties authored on page /
// snippet / alter-page widgets against the project's theme registry
// (themesource/*/web/design-properties.json). It reports:
//
//   - MDL-WIDGET11: a design-property key that is not defined for the widget type.
//   - MDL-WIDGET12: an option/toggle-group value that is not one of the property's
//     allowed values — the message lists the allowed values (design-property
//     values are case-sensitive, so this catches the common casing typo).
//
// Both are errors (ako/mxcli#2236). They were warnings on the grounds that a
// newer theme may add keys the snapshot lacks, but the registry IS the
// project's own themesource — the theme mxbuild builds against — and mxbuild
// refuses both, measured on 11.12.6: an unknown key is CE6083 "Design property
// Hoover is not supported by your theme", an unknown option CE6085 "Unknown
// option Tiny for design property Shadow". Warned, exec wrote them anyway. An
// OLD name of a renamed property stays a warning: CE6087 does not apply to an
// excluded page.
//
// It only runs with --project AND a themesource that actually defines design
// properties; otherwise there is no metadata to validate against and it is a
// no-op. Compound (nested) entries are skipped — the registry does not model
// their sub-properties, so validating them would produce false positives.
//
// A page on a native layout is checked against the native definitions; checked
// against the web ones, Atlas's native `'Color': 'Brand primary'` and `'Size':
// 'Smallest'` were reported as not allowed (ako/mxcli#1679). project opens the
// project to resolve a layout the script does not create; it may be nil or
// return nil, and a page whose layout cannot be resolved is checked as web.
func ValidateDesignProperties(prog *ast.Program, projectPath string, project func() backend.FullBackend) []linter.Violation {
	reg := LoadThemeRegistryForProject(projectPath)
	if reg == nil {
		return nil
	}
	native := nativeLayoutResolver(prog, project)
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		r := reg
		if s, ok := stmt.(*ast.CreatePageStmtV3); ok && s.Layout != "" && native(s.Layout) {
			r = reg.ForPlatform(true)
		}
		out = append(out, ValidateDesignPropertiesForStatement(stmt, r)...)
	}
	return out
}

// nativeLayoutResolver answers whether a layout, by qualified name, is native:
// one the script creates with a native layout type, or one the project holds.
// The project is read once, on the first question.
func nativeLayoutResolver(prog *ast.Program, project func() backend.FullBackend) func(string) bool {
	native := map[string]bool{}
	for _, stmt := range prog.Statements {
		if s, ok := stmt.(*ast.CreateLayoutStmt); ok {
			if n, ok := platformForLayoutType(pages.LayoutType((&ast.WidgetV3{Properties: s.Properties}).GetStringProp("LayoutType"))); ok {
				native[s.Name.String()] = n
			}
		}
	}
	loaded := project == nil
	return func(qn string) bool {
		if n, ok := native[qn]; ok {
			return n
		}
		if loaded {
			return false
		}
		loaded = true
		b := project()
		if b == nil {
			return false
		}
		layouts, err := b.ListLayouts()
		if err != nil {
			return false
		}
		h, err := NewContainerHierarchyFromBackend(b)
		if err != nil {
			return false
		}
		for _, l := range layouts {
			name := h.GetModuleName(h.FindModuleID(l.ContainerID)) + "." + l.Name
			if _, scripted := native[name]; !scripted {
				native[name] = l.Native
			}
		}
		return native[qn]
	}
}

// ValidateDesignPropertiesForStatement validates one statement's widget design
// properties against a pre-loaded registry (LSP entry point).
func ValidateDesignPropertiesForStatement(stmt ast.Statement, reg *ThemeRegistry) []linter.Violation {
	if reg == nil {
		return nil
	}
	if label, widgets, ok := documentWidgets(stmt); ok {
		out := validateDesignPropsTree(widgets, reg, label)
		// The page's own, typed against the theme's "Page" group (ako/mxcli#1678).
		if s, isPage := stmt.(*ast.CreatePageStmtV3); isPage && len(s.DesignProperties) > 0 {
			page := &ast.WidgetV3{Type: "page", Name: s.Name.Name,
				Properties: map[string]any{"DesignProperties": s.DesignProperties}}
			out = append(out, validateDesignPropsAs(page, pageThemeKey, reg, label)...)
		}
		return out
	}
	switch s := stmt.(type) {
	case *ast.AlterPageStmt:
		var out []linter.Violation
		for _, op := range s.Operations {
			switch o := op.(type) {
			case *ast.InsertWidgetOp:
				out = append(out, validateDesignPropsTree(o.Widgets, reg, "alter "+s.PageName.String())...)
			case *ast.ReplaceWidgetOp:
				out = append(out, validateDesignPropsTree(o.NewWidgets, reg, "alter "+s.PageName.String())...)
			}
		}
		return out
	}
	return nil
}

func validateDesignPropsTree(widgets []*ast.WidgetV3, reg *ThemeRegistry, locationPrefix string) []linter.Violation {
	return validateDesignPropsSubtree(nil, widgets, reg, locationPrefix)
}

func validateDesignPropsSubtree(parent *ast.WidgetV3, widgets []*ast.WidgetV3, reg *ThemeRegistry, locationPrefix string) []linter.Violation {
	var out []linter.Violation
	for _, w := range widgets {
		if w == nil {
			continue
		}
		switch designPropsSlotOf(parent, w) {
		case slotDropped:
			out = append(out, droppedSlotDesignProps(parent, w, locationPrefix)...)
		case slotLayoutGridRow:
			out = append(out, validateDesignPropsAs(w, "LayoutGridRow", reg, locationPrefix)...)
		case slotLayoutGridColumn:
			out = append(out, validateDesignPropsAs(w, "LayoutGridColumn", reg, locationPrefix)...)
		case slotOfPluggable:
			// Built by the pluggable engine from the parent's object lists; the
			// keyword names no native widget here, so there is nothing to resolve.
		default:
			out = append(out, validateWidgetDesignProps(w, reg, locationPrefix)...)
		}
		if len(w.Children) > 0 {
			out = append(out, validateDesignPropsSubtree(w, w.Children, reg, locationPrefix)...)
		}
	}
	return out
}

type designPropsSlot int

const (
	notASlot designPropsSlot = iota
	// slotDropped: a structural part of its native parent that the builder
	// assembles itself, never through buildWidgetV3, so applyWidgetAppearance
	// never sees its design properties.
	slotDropped
	// slotLayoutGridRow / slotLayoutGridColumn: a layout grid's row and a row's
	// column. Not widgets either, but the builder writes their appearance, and
	// the theme defines their design properties under their own class names.
	slotLayoutGridRow
	slotLayoutGridColumn
	// slotOfPluggable: a keyword that is a native widget elsewhere, used as a
	// child of a pluggable widget — one of its object-list entries or slots.
	slotOfPluggable
)

// slotKeywords are the keywords that build a Forms$DivContainer on their own but
// name a part of their parent when nested in one: a layout grid's row, a row's
// column, a dataview's footer, a data grid's column or a gallery's template.
var slotKeywords = map[string]bool{
	"row": true, "column": true, "header": true, "footer": true,
	"controlbar": true, "template": true, "filter": true,
}

// designPropsSlotOf reports whether child is a slot of parent rather than a
// widget. mdlKeywordStorageType maps `row` / `column` / `footer` to the
// Forms$DivContainer they build at the top level; that is only true where
// buildWidgetV3 builds them.
func designPropsSlotOf(parent, child *ast.WidgetV3) designPropsSlot {
	if parent == nil || child == nil {
		return notASlot
	}
	p, c := strings.ToLower(parent.Type), strings.ToLower(child.Type)
	switch {
	case p == "layoutgrid" && c == "row": // buildLayoutGridRowV3
		return slotLayoutGridRow
	case p == "row" && c == "column": // buildLayoutGridColumnV3
		return slotLayoutGridColumn
	case p == "dataview" && c == "footer": // children moved into FooterWidgets
		return slotDropped
	}
	if !slotKeywords[c] {
		return notASlot
	}
	if _, native := mdlKeywordStorageType[p]; !native {
		return slotOfPluggable
	}
	return notASlot
}

// droppedSlotDesignProps reports design properties written on a slot the builder
// drops them from. Silence here read as approval of a value that never reaches
// the model — the "check passes, exec succeeds, the value is gone" shape.
func droppedSlotDesignProps(parent, w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if len(w.GetDesignProperties()) == 0 {
		return nil
	}
	return []linter.Violation{{
		RuleID:   "MDL-WIDGET07",
		Severity: linter.SeverityWarning,
		Message: fmt.Sprintf("%s: %s inside %s sets DesignProperties, but it is part of its parent, "+
			"not a widget — mxcli does not write design properties there, so they are silently dropped",
			locationPrefix, elementLabel(w), elementLabel(parent)),
		Location:   linter.Location{DocumentType: "page", DocumentName: locationPrefix},
		Suggestion: fmt.Sprintf("Put them on a container inside the %s instead.", strings.ToLower(w.Type)),
	}}
}

// elementLabel names a page element in a message: its kind, and its name when
// it has one — a layout grid's row or column has none (#749).
func elementLabel(w *ast.WidgetV3) string {
	if w.Name == "" {
		return strings.ToLower(w.Type)
	}
	return fmt.Sprintf("%s %q", strings.ToLower(w.Type), w.Name)
}

func validateWidgetDesignProps(w *ast.WidgetV3, reg *ThemeRegistry, locationPrefix string) []linter.Violation {
	return validateDesignPropsAs(w, resolveDesignPropsKey(w.Type), reg, locationPrefix)
}

// validateDesignPropsAs validates w's design properties against the theme's
// definitions for key (a design-properties.json group).
func validateDesignPropsAs(w *ast.WidgetV3, key string, reg *ThemeRegistry, locationPrefix string) []linter.Violation {
	astProps := w.GetDesignProperties()
	if len(astProps) == 0 {
		return nil
	}
	// Only validate widgets we have type-specific metadata for. For an unknown
	// widget type (e.g. a pluggable widget not in the theme registry) we don't
	// know the full property set, so we must not flag its keys as unknown.
	if !reg.declaresGroup(key) {
		return nil
	}
	props := reg.GetPropertiesForWidget(key)

	var out []linter.Violation
	for _, p := range astProps {
		if len(p.Nested) > 0 {
			continue // compound — registry doesn't model sub-properties
		}
		tp := findThemeProp(props, p.Key)
		if tp != nil && tp.MultiSelect && len(p.Nested) == 0 && p.Value != "" {
			// A multi-select property is stored as a compound, one entry per
			// selected option. A flat value names a declared option, so nothing
			// else here objects — and mxbuild then refuses the document with
			// CE6084 (ako/mxcli#511). exec refuses it too; saying so at check
			// time is what keeps the two in step.
			out = append(out, linter.Violation{
				RuleID:   "MDL-WIDGET12",
				Severity: linter.SeverityWarning,
				Message: fmt.Sprintf("%s: widget %q (%s) sets design property %q to a single value, "+
					"but it takes a SET of options — mxbuild refuses that with CE6084",
					locationPrefix, w.Name, w.Type, p.Key),
				Location: linter.Location{DocumentType: "page", DocumentName: locationPrefix},
				Suggestion: fmt.Sprintf("Write it as `'%s': ['%s': on]`, with one `'<option>': on` "+
					"per selection.", p.Key, firstOptionName(tp, p.Value)),
			})
			continue
		}
		if tp == nil {
			// An OLD name of a current property is not a typo: it is what a page
			// authored against an earlier theme version stores. It is still
			// flagged — mxbuild refuses it on a live page with CE6087 — but as a
			// rename with its current spelling, which "not defined" plus a list
			// of every key could not give.
			if r := findRenamedThemeProp(props, p.Key, p.Value); r != nil {
				out = append(out, linter.Violation{
					RuleID:   "MDL-WIDGET11",
					Severity: linter.SeverityWarning,
					Message: fmt.Sprintf("%s: widget %q (%s) sets design property %q, which the theme has renamed to %q"+
						" — mxbuild reports CE6087 \"Design properties have been renamed in your theme\" unless the page is excluded",
						locationPrefix, w.Name, w.Type, p.Key, r.NewKey),
					Location:   linter.Location{DocumentType: "page", DocumentName: locationPrefix},
					Suggestion: renamedDesignPropSuggestion(r, p.Value),
				})
				continue
			}
			out = append(out, linter.Violation{
				RuleID:   "MDL-WIDGET11",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf("%s: widget %q (%s) sets design property %q, which is not defined for this widget type"+
					" — mxbuild reports CE6083 \"Design property %s is not supported by your theme\"",
					locationPrefix, w.Name, w.Type, p.Key, p.Key),
				Location:   linter.Location{DocumentType: "page", DocumentName: locationPrefix},
				Suggestion: designPropKeySuggestion(props, p.Key),
			})
			continue
		}
		// Value validation for enumerated types. Toggle values are on/off (handled
		// by the writer); options/pickers must be one of the declared values.
		if len(tp.Options) > 0 && !strings.EqualFold(p.Value, "on") && !strings.EqualFold(p.Value, "off") {
			if !themeOptionAllowed(tp.Options, p.Value) {
				out = append(out, linter.Violation{
					RuleID:   "MDL-WIDGET12",
					Severity: linter.SeverityError,
					Message: fmt.Sprintf("%s: widget %q (%s) design property %q has value %q, which is not an allowed value"+
						" — mxbuild reports CE6085 \"Unknown option %s for design property %s\"",
						locationPrefix, w.Name, w.Type, p.Key, p.Value, p.Value, p.Key),
					Location:   linter.Location{DocumentType: "page", DocumentName: locationPrefix},
					Suggestion: fmt.Sprintf("Allowed values (case-sensitive): %s", themeOptionNames(tp.Options)),
				})
			}
		}
	}
	return out
}

// ThemeDeclaresDesignProperty reports whether reg declares design property key
// for the design-properties.json group widgetKey (e.g. "DivContainer", with
// the inherited "Widget" group) and accepts value for it: "on" for a Toggle,
// one of the declared option names otherwise. A multi-select property is never
// accepted, since a single value is the wrong stored shape for it (CE6084).
//
// For generated MDL that must not name a property the project's theme lacks:
// mxbuild refuses every such one with CE6083, and the theme is versioned with
// Atlas, not with mxcli (mendixlabs/mxcli#1433).
func ThemeDeclaresDesignProperty(reg *ThemeRegistry, widgetKey, key, value string) bool {
	if reg == nil {
		return false
	}
	tp := findThemeProp(reg.GetPropertiesForWidget(widgetKey), key)
	if tp == nil || tp.MultiSelect {
		return false
	}
	if tp.Type == "Toggle" {
		return strings.EqualFold(value, "on")
	}
	return themeOptionAllowed(tp.Options, value)
}

// findThemeProp returns the property whose Name matches key exactly.
func findThemeProp(props []ThemeProperty, key string) *ThemeProperty {
	for i := range props {
		if props[i].Name == key {
			return &props[i]
		}
	}
	return nil
}

// designPropKeySuggestion offers a case-insensitive near match (design-property
// keys are case-sensitive, so a casing typo is the most common cause), otherwise
// lists the defined keys for the widget.
func designPropKeySuggestion(props []ThemeProperty, key string) string {
	for i := range props {
		if strings.EqualFold(props[i].Name, key) {
			return fmt.Sprintf("Design-property keys are case-sensitive — did you mean %q?", props[i].Name)
		}
	}
	var names []string
	for i := range props {
		names = append(names, props[i].Name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "Run 'mxcli show design properties' to list the valid design properties for this widget."
	}
	return "Valid design properties for this widget: " + strings.Join(names, ", ")
}

func themeOptionAllowed(options []ThemeOption, value string) bool {
	for _, o := range options {
		if o.Name == value {
			return true
		}
	}
	return false
}

func themeOptionNames(options []ThemeOption) string {
	names := make([]string, 0, len(options))
	for _, o := range options {
		names = append(names, o.Name)
	}
	return strings.Join(names, ", ")
}
