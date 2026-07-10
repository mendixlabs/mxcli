// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genCw "github.com/mendixlabs/mxcli/modelsdk/gen/customwidgets"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	genTexts "github.com/mendixlabs/mxcli/modelsdk/gen/texts"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func init() {
	// Conditional-visibility / native-accessibility slots are null when unset, on
	// every widget that has them (verified against real page BSON).
	for _, t := range []string{"Forms$DivContainer", "Forms$DynamicText"} {
		codec.RegisterTypeDefaults(t, codec.TypeDefaults{
			NullFields: []string{"ConditionalVisibilitySettings", "NativeAccessibilitySettings"},
		})
		// Widgets nested in a Widgets list use the typed-array marker 2 when present.
		codec.RegisterListMarker(t, 2)
	}
	// A ClientTemplate's Parameters list is always emitted with marker 2, even empty
	// (unusual — most empty lists are marker 3).
	codec.RegisterTypeDefaults("Forms$ClientTemplate", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Parameters": 2},
	})
	// LayoutGrid and its rows carry a null ConditionalVisibilitySettings; the grid,
	// rows, and columns all use the typed-array marker 2 in their parent lists.
	for _, t := range []string{"Forms$LayoutGrid", "Forms$LayoutGridRow"} {
		codec.RegisterTypeDefaults(t, codec.TypeDefaults{
			NullFields: []string{"ConditionalVisibilitySettings"},
		})
	}
	codec.RegisterListMarker("Forms$LayoutGrid", 2)
	codec.RegisterListMarker("Forms$LayoutGridRow", 2)
	codec.RegisterListMarker("Forms$LayoutGridColumn", 2)
	// ActionButton: null Icon/visibility/accessibility slots; marker 2 as a widget.
	codec.RegisterTypeDefaults("Forms$ActionButton", codec.TypeDefaults{
		NullFields: []string{"Icon", "ConditionalVisibilitySettings", "NativeAccessibilitySettings"},
	})
	codec.RegisterListMarker("Forms$ActionButton", 2)
	// Title: null visibility/accessibility slots; marker 2 as a widget.
	codec.RegisterTypeDefaults("Forms$Title", codec.TypeDefaults{
		NullFields: []string{"ConditionalVisibilitySettings", "NativeAccessibilitySettings"},
	})
	codec.RegisterListMarker("Forms$Title", 2)
	// Conditional visibility/editability settings (issue #627). When a widget
	// carries one, applyWidgetBase emits the node; these defaults fill the
	// sub-fields Studio Pro writes: empty-string Attribute, null SourceVariable, and
	// empty (marker-3) Conditions/ModuleRoles lists. Expression/IgnoreSecurity are
	// set from the model. ExpressionModel is intentionally omitted (legacy omits it).
	// Attribute is "" (not null): it is a BY_NAME AttributeIdentifier and Mendix
	// 11.12's reader rejects a null there (StorageLoadException, "not a valid
	// AttributeIdentifier").
	codec.RegisterTypeDefaults("Forms$ConditionalVisibilitySettings", codec.TypeDefaults{
		NullFields:        []string{"SourceVariable"},
		EmptyStringFields: []string{"Attribute"},
		MandatoryLists:    []string{"Conditions", "ModuleRoles"},
	})
	codec.RegisterTypeDefaults("Forms$ConditionalEditabilitySettings", codec.TypeDefaults{
		NullFields:        []string{"SourceVariable"},
		EmptyStringFields: []string{"Attribute"},
		MandatoryLists:    []string{"Conditions"},
	})
	// A caption parameter's AttributeRef/SourceVariable are null for the literal-
	// expression form; populated Parameters lists use marker 2.
	codec.RegisterTypeDefaults("Forms$ClientTemplateParameter", codec.TypeDefaults{
		NullFields: []string{"AttributeRef", "SourceVariable"},
	})
	codec.RegisterListMarker("Forms$ClientTemplateParameter", 2)
	// DataView: null visibility/editability settings; Widgets/FooterWidgets always
	// emitted with marker 2 (even empty). A page-context DataViewSource has null
	// EntityRef/SourceVariable when unbound.
	codec.RegisterTypeDefaults("Forms$DataView", codec.TypeDefaults{
		NullFields:           []string{"ConditionalVisibilitySettings", "ConditionalEditabilitySettings"},
		MandatoryListMarkers: map[string]int32{"Widgets": 2, "FooterWidgets": 2},
	})
	codec.RegisterListMarker("Forms$DataView", 2)
	codec.RegisterTypeDefaults("Forms$DataViewSource", codec.TypeDefaults{
		NullFields: []string{"EntityRef", "SourceVariable"},
	})
	// A microflow data source's settings carry an always-emitted (empty) parameter
	// mapping list and null progress/confirmation slots.
	codec.RegisterTypeDefaults("Forms$MicroflowSettings", codec.TypeDefaults{
		MandatoryLists: []string{"ParameterMappings"},
		NullFields:     []string{"ProgressMessage", "ConfirmationInfo"},
	})
	// TextBox: many null slots when unbound (attribute ref, screen-reader label,
	// source variable, label template, visibility/editability/native settings).
	codec.RegisterTypeDefaults("Forms$TextBox", codec.TypeDefaults{
		NullFields: []string{
			"AttributeRef", "ScreenReaderLabel", "SourceVariable", "LabelTemplate",
			"ConditionalVisibilitySettings", "ConditionalEditabilitySettings",
			"NativeAccessibilitySettings",
		},
	})
	codec.RegisterListMarker("Forms$TextBox", 2)
	// Pluggable widget container: null visibility/editability slots; marker 2.
	codec.RegisterTypeDefaults("CustomWidgets$CustomWidget", codec.TypeDefaults{
		NullFields: []string{"ConditionalVisibilitySettings", "ConditionalEditabilitySettings"},
	})
	codec.RegisterListMarker("CustomWidgets$CustomWidget", 2)
	// RadioButtonGroup (the MDL `radiobuttons` widget): same null-slot set as TextBox.
	codec.RegisterTypeDefaults("Forms$RadioButtonGroup", codec.TypeDefaults{
		NullFields: []string{
			"AttributeRef", "ScreenReaderLabel", "SourceVariable", "LabelTemplate",
			"ConditionalVisibilitySettings", "ConditionalEditabilitySettings",
		},
	})
	codec.RegisterListMarker("Forms$RadioButtonGroup", 2)
	// DatePicker: TextBox-like null slots (+ native accessibility).
	codec.RegisterTypeDefaults("Forms$DatePicker", codec.TypeDefaults{
		NullFields: []string{
			"AttributeRef", "ScreenReaderLabel", "SourceVariable", "LabelTemplate",
			"ConditionalVisibilitySettings", "ConditionalEditabilitySettings",
			"NativeAccessibilitySettings",
		},
	})
	codec.RegisterListMarker("Forms$DatePicker", 2)
	// TextArea: TextBox-like null slots (+ native accessibility).
	codec.RegisterTypeDefaults("Forms$TextArea", codec.TypeDefaults{
		NullFields: []string{
			"AttributeRef", "ScreenReaderLabel", "SourceVariable", "LabelTemplate",
			"ConditionalVisibilitySettings", "ConditionalEditabilitySettings",
			"NativeAccessibilitySettings",
		},
	})
	codec.RegisterListMarker("Forms$TextArea", 2)
	// CheckBox: boolean input; same null-slot set.
	codec.RegisterTypeDefaults("Forms$CheckBox", codec.TypeDefaults{
		NullFields: []string{
			"AttributeRef", "ScreenReaderLabel", "SourceVariable", "LabelTemplate",
			"ConditionalVisibilitySettings", "ConditionalEditabilitySettings",
			"NativeAccessibilitySettings",
		},
	})
	codec.RegisterListMarker("Forms$CheckBox", 2)
	// NavigationList: null visibility; items + the widget itself use marker 2.
	codec.RegisterTypeDefaults("Forms$NavigationList", codec.TypeDefaults{
		NullFields: []string{"ConditionalVisibilitySettings"},
	})
	codec.RegisterListMarker("Forms$NavigationList", 2)
	codec.RegisterListMarker("Forms$NavigationListItem", 2)
	// SnippetCallWidget: null visibility; the inner SnippetCall always emits its
	// (empty) ParameterMappings array.
	codec.RegisterTypeDefaults("Forms$SnippetCallWidget", codec.TypeDefaults{
		NullFields: []string{"ConditionalVisibilitySettings"},
	})
	codec.RegisterListMarker("Forms$SnippetCallWidget", 2)
	codec.RegisterTypeDefaults("Forms$SnippetCall", codec.TypeDefaults{
		MandatoryLists: []string{"ParameterMappings"},
	})
	// ListView: null visibility; always emits its Templates list; marker 2.
	codec.RegisterTypeDefaults("Forms$ListView", codec.TypeDefaults{
		NullFields:     []string{"ConditionalVisibilitySettings"},
		MandatoryLists: []string{"Templates"},
	})
	codec.RegisterListMarker("Forms$ListView", 2)
	codec.RegisterListMarker("Forms$ListViewTemplate", 2)
	// GroupBox: container with caption/header; null visibility; marker 2.
	codec.RegisterTypeDefaults("Forms$GroupBox", codec.TypeDefaults{
		NullFields: []string{"ConditionalVisibilitySettings"},
	})
	codec.RegisterListMarker("Forms$GroupBox", 2)
	// show_page action (Forms$FormAction) + its FormSettings always emit their
	// (empty) typed-array lists with marker 2.
	codec.RegisterTypeDefaults("Forms$FormAction", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"PagesForSpecializations": 2},
	})
	codec.RegisterTypeDefaults("Forms$FormSettings", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"ParameterMappings": 2},
	})
	// create_object action: EntityRef is null when no entity is specified.
	codec.RegisterTypeDefaults("Forms$CreateObjectClientAction", codec.TypeDefaults{
		NullFields: []string{"EntityRef"},
	})
}

