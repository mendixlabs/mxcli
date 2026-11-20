// SPDX-License-Identifier: Apache-2.0

package meta

// The System module's built-in microflows and image collections.
//
// Like its Java actions (system_java_actions.go) they are not stored in the
// .mpr — Mendix ships them with the platform — so a reference check that only
// lists the project calls them missing. `describe navigation` prints
// `call microflow System.ShowHomePage` for the Home item of many starter apps,
// and check refused that output (ako/mxcli#2247); `System.Images.Completed`
// was "image collection not found" although mx check accepts it
// (ako/mxcli#2235).
//
// Measured on Mendix 11.12.6 with
// `mx dump-mpr --module-names=System --unit-type='Microflows$Microflow,Microflows$Nanoflow,Images$ImageCollection'`
// on a blank project: one microflow, no nanoflows, two image collections.

// SystemMicroflows are the System module's microflows, qualified.
var SystemMicroflows = []string{"System.ShowHomePage"}

// SystemImageCollections are the System module's image collections, qualified,
// with the images each held on 11.12.6. Other versions may hold more or fewer
// images, so a caller that resolves an image inside one should accept a name
// not listed here rather than refuse it; the collection names are stable.
var SystemImageCollections = map[string][]string{
	"System.Images": {
		"Add", "Box_plus", "Box_share", "CSV", "Cancel", "Close", "Completed",
		"Delete", "DeselectAll", "Desktop", "Edit", "Error", "Excel", "Hammer",
		"Microflow", "Module", "New", "Phone", "Remove", "Running", "Save",
		"Search", "Select", "SelectAll", "Tablet", "key_go",
	},
	"System.WorkflowImages": {
		"WorkflowAIAgentTask", "WorkflowCallMicroflow", "WorkflowCallWorkflow",
		"WorkflowDecision", "WorkflowEndOfBoundaryEventPath", "WorkflowEndOfSplit",
		"WorkflowEventEnd", "WorkflowEventStart", "WorkflowInterruptingNotificationEvent",
		"WorkflowInterruptingNotificationEventSubProcessStart", "WorkflowInterruptingTimerEvent",
		"WorkflowJumpTo", "WorkflowMultiUserTask", "WorkflowNonInterruptingNotificationEvent",
		"WorkflowNonInterruptingNotificationEventSubProcessStart", "WorkflowNonInterruptingTimerEvent",
		"WorkflowNotification", "WorkflowParallelSplit", "WorkflowTimer", "WorkflowUserTask",
		"WorkflowWaitForNotification",
	},
}
