// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// ContainerKind represents the type of page container (page, layout, or snippet).
type ContainerKind string

// PageParameterSpec is a page or snippet parameter for PageMutator.AddParameter.
// Exactly one of PrimitiveType and EntityName is set: PrimitiveType is the
// primitive's DataTypes $Type (e.g. "DataTypes$StringType"), EntityName the
// qualified name of the entity an object parameter points at.
type PageParameterSpec struct {
	Name          string
	PrimitiveType string
	EntityName    string
	// Optional writes a page parameter with IsRequired false (11.5+,
	// mendixlabs/mxcli#1640). A snippet parameter has no such flag.
	Optional bool
}

const (
	ContainerPage    ContainerKind = "page"
	ContainerLayout  ContainerKind = "layout"
	ContainerSnippet ContainerKind = "snippet"
)

// InsertPosition represents where a widget is inserted relative to a target.
type InsertPosition string

const (
	InsertBefore InsertPosition = "before"
	InsertAfter  InsertPosition = "after"
)

// PluggablePropertyOp represents the operation type for SetPluggableProperty.
type PluggablePropertyOp string

const (
	PluggableOpAttribute        PluggablePropertyOp = "attribute"
	PluggableOpAssociation      PluggablePropertyOp = "association"
	PluggableOpPrimitive        PluggablePropertyOp = "primitive"
	PluggableOpSelection        PluggablePropertyOp = "selection"
	PluggableOpDataSource       PluggablePropertyOp = "datasource"
	PluggableOpWidgets          PluggablePropertyOp = "widgets"
	PluggableOpTextTemplate     PluggablePropertyOp = "texttemplate"
	PluggableOpAction           PluggablePropertyOp = "action"
	PluggableOpAttributeObjects PluggablePropertyOp = "attributeObjects"
)

// WidgetRef identifies a widget or a column within a widget.
type WidgetRef struct {
	Widget string
	Column string // empty for non-column targeting
}

// IsColumn returns true if this targets a column within a widget.
func (r WidgetRef) IsColumn() bool { return r.Column != "" }

// Name returns the full reference string for error messages.
func (r WidgetRef) Name() string {
	if r.IsColumn() {
		return joinColumnAddress(r.Widget, r.Column)
	}
	return r.Widget
}

