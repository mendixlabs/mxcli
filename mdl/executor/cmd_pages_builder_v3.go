// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"log"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/widgets/mpk"
)

// ============================================================================
// V3 Page Builder
// ============================================================================

// buildPageV3 creates a Page from a CreatePageStmtV3.
func (pb *pageBuilder) buildPageV3(s *ast.CreatePageStmtV3) (*pages.Page, error) {
	// Resolve folder if specified
	containerID := pb.moduleID
	if s.Folder != "" {
		folderID, err := pb.resolveFolder(s.Folder)
		if err != nil {
			return nil, mdlerrors.NewBackend("resolve folder "+s.Folder, err)
		}
		containerID = folderID
	}

	page := &pages.Page{
		BaseElement: model.BaseElement{
			ID:       model.ID(types.GenerateID()),
			TypeName: "Forms$Page",
		},
		ContainerID:   containerID,
		Name:          s.Name.Name,
		Documentation: s.Documentation,
		URL:           s.URL,
		MarkAsUsed:    false,
		Excluded:      s.Excluded,
		// Pop-up dimensions (issues #661, #713): Studio Pro's own default for a
		// pop-up page is 0/0 (auto-size) — verified against a live 11.12 model —
		// so an unset dimension stays 0, matching what Studio Pro stores. The
		// page header can override. The writers serialize these top-level
		// Forms$Page fields.
		PopupWidth:     0,
		PopupHeight:    0,
		PopupResizable: false,
		// Page CSS class / inline style (issue #714).
		Class: s.Class,
		Style: s.Style,
	}
	if s.PopupWidth != nil {
		page.PopupWidth = *s.PopupWidth
	}
	if s.PopupHeight != nil {
		page.PopupHeight = *s.PopupHeight
	}
	if s.PopupResizable != nil {
		page.PopupResizable = *s.PopupResizable
	}
	page.PopupCloseAction = s.PopupCloseAction

	// Set title
	if s.Title != "" {
		page.Title = &model.Text{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Texts$Text",
			},
			Translations: map[string]string{pb.textLang(): s.Title},
		}
	}

	// Resolve layout
	if s.Layout != "" {
		layoutID, err := pb.resolveLayout(s.Layout)
		if err != nil {
			// Layout not found is not fatal - page will work but may not render correctly
			log.Printf("warning: layout %s not found", s.Layout)
		} else {
			page.LayoutID = layoutID

			// Create LayoutCall with arguments for placeholders
			page.LayoutCall = &pages.LayoutCall{
				BaseElement: model.BaseElement{
					ID:       model.ID(types.GenerateID()),
					TypeName: "Forms$LayoutCall",
				},
				LayoutID:   layoutID,
				LayoutName: s.Layout, // Qualified name for "Form" field in BSON
			}
		}
	}

	// Build parameters
	for _, param := range s.Parameters {
		pageParam := &pages.PageParameter{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$PageParameter",
			},
			ContainerID: page.ID,
			Name:        param.Name,
			IsRequired:  true, // Page parameters are required by default
		}

		// Check if this is a primitive type or entity type
		if bsonType := pageParamBSONType(param.Type); bsonType != "" {
			// Primitive type parameter
			pageParam.TypeName = bsonType
		} else if param.EntityType.Name != "" {
			// Entity type parameter
			entityID, err := pb.resolveEntity(param.EntityType)
			if err != nil {
				return nil, mdlerrors.NewBackend("resolve entity "+param.EntityType.String(), err)
			}
			entityName := param.EntityType.String()
			pageParam.EntityID = entityID
			pageParam.EntityName = entityName // Qualified entity name for BSON
			pb.paramScope[param.Name] = entityID
			pb.paramEntityNames[param.Name] = entityName
		}

		page.Parameters = append(page.Parameters, pageParam)
	}

	// Build variables
	if pb.localVariables == nil {
		pb.localVariables = make(map[string]bool, len(s.Variables))
	}
	for _, v := range s.Variables {
		localVar := &pages.LocalVariable{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$LocalVariable",
			},
			ContainerID:  page.ID,
			Name:         v.Name,
			DefaultValue: v.DefaultValue,
		}
		localVar.VariableType, localVar.EnumerationRef = pageVariableType(v.DataType)
		page.Variables = append(page.Variables, localVar)
		pb.localVariables[v.Name] = true
	}

	// A page's widget tree is built into the LayoutCall's placeholder arguments
	// below — so without a LayoutCall the widgets have nowhere to go and are
	// silently dropped, and Mendix rejects the layout-less page at build time
	// (CE1613, "layout … no longer exists"). This happens when the `Layout:` clause
	// is omitted, or names a layout that does not exist. Reject it with an
	// actionable error instead of producing a broken page that lost its widgets.
	if page.LayoutCall == nil && (len(s.Widgets) > 0 || len(s.Placeholders) > 0) {
		if s.Layout == "" {
			return nil, mdlerrors.NewValidationf(
				"page '%s' has widgets but no Layout: clause — a Mendix page requires a layout to place its widgets (without one they are dropped and the build fails CE1613). Add a layout, e.g. `Layout: Atlas_Core.Atlas_Default`.",
				s.Name.String())
		}
		return nil, mdlerrors.NewValidationf(
			"page '%s' references layout '%s', which was not found — its widgets would be dropped. Use an existing layout (list them with `show catalog table layouts`).",
			s.Name.String(), s.Layout)
	}

	// Build one FormCallArgument per layout placeholder (issue #532). Bare body
	// widgets bind to Main; a `placeholder <Name> { … }` block binds to that
	// named placeholder. The Main argument is always emitted (possibly empty) to
	// match Studio Pro. Placeholder refs are BY_NAME: "<Layout>.<Placeholder>".
	if page.LayoutCall != nil {
		order := []string{"Main"}
		byName := map[string][]*ast.WidgetV3{"Main": append([]*ast.WidgetV3(nil), s.Widgets...)}
		for _, ph := range s.Placeholders {
			name := ph.Name
			if name == "" {
				name = "Main"
			}
			if _, seen := byName[name]; !seen {
				order = append(order, name)
			}
			byName[name] = append(byName[name], ph.Widgets...)
		}

		for _, name := range order {
			arg := &pages.LayoutCallArgument{
				BaseElement: model.BaseElement{
					ID:       model.ID(types.GenerateID()),
					TypeName: "Forms$FormCallArgument",
				},
				ParameterID: model.ID(s.Layout + "." + name),
			}
			// The placeholder's widgets go in directly. Forms$FormCallArgument carries a
			// Widgets array and Studio Pro fills it with the page's top-level widgets;
			// wrapping them in a synthetic DivContainer added a phantom container to
			// every mxcli-authored page (#760).
			if widgets := byName[name]; len(widgets) > 0 {
				expanded, err := pb.expandFragments(widgets)
				if err != nil {
					return nil, err
				}
				for _, astWidget := range expanded {
					w, err := pb.buildWidgetV3(astWidget)
					if err != nil {
						return nil, mdlerrors.NewBackend("build widget", err)
					}
					arg.Widgets = append(arg.Widgets, w)
				}
			}
			page.LayoutCall.Arguments = append(page.LayoutCall.Arguments, arg)
		}
	}

	return page, nil
}

// buildSnippetV3 creates a Snippet from a CreateSnippetStmtV3.
func (pb *pageBuilder) buildSnippetV3(s *ast.CreateSnippetStmtV3) (*pages.Snippet, error) {
	// Resolve folder if specified
	containerID := pb.moduleID
	if s.Folder != "" {
		folderID, err := pb.resolveFolder(s.Folder)
		if err != nil {
			return nil, mdlerrors.NewBackend("resolve folder "+s.Folder, err)
		}
		containerID = folderID
	}

	snippet := &pages.Snippet{
		BaseElement: model.BaseElement{
			ID:       model.ID(types.GenerateID()),
			TypeName: "Forms$Snippet",
		},
		ContainerID:   containerID,
		Name:          s.Name.Name,
		Documentation: s.Documentation,
	}

	// Build parameters
	for _, param := range s.Parameters {
		snippetParam := &pages.SnippetParameter{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$SnippetParameter",
			},
			ContainerID: snippet.ID,
			Name:        param.Name,
		}

		// A snippet parameter must name an entity. A primitive one is refused
		// rather than resolved as an entity name — the reported symptom was
		// "entity not found: string", for a type nobody spelled — and rather
		// than written, which storage would allow and mxbuild would not
		// (CE0046). Same rule check applies, so a script cannot pass one and
		// fail the other (mendixlabs/mxcli#1028).
		if caption := types.SnippetParameterTypeRule(pageParamBSONType(param.Type)); caption != "" {
			return nil, mdlerrors.NewValidationf(
				"snippet '%s' declares parameter $%s with the primitive type %s — a snippet "+
					"parameter must be an entity, and mxbuild rejects a primitive one with "+
					"CE0046 (\"Invalid data type '%s'.\"). Pass the value on an object, or "+
					"keep the primitive on the calling page's parameters.",
				s.Name.String(), param.Name, paramTypeSourceName(param.Type), caption)
		}
		if param.EntityType.Name != "" {
			entityID, err := pb.resolveEntity(param.EntityType)
			if err != nil {
				return nil, mdlerrors.NewBackend("resolve entity "+param.EntityType.String(), err)
			}
			entityName := param.EntityType.String()
			snippetParam.EntityID = entityID
			snippetParam.EntityName = entityName
			// Only entity-typed parameters enter paramScope — it maps a name to
			// an entity ID, and a primitive has none. Same as the page path.
			pb.paramScope[param.Name] = entityID
			pb.paramEntityNames[param.Name] = entityName
		}

		snippet.Parameters = append(snippet.Parameters, snippetParam)
	}

	// Build variables
	if pb.localVariables == nil {
		pb.localVariables = make(map[string]bool, len(s.Variables))
	}
	for _, v := range s.Variables {
		localVar := &pages.LocalVariable{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$LocalVariable",
			},
			ContainerID:  snippet.ID,
			Name:         v.Name,
			DefaultValue: v.DefaultValue,
		}
		localVar.VariableType, localVar.EnumerationRef = pageVariableType(v.DataType)
		snippet.Variables = append(snippet.Variables, localVar)
		pb.localVariables[v.Name] = true
	}

	// Build widgets (expanding fragments)
	pb.isSnippet = true
	defer func() { pb.isSnippet = false }()

	expanded, err := pb.expandFragments(s.Widgets)
	if err != nil {
		return nil, err
	}
	for _, astWidget := range expanded {
		w, err := pb.buildWidgetV3(astWidget)
		if err != nil {
			return nil, mdlerrors.NewBackend("build widget", err)
		}
		snippet.Widgets = append(snippet.Widgets, w)
	}

	return snippet, nil
}

