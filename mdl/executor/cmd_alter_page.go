// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/mpr"
)

// execAlterPage handles ALTER PAGE/SNIPPET Module.Name { operations }.
func execAlterPage(ctx *ExecContext, s *ast.AlterPageStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	var unitID model.ID
	var containerID model.ID
	containerType := s.ContainerType
	if containerType == "" {
		containerType = "PAGE"
	}

	if containerType == "SNIPPET" {
		snippet, modID, err := findSnippetByName(ctx, s.PageName, h)
		if err != nil {
			return err
		}
		unitID = snippet.ID
		containerID = modID
	} else {
		page, err := findPageByName(ctx, s.PageName, h)
		if err != nil {
			return err
		}
		unitID = page.ID
		containerID = h.FindModuleID(page.ContainerID)
	}

	// Load raw BSON as ordered document (bson.D preserves field ordering,
	// which is required by Mendix Studio Pro).
	rawBytes, err := ctx.Backend.GetRawUnitBytes(unitID)
	if err != nil {
		return mdlerrors.NewBackend("load raw "+strings.ToLower(containerType)+" data", err)
	}
	var rawData bson.D
	if err := bson.Unmarshal(rawBytes, &rawData); err != nil {
		return mdlerrors.NewBackend("unmarshal "+strings.ToLower(containerType)+" BSON", err)
	}

	// Resolve module name for building new widgets
	modName := h.GetModuleName(containerID)

	// Apply operations sequentially using the appropriate BSON finder
	findWidget := findBsonWidget // page default
	if containerType == "SNIPPET" {
		findWidget = findBsonWidgetInSnippet
	}

	for _, op := range s.Operations {
		switch o := op.(type) {
		case *ast.SetPropertyOp:
			if err := applySetPropertyWith(rawData, o, findWidget); err != nil {
				return mdlerrors.NewBackend("SET", err)
			}
		case *ast.InsertWidgetOp:
			if err := applyInsertWidgetWith(ctx, rawData, o, modName, containerID, findWidget); err != nil {
				return mdlerrors.NewBackend("INSERT", err)
			}
		case *ast.DropWidgetOp:
			if err := applyDropWidgetWith(rawData, o, findWidget); err != nil {
				return mdlerrors.NewBackend("DROP", err)
			}
		case *ast.ReplaceWidgetOp:
			if err := applyReplaceWidgetWith(ctx, rawData, o, modName, containerID, findWidget); err != nil {
				return mdlerrors.NewBackend("REPLACE", err)
			}
		case *ast.AddVariableOp:
			if err := applyAddVariable(&rawData, o); err != nil {
				return mdlerrors.NewBackend("ADD VARIABLE", err)
			}
		case *ast.DropVariableOp:
			if err := applyDropVariable(rawData, o); err != nil {
				return mdlerrors.NewBackend("DROP VARIABLE", err)
			}
		case *ast.SetLayoutOp:
			if containerType == "SNIPPET" {
				return mdlerrors.NewUnsupported("SET Layout is not supported for snippets")
			}
			if err := applySetLayout(rawData, o); err != nil {
				return mdlerrors.NewBackend("SET Layout", err)
			}
		default:
			return mdlerrors.NewUnsupported(fmt.Sprintf("unknown ALTER %s operation type: %T", containerType, op))
		}
	}

	// Marshal back to BSON bytes (bson.D preserves field ordering)
	outBytes, err := bson.Marshal(rawData)
	if err != nil {
		return mdlerrors.NewBackend("marshal modified "+strings.ToLower(containerType), err)
	}

	// Save
	if err := ctx.Backend.UpdateRawUnit(string(unitID), outBytes); err != nil {
		return mdlerrors.NewBackend("save modified "+strings.ToLower(containerType), err)
	}

	fmt.Fprintf(ctx.Output, "Altered %s %s\n", strings.ToLower(containerType), s.PageName.String())
	return nil
}

// applySetLayout rewrites the FormCall to reference a new layout.
// It updates the Form field and remaps Parameter strings in each FormCallArgument.
func applySetLayout(rawData bson.D, op *ast.SetLayoutOp) error {
	newLayoutQN := op.NewLayout.Module + "." + op.NewLayout.Name

	// Find FormCall in the page BSON
	var formCall bson.D
	for _, elem := range rawData {
		if elem.Key == "FormCall" {
			if doc, ok := elem.Value.(bson.D); ok {
				formCall = doc
			}
			break
		}
	}
	if formCall == nil {
		return mdlerrors.NewValidation("page has no FormCall (layout reference)")
	}

	// Detect the old layout name from existing Parameter values
	oldLayoutQN := ""
	for _, elem := range formCall {
		if elem.Key == "Form" {
			if s, ok := elem.Value.(string); ok && s != "" {
				oldLayoutQN = s
			}
		}
		if elem.Key == "Arguments" {
			if arr, ok := elem.Value.(bson.A); ok {
				for _, item := range arr {
					if doc, ok := item.(bson.D); ok {
						for _, field := range doc {
							if field.Key == "Parameter" {
								if s, ok := field.Value.(string); ok && oldLayoutQN == "" {
									// Extract layout QN from "Atlas_Core.Atlas_TopBar.Main"
									if lastDot := strings.LastIndex(s, "."); lastDot > 0 {
										oldLayoutQN = s[:lastDot]
									}
								}
							}
						}
					}
				}
			}
		}
	}

	if oldLayoutQN == "" {
		return mdlerrors.NewValidation("cannot determine current layout from FormCall")
	}

	if oldLayoutQN == newLayoutQN {
		return nil // Already using the target layout
	}

	// Update Form field
	for i, elem := range formCall {
		if elem.Key == "Form" {
			formCall[i].Value = newLayoutQN
		}
	}

	// If Form field doesn't exist, add it
	hasForm := false
	for _, elem := range formCall {
		if elem.Key == "Form" {
			hasForm = true
			break
		}
	}
	if !hasForm {
		// Insert before Arguments
		for i, elem := range formCall {
			if elem.Key == "Arguments" {
				formCall = append(formCall[:i+1], formCall[i:]...)
				formCall[i] = bson.E{Key: "Form", Value: newLayoutQN}
				break
			}
		}
	}

	// Remap Parameter strings in each FormCallArgument
	for _, elem := range formCall {
		if elem.Key != "Arguments" {
			continue
		}
		arr, ok := elem.Value.(bson.A)
		if !ok {
			continue
		}
		for _, item := range arr {
			doc, ok := item.(bson.D)
			if !ok {
				continue
			}
			for j, field := range doc {
				if field.Key != "Parameter" {
					continue
				}
				paramStr, ok := field.Value.(string)
				if !ok {
					continue
				}
				// Extract placeholder name: "Atlas_Core.Atlas_Default.Main" -> "Main"
				placeholder := paramStr
				if strings.HasPrefix(paramStr, oldLayoutQN+".") {
					placeholder = paramStr[len(oldLayoutQN)+1:]
				}

				// Apply explicit mapping if provided
				if op.Mappings != nil {
					if mapped, ok := op.Mappings[placeholder]; ok {
						placeholder = mapped
					}
				}

				// Write new parameter value
				doc[j].Value = newLayoutQN + "." + placeholder
			}
		}
	}

	// Write FormCall back into rawData
	for i, elem := range rawData {
		if elem.Key == "FormCall" {
			rawData[i].Value = formCall
			break
		}
	}

	return nil
}