// PageMutator provides fine-grained mutation operations on a single
// page, layout, or snippet unit. Obtain one via PageMutationBackend.OpenPageForMutation.
// All methods operate on the in-memory representation; call Save to persist.
//
// Widget addressing: most methods accept a widgetRef string (widget name).
// Column-aware operations additionally accept a columnRef string.
// DropWidget uses []WidgetRef to support mixed widget/column targets in a
// single call.
type PageMutator interface {
	// ContainerType returns the kind of container (page, layout, or snippet).
	ContainerType() ContainerKind

	// AlterTargetResolver resolves a generic ALTER target against this widget
	// tree: the page family's half of `alter <type> X { … }` (ADR-0012). The
	// executor resolves every operation's target through it before applying
	// the operation, so each document type answers "what does this address
	// mean" in one place.
	AlterTargetResolver

	// --- Widget property operations ---

	// SetWidgetProperty sets a simple property on the named widget.
	// For pluggable widget properties, prop is the Mendix property key
	// and value is the string representation.
	SetWidgetProperty(widgetRef string, prop string, value any) error

	// SetWidgetDataSource sets the DataSource on the named widget.
	SetWidgetDataSource(widgetRef string, ds pages.DataSource) error

	// SetWidgetAction retargets the on-click action of the named widget,
	// under the key its type stores it in (Action, OnClickAction or
	// ClickAction — types.WidgetClickActionKeys). Refuses a widget with none.
	SetWidgetAction(widgetRef string, action pages.ClientAction) error

	// SetWidgetNamedAction writes an action into a pluggable widget's action
	// slot addressed by the widget's own property key (e.g. the File Uploader's
	// createFileAction). Refuses a key that is not an action-typed property.
	SetWidgetNamedAction(widgetRef string, propertyKey string, action pages.ClientAction) error

	// SetColumnProperty sets a property on a column within a grid widget.
	SetColumnProperty(gridRef string, columnRef string, prop string, value any) error

	// --- Design property (Atlas styling) operations ---

	// SetDesignProperty sets or updates an Atlas design property on the named
	// widget's Appearance. valueType is "toggle" (no value) or "option" (carries
	// option). An existing entry's value is fully rewritten to the new valueType:
	// an option-type set on a stale "custom" value (ToggleButtonGroup/ColorPicker)
	// overwrites it with an option value, repairing the CE6084 a Custom encoding
	// triggers.
	SetDesignProperty(widgetRef string, key string, valueType string, option string) error

	// RemoveDesignProperty removes a single design property by key from the named
	// widget (e.g. a toggle set to OFF).
	RemoveDesignProperty(widgetRef string, key string) error

	// ClearDesignProperties removes all design properties from the named widget.
	ClearDesignProperties(widgetRef string) error

	// --- Widget tree operations ---

	// InsertWidget inserts serialized widgets at the given position
	// relative to the target widget or column. Position is "before" or "after".
	// columnRef is "" for widget targeting; non-empty for column targeting.
	InsertWidget(widgetRef string, columnRef string, position InsertPosition, widgets []pages.Widget) error

	// DropWidget removes widgets by ref from the tree.
	DropWidget(refs []WidgetRef) error

	// ReplaceWidget replaces the target widget or column with the given widgets.
	// columnRef is "" for widget targeting.
	ReplaceWidget(widgetRef string, columnRef string, widgets []pages.Widget) error

	// InsertColumns inserts new DataGrid2 columns relative to an existing column.
	// Used when the INSERT target is a column ref (e.g., `grid.colName`).
	// Columns are serialized as CustomWidgets$WidgetObject, not as form widgets.
	InsertColumns(gridRef string, afterColumnRef string, position InsertPosition, columns []*DataGridColumnSpec) error

	// InsertListViewTemplates appends specialization templates to a List View.
	//
	// Templates live in the list view's Templates array, NOT in its Widgets array
	// (which holds the default body), and a Forms$ListViewTemplate is not a
	// widget — the same reason DataGrid2 columns get their own method above.
	// Routing them through InsertWidget would append a non-widget to the widget
	// list, which Studio Pro cannot open.
	InsertListViewTemplates(listViewRef string, templates []*pages.ListViewTemplate) error

	// InsertTabPages adds tab pages to a tab container: INTO the container
	// appends, BEFORE/AFTER a tab page places them next to that sibling. A tab
	// page lives in the container's TabPages list and is not a widget, so it
	// cannot go through InsertWidget (#1215).
	InsertTabPages(targetRef string, position InsertPosition, tabPages []*pages.TabPage) error

	// InsertNavigationListItems adds items to a navigation list: INTO the list
	// appends, BEFORE/AFTER an item places them next to it. An item lives in the
	// list's Items and is not a widget, so it cannot go through InsertWidget. It
	// has no Name either, so it is addressed as backend.NavItemSelector spells
	// it — `nav item(t1)`, `nav item('Home')`, `nav item@2` (mendixlabs/mxcli#1388).
	InsertNavigationListItems(targetRef string, position InsertPosition, items []*pages.NavigationListItem) error

	// ReplaceNavigationListItem replaces the item targetRef addresses with the
	// given items, in its place (mendixlabs/mxcli#1388).
	ReplaceNavigationListItem(targetRef string, items []*pages.NavigationListItem) error

	// DropListViewTemplate removes the template rendering the given
	// specialization from a List View. A template has no name, so it is addressed
	// by entity. Returns an error naming the templates that ARE present when the
	// entity does not match one — silently dropping nothing would read as success.
	DropListViewTemplate(listViewRef string, specialization string) error

	// ReplaceColumn replaces a single DataGrid2 column with new columns.
	// Columns are serialized as CustomWidgets$WidgetObject, not as form widgets.
	ReplaceColumn(gridRef string, columnRef string, columns []*DataGridColumnSpec) error

	// FindWidget checks if a widget with the given name exists in the tree.
	FindWidget(name string) bool

	// --- Variable operations ---

	// AddVariable adds a local variable to the page/snippet.
	AddVariable(name, dataType, defaultValue string) error

	// DropVariable removes a local variable by name.
	DropVariable(name string) error

	// --- Parameter operations ---

	// AddParameter declares a new page or snippet parameter. It refuses a name
	// already used by a parameter or a variable, and a layout, which has none.
	AddParameter(param PageParameterSpec) error

	// DropParameter removes a parameter by name. It refuses one the document
	// still references — a data source or an expression bound to it — since
	// dropping it would leave a reference that resolves to nothing.
	DropParameter(name string) error

	// --- Layout operations ---

	// SetLayout changes the layout reference and remaps placeholder parameters.
	SetLayout(newLayout string, paramMappings map[string]string) error

	// BoundPlaceholders returns the layout placeholder names this page binds
	// content to, unqualified (e.g. "Main", "HeaderLeft").
	//
	// The caller needs these to check a repoint before it happens: a page bound
	// to a placeholder the new layout does not declare gets a Parameter pointing
	// at nothing, which mxbuild reports as CE1613 at the far end of the run
	// rather than at the statement that caused it.
	BoundPlaceholders() []string

	// --- Pluggable widget operations ---

	// SetPluggableProperty sets a typed property on a pluggable widget's object.
	// propKey is the Mendix property key, op identifies the operation type,
	// and ctx carries the operation-specific values.
	SetPluggableProperty(widgetRef string, propKey string, op PluggablePropertyOp, ctx PluggablePropertyContext) error

	// --- Introspection ---

	// EnclosingEntity returns the qualified entity name for the given widget's
	// data context, or "" if none.
	EnclosingEntity(widgetRef string) string

	// EnclosingEntityForChildren returns the entity context that applies to
	// children of the named widget — i.e., the widget's own data source entity
	// if it has one (DataView, DataGrid, ListView, DataGrid2), otherwise the
	// surrounding enclosing entity. Used for column inserts/replaces.
	EnclosingEntityForChildren(widgetRef string) string

	// EnclosingDataSourceFlow returns the microflow/nanoflow qualified name of the
	// datasource governing widgetRef's context, or "","" when that source is not a
	// flow (database/association, which EnclosingEntity[ForChildren] already
	// resolve). A microflow/nanoflow datasource's entity is its RETURN type, which
	// lives in the flow document, not the datasource BSON — so the caller resolves
	// the returned qualified name to an entity via the model. forChildren consults
	// the widget's OWN datasource (INSERT INTO / column inserts); otherwise the
	// nearest ENCLOSING datasource (sibling INSERT BEFORE/AFTER, REPLACE).
	EnclosingDataSourceFlow(widgetRef string, forChildren bool) (microflow, nanoflow string)

	// WidgetScope returns a map of widget name → unit ID for all widgets in the tree.
	WidgetScope() map[string]model.ID

	// ParamScope returns page/snippet parameter maps:
	// paramIDs maps param name → entity ID, paramEntityNames maps param name → qualified entity name.
	ParamScope() (paramIDs map[string]model.ID, paramEntityNames map[string]string)

	// Save persists the mutations to the backend.
	Save() error
}