// buildWidgetV3 converts a V3 AST widget to a pages.Widget.
//
// Keyword dispatch (Phase 2 — issue #539): the keywordDispatchTable encodes
// our editorial policy for dual-stack keywords (e.g. DATAGRID → pluggable
// Datagrid 2.x). DATAGRID has no switch case below; it falls through to the
// default branch, which resolves it via the widget registry (a datagrid.def.json
// generated on demand from the project .mpk) and builds it through the generic
// pluggable engine (buildPluggable) — columns as an ObjectList, per-column filters
// as item slots. The dispatch table is consumed by inspection commands and
// DESCRIBE-side keyword resolution rather than overriding write-side routing here.
func (pb *pageBuilder) buildWidgetV3(w *ast.WidgetV3) (pages.Widget, error) {
	// What a SHOW_PAGE argument inside this widget may bind to. The data widgets
	// below overwrite it with the context they actually create; this only stops
	// "no context object at all" surviving past a widget whose data source this
	// pass cannot read. See argContextForSubtreeOf.
	if next := argContextForSubtreeOf(w, pb.argCtx); next != pb.argCtx {
		old := pb.argCtx
		pb.argCtx = next
		defer func() { pb.argCtx = old }()
	}
	oldWidget := pb.currentWidget
	pb.currentWidget = w.Name
	defer func() { pb.currentWidget = oldWidget }()

	var widget pages.Widget
	var err error

	if err := checkSearchByIsOnAListView(w); err != nil {
		return nil, err
	}

	switch strings.ToLower(w.Type) {
	case "dataview":
		widget, err = pb.buildDataViewV3(w)
	case "legacydatagrid":
		// LEGACYDATAGRID requests the dojo-based native Forms$DataGrid (the
		// pre-pluggable widget). The codebase doesn't yet have a builder for
		// it — pluggable Datagrid (the DATAGRID keyword default) covers the
		// modern path. Native implementation is tracked under Phase 2.1; for
		// now, return an actionable error so the silent-wrong-output path is
		// closed.
		return nil, mdlerrors.NewUnsupported(
			"LEGACYDATAGRID (native Forms$DataGrid) is not yet implemented. " +
				"Use DATAGRID for the pluggable equivalent on Mendix 11+, " +
				"or open the project in Studio Pro to add native datagrids manually.")
	case "listview":
		widget, err = pb.buildListViewV3(w)
	case "layoutgrid":
		widget, err = pb.buildLayoutGridV3(w)
	case "row":
		// ROW creates a container with LayoutGrid that contains one row
		widget, err = pb.buildContainerWithRowV3(w)
	case "column":
		// COLUMN creates a container with LayoutGrid that contains one column
		widget, err = pb.buildContainerWithColumnV3(w)
	case "container", "customcontainer":
		widget, err = pb.buildContainerV3(w)
	case "slot":
		// A slot that reaches the builder was written somewhere the fragment
		// expander doesn't reach (e.g. nested in a page container rather than a
		// `define fragment` body). Slots are resolved during fragment expansion.
		return nil, mdlerrors.NewValidation("`slot` is only valid inside a `define fragment` body")
	case "textbox":
		widget, err = pb.buildTextBoxV3(w)
	case "textarea":
		widget, err = pb.buildTextAreaV3(w)
	case "datepicker":
		widget, err = pb.buildDatePickerV3(w)
	case "dropdown":
		widget, err = pb.buildDropdownV3(w)
	case "checkbox":
		widget, err = pb.buildCheckBoxV3(w)
	case "text", "statictext":
		widget, err = pb.buildTextWidgetV3(w)
	case "dynamictext":
		widget, err = pb.buildDynamicTextV3(w)
	case "title":
		widget, err = pb.buildTitleV3(w)
	case "button", "actionbutton", "linkbutton":
		widget, err = pb.buildButtonV3(w)
	case "tabcontainer":
		widget, err = pb.buildTabContainerV3(w)
	case "tabpage":
		// Tab pages are handled inside TabContainer
		return nil, mdlerrors.NewValidation("tabpage must be a direct child of tabcontainer")
	case "groupbox":
		widget, err = pb.buildGroupBoxV3(w)
	case "scrollcontainer":
		widget, err = pb.buildScrollContainerV3(w)
	case "region":
		// A region is a slot of a scroll container, not a widget in its own
		// right — it has no Name in the BSON, only a position.
		return nil, mdlerrors.NewValidation("region must be a direct child of scrollcontainer")
	case "navigationtree":
		widget, err = pb.buildNavigationTreeV3(w)
	case "menubar":
		widget, err = pb.buildMenuBarV3(w)
	case "placeholder":
		widget, err = pb.buildPlaceholderV3(w)
	case "radiobuttons":
		widget, err = pb.buildRadioButtonsV3(w)
	case "navigationlist":
		widget, err = pb.buildNavigationListV3(w)
	case "item":
		// Items are handled inside NavigationList
		return nil, mdlerrors.NewValidation("item must be a direct child of navigationlist")
	case "snippetcall":
		widget, err = pb.buildSnippetCallV3(w)
	case "footer":
		widget, err = pb.buildFooterV3(w)
	case "header":
		widget, err = pb.buildHeaderV3(w)
	case "controlbar":
		widget, err = pb.buildControlBarV3(w)
	case "template":
		widget, err = pb.buildTemplateV3(w)
	case "filter":
		widget, err = pb.buildFilterV3(w)
	case "staticimage":
		widget, err = pb.buildStaticImageV3(w)
	case "dynamicimage":
		widget, err = pb.buildDynamicImageV3(w)
	case "image":
		// IMAGE routes to the pluggable React widget (com.mendix.widget.web.image.Image)
		pb.initPluggableEngine()
		if pb.widgetRegistry != nil {
			if def, ok := pb.widgetRegistry.Get("image"); ok {
				return pb.buildPluggable(def, w)
			}
		}
		// Fallback to static image if pluggable engine unavailable
		widget, err = pb.buildStaticImageV3(w)
	default:
		pb.initPluggableEngine()
		if pb.widgetRegistry != nil {
			// Try by MDL name first
			if def, ok := pb.widgetRegistry.Get(strings.ToUpper(w.Type)); ok {
				return pb.buildPluggable(def, w)
			}
			// PLUGGABLEWIDGET/CUSTOMWIDGET 'widget.id' name — lookup by widget ID
			if w.Type == "pluggablewidget" || w.Type == "customwidget" {
				if widgetType, ok := w.Properties["WidgetType"].(string); ok {
					if def, ok := pb.widgetRegistry.GetByWidgetID(widgetType); ok {
						return pb.buildPluggable(def, w)
					}
					return nil, mdlerrors.NewNotFoundMsg("widget", widgetType, pb.missingWidgetMessage(widgetType))
				}
			}
		}
		if pb.pluggableEngineErr != nil {
			return nil, mdlerrors.NewUnsupported(fmt.Sprintf("unsupported widget type: %s (%v)", w.Type, pb.pluggableEngineErr))
		}
		// Common cause: this is an object-list child keyword (group, item, marker,
		// series, …) whose parent widget's .def.json was generated by an older
		// mxcli build that didn't yet emit `objectLists`. Suggest a refresh.
		return nil, mdlerrors.NewUnsupported(fmt.Sprintf(
			"unsupported widget type: %s — if this is a child of a pluggable widget (e.g. accordion group, popupmenu item), refresh the project's widget definitions: 'mxcli widget init -p app.mpr'",
			w.Type,
		))
	}

	if err != nil {
		return nil, err
	}

	// Apply Class/Style appearance properties to the widget
	if err := applyWidgetAppearance(widget, w, pb.themeRegistry); err != nil {
		return nil, err
	}

	// Apply conditional visibility/editability
	applyConditionalSettings(widget, w)

	return widget, nil
}

// buildPluggable builds a pluggable widget via the engine and then applies
// `class`/`style`/design-property appearance, which the engine itself does not
// handle. CustomWidget embeds BaseWidget and already serializes an Appearance
// node, so this only fills in values the user set — no structural BSON change.
//
// Conditional visibility/editability is intentionally NOT applied here: the
// CustomWidget serializer currently hardcodes those settings to nil, so wiring
// them would have no effect (and is tracked separately).
func (pb *pageBuilder) buildPluggable(def *WidgetDefinition, w *ast.WidgetV3) (pages.Widget, error) {
	widget, err := pb.pluggableEngine.Build(def, w)
	if err != nil {
		return nil, err
	}
	if err := applyWidgetAppearance(widget, w, pb.themeRegistry); err != nil {
		return nil, err
	}
	return widget, nil
}

// applyConditionalSettings sets ConditionalVisibility and ConditionalEditability
// on a widget if VISIBLE IF or EDITABLE IF properties are specified in the AST.
func applyConditionalSettings(widget pages.Widget, w *ast.WidgetV3) {
	type baseWidgetGetter interface {
		GetBaseWidget() *pages.BaseWidget
	}
	bwg, ok := widget.(baseWidgetGetter)
	if !ok {
		return
	}
	bw := bwg.GetBaseWidget()

	if visibleIf := w.GetStringProp("VisibleIf"); visibleIf != "" {
		// `Visible: [expr]` — bracket form, expression already rooted by the visitor.
		bw.ConditionalVisibility = &pages.ConditionalVisibilitySettings{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$ConditionalVisibilitySettings",
			},
			Expression: visibleIf,
		}
	} else if expr, ok := pages.StaticVisibleExpression(w.Properties["Visible"]); ok {
		// `Visible: false` or `Visible: '<expr>'` — a page widget has no plain
		// boolean Visible field, so route it through ConditionalVisibilitySettings
		// (previously this value was parsed but never consumed → silently dropped).
		bw.ConditionalVisibility = &pages.ConditionalVisibilitySettings{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$ConditionalVisibilitySettings",
			},
			Expression: expr,
		}
	}

	if editableIf := w.GetStringProp("EditableIf"); editableIf != "" {
		bw.ConditionalEditability = &pages.ConditionalEditabilitySettings{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$ConditionalEditabilitySettings",
			},
			Expression: editableIf,
		}
	} else if editable, ok := pages.CanonicalEditability(w.GetStringProp("Editable")); ok {
		// `Editable: Never` — parsed and validated (MDL-WIDGET20 checks the widget
		// TYPE) and then dropped, because nothing carried it to the writers, which
		// hardcoded "Always". Same shape as the `Visible: false` case above.
		//
		// EDITABLE IF wins when both are given: the conditional settings element is
		// what makes the enum "Conditional", so honouring a plain `Editable` too
		// would write an enum contradicting the element beside it.
		bw.Editable = editable
	}
}

// applyWidgetAppearance sets Class, Style, DynamicClasses, and DesignProperties on a widget
// if specified in the AST.
// The theme registry (if non-nil) is used to determine the correct BSON type for each design property.
func applyWidgetAppearance(widget pages.Widget, w *ast.WidgetV3, theme *ThemeRegistry) error {
	class, style := w.GetClass(), w.GetStyle()

	// A DynamicText with an inline Style crashes MxBuild with a
	// NullReferenceException. Reject it here (the workaround is to wrap the
	// widget in a container and style the container instead).
	if style != "" && strings.EqualFold(w.Type, "dynamictext") {
		return mdlerrors.NewValidationf(
			"dynamictext %q: an inline `style` crashes MxBuild — wrap it in a container and style the container instead",
			w.Name,
		)
	}

	if class != "" || style != "" {
		type appearanceSetter interface {
			SetAppearance(class, style string)
		}
		if setter, ok := widget.(appearanceSetter); ok {
			setter.SetAppearance(class, style)
		}
	}

	// Apply the DynamicClasses expression (a runtime-computed class list on the
	// widget's Forms$Appearance). Every widget that embeds BaseWidget supports it.
	if dynamicClasses := w.GetDynamicClasses(); dynamicClasses != "" {
		type dynamicClassesSetter interface {
			SetDynamicClasses(dynamicClasses string)
		}
		if setter, ok := widget.(dynamicClassesSetter); ok {
			setter.SetDynamicClasses(dynamicClasses)
		}
	}

	// Apply design properties
	astProps := w.GetDesignProperties()
	if len(astProps) > 0 {
		// Resolve the widget's design-property definitions from the theme registry
		// (when loaded) so each value's BSON type is taken from metadata
		// (ColorPicker/ToggleButtonGroup → custom) rather than guessed. Nil/empty
		// when no project/themesource — the converter then falls back to option.
		var themeProps []ThemeProperty
		if theme != nil {
			themeProps = theme.GetPropertiesForWidget(resolveDesignPropsKey(w.Type))
		}
		var dpValues []pages.DesignPropertyValue
		for _, p := range astProps {
			// Refuse rather than write, when the theme proves the shape wrong —
			// a flat value on a multi-select property (ako/mxcli#511). Silently
			// writing it produced a document mxbuild rejects with CE6084, whose
			// wording names a type mismatch and not the spelling that fixes it.
			dp, ok, err := astDesignPropToValueChecked(p, themeProps)
			if err != nil {
				return fmt.Errorf("widget %q: %w", w.Name, err)
			}
			if ok {
				dpValues = append(dpValues, dp)
			}
		}
		if len(dpValues) > 0 {
			type designPropSetter interface {
				SetDesignProperties(props []pages.DesignPropertyValue)
			}
			if setter, ok := widget.(designPropSetter); ok {
				setter.SetDesignProperties(dpValues)
			}
		}
	}
	return nil
}

// astDesignPropToValue converts one MDL design-property entry to a
// pages.DesignPropertyValue. Compound entries (a key whose value is a nested
// list, e.g. 'Spacing': ['margin-top': 'Large', …]) recurse into sub-properties.
// Returns ok=false for an entry that should be skipped (a flat toggle set OFF).
//
// themeProps are the widget's design-property definitions from the theme registry
// (nil/empty when no project/themesource). When available, a flat value's BSON
// type is taken from the property's declared Type — a ColorPicker or
// ToggleButtonGroup materializes as Forms$CustomDesignPropertyValue rather than
// the option default (findings: typed design properties). Without metadata the
// prior syntactic behaviour (on→toggle, else→option) is preserved.
// astDesignPropToValueChecked is astDesignPropToValue plus the one shape the
// theme can prove wrong: a FLAT value on a property declared `"multiSelect": true`.
//
// Such a property is a SET of the declared options, and Mendix stores it as a
// Forms$CompoundDesignPropertyValue holding one entry per selected option, each
// valued with a bare Forms$ToggleDesignPropertyValue — measured by decoding a
// Studio Pro-authored Atlas page in a blank 11.12.2 project. Structurally that is
// `Spacing`, which MDL already writes, so the capability is not missing: the
// compound spelling works, round-trips through DESCRIBE, and builds at 0 errors.
//
// The flat spelling is the trap. `'Hide on': 'Phone'` names a declared option, so
// resolveDesignPropertyValueType returned "option" and the write produced a
// document mxbuild refuses:
//
//	[CE6084] "Expected design property Hide on to be of type Toggle button group,
//	         but found Option."
//
// Refusing it and naming the spelling that works is the fix; the author cannot
// derive `['Phone': on]` from CE6084's wording (ako/mxcli#511).
func astDesignPropToValueChecked(p ast.DesignPropertyEntryV3, themeProps []ThemeProperty) (pages.DesignPropertyValue, bool, error) {
	if len(p.Nested) == 0 && p.Value != "" && isMultiSelectDesignProperty(p.Key, themeProps) {
		return pages.DesignPropertyValue{}, false, mdlerrors.NewValidation(fmt.Sprintf(
			"design property %q takes a SET of options, not one value — write it as "+
				"`'%s': ['%s': on]` (add one `'<option>': on` per selection). "+
				"A single value is stored as an Option and mxbuild refuses it with CE6084.",
			p.Key, p.Key, p.Value))
	}
	dp, ok := astDesignPropToValueInner(p, themeProps)
	return dp, ok, nil
}

// isMultiSelectDesignProperty reports whether the theme declares this key as
// multi-select. Unknown keys answer false: with no metadata there is nothing to
// refuse on the strength of, and a theme newer than the snapshot must not be
// blocked.
func isMultiSelectDesignProperty(key string, themeProps []ThemeProperty) bool {
	for i := range themeProps {
		if strings.EqualFold(themeProps[i].Name, key) {
			return themeProps[i].MultiSelect
		}
	}
	return false
}