// ============================================================================
// bson.D helper functions for ordered document access
// ============================================================================

// dGet returns the value for a key in a bson.D, or nil if not found.
func dGet(doc bson.D, key string) any {
	for _, elem := range doc {
		if elem.Key == key {
			return elem.Value
		}
	}
	return nil
}

// dGetDoc returns a nested bson.D field value, or nil.
func dGetDoc(doc bson.D, key string) bson.D {
	v := dGet(doc, key)
	if d, ok := v.(bson.D); ok {
		return d
	}
	return nil
}

// dGetString returns a string field value, or "".
func dGetString(doc bson.D, key string) string {
	v := dGet(doc, key)
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// dSet sets a field value in a bson.D in place. If the key exists, it's updated
// and returns true. If the key is not found, returns false.
func dSet(doc bson.D, key string, value any) bool {
	for i := range doc {
		if doc[i].Key == key {
			doc[i].Value = value
			return true
		}
	}
	return false
}

// dGetArrayElements extracts Mendix array elements from a bson.D field value.
// Handles the int32 type marker at index 0. Works with bson.A and []any.
func dGetArrayElements(val any) []any {
	arr := toBsonA(val)
	if len(arr) == 0 {
		return nil
	}
	// Skip type marker (int32) at index 0
	if _, ok := arr[0].(int32); ok {
		return arr[1:]
	}
	if _, ok := arr[0].(int); ok {
		return arr[1:]
	}
	return arr
}

// toBsonA converts various BSON array types to []any.
func toBsonA(v any) []any {
	switch arr := v.(type) {
	case bson.A:
		return []any(arr)
	case []any:
		return arr
	default:
		return nil
	}
}

// dSetArray sets a Mendix-style BSON array field, preserving the int32 marker.
func dSetArray(doc bson.D, key string, elements []any) {
	existing := toBsonA(dGet(doc, key))
	var marker any
	if len(existing) > 0 {
		if _, ok := existing[0].(int32); ok {
			marker = existing[0]
		} else if _, ok := existing[0].(int); ok {
			marker = existing[0]
		}
	}
	var result bson.A
	if marker != nil {
		result = make(bson.A, 0, len(elements)+1)
		result = append(result, marker)
		result = append(result, elements...)
	} else {
		result = make(bson.A, len(elements))
		copy(result, elements)
	}
	dSet(doc, key, result)
}

// extractBinaryIDFromDoc extracts a binary ID string from a bson.D field.
func extractBinaryIDFromDoc(val any) string {
	if bin, ok := val.(primitive.Binary); ok {
		return types.BlobToUUID(bin.Data)
	}
	return ""
}

// ============================================================================
// BSON widget tree walking
// ============================================================================

// bsonWidgetResult holds a found widget and its parent context.
type bsonWidgetResult struct {
	widget      bson.D            // the widget document itself
	parentArr   []any             // the parent array elements (without marker)
	parentKey   string            // key in the parent doc that holds this array
	parentDoc   bson.D            // the doc containing parentKey
	index       int               // index in parentArr
	colPropKeys map[string]string // column property TypePointer → key map (only set for column results)
}

// widgetFinder is a function type for locating widgets in a raw BSON tree.
type widgetFinder func(rawData bson.D, widgetName string) *bsonWidgetResult

// findBsonWidget searches the raw BSON page tree for a widget by name.
// Page format: FormCall.Arguments[].Widgets[]
func findBsonWidget(rawData bson.D, widgetName string) *bsonWidgetResult {
	formCall := dGetDoc(rawData, "FormCall")
	if formCall == nil {
		return nil
	}

	args := dGetArrayElements(dGet(formCall, "Arguments"))
	for _, arg := range args {
		argDoc, ok := arg.(bson.D)
		if !ok {
			continue
		}
		if result := findInWidgetArray(argDoc, "Widgets", widgetName); result != nil {
			return result
		}
	}
	return nil
}

// findBsonWidgetInSnippet searches the raw BSON snippet tree for a widget by name.
// Snippet format: Widgets[] (Studio Pro) or Widget.Widgets[] (mxcli).
func findBsonWidgetInSnippet(rawData bson.D, widgetName string) *bsonWidgetResult {
	// Studio Pro format: top-level "Widgets" array
	if result := findInWidgetArray(rawData, "Widgets", widgetName); result != nil {
		return result
	}
	// mxcli format: "Widget" (singular) container with "Widgets" inside
	if widgetContainer := dGetDoc(rawData, "Widget"); widgetContainer != nil {
		if result := findInWidgetArray(widgetContainer, "Widgets", widgetName); result != nil {
			return result
		}
	}
	return nil
}

// findInWidgetArray searches a widget array (by key in parentDoc) for a named widget.
func findInWidgetArray(parentDoc bson.D, key string, widgetName string) *bsonWidgetResult {
	elements := dGetArrayElements(dGet(parentDoc, key))
	for i, elem := range elements {
		wDoc, ok := elem.(bson.D)
		if !ok {
			continue
		}
		if dGetString(wDoc, "Name") == widgetName {
			return &bsonWidgetResult{
				widget:    wDoc,
				parentArr: elements,
				parentKey: key,
				parentDoc: parentDoc,
				index:     i,
			}
		}
		// Recurse into children
		if result := findInWidgetChildren(wDoc, widgetName); result != nil {
			return result
		}
	}
	return nil
}

// findInWidgetChildren recursively searches widget children for a named widget.
func findInWidgetChildren(wDoc bson.D, widgetName string) *bsonWidgetResult {
	typeName := dGetString(wDoc, "$Type")

	// Direct Widgets[] children (Container, DataView body, TabPage, GroupBox, etc.)
	if result := findInWidgetArray(wDoc, "Widgets", widgetName); result != nil {
		return result
	}

	// FooterWidgets[] (DataView footer)
	if result := findInWidgetArray(wDoc, "FooterWidgets", widgetName); result != nil {
		return result
	}

	// LayoutGrid: Rows[].Columns[].Widgets[]
	if strings.Contains(typeName, "LayoutGrid") {
		rows := dGetArrayElements(dGet(wDoc, "Rows"))
		for _, row := range rows {
			rowDoc, ok := row.(bson.D)
			if !ok {
				continue
			}
			cols := dGetArrayElements(dGet(rowDoc, "Columns"))
			for _, col := range cols {
				colDoc, ok := col.(bson.D)
				if !ok {
					continue
				}
				if result := findInWidgetArray(colDoc, "Widgets", widgetName); result != nil {
					return result
				}
			}
		}
	}

	// TabContainer: TabPages[].Widgets[]
	if result := findInTabPages(wDoc, widgetName); result != nil {
		return result
	}

	// ControlBar widgets
	if result := findInControlBar(wDoc, widgetName); result != nil {
		return result
	}

	// CustomWidget (pluggable): Object.Properties[].Value.Widgets[]
	if strings.Contains(typeName, "CustomWidget") {
		if obj := dGetDoc(wDoc, "Object"); obj != nil {
			props := dGetArrayElements(dGet(obj, "Properties"))
			for _, prop := range props {
				propDoc, ok := prop.(bson.D)
				if !ok {
					continue
				}
				if valDoc := dGetDoc(propDoc, "Value"); valDoc != nil {
					if result := findInWidgetArray(valDoc, "Widgets", widgetName); result != nil {
						return result
					}
				}
			}
		}
	}

	return nil
}

// findInTabPages searches TabPages[].Widgets[] for a named widget.
func findInTabPages(wDoc bson.D, widgetName string) *bsonWidgetResult {
	tabPages := dGetArrayElements(dGet(wDoc, "TabPages"))
	for _, tp := range tabPages {
		tpDoc, ok := tp.(bson.D)
		if !ok {
			continue
		}
		if result := findInWidgetArray(tpDoc, "Widgets", widgetName); result != nil {
			return result
		}
	}
	return nil
}

// findInControlBar searches ControlBarItems within a ControlBar for a named widget.
func findInControlBar(wDoc bson.D, widgetName string) *bsonWidgetResult {
	controlBar := dGetDoc(wDoc, "ControlBar")
	if controlBar == nil {
		return nil
	}
	return findInWidgetArray(controlBar, "Items", widgetName)
}

// ============================================================================
// DataGrid2 column finder
// ============================================================================

// findBsonColumn finds a column inside a DataGrid2 widget by derived name.
// It locates the grid widget first, then searches its columns Objects[] array.
// Returns a bsonWidgetResult where parentArr/parentDoc/parentKey point to the
// columns array, so INSERT/DROP/REPLACE work via standard array manipulation.
func findBsonColumn(rawData bson.D, gridName, columnName string, find widgetFinder) *bsonWidgetResult {
	// Find the DataGrid2 widget
	gridResult := find(rawData, gridName)
	if gridResult == nil {
		return nil
	}

	// Build grid-level PropertyTypeID -> key map
	gridPropKeyMap := buildPropKeyMap(gridResult.widget)

	// Navigate to the "columns" property's Value.Objects[]
	obj := dGetDoc(gridResult.widget, "Object")
	if obj == nil {
		return nil
	}

	props := dGetArrayElements(dGet(obj, "Properties"))
	for _, prop := range props {
		propDoc, ok := prop.(bson.D)
		if !ok {
			continue
		}
		typePointerID := extractBinaryIDFromDoc(dGet(propDoc, "TypePointer"))
		propKey := gridPropKeyMap[typePointerID]
		if propKey != "columns" {
			continue
		}

		valDoc := dGetDoc(propDoc, "Value")
		if valDoc == nil {
			return nil
		}

		// Build column-level PropertyTypeID -> key map for name derivation
		colPropKeyMap := buildColumnPropKeyMap(gridResult.widget, typePointerID)

		// Search columns by derived name
		columns := dGetArrayElements(dGet(valDoc, "Objects"))
		for i, colItem := range columns {
			colDoc, ok := colItem.(bson.D)
			if !ok {
				continue
			}
			derived := deriveColumnNameBson(colDoc, colPropKeyMap, i)
			if derived == columnName {
				return &bsonWidgetResult{
					widget:      colDoc,
					parentArr:   columns,
					parentKey:   "Objects",
					parentDoc:   valDoc,
					index:       i,
					colPropKeys: colPropKeyMap,
				}
			}
		}
		return nil // found columns property but no matching column
	}
	return nil
}

// buildPropKeyMap builds a TypePointer ID -> PropertyKey map from a widget's
// Type.ObjectType.PropertyTypes array.
func buildPropKeyMap(widgetDoc bson.D) map[string]string {
	m := make(map[string]string)
	widgetType := dGetDoc(widgetDoc, "Type")
	if widgetType == nil {
		return m
	}
	objType := dGetDoc(widgetType, "ObjectType")
	if objType == nil {
		return m
	}
	for _, pt := range dGetArrayElements(dGet(objType, "PropertyTypes")) {
		ptDoc, ok := pt.(bson.D)
		if !ok {
			continue
		}
		key := dGetString(ptDoc, "PropertyKey")
		id := extractBinaryIDFromDoc(dGet(ptDoc, "$ID"))
		if key != "" && id != "" {
			m[id] = key
		}
	}
	return m
}

// buildColumnPropKeyMap builds a TypePointer ID -> PropertyKey map for column
// properties. It navigates: Type.ObjectType.PropertyTypes["columns"].ValueType.ObjectType.PropertyTypes
func buildColumnPropKeyMap(widgetDoc bson.D, columnsTypePointerID string) map[string]string {
	m := make(map[string]string)
	widgetType := dGetDoc(widgetDoc, "Type")
	if widgetType == nil {
		return m
	}
	objType := dGetDoc(widgetType, "ObjectType")
	if objType == nil {
		return m
	}
	// Find the columns PropertyType entry
	for _, pt := range dGetArrayElements(dGet(objType, "PropertyTypes")) {
		ptDoc, ok := pt.(bson.D)
		if !ok {
			continue
		}
		id := extractBinaryIDFromDoc(dGet(ptDoc, "$ID"))
		if id != columnsTypePointerID {
			continue
		}
		// Navigate to ValueType.ObjectType.PropertyTypes
		valType := dGetDoc(ptDoc, "ValueType")
		if valType == nil {
			return m
		}
		colObjType := dGetDoc(valType, "ObjectType")
		if colObjType == nil {
			return m
		}
		for _, cpt := range dGetArrayElements(dGet(colObjType, "PropertyTypes")) {
			cptDoc, ok := cpt.(bson.D)
			if !ok {
				continue
			}
			key := dGetString(cptDoc, "PropertyKey")
			cid := extractBinaryIDFromDoc(dGet(cptDoc, "$ID"))
			if key != "" && cid != "" {
				m[cid] = key
			}
		}
		return m
	}
	return m
}

// deriveColumnNameBson derives a column name from its BSON WidgetObject,
// matching the logic in deriveColumnName() in cmd_pages_describe_output.go.
func deriveColumnNameBson(colDoc bson.D, propKeyMap map[string]string, index int) string {
	var attribute, caption string

	props := dGetArrayElements(dGet(colDoc, "Properties"))
	for _, prop := range props {
		propDoc, ok := prop.(bson.D)
		if !ok {
			continue
		}
		typePointerID := extractBinaryIDFromDoc(dGet(propDoc, "TypePointer"))
		propKey := propKeyMap[typePointerID]

		valDoc := dGetDoc(propDoc, "Value")
		if valDoc == nil {
			continue
		}

		switch propKey {
		case "attribute":
			// Extract attribute path from AttributeRef
			if attrRef := dGetString(valDoc, "AttributeRef"); attrRef != "" {
				attribute = attrRef
			} else if attrDoc := dGetDoc(valDoc, "AttributeRef"); attrDoc != nil {
				attribute = dGetString(attrDoc, "Attribute")
			}
		case "header":
			// Extract caption from TextTemplate
			if tmpl := dGetDoc(valDoc, "TextTemplate"); tmpl != nil {
				items := dGetArrayElements(dGet(tmpl, "Items"))
				for _, item := range items {
					if itemDoc, ok := item.(bson.D); ok {
						if text := dGetString(itemDoc, "Text"); text != "" {
							caption = text
						}
					}
				}
			}
		}
	}

	// Apply same derivation logic as deriveColumnName
	if attribute != "" {
		parts := strings.Split(attribute, ".")
		return parts[len(parts)-1]
	}
	if caption != "" {
		return sanitizeColumnName(caption)
	}
	return fmt.Sprintf("col%d", index+1)
}

// sanitizeColumnName converts a caption string into a valid column identifier.
func sanitizeColumnName(caption string) string {
	var result []rune
	for _, r := range caption {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			result = append(result, r)
		} else {
			result = append(result, '_')
		}
	}
	return string(result)
}

