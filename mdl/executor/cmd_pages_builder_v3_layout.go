// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func (pb *pageBuilder) buildLayoutGridV3(w *ast.WidgetV3) (*pages.LayoutGrid, error) {
	lg := &pages.LayoutGrid{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$LayoutGrid",
			},
			Name: w.Name,
		},
	}

	// Build rows from children
	for _, child := range w.Children {
		if strings.ToUpper(child.Type) == "ROW" {
			row, err := pb.buildLayoutGridRowV3(child)
			if err != nil {
				return nil, err
			}
			lg.Rows = append(lg.Rows, row)
		}
	}

	return lg, nil
}

func (pb *pageBuilder) buildLayoutGridRowV3(w *ast.WidgetV3) (*pages.LayoutGridRow, error) {
	row := &pages.LayoutGridRow{
		BaseElement: model.BaseElement{
			ID:       model.ID(types.GenerateID()),
			TypeName: "Forms$LayoutGridRow",
		},
	}

	// Build columns from children
	for _, child := range w.Children {
		if strings.ToUpper(child.Type) == "COLUMN" {
			col, err := pb.buildLayoutGridColumnV3(child)
			if err != nil {
				return nil, err
			}
			row.Columns = append(row.Columns, col)
		}
	}

	return row, nil
}

func (pb *pageBuilder) buildLayoutGridColumnV3(w *ast.WidgetV3) (*pages.LayoutGridColumn, error) {
	col := &pages.LayoutGridColumn{
		BaseElement: model.BaseElement{
			ID:       model.ID(types.GenerateID()),
			TypeName: "Forms$LayoutGridColumn",
		},
		Weight: 1,
	}

	// Handle DesktopWidth
	if dw := w.GetDesktopWidth(); dw != nil {
		switch v := dw.(type) {
		case int:
			col.Weight = v
		case string:
			if strings.ToUpper(v) == "AUTOFILL" {
				col.Weight = -1 // Auto
			}
		}
	}

	// Handle TabletWidth
	if tw := w.Properties["TabletWidth"]; tw != nil {
		switch v := tw.(type) {
		case int:
			col.TabletWeight = v
		case string:
			if strings.ToUpper(v) == "AUTOFILL" {
				col.TabletWeight = -1
			}
		}
	}

	// Handle PhoneWidth
	if pw := w.Properties["PhoneWidth"]; pw != nil {
		switch v := pw.(type) {
		case int:
			col.PhoneWeight = v
		case string:
			if strings.ToUpper(v) == "AUTOFILL" {
				col.PhoneWeight = -1
			}
		}
	}

	// Build child widgets
	for _, child := range w.Children {
		widget, err := pb.buildWidgetV3(child)
		if err != nil {
			return nil, err
		}
		col.Widgets = append(col.Widgets, widget)
	}

	return col, nil
}

// buildContainerWithRowV3 creates a Container holding a LayoutGrid with one row.
func (pb *pageBuilder) buildContainerWithRowV3(w *ast.WidgetV3) (*pages.Container, error) {
	container := &pages.Container{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$DivContainer",
			},
			Name: w.Name,
		},
	}

	lg := &pages.LayoutGrid{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$LayoutGrid",
			},
			Name: w.Name + "_grid",
		},
	}

	row, err := pb.buildLayoutGridRowV3(w)
	if err != nil {
		return nil, err
	}
	lg.Rows = append(lg.Rows, row)
	container.Widgets = append(container.Widgets, lg)

	return container, nil
}

// buildContainerWithColumnV3 creates a Container holding a LayoutGrid with one column.
func (pb *pageBuilder) buildContainerWithColumnV3(w *ast.WidgetV3) (*pages.Container, error) {
	container := &pages.Container{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$DivContainer",
			},
			Name: w.Name,
		},
	}

	lg := &pages.LayoutGrid{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$LayoutGrid",
			},
			Name: w.Name + "_grid",
		},
	}

	row := &pages.LayoutGridRow{
		BaseElement: model.BaseElement{
			ID:       model.ID(types.GenerateID()),
			TypeName: "Forms$LayoutGridRow",
		},
	}

	col, err := pb.buildLayoutGridColumnV3(w)
	if err != nil {
		return nil, err
	}
	row.Columns = append(row.Columns, col)
	lg.Rows = append(lg.Rows, row)
	container.Widgets = append(container.Widgets, lg)

	return container, nil
}

func (pb *pageBuilder) buildContainerV3(w *ast.WidgetV3) (*pages.Container, error) {
	container := &pages.Container{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$DivContainer",
			},
			Name: w.Name,
		},
	}

	// Handle RenderMode
	if rm := w.GetRenderMode(); rm != "" {
		container.RenderMode = pages.ContainerRenderMode(rm)
	}

	// Build child widgets
	for _, child := range w.Children {
		widget, err := pb.buildWidgetV3(child)
		if err != nil {
			return nil, err
		}
		container.Widgets = append(container.Widgets, widget)
	}

	return container, nil
}

