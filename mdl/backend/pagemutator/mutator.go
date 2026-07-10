// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"fmt"
	"math"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/bsonnav"
	"github.com/JordtenBulte-OLC/mxcli/mdl/bsonutil"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// Compile-time check.
var _ backend.PageMutator = (*Mutator)(nil)

// Deps abstracts the engine-specific operations the page mutator needs: child
// serialization (widget / client-action / pluggable-widget data-source → raw
// bson.D), DataGrid2 column construction, and persisting the mutated unit. The
// MPR backend wires these to its sdk/mpr serializers + writer; the modelsdk
// backend wires them to the codec. Everything else in the mutator is pure
// bson.D tree manipulation and is engine-agnostic.
type Deps interface {
	// SerializeWidget converts a semantic widget to its raw bson.D form.
	SerializeWidget(w pages.Widget) bson.D
	// SerializeClientAction converts a semantic client action to its raw bson.D form.
	SerializeClientAction(a pages.ClientAction) bson.D
	// SerializeCustomWidgetDataSource converts a pluggable-widget data source to raw bson.D.
	SerializeCustomWidgetDataSource(ds pages.DataSource) bson.D
	// BuildDataGrid2Column builds a DataGrid2 column object (raw bson.D) from a
	// column spec, given the column object's type ID and per-property type IDs.
	// Returns an error if the engine does not support DataGrid2 column ALTER (so
	// the op refuses loudly rather than writing a corrupt column).
	BuildDataGrid2Column(col *backend.DataGridColumnSpec, columnObjectTypeID string, columnPropertyIDs map[string]pages.PropertyTypeIDEntry) (bson.D, error)
	// SaveUnit writes the (re-marshaled) unit bytes back to storage.
	SaveUnit(unitID string, contents []byte) error
}

// Mutator is the engine-agnostic backend.PageMutator implementation. It operates
// on a raw (bson v1) document tree and delegates the few engine-specific steps to
// Deps. Both the MPR and modelsdk backends construct it via New().
type Mutator struct {
	rawData       bson.D
	containerType backend.ContainerKind // "page", "snippet", or "layout"
	unitID        model.ID
	deps          Deps
	widgetFinder  widgetFinder
}

// New constructs a Mutator over an already-decoded unit document. It derives the
// container kind from the $Type field and selects the matching widget finder.
// Loading the raw bytes (and decoding to bson.D) is the caller's responsibility,
// since that is engine-specific (reader access).
func New(rawData bson.D, unitID model.ID, deps Deps) *Mutator {
	typeName := bsonnav.DGetString(rawData, "$Type")
	containerType := backend.ContainerPage
	switch {
	case strings.Contains(typeName, "Snippet"):
		containerType = backend.ContainerSnippet
	case strings.Contains(typeName, "Layout"):
		containerType = backend.ContainerLayout
	}

	finder := findBsonWidget
	if containerType == backend.ContainerSnippet {
		finder = findBsonWidgetInSnippet
	}

	return &Mutator{
		rawData:       rawData,
		containerType: containerType,
		unitID:        unitID,
		deps:          deps,
		widgetFinder:  finder,
	}
}

// ---------------------------------------------------------------------------
// PageMutator interface implementation
// ---------------------------------------------------------------------------

func (m *Mutator) ContainerType() backend.ContainerKind { return m.containerType }

func (m *Mutator) SetWidgetProperty(widgetRef string, prop string, value any) error {
	if widgetRef == "" {
		// Page-level property
		newRaw, err := applyPageLevelSetMut(m.rawData, prop, value)
		if err != nil {
			return err
		}
		m.rawData = newRaw
		return nil
	}
	result := m.widgetFinder(m.rawData, widgetRef)
	if result == nil {
		return fmt.Errorf("widget %q not found", widgetRef)
	}
	// DataGrid2 columns are WidgetObjects, not form widgets — use the column setter.
	if len(result.colPropKeys) > 0 {
		return setColumnPropertyMut(result.widget, result.colPropKeys, prop, value)
	}
	return setRawWidgetPropertyMut(result.widget, prop, value)
}

func (m *Mutator) SetWidgetDataSource(widgetRef string, ds pages.DataSource) error {
	result := m.widgetFinder(m.rawData, widgetRef)
	if result == nil {
		return fmt.Errorf("widget %q not found", widgetRef)
	}
	serialized := serializeDataSourceBson(ds)
	if serialized == nil {
		return fmt.Errorf("unsupported DataSource type %T", ds)
	}
	bsonnav.DSet(result.widget, "DataSource", serialized)
	return nil
}

func (m *Mutator) SetColumnProperty(gridRef string, columnRef string, prop string, value any) error {
	result := findBsonColumn(m.rawData, gridRef, columnRef, m.widgetFinder)
	if result == nil {
		return fmt.Errorf("column %q on grid %q not found", columnRef, gridRef)
	}
	return setColumnPropertyMut(result.widget, result.colPropKeys, prop, value)
}

func (m *Mutator) SetDesignProperty(widgetRef, key, valueType, option string) error {
	widget, err := m.findStyleableWidget(widgetRef)
	if err != nil {
		return err
	}
	return setDesignPropertyMut(widget, key, valueType, option)
}

func (m *Mutator) RemoveDesignProperty(widgetRef, key string) error {
	widget, err := m.findStyleableWidget(widgetRef)
	if err != nil {
		return err
	}
	return removeDesignPropertyMut(widget, key)
}

func (m *Mutator) ClearDesignProperties(widgetRef string) error {
	widget, err := m.findStyleableWidget(widgetRef)
	if err != nil {
		return err
	}
	return clearDesignPropertiesMut(widget)
}

// findStyleableWidget locates a widget by name for design-property operations.
func (m *Mutator) findStyleableWidget(widgetRef string) (bson.D, error) {
	result := m.widgetFinder(m.rawData, widgetRef)
	if result == nil {
		return nil, fmt.Errorf("widget %q not found", widgetRef)
	}
	return result.widget, nil
}

func (m *Mutator) InsertWidget(widgetRef string, columnRef string, position backend.InsertPosition, widgets []pages.Widget) error {
	var result *bsonWidgetResult
	if columnRef != "" {
		result = findBsonColumn(m.rawData, widgetRef, columnRef, m.widgetFinder)
	} else {
		result = m.widgetFinder(m.rawData, widgetRef)
	}
	if result == nil {
		if columnRef != "" {
			return fmt.Errorf("column %q on widget %q not found", columnRef, widgetRef)
		}
		return fmt.Errorf("widget %q not found", widgetRef)
	}

	// Serialize widgets
	newBsonWidgets, err := m.serializeWidgets(widgets)
	if err != nil {
		return fmt.Errorf("serialize widgets: %w", err)
	}

	insertIdx := result.index
	if strings.EqualFold(string(position), "after") {
		insertIdx = result.index + 1
	}

	newArr := make([]any, 0, len(result.parentArr)+len(newBsonWidgets))
	newArr = append(newArr, result.parentArr[:insertIdx]...)
	newArr = append(newArr, newBsonWidgets...)
	newArr = append(newArr, result.parentArr[insertIdx:]...)

	bsonnav.DSetArray(result.parentDoc, result.parentKey, newArr)
	return nil
}

func (m *Mutator) DropWidget(refs []backend.WidgetRef) error {
	for _, ref := range refs {
		// Re-find widget each iteration because previous drops mutate the tree.
		var result *bsonWidgetResult
		if ref.IsColumn() {
			result = findBsonColumn(m.rawData, ref.Widget, ref.Column, m.widgetFinder)
		} else {
			result = m.widgetFinder(m.rawData, ref.Widget)
		}
		if result == nil {
			return fmt.Errorf("widget %q not found", ref.Name())
		}
		newArr := make([]any, 0, len(result.parentArr)-1)
		newArr = append(newArr, result.parentArr[:result.index]...)
		newArr = append(newArr, result.parentArr[result.index+1:]...)
		bsonnav.DSetArray(result.parentDoc, result.parentKey, newArr)
	}
	return nil
}

func (m *Mutator) ReplaceWidget(widgetRef string, columnRef string, widgets []pages.Widget) error {
	var result *bsonWidgetResult
	if columnRef != "" {
		result = findBsonColumn(m.rawData, widgetRef, columnRef, m.widgetFinder)
	} else {
		result = m.widgetFinder(m.rawData, widgetRef)
	}
	if result == nil {
		if columnRef != "" {
			return fmt.Errorf("column %q on widget %q not found", columnRef, widgetRef)
		}
		return fmt.Errorf("widget %q not found", widgetRef)
	}

	newBsonWidgets, err := m.serializeWidgets(widgets)
	if err != nil {
		return fmt.Errorf("serialize widgets: %w", err)
	}

	newArr := make([]any, 0, len(result.parentArr)-1+len(newBsonWidgets))
	newArr = append(newArr, result.parentArr[:result.index]...)
	newArr = append(newArr, newBsonWidgets...)
	newArr = append(newArr, result.parentArr[result.index+1:]...)

	bsonnav.DSetArray(result.parentDoc, result.parentKey, newArr)
	return nil
}