// columnPropertyAliases maps user-facing property names to internal column property keys.
var columnPropertyAliases = map[string]string{
	"Caption":       "header",
	"Attribute":     "attribute",
	"Visible":       "visible",
	"Alignment":     "alignment",
	"WrapText":      "wrapText",
	"Sortable":      "sortable",
	"Resizable":     "resizable",
	"Draggable":     "draggable",
	"Hidable":       "hidable",
	"ColumnWidth":   "width",
	"Size":          "size",
	"ShowContentAs": "showContentAs",
	"ColumnClass":   "columnClass",
	"Tooltip":       "tooltip",
}

// setColumnProperty sets a property on a DataGrid2 column WidgetObject.
// propKeyMap maps TypePointer IDs to property keys (from the parent grid's column type).
func setColumnProperty(colDoc bson.D, propKeyMap map[string]string, propName string, value interface{}) error {
	// Map user-facing name to internal property key
	internalKey := columnPropertyAliases[propName]
	if internalKey == "" {
		internalKey = propName
	}

	// Search column Properties[] for matching property and update
	props := dGetArrayElements(dGet(colDoc, "Properties"))
	for _, prop := range props {
		propDoc, ok := prop.(bson.D)
		if !ok {
			continue
		}
		typePointerID := extractBinaryIDFromDoc(dGet(propDoc, "TypePointer"))
		propKey := propKeyMap[typePointerID]
		if propKey != internalKey {
			continue
		}
		if valDoc := dGetDoc(propDoc, "Value"); valDoc != nil {
			strVal := fmt.Sprintf("%v", value)
			dSet(valDoc, "PrimitiveValue", strVal)
			return nil
		}
		return mdlerrors.NewValidation(fmt.Sprintf("column property %q has no Value", propName))
	}
	return mdlerrors.NewNotFound("column property", propName)
}

