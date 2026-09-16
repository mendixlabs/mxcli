// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ============================================================================
// Describe Page
// ============================================================================

// describePage handles DESCRIBE PAGE command - outputs MDL V3 syntax.
func describePage(ctx *ExecContext, name ast.QualifiedName) error {
	// Get hierarchy for module/folder resolution
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
		if p.Name == name.Name && (name.Module == "" || modName == name.Module) {
			foundPage = p
			break
		}
	}

	if foundPage == nil {
		return mdlerrors.NewNotFound("page", name.String())
	}

	// Get module name for the page
	modID := h.FindModuleID(foundPage.ContainerID)
	modName := h.GetModuleName(modID)

	// Output documentation if present
	if foundPage.Documentation != "" {
		lines := strings.Split(foundPage.Documentation, "\n")
		fmt.Fprint(ctx.Output, "/**\n")
		for _, line := range lines {
			fmt.Fprintf(ctx.Output, " * %s\n", line)
		}
		fmt.Fprint(ctx.Output, " */\n")
	}

	// Pre-warm the project default language before any (possibly parallel) widget
	// text extraction, so the getter is a race-free cached read (issue #702).
	lang := describeDefaultLanguage(ctx)

	// Get title in the project default language (falls back to en_US, then any).
	title := pickTextTranslation(foundPage.Title, lang)

	// Get layout from raw data
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

	// @excluded annotation
	if foundPage.Excluded {
		fmt.Fprintln(ctx.Output, "@excluded")
	}

	// V3 syntax: CREATE PAGE Module.Page (Title: '...', Layout: ..., Params: { })
	header := fmt.Sprintf("create or modify page %s.%s", modName, foundPage.Name)
	props := []string{}
	if title != "" {
		props = append(props, fmt.Sprintf("Title: %s", mdlQuote(title)))
	}
	if layoutName != "" {
		props = append(props, fmt.Sprintf("Layout: %s", layoutName))
	}
	if foundPage.URL != "" {
		props = append(props, fmt.Sprintf("Url: %s", mdlQuote(foundPage.URL)))
	}
	if folderPath := h.BuildFolderPath(foundPage.ContainerID); folderPath != "" {
		props = append(props, fmt.Sprintf("Folder: %s", mdlQuote(folderPath)))
	}
	// Pop-up dimensions (issues #661, #713) — emit only non-default values so
	// the CREATE PAGE header round-trips. Studio Pro's default is 0/0 (auto-size);
	// an explicit non-zero value (e.g. 600) is emitted so it round-trips.
	if rawData != nil {
		if w := toInt(rawData["PopupWidth"]); w != 0 {
			props = append(props, fmt.Sprintf("PopupWidth: %d", w))
		}
		if hgt := toInt(rawData["PopupHeight"]); hgt != 0 {
			props = append(props, fmt.Sprintf("PopupHeight: %d", hgt))
		}
		if r, ok := rawData["PopupResizable"].(bool); ok && r {
			props = append(props, "PopupResizable: true")
		}
		// Page CSS class / inline style from Forms$Appearance (issue #714) — emit
		// only when set so the CREATE PAGE header round-trips.
		if ap, ok := rawData["Appearance"].(map[string]any); ok {
			if cls, _ := ap["Class"].(string); cls != "" {
				props = append(props, fmt.Sprintf("Class: %s", mdlQuote(cls)))
			}
			if st, _ := ap["Style"].(string); st != "" {
				props = append(props, fmt.Sprintf("Style: %s", mdlQuote(st)))
			}
		}
	}
	if len(foundPage.Parameters) > 0 {
		params := []string{}
		for _, p := range foundPage.Parameters {
			typeName := pageParamTypeMDL(p)
			params = append(params, fmt.Sprintf("$%s: %s", p.Name, typeName))
		}
		props = append(props, fmt.Sprintf("Params: { %s }", strings.Join(params, ", ")))
	}
	// Output page variables from raw BSON
	if rawData != nil {
		vars := getBsonArrayMaps(rawData["Variables"])
		if len(vars) > 0 {
			varParts := []string{}
			for _, v := range vars {
				varName, _ := v["Name"].(string)
				defaultVal, _ := v["DefaultValue"].(string)
				varTypeName := "Unknown"
				if vt, ok := v["VariableType"].(map[string]any); ok {
					if vtType, ok := vt["$Type"].(string); ok {
						// An enumeration carries the name it points at, so it has to
						// be read alongside the $Type or DESCRIBE loses the type again
						// at the far end of the round trip (#977).
						enumQN, _ := vt["Enumeration"].(string)
						varTypeName = pageVariableMDLType(vtType, enumQN)
					}
				}
				varParts = append(varParts, fmt.Sprintf("$%s: %s = %s", varName, varTypeName, mdlQuote(defaultVal)))
			}
			props = append(props, fmt.Sprintf("Variables: { %s }", strings.Join(varParts, ", ")))
		}
	}

	// Output widgets from raw page data, grouped by layout placeholder. A single
	// Main placeholder renders as bare widgets (backward-compatible); a page that
	// binds widgets to more than one placeholder emits explicit `placeholder
	// <Name> { … }` blocks so it round-trips (issue #532).
	groups := getPageWidgetGroupsFromRaw(ctx, foundPage.ID)
	var nonEmpty []pageWidgetGroup
	for _, g := range groups {
		if len(g.Widgets) > 0 {
			nonEmpty = append(nonEmpty, g)
		}
	}
	formatWidgetProps(ctx.Output, "", header, props, " {\n")
	switch {
	case len(nonEmpty) == 0:
		// A widget-less page still needs an (empty) body block: the CREATE PAGE
		// grammar requires `{ }`, so omitting it made DESCRIBE output fail to
		// re-parse through `mxcli check` (#626).
	case len(nonEmpty) == 1 && nonEmpty[0].Placeholder == "Main":
		for _, w := range nonEmpty[0].Widgets {
			outputWidgetMDLV3(ctx, w, 1)
		}
	default:
		for _, g := range nonEmpty {
			fmt.Fprintf(ctx.Output, "  placeholder %s {\n", mdlIdent(g.Placeholder))
			for _, w := range g.Widgets {
				outputWidgetMDLV3(ctx, w, 2)
			}
			fmt.Fprint(ctx.Output, "  }\n")
		}
	}
	fmt.Fprint(ctx.Output, "}")

	// Add GRANT VIEW if roles are assigned
	if len(foundPage.AllowedRoles) > 0 {
		roles := make([]string, len(foundPage.AllowedRoles))
		for i, r := range foundPage.AllowedRoles {
			roles[i] = string(r)
		}
		fmt.Fprintf(ctx.Output, "\n\ngrant view on page %s.%s to %s;",
			modName, foundPage.Name, strings.Join(roles, ", "))
	}

	fmt.Fprint(ctx.Output, "\n")
	return nil
}