func astDesignPropToValueInner(p ast.DesignPropertyEntryV3, themeProps []ThemeProperty) (pages.DesignPropertyValue, bool) {
	if len(p.Nested) > 0 {
		dp := pages.DesignPropertyValue{Key: p.Key, ValueType: "compound"}
		for _, sub := range p.Nested {
			// The inner function: a sub-entry is `'Phone': on`, which is a toggle
			// by construction and never itself multi-select.
			if sv, ok := astDesignPropToValueInner(sub, themeProps); ok {
				dp.Compound = append(dp.Compound, sv)
			}
		}
		return dp, true
	}
	switch strings.ToLower(p.Value) {
	case "on":
		return pages.DesignPropertyValue{Key: p.Key, ValueType: "toggle"}, true
	case "off":
		return pages.DesignPropertyValue{}, false // toggle absence — skip
	default:
		return pages.DesignPropertyValue{
			Key:       p.Key,
			ValueType: resolveDesignPropertyValueType(p.Key, p.Value, themeProps),
			Option:    p.Value,
		}, true
	}
}

// resolveDesignPropertyValueType determines the BSON ValueType for a flat design
// property value from the theme definition.
//
// The control type alone does NOT decide the value type: a value that is one of
// the property's declared options is always stored as an "option", whether the
// control is a Dropdown, a ToggleButtonGroup, or a ColorPicker's predefined
// swatches. Studio Pro rejects a mismatch (CE6084 "Expected design property … to
// be of type Toggle button group, but found Custom"), so a ToggleButtonGroup
// selection like 'Column gap': 'Medium' must serialize as an option, not custom.
//
// Only a value that is NOT a declared option, on a ColorPicker, is a free-form
// value (a custom hex/color) stored as "custom". Any other off-list value stays
// "option" (an invalid value is reported separately by MDL-WIDGET12). Falls back
// to "option" when no theme metadata is available (backward compatible).
func resolveDesignPropertyValueType(key, value string, themeProps []ThemeProperty) string {
	for _, tp := range themeProps {
		if tp.Name != key {
			continue
		}
		if themeOptionAllowed(tp.Options, value) {
			return "option"
		}
		if tp.Type == "ColorPicker" {
			return "custom"
		}
		return "option"
	}
	// No theme info available — default to "option".
	return "option"
}

// =============================================================================
// V3 DataSource and Action Builders
// =============================================================================

// checkSearchByIsOnAListView refuses `search by` on a widget that cannot store it.
//
// The clause hangs off the shared database-source rule, so the grammar accepts it
// on a gallery or a data grid too — and only Forms$ListViewXPathSource declares
// Search, so listViewSourceToGen is the only writer that emits it. Everything
// else accepted the clause and dropped it: check passed, exec reported success,
// and DESCRIBE did not echo it back. That is the silent-drop this whole area
// keeps producing, and the reason MDL-WIDGET07 exists (ako/mxcli#512).
//
// An error rather than a warning: unlike an unrecognised PROPERTY key, which a
// newer widget package might legitimately define, this one is decided by Mendix's
// metamodel and cannot become valid later.
func checkSearchByIsOnAListView(w *ast.WidgetV3) error {
	if w == nil || strings.EqualFold(w.Type, "listview") {
		return nil
	}
	ds := w.GetDataSource()
	if ds == nil || len(ds.SearchAttributes) == 0 {
		return nil
	}
	return mdlerrors.NewValidation(fmt.Sprintf(
		"widget %q (%s): `search by` is a LIST VIEW search bar and %s cannot store one — "+
			"only Forms$ListViewXPathSource declares Search. Drop the clause, or use a `listview`.",
		w.Name, strings.ToLower(w.Type), strings.ToLower(w.Type)))
}

// buildDataSourceV3 converts a V3 DataSource AST to a pages.DataSource.
// Returns the datasource, the entity name for context, and any error.
func (pb *pageBuilder) buildDataSourceV3(ds *ast.DataSourceV3) (pages.DataSource, string, error) {
	switch ds.Type {
	case "parameter":
		// Parameter reference: $ParamName
		// Page parameters store names WITHOUT $ prefix (e.g., "Customer")
		// Snippet parameters store names WITH $ prefix (e.g., "$Customer")
		// Try both variants for compatibility
		paramName := strings.TrimPrefix(ds.Reference, "$")
		entityID, ok := pb.paramScope[paramName]
		entityName := pb.paramEntityNames[paramName]
		if !ok {
			// Try with $ prefix (for snippets)
			entityID, ok = pb.paramScope["$"+paramName]
			entityName = pb.paramEntityNames["$"+paramName]
		}
		if !ok {
			return nil, "", mdlerrors.NewNotFound("parameter", ds.Reference)
		}

		// Fallback to lookup if entity name not stored
		if entityName == "" {
			var err error
			entityName, err = pb.getEntityNameByID(entityID)
			if err != nil {
				log.Printf("warning: could not resolve entity name for ID %s: %v", entityID, err)
			}
		}

		// Use DataViewSource with IsSnippetParameter flag
		return &pages.DataViewSource{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$DataViewSource",
			},
			EntityID:           entityID,
			EntityName:         entityName,
			ParameterName:      paramName,
			IsSnippetParameter: pb.isSnippet,
		}, entityName, nil

	case "database":
		// Database source: DATABASE Entity
		entityID, err := pb.resolveEntity(ast.QualifiedName{
			Module: pb.extractModule(ds.Reference),
			Name:   pb.extractName(ds.Reference),
		})
		if err != nil {
			return nil, "", mdlerrors.NewBackend("resolve entity", err)
		}

		dbSource := &pages.DatabaseSource{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$DatabaseSource", // Note: actual BSON $Type depends on widget context (grid/listview/dataview)
			},
			EntityID:   entityID,
			EntityName: ds.Reference,
		}

		// Handle WHERE clause. Expand association-only paths to the Assoc/Entity/Assoc
		// form Mendix requires (see expandXPathAssociationPath), so the shorthand
		// `[Mod.Assoc1/Mod.Assoc2 = $x]` doesn't trip CE1613 at build time.
		// Formatting comes last, after the expansion above has settled the text:
		// a constraint too long to read on one line is broken at its boolean
		// joints (upstream #979). One that already fits is returned unchanged, so
		// this does not churn existing pages.
		if ds.Where != "" {
			dbSource.XPathConstraint = visitor.FormatXPathConstraint(
				pb.expandXPathAssociationPath(ds.Where, ds.Reference))
		}

		// Handle ORDER BY
		for _, ob := range ds.OrderBy {
			direction := pages.SortDirectionAscending
			if strings.ToLower(ob.Direction) == "desc" {
				direction = pages.SortDirectionDescending
			}
			attrPath := pb.resolveAttributePathForEntity(ob.Attribute, ds.Reference)
			var steps []pages.AttributeRefStep
			if len(ob.Associations) > 0 {
				// A sort that navigates associations. Resolved through the same
				// walker DataGrid2 columns and dynamictext params use, so the two
				// cannot disagree about a path that means the same thing in both.
				// Refused rather than flattened: an attribute of a far entity with
				// no EntityRef beside it is CE7247 at build time
				// (mendixlabs/mxcli#1152).
				path := strings.Join(append(append([]string{}, ob.Associations...), ob.Attribute), "/")
				finalQN, hops, ok := pb.resolveAssociationAttributePathForEntity(path, ds.Reference)
				if !ok {
					return nil, "", mdlerrors.NewValidation(fmt.Sprintf(
						"sort by %s: the association path could not be resolved from %s",
						path, ds.Reference))
				}
				attrPath, steps = finalQN, hops
			}
			sortItem := &pages.GridSort{
				BaseElement: model.BaseElement{
					ID:       model.ID(types.GenerateID()),
					TypeName: "Forms$GridSort",
				},
				AttributePath:     attrPath,
				AttributeRefSteps: steps,
				Direction:         direction,
			}
			dbSource.Sorting = append(dbSource.Sorting, sortItem)
		}

		// Handle SEARCH BY — the List View search bar's attributes. Resolved to
		// the same fully-qualified Module.Entity.Attribute form a sort column
		// uses, because both are stored as a DomainModels$AttributeRef and a
		// bare name in one would be a bare name in the other (ako/mxcli#512).
		for _, attr := range ds.SearchAttributes {
			dbSource.SearchAttributes = append(dbSource.SearchAttributes,
				pb.resolveAttributePathForEntity(attr, ds.Reference))
		}

		return dbSource, ds.Reference, nil

	case "microflow":
		// Microflow source
		mfID, err := pb.resolveMicroflow(ds.Reference)
		if err != nil {
			return nil, "", mdlerrors.NewBackend("resolve microflow", err)
		}

		// Get entity name from microflow's return type for context resolution
		entityName := pb.getMicroflowReturnEntityName(ds.Reference)

		return &pages.MicroflowSource{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$MicroflowSource",
			},
			MicroflowID:       mfID,
			Microflow:         ds.Reference,
			ParameterMappings: pb.flowArgsToParameterMappings(ds.Args),
		}, entityName, nil

	case "nanoflow":
		// Nanoflow source - resolve by listing all nanoflows
		nfID, err := pb.resolveNanoflowByName(ds.Reference)
		if err != nil {
			return nil, "", mdlerrors.NewBackend("resolve nanoflow", err)
		}

		// Get entity name from nanoflow's return type for context resolution
		entityName := pb.getNanoflowReturnEntityName(ds.Reference)

		return &pages.NanoflowSource{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$NanoflowSource",
			},
			NanoflowID:        nfID,
			Nanoflow:          ds.Reference,
			ParameterMappings: pb.flowArgsToParameterMappings(ds.Args),
		}, entityName, nil

	case "association":
		// Association path source — emits Forms$AssociationSource BSON.
		// ds.Reference is either "Module.Assoc" (single-segment) or
		// "Module.Assoc/Module.DestEntity" (multi-segment, dest explicit).
		// For single-segment, resolve DestinationEntity from the association
		// definition (the side opposite to the parent context entity).
		ctxVar := ds.ContextVariable
		if ctxVar == "currentObject" {
			ctxVar = "" // implicit context — no SourceVariable in BSON
		}

		// Which entity the association is traversed FROM.
		//
		// pb.entityContext is the ENCLOSING data container's entity, which is the
		// right answer for `$currentObject/Assoc` inside a data view and the
		// wrong one at page level: there is no enclosing container, so it is
		// empty, resolveAssociationDestination matched neither end, and its
		// last-resort fallback returned the association's TO side — the entity
		// the grid was navigating AWAY from. The rows were then typed as the
		// context entity and every column bound against it:
		//
		//	datagrid gA (datasource: $Customer/Bench.Order_Customer) { … OrderNo … }
		//	mx check -> [CE1613] "The selected attribute 'Bench.Customer.OrderNo'
		//	            no longer exists." at Columns (1/1) of data grid 2 'gA'
		//
		// The same path inside a data view was correct, which is what made the
		// report's diagnosis land on page level specifically
		// (mendixlabs/mxcli#1045).
		//
		// A NAMED context variable answers the question directly: `$Customer/…`
		// traverses from whatever $Customer holds, whether or not anything
		// encloses the widget. Falling back to pb.entityContext keeps the
		// data-view case exactly as it was.
		fromEntity := pb.entityContext
		if ds.ContextVariable != "" && ds.ContextVariable != "currentObject" {
			name := strings.TrimPrefix(ds.ContextVariable, "$")
			if qn := pb.paramEntityNames[name]; qn != "" {
				fromEntity = qn
			} else if qn := pb.paramEntityNames["$"+name]; qn != "" {
				fromEntity = qn
			}
		}

		path := ds.Reference
		destEntity := ""
		if idx := strings.Index(path, "/"); idx >= 0 {
			destEntity = path[idx+1:]
			path = path[:idx]
		}

		// Both halves of an EntityRefStep are BY_NAME references, and Mendix
		// resolves either one it cannot find to null — so the association half
		// needs the same care as the destination. A bare name (the spelling
		// attribute paths accept, and the one the guard below suggests) reached
		// BSON unqualified and the loader threw ArgumentNullException at
		// `EntityRefStep.set_AssociationId`: same unopenable project as an empty
		// DestinationEntity, a different property. Qualify with the context
		// entity's module, exactly as attribute-path hops do (upstream #854).
		path = pb.resolveAssociationPathIn(path, fromEntity)

		if destEntity == "" {
			destEntity = pb.resolveAssociationDestination(path, fromEntity)
		} else if _, _, ok := pb.associationEndpoints(path); !ok {
			// An author-supplied destination satisfies the guard below, so it is
			// the one path where a misspelled — or wrongly-moduled — association
			// would sail through and be written qualified-but-nonexistent, which
			// Mendix resolves to null just the same. Verify it exists.
			return nil, "", mdlerrors.NewValidationf(
				"association %q for datasource %q does not exist — "+
					"writing it would produce a project Mendix cannot open; "+
					"a bare name is qualified with the module of the context entity (%s), "+
					"so an association declared elsewhere must be named in full",
				path, ds.Reference, fromEntity)
		}

		// An empty DestinationEntity is a by-name reference Mendix resolves to
		// null: the loader throws ArgumentNullException setting DestinationEntityId
		// and the whole project becomes unopenable — Studio Pro refuses it and
		// `mx check` dies before validating anything. Refuse instead of writing a
		// structurally invalid unit; the author can name the destination explicitly
		// as `Assoc/Module.Entity`. (issuetracker #14)
		if destEntity == "" {
			return nil, "", mdlerrors.NewValidationf(
				"cannot resolve the destination entity of association %q for datasource %q — "+
					"writing it unresolved would produce a project Mendix cannot open; "+
					"name the destination explicitly, e.g. `%s/Module.Entity`",
				path, ds.Reference, path)
		}

		// Return destEntity as the child context so column bindings inside the
		// widget can resolve short attribute names against it.
		return &pages.AssociationSource{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$AssociationSource",
			},
			EntityPath:      path + "/" + destEntity,
			ContextVariable: ctxVar,
		}, destEntity, nil

	case "selection":
		// Selection from another widget
		widgetName := ds.Reference
		widgetID, ok := pb.widgetScope[widgetName]
		if !ok {
			return nil, "", mdlerrors.NewNotFound("widget", widgetName)
		}

		// Get the entity context from the source widget if available
		entityName := pb.paramEntityNames[widgetName]

		return &pages.ListenToWidgetSource{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$ListenTargetSource",
			},
			WidgetID:   widgetID,
			WidgetName: widgetName, // Widget name for BSON serialization
		}, entityName, nil

	default:
		return nil, "", mdlerrors.NewUnsupported("unsupported datasource type: " + ds.Type)
	}
}

