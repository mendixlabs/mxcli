// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// buildLayoutV3 converts `create layout Module.Name ( … ) { … }` to the
// semantic model. The codec (mdl/backend/modelsdk/layout_write.go) decides the
// BSON; nothing here knows about Forms$WebLayoutContent.
func (pb *pageBuilder) buildLayoutV3(s *ast.CreateLayoutStmt) (*pages.Layout, error) {
	props := &ast.WidgetV3{Properties: s.Properties}

	// A layout header has exactly one property. Ignoring the rest would let
	// `mainplaceholder: 'Main'` — the obvious thing to reach for, and a real
	// gen property — read as accepted while nothing was written; the metamodel
	// does not declare it and no Studio Pro layout carries it.
	for k := range s.Properties {
		if !strings.EqualFold(k, "LayoutType") && !strings.EqualFold(k, "Class") &&
			!strings.EqualFold(k, "Style") && !strings.EqualFold(k, "Layout") &&
			!strings.EqualFold(k, "ShowBottomBar") && !strings.EqualFold(k, "Folder") {
			return nil, mdlerrors.NewValidation(fmt.Sprintf(
				"layout %s: unknown property %q (a layout header takes layouttype, class, style, layout, "+
					"folder and, on a native layout, showbottombar; "+
					"which placeholder is \"main\" is set by naming one Main, not by a property)",
				s.Name.String(), k))
		}
	}

	layoutType := pages.LayoutType(props.GetStringProp("LayoutType"))
	if layoutType == "" {
		return nil, mdlerrors.NewValidation(
			"layout needs a layouttype: Responsive, Phone, Tablet or ModalPopup for web; Default or Popup for native")
	}
	// The platform is inferred rather than declared: the two vocabularies are
	// disjoint (measured across all 22 layouts Atlas ships on 11.13.0), so a
	// separate `native:` property could only ever contradict the layout type.
	native, ok := platformForLayoutType(layoutType)
	if !ok {
		return nil, mdlerrors.NewValidation(fmt.Sprintf(
			"layout %s: unknown layouttype %q (want Responsive, Phone, Tablet, ModalPopup, Default or Popup)",
			s.Name.String(), layoutType))
	}
	// A native layout's widgets take the native design-property definitions
	// (ako/mxcli#1679).
	pb.themeRegistry = pb.themeRegistry.ForPlatform(native)

	// `folder:` files the layout as a page's is filed; it was refused as an
	// unknown property after check had passed it (ako/mxcli#2460).
	containerID := pb.moduleID
	if folder := props.GetStringProp("Folder"); folder != "" {
		id, err := pb.resolveFolder(folder)
		if err != nil {
			return nil, err
		}
		containerID = id
	}

	layout := &pages.Layout{
		BaseElement: model.BaseElement{
			ID:       model.ID(types.GenerateID()),
			TypeName: "Forms$Layout",
		},
		ContainerID:   containerID,
		Name:          s.Name.Name,
		Documentation: s.Documentation,
		LayoutType:    layoutType,
		Native:        native,
		Class:         props.GetStringProp("Class"),
		Style:         props.GetStringProp("Style"),
	}

	// Built on another layout (mendixlabs/mxcli#1411): the body fills that
	// layout's placeholders exactly as a page body does — bare widgets into
	// Main, `placeholder X { … }` into X — and the layout has no content tree
	// of its own. Studio Pro stores Content.Widgets empty for such a layout.
	if outer := s.OuterLayout(); outer != "" {
		if native {
			return nil, mdlerrors.NewValidation(fmt.Sprintf(
				"layout %s: a native layout cannot be built on another layout "+
					"(only web layouts carry a layout call)", s.Name.String()))
		}
		if strings.EqualFold(outer, s.Name.String()) {
			return nil, mdlerrors.NewValidation(fmt.Sprintf(
				"layout %s cannot be built on itself", s.Name.String()))
		}
		outerID, err := pb.resolveLayout(outer)
		if err != nil {
			return nil, mdlerrors.NewValidation(fmt.Sprintf(
				"layout %s is built on %s, which was not found (list layouts with `show layouts`)",
				s.Name.String(), outer))
		}
		args, err := pb.buildLayoutCallArguments(outer, s.Widgets, s.Placeholders)
		if err != nil {
			return nil, err
		}
		layout.LayoutCall = &pages.LayoutCall{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$LayoutCall",
			},
			LayoutID:   outerID,
			LayoutName: outer,
			Arguments:  args,
		}
		return layout, nil
	}

	expanded, err := pb.expandFragments(s.Widgets)
	if err != nil {
		return nil, err
	}
	expanded, err = pb.applyNativeLayoutParts(s, layout, props, expanded)
	if err != nil {
		return nil, err
	}
	for _, astWidget := range expanded {
		w, err := pb.buildWidgetV3(astWidget)
		if err != nil {
			return nil, mdlerrors.NewBackend("build widget", err)
		}
		layout.Widgets = append(layout.Widgets, w)
	}

	return layout, nil
}