// formatParametersV3 formats parameter expressions for MDL V3 ContentParams clause.
// Returns format like: {1} = FirstName, {2} = $ParamName.Attribute
// Parameter references keep their $ prefix, entity paths are shown without prefix.
func formatParametersV3(params []string) []string {
	result := make([]string, len(params))
	for i, p := range params {
		// Keep the parameter as-is - extraction already formats correctly:
		// - Entity paths: Module.Entity.Attribute (no $ prefix)
		// - Parameter refs: $ParamName.Attribute (with $ prefix)
		result[i] = fmt.Sprintf("{%d} = %s", i+1, p)
	}
	return result
}

// describeSnippet handles DESCRIBE SNIPPET command - outputs MDL V3 syntax.
func describeSnippet(ctx *ExecContext, name ast.QualifiedName) error {
	// Get hierarchy for module/folder resolution
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find the snippet
	allSnippets, err := ctx.Backend.ListSnippets()
	if err != nil {
		return mdlerrors.NewBackend("list snippets", err)
	}

	var foundSnippet *pages.Snippet
	for _, s := range allSnippets {
		modID := h.FindModuleID(s.ContainerID)
		modName := h.GetModuleName(modID)
		if s.Name == name.Name && (name.Module == "" || modName == name.Module) {
			foundSnippet = s
			break
		}
	}

	if foundSnippet == nil {
		return mdlerrors.NewNotFound("snippet", name.String())
	}

	// Get module name for the snippet
	modID := h.FindModuleID(foundSnippet.ContainerID)
	modName := h.GetModuleName(modID)

	// Output documentation if present
	if foundSnippet.Documentation != "" {
		lines := strings.Split(foundSnippet.Documentation, "\n")
		fmt.Fprint(ctx.Output, "/**\n")
		for _, line := range lines {
			fmt.Fprintf(ctx.Output, " * %s\n", line)
		}
		fmt.Fprint(ctx.Output, " */\n")
	}

	// Get raw data to check for parameters
	rawData, _ := ctx.Backend.GetRawUnit(foundSnippet.ID)
	var params []map[string]any
	if rawData != nil {
		params = getBsonArrayMaps(rawData["Parameters"])
	}

	// Output CREATE SNIPPET statement (V3 syntax)
	fmt.Fprintf(ctx.Output, "create or modify snippet %s.%s", modName, foundSnippet.Name)
	folderPath := h.BuildFolderPath(foundSnippet.ContainerID)
	if len(params) > 0 || folderPath != "" {
		snippetProps := []string{}
		if len(params) > 0 {
			paramParts := []string{}
			for _, p := range params {
				paramName, _ := p["Name"].(string)
				entityName := extractEntityQualifiedName(p["ParameterType"])
				paramParts = append(paramParts, fmt.Sprintf("$%s: %s", paramName, entityName))
			}
			snippetProps = append(snippetProps, fmt.Sprintf("Params: { %s }", strings.Join(paramParts, ", ")))
		}
		if folderPath != "" {
			snippetProps = append(snippetProps, fmt.Sprintf("Folder: %s", mdlQuote(folderPath)))
		}
		fmt.Fprintf(ctx.Output, " (%s)", strings.Join(snippetProps, ", "))
	}

	// Output widgets from raw snippet data
	rawWidgets := getSnippetWidgetsFromRaw(ctx, foundSnippet.ID)
	if len(rawWidgets) > 0 {
		fmt.Fprint(ctx.Output, " {\n")
		for _, w := range rawWidgets {
			outputWidgetMDLV3(ctx, w, 1)
		}
		fmt.Fprint(ctx.Output, "}")
	} else {
		// A widget-less snippet still needs an (empty) body block to re-parse (#626).
		fmt.Fprint(ctx.Output, " {\n}")
	}

	fmt.Fprint(ctx.Output, "\n")
	return nil
}

