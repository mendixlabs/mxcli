// SPDX-License-Identifier: Apache-2.0

// Reference validation for icon-collection references.
//
// `icon: 'Atlas_Core.Atlas_Filled.pencil'` names an icon inside an icon
// collection document. Nothing resolved it: the name was written through to
// BSON verbatim, `mxcli check` passed, and the first sign of a typo was MxBuild:
//
//	[error] [CE1613] "The selected custom icon
//	'Atlas_Core.Atlas_Filled.no-such-icon' no longer exists." at Action button 'btnBad'
//
// The collections live in the project (Atlas_Core ships three, ~770 icons), so
// this needs -p — it runs in the --references pass alongside the other
// project-resolved references.
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// iconIndex holds the project's icon collections AND its image collections,
// each keyed by qualified collection name (Module.Collection) → set of member
// names.
//
// Both are needed because a widget icon can point into either, and they are
// DIFFERENT documents: `Icon: 'Mod.Coll.name'` is a CustomIcons$CustomIcon and
// `Icon: image Mod.Coll.name` is an Images$Image. Resolving one against the
// other's listing reports a correct reference as a typo — which is the mirror
// image of the defect that made these kinds distinguishable in the first place
// (mendixlabs/mxcli#1059).
type iconIndex struct {
	collections map[string]map[string]bool
	images      map[string]map[string]bool
	// order and imageOrder preserve a stable listing for error messages.
	order      []string
	imageOrder []string
}

// buildIconIndex reads the project's icon and image collections once per
// validation run. Returns nil when the project exposes neither, which disables
// the check rather than reporting every icon as unknown.
//
// An EMPTY listing means the backend could not answer, not that the project has
// none — reporting every reference as unknown on that basis is the false error
// the third state exists to prevent. Each half is therefore independent: a
// project that yields icon collections but no image collections still has its
// collection references resolved, and its image references left alone.
func buildIconIndex(ctx *ExecContext) *iconIndex {
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil
	}
	idx := &iconIndex{
		collections: map[string]map[string]bool{},
		images:      map[string]map[string]bool{},
	}

	if cols, err := ctx.Backend.ListIconCollections(); err == nil {
		for _, c := range cols {
			qn := iconCollectionQualifiedName(h, c.ContainerID, c.Name)
			if qn == "" {
				continue
			}
			names := make(map[string]bool, len(c.Icons))
			for _, ic := range c.Icons {
				names[ic.Name] = true
			}
			idx.collections[qn] = names
			idx.order = append(idx.order, qn)
		}
	}

	if cols, err := ctx.Backend.ListImageCollections(); err == nil {
		for _, c := range cols {
			qn := iconCollectionQualifiedName(h, c.ContainerID, c.Name)
			if qn == "" {
				continue
			}
			names := make(map[string]bool, len(c.Images))
			for _, img := range c.Images {
				names[img.Name] = true
			}
			idx.images[qn] = names
			idx.imageOrder = append(idx.imageOrder, qn)
		}
	}

	sort.Strings(idx.order)
	sort.Strings(idx.imageOrder)
	if len(idx.collections) == 0 && len(idx.images) == 0 {
		return nil
	}
	return idx
}

func iconCollectionQualifiedName(h *ContainerHierarchy, container model.ID, name string) string {
	mod := h.GetModuleName(h.FindModuleID(container))
	if mod == "" || name == "" {
		return ""
	}
	return mod + "." + name
}

// validateIconRefs resolves every icon reference in the program against the
// project's icon collections.
func validateIconRefs(ctx *ExecContext, prog *ast.Program) []error {
	if !ctx.Connected() {
		return nil
	}
	idx := buildIconIndex(ctx)
	if idx == nil {
		return nil
	}

	registry := ctx.GetWidgetRegistry()
	var errs []error
	for i, stmt := range prog.Statements {
		for _, ref := range iconRefsToResolve(stmt, registry) {
			// A glyph names no document. MDL078 checks its code against the
			// font's own table; there is nothing for this pass to resolve.
			if ref.kind == types.MenuIconGlyph {
				continue
			}
			if ref.setOn != nil && storedWidgetOwnsIcon(ctx, ref.setOn, ref.setTarget) {
				continue
			}
			if err := idx.check(ref); err != nil {
				errs = append(errs, fmt.Errorf("statement %d: %w", i+1, err))
			}
		}
	}
	return errs
}

