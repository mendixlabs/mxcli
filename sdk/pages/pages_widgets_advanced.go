// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Navigation Widgets

// NavigationTree represents a navigation tree widget.
type NavigationTree struct {
	BaseWidget
	// NavigationProfile names the profile the menu is drawn from. It is stored
	// inside a Forms$NavigationSource under MenuSource, not on the tree itself.
	NavigationProfile string `json:"navigationProfile,omitempty"`
	// Menu names a Menus$MenuDocument instead, stored in a
	// Forms$MenuDocumentSource. At most one of Menu and NavigationProfile is set.
	Menu  string            `json:"menu,omitempty"`
	Items []*NavigationItem `json:"items,omitempty"`
}

// NavigationItem represents an item in navigation.
type NavigationItem struct {
	model.BaseElement
	Caption  *model.Text       `json:"caption,omitempty"`
	Icon     *Icon             `json:"icon,omitempty"`
	Action   ClientAction      `json:"action,omitempty"`
	SubItems []*NavigationItem `json:"subItems,omitempty"`
}

// MenuBar represents a menu bar widget — the horizontal navigation a topbar
// layout carries, where a NavigationTree is the vertical one a sidebar carries.
//
// Its stored shape is NavigationTree's: Appearance, MenuSource, Name, TabIndex
// and nothing else (measured on Atlas_Core.Atlas_TopBar, and exactly what
// generated/metamodel's PagesMenuBar declares). NavigationProfile is therefore
// carried the same way — as the profile's name, which the codec wraps in a
// Forms$NavigationSource.
type MenuBar struct {
	BaseWidget
	NavigationProfile string `json:"navigationProfile,omitempty"`
	// Menu names a menu document instead of a profile — see NavigationTree.
	Menu string `json:"menu,omitempty"`

	// MenuSource is the older polymorphic form, kept because the type is
	// exported. Nothing reads or writes it.
	MenuSource MenuSource `json:"menuSource,omitempty"`
}

// MenuSource represents the source for a menu.
type MenuSource interface {
	isMenuSource()
}

// NavigationMenuSource uses a navigation profile.
type NavigationMenuSource struct {
	model.BaseElement
}

func (NavigationMenuSource) isMenuSource() {}

// CustomMenuSource uses custom items.
type CustomMenuSource struct {
	model.BaseElement
	Items []*NavigationItem `json:"items,omitempty"`
}

func (CustomMenuSource) isMenuSource() {}

// SimpleMenuBar represents a simple menu bar — the widget Atlas's phone
// layouts put in their bottom region (ako/mxcli#573).
//
// Measured on Atlas_Core.Phone_BottomBar (11.14.0): Appearance, MenuSource,
// Name, Orientation, TabIndex, with MenuSource a Forms$MenuDocumentSource
// naming Atlas_Core.Phone_Menu. Like every menu widget it can take a
// navigation profile instead; at most one of Menu and NavigationProfile is set.
type SimpleMenuBar struct {
	BaseWidget
	Orientation       MenuOrientation `json:"orientation"`
	Menu              string          `json:"menu,omitempty"`
	NavigationProfile string          `json:"navigationProfile,omitempty"`

	// MenuSource is the older polymorphic form, kept because the type is
	// exported. Nothing reads or writes it.
	MenuSource MenuSource `json:"menuSource,omitempty"`
}

// SidebarToggleButton is the button that opens and closes a responsive
// layout's sidebar — Atlas's slide-in navigation on phones and tablets
// (mendixlabs/mxcli#1220).
//
// Measured on Atlas_Core.Atlas_Default (11.13.0): Appearance, ButtonStyle,
// CaptionTemplate, ConditionalVisibilitySettings, Icon, Name, RenderType,
// TabIndex, Tooltip — an action button's keys without the action, which is
// fixed: the button toggles the sidebar.
type SidebarToggleButton struct {
	BaseWidget
	CaptionTemplate *ClientTemplate  `json:"captionTemplate,omitempty"`
	Tooltip         *model.Text      `json:"tooltip,omitempty"`
	Icon            *Icon            `json:"icon,omitempty"`
	ButtonStyle     ButtonStyle      `json:"buttonStyle,omitempty"`
	RenderMode      ButtonRenderMode `json:"renderMode,omitempty"`
}

