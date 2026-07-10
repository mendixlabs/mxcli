// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// PagesAPI provides methods for working with pages.
type PagesAPI struct {
	api *ModelAPI
}

// CreatePage starts building a new page.
func (p *PagesAPI) CreatePage(name string) *PageBuilder {
	return &PageBuilder{
		api:  p,
		name: name,
		page: &pages.Page{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "Pages$Page",
			},
			Name: name,
		},
	}
}

// GetPage retrieves a page by qualified name.
func (p *PagesAPI) GetPage(qualifiedName string) (*pages.Page, error) {
	qn := ParseQualifiedName(qualifiedName)

	// List all pages and find by module/name
	allPages, err := p.api.reader.ListPages()
	if err != nil {
		return nil, err
	}

	// Get the module to match container ID
	module, err := p.api.reader.GetModuleByName(qn.ModuleName)
	if err != nil {
		return nil, fmt.Errorf("module not found: %s", qn.ModuleName)
	}

	for _, page := range allPages {
		if page.ContainerID == module.ID && page.Name == qn.ElementName {
			return page, nil
		}
	}

	return nil, fmt.Errorf("page not found: %s", qualifiedName)
}

// GetLayout retrieves a layout by qualified name.
func (p *PagesAPI) GetLayout(qualifiedName string) (*pages.Layout, error) {
	qn := ParseQualifiedName(qualifiedName)

	// List all layouts and find by module/name
	layouts, err := p.api.reader.ListLayouts()
	if err != nil {
		return nil, err
	}

	// Get the module to match container ID
	module, err := p.api.reader.GetModuleByName(qn.ModuleName)
	if err != nil {
		return nil, fmt.Errorf("module not found: %s", qn.ModuleName)
	}

	for _, layout := range layouts {
		if layout.ContainerID == module.ID && layout.Name == qn.ElementName {
			return layout, nil
		}
	}

	return nil, fmt.Errorf("layout not found: %s", qualifiedName)
}

// FindPagesWithEntity finds all pages that reference a given entity.
func (p *PagesAPI) FindPagesWithEntity(entityName string) ([]*pages.Page, error) {
	allPages, err := p.api.reader.ListPages()
	if err != nil {
		return nil, err
	}

	// This is a simplified implementation - would need deep widget traversal
	// for full implementation
	var result []*pages.Page
	for _, page := range allPages {
		// Check parameters
		for _, param := range page.Parameters {
			if param.EntityName == entityName {
				result = append(result, page)
				break
			}
		}
	}

	return result, nil
}

// CreateDataView starts building a new DataView widget.
func (p *PagesAPI) CreateDataView() *DataViewBuilder {
	return &DataViewBuilder{
		api: p,
		dataView: &pages.DataView{
			BaseWidget: pages.BaseWidget{
				BaseElement: model.BaseElement{
					ID:       generateID(),
					TypeName: "Forms$DataView",
				},
			},
		},
	}
}

// CreateTextBox starts building a new TextBox widget.
func (p *PagesAPI) CreateTextBox(name string) *TextBoxBuilder {
	return &TextBoxBuilder{
		api: p,
		textBox: &pages.TextBox{
			BaseWidget: pages.BaseWidget{
				BaseElement: model.BaseElement{
					ID:       generateID(),
					TypeName: "Forms$TextBox",
				},
				Name: name,
			},
		},
	}
}

// CreateTextArea starts building a new TextArea widget.
func (p *PagesAPI) CreateTextArea(name string) *TextAreaBuilder {
	return &TextAreaBuilder{
		api: p,
		textArea: &pages.TextArea{
			BaseWidget: pages.BaseWidget{
				BaseElement: model.BaseElement{
					ID:       generateID(),
					TypeName: "Forms$TextArea",
				},
				Name: name,
			},
		},
	}
}

// CreateDatePicker starts building a new DatePicker widget.
func (p *PagesAPI) CreateDatePicker(name string) *DatePickerBuilder {
	return &DatePickerBuilder{
		api: p,
		datePicker: &pages.DatePicker{
			BaseWidget: pages.BaseWidget{
				BaseElement: model.BaseElement{
					ID:       generateID(),
					TypeName: "Forms$DatePicker",
				},
				Name: name,
			},
		},
	}
}

// CreateCheckBox starts building a new CheckBox widget.
func (p *PagesAPI) CreateCheckBox(name string) *CheckBoxBuilder {
	return &CheckBoxBuilder{
		api: p,
		checkBox: &pages.CheckBox{
			BaseWidget: pages.BaseWidget{
				BaseElement: model.BaseElement{
					ID:       generateID(),
					TypeName: "Forms$CheckBox",
				},
				Name: name,
			},
		},
	}
}