// describeLayout handles DESCRIBE LAYOUT command - outputs MDL-style representation.
func describeLayout(ctx *ExecContext, name ast.QualifiedName) error {
	// Get hierarchy for module/folder resolution
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find the layout
	allLayouts, err := ctx.Backend.ListLayouts()
	if err != nil {
		return mdlerrors.NewBackend("list layouts", err)
	}

	var foundLayout *pages.Layout
	for _, l := range allLayouts {
		modID := h.FindModuleID(l.ContainerID)
		modName := h.GetModuleName(modID)
		if l.Name == name.Name && (name.Module == "" || modName == name.Module) {
			foundLayout = l
			break
		}
	}

	if foundLayout == nil {
		return mdlerrors.NewNotFound("layout", name.String())
	}

	// Get module name for the layout
	modID := h.FindModuleID(foundLayout.ContainerID)
	modName := h.GetModuleName(modID)

	// Output documentation if present
	if foundLayout.Documentation != "" {
		lines := strings.Split(foundLayout.Documentation, "\n")
		fmt.Fprint(ctx.Output, "/**\n")
		for _, line := range lines {
			fmt.Fprintf(ctx.Output, " * %s\n", line)
		}
		fmt.Fprint(ctx.Output, " */\n")
	}

	// Output layout type comment.
	//
	// No fallback. Every Studio Pro layout stores a LayoutType — measured on
	// all 22 Atlas ships, across six values that split by platform (web:
	// Responsive / Phone / Tablet / ModalPopup, native: Default / Popup) — so
	// an empty one means the read failed, and defaulting it to "Responsive"
	// reported that failure as a fact.
	layoutTypeStr := string(foundLayout.LayoutType)
	if layoutTypeStr == "" {
		layoutTypeStr = "(not read)"
	}

	// Re-executable MDL, not a comment dump: `create layout` exists now, so a
	// describe that only narrated the tree would be the one document type whose
	// output cannot be fed back in.
	header := fmt.Sprintf("  layouttype: %s", mdlQuote(layoutTypeStr))
	if foundLayout.Class != "" {
		header += fmt.Sprintf(",\n  class: %s", mdlQuote(foundLayout.Class))
	}
	fmt.Fprintf(ctx.Output, "create layout %s.%s (\n%s\n) {\n",
		modName, mdlIdent(foundLayout.Name), header)

	for _, w := range getLayoutWidgetsFromRaw(ctx, foundLayout.ID) {
		outputWidgetMDLV3(ctx, w, 1)
	}

	fmt.Fprint(ctx.Output, "}\n\n")
	return nil
}

