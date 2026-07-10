// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"

	"go.mongodb.org/mongo-driver/bson"
)

// serializeSnippetCall serializes a SnippetCallWidget.
func serializeSnippetCall(s *pages.SnippetCallWidget) bson.D {
	// Build parameter mappings array.
	// Format: [count, mapping1, mapping2, ...] where count is the Mendix array version marker.
	// Type is Forms$SnippetParameterMapping (not Forms$PageParameterMapping).
	// The variable reference goes in Variable.PageParameter; Argument is always empty.
	paramMappings := bson.A{int32(len(s.ParameterMappings))}
	for _, pm := range s.ParameterMappings {
		// Parameter is BY_NAME_REFERENCE: SnippetQualifiedName.ParameterName
		paramRef := s.SnippetName + "." + pm.ParamName
		// Strip leading $ from the variable name for PageParameter sub-field
		varName := strings.TrimPrefix(pm.Argument, "$")
		paramMappings = append(paramMappings, bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Forms$SnippetParameterMapping"},
			{Key: "Argument", Value: ""},
			{Key: "Parameter", Value: paramRef},
			{Key: "Variable", Value: buildFormPageVariable(varName)},
		})
	}

	// Build the inner SnippetCall object
	snippetCallID := generateUUID()
	snippetCall := bson.D{
		{Key: "$ID", Value: idToBsonBinary(snippetCallID)},
		{Key: "$Type", Value: "Forms$SnippetCall"},
		{Key: "ParameterMappings", Value: paramMappings},
	}

	// Add snippet reference - prefer qualified name (BY_NAME_REFERENCE) over binary ID
	if s.SnippetName != "" {
		snippetCall = append(snippetCall, bson.E{Key: "Form", Value: s.SnippetName})
	} else if s.SnippetID != "" {
		snippetCall = append(snippetCall, bson.E{Key: "Form", Value: idToBsonBinary(string(s.SnippetID))})
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(s.ID))},
		{Key: "$Type", Value: "Forms$SnippetCallWidget"},
		{Key: "Appearance", Value: serializeAppearance(s.Class, s.Style, s.DynamicClasses, s.DesignProperties)},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "FormCall", Value: snippetCall},
		{Key: "Name", Value: s.Name},
		{Key: "TabIndex", Value: int64(0)},
	}

	return doc
}

// serializeGallery serializes a Gallery widget as Forms$ListView.
// Note: Forms$Gallery is not available in all Mendix versions, so we use ListView as a fallback.
// ListView provides similar grid-based item display functionality.
func serializeGallery(g *pages.Gallery) bson.D {
	// Default values
	pageSize := g.PageSize
	if pageSize == 0 {
		pageSize = 20
	}
	numberOfColumns := g.DesktopItems
	if numberOfColumns == 0 {
		numberOfColumns = 4
	}

	// Serialize datasource - Gallery (as ListView) requires a non-null DataSource
	var dataSource any
	if g.DataSource != nil {
		dataSource = serializeListViewDataSource(g.DataSource)
	}
	// Fallback: provide empty ListViewXPathSource to prevent Studio Pro crash
	if dataSource == nil {
		dataSource = bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Forms$ListViewXPathSource"},
			{Key: "EntityRef", Value: nil},
			{Key: "Search", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Forms$ListViewSearch"},
				{Key: "Paths", Value: bson.A{int32(3)}},
			}},
			{Key: "Sort", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Forms$ListViewSort"},
				{Key: "Paths", Value: bson.A{int32(3)}},
			}},
			{Key: "XPathConstraint", Value: ""},
		}
	}

	// Build content widgets
	contentWidgets := bson.A{int32(3)}
	if g.ContentWidget != nil {
		contentWidgets = append(contentWidgets, serializeWidget(g.ContentWidget))
	}

	// Templates array (empty for basic ListView)
	templates := bson.A{int32(3)}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(g.ID))},
		{Key: "$Type", Value: "Forms$ListView"},
		{Key: "Appearance", Value: serializeAppearance(g.Class, g.Style, g.DynamicClasses, g.DesignProperties)},
		{Key: "ClickAction", Value: serializeClientAction(nil)},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "DataSource", Value: dataSource},
		{Key: "Editable", Value: false},
		{Key: "Name", Value: g.Name},
		{Key: "NumberOfColumns", Value: int64(numberOfColumns)},
		{Key: "PageSize", Value: int64(pageSize)},
		{Key: "PullDownAction", Value: serializeClientAction(nil)},
		{Key: "ScrollDirection", Value: "Vertical"},
		{Key: "TabIndex", Value: int64(0)},
		{Key: "Templates", Value: templates},
		{Key: "Widgets", Value: contentWidgets},
	}

	return doc
}