// CreateDropDown starts building a new DropDown widget.
func (p *PagesAPI) CreateDropDown(name string) *DropDownBuilder {
	return &DropDownBuilder{
		api: p,
		dropDown: &pages.DropDown{
			BaseWidget: pages.BaseWidget{
				BaseElement: model.BaseElement{
					ID:       generateID(),
					TypeName: "Forms$DropDown",
				},
				Name: name,
			},
		},
	}
}

// CreateButton starts building a new ActionButton widget.
func (p *PagesAPI) CreateButton(caption string) *ButtonBuilder {
	return &ButtonBuilder{
		api:     p,
		caption: caption,
		button: &pages.ActionButton{
			BaseWidget: pages.BaseWidget{
				BaseElement: model.BaseElement{
					ID:       generateID(),
					TypeName: "Forms$ActionButton",
				},
			},
			Caption: &model.Text{
				BaseElement: model.BaseElement{
					ID:       generateID(),
					TypeName: "Texts$Text",
				},
				Translations: map[string]string{
					"en_US": caption,
				},
			},
		},
	}
}

// CreateLayoutGrid starts building a new LayoutGrid widget.
func (p *PagesAPI) CreateLayoutGrid() *LayoutGridBuilder {
	return &LayoutGridBuilder{
		api: p,
		grid: &pages.LayoutGrid{
			BaseWidget: pages.BaseWidget{
				BaseElement: model.BaseElement{
					ID:       generateID(),
					TypeName: "Forms$LayoutGrid",
				},
			},
		},
	}
}

// CreateContainer starts building a new Container (DivContainer) widget.
func (p *PagesAPI) CreateContainer() *ContainerBuilder {
	return &ContainerBuilder{
		api: p,
		container: &pages.Container{
			BaseWidget: pages.BaseWidget{
				BaseElement: model.BaseElement{
					ID:       generateID(),
					TypeName: "Forms$DivContainer",
				},
			},
		},
	}
}

// PageBuilder builds a new page with fluent API.
type PageBuilder struct {
	api        *PagesAPI
	name       string
	page       *pages.Page
	module     *model.Module
	layoutName string
	err        error
}

// InModule sets the module for this page.
func (b *PageBuilder) InModule(module *model.Module) *PageBuilder {
	b.module = module
	return b
}

// WithTitle sets the page title.
func (b *PageBuilder) WithTitle(title string) *PageBuilder {
	b.page.Title = &model.Text{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "Texts$Text",
		},
		Translations: map[string]string{
			"en_US": title,
		},
	}
	return b
}

// WithTitleTranslations sets the page title with multiple translations.
func (b *PageBuilder) WithTitleTranslations(translations map[string]string) *PageBuilder {
	b.page.Title = &model.Text{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "Texts$Text",
		},
		Translations: translations,
	}
	return b
}

// WithLayout sets the layout for this page.
func (b *PageBuilder) WithLayout(layoutName string) *PageBuilder {
	b.layoutName = layoutName
	return b
}

// WithURL sets the page URL.
func (b *PageBuilder) WithURL(url string) *PageBuilder {
	b.page.URL = url
	return b
}

// WithParameter adds a page parameter.
func (b *PageBuilder) WithParameter(name string, entityName string) *PageBuilder {
	param := &pages.PageParameter{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "Forms$PageParameter",
		},
		Name:       name,
		EntityName: entityName,
		IsRequired: true,
	}
	b.page.Parameters = append(b.page.Parameters, param)
	return b
}

// Build creates the page and saves it to the project.
func (b *PageBuilder) Build() (*pages.Page, error) {
	if b.err != nil {
		return nil, b.err
	}

	// Determine module
	module := b.module
	if module == nil {
		module = b.api.api.currentModule
	}
	if module == nil {
		return nil, fmt.Errorf("no module specified; use InModule() or api.SetModule()")
	}

	// Set container
	b.page.ContainerID = module.ID

	// Set up layout call if specified
	if b.layoutName != "" {
		b.page.LayoutCall = &pages.LayoutCall{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "Forms$LayoutCall",
			},
			LayoutName: b.layoutName,
		}
	}

	// Create the page
	err := b.api.api.writer.CreatePage(b.page)
	if err != nil {
		return nil, fmt.Errorf("failed to create page: %w", err)
	}

	return b.page, nil
}