// iconRef is one icon reference and where it was written, for the message.
type iconRef struct {
	value string             // as authored, e.g. Atlas_Core.Atlas_Filled.pencil
	kind  types.MenuIconKind // which document the name lives in
	code  int                // the character code, for MenuIconGlyph (which has no name)
	where string             // e.g. `button "btnSave"` or `menu item 'Home'`

	// widget is the widget the icon was written on, for a widget tree the
	// statement carries; setOn and setTarget name the stored widget an ALTER
	// PAGE `SET` writes it to. Either lets the check stand down where `icon`
	// is a pluggable widget's own key rather than an icon reference
	// (ako/mxcli#2180).
	widget    *ast.WidgetV3
	setOn     *ast.AlterPageStmt
	setTarget string
}

// iconRefsToResolve is iconRefsInStatement less the `icon:` values that are a
// pluggable widget's own property — a PDS Button's `icon: 'bookmark'` is the
// name of one of the design system's icons, a plain string, and refusing it as
// "not a qualified reference" refused a widget mx check accepts
// (ako/mxcli#2180). registry may be nil; then nothing is excepted.
func iconRefsToResolve(stmt ast.Statement, registry *WidgetRegistry) []iconRef {
	refs := iconRefsInStatement(stmt)
	if registry == nil {
		return refs
	}
	out := refs[:0:0]
	for _, ref := range refs {
		if ref.widget != nil && defOwnsCoreName(lookupWidgetDef(ref.widget, registry), "icon") {
			continue
		}
		out = append(out, ref)
	}
	return out
}

// storedWidgetOwnsIcon reports whether the stored widget an ALTER PAGE `SET`
// names is a pluggable widget that declares an `icon` key of its own — which
// is where exec writes the value (the pluggable setter), so it is that key's
// value, not an icon reference. Unknown — no project, no document, no such
// widget — reports false and the reference is checked as before.
func storedWidgetOwnsIcon(ctx *ExecContext, s *ast.AlterPageStmt, widgetRef string) bool {
	if widgetRef == "" {
		return false
	}
	h, err := getHierarchy(ctx)
	if err != nil || h == nil {
		return false
	}
	unitID, _, _, err := resolveAlterPageUnit(ctx, s, h)
	if err != nil {
		return false
	}
	probe := openPageProbe(ctx, unitID)
	if probe == nil {
		return false
	}
	for _, key := range probe.WidgetPropertyKeys(widgetRef, "") {
		if strings.EqualFold(key, "icon") {
			return true
		}
	}
	return false
}

// iconRefsInStatement collects EVERY icon in a statement — page/snippet/layout
// widget trees, navigation menus and menu documents — glyphs included, even
// though a glyph resolves against no document.
//
// It is deliberately the single icon walk in this package. MDL078 (glyph codes)
// consumes the same list, because a second walk over the same statements is how
// one of them comes to know about a widget kind the other does not: the icon
// reference check already had to be widened once for a field it was not looking
// at, and that escape is silent both ways.
//
// Every widget-bearing field has to be walked, not just the obvious one. A
// reference the walker misses is a reference nothing checks, and the escape is
// silent both ways: `mxcli check --references` passes and `exec` succeeds, so
// the first sign is CE1613 from MxBuild — exactly the deferral this validator
// exists to prevent (mendixlabs/mxcli#1008).
func iconRefsInStatement(stmt ast.Statement) []iconRef {
	var out []iconRef
	if v := menuAlterView(stmt); v != nil {
		stmt = v // the icons an ALTER NAVIGATION / ALTER MENU writes
	}
	switch s := stmt.(type) {
	case *ast.CreatePageStmtV3:
		for _, w := range s.Widgets {
			out = append(out, iconRefsInWidget(w)...)
		}
		// Widgets is the bare body only. Content bound to a named layout
		// placeholder is held apart in Placeholders, so walking Widgets alone
		// misses every icon inside a `placeholder X { … }` block.
		for _, ph := range s.Placeholders {
			if ph == nil {
				continue
			}
			for _, w := range ph.Widgets {
				out = append(out, iconRefsInWidget(w)...)
			}
		}
	case *ast.CreateSnippetStmtV3:
		for _, w := range s.Widgets {
			out = append(out, iconRefsInWidget(w)...)
		}
	case *ast.CreateLayoutStmt:
		// A layout's icons are the costliest to get wrong: its topbar is shared,
		// so one bad reference is an error on every page using the layout.
		for _, w := range s.AllWidgets() {
			out = append(out, iconRefsInWidget(w)...)
		}
	case *ast.CreateMenuStmt:
		// A standalone menu document carries the same NavMenuItemDef as a
		// profile menu, sub-items included.
		for _, item := range s.Items {
			out = append(out, iconRefsInMenu(item)...)
		}
	case *ast.AlterPageStmt:
		for _, op := range s.Operations {
			switch o := op.(type) {
			case *ast.SetPropertyOp:
				if ref, ok := iconPropRef(o.Properties); ok {
					ref.where = "widget " + quoteName(o.Target.Name())
					if !o.Target.IsColumn() && o.Target.Item == nil {
						ref.setOn, ref.setTarget = s, o.Target.Widget
					}
					out = append(out, ref)
				}
			case *ast.InsertWidgetOp:
				for _, w := range o.Widgets {
					out = append(out, iconRefsInWidget(w)...)
				}
			case *ast.ReplaceWidgetOp:
				for _, w := range o.NewWidgets {
					out = append(out, iconRefsInWidget(w)...)
				}
			}
		}
	case *ast.CreateNavigationStmt:
		for _, item := range s.MenuItems {
			out = append(out, iconRefsInMenu(item)...)
		}
	}
	return out
}