// serializeListView serializes a ListView widget.
func serializeListView(lv *pages.ListView) bson.D {
	// Default values
	pageSize := lv.PageSize
	if pageSize == 0 {
		pageSize = 20
	}

	// Serialize datasource - ListView requires a non-null DataSource (EntityWidget)
	var dataSource any
	if lv.DataSource != nil {
		dataSource = serializeListViewDataSource(lv.DataSource)
	}
	// Fallback: provide empty ListViewXPathSource to prevent Studio Pro crash
	if dataSource == nil {
		dataSource = bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Forms$ListViewXPathSource"},
			{Key: "EntityRef", Value: nil},
			{Key: "Search", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Forms$ListViewSearch"},
				{Key: "Paths", Value: bson.A{int32(3)}},
			}},
			{Key: "Sort", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Forms$ListViewSort"},
				{Key: "Paths", Value: bson.A{int32(3)}},
			}},
			{Key: "XPathConstraint", Value: ""},
		}
	}

	// Build content widgets
	contentWidgets := serializeWidgetArray(lv.Widgets)

	// Templates array
	templates := bson.A{int32(3)}
	if len(lv.Templates) > 0 {
		templates = bson.A{int32(2)}
		for _, t := range lv.Templates {
			templateWidgets := bson.A{int32(3)}
			if len(t.Widgets) > 0 {
				templateWidgets = bson.A{int32(2)}
				for _, w := range t.Widgets {
					templateWidgets = append(templateWidgets, serializeWidget(w))
				}
			}
			template := bson.D{
				{Key: "$ID", Value: idToBsonBinary(string(t.ID))},
				{Key: "$Type", Value: "Forms$ListViewTemplate"},
				{Key: "Widgets", Value: templateWidgets},
			}
			templates = append(templates, template)
		}
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(lv.ID))},
		{Key: "$Type", Value: "Forms$ListView"},
		{Key: "Appearance", Value: serializeAppearance(lv.Class, lv.Style, lv.DynamicClasses, lv.DesignProperties)},
		{Key: "ClickAction", Value: serializeClientAction(lv.ClickAction)},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "DataSource", Value: dataSource},
		{Key: "Editable", Value: lv.Editable},
		{Key: "Name", Value: lv.Name},
		{Key: "NumberOfColumns", Value: int64(1)},
		{Key: "PageSize", Value: int64(pageSize)},
		{Key: "PullDownAction", Value: serializeClientAction(nil)},
		{Key: "ScrollDirection", Value: "Vertical"},
		{Key: "TabIndex", Value: int64(0)},
		{Key: "Templates", Value: templates},
		{Key: "Widgets", Value: contentWidgets},
	}

	return doc
}