// DataViewBuilder builds a DataView widget.
type DataViewBuilder struct {
	api        *PagesAPI
	dataView   *pages.DataView
	dataSource *pages.DataViewSource
}

// WithName sets the widget name.
func (b *DataViewBuilder) WithName(name string) *DataViewBuilder {
	b.dataView.Name = name
	return b
}

// WithEntity sets the entity for the data source.
func (b *DataViewBuilder) WithEntity(entityName string) *DataViewBuilder {
	if b.dataSource == nil {
		b.dataSource = &pages.DataViewSource{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "Forms$DataViewSource",
			},
		}
	}
	b.dataSource.EntityName = entityName
	return b
}

// FromParameter sets the data source to use a page parameter.
func (b *DataViewBuilder) FromParameter(paramName string) *DataViewBuilder {
	if b.dataSource == nil {
		b.dataSource = &pages.DataViewSource{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "Forms$DataViewSource",
			},
		}
	}
	b.dataSource.ParameterName = paramName
	return b
}

// Build returns the DataView widget.
func (b *DataViewBuilder) Build() *pages.DataView {
	if b.dataSource != nil {
		b.dataView.DataSource = b.dataSource
	}
	return b.dataView
}

// TextBoxBuilder builds a TextBox widget.
type TextBoxBuilder struct {
	api     *PagesAPI
	textBox *pages.TextBox
	parent  any
}

// WithLabel sets the label.
func (b *TextBoxBuilder) WithLabel(label string) *TextBoxBuilder {
	b.textBox.Label = label
	return b
}

// WithAttribute sets the attribute path.
func (b *TextBoxBuilder) WithAttribute(attributePath string) *TextBoxBuilder {
	b.textBox.AttributePath = attributePath
	return b
}

// WithPlaceholder sets the placeholder text.
func (b *TextBoxBuilder) WithPlaceholder(placeholder string) *TextBoxBuilder {
	b.textBox.Placeholder = &model.Text{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "Texts$Text",
		},
		Translations: map[string]string{
			"en_US": placeholder,
		},
	}
	return b
}

// AddTo adds this widget to a parent container.
func (b *TextBoxBuilder) AddTo(parent any) *TextBoxBuilder {
	b.parent = parent
	addWidgetToParent(b.textBox, parent)
	return b
}

// Build returns the TextBox widget.
func (b *TextBoxBuilder) Build() *pages.TextBox {
	return b.textBox
}

// TextAreaBuilder builds a TextArea widget.
type TextAreaBuilder struct {
	api      *PagesAPI
	textArea *pages.TextArea
}

// WithLabel sets the label.
func (b *TextAreaBuilder) WithLabel(label string) *TextAreaBuilder {
	b.textArea.Label = label
	return b
}

// WithAttribute sets the attribute path.
func (b *TextAreaBuilder) WithAttribute(attributePath string) *TextAreaBuilder {
	b.textArea.AttributePath = attributePath
	return b
}

// AddTo adds this widget to a parent container.
func (b *TextAreaBuilder) AddTo(parent any) *TextAreaBuilder {
	addWidgetToParent(b.textArea, parent)
	return b
}

// Build returns the TextArea widget.
func (b *TextAreaBuilder) Build() *pages.TextArea {
	return b.textArea
}

// DatePickerBuilder builds a DatePicker widget.
type DatePickerBuilder struct {
	api        *PagesAPI
	datePicker *pages.DatePicker
}

// WithLabel sets the label.
func (b *DatePickerBuilder) WithLabel(label string) *DatePickerBuilder {
	b.datePicker.Label = label
	return b
}

// WithAttribute sets the attribute path.
func (b *DatePickerBuilder) WithAttribute(attributePath string) *DatePickerBuilder {
	b.datePicker.AttributePath = attributePath
	return b
}

// AddTo adds this widget to a parent container.
func (b *DatePickerBuilder) AddTo(parent any) *DatePickerBuilder {
	addWidgetToParent(b.datePicker, parent)
	return b
}

// Build returns the DatePicker widget.
func (b *DatePickerBuilder) Build() *pages.DatePicker {
	return b.datePicker
}

// CheckBoxBuilder builds a CheckBox widget.
type CheckBoxBuilder struct {
	api      *PagesAPI
	checkBox *pages.CheckBox
}

// WithLabel sets the label.
func (b *CheckBoxBuilder) WithLabel(label string) *CheckBoxBuilder {
	b.checkBox.Label = label
	return b
}

// WithAttribute sets the attribute path.
func (b *CheckBoxBuilder) WithAttribute(attributePath string) *CheckBoxBuilder {
	b.checkBox.AttributePath = attributePath
	return b
}