// widgetToGen converts a model widget to its gen element, recursing into
// containers. Unsupported widget types are refused loudly (ADR-0005) so a page
// is never written with a silently-dropped widget.
func widgetToGen(w pages.Widget) (element.Element, error) {
	switch x := w.(type) {
	case *pages.Container:
		g := genPg.NewDivContainer()
		applyWidgetBase(g, &x.BaseWidget)
		g.SetRenderMode(orDefaultStr(string(x.RenderMode), "Div"))
		g.SetScreenReaderHidden(false)
		onClick, err := clientActionToGen(x.OnClickAction)
		if err != nil {
			return nil, err
		}
		g.SetOnClickAction(onClick)
		for _, c := range x.Widgets {
			cg, err := widgetToGen(c)
			if err != nil {
				return nil, err
			}
			g.AddWidgets(cg)
		}
		return g, nil

	case *pages.TabContainer:
		// pages.TabContainer → Forms$TabControl (gen type genPg.TabContainer).
		g := genPg.NewTabContainer()
		applyWidgetBase(g, &x.BaseWidget)
		g.SetActivePageOnChangeAction(noActionGen())
		var defaultID element.ID
		for i, tp := range x.TabPages {
			tpg := genPg.NewTabPage()
			if tp.ID != "" {
				tpg.SetID(element.ID(tp.ID))
			}
			assignID(tpg)
			tpg.SetName(tp.Name)
			tpg.SetRefreshOnShow(tp.RefreshOnShow)
			// Caption defaults to the tab name when unset (matches legacy).
			capText := tp.Caption
			if capText == nil {
				capText = &model.Text{Translations: map[string]string{"en_US": tp.Name}}
			}
			tpg.SetCaption(captionToGen(capText))
			for _, c := range tp.Widgets {
				cg, err := widgetToGen(c)
				if err != nil {
					return nil, err
				}
				tpg.AddWidgets(cg)
			}
			g.AddTabPages(tpg)
			if i == 0 {
				defaultID = tpg.ID()
			}
		}
		if x.DefaultPageID != "" {
			defaultID = element.ID(x.DefaultPageID)
		}
		if defaultID != "" {
			g.SetDefaultPageID(defaultID)
		}
		return g, nil

	case *pages.DynamicText:
		g := genPg.NewDynamicText()
		applyWidgetBase(g, &x.BaseWidget)
		g.SetRenderMode(orDefaultStr(string(x.RenderMode), "Text"))
		g.SetNativeTextStyle("Text")
		g.SetContent(clientTemplateToGen(x.Content))
		return g, nil

	case *pages.LayoutGrid:
		g := genPg.NewLayoutGrid()
		applyWidgetBase(g, &x.BaseWidget)
		g.SetWidth("FullWidth")
		for _, row := range x.Rows {
			rg, err := layoutGridRowToGen(row)
			if err != nil {
				return nil, err
			}
			g.AddRows(rg)
		}
		return g, nil

	case *pages.DataView:
		g := genPg.NewDataView()
		applyWidgetBase(g, &x.BaseWidget)
		ds, err := dataViewSourceToGen(x.DataSource)
		if err != nil {
			return nil, err
		}
		g.SetDataSource(ds)
		g.SetEditability(editability(x.ReadOnly))
		g.SetReadOnlyStyle("Control")
		g.SetShowFooter(x.ShowFooter)
		if x.LabelWidth != nil {
			g.SetLabelWidth(int32(*x.LabelWidth))
		}
		g.SetNoEntityMessage(captionToGen(x.NoEntityMessage))
		for _, c := range x.Widgets {
			cg, err := widgetToGen(c)
			if err != nil {
				return nil, err
			}
			g.AddWidgets(cg)
		}
		for _, c := range x.FooterWidgets {
			cg, err := widgetToGen(c)
			if err != nil {
				return nil, err
			}
			g.AddFooterWidgets(cg)
		}
		return g, nil

	case *pages.Title:
		g := genPg.NewTitle()
		applyWidgetBase(g, &x.BaseWidget)
		g.SetCaption(captionToGen(x.Caption))
		return g, nil

	case *pages.TextBox:
		g := genPg.NewTextBox()
		applyWidgetBase(g, &x.BaseWidget)
		g.SetAriaRequired(false)
		g.SetAutoFocus(false)
		g.SetAutocomplete(true)
		g.SetAutocompletePurpose("On")
		if ref := attributeRefToGen(x.AttributePath); ref != nil {
			g.SetAttributeRef(ref)
		}
		g.SetEditable("Always")
		g.SetFormattingInfo(newFormattingInfo())
		g.SetInputMask("")
		g.SetIsPasswordBox(x.IsPassword)
		g.SetKeyboardType("Default")
		if x.Label != "" {
			g.SetLabelTemplate(textAsClientTemplate(textFromString(x.Label)))
		}
		g.SetMaxLengthCode(-1)
		onChange, err := clientActionToGen(x.OnChangeAction)
		if err != nil {
			return nil, err
		}
		onEnter, err := clientActionToGen(x.OnEnterAction)
		if err != nil {
			return nil, err
		}
		g.SetOnChangeAction(onChange)
		g.SetOnEnterAction(onEnter)
		g.SetOnEnterKeyPressAction(noActionGen())
		g.SetOnLeaveAction(noActionGen())
		g.SetPlaceholderTemplate(textAsClientTemplate(x.Placeholder))
		g.SetReadOnlyStyle("Inherit")
		g.SetSubmitBehaviour("OnEndEditing")
		g.SetSubmitOnInputDelay(300)
		g.SetValidation(widgetValidationToGen())
		return g, nil

	case *pages.ActionButton:
		g := genPg.NewActionButton()
		applyWidgetBase(g, &x.BaseWidget)
		g.SetAriaRole("Button")
		// "Link" for a `linkbutton` (action button rendered as a link), else "Button".
		g.SetRenderType(orDefaultStr(string(x.RenderMode), "Button"))
		g.SetButtonStyle(orDefaultStr(string(x.ButtonStyle), "Default"))
		if x.CaptionTemplate != nil {
			g.SetCaption(clientTemplateToGen(x.CaptionTemplate))
		} else {
			g.SetCaption(textAsClientTemplate(x.Caption))
		}
		// Tooltip is a Texts$Text (not a ClientTemplate, unlike Caption) — Studio
		// Pro's loader rejects a ClientTemplate here with a type-cast error.
		g.SetTooltip(captionToGen(x.Tooltip))
		act, err := clientActionToGen(x.Action)
		if err != nil {
			return nil, err
		}
		g.SetAction(act)
		return g, nil

	case *pages.CheckBox:
		g := genPg.NewCheckBox()
		applyWidgetBase(g, &x.BaseWidget)
		if ref := attributeRefToGen(x.AttributePath); ref != nil {
			g.SetAttributeRef(ref)
		}
		g.SetEditable("Always")
		if x.Label != "" {
			g.SetLabelTemplate(textAsClientTemplate(textFromString(x.Label)))
		}
		onChangeCB, err := clientActionToGen(x.OnChangeAction)
		if err != nil {
			return nil, err
		}
		g.SetOnChangeAction(onChangeCB)
		g.SetOnEnterAction(noActionGen())
		g.SetReadOnlyStyle("Inherit")
		g.SetValidation(widgetValidationToGen())
		return g, nil

	case *pages.TextArea:
		g := genPg.NewTextArea()
		applyWidgetBase(g, &x.BaseWidget)
		g.SetAriaRequired(false)
		g.SetAutoFocus(false)
		if ref := attributeRefToGen(x.AttributePath); ref != nil {
			g.SetAttributeRef(ref)
		}
		g.SetCounterMessage(captionToGen(x.CounterMessage))
		g.SetEditable("Always")
		if x.Label != "" {
			g.SetLabelTemplate(textAsClientTemplate(textFromString(x.Label)))
		}
		g.SetMaxLengthCode(-1)
		lines := int32(x.Rows)
		if lines == 0 {
			lines = 5
		}
		g.SetNumberOfLines(lines)
		onChangeTA, err := clientActionToGen(x.OnChangeAction)
		if err != nil {
			return nil, err
		}
		g.SetOnChangeAction(onChangeTA)
		g.SetOnEnterAction(noActionGen())
		g.SetOnLeaveAction(noActionGen())
		g.SetPlaceholderTemplate(textAsClientTemplate(x.Placeholder))
		g.SetReadOnlyStyle("Inherit")
		g.SetSubmitBehaviour("OnEndEditing")
		g.SetSubmitOnInputDelay(300)
		g.SetValidation(widgetValidationToGen())
		return g, nil

	case *pages.DatePicker:
		g := genPg.NewDatePicker()
		applyWidgetBase(g, &x.BaseWidget)
		g.SetAriaRequired(false)
		if ref := attributeRefToGen(x.AttributePath); ref != nil {
			g.SetAttributeRef(ref)
		}
		g.SetEditable("Always")
		g.SetFormattingInfo(newFormattingInfo())
		if x.Label != "" {
			g.SetLabelTemplate(textAsClientTemplate(textFromString(x.Label)))
		}
		onChangeDP, err := clientActionToGen(x.OnChangeAction)
		if err != nil {
			return nil, err
		}
		g.SetOnChangeAction(onChangeDP)
		g.SetOnEnterAction(noActionGen())
		g.SetPlaceholderTemplate(textAsClientTemplate(x.Placeholder))
		g.SetReadOnlyStyle("Inherit")
		g.SetValidation(widgetValidationToGen())
		return g, nil

	case *pages.RadioButtons:
		g := genPg.NewRadioButtonGroup()
		applyWidgetBase(g, &x.BaseWidget)
		g.SetAriaRequired(false)
		if ref := attributeRefToGen(x.AttributePath); ref != nil {
			g.SetAttributeRef(ref)
		}
		g.SetEditable("Always")
		if x.Label != "" {
			g.SetLabelTemplate(textAsClientTemplate(textFromString(x.Label)))
		}
		g.SetReadOnlyStyle("Inherit")
		g.SetRenderHorizontal(x.RenderDirection != pages.RenderDirectionVertical)
		onChange, err := clientActionToGen(x.OnChangeAction)
		if err != nil {
			return nil, err
		}
		g.SetOnChangeAction(onChange)
		g.SetOnEnterAction(noActionGen())
		g.SetValidation(widgetValidationToGen())
		return g, nil

	case *pages.GroupBox:
		g := genPg.NewGroupBox()
		applyWidgetBase(g, &x.BaseWidget)
		g.SetCaption(clientTemplateToGen(x.Caption))
		g.SetCollapsible(orDefaultStr(x.Collapsible, "No"))
		g.SetHeaderMode(orDefaultStr(x.HeaderMode, "Div"))
		for _, w := range x.Widgets {
			wg, err := widgetToGen(w)
			if err != nil {
				return nil, err
			}
			g.AddWidgets(wg)
		}
		return g, nil

	case *pages.ListView:
		g := genPg.NewListView()
		applyWidgetBase(g, &x.BaseWidget)
		ds, err := listViewSourceToGen(x.DataSource)
		if err != nil {
			return nil, err
		}
		g.SetDataSource(ds)
		clickAct, err := clientActionToGen(x.ClickAction)
		if err != nil {
			return nil, err
		}
		g.SetClickAction(clickAct)
		g.SetEditable(x.Editable)
		g.SetNumberOfColumns(1)
		pageSize := int32(x.PageSize)
		if pageSize == 0 {
			pageSize = 20
		}
		g.SetPageSize(pageSize)
		g.SetPullDownAction(noActionGen())
		g.SetScrollDirection("Vertical")
		for _, t := range x.Templates {
			tg := genPg.NewListViewTemplate()
			assignID(tg)
			for _, w := range t.Widgets {
				wg, err := widgetToGen(w)
				if err != nil {
					return nil, err
				}
				tg.AddWidgets(wg)
			}
			g.AddTemplates(tg)
		}
		for _, w := range x.Widgets {
			wg, err := widgetToGen(w)
			if err != nil {
				return nil, err
			}
			g.AddWidgets(wg)
		}
		return g, nil

	case *pages.SnippetCallWidget:
		g := genPg.NewSnippetCallWidget()
		applyWidgetBase(g, &x.BaseWidget)
		call := genPg.NewSnippetCall()
		assignID(call)
		call.SetSnippetQualifiedName(x.SnippetName)
		// Parameter mappings: each maps a snippet parameter (BY_NAME, as
		// "Snippet.Param") to the page variable being passed. Argument stays empty;
		// the variable reference lives in Variable.PageParameter (mirrors the legacy
		// serializer). Without these, Studio Pro reports CE1571 "no argument selected".
		for _, pm := range x.ParameterMappings {
			m := genPg.NewSnippetParameterMapping()
			assignID(m)
			m.SetParameterQualifiedName(x.SnippetName + "." + pm.ParamName)
			m.SetArgument("")
			pv := genPg.NewPageVariable()
			assignID(pv)
			pv.SetPageParameterQualifiedName(strings.TrimPrefix(pm.Argument, "$"))
			m.SetVariable(pv)
			call.AddParameterMappings(m)
		}
		g.SetSnippetCall(call)
		return g, nil

	case *pages.NavigationList:
		g := genPg.NewNavigationList()
		applyWidgetBase(g, &x.BaseWidget)
		for _, item := range x.Items {
			ig, err := navListItemToGen(item)
			if err != nil {
				return nil, err
			}
			g.AddItems(ig)
		}
		return g, nil

	case *pages.CustomWidget:
		return customWidgetToGen(x)

	default:
		return nil, fmt.Errorf("CreatePage: widget %T not yet supported by the modelsdk engine — rerun with MXCLI_ENGINE=legacy", w)
	}
}

