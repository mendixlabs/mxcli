// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// pageWireframeData is the JSON output for page wireframe diagrams.
type pageWireframeData struct {
	Format     string                    `json:"format"`
	Type       string                    `json:"type"`
	Name       string                    `json:"name"`
	Title      string                    `json:"title,omitempty"`
	Layout     string                    `json:"layout,omitempty"`
	Parameters []pageWireframeParam      `json:"parameters,omitempty"`
	Root       []wireframeNode           `json:"root"`
	MdlSource  string                    `json:"mdlSource,omitempty"`
	SourceMap  map[string]elkSourceRange `json:"sourceMap,omitempty"`
}

type pageWireframeParam struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type wireframeNode struct {
	ID               string                `json:"id"`
	Widget           string                `json:"widget"`
	Name             string                `json:"name,omitempty"`
	Label            string                `json:"label,omitempty"`
	Binding          string                `json:"binding,omitempty"`
	Caption          string                `json:"caption,omitempty"`
	Content          string                `json:"content,omitempty"`
	ButtonStyle      string                `json:"buttonStyle,omitempty"`
	Action           string                `json:"action,omitempty"`
	DataSource       string                `json:"datasource,omitempty"`
	Class            string                `json:"class,omitempty"`
	Style            string                `json:"style,omitempty"`
	DesignProperties []wireframeDesignProp `json:"designProperties,omitempty"`
	Columns          []wireframeColumn     `json:"columns,omitempty"`
	Rows             []wireframeRow        `json:"rows,omitempty"`
	TabPages         []wireframeTabPage    `json:"tabPages,omitempty"`
	// Slots are a pluggable widget's named widget slots other than its body:
	// a data grid's control bar and empty-state placeholder, a gallery's
	// filters (ako/mxcli#2465).
	Slots    []wireframeSlot `json:"slots,omitempty"`
	Children []wireframeNode `json:"children,omitempty"`
}

// wireframeSlot is one named widget slot, named as describe names it.
type wireframeSlot struct {
	Name     string          `json:"name"`
	Children []wireframeNode `json:"children"`
}

type wireframeDesignProp struct {
	Key   string `json:"key"`
	Value string `json:"value"` // "ON" for toggle, option value for option
	Type  string `json:"type"`  // "toggle" or "option"
}

type wireframeRow struct {
	Columns []wireframeRowColumn `json:"columns"`
}

type wireframeRowColumn struct {
	Weight   int             `json:"weight"`
	Children []wireframeNode `json:"children"`
}

type wireframeColumn struct {
	Caption string `json:"caption"`
	Binding string `json:"binding,omitempty"`
	// Children are the column's custom content and Filter its filter widgets
	// (ako/mxcli#2465).
	Children []wireframeNode `json:"children,omitempty"`
	Filter   []wireframeNode `json:"filter,omitempty"`
}

type wireframeTabPage struct {
	Caption  string          `json:"caption"`
	Children []wireframeNode `json:"children"`
}

// wireframeCounter generates unique IDs for wireframe nodes.
type wireframeCounter struct {
	count int
}

func (c *wireframeCounter) next() string {
	id := fmt.Sprintf("wf-%d", c.count)
	c.count++
	return id
}