// InsertColumns inserts new DataGrid2 columns before/after an existing column.
// Columns are serialized as CustomWidgets$WidgetObject (not as form widgets).
func (m *Mutator) InsertColumns(gridRef, afterColumnRef string, position backend.InsertPosition, columns []*backend.DataGridColumnSpec) error {
	if afterColumnRef == "" {
		return fmt.Errorf("InsertColumns requires a column reference")
	}
	result := findBsonColumn(m.rawData, gridRef, afterColumnRef, m.widgetFinder)
	if result == nil {
		return fmt.Errorf("column %q on widget %q not found", afterColumnRef, gridRef)
	}
	gridResult := m.widgetFinder(m.rawData, gridRef)
	if gridResult == nil {
		return fmt.Errorf("widget %q not found", gridRef)
	}
	columnsTypePointerID := findColumnsPropertyTypePointer(gridResult.widget)
	if columnsTypePointerID == "" {
		return fmt.Errorf("widget %q is not a DataGrid2 (no columns property)", gridRef)
	}
	columnObjectTypeID, columnPropertyIDs := extractColumnPropertyIDs(gridResult.widget, columnsTypePointerID)
	if columnObjectTypeID == "" {
		return fmt.Errorf("could not extract column type schema from %q", gridRef)
	}
	var newBsonColumns []any
	for _, col := range columns {
		colBson, err := m.deps.BuildDataGrid2Column(col, columnObjectTypeID, columnPropertyIDs)
		if err != nil {
			return err
		}
		newBsonColumns = append(newBsonColumns, colBson)
	}
	insertIdx := result.index
	if strings.EqualFold(string(position), "after") {
		insertIdx = result.index + 1
	}
	newArr := make([]any, 0, len(result.parentArr)+len(newBsonColumns))
	newArr = append(newArr, result.parentArr[:insertIdx]...)
	newArr = append(newArr, newBsonColumns...)
	newArr = append(newArr, result.parentArr[insertIdx:]...)
	bsonnav.DSetArray(result.parentDoc, result.parentKey, newArr)
	return nil
}

// ReplaceColumn replaces a single DataGrid2 column with new columns.
// Columns are serialized as CustomWidgets$WidgetObject (not as form widgets).
func (m *Mutator) ReplaceColumn(gridRef, columnRef string, columns []*backend.DataGridColumnSpec) error {
	if columnRef == "" {
		return fmt.Errorf("ReplaceColumn requires a column reference")
	}
	result := findBsonColumn(m.rawData, gridRef, columnRef, m.widgetFinder)
	if result == nil {
		return fmt.Errorf("column %q on widget %q not found", columnRef, gridRef)
	}
	gridResult := m.widgetFinder(m.rawData, gridRef)
	if gridResult == nil {
		return fmt.Errorf("widget %q not found", gridRef)
	}
	columnsTypePointerID := findColumnsPropertyTypePointer(gridResult.widget)
	if columnsTypePointerID == "" {
		return fmt.Errorf("widget %q is not a DataGrid2 (no columns property)", gridRef)
	}
	columnObjectTypeID, columnPropertyIDs := extractColumnPropertyIDs(gridResult.widget, columnsTypePointerID)
	if columnObjectTypeID == "" {
		return fmt.Errorf("could not extract column type schema from %q", gridRef)
	}
	var newBsonColumns []any
	for _, col := range columns {
		colBson, err := m.deps.BuildDataGrid2Column(col, columnObjectTypeID, columnPropertyIDs)
		if err != nil {
			return err
		}
		newBsonColumns = append(newBsonColumns, colBson)
	}
	newArr := make([]any, 0, len(result.parentArr)-1+len(newBsonColumns))
	newArr = append(newArr, result.parentArr[:result.index]...)
	newArr = append(newArr, newBsonColumns...)
	newArr = append(newArr, result.parentArr[result.index+1:]...)
	bsonnav.DSetArray(result.parentDoc, result.parentKey, newArr)
	return nil
}

// findColumnsPropertyTypePointer locates the "columns" property's $ID in the
// widget's Type.ObjectType.PropertyTypes array. Returns "" if not found.
func findColumnsPropertyTypePointer(widgetDoc bson.D) string {
	widgetType := bsonnav.DGetDoc(widgetDoc, "Type")
	if widgetType == nil {
		return ""
	}
	objType := bsonnav.DGetDoc(widgetType, "ObjectType")
	if objType == nil {
		return ""
	}
	for _, pt := range bsonnav.DGetArrayElements(bsonnav.DGet(objType, "PropertyTypes")) {
		ptDoc, ok := pt.(bson.D)
		if !ok {
			continue
		}
		if bsonnav.DGetString(ptDoc, "PropertyKey") == "columns" {
			return bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(ptDoc, "$ID"))
		}
	}
	return ""
}

// extractColumnPropertyIDs walks an existing CustomWidget's Type tree and
// builds the pages.PropertyTypeIDEntry map for the column object type.
// Returns the columnObjectTypeID and the per-column-property map (forward
// direction; the reverse of buildColumnPropKeyMap).
func extractColumnPropertyIDs(widgetDoc bson.D, columnsTypePointerID string) (objectTypeID string, propIDs map[string]pages.PropertyTypeIDEntry) {
	propIDs = make(map[string]pages.PropertyTypeIDEntry)
	widgetType := bsonnav.DGetDoc(widgetDoc, "Type")
	if widgetType == nil {
		return
	}
	objType := bsonnav.DGetDoc(widgetType, "ObjectType")
	if objType == nil {
		return
	}
	for _, pt := range bsonnav.DGetArrayElements(bsonnav.DGet(objType, "PropertyTypes")) {
		ptDoc, ok := pt.(bson.D)
		if !ok || bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(ptDoc, "$ID")) != columnsTypePointerID {
			continue
		}
		valType := bsonnav.DGetDoc(ptDoc, "ValueType")
		if valType == nil {
			return
		}
		colObjType := bsonnav.DGetDoc(valType, "ObjectType")
		if colObjType == nil {
			return
		}
		objectTypeID = bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(colObjType, "$ID"))
		for _, cpt := range bsonnav.DGetArrayElements(bsonnav.DGet(colObjType, "PropertyTypes")) {
			cptDoc, ok := cpt.(bson.D)
			if !ok {
				continue
			}
			key := bsonnav.DGetString(cptDoc, "PropertyKey")
			cid := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(cptDoc, "$ID"))
			cvt := bsonnav.DGetDoc(cptDoc, "ValueType")
			var vid, vtype, defVal string
			if cvt != nil {
				vid = bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(cvt, "$ID"))
				// The discriminator is the inner "Type" field on the
				// CustomWidgets$WidgetValueType document, e.g. "Expression",
				// "TextTemplate", "Widgets", "Enumeration", "Boolean".
				vtype = bsonnav.DGetString(cvt, "Type")
				defVal = bsonnav.DGetString(cvt, "DefaultValue")
			}
			if key == "" || cid == "" {
				continue
			}
			propIDs[key] = pages.PropertyTypeIDEntry{
				PropertyTypeID: cid,
				ValueTypeID:    vid,
				ValueType:      vtype,
				DefaultValue:   defVal,
			}
		}
		return
	}
	return
}

func (m *Mutator) AddVariable(name, dataType, defaultValue string) error {
	// Check for duplicate variable name
	existingVars := bsonnav.DGetArrayElements(bsonnav.DGet(m.rawData, "Variables"))
	for _, ev := range existingVars {
		if evDoc, ok := ev.(bson.D); ok {
			if bsonnav.DGetString(evDoc, "Name") == name {
				return fmt.Errorf("variable $%s already exists", name)
			}
		}
	}

	varTypeID := types.GenerateID()
	bsonTypeName := mdlTypeToBsonType(dataType)
	varType := bson.D{
		{Key: "$ID", Value: bsonutil.IDToBsonBinary(varTypeID)},
		{Key: "$Type", Value: bsonTypeName},
	}
	if bsonTypeName == "DataTypes$ObjectType" {
		varType = append(varType, bson.E{Key: "Entity", Value: dataType})
	}

	varID := types.GenerateID()
	varDoc := bson.D{
		{Key: "$ID", Value: bsonutil.IDToBsonBinary(varID)},
		{Key: "$Type", Value: "Forms$LocalVariable"},
		{Key: "DefaultValue", Value: defaultValue},
		{Key: "Name", Value: name},
		{Key: "VariableType", Value: varType},
	}

	existing := bsonnav.ToBsonA(bsonnav.DGet(m.rawData, "Variables"))
	if existing != nil {
		elements := bsonnav.DGetArrayElements(bsonnav.DGet(m.rawData, "Variables"))
		elements = append(elements, varDoc)
		bsonnav.DSetArray(m.rawData, "Variables", elements)
	} else {
		m.rawData = append(m.rawData, bson.E{Key: "Variables", Value: bson.A{int32(3), varDoc}})
	}
	return nil
}