// customWidgetToGen embeds a pluggable widget (CustomWidgets$CustomWidget). Its
// Type (PropertyTypes schema) and Object (filled WidgetObject) are the pluggable
// widget's own raw BSON — not metamodel types — so they're decoded into the codec
// as round-trippable passthrough elements and re-emitted verbatim.
func customWidgetToGen(x *pages.CustomWidget) (element.Element, error) {
	g := genCw.NewCustomWidget()
	applyWidgetBase(g, &x.BaseWidget)
	g.SetEditable(orDefaultStr(x.Editable, "Always"))
	if x.Label != "" {
		g.SetLabelTemplate(textAsClientTemplate(textFromString(x.Label)))
	}
	if x.RawType != nil {
		t, err := decodeRawBSON(x.RawType)
		if err != nil {
			return nil, fmt.Errorf("custom widget %q: decode Type: %w", x.Name, err)
		}
		g.SetType(t)
	}
	if x.RawObject != nil {
		o, err := decodeRawBSON(x.RawObject)
		if err != nil {
			return nil, fmt.Errorf("custom widget %q: decode Object: %w", x.Name, err)
		}
		g.SetObject(o)
	}
	return g, nil
}

// decodeRawBSON turns a raw widget-schema bson.D into a codec element. Unknown
// pluggable $Types are preserved as raw passthrough by the decoder, so the
// element re-emits byte-for-byte on encode.
func decodeRawBSON(d bson.D) (element.Element, error) {
	raw, err := bson.Marshal(d)
	if err != nil {
		return nil, err
	}
	return codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
}