// PageWireframeJSON generates wireframe JSON for a page.
func PageWireframeJSON(ctx *ExecContext, name string) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	parts := strings.SplitN(name, ".", 2)
	if len(parts) != 2 {
		return mdlerrors.NewValidationf("expected qualified name Module.Page, got: %s", name)
	}

	qn := ast.QualifiedName{Module: parts[0], Name: parts[1]}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find the page
	allPages, err := ctx.Backend.ListPages()
	if err != nil {
		return mdlerrors.NewBackend("list pages", err)
	}

	var foundPage *pages.Page
	for _, p := range allPages {
		modID := h.FindModuleID(p.ContainerID)
		modName := h.GetModuleName(modID)
		if p.Name == qn.Name && (qn.Module == "" || modName == qn.Module) {
			foundPage = p
			break
		}
	}

	if foundPage == nil {
		return mdlerrors.NewNotFound("page", name)
	}

	modID := h.FindModuleID(foundPage.ContainerID)
	modName := h.GetModuleName(modID)
	qualifiedName := modName + "." + foundPage.Name

	// Extract page metadata
	title := pickTextTranslation(foundPage.Title, describeDefaultLanguage(ctx))

	layoutName := ""
	rawData, _ := ctx.Backend.GetRawUnit(foundPage.ID)
	if rawData != nil {
		if formCall, ok := rawData["FormCall"].(map[string]any); ok {
			if layoutID := extractBinaryID(formCall["Layout"]); layoutID != "" {
				layoutName = resolveLayoutName(ctx, model.ID(layoutID))
			} else if formName, ok := formCall["Form"].(string); ok && formName != "" {
				layoutName = formName
			}
		}
	}

	// Extract parameters
	var params []pageWireframeParam
	for _, p := range foundPage.Parameters {
		entityName := p.EntityName
		if entityName == "" {
			entityName = string(p.EntityID)
		}
		params = append(params, pageWireframeParam{
			Name: p.Name,
			Type: entityName,
		})
	}

	// Get widget tree
	rawWidgets := getPageWidgetsFromRaw(ctx, foundPage.ID)

	// Convert to wireframe nodes
	counter := &wireframeCounter{}
	var root []wireframeNode
	for _, w := range rawWidgets {
		root = append(root, rawWidgetToWireframe(w, counter))
	}

	// Generate MDL source
	mdlSource, sourceMap := pageToMdlString(ctx, qn, root)

	data := pageWireframeData{
		Format:     "wireframe",
		Type:       "page",
		Name:       qualifiedName,
		Title:      title,
		Layout:     layoutName,
		Parameters: params,
		Root:       root,
		MdlSource:  mdlSource,
		SourceMap:  sourceMap,
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return mdlerrors.NewBackend("marshal wireframe json", err)
	}

	fmt.Fprint(ctx.Output, string(jsonBytes))
	return nil
}

// SnippetWireframeJSON generates wireframe JSON for a snippet.
func SnippetWireframeJSON(ctx *ExecContext, name string) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	parts := strings.SplitN(name, ".", 2)
	if len(parts) != 2 {
		return mdlerrors.NewValidationf("expected qualified name Module.Snippet, got: %s", name)
	}
	qn := ast.QualifiedName{Module: parts[0], Name: parts[1]}

	allSnippets, err := ctx.Backend.ListSnippets()
	if err != nil {
		return mdlerrors.NewBackend("list snippets", err)
	}

	var foundSnippet *pages.Snippet
	for _, s := range allSnippets {
		modID := h.FindModuleID(s.ContainerID)
		modName := h.GetModuleName(modID)
		if s.Name == qn.Name && (qn.Module == "" || modName == qn.Module) {
			foundSnippet = s
			break
		}
	}

	if foundSnippet == nil {
		return mdlerrors.NewNotFound("snippet", name)
	}

	modID := h.FindModuleID(foundSnippet.ContainerID)
	modName := h.GetModuleName(modID)
	qualifiedName := modName + "." + foundSnippet.Name

	rawWidgets := getSnippetWidgetsFromRaw(ctx, foundSnippet.ID)

	counter := &wireframeCounter{}
	var root []wireframeNode
	for _, w := range rawWidgets {
		root = append(root, rawWidgetToWireframe(w, counter))
	}

	data := pageWireframeData{
		Format: "wireframe",
		Type:   "page",
		Name:   qualifiedName,
		Root:   root,
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return mdlerrors.NewBackend("marshal wireframe json", err)
	}

	fmt.Fprint(ctx.Output, string(jsonBytes))
	return nil
}