func (m *Mutator) DropVariable(name string) error {
	elements := bsonnav.DGetArrayElements(bsonnav.DGet(m.rawData, "Variables"))
	if elements == nil {
		return fmt.Errorf("variable $%s not found", name)
	}

	found := false
	var kept []any
	for _, elem := range elements {
		if doc, ok := elem.(bson.D); ok {
			if bsonnav.DGetString(doc, "Name") == name {
				found = true
				continue
			}
		}
		kept = append(kept, elem)
	}
	if !found {
		return fmt.Errorf("variable $%s not found", name)
	}
	bsonnav.DSetArray(m.rawData, "Variables", kept)
	return nil
}

func (m *Mutator) SetLayout(newLayout string, paramMappings map[string]string) error {
	if m.containerType == backend.ContainerSnippet {
		return fmt.Errorf("set Layout is not supported for snippets")
	}

	formCall := bsonnav.DGetDoc(m.rawData, "FormCall")
	if formCall == nil {
		return fmt.Errorf("page has no FormCall (layout reference)")
	}

	// Detect old layout name
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
		return fmt.Errorf("cannot determine current layout from FormCall")
	}
	if oldLayoutQN == newLayout {
		return nil
	}

	// Update Form field
	for i, elem := range formCall {
		if elem.Key == "Form" {
			formCall[i].Value = newLayout
		}
	}

	// Remap Parameter strings
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
				placeholder := paramStr
				if strings.HasPrefix(paramStr, oldLayoutQN+".") {
					placeholder = paramStr[len(oldLayoutQN)+1:]
				}
				if paramMappings != nil {
					if mapped, ok := paramMappings[placeholder]; ok {
						placeholder = mapped
					}
				}
				doc[j].Value = newLayout + "." + placeholder
			}
		}
	}

	// Write FormCall back
	for i, elem := range m.rawData {
		if elem.Key == "FormCall" {
			m.rawData[i].Value = formCall
			break
		}
	}
	return nil
}

func (m *Mutator) SetPluggableProperty(widgetRef string, propKey string, opName backend.PluggablePropertyOp, ctx backend.PluggablePropertyContext) error {
	result := m.widgetFinder(m.rawData, widgetRef)
	if result == nil {
		return fmt.Errorf("widget %q not found", widgetRef)
	}

	obj := bsonnav.DGetDoc(result.widget, "Object")
	if obj == nil {
		return fmt.Errorf("widget %q has no pluggable Object", widgetRef)
	}

	propTypeKeyMap := buildPropKeyMap(result.widget)

	props := bsonnav.DGetArrayElements(bsonnav.DGet(obj, "Properties"))
	for _, prop := range props {
		propDoc, ok := prop.(bson.D)
		if !ok {
			continue
		}
		typePointerID := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(propDoc, "TypePointer"))
		resolvedKey := propTypeKeyMap[typePointerID]
		if resolvedKey != propKey {
			continue
		}
		valDoc := bsonnav.DGetDoc(propDoc, "Value")
		if valDoc == nil {
			return fmt.Errorf("property %q has no Value", propKey)
		}

		switch opName {
		case "primitive":
			bsonnav.DSet(valDoc, "PrimitiveValue", ctx.PrimitiveVal)
		case "attribute":
			if attrDoc := bsonnav.DGetDoc(valDoc, "AttributeRef"); attrDoc != nil {
				bsonnav.DSet(attrDoc, "Attribute", ctx.AttributePath)
			} else {
				bsonnav.DSet(valDoc, "AttributeRef", bson.D{
					{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
					{Key: "$Type", Value: "DomainModels$AttributeRef"},
					{Key: "Attribute", Value: ctx.AttributePath},
					{Key: "EntityRef", Value: nil},
				})
			}
		case "association":
			bsonnav.DSet(valDoc, "AssociationRef", ctx.AssocPath)
			if ctx.EntityName != "" {
				bsonnav.DSet(valDoc, "EntityRef", bson.D{
					{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
					{Key: "$Type", Value: "DomainModels$DirectEntityRef"},
					{Key: "Entity", Value: ctx.EntityName},
				})
			}
		case "datasource":
			serialized := m.deps.SerializeCustomWidgetDataSource(ctx.DataSource)
			bsonnav.DSet(valDoc, "DataSource", serialized)
		case "widgets":
			serialized, err := m.serializeWidgets(ctx.ChildWidgets)
			if err != nil {
				return fmt.Errorf("serialize child widgets: %w", err)
			}
			var bsonArr bson.A
			bsonArr = append(bsonArr, int32(2))
			for _, w := range serialized {
				bsonArr = append(bsonArr, w)
			}
			bsonnav.DSet(valDoc, "Widgets", bsonArr)
		case "texttemplate":
			if tmpl := bsonnav.DGetDoc(valDoc, "TextTemplate"); tmpl != nil {
				items := bsonnav.DGetArrayElements(bsonnav.DGet(tmpl, "Items"))
				if len(items) > 0 {
					if itemDoc, ok := items[0].(bson.D); ok {
						bsonnav.DSet(itemDoc, "Text", ctx.TextTemplate)
					}
				}
			}
		case "action":
			serialized := m.deps.SerializeClientAction(ctx.Action)
			bsonnav.DSet(valDoc, "Action", serialized)
		case "selection":
			bsonnav.DSet(valDoc, "PrimitiveValue", ctx.Selection)
		case "attributeObjects":
			// Set multiple attribute paths on sub-objects
			objects := bsonnav.DGetArrayElements(bsonnav.DGet(valDoc, "Objects"))
			for i, attrPath := range ctx.AttributePaths {
				if i >= len(objects) {
					break
				}
				if objDoc, ok := objects[i].(bson.D); ok {
					objProps := bsonnav.DGetArrayElements(bsonnav.DGet(objDoc, "Properties"))
					for _, op := range objProps {
						opDoc, ok := op.(bson.D)
						if !ok {
							continue
						}
						if opVal := bsonnav.DGetDoc(opDoc, "Value"); opVal != nil {
							if attrRef := bsonnav.DGetDoc(opVal, "AttributeRef"); attrRef != nil {
								bsonnav.DSet(attrRef, "Attribute", attrPath)
							}
						}
					}
				}
			}
		default:
			return fmt.Errorf("unsupported pluggable property operation: %s", opName)
		}
		return nil
	}
	return fmt.Errorf("pluggable property %q not found on widget %q", propKey, widgetRef)
}

func (m *Mutator) EnclosingEntity(widgetRef string) string {
	return findEnclosingEntityContext(m.rawData, widgetRef)
}

// EnclosingEntityForChildren returns the entity context that applies to
// children of the named widget. For widgets with their own data source
// (DataView, DataGrid, ListView, DataGrid2), this is the data source entity.
// Used for ALTER PAGE column inserts/replaces, where new columns inherit the
// grid's data source as their entity context.
func (m *Mutator) EnclosingEntityForChildren(widgetRef string) string {
	result := m.widgetFinder(m.rawData, widgetRef)
	if result == nil {
		return ""
	}
	if ent := extractEntityFromDataSource(result.widget); ent != "" {
		return ent
	}
	if ent := extractPluggableDataSourceEntity(result.widget); ent != "" {
		return ent
	}
	return findEnclosingEntityContext(m.rawData, widgetRef)
}

// extractPluggableDataSourceEntity walks a CustomWidget's Object.Properties[]
// looking for a "datasource" property and returns the EntityRef.Entity if any.
func extractPluggableDataSourceEntity(widgetDoc bson.D) string {
	obj := bsonnav.DGetDoc(widgetDoc, "Object")
	if obj == nil {
		return ""
	}
	propKeyMap := buildPropKeyMap(widgetDoc)
	if len(propKeyMap) == 0 {
		return ""
	}
	for _, prop := range bsonnav.DGetArrayElements(bsonnav.DGet(obj, "Properties")) {
		propDoc, ok := prop.(bson.D)
		if !ok {
			continue
		}
		typePointerID := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(propDoc, "TypePointer"))
		if propKeyMap[typePointerID] != "datasource" {
			continue
		}
		valDoc := bsonnav.DGetDoc(propDoc, "Value")
		if valDoc == nil {
			continue
		}
		dsDoc := bsonnav.DGetDoc(valDoc, "DataSource")
		if dsDoc == nil {
			continue
		}
		if entityRef := bsonnav.DGetDoc(dsDoc, "EntityRef"); entityRef != nil {
			if entity := bsonnav.DGetString(entityRef, "Entity"); entity != "" {
				return entity
			}
		}
	}
	return ""
}

func (m *Mutator) WidgetScope() map[string]model.ID {
	return extractWidgetScopeFromBSON(m.rawData)
}

func (m *Mutator) ParamScope() (map[string]model.ID, map[string]string) {
	return extractPageParamsFromBSON(m.rawData)
}

func (m *Mutator) FindWidget(name string) bool {
	return m.widgetFinder(m.rawData, name) != nil
}

func (m *Mutator) Save() error {
	outBytes, err := bson.Marshal(m.rawData)
	if err != nil {
		return fmt.Errorf("marshal modified %s: %w", m.containerType, err)
	}
	return m.deps.SaveUnit(string(m.unitID), outBytes)
}

