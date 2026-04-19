// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// PageMutator provides fine-grained mutation operations on a single
// page, layout, or snippet unit. Obtain one via PageMutationBackend.OpenPageForMutation.
// All methods operate on the in-memory representation; call Save to persist.
type PageMutator interface {
	// ContainerType returns "page", "layout", or "snippet".
	ContainerType() string

	// --- Widget property operations ---

	// SetWidgetProperty sets a simple property on the named widget.
	// For pluggable widget properties, prop is the Mendix property key
	// and value is the string representation.
	SetWidgetProperty(widgetRef string, prop string, value any) error

	// SetWidgetDataSource sets the DataSource on the named widget.
	SetWidgetDataSource(widgetRef string, ds pages.DataSource) error

	// SetColumnProperty sets a property on a column within a grid widget.
	SetColumnProperty(gridRef string, columnRef string, prop string, value any) error

	// --- Widget tree operations ---

	// InsertWidget inserts serialized widgets at the given position
	// relative to the target widget. Position is "before" or "after".
	InsertWidget(targetWidget string, position string, widgets []pages.Widget) error

	// DropWidget removes widgets by name from the tree.
	DropWidget(widgetRefs []string) error

	// ReplaceWidget replaces the target widget with the given widgets.
	ReplaceWidget(targetWidget string, widgets []pages.Widget) error

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
	// propKey is the Mendix property key, opName is the operation type
	// ("attribute", "association", "primitive", "selection", "datasource",
	// "widgets", "texttemplate", "action", "attributeObjects").
	// ctx carries the operation-specific values.
	SetPluggableProperty(widgetRef string, propKey string, opName string, ctx PluggablePropertyContext) error

	// --- Introspection ---

	// EnclosingEntity returns the qualified entity name for the given widget's
	// data context, or "" if none.
	EnclosingEntity(widgetRef string) string

	// WidgetScope returns a map of widget name → unit ID for all widgets in the tree.
	WidgetScope() map[string]model.ID

	// Save persists the mutations to the backend.
	Save() error
}

// PluggablePropertyContext carries operation-specific values for
// SetPluggableProperty. Only fields relevant to the operation are used.
type PluggablePropertyContext struct {
	AttributePath  string           // "attribute", "association"
	AttributePaths []string         // "attributeObjects"
	AssocPath      string           // "association"
	EntityName     string           // "association"
	PrimitiveVal   string           // "primitive"
	DataSource     pages.DataSource // "datasource"
	ChildWidgets   []pages.Widget   // "widgets"
	Action         pages.ClientAction // "action"
	TextTemplate   string           // "texttemplate"
	Selection      string           // "selection"
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

	InsertPath(activityRef string, atPos int, pathCaption string, activities []workflows.WorkflowActivity) error
	DropPath(activityRef string, atPos int, pathCaption string) error

	// --- Branch operations (exclusive split) ---

	InsertBranch(activityRef string, atPos int, condition string, activities []workflows.WorkflowActivity) error
	DropBranch(activityRef string, atPos int, branchName string) error

	// --- Boundary event operations ---

	InsertBoundaryEvent(activityRef string, atPos int, eventType string, delay string, activities []workflows.WorkflowActivity) error
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