// getLayoutWidgetsFromRaw extracts widgets from raw layout BSON.
func getLayoutWidgetsFromRaw(ctx *ExecContext, layoutID model.ID) []rawWidget {
	// Get raw layout data
	rawData, err := ctx.Backend.GetRawUnit(layoutID)
	if err != nil {
		return nil
	}

	// A Forms$Layout has no Widget key. Its tree hangs off Content — a
	// Forms$WebLayoutContent (or Forms$NativeLayoutContent for the native
	// layouts) — whose Widgets array holds the root, normally a ScrollContainer.
	//
	// Reading rawData["Widget"] therefore never matched, and the miss was
	// silent: the type assertion failed, nil came back, and DESCRIBE LAYOUT
	// printed a header with no widget structure at all for an 84-element
	// document. Nothing distinguished that from a genuinely empty layout.
	content, ok := rawData["Content"].(map[string]any)
	if !ok {
		return nil
	}

	var out []rawWidget
	for _, w := range getBsonArrayElements(content["Widgets"]) {
		if wMap, ok := w.(map[string]any); ok {
			out = append(out, parseRawWidget(ctx, wMap)...)
		}
	}
	return out
}

// getSnippetWidgetsFromRaw extracts widgets from raw snippet BSON.
func getSnippetWidgetsFromRaw(ctx *ExecContext, snippetID model.ID) []rawWidget {
	// Get raw snippet data
	rawData, err := ctx.Backend.GetRawUnit(snippetID)
	if err != nil {
		return nil
	}

	// Handle both snippet formats:
	// - Studio Pro uses "Widgets" (plural): a top-level array of widgets
	// - mxcli uses "Widget" (singular): a single container whose "Widgets" field holds children
	var widgetsArray []any
	if wa := getBsonArrayElements(rawData["Widgets"]); wa != nil {
		widgetsArray = wa
	} else if widgetContainer, ok := rawData["Widget"].(map[string]any); ok {
		widgetsArray = getBsonArrayElements(widgetContainer["Widgets"])
	}
	if widgetsArray == nil {
		return nil
	}

	var result []rawWidget
	for _, w := range widgetsArray {
		if wMap, ok := w.(map[string]any); ok {
			result = append(result, parseRawWidget(ctx, wMap)...)
		}
	}
	return result
}

// extractEntityQualifiedName extracts the entity qualified name from a parameter type.
func extractEntityQualifiedName(paramType any) string {
	if paramType == nil {
		return "Unknown"
	}
	ptMap, ok := paramType.(map[string]any)
	if !ok {
		return "Unknown"
	}

	// Check for EntityType or ObjectType (snippet parameters use DataTypes$ObjectType)
	if entityType, ok := ptMap["$Type"].(string); ok {
		if entityType == "Pages$EntityType" || entityType == "Forms$EntityType" || entityType == "DataTypes$ObjectType" {
			if entityRef, ok := ptMap["Entity"].(string); ok && entityRef != "" {
				return entityRef
			}
		}
	}
	return "Unknown"
}

// getBsonArrayMaps extracts []map[string]interface{} from BSON array types.
func getBsonArrayMaps(v any) []map[string]any {
	if v == nil {
		return nil
	}
	switch arr := v.(type) {
	case []any:
		var result []map[string]any
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				result = append(result, m)
			}
		}
		return result
	case primitive.A:
		var result []map[string]any
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				result = append(result, m)
			}
		}
		return result
	default:
		return nil
	}
}

// resolveLayoutName resolves a layout ID to its qualified name.
func resolveLayoutName(ctx *ExecContext, layoutID model.ID) string {
	layouts, err := ctx.Backend.ListLayouts()
	if err != nil {
		return string(layoutID)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return string(layoutID)
	}

	for _, l := range layouts {
		if l.ID == layoutID {
			return h.GetQualifiedName(l.ContainerID, l.Name)
		}
	}
	return string(layoutID)
}

// rawSortColumn represents a sort column for describe output.
type rawSortColumn struct {
	Attribute string // Qualified name or simple identifier
	Order     string // "ASC" or "DESC"
}

// rawDataSource represents a data source for describe output.
type rawDataSource struct {
	Type            string          // "microflow", "nanoflow", "parameter", "database", "selection", "association"
	Reference       string          // Qualified name, parameter name, selection-source widget name, or association path
	XPathConstraint string          // XPath constraint (WHERE clause)
	SortColumns     []rawSortColumn // Multiple sort columns
	ContextVariable string          // association source: context variable name (empty → $currentObject)
	// Args carries a flow datasource's argument bindings, in stored order. A
	// microflow used as a datasource needs an argument for every parameter,
	// exactly as a call action does (#835) — describing it without them yields
	// MDL that rebuilds into CE1571 (mxcli-formula1 §57.2).
	Args []rawDataSourceArg
	// Unsupported carries the stored $Type of a datasource that has no MDL
	// spelling. Set instead of guessing a Type: describe reports it as a
	// comment, so the binding is visible in the output without producing a
	// statement that cannot be re-executed (#941).
	Unsupported string
}