// serializeListViewDataSource serializes a datasource for ListView widgets.
// Supports DatabaseSource (XPath), MicroflowSource, NanoflowSource, and AssociationSource.
func serializeListViewDataSource(ds pages.DataSource) bson.D {
	if ds == nil {
		return nil
	}

	switch d := ds.(type) {
	case *pages.DatabaseSource:
		// EntityRef for database source - use EntityName (qualified name) not EntityID
		var entityRef any
		if d.EntityName != "" {
			entityRef = bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "DomainModels$DirectEntityRef"},
				{Key: "Entity", Value: d.EntityName},
			}
		}
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(d.ID))},
			{Key: "$Type", Value: "Forms$ListViewXPathSource"},
			{Key: "EntityRef", Value: entityRef},
			{Key: "Search", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Forms$ListViewSearch"},
				{Key: "Paths", Value: bson.A{int32(3)}},
			}},
			{Key: "Sort", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Forms$ListViewSort"},
				{Key: "Paths", Value: bson.A{int32(3)}},
			}},
			{Key: "XPathConstraint", Value: d.XPathConstraint},
		}
	case *pages.MicroflowSource:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(d.ID))},
			{Key: "$Type", Value: "Forms$MicroflowSource"},
			{Key: "MicroflowSettings", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Forms$MicroflowSettings"},
				{Key: "Asynchronous", Value: false},
				{Key: "ConfirmationInfo", Value: nil},
				{Key: "FormValidations", Value: "All"},
				{Key: "Microflow", Value: d.Microflow},
				{Key: "ParameterMappings", Value: bson.A{int32(3)}},
				{Key: "ProgressBar", Value: "None"},
				{Key: "ProgressMessage", Value: nil},
			}},
		}
	case *pages.NanoflowSource:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(d.ID))},
			{Key: "$Type", Value: "Forms$NanoflowSource"},
			{Key: "NanoflowSettings", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Forms$NanoflowSettings"},
				{Key: "Nanoflow", Value: d.Nanoflow},
				{Key: "ParameterMappings", Value: bson.A{int32(3)}},
			}},
		}
	case *pages.AssociationSource:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(d.ID))},
			{Key: "$Type", Value: "Forms$AssociationSource"},
			{Key: "EntityRef", Value: nil},
		}
	default:
		return nil
	}
}

// serializeDynamicText serializes a DynamicText widget.
func serializeDynamicText(dt *pages.DynamicText) bson.D {
	renderMode := string(dt.RenderMode)
	if renderMode == "" {
		renderMode = "Text"
	}

	// Create fallback text from AttributePath for backward compatibility
	var fallbackText *model.Text
	if dt.AttributePath != "" && dt.Content == nil {
		fallbackText = &model.Text{
			Translations: map[string]string{"en_US": dt.AttributePath},
		}
	}

	// Build content as ClientTemplate
	content := serializeClientTemplate(dt.Content, fallbackText, "Dynamic Text")

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(dt.ID))},
		{Key: "$Type", Value: "Forms$DynamicText"},
		{Key: "Appearance", Value: serializeAppearance(dt.Class, dt.Style, dt.DynamicClasses, dt.DesignProperties)},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "Content", Value: content},
		{Key: "Name", Value: dt.Name},
		{Key: "NativeAccessibilitySettings", Value: nil},
		{Key: "NativeTextStyle", Value: "Text"},
		{Key: "RenderMode", Value: renderMode},
		{Key: "TabIndex", Value: int64(0)},
	}
	return doc
}

// serializeActionButton serializes an ActionButton widget.
func serializeActionButton(ab *pages.ActionButton) bson.D {
	buttonStyle := string(ab.ButtonStyle)
	if buttonStyle == "" {
		buttonStyle = "Default"
	}

	// RenderType distinguishes a normal action button ("Button") from a
	// link-rendered one ("Link", authored as `linkbutton`).
	renderType := string(ab.RenderMode)
	if renderType == "" {
		renderType = "Button"
	}

	// Build caption as ClientTemplate
	caption := serializeClientTemplate(ab.CaptionTemplate, ab.Caption, "Button")

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(ab.ID))},
		{Key: "$Type", Value: "Forms$ActionButton"},
		{Key: "Action", Value: serializeClientAction(ab.Action)},
		{Key: "Appearance", Value: serializeAppearance(ab.Class, ab.Style, ab.DynamicClasses, ab.DesignProperties)},
		{Key: "AriaRole", Value: "Button"},
		{Key: "ButtonStyle", Value: buttonStyle},
		{Key: "CaptionTemplate", Value: caption}, // Must be CaptionTemplate, not Caption
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "Icon", Value: nil},
		{Key: "Name", Value: ab.Name},
		{Key: "NativeAccessibilitySettings", Value: nil},
		{Key: "RenderType", Value: renderType},
		{Key: "TabIndex", Value: int64(0)},
		{Key: "Tooltip", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3)}},
		}},
	}
	return doc
}