// resolveAssociationDestination looks up an association by qualified name and returns
// the qualified name of the entity OPPOSITE to contextEntity. Returns "" if the
// association can't be resolved or the context isn't on either end.
//
// Convention (per CLAUDE.md): ParentID = FROM entity, ChildID = TO entity.
// For `Module.OrderLine_Order` (`FROM OrderLine TO Order`), context=Order → dest=OrderLine (parent side).
// xpathAssocRunRe matches a run of two or more slash-joined qualified names
// (Module.Name/Module.Name[/...]) — an association/entity path. Single qualified
// names (one-hop `[Mod.Assoc = $x]`) and unqualified attribute steps (`Mod.Assoc/Attr`)
// don't match, so they're never rewritten.
var xpathAssocRunRe = regexp.MustCompile(`[A-Za-z_]\w*\.[A-Za-z_]\w*(?:/[A-Za-z_]\w*\.[A-Za-z_]\w*)+`)

// expandXPathAssociationPath rewrites an XPath constraint so that consecutive
// association segments get the intermediate entity Mendix requires between them:
// `[Mod.Assoc1/Mod.Assoc2 = $x]` → `[Mod.Assoc1/Mod.Entity/Mod.Assoc2 = $x]`. Without
// this, mxbuild reads the second association as an entity and fails with CE1613
// ("selected entity ... no longer exists").
//
// The rewrite is safe by construction: it only touches runs of slash-joined qualified
// names anchored at an association from the datasource entity, and inserts an entity
// only between two segments that both resolve as associations. The already-correct
// full form (where the middle segment is an entity, not an association), single-hop
// constraints, attribute paths, and anything unresolvable are returned verbatim.
func (pb *pageBuilder) expandXPathAssociationPath(constraint, contextEntity string) string {
	if contextEntity == "" || !strings.Contains(constraint, "/") {
		return constraint
	}
	return xpathAssocRunRe.ReplaceAllStringFunc(constraint, func(run string) string {
		segs := strings.Split(run, "/")
		// Only expand a run that starts with an association reachable from the
		// datasource entity; otherwise leave it untouched (it may be an unrelated
		// qualified path we don't understand).
		if pb.resolveAssociationDestination(segs[0], contextEntity) == "" {
			return run
		}
		out := make([]string, 0, len(segs)*2)
		ctx := contextEntity
		for i, seg := range segs {
			dest := pb.resolveAssociationDestination(seg, ctx)
			out = append(out, seg)
			if dest == "" {
				// Not an association from here — treat as an entity segment.
				ctx = seg
				continue
			}
			// Association: if the next segment is also an association (i.e. the
			// intermediate entity was omitted), insert the destination entity.
			if i+1 < len(segs) {
				next := segs[i+1]
				if next != dest && pb.resolveAssociationDestination(next, dest) != "" {
					out = append(out, dest)
				}
			}
			ctx = dest
		}
		return strings.Join(out, "/")
	})
}

// isSpecializationOf reports whether entityQN is ancestorQN or inherits from it,
// walking the generalization chain over the domain models already loaded — the
// same list the association ends are resolved against, so the two cannot
// disagree about what an entity is.
func (pb *pageBuilder) isSpecializationOf(entityQN, ancestorQN string) bool {
	if entityQN == "" || ancestorQN == "" {
		return false
	}
	seen := map[string]bool{}
	for current := entityQN; current != ""; {
		if seen[current] {
			return false // a cycle a corrupt model could contain
		}
		seen[current] = true
		if current == ancestorQN {
			return true
		}
		current = pb.generalizationOf(current)
	}
	return false
}

// generalizationOf returns an entity's EXTENDS target, or "" when the entity is
// not in the loaded domain models or has no generalization.
func (pb *pageBuilder) generalizationOf(entityQN string) string {
	parts := strings.SplitN(entityQN, ".", 2)
	if len(parts) != 2 {
		return ""
	}
	domainModels, err := pb.getDomainModels()
	if err != nil {
		return ""
	}
	for _, dm := range domainModels {
		if pb.moduleNameByID(dm.ContainerID) != parts[0] {
			continue
		}
		for _, e := range dm.Entities {
			if e.Name == parts[1] {
				return e.GeneralizationRef
			}
		}
	}
	return ""
}

func (pb *pageBuilder) resolveAssociationDestination(assocQN, contextEntity string) string {
	if assocQN == "" {
		return ""
	}
	parts := strings.SplitN(assocQN, ".", 2)
	if len(parts) != 2 {
		return ""
	}
	modName, assocName := parts[0], parts[1]

	domainModels, err := pb.getDomainModels()
	if err != nil {
		return ""
	}
	for _, dm := range domainModels {
		// Module-scope the search: only look at the domain model whose module name
		// matches the first segment of the qualified association name. Association
		// names are not unique across the project (e.g., both AssocGrid and ODataSvc
		// can have an "OrderLine_Order" association) — without this check, we'd
		// pick the wrong one.
		if pb.moduleNameByID(dm.ContainerID) != modName {
			continue
		}
		// A cross-module association (target in another module, including
		// `System`) lives in a separate list where the remote end is the BY_NAME
		// ChildRef rather than a BY_ID pointer — see associationEndpoints
		// (issuetracker #19). Resolving it here means the empty-end fallbacks
		// below are a last resort, not the normal path for these.
		for _, ca := range dm.CrossAssociations {
			if ca.Name != assocName {
				continue
			}
			parentEntity := pb.entityQNByID(ca.ParentID)
			// A specialization of the remote end navigates it in reverse just as
			// the end itself does (#975).
			if contextEntity != "" && (contextEntity == ca.ChildRef || pb.isSpecializationOf(contextEntity, ca.ChildRef)) {
				return parentEntity
			}
			return ca.ChildRef
		}
		for _, a := range dm.Associations {
			if a.Name != assocName {
				continue
			}
			// Resolve entity qualified names for ParentID and ChildID.
			parentEntity := pb.entityQNByID(a.ParentID)
			childEntity := pb.entityQNByID(a.ChildID)
			// The "destination" is the end OPPOSITE to the context.
			if contextEntity != "" {
				if contextEntity == childEntity {
					return parentEntity
				}
				if contextEntity == parentEntity {
					return childEntity
				}
				// Neither end matched BY NAME, but the context may be a
				// SPECIALIZATION of one: a page bound to `Sub` navigating an
				// association declared on `Base` is ordinary, and Mendix resolves
				// it through the generalization. Without this the walk fell
				// through to the guess at the bottom and typed the rows as the
				// wrong end — silently, and only mxbuild disagreed (#975).
				//
				// Only when BOTH ends resolved: when one is empty the fallbacks
				// below are load-bearing (issuetracker #14), and a destination of
				// "" makes the .mpr unloadable.
				if childEntity != "" && parentEntity != "" {
					if pb.isSpecializationOf(contextEntity, childEntity) {
						return parentEntity
					}
					if pb.isSpecializationOf(contextEntity, parentEntity) {
						return childEntity
					}
				}
			}
			// One end may be unresolvable: entityQNByID only sees the project's
			// own domain models, so an association ending in a System entity
			// (e.g. `from W.Issue to System.Workflow`) yields "" for that side.
			// The context then matches neither end and the old code returned the
			// empty child — an empty DestinationEntity is a by-name reference
			// Mendix resolves to null, which makes the whole .mpr UNLOADABLE
			// (issuetracker #14). Prefer whichever end actually resolved and is
			// not the context.
			if childEntity == "" && parentEntity != "" && parentEntity != contextEntity {
				return parentEntity
			}
			if parentEntity == "" && childEntity != "" && childEntity != contextEntity {
				return childEntity
			}
			// No context or mismatch — default to the child (TO) side, which
			// matches the common FROM=context pattern.
			return childEntity
		}
	}
	return ""
}

// entityQNByID returns the qualified name (Module.Entity) for a given entity ID
// by scanning all domain models. Returns "" if not found.
func (pb *pageBuilder) entityQNByID(entityID model.ID) string {
	if entityID == "" {
		return ""
	}
	domainModels, err := pb.getDomainModels()
	if err != nil {
		return ""
	}
	for _, dm := range domainModels {
		for _, e := range dm.Entities {
			if e.ID == entityID {
				// Look up module name via the domain model's container
				modName := pb.moduleNameByID(dm.ContainerID)
				if modName == "" {
					return e.Name
				}
				return modName + "." + e.Name
			}
		}
	}
	return ""
}

// moduleNameByID returns the module name for a given module ID. Cached via hierarchy.
func (pb *pageBuilder) moduleNameByID(moduleID model.ID) string {
	if moduleID == "" {
		return ""
	}
	// The hierarchy already indexes module names and is the source the sibling
	// resolvers (associationEndpoints, entityGeneralizations) read, so consult it
	// first — same answer, one less backend round trip.
	if h, err := pb.getHierarchy(); err == nil {
		if name := h.GetModuleName(moduleID); name != "" {
			return name
		}
	}
	modules, err := pb.backend.ListModules()
	if err != nil {
		return ""
	}
	for _, m := range modules {
		if m.ID == moduleID {
			return m.Name
		}
	}
	return ""
}