// ---------------------------------------------------------------------------
// BSON helpers (moved from executor/cmd_alter_page.go)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// BSON widget tree walking
// ---------------------------------------------------------------------------

// bsonWidgetResult holds a found widget and its parent context.
type bsonWidgetResult struct {
	widget      bson.D
	parentArr   []any
	parentKey   string
	parentDoc   bson.D
	index       int
	colPropKeys map[string]string
}

// widgetFinder is a function type for locating widgets in a raw BSON tree.
type widgetFinder func(rawData bson.D, widgetName string) *bsonWidgetResult

// findBsonWidget searches the raw BSON page tree for a widget by name.
func findBsonWidget(rawData bson.D, widgetName string) *bsonWidgetResult {
	formCall := bsonnav.DGetDoc(rawData, "FormCall")
	if formCall == nil {
		return nil
	}
	args := bsonnav.DGetArrayElements(bsonnav.DGet(formCall, "Arguments"))
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
func findBsonWidgetInSnippet(rawData bson.D, widgetName string) *bsonWidgetResult {
	if result := findInWidgetArray(rawData, "Widgets", widgetName); result != nil {
		return result
	}
	if widgetContainer := bsonnav.DGetDoc(rawData, "Widget"); widgetContainer != nil {
		if result := findInWidgetArray(widgetContainer, "Widgets", widgetName); result != nil {
			return result
		}
	}
	return nil
}

// findInWidgetArray searches a widget array for a named widget.
func findInWidgetArray(parentDoc bson.D, key string, widgetName string) *bsonWidgetResult {
	elements := bsonnav.DGetArrayElements(bsonnav.DGet(parentDoc, key))
	for i, elem := range elements {
		wDoc, ok := elem.(bson.D)
		if !ok {
			continue
		}
		if bsonnav.DGetString(wDoc, "Name") == widgetName {
			return &bsonWidgetResult{
				widget:    wDoc,
				parentArr: elements,
				parentKey: key,
				parentDoc: parentDoc,
				index:     i,
			}
		}
		if result := findInWidgetChildren(wDoc, widgetName); result != nil {
			return result
		}
	}
	return nil
}

// findInWidgetChildren recursively searches widget children for a named widget.
func findInWidgetChildren(wDoc bson.D, widgetName string) *bsonWidgetResult {
	typeName := bsonnav.DGetString(wDoc, "$Type")

	if result := findInWidgetArray(wDoc, "Widgets", widgetName); result != nil {
		return result
	}
	if result := findInWidgetArray(wDoc, "FooterWidgets", widgetName); result != nil {
		return result
	}

	// LayoutGrid: Rows[].Columns[].Widgets[]
	if strings.Contains(typeName, "LayoutGrid") {
		rows := bsonnav.DGetArrayElements(bsonnav.DGet(wDoc, "Rows"))
		for _, row := range rows {
			rowDoc, ok := row.(bson.D)
			if !ok {
				continue
			}
			cols := bsonnav.DGetArrayElements(bsonnav.DGet(rowDoc, "Columns"))
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
	tabPages := bsonnav.DGetArrayElements(bsonnav.DGet(wDoc, "TabPages"))
	for _, tp := range tabPages {
		tpDoc, ok := tp.(bson.D)
		if !ok {
			continue
		}
		if result := findInWidgetArray(tpDoc, "Widgets", widgetName); result != nil {
			return result
		}
	}

	// ControlBar
	if controlBar := bsonnav.DGetDoc(wDoc, "ControlBar"); controlBar != nil {
		if result := findInWidgetArray(controlBar, "Items", widgetName); result != nil {
			return result
		}
	}

	// CustomWidget (pluggable): Object.Properties[].Value.Widgets[]
	if strings.Contains(typeName, "CustomWidget") {
		if obj := bsonnav.DGetDoc(wDoc, "Object"); obj != nil {
			props := bsonnav.DGetArrayElements(bsonnav.DGet(obj, "Properties"))
			for _, prop := range props {
				propDoc, ok := prop.(bson.D)
				if !ok {
					continue
				}
				if valDoc := bsonnav.DGetDoc(propDoc, "Value"); valDoc != nil {
					if result := findInWidgetArray(valDoc, "Widgets", widgetName); result != nil {
						return result
					}
				}
			}
			// DataGrid2: search columns by derived name (stored in Objects, not Widgets)
			propKeyMap := buildPropKeyMap(wDoc)
			for _, prop := range props {
				propDoc, ok := prop.(bson.D)
				if !ok {
					continue
				}
				typePointerID := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(propDoc, "TypePointer"))
				if propKeyMap[typePointerID] != "columns" {
					continue
				}
				valDoc := bsonnav.DGetDoc(propDoc, "Value")
				if valDoc == nil {
					break
				}
				colPropKeyMap := buildColumnPropKeyMap(wDoc, typePointerID)
				columns := bsonnav.DGetArrayElements(bsonnav.DGet(valDoc, "Objects"))
				for i, colItem := range columns {
					colDoc, ok := colItem.(bson.D)
					if !ok {
						continue
					}
					if deriveColumnNameBson(colDoc, colPropKeyMap, i) == widgetName {
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
				break // only one "columns" property per widget
			}
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// DataGrid2 column finder
// ---------------------------------------------------------------------------

// findBsonColumn finds a column inside a DataGrid2 widget by derived name.
func findBsonColumn(rawData bson.D, gridName, columnName string, find widgetFinder) *bsonWidgetResult {
	gridResult := find(rawData, gridName)
	if gridResult == nil {
		return nil
	}

	gridPropKeyMap := buildPropKeyMap(gridResult.widget)

	obj := bsonnav.DGetDoc(gridResult.widget, "Object")
	if obj == nil {
		return nil
	}

	props := bsonnav.DGetArrayElements(bsonnav.DGet(obj, "Properties"))
	for _, prop := range props {
		propDoc, ok := prop.(bson.D)
		if !ok {
			continue
		}
		typePointerID := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(propDoc, "TypePointer"))
		propKey := gridPropKeyMap[typePointerID]
		if propKey != "columns" {
			continue
		}

		valDoc := bsonnav.DGetDoc(propDoc, "Value")
		if valDoc == nil {
			return nil
		}

		colPropKeyMap := buildColumnPropKeyMap(gridResult.widget, typePointerID)

		columns := bsonnav.DGetArrayElements(bsonnav.DGet(valDoc, "Objects"))
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
		return nil
	}
	return nil
}

// buildPropKeyMap builds a TypePointer ID -> PropertyKey map.
func buildPropKeyMap(widgetDoc bson.D) map[string]string {
	m := make(map[string]string)
	widgetType := bsonnav.DGetDoc(widgetDoc, "Type")
	if widgetType == nil {
		return m
	}
	objType := bsonnav.DGetDoc(widgetType, "ObjectType")
	if objType == nil {
		return m
	}
	for _, pt := range bsonnav.DGetArrayElements(bsonnav.DGet(objType, "PropertyTypes")) {
		ptDoc, ok := pt.(bson.D)
		if !ok {
			continue
		}
		key := bsonnav.DGetString(ptDoc, "PropertyKey")
		id := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(ptDoc, "$ID"))
		if key != "" && id != "" {
			m[id] = key
		}
	}
	return m
}

// buildColumnPropKeyMap builds a TypePointer ID -> PropertyKey map for column properties.
func buildColumnPropKeyMap(widgetDoc bson.D, columnsTypePointerID string) map[string]string {
	m := make(map[string]string)
	widgetType := bsonnav.DGetDoc(widgetDoc, "Type")
	if widgetType == nil {
		return m
	}
	objType := bsonnav.DGetDoc(widgetType, "ObjectType")
	if objType == nil {
		return m
	}
	for _, pt := range bsonnav.DGetArrayElements(bsonnav.DGet(objType, "PropertyTypes")) {
		ptDoc, ok := pt.(bson.D)
		if !ok {
			continue
		}
		id := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(ptDoc, "$ID"))
		if id != columnsTypePointerID {
			continue
		}
		valType := bsonnav.DGetDoc(ptDoc, "ValueType")
		if valType == nil {
			return m
		}
		colObjType := bsonnav.DGetDoc(valType, "ObjectType")
		if colObjType == nil {
			return m
		}
		for _, cpt := range bsonnav.DGetArrayElements(bsonnav.DGet(colObjType, "PropertyTypes")) {
			cptDoc, ok := cpt.(bson.D)
			if !ok {
				continue
			}
			key := bsonnav.DGetString(cptDoc, "PropertyKey")
			cid := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(cptDoc, "$ID"))
			if key != "" && cid != "" {
				m[cid] = key
			}
		}
		return m
	}
	return m
}

// deriveColumnNameBson derives a column name from its BSON WidgetObject.
func deriveColumnNameBson(colDoc bson.D, propKeyMap map[string]string, index int) string {
	var attribute, caption string

	props := bsonnav.DGetArrayElements(bsonnav.DGet(colDoc, "Properties"))
	for _, prop := range props {
		propDoc, ok := prop.(bson.D)
		if !ok {
			continue
		}
		typePointerID := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(propDoc, "TypePointer"))
		propKey := propKeyMap[typePointerID]

		valDoc := bsonnav.DGetDoc(propDoc, "Value")
		if valDoc == nil {
			continue
		}

		switch propKey {
		case "attribute":
			if attrRef := bsonnav.DGetString(valDoc, "AttributeRef"); attrRef != "" {
				attribute = attrRef
			} else if attrDoc := bsonnav.DGetDoc(valDoc, "AttributeRef"); attrDoc != nil {
				attribute = bsonnav.DGetString(attrDoc, "Attribute")
			}
		case "header":
			// TextTemplate → Template (Forms$Text) → Items[] → Translation{Text}.
			// Must traverse the intermediate Template document — same path as
			// deriveColumnName on the DESCRIBE side.
			if tmpl := bsonnav.DGetDoc(valDoc, "TextTemplate"); tmpl != nil {
				if template := bsonnav.DGetDoc(tmpl, "Template"); template != nil {
					items := bsonnav.DGetArrayElements(bsonnav.DGet(template, "Items"))
					for _, item := range items {
						if itemDoc, ok := item.(bson.D); ok {
							if text := bsonnav.DGetString(itemDoc, "Text"); text != "" {
								caption = text
							}
						}
					}
				}
			}
		}
	}

	if attribute != "" {
		parts := strings.Split(attribute, ".")
		return parts[len(parts)-1]
	}
	if caption != "" {
		if name := sanitizeColumnName(caption); name != "" {
			return name
		}
	}
	return fmt.Sprintf("col%d", index+1)
}

// sanitizeColumnName converts a caption string into a valid column identifier,
// matching deriveColumnName() in cmd_pages_describe_output.go exactly.
// Returns "" when the result would be all underscores so the caller falls
// through to the col{N} index fallback.
func sanitizeColumnName(caption string) string {
	sanitized := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			return r
		}
		return '_'
	}, caption)
	return strings.TrimFunc(sanitized, func(r rune) bool { return r == '_' })
}