// navListItemToGen converts a NavigationListItem (its click action + content
// widgets; a caption with no explicit widgets becomes a DynamicText).
func navListItemToGen(item *pages.NavigationListItem) (element.Element, error) {
	g := genPg.NewNavigationListItem()
	if item.ID != "" {
		g.SetID(element.ID(item.ID))
	}
	assignID(g)
	g.SetAppearance(newAppearance("", "", "", nil))
	act, err := clientActionToGen(item.Action)
	if err != nil {
		return nil, err
	}
	g.SetAction(act)

	widgets := item.Widgets
	if len(widgets) == 0 && item.Caption != nil {
		widgets = []pages.Widget{&pages.DynamicText{Content: &pages.ClientTemplate{Template: item.Caption}}}
	}
	for _, w := range widgets {
		wg, err := widgetToGen(w)
		if err != nil {
			return nil, err
		}
		g.AddWidgets(wg)
	}
	return g, nil
}

// layoutGridRowToGen converts a LayoutGridRow (alignment defaults match the
// legacy serializer; not a full widget, so no name/tabindex).
func layoutGridRowToGen(row *pages.LayoutGridRow) (element.Element, error) {
	g := genPg.NewLayoutGridRow()
	if row.ID != "" {
		g.SetID(element.ID(row.ID))
	}
	assignID(g)
	g.SetAppearance(newAppearance("", "", "", nil))
	g.SetHorizontalAlignment("None")
	g.SetSpacingBetweenColumns(true)
	g.SetVerticalAlignment("None")
	for _, col := range row.Columns {
		cg, err := layoutGridColumnToGen(col)
		if err != nil {
			return nil, err
		}
		g.AddColumns(cg)
	}
	return g, nil
}