// LayoutHeader is a phone or tablet layout's header bar (Forms$Header): two
// widget lists, left and right. Atlas's Phone_ and Tablet_ layouts keep their
// HeaderLeft / HeaderRight placeholders in it, and Phone_Sidebar /
// Tablet_Sidebar their sidebar toggle.
//
// Measured on Atlas_Core (11.13.0): Appearance, LeftWidgets, Name,
// RightWidgets, TabIndex, both lists marker 2.
type LayoutHeader struct {
	BaseWidget
	LeftWidgets  []Widget `json:"leftWidgets,omitempty"`
	RightWidgets []Widget `json:"rightWidgets,omitempty"`
}

// MenuOrientation represents menu orientation.
type MenuOrientation string

const (
	MenuOrientationHorizontal MenuOrientation = "Horizontal"
	MenuOrientationVertical   MenuOrientation = "Vertical"
)

// Table Widget

// Table represents a table widget.
type Table struct {
	BaseWidget
	Rows []*TableRow `json:"rows,omitempty"`
}

// TableRow represents a row in a table.
type TableRow struct {
	model.BaseElement
	Cells []*TableCell `json:"cells,omitempty"`
}

// TableCell represents a cell in a table.
type TableCell struct {
	model.BaseElement
	ColumnSpan int      `json:"columnSpan"`
	RowSpan    int      `json:"rowSpan"`
	Widgets    []Widget `json:"widgets,omitempty"`
}

// Pluggable Widgets (Custom Widgets)

// PluggableWidget represents a pluggable/custom widget.
type PluggableWidget struct {
	BaseWidget
	WidgetID   string         `json:"widgetId"`
	Properties map[string]any `json:"properties,omitempty"`
}

// HTMLSnippet represents an HTML snippet widget.
type HTMLSnippet struct {
	BaseWidget
	Type        HTMLSnippetType `json:"type"`
	Content     string          `json:"content,omitempty"`
	ExternalURL string          `json:"externalUrl,omitempty"`
}

// HTMLSnippetType represents the type of HTML snippet.
type HTMLSnippetType string

const (
	HTMLSnippetTypeHTML     HTMLSnippetType = "HTML"
	HTMLSnippetTypeScript   HTMLSnippetType = "Script"
	HTMLSnippetTypeStyle    HTMLSnippetType = "Style"
	HTMLSnippetTypeExternal HTMLSnippetType = "External"
)

// SnippetParamMapping represents one parameter mapping in a snippet call.
type SnippetParamMapping struct {
	ParamName string // Snippet parameter name without leading $ (e.g. "Asset")
	Argument  string // Variable being passed, includes leading $ (e.g. "$asset")
	// IsSnippetParameter puts Argument in the SnippetParameter slot of the
	// mapping's Forms$PageVariable instead of PageParameter: the call sits in a
	// snippet and passes that snippet's own parameter (ako/mxcli#721 L3).
	IsSnippetParameter bool
	// IsLocalVariable puts Argument in the LocalVariable slot: the call passes a
	// page `Variables:` entry, not a parameter (ako/mxcli#826).
	IsLocalVariable bool
	// IsWidget puts Argument in the Widget slot: the call passes a data
	// view's object by naming the data view (ako/mxcli#1968). Written in
	// PageParameter, it named a page parameter that does not exist — CE0115.
	IsWidget bool
}

// SnippetCallWidget represents a snippet call widget.
type SnippetCallWidget struct {
	BaseWidget
	SnippetID         model.ID              `json:"snippetId"`
	SnippetName       string                `json:"snippetName,omitempty"` // Qualified name for BY_NAME_REFERENCE
	ParameterMappings []SnippetParamMapping `json:"parameterMappings,omitempty"`
}

// Gallery represents a gallery widget for displaying items in a grid layout (Forms$Gallery).
type Gallery struct {
	BaseWidget
	DataSource           DataSource    `json:"dataSource,omitempty"`
	ContentWidget        Widget        `json:"contentWidget,omitempty"`
	FilterWidgets        []Widget      `json:"filterWidgets,omitempty"`
	SelectionMode        SelectionMode `json:"selectionMode"`
	DesktopItems         int           `json:"desktopItems"`
	TabletItems          int           `json:"tabletItems"`
	PhoneItems           int           `json:"phoneItems"`
	PageSize             int           `json:"pageSize"`
	ShowEmptyPlaceholder bool          `json:"showEmptyPlaceholder"`
}