// ---------------------------------------------------------------------------
// Entity context extraction
// ---------------------------------------------------------------------------

// findEnclosingEntityContext walks the raw BSON tree to find the entity context.
func findEnclosingEntityContext(rawData bson.D, widgetName string) string {
	if formCall := bsonnav.DGetDoc(rawData, "FormCall"); formCall != nil {
		args := bsonnav.DGetArrayElements(bsonnav.DGet(formCall, "Arguments"))
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
	if ctx := findEntityContextInWidgets(rawData, "Widgets", widgetName, ""); ctx != "" {
		return ctx
	}
	if widgetContainer := bsonnav.DGetDoc(rawData, "Widget"); widgetContainer != nil {
		if ctx := findEntityContextInWidgets(widgetContainer, "Widgets", widgetName, ""); ctx != "" {
			return ctx
		}
	}
	return ""
}

func findEntityContextInWidgets(parentDoc bson.D, key string, widgetName string, currentEntity string) string {
	elements := bsonnav.DGetArrayElements(bsonnav.DGet(parentDoc, key))
	for _, elem := range elements {
		wDoc, ok := elem.(bson.D)
		if !ok {
			continue
		}
		if bsonnav.DGetString(wDoc, "Name") == widgetName {
			return currentEntity
		}
		entityCtx := currentEntity
		if ent := extractEntityFromDataSource(wDoc); ent != "" {
			entityCtx = ent
		}
		if ctx := findEntityContextInChildren(wDoc, widgetName, entityCtx); ctx != "" {
			return ctx
		}
	}
	return ""
}

func findEntityContextInChildren(wDoc bson.D, widgetName string, currentEntity string) string {
	typeName := bsonnav.DGetString(wDoc, "$Type")

	if ctx := findEntityContextInWidgets(wDoc, "Widgets", widgetName, currentEntity); ctx != "" {
		return ctx
	}
	if ctx := findEntityContextInWidgets(wDoc, "FooterWidgets", widgetName, currentEntity); ctx != "" {
		return ctx
	}
	if strings.Contains(typeName, "LayoutGrid") {
		rows := bsonnav.DGetArrayElements(bsonnav.DGet(wDoc, "Rows"))
		for _, row := range rows {
			rowDoc, ok := row.(bson.D)
			if !ok {
				continue
			}
			cols := bsonnav.DGetArrayElements(bsonnav.DGet(rowDoc, "Columns"))
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
	tabPages := bsonnav.DGetArrayElements(bsonnav.DGet(wDoc, "TabPages"))
	for _, tp := range tabPages {
		tpDoc, ok := tp.(bson.D)
		if !ok {
			continue
		}
		if ctx := findEntityContextInWidgets(tpDoc, "Widgets", widgetName, currentEntity); ctx != "" {
			return ctx
		}
	}
	if controlBar := bsonnav.DGetDoc(wDoc, "ControlBar"); controlBar != nil {
		if ctx := findEntityContextInWidgets(controlBar, "Items", widgetName, currentEntity); ctx != "" {
			return ctx
		}
	}
	if strings.Contains(typeName, "CustomWidget") {
		if obj := bsonnav.DGetDoc(wDoc, "Object"); obj != nil {
			props := bsonnav.DGetArrayElements(bsonnav.DGet(obj, "Properties"))
			for _, prop := range props {
				propDoc, ok := prop.(bson.D)
				if !ok {
					continue
				}
				if valDoc := bsonnav.DGetDoc(propDoc, "Value"); valDoc != nil {
					if ctx := findEntityContextInWidgets(valDoc, "Widgets", widgetName, currentEntity); ctx != "" {
						return ctx
					}
				}
			}
		}
	}
	return ""
}

func extractEntityFromDataSource(wDoc bson.D) string {
	ds := bsonnav.DGetDoc(wDoc, "DataSource")
	if ds == nil {
		return ""
	}
	if entityRef := bsonnav.DGetDoc(ds, "EntityRef"); entityRef != nil {
		if entity := bsonnav.DGetString(entityRef, "Entity"); entity != "" {
			return entity
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Widget scope extraction
// ---------------------------------------------------------------------------

func extractWidgetScopeFromBSON(rawData bson.D) map[string]model.ID {
	scope := make(map[string]model.ID)
	if rawData == nil {
		return scope
	}
	if formCall := bsonnav.DGetDoc(rawData, "FormCall"); formCall != nil {
		args := bsonnav.DGetArrayElements(bsonnav.DGet(formCall, "Arguments"))
		for _, arg := range args {
			argDoc, ok := arg.(bson.D)
			if !ok {
				continue
			}
			collectWidgetScope(argDoc, "Widgets", scope)
		}
	}
	collectWidgetScope(rawData, "Widgets", scope)
	if widgetContainer := bsonnav.DGetDoc(rawData, "Widget"); widgetContainer != nil {
		collectWidgetScope(widgetContainer, "Widgets", scope)
	}
	return scope
}

// extractPageParamsFromBSON extracts page/snippet parameter names and entity
// IDs from the raw BSON document.
func extractPageParamsFromBSON(rawData bson.D) (map[string]model.ID, map[string]string) {
	paramScope := make(map[string]model.ID)
	paramEntityNames := make(map[string]string)
	if rawData == nil {
		return paramScope, paramEntityNames
	}

	params := bsonnav.DGetArrayElements(bsonnav.DGet(rawData, "Parameters"))
	for _, p := range params {
		pDoc, ok := p.(bson.D)
		if !ok {
			continue
		}
		name := bsonnav.DGetString(pDoc, "Name")
		if name == "" {
			continue
		}
		paramType := bsonnav.DGetDoc(pDoc, "ParameterType")
		if paramType == nil {
			continue
		}
		typeName := bsonnav.DGetString(paramType, "$Type")
		if typeName != "DataTypes$ObjectType" {
			continue
		}
		entityName := bsonnav.DGetString(paramType, "Entity")
		if entityName == "" {
			continue
		}
		idVal := bsonnav.DGet(pDoc, "$ID")
		paramID := model.ID(bsonnav.ExtractBinaryIDFromDoc(idVal))
		paramScope[name] = paramID
		paramEntityNames[name] = entityName
	}
	return paramScope, paramEntityNames
}

func collectWidgetScope(parentDoc bson.D, key string, scope map[string]model.ID) {
	elements := bsonnav.DGetArrayElements(bsonnav.DGet(parentDoc, key))
	for _, elem := range elements {
		wDoc, ok := elem.(bson.D)
		if !ok {
			continue
		}
		name := bsonnav.DGetString(wDoc, "Name")
		if name != "" {
			idVal := bsonnav.DGet(wDoc, "$ID")
			if wID := bsonnav.ExtractBinaryIDFromDoc(idVal); wID != "" {
				scope[name] = model.ID(wID)
			}
		}
		collectWidgetScopeInChildren(wDoc, scope)
	}
}

func collectWidgetScopeInChildren(wDoc bson.D, scope map[string]model.ID) {
	typeName := bsonnav.DGetString(wDoc, "$Type")

	collectWidgetScope(wDoc, "Widgets", scope)
	collectWidgetScope(wDoc, "FooterWidgets", scope)

	if strings.Contains(typeName, "LayoutGrid") {
		rows := bsonnav.DGetArrayElements(bsonnav.DGet(wDoc, "Rows"))
		for _, row := range rows {
			rowDoc, ok := row.(bson.D)
			if !ok {
				continue
			}
			cols := bsonnav.DGetArrayElements(bsonnav.DGet(rowDoc, "Columns"))
			for _, col := range cols {
				colDoc, ok := col.(bson.D)
				if !ok {
					continue
				}
				collectWidgetScope(colDoc, "Widgets", scope)
			}
		}
	}
	tabPages := bsonnav.DGetArrayElements(bsonnav.DGet(wDoc, "TabPages"))
	for _, tp := range tabPages {
		tpDoc, ok := tp.(bson.D)
		if !ok {
			continue
		}
		collectWidgetScope(tpDoc, "Widgets", scope)
	}
	if controlBar := bsonnav.DGetDoc(wDoc, "ControlBar"); controlBar != nil {
		collectWidgetScope(controlBar, "Items", scope)
	}
	if strings.Contains(typeName, "CustomWidget") {
		if obj := bsonnav.DGetDoc(wDoc, "Object"); obj != nil {
			props := bsonnav.DGetArrayElements(bsonnav.DGet(obj, "Properties"))
			for _, prop := range props {
				propDoc, ok := prop.(bson.D)
				if !ok {
					continue
				}
				if valDoc := bsonnav.DGetDoc(propDoc, "Value"); valDoc != nil {
					collectWidgetScope(valDoc, "Widgets", scope)
				}
			}
			// DataGrid2: add column derived names to scope for duplicate-name detection
			propKeyMap := buildPropKeyMap(wDoc)
			for _, prop := range props {
				propDoc, ok := prop.(bson.D)
				if !ok {
					continue
				}
				typePointerID := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(propDoc, "TypePointer"))
				if propKeyMap[typePointerID] != "columns" {
					continue
				}
				valDoc := bsonnav.DGetDoc(propDoc, "Value")
				if valDoc == nil {
					break
				}
				colPropKeyMap := buildColumnPropKeyMap(wDoc, typePointerID)
				columns := bsonnav.DGetArrayElements(bsonnav.DGet(valDoc, "Objects"))
				for i, colItem := range columns {
					colDoc, ok := colItem.(bson.D)
					if !ok {
						continue
					}
					derived := deriveColumnNameBson(colDoc, colPropKeyMap, i)
					if derived != "" {
						idVal := bsonnav.DGet(colDoc, "$ID")
						if wID := bsonnav.ExtractBinaryIDFromDoc(idVal); wID != "" {
							scope[derived] = model.ID(wID)
						}
					}
				}
				break
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Property setting helpers
// ---------------------------------------------------------------------------

// columnPropertyAliases maps user-facing property names to internal column property keys.
// MDL lookup is case-insensitive (see columnPropertyAliasesCI below); the values
// here are the BSON-internal PropertyKeys defined by the DataGrid2 widget schema
// and must stay case-sensitive.
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

// columnPropertyAliasesCI is a lowercase-keyed view of columnPropertyAliases
// used for case-insensitive MDL lookup (set caption = … vs set Caption = …).
var columnPropertyAliasesCI = func() map[string]string {
	m := make(map[string]string, len(columnPropertyAliases))
	for k, v := range columnPropertyAliases {
		m[strings.ToLower(k)] = v
	}
	return m
}()

func setColumnPropertyMut(colDoc bson.D, propKeyMap map[string]string, propName string, value any) error {
	internalKey := columnPropertyAliasesCI[strings.ToLower(propName)]
	if internalKey == "" {
		internalKey = propName
	}

	props := bsonnav.DGetArrayElements(bsonnav.DGet(colDoc, "Properties"))
	for _, prop := range props {
		propDoc, ok := prop.(bson.D)
		if !ok {
			continue
		}
		typePointerID := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(propDoc, "TypePointer"))
		propKey := propKeyMap[typePointerID]
		if propKey != internalKey {
			continue
		}
		valDoc := bsonnav.DGetDoc(propDoc, "Value")
		if valDoc == nil {
			return fmt.Errorf("column property %q has no Value", propName)
		}
		strVal := fmt.Sprintf("%v", value)
		// TextTemplate-valued properties (header, tooltip) store the text inside
		// a nested Forms$ClientTemplate → Texts$Text → Items[Translation].Text.
		if textTemplate := bsonnav.DGetDoc(valDoc, "TextTemplate"); textTemplate != nil {
			if updateClientTemplateText(textTemplate, strVal) {
				return nil
			}
		}
		// Primitive-valued properties (sortable, visible, alignment, etc.)
		bsonnav.DSet(valDoc, "PrimitiveValue", strVal)
		return nil
	}
	return fmt.Errorf("column property %q not found", propName)
}

// updateClientTemplateText replaces the Template.Items[*].Text of a
// Forms$ClientTemplate. Returns true if a Translation entry was updated.
// If no Translation exists, a new en_US one is appended.
func updateClientTemplateText(clientTemplate bson.D, text string) bool {
	template := bsonnav.DGetDoc(clientTemplate, "Template")
	if template == nil {
		return false
	}
	items := bsonnav.DGetArrayElements(bsonnav.DGet(template, "Items"))
	updated := false
	for _, item := range items {
		itemDoc, ok := item.(bson.D)
		if !ok {
			continue
		}
		if bsonnav.DGetString(itemDoc, "$Type") == "Texts$Translation" {
			bsonnav.DSet(itemDoc, "Text", text)
			updated = true
		}
	}
	if updated {
		return true
	}
	// No existing Translation — append an en_US one.
	newItem := bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Texts$Translation"},
		{Key: "LanguageCode", Value: "en_US"},
		{Key: "Text", Value: text},
	}
	newArr := bson.A{int32(3)}
	for _, item := range items {
		newArr = append(newArr, item)
	}
	newArr = append(newArr, newItem)
	bsonnav.DSet(template, "Items", newArr)
	return true
}

// applyPageLevelSetMut applies a page-level SET (no widget target). It returns
// the (possibly extended) rawData so the caller can pick up appended top-level
// fields — bson.D is a slice, so appending a new key isn't visible through the
// value parameter alone.
func applyPageLevelSetMut(rawData bson.D, prop string, value any) (bson.D, error) {
	switch prop {
	case "Title":
		strVal, ok := value.(string)
		if !ok {
			return rawData, fmt.Errorf("Title value must be a string")
		}
		// The page's Title is at the top level of the Forms$Page document,
		// parallel to FormCall (not nested inside it). It's a Texts$Text doc
		// whose Items[] array holds Texts$Translation entries.
		titleDoc := bsonnav.DGetDoc(rawData, "Title")
		if titleDoc == nil {
			return rawData, fmt.Errorf("page has no Title field")
		}
		if !updateTextsTextValue(titleDoc, strVal) {
			return rawData, fmt.Errorf("could not update Title text")
		}
	case "Url":
		strVal, _ := value.(string)
		rawData = dSetOrAppend(rawData, "Url", strVal)
	case "PopupWidth", "PopupHeight":
		// Pop-up dimensions live at the top level of the Forms$Page document and
		// are stored as int64 (matching what Studio Pro and the legacy writer
		// emit). They apply when the page is shown in a pop-up.
		n, err := coercePopupDimension(prop, value)
		if err != nil {
			return rawData, err
		}
		rawData = dSetOrAppend(rawData, prop, n)
	case "PopupResizable":
		boolVal, ok := value.(bool)
		if !ok {
			return rawData, fmt.Errorf("PopupResizable value must be a boolean (true or false)")
		}
		rawData = dSetOrAppend(rawData, "PopupResizable", boolVal)
	case "PopupCloseAction":
		strVal, ok := value.(string)
		if !ok {
			return rawData, fmt.Errorf("PopupCloseAction value must be a string")
		}
		rawData = dSetOrAppend(rawData, "PopupCloseAction", strVal)
	case "Class", "Style":
		// The page's CSS class / inline style live on its Forms$Appearance
		// sub-document (issue #714), not at the top level of the Forms$Page.
		strVal, ok := value.(string)
		if !ok {
			return rawData, fmt.Errorf("%s value must be a string", prop)
		}
		appearance := bsonnav.DGetDoc(rawData, "Appearance")
		if appearance == nil {
			appearance = bson.D{{Key: "$Type", Value: "Forms$Appearance"}, {Key: prop, Value: strVal}}
			rawData = dSetOrAppend(rawData, "Appearance", appearance)
		} else if !bsonnav.DSet(appearance, prop, strVal) {
			appearance = append(appearance, bson.E{Key: prop, Value: strVal})
			rawData = dSetOrAppend(rawData, "Appearance", appearance)
		}
	default:
		return rawData, fmt.Errorf("unsupported page-level property: %s "+
			"(supported: Title, Url, PopupWidth, PopupHeight, PopupResizable, PopupCloseAction, Class, Style)", prop)
	}
	return rawData, nil
}

// dSetOrAppend updates the value of an existing top-level key, or appends the key
// when it is absent. Returns the (possibly grown) doc.
func dSetOrAppend(doc bson.D, key string, value any) bson.D {
	if bsonnav.DSet(doc, key, value) {
		return doc
	}
	return append(doc, bson.E{Key: key, Value: value})
}

// coercePopupDimension converts an MDL numeric value to the int64 BSON form used
// by the page's PopupWidth/PopupHeight fields. Integer literals arrive from the
// visitor as int (strconv.Atoi); a value written with a decimal point arrives as
// float64. The result is bounds-checked to a non-negative int32-range pixel count
// — the range Studio Pro accepts — so silent overflow can't reach the serializer.
// 0 is valid: it is Studio Pro's default and means auto-size (issue #713).
func coercePopupDimension(prop string, value any) (int64, error) {
	var n int64
	switch v := value.(type) {
	case int:
		n = int64(v)
	case int32:
		n = int64(v)
	case int64:
		n = v
	case float64:
		if v != math.Trunc(v) {
			return 0, fmt.Errorf("%s must be a whole number, got %v", prop, v)
		}
		if v < math.MinInt32 || v > math.MaxInt32 {
			return 0, fmt.Errorf("%s value %v is out of range", prop, v)
		}
		n = int64(v)
	default:
		return 0, fmt.Errorf("%s value must be a number, got %T", prop, value)
	}
	if n < 0 {
		return 0, fmt.Errorf("%s must be >= 0 (0 = auto-size), got %d", prop, n)
	}
	if n > math.MaxInt32 {
		return 0, fmt.Errorf("%s value %d is out of range", prop, n)
	}
	return n, nil
}

// updateTextsTextValue updates the Text field of a Texts$Text doc's en_US
// Translation in its Items[] array. If no Translation exists, an en_US one is
// appended. Returns true on success.
func updateTextsTextValue(textsTextDoc bson.D, text string) bool {
	items := bsonnav.DGetArrayElements(bsonnav.DGet(textsTextDoc, "Items"))
	updated := false
	for _, item := range items {
		itemDoc, ok := item.(bson.D)
		if !ok {
			continue
		}
		if bsonnav.DGetString(itemDoc, "$Type") == "Texts$Translation" {
			bsonnav.DSet(itemDoc, "Text", text)
			updated = true
		}
	}
	if updated {
		return true
	}
	// No existing Translation — append an en_US one.
	newItem := bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Texts$Translation"},
		{Key: "LanguageCode", Value: "en_US"},
		{Key: "Text", Value: text},
	}
	newArr := bson.A{int32(3)}
	for _, item := range items {
		newArr = append(newArr, item)
	}
	newArr = append(newArr, newItem)
	bsonnav.DSet(textsTextDoc, "Items", newArr)
	return true
}

// setWidgetConditionalSettingMut replaces a widget's ConditionalVisibility/
// EditabilitySettings slot (null when unset) with a node carrying the expression,
// mirroring the legacy/Studio Pro structure (null Attribute/SourceVariable, empty
// marker-3 Conditions, plus IgnoreSecurity/ModuleRoles for visibility). Returns
// false when the widget has no such slot (e.g. editability on a non-input widget).
func setWidgetConditionalSettingMut(widget bson.D, field, typeName, expression string, withSecurity bool) bool {
	doc := bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: typeName},
		{Key: "Attribute", Value: nil},
		{Key: "Conditions", Value: bson.A{int32(3)}},
		{Key: "Expression", Value: expression},
	}
	if withSecurity {
		doc = append(doc,
			bson.E{Key: "IgnoreSecurity", Value: false},
			bson.E{Key: "ModuleRoles", Value: bson.A{int32(3)}},
		)
	}
	doc = append(doc, bson.E{Key: "SourceVariable", Value: nil})
	return bsonnav.DSet(widget, field, doc)
}

func setRawWidgetPropertyMut(widget bson.D, propName string, value any) error {
	switch propName {
	case "Caption":
		return setWidgetCaptionMut(widget, value)
	case "Content":
		return setWidgetContentMut(widget, value)
	case "Label":
		return setWidgetLabelMut(widget, value)
	case "ButtonStyle":
		if s, ok := value.(string); ok {
			bsonnav.DSet(widget, "ButtonStyle", s)
		}
		return nil
	case "Class":
		if appearance := bsonnav.DGetDoc(widget, "Appearance"); appearance != nil {
			if s, ok := value.(string); ok {
				bsonnav.DSet(appearance, "Class", s)
			}
		}
		return nil
	case "Style":
		if appearance := bsonnav.DGetDoc(widget, "Appearance"); appearance != nil {
			if s, ok := value.(string); ok {
				bsonnav.DSet(appearance, "Style", s)
			}
		}
		return nil
	case "DynamicClasses":
		if appearance := bsonnav.DGetDoc(widget, "Appearance"); appearance != nil {
			if s, ok := value.(string); ok {
				bsonnav.DSet(appearance, "DynamicClasses", s)
			}
		}
		return nil
	case "Editable":
		if s, ok := value.(string); ok {
			bsonnav.DSet(widget, "Editable", s)
		}
		return nil
	case "Visible":
		// A page widget has no plain boolean "Visible" field — visibility is modeled
		// via ConditionalVisibilitySettings. Route static booleans and expression
		// strings into it (previously a bare "Visible" string was written and Studio
		// Pro silently dropped it). The `[expr]` bracket form arrives as VisibleIf.
		expr, hasSetting := pages.StaticVisibleExpression(value)
		if !hasSetting {
			// `Visible = true` (default-visible): clear any conditional-visibility node.
			bsonnav.DSet(widget, "ConditionalVisibilitySettings", nil)
			return nil
		}
		if !setWidgetConditionalSettingMut(widget, "ConditionalVisibilitySettings",
			"Forms$ConditionalVisibilitySettings", expr, true) {
			return fmt.Errorf("widget does not support conditional visibility")
		}
		return nil
	case "VisibleIf":
		// Conditional visibility expression (issue #627): replace the widget's
		// ConditionalVisibilitySettings node (null when unset) with one carrying
		// the rooted expression the visitor produced.
		expr, _ := value.(string)
		if !setWidgetConditionalSettingMut(widget, "ConditionalVisibilitySettings",
			"Forms$ConditionalVisibilitySettings", expr, true) {
			return fmt.Errorf("widget does not support conditional visibility")
		}
		return nil
	case "EditableIf":
		expr, _ := value.(string)
		if !setWidgetConditionalSettingMut(widget, "ConditionalEditabilitySettings",
			"Forms$ConditionalEditabilitySettings", expr, false) {
			return fmt.Errorf("widget does not support conditional editability (only input widgets are editable)")
		}
		return nil
	case "Name":
		if s, ok := value.(string); ok {
			bsonnav.DSet(widget, "Name", s)
		}
		return nil
	case "Attribute":
		return setWidgetAttributeRefMut(widget, value)
	default:
		// Try as pluggable widget property
		return setPluggableWidgetPropertyMut(widget, propName, value)
	}
}

// ---------------------------------------------------------------------------
// Design property (Atlas styling) mutation
// ---------------------------------------------------------------------------

const (
	designPropertyEntryType  = "Forms$DesignPropertyValue"
	toggleDesignPropertyType = "Forms$ToggleDesignPropertyValue"
	optionDesignPropertyType = "Forms$OptionDesignPropertyValue"
	customDesignPropertyType = "Forms$CustomDesignPropertyValue"
)

// setDesignPropertyMut sets or updates a single design property in the widget's
// Appearance.DesignProperties array. valueType is "toggle" (no value) or "option"
// (carries option). An existing entry's Value is fully rewritten to the new
// valueType — so an option-type set on a stale "custom" value
// (ToggleButtonGroup/ColorPicker) overwrites it with an OptionDesignPropertyValue,
// repairing the CE6084 that a Custom encoding triggers (see
// buildDesignPropertyValueDoc and TestSetDesignProperty_OptionOverwritesCustom).
func setDesignPropertyMut(widget bson.D, key, valueType, option string) error {
	appearance := bsonnav.DGetDoc(widget, "Appearance")
	if appearance == nil {
		return fmt.Errorf("widget has no Appearance; cannot set design property %q", key)
	}
	elements := bsonnav.DGetArrayElements(bsonnav.DGet(appearance, "DesignProperties"))

	for _, el := range elements {
		entry, ok := el.(bson.D)
		if !ok || bsonnav.DGetString(entry, "Key") != key {
			continue
		}
		bsonnav.DSet(entry, "Value", buildDesignPropertyValueDoc(valueType, option))
		bsonnav.DSetArray(appearance, "DesignProperties", elements)
		return nil
	}

	entry := bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: designPropertyEntryType},
		{Key: "Key", Value: key},
		{Key: "Value", Value: buildDesignPropertyValueDoc(valueType, option)},
	}
	bsonnav.DSetArray(appearance, "DesignProperties", append(elements, entry))
	return nil
}

// removeDesignPropertyMut removes a single design property by key.
func removeDesignPropertyMut(widget bson.D, key string) error {
	appearance := bsonnav.DGetDoc(widget, "Appearance")
	if appearance == nil {
		return nil
	}
	elements := bsonnav.DGetArrayElements(bsonnav.DGet(appearance, "DesignProperties"))
	kept := make([]any, 0, len(elements))
	for _, el := range elements {
		if entry, ok := el.(bson.D); ok && bsonnav.DGetString(entry, "Key") == key {
			continue
		}
		kept = append(kept, el)
	}
	bsonnav.DSetArray(appearance, "DesignProperties", kept)
	return nil
}

// clearDesignPropertiesMut removes all design properties from the widget,
// leaving an empty (marker-only) array.
func clearDesignPropertiesMut(widget bson.D) error {
	appearance := bsonnav.DGetDoc(widget, "Appearance")
	if appearance == nil {
		return nil
	}
	bsonnav.DSetArray(appearance, "DesignProperties", nil)
	return nil
}

// buildDesignPropertyValueDoc builds the typed Value sub-document for a design
// property entry. valueType is "toggle", "option", or "custom". Single-selection
// design properties (Dropdown AND ToggleButtonGroup) use "option"
// (Forms$OptionDesignPropertyValue) — verified against Studio Pro-authored
// widgets; encoding a ToggleButtonGroup value as "custom" triggers CE6084.
func buildDesignPropertyValueDoc(valueType, option string) bson.D {
	switch valueType {
	case "toggle":
		return bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: toggleDesignPropertyType},
		}
	case "custom":
		return bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: customDesignPropertyType},
			{Key: "Value", Value: option},
		}
	default:
		return bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: optionDesignPropertyType},
			{Key: "Option", Value: option},
		}
	}
}