// AddTo adds this widget to a parent container.
func (b *CheckBoxBuilder) AddTo(parent any) *CheckBoxBuilder {
	addWidgetToParent(b.checkBox, parent)
	return b
}

// Build returns the CheckBox widget.
func (b *CheckBoxBuilder) Build() *pages.CheckBox {
	return b.checkBox
}

// DropDownBuilder builds a DropDown widget.
type DropDownBuilder struct {
	api      *PagesAPI
	dropDown *pages.DropDown
}

// WithLabel sets the label.
func (b *DropDownBuilder) WithLabel(label string) *DropDownBuilder {
	b.dropDown.Label = label
	return b
}

// WithAttribute sets the attribute path.
func (b *DropDownBuilder) WithAttribute(attributePath string) *DropDownBuilder {
	b.dropDown.AttributePath = attributePath
	return b
}

// AddTo adds this widget to a parent container.
func (b *DropDownBuilder) AddTo(parent any) *DropDownBuilder {
	addWidgetToParent(b.dropDown, parent)
	return b
}

// Build returns the DropDown widget.
func (b *DropDownBuilder) Build() *pages.DropDown {
	return b.dropDown
}

// ButtonBuilder builds an ActionButton widget.
type ButtonBuilder struct {
	api     *PagesAPI
	caption string
	button  *pages.ActionButton
}

// WithStyle sets the button style.
func (b *ButtonBuilder) WithStyle(style string) *ButtonBuilder {
	switch style {
	case "Primary":
		b.button.ButtonStyle = pages.ButtonStylePrimary
	case "Success":
		b.button.ButtonStyle = pages.ButtonStyleSuccess
	case "Warning":
		b.button.ButtonStyle = pages.ButtonStyleWarning
	case "Danger":
		b.button.ButtonStyle = pages.ButtonStyleDanger
	default:
		b.button.ButtonStyle = pages.ButtonStyleDefault
	}
	return b
}

// WithSaveAction sets the button to save changes.
func (b *ButtonBuilder) WithSaveAction() *ButtonActionBuilder {
	action := &pages.SaveChangesClientAction{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "Forms$SaveChangesClientAction",
		},
	}
	b.button.Action = action
	return &ButtonActionBuilder{builder: b, saveAction: action}
}

// WithCancelAction sets the button to cancel changes.
func (b *ButtonBuilder) WithCancelAction() *ButtonActionBuilder {
	action := &pages.CancelChangesClientAction{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "Forms$CancelChangesClientAction",
		},
	}
	b.button.Action = action
	return &ButtonActionBuilder{builder: b, cancelAction: action}
}

// AddTo adds this widget to a parent container.
func (b *ButtonBuilder) AddTo(parent any) *ButtonBuilder {
	addWidgetToParent(b.button, parent)
	return b
}

// AddToFooter adds this widget to a DataView's footer.
func (b *ButtonBuilder) AddToFooter(dataView *pages.DataView) *ButtonBuilder {
	dataView.FooterWidgets = append(dataView.FooterWidgets, b.button)
	return b
}

// Build returns the ActionButton widget.
func (b *ButtonBuilder) Build() *pages.ActionButton {
	return b.button
}

// ButtonActionBuilder allows configuring button actions.
type ButtonActionBuilder struct {
	builder      *ButtonBuilder
	saveAction   *pages.SaveChangesClientAction
	cancelAction *pages.CancelChangesClientAction
}

// ClosePage sets the action to close the page after execution.
func (b *ButtonActionBuilder) ClosePage() *ButtonBuilder {
	if b.saveAction != nil {
		b.saveAction.ClosePage = true
	}
	if b.cancelAction != nil {
		b.cancelAction.ClosePage = true
	}
	return b.builder
}

// LayoutGridBuilder builds a LayoutGrid widget.
type LayoutGridBuilder struct {
	api  *PagesAPI
	grid *pages.LayoutGrid
}

// WithRow adds a row to the grid.
func (b *LayoutGridBuilder) WithRow() *LayoutGridRowBuilder {
	row := &pages.LayoutGridRow{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "Forms$LayoutGridRow",
		},
	}
	b.grid.Rows = append(b.grid.Rows, row)
	return &LayoutGridRowBuilder{
		gridBuilder: b,
		row:         row,
	}
}

// Build returns the LayoutGrid widget.
func (b *LayoutGridBuilder) Build() *pages.LayoutGrid {
	return b.grid
}