// rawDataSourceArg is one argument bound to a flow datasource's parameter.
// Name is the bare parameter name; storage qualifies it with the flow it
// belongs to, which MDL does not repeat.
type rawDataSourceArg struct {
	Name  string
	Value string
}

// associationSourcePath reconstructs the association navigation of a
// Forms$AssociationSource (EntityRef = IndirectEntityRef of association steps)
// into its path ("Module.Assoc" or "Module.Assoc/…") plus the context variable
// ("currentObject" when the source has no page-parameter SourceVariable).
func associationSourcePath(ds map[string]any) (path, contextVar string) {
	entityRef, ok := ds["EntityRef"].(map[string]any)
	if !ok || entityRef == nil {
		return "", ""
	}
	steps := getBsonArrayElements(entityRef["Steps"])
	assocs := make([]string, 0, len(steps))
	for _, s := range steps {
		sm, ok := s.(map[string]any)
		if !ok {
			return "", ""
		}
		assoc := extractString(sm["Association"])
		if assoc == "" {
			return "", ""
		}
		assocs = append(assocs, assoc)
	}
	if len(assocs) == 0 {
		return "", ""
	}
	contextVar = "currentObject"
	if sv, ok := ds["SourceVariable"].(map[string]any); ok && sv != nil {
		if pp := extractString(sv["PageParameter"]); pp != "" {
			contextVar = pp
		}
	}
	return strings.Join(assocs, "/"), contextVar
}

// rawDataGridColumn represents a DataGrid2 column for describe output.
type rawDataGridColumn struct {
	Name              string // Widget name (generated if not stored)
	Attribute         string
	Caption           string
	CaptionParams     []string    // Parameters for template placeholders in caption
	ShowContentAs     string      // "attribute", "customContent", or "dynamicText"
	ContentWidgets    []rawWidget // Widgets inside the column (for custom content)
	DynamicText       string      // Template text for dynamicText mode
	DynamicTextParams []string    // Parameters for dynamicText template
	Alignment         string      // "left", "center", or "right" (empty = default "left")
	WrapText          string      // "true" or "false" (empty = default "false")
	Sortable          string      // "true" or "false"
	Resizable         string      // "true" or "false"
	Draggable         string      // "true" or "false"
	Hidable           string      // "yes", "hidden", or "no"
	ColumnWidth       string      // "autoFill", "autoFit", or "manual"
	Size              string      // e.g. "200" (default "1")
	Visible           string      // expression, e.g. "true"
	DynamicCellClass  string      // expression
	Tooltip           string      // text
}