func setWidgetCaptionMut(widget bson.D, value any) error {
	caption := bsonnav.DGetDoc(widget, "Caption")
	if caption == nil {
		return mdlerrors.NewValidation("widget has no Caption property")
	}
	setTranslatableText(caption, "", value)
	return nil
}

func setWidgetContentMut(widget bson.D, value any) error {
	strVal, ok := value.(string)
	if !ok {
		return fmt.Errorf("Content value must be a string")
	}
	content := bsonnav.DGetDoc(widget, "Content")
	if content == nil {
		return fmt.Errorf("widget has no Content property")
	}
	template := bsonnav.DGetDoc(content, "Template")
	if template == nil {
		return fmt.Errorf("Content has no Template")
	}
	items := bsonnav.DGetArrayElements(bsonnav.DGet(template, "Items"))
	if len(items) > 0 {
		if itemDoc, ok := items[0].(bson.D); ok {
			bsonnav.DSet(itemDoc, "Text", strVal)
			return nil
		}
	}
	return fmt.Errorf("Content.Template has no Items with Text")
}

// setWidgetLabelMut sets the widget's Label caption. Returns nil without error
// if the widget has no Label field — not all widget types support labels.
func setWidgetLabelMut(widget bson.D, value any) error {
	label := bsonnav.DGetDoc(widget, "Label")
	if label == nil {
		return nil
	}
	setTranslatableText(label, "Caption", value)
	return nil
}