// rawWidgetToWireframe converts a rawWidget to a wireframeNode.
func rawWidgetToWireframe(w rawWidget, counter *wireframeCounter) wireframeNode {
	node := wireframeNode{
		ID:    counter.next(),
		Name:  w.Name,
		Class: w.Class,
		Style: w.Style,
	}
	for _, dp := range w.DesignProperties {
		val := dp.Option
		if dp.ValueType == "toggle" {
			val = "on"
		}
		node.DesignProperties = append(node.DesignProperties, wireframeDesignProp{
			Key:   dp.Key,
			Value: val,
			Type:  dp.ValueType,
		})
	}

	switch w.Type {
	case "Forms$LayoutGrid", "Pages$LayoutGrid":
		node.Widget = "layoutgrid"
		for _, row := range w.Rows {
			wfRow := wireframeRow{}
			for _, col := range row.Columns {
				wfCol := wireframeRowColumn{
					Weight: col.Width,
				}
				for _, child := range col.Widgets {
					wfCol.Children = append(wfCol.Children, rawWidgetToWireframe(child, counter))
				}
				wfRow.Columns = append(wfRow.Columns, wfCol)
			}
			node.Rows = append(node.Rows, wfRow)
		}

	case "Forms$DataView", "Pages$DataView":
		node.Widget = "dataview"
		if w.DataSource != nil {
			node.DataSource = formatDataSourceRef(w.DataSource)
		}
		for _, child := range w.Children {
			node.Children = append(node.Children, rawWidgetToWireframe(child, counter))
		}

	case "Forms$TextBox", "Pages$TextBox":
		node.Widget = "textbox"
		node.Label = w.Caption
		node.Binding = w.Content

	case "Forms$TextArea", "Pages$TextArea":
		node.Widget = "textarea"
		node.Label = w.Caption
		node.Binding = w.Content

	case "Forms$DatePicker", "Pages$DatePicker":
		node.Widget = "datepicker"
		node.Label = w.Caption
		node.Binding = w.Content

	case "Forms$CheckBox", "Pages$CheckBox":
		node.Widget = "checkbox"
		node.Label = w.Caption
		node.Binding = w.Content

	case "Forms$RadioButtons", "Pages$RadioButtons":
		node.Widget = "radiobuttons"
		node.Label = w.Caption
		node.Binding = w.Content

	case "Forms$ActionButton", "Pages$ActionButton":
		node.Widget = "actionbutton"
		node.Caption = w.Caption
		node.ButtonStyle = w.ButtonStyle
		node.Action = w.Action

	case "Forms$Title", "Pages$Title":
		node.Widget = "title"
		node.Caption = w.Caption

	case "Forms$Text", "Pages$Text":
		node.Widget = "text"
		node.Content = w.Content

	case "Forms$DynamicText", "Pages$DynamicText":
		node.Widget = "dynamictext"
		node.Content = w.Content

	case "Forms$Label", "Pages$Label":
		node.Widget = "label"
		node.Content = w.Content

	case "Forms$SnippetCallWidget", "Pages$SnippetCallWidget":
		node.Widget = "snippetcall"
		node.Content = w.Content

	case "Footer":
		node.Widget = "footer"
		for _, child := range w.Children {
			node.Children = append(node.Children, rawWidgetToWireframe(child, counter))
		}

	case "Forms$NavigationList", "Pages$NavigationList":
		node.Widget = "navigationlist"
		for _, child := range w.Children {
			node.Children = append(node.Children, rawWidgetToWireframe(child, counter))
		}

	case "NavigationListItem":
		node.Widget = "navigationlistitem"
		node.Action = w.Action
		for _, child := range w.Children {
			node.Children = append(node.Children, rawWidgetToWireframe(child, counter))
		}

	case "Forms$ListView", "Pages$ListView":
		node.Widget = "listview"
		if w.DataSource != nil {
			node.DataSource = formatDataSourceRef(w.DataSource)
		}
		for _, child := range w.Children {
			node.Children = append(node.Children, rawWidgetToWireframe(child, counter))
		}

	case "Forms$Gallery", "Pages$Gallery":
		node.Widget = "gallery"
		if w.DataSource != nil {
			node.DataSource = formatDataSourceRef(w.DataSource)
		}
		for _, child := range w.Children {
			node.Children = append(node.Children, rawWidgetToWireframe(child, counter))
		}

	case "CustomWidgets$CustomWidget":
		node.Widget = mapCustomWidgetType(w.RenderMode)
		node.Label = w.Caption
		node.Binding = w.Content
		if w.DataSource != nil {
			node.DataSource = formatDataSourceRef(w.DataSource)
		}
		// DataGrid2 columns
		for _, col := range w.DataGridColumns {
			node.Columns = append(node.Columns, wireframeColumn{
				Caption:  col.Caption,
				Binding:  col.Attribute,
				Children: wireframeNodes(col.ContentWidgets, counter),
				Filter:   wireframeNodes(col.FilterWidgets, counter),
			})
		}
		// Gallery/DataGrid children
		for _, child := range w.Children {
			node.Children = append(node.Children, rawWidgetToWireframe(child, counter))
		}
		// The other slots, in describe's order (ako/mxcli#2465).
		addSlot := func(name string, ws []rawWidget) {
			if len(ws) > 0 {
				node.Slots = append(node.Slots, wireframeSlot{Name: name, Children: wireframeNodes(ws, counter)})
			}
		}
		addSlot("controlbar", w.ControlBar)
		addSlot("filter", w.FilterWidgets)
		for _, slot := range w.ChildSlots {
			addSlot(slot.Keyword, slot.Widgets)
		}

	default:
		// Unknown widget types
		node.Widget = normalizeWidgetType(w.Type)
		for _, child := range w.Children {
			node.Children = append(node.Children, rawWidgetToWireframe(child, counter))
		}
	}

	return node
}