// CustomWidget represents a pluggable/custom widget (CustomWidgets$CustomWidget).
type CustomWidget struct {
	BaseWidget
	Label        string            `json:"label,omitempty"`
	WidgetType   *CustomWidgetType `json:"widgetType"`
	WidgetObject *WidgetObject     `json:"widgetObject,omitempty"`
	Editable     string            `json:"editable,omitempty"` // Always, Conditional, Never

	// RawType holds a cloned widget type definition from an existing widget.
	// When set, the serializer will use this raw BSON instead of building from WidgetType.
	RawType bson.D `json:"-"`

	// RawObject holds a cloned WidgetObject from an existing widget.
	// When set, the serializer will use this raw BSON instead of building from WidgetObject.
	// This contains all property values from the source widget with updated TypePointers.
	RawObject bson.D `json:"-"`

	// PropertyTypeIDMap maps property keys to their PropertyType and ValueType IDs
	// from the cloned RawType. This is used to correctly reference IDs when serializing.
	PropertyTypeIDMap map[string]PropertyTypeIDEntry `json:"-"`

	// ObjectTypeID holds the ID of the WidgetObjectType from the cloned RawType.
	// This is used as the TypePointer in the WidgetObject during serialization.
	ObjectTypeID string `json:"-"`
}

// PropertyTypeIDEntry and PropertyTranslation are re-exports of the canonical
// types in mdl/types, NOT separate definitions.
//
// They used to be a second declaration of the same shape, with
// convertPropTypeIDs (mdl/backend/modelsdk) copying field by field between the
// two. That copy is where a template's DataSourceProperty — the widget's own
// statement of which datasource a dependent property binds against — was
// silently dropped: the loader reads it, the engine never saw it, and nothing
// failed. A hand-maintained copy between two spellings of one type can only
// lose fields, never gain them, so the two spellings are now one type.
//
// mdl/types has no internal dependencies, so aliasing it here adds no import
// cycle. Same pattern as sdk/mpr's re-exports (CLAUDE.md: shared types live in
// mdl/types; other packages re-export as aliases, never as duplicates).
type (
	PropertyTranslation = types.PropertyTranslation
	PropertyTypeIDEntry = types.PropertyTypeIDEntry
)

// CustomWidgetType defines the pluggable widget type (CustomWidgets$CustomWidgetType).
type CustomWidgetType struct {
	ID                 model.ID          `json:"id"`
	WidgetID           string            `json:"widgetId"` // e.g., "com.mendix.widget.web.gallery.Gallery"
	Name               string            `json:"name"`     // e.g., "Gallery"
	Description        string            `json:"description,omitempty"`
	HelpURL            string            `json:"helpUrl,omitempty"`
	NeedsEntityContext bool              `json:"needsEntityContext"`
	OfflineCapable     bool              `json:"offlineCapable"`
	PluginWidget       bool              `json:"pluginWidget"`
	SupportedPlatform  string            `json:"supportedPlatform,omitempty"` // Web, Native, All
	ObjectType         *WidgetObjectType `json:"objectType,omitempty"`
}

// WidgetObjectType defines the property types for a pluggable widget.
type WidgetObjectType struct {
	ID            model.ID              `json:"id"`
	PropertyTypes []*WidgetPropertyType `json:"propertyTypes,omitempty"`
}

// WidgetPropertyType defines a single property type for a pluggable widget.
type WidgetPropertyType struct {
	ID          model.ID `json:"id"`
	Key         string   `json:"key"`
	ValueType   string   `json:"valueType"`   // e.g., "Action", "Attribute", "DataSource", "Expression", etc.
	ValueTypeID model.ID `json:"valueTypeId"` // ID of the embedded WidgetValueType object
	Caption     string   `json:"caption,omitempty"`
	Description string   `json:"description,omitempty"`
	IsDefault   bool     `json:"isDefault"`
	Required    bool     `json:"required"`
}

// WidgetObject contains the property values for a pluggable widget instance.
type WidgetObject struct {
	ID         model.ID          `json:"id"`
	Properties []*WidgetProperty `json:"properties,omitempty"`
}

// WidgetProperty represents a single property value in a pluggable widget.
type WidgetProperty struct {
	ID          model.ID     `json:"id"`
	TypePointer model.ID     `json:"typePointer"` // Reference to WidgetPropertyType
	Value       *WidgetValue `json:"value,omitempty"`
	PropertyKey string       `json:"-"` // Key for looking up PropertyType IDs from cloned type
}