// rawWidget represents a widget from raw BSON data for MDL output.
type rawWidget struct {
	Type        string
	Name        string
	Content     string
	Caption     string
	RenderMode  string
	Action      string
	ButtonStyle string
	// Icon is the qualified name an icon-collection or image icon points at.
	// Empty for no icon and for a glyph icon, which carries a numeric Code
	// instead. IconType keeps the storage $Type so the emitter can tell the
	// three apart — only Forms$IconCollectionIcon is authorable, and the two
	// named variants are spelled identically, so a reader that keeps the name
	// without the kind cannot avoid converting one into the other
	// (mendixlabs/mxcli#1059). Same split as types.NavMenuItem's.
	Icon           string // e.g. Atlas_Core.Atlas_Filled.pencil
	IconType       string // storage $Type, "" when the widget carries no icon
	IconCode       int    // Forms$GlyphIcon's Code — the only identity a glyph icon has
	Selection      string // For Gallery selection mode (Single, Multi, None)
	Class          string // CSS class from Appearance
	Style          string // Inline CSS style from Appearance
	DynamicClasses string // Dynamic-classes expression from Appearance
	Parameters     []string
	Children       []rawWidget
	FilterWidgets  []rawWidget // For Gallery filter widgets
	ControlBar     []rawWidget // For DataGrid2 CONTROLBAR widgets
	Rows           []rawWidgetRow
	DataSource     *rawDataSource
	// NamedDataSources preserves widgets that expose more than one datasource.
	// A single datasource continues to use DataSource and its friendly MDL
	// keyword; multiple sources must retain their schema property keys or a
	// describe -> exec round trip would fan one source out over every mapping.
	NamedDataSources []rawNamedDataSource
	DataGridColumns  []rawDataGridColumn // For DataGrid2 widgets
	// Input widget properties
	Editable      string // "Always", "Never", "Conditional"
	ReadOnlyStyle string // "Inherit", "Control", "Text"
	ShowLabel     bool   // Whether label is shown (from LabelTemplate visibility)
	LabelPosition string // "Left", "Top", etc.
	Placeholder   string // Placeholder hint text (from PlaceholderTemplate)
	OnChange      string // MDL rendering of the OnChangeAction client action
	OnClick       string // MDL rendering of a pluggable widget's onClick action (e.g. DataGrid2)
	// Filter widget properties
	FilterAttributes []string // Attributes to filter on
	FilterExpression string   // Default filter expression (contains, startsWith, etc.)
	// Paging properties (DataGrid2)
	PageSize          string // e.g. "20", "50"
	Pagination        string // "buttons", "virtualScrolling", "loadMore"
	PagingPosition    string // "bottom", "top", "both"
	ShowPagingButtons string // "always", "auto"
	// Gallery column properties
	DesktopColumns string // e.g. "9", "4"
	TabletColumns  string // e.g. "4", "2"
	PhoneColumns   string // e.g. "2", "1"
	// ComboBox association mode properties
	CaptionAttribute string // Display attribute for association-mode ComboBox
	// GroupBox properties
	Collapsible string // "No", "YesInitiallyExpanded", "YesInitiallyCollapsed"
	HeaderMode  string // "Div", "H1"-"H6"
	// TabPage property (only set on synthetic rawWidget wrappers emitted by
	// TabControl parsing — preserves the original tab page name/caption so
	// DESCRIBE output shows which tab each nested widget belongs to).
	TabCaption string
	// Scroll-container region properties, set only on the synthetic wrappers
	// the region walk emits (the slot name goes in Name). Same shape as
	// TabCaption: a nameless container whose identity DESCRIBE has to put back.
	RegionSize     int
	RegionSizeMode string
	// NavigationProfile is a Forms$NavigationTree's profile, which the document
	// keeps one level down in MenuSource rather than on the tree.
	NavigationProfile string
	// Specialization is the entity a List View template renders. Set only on the
	// synthetic wrappers parseListViewContent emits for Forms$ListViewTemplate,
	// which is the same shape as TabCaption above: a container with no name, whose
	// identity is a single field DESCRIBE has to put back.
	Specialization string
	// Conditional visibility/editability
	VisibleIf  string // Expression from ConditionalVisibilitySettings
	EditableIf string // Expression from ConditionalEditabilitySettings
	// Design properties from Appearance
	DesignProperties []rawDesignProp
	// Explicit widget properties (for generic PLUGGABLEWIDGET output)
	ExplicitProperties []rawExplicitProp
	// NamedActions holds the widget's action slots that MDL addresses by the
	// widget's own property key — every slot but the click and change ones,
	// which have MDL names and are carried in OnClick/OnChange (#956).
	NamedActions []rawNamedAction
	// Object-list child blocks (for generic PLUGGABLEWIDGET output): chart series,
	// lines, scale colors, etc. Reconstructed from the widget's WidgetObject lists.
	ObjectLists []rawObjectList

	// ChildSlots are the widget's reconstructed child slots — fixed properties
	// holding widgets, as opposed to ObjectLists' repeated items.
	ChildSlots []rawChildSlot

	// OmittedContainers names container-shaped properties present in the stored
	// document that DESCRIBE could not reproduce. Emitted as a comment so a
	// describe -> exec round trip cannot silently delete a widget's body.
	// See unreconstructedContainers.
	OmittedContainers []string
	// Data container context: entity qualified name provided by this container
	EntityContext string
	// Full widget ID (e.g. "com.mendix.widget.custom.switch.Switch")
	WidgetID string
	// DataView-only: LabelWidth read from BSON (-1 = not set, 0..12 = explicit)
	LabelWidth int
	// DataView-only: ShowFooter read from BSON. Round-tripping it matters only when
	// the footer block would not imply the same value (#813).
	ShowFooter bool
	// Pluggable Image widget properties
	ImageUrl        string // Image URL (from textTemplate)
	AlternativeText string // Alt text (from textTemplate)
	ImageWidth      string // Width in pixels/percentage
	ImageHeight     string // Height in pixels/percentage
	WidthUnit       string // "auto", "pixels", "percentage"
	HeightUnit      string // "auto", "pixels", "percentage", "viewport"
	DisplayAs       string // "fullImage", "thumbnail"
	Responsive      string // "true", "false"
	ImageType       string // "image", "imageUrl", "icon"
	// ImageObject is the image collection entry the widget shows, as the
	// three-part qualified name Module.Collection.Image. Empty when the source
	// is not an image collection, or when none is selected. Without it a
	// describe -> exec copy loses the image (mxcli-formula1 FINDINGS §142).
	ImageObject string
	OnClickType string // "action", "enlarge"
}