// applyNativeLayoutParts reads what a native layout keeps beside its content
// tree — `showbottombar: true`, `region header { placeholder X }` and
// `region sidebar { … }` — and returns the remaining top-level widgets. On a
// web layout every one of them is refused: Forms$WebLayoutContent has none.
func (pb *pageBuilder) applyNativeLayoutParts(s *ast.CreateLayoutStmt, layout *pages.Layout, props *ast.WidgetV3, widgets []*ast.WidgetV3) ([]*ast.WidgetV3, error) {
	for k, v := range props.Properties {
		if !strings.EqualFold(k, "ShowBottomBar") {
			continue
		}
		if !layout.Native {
			return nil, mdlerrors.NewValidation(fmt.Sprintf(
				"layout %s: showbottombar is a native layout's property; a web layout has no bottom bar setting", s.Name.String()))
		}
		on, ok := v.(bool)
		if !ok {
			return nil, mdlerrors.NewValidation(fmt.Sprintf("layout %s: showbottombar takes true or false", s.Name.String()))
		}
		layout.ShowBottomBar = on
	}
	var rest []*ast.WidgetV3
	for _, w := range widgets {
		if !strings.EqualFold(w.Type, "region") {
			rest = append(rest, w)
			continue
		}
		part := strings.ToLower(w.Name)
		if !layout.Native || (part != "header" && part != "sidebar") {
			return nil, mdlerrors.NewValidation(fmt.Sprintf(
				"layout %s: a region at the top of a layout is a native layout's `region header { placeholder X }` "+
					"or `region sidebar { … }`; a web layout's regions belong to its scrollcontainer", s.Name.String()))
		}
		switch part {
		case "header":
			if layout.HeaderPlaceholder != nil || len(w.Children) != 1 || !strings.EqualFold(w.Children[0].Type, "placeholder") {
				return nil, mdlerrors.NewValidation(fmt.Sprintf(
					"layout %s: `region header` holds exactly one placeholder — the one on the header's right", s.Name.String()))
			}
			ph, err := pb.buildPlaceholderV3(w.Children[0])
			if err != nil {
				return nil, err
			}
			layout.HeaderPlaceholder = ph.(*pages.LayoutPlaceholder)
		case "sidebar":
			if layout.Sidebar {
				return nil, mdlerrors.NewValidation(fmt.Sprintf("layout %s: region sidebar given twice", s.Name.String()))
			}
			layout.Sidebar = true
			for _, c := range w.Children {
				cw, err := pb.buildWidgetV3(c)
				if err != nil {
					return nil, mdlerrors.NewBackend("build widget", err)
				}
				layout.SidebarWidgets = append(layout.SidebarWidgets, cw)
			}
		}
	}
	return rest, nil
}

// platformForLayoutType maps a layout type onto the platform that uses it.
// Reports false for a value neither platform attests to, so a typo is an error
// rather than a web layout with a meaningless type.
func platformForLayoutType(t pages.LayoutType) (native bool, ok bool) {
	switch {
	case pages.ValidLayoutType(t, false):
		return false, true
	case pages.ValidLayoutType(t, true):
		return true, true
	}
	return false, false
}