// PluggablePropertyContext carries operation-specific values for
// SetPluggableProperty. Only fields relevant to the operation are used.
type PluggablePropertyContext struct {
	AttributePath  string             // "attribute", "association"
	AttributePaths []string           // "attributeObjects"
	AssocPath      string             // "association"
	EntityName     string             // "association"
	PrimitiveVal   string             // "primitive"
	DataSource     pages.DataSource   // "datasource"
	ChildWidgets   []pages.Widget     // "widgets"
	Action         pages.ClientAction // "action"
	TextTemplate   string             // "texttemplate"
	Selection      string             // "selection"
}

// WorkflowMutator provides fine-grained mutation operations on a single
// workflow unit. Obtain one via WorkflowMutationBackend.OpenWorkflowForMutation.
// All methods operate on the in-memory representation; call Save to persist.
type WorkflowMutator interface {
	// AlterTargetResolver resolves a generic ALTER target against this
	// workflow's activities: the workflow's half of `alter workflow X { … }`
	// (ADR-0012). An activity is addressed by its name or its caption, @n
	// choosing one of several matches; a name wins over a caption that repeats
	// it. The executor resolves every operation's target through it first, so
	// a miss or an ambiguity refuses the statement before anything changes.
	AlterTargetResolver

	// --- Top-level property operations ---

	// SetProperty sets a workflow-level property (DisplayName, Description,
	// ExportLevel, DueDate, Parameter, OverviewPage).
	SetProperty(prop string, value string) error

	// SetPropertyWithEntity sets a workflow-level property that references
	// an entity (e.g. Parameter).
	SetPropertyWithEntity(prop string, value string, entity string) error

	// --- Activity operations ---

	// SetActivityProperty sets a property on an activity identified by
	// caption and optional position index.
	SetActivityProperty(activityRef string, atPos int, prop string, value string) error

	// SetTemplateParameters sets the expressions filling a text's {1}, {2}, …
	// in order: the workflow's "display" or "description" when activityRef is
	// "", else a user task's "task_name" or "description". Empty clears them.
	SetTemplateParameters(activityRef string, atPos int, prop string, params []string) error

	// InsertAfterActivity inserts new activities after the referenced activity.
	InsertAfterActivity(activityRef string, atPos int, activities []workflows.WorkflowActivity) error

	// InsertBeforeActivity inserts new activities before the referenced activity.
	InsertBeforeActivity(activityRef string, atPos int, activities []workflows.WorkflowActivity) error

	// DropActivity removes the referenced activity.
	DropActivity(activityRef string, atPos int) error

	// ReplaceActivity replaces the referenced activity with new ones.
	ReplaceActivity(activityRef string, atPos int, activities []workflows.WorkflowActivity) error

	// --- Outcome operations ---

	// InsertOutcome adds a new outcome to the referenced activity.
	InsertOutcome(activityRef string, atPos int, outcomeName string, activities []workflows.WorkflowActivity) error

	// DropOutcome removes an outcome by name from the referenced activity.
	DropOutcome(activityRef string, atPos int, outcomeName string) error

	// --- Path operations (parallel split) ---

	// InsertPath adds a new path to a parallel split activity.
	InsertPath(activityRef string, atPos int, pathCaption string, activities []workflows.WorkflowActivity) error

	// DropPath removes a path by caption from a parallel split activity.
	DropPath(activityRef string, atPos int, pathCaption string) error

	// --- Branch operations (exclusive split) ---

	// InsertBranch adds a new branch with a condition to an exclusive split activity.
	InsertBranch(activityRef string, atPos int, condition string, activities []workflows.WorkflowActivity) error

	// DropBranch removes a branch by name from an exclusive split activity.
	DropBranch(activityRef string, atPos int, branchName string) error

	// --- Boundary event operations ---

	// InsertBoundaryEvent adds a boundary event to the referenced activity.
	InsertBoundaryEvent(activityRef string, atPos int, eventType string, delay string, caption string, recurrence *workflows.Recurrence, activities []workflows.WorkflowActivity) error

	// DropBoundaryEvent removes the boundary event from the referenced activity.
	DropBoundaryEvent(activityRef string, atPos int) error

	// Save persists the mutations to the backend.
	Save() error
}

