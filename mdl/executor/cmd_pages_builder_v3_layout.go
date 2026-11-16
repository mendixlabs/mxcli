// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
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
	// Width: FullWidth | FixedWidth (ako/mxcli-ledger FINDINGS #170).
	width, err := layoutGridWidth(w)
	if err != nil {
		return nil, mdlerrors.NewValidation(err.Error())
	}
	lg.Width = width

	// Build rows from children
	for _, child := range w.Children {
		if strings.ToLower(child.Type) == "row" {
			// Built here, not by buildWidgetV3 (ako/mxcli#1698).
			if err := refuseNativeWidgetProps(pb.ctx, "layoutgrid row", child); err != nil {
				return nil, err
			}
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
		Class:          w.GetClass(),
		Style:          w.GetStyle(),
		DynamicClasses: w.GetDynamicClasses(),
	}
	var err error
	if row.DesignProperties, err = designPropertyValuesV3(w, "LayoutGridRow", pb.themeRegistry); err != nil {
		return nil, fmt.Errorf("layout grid row: %w", err)
	}
	if row.VerticalAlignment, err = layoutGridAlignment(w, "VerticalAlignment"); err != nil {
		return nil, err
	}
	if row.HorizontalAlignment, err = layoutGridAlignment(w, "HorizontalAlignment"); err != nil {
		return nil, err
	}
	if raw, ok := lookupPropCI(w, "SpacingBetweenColumns"); ok {
		v, err := propBool(raw)
		if err != nil {
			return nil, fmt.Errorf("layout grid row SpacingBetweenColumns: %w", err)
		}
		row.NoSpacingBetweenColumns = !v
	}
	// A row's own visibility (Forms$LayoutGridRow.ConditionalVisibilitySettings).
	// Like a tab page, a row is built here and not by buildWidgetV3, whose tail
	// applies Visible: to every widget, so it was parsed and dropped
	// (mendixlabs/mxcli#1461).
	if row.ConditionalVisibility, err = pb.childVisibility(w); err != nil {
		return nil, err
	}

	// Build columns from children
	for _, child := range w.Children {
		if strings.ToLower(child.Type) == "column" {
			// Built here, not by buildWidgetV3 (ako/mxcli#1698).
			if err := refuseNativeWidgetProps(pb.ctx, "layoutgrid column", child); err != nil {
				return nil, err
			}
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
		Weight:         1,
		Class:          w.GetClass(),
		Style:          w.GetStyle(),
		DynamicClasses: w.GetDynamicClasses(),
	}
	var err error
	if col.DesignProperties, err = designPropertyValuesV3(w, "LayoutGridColumn", pb.themeRegistry); err != nil {
		return nil, fmt.Errorf("layout grid column: %w", err)
	}
	if col.VerticalAlignment, err = layoutGridAlignment(w, "VerticalAlignment"); err != nil {
		return nil, err
	}

	if dw := w.GetDesktopWidth(); dw != nil {
		col.Weight = layoutGridWeight(dw, col.Weight)
	}
	if tw := w.Properties["TabletWidth"]; tw != nil {
		col.TabletWeight = layoutGridWeight(tw, col.TabletWeight)
	}
	if pw := w.Properties["PhoneWidth"]; pw != nil {
		col.PhoneWeight = layoutGridWeight(pw, col.PhoneWeight)
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

// layoutGridAlignment reads a row's or column's VerticalAlignment /
// HorizontalAlignment: Start, Center or End (any case), "" when unset — which
// the writer stores as Mendix's default, None.
func layoutGridAlignment(w *ast.WidgetV3, key string) (string, error) {
	raw, ok := lookupPropCI(w, key)
	if !ok {
		return "", nil
	}
	if s, ok := raw.(string); ok {
		for _, a := range []string{"None", "Start", "Center", "End"} {
			if strings.EqualFold(s, a) {
				return a, nil
			}
		}
	}
	return "", mdlerrors.NewValidationf("layout grid %s %s: invalid value %v (expected Start, Center or End)",
		strings.ToLower(w.Type), key, raw)
}

// Stored layout-grid column weights besides 1..12.
const (
	layoutGridWeightAutoFill = -1
	layoutGridWeightAutoFit  = -2 // "Auto-fit content" in Studio Pro
)

// layoutGridWeight maps a DesktopWidth / TabletWidth / PhoneWidth value to the
// stored weight: a number as-is, AutoFill to -1, AutoFit to -2. Any other value
// keeps current.
func layoutGridWeight(v any, current int) int {
	switch v := v.(type) {
	case int:
		return v
	case string:
		switch {
		case strings.EqualFold(v, "autofill"):
			return layoutGridWeightAutoFill
		case strings.EqualFold(v, "autofit"):
			return layoutGridWeightAutoFit
		}
	}
	return current
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
	// The appearance written on a top-level `row` is the container's
	// (applyWidgetAppearance sets it there); writing it on the row too would
	// apply every class and design property twice.
	// Its Visible: likewise (buildWidgetV3's tail sets it on the container).
	row.Class, row.Style, row.DynamicClasses, row.DesignProperties = "", "", "", nil
	row.ConditionalVisibility = nil
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
	// The container carries a top-level `column`'s appearance, as for `row`.
	col.Class, col.Style, col.DynamicClasses, col.DesignProperties = "", "", "", nil
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

	// Handle RenderMode. The codec stores it verbatim, so an unknown member is
	// refused here rather than written into a document Studio Pro cannot open.
	if rm := w.GetRenderMode(); rm != "" {
		mode, ok := pages.CanonicalContainerRenderMode(rm)
		if !ok {
			names := make([]string, len(pages.ContainerRenderModes))
			for i, m := range pages.ContainerRenderModes {
				names[i] = string(m)
			}
			return nil, mdlerrors.NewValidationf("container %q: unknown RenderMode %q — valid modes are %s",
				w.Name, rm, strings.Join(names, ", "))
		}
		container.RenderMode = mode
	}

	// Handle Action / OnClick property — a Container is clickable in Mendix via
	// its OnClickAction (Forms$DivContainer.onClickAction). See issue #603.
	if action := w.GetAction(); action != nil {
		clientAction, err := pb.buildClientActionV3(action)
		if err != nil {
			return nil, err
		}
		container.OnClickAction = clientAction
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
		if strings.ToLower(child.Type) == "tabpage" {
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
	// A key the vocabulary accepts and Forms$TabPage cannot hold (#1465).
	if err := refuseTabPageDroppedProps(pb.ctx, w); err != nil {
		return nil, err
	}
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
			Translations: map[string]string{pb.textLang(): caption},
		}
	}

	// Visibility: a tab page bypasses buildWidgetV3, whose common tail applies
	// it to every widget, so it is applied here — it used to be parsed and
	// dropped, the tab always visible (mendixlabs/mxcli#1465).
	cvs, err := pb.childVisibility(w)
	if err != nil {
		return nil, err
	}
	tp.ConditionalVisibility = cvs

	// Its badge (mendixlabs/mxcli#1644, tabpage_badge.go).
	if tp.Badge, err = pb.buildTabPageBadge(w); err != nil {
		return nil, err
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
				Translations: map[string]string{pb.textLang(): caption},
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

// childVisibility is the ConditionalVisibilitySettings for `Visible:` on an
// element its parent builds outside buildWidgetV3 — a tab page (#1465) or a
// layout-grid row (#1461) — in either form: an expression, or `Attr in (…)` —
// plus any `Visible: roles (…)` (#1460). nil when there is none.
func (pb *pageBuilder) childVisibility(w *ast.WidgetV3) (*pages.ConditionalVisibilitySettings, error) {
	if err := refuseRolesInVisibility(w); err != nil {
		return nil, err
	}
	if vw, ok := w.Properties["VisibleWhen"].(*ast.VisibleWhenV3); ok && vw != nil {
		cvs, err := pb.visibleWhenSettings(w, vw)
		if err != nil {
			return nil, err
		}
		return withVisibleRoles(cvs, w), nil
	}
	return withVisibleRoles(expressionVisibility(w), w), nil
}