// getMicroflowReturnEntityName looks up a microflow and returns its return type entity name.
// Returns empty string if the microflow doesn't return an entity or list of entities.
func (pb *pageBuilder) getMicroflowReturnEntityName(qualifiedName string) string {
	// First, check if the microflow was created during this session (not yet in backend cache)
	if pb.execCache != nil && pb.execCache.createdMicroflows != nil {
		if info, ok := pb.execCache.createdMicroflows[qualifiedName]; ok {
			return info.ReturnEntityName
		}
	}

	// Parse qualified name
	parts := strings.Split(qualifiedName, ".")
	if len(parts) < 2 {
		return ""
	}
	moduleName := parts[0]
	mfName := strings.Join(parts[1:], ".")

	// Get microflows from backend
	mfs, err := pb.getMicroflows()
	if err != nil {
		return ""
	}

	// Use hierarchy to resolve module names (handles microflows in folders)
	h, err := pb.getHierarchy()
	if err != nil {
		return ""
	}

	// Find matching microflow
	for _, mf := range mfs {
		modID := h.FindModuleID(mf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == moduleName && mf.Name == mfName {
			// Extract entity name from return type
			return extractEntityFromReturnType(mf.ReturnType)
		}
	}

	return ""
}

// extractEntityFromReturnType extracts the entity qualified name from a DataType.
func extractEntityFromReturnType(dt microflows.DataType) string {
	if dt == nil {
		return ""
	}

	switch t := dt.(type) {
	case *microflows.ObjectType:
		return t.EntityQualifiedName
	case *microflows.ListType:
		return t.EntityQualifiedName
	default:
		return ""
	}
}

// getNanoflowReturnEntityName looks up a nanoflow and returns its return type entity name.
// Returns empty string if the nanoflow doesn't return an entity or list of entities.
func (pb *pageBuilder) getNanoflowReturnEntityName(qualifiedName string) string {
	parts := strings.Split(qualifiedName, ".")
	var moduleName, name string
	if len(parts) >= 2 {
		moduleName = parts[0]
		name = parts[1]
	} else {
		moduleName = pb.moduleName
		name = qualifiedName
	}

	nanoflows, err := pb.backend.ListNanoflows()
	if err != nil {
		return ""
	}

	h, err := pb.getHierarchy()
	if err != nil {
		return ""
	}

	for _, nf := range nanoflows {
		modID := h.FindModuleID(nf.ContainerID)
		modName := ""
		for _, m := range pb.getModules() {
			if m.ID == modID {
				modName = m.Name
				break
			}
		}
		if modName == moduleName && nf.Name == name {
			return extractEntityFromReturnType(nf.ReturnType)
		}
	}

	return ""
}

// buildClientActionV3 converts a V3 Action AST to a pages.ClientAction.
func (pb *pageBuilder) buildClientActionV3(action *ast.ActionV3) (pages.ClientAction, error) {
	switch action.Type {
	case "none":
		// `Action: NOTHING` — deliberately inert. The same Forms$NoAction the
		// default branch of serializeClientAction has always produced for this
		// spelling; what is new is that it arrives as an action rather than as a
		// string the grammar failed to parse. Without this case the promotion in
		// actionExprV3 would turn a documented, working spelling into
		// "unsupported action type" at exec (mendixlabs/mxcli#1062).
		return &pages.NoClientAction{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$NoAction",
			},
		}, nil

	case "save":
		return &pages.SaveChangesClientAction{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$SaveChangesClientAction",
			},
			ClosePage: action.ClosePage,
		}, nil

	case "cancel":
		return &pages.CancelChangesClientAction{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$CancelChangesClientAction",
			},
			ClosePage: action.ClosePage,
		}, nil

	case "close":
		return &pages.ClosePageClientAction{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$ClosePageClientAction",
			},
		}, nil

	case "delete":
		return &pages.DeleteClientAction{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$DeleteClientAction",
			},
		}, nil

	case "create":
		entityID, err := pb.resolveEntity(ast.QualifiedName{
			Module: pb.extractModule(action.Target),
			Name:   pb.extractName(action.Target),
		})
		if err != nil {
			return nil, mdlerrors.NewBackend("resolve entity for create", err)
		}

		createAct := &pages.CreateObjectClientAction{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$CreateObjectClientAction",
			},
			EntityID:   entityID,
			EntityName: action.Target,
		}

		// Handle THEN action (show page)
		if action.ThenAction != nil && action.ThenAction.Type == "showPage" {
			pageID, err := pb.resolvePageRef(action.ThenAction.Target)
			if err != nil {
				return nil, mdlerrors.NewBackend("resolve page", err)
			}
			createAct.PageID = pageID
			createAct.PageName = action.ThenAction.Target
		}

		return createAct, nil

	case "showPage":
		_, err := pb.resolvePageRef(action.Target)
		if err != nil {
			return nil, mdlerrors.NewBackend("resolve page", err)
		}

		pageAction := &pages.PageClientAction{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$PageClientAction",
			},
			PageName: action.Target,
		}

		// Build parameter mappings from Args
		for _, arg := range action.Args {
			// mxcli stores this action with an empty ParameterMappings array and
			// lets Mendix infer the argument from the enclosing widget's context
			// object — required, because an explicit mapping is rejected as CE0115
			// (#296). An argument naming anything else therefore cannot be honoured,
			// and was previously dropped in silence: the button opened the page with
			// the context object, `mx check` reported 0 errors, and DESCRIBE printed
			// the inferred mapping. Outside any data widget there is no context
			// object to infer at all, so every argument is dropped and the build
			// fails CE1571 (#1029). Refuse instead of re-pointing the argument.
			if strVal, ok := arg.Value.(string); ok && !pb.argCtx.binds(strVal) {
				return nil, mdlerrors.NewValidation(
					refuseShowPageArgument(pb.currentWidget, action.Target, arg.Name, strVal, pb.argCtx) +
						" [MDL-PAGEARG01]")
			}

			mapping := &pages.PageClientParameterMapping{
				BaseElement: model.BaseElement{
					ID:       model.ID(types.GenerateID()),
					TypeName: "Forms$PageParameterMapping",
				},
				ParameterName: arg.Name,
			}

			// Determine if value is a variable reference or expression
			if strVal, ok := arg.Value.(string); ok {
				if strings.HasPrefix(strVal, "$") {
					// Variable reference (including $currentObject)
					mapping.Variable = strVal
				} else {
					mapping.Expression = strVal
				}
			}

			pageAction.ParameterMappings = append(pageAction.ParameterMappings, mapping)
		}

		return pageAction, nil

	case "microflow":
		mfID, err := pb.resolveMicroflow(action.Target)
		if err != nil {
			return nil, mdlerrors.NewBackend("resolve microflow", err)
		}

		mfAction := &pages.MicroflowClientAction{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$MicroflowAction",
			},
			MicroflowID:   mfID,
			MicroflowName: action.Target,
		}

		// Build parameter mappings from Args
		for _, arg := range action.Args {
			mapping := &pages.MicroflowParameterMapping{
				BaseElement: model.BaseElement{
					ID:       model.ID(types.GenerateID()),
					TypeName: "Forms$MicroflowParameterMapping",
				},
				ParameterName: arg.Name,
			}

			// A page/snippet parameter or page variable binds through
			// Variable (a Forms$PageVariable); anything else is an
			// Expression. See classifyFlowArgValue — writing a $-reference
			// as an Expression leaves the parameter unbound (CE1571, #1140).
			if strVal, ok := arg.Value.(string); ok {
				if v, kind := pb.classifyFlowArgValue(strVal); kind != "" {
					mapping.Variable, mapping.VariableKind = v, kind
				} else if strings.HasPrefix(strVal, "$") {
					mapping.Variable = strVal
				} else {
					mapping.Expression = strVal
				}
			}

			mfAction.ParameterMappings = append(mfAction.ParameterMappings, mapping)
		}

		return mfAction, nil

	case "nanoflow":
		nfID, err := pb.resolveNanoflowByName(action.Target)
		if err != nil {
			return nil, mdlerrors.NewBackend("resolve nanoflow", err)
		}

		nfAction := &pages.NanoflowClientAction{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$NanoflowAction",
			},
			NanoflowID:   nfID,
			NanoflowName: action.Target,
		}

		// Build parameter mappings from Args
		for _, arg := range action.Args {
			mapping := &pages.NanoflowParameterMapping{
				BaseElement: model.BaseElement{
					ID:       model.ID(types.GenerateID()),
					TypeName: "Forms$NanoflowParameterMapping",
				},
				ParameterName: arg.Name,
			}

			// A page/snippet parameter or page variable binds through
			// Variable (a Forms$PageVariable); anything else is an
			// Expression. See classifyFlowArgValue — writing a $-reference
			// as an Expression leaves the parameter unbound (CE1571, #1140).
			if strVal, ok := arg.Value.(string); ok {
				if v, kind := pb.classifyFlowArgValue(strVal); kind != "" {
					mapping.Variable, mapping.VariableKind = v, kind
				} else if strings.HasPrefix(strVal, "$") {
					mapping.Variable = strVal
				} else {
					mapping.Expression = strVal
				}
			}

			nfAction.ParameterMappings = append(nfAction.ParameterMappings, mapping)
		}

		return nfAction, nil

	case "openLink":
		return &pages.LinkClientAction{
			BaseElement: model.BaseElement{
				ID: model.ID(types.GenerateID()),
				// Mendix stores this as Forms$OpenLinkClientAction — the
				// storage name differs from the SDK type name, the split
				// CLAUDE.md documents. The wrong value here never reached disk
				// only because neither engine could write the action at all.
				TypeName: "Forms$OpenLinkClientAction",
			},
			LinkType: pages.LinkTypeWeb,
			Address:  action.LinkURL,
		}, nil

	case "signOut":
		return &pages.SignOutClientAction{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$SignOutClientAction",
			},
		}, nil

	case "completeTask":
		return &pages.SetTaskOutcomeClientAction{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$SetTaskOutcomeClientAction",
			},
			ClosePage:    true,
			Commit:       true,
			OutcomeValue: action.OutcomeValue,
		}, nil

	default:
		return nil, mdlerrors.NewUnsupported("unsupported action type: " + action.Type)
	}
}

// =============================================================================
// Helper functions
// =============================================================================

func (pb *pageBuilder) extractModule(qualifiedName string) string {
	qualifiedName = unquoteQualifiedName(qualifiedName)
	parts := strings.Split(qualifiedName, ".")
	if len(parts) >= 2 {
		return parts[0]
	}
	return pb.moduleName
}

func (pb *pageBuilder) extractName(qualifiedName string) string {
	qualifiedName = unquoteQualifiedName(qualifiedName)
	parts := strings.Split(qualifiedName, ".")
	if len(parts) >= 2 {
		return parts[1]
	}
	return qualifiedName
}

func (pb *pageBuilder) getEntityNameByID(entityID model.ID) (string, error) {
	domainModels, err := pb.getDomainModels()
	if err != nil {
		return "", err
	}

	modules := pb.getModules()
	moduleNames := make(map[model.ID]string)
	for _, m := range modules {
		moduleNames[m.ID] = m.Name
	}

	for _, dm := range domainModels {
		for _, e := range dm.Entities {
			if e.ID == entityID {
				moduleName := moduleNames[dm.ContainerID]
				return moduleName + "." + e.Name, nil
			}
		}
	}
	return "", mdlerrors.NewNotFound("entity", string(entityID))
}

// pageParamBSONType maps a DataType to the BSON $Type string for a primitive
// page or snippet parameter. Returns empty string for entity/enum types (which
// use DataTypes$ObjectType instead), which is the signal the callers branch on.
//
// Long maps to DataTypes$IntegerType because storage has no LongType: neither
// generated/metamodel (the 11.6.0 arbiter) nor modelsdk/gen declares one, and
// Studio Pro's own parameter type is the single "Integer/Long". This used to
// return "DataTypes$LongType", a $Type Mendix does not have — the CLAUDE.md
// "never invent a key" case, which on the way to disk was quietly rescued into
// a String by pageParamTypeToGen's default arm. constant_write.go has carried
// the same note ("storage has no LongType") all along.
func pageParamBSONType(dt ast.DataType) string {
	switch dt.Kind {
	case ast.TypeString:
		return "DataTypes$StringType"
	case ast.TypeInteger, ast.TypeLong:
		return "DataTypes$IntegerType"
	case ast.TypeDecimal:
		return "DataTypes$DecimalType"
	case ast.TypeBoolean:
		return "DataTypes$BooleanType"
	case ast.TypeDateTime:
		return "DataTypes$DateTimeType"
	default:
		return ""
	}
}

// resolveNanoflowByName resolves a nanoflow qualified name to its ID.
func (pb *pageBuilder) resolveNanoflowByName(nfName string) (model.ID, error) {
	parts := strings.Split(nfName, ".")
	var moduleName, name string
	if len(parts) >= 2 {
		moduleName = parts[0]
		name = parts[1]
	} else {
		moduleName = pb.moduleName
		name = nfName
	}

	nanoflows, err := pb.backend.ListNanoflows()
	if err != nil {
		return "", mdlerrors.NewBackend("list nanoflows", err)
	}

	h, err := pb.getHierarchy()
	if err != nil {
		return "", err
	}

	for _, nf := range nanoflows {
		modID := h.FindModuleID(nf.ContainerID)
		modName := ""
		for _, m := range pb.getModules() {
			if m.ID == modID {
				modName = m.Name
				break
			}
		}
		if modName == moduleName && nf.Name == name {
			return nf.ID, nil
		}
	}

	return "", mdlerrors.NewNotFound("nanoflow", nfName)
}

// mdlTypeToBsonType converts an MDL type name to a BSON DataTypes$* type string.
func mdlTypeToBsonType(mdlType string) string {
	switch strings.ToLower(mdlType) {
	case "boolean":
		return "DataTypes$BooleanType"
	case "string":
		return "DataTypes$StringType"
	case "integer":
		return "DataTypes$IntegerType"
	case "long":
		return "DataTypes$LongType"
	case "decimal":
		return "DataTypes$DecimalType"
	case "datetime", "date":
		return "DataTypes$DateTimeType"
	default:
		// Could be an entity type - use ObjectType
		return "DataTypes$ObjectType"
	}
}

// pageVariableEnumRe matches an MDL enumeration type, `Enumeration(Module.Name)`,
// with `Enum` accepted as the grammar's short spelling.
var pageVariableEnumRe = regexp.MustCompile(`(?i)^enum(?:eration)?\s*\(\s*([^)\s]+)\s*\)$`)

// pageVariableType maps a page variable's MDL type to the BSON $Type Mendix
// stores, plus the enumeration it points at when there is one.
//
// The enumeration's qualified name is in the AST already — the visitor keeps the
// data type's raw source text — so `Enumeration(Mod.Status)` arrives complete.
// Before this it fell through mdlTypeToBsonType's default to ObjectType and was
// then flattened to a StringType by the writer, losing the type twice over and
// saying nothing either time (upstream #977).
//
// An enumeration with no qualified name in the parentheses is NOT reported as
// one: an EnumerationType with nothing to resolve is a by-name reference Mendix
// reads as null, which is worse than the old fallback.
func pageVariableType(mdlType string) (bsonType, enumerationQN string) {
	if m := pageVariableEnumRe.FindStringSubmatch(strings.TrimSpace(mdlType)); m != nil && m[1] != "" {
		return "DataTypes$EnumerationType", m[1]
	}
	return mdlTypeToBsonType(mdlType), ""
}

// pageVariableMDLType is the inverse, for DESCRIBE.
func pageVariableMDLType(bsonType, enumerationQN string) string {
	if bsonType == "DataTypes$EnumerationType" {
		if enumerationQN == "" {
			// The reference did not survive; `Enumeration()` would not re-parse.
			return "Unknown"
		}
		return "Enumeration(" + enumerationQN + ")"
	}
	return bsonTypeToMDLType(bsonType)
}

// bsonTypeToMDLType converts a BSON DataTypes$* type to an MDL type name.
func bsonTypeToMDLType(bsonType string) string {
	switch bsonType {
	case "DataTypes$BooleanType":
		return "Boolean"
	case "DataTypes$StringType":
		return "String"
	case "DataTypes$IntegerType":
		return "Integer"
	case "DataTypes$LongType":
		return "Long"
	case "DataTypes$DecimalType":
		return "Decimal"
	case "DataTypes$DateTimeType":
		return "DateTime"
	case "DataTypes$ObjectType":
		return "Object"
	default:
		return "Unknown"
	}
}

func (pb *pageBuilder) resolveAttributePathForEntity(attrName string, entityName string) string {
	// Save and restore entity context
	oldContext := pb.entityContext
	pb.entityContext = entityName
	defer func() { pb.entityContext = oldContext }()

	return pb.resolveAttributePath(attrName)
}