// serializeStaticText serializes a static Text widget.
func serializeStaticText(t *pages.Text) bson.D {
	textValue := "Text"
	if t.Caption != nil {
		for _, text := range t.Caption.Translations {
			textValue = text
			break
		}
	}

	renderMode := string(t.RenderMode)
	if renderMode == "" {
		renderMode = "Text"
	}

	// Mendix uses [3] as version marker, followed by array items
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(t.ID))},
		{Key: "$Type", Value: "Forms$Text"},
		{Key: "Appearance", Value: serializeAppearance(t.Class, t.Style, t.DynamicClasses, t.DesignProperties)},
		{Key: "Caption", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3), bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Texts$Translation"},
				{Key: "LanguageCode", Value: "en_US"},
				{Key: "Text", Value: textValue},
			}}},
		}},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "Name", Value: t.Name},
		{Key: "NativeAccessibilitySettings", Value: nil},
		{Key: "NativeTextStyle", Value: "Text"},
		{Key: "RenderMode", Value: renderMode},
		{Key: "TabIndex", Value: int64(0)},
	}
	return doc
}

// serializeTitle serializes a Title widget.
func serializeTitle(t *pages.Title) bson.D {
	textValue := "Title"
	if t.Caption != nil {
		for _, text := range t.Caption.Translations {
			textValue = text
			break
		}
	}

	// Mendix uses [3] as version marker, followed by array items
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(t.ID))},
		{Key: "$Type", Value: "Forms$Title"},
		{Key: "Appearance", Value: serializeAppearance(t.Class, t.Style, t.DynamicClasses, t.DesignProperties)},
		{Key: "Caption", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3), bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Texts$Translation"},
				{Key: "LanguageCode", Value: "en_US"},
				{Key: "Text", Value: textValue},
			}}},
		}},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "Name", Value: t.Name},
		{Key: "NativeAccessibilitySettings", Value: nil},
		{Key: "TabIndex", Value: int64(0)},
	}
	return doc
}

// dataViewLabelWidth resolves the LabelWidth to write to BSON. LabelWidth wins
// if explicitly set; otherwise FormOrientation is the source (Vertical -> 0,
// Horizontal/unset -> Mendix's metamodel default of 3).
func dataViewLabelWidth(dv *pages.DataView) int64 {
	if dv.LabelWidth != nil {
		return int64(*dv.LabelWidth)
	}
	if dv.FormOrientation == pages.FormOrientationVertical {
		return 0
	}
	return 3
}

// serializeDataView serializes a DataView widget with all required properties.
func serializeDataView(dv *pages.DataView) bson.D {
	// Build NoEntityMessage as Texts$Text
	noEntityMessage := bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Texts$Text"},
		{Key: "Items", Value: bson.A{int32(3)}},
	}

	// Build data source - DataView requires a non-null DataSource (EntityWidget)
	var dataSource any
	if dv.DataSource != nil {
		dataSource = serializeDataViewDataSource(dv.DataSource)
	}
	// Fallback: provide empty DataViewSource to prevent Studio Pro crash
	if dataSource == nil {
		dataSource = bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Forms$DataViewSource"},
			{Key: "EntityRef", Value: nil},
			{Key: "ForceFullObjects", Value: false},
			{Key: "SourceVariable", Value: nil},
		}
	}

	// Build widgets
	widgets := serializeWidgetArray(dv.Widgets)

	// Build footer widgets
	footerWidgets := serializeWidgetArray(dv.FooterWidgets)

	// Determine editability
	editability := "Always"
	if dv.ReadOnly {
		editability = "Never"
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(dv.ID))},
		{Key: "$Type", Value: "Forms$DataView"},
		{Key: "Appearance", Value: serializeAppearance(dv.Class, dv.Style, dv.DynamicClasses, dv.DesignProperties)},
		{Key: "ConditionalEditabilitySettings", Value: nil},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "DataSource", Value: dataSource},
		{Key: "Editability", Value: editability},
		{Key: "FooterWidgets", Value: footerWidgets},
		{Key: "LabelWidth", Value: dataViewLabelWidth(dv)},
		{Key: "Name", Value: dv.Name},
		{Key: "NoEntityMessage", Value: noEntityMessage},
		{Key: "ReadOnlyStyle", Value: "Control"},
		{Key: "ShowFooter", Value: dv.ShowFooter},
		{Key: "TabIndex", Value: int64(0)},
		{Key: "Widgets", Value: widgets},
	}

	return doc
}

