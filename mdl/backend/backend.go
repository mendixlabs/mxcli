// SPDX-License-Identifier: Apache-2.0

package backend

// FullBackend composes every domain backend into a single interface.
// Implementations must satisfy all sub-interfaces.
//
// Handler functions receive the specific sub-interface they need via
// ExecContext; FullBackend exists primarily as a construction-time
// constraint on backend implementations.
type FullBackend interface {
	ConnectionBackend
	ModuleBackend
	FolderBackend
	DomainModelBackend
	MicroflowBackend
	PageBackend
	EnumerationBackend
	ConstantBackend
	SecurityBackend
	NavigationBackend
	ServiceBackend
	MappingBackend
	JavaBackend
	WorkflowBackend
	SettingsBackend
	ImageBackend
	ScheduledEventBackend
	RenameBackend
	RawUnitBackend
	MetadataBackend
	WidgetBackend
	AgentEditorBackend
	PageMutationBackend
	WorkflowMutationBackend
	WidgetSerializationBackend
}