// ============================================================================
// SET property
// ============================================================================

// applySetProperty modifies widget properties in the raw BSON tree (page format).
func applySetProperty(rawData bson.D, op *ast.SetPropertyOp) error {
	return applySetPropertyWith(rawData, op, findBsonWidget)
}

// applySetPropertyWith modifies widget properties using the given widget finder.
func applySetPropertyWith(rawData bson.D, op *ast.SetPropertyOp, find widgetFinder) error {
	if op.Target.Widget == "" {
		// Page/snippet-level SET
		return applyPageLevelSet(rawData, op.Properties)
	}

	// Find the widget (or column via dotted ref)
	var result *bsonWidgetResult
	if op.Target.IsColumn() {
		result = findBsonColumn(rawData, op.Target.Widget, op.Target.Column, find)
	} else {
		result = find(rawData, op.Target.Widget)
	}
	if result == nil {
		return mdlerrors.NewNotFound("widget", op.Target.Name())
	}

	// Apply each property
	for propName, value := range op.Properties {
		if op.Target.IsColumn() {
			if err := setColumnProperty(result.widget, result.colPropKeys, propName, value); err != nil {
				return mdlerrors.NewBackend("set "+propName+" on "+op.Target.Name(), err)
			}
		} else {
			if err := setRawWidgetProperty(result.widget, propName, value); err != nil {
				return mdlerrors.NewBackend("set "+propName+" on "+op.Target.Name(), err)
			}
		}
	}
	return nil
}

// applyPageLevelSet handles page-level SET (e.g., SET Title = 'New Title').
func applyPageLevelSet(rawData bson.D, properties map[string]interface{}) error {
	for propName, value := range properties {
		switch propName {
		case "Title":
			// Title is stored as FormCall.Title or at the top level
			if formCall := dGetDoc(rawData, "FormCall"); formCall != nil {
				setTranslatableText(formCall, "Title", value)
			} else {
				setTranslatableText(rawData, "Title", value)
			}
		case "Url":
			// URL is stored as a plain string at the top level
			strVal, _ := value.(string)
			dSet(rawData, "Url", strVal)
		default:
			return mdlerrors.NewUnsupported("unsupported page-level property: " + propName)
		}
	}
	return nil
}

// setRawWidgetProperty sets a property on a raw BSON widget document.
func setRawWidgetProperty(widget bson.D, propName string, value interface{}) error {
	// Handle known standard BSON properties
	switch propName {
	case "Caption":
		return setWidgetCaption(widget, value)
	case "Content":
		return setWidgetContent(widget, value)
	case "Label":
		return setWidgetLabel(widget, value)
	case "ButtonStyle":
		if s, ok := value.(string); ok {
			dSet(widget, "ButtonStyle", s)
		}
		return nil
	case "Class":
		if appearance := dGetDoc(widget, "Appearance"); appearance != nil {
			if s, ok := value.(string); ok {
				dSet(appearance, "Class", s)
			}
		}
		return nil
	case "Style":
		if appearance := dGetDoc(widget, "Appearance"); appearance != nil {
			if s, ok := value.(string); ok {
				dSet(appearance, "Style", s)
			}
		}
		return nil
	case "Editable":
		if s, ok := value.(string); ok {
			dSet(widget, "Editable", s)
		}
		return nil
	case "Visible":
		if s, ok := value.(string); ok {
			dSet(widget, "Visible", s)
		} else if b, ok := value.(bool); ok {
			if b {
				dSet(widget, "Visible", "True")
			} else {
				dSet(widget, "Visible", "False")
			}
		}
		return nil
	case "Name":
		if s, ok := value.(string); ok {
			dSet(widget, "Name", s)
		}
		return nil
	case "Attribute":
		return setWidgetAttributeRef(widget, value)
	case "DataSource":
		return setWidgetDataSource(widget, value)
	default:
		// Try as pluggable widget property (quoted string property name)
		return setPluggableWidgetProperty(widget, propName, value)
	}
}

// setWidgetCaption sets the Caption property on a button or text widget.
func setWidgetCaption(widget bson.D, value interface{}) error {
	caption := dGetDoc(widget, "Caption")
	if caption == nil {
		// Try direct caption text
		setTranslatableText(widget, "Caption", value)
		return nil
	}
	setTranslatableText(caption, "", value)
	return nil
}