// formatDataSourceRef formats a data source reference for display.
func wireframeNodes(ws []rawWidget, counter *wireframeCounter) []wireframeNode {
	var out []wireframeNode
	for _, w := range ws {
		out = append(out, rawWidgetToWireframe(w, counter))
	}
	return out
}

func formatDataSourceRef(ds *rawDataSource) string {
	switch ds.Type {
	case "parameter":
		return "$" + ds.Reference
	case "microflow":
		return "microflow " + ds.Reference
	case "nanoflow":
		return "nanoflow " + ds.Reference
	case "database":
		if ds.Reference != "" {
			return "database " + ds.Reference
		}
		return "database"
	default:
		return ds.Reference
	}
}

// mapCustomWidgetType maps RenderMode to wireframe widget type.
func mapCustomWidgetType(renderMode string) string {
	switch renderMode {
	case "datagrid2":
		return "datagrid"
	case "gallery":
		return "gallery"
	case "combobox":
		return "combobox"
	case "textfilter":
		return "textfilter"
	case "numberfilter":
		return "numberfilter"
	case "dropdownfilter":
		return "dropdownfilter"
	case "datefilter":
		return "datefilter"
	default:
		return strings.ToLower(renderMode)
	}
}

// normalizeWidgetType extracts a simple widget name from a BSON type.
func normalizeWidgetType(typeName string) string {
	// "Forms$TabContainer" -> "tabcontainer"
	if idx := strings.LastIndex(typeName, "$"); idx >= 0 {
		return strings.ToLower(typeName[idx+1:])
	}
	return strings.ToLower(typeName)
}

// pageToMdlString generates MDL source for the page using the output-swap technique.
func pageToMdlString(ctx *ExecContext, name ast.QualifiedName, root []wireframeNode) (string, map[string]elkSourceRange) {
	var buf strings.Builder
	origOutput := ctx.Output
	ctx.Output = &buf
	if err := describePage(ctx, name); err != nil {
		log.Printf("warning: describePage failed for %s.%s: %v", name.Module, name.Name, err)
	}
	ctx.Output = origOutput

	mdlSource := buf.String()
	if mdlSource == "" {
		return "", nil
	}

	// Build source map: map widget IDs to line ranges in the MDL source.
	// We do a simple scan: for each wireframe node, find a line containing its name.
	sourceMap := make(map[string]elkSourceRange)
	lines := strings.Split(mdlSource, "\n")
	mapWidgetToLines(root, lines, sourceMap)

	return mdlSource, sourceMap
}

// mapWidgetToLines maps wireframe node IDs to line ranges in MDL source.
func mapWidgetToLines(nodes []wireframeNode, lines []string, sourceMap map[string]elkSourceRange) {
	for _, node := range nodes {
		searchTerm := ""
		if node.Name != "" {
			searchTerm = node.Name
		} else if node.Label != "" {
			searchTerm = node.Label
		} else if node.Caption != "" {
			searchTerm = node.Caption
		}

		if searchTerm != "" {
			for i, line := range lines {
				if strings.Contains(line, searchTerm) {
					sourceMap[node.ID] = elkSourceRange{StartLine: i, EndLine: i}
					break
				}
			}
		}

		// Recurse into children
		mapWidgetToLines(node.Children, lines, sourceMap)
		for _, row := range node.Rows {
			for _, col := range row.Columns {
				mapWidgetToLines(col.Children, lines, sourceMap)
			}
		}
		for _, tab := range node.TabPages {
			mapWidgetToLines(tab.Children, lines, sourceMap)
		}
		for _, col := range node.Columns {
			mapWidgetToLines(col.Children, lines, sourceMap)
			mapWidgetToLines(col.Filter, lines, sourceMap)
		}
		for _, slot := range node.Slots {
			mapWidgetToLines(slot.Children, lines, sourceMap)
		}
	}
}

func (e *Executor) PageWireframeJSON(name string) error {
	return PageWireframeJSON(e.newExecContext(context.Background()), name)
}

func (e *Executor) SnippetWireframeJSON(name string) error {
	return SnippetWireframeJSON(e.newExecContext(context.Background()), name)
}