func iconRefsInWidget(w *ast.WidgetV3) []iconRef {
	if w == nil {
		return nil
	}
	var out []iconRef
	if ref, ok := iconPropRef(w.Properties); ok {
		ref.where = strings.ToLower(w.Type) + " " + quoteName(w.Name)
		ref.widget = w
		out = append(out, ref)
	}
	for _, c := range w.Children {
		out = append(out, iconRefsInWidget(c)...)
	}
	return out
}

func iconRefsInMenu(item ast.NavMenuItemDef) []iconRef {
	var out []iconRef
	where := "menu item " + quoteName(item.Caption)
	switch {
	case item.IconKind == types.MenuIconGlyph:
		// A glyph has a code and no name. It resolves against no document, so
		// validateIconRefs skips it — but it is walked here so MDL078 can read it
		// from the same list rather than keeping a second walk of its own.
		out = append(out, iconRef{kind: types.MenuIconGlyph, code: item.IconCode, where: where})
	case normalizeIconRef(item.Icon) != "":
		kind := item.IconKind
		if kind == types.MenuIconNone {
			// An item built before the kind existed carries a name and nothing
			// else, and that name has only ever meant a collection icon.
			kind = types.MenuIconCollection
		}
		out = append(out, iconRef{value: normalizeIconRef(item.Icon), kind: kind, where: where})
	}
	for _, sub := range item.Items {
		out = append(out, iconRefsInMenu(sub)...)
	}
	return out
}

// iconPropRef pulls a widget's icon property out as a resolvable reference. MDL
// property keys are case-insensitive, so both `Icon:` and `icon:` are accepted.
//
// A glyph is returned too, carrying its code and no name: it resolves against no
// document, so validateIconRefs skips it, but MDL078 reads it from the same walk.
// Reports ok=false only when the widget carries no icon at all.
//
// The plain-string shape is still handled: `Icon:` carried one before the kinds
// existed, and callers that build a WidgetV3 directly still pass one. A reader
// that handles ONLY the typed shape stops resolving icons entirely and says
// nothing about it, which is how this check would quietly become a no-op.
func iconPropRef(props map[string]any) (iconRef, bool) {
	if props == nil {
		return iconRef{}, false
	}
	for k, v := range props {
		if !strings.EqualFold(k, "icon") {
			continue
		}
		switch val := v.(type) {
		case *ast.WidgetIcon:
			if val == nil {
				return iconRef{}, false
			}
			return widgetIconRef(*val)
		case ast.WidgetIcon:
			return widgetIconRef(val)
		case string:
			if name := normalizeIconRef(val); name != "" {
				return iconRef{value: name, kind: types.MenuIconCollection}, true
			}
		}
	}
	return iconRef{}, false
}

// normalizeIconRef strips the quoting MDL allows around an icon reference.
func normalizeIconRef(s string) string {
	return strings.Trim(strings.TrimSpace(s), "'\"")
}

func quoteName(s string) string {
	if s == "" {
		return "(unnamed)"
	}
	return "'" + s + "'"
}

// widgetIconRef turns one parsed icon into a walkable reference.
func widgetIconRef(icon ast.WidgetIcon) (iconRef, bool) {
	if icon.Kind == types.MenuIconGlyph {
		return iconRef{kind: types.MenuIconGlyph, code: icon.Code}, true
	}
	if icon.Name == "" {
		return iconRef{}, false
	}
	return iconRef{value: normalizeIconRef(icon.Name), kind: icon.Kind}, true
}