// setWidgetAttributeRef sets or updates the AttributeRef on an input widget.
// The value must be a fully qualified path (Module.Entity.Attribute, 2+ dots).
// If not fully qualified, AttributeRef is set to nil to avoid Studio Pro crash.
func setWidgetAttributeRef(widget bson.D, value interface{}) error {
	attrPath, ok := value.(string)
	if !ok {
		return mdlerrors.NewValidation("Attribute value must be a string")
	}

	// Build the new AttributeRef value
	var attrRefValue interface{}
	if strings.Count(attrPath, ".") >= 2 {
		attrRefValue = bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "DomainModels$AttributeRef"},
			{Key: "Attribute", Value: attrPath},
			{Key: "EntityRef", Value: nil},
		}
	} else {
		// Not fully qualified — clear the ref to avoid Mendix crash
		attrRefValue = nil
	}

	// Try to update existing AttributeRef field
	for i, elem := range widget {
		if elem.Key == "AttributeRef" {
			widget[i].Value = attrRefValue
			return nil
		}
	}

	// No existing AttributeRef field — this widget may not support it
	return mdlerrors.NewValidation("widget does not have an AttributeRef property; Attribute can only be SET on input widgets (TextBox, TextArea, DatePicker, etc.)")
}

// setWidgetDataSource sets the DataSource on a DataView or list widget.
func setWidgetDataSource(widget bson.D, value interface{}) error {
	ds, ok := value.(*ast.DataSourceV3)
	if !ok {
		return mdlerrors.NewValidation("DataSource value must be a datasource expression")
	}

	var serialized interface{}

	switch ds.Type {
	case "selection":
		// SELECTION widgetName → Forms$ListenTargetSource
		serialized = bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$ListenTargetSource"},
			{Key: "ListenTarget", Value: ds.Reference},
		}
	case "database":
		// DATABASE Entity → Forms$DataViewSource with entity ref
		var entityRef interface{}
		if ds.Reference != "" {
			entityRef = bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "DomainModels$DirectEntityRef"},
				{Key: "Entity", Value: ds.Reference},
			}
		}
		serialized = bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$DataViewSource"},
			{Key: "EntityRef", Value: entityRef},
			{Key: "ForceFullObjects", Value: false},
			{Key: "SourceVariable", Value: nil},
		}
	case "microflow":
		serialized = bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$MicroflowSource"},
			{Key: "MicroflowSettings", Value: bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Forms$MicroflowSettings"},
				{Key: "Asynchronous", Value: false},
				{Key: "ConfirmationInfo", Value: nil},
				{Key: "FormValidations", Value: "All"},
				{Key: "Microflow", Value: ds.Reference},
				{Key: "ParameterMappings", Value: bson.A{int32(3)}},
				{Key: "ProgressBar", Value: "None"},
				{Key: "ProgressMessage", Value: nil},
			}},
		}
	case "nanoflow":
		serialized = bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$NanoflowSource"},
			{Key: "NanoflowSettings", Value: bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Forms$NanoflowSettings"},
				{Key: "Nanoflow", Value: ds.Reference},
				{Key: "ParameterMappings", Value: bson.A{int32(3)}},
			}},
		}
	default:
		return mdlerrors.NewUnsupported("unsupported DataSource type for ALTER PAGE SET: " + ds.Type)
	}

	dSet(widget, "DataSource", serialized)
	return nil
}

// setWidgetLabel sets the Label.Caption text on input widgets.
func setWidgetLabel(widget bson.D, value interface{}) error {
	label := dGetDoc(widget, "Label")
	if label == nil {
		return nil
	}
	setTranslatableText(label, "Caption", value)
	return nil
}

// setWidgetContent sets the Content property on a DYNAMICTEXT widget.
// Content is stored as Forms$ClientTemplate → Template (Forms$Text) → Items[] → Translation{Text}.
// This mirrors extractTextContent which reads Content.Template.Items[].Text.
func setWidgetContent(widget bson.D, value interface{}) error {
	strVal, ok := value.(string)
	if !ok {
		return mdlerrors.NewValidation("Content value must be a string")
	}
	content := dGetDoc(widget, "Content")
	if content == nil {
		return mdlerrors.NewValidation("widget has no Content property")
	}
	template := dGetDoc(content, "Template")
	if template == nil {
		return mdlerrors.NewValidation("Content has no Template")
	}
	items := dGetArrayElements(dGet(template, "Items"))
	if len(items) > 0 {
		if itemDoc, ok := items[0].(bson.D); ok {
			dSet(itemDoc, "Text", strVal)
			return nil
		}
	}
	return mdlerrors.NewValidation("Content.Template has no Items with Text")
}

// setTranslatableText sets a translatable text value in BSON.
// If key is empty, modifies the doc directly; otherwise navigates to doc[key].
func setTranslatableText(parent bson.D, key string, value interface{}) {
	strVal, ok := value.(string)
	if !ok {
		return
	}

	target := parent
	if key != "" {
		if nested := dGetDoc(parent, key); nested != nil {
			target = nested
		} else {
			// Try to set directly
			dSet(parent, key, strVal)
			return
		}
	}

	// Navigate to Translations[].Text
	translations := dGetArrayElements(dGet(target, "Translations"))
	if len(translations) > 0 {
		if tDoc, ok := translations[0].(bson.D); ok {
			dSet(tDoc, "Text", strVal)
			return
		}
	}

	// Direct text value
	dSet(target, "Text", strVal)
}

// setPluggableWidgetProperty sets a property on a pluggable widget's Object.Properties[].
// Properties are identified by TypePointer referencing a PropertyType entry in the widget's
// Type.ObjectType.PropertyTypes array, NOT by a "Key" field on the property itself.
func setPluggableWidgetProperty(widget bson.D, propName string, value interface{}) error {
	obj := dGetDoc(widget, "Object")
	if obj == nil {
		return mdlerrors.NewNotFoundMsg("property", propName, fmt.Sprintf("property %q not found (widget has no pluggable Object)", propName))
	}

	// Build TypePointer ID -> PropertyKey map from Type.ObjectType.PropertyTypes
	propTypeKeyMap := make(map[string]string)
	if widgetType := dGetDoc(widget, "Type"); widgetType != nil {
		if objType := dGetDoc(widgetType, "ObjectType"); objType != nil {
			propTypes := dGetArrayElements(dGet(objType, "PropertyTypes"))
			for _, pt := range propTypes {
				ptDoc, ok := pt.(bson.D)
				if !ok {
					continue
				}
				key := dGetString(ptDoc, "PropertyKey")
				if key == "" {
					continue
				}
				id := extractBinaryIDFromDoc(dGet(ptDoc, "$ID"))
				if id != "" {
					propTypeKeyMap[id] = key
				}
			}
		}
	}

	props := dGetArrayElements(dGet(obj, "Properties"))
	for _, prop := range props {
		propDoc, ok := prop.(bson.D)
		if !ok {
			continue
		}
		// Resolve property key via TypePointer
		typePointerID := extractBinaryIDFromDoc(dGet(propDoc, "TypePointer"))
		propKey := propTypeKeyMap[typePointerID]
		if propKey != propName {
			continue
		}
		// Set the value
		if valDoc := dGetDoc(propDoc, "Value"); valDoc != nil {
			switch v := value.(type) {
			case string:
				dSet(valDoc, "PrimitiveValue", v)
			case bool:
				if v {
					dSet(valDoc, "PrimitiveValue", "yes")
				} else {
					dSet(valDoc, "PrimitiveValue", "no")
				}
			case int:
				dSet(valDoc, "PrimitiveValue", fmt.Sprintf("%d", v))
			case float64:
				dSet(valDoc, "PrimitiveValue", fmt.Sprintf("%g", v))
			default:
				dSet(valDoc, "PrimitiveValue", fmt.Sprintf("%v", v))
			}
			return nil
		}
		return mdlerrors.NewValidation(fmt.Sprintf("property %q has no Value map", propName))
	}
	return mdlerrors.NewNotFound("pluggable property", propName)
}