// PageMutationBackend provides page/layout/snippet mutation capabilities.
type PageMutationBackend interface {
	// OpenPageForMutation loads a page, layout, or snippet unit and returns
	// a mutator for applying changes. Call Save() on the returned mutator
	// to persist.
	OpenPageForMutation(unitID model.ID) (PageMutator, error)
}

// WorkflowMutationBackend provides workflow mutation capabilities.
type WorkflowMutationBackend interface {
	// OpenWorkflowForMutation loads a workflow unit and returns a mutator
	// for applying changes. Call Save() on the returned mutator to persist.
	OpenWorkflowForMutation(unitID model.ID) (WorkflowMutator, error)
}

// WidgetObjectBuilder provides storage-agnostic operations on a loaded pluggable widget template.
// The executor calls these methods with domain-typed values; the backend handles
// all storage-specific manipulation internally.
//
// Workflow: LoadTemplate → apply operations → EnsureRequiredObjectLists → Finalize
type WidgetObjectBuilder interface {
	// --- Property operations ---
	// Each operation finds the property by key (via TypePointer matching) and updates its value.
	// Set* methods do not return errors — invalid operations are logged as warnings
	// and deferred to Finalize, which returns the aggregate result.

	SetAttribute(propertyKey string, attributePath string)
	SetAssociation(propertyKey string, assocPath string, entityName string)
	// SetSourceVariable names the object an attribute or association property
	// reads, when that is not the enclosing data context: a page or snippet
	// parameter, stored as the WidgetValue's Forms$PageVariable (`$Param.Attr`).
	SetSourceVariable(propertyKey string, sv *pages.WidgetVariable)
	SetPrimitive(propertyKey string, value string)
	SetSelection(propertyKey string, value string)
	// SetImage points an image-typed property (e.g. the Image widget's
	// `imageObject`) at an image collection entry, by its three-part qualified
	// name `Module.Collection.Image`.
	SetImage(propertyKey string, imageQN string)
	// SetImageEntity binds an image-typed property to the object in context,
	// an entity specializing System.Image, rather than a collection entry.
	SetImageEntity(propertyKey string, entityName string)
	SetExpression(propertyKey string, value string)
	SetDataSource(propertyKey string, ds pages.DataSource)
	SetChildWidgets(propertyKey string, children []pages.Widget)
	SetTextTemplate(propertyKey string, text string)
	SetTextTemplateWithParams(propertyKey string, text string, entityContext string)
	// SetTextTemplateWithClientParams sets a text template whose `{1}`-style
	// placeholders are backed by parameters the AUTHOR supplied (MDL
	// `contentparams:`), rather than derived from `{AttrName}` placeholders
	// against the entity context. Both spellings reach the same stored shape;
	// without this one a pluggable widget's `{1}` template was written with an
	// empty parameter list, which mxbuild rejects with CE0720 (#928).
	SetTextTemplateWithClientParams(propertyKey string, text string, params []*pages.ClientTemplateParameter)
	SetAction(propertyKey string, action pages.ClientAction)
	SetAttributeObjects(propertyKey string, attributePaths []string)

	// SetObjectList sets a list of structured items on an object-list property
	// (e.g. Accordion `groups`, PopupMenu `basicItems`, DataGrid `columns` —
	// for widgets routed through the generic pluggable engine, not the dedicated
	// DataGrid builder). The backend uses the template's nested PropertyTypeIDs
	// to convert each spec entry into the correct BSON shape.
	SetObjectList(propertyKey string, items []ObjectListItemSpec)

	// --- Template metadata ---

	// PropertyTypeIDs returns the property type metadata for the loaded template.
	PropertyTypeIDs() map[string]pages.PropertyTypeIDEntry

	// PrimitiveValues returns each property's current comparable value in the
	// object being built. Read before the mappings are applied it is the
	// template's captured configuration, which is what a property MDL cannot
	// name will actually be stored with.
	PrimitiveValues() map[string]string

	// --- Object list defaults ---

	// EnsureRequiredObjectLists auto-populates required empty object lists.
	EnsureRequiredObjectLists()

	// --- Property visibility ---

	// ApplyPropertyVisibility nulls the TextTemplate of any TextTemplate-typed
	// property the rules mark as hidden under the widget's current property
	// values, matching Studio Pro's editorConfig.js-driven behavior (#574).
	ApplyPropertyVisibility(rules []types.WidgetVisibilityRule)

	// --- Gallery-specific ---

	// CloneGallerySelectionProperty clones the itemSelection property with a new Selection value.
	CloneGallerySelectionProperty(propertyKey string, selectionMode string)

	// --- Finalize ---

	// Finalize builds the CustomWidget from the mutated template.
	// Returns the widget with RawType/RawObject populated from internal state.
	Finalize(id model.ID, name string, label string, editable string) *pages.CustomWidget
}

