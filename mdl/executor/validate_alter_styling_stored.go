// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// The second half of ALTER STYLING's design-property check (ako/mxcli#613): a
// key the theme declares for SOME widget type, judged against the type of the
// widget the statement actually names. The file comment of
// validate_alter_styling.go has the reasoning; this is the reference pass's
// half, because only that pass holds a connection that can open the document.
//
// Measured on blank 11.12.2 and 11.14.0 projects (Forms$DynamicText dt and
// Forms$ListView lvH on one page):
//
//	alter styling on page MyFirstModule.HideOn widget dt set 'Row size' = 'Small';
//	  check -p --references -> Check passed!         (before this pass)
//	  docker check          -> [CE6083] "Design property Row size is not
//	                           supported by your theme." at Text 'dt'
//
// and the same key on lvH, the widget whose type declares it, is 0 errors.

func alterStylingLabel(s *ast.AlterStylingStmt) string {
	return fmt.Sprintf("alter styling on %s %s", strings.ToLower(s.ContainerType), s.ContainerName.String())
}

// validateAlterStylingStoredTypes adds a warning (MDL-WIDGET11) for each ALTER
// STYLING design-property key that some widget type in the theme declares but
// the stored widget's own type does not. Warns, never blocks: a newer theme may
// add keys the snapshot lacks (#509's scope note).
//
// Only keys the any-type pass accepted are judged here — a key no type declares
// is that pass's finding, and reporting it twice reads as two problems. Silent
// with no connection, with no theme metadata, and wherever the stored type
// cannot be established.
func validateAlterStylingStoredTypes(ctx *ExecContext, prog *ast.Program, sc *scriptContext) {
	if prog == nil || sc == nil || !ctx.Connected() {
		return
	}
	reg := ctx.GetThemeRegistry()
	if reg == nil || len(reg.WidgetProperties) == 0 {
		return
	}
	declared := allDeclaredDesignPropertyNames(reg)
	stored := newStoredStylingTypes(ctx, reg, prog, sc)
	for _, stmt := range prog.Statements {
		s, ok := stmt.(*ast.AlterStylingStmt)
		if !ok || !hasDesignPropertyAssignment(s) {
			continue
		}
		own := stored.resolve(s)
		if own == nil {
			continue
		}
		for _, a := range s.Assignments {
			if a.IsCSS || a.Property == "" || !declaredHas(declared, a.Property) {
				continue
			}
			if msg := own.judge(alterStylingLabel(s), s.WidgetName, a); msg != "" {
				sc.warnings = append(sc.warnings, msg)
			}
		}
	}
}

func hasDesignPropertyAssignment(s *ast.AlterStylingStmt) bool {
	for _, a := range s.Assignments {
		if !a.IsCSS && a.Property != "" {
			return true
		}
	}
	return false
}

// storedStylingType is what an ALTER STYLING target IS in the stored document:
// its theme group, and the keys that group declares with the `Widget` group's
// inherited ones (GetPropertiesForWidget).
type storedStylingType struct {
	group string
	props []ThemeProperty
}

// judge applies the authoring paths' per-key condition (validateDesignPropsAs:
// an exact match, else a rename, else undeclared) to the stored widget's own
// type, so ALTER STYLING and CREATE PAGE cannot disagree about one key on one
// widget type. "" when the key is the type's own.
//
// A multi-select key the type declares (`Hide on`) is known, not unknown: the
// any-type pass already reports it as MDL-WIDGET12, the writer limit of #511.
func (t *storedStylingType) judge(label, widget string, a ast.StylingAssignment) string {
	if findThemeProp(t.props, a.Property) != nil {
		return ""
	}
	if r := findRenamedThemeProp(t.props, a.Property, a.Value); r != nil {
		return fmt.Sprintf("%s: sets design property %q on %q (a %s), which the theme renamed to %q "+
			"— mxbuild reports the old name as CE6087 [MDL-WIDGET11] — %s",
			label, a.Property, widget, t.group, r.NewKey, renamedStylingSuggestion(r, a.Value))
	}
	return fmt.Sprintf("%s: sets design property %q on %q, a %s, which the theme declares only for "+
		"other widget types — mxbuild reports this as CE6083 [MDL-WIDGET11] — %s",
		label, a.Property, widget, t.group,
		strings.Replace(designPropKeySuggestion(t.props, a.Property), "this widget", "a "+t.group, 1))
}

// storedStylingTypes resolves ALTER STYLING targets against the stored
// documents, opening each at most once.
type storedStylingTypes struct {
	ctx    *ExecContext
	reg    *ThemeRegistry
	h      *ContainerHierarchy
	sc     *scriptContext
	grows  map[string]bool
	opened map[string]pageProbe
}

func newStoredStylingTypes(ctx *ExecContext, reg *ThemeRegistry, prog *ast.Program, sc *scriptContext) *storedStylingTypes {
	h, _ := getHierarchy(ctx)
	return &storedStylingTypes{
		ctx:    ctx,
		reg:    reg,
		h:      h,
		sc:     sc,
		grows:  documentsTheScriptAddsWidgetsTo(prog),
		opened: map[string]pageProbe{},
	}
}

// resolve returns the stored widget's own type, or nil when it cannot be
// established: a document the script creates or grows (its stored tree is not
// the one exec will style), a widget that is not stored (exec reports that), a
// type with no theme group (its full key set is unknown — the same guard
// validateDesignPropsAs applies). nil is silence, never a finding.
func (r *storedStylingTypes) resolve(s *ast.AlterStylingStmt) *storedStylingType {
	if r.h == nil || s.WidgetName == "" {
		return nil
	}
	qn := s.ContainerName.String()
	kind := strings.ToLower(s.ContainerType)
	if r.grows[qn] || r.sc.modules[s.ContainerName.Module] ||
		(kind == "page" && r.sc.pages[qn]) || (kind == "snippet" && r.sc.snippets[qn]) {
		return nil
	}
	probe, seen := r.opened[kind+" "+qn]
	if !seen {
		probe = r.open(kind, s.ContainerName)
		r.opened[kind+" "+qn] = probe
	}
	if probe == nil || !probe.ResolvesTarget(s.WidgetName, "") {
		return nil
	}
	group := designPropsKeyOfStored(probe, s.WidgetName)
	theme := documentThemeOf(r.ctx, r.reg, probe)
	if group == "" || theme == nil || !theme.declaresGroup(group) {
		return nil
	}
	return &storedStylingType{group: group, props: theme.GetPropertiesForWidget(group)}
}

// open finds the page or snippet as execAlterStyling does and opens it as
// validateAlterSetProperties does, or returns nil.
func (r *storedStylingTypes) open(kind string, name ast.QualifiedName) pageProbe {
	switch kind {
	case "page":
		if pg, err := findPageByName(r.ctx, name, r.h); err == nil {
			return openPageProbe(r.ctx, pg.ID)
		}
	case "snippet":
		if snp, _, err := findSnippetByName(r.ctx, name, r.h); err == nil {
			return openPageProbe(r.ctx, snp.ID)
		}
	}
	return nil
}