// ============================================================================
// INSERT widget
// ============================================================================

// applyInsertWidget inserts new widgets before or after a target widget (page format).
func applyInsertWidget(ctx *ExecContext, rawData bson.D, op *ast.InsertWidgetOp, moduleName string, moduleID model.ID) error {
	return applyInsertWidgetWith(ctx, rawData, op, moduleName, moduleID, findBsonWidget)
}

// applyInsertWidgetWith inserts new widgets using the given widget finder.
func applyInsertWidgetWith(ctx *ExecContext, rawData bson.D, op *ast.InsertWidgetOp, moduleName string, moduleID model.ID, find widgetFinder) error {
	var result *bsonWidgetResult
	if op.Target.IsColumn() {
		result = findBsonColumn(rawData, op.Target.Widget, op.Target.Column, find)
	} else {
		result = find(rawData, op.Target.Widget)
	}
	if result == nil {
		return mdlerrors.NewNotFound("widget", op.Target.Name())
	}

	// Check for duplicate widget names before building
	for _, w := range op.Widgets {
		if w.Name != "" && find(rawData, w.Name) != nil {
			return mdlerrors.NewAlreadyExistsMsg("widget", w.Name, fmt.Sprintf("duplicate widget name '%s': a widget with this name already exists on the page", w.Name))
		}
	}

	// Find entity context from enclosing DataView/DataGrid/ListView
	entityCtx := findEnclosingEntityContext(rawData, op.Target.Widget)

	// Build new widget BSON from AST (pass rawData for page param + widget scope resolution)
	newBsonWidgets, err := buildWidgetsBson(ctx, op.Widgets, moduleName, moduleID, entityCtx, rawData)
	if err != nil {
		return mdlerrors.NewBackend("build widgets", err)
	}

	// Calculate insertion index
	insertIdx := result.index
	if op.Position == "AFTER" {
		insertIdx = result.index + 1
	}

	// Insert into the parent array
	newArr := make([]any, 0, len(result.parentArr)+len(newBsonWidgets))
	newArr = append(newArr, result.parentArr[:insertIdx]...)
	newArr = append(newArr, newBsonWidgets...)
	newArr = append(newArr, result.parentArr[insertIdx:]...)

	// Update parent
	dSetArray(result.parentDoc, result.parentKey, newArr)

	return nil
}

// ============================================================================
// DROP widget
// ============================================================================

// applyDropWidget removes widgets from the raw BSON tree (page format).
func applyDropWidget(rawData bson.D, op *ast.DropWidgetOp) error {
	return applyDropWidgetWith(rawData, op, findBsonWidget)
}

// applyDropWidgetWith removes widgets using the given widget finder.
func applyDropWidgetWith(rawData bson.D, op *ast.DropWidgetOp, find widgetFinder) error {
	for _, target := range op.Targets {
		var result *bsonWidgetResult
		if target.IsColumn() {
			result = findBsonColumn(rawData, target.Widget, target.Column, find)
		} else {
			result = find(rawData, target.Widget)
		}
		if result == nil {
			return mdlerrors.NewNotFound("widget", target.Name())
		}

		// Remove from parent array
		newArr := make([]any, 0, len(result.parentArr)-1)
		newArr = append(newArr, result.parentArr[:result.index]...)
		newArr = append(newArr, result.parentArr[result.index+1:]...)

		// Update parent
		dSetArray(result.parentDoc, result.parentKey, newArr)
	}
	return nil
}

// ============================================================================
// REPLACE widget
// ============================================================================

// applyReplaceWidget replaces a widget with new widgets (page format).
func applyReplaceWidget(ctx *ExecContext, rawData bson.D, op *ast.ReplaceWidgetOp, moduleName string, moduleID model.ID) error {
	return applyReplaceWidgetWith(ctx, rawData, op, moduleName, moduleID, findBsonWidget)
}

// applyReplaceWidgetWith replaces a widget using the given widget finder.
func applyReplaceWidgetWith(ctx *ExecContext, rawData bson.D, op *ast.ReplaceWidgetOp, moduleName string, moduleID model.ID, find widgetFinder) error {
	var result *bsonWidgetResult
	if op.Target.IsColumn() {
		result = findBsonColumn(rawData, op.Target.Widget, op.Target.Column, find)
	} else {
		result = find(rawData, op.Target.Widget)
	}
	if result == nil {
		return mdlerrors.NewNotFound("widget", op.Target.Name())
	}

	// Check for duplicate widget names (skip the widget being replaced)
	for _, w := range op.NewWidgets {
		if w.Name != "" && w.Name != op.Target.Widget && find(rawData, w.Name) != nil {
			return mdlerrors.NewAlreadyExistsMsg("widget", w.Name, fmt.Sprintf("duplicate widget name '%s': a widget with this name already exists on the page", w.Name))
		}
	}

	// Find entity context from enclosing DataView/DataGrid/ListView
	entityCtx := findEnclosingEntityContext(rawData, op.Target.Widget)

	// Build new widget BSON from AST (pass rawData for page param + widget scope resolution)
	newBsonWidgets, err := buildWidgetsBson(ctx, op.NewWidgets, moduleName, moduleID, entityCtx, rawData)
	if err != nil {
		return mdlerrors.NewBackend("build replacement widgets", err)
	}

	// Replace: remove old widget, insert new ones at same position
	newArr := make([]any, 0, len(result.parentArr)-1+len(newBsonWidgets))
	newArr = append(newArr, result.parentArr[:result.index]...)
	newArr = append(newArr, newBsonWidgets...)
	newArr = append(newArr, result.parentArr[result.index+1:]...)

	// Update parent
	dSetArray(result.parentDoc, result.parentKey, newArr)

	return nil
}

// ============================================================================
// Entity context extraction from BSON tree
// ============================================================================

// findEnclosingEntityContext walks the raw BSON tree to find the DataView, DataGrid,
// ListView, or Gallery ancestor of a target widget and extracts the entity name.
// This is needed for INSERT/REPLACE operations so that input widget Binds can be
// resolved to fully qualified attribute paths.
func findEnclosingEntityContext(rawData bson.D, widgetName string) string {
	// Start from FormCall.Arguments[].Widgets[] (page format)
	if formCall := dGetDoc(rawData, "FormCall"); formCall != nil {
		args := dGetArrayElements(dGet(formCall, "Arguments"))
		for _, arg := range args {
			argDoc, ok := arg.(bson.D)
			if !ok {
				continue
			}
			if ctx := findEntityContextInWidgets(argDoc, "Widgets", widgetName, ""); ctx != "" {
				return ctx
			}
		}
	}
	// Snippet format: Widgets[] or Widget.Widgets[]
	if ctx := findEntityContextInWidgets(rawData, "Widgets", widgetName, ""); ctx != "" {
		return ctx
	}
	if widgetContainer := dGetDoc(rawData, "Widget"); widgetContainer != nil {
		if ctx := findEntityContextInWidgets(widgetContainer, "Widgets", widgetName, ""); ctx != "" {
			return ctx
		}
	}
	return ""
}