// WidgetPropertyDeclarer is implemented by a WidgetObjectBuilder that holds the
// widget's full property set, and so can say authoritatively whether a key
// exists. Every Set* operation on a key the widget does not declare is a no-op,
// so a definition mapping an authored value to such a key drops the value with
// no error (mendixlabs/mxcli#1519: a drop-down filter's attribute, routed to the
// text filter's `attributes`). The engine asks this before applying an authored
// value and refuses the statement instead. Optional: a builder that only
// records property sets (the Studio Pro MCP backend) cannot answer and does not
// implement it.
type WidgetPropertyDeclarer interface {
	DeclaresProperty(propertyKey string) bool
}

// WidgetEnumerationDeclarer is implemented by a WidgetObjectBuilder that holds
// the widget's property schema, and so can name the keys an enumeration
// property declares — nil for a property that is not an enumeration. The
// widget compares its keys exactly, so the engine writes an authored value as
// the declared key it matches (ako/mxcli#854). Optional, like
// WidgetPropertyDeclarer.
type WidgetEnumerationDeclarer interface {
	EnumerationKeys(propertyKey string) []string
}

// ColumnHiddenPropertyResetter is implemented by a PageMutator that can bring a
// Data grid 2 column back in line with its widget's visibility rules after a
// SET: every primitive property a rule now hides goes back to the default the
// stored Type declares (`size` once `width` leaves "manual" — CE0463 otherwise,
// mendixlabs/mxcli#1519). setProps are the MDL names the statement set; a
// hidden one holding a non-default value is refused rather than reset. Returns
// the schema keys reset. Optional: the Studio Pro MCP backend hands property
// sets to Studio Pro, which applies the widget's rules itself.
type ColumnHiddenPropertyResetter interface {
	ResetHiddenColumnProperties(gridRef, columnRef string, rules []types.WidgetVisibilityRule, setProps []string) ([]string, error)
}