func setWidgetAttributeRefMut(widget bson.D, value any) error {
	attrPath, ok := value.(string)
	if !ok {
		return fmt.Errorf("Attribute value must be a string")
	}

	var attrRefValue any
	if strings.Count(attrPath, ".") >= 2 {
		attrRefValue = bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "DomainModels$AttributeRef"},
			{Key: "Attribute", Value: attrPath},
			{Key: "EntityRef", Value: nil},
		}
	} else {
		attrRefValue = nil
	}

	for i, elem := range widget {
		if elem.Key == "AttributeRef" {
			widget[i].Value = attrRefValue
			return nil
		}
	}
	return fmt.Errorf("widget does not have an AttributeRef property")
}

func setPluggableWidgetPropertyMut(widget bson.D, propName string, value any) error {
	obj := bsonnav.DGetDoc(widget, "Object")
	if obj == nil {
		return fmt.Errorf("property %q not found (widget has no pluggable Object)", propName)
	}

	propTypeKeyMap := make(map[string]string)
	if widgetType := bsonnav.DGetDoc(widget, "Type"); widgetType != nil {
		if objType := bsonnav.DGetDoc(widgetType, "ObjectType"); objType != nil {
			propTypes := bsonnav.DGetArrayElements(bsonnav.DGet(objType, "PropertyTypes"))
			for _, pt := range propTypes {
				ptDoc, ok := pt.(bson.D)
				if !ok {
					continue
				}
				key := bsonnav.DGetString(ptDoc, "PropertyKey")
				if key == "" {
					continue
				}
				id := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(ptDoc, "$ID"))
				if id != "" {
					propTypeKeyMap[id] = key
				}
			}
		}
	}

	props := bsonnav.DGetArrayElements(bsonnav.DGet(obj, "Properties"))
	for _, prop := range props {
		propDoc, ok := prop.(bson.D)
		if !ok {
			continue
		}
		typePointerID := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(propDoc, "TypePointer"))
		propKey := propTypeKeyMap[typePointerID]
		if propKey != propName {
			continue
		}
		if valDoc := bsonnav.DGetDoc(propDoc, "Value"); valDoc != nil {
			switch v := value.(type) {
			case string:
				bsonnav.DSet(valDoc, "PrimitiveValue", v)
			case bool:
				if v {
					bsonnav.DSet(valDoc, "PrimitiveValue", "yes")
				} else {
					bsonnav.DSet(valDoc, "PrimitiveValue", "no")
				}
			case int:
				bsonnav.DSet(valDoc, "PrimitiveValue", fmt.Sprintf("%d", v))
			case float64:
				bsonnav.DSet(valDoc, "PrimitiveValue", fmt.Sprintf("%g", v))
			default:
				bsonnav.DSet(valDoc, "PrimitiveValue", fmt.Sprintf("%v", v))
			}
			return nil
		}
		return fmt.Errorf("property %q has no Value map", propName)
	}
	return fmt.Errorf("pluggable property %q not found", propName)
}