// findEntityContextInWidgets searches a widget array for the target widget,
// tracking entity context from DataView/DataGrid/ListView/Gallery ancestors.
func findEntityContextInWidgets(parentDoc bson.D, key string, widgetName string, currentEntity string) string {
	elements := dGetArrayElements(dGet(parentDoc, key))
	for _, elem := range elements {
		wDoc, ok := elem.(bson.D)
		if !ok {
			continue
		}
		if dGetString(wDoc, "Name") == widgetName {
			return currentEntity
		}
		// Update entity context if this is a data container
		entityCtx := currentEntity
		if ent := extractEntityFromDataSource(wDoc); ent != "" {
			entityCtx = ent
		}
		// Recurse into children
		if ctx := findEntityContextInChildren(wDoc, widgetName, entityCtx); ctx != "" {
			return ctx
		}
	}
	return ""
}

// findEntityContextInChildren recursively searches widget children for the target,
// tracking entity context. Mirrors the traversal logic of findInWidgetChildren.
func findEntityContextInChildren(wDoc bson.D, widgetName string, currentEntity string) string {
	typeName := dGetString(wDoc, "$Type")

	// Direct Widgets[] children
	if ctx := findEntityContextInWidgets(wDoc, "Widgets", widgetName, currentEntity); ctx != "" {
		return ctx
	}
	// FooterWidgets[]
	if ctx := findEntityContextInWidgets(wDoc, "FooterWidgets", widgetName, currentEntity); ctx != "" {
		return ctx
	}
	// LayoutGrid: Rows[].Columns[].Widgets[]
	if strings.Contains(typeName, "LayoutGrid") {
		rows := dGetArrayElements(dGet(wDoc, "Rows"))
		for _, row := range rows {
			rowDoc, ok := row.(bson.D)
			if !ok {
				continue
			}
			cols := dGetArrayElements(dGet(rowDoc, "Columns"))
			for _, col := range cols {
				colDoc, ok := col.(bson.D)
				if !ok {
					continue
				}
				if ctx := findEntityContextInWidgets(colDoc, "Widgets", widgetName, currentEntity); ctx != "" {
					return ctx
				}
			}
		}
	}
	// TabContainer: TabPages[].Widgets[]
	tabPages := dGetArrayElements(dGet(wDoc, "TabPages"))
	for _, tp := range tabPages {
		tpDoc, ok := tp.(bson.D)
		if !ok {
			continue
		}
		if ctx := findEntityContextInWidgets(tpDoc, "Widgets", widgetName, currentEntity); ctx != "" {
			return ctx
		}
	}
	// ControlBar
	if controlBar := dGetDoc(wDoc, "ControlBar"); controlBar != nil {
		if ctx := findEntityContextInWidgets(controlBar, "Items", widgetName, currentEntity); ctx != "" {
			return ctx
		}
	}
	// CustomWidget (pluggable): Object.Properties[].Value.Widgets[]
	if strings.Contains(typeName, "CustomWidget") {
		if obj := dGetDoc(wDoc, "Object"); obj != nil {
			props := dGetArrayElements(dGet(obj, "Properties"))
			for _, prop := range props {
				propDoc, ok := prop.(bson.D)
				if !ok {
					continue
				}
				if valDoc := dGetDoc(propDoc, "Value"); valDoc != nil {
					if ctx := findEntityContextInWidgets(valDoc, "Widgets", widgetName, currentEntity); ctx != "" {
						return ctx
					}
				}
			}
		}
	}
	return ""
}

// extractEntityFromDataSource extracts the entity qualified name from a widget's
// DataSource BSON. Handles DataView, DataGrid, ListView, and Gallery data sources.
func extractEntityFromDataSource(wDoc bson.D) string {
	ds := dGetDoc(wDoc, "DataSource")
	if ds == nil {
		return ""
	}
	// EntityRef.Entity contains the qualified name (e.g., "Module.Entity")
	if entityRef := dGetDoc(ds, "EntityRef"); entityRef != nil {
		if entity := dGetString(entityRef, "Entity"); entity != "" {
			return entity
		}
	}
	return ""
}

// ============================================================================
// ADD / DROP variable
// ============================================================================

// applyAddVariable adds a new LocalVariable to the raw BSON page/snippet.
func applyAddVariable(rawData *bson.D, op *ast.AddVariableOp) error {
	// Check for duplicate variable name
	existingVars := dGetArrayElements(dGet(*rawData, "Variables"))
	for _, ev := range existingVars {
		if evDoc, ok := ev.(bson.D); ok {
			if dGetString(evDoc, "Name") == op.Variable.Name {
				return mdlerrors.NewAlreadyExists("variable", "$"+op.Variable.Name)
			}
		}
	}

	// Build VariableType BSON
	varTypeID := types.GenerateID()
	bsonTypeName := mdlTypeToBsonType(op.Variable.DataType)
	varType := bson.D{
		{Key: "$ID", Value: bsonutil.IDToBsonBinary(varTypeID)},
		{Key: "$Type", Value: bsonTypeName},
	}
	if bsonTypeName == "DataTypes$ObjectType" {
		varType = append(varType, bson.E{Key: "Entity", Value: op.Variable.DataType})
	}

	// Build LocalVariable BSON document
	varID := types.GenerateID()
	varDoc := bson.D{
		{Key: "$ID", Value: bsonutil.IDToBsonBinary(varID)},
		{Key: "$Type", Value: "Forms$LocalVariable"},
		{Key: "DefaultValue", Value: op.Variable.DefaultValue},
		{Key: "Name", Value: op.Variable.Name},
		{Key: "VariableType", Value: varType},
	}

	// Append to existing Variables array, or create new field
	existing := toBsonA(dGet(*rawData, "Variables"))
	if existing != nil {
		elements := dGetArrayElements(dGet(*rawData, "Variables"))
		elements = append(elements, varDoc)
		dSetArray(*rawData, "Variables", elements)
	} else {
		// Field doesn't exist — append to the document
		*rawData = append(*rawData, bson.E{Key: "Variables", Value: bson.A{int32(3), varDoc}})
	}

	return nil
}

// applyDropVariable removes a LocalVariable from the raw BSON page/snippet.
func applyDropVariable(rawData bson.D, op *ast.DropVariableOp) error {
	elements := dGetArrayElements(dGet(rawData, "Variables"))
	if elements == nil {
		return mdlerrors.NewNotFound("variable", "$"+op.VariableName)
	}

	// Find and remove the variable
	found := false
	var kept []any
	for _, elem := range elements {
		if doc, ok := elem.(bson.D); ok {
			if dGetString(doc, "Name") == op.VariableName {
				found = true
				continue
			}
		}
		kept = append(kept, elem)
	}

	if !found {
		return mdlerrors.NewNotFound("variable", "$"+op.VariableName)
	}

	dSetArray(rawData, "Variables", kept)
	return nil
}