// layoutGridColumnToGen converts a LayoutGridColumn. Weights default to -1 (auto,
// via columnWeight); PreviewWidth is always -1 — matching the legacy serializer.
func layoutGridColumnToGen(col *pages.LayoutGridColumn) (element.Element, error) {
	g := genPg.NewLayoutGridColumn()
	if col.ID != "" {
		g.SetID(element.ID(col.ID))
	}
	assignID(g)
	g.SetAppearance(newAppearance("", "", "", nil))
	g.SetWeight(int32(columnWeight(col.Weight)))
	g.SetTabletWeight(int32(columnWeight(col.TabletWeight)))
	g.SetPhoneWeight(int32(columnWeight(col.PhoneWeight)))
	g.SetPreviewWidth(-1)
	g.SetVerticalAlignment("None")
	for _, w := range col.Widgets {
		wg, err := widgetToGen(w)
		if err != nil {
			return nil, err
		}
		g.AddWidgets(wg)
	}
	return g, nil
}

// columnWeight maps an unset weight (0) to -1 (auto-fill), matching the legacy
// serializer's columnWeight.
func columnWeight(w int) int32 {
	if w == 0 {
		return -1
	}
	return int32(w)
}

// widgetBaseGen is the shared setter surface of a gen widget element.
type widgetBaseGen interface {
	element.Element
	SetID(element.ID)
	SetName(string)
	SetAppearance(element.Element)
	SetTabIndex(int32)
}

// applyWidgetBase sets the fields every widget shares: identity, name, appearance
// (carrying class/style), and tab index. ConditionalVisibility/native
// accessibility are emitted null via the registered defaults.
func applyWidgetBase(g widgetBaseGen, b *pages.BaseWidget) {
	if b.ID != "" {
		g.SetID(element.ID(b.ID))
	}
	assignID(g)
	g.SetName(b.Name)
	g.SetAppearance(newAppearance(b.Class, b.Style, b.DynamicClasses, b.DesignProperties))
	g.SetTabIndex(int32(b.TabIndex))

	// Conditional visibility/editability. When unset these stay null via the
	// per-type NullFields defaults; when set we must emit the settings node, or
	// the codec silently drops the expression (issue #627). Type-asserted because
	// only the widgets that actually carry these slots expose the setters.
	if b.ConditionalVisibility != nil {
		if cv, ok := g.(interface {
			SetConditionalVisibilitySettings(element.Element)
		}); ok {
			cv.SetConditionalVisibilitySettings(conditionalVisibilityToGen(b.ConditionalVisibility))
		}
	}
	if b.ConditionalEditability != nil {
		if ce, ok := g.(interface {
			SetConditionalEditabilitySettings(element.Element)
		}); ok {
			ce.SetConditionalEditabilitySettings(conditionalEditabilityToGen(b.ConditionalEditability))
		}
	}
}

// conditionalVisibilityToGen builds a Forms$ConditionalVisibilitySettings element
// for the codec. The empty Conditions/ModuleRoles lists, null Attribute/
// SourceVariable, and IgnoreSecurity=false come from the registered TypeDefaults.
func conditionalVisibilityToGen(cvs *pages.ConditionalVisibilitySettings) element.Element {
	g := genPg.NewConditionalVisibilitySettings()
	if cvs.ID != "" {
		g.SetID(element.ID(cvs.ID))
	}
	assignID(g)
	g.SetExpression(cvs.Expression)
	g.SetIgnoreSecurity(false)
	return g
}

// conditionalEditabilityToGen builds a Forms$ConditionalEditabilitySettings element.
func conditionalEditabilityToGen(ces *pages.ConditionalEditabilitySettings) element.Element {
	g := genPg.NewConditionalEditabilitySettings()
	if ces.ID != "" {
		g.SetID(element.ID(ces.ID))
	}
	assignID(g)
	g.SetExpression(ces.Expression)
	return g
}

// newAppearance builds a Forms$Appearance with the given class/style, dynamic
// classes expression, and design properties.
func newAppearance(class, style, dynamicClasses string, dps []pages.DesignPropertyValue) *genPg.Appearance {
	a := genPg.NewAppearance()
	assignID(a)
	a.SetClass(class)
	a.SetStyle(style)
	a.SetDynamicClasses(dynamicClasses)
	for _, dp := range dps {
		a.AddDesignProperties(designPropertyGen(dp))
	}
	return a
}

// designPropertyGen builds a Forms$DesignPropertyValue wrapper (Key + typed Value
// element) for the codec, mirroring the on-disk structure.
func designPropertyGen(dp pages.DesignPropertyValue) element.Element {
	w := genPg.NewDesignPropertyValue()
	assignID(w)
	w.SetKey(dp.Key)
	w.SetValue(designPropertyValueGen(dp))
	return w
}

// designPropertyValueGen builds the typed value element. Compound values nest a
// Forms$CompoundDesignPropertyValue whose Properties are themselves
// DesignPropertyValue wrappers (recursive), e.g. Spacing → margin-top/bottom.
func designPropertyValueGen(dp pages.DesignPropertyValue) element.Element {
	switch dp.ValueType {
	case "toggle":
		v := genPg.NewToggleDesignPropertyValue()
		assignID(v)
		return v
	case "custom":
		v := genPg.NewCustomDesignPropertyValue()
		assignID(v)
		v.SetValue(dp.Option)
		return v
	case "compound":
		v := genPg.NewCompoundDesignPropertyValue()
		assignID(v)
		for _, sub := range dp.Compound {
			v.AddProperties(designPropertyGen(sub))
		}
		return v
	default: // "option"
		v := genPg.NewOptionDesignPropertyValue()
		assignID(v)
		v.SetOption(dp.Option)
		return v
	}
}

// noActionGen builds the default Forms$NoAction (DisabledDuringExecution=true)
// used by widget OnClick slots that have no action.
func noActionGen() element.Element {
	a := genPg.NewNoClientAction() // emits $Type Forms$NoAction
	assignID(a)
	a.SetDisabledDuringExecution(true)
	return a
}