// setTranslatableText sets a translatable text value in BSON.
func setTranslatableText(parent bson.D, key string, value any) {
	strVal, ok := value.(string)
	if !ok {
		return
	}

	target := parent
	if key != "" {
		if nested := bsonnav.DGetDoc(parent, key); nested != nil {
			target = nested
		} else {
			bsonnav.DSet(parent, key, strVal)
			return
		}
	}

	translations := bsonnav.DGetArrayElements(bsonnav.DGet(target, "Translations"))
	if len(translations) > 0 {
		if tDoc, ok := translations[0].(bson.D); ok {
			bsonnav.DSet(tDoc, "Text", strVal)
			return
		}
	}
	bsonnav.DSet(target, "Text", strVal)
}

// ---------------------------------------------------------------------------
// Widget serialization helpers
// ---------------------------------------------------------------------------

func (m *Mutator) serializeWidgets(widgets []pages.Widget) ([]any, error) {
	var result []any
	for _, w := range widgets {
		bsonDoc := m.deps.SerializeWidget(w)
		if bsonDoc == nil {
			continue
		}
		result = append(result, bsonDoc)
	}
	return result, nil
}

// serializeDataSourceBson converts a pages.DataSource to a BSON document for widget-level DataSource fields.
func serializeDataSourceBson(ds pages.DataSource) bson.D {
	switch d := ds.(type) {
	case *pages.ListenToWidgetSource:
		return bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$ListenTargetSource"},
			{Key: "ListenTarget", Value: d.WidgetName},
		}
	case *pages.DatabaseSource:
		var entityRef any
		if d.EntityName != "" {
			entityRef = bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "DomainModels$DirectEntityRef"},
				{Key: "Entity", Value: d.EntityName},
			}
		}
		return bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$DataViewSource"},
			{Key: "EntityRef", Value: entityRef},
			{Key: "ForceFullObjects", Value: false},
			{Key: "SourceVariable", Value: nil},
		}
	case *pages.MicroflowSource:
		return bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$MicroflowSource"},
			{Key: "MicroflowSettings", Value: bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
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
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$NanoflowSource"},
			{Key: "NanoflowSettings", Value: bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Forms$NanoflowSettings"},
				{Key: "Nanoflow", Value: d.Nanoflow},
				{Key: "ParameterMappings", Value: bson.A{int32(3)}},
			}},
		}
	default:
		return nil
	}
}

// mdlTypeToBsonType converts an MDL type name to a BSON DataTypes$* type string.
func mdlTypeToBsonType(mdlType string) string {
	switch strings.ToLower(mdlType) {
	case "boolean":
		return "DataTypes$BooleanType"
	case "string":
		return "DataTypes$StringType"
	case "integer":
		return "DataTypes$IntegerType"
	case "long":
		return "DataTypes$LongType"
	case "decimal":
		return "DataTypes$DecimalType"
	case "datetime", "date":
		return "DataTypes$DateTimeType"
	default:
		return "DataTypes$ObjectType"
	}
}