// serializeDataViewDataSource serializes a data source for DataView widgets.
// DataView requires Forms$DataViewSource with EntityRef and SourceVariable for parameter references.
func serializeDataViewDataSource(ds pages.DataSource) any {
	if ds == nil {
		return nil
	}

	switch d := ds.(type) {
	case *pages.DataViewSource:
		// DataView using page parameter - needs Forms$DataViewSource with EntityRef and SourceVariable
		var entityRef any
		if d.EntityName != "" {
			entityRef = bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "DomainModels$DirectEntityRef"},
				{Key: "Entity", Value: d.EntityName},
			}
		}

		// Build SourceVariable as Forms$PageVariable
		var sourceVariable any
		if d.ParameterName != "" {
			// Determine if this is a snippet parameter or page parameter
			pageParam := d.ParameterName
			snippetParam := ""
			if d.IsSnippetParameter {
				pageParam = ""
				snippetParam = d.ParameterName
			}
			sourceVariable = bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Forms$PageVariable"},
				{Key: "LocalVariable", Value: ""},
				{Key: "PageParameter", Value: pageParam},
				{Key: "SnippetParameter", Value: snippetParam},
				{Key: "SubKey", Value: ""},
				{Key: "UseAllPages", Value: false},
				{Key: "Widget", Value: ""},
			}
		}

		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(d.ID))},
			{Key: "$Type", Value: "Forms$DataViewSource"},
			{Key: "EntityRef", Value: entityRef},
			{Key: "ForceFullObjects", Value: false},
			{Key: "SourceVariable", Value: sourceVariable},
		}
	case *pages.DatabaseSource:
		// For database source in DataView, use standard serialization
		return serializeDataSource(d)
	case *pages.MicroflowSource:
		return serializeDataSource(d)
	case *pages.NanoflowSource:
		return serializeDataSource(d)
	case *pages.ListenToWidgetSource:
		// ListenTargetSource - listens to another widget's selection
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(d.ID))},
			{Key: "$Type", Value: "Forms$ListenTargetSource"},
			{Key: "ForceFullObjects", Value: false},
			{Key: "ListenTarget", Value: d.WidgetName},
		}
	case *pages.AssociationSource:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(d.ID))},
			{Key: "$Type", Value: "Forms$AssociationSource"},
			{Key: "EntityRef", Value: nil},
		}
	default:
		// Fallback to generic datasource serialization
		return nil
	}
}

// serializeDataGrid serializes a DataGrid widget with columns.
func serializeDataGrid(dg *pages.DataGrid) bson.D {
	// Build data source - DataGrid requires a non-null DataSource (EntityWidget)
	var dataSource any
	if dg.DataSource != nil {
		dataSource = serializeDataGridDataSource(dg.DataSource)
	}
	// Fallback: provide empty NewGridDatabaseSource to prevent Studio Pro crash
	if dataSource == nil {
		dataSource = bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Forms$NewGridDatabaseSource"},
			{Key: "EntityRef", Value: nil},
			{Key: "SortBar", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Forms$GridSortBar"},
				{Key: "SortItems", Value: bson.A{int32(3)}},
			}},
			{Key: "XPathConstraint", Value: ""},
		}
	}

	// Build columns
	columns := bson.A{int32(3)} // Start with empty marker
	if len(dg.Columns) > 0 {
		columns = bson.A{int32(2)}
		for _, col := range dg.Columns {
			columns = append(columns, serializeDataGridColumn(col))
		}
	}

	// Build control bar widgets
	controlBarWidgets := serializeWidgetArray(dg.ControlBarWidgets)

	// Selection mode
	selectionMode := "Single"
	switch dg.SelectionMode {
	case pages.SelectionModeMulti:
		selectionMode = "Multi"
	case pages.SelectionModeNone:
		selectionMode = "No"
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(dg.ID))},
		{Key: "$Type", Value: "Forms$DataGrid"},
		{Key: "Appearance", Value: serializeAppearance(dg.Class, dg.Style, dg.DynamicClasses, dg.DesignProperties)},
		{Key: "ClickAction", Value: serializeClientAction(nil)},
		{Key: "Columns", Value: columns},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "ControlBar", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Forms$ControlBar"},
			{Key: "DefaultButton", Value: nil},
			{Key: "Widgets", Value: controlBarWidgets},
		}},
		{Key: "DataSource", Value: dataSource},
		{Key: "IsControlBarVisible", Value: len(dg.ControlBarWidgets) > 0},
		{Key: "Name", Value: dg.Name},
		{Key: "NumberOfRows", Value: int64(20)},
		{Key: "RefreshTime", Value: int64(0)},
		{Key: "SelectFirst", Value: dg.SelectFirst},
		{Key: "SelectionMode", Value: selectionMode},
		{Key: "ShowEmptyRows", Value: dg.ShowEmptyRows},
		{Key: "ShowPagingBar", Value: "YesWithTotalCount"},
		{Key: "TabIndex", Value: int64(0)},
		{Key: "TooltipForm", Value: nil},
		{Key: "WidthUnit", Value: "Percentage"},
	}

	return doc
}