// execCreateLayout handles CREATE [OR REPLACE|MODIFY] LAYOUT.
func execCreateLayout(ctx *ExecContext, s *ast.CreateLayoutStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return mdlerrors.NewBackend(fmt.Sprintf("find module %s", s.Name.Module), err)
	}

	// A layout in a marketplace module is not ours to rewrite: an upgrade
	// replaces the module wholesale and the edit is gone, which is the whole
	// reason Mendix's own guidance is to put custom layouts in a module of your
	// own. Refusing here is cheaper than discovering it at upgrade time.
	if isMarketplaceModule(ctx, module) {
		return mdlerrors.NewValidation(fmt.Sprintf(
			"layout %s: %s is a marketplace module — a layout written there is overwritten by the next module update. "+
				"Create the layout in a module of your own (Mendix's own guidance) and point pages at it with ALTER PAGE … SET LAYOUT.",
			s.Name.String(), s.Name.Module))
	}

	// The stored layout this statement rewrites, if there is one, plus any
	// duplicates of the same name to clear out.
	existing, _ := ctx.Backend.ListLayouts()
	var stored *pages.Layout
	var duplicates []model.ID
	for _, l := range existing {
		modName := getModuleName(ctx, getModuleID(ctx, l.ContainerID))
		if modName != s.Name.Module || l.Name != s.Name.Name {
			continue
		}
		if !s.IsReplace && !s.IsModify {
			return mdlerrors.NewAlreadyExists("layout", s.Name.String())
		}
		if stored == nil {
			stored = l
			continue
		}
		duplicates = append(duplicates, l.ID)
	}

	// A rewrite must not delete widgets MDL cannot author (mendixlabs/mxcli#1622).
	// describe finds a layout by name, so only a name with one stored layout
	// can be described.
	if stored != nil && len(duplicates) == 0 {
		if err := checkNoUnauthorableWidgetsDropped(ctx, "layout", s.Name, stored.ID, s.AllWidgets()); err != nil {
			return err
		}
	}

	pb := &pageBuilder{
		ctx:              ctx,
		backend:          ctx.Backend,
		moduleID:         module.ID,
		moduleName:       s.Name.Module,
		widgetScope:      make(map[string]model.ID),
		paramScope:       make(map[string]model.ID),
		paramEntityNames: make(map[string]string),
		execCache:        ctx.Cache,
		fragments:        ctx.Fragments,
		themeRegistry:    ctx.GetThemeRegistry(),
		widgetBackend:    ctx.Backend,
		// The root of a document that this pass walks in full: there is no
		// enclosing data widget, so there is no context object. #1029.
		argCtx:           atDocumentRoot(),
		isDocumentWidget: widgetNameSet(s.AllWidgets()),
	}

	// As for pages and snippets (#721 L4, #832): a layout's pluggable widgets
	// were rebuilt from the template on every rewrite, unchanged or not.
	if stored != nil && len(duplicates) == 0 {
		pb.storedPluggables = loadPluggablePassthrough(ctx, stored.ID, func() error {
			return describeLayout(ctx, s.Name)
		})
	}

	// Built before the old one is deleted: a build failure must leave the
	// project as it was, not layout-less.
	layout, err := pb.buildLayoutV3(s)
	if err != nil {
		return err
	}

	if err := checkLayoutCallArguments(ctx, layout); err != nil {
		return err
	}

	// A rewrite that carried no doc comment keeps the stored one (#1018).
	if stored != nil {
		layout.Documentation = carriedDocumentation(s.DocumentationSet, s.Documentation, stored.Documentation)
	}

	// A duplicate of the same name is cleared; the FIRST stored layout is
	// rewritten rather than deleted (see below), so it is not in this list.
	for _, id := range duplicates {
		if err := ctx.Backend.DeleteLayout(id); err != nil {
			return mdlerrors.NewBackend("delete existing layout", err)
		}
	}

	verb := "Created"
	if stored != nil {
		verb = "Replaced"
		// REWRITE THE STORED UNIT, do not replace it. A delete followed by a
		// create goes through InsertUnit under a freshly minted id, so an
		// identical re-run replaced the layout's unit under a NEW GUID every
		// time — measured on 11.14.0, three runs produced three different
		// .mxunit files, each run a delete plus an untracked add, and the tree
		// never came back clean (ako/mxcli#600). The storage layer's net for
		// delete+insert recreates (#556) keys on the unit id and so cannot
		// catch a path that re-mints it; this has to be decided here.
		//
		// Carrying the stored container is the other half: there is no FOLDER
		// clause on CREATE LAYOUT, so the rebuilt layout always names the
		// module root, and an insert would file a foldered layout back into it
		// (the defect #932 fixed for REST clients). UpdateLayout does not touch
		// the unit's row at all.
		wanted := layout.ContainerID
		layout.ID = stored.ID
		layout.ContainerID = stored.ContainerID
		if err := ctx.Backend.UpdateLayout(layout); err != nil {
			return mdlerrors.NewBackend("update layout", err)
		}
		// A `folder:` on the rewrite moves the layout there; without one the
		// stored folder stands (ako/mxcli#2460).
		if layoutHasFolder(s) {
			if _, err := applyDocumentFolder(ctx, layout.ID, stored.ContainerID, wanted); err != nil {
				return err
			}
		}
	} else if err := ctx.Backend.CreateLayout(layout); err != nil {
		return mdlerrors.NewBackend("create layout", err)
	}

	invalidateHierarchy(ctx)

	// Through ReportMutation: the rewrite now reaches canon.Reconcile, so a
	// statement that matches what is stored is elided and must say so rather
	// than claiming a replacement that did not happen.
	ctx.ReportMutation(verb, "layout %s", s.Name.String())
	return nil
}