// WidgetValue holds the actual value for a widget property.
type WidgetValue struct {
	ID             model.ID        `json:"id"`
	TypePointer    model.ID        `json:"typePointer,omitempty"`
	Action         ClientAction    `json:"action,omitempty"`
	DataSource     DataSource      `json:"dataSource,omitempty"`
	AttributeRef   string          `json:"attributeRef,omitempty"`
	EntityRef      string          `json:"entityRef,omitempty"`
	Expression     string          `json:"expression,omitempty"`
	PrimitiveValue string          `json:"primitiveValue,omitempty"`
	Selection      string          `json:"selection,omitempty"`
	Widgets        []Widget        `json:"widgets,omitempty"`
	Objects        []*WidgetObject `json:"objects,omitempty"`
	TextTemplate   *model.Text     `json:"textTemplate,omitempty"`
	Form           string          `json:"form,omitempty"`
	Microflow      string          `json:"microflow,omitempty"`
	Nanoflow       string          `json:"nanoflow,omitempty"`
	Image          string          `json:"image,omitempty"`
}

// Predefined CustomWidget IDs for common pluggable widgets
const (
	WidgetIDGallery                = "com.mendix.widget.web.gallery.Gallery"
	WidgetIDDataGrid2              = "com.mendix.widget.web.datagrid.Datagrid"
	WidgetIDDataGridTextFilter     = "com.mendix.widget.web.datagridtextfilter.DatagridTextFilter"
	WidgetIDDataGridDateFilter     = "com.mendix.widget.web.datagriddatefilter.DatagridDateFilter"
	WidgetIDDataGridDropdownFilter = "com.mendix.widget.web.datagriddropdownfilter.DatagridDropdownFilter"
	WidgetIDDataGridNumberFilter   = "com.mendix.widget.web.datagridnumberfilter.DatagridNumberFilter"
	WidgetIDComboBox               = "com.mendix.widget.web.combobox.Combobox"
	WidgetIDImage                  = "com.mendix.widget.web.image.Image"
)

// Report Widgets

// ReportPane represents a report pane widget.
type ReportPane struct {
	BaseWidget
	ReportWidgets []Widget `json:"reportWidgets,omitempty"`
}

// ReportChart represents a report chart widget.
type ReportChart struct {
	BaseWidget
	ChartType ReportChartType `json:"chartType"`
}

// ReportChartType represents the type of report chart.
type ReportChartType string

const (
	ReportChartTypeLine ReportChartType = "Line"
	ReportChartTypeBar  ReportChartType = "Bar"
	ReportChartTypePie  ReportChartType = "Pie"
)

// ReportParameter represents a report parameter widget.
type ReportParameter struct {
	BaseWidget
	ParameterID model.ID `json:"parameterId"`
}

// ReportDateRangeSelector represents a date range selector for reports.
type ReportDateRangeSelector struct {
	BaseWidget
	FromParameter model.ID `json:"fromParameter"`
	ToParameter   model.ID `json:"toParameter"`
}

// Visibility and Conditional Rendering

// ConditionalContainer represents a conditionally visible container.
type ConditionalContainer struct {
	BaseWidget
	Condition          VisibilityCondition `json:"condition,omitempty"`
	Widgets            []Widget            `json:"widgets,omitempty"`
	AlternativeWidgets []Widget            `json:"alternativeWidgets,omitempty"`
}

// VisibilityCondition represents a visibility condition.
type VisibilityCondition interface {
	isVisibilityCondition()
}

// AttributeCondition bases visibility on an attribute.
type AttributeCondition struct {
	model.BaseElement
	AttributePath string            `json:"attributePath"`
	Operator      ConditionOperator `json:"operator"`
	Value         string            `json:"value,omitempty"`
}

func (AttributeCondition) isVisibilityCondition() {}

// ConditionOperator represents a condition operator.
type ConditionOperator string

const (
	ConditionOperatorEquals    ConditionOperator = "Equals"
	ConditionOperatorNotEquals ConditionOperator = "NotEquals"
	ConditionOperatorEmpty     ConditionOperator = "Empty"
	ConditionOperatorNotEmpty  ConditionOperator = "NotEmpty"
)

// ModuleRoleCondition bases visibility on module roles.
type ModuleRoleCondition struct {
	model.BaseElement
	Roles []model.ID `json:"roles,omitempty"`
}

func (ModuleRoleCondition) isVisibilityCondition() {}

// ExpressionCondition uses an expression.
type ExpressionCondition struct {
	model.BaseElement
	Expression string `json:"expression"`
}

func (ExpressionCondition) isVisibilityCondition() {}