// rawExplicitProp represents a non-default property extracted from a CustomWidget.
// rawNamedAction is one named action slot read back off a stored widget: the
// widget's own property key, and the action rendered as MDL.
type rawNamedAction struct {
	Key string
	MDL string
}

type rawNamedDataSource struct {
	Key        string
	DataSource *rawDataSource
}

type rawExplicitProp struct {
	Key   string
	Value string // attribute short name or primitive value
	IsRef bool   // true if this is an attribute reference, false for primitive
	// ValueType is the property's DECLARED type from the widget package
	// ("String", "Boolean", "Integer", "Enumeration", ...), and is what decides
	// whether the emitted value is quoted. The value's own shape cannot: a
	// String property holding "30" or "true" still has to come back quoted.
	// Empty when the widget's schema is not in the document (ledger #104).
	ValueType string
}

// rawDesignProp represents a parsed design property from BSON.
type rawDesignProp struct {
	Key       string          // Design property key, e.g., "Spacing top"
	ValueType string          // "toggle", "option", or "compound"
	Option    string          // For "option" type: the selected option value
	Nested    []rawDesignProp // For "compound" type: sub-properties (e.g. Spacing → margin-top/bottom)
}

type rawWidgetRow struct {
	Columns []rawWidgetColumn
}

type rawWidgetColumn struct {
	Width       int
	TabletWidth int
	PhoneWidth  int
	Widgets     []rawWidget
}

// toBsonArray converts various BSON array types to []interface{}.
func toBsonArray(v any) []any {
	switch arr := v.(type) {
	case []any:
		return arr
	case primitive.A:
		// primitive.A is already []interface{} under the hood
		return []any(arr)
	default:
		return nil
	}
}

// getBsonArrayElements extracts array elements from BSON array format.
// BSON arrays have format [typeIndicator, item1, item2, ...] where typeIndicator is a number.
func getBsonArrayElements(v any) []any {
	arr := toBsonArray(v)
	if len(arr) == 0 {
		return nil
	}
	// Check if first element is a type indicator (integer)
	if _, ok := arr[0].(int32); ok {
		return arr[1:]
	}
	if _, ok := arr[0].(int); ok {
		return arr[1:]
	}
	// No type indicator, return as-is
	return arr
}

// getPageWidgetsFromRaw extracts widgets from raw page BSON.
func getPageWidgetsFromRaw(ctx *ExecContext, pageID model.ID) []rawWidget {
	// Get raw page data
	rawData, err := ctx.Backend.GetRawUnit(pageID)
	if err != nil {
		return nil
	}

	// Parse FormCall.Arguments to get widgets
	formCall, ok := rawData["FormCall"].(map[string]any)
	if !ok {
		return nil
	}

	// Handle both []interface{} and primitive.A types
	args := getBsonArrayElements(formCall["Arguments"])
	if args == nil {
		return nil
	}

	var widgets []rawWidget
	for _, arg := range args {
		argMap, ok := arg.(map[string]any)
		if !ok {
			continue
		}
		argWidgets := getBsonArrayElements(argMap["Widgets"])
		for _, w := range argWidgets {
			if wMap, ok := w.(map[string]any); ok {
				parsed := parseRawWidget(ctx, wMap)
				for _, pw := range parsed {
					// Unwrap the conditionalVisibilityWidget wrapper that
					// mxcli (and Studio Pro) adds as a layout placeholder
					// container. Without this, DESCRIBE PAGE shows a phantom
					// CONTAINER wrapping all widgets.
					if isConditionalVisibilityWrapper(pw) {
						widgets = append(widgets, pw.Children...)
					} else {
						widgets = append(widgets, pw)
					}
				}
			}
		}
	}
	return widgets
}

// pageWidgetGroup is one layout placeholder's widgets (issue #532).
type pageWidgetGroup struct {
	Placeholder string // placeholder name (e.g. "Main", "Topbar")
	Widgets     []rawWidget
}