// clientTemplateToGen builds the Forms$ClientTemplate that backs a dynamic text
// or button caption (Template + Fallback are Texts$Text; Parameters supply the
// {1}/{2}… placeholder values).
func clientTemplateToGen(ct *pages.ClientTemplate) element.Element {
	g := genPg.NewClientTemplate()
	assignID(g)
	if ct == nil {
		g.SetTemplate(genTexts.NewText())
		g.SetFallback(genTexts.NewText())
		return g
	}
	g.SetTemplate(captionToGen(ct.Template))
	g.SetFallback(captionToGen(ct.Fallback))
	for _, p := range ct.Parameters {
		g.AddParameters(clientTemplateParameterToGen(p))
	}
	return g
}

// textAsClientTemplate wraps a plain caption/tooltip Text in a ClientTemplate.
func textAsClientTemplate(t *model.Text) element.Element {
	return clientTemplateToGen(&pages.ClientTemplate{Template: t})
}

// clientTemplateParameterToGen converts a caption parameter ({n} value). Only the
// literal-expression form is supported; AttributeRef/SourceVariable stay null
// (registered defaults). FormattingInfo carries the standard defaults.
func clientTemplateParameterToGen(p *pages.ClientTemplateParameter) element.Element {
	g := genPg.NewClientTemplateParameter()
	if p.ID != "" {
		g.SetID(element.ID(p.ID))
	}
	assignID(g)
	g.SetExpression(p.Expression)
	if p.AttributeRef != "" {
		// The parameter's value source is a DomainModels$AttributeRef sub-object,
		// NOT the AttributePath scalar — Studio Pro reads AttributeRef and reports
		// CE0402 "No value specified" if it is null. Matches legacy.
		if len(p.AttributeRefSteps) > 0 {
			// Attribute navigated over associations: AttributeRef carries the final
			// attribute plus an IndirectEntityRef of association steps.
			g.SetAttributeRef(attributeRefWithStepsToGen(p.AttributeRef, p.AttributeRefSteps))
		} else if ref := attributeRefToGen(p.AttributeRef); ref != nil {
			g.SetAttributeRef(ref)
		} else {
			g.SetAttributePath(p.AttributeRef)
		}
	}
	// When the attribute is read from a page/local/snippet variable (e.g.
	// {1}=$Product.Name where $Product is a page parameter), the variable source
	// MUST be emitted as a Forms$PageVariable. Without it Studio Pro can't resolve
	// the attribute's data context → CE1365 "move into a data container" + CE7006
	// "selected value is not valid for attribute". Matches the legacy serializer.
	if p.SourceVariable != "" {
		g.SetSourceVariable(sourceVariableToGen(p.SourceVariable, p.SourceVariableKind))
	}
	g.SetFormattingInfo(newFormattingInfo())
	return g
}

// sourceVariableToGen builds the Forms$PageVariable that names a template
// parameter's (or similar) data source. kind selects which slot the variable
// name fills: "" = page parameter, "local" = page-level Variables entry,
// "snippet" = snippet parameter.
func sourceVariableToGen(name, kind string) element.Element {
	pv := genPg.NewPageVariable()
	assignID(pv)
	switch kind {
	case "local":
		pv.SetLocalVariableQualifiedName(name)
	case "snippet":
		pv.SetSnippetParameterQualifiedName(name)
	default:
		pv.SetPageParameterQualifiedName(name)
	}
	return pv
}

// newFormattingInfo builds the default Forms$FormattingInfo (matches the legacy
// serializer; TimeFormat is intentionally omitted — it triggers CE0463).
func newFormattingInfo() element.Element {
	f := genPg.NewFormattingInfo()
	assignID(f)
	f.SetCustomDateFormat("")
	f.SetDateFormat("Date")
	f.SetDecimalPrecision(2)
	f.SetEnumFormat("Text")
	f.SetGroupDigits(false)
	return f
}

// textFromString wraps a non-empty string as a single-translation model.Text.
func textFromString(s string) *model.Text {
	if s == "" {
		return nil
	}
	return &model.Text{Translations: map[string]string{"en_US": s}}
}

// attributeRefToGen builds a DomainModels$AttributeRef for a fully-qualified
// attribute path (Module.Entity.Attribute); returns nil otherwise (the slot is
// then emitted null via the registered default), matching the legacy serializer.
func attributeRefToGen(path string) element.Element {
	if strings.Count(path, ".") < 2 {
		return nil
	}
	r := genDm.NewAttributeRef()
	assignID(r)
	r.SetAttributeQualifiedName(path)
	return r
}

// attributeRefWithStepsToGen builds a DomainModels$AttributeRef for an attribute
// navigated over one or more associations: the final attribute qualified name
// plus an EntityRef (DomainModels$IndirectEntityRef) of association hops. Reuses
// entityRefToGen (the microflow IndirectEntityRef builder) — the storage names
// are identical.
func attributeRefWithStepsToGen(attrQN string, steps []pages.AttributeRefStep) element.Element {
	r := genDm.NewAttributeRef()
	assignID(r)
	r.SetAttributeQualifiedName(attrQN)
	mSteps := make([]microflows.EntityRefStep, len(steps))
	for i, s := range steps {
		mSteps[i] = microflows.EntityRefStep{Association: s.Association, DestinationEntity: s.DestinationEntity}
	}
	r.SetEntityRef(entityRefToGen(mSteps))
	return r
}

// widgetValidationToGen builds the default empty Forms$WidgetValidation.
func widgetValidationToGen() element.Element {
	v := genPg.NewWidgetValidation()
	assignID(v)
	v.SetExpression("")
	v.SetMessage(genTexts.NewText())
	return v
}

// editability maps a read-only flag to the Forms editability enum.
func editability(readOnly bool) string {
	if readOnly {
		return "Never"
	}
	return "Always"
}

