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
		return r.Widget + "." + r.Column
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

	// --- Widget property operations ---

	// SetWidgetProperty sets a simple property on the named widget.
	// For pluggable widget properties, prop is the Mendix property key
	// and value is the string representation.
	SetWidgetProperty(widgetRef string, prop string, value any) error

	// SetWidgetDataSource sets the DataSource on the named widget.
	SetWidgetDataSource(widgetRef string, ds pages.DataSource) error

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

	// --- Layout operations ---

	// SetLayout changes the layout reference and remaps placeholder parameters.
	SetLayout(newLayout string, paramMappings map[string]string) error

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

	// InsertAfterActivity inserts new activities after the referenced activity.
	InsertAfterActivity(activityRef string, atPos int, activities []workflows.WorkflowActivity) error

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
	InsertBoundaryEvent(activityRef string, atPos int, eventType string, delay string, activities []workflows.WorkflowActivity) error

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

// WidgetSerializationBackend provides widget and activity serialization
// for CREATE paths where the executor builds domain objects that need
// to be converted to the storage format.
type WidgetSerializationBackend interface {
	// SerializeWidget converts a domain Widget to its storage representation.
	// The returned value is opaque to the caller; it is only used as input
	// to mutation operations or passed to the backend for persistence.
	SerializeWidget(w pages.Widget) (any, error)

	// SerializeClientAction converts a domain ClientAction to storage format.
	SerializeClientAction(a pages.ClientAction) (any, error)

	// SerializeDataSource converts a domain DataSource to storage format.
	SerializeDataSource(ds pages.DataSource) (any, error)

	// SerializeWorkflowActivity converts a domain WorkflowActivity to storage format.
	SerializeWorkflowActivity(a workflows.WorkflowActivity) (any, error)
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
	SetPrimitive(propertyKey string, value string)
	SetSelection(propertyKey string, value string)
	SetExpression(propertyKey string, value string)
	SetDataSource(propertyKey string, ds pages.DataSource)
	SetChildWidgets(propertyKey string, children []pages.Widget)
	SetTextTemplate(propertyKey string, text string)
	SetTextTemplateWithParams(propertyKey string, text string, entityContext string)
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