// resolveAssociationAttributePathForEntity resolves an `Assoc/.../Attr` path
// against an explicit root entity rather than the builder's current widget
// context — a datasource's sort is rooted in the datasource's own entity.
// Mirrors resolveAttributePathForEntity.
func (pb *pageBuilder) resolveAssociationAttributePathForEntity(path, entityName string) (string, []pages.AttributeRefStep, bool) {
	oldContext := pb.entityContext
	pb.entityContext = entityName
	defer func() { pb.entityContext = oldContext }()

	return pb.resolveAssociationAttributePath(path)
}

// resolveTemplateAttributePath resolves template parameter values like $widgetName.Attribute
// to fully qualified entity paths like Module.Entity.Attribute.
// It handles patterns like:
// - $widgetName.Attribute -> looks up widget's entity and returns Entity.Attribute
// - simple Attribute -> uses current entity context
// - Module.Entity.Attribute -> returns as-is
func (pb *pageBuilder) resolveTemplateAttributePath(attrRef string) string {
	if attrRef == "" {
		return ""
	}

	// Check for $widgetName.Attribute pattern
	if after, ok := strings.CutPrefix(attrRef, "$"); ok {
		// Parse $widgetName.Attribute
		withoutDollar := after
		parts := strings.SplitN(withoutDollar, ".", 2)
		if len(parts) == 2 {
			widgetName := parts[0]
			attrName := parts[1]

			// Look up the widget's entity context from paramEntityNames
			// The widget name should match a parameter or widget scope entry
			if entityName, ok := pb.paramEntityNames[widgetName]; ok {
				return entityName + "." + attrName
			}
			// Try with $ prefix (for snippet parameters)
			if entityName, ok := pb.paramEntityNames["$"+widgetName]; ok {
				return entityName + "." + attrName
			}
			// Use current entity context as fallback
			if pb.entityContext != "" {
				return pb.entityContext + "." + attrName
			}
			// Return as-is if we can't resolve
			return attrRef
		}
	}

	// For other patterns, use regular attribute path resolution
	return pb.resolveAttributePath(attrRef)
}

// resolveTemplateAttributePathFull resolves a template parameter reference and sets
// both AttributeRef and SourceVariable on the parameter. This preserves the page
// parameter context so that DESCRIBE can output $Product.Name instead of Entity.Name.
//
// When attrRef is $paramName.Attribute (where paramName is a page/snippet parameter),
// it sets SourceVariable to paramName and AttributeRef to the resolved entity path.
//
// Non-String attributes (Integer, Decimal, DateTime, Boolean, …) bind as a
// structured AttributeRef, not a `toString(...)` Expression — the runtime renders
// them through the parameter's FormattingInfo (decimalPrecision, dateFormat, …),
// which an Expression parameter bypasses entirely (ledger #76).
func (pb *pageBuilder) resolveTemplateAttributePathFull(attrRef string, param *pages.ClientTemplateParameter) {
	if attrRef == "" {
		return
	}

	// Bare $localVar reference (no .attribute suffix) for a page-level local
	// variable: emit as Forms$PageVariable.LocalVariable so Studio Pro doesn't
	// interpret the literal "$localVar" as an entity attribute path.
	if after, ok := strings.CutPrefix(attrRef, "$"); ok && !strings.Contains(after, ".") {
		if pb.localVariables[after] {
			param.SourceVariable = after
			param.SourceVariableKind = "local"
			return
		}
	}

	// Check for $paramName.Attribute pattern where paramName is a page parameter
	if after, ok := strings.CutPrefix(attrRef, "$"); ok {
		withoutDollar := after
		parts := strings.SplitN(withoutDollar, ".", 2)
		if len(parts) == 2 {
			paramName := parts[0]
			attrName := parts[1]

			// Check if this is a page/snippet parameter (not a widget reference)
			if entityName, ok := pb.paramEntityNames[paramName]; ok {
				fullPath := entityName + "." + attrName
				if pb.isNonStringAttribute(fullPath) {
					param.Expression = "toString($" + paramName + "/" + attrName + ")"
					return
				}
				param.SourceVariable = paramName
				param.AttributeRef = fullPath
				return
			}
			// Try with $ prefix (for snippet parameters)
			if entityName, ok := pb.paramEntityNames["$"+paramName]; ok {
				fullPath := entityName + "." + attrName
				if pb.isNonStringAttribute(fullPath) {
					param.Expression = "toString($" + paramName + "/" + attrName + ")"
					return
				}
				param.SourceVariable = paramName
				param.AttributeRef = fullPath
				return
			}
		}
	}

	// Attribute navigated over one or more associations (e.g.
	// Order_Customer/Name or $currentObject/Sales.Order_Customer/Name). Mendix
	// stores this as an AttributeRef whose EntityRef is an IndirectEntityRef of
	// association steps — a flat "Assoc/Attr" string binds nothing (CE "No value
	// specified"). Resolve the hops against the domain model.
	if pb.resolveTemplateAssociationPath(attrRef, param) {
		return
	}

	// For other patterns, resolve to a structured AttributeRef. A non-String
	// attribute (Decimal/DateTime/Integer/…) binds directly as an AttributeRef —
	// the runtime renders it via the parameter's FormattingInfo, exactly as Studio
	// Pro does. (Previously mxcli wrapped non-String attrs in a
	// `toString($currentObject/Attr)` Expression, which bypassed FormattingInfo so
	// decimalPrecision/dateFormat had no runtime effect — ledger #76.)
	param.AttributeRef = pb.resolveTemplateAttributePath(attrRef)
}

// resolveTemplateAssociationPath resolves a template-parameter value that
// navigates one or more associations (e.g. "Order_Customer/Name" or
// "$currentObject/Sales.Order_Customer/Name") against the current entity
// context, populating param.AttributeRef (the fully-qualified FINAL attribute)
// and param.AttributeRefSteps (one hop per association). Returns false when the
// value is not an association path or cannot be resolved, so the caller falls
// back to the previous behavior.
func (pb *pageBuilder) resolveTemplateAssociationPath(attrRef string, param *pages.ClientTemplateParameter) bool {
	finalQN, steps, ok := pb.resolveAssociationAttributePath(attrRef)
	if !ok {
		return false
	}
	param.AttributeRef = finalQN
	param.AttributeRefSteps = steps
	return true
}

// resolveInputAttribute resolves the `attribute:` of an input widget (text box,
// text area, date picker, drop-down, check box, radio buttons) into the
// qualified final attribute plus the association hops to reach it.
//
// A bare name resolves against the enclosing entity context as before and
// carries no steps. `Assoc/Attr` navigates: Studio Pro stores exactly this on a
// plain text box, as ako/TestApp's Rules.RuleAction_NewEdit does for
// Rules.BusinessRule.Name over Rules.RuleAction_BusinessRule.
//
// Before this, every input builder called resolveAttributePath, which knows
// nothing about associations — the slashes survived into a flat path that
// resolved to nothing and the build failed CE1613 (ako/mxcli#529). DataGrid2
// columns and DynamicText parameters already resolved it, so one page could
// bind an associated attribute in a grid column and fail on the text box beside
// it.
//
// An unresolvable path falls back to resolveAttributePath rather than erroring,
// matching what the column builder does: the reference checker
// (--references) is where an unknown member is reported, and failing here would
// reject paths whose entity context this pass cannot see.
func (pb *pageBuilder) resolveInputAttribute(attr string) (string, []pages.AttributeRefStep) {
	if finalQN, steps, ok := pb.resolveAssociationAttributePath(attr); ok {
		return finalQN, steps
	}
	return pb.resolveAttributePath(attr), nil
}

// resolveAssociationAttributePath resolves a context-relative attribute path that
// navigates one or more associations (e.g. "Order_Customer/Name" or
// "$currentObject/Sales.Order_Customer/Name") into the fully-qualified FINAL
// attribute and the association hops (one per `/` segment). Returns ok=false when
// the value is not an association path or cannot be resolved, so callers fall back
// to their own-attribute handling. Shared by DynamicText template parameters and
// DataGrid2 columns — both store the binding as a DomainModels$AttributeRef whose
// EntityRef is an IndirectEntityRef of these steps.
func (pb *pageBuilder) resolveAssociationAttributePath(attrRef string) (finalQN string, steps []pages.AttributeRefStep, ok bool) {
	path := strings.TrimPrefix(attrRef, "$currentObject/")
	// Only context-relative association paths are handled here; a $param- or
	// $widget-rooted navigation is a different (unsupported) shape.
	if strings.HasPrefix(path, "$") || !strings.Contains(path, "/") {
		return "", nil, false
	}
	segs := strings.Split(path, "/")
	if len(segs) < 2 {
		return "", nil, false
	}
	attrName := segs[len(segs)-1]
	current := pb.entityContext
	if current == "" {
		return "", nil, false
	}

	steps = make([]pages.AttributeRefStep, 0, len(segs)-1)
	for _, seg := range segs[:len(segs)-1] {
		assocQN := pb.resolveAssociationPath(seg)
		dest, ok := pb.associationDestination(assocQN, current)
		if !ok {
			return "", nil, false
		}
		steps = append(steps, pages.AttributeRefStep{Association: assocQN, DestinationEntity: dest})
		current = dest
	}

	// The final attribute is qualified with the entity that DECLARES it, which
	// for an inherited attribute is an ancestor of the association's destination
	// — same rule (and same CE1613 when broken) as a direct binding.
	stored := storedSystemMemberName(attrName)
	if declaring, ok := pb.declaringEntityFor(current, stored); ok {
		return declaring + "." + stored, steps, true
	}
	return current + "." + stored, steps, true
}

// associationDestination returns the entity reached by navigating assocQN from
// currentEntityQN. Uses the FROM/TO endpoints (ParentID = FROM, ChildID = TO);
// forward navigation from the FROM entity yields the TO entity and vice versa.
//
// The context may be a **specialization** of an endpoint: associations are often
// declared on a base entity while the widget is bound to a subclass (e.g. an
// association on Expense, a grid over SpecialExpense extends Expense). Match the
// endpoint the context *is or descends from* — an exact-equality check would
// drop the binding and MxBuild would fail CE0402 "No value specified" (Bug 3).
func (pb *pageBuilder) associationDestination(assocQN, currentEntityQN string) (string, bool) {
	from, to, ok := pb.associationEndpoints(assocQN)
	if !ok {
		return "", false
	}
	switch {
	case pb.entityIsOrDescendsFrom(currentEntityQN, from):
		return to, true
	case pb.entityIsOrDescendsFrom(currentEntityQN, to):
		return from, true
	default:
		// Context is related to neither endpoint — can't pick a direction reliably;
		// refuse rather than emit a wrong ref.
		return "", false
	}
}

// checkListViewTemplateSpecialization reports why a `template for X` cannot
// belong to a list view over listEntity, or nil when it can.
//
// One function for both call sites — CREATE PAGE (buildListViewTemplateV3) and
// ALTER PAGE INSERT/REPLACE (cmd_alter_page.go) — because two copies of a guard
// is how the two drift, and this one was already wrong in both.
//
// The rule is a STRICT specialization, and that strictness is ako/mxcli#514.
// Both copies gated on entityIsOrDescendsFrom, which returns true for the entity
// itself, so `template for <the list view's own entity>` was accepted, written,
// and refused by mxbuild:
//
//	[CE0543] "The entity of the list view template is 'MyFirstModule.Vehicle' and
//	         this is not a specialization of the entity of the list view."
//
// Measured on 11.12.2; a template for a real specialization is 0 errors. The
// list view's own body already renders an object no template matches, so a
// template for the base entity would be a second, unreachable default.
//
// An empty listEntity means the datasource did not resolve to an entity, which
// is reported elsewhere — do not report it a second time as a bogus
// specialization error.
func (pb *pageBuilder) checkListViewTemplateSpecialization(spec, listEntity, listViewName string) error {
	if listEntity == "" || spec == "" {
		return nil
	}
	if spec == listEntity {
		return mdlerrors.NewValidation(fmt.Sprintf(
			"template for %s in list view %s: %s is the list view's own entity, and a template "+
				"must be for a specialization of it — the list view's own body already renders "+
				"objects no template matches",
			spec, listViewName, spec))
	}
	if !pb.entityIsOrDescendsFrom(spec, listEntity) {
		return mdlerrors.NewValidation(fmt.Sprintf(
			"template for %s in list view %s: %s is not a specialization of %s, "+
				"so the template can never match an object the list view shows",
			spec, listViewName, spec, listEntity))
	}
	return nil
}

// entityIsOrDescendsFrom reports whether entityQN equals baseQN or is a
// specialization of it (following the generalization chain transitively). Used
// so an association declared on a base entity resolves from a subclass context.
func (pb *pageBuilder) entityIsOrDescendsFrom(entityQN, baseQN string) bool {
	if entityQN == "" || baseQN == "" {
		return false
	}
	if entityQN == baseQN {
		return true
	}
	parents, err := pb.entityGeneralizations()
	if err != nil {
		return false
	}
	seen := map[string]bool{}
	for cur := entityQN; cur != "" && !seen[cur]; cur = parents[cur] {
		seen[cur] = true
		if parents[cur] == baseQN {
			return true
		}
	}
	return false
}