// dataViewSourceToGen builds a DataView's data source. The page-context source
// (Forms$DataViewSource: entity ref + page/snippet parameter) is supported; flow
// and database sources (which carry settings sub-objects) are refused for now.
func dataViewSourceToGen(ds pages.DataSource) (element.Element, error) {
	switch d := ds.(type) {
	case nil:
		// empty DataViewSource — DataView requires a non-null source object.
		src := genPg.NewDataViewSource()
		src.SetForceFullObjects(false)
		assignID(src)
		return src, nil

	case *pages.DataViewSource:
		src := genPg.NewDataViewSource()
		src.SetForceFullObjects(false)
		if d.ID != "" {
			src.SetID(element.ID(d.ID))
		}
		if d.EntityName != "" {
			ref := genDm.NewDirectEntityRef()
			assignID(ref)
			ref.SetEntityQualifiedName(d.EntityName)
			src.SetEntityRef(ref)
		}
		if d.ParameterName != "" {
			pv := genPg.NewPageVariable()
			assignID(pv)
			if d.IsSnippetParameter {
				pv.SetSnippetParameterQualifiedName(d.ParameterName)
			} else {
				pv.SetPageParameterQualifiedName(d.ParameterName)
			}
			src.SetSourceVariable(pv)
		}
		assignID(src)
		return src, nil

	case *pages.ListenToWidgetSource:
		src := genPg.NewListenTargetSource()
		if d.ID != "" {
			src.SetID(element.ID(d.ID))
		}
		assignID(src)
		src.SetForceFullObjects(false)
		src.SetListenTarget(d.WidgetName)
		return src, nil

	case *pages.MicroflowSource:
		ms := genPg.NewMicroflowSource()
		if d.ID != "" {
			ms.SetID(element.ID(d.ID))
		}
		assignID(ms)
		ms.SetForceFullObjects(false)
		ms.SetMicroflowSettings(microflowSettingsToGen(d.Microflow))
		return ms, nil

	case *pages.AssociationSource:
		// A DataView showing a to-one referenced object ("data from context over
		// an association") is a Forms$DataViewSource whose EntityRef is an
		// IndirectEntityRef navigating the association — NOT a Forms$AssociationSource
		// (which MxBuild rejects on a plain DataView: CE6705). Only list-producing
		// widgets use AssociationSource.
		return dataViewContextAssociationSourceToGen(d), nil

	default:
		return nil, fmt.Errorf("CreatePage: DataView source %T not yet supported by the modelsdk engine — rerun with MXCLI_ENGINE=legacy", ds)
	}
}

// dataViewContextAssociationSourceToGen builds a Forms$DataViewSource for a
// DataView bound to a to-one referenced object over an association (Studio Pro's
// "Data from context"). The association navigation rides in the DataViewSource's
// EntityRef as a DomainModels$IndirectEntityRef of EntityRefStep hops; the
// optional page-parameter SourceVariable provides the context object
// ($currentObject → none). Mirrors associationSourceToGen but wraps the same
// IndirectEntityRef in a DataViewSource instead of an AssociationSource.
func dataViewContextAssociationSourceToGen(d *pages.AssociationSource) element.Element {
	src := genPg.NewDataViewSource()
	if d.ID != "" {
		src.SetID(element.ID(d.ID))
	}
	assignID(src)
	src.SetForceFullObjects(false)
	parts := strings.SplitN(d.EntityPath, "/", 2)
	step := genDm.NewEntityRefStep()
	assignID(step)
	step.SetAssociationQualifiedName(parts[0])
	if len(parts) == 2 {
		step.SetDestinationEntityQualifiedName(parts[1])
	}
	ref := genDm.NewIndirectEntityRef()
	assignID(ref)
	ref.AddSteps(step)
	src.SetEntityRef(ref)
	if d.ContextVariable != "" {
		pv := genPg.NewPageVariable()
		assignID(pv)
		pv.SetPageParameterQualifiedName(d.ContextVariable)
		src.SetSourceVariable(pv)
	}
	return src
}

// listViewSourceToGen builds a ListView data source. A database source becomes a
// Forms$ListViewXPathSource (EntityRef + XPathConstraint + a GridSortBar and a
// ListViewSearch sub-element, matching what Studio Pro/the legacy writer emit);
// a microflow source uses Forms$MicroflowSource (as DataView does).
func listViewSourceToGen(ds pages.DataSource) (element.Element, error) {
	switch d := ds.(type) {
	case *pages.DatabaseSource:
		src := genPg.NewListViewXPathSource()
		if d.ID != "" {
			src.SetID(element.ID(d.ID))
		}
		assignID(src)
		src.SetForceFullObjects(false)
		src.SetXPathConstraint(d.XPathConstraint)
		if d.EntityName != "" {
			ref := genDm.NewDirectEntityRef()
			assignID(ref)
			ref.SetEntityQualifiedName(d.EntityName)
			src.SetEntityRef(ref)
		}
		bar := genPg.NewGridSortBar()
		assignID(bar)
		for _, s := range d.Sorting {
			item := genPg.NewGridSortItem()
			assignID(item)
			item.SetSortDirection(string(s.Direction))
			if ref := attributeRefToGen(s.AttributePath); ref != nil {
				item.SetAttributeRef(ref)
			}
			bar.AddSortItems(item)
		}
		src.SetSortBar(bar)
		search := genPg.NewListViewSearch()
		assignID(search)
		src.SetSearch(search)
		return src, nil
	case *pages.MicroflowSource:
		ms := genPg.NewMicroflowSource()
		if d.ID != "" {
			ms.SetID(element.ID(d.ID))
		}
		assignID(ms)
		ms.SetForceFullObjects(false)
		ms.SetMicroflowSettings(microflowSettingsToGen(d.Microflow))
		return ms, nil
	case *pages.AssociationSource:
		return associationSourceToGen(d), nil
	default:
		return nil, fmt.Errorf("CreatePage: ListView source %T not yet supported by the modelsdk engine — rerun with MXCLI_ENGINE=legacy", ds)
	}
}

// customWidgetDataSourceToGen builds the data source embedded in a pluggable
// widget (DataGrid2, Gallery, …) — the CustomWidgets$CustomWidgetXPathSource for
// a database (XPath) source, or Forms$MicroflowSource for a microflow source.
// Mirrors sdk/mpr.SerializeCustomWidgetDataSource. Nanoflow and association
// sources are refused loudly (their gen shapes are not yet verified).
func customWidgetDataSourceToGen(ds pages.DataSource) (element.Element, error) {
	switch d := ds.(type) {
	case nil:
		return nil, nil

	case *pages.DatabaseSource:
		src := genCw.NewCustomWidgetXPathSource()
		if d.ID != "" {
			src.SetID(element.ID(d.ID))
		}
		assignID(src)
		src.SetForceFullObjects(false)
		src.SetXPathConstraint(d.XPathConstraint)
		if d.EntityName != "" {
			ref := genDm.NewDirectEntityRef()
			assignID(ref)
			ref.SetEntityQualifiedName(d.EntityName)
			src.SetEntityRef(ref)
		}
		bar := genPg.NewGridSortBar()
		assignID(bar)
		for _, s := range d.Sorting {
			item := genPg.NewGridSortItem()
			assignID(item)
			item.SetSortDirection(string(s.Direction))
			if ref := attributeRefToGen(s.AttributePath); ref != nil {
				item.SetAttributeRef(ref)
			}
			bar.AddSortItems(item)
		}
		src.SetSortBar(bar)
		return src, nil

	case *pages.MicroflowSource:
		ms := genPg.NewMicroflowSource()
		if d.ID != "" {
			ms.SetID(element.ID(d.ID))
		}
		assignID(ms)
		ms.SetForceFullObjects(false)
		ms.SetMicroflowSettings(microflowSettingsToGen(d.Microflow))
		return ms, nil

	case *pages.AssociationSource:
		return associationSourceToGen(d), nil

	default:
		return nil, fmt.Errorf("modelsdk: pluggable widget data source %T not yet supported — rerun with MXCLI_ENGINE=legacy", ds)
	}
}