// getPageWidgetGroupsFromRaw returns the page's widgets grouped by layout
// placeholder (one group per FormCallArgument), preserving order. Used by
// DESCRIBE to emit `placeholder <Name> { … }` blocks for multi-placeholder
// pages (issue #532) instead of flattening everything into Main.
func getPageWidgetGroupsFromRaw(ctx *ExecContext, pageID model.ID) []pageWidgetGroup {
	rawData, err := ctx.Backend.GetRawUnit(pageID)
	if err != nil {
		return nil
	}
	formCall, ok := rawData["FormCall"].(map[string]any)
	if !ok {
		return nil
	}
	layoutName := extractString(formCall["Form"]) // e.g. "Atlas_Core.Atlas_SideBar"
	args := getBsonArrayElements(formCall["Arguments"])
	if args == nil {
		return nil
	}

	var groups []pageWidgetGroup
	for _, arg := range args {
		argMap, ok := arg.(map[string]any)
		if !ok {
			continue
		}
		// Parameter is a BY_NAME ref "<Layout>.<Placeholder>"; strip the layout
		// prefix (fall back to the last dot-segment) to get the placeholder name.
		ph := extractString(argMap["Parameter"])
		if layoutName != "" && strings.HasPrefix(ph, layoutName+".") {
			ph = ph[len(layoutName)+1:]
		} else if i := strings.LastIndex(ph, "."); i >= 0 {
			ph = ph[i+1:]
		}
		var widgets []rawWidget
		for _, w := range getBsonArrayElements(argMap["Widgets"]) {
			wMap, ok := w.(map[string]any)
			if !ok {
				continue
			}
			for _, pw := range parseRawWidget(ctx, wMap) {
				if isConditionalVisibilityWrapper(pw) {
					widgets = append(widgets, pw.Children...)
				} else {
					widgets = append(widgets, pw)
				}
			}
		}
		groups = append(groups, pageWidgetGroup{Placeholder: ph, Widgets: widgets})
	}
	return groups
}

// isConditionalVisibilityWrapper returns true if the widget is a DivContainer
// named "conditionalVisibilityWidget*" — the transparent wrapper that layouts
// use to hold placeholder content. We unwrap it so DESCRIBE output is clean
// and round-trippable without phantom CONTAINER nesting.
func isConditionalVisibilityWrapper(w rawWidget) bool {
	if w.Type != "Forms$DivContainer" && w.Type != "Pages$DivContainer" {
		return false
	}
	return strings.HasPrefix(w.Name, "conditionalVisibilityWidget") &&
		w.Class == "" && w.Style == "" && len(w.DesignProperties) == 0
}

// extractBinaryID extracts a UUID string from a BSON binary or string value.
// GUIDs use little-endian byte order for the first three groups:
// - First 4 bytes (group 1): little-endian
// - Next 2 bytes (group 2): little-endian
// - Next 2 bytes (group 3): little-endian
// - Last 8 bytes: big-endian
func extractBinaryID(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return formatGUID(val)
	case primitive.Binary:
		return formatGUID(val.Data)
	default:
		return ""
	}
}

// formatGUID converts a 16-byte GUID to its string representation with proper byte ordering.
func formatGUID(data []byte) string {
	if len(data) != 16 {
		return string(data)
	}
	// Reverse first 4 bytes (group 1)
	g1 := []byte{data[3], data[2], data[1], data[0]}
	// Reverse next 2 bytes (group 2)
	g2 := []byte{data[5], data[4]}
	// Reverse next 2 bytes (group 3)
	g3 := []byte{data[7], data[6]}
	// Last 8 bytes stay in order
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		g1, g2, g3, data[8:10], data[10:16])
}

// wrapStringLiteralExpression wraps a string value in single quotes for Mendix expression format.
// If the value already looks like an expression (starts with $ for variable, or contains operators),
// it is returned as-is.
func wrapStringLiteralExpression(value string) string {
	// If it's already quoted, return as-is
	if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		return value
	}
	// If it's a variable reference, return as-is
	if strings.HasPrefix(value, "$") {
		return value
	}
	// If it looks like a number, return as-is
	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return value
	}
	// If it's a boolean, return as-is
	if value == "true" || value == "false" {
		return value
	}
	// Otherwise wrap in single quotes as a string literal
	return "'" + value + "'"
}

// pageParamTypeMDL returns the MDL type string for a page parameter.
// Primitive params return "String", "Integer", etc.; entity params return the qualified name.
func pageParamTypeMDL(p *pages.PageParameter) string {
	if p.TypeName != "" {
		switch p.TypeName {
		case "DataTypes$StringType":
			return "String"
		case "DataTypes$IntegerType":
			return "Integer"
		case "DataTypes$LongType":
			return "Long"
		case "DataTypes$DecimalType":
			return "Decimal"
		case "DataTypes$BooleanType":
			return "Boolean"
		case "DataTypes$DateTimeType":
			return "DateTime"
		default:
			return p.TypeName
		}
	}
	if p.EntityName != "" {
		return p.EntityName
	}
	return string(p.EntityID)
}