// check resolves one reference, distinguishing an unknown collection from an
// unknown member within a known one — the two need different fixes.
//
// Which listing it resolves against is decided by the reference's KIND, not by
// trying both: a collection icon and an image icon are spelled identically and
// live in different documents, so resolving one against the other's listing
// turns a correct reference into a reported typo. That is the same conflation,
// pointed the other way, that made an image icon re-execute as a custom icon.
func (idx *iconIndex) check(ref iconRef) error {
	listing, order, what, lister := idx.collections, idx.order, "icon collection", "describe icon collection"
	if ref.kind == types.MenuIconImage {
		listing, order, what, lister = idx.images, idx.imageOrder, "image collection", "describe image collection"
	}
	// An empty listing means the backend could not answer, not that the project
	// has none — reporting every reference as unknown on that basis is a false
	// error blocking a script that builds cleanly.
	if len(listing) == 0 {
		return nil
	}

	// Module.Collection.Name — the member name is the last segment, and the
	// collection is everything before it. Splitting from the right keeps working
	// if a module name ever contains a dot.
	dot := strings.LastIndex(ref.value, ".")
	if dot <= 0 || dot == len(ref.value)-1 {
		return mdlerrors.NewValidation(fmt.Sprintf(
			"%s: icon %q is not a qualified reference — write Module.Collection.Name, "+
				"for example 'Atlas_Core.Atlas_Filled.pencil'.\n  %ss in this project: %s",
			ref.where, ref.value, strings.Title(what), strings.Join(order, ", ")))
	}
	collection, member := ref.value[:dot], ref.value[dot+1:]

	members, known := listing[collection]
	if !known && isSystemCollection(collection) {
		// The System module is not stored in the project — its image
		// collection (Studio Pro's System.Images: Save, Close, …) is part of
		// the platform — so a listing of the project cannot contain it, and
		// refusing a reference into it refused describe's own navigation
		// output (ako/mxcli#980). mxbuild resolves it.
		return nil
	}
	if !known {
		msg := fmt.Sprintf("%s: unknown %s %q in icon reference %q.\n  %ss in this project: %s",
			ref.where, what, collection, ref.value, strings.Title(what), strings.Join(order, ", "))
		// The likeliest cause of this exact miss is the wrong KIND rather than a
		// typo: the name resolves, just in the other kind of collection. Saying so
		// is the difference between a two-second fix and a hunt.
		if other := idx.otherKindHint(ref, collection); other != "" {
			msg += "\n  " + other
		}
		return mdlerrors.NewValidation(msg)
	}
	if members[member] {
		return nil
	}

	msg := fmt.Sprintf("%s: %q does not exist in %s %q (MxBuild reports this as CE1613)",
		ref.where, member, what, collection)
	if near := nearestIcons(members, member); len(near) > 0 {
		msg += ".\n  Did you mean: " + strings.Join(near, ", ")
	}
	msg += fmt.Sprintf(".\n  List it with: %s %s", lister, collection)
	return mdlerrors.NewValidation(msg)
}

// isSystemCollection reports a collection of the platform's System module,
// which no project stores.
func isSystemCollection(collection string) bool {
	mod, _, ok := strings.Cut(collection, ".")
	return ok && mod == "System"
}

// otherKindHint names the fix when a reference resolves under the OTHER kind:
// the author wrote `Icon: 'Mod.Coll.x'` for what is really an image, or
// `Icon: image Mod.Coll.x` for what is really a collection icon. Both spell the
// same name, so the miss looks like a typo and is not one.
func (idx *iconIndex) otherKindHint(ref iconRef, collection string) string {
	if ref.kind == types.MenuIconImage {
		if _, ok := idx.collections[collection]; ok {
			return fmt.Sprintf("%q IS an icon collection — write `Icon: '%s'` (no `image`) for a "+
				"CustomIcons$CustomIcon.", collection, ref.value)
		}
		return ""
	}
	if _, ok := idx.images[collection]; ok {
		return fmt.Sprintf("%q IS an image collection — write `Icon: image %s` for an Images$Image. "+
			"Writing it as a plain icon reference stores a custom-icon reference, which fails the "+
			"build with CE1613.", collection, ref.value)
	}
	return ""
}

// nearestIcons suggests up to five icons whose names are close to the one
// written. Substring matching in both directions covers the common typos
// (a truncated name, an extra qualifier) without needing an edit-distance table.
func nearestIcons(icons map[string]bool, want string) []string {
	lower := strings.ToLower(want)
	var hits []string
	for name := range icons {
		l := strings.ToLower(name)
		if l == lower {
			continue
		}
		if strings.Contains(l, lower) || strings.Contains(lower, l) {
			hits = append(hits, name)
		}
	}
	sort.Strings(hits)
	if len(hits) > 5 {
		hits = hits[:5]
	}
	return hits
}