// serializeDataGridColumn serializes a DataGridColumn.
func serializeDataGridColumn(col *pages.DataGridColumn) bson.D {
	// Build caption text
	var caption any
	if col.Caption != nil {
		caption = serializeText(col.Caption)
	} else {
		caption = serializeEmptyText()
	}

	// Build attribute reference
	var attrRef any
	if col.AttributePath != "" {
		attrRef = serializeAttributeRef(col.AttributePath)
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(col.ID))},
		{Key: "$Type", Value: "Forms$DataGridColumn"},
		{Key: "AggregateCaption", Value: serializeEmptyText()},
		{Key: "AggregateFunction", Value: "None"},
		{Key: "Appearance", Value: serializeAppearance("", "", "", nil)},
		{Key: "AttributeRef", Value: attrRef},
		{Key: "Caption", Value: caption},
		{Key: "ConditionalEditabilitySettings", Value: nil},
		{Key: "Editable", Value: col.Editable},
		{Key: "FormatType", Value: "Attribute"},
		{Key: "Name", Value: col.Name},
		{Key: "ShowTooltip", Value: true},
		{Key: "Width", Value: int64(100)},
	}

	return doc
}

// serializeDataGridDataSource serializes a data source for DataGrid widgets.
func serializeDataGridDataSource(ds pages.DataSource) any {
	if ds == nil {
		return nil
	}

	switch d := ds.(type) {
	case *pages.DatabaseSource:
		// Build entity reference
		var entityRef any
		if d.EntityName != "" {
			entityRef = bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "DomainModels$IndirectEntityRef"},
				{Key: "Entity", Value: d.EntityName},
			}
		}

		// Build sort bar
		var sortBar any
		if len(d.Sorting) > 0 {
			sortItems := bson.A{int32(2)}
			for _, sort := range d.Sorting {
				sortDir := "Ascending"
				if sort.Direction == pages.SortDirectionDescending {
					sortDir = "Descending"
				}
				sortItem := bson.D{
					{Key: "$ID", Value: idToBsonBinary(generateUUID())},
					{Key: "$Type", Value: "Forms$GridSort"},
					{Key: "AttributeRef", Value: serializeAttributeRef(sort.AttributePath)},
					{Key: "SortOrder", Value: sortDir},
				}
				sortItems = append(sortItems, sortItem)
			}
			sortBar = bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Forms$GridSortBar"},
				{Key: "SortItems", Value: sortItems},
			}
		} else {
			sortBar = bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Forms$GridSortBar"},
				{Key: "SortItems", Value: bson.A{int32(3)}},
			}
		}

		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(d.ID))},
			{Key: "$Type", Value: "Forms$NewGridDatabaseSource"},
			{Key: "EntityRef", Value: entityRef},
			{Key: "SortBar", Value: sortBar},
			{Key: "XPathConstraint", Value: d.XPathConstraint},
		}
	default:
		return nil
	}
}

// serializeNavigationList serializes a NavigationList widget.
func serializeNavigationList(nl *pages.NavigationList) bson.D {
	// Build items array
	items := bson.A{int32(3)} // Empty marker
	hasItems := false
	for _, item := range nl.Items {
		if !hasItems {
			items = bson.A{int32(2)} // First item: change to version 2
			hasItems = true
		}
		items = append(items, serializeNavigationListItem(item))
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(nl.ID))},
		{Key: "$Type", Value: "Forms$NavigationList"},
		{Key: "Appearance", Value: serializeAppearance(nl.Class, nl.Style, nl.DynamicClasses, nl.DesignProperties)},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "Items", Value: items},
		{Key: "Name", Value: nl.Name},
		{Key: "TabIndex", Value: int64(0)},
	}
	return doc
}