// SourceScope is what one data source hands to the widgets it governs: the
// entity it names, or the microflow/nanoflow whose return entity it is — which
// lives in the flow document, so only a caller holding the model can resolve it.
type SourceScope struct {
	Entity    string
	Microflow string
	Nanoflow  string
}

// PluggablePropertyScope describes one top-level property of a stored pluggable
// widget, for a SET that has to bind it: its kind, the widget's own datasource
// it is linked to (widget.xml `dataSource="…"`, "" when none), and the scope of
// that datasource and of the enclosing data container.
type PluggablePropertyScope struct {
	Key        string // the property key as the template spells it
	Kind       string // ValueType.Type
	DataSource string // the linked datasource property key, "" when linked to none
	Linked     SourceScope
	Enclosing  SourceScope
	// EnumKeys are the keys an Enumeration-kind property declares, so the
	// executor can write a value as the key it names (ako/mxcli#854).
	EnumKeys []string
}

// PluggablePropertyScoper is implemented by a PageMutator that can describe a
// stored pluggable widget's property, so the executor can resolve an attribute
// binding for ALTER … SET by the rule CREATE uses (ako/mxcli#1622). ok
// is false when widgetRef is not a pluggable widget or has no such property.
// Optional: the Studio Pro MCP backend has no pluggable SET path.
type PluggablePropertyScoper interface {
	PluggablePropertyScope(widgetRef, propKey string) (PluggablePropertyScope, bool)
}

// AttributeBinding is the value SetWidgetProperty takes for an Attribute-kind
// pluggable property: the attribute path already resolved by the executor, and
// the page or snippet parameter it is read from (nil for the context object).
type AttributeBinding struct {
	Path           string
	SourceVariable *pages.WidgetVariable
}

// ObjectListItemSpec describes one item of an object-list property (e.g. one
// Accordion group, one PopupMenu basicItem, one Maps marker). The backend
// applies these specs to the template's nested PropertyTypeIDs to produce
// item BSON.
//
// Each property in the item is dispatched by Operation. Only the field
// matching Operation is used; others are ignored. Operations correspond to
// the same names as the engine's top-level operations (primitive, attribute,
// datasource, texttemplate, expression, action) — see PluggablePropertyOp.
type ObjectListItemSpec struct {
	Properties []ObjectListItemProperty
	// ChildWidgets carries pre-built widgets for Widgets-typed sub-properties
	// of the item (e.g. Accordion group's headerContent / content). Keyed by
	// the sub-property's key (matching the def.json itemSlots[].propertyKey).
	ChildWidgets map[string][]pages.Widget
}