func (pb *pageBuilder) buildTabContainerV3(w *ast.WidgetV3) (*pages.TabContainer, error) {
	tc := &pages.TabContainer{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$TabControl",
			},
			Name: w.Name,
		},
	}

	// Build tab pages from children
	for _, child := range w.Children {
		if strings.ToUpper(child.Type) == "TABPAGE" {
			tp, err := pb.buildTabPageV3(child)
			if err != nil {
				return nil, err
			}
			tc.TabPages = append(tc.TabPages, tp)
		}
	}

	if err := pb.registerWidgetName(w.Name, tc.ID); err != nil {
		return nil, err
	}

	return tc, nil
}

func (pb *pageBuilder) buildTabPageV3(w *ast.WidgetV3) (*pages.TabPage, error) {
	tp := &pages.TabPage{
		BaseElement: model.BaseElement{
			ID:       model.ID(types.GenerateID()),
			TypeName: "Forms$TabPage",
		},
		Name: w.Name,
	}

	// Handle Caption
	if caption := w.GetCaption(); caption != "" {
		tp.Caption = &model.Text{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Texts$Text",
			},
			Translations: map[string]string{"en_US": caption},
		}
	}

	// Build child widgets
	for _, child := range w.Children {
		widget, err := pb.buildWidgetV3(child)
		if err != nil {
			return nil, err
		}
		tp.Widgets = append(tp.Widgets, widget)
	}

	if err := pb.registerWidgetName(w.Name, tp.ID); err != nil {
		return nil, err
	}

	return tp, nil
}

func (pb *pageBuilder) buildGroupBoxV3(w *ast.WidgetV3) (*pages.GroupBox, error) {
	gb := &pages.GroupBox{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$GroupBox",
			},
			Name: w.Name,
		},
		Collapsible: "No",
		HeaderMode:  "Div",
	}

	// Handle Caption — uses ClientTemplate (same as DynamicText Content)
	if caption := w.GetCaption(); caption != "" {
		gb.Caption = &pages.ClientTemplate{
			Template: &model.Text{
				BaseElement: model.BaseElement{
					ID:       model.ID(types.GenerateID()),
					TypeName: "Texts$Text",
				},
				Translations: map[string]string{"en_US": caption},
			},
		}
	}

	// Handle Collapsible: Yes/YesExpanded/YesCollapsed/No
	if collapsible := w.GetStringProp("Collapsible"); collapsible != "" {
		switch strings.ToLower(collapsible) {
		case "yesexpanded", "yesinitiallyexpanded", "yes":
			gb.Collapsible = "YesInitiallyExpanded"
		case "yescollapsed", "yesinitiallycollapsed":
			gb.Collapsible = "YesInitiallyCollapsed"
		case "no":
			gb.Collapsible = "No"
		default:
			gb.Collapsible = collapsible
		}
	}

	// Handle HeaderMode: Div, H1-H6
	if headerMode := w.GetStringProp("HeaderMode"); headerMode != "" {
		gb.HeaderMode = headerMode
	}

	// Build child widgets
	for _, child := range w.Children {
		widget, err := pb.buildWidgetV3(child)
		if err != nil {
			return nil, err
		}
		gb.Widgets = append(gb.Widgets, widget)
	}

	if err := pb.registerWidgetName(w.Name, gb.ID); err != nil {
		return nil, err
	}

	return gb, nil
}

// buildFooterV3 creates a Footer container widget from V3 syntax.
func (pb *pageBuilder) buildFooterV3(w *ast.WidgetV3) (*pages.Container, error) {
	footer := &pages.Container{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$DivContainer",
			},
			Name: w.Name,
		},
	}

	// Build children
	for _, child := range w.Children {
		childWidget, err := pb.buildWidgetV3(child)
		if err != nil {
			return nil, err
		}
		footer.Widgets = append(footer.Widgets, childWidget)
	}

	if err := pb.registerWidgetName(w.Name, footer.ID); err != nil {
		return nil, err
	}

	return footer, nil
}

// buildHeaderV3 creates a Header container widget from V3 syntax.
func (pb *pageBuilder) buildHeaderV3(w *ast.WidgetV3) (*pages.Container, error) {
	header := &pages.Container{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$DivContainer",
			},
			Name: w.Name,
		},
	}

	// Build children
	for _, child := range w.Children {
		childWidget, err := pb.buildWidgetV3(child)
		if err != nil {
			return nil, err
		}
		header.Widgets = append(header.Widgets, childWidget)
	}

	if err := pb.registerWidgetName(w.Name, header.ID); err != nil {
		return nil, err
	}

	return header, nil
}

// buildControlBarV3 creates a ControlBar container widget from V3 syntax.
func (pb *pageBuilder) buildControlBarV3(w *ast.WidgetV3) (*pages.Container, error) {
	controlBar := &pages.Container{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$DivContainer",
			},
			Name: w.Name,
		},
	}

	// Build children
	for _, child := range w.Children {
		childWidget, err := pb.buildWidgetV3(child)
		if err != nil {
			return nil, err
		}
		controlBar.Widgets = append(controlBar.Widgets, childWidget)
	}

	if err := pb.registerWidgetName(w.Name, controlBar.ID); err != nil {
		return nil, err
	}

	return controlBar, nil
}