// ============================================================================
// Widget BSON building
// ============================================================================

// buildWidgetsBson converts AST widgets to ordered BSON documents.
// Returns bson.D elements (not map[string]any) to preserve field ordering.
// rawPageData is the full page/snippet BSON, used to extract page parameters
// and existing widget IDs for PARAMETER/SELECTION DataSource resolution.
func buildWidgetsBson(ctx *ExecContext, widgets []*ast.WidgetV3, moduleName string, moduleID model.ID, entityContext string, rawPageData bson.D) ([]any, error) {
	e := ctx.executor
	paramScope, paramEntityNames := extractPageParamsFromBSON(rawPageData)
	widgetScope := extractWidgetScopeFromBSON(rawPageData)

	pb := &pageBuilder{
		writer:           e.writer,
		reader:           e.reader,
		moduleID:         moduleID,
		moduleName:       moduleName,
		entityContext:    entityContext,
		widgetScope:      widgetScope,
		paramScope:       paramScope,
		paramEntityNames: paramEntityNames,
		execCache:        ctx.Cache,
		fragments:        ctx.Fragments,
		themeRegistry:    ctx.GetThemeRegistry(),
	}

	var result []any
	for _, w := range widgets {
		bsonD, err := pb.buildWidgetV3ToBSON(w)
		if err != nil {
			return nil, mdlerrors.NewBackend("build widget "+w.Name, err)
		}
		if bsonD == nil {
			continue
		}

		// Keep as bson.D (ordered document) - no conversion to map[string]any needed.
		// This preserves field ordering when marshaled back to BSON bytes.
		result = append(result, bsonD)
	}
	return result, nil
}

// extractPageParamsFromBSON extracts page/snippet parameter names and entity
// IDs from the raw BSON document. This enables ALTER PAGE REPLACE/INSERT
// operations to resolve PARAMETER DataSource references (e.g., DataSource: $Customer).
func extractPageParamsFromBSON(rawData bson.D) (map[string]model.ID, map[string]string) {
	paramScope := make(map[string]model.ID)
	paramEntityNames := make(map[string]string)
	if rawData == nil {
		return paramScope, paramEntityNames
	}

	params := dGetArrayElements(dGet(rawData, "Parameters"))
	for _, p := range params {
		pDoc, ok := p.(bson.D)
		if !ok {
			continue
		}
		name := dGetString(pDoc, "Name")
		if name == "" {
			continue
		}
		paramType := dGetDoc(pDoc, "ParameterType")
		if paramType == nil {
			continue
		}
		typeName := dGetString(paramType, "$Type")
		if typeName != "DataTypes$ObjectType" {
			continue
		}
		entityName := dGetString(paramType, "Entity")
		if entityName == "" {
			continue
		}
		idVal := dGet(pDoc, "$ID")
		paramID := model.ID(extractBinaryIDFromDoc(idVal))
		paramScope[name] = paramID
		paramEntityNames[name] = entityName
	}
	return paramScope, paramEntityNames
}

// extractWidgetScopeFromBSON walks the entire raw BSON widget tree and
// collects a map of widget name → widget ID. This enables ALTER PAGE INSERT
// operations to resolve SELECTION DataSource references to existing sibling widgets.
func extractWidgetScopeFromBSON(rawData bson.D) map[string]model.ID {
	scope := make(map[string]model.ID)
	if rawData == nil {
		return scope
	}
	// Page format: FormCall.Arguments[].Widgets[]
	if formCall := dGetDoc(rawData, "FormCall"); formCall != nil {
		args := dGetArrayElements(dGet(formCall, "Arguments"))
		for _, arg := range args {
			argDoc, ok := arg.(bson.D)
			if !ok {
				continue
			}
			collectWidgetScope(argDoc, "Widgets", scope)
		}
	}
	// Snippet format: Widgets[] or Widget.Widgets[]
	collectWidgetScope(rawData, "Widgets", scope)
	if widgetContainer := dGetDoc(rawData, "Widget"); widgetContainer != nil {
		collectWidgetScope(widgetContainer, "Widgets", scope)
	}
	return scope
}

// collectWidgetScope recursively walks a widget array and collects name→ID mappings.
func collectWidgetScope(parentDoc bson.D, key string, scope map[string]model.ID) {
	elements := dGetArrayElements(dGet(parentDoc, key))
	for _, elem := range elements {
		wDoc, ok := elem.(bson.D)
		if !ok {
			continue
		}
		name := dGetString(wDoc, "Name")
		if name != "" {
			idVal := dGet(wDoc, "$ID")
			if wID := extractBinaryIDFromDoc(idVal); wID != "" {
				scope[name] = model.ID(wID)
			}
		}
		// Also register entity context for widgets with DataSource
		// so SELECTION can resolve the entity type
		collectWidgetScopeInChildren(wDoc, scope)
	}
}

// collectWidgetScopeInChildren recursively walks widget children to collect scope.
func collectWidgetScopeInChildren(wDoc bson.D, scope map[string]model.ID) {
	typeName := dGetString(wDoc, "$Type")

	// Direct Widgets[]
	collectWidgetScope(wDoc, "Widgets", scope)
	// FooterWidgets[]
	collectWidgetScope(wDoc, "FooterWidgets", scope)
	// LayoutGrid: Rows[].Columns[].Widgets[]
	if strings.Contains(typeName, "LayoutGrid") {
		rows := dGetArrayElements(dGet(wDoc, "Rows"))
		for _, row := range rows {
			rowDoc, ok := row.(bson.D)
			if !ok {
				continue
			}
			cols := dGetArrayElements(dGet(rowDoc, "Columns"))
			for _, col := range cols {
				colDoc, ok := col.(bson.D)
				if !ok {
					continue
				}
				collectWidgetScope(colDoc, "Widgets", scope)
			}
		}
	}
	// TabContainer: TabPages[].Widgets[]
	tabPages := dGetArrayElements(dGet(wDoc, "TabPages"))
	for _, tp := range tabPages {
		tpDoc, ok := tp.(bson.D)
		if !ok {
			continue
		}
		collectWidgetScope(tpDoc, "Widgets", scope)
	}
	// ControlBar
	if controlBar := dGetDoc(wDoc, "ControlBar"); controlBar != nil {
		collectWidgetScope(controlBar, "Items", scope)
	}
	// CustomWidget (pluggable): Object.Properties[].Value.Widgets[]
	if strings.Contains(typeName, "CustomWidget") {
		if obj := dGetDoc(wDoc, "Object"); obj != nil {
			props := dGetArrayElements(dGet(obj, "Properties"))
			for _, prop := range props {
				propDoc, ok := prop.(bson.D)
				if !ok {
					continue
				}
				if valDoc := dGetDoc(propDoc, "Value"); valDoc != nil {
					collectWidgetScope(valDoc, "Widgets", scope)
				}
			}
		}
	}
}

// ============================================================================
// Helper: SerializeWidget is already available via mpr package
// ============================================================================

var _ = mpr.SerializeWidget // ensure import is used