// serializeNavigationListItem serializes a NavigationListItem.
func serializeNavigationListItem(item *pages.NavigationListItem) bson.D {
	var widgets bson.A

	if len(item.Widgets) > 0 {
		// Item has explicit child widgets - serialize them directly
		widgets = bson.A{int32(2)}
		for _, w := range item.Widgets {
			widgetDoc := serializeWidget(w)
			if widgetDoc != nil {
				widgets = append(widgets, widgetDoc)
			}
		}
	} else {
		// No explicit widgets - create a DynamicText from the Caption field
		captionText := "Item"
		if item.Caption != nil {
			for _, text := range item.Caption.Translations {
				captionText = text
				break
			}
		}

		dt := &pages.DynamicText{
			BaseWidget: pages.BaseWidget{
				BaseElement: model.BaseElement{
					ID:       model.ID(generateUUID()),
					TypeName: "Forms$DynamicText",
				},
				Name: "text_" + item.Name,
			},
			Content: &pages.ClientTemplate{
				BaseElement: model.BaseElement{
					ID:       model.ID(generateUUID()),
					TypeName: "Forms$ClientTemplate",
				},
				Template: &model.Text{
					BaseElement: model.BaseElement{
						ID:       model.ID(generateUUID()),
						TypeName: "Texts$Text",
					},
					Translations: map[string]string{"en_US": captionText},
				},
			},
			RenderMode: pages.TextRenderModeText,
		}
		widgets = bson.A{int32(2), serializeDynamicText(dt)}
	}

	// Build action
	action := serializeClientAction(item.Action)

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(item.ID))},
		{Key: "$Type", Value: "Forms$NavigationListItem"},
		{Key: "Action", Value: action},
		{Key: "Appearance", Value: serializeAppearance("", "", "", nil)},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "Name", Value: item.Name},
		{Key: "Widgets", Value: widgets},
	}
}

// serializeStaticImage serializes a StaticImage widget.
func serializeStaticImage(img *pages.StaticImage) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(img.ID))},
		{Key: "$Type", Value: "Forms$StaticImageViewer"},
		{Key: "Appearance", Value: serializeAppearance(img.Class, img.Style, img.DynamicClasses, img.DesignProperties)},
		{Key: "ClickAction", Value: serializeClientAction(img.OnClickAction)},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "Height", Value: int64(img.Height)},
		{Key: "HeightUnit", Value: "Auto"},
		{Key: "Image", Value: nil},
		{Key: "Name", Value: img.Name},
		{Key: "NativeAccessibilitySettings", Value: nil},
		{Key: "Responsive", Value: img.Responsive},
		{Key: "TabIndex", Value: int64(0)},
		{Key: "Width", Value: int64(img.Width)},
		{Key: "WidthUnit", Value: "Auto"},
	}
	return doc
}

// serializeDynamicImage serializes a DynamicImage widget.
func serializeDynamicImage(img *pages.DynamicImage) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(img.ID))},
		{Key: "$Type", Value: "Forms$ImageViewer"},
		{Key: "AlternativeText", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Forms$ClientTemplate"},
			{Key: "FallbackValue", Value: ""},
			{Key: "Template", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Texts$Text"},
				{Key: "Items", Value: bson.A{int32(3)}},
			}},
		}},
		{Key: "Appearance", Value: serializeAppearance(img.Class, img.Style, img.DynamicClasses, img.DesignProperties)},
		{Key: "ClickAction", Value: serializeClientAction(img.OnClickAction)},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "DataSource", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Forms$ImageViewerSource"},
			{Key: "EntityRef", Value: nil},
		}},
		{Key: "DefaultImage", Value: nil},
		{Key: "Height", Value: int64(img.Height)},
		{Key: "HeightUnit", Value: "Auto"},
		{Key: "Name", Value: img.Name},
		{Key: "NativeAccessibilitySettings", Value: nil},
		{Key: "OnClickEnlarge", Value: false},
		{Key: "Responsive", Value: img.Responsive},
		{Key: "ShowAsThumbnail", Value: false},
		{Key: "TabIndex", Value: int64(0)},
		{Key: "Width", Value: int64(img.Width)},
		{Key: "WidthUnit", Value: "Auto"},
	}
	return doc
}