// LayoutGridRowBuilder builds a row in a LayoutGrid.
type LayoutGridRowBuilder struct {
	gridBuilder *LayoutGridBuilder
	row         *pages.LayoutGridRow
}

// WithColumn adds a column to the row.
func (b *LayoutGridRowBuilder) WithColumn(weight int) *LayoutGridColumnBuilder {
	col := &pages.LayoutGridColumn{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "Forms$LayoutGridColumn",
		},
		Weight: weight,
	}
	b.row.Columns = append(b.row.Columns, col)
	return &LayoutGridColumnBuilder{
		rowBuilder: b,
		column:     col,
	}
}

// Done returns to the grid builder.
func (b *LayoutGridRowBuilder) Done() *LayoutGridBuilder {
	return b.gridBuilder
}

// LayoutGridColumnBuilder builds a column in a LayoutGrid row.
type LayoutGridColumnBuilder struct {
	rowBuilder *LayoutGridRowBuilder
	column     *pages.LayoutGridColumn
}

// WithWidget adds a widget to this column.
func (b *LayoutGridColumnBuilder) WithWidget(widget pages.Widget) *LayoutGridColumnBuilder {
	b.column.Widgets = append(b.column.Widgets, widget)
	return b
}

// Done returns to the row builder.
func (b *LayoutGridColumnBuilder) Done() *LayoutGridRowBuilder {
	return b.rowBuilder
}

// ContainerBuilder builds a Container widget.
type ContainerBuilder struct {
	api       *PagesAPI
	container *pages.Container
}

// WithName sets the container name.
func (b *ContainerBuilder) WithName(name string) *ContainerBuilder {
	b.container.Name = name
	return b
}

// WithWidget adds a widget to the container.
func (b *ContainerBuilder) WithWidget(widget pages.Widget) *ContainerBuilder {
	b.container.Widgets = append(b.container.Widgets, widget)
	return b
}

// Build returns the Container widget.
func (b *ContainerBuilder) Build() *pages.Container {
	return b.container
}

// Helper function to add a widget to various parent types.
func addWidgetToParent(widget pages.Widget, parent any) {
	switch p := parent.(type) {
	case *pages.DataView:
		p.Widgets = append(p.Widgets, widget)
	case *pages.Container:
		p.Widgets = append(p.Widgets, widget)
	case *pages.LayoutGridColumn:
		p.Widgets = append(p.Widgets, widget)
	}
}

// DataGrid2Column represents a column definition for DataGrid2.
type DataGrid2Column struct {
	Attribute string // Attribute name (short or qualified)
	Caption   string // Column header caption
}

// DataGrid2Builder builds a DataGrid2 pluggable widget.
type DataGrid2Builder struct {
	api        *PagesAPI
	name       string
	entityName string
	columns    []DataGrid2Column
}

// CreateDataGrid2 starts building a new DataGrid2 widget.
// DataGrid2 is the modern pluggable widget replacement for the deprecated DataGrid.
func (p *PagesAPI) CreateDataGrid2(name string) *DataGrid2Builder {
	return &DataGrid2Builder{
		api:  p,
		name: name,
	}
}

// WithEntity sets the entity for the database datasource.
func (b *DataGrid2Builder) WithEntity(entityName string) *DataGrid2Builder {
	b.entityName = entityName
	return b
}

// WithColumn adds a column to the DataGrid2.
func (b *DataGrid2Builder) WithColumn(attribute string, caption string) *DataGrid2Builder {
	b.columns = append(b.columns, DataGrid2Column{
		Attribute: attribute,
		Caption:   caption,
	})
	return b
}

// Build creates the DataGrid2 widget using the embedded template.
// Returns the CustomWidget that can be added to a page.
func (b *DataGrid2Builder) Build() (*pages.CustomWidget, error) {
	if b.entityName == "" {
		return nil, fmt.Errorf("entity name is required; use WithEntity()")
	}

	// Use the MDL executor to build the DataGrid2 via MDL syntax
	// This ensures we use the same template-based approach
	mdlColumns := ""
	for _, col := range b.columns {
		if mdlColumns != "" {
			mdlColumns += "\n    "
		}
		mdlColumns += fmt.Sprintf("COLUMN %s AS '%s';", col.Attribute, col.Caption)
	}

	// For now, return an error indicating to use MDL syntax
	// A full implementation would call the page builder directly
	return nil, fmt.Errorf(
		"DataGrid2 is best created via MDL syntax. Use:\n\n"+
			"DATAGRID %s\n  SOURCE DATABASE %s\nBEGIN\n    %s\nEND",
		b.name, b.entityName, mdlColumns)
}
