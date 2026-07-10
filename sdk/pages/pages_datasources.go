// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// Data Sources

// DataSource represents a data source for widgets.
type DataSource interface {
	isDataSource()
}

// EntityPathSource retrieves data via an entity path.
type EntityPathSource struct {
	model.BaseElement
	EntityPath string `json:"entityPath"`
}

func (EntityPathSource) isDataSource() {}

// DatabaseSource retrieves data from the database.
type DatabaseSource struct {
	model.BaseElement
	EntityID        model.ID    `json:"entityId"`
	EntityName      string      `json:"entityName,omitempty"` // Qualified name e.g. "Module.Entity"
	XPathConstraint string      `json:"xPathConstraint,omitempty"`
	Sorting         []*GridSort `json:"sorting,omitempty"`
}

func (DatabaseSource) isDataSource() {}

// GridSort represents sorting configuration.
type GridSort struct {
	model.BaseElement
	AttributePath string        `json:"attributePath"`
	Direction     SortDirection `json:"direction"`
}

// SortDirection represents the sort direction.
type SortDirection string

const (
	SortDirectionAscending  SortDirection = "Ascending"
	SortDirectionDescending SortDirection = "Descending"
)

// MicroflowSource retrieves data from a microflow.
type MicroflowSource struct {
	model.BaseElement
	MicroflowID model.ID `json:"microflowId"`
	Microflow   string   `json:"microflow"` // Qualified name (e.g., "Module.MicroflowName")
}

func (MicroflowSource) isDataSource() {}

// NanoflowSource retrieves data from a nanoflow.
type NanoflowSource struct {
	model.BaseElement
	NanoflowID model.ID `json:"nanoflowId"`
	Nanoflow   string   `json:"nanoflow"` // Qualified name (e.g., "Module.NanoflowName")
}

func (NanoflowSource) isDataSource() {}

// ListenToWidgetSource listens to another widget (selection datasource).
// BSON type is Forms$ListenTargetSource with ListenTarget as widget name.
type ListenToWidgetSource struct {
	model.BaseElement
	WidgetID   model.ID `json:"widgetId"`             // Widget ID for internal reference
	WidgetName string   `json:"widgetName,omitempty"` // Widget name for BSON serialization (ListenTarget field)
}

func (ListenToWidgetSource) isDataSource() {}

// AssociationSource retrieves data via association.
type AssociationSource struct {
	model.BaseElement
	EntityPath      string `json:"entityPath"`                // "Module.Assoc" or "Module.Assoc/Module.DestEntity"
	ContextVariable string `json:"contextVariable,omitempty"` // page parameter name (without $) — empty for $currentObject
}

func (AssociationSource) isDataSource() {}

// DataViewSource is the datasource for DataView widgets using page/snippet parameters.
type DataViewSource struct {
	model.BaseElement
	EntityID           model.ID `json:"entityId,omitempty"`           // Entity reference (qualified name stored here)
	EntityName         string   `json:"entityName,omitempty"`         // Qualified entity name for BSON serialization
	ParameterName      string   `json:"parameterName,omitempty"`      // Name of page/snippet parameter (without $)
	IsSnippetParameter bool     `json:"isSnippetParameter,omitempty"` // True if this is a snippet parameter, false for page parameter
}

func (DataViewSource) isDataSource() {}
