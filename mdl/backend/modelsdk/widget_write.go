// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genCw "github.com/mendixlabs/mxcli/modelsdk/gen/customwidgets"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	genEnum "github.com/mendixlabs/mxcli/modelsdk/gen/enumerations"
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
	// An AttributeRef always carries EntityRef: it is null for a plain binding
	// and an IndirectEntityRef when the attribute is reached over associations.
	// Measured 338 of 338 across the 67 pages of ako/TestApp at 11.14.0 (313
	// null, 25 navigated). mxcli only ever set it on the navigated branch, so a
	// rewrite dropped the key from every plain one (ako/mxcli#541). The default
	// applies only when the field was not otherwise set, so it cannot flatten a
	// navigated ref.
	codec.RegisterTypeDefaults("DomainModels$AttributeRef", codec.TypeDefaults{
		NullFields: []string{"EntityRef"},
	})
	// A PageVariable names its source in exactly one of four fields and Studio
	// Pro writes all six keys regardless. mxcli sets whichever one carries the
	// value, leaving the rest at Go's zero value, never marked dirty, and so
	// omitted. Registering them here covers all three construction sites and
	// any future one, which setting every field at each site would not.
	// A Delete action that deletes its context object stores SourceVariable
	// null — 13 of 13 in ako/TestApp — and mxcli omitted the key; one that
	// deletes a grid's selection stores a PageVariable there (ako/mxcli#1932).
	// The property is 10.4+; below that the null is stripped.
	codec.RegisterTypeDefaults("Forms$DeleteClientAction", codec.TypeDefaults{
		NullFields: []string{"SourceVariable"},
	})
	codec.RegisterTypeDefaults("Forms$PageVariable", codec.TypeDefaults{
		EmptyStringFields: []string{"LocalVariable", "PageParameter", "SnippetParameter", "SubKey", "Widget"},
		FalseFields:       []string{"UseAllPages"},
	})
	// A compound design property's nested Properties list is marker 2, though it
	// holds the same Forms$DesignPropertyValue children as the Appearance's
	// DesignProperties list (marker 3) — so it is keyed on the owner, not the
	// child type. Measured 373 of 373 across Studio Pro-authored pages, layouts,
	// building blocks and page templates in a Mendix 11.13.0 app.
	codec.RegisterPropertyListMarker("Forms$CompoundDesignPropertyValue", "Properties", 2)
	// A ClientTemplate's Parameters list is always emitted with marker 2, even empty
	// (unusual — most empty lists are marker 3).
	codec.RegisterTypeDefaults("Forms$ClientTemplate", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Parameters": 2},
	})
	// A widget action's PageSettings never overrides the opened page's title — there
	// is no MDL syntax for it — so TitleOverride is always null. It used to be written
	// as an empty Microflows$TextTemplate, which overrides the title with the empty
	// string: every popup an mxcli-authored button opened showed a blank caption with
	// only the close button (#812).
	codec.RegisterTypeDefaults("Forms$PageSettings", codec.TypeDefaults{
		NullFields: []string{"TitleOverride"},
	})
	// A pluggable-widget XPath datasource (DataGrid2 / Gallery / …) always serializes
	// SourceVariable as null when unbound, and its GridSortBar always emits an (empty)
	// SortItems list with marker 2. An older embedded template extracted before these
	// existed omits them, so a project whose installed widget expects them (e.g. Data
	// Widgets updated past the 11.6 template — Gallery@10.24) reports CE0463. Registering
	// the defaults makes the emitted datasource match the installed widget. (Axis 2 of
	// the object-list drift — see PROPOSAL_multi_version_pluggable_widgets.md.)
	codec.RegisterTypeDefaults("CustomWidgets$CustomWidgetXPathSource", codec.TypeDefaults{
		NullFields: []string{"SourceVariable"},
	})
	// Studio Pro 9's pluggable "Database" source always stores its constraint
	// list, marker 2 whether empty or not: 10 of 10 in the FSM template at
	// 9.24.2 (mendixlabs/mxcli#1399).
	codec.RegisterTypeDefaults("CustomWidgets$CustomWidgetDatabaseSource", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"DatabaseConstraints": 2},
	})
	codec.RegisterPropertyListMarker("CustomWidgets$CustomWidgetDatabaseSource", "DatabaseConstraints", 2)
	codec.RegisterTypeDefaults("Forms$GridSortBar", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"SortItems": 2},
	})
	// Populated too: 164 of 164 GridSortBar.SortItems in the Studio Pro-authored
	// fixtures (TestApp, PedApp) are marker 2, empty or not. The child type's
	// default was 3, so every sorted list view described and re-executed was
	// rewritten (ako/mxcli#721 L5).
	codec.RegisterPropertyListMarker("Forms$GridSortBar", "SortItems", 2)
	// A native ListView's Search (Forms$ListViewSearch) always carries a (usually
	// empty) SearchRefs list — Studio Pro serializes it even with search disabled.
	// The codec encoder omits an empty, never-Set PartList on a freshly-created
	// element, so without this default listViewSourceToGen emits the Search node as
	// {$ID,$Type} only, with SearchRefs absent. The Mendix client reads
	// search.searchRefs.length; a missing list surfaces as undefined and throws
	// "Cannot read properties of undefined (reading 'length')" in
	// retrieveByXPath/processResult once search is exercised. (searchPaths was
	// removed in 7.11.0; searchRefs is the valid list on 11.x.)
	codec.RegisterTypeDefaults("Forms$ListViewSearch", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"SearchRefs": 2},
	})
	// Marker 2, empty or not: 44 of 44 in the Studio Pro-authored fixtures; the
	// empty list used to take the encoder's default 3 (ako/mxcli#721 L5).
	codec.RegisterPropertyListMarker("Forms$ListViewSearch", "SearchRefs", 2)
	// A List View's XPath source always carries its SourceVariable key, null
	// when the source is not reached from a parameter: 44 of 44 in the
	// fixtures, as on the pluggable XPath source above (ako/mxcli#721 L5).
	// EntityRef likewise: a List View with no entity configured yet stores the
	// key null — every such List View in the testdata/testapp page templates —
	// and mxcli omitted it (ako/mxcli#2183).
	codec.RegisterTypeDefaults("Forms$ListViewXPathSource", codec.TypeDefaults{
		NullFields: []string{"EntityRef", "SourceVariable"},
	})
	// An association source likewise: 40 of 40 in FactoryManagement carry the
	// key, null (30) when the source starts from $currentObject (ako/mxcli#2106).
	codec.RegisterTypeDefaults("Forms$AssociationSource", codec.TypeDefaults{
		NullFields: []string{"SourceVariable"},
	})
	// LayoutGrid and its rows carry a null ConditionalVisibilitySettings; the grid,
	// rows, and columns all use the typed-array marker 2 in their parent lists.
	for _, t := range []string{"Forms$LayoutGrid", "Forms$LayoutGridRow"} {
		codec.RegisterTypeDefaults(t, codec.TypeDefaults{
			NullFields: []string{"ConditionalVisibilitySettings"},
		})
	}
	codec.RegisterListMarker("Forms$LayoutGrid", 2)
	// A navigation-list item carries a null ConditionalVisibilitySettings: six of
	// six in ako/TestApp at 11.14.0 (ako/mxcli#950).
	codec.RegisterTypeDefaults("Forms$NavigationListItem", codec.TypeDefaults{
		NullFields: []string{"ConditionalVisibilitySettings"},
	})
	codec.RegisterListMarker("Forms$LayoutGridRow", 2)
	codec.RegisterListMarker("Forms$LayoutGridColumn", 2)
	// ActionButton: null Icon/visibility/accessibility slots; marker 2 as a widget.
	codec.RegisterTypeDefaults("Forms$ActionButton", codec.TypeDefaults{
		NullFields: []string{"Icon", "ConditionalVisibilitySettings", "NativeAccessibilitySettings"},
	})
	codec.RegisterListMarker("Forms$ActionButton", 2)
	// SidebarToggleButton: null Icon/visibility slots; marker 2 as a widget.
	// Measured on Atlas_Core.Atlas_Default (11.13.0, mendixlabs/mxcli#1220).
	codec.RegisterTypeDefaults("Forms$SidebarToggleButton", codec.TypeDefaults{
		NullFields: []string{"Icon", "ConditionalVisibilitySettings"},
	})
	codec.RegisterListMarker("Forms$SidebarToggleButton", 2)
	// A native layout's content: RightHeaderPlaceholder is null when unset
	// (NativePhone_FullPage) and SidebarWidgets always stored, marker 2 —
	// 5 of 5 native Atlas_Core layouts (11.13.0).
	codec.RegisterTypeDefaults("Forms$NativeLayoutContent", codec.TypeDefaults{
		NullFields:           []string{"RightHeaderPlaceholder"},
		MandatoryListMarkers: map[string]int32{"SidebarWidgets": 2},
	})
	codec.RegisterPropertyListMarker("Forms$NativeLayoutContent", "SidebarWidgets", 2)
	// Header: both widget lists always stored, marker 2 — 9 of 9 in Atlas_Core
	// (11.13.0).
	codec.RegisterTypeDefaults("Forms$Header", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"LeftWidgets": 2, "RightWidgets": 2},
	})
	codec.RegisterPropertyListMarker("Forms$Header", "LeftWidgets", 2)
	codec.RegisterPropertyListMarker("Forms$Header", "RightWidgets", 2)
	codec.RegisterListMarker("Forms$Header", 2)
	// Title: null visibility/accessibility slots; marker 2 as a widget.
	codec.RegisterTypeDefaults("Forms$Title", codec.TypeDefaults{
		NullFields: []string{"ConditionalVisibilitySettings", "NativeAccessibilitySettings"},
	})
	codec.RegisterListMarker("Forms$Title", 2)
	// Label (Studio Pro's Label widget): a null visibility slot — the one it has;
	// no NativeAccessibilitySettings — and marker 2 as a widget. Measured on the
	// three Forms$Label in a stock Administration + Feedback project (11.13.0).
	codec.RegisterTypeDefaults("Forms$Label", codec.TypeDefaults{
		NullFields: []string{"ConditionalVisibilitySettings"},
	})
	codec.RegisterListMarker("Forms$Label", 2)
	// Conditional visibility/editability settings (issue #627). When a widget
	// carries one, applyWidgetBase emits the node; these defaults fill the
	// sub-fields Studio Pro writes: empty-string Attribute, null SourceVariable, and
	// empty (marker-3) Conditions/ModuleRoles lists. Expression/IgnoreSecurity are
	// set from the model. ExpressionModel is intentionally omitted (legacy omits it).
	// Attribute is "" (not null): it is a BY_NAME AttributeIdentifier and Mendix
	// 11.12's reader rejects a null there (StorageLoadException, "not a valid
	// AttributeIdentifier").
	// Markers measured on all 12 conditional-visibility settings in a stock
	// Administration + Feedback project (Mendix 11.13.0): Conditions is [2] and
	// ModuleRoles [1], empty or not. They were written as the default [3].
	codec.RegisterTypeDefaults("Forms$ConditionalVisibilitySettings", codec.TypeDefaults{
		NullFields:           []string{"SourceVariable"},
		EmptyStringFields:    []string{"Attribute"},
		MandatoryListMarkers: map[string]int32{"Conditions": 2, "ModuleRoles": 1},
	})
	codec.RegisterListMarker("Enumerations$Condition", 2)
	// Conditions is [2] here too, empty: measured on both Studio Pro-authored
	// editability settings in TestApp (WorkflowCommons combo boxes); the default
	// [3] rewrote them on every describe → exec.
	codec.RegisterTypeDefaults("Forms$ConditionalEditabilitySettings", codec.TypeDefaults{
		NullFields:           []string{"SourceVariable"},
		EmptyStringFields:    []string{"Attribute"},
		MandatoryListMarkers: map[string]int32{"Conditions": 2},
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
	//
	// Both mapping lists are always emitted. The markers are measured, not
	// assumed: across ako/TestApp's 67 pages at 11.14.0, ParameterMappings
	// carries marker 2 on 220 of 220 lists in every parent type (empty or
	// populated), and OutputMappings is present on 91 of 91 MicroflowSettings
	// with marker 3 and no items. MandatoryLists emits the encoder's default 3,
	// so ParameterMappings needs the explicit marker (ako/mxcli#550).
	codec.RegisterTypeDefaults("Forms$MicroflowSettings", codec.TypeDefaults{
		MandatoryLists:       []string{"OutputMappings"},
		MandatoryListMarkers: map[string]int32{"ParameterMappings": 2},
		NullFields:           []string{"ProgressMessage", "ConfirmationInfo"},
	})
	// A nanoflow client action carries its (possibly empty) parameter-mapping
	// list directly and nulls its progress/confirmation slots. Bug 2.
	//
	// OutputMappings, marker 3 and empty, is on every one Studio Pro stores:
	// 60 of 60 in ako/TestApp and 18 of 18 in PedApp at 11.14.0 — buttons and
	// the navigation menu item alike. Its absence made a describe -> exec of a
	// nanoflow menu item a rewrite (ako/mxcli#980).
	codec.RegisterTypeDefaults("Forms$CallNanoflowClientAction", codec.TypeDefaults{
		MandatoryLists:       []string{"OutputMappings"},
		MandatoryListMarkers: map[string]int32{"ParameterMappings": 2},
		NullFields:           []string{"ProgressMessage", "ConfirmationInfo"},
	})
	// Populated too: 78 of 78 Forms$CallNanoflowClientAction.ParameterMappings
	// in ako/TestApp and PedApp are marker 2, six of them holding mappings. The
	// child type (Forms$NanoflowParameterMapping) has no marker of its own, so
	// a populated list took the default, 3 (ako/mxcli#1640).
	codec.RegisterPropertyListMarker("Forms$CallNanoflowClientAction", "ParameterMappings", 2)
	// The workflow events' unset slots (ako/mxcli#832): a workflow call with
	// no confirmation, and an admin-page action with no default page.
	codec.RegisterTypeDefaults("Forms$CallWorkflowClientAction", codec.TypeDefaults{
		NullFields: []string{"ConfirmationInfo"},
	})
	codec.RegisterTypeDefaults("Forms$OpenWorkflowClientAction", codec.TypeDefaults{
		EmptyStringFields: []string{"DefaultPage"},
	})
	// One return value mapping: Studio Pro writes all four slots on every row,
	// null where the row does not use them and Expression "" (ako/TestApp
	// Pages.OutputMappingButton, 11.15).
	codec.RegisterTypeDefaults("Forms$OutputMapping", codec.TypeDefaults{
		NullFields:        []string{"AttributeRef", "SourceAttributeRef", "SourceVariable"},
		EmptyStringFields: []string{"Expression"},
	})
	// A nanoflow DATA SOURCE is flat — no settings child — and carries its
	// (possibly empty) mapping list directly, marker 2. Measured: 5 of 5 Studio
	// Pro-authored Forms$NanoflowSource in Feedback v4.0.2 at 11.13.0.
	codec.RegisterTypeDefaults("Forms$NanoflowSource", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"ParameterMappings": 2},
	})
	// And populated: 21 of 21 in ako/TestApp and PedApp, eight holding mappings
	// (ako/mxcli#1640).
	codec.RegisterPropertyListMarker("Forms$NanoflowSource", "ParameterMappings", 2)
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
	// LabelTemplate is null too — Studio Pro writes the key on every CustomWidget,
	// mxcli omitted it. Measured by handing an mxcli-authored Accordion page to
	// `mx update-widgets` and diffing the unit it wrote back (upstream #931).
	codec.RegisterTypeDefaults("CustomWidgets$CustomWidget", codec.TypeDefaults{
		NullFields: []string{
			"ConditionalVisibilitySettings", "ConditionalEditabilitySettings",
			"LabelTemplate",
		},
	})
	codec.RegisterListMarker("CustomWidgets$CustomWidget", 2)
	// Every Forms$Appearance carries a DesignProperties list, emitted as the empty
	// typed-array marker [3] when the widget has no design properties. The codec
	// omits an empty, never-appended PartList, so an mxcli-authored widget had no
	// DesignProperties key at all — which is also what made ALTER STYLING's design
	// property write a silent no-op (bsonnav.DSet cannot add an absent key).
	// Same measurement as above (upstream #931).
	codec.RegisterTypeDefaults("Forms$Appearance", codec.TypeDefaults{
		MandatoryLists: []string{"DesignProperties"},
	})
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
	// FileManager (mendixlabs/mxcli#151): the InputWidget null slots it has on
	// Mendix 10/11 — no AttributeRef, SourceVariable or native accessibility,
	// since it binds no attribute and has no native counterpart. Property set
	// from the Model SDK (mendixmodelsdk 4.116.0) and the version table; no
	// Studio Pro-authored file manager was available to diff against.
	codec.RegisterTypeDefaults("Forms$FileManager", codec.TypeDefaults{
		NullFields: []string{
			"ScreenReaderLabel", "LabelTemplate",
			"ConditionalVisibilitySettings", "ConditionalEditabilitySettings",
		},
	})
	codec.RegisterListMarker("Forms$FileManager", 2)
	// NavigationList: null visibility; items + the widget itself use marker 2.
	codec.RegisterTypeDefaults("Forms$NavigationList", codec.TypeDefaults{
		NullFields: []string{"ConditionalVisibilitySettings"},
	})
	codec.RegisterListMarker("Forms$NavigationList", 2)
	codec.RegisterListMarker("Forms$NavigationListItem", 2)
	// SnippetCallWidget: NO ConditionalVisibilitySettings — the type has no such
	// property, and none of the 139 snippet calls Studio Pro stored in PedApp and
	// TestApp carries the key. Registering it as a null field added the key to
	// every snippet call written (ako/mxcli#826). The inner SnippetCall always
	// emits its (empty) ParameterMappings array.
	codec.RegisterListMarker("Forms$SnippetCallWidget", 2)
	// The three parameter-mapping child types. MandatoryListMarkers covers an
	// EMPTY list; a populated one takes its marker from the child type, and all
	// three measured 2 (ako/mxcli#550).
	codec.RegisterListMarker("Forms$MicroflowParameterMapping", 2)
	codec.RegisterListMarker("Forms$PageParameterMapping", 2)
	codec.RegisterListMarker("Forms$SnippetParameterMapping", 2)
	// A flow argument fills one of two slots and Studio Pro writes both. The
	// Variable side already writes Expression "" (bindParameterMappingValue);
	// an Expression-bound argument left Variable unset, so the key was omitted
	// and Studio Pro's Changes panel threw "do not have the same properties.
	// baseNames = Expression, Parameter, Variable; newNames = Parameter,
	// Expression" (mendixlabs/mxcli#1317, the #1180 class).
	codec.RegisterTypeDefaults("Forms$MicroflowParameterMapping", codec.TypeDefaults{
		NullFields: []string{"Variable"},
	})
	codec.RegisterTypeDefaults("Forms$NanoflowParameterMapping", codec.TypeDefaults{
		NullFields: []string{"Variable"},
	})
	codec.RegisterTypeDefaults("Forms$SnippetCall", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"ParameterMappings": 2},
	})
	// ListView: null visibility; always emits its Templates list; marker 2.
	codec.RegisterTypeDefaults("Forms$ListView", codec.TypeDefaults{
		NullFields:           []string{"ConditionalVisibilitySettings"},
		MandatoryListMarkers: map[string]int32{"Templates": 2},
	})
	codec.RegisterListMarker("Forms$ListView", 2)
	// Templates is marker 2, empty or not — 45 of 45 in the fixtures; the empty
	// list used to take the encoder's default 3 (ako/mxcli#721 L5).
	codec.RegisterPropertyListMarker("Forms$ListView", "Templates", 2)
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
	// The single Forms$FormSettings registration — shared by the widget-action and
	// microflow show-page paths. Keep it here and nowhere else: RegisterTypeDefaults
	// overwrites rather than merges, so a second registration for the same $Type
	// silently wins by init order. That is what hid #812 — a NullFields entry added
	// next to the microflow writer was clobbered by this one.
	//
	// TitleOverride is null unless the action really overrides the opened page's
	// title. An empty Microflows$TextTemplate is not "no override" — it overrides with
	// the empty string, blanking the caption of every popup mxcli authored (#812).
	// showPageFormSettingsToGen emits the part only when OverridePageTitle is set.
	codec.RegisterTypeDefaults("Forms$FormSettings", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"ParameterMappings": 2},
		NullFields:           []string{"TitleOverride"},
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
				capText = &model.Text{Translations: map[string]string{model.AuthoringLanguage(): tp.Name}}
			}
			tpg.SetCaption(captionToGen(capText))
			// A tab page's own visibility condition (mendixlabs/mxcli#1465);
			// null otherwise, as Studio Pro writes it.
			if tp.ConditionalVisibility != nil {
				tpg.SetConditionalVisibilitySettings(conditionalVisibilityToGen(tp.ConditionalVisibility))
			}
			// Its badge, a client template like a dynamic text's content
			// (mendixlabs/mxcli#1644); null otherwise, as Studio Pro writes it.
			if tp.Badge != nil {
				tpg.SetBadge(clientTemplateToGen(tp.Badge))
			}
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
		g.SetNativeTextStyle(orDefaultStr(string(x.NativeTextStyle), string(pages.NativeTextStyleText)))
		g.SetContent(clientTemplateToGen(x.Content))
		return g, nil

	case *pages.LayoutGrid:
		g := genPg.NewLayoutGrid()
		applyWidgetBase(g, &x.BaseWidget)
		// FixedWidth was rewritten to the default on every describe → exec
		// (ako/mxcli-ledger FINDINGS #170). Empty is the default.
		g.SetWidth(orDefaultStr(x.Width, pages.DefaultLayoutGridWidth))
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
		g.SetEditability(dataViewEditability(x))
		// Control is the DataView default, measured 47 of 56 across ako/TestApp's
		// 67 pages with not one Inherit — so it is NOT the "Inherit" every other
		// input widget uses. An authored or carried value wins (ako/mxcli#550).
		if x.ReadOnlyStyle != "" {
			g.SetReadOnlyStyle(x.ReadOnlyStyle)
		} else {
			g.SetReadOnlyStyle("Control")
		}
		g.SetShowFooter(x.ShowFooter)
		// Always emit LabelWidth. It carries Studio Pro's "Form orientation" radio,
		// which has no BSON field of its own — so writing it only when an explicit
		// `LabelWidth:` was given dropped `FormOrientation: Vertical` entirely
		// (mendixlabs/mxcli#762). The resolution rule lives on the model, shared with
		// the legacy writer.
		g.SetLabelWidth(int32(x.ResolvedLabelWidth()))
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
		// No Caption: a Title widget shows its page's title, and Forms$Title
		// declares no caption (generated/metamodel PagesTitle; `mx convert`
		// strips one on 10.24 and 11.14 alike). Writing it left a key Mendix's
		// merge engine cannot compare once Studio Pro saves the page
		// (mendixlabs/mxcli#1373) — and never changed what the page showed.
		g := genPg.NewTitle()
		applyWidgetBase(g, &x.BaseWidget)
		return g, nil

	case *pages.Label:
		// Studio Pro's Label widget. Stored with exactly Appearance, Caption,
		// ConditionalVisibilitySettings, Name and TabIndex — measured on the
		// three in a stock Administration v4.3.2 + Feedback v4.0.2 project at
		// 11.13.0. gen's Label also declares top-level Class/Style and
		// AccessibilitySettings; they are left unset, so not written.
		g := genPg.NewLabel()
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
		if ref := inputAttributeRefToGen(x.AttributePath, x.AttributeRefSteps); ref != nil {
			g.SetAttributeRef(ref)
		}
		if sv := inputSourceVariableToGen(x.SourceVariable); sv != nil {
			g.SetSourceVariable(sv)
		}
		g.SetEditable(pages.WidgetEditability(&x.BaseWidget))
		g.SetFormattingInfo(formattingInfoToGen(x.FormattingInfo))
		g.SetInputMask("")
		g.SetIsPasswordBox(x.IsPassword)
		g.SetKeyboardType(orDefaultStr(x.KeyboardType, "Default")) // ako/mxcli#1209
		if x.Label != "" {
			g.SetLabelTemplate(textAsClientTemplate(textFromString(x.Label)))
		}
		g.SetMaxLengthCode(maxLengthCodeFor(x.SourceVariable))
		onChange, err := clientActionToGen(x.OnChangeAction)
		if err != nil {
			return nil, err
		}
		onEnter, err := clientActionToGen(x.OnEnterAction)
		if err != nil {
			return nil, err
		}
		// Studio Pro's "On enter key press". A NoAction was hard-coded here,
		// so the action could not be written and describe → exec replaced a
		// stored one with nothing (ako/mxcli#1743). nil still writes the
		// NoAction.
		onEnterKey, err := clientActionToGen(x.OnEnterKeyPressAction)
		if err != nil {
			return nil, err
		}
		onLeave, err := clientActionToGen(x.OnLeaveAction)
		if err != nil {
			return nil, err
		}
		g.SetOnChangeAction(onChange)
		g.SetOnEnterAction(onEnter)
		g.SetOnEnterKeyPressAction(onEnterKey)
		g.SetOnLeaveAction(onLeave)
		g.SetPlaceholderTemplate(textAsClientTemplate(x.Placeholder))
		g.SetReadOnlyStyle(orDefaultStr(x.ReadOnlyStyle, "Inherit")) // ako/mxcli#977
		g.SetSubmitBehaviour(orDefaultStr(x.SubmitBehaviour, pages.DefaultSubmitBehaviour))
		g.SetSubmitOnInputDelay(submitOnInputDelay(x.SubmitOnInputDelay))
		g.SetValidation(widgetValidationToGenWith(x.ValidationExpression, x.ValidationMessage))
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
		// Icon (issue #602): overrides the null-Icon TypeDefault when set.
		// iconToGen returns nil for an icon that identifies nothing, and that nil
		// must not be handed to SetIcon — a typed nil there is an Icon property
		// holding a broken element rather than the null the TypeDefault gives.
		if x.Icon != nil {
			if icon := iconToGen(x.Icon); icon != nil {
				g.SetIcon(icon)
			}
		}
		act, err := clientActionToGen(x.Action)
		if err != nil {
			return nil, err
		}
		g.SetAction(act)
		return g, nil

	case *pages.CheckBox:
		g := genPg.NewCheckBox()
		applyWidgetBase(g, &x.BaseWidget)
		if ref := inputAttributeRefToGen(x.AttributePath, x.AttributeRefSteps); ref != nil {
			g.SetAttributeRef(ref)
		}
		if sv := inputSourceVariableToGen(x.SourceVariable); sv != nil {
			g.SetSourceVariable(sv)
		}
		g.SetEditable(pages.WidgetEditability(&x.BaseWidget))
		if x.Label != "" {
			g.SetLabelTemplate(textAsClientTemplate(textFromString(x.Label)))
		}
		if x.LabelPosition != "" {
			g.SetLabelPosition(x.LabelPosition)
		}
		onChangeCB, err := clientActionToGen(x.OnChangeAction)
		if err != nil {
			return nil, err
		}
		g.SetOnChangeAction(onChangeCB)
		enterCB, leaveCB, err := focusEventsToGen(x.OnEnterAction, x.OnLeaveAction)
		if err != nil {
			return nil, err
		}
		g.SetOnEnterAction(enterCB)
		g.SetOnLeaveAction(leaveCB)
		// Unset keeps Mendix's default; an authored Control/Text is what decides
		// whether a read-only check box renders as the glyph or as "Yes"/"No"
		// text (ako/mxcli#490). The value is canonicalised at build time.
		g.SetReadOnlyStyle(orDefaultStr(x.ReadOnlyStyle, "Inherit"))
		g.SetValidation(widgetValidationToGen())
		return g, nil

	case *pages.FileManager:
		g := genPg.NewFileManager()
		applyWidgetBase(g, &x.BaseWidget)
		g.SetEditable(pages.WidgetEditability(&x.BaseWidget))
		if x.Label != "" {
			g.SetLabelTemplate(textAsClientTemplate(textFromString(x.Label)))
		}
		g.SetType(string(orDefaultFileManagerType(x.Type)))
		g.SetMaxFileSize(int32(x.MaxFileSize))
		g.SetAllowedExtensions(x.AllowedExtensions)
		g.SetShowFileInBrowser(x.ShowFileInBrowser)
		return g, nil

	case *pages.TextArea:
		g := genPg.NewTextArea()
		applyWidgetBase(g, &x.BaseWidget)
		g.SetAriaRequired(false)
		g.SetAutoFocus(false)
		if ref := inputAttributeRefToGen(x.AttributePath, x.AttributeRefSteps); ref != nil {
			g.SetAttributeRef(ref)
		}
		if sv := inputSourceVariableToGen(x.SourceVariable); sv != nil {
			g.SetSourceVariable(sv)
		}
		g.SetCounterMessage(captionToGen(x.CounterMessage))
		g.SetEditable(pages.WidgetEditability(&x.BaseWidget))
		if x.Label != "" {
			g.SetLabelTemplate(textAsClientTemplate(textFromString(x.Label)))
		}
		g.SetMaxLengthCode(maxLengthCodeFor(x.SourceVariable))
		// Unset writes Studio Pro's defaults: 5 lines and Autocomplete on, as on
		// all 58 text areas in PedApp and TestApp. Autocomplete was not written
		// at all, and a stored 0 lines came back as 5 (ako/mxcli#977).
		lines := int32(5)
		if x.NumberOfLines != nil {
			lines = *x.NumberOfLines
		}
		g.SetNumberOfLines(lines)
		g.SetAutocomplete(x.Autocomplete == nil || *x.Autocomplete)
		onChangeTA, err := clientActionToGen(x.OnChangeAction)
		if err != nil {
			return nil, err
		}
		g.SetOnChangeAction(onChangeTA)
		enterTA, leaveTA, err := focusEventsToGen(x.OnEnterAction, x.OnLeaveAction)
		if err != nil {
			return nil, err
		}
		g.SetOnEnterAction(enterTA)
		g.SetOnLeaveAction(leaveTA)
		g.SetPlaceholderTemplate(textAsClientTemplate(x.Placeholder))
		g.SetReadOnlyStyle(orDefaultStr(x.ReadOnlyStyle, "Inherit")) // ako/mxcli#977
		g.SetSubmitBehaviour(orDefaultStr(x.SubmitBehaviour, pages.DefaultSubmitBehaviour))
		g.SetSubmitOnInputDelay(submitOnInputDelay(x.SubmitOnInputDelay))
		// Studio Pro stores one on every textarea; without the key a rewrite
		// had nowhere to carry the stored translations and deleted the message
		// (ako/mxcli#705). Empty, like CounterMessage above.
		g.SetTextTooLongMessage(captionToGen(nil))
		g.SetValidation(widgetValidationToGen())
		return g, nil

	case *pages.DatePicker:
		g := genPg.NewDatePicker()
		applyWidgetBase(g, &x.BaseWidget)
		g.SetAriaRequired(false)
		if ref := inputAttributeRefToGen(x.AttributePath, x.AttributeRefSteps); ref != nil {
			g.SetAttributeRef(ref)
		}
		if sv := inputSourceVariableToGen(x.SourceVariable); sv != nil {
			g.SetSourceVariable(sv)
		}
		g.SetEditable(pages.WidgetEditability(&x.BaseWidget))
		// The picker's format IS its mode: DateFormat Time or DateTime is what
		// makes it a time / date-time picker. The defaults were hard-coded here,
		// so every picker was written date-only (ako/mxcli#968).
		g.SetFormattingInfo(formattingInfoToGen(x.FormattingInfo))
		if x.Label != "" {
			g.SetLabelTemplate(textAsClientTemplate(textFromString(x.Label)))
		}
		onChangeDP, err := clientActionToGen(x.OnChangeAction)
		if err != nil {
			return nil, err
		}
		g.SetOnChangeAction(onChangeDP)
		enterDP, leaveDP, err := focusEventsToGen(x.OnEnterAction, x.OnLeaveAction)
		if err != nil {
			return nil, err
		}
		g.SetOnEnterAction(enterDP)
		g.SetOnLeaveAction(leaveDP)
		g.SetPlaceholderTemplate(textAsClientTemplate(x.Placeholder))
		g.SetReadOnlyStyle(orDefaultStr(x.ReadOnlyStyle, "Inherit")) // ako/mxcli#977
		g.SetValidation(widgetValidationToGen())
		return g, nil

	case *pages.RadioButtons:
		g := genPg.NewRadioButtonGroup()
		applyWidgetBase(g, &x.BaseWidget)
		g.SetAriaRequired(false)
		if ref := inputAttributeRefToGen(x.AttributePath, x.AttributeRefSteps); ref != nil {
			g.SetAttributeRef(ref)
		}
		if sv := inputSourceVariableToGen(x.SourceVariable); sv != nil {
			g.SetSourceVariable(sv)
		}
		g.SetEditable(pages.WidgetEditability(&x.BaseWidget))
		if x.Label != "" {
			g.SetLabelTemplate(textAsClientTemplate(textFromString(x.Label)))
		}
		g.SetReadOnlyStyle(orDefaultStr(x.ReadOnlyStyle, "Inherit")) // ako/mxcli#977
		g.SetRenderHorizontal(x.RenderDirection != pages.RenderDirectionVertical)
		onChange, err := clientActionToGen(x.OnChangeAction)
		if err != nil {
			return nil, err
		}
		g.SetOnChangeAction(onChange)
		// Studio Pro stores all three event slots on every check box, date
		// picker and radio button group (PedApp and ako/TestApp: 16, 76 and 4
		// of 4). Without OnLeaveAction a rewrite deleted the slot (#721 L2).
		enterRB, leaveRB, err := focusEventsToGen(x.OnEnterAction, x.OnLeaveAction)
		if err != nil {
			return nil, err
		}
		g.SetOnEnterAction(enterRB)
		g.SetOnLeaveAction(leaveRB)
		g.SetValidation(widgetValidationToGen())
		return g, nil

	case *pages.ScrollContainer:
		g := genPg.NewScrollContainer()
		applyWidgetBase(g, &x.BaseWidget)
		// Only when stated: unset, the key is left to Studio Pro's default
		// (Headline, canon's measured property defaults), so a layout that
		// never said it is written as before (ako/mxcli#1703).
		if x.LayoutMode != "" {
			g.SetLayoutMode(x.LayoutMode)
		}
		for _, r := range x.Regions {
			rg, err := scrollRegionToGen(r)
			if err != nil {
				return nil, err
			}
			// Five named slots, not a list. gen's setter for the centre is
			// SetCenter but the BSON key is CenterRegion — see the
			// STORAGE-NAME OVERRIDE in modelsdk/gen/pages.
			switch r.Slot {
			case pages.ScrollSlotTop:
				g.SetTop(rg)
			case pages.ScrollSlotRight:
				g.SetRight(rg)
			case pages.ScrollSlotBottom:
				g.SetBottom(rg)
			case pages.ScrollSlotLeft:
				g.SetLeft(rg)
			case pages.ScrollSlotCenter:
				g.SetCenter(rg)
			default:
				return nil, fmt.Errorf("scroll container %q: unknown region %q (want top, right, bottom, left or center)", x.Name, r.Slot)
			}
		}
		// Flat children go in the centre, which is where a container with no
		// explicit regions puts them. Both at once would silently discard one
		// of the two, so it is refused.
		if len(x.Widgets) > 0 {
			for _, r := range x.Regions {
				if r.Slot == pages.ScrollSlotCenter {
					return nil, fmt.Errorf("scroll container %q has both a center region and loose widgets; put the loose widgets in the center region", x.Name)
				}
			}
			flat := &pages.ScrollContainerRegion{Slot: pages.ScrollSlotCenter, Widgets: x.Widgets}
			rg, err := scrollRegionToGen(flat)
			if err != nil {
				return nil, err
			}
			g.SetCenter(rg)
		}
		return g, nil

	case *pages.LayoutPlaceholder:
		// A placeholder's Name is API: a page binds to it as
		// Module.Layout.<Name>, so renaming one unbinds every page that used it.
		g := genPg.NewPlaceholder()
		applyWidgetBase(g, &x.BaseWidget)
		if x.Name == "" {
			return nil, fmt.Errorf("placeholder needs a name: pages reference it as Module.Layout.<Name>")
		}
		return g, nil

	case *pages.NavigationTree:
		g := genPg.NewNavigationTree()
		applyWidgetBase(g, &x.BaseWidget)
		src, err := menuSourceToGen(x.Name, x.Menu, x.NavigationProfile)
		if err != nil {
			return nil, err
		}
		g.SetMenuSource(src)
		return g, nil

	case *pages.MenuBar:
		// Same four keys as a NavigationTree, and the same MenuSource wrapper —
		// a menu bar is the horizontal navigation a topbar carries.
		g := genPg.NewMenuBar()
		applyWidgetBase(g, &x.BaseWidget)
		src, err := menuSourceToGen(x.Name, x.Menu, x.NavigationProfile)
		if err != nil {
			return nil, err
		}
		g.SetMenuSource(src)
		return g, nil

	case *pages.SimpleMenuBar:
		// The bottom bar of Atlas's phone layouts (ako/mxcli#573). A menu bar's
		// keys plus Orientation, measured on Atlas_Core.Phone_BottomBar.
		g := genPg.NewSimpleMenuBar()
		applyWidgetBase(g, &x.BaseWidget)
		src, err := menuSourceToGen(x.Name, x.Menu, x.NavigationProfile)
		if err != nil {
			return nil, err
		}
		g.SetMenuSource(src)
		g.SetOrientation(orDefaultStr(string(x.Orientation), string(pages.MenuOrientationHorizontal)))
		return g, nil

	case *pages.LayoutHeader:
		// Forms$Header, a phone/tablet layout's header bar. gen also exposes
		// the singular LeftWidget / RightWidget, deleted in 7.15 and never set.
		g := genPg.NewHeader()
		applyWidgetBase(g, &x.BaseWidget)
		for _, w := range x.LeftWidgets {
			wg, err := widgetToGen(w)
			if err != nil {
				return nil, err
			}
			g.AddLeftWidgets(wg)
		}
		for _, w := range x.RightWidgets {
			wg, err := widgetToGen(w)
			if err != nil {
				return nil, err
			}
			g.AddRightWidgets(wg)
		}
		return g, nil

	case *pages.SidebarToggleButton:
		// An action button's face without the action (mendixlabs/mxcli#1220).
		// Studio Pro always stores a CaptionTemplate, empty on Atlas's own.
		g := genPg.NewSidebarToggleButton()
		applyWidgetBase(g, &x.BaseWidget)
		g.SetRenderType(orDefaultStr(string(x.RenderMode), "Button"))
		g.SetButtonStyle(orDefaultStr(string(x.ButtonStyle), "Default"))
		if x.CaptionTemplate != nil {
			g.SetCaption(clientTemplateToGen(x.CaptionTemplate))
		} else {
			g.SetCaption(textAsClientTemplate(nil))
		}
		g.SetTooltip(captionToGen(x.Tooltip))
		if x.Icon != nil {
			if icon := iconToGen(x.Icon); icon != nil {
				g.SetIcon(icon)
			}
		}
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
			tg.SetSpecializationQualifiedName(t.Specialization)
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
			// A snippet's own parameter, passed on from inside that snippet,
			// fills the SnippetParameter slot (ako/mxcli#721 L3).
			// A page `Variables:` entry fills LocalVariable (ako/mxcli#826).
			// A data view's object, passed by naming the data view, fills
			// Widget alone, as a flow call's widget argument does
			// (ako/mxcli#1968); in PageParameter it was CE0115.
			if pm.IsWidget {
				m.SetVariable(pageVariableToGen(strings.TrimPrefix(pm.Argument, "$"), "", ""))
				call.AddParameterMappings(m)
				continue
			}
			kind := ""
			switch {
			case pm.IsSnippetParameter:
				kind = "snippet"
			case pm.IsLocalVariable:
				kind = "local"
			}
			m.SetVariable(sourceVariableToGen(strings.TrimPrefix(pm.Argument, "$"), kind))
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

	// The widgets that used to send a user to the legacy engine, none of them
	// covered by the doctype gate — see widget_write_legacy_gaps.go.
	//
	// pages.Text (Forms$Text) is deliberately NOT here. Mendix has no such type,
	// so writing one makes the project unopenable; the keyword that built it is
	// refused (mdl/executor/validate_widget_retired.go) and nothing constructs
	// the struct any more. An old project that carries one keeps it because
	// ALTER PAGE mutates the stored gen document rather than rebuilding from the
	// semantic model — this switch is never asked about it.
	case *pages.DropDown:
		return dropDownToGen(x)

	case *pages.StaticImage:
		return staticImageToGen(x)

	case *pages.DynamicImage:
		return dynamicImageToGen(x)

	default:
		return nil, fmt.Errorf("CreatePage: widget %T is not supported by either engine — "+
			"please file an issue with the MDL that produced it", w)
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
	// No Name: Forms$NavigationListItem declares none. Studio Pro stores none
	// on any item it creates (all six in ako/TestApp at 11.14.0), and `mx
	// convert` strips one on 10.24 and 11.14 alike, so writing the MDL item name
	// left a key Mendix's merge engine cannot compare once Studio Pro saves the
	// document (mendixlabs/mxcli#1373). `mx check` is clean without it (measured
	// on 11.14.0 with two named items) — the CE7247 / CE0495 an older note
	// blamed on its absence is not reproduced.
	g.SetAppearance(newAppearance(item.Class, item.Style, item.DynamicClasses, nil))
	if item.ConditionalVisibility != nil {
		g.SetConditionalVisibilitySettings(conditionalVisibilityToGen(item.ConditionalVisibility))
	}
	act, err := clientActionToGen(item.Action)
	if err != nil {
		return nil, err
	}
	g.SetAction(act)

	widgets := item.Widgets
	if len(widgets) == 0 && item.Caption != nil {
		// The caption becomes a DynamicText — which itself needs a name, or it
		// hits the same empty-name errors. Mirror the legacy writer's `text_<name>`.
		widgets = []pages.Widget{&pages.DynamicText{
			BaseWidget: pages.BaseWidget{Name: "text_" + item.Name},
			Content:    &pages.ClientTemplate{Template: item.Caption},
			RenderMode: pages.TextRenderModeText,
		}}
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

// layoutGridRowToGen converts a LayoutGridRow (not a full widget, so no
// name/tabindex). Its appearance and alignment are carried: hardcoding them
// dropped every row's design properties and reset a centred row on each
// describe → exec. Unset values write the defaults the legacy serializer did.
func layoutGridRowToGen(row *pages.LayoutGridRow) (element.Element, error) {
	g := genPg.NewLayoutGridRow()
	if row.ID != "" {
		g.SetID(element.ID(row.ID))
	}
	assignID(g)
	g.SetAppearance(newAppearance(row.Class, row.Style, row.DynamicClasses, row.DesignProperties))
	g.SetHorizontalAlignment(alignmentOrNone(row.HorizontalAlignment))
	g.SetSpacingBetweenColumns(!row.NoSpacingBetweenColumns)
	g.SetVerticalAlignment(alignmentOrNone(row.VerticalAlignment))
	// The row's own visibility (mendixlabs/mxcli#1461). It was never set, so the
	// registered null default replaced a Studio Pro row's condition on every
	// rewrite of its page, the row then always visible.
	if row.ConditionalVisibility != nil {
		g.SetConditionalVisibilitySettings(conditionalVisibilityToGen(row.ConditionalVisibility))
	}
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
// via columnWeight); PreviewWidth is -1, Studio Pro's value for a new column —
// a rewrite carries the stored one (carryLayoutGridPreviewWidth).
func layoutGridColumnToGen(col *pages.LayoutGridColumn) (element.Element, error) {
	g := genPg.NewLayoutGridColumn()
	if col.ID != "" {
		g.SetID(element.ID(col.ID))
	}
	assignID(g)
	g.SetAppearance(newAppearance(col.Class, col.Style, col.DynamicClasses, col.DesignProperties))
	g.SetWeight(int32(columnWeight(col.Weight)))
	g.SetTabletWeight(int32(columnWeight(col.TabletWeight)))
	g.SetPhoneWeight(int32(columnWeight(col.PhoneWeight)))
	g.SetPreviewWidth(-1)
	g.SetVerticalAlignment(alignmentOrNone(col.VerticalAlignment))
	for _, w := range col.Widgets {
		wg, err := widgetToGen(w)
		if err != nil {
			return nil, err
		}
		g.AddWidgets(wg)
	}
	return g, nil
}

// alignmentOrNone maps an unset layout-grid alignment to Mendix's default.
func alignmentOrNone(a string) string {
	if a == "" {
		return "None"
	}
	return a
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
	// "Visible for module roles" (mendixlabs/mxcli#1460): by-name references,
	// independent of the expression/attribute condition beside them.
	if len(cvs.ModuleRoles) > 0 {
		g.SetModuleRolesQualifiedNames(cvs.ModuleRoles)
	}
	// "Visible: based on attribute value": the attribute plus one condition per
	// value it can hold (ako/mxcli attribute-condition visibility).
	if cvs.Attribute != "" {
		g.SetAttributeQualifiedName(cvs.Attribute)
		for _, c := range cvs.Conditions {
			cg := genEnum.NewCondition()
			assignID(cg)
			cg.SetAttributeValue(c.Value)
			cg.SetEditableVisible(c.Visible)
			g.AddConditions(cg)
		}
		// Read from a page or snippet parameter (`Visible: $Param.Attr in
		// (…)`): Studio Pro names it here, as on TestApp's
		// WorkflowCommons.Snip_UserTask_NameColumnWithIcon. Null otherwise —
		// read from the enclosing object.
		if sv := inputSourceVariableToGen(cvs.SourceVariable); sv != nil {
			g.SetSourceVariable(sv)
		}
	}
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
	// "Editable: based on attribute value" (ako/mxcli#2111): written exactly as
	// the visibility settings' attribute form above.
	if ces.Attribute != "" {
		g.SetAttributeQualifiedName(ces.Attribute)
		for _, c := range ces.Conditions {
			cg := genEnum.NewCondition()
			assignID(cg)
			cg.SetAttributeValue(c.Value)
			cg.SetEditableVisible(c.Visible)
			g.AddConditions(cg)
		}
		if sv := inputSourceVariableToGen(ces.SourceVariable); sv != nil {
			g.SetSourceVariable(sv)
		}
	}
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

// iconToGen builds a widget Icon element — one of Mendix's three, dispatched on
// the kind rather than inferred from the payload.
//
//	Forms$IconCollectionIcon{Image: QN} -> a CustomIcons$CustomIcon
//	Forms$ImageIcon{Image: QN}          -> an Images$Image (a DIFFERENT document)
//	Forms$GlyphIcon{Code: int}          -> a font code point, with no name
//
// This emitted the first unconditionally, which was fine while it was also the
// only one anyone could author and wrong the moment DESCRIBE started reading
// real projects: a stored image icon round-tripped into a custom-icon reference
// and failed the build with CE1613 (mendixlabs/mxcli#1059). The two named kinds
// are indistinguishable by payload, so nothing but the carried kind can tell
// them apart.
//
// Verified against a Studio-Pro-authored button (issue #602). None has a typed
// gen struct, so they are built as raw elements (like the workflow simple
// activities).
func iconToGen(ic *pages.Icon) element.Element {
	storage := types.MenuIconStorageType(ic.Kind)
	if storage == "" {
		// A kind this build does not know. Emitting SOME icon would be guessing
		// between variants, which is the failure that produces a document
		// Studio Pro cannot open; the builder refuses such an icon before here.
		return nil
	}
	e := newElem(storage, "")
	if ic.Kind == types.MenuIconGlyph {
		// A glyph with no code identifies no glyph. Emit no icon rather than an
		// element nobody can see.
		if ic.Code == 0 {
			return nil
		}
		addInt32(e, "Code", int32(ic.Code))
		return e
	}
	if ic.Image == "" {
		return nil
	}
	addStr(e, "Image", ic.Image)
	return e
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
	if p.SourceVariable != "" || p.SourceWidget != "" {
		g.SetSourceVariable(pageVariableToGen(p.SourceWidget, p.SourceVariable, p.SourceVariableKind))
	}
	g.SetFormattingInfo(formattingInfoToGen(p.FormattingInfo))
	return g
}

// sourceVariableToGen builds the Forms$PageVariable that names a template
// parameter's (or similar) data source. kind selects which slot the variable
// name fills: "" = page parameter, "local" = page-level Variables entry,
// "snippet" = snippet parameter.
func sourceVariableToGen(name, kind string) element.Element {
	return pageVariableToGen("", name, kind)
}

// pageVariableToGen is sourceVariableToGen with the Widget slot: a binding read
// through a data view names the data view in Widget and, beside it, the data
// view's own variable in the slot kind selects. That pair is what Studio Pro
// stores — given only {widget}, its model fills in the data view's page
// parameter (ako/mxcli#826). An empty name leaves every name slot empty.
func pageVariableToGen(widget, name, kind string) element.Element {
	pv := genPg.NewPageVariable()
	assignID(pv)
	if name != "" {
		switch kind {
		case "local":
			pv.SetLocalVariableQualifiedName(name)
		case "snippet":
			pv.SetSnippetParameterQualifiedName(name)
		default:
			pv.SetPageParameterQualifiedName(name)
		}
	}
	if widget != "" {
		pv.SetWidgetQualifiedName(widget)
	}
	return pv
}

// maxLengthCodeFor is a text input's "Maximum length": -1, the attribute's
// length, for an attribute binding; 0, unlimited, for one bound to a page
// variable, which has no attribute length to take — Studio Pro's default for
// variables, and the only valid one: mx check reports -1 there as CE6553 "Text
// length cannot be the attribute length when configured with a variable"
// (mendixlabs/mxcli#1235).
func maxLengthCodeFor(sv *pages.WidgetVariable) int32 {
	if sv != nil && sv.Kind == "local" && sv.Widget == "" && sv.Variable != "" {
		return 0
	}
	return -1
}

// inputSourceVariableToGen writes an input widget's widget-scoped
// SourceVariable, or nil — the null Studio Pro stores on an input bound to its
// enclosing data context.
func inputSourceVariableToGen(sv *pages.WidgetVariable) element.Element {
	if sv == nil || (sv.Widget == "" && sv.Variable == "") {
		return nil
	}
	return pageVariableToGen(sv.Widget, sv.Variable, sv.Kind)
}

// formattingInfoToGen builds a Forms$FormattingInfo, using the parameter's
// per-parameter formatting when present and the standard defaults otherwise. A
// nil fi reproduces the previous hardcoded defaults, so every unformatted
// parameter is byte-identical to before. TimeFormat is intentionally omitted —
// it is not a schema field and triggers CE0463.
func formattingInfoToGen(fi *pages.FormattingInfo) element.Element {
	dateFormat, customDateFormat, enumFormat := "Date", "", "Text"
	decimalPrecision := 2
	groupDigits := false
	if fi != nil {
		if fi.DateFormat != "" {
			dateFormat = fi.DateFormat
		}
		customDateFormat = fi.CustomDateFormat
		if fi.EnumFormat != "" {
			enumFormat = fi.EnumFormat
		}
		decimalPrecision = fi.DecimalPrecision
		groupDigits = fi.GroupDigits
	}
	f := genPg.NewFormattingInfo()
	assignID(f)
	f.SetCustomDateFormat(customDateFormat)
	f.SetDateFormat(dateFormat)
	f.SetDecimalPrecision(int32(decimalPrecision))
	f.SetEnumFormat(enumFormat)
	f.SetGroupDigits(groupDigits)
	return f
}

// textFromString wraps a non-empty string as a single-translation model.Text.
func textFromString(s string) *model.Text {
	if s == "" {
		return nil
	}
	return &model.Text{Translations: map[string]string{model.AuthoringLanguage(): s}}
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

// inputAttributeRefToGen builds the AttributeRef for an input widget, carrying
// association hops when the binding navigates them.
//
// Studio Pro stores an attribute-over-association binding on a plain text box —
// measured on ako/TestApp's Rules.RuleAction_NewEdit, whose textBox4 holds
// Attribute "Rules.BusinessRule.Name" with an IndirectEntityRef over
// Rules.RuleAction_BusinessRule. mxcli could read that page and not write one:
// every input builder resolved the path with resolveAttributePath, which knows
// nothing about associations, so `attribute: Assoc/Attr` produced a flat
// unresolvable path and the build failed CE1613 (ako/mxcli#529).
//
// Steps with no attribute qualified name fall through to nil the same way
// attributeRefToGen does, rather than emitting an EntityRef hanging off
// nothing.
func inputAttributeRefToGen(path string, steps []pages.AttributeRefStep) element.Element {
	if len(steps) == 0 {
		return attributeRefToGen(path)
	}
	if strings.Count(path, ".") < 2 {
		return nil
	}
	return attributeRefWithStepsToGen(path, steps)
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
	r.SetEntityRef(pageEntityRefToGen(steps))
	return r
}

// pageEntityRefToGen builds the DomainModels$IndirectEntityRef for page-side
// association hops through entityRefToGen, so a sort binding and a List View
// source cannot store the same path two ways.
func pageEntityRefToGen(steps []pages.AttributeRefStep) element.Element {
	mSteps := make([]microflows.EntityRefStep, len(steps))
	for i, s := range steps {
		mSteps[i] = microflows.EntityRefStep{Association: s.Association, DestinationEntity: s.DestinationEntity}
	}
	return entityRefToGen(mSteps)
}

// widgetValidationToGen builds the default empty Forms$WidgetValidation.
func widgetValidationToGen() element.Element {
	return widgetValidationToGenWith("", "")
}

// widgetValidationToGenWith builds a Forms$WidgetValidation carrying the widget's
// own validation: the expression Mendix evaluates over $value, and the message
// shown when it fails.
//
// The element is written either way — Studio Pro stores it on every input widget,
// empty or not — so the empty form here is the same document the unconditional
// default used to produce. What changed is that an authored expression is no
// longer overwritten by it: a rewrite used to blank the validation on every text
// box it touched, with mx check at 0 errors (ako/mxcli#550).
func widgetValidationToGenWith(expression, message string) element.Element {
	v := genPg.NewWidgetValidation()
	assignID(v)
	v.SetExpression(expression)
	v.SetMessage(captionToGen(textFromString(message)))
	return v
}

// dataViewEditability is the Forms editability enum a data view stores.
//
// The builder puts `Editable:` on the BaseWidget, as it does for every widget.
// Reading only ReadOnly, which nothing sets, wrote Always whatever was authored,
// so a read-only data view became editable on describe → exec (ako/mxcli#1676).
func dataViewEditability(x *pages.DataView) string {
	if x.ReadOnly {
		return "Never"
	}
	return pages.WidgetEditability(&x.BaseWidget)
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
		ms.SetMicroflowSettings(microflowSettingsToGen(d.Microflow, d.ParameterMappings, &pages.FlowCallSettings{FormValidations: d.FormValidations}, nil))
		return ms, nil

	// A NANOFLOW data source. Flat, unlike the microflow source above: the
	// nanoflow binds directly on the source, no settings child (CE2633) — see
	// nanoflowSourceToGen.
	case *pages.NanoflowSource:
		return nanoflowSourceToGen(d), nil

	case *pages.AssociationSource:
		// A DataView showing a to-one referenced object ("data from context over
		// an association") is a Forms$DataViewSource whose EntityRef is an
		// IndirectEntityRef navigating the association — NOT a Forms$AssociationSource
		// (which MxBuild rejects on a plain DataView: CE6705). Only list-producing
		// widgets use AssociationSource.
		return dataViewContextAssociationSourceToGen(d), nil

	default:
		return nil, fmt.Errorf("CreatePage: DataView source %T is not supported by either engine — please file an issue", ds)
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
	src.SetEntityRef(associationEntityPathToGen(d.EntityPath))
	if d.ContextVariable != "" {
		src.SetSourceVariable(associationContextVariableToGen(d))
	}
	return src
}

// associationEntityPathToGen builds the DomainModels$IndirectEntityRef of an
// association source's EntityPath: `Assoc/Dest[/Assoc/Dest…]`, one
// EntityRefStep per pair (a trailing association without a destination leaves
// it unset). Splitting at the first `/` only wrote a two-hop path as ONE step
// whose DestinationEntity was "E1/A2/E2", and the project would not load
// (mendixlabs/mxcli#1467). Shared by both association writers.
func associationEntityPathToGen(entityPath string) element.Element {
	parts := strings.Split(entityPath, "/")
	ref := genDm.NewIndirectEntityRef()
	assignID(ref)
	for i := 0; i < len(parts); i += 2 {
		step := genDm.NewEntityRefStep()
		assignID(step)
		step.SetAssociationQualifiedName(parts[i])
		if i+1 < len(parts) {
			step.SetDestinationEntityQualifiedName(parts[i+1])
		}
		ref.AddSteps(step)
	}
	return ref
}

// associationContextVariableToGen builds the Forms$PageVariable an association
// source is traversed from. A snippet parameter fills the SnippetParameter slot:
// the PageParameter slot would name a page parameter the snippet does not have
// (ako/mxcli#721 L3). Shared by both association writers so they cannot diverge.
func associationContextVariableToGen(d *pages.AssociationSource) element.Element {
	kind := ""
	if d.IsSnippetParameter {
		kind = "snippet"
	}
	return sourceVariableToGen(d.ContextVariable, kind)
}

// listViewSourceToGen builds a ListView data source. A database source becomes a
// Forms$ListViewXPathSource (EntityRef + XPathConstraint + a GridSortBar and a
// ListViewSearch sub-element, matching what Studio Pro/the legacy writer emit);
// a microflow source uses Forms$MicroflowSource (as DataView does).
//
// No source at all is a List View whose data source is not configured yet — a
// building block's (PlatformDesignSystem's Avatar_Group_SM) or a hand-written
// `listview` without `DataSource:`. Studio Pro stores that as an XPath source
// naming no entity: the same element with EntityRef and SourceVariable null and
// an empty sort bar and search (every such List View in the page templates of
// testdata/testapp). `mx check` then asks for the entity, which is the author's
// to choose. Refusing it instead blocked the whole block (ako/mxcli#2183).
func listViewSourceToGen(ds pages.DataSource) (element.Element, error) {
	if ds == nil {
		return listViewSourceToGen(&pages.DatabaseSource{})
	}
	switch d := ds.(type) {
	case *pages.DatabaseSource:
		src := genPg.NewListViewXPathSource()
		if d.ID != "" {
			src.SetID(element.ID(d.ID))
		}
		assignID(src)
		src.SetForceFullObjects(false)
		src.SetXPathConstraint(d.XPathConstraint)
		switch {
		case len(d.EntitySteps) > 0:
			// Reached over associations from a context object: the same
			// IndirectEntityRef an association source holds, but inside the
			// XPath source, so the retrieve stays a database one with its
			// constraint, sort and search (ako/mxcli#721 L5).
			src.SetEntityRef(pageEntityRefToGen(d.EntitySteps))
			if d.ContextVariable != "" {
				kind := ""
				if d.IsSnippetParameter {
					kind = "snippet"
				}
				src.SetSourceVariable(sourceVariableToGen(d.ContextVariable, kind))
			}
		case d.EntityName != "":
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
			// inputAttributeRefToGen, not attributeRefToGen: a sort that navigates
			// associations needs its hops stored as the AttributeRef's EntityRef,
			// exactly as an input widget's binding does. Without them the far
			// entity's attribute does not resolve and mxbuild answers CE7247
			// (mendixlabs/mxcli#1152). With no steps the two are identical.
			if ref := inputAttributeRefToGen(s.AttributePath, s.AttributeRefSteps); ref != nil {
				item.SetAttributeRef(ref)
			}
			bar.AddSortItems(item)
		}
		src.SetSortBar(bar)
		search := genPg.NewListViewSearch()
		assignID(search)
		// The search bar's attributes. Each is a DomainModels$AttributeRef —
		// the same element a Forms$GridSortItem carries, pinned against a
		// Studio Pro-authored sort bar in a blank 11.12.2 app. An empty Search
		// element is written either way: every reference ListViewXPathSource
		// carries one (ako/mxcli#512).
		for _, attr := range d.SearchAttributes {
			if ref := attributeRefToGen(attr); ref != nil {
				search.AddSearchRefs(ref)
			}
		}
		src.SetSearch(search)
		return src, nil
	case *pages.MicroflowSource:
		ms := genPg.NewMicroflowSource()
		if d.ID != "" {
			ms.SetID(element.ID(d.ID))
		}
		assignID(ms)
		ms.SetForceFullObjects(false)
		ms.SetMicroflowSettings(microflowSettingsToGen(d.Microflow, d.ParameterMappings, &pages.FlowCallSettings{FormValidations: d.FormValidations}, nil))
		return ms, nil

	// A NANOFLOW data source. Flat, unlike the microflow source above: the
	// nanoflow binds directly on the source, no settings child (CE2633) — see
	// nanoflowSourceToGen.
	case *pages.NanoflowSource:
		return nanoflowSourceToGen(d), nil
	case *pages.AssociationSource:
		return associationSourceToGen(d), nil
	default:
		return nil, fmt.Errorf("CreatePage: ListView source %T is not supported by either engine — please file an issue", ds)
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
		if d.UseConstraints {
			return customWidgetDatabaseSourceToGen(d), nil
		}
		src := genCw.NewCustomWidgetXPathSource()
		if d.ID != "" {
			src.SetID(element.ID(d.ID))
		}
		assignID(src)
		src.SetForceFullObjects(false)
		src.SetXPathConstraint(d.XPathConstraint)
		switch {
		case len(d.EntitySteps) > 0:
			// "Database" reached over associations from a context object: the
			// hops as an IndirectEntityRef inside the XPath source, the start
			// parameter in its SourceVariable — the shape listViewSourceToGen
			// writes, and the one Studio Pro stores for a gallery or grid
			// bound this way (mendixlabs/mxcli#1599).
			src.SetEntityRef(pageEntityRefToGen(d.EntitySteps))
			if d.ContextVariable != "" {
				kind := ""
				if d.IsSnippetParameter {
					kind = "snippet"
				}
				src.SetSourceVariable(sourceVariableToGen(d.ContextVariable, kind))
			}
		case d.EntityName != "":
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
			// inputAttributeRefToGen, not attributeRefToGen: a sort that navigates
			// associations needs its hops stored as the AttributeRef's EntityRef,
			// exactly as an input widget's binding does. Without them the far
			// entity's attribute does not resolve and mxbuild answers CE7247
			// (mendixlabs/mxcli#1152). With no steps the two are identical.
			if ref := inputAttributeRefToGen(s.AttributePath, s.AttributeRefSteps); ref != nil {
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
		ms.SetMicroflowSettings(microflowSettingsToGen(d.Microflow, d.ParameterMappings, &pages.FlowCallSettings{FormValidations: d.FormValidations}, nil))
		return ms, nil

	// A NANOFLOW data source. Flat, unlike the microflow source above: the
	// nanoflow binds directly on the source, no settings child (CE2633) — see
	// nanoflowSourceToGen.
	case *pages.NanoflowSource:
		return nanoflowSourceToGen(d), nil

	case *pages.AssociationSource:
		return associationSourceToGen(d), nil

	default:
		return nil, fmt.Errorf("modelsdk: pluggable widget data source %T is not supported by either engine — please file an issue", ds)
	}
}

// customWidgetDatabaseSourceToGen builds Studio Pro 9's "Database" source on a
// pluggable widget, a CustomWidgets$CustomWidgetDatabaseSource
// (mendixlabs/mxcli#1399). Its stored property set, measured on the FSM
// template's ten at 9.24.2, is DatabaseConstraints, EntityRef and SortBar and
// nothing else, so ForceFullObjects, EntityPath and SourceVariable are left
// unset: the first two do not exist before 10, and the parameter a path starts
// from is stored on the WidgetValue (widgetobj.setDataSource). The executor
// only builds one for a 9.x project.
func customWidgetDatabaseSourceToGen(d *pages.DatabaseSource) element.Element {
	src := genCw.NewCustomWidgetDatabaseSource()
	if d.ID != "" {
		src.SetID(element.ID(d.ID))
	}
	assignID(src)
	switch {
	case len(d.EntitySteps) > 0:
		src.SetEntityRef(pageEntityRefToGen(d.EntitySteps))
	case d.EntityName != "":
		ref := genDm.NewDirectEntityRef()
		assignID(ref)
		ref.SetEntityQualifiedName(d.EntityName)
		src.SetEntityRef(ref)
	}
	for _, c := range d.Constraints {
		row := genPg.NewDatabaseConstraint()
		assignID(row)
		row.SetAttributeQualifiedName(c.Attribute)
		row.SetOperator(c.Operator)
		row.SetValue(c.Value)
		src.AddDatabaseConstraints(row)
	}
	bar := genPg.NewGridSortBar()
	assignID(bar)
	for _, s := range d.Sorting {
		item := genPg.NewGridSortItem()
		assignID(item)
		item.SetSortDirection(string(s.Direction))
		if ref := inputAttributeRefToGen(s.AttributePath, s.AttributeRefSteps); ref != nil {
			item.SetAttributeRef(ref)
		}
		bar.AddSortItems(item)
	}
	src.SetSortBar(bar)
	return src
}

// associationSourceToGen builds a Forms$AssociationSource: an IndirectEntityRef
// with one EntityRefStep per hop (Association[/DestinationEntity]) and an optional
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
	src.SetEntityRef(associationEntityPathToGen(d.EntityPath))
	if d.ContextVariable != "" {
		src.SetSourceVariable(associationContextVariableToGen(d))
	}
	return src
}

// parameterMappingTarget is the half of Forms$MicroflowParameterMapping and
// Forms$NanoflowParameterMapping that carries an argument's value. The two gen
// types are unrelated Go types with identical shape, so the binding rule is
// written once against what they have in common rather than twice.
type parameterMappingTarget interface {
	SetExpression(string)
	SetVariable(element.Element)
}

// bindParameterMappingValue writes an argument into whichever of the mapping's
// two value slots Mendix uses for it.
//
// A reference to a page parameter, snippet parameter or page variable is a
// Forms$PageVariable under Variable; a literal or expression is text under
// Expression. Measured on Workflow Commons 4.11.0 (Studio Pro-authored): 95 of
// 101 flow parameter mappings bind through Variable, the other 6 through
// Expression — and every one of those 6 is a Boolean literal. A $-prefixed
// Expression, which is all mxcli wrote before #1140, occurs zero times; it leaves
// the parameter unbound, so Studio Pro reports CE1571 while mxbuild builds the
// same document at 0 errors.
//
// kind empty means "not a page-variable reference": variable is then written as
// the expression, preserving what every caller before #1140 relied on —
// $currentObject among them, whose stored form has not been measured.
//
// Kind "widget" is a widget's selection: the PageVariable names the widget and
// nothing else, as on all 13 such mappings in TestApp's WorkflowCommons
// (ako/mxcli#1560). It used to fall to the page-parameter slot. A data view
// showing a page or snippet parameter names that parameter beside the widget
// (widgetParam, widgetParamKind "" or "snippet"): Widget alone is CE0115 there
// (ako/mxcli#2095), while a data view with no parameter behind it keeps Widget
// alone, which passes.
func bindParameterMappingValue(m parameterMappingTarget, variable, kind, expression, widgetParam, widgetParamKind string) {
	if variable != "" && kind != "" {
		name := strings.TrimPrefix(variable, "$")
		switch kind {
		case "widget":
			m.SetVariable(pageVariableToGen(name, widgetParam, widgetParamKind))
		case "parameter":
			// sourceVariableToGen spells the page-parameter slot as the empty kind.
			m.SetVariable(sourceVariableToGen(name, ""))
		default:
			m.SetVariable(sourceVariableToGen(name, kind))
		}
		// Studio Pro writes both keys, the unused one empty.
		m.SetExpression("")
		return
	}
	if variable != "" {
		m.SetExpression(variable)
		return
	}
	m.SetExpression(expression)
}

// microflowSettingsToGen builds the Forms$MicroflowSettings shared by the
// microflow DataView source and the call-microflow action. mappings carries the
// argument bindings — for an action's call, and (since #835) for a parameterized
// source microflow, which Mendix requires arguments for just the same; dropping them
// left a parameterized button/row invoking its microflow with no argument — the
// widget no-ops at runtime yet mx check passes clean (Bug 1).
//
// settings carries what a call-microflow action's `with ( … )` says, or for a
// data source its FormValidations (ako/mxcli#1729); nil writes Mendix's defaults.
// outputs are its `returns ( … )` mappings (ako/mxcli#977 item 3).
func microflowSettingsToGen(microflowName string, mappings []*pages.MicroflowParameterMapping, settings *pages.FlowCallSettings, outputs []*pages.FlowOutputMapping) element.Element {
	if settings == nil {
		settings = &pages.FlowCallSettings{}
	}
	s := genPg.NewMicroflowSettings()
	assignID(s)
	s.SetAsynchronous(settings.Asynchronous)
	s.SetFormValidations(orDefaultStr(settings.FormValidations, "All"))
	s.SetMicroflowQualifiedName(microflowName)
	s.SetProgressBar(progressBarOrNone(settings.ProgressBar))
	if settings.ProgressMessage != nil {
		s.SetProgressMessage(textToGen(settings.ProgressMessage))
	}
	if settings.Confirmation != nil {
		s.SetConfirmationInfo(confirmationInfoToGen(settings.Confirmation))
	}
	for _, pm := range mappings {
		gm := genPg.NewMicroflowParameterMapping()
		assignID(gm)
		// Parameter is a BY_NAME reference: <MicroflowQName>.<ParameterName>.
		gm.SetParameterQualifiedName(microflowName + "." + pm.ParameterName)
		bindParameterMappingValue(gm, pm.Variable, pm.VariableKind, pm.Expression, pm.WidgetParameter, pm.WidgetParameterKind)
		s.AddParameterMappings(gm)
	}
	for _, om := range outputs {
		s.AddOutputMappings(outputMappingToGen(om))
	}
	return s
}

// outputMappingToGen builds a Forms$OutputMapping, one row of a flow call's
// return value mappings (ako/mxcli#977 item 3). The slots a row does not use
// stay null, Expression empty — the shape Studio Pro stores for each of the
// three kinds of row on ako/TestApp's Pages.OutputMappingButton.
func outputMappingToGen(om *pages.FlowOutputMapping) element.Element {
	g := genPg.NewOutputMapping()
	assignID(g)
	if r := attributeRefToGen(om.TargetAttribute); r != nil {
		g.SetAttributeRef(r)
	}
	if om.TargetVariable != "" {
		g.SetSourceVariable(pageVariableToGen("", om.TargetVariable, "local"))
	}
	if r := attributeRefToGen(om.SourceAttribute); r != nil {
		g.SetSourceAttributeRef(r)
	}
	g.SetExpression(om.Expression)
	return g
}

// formSettingsToGen builds the Forms$FormSettings (PageSettings) shared by the
// page-opening actions: target page by-name and empty parameter mappings.
//
// TitleOverride is deliberately left unset. A widget action (a button opening a
// page) has no MDL syntax for overriding the title, so the correct value is always
// null — the page keeps its own title. Setting an empty Microflows$TextTemplate here
// overrode it with the empty string, so every popup opened by an mxcli-authored
// button rendered with a blank caption and just the close button (#812).
func formSettingsToGen(pageName string) element.Element {
	ps := genPg.NewPageSettings()
	assignID(ps)
	ps.SetPageQualifiedName(pageName)
	return ps
}

// pageSettingsWithMappings is formSettingsToGen for a show page action, adding
// the mappings whose argument is a page or snippet parameter bound by name.
//
// Studio Pro stores such an argument as an explicit Forms$PageParameterMapping:
// Argument "" and a Forms$PageVariable naming the parameter (ako/TestApp
// WorkflowCommons TaskAssignmentHelper_UserTask_Reassign, PageParameter;
// Snip_UserTask_Detail, SnippetParameter). It is how a button outside every data
// container passes the page's own parameter, which an empty array cannot say —
// there is no context object to infer it from (#1029: CE1571). A mapping with
// no VariableKind is the context object and stays inferred (#296: CE0115), so a
// page that passes only that is written exactly as before (mendixlabs/mxcli#1403).
func pageSettingsWithMappings(pageName string, mappings []*pages.PageClientParameterMapping) element.Element {
	ps := genPg.NewPageSettings()
	assignID(ps)
	ps.SetPageQualifiedName(pageName)
	for _, pm := range mappings {
		if pm == nil || pm.VariableKind == "" || pm.Variable == "" {
			continue
		}
		m := genPg.NewPageParameterMapping()
		// gen stamps this type "Forms$FormCallArgument", which is Mendix's
		// LayoutCallArgument: mxbuild then refuses to load the page ("Type
		// …LayoutCallArgument does not contain a constructor with a parameter of
		// type …PageSettings"). Studio Pro stores Forms$PageParameterMapping.
		m.SetTypeName("Forms$PageParameterMapping")
		assignID(m)
		m.SetParameterQualifiedName(pageName + "." + pm.ParameterName)
		m.SetArgument("")
		if pm.VariableKind == "widget" {
			// A control-bar button passing its grid's selection names the grid
			// in the Widget slot (mendixlabs/mxcli#122). mxbuild 11.12.5: 0
			// errors; the empty array is CE1571, Argument "$grid" CE0115.
			m.SetVariable(pageVariableToGen(strings.TrimPrefix(pm.Variable, "$"), "", ""))
		} else {
			m.SetVariable(sourceVariableToGen(strings.TrimPrefix(pm.Variable, "$"), pm.VariableKind))
		}
		ps.AddParameterMappings(m)
	}
	return ps
}

// A static address stores AttributeRef as null: 21 of 21 static
// Forms$StaticOrDynamicString across ako/TestApp and PedApp carry the key.
// staticAddressToGen never sets it, so without this default the key was
// dropped and every describe → exec of a page with a link button rewrote the
// address (ako/mxcli-ledger FINDINGS #170). dynamicAddressToGen sets a real
// ref, which the default (applied only to an unset key) does not touch.
func init() {
	codec.RegisterTypeDefaults("Forms$StaticOrDynamicString", codec.TypeDefaults{
		NullFields: []string{"AttributeRef"},
	})
}

// dynamicAddressToGen builds a link address read from an attribute at runtime.
// Pinned against FeedbackModule.PopupSuccess (Feedback v4.0.2): IsDynamic true,
// Value "", and an AttributeRef with a null EntityRef naming the attribute.
func dynamicAddressToGen(attrQN string) element.Element {
	s := genPg.NewStaticOrDynamicString()
	assignID(s)
	s.SetIsDynamic(true)
	s.SetValue("")
	s.SetAttributeRef(attributeRefToGen(attrQN))
	return s
}

// staticAddressToGen builds the Forms$StaticOrDynamicString an open-link action
// nests as its Address.
//
// Only the STATIC form is authored: MDL spells `OPEN_LINK 'https://…'` and has
// no syntax for the dynamic one. 6 of the 31 Studio Pro references are dynamic
// (IsDynamic true, an AttributeRef, an empty Value) — DESCRIBE flags those
// rather than rendering them as a literal, because a dynamic address printed as
// a static one round-trips into a different link.
//
// gen declares a fourth property here, `Attribute` (deleted in 7.11.0), that not
// one of the 31 documents carries; it is never set, because writing a key Mendix
// does not store is what makes a document mxbuild accepts and Studio Pro cannot
// open. AttributeRef is a different key and IS stored on a static address — as
// null, on 21 of 21 static addresses across ako/TestApp and PedApp. The init
// above registers that null; dynamicAddressToGen sets a real ref, which the
// default does not touch.
func staticAddressToGen(address string) element.Element {
	s := genPg.NewStaticOrDynamicString()
	assignID(s)
	s.SetIsDynamic(false)
	s.SetValue(address)
	return s
}

// focusEventsToGen builds an input widget's OnEnterAction and OnLeaveAction,
// Studio Pro's "On enter" / "On leave". Both were hard-coded NoActions on every
// input but the text box's OnEnterAction, so neither could be written and
// describe → exec deleted a stored one (ako/mxcli#1743). nil writes the
// NoAction, as before.
func focusEventsToGen(onEnter, onLeave pages.ClientAction) (element.Element, element.Element, error) {
	enter, err := clientActionToGen(onEnter)
	if err != nil {
		return nil, nil, err
	}
	leave, err := clientActionToGen(onLeave)
	if err != nil {
		return nil, nil, err
	}
	return enter, leave, nil
}

// clientActionToGen converts a widget client action. Simple actions are supported;
// the page/microflow/nanoflow/create-object actions (which carry settings sub-
// objects) are refused loudly for now.
func clientActionToGen(a pages.ClientAction) (element.Element, error) {
	switch x := a.(type) {
	case nil, *pages.NoClientAction:
		return noActionGen(), nil
	// The four simple actions below were the only ones that did not write
	// DisabledDuringExecution. Studio Pro stores it true on all 39 of them
	// across the 67 pages of ako/TestApp at 11.14.0 (CancelChanges 16, ClosePage
	// 10, SaveChanges 8, Delete 5), so its absence was a round-trip loss rather
	// than a default (ako/mxcli#541). The seven other cases in this switch
	// already set it.
	//
	// Other types carrying the property are NOT unanimous — Forms$NoAction
	// stores false on 83 of ~7,300 and Forms$MicroflowAction on 5 of 81 — but
	// those are stored values mxcli overwrites, a carry problem that predates
	// this change and is tracked separately.
	case *pages.SaveChangesClientAction:
		g := genPg.NewSaveChangesClientAction()
		assignID(g)
		g.SetClosePage(x.ClosePage)
		g.SetDisabledDuringExecution(actionDisabledDuringExecution(x.ActionExecution))
		// false, not true: all eight Studio Pro SaveChanges actions in that
		// same sweep store false. Writing true turned a describe → exec of any
		// page with a Save button into a change nobody asked for.
		g.SetSyncAutomatically(false)
		return g, nil
	case *pages.CancelChangesClientAction:
		g := genPg.NewCancelChangesClientAction()
		assignID(g)
		g.SetClosePage(x.ClosePage)
		g.SetDisabledDuringExecution(actionDisabledDuringExecution(x.ActionExecution))
		return g, nil
	case *pages.ClosePageClientAction:
		g := genPg.NewClosePageClientAction()
		assignID(g)
		g.SetDisabledDuringExecution(actionDisabledDuringExecution(x.ActionExecution))
		return g, nil
	case *pages.DeleteClientAction:
		g := genPg.NewDeleteClientAction()
		assignID(g)
		g.SetClosePage(x.ClosePage)
		// A grid's selection is a PageVariable naming only the widget, as
		// Studio Pro stores it for a control-bar Delete button; without it
		// mx check reports CE7005 (ako/mxcli#1932). The property is 10.4+:
		// canon.StripUndeclaredProperties refuses it on an older project.
		if sv := x.SourceVariable; sv != nil {
			g.SetSourceVariable(pageVariableToGen(sv.Widget, sv.Variable, sv.Kind))
		}
		g.SetDisabledDuringExecution(actionDisabledDuringExecution(x.ActionExecution))
		return g, nil
	case *pages.PageClientAction:
		// show_page → Forms$FormAction with a Forms$FormSettings (PageSettings).
		g := genPg.NewPageClientAction()
		if x.ID != "" {
			g.SetID(element.ID(x.ID))
		}
		assignID(g)
		g.SetDisabledDuringExecution(actionDisabledDuringExecution(x.ActionExecution))
		g.SetNumberOfPagesToClose2(x.NumberOfPagesToClose)
		g.SetPageSettings(pageSettingsWithMappings(x.PageName, x.ParameterMappings))
		return g, nil
	case *pages.LinkClientAction:
		// open_link → Forms$OpenLinkClientAction. Note the storage name: the
		// semantic type is LinkClientAction and the executor stamped it
		// "Forms$LinkClientAction", which is not what Mendix stores — a wrong
		// $Type that never reached disk only because neither engine could write
		// the action at all.
		//
		// Pinned against 31 Studio Pro-authored link buttons (ako/TestApp,
		// FeedbackModule): exactly five keys, LinkType "Web" in all 31, and the
		// address nested as a Forms$StaticOrDynamicString.
		g := genPg.NewOpenLinkClientAction()
		if x.ID != "" {
			g.SetID(element.ID(x.ID))
		}
		assignID(g)
		g.SetDisabledDuringExecution(actionDisabledDuringExecution(x.ActionExecution))
		linkType := string(x.LinkType)
		if linkType == "" {
			linkType = "Web"
		}
		g.SetLinkType(linkType)
		if x.AddressAttribute != "" {
			g.SetAddress(dynamicAddressToGen(x.AddressAttribute))
		} else {
			g.SetAddress(staticAddressToGen(x.Address))
		}
		return g, nil
	case *pages.SignOutClientAction:
		// sign_out → Forms$SignOutClientAction. One property, and the reference
		// pins its value: a Studio Pro-authored sign-out button (ako/TestApp,
		// Mendix 11) stores exactly
		//
		//	{ "$Type": "Forms$SignOutClientAction", "DisabledDuringExecution": true }
		//
		// That document is provably Studio Pro's rather than mxcli's, because
		// until now NEITHER engine could emit the type — modelsdk refused it and
		// legacy wrote Forms$NoAction (CapTrackV2 FINDINGS §10).
		g := genPg.NewSignOutClientAction()
		if x.ID != "" {
			g.SetID(element.ID(x.ID))
		}
		assignID(g)
		g.SetDisabledDuringExecution(actionDisabledDuringExecution(x.ActionExecution))
		return g, nil
	case *pages.SetTaskOutcomeClientAction:
		g := genPg.NewSetTaskOutcomeClientAction()
		if x.ID != "" {
			g.SetID(element.ID(x.ID))
		}
		assignID(g)
		g.SetClosePage(x.ClosePage)
		g.SetCommit(x.Commit)
		g.SetDisabledDuringExecution(actionDisabledDuringExecution(x.ActionExecution))
		g.SetOutcomeValue(x.OutcomeValue)
		return g, nil
	case *pages.OpenUserTaskClientAction:
		// open user task → Forms$OpenUserTaskClientAction, Studio Pro's "Show
		// user task page". ako/TestApp's six (WorkflowCommons, 11.15) store
		// exactly AssignOnOpen, DisabledDuringExecution and OpenWhenAssigned;
		// the two flags are nil below 9.20, which introduced them.
		g := genPg.NewOpenUserTaskClientAction()
		if x.ID != "" {
			g.SetID(element.ID(x.ID))
		}
		assignID(g)
		if x.AssignOnOpen != nil {
			g.SetAssignOnOpen(*x.AssignOnOpen)
		}
		g.SetDisabledDuringExecution(actionDisabledDuringExecution(x.ActionExecution))
		if x.OpenWhenAssigned != nil {
			g.SetOpenWhenAssigned(*x.OpenWhenAssigned)
		}
		return g, nil
	case *pages.OpenWorkflowClientAction:
		// open workflow → Forms$OpenWorkflowClientAction ("Show workflow admin
		// page"). DefaultPage is a by-name page reference, "" when unset (the
		// type default registered above).
		g := genPg.NewOpenWorkflowClientAction()
		if x.ID != "" {
			g.SetID(element.ID(x.ID))
		}
		assignID(g)
		if x.DefaultPage != "" {
			g.SetDefaultPageQualifiedName(x.DefaultPage)
		}
		g.SetDisabledDuringExecution(actionDisabledDuringExecution(x.ActionExecution))
		return g, nil
	case *pages.CallWorkflowClientAction:
		// call workflow → Forms$CallWorkflowClientAction. Commit is never
		// written: Mendix 9.12 removed it (a workflow call always commits),
		// and a key the metamodel no longer declares is one Studio Pro cannot
		// open. ConfirmationInfo is null when none is asked (type default).
		g := genPg.NewCallWorkflowClientAction()
		if x.ID != "" {
			g.SetID(element.ID(x.ID))
		}
		assignID(g)
		g.SetClosePage(x.ClosePage)
		if x.Confirmation != nil {
			g.SetConfirmationInfo(confirmationInfoToGen(x.Confirmation))
		}
		g.SetDisabledDuringExecution(actionDisabledDuringExecution(x.ActionExecution))
		g.SetWorkflowQualifiedName(x.WorkflowName)
		return g, nil
	case *pages.MicroflowClientAction:
		// call_microflow → Forms$MicroflowAction.
		g := genPg.NewMicroflowClientAction()
		if x.ID != "" {
			g.SetID(element.ID(x.ID))
		}
		assignID(g)
		g.SetDisabledDuringExecution(actionDisabledDuringExecution(x.ActionExecution))
		g.SetMicroflowSettings(microflowSettingsToGen(x.MicroflowName, x.ParameterMappings, &x.FlowCallSettings, x.OutputMappings))
		return g, nil
	case *pages.NanoflowClientAction:
		// call_nanoflow → Forms$CallNanoflowClientAction. Unlike the microflow
		// action (which nests a Forms$MicroflowSettings), the nanoflow name and
		// parameter mappings live directly on the action. Mirrors the legacy
		// serializer in sdk/mpr/writer_widgets_action.go. Bug 2.
		g := genPg.NewCallNanoflowClientAction()
		if x.ID != "" {
			g.SetID(element.ID(x.ID))
		}
		assignID(g)
		g.SetDisabledDuringExecution(actionDisabledDuringExecution(x.ActionExecution))
		g.SetNanoflowQualifiedName(x.NanoflowName)
		g.SetProgressBar(progressBarOrNone(x.ProgressBar))
		if x.ProgressMessage != nil {
			g.SetProgressMessage(textToGen(x.ProgressMessage))
		}
		if x.Confirmation != nil {
			g.SetConfirmationInfo(confirmationInfoToGen(x.Confirmation))
		}
		for _, pm := range x.ParameterMappings {
			m := genPg.NewNanoflowParameterMapping()
			assignID(m)
			// Parameter is a BY_NAME reference: Nanoflow.ParamName.
			m.SetParameterQualifiedName(x.NanoflowName + "." + pm.ParameterName)
			bindParameterMappingValue(m, pm.Variable, pm.VariableKind, pm.Expression, pm.WidgetParameter, pm.WidgetParameterKind)
			g.AddParameterMappings(m)
		}
		for _, om := range x.OutputMappings {
			g.AddOutputMappings(outputMappingToGen(om))
		}
		return g, nil
	case *pages.CreateObjectClientAction:
		// create_object → Forms$CreateObjectClientAction (entity ref + page settings).
		g := genPg.NewCreateObjectClientAction()
		if x.ID != "" {
			g.SetID(element.ID(x.ID))
		}
		assignID(g)
		g.SetDisabledDuringExecution(actionDisabledDuringExecution(x.ActionExecution))
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
		return nil, fmt.Errorf("CreatePage: client action %T is not supported by either engine — please file an issue", a)
	}
}

// actionDisabledDuringExecution is an action's "Disabled during action": what the
// script says, else true — Studio Pro's value on all but a few stored actions.
func actionDisabledDuringExecution(e pages.ActionExecution) bool {
	if e.DisabledDuringExecution != nil {
		return *e.DisabledDuringExecution
	}
	return true
}

func progressBarOrNone(v string) string { return orDefaultStr(v, "None") }

// confirmationInfoToGen builds the Forms$ConfirmationInfo a flow call asks
// before it runs. Studio Pro writes all three texts (ako/TestApp, 21 of 21).
func confirmationInfoToGen(c *pages.ConfirmationInfo) element.Element {
	ci := genPg.NewConfirmationInfo()
	assignID(ci)
	ci.SetQuestion(captionToGen(c.Question))
	ci.SetProceedButtonCaption(captionToGen(c.ProceedCaption))
	ci.SetCancelButtonCaption(captionToGen(c.CancelCaption))
	return ci
}

// orDefaultStr returns s, or def when s is empty.
func orDefaultStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// scrollRegionToGen builds one Forms$ScrollContainerRegion.
//
// The region carries no Name — its slot is its identity — and its Class lives
// in Appearance like every other widget, not as a top-level key. Children go in
// Widgets (a list); gen also exposes a singular Widget, which no real document
// uses.
func scrollRegionToGen(r *pages.ScrollContainerRegion) (element.Element, error) {
	g := genPg.NewScrollContainerRegion()
	assignID(g)
	g.SetAppearance(newAppearance(r.Class, "", "", nil))
	// 200/Auto is what Studio Pro writes for a region whose size was never
	// touched — every unsized Atlas region carries it, including the centre
	// ones where the number is inert. Writing a bare 0 instead would be a size
	// nothing in the model ever means.
	size := r.Size
	if size == 0 {
		size = 200
	}
	g.SetSize(int32(size))
	g.SetSizeMode(orDefaultStr(r.SizeMode, "Auto"))
	g.SetToggleMode(orDefaultStr(r.ToggleMode, "None"))
	for _, w := range r.Widgets {
		wg, err := widgetToGen(w)
		if err != nil {
			return nil, err
		}
		g.AddWidgets(wg)
	}
	return g, nil
}

// menuSourceToGen builds the MenuSource every menu widget carries: a
// Forms$MenuDocumentSource when it renders a menu document, otherwise a
// Forms$NavigationSource naming a profile (Responsive when none is given).
// The two are alternatives for one slot, so both set is an error.
func menuSourceToGen(widget, menu, profile string) (element.Element, error) {
	if menu != "" {
		if profile != "" {
			return nil, fmt.Errorf("menu widget %q names both menu document %q and navigation profile %q; it renders one", widget, menu, profile)
		}
		src := genPg.NewMenuDocumentSource()
		assignID(src)
		src.SetMenuQualifiedName(menu)
		return src, nil
	}
	src := genPg.NewNavigationSource()
	assignID(src)
	src.SetNavigationProfileQualifiedName(orDefaultStr(profile, "Responsive"))
	return src, nil
}

// orDefaultFileManagerType is the Model SDK's default, Both, for an unset Type:
// an empty enum is a value Studio Pro cannot load.
func orDefaultFileManagerType(t pages.FileManagerType) pages.FileManagerType {
	if t == "" {
		return pages.FileManagerTypeBoth
	}
	return t
}