// entityGeneralizations builds a map of entity qualified name → its direct
// parent (generalization) qualified name, across all domain models.
func (pb *pageBuilder) entityGeneralizations() (map[string]string, error) {
	dms, err := pb.getDomainModels()
	if err != nil {
		return nil, err
	}
	h, err := pb.getHierarchy()
	if err != nil {
		return nil, err
	}
	m := make(map[string]string)
	for _, dm := range dms {
		mod := h.GetModuleName(dm.ContainerID)
		for _, e := range dm.Entities {
			if e.GeneralizationRef != "" {
				m[mod+"."+e.Name] = e.GeneralizationRef
			}
		}
	}
	return m, nil
}

// associationEndpoints resolves a qualified association name to its FROM
// (ParentID) and TO (ChildID) entity qualified names.
//
// A domain model keeps associations in **two** lists. `Associations` holds the
// intra-module ones, where both ends are BY_ID. An association whose target is
// in another module — including the platform's own `System` module — is a
// `DomainModels$CrossAssociation` and lives in `CrossAssociations`, where only
// the local (FROM) end is BY_ID and the remote end is the BY_NAME `ChildRef`.
// Searching only the first list left every cross-module hop unresolvable, so a
// widget bound to `Issue_Assignee/Name` fell back to a flat attribute path and
// the build failed CE1613 "The selected attribute … no longer exists"
// (issuetracker #19).
func (pb *pageBuilder) associationEndpoints(assocQN string) (fromEntity, toEntity string, ok bool) {
	parts := strings.SplitN(assocQN, ".", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	modName, assocName := parts[0], parts[1]

	dms, err := pb.getDomainModels()
	if err != nil {
		return "", "", false
	}
	h, err := pb.getHierarchy()
	if err != nil {
		return "", "", false
	}

	// Index every entity ID → qualified name (association endpoints are BY_ID).
	entityQN := make(map[model.ID]string)
	for _, dm := range dms {
		mod := h.GetModuleName(dm.ContainerID)
		for _, e := range dm.Entities {
			entityQN[e.ID] = mod + "." + e.Name
		}
	}

	for _, dm := range dms {
		if h.GetModuleName(dm.ContainerID) != modName {
			continue
		}
		if a := dm.FindAssociationByName(assocName); a != nil {
			from, to := entityQN[a.ParentID], entityQN[a.ChildID]
			if from == "" || to == "" {
				return "", "", false
			}
			return from, to, true
		}
		// Cross-module: the remote end is already a qualified name.
		for _, ca := range dm.CrossAssociations {
			if ca.Name != assocName {
				continue
			}
			from := entityQN[ca.ParentID]
			if from == "" || ca.ChildRef == "" {
				return "", "", false
			}
			return from, ca.ChildRef, true
		}
	}
	return "", "", false
}

// isNonStringAttribute checks if an attribute path refers to a non-String type.
// Returns false if the type can't be determined (fail-open to preserve existing behavior).
func (pb *pageBuilder) isNonStringAttribute(attrPath string) bool {
	attrType := pb.findAttributeType(attrPath)
	if attrType == nil {
		return false // can't determine type, assume String
	}
	_, isString := attrType.(*domainmodel.StringAttributeType)
	return !isString
}

// ============================================================================
// Fragment Expansion
// ============================================================================

// expandFragments processes a widget list, expanding any USE_FRAGMENT /
// USE_BUILDING_BLOCK sentinels into their referenced widgets. It recurses into
// every widget's children, so a fragment or building block nested inside a
// container/layout/dataview (not just at the page-body top level) is expanded
// too. Non-sentinel widgets pass through with their (expanded) children.
func (pb *pageBuilder) expandFragments(widgets []*ast.WidgetV3) ([]*ast.WidgetV3, error) {
	var result []*ast.WidgetV3
	for _, w := range widgets {
		expanded, err := pb.expandIfFragment(w)
		if err != nil {
			return nil, err
		}
		for _, e := range expanded {
			if len(e.Children) > 0 {
				kids, err := pb.expandFragments(e.Children)
				if err != nil {
					return nil, err
				}
				e.Children = kids
			}
			result = append(result, e)
		}
	}
	return result, nil
}

// expandIfFragment returns the widget as-is if it's not a USE_FRAGMENT or
// USE_BUILDING_BLOCK sentinel, or expands it into cloned/copied widgets with an
// optional prefix.
func (pb *pageBuilder) expandIfFragment(w *ast.WidgetV3) ([]*ast.WidgetV3, error) {
	switch w.Type {
	case "USE_FRAGMENT":
		return pb.expandFragmentRef(w)
	case "USE_BUILDING_BLOCK":
		return pb.expandBuildingBlockRef(w)
	case "SLOT":
		// A slot marker is only meaningful inside a `define fragment` body, where
		// it is resolved during expandFragmentRef. Reaching here means a bare
		// `slot` was written directly in a page/snippet body.
		return nil, mdlerrors.NewValidation("`slot` is only valid inside a `define fragment` body")
	default:
		return []*ast.WidgetV3{w}, nil
	}
}

// expandFragmentRef expands a USE_FRAGMENT sentinel into cloned fragment widgets
// with an optional prefix rename.
func (pb *pageBuilder) expandFragmentRef(w *ast.WidgetV3) ([]*ast.WidgetV3, error) {
	if pb.fragments == nil {
		return nil, mdlerrors.NewNotFound("fragment", w.Name)
	}
	frag, ok := pb.fragments[w.Name]
	if !ok {
		return nil, mdlerrors.NewNotFound("fragment", w.Name)
	}

	widgets := cloneWidgets(frag.Widgets)
	if prefix, ok := w.Properties["Prefix"].(string); ok && prefix != "" {
		prefixWidgetNames(widgets, prefix)
	}

	// Resolve declared datasource/action parameters from the supplied args, then
	// substitute each `$param` reference in the cloned tree with the caller value.
	if err := substituteFragmentParams(w.Name, frag.Params, w.Properties["Args"], widgets); err != nil {
		return nil, err
	}

	// Splice any content-slot payload into the fragment's slot marker. The payload
	// (`use fragment X { … }`) rides on the USE_FRAGMENT sentinel's Children.
	payload := w.Children
	slotCount := countSlots(widgets)
	switch {
	case slotCount == 0 && len(payload) > 0:
		return nil, mdlerrors.NewValidation(fmt.Sprintf(
			"fragment %q does not declare a `slot`, but content was supplied via `use fragment %s { … }`",
			w.Name, w.Name))
	case slotCount == 0:
		return widgets, nil
	case slotCount > 1:
		return nil, mdlerrors.NewValidation(fmt.Sprintf(
			"fragment %q declares %d slots; a content-slot fragment supports a single slot",
			w.Name, slotCount))
	}
	// Expand nested fragment / building-block refs in the payload before splicing,
	// so a payload can itself compose other fragments.
	expandedPayload, err := pb.expandFragments(payload)
	if err != nil {
		return nil, err
	}
	widgets = replaceSlots(widgets, expandedPayload)
	return widgets, nil
}

// countSlots returns the number of SLOT sentinels anywhere in the tree.
func countSlots(widgets []*ast.WidgetV3) int {
	n := 0
	for _, w := range widgets {
		if w.Type == "SLOT" {
			n++
		}
		n += countSlots(w.Children)
	}
	return n
}

// replaceSlots returns a new widget list with each SLOT sentinel replaced by a
// fresh clone of the payload, recursing into children. Callers guarantee a single
// slot (v1), so the per-slot clone never duplicates caller-named widgets.
func replaceSlots(widgets []*ast.WidgetV3, payload []*ast.WidgetV3) []*ast.WidgetV3 {
	var out []*ast.WidgetV3
	for _, w := range widgets {
		if w.Type == "SLOT" {
			out = append(out, cloneWidgets(payload)...)
			continue
		}
		w.Children = replaceSlots(w.Children, payload)
		out = append(out, w)
	}
	return out
}

// substituteFragmentParams resolves a fragment's declared datasource/action
// parameters against the supplied args and rewrites every `$param` reference in
// the cloned widget tree. It errors on a missing arg, an unknown arg, or a
// type mismatch. A no-param fragment with no args is a no-op.
func substituteFragmentParams(fragName string, params []ast.FragmentParam, rawArgs any, widgets []*ast.WidgetV3) error {
	args, _ := rawArgs.([]ast.FragmentArg)
	if len(params) == 0 {
		if len(args) > 0 {
			return mdlerrors.NewValidation(fmt.Sprintf(
				"fragment %q declares no parameters, but %d argument(s) were supplied", fragName, len(args)))
		}
		return nil
	}

	// Index args by name and validate against the declared parameter set.
	argByName := make(map[string]ast.FragmentArg, len(args))
	for _, a := range args {
		argByName[a.Name] = a
	}
	declared := make(map[string]bool, len(params))
	for _, p := range params {
		declared[p.Name] = true
	}
	for name := range argByName {
		if !declared[name] {
			return mdlerrors.NewValidation(fmt.Sprintf(
				"fragment %q: unknown argument $%s (not a declared parameter)", fragName, name))
		}
	}

	dsSubst := map[string]*ast.DataSourceV3{}
	actSubst := map[string]*ast.ActionV3{}
	for _, p := range params {
		arg, ok := argByName[p.Name]
		if !ok {
			return mdlerrors.NewValidation(fmt.Sprintf(
				"fragment %q: missing argument for parameter $%s (%s)", fragName, p.Name, p.Kind))
		}
		switch p.Kind {
		case "datasource":
			if arg.DataSource == nil {
				return mdlerrors.NewValidation(fmt.Sprintf(
					"fragment %q: parameter $%s expects a datasource", fragName, p.Name))
			}
			dsSubst[p.Name] = arg.DataSource
		case "action":
			act := arg.Action
			if act == nil && arg.DataSource != nil {
				// The value parsed as a datasource (microflow/nanoflow overlap).
				// Reinterpret those two kinds as a call action.
				if ds := arg.DataSource; ds.Type == "microflow" || ds.Type == "nanoflow" {
					act = &ast.ActionV3{Type: ds.Type, Target: ds.Reference, Args: ds.Args}
				}
			}
			if act == nil {
				return mdlerrors.NewValidation(fmt.Sprintf(
					"fragment %q: parameter $%s expects an action (e.g. a microflow, show_page, save)", fragName, p.Name))
			}
			actSubst[p.Name] = act
		}
	}

	substituteParamRefs(widgets, dsSubst, actSubst)
	return nil
}

// substituteParamRefs rewrites `$param` datasource/action references in the tree.
func substituteParamRefs(widgets []*ast.WidgetV3, ds map[string]*ast.DataSourceV3, act map[string]*ast.ActionV3) {
	for _, w := range widgets {
		if cur, ok := w.Properties["DataSource"].(*ast.DataSourceV3); ok && cur != nil &&
			cur.Type == "parameter" {
			// A parameter datasource keeps the leading '$' in Reference; param
			// names are stored without it.
			if repl, hit := ds[strings.TrimPrefix(cur.Reference, "$")]; hit {
				w.Properties["DataSource"] = repl
			}
		}
		if cur, ok := w.Properties["Action"].(*ast.ActionV3); ok && cur != nil {
			w.Properties["Action"] = substituteActionParam(cur, act)
		}
		substituteParamRefs(w.Children, ds, act)
	}
}

// substituteActionParam replaces a `$param` action (and any nested THEN action)
// with the caller-supplied action.
func substituteActionParam(a *ast.ActionV3, act map[string]*ast.ActionV3) *ast.ActionV3 {
	if a == nil {
		return nil
	}
	if a.Type == "param" {
		if repl, ok := act[a.Target]; ok {
			return repl
		}
	}
	if a.ThenAction != nil {
		a.ThenAction = substituteActionParam(a.ThenAction, act)
	}
	return a
}

// expandBuildingBlockRef deep-copies a building block's widget tree into the
// page/container. The DESCRIBE renderer already emits faithful, re-parseable MDL
// for a block's widgets, so expansion = render the block's widgets to MDL text →
// re-parse them via a `define fragment` wrapper → apply the optional prefix. This
// reuses the whole existing widget parser instead of a hand-written BSON→AST
// converter.
func (pb *pageBuilder) expandBuildingBlockRef(w *ast.WidgetV3) ([]*ast.WidgetV3, error) {
	if pb.ctx == nil {
		return nil, mdlerrors.NewNotFound("building block", w.Name)
	}
	ctx := pb.ctx

	// Resolve the block by module + name.
	qn := parseQualifiedNameStr(w.Name)
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil, mdlerrors.NewBackend("build hierarchy", err)
	}
	blocks, err := ctx.Backend.ListBuildingBlocks()
	if err != nil {
		return nil, mdlerrors.NewBackend("list building blocks", err)
	}
	var found *pages.BuildingBlock
	for _, bb := range blocks {
		modID := h.FindModuleID(bb.ContainerID)
		modName := h.GetModuleName(modID)
		if bb.Name == qn.Name && (qn.Module == "" || modName == qn.Module) {
			found = bb
			break
		}
	}
	if found == nil {
		return nil, mdlerrors.NewNotFound("building block", w.Name)
	}

	// Read the block's widgets as raw BSON.
	rawWidgets := getBuildingBlockWidgetsFromRaw(ctx, found.ID)
	if len(rawWidgets) == 0 {
		return nil, nil
	}

	// Render to MDL text. outputWidgetMDLV3 writes to ctx.Output; redirect it to a
	// buffer via a shallow ExecContext copy so we can capture the rendered MDL.
	var sb strings.Builder
	renderCtx := *ctx
	renderCtx.Output = &sb
	for _, rw := range rawWidgets {
		outputWidgetMDLV3(&renderCtx, rw, 1)
	}

	// Re-parse via a `define fragment` wrapper to obtain []*ast.WidgetV3.
	src := "define fragment __bbtmp as {\n" + sb.String() + "\n};"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		return nil, fmt.Errorf("use building block %s: could not expand widget tree: %v", w.Name, errs)
	}
	if len(prog.Statements) == 0 {
		return nil, fmt.Errorf("use building block %s: could not expand widget tree", w.Name)
	}
	def, ok := prog.Statements[0].(*ast.DefineFragmentStmt)
	if !ok {
		return nil, fmt.Errorf("use building block %s: could not expand widget tree", w.Name)
	}
	widgets := def.Widgets

	// Apply the optional prefix rename.
	if prefix, ok := w.Properties["Prefix"].(string); ok && prefix != "" {
		prefixWidgetNames(widgets, prefix)
	}

	// Apply optional rebind overrides. Binding-point rule (prototype): the
	// override rewrites the FIRST widget in pre-order that CAN carry a datasource
	// / an action — the block's outermost datasource and primary action.
	//
	// Can, not does. A reusable block is a template: its datasource is unbound
	// and its buttons have no action, so neither property is present in the
	// rendered MDL. The action override already matched by widget type for that
	// reason; the datasource override matched on an existing DataSource property
	// and only worked because an unbound datasource used to render as the
	// malformed `DataSource: database from ,` — the very output #941 fixed.
	if ds, ok := w.Properties["DataSourceOverride"].(*ast.DataSourceV3); ok && ds != nil {
		// Datasource target: the first widget that already carries a datasource
		// (a block's outermost list/grid/dataview always emits one).
		hit := rebindFirst(widgets, isDataSourceWidget,
			func(t *ast.WidgetV3) { t.Properties["DataSource"] = ds })
		if !hit {
			return nil, mdlerrors.NewValidation(fmt.Sprintf(
				"use building block %s: datasource override supplied, but the block has no datasource widget to rebind", w.Name))
		}
	}
	if act, ok := w.Properties["ActionOverride"].(*ast.ActionV3); ok && act != nil {
		// Action target: the first button-type widget. Atlas blocks ship
		// placeholder buttons with no action, so match by widget type (not by an
		// existing Action property) and set the handler.
		hit := rebindFirst(widgets, isButtonWidget,
			func(t *ast.WidgetV3) { t.Properties["Action"] = act })
		if !hit {
			return nil, mdlerrors.NewValidation(fmt.Sprintf(
				"use building block %s: action override supplied, but the block has no button to rebind", w.Name))
		}
	}
	return widgets, nil
}

