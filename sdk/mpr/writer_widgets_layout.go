// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"

	"go.mongodb.org/mongo-driver/bson"
)

// serializeContainer serializes a Container widget.
func serializeContainer(c *pages.Container) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(c.ID))},
		{Key: "$Type", Value: "Forms$DivContainer"},
		{Key: "Appearance", Value: serializeAppearance(c.Class, c.Style, c.DynamicClasses, c.DesignProperties)},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "Name", Value: c.Name},
		{Key: "NativeAccessibilitySettings", Value: nil},
		{Key: "OnClickAction", Value: serializeClientAction(c.OnClickAction)},
		{Key: "RenderMode", Value: "Div"},
		{Key: "ScreenReaderHidden", Value: false},
		{Key: "TabIndex", Value: int64(0)},
		{Key: "Widgets", Value: serializeWidgetArray(c.Widgets)},
	}
	return doc
}

// serializeGroupBox serializes a GroupBox widget.
func serializeGroupBox(gb *pages.GroupBox) bson.D {
	collapsible := gb.Collapsible
	if collapsible == "" {
		collapsible = "No"
	}
	headerMode := gb.HeaderMode
	if headerMode == "" {
		headerMode = "Div"
	}

	// Serialize CaptionTemplate
	var captionTemplate bson.D
	if gb.Caption != nil {
		captionTemplate = serializeClientTemplate(gb.Caption, nil, "")
	} else {
		captionTemplate = serializeClientTemplate(nil, nil, "")
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(gb.ID))},
		{Key: "$Type", Value: "Forms$GroupBox"},
		{Key: "Appearance", Value: serializeAppearance(gb.Class, gb.Style, gb.DynamicClasses, gb.DesignProperties)},
		{Key: "CaptionTemplate", Value: captionTemplate},
		{Key: "Collapsible", Value: collapsible},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "HeaderMode", Value: headerMode},
		{Key: "Name", Value: gb.Name},
		{Key: "TabIndex", Value: int64(0)},
		{Key: "Widgets", Value: serializeWidgetArray(gb.Widgets)},
	}
	return doc
}

// serializeTabContainer serializes a TabContainer widget.
func serializeTabContainer(tc *pages.TabContainer) bson.D {
	tabPages := bson.A{int32(3)} // marker=3 for TabPages array
	var defaultPageID []byte
	for i, tp := range tc.TabPages {
		tpDoc := serializeTabPage(tp)
		tabPages = append(tabPages, tpDoc)
		if i == 0 {
			// Default to first tab
			defaultPageID = idToBsonBinary(string(tp.ID)).Data
		}
	}
	if tc.DefaultPageID != "" {
		defaultPageID = idToBsonBinary(string(tc.DefaultPageID)).Data
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(tc.ID))},
		{Key: "$Type", Value: "Forms$TabControl"},
		{Key: "ActivePageAttributeRef", Value: nil},
		{Key: "ActivePageOnChangeAction", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(GenerateID())},
			{Key: "$Type", Value: "Forms$NoAction"},
			{Key: "DisabledDuringExecution", Value: true},
		}},
		{Key: "ActivePageSourceVariable", Value: nil},
		{Key: "Appearance", Value: serializeAppearance(tc.Class, tc.Style, tc.DynamicClasses, tc.DesignProperties)},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "DefaultPagePointer", Value: defaultPageID},
		{Key: "Name", Value: tc.Name},
		{Key: "TabIndex", Value: int64(0)},
		{Key: "TabPages", Value: tabPages},
	}
	return doc
}

// serializeTabPage serializes a TabPage within a TabContainer.
func serializeTabPage(tp *pages.TabPage) bson.D {
	// Caption
	var caption bson.D
	if tp.Caption != nil {
		caption = serializeText(tp.Caption)
	} else {
		caption = serializeText(&model.Text{
			BaseElement: model.BaseElement{
				ID:       model.ID(GenerateID()),
				TypeName: "Texts$Text",
			},
			Translations: map[string]string{"en_US": tp.Name},
		})
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(tp.ID))},
		{Key: "$Type", Value: "Forms$TabPage"},
		{Key: "Badge", Value: nil},
		{Key: "Caption", Value: caption},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "Name", Value: tp.Name},
		{Key: "RefreshOnShow", Value: tp.RefreshOnShow},
		{Key: "Widgets", Value: serializeWidgetArray(tp.Widgets)},
	}
	return doc
}

// serializeLayoutGrid serializes a LayoutGrid widget.
func serializeLayoutGrid(lg *pages.LayoutGrid) bson.D {
	// Mendix uses [3] for empty arrays, [2, item1, item2, ...] for non-empty arrays
	// Items go directly after the version marker, NOT nested in another array
	rows := bson.A{int32(3)} // Start with empty marker
	hasRows := false
	for _, row := range lg.Rows {
		if !hasRows {
			rows = bson.A{int32(2)} // First item: change to version 2
			hasRows = true
		}
		rows = append(rows, serializeLayoutGridRow(row))
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(lg.ID))},
		{Key: "$Type", Value: "Forms$LayoutGrid"},
		{Key: "Appearance", Value: serializeAppearance(lg.Class, lg.Style, lg.DynamicClasses, lg.DesignProperties)},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "Name", Value: lg.Name},
		{Key: "Rows", Value: rows},
		{Key: "TabIndex", Value: int64(0)},
		{Key: "Width", Value: "FullWidth"},
	}
	return doc
}

// serializeLayoutGridRow serializes a LayoutGridRow.
func serializeLayoutGridRow(row *pages.LayoutGridRow) bson.D {
	// Mendix uses [3] for empty arrays, [2, item1, item2, ...] for non-empty arrays
	// Items go directly after the version marker, NOT nested in another array
	cols := bson.A{int32(3)} // Start with empty marker
	hasCols := false
	for _, col := range row.Columns {
		if !hasCols {
			cols = bson.A{int32(2)} // First item: change to version 2
			hasCols = true
		}
		cols = append(cols, serializeLayoutGridColumn(col))
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(row.ID))},
		{Key: "$Type", Value: "Forms$LayoutGridRow"},
		{Key: "Appearance", Value: serializeAppearance("", "", "", nil)},
		{Key: "Columns", Value: cols},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "HorizontalAlignment", Value: "None"},
		{Key: "SpacingBetweenColumns", Value: true},
		{Key: "VerticalAlignment", Value: "None"},
	}
}

// columnWeight returns the column weight, defaulting to -1 (auto) if 0.
func columnWeight(w int) int {
	if w == 0 {
		return -1
	}
	return w
}

// serializeLayoutGridColumn serializes a LayoutGridColumn.
func serializeLayoutGridColumn(col *pages.LayoutGridColumn) bson.D {
	// Weight for column width: -1 means auto-fill, 1-12 are explicit widths
	weight := col.Weight
	if weight == 0 {
		weight = -1 // Default to auto-fill
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(col.ID))},
		{Key: "$Type", Value: "Forms$LayoutGridColumn"},
		{Key: "Appearance", Value: serializeAppearance("", "", "", nil)},
		{Key: "PhoneWeight", Value: int64(columnWeight(col.PhoneWeight))},
		{Key: "PreviewWidth", Value: int64(-1)}, // Default preview width
		{Key: "TabletWeight", Value: int64(columnWeight(col.TabletWeight))},
		{Key: "VerticalAlignment", Value: "None"},
		{Key: "Weight", Value: int64(weight)}, // Desktop weight
		{Key: "Widgets", Value: serializeWidgetArray(col.Widgets)},
	}
}
