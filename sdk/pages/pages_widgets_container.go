// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// Container Widgets

// LayoutGrid represents a layout grid container.
type LayoutGrid struct {
	BaseWidget
	Rows []*LayoutGridRow `json:"rows,omitempty"`
}

// LayoutGridRow represents a row in a layout grid.
type LayoutGridRow struct {
	model.BaseElement
	Columns []*LayoutGridColumn `json:"columns,omitempty"`
}

// LayoutGridColumn represents a column in a layout grid.
type LayoutGridColumn struct {
	model.BaseElement
	Weight       int      `json:"weight"`
	TabletWeight int      `json:"tabletWeight"`
	PhoneWeight  int      `json:"phoneWeight"`
	Widgets      []Widget `json:"widgets,omitempty"`
}

// Container represents a generic container widget.
type Container struct {
	BaseWidget
	Widgets       []Widget            `json:"widgets,omitempty"`
	RenderMode    ContainerRenderMode `json:"renderMode,omitempty"`
	OnClickAction ClientAction        `json:"onClickAction,omitempty"` // optional "On click" action (clickable container)
}

// ContainerRenderMode represents how a container is rendered.
type ContainerRenderMode string

const (
	ContainerRenderModeDiv  ContainerRenderMode = "Div"
	ContainerRenderModeForm ContainerRenderMode = "Form"
)

// GroupBox represents a group box container.
type GroupBox struct {
	BaseWidget
	Caption     *ClientTemplate `json:"captionTemplate,omitempty"`
	Collapsible string          `json:"collapsible"` // "No", "YesInitiallyCollapsed", "YesInitiallyExpanded"
	HeaderMode  string          `json:"headerMode"`  // "Div", "H1"-"H6"
	Widgets     []Widget        `json:"widgets,omitempty"`
}

// TabContainer represents a tab container.
type TabContainer struct {
	BaseWidget
	TabPages      []*TabPage `json:"tabPages,omitempty"`
	DefaultPageID model.ID   `json:"defaultPageId,omitempty"`
}

// TabPage represents a page within a tab container.
type TabPage struct {
	model.BaseElement
	Name          string      `json:"name"`
	Caption       *model.Text `json:"caption,omitempty"`
	Widgets       []Widget    `json:"widgets,omitempty"`
	RefreshOnShow bool        `json:"refreshOnShow,omitempty"`
}

// GetName returns the tab page's name.
func (tp *TabPage) GetName() string {
	return tp.Name
}

// ScrollContainer represents a scrollable container.
type ScrollContainer struct {
	BaseWidget
	ScrollBehavior ScrollBehavior `json:"scrollBehavior"`
	Widgets        []Widget       `json:"widgets,omitempty"`
}

// ScrollBehavior represents how scrolling behaves.
type ScrollBehavior string

const (
	ScrollBehaviorVertical   ScrollBehavior = "Vertical"
	ScrollBehaviorHorizontal ScrollBehavior = "Horizontal"
	ScrollBehaviorBoth       ScrollBehavior = "Both"
)