// associationSourceToGen builds a Forms$AssociationSource: an IndirectEntityRef
// with one EntityRefStep (Association[/DestinationEntity]) and an optional
// page-parameter source variable ($currentObject leaves it empty). This is a
// universal page data-source type — shared by DataView, ListView, and pluggable
// widgets — mirroring the legacy sdk/mpr.serializeAssociationSource.
func associationSourceToGen(d *pages.AssociationSource) element.Element {
	src := genPg.NewAssociationSource()
	if d.ID != "" {
		src.SetID(element.ID(d.ID))
	}
	assignID(src)
	src.SetForceFullObjects(false)
	parts := strings.SplitN(d.EntityPath, "/", 2)
	step := genDm.NewEntityRefStep()
	assignID(step)
	step.SetAssociationQualifiedName(parts[0])
	if len(parts) == 2 {
		step.SetDestinationEntityQualifiedName(parts[1])
	}
	ref := genDm.NewIndirectEntityRef()
	assignID(ref)
	ref.AddSteps(step)
	src.SetEntityRef(ref)
	if d.ContextVariable != "" {
		pv := genPg.NewPageVariable()
		assignID(pv)
		pv.SetPageParameterQualifiedName(d.ContextVariable)
		src.SetSourceVariable(pv)
	}
	return src
}

// microflowSettingsToGen builds the Forms$MicroflowSettings shared by the
// microflow DataView source and the call-microflow action.
func microflowSettingsToGen(microflowName string) element.Element {
	s := genPg.NewMicroflowSettings()
	assignID(s)
	s.SetAsynchronous(false)
	s.SetFormValidations("All")
	s.SetMicroflowQualifiedName(microflowName)
	s.SetProgressBar("None")
	return s
}

// formSettingsToGen builds the Forms$FormSettings (PageSettings) shared by the
// page-opening actions: target page by-name, empty parameter mappings, empty
// title override.
func formSettingsToGen(pageName string) element.Element {
	ps := genPg.NewPageSettings()
	assignID(ps)
	ps.SetPageQualifiedName(pageName)
	ps.SetTitleOverride(emptyTextTemplateToGen())
	return ps
}

// emptyTextTemplateToGen builds an empty Microflows$TextTemplate. This is the type
// of FormSettings/PageSettings TitleOverride (NOT a Forms$ClientTemplate, which
// Studio Pro's loader rejects with a type-cast error) and it must be non-nil
// (issue #468). Mirrors sdk/mpr.emptyTextTemplate.
func emptyTextTemplateToGen() element.Element {
	tt := genMf.NewTextTemplate()
	assignID(tt)
	tt.SetText(genTexts.NewText())
	return tt
}

// clientActionToGen converts a widget client action. Simple actions are supported;
// the page/microflow/nanoflow/create-object actions (which carry settings sub-
// objects) are refused loudly for now.
func clientActionToGen(a pages.ClientAction) (element.Element, error) {
	switch x := a.(type) {
	case nil, *pages.NoClientAction:
		return noActionGen(), nil
	case *pages.SaveChangesClientAction:
		g := genPg.NewSaveChangesClientAction()
		assignID(g)
		g.SetClosePage(x.ClosePage)
		g.SetSyncAutomatically(true)
		return g, nil
	case *pages.CancelChangesClientAction:
		g := genPg.NewCancelChangesClientAction()
		assignID(g)
		g.SetClosePage(x.ClosePage)
		return g, nil
	case *pages.ClosePageClientAction:
		g := genPg.NewClosePageClientAction()
		assignID(g)
		return g, nil
	case *pages.DeleteClientAction:
		g := genPg.NewDeleteClientAction()
		assignID(g)
		g.SetClosePage(x.ClosePage)
		return g, nil
	case *pages.PageClientAction:
		// show_page → Forms$FormAction with a Forms$FormSettings (PageSettings).
		g := genPg.NewPageClientAction()
		if x.ID != "" {
			g.SetID(element.ID(x.ID))
		}
		assignID(g)
		g.SetDisabledDuringExecution(true)
		g.SetNumberOfPagesToClose2("")
		g.SetPageSettings(formSettingsToGen(x.PageName))
		return g, nil
	case *pages.SetTaskOutcomeClientAction:
		g := genPg.NewSetTaskOutcomeClientAction()
		if x.ID != "" {
			g.SetID(element.ID(x.ID))
		}
		assignID(g)
		g.SetClosePage(x.ClosePage)
		g.SetCommit(x.Commit)
		g.SetDisabledDuringExecution(true)
		g.SetOutcomeValue(x.OutcomeValue)
		return g, nil
	case *pages.MicroflowClientAction:
		// call_microflow → Forms$MicroflowAction.
		g := genPg.NewMicroflowClientAction()
		if x.ID != "" {
			g.SetID(element.ID(x.ID))
		}
		assignID(g)
		g.SetDisabledDuringExecution(true)
		g.SetMicroflowSettings(microflowSettingsToGen(x.MicroflowName))
		return g, nil
	case *pages.CreateObjectClientAction:
		// create_object → Forms$CreateObjectClientAction (entity ref + page settings).
		g := genPg.NewCreateObjectClientAction()
		if x.ID != "" {
			g.SetID(element.ID(x.ID))
		}
		assignID(g)
		g.SetDisabledDuringExecution(true)
		g.SetNumberOfPagesToClose2("")
		if x.EntityName != "" {
			ref := genDm.NewDirectEntityRef()
			assignID(ref)
			ref.SetEntityQualifiedName(x.EntityName)
			g.SetEntityRef(ref)
		}
		g.SetPageSettings(formSettingsToGen(x.PageName))
		return g, nil
	default:
		return nil, fmt.Errorf("CreatePage: client action %T not yet supported by the modelsdk engine — rerun with MXCLI_ENGINE=legacy", a)
	}
}

// orDefaultStr returns s, or def when s is empty.
func orDefaultStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