// ObjectListItemProperty describes one scalar property within an
// ObjectListItemSpec. Mirrors the engine's PluggablePropertyContext but
// scoped to a list item's sub-properties.
type ObjectListItemProperty struct {
	PropertyKey       string
	Operation         string // primitive | attribute | datasource | texttemplate | expression | action
	PrimitiveVal      string
	AttributePath     string
	AttributeRefSteps []pages.AttributeRefStep // association hops when AttributePath navigates associations (AttributeRef.EntityRef)
	DataSource        pages.DataSource
	TextTemplate      string
	Expression        string
	Action            pages.ClientAction
	EntityContext     string                           // for texttemplate operations needing param resolution
	Parameters        []*pages.ClientTemplateParameter // resolved CaptionParams / ContentParams for texttemplate operations
	// EmptyTemplate, on a texttemplate operation with an empty TextTemplate,
	// forces an empty Forms$ClientTemplate value instead of leaving the field
	// null. A VISIBLE-but-unset texttemplate sub-property (e.g. a chart series'
	// staticName) must serialize as an empty ClientTemplate or Studio Pro flags
	// CE0463 "widget definition changed". Chart series (9a).
	EmptyTemplate bool
	// EmptyText, on a texttemplate operation with an empty TextTemplate, writes
	// a ClientTemplate whose text is ONE empty translation in the authoring
	// language — what the statement said — rather than none. A text with no
	// translation reads, to a rewrite, as one MDL could not express, and
	// canon.CarryTranslations carries the stored one back: `Caption: ''` on a
	// data grid column could never clear a stored caption.
	EmptyText bool
}

// DataGridColumnSpec carries pre-resolved column data for DataGrid2 construction.
// All attribute paths are fully qualified. Child widgets are already built as
// domain objects; the backend serializes them to storage format internally.
type DataGridColumnSpec struct {
	Attribute         string                           // Fully qualified attribute path (empty for action/custom-content columns)
	AttributeRefSteps []pages.AttributeRefStep         // association hops when Attribute navigates associations (AttributeRef.EntityRef)
	Caption           string                           // Column header caption (may be a template like "{1}")
	CaptionParams     []*pages.ClientTemplateParameter // Header TextTemplate parameters (populated when Caption uses placeholders)
	ShowContentAs     string                           // "", "attribute" (default), "dynamicText", or "customContent" (auto-inferred when ChildWidgets is non-empty)
	Content           string                           // Cell body template for ShowContentAs: dynamicText
	ContentParams     []*pages.ClientTemplateParameter // dynamicText TextTemplate parameters
	ChildWidgets      []pages.Widget                   // Pre-built child widgets (for custom-content columns)
	FilterWidget      pages.Widget                     // Pre-built filter widget for the column's filter slot (optional)
	Properties        map[string]any                   // Column properties (Sortable, Resizable, Visible, etc.)
}

// FilterWidgetSpec carries inputs for building a filter widget.
type FilterWidgetSpec struct {
	WidgetID   string // e.g. pages.WidgetIDDataGridTextFilter
	FilterName string // widget name
	// VisibilityRules are the widget's editorConfig-derived property-visibility
	// rules (#574). A filter is built with its default configuration, so any
	// TextTemplate the widget hides in that state must be nulled to avoid CE0463;
	// the executor resolves these from the installed .mpk and the backend applies
	// them via widgetobj.ApplyVisibilityRules.
	VisibilityRules []types.WidgetVisibilityRule
}

// WidgetBuilderBackend provides pluggable widget construction capabilities.
type WidgetBuilderBackend interface {
	// LoadWidgetTemplate loads a widget template by ID and returns a builder
	// for applying property operations. projectPath is used for runtime template
	// augmentation from .mpk files.
	LoadWidgetTemplate(widgetID string, projectPath string) (WidgetObjectBuilder, error)

	// SerializeWidgetToOpaque converts a domain Widget to an opaque form
	// suitable for passing to WidgetObjectBuilder.SetChildWidgets.
	// This replaces the direct mpr.SerializeWidget call.
	SerializeWidgetToOpaque(w pages.Widget) any

	// SerializeDataSourceToOpaque converts a domain DataSource to an opaque
	// form suitable for embedding in widget properties.
	SerializeDataSourceToOpaque(ds pages.DataSource) any

	// BuildCreateAttributeObject creates an attribute object for filter widgets.
	// Returns an opaque value to be collected into attribute object lists.
	BuildCreateAttributeObject(attributePath string, objectTypeID, propertyTypeID, valueTypeID string) (any, error)

	// BuildFilterWidget builds a filter widget (text, number, date, or dropdown filter)
	// for use inside DataGrid2 filtersPlaceholder or CONTROLBAR sections.
	BuildFilterWidget(spec FilterWidgetSpec, projectPath string) (pages.Widget, error)
}