// isButtonWidget reports whether a widget is an action-capable button.
// isDataSourceWidget reports whether a widget is one that takes a datasource,
// whether or not it currently carries one. The list is the data containers a
// building block can be built around; anything else in a block is layout or a
// leaf.
func isDataSourceWidget(w *ast.WidgetV3) bool {
	switch strings.ToLower(w.Type) {
	case "gallery", "listview", "datagrid", "datagrid2", "dataview", "templategrid", "referenceselector":
		return true
	}
	return false
}

func isButtonWidget(w *ast.WidgetV3) bool {
	switch strings.ToLower(w.Type) {
	case "actionbutton", "linkbutton", "button":
		return true
	}
	return false
}

// rebindFirst applies set to the first widget (pre-order) matching pred,
// returning whether a target was found.
func rebindFirst(widgets []*ast.WidgetV3, pred func(*ast.WidgetV3) bool, set func(*ast.WidgetV3)) bool {
	for _, w := range widgets {
		if pred(w) {
			set(w)
			return true
		}
		if rebindFirst(w.Children, pred, set) {
			return true
		}
	}
	return false
}

// cloneWidgets deep-copies a widget tree to avoid mutating the fragment definition.
func cloneWidgets(widgets []*ast.WidgetV3) []*ast.WidgetV3 {
	if widgets == nil {
		return nil
	}
	result := make([]*ast.WidgetV3, len(widgets))
	for i, w := range widgets {
		result[i] = cloneWidget(w)
	}
	return result
}

func cloneWidget(w *ast.WidgetV3) *ast.WidgetV3 {
	clone := &ast.WidgetV3{
		Type:       w.Type,
		Name:       w.Name,
		Properties: make(map[string]interface{}, len(w.Properties)),
		Children:   cloneWidgets(w.Children),
	}
	for k, v := range w.Properties {
		clone.Properties[k] = v // Property values are immutable (strings, ints, etc.)
	}
	return clone
}

// prefixWidgetNames recursively prepends a prefix to all widget names.
func prefixWidgetNames(widgets []*ast.WidgetV3, prefix string) {
	for _, w := range widgets {
		if w.Name != "" {
			w.Name = prefix + w.Name
		}
		prefixWidgetNames(w.Children, prefix)
	}
}

// flowArgsToParameterMappings converts parsed datasource/action arguments into
// model parameter mappings. A microflow or nanoflow used as a widget datasource
// needs an argument for every parameter exactly as a call action does — Mendix
// reports CE1571 "No argument has been selected for parameter 'X'" otherwise
// (#835). The datasource path previously parsed the arguments and dropped them.
func (pb *pageBuilder) flowArgsToParameterMappings(args []ast.FlowArgV3) []*pages.MicroflowParameterMapping {
	var out []*pages.MicroflowParameterMapping
	for _, arg := range args {
		mapping := &pages.MicroflowParameterMapping{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$MicroflowParameterMapping",
			},
			ParameterName: arg.Name,
		}
		// A page/snippet parameter or page variable binds through Variable (a
		// Forms$PageVariable); $currentObject and anything else stays an
		// expression. See classifyFlowArgValue (#1140).
		if strVal, ok := arg.Value.(string); ok {
			if v, kind := pb.classifyFlowArgValue(strVal); kind != "" {
				mapping.Variable, mapping.VariableKind = v, kind
			} else if strings.HasPrefix(strVal, "$") {
				mapping.Variable = strVal
			} else {
				mapping.Expression = strVal
			}
		}
		out = append(out, mapping)
	}
	return out
}

// buildScrollContainerV3 builds a scroll container from its five named regions.
//
// `region top { … }` rather than a bare `top { … }`: every other widget in MDL
// is `<type> <name> (props) { body }`, and a region is the one place where the
// "name" is a fixed position rather than free text. Keeping the shape means one
// keyword instead of five and no special case in the widget grammar.
func (pb *pageBuilder) buildScrollContainerV3(w *ast.WidgetV3) (pages.Widget, error) {
	sc := &pages.ScrollContainer{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$ScrollContainer",
			},
			Name: w.Name,
		},
	}

	seen := map[pages.ScrollContainerSlot]bool{}
	for _, child := range w.Children {
		if strings.ToLower(child.Type) != "region" {
			return nil, mdlerrors.NewValidation(fmt.Sprintf(
				"scrollcontainer %q: %q is not a region; a scroll container's children are its regions", w.Name, child.Type))
		}
		slot := pages.ScrollContainerSlot(strings.ToLower(child.Name))
		switch slot {
		case pages.ScrollSlotTop, pages.ScrollSlotRight, pages.ScrollSlotBottom,
			pages.ScrollSlotLeft, pages.ScrollSlotCenter:
		default:
			return nil, mdlerrors.NewValidation(fmt.Sprintf(
				"scrollcontainer %q: unknown region %q (want top, right, bottom, left or center)", w.Name, child.Name))
		}
		// Two regions in one slot is a script that means two different things
		// and gets one; the second would silently win.
		if seen[slot] {
			return nil, mdlerrors.NewValidation(fmt.Sprintf(
				"scrollcontainer %q: region %q given twice", w.Name, slot))
		}
		seen[slot] = true

		region := &pages.ScrollContainerRegion{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID()), TypeName: "Forms$ScrollContainerRegion"},
			Slot:        slot,
			Class:       child.GetStringProp("Class"),
			SizeMode:    child.GetStringProp("SizeMode"),
			Size:        child.GetIntProp("Size"),
		}
		for _, gc := range child.Children {
			cw, err := pb.buildWidgetV3(gc)
			if err != nil {
				return nil, err
			}
			region.Widgets = append(region.Widgets, cw)
		}
		sc.Regions = append(sc.Regions, region)
	}
	return sc, nil
}

// buildNavigationTreeV3 builds the sidebar menu. The profile is stored inside a
// Forms$NavigationSource, not on the tree — see widget_write.go.
func (pb *pageBuilder) buildNavigationTreeV3(w *ast.WidgetV3) (pages.Widget, error) {
	menu, err := pb.menuSourceDocument(w)
	if err != nil {
		return nil, err
	}
	nt := &pages.NavigationTree{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$NavigationTree",
			},
			Name: w.Name,
		},
		NavigationProfile: w.GetStringProp("Profile"),
		MenuDocument:      menu,
	}
	return nt, nil
}

// menuSourceDocument reads `Menu: Module.Name`, the menu document a navigation
// tree or menu bar draws its items from instead of a profile. It used to be
// ignored: the widget was stored with the Responsive profile, and a copy of
// Atlas_Core.Tablet_Sidebar showed the desktop menu (mendixlabs/mxcli#1189).
// The menu must exist when the layout is written: a dangling by-name reference
// passes mx check and draws an empty menu.
func (pb *pageBuilder) menuSourceDocument(w *ast.WidgetV3) (string, error) {
	menu := strings.TrimSpace(w.GetStringProp("Menu"))
	if menu == "" {
		return "", nil
	}
	if w.GetStringProp("Profile") != "" {
		return "", mdlerrors.NewValidationf(
			"%s %s: Menu and Profile are two sources for the same items -- give one", strings.ToLower(w.Type), w.Name)
	}
	qn := parseQualifiedNameStr(menu)
	if qn.Module == "" {
		return "", mdlerrors.NewValidationf("%s %s: Menu needs a qualified name, Module.Menu -- got %q",
			strings.ToLower(w.Type), w.Name, menu)
	}
	if pb.backend != nil {
		if _, err := pb.backend.GetMenuDocumentByQualifiedName(qn.Module, qn.Name); err != nil {
			return "", mdlerrors.NewValidationf("%s %s: menu not found: %s (create it with `create menu`)",
				strings.ToLower(w.Type), w.Name, qn.String())
		}
	}
	return qn.String(), nil
}

// buildPlaceholderV3 declares a slot a page can bind to.
//
// The name is the API: a page references it as Module.Layout.<Name>, so it is
// required here rather than defaulted.
func (pb *pageBuilder) buildPlaceholderV3(w *ast.WidgetV3) (pages.Widget, error) {
	if w.Name == "" {
		return nil, mdlerrors.NewValidation("placeholder needs a name: pages bind to it as Module.Layout.<Name>")
	}
	ph := &pages.LayoutPlaceholder{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$Placeholder",
			},
			Name: w.Name,
		},
	}
	return ph, nil
}

// buildMenuBarV3 builds the horizontal navigation a topbar carries. Same shape
// as a navigation tree — see widget_write.go.
func (pb *pageBuilder) buildMenuBarV3(w *ast.WidgetV3) (pages.Widget, error) {
	menu, err := pb.menuSourceDocument(w)
	if err != nil {
		return nil, err
	}
	return &pages.MenuBar{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$MenuBar",
			},
			Name: w.Name,
		},
		NavigationProfile: w.GetStringProp("Profile"),
		MenuDocument:      menu,
	}, nil
}

// missingWidgetMessage explains why a widget has no definition, and — the part
// that matters — names a remedy that can actually work.
//
// The old message always said "run 'mxcli widget init -p app.mpr'". When the
// widget's package is not in the project at all — File Uploader, Events, Google
// Tag and Markdown viewer are in no blank project, measured on 11.13 — that is
// worse than unhelpful: `widget init` scans `widgets/`, the .mpk is not there,
// and re-running it can never help. Reported as the postscript to
// mendixlabs/mxcli#1036, where it cost the reporter a debugging session.
//
// The remedy is to install the widget, which is also the only way to use it in
// Studio Pro. Measured: a blank 11.13 project ships 33 widgets and none of those
// four; installing File Uploader takes widgets/ from 33 to 34, and mxcli then
// builds the page with no further action, because initPluggableEngine refreshes
// definitions from installed packages on its own. mxcli therefore ships no
// definitions for them — there is nothing to ship that the project does not
// already carry once the widget is usable at all.
//
// The distinguishing question is exactly the one FindMPK answers, and it is the
// same lookup the template loader makes before giving up.
func (pb *pageBuilder) missingWidgetMessage(widgetID string) string {
	projectDir := ""
	if pb.backend != nil {
		projectDir = filepath.Dir(pb.backend.Path())
	}
	return missingWidgetMessage(projectDir, widgetID)
}

// missingWidgetMessage is the pure form, so the branch can be tested without
// standing up a whole backend.
func missingWidgetMessage(projectDir, widgetID string) string {
	if projectDir != "" {
		if found, err := mpk.FindMPK(projectDir, widgetID); err == nil && found != "" {
			// The package is installed; the definition just has not been
			// extracted from it yet. This is the case `widget init` exists for.
			return "no definition for widget " + widgetID +
				" — its package is installed but not extracted yet (run 'mxcli widget init -p app.mpr')"
		}
	}
	return "no definition for widget " + widgetID +
		" — the project has no widget package for it in widgets/." +
		" 'mxcli widget init' cannot help: it scans widgets/, and the package is not there." +
		" Install the widget or its module from the Marketplace; that puts the .mpk in widgets/," +
		" after which mxcli picks it up automatically."
}