// isMarketplaceModule reports whether the module came from the Marketplace.
//
// Both stored signals count. FromAppStore is the flag Studio Pro sets and
// SHOW MODULES reports; AppStoreGuid is the version UUID the importer writes.
// A real import carries both, but checking only one makes the guard depend on
// which field a given path happened to populate — and a marketplace guard that
// quietly stops firing is worse than no guard, because the refusal is what
// stops an edit an update will erase. The module *name* is not a signal: a user
// may name a module Atlas_Core.
func isMarketplaceModule(ctx *ExecContext, module *model.Module) bool {
	if module == nil {
		return false
	}
	return module.FromAppStore || strings.TrimSpace(module.AppStoreGuid) != ""
}

// checkLayoutCallArguments refuses an argument naming a placeholder the outer
// layout does not declare. It fills nothing: mxbuild reports it as CE1613 ("The
// selected placeholder … no longer exists") a whole build later, naming the
// layout rather than the statement. The same rule ALTER PAGE … SET LAYOUT
// applies to a page's bindings.
func checkLayoutCallArguments(ctx *ExecContext, layout *pages.Layout) error {
	lc := layout.LayoutCall
	if lc == nil || lc.LayoutID == "" {
		return nil
	}
	declared, err := ctx.Backend.LayoutPlaceholders(lc.LayoutID)
	if err != nil {
		return mdlerrors.NewBackend("read placeholders of "+lc.LayoutName, err)
	}
	// A layout that reports none is one the backend could not read, not one
	// that has none (see checkRepointPlaceholdersAgainst).
	if len(declared) == 0 {
		return nil
	}
	have := make(map[string]bool, len(declared))
	for _, d := range declared {
		have[d] = true
	}
	var missing []string
	for _, arg := range lc.Arguments {
		name := strings.TrimPrefix(string(arg.ParameterID), lc.LayoutName+".")
		if !have[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return mdlerrors.NewValidation(fmt.Sprintf(
		"layout %s is built on %s, which does not declare the placeholder %s; it has %s",
		layout.Name, lc.LayoutName, strings.Join(missing, ", "), strings.Join(declared, ", ")))
}

// layoutHasFolder reports whether a create layout header names a folder.
func layoutHasFolder(s *ast.CreateLayoutStmt) bool {
	for k, v := range s.Properties {
		if strings.EqualFold(k, "Folder") {
			if str, ok := v.(string); ok && str != "" {
				return true
			}
		}
	}
	return false
}
