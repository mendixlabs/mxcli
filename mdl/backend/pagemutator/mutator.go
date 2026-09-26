// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
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
	// probe marks a discardable copy handed out by Probe(), which exists to be
	// written to and thrown away. Save refuses on one — see probe.go.
	probe bool
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
	switch containerType {
	case backend.ContainerSnippet:
		finder = findBsonWidgetInSnippet
	case backend.ContainerLayout:
		finder = findBsonWidgetInLayout
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
		return m.widgetNotFoundError(widgetRef)
	}
	// DataGrid2 columns are WidgetObjects, not form widgets — use the column setter.
	if len(result.colPropKeys) > 0 {
		if n := m.columnMatchCount(widgetRef); n > 1 {
			return columnAmbiguityError(widgetRef, n)
		}
		return setColumnPropertyMut(result.widget, result.colPropKeys, result.colPropKinds, prop, value)
	}
	return setRawWidgetPropertyMut(result.widget, prop, value)
}

func (m *Mutator) SetWidgetDataSource(widgetRef string, ds pages.DataSource) error {
	result := m.widgetFinder(m.rawData, widgetRef)
	if result == nil {
		return fmt.Errorf("widget %q not found", widgetRef)
	}
	// A parameter source names only the parameter; the entity it binds to lives
	// in the container's own Parameters list. Resolve it here rather than in the
	// executor — the mutator is the only layer holding the document (#855).
	if dv, ok := ds.(*pages.DataViewSource); ok && dv.ParameterName != "" && dv.EntityName == "" {
		entity, isSnippetParam, found := m.lookupParameter(dv.ParameterName)
		if !found {
			return fmt.Errorf("%s has no parameter %q", m.containerType, dv.ParameterName)
		}
		// Copy: the caller's value is not ours to mutate.
		resolved := *dv
		resolved.EntityName = entity
		resolved.IsSnippetParameter = isSnippetParam
		ds = &resolved
	}

	if err := databaseSourceRefusal(result.widget, ds); err != nil {
		return err
	}

	serialized := serializeDataSourceBson(ds)
	if serialized == nil {
		return fmt.Errorf("unsupported DataSource type %T", ds)
	}
	bsonnav.DSet(result.widget, "DataSource", serialized)
	return nil
}

// databaseSourceRefusal turns away `set DataSource = DATABASE …`, which this
// setter cannot write correctly for any widget (#1032).
//
// A DATABASE source is not one element but several, chosen by the widget that
// holds it: Forms$ListViewXPathSource on a list view,
// CustomWidgets$CustomWidgetXPathSource on a pluggable widget,
// Forms$GridXPathSource on a grid — each with its own sort bar and search
// sub-elements. A DATA VIEW has no database form at all, which is why the CREATE
// PAGE builder refuses that pairing outright.
//
// One mapping stood in for all of them and wrote a Forms$DataViewSource — the
// "data from context" source — with the entity in EntityRef and SourceVariable
// left null. Nothing rejected it: `exec` reported success, DESCRIBE read it back
// as no datasource at all (the context reader needs a SourceVariable), and the
// first signal was CE7007 from mxbuild, naming the widget rather than the
// statement that broke it.
//
// Refusing is what the two reads agree on. Rebuilding the shapes here would be a
// second copy of listViewSourceToGen and friends in a second currency — the
// drift CLAUDE.md's duplicate-resolver rule is about — while REPLACE already
// reaches the one that exists, by rebuilding the widget through CREATE PAGE.
func databaseSourceRefusal(widget bson.D, ds pages.DataSource) error {
	if _, ok := ds.(*pages.DatabaseSource); !ok {
		return nil
	}
	if bsonnav.DGetString(widget, "$Type") == "Forms$DataView" {
		// Not "use replace": REPLACE goes through the same CREATE PAGE builder,
		// which refuses a database source on a data view as well. Naming it
		// would send the author down a dead end.
		return fmt.Errorf("a data view cannot take a database datasource — a data view binds to a " +
			"single object, so its source is a context parameter (`$Param`), a microflow, a nanoflow " +
			"or `selection <widget>`; to show the result of a database query, use a list view or a " +
			"data grid instead")
	}
	return fmt.Errorf("setting a database datasource on %q (%s) is not supported by `set` — "+
		"its stored shape depends on the widget and is built by the CREATE PAGE path; "+
		"use `replace <widget> with …` instead, which rebuilds the widget through that path",
		bsonnav.DGetString(widget, "Name"), widgetTypeName(widget))
}

// SetWidgetAction retargets the on-click action of an existing widget.
//
// The action is serialized through the same engine hook CREATE PAGE uses, so
// every action form is available — not a subset maintained here. Before this,
// changing a button's action meant REPLACEing the whole widget, which silently
// drops any property the author did not restate.
func (m *Mutator) SetWidgetAction(widgetRef string, action pages.ClientAction) error {
	result := m.widgetFinder(m.rawData, widgetRef)
	if result == nil {
		return m.widgetNotFoundError(widgetRef)
	}
	// Refuse rather than write an Action onto something that has no such
	// property: Studio Pro resolves every stored property against the type's
	// property list and throws on one it does not know, while mxbuild's
	// deserializer tolerates it — so a silent write here builds clean and fails
	// to open.
	if bsonnav.DGet(result.widget, "Action") == nil {
		return fmt.Errorf("widget %q (%s) has no Action property — Action can only be set on a widget that "+
			"performs an on-click action, such as a button or a clickable container",
			widgetRef, widgetTypeName(result.widget))
	}
	serialized := m.deps.SerializeClientAction(action)
	if serialized == nil {
		return fmt.Errorf("unsupported action type %T", action)
	}
	bsonnav.DSet(result.widget, "Action", serialized)
	return nil
}

// SetWidgetNamedAction writes an action into a pluggable widget's action slot
// addressed by the widget's own property key — the ALTER-level twin of CREATE
// PAGE's `createFileAction: microflow M.F` (#956), so one mis-wired slot can be
// retargeted without REPLACEing the widget and restating everything else
// (mendixlabs/mxcli#995). It writes the same Value.Action field
// widgetobj.Builder.SetAction does.
//
// The property TYPE decides, not the presence of the field: every stored
// WidgetValue carries an Action (a NoAction by default) whatever its type, so
// writing one into an Integer property would build clean and do nothing.
func (m *Mutator) SetWidgetNamedAction(widgetRef, propertyKey string, action pages.ClientAction) error {
	result := m.widgetFinder(m.rawData, widgetRef)
	if result == nil {
		return m.widgetNotFoundError(widgetRef)
	}
	obj := bsonnav.DGetDoc(result.widget, "Object")
	if obj == nil {
		return fmt.Errorf("widget %q (%s) is not a pluggable widget and has no named action slots — "+
			"a built-in widget's click action is set with `set Action = … on %s`",
			widgetRef, widgetTypeName(result.widget), widgetRef)
	}

	keys, kinds := pluggablePropertyTypes(result.widget)
	var actionSlots []string
	for id, key := range keys {
		if kinds[id] == "Action" {
			actionSlots = append(actionSlots, key)
		}
	}
	sort.Strings(actionSlots)

	for _, prop := range bsonnav.DGetArrayElements(bsonnav.DGet(obj, "Properties")) {
		propDoc, ok := prop.(bson.D)
		if !ok {
			continue
		}
		id := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(propDoc, "TypePointer"))
		if key := keys[id]; key == "" || !strings.EqualFold(key, propertyKey) {
			continue
		}
		if kinds[id] != "Action" {
			return fmt.Errorf("property %q of widget %q has type %s, not Action — "+
				"its action slots are: %s", propertyKey, widgetRef, kinds[id], slotList(actionSlots))
		}
		valDoc := bsonnav.DGetDoc(propDoc, "Value")
		if valDoc == nil {
			return fmt.Errorf("property %q has no Value map", propertyKey)
		}
		serialized := m.deps.SerializeClientAction(action)
		if serialized == nil {
			return fmt.Errorf("unsupported action type %T", action)
		}
		bsonnav.DSet(valDoc, "Action", serialized)
		return nil
	}
	return fmt.Errorf("widget %q has no action slot %q — its action slots are: %s",
		widgetRef, propertyKey, slotList(actionSlots))
}

// pluggablePropertyTypes maps a pluggable widget's PropertyType IDs to their
// keys and to their value types ("Action", "Integer", …).
func pluggablePropertyTypes(widget bson.D) (keys, kinds map[string]string) {
	keys = buildPropKeyMap(widget)
	kinds = make(map[string]string, len(keys))
	objType := bsonnav.DGetDoc(bsonnav.DGetDoc(widget, "Type"), "ObjectType")
	for _, pt := range bsonnav.DGetArrayElements(bsonnav.DGet(objType, "PropertyTypes")) {
		ptDoc, ok := pt.(bson.D)
		if !ok {
			continue
		}
		id := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(ptDoc, "$ID"))
		if vt := bsonnav.DGetDoc(ptDoc, "ValueType"); vt != nil && id != "" {
			kinds[id] = bsonnav.DGetString(vt, "Type")
		}
	}
	return keys, kinds
}

func slotList(slots []string) string {
	if len(slots) == 0 {
		return "(none)"
	}
	return strings.Join(slots, ", ")
}

// widgetTypeName reports a widget's $Type for error messages, or "unknown type".
func widgetTypeName(widget bson.D) string {
	if t, ok := bsonnav.DGet(widget, "$Type").(string); ok && t != "" {
		return t
	}
	return "unknown type"
}

func (m *Mutator) SetColumnProperty(gridRef string, columnRef string, prop string, value any) error {
	result, err := findBsonColumn(m.rawData, gridRef, columnRef, m.widgetFinder)
	if err != nil {
		return err
	}
	return setColumnPropertyMut(result.widget, result.colPropKeys, result.colPropKinds, prop, value)
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

// objectListItemType is the $Type of an object-list item — a DataGrid2 column,
// an Accordion group, a PopupMenu basicItem. These live inside a pluggable
// widget's property tree, not in a Widgets array, and the generic widget
// insert/replace path cannot write one.
const objectListItemType = "CustomWidgets$WidgetObject"

// refuseObjectListItemTarget refuses an INSERT/REPLACE whose bare target resolved
// to an object-list item rather than a widget (#891).
//
// findBsonWidget recurses into a pluggable widget's internals, so a bare
// `NextRunAt` DOES resolve — to the grid column of that name. The op then took
// the generic widget path, which built the replacement as a layout container and
// wrote it into the grid's column list. Both INSERT and REPLACE reported success
// while leaving a project mxbuild could not even load:
//
//	System.InvalidCastException: Unable to cast object of type
//	'...LayoutWidgets.DivContainers.DivContainer' to type '...CustomWidgets.WidgetObject'
//
// DESCRIBE PAGE skipped the malformed node, which is why this looked like a
// clean deletion (REPLACE) or a no-op (INSERT).
//
// The dotted form `grid.column` routes to ReplaceColumn/InsertColumns and works,
// so this refuses and names that form rather than guessing which grid was meant.
// Guessing is what produced the invalid document.
func refuseObjectListItemTarget(result *bsonWidgetResult, name string) error {
	if result == nil || bsonnav.DGetString(result.widget, "$Type") != objectListItemType {
		return nil
	}
	return fmt.Errorf(
		"%q is a DataGrid2 column, not a widget — qualify it as `gridName.%s` so the column "+
			"path is used. Addressing it bare writes into the grid's column list as a layout "+
			"container, leaving a project Studio Pro cannot open",
		name, name)
}

// refuseWidgetsAtColumnTarget refuses an INSERT/REPLACE that would write plain
// widgets into a pluggable widget's object list (#935).
//
// This is the other half of #891, reached by the form that fix pointed authors
// at. A `grid.column` target resolves to a CustomWidgets$WidgetObject, and the
// executor routes it to InsertColumns/ReplaceColumn only when the body is
// entirely `column …` blocks. Any other body — a container, a text, a button —
// fell through to the generic widget path and was serialized into the grid's
// column list, producing the same document the loader cannot read:
//
//	System.InvalidCastException: Unable to cast object of type
//	'...LayoutWidgets.DivContainers.DivContainer' to type '...CustomWidgets.WidgetObject'
//
// So the guard cannot key on the *target* alone, the way #891's does — the
// target is legitimate here. What is wrong is the pairing: widgets reaching a
// path that only object-list items may enter. Refusing at the point of that
// pairing covers INSERT (before/after/into) and REPLACE at once.
//
// A cell's contents are still editable: #834 made the widgets inside a
// customContent column addressable by their own names, which is the form the
// message names.
func refuseWidgetsAtColumnTarget(gridRef, columnRef string) error {
	return fmt.Errorf(
		"%s.%s is a DataGrid2 column, so only a `column …` block may be written there — "+
			"a widget put in the grid's column list leaves a project Studio Pro cannot open. "+
			"To edit the cell's contents instead, target the widget inside it by its own name "+
			"(for example `insert into <containerName> { … }`); `describe page` lists them",
		gridRef, columnRef)
}

func (m *Mutator) InsertWidget(widgetRef string, columnRef string, position backend.InsertPosition, widgets []pages.Widget) error {
	if columnRef != "" {
		// `layoutContainer.top` is a scroll-container region, not a grid column.
		// A region has no Name — its slot is its identity — so the dotted ref is
		// the only way to address one, and it reuses the widgetRef the grammar
		// already has rather than inventing a syntax for five fixed positions.
		if handled, err := m.insertIntoScrollRegion(widgetRef, columnRef, position, widgets); handled {
			return err
		}
		// Resolve first, so a mistyped column still reports "not found" (with the
		// available names) rather than the refusal below.
		if _, err := findBsonColumn(m.rawData, widgetRef, columnRef, m.widgetFinder); err != nil {
			return err
		}
		return refuseWidgetsAtColumnTarget(widgetRef, columnRef)
	}
	result := m.widgetFinder(m.rawData, widgetRef)
	if result == nil {
		return m.widgetNotFoundError(widgetRef)
	}
	if err := refuseObjectListItemTarget(result, widgetRef); err != nil {
		return err
	}
	if n := m.columnMatchCount(widgetRef); n > 1 {
		return columnAmbiguityError(widgetRef, n)
	}

	// Serialize widgets
	newBsonWidgets, err := m.serializeWidgets(widgets)
	if err != nil {
		return fmt.Errorf("serialize widgets: %w", err)
	}

	// INSERT INTO: append the widgets as children of the target container itself
	// (its `Widgets` array), rather than as siblings in the target's parent array.
	// Enables inserting into an empty container or as a container's last child.
	if strings.EqualFold(string(position), "into") {
		newContainer, err := appendChildrenToContainer(result.widget, widgetRef, newBsonWidgets)
		if err != nil {
			return err
		}
		// Write the (possibly reallocated — an empty container gains a new Widgets
		// field) container doc back into its parent slot.
		result.parentArr[result.index] = newContainer
		bsonnav.DSetArray(result.parentDoc, result.parentKey, result.parentArr)
		return nil
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

// insertIntoContainer appends widgets as the last children of a container widget
// (its `Widgets` array). Simple containers (Pages$DivContainer, Forms$Container,
// DataView, GroupBox, ScrollContainer, …) keep their children in a single
// `Widgets` list. LayoutGrid (Rows/Columns) and TabContainer (TabPages) have no
// single child list, so INSERT INTO can't target them directly — the caller is
// told to insert relative to a widget inside the target column/tab instead.
// appendChildrenToContainer appends widgets to a container's `Widgets` list and
// returns the (possibly reallocated) container doc. An empty container omits the
// Widgets field entirely, so it is added — which grows the bson.D slice, hence the
// caller must store the returned doc back into its parent slot.
func appendChildrenToContainer(container bson.D, widgetRef string, newBsonWidgets []any) (bson.D, error) {
	if !containerAcceptsWidgets(container) {
		typeName := bsonnav.DGetString(container, "$Type")
		return nil, fmt.Errorf("cannot INSERT INTO %q (%s): it is not a simple container — "+
			"use INSERT BEFORE/AFTER a widget inside the target column or tab instead", widgetRef, typeName)
	}
	// Append after any existing children, preserving the Mendix list marker (a
	// leading int32). An empty container omits the Widgets field, so create it with
	// the default marker (2) that Mendix uses for widget lists.
	raw := bsonnav.ToBsonA(bsonnav.DGet(container, "Widgets"))
	var out bson.A
	switch {
	case len(raw) > 0 && isListMarker(raw[0]):
		out = append(out, raw...) // marker + existing children
		out = append(out, newBsonWidgets...)
	case len(raw) == 0:
		out = append(out, int32(2)) // fresh (empty container): Mendix widget-list marker
		out = append(out, newBsonWidgets...)
	default:
		out = append(out, raw...) // children without a marker (unusual): keep as-is
		out = append(out, newBsonWidgets...)
	}
	// DSet updates an existing field in place; if Widgets is absent (empty
	// container), append the field, growing the slice.
	if bsonnav.DSet(container, "Widgets", out) {
		return container, nil
	}
	return append(container, bson.E{Key: "Widgets", Value: out}), nil
}

// isListMarker reports whether a value is a Mendix list-version marker (a leading int).
func isListMarker(v any) bool {
	switch v.(type) {
	case int32, int:
		return true
	}
	return false
}

// containerAcceptsWidgets reports whether a widget keeps its children in a single
// `Widgets` list that INSERT INTO can append to. A container that currently holds
// children always has the field; an empty one omits it, so recognise the common
// simple-container types by $Type. LayoutGrid (Rows/Columns) and TabContainer
// (TabPages) are intentionally excluded — they have no single child list.
func containerAcceptsWidgets(container bson.D) bool {
	for _, e := range container {
		if e.Key == "Widgets" {
			return true
		}
	}
	switch bsonnav.DGetString(container, "$Type") {
	case "Forms$DivContainer",
		"Forms$Container",
		"Forms$DataView",
		"Forms$GroupBox",
		"Forms$ScrollContainerRegion",
		"Forms$Section":
		return true
	}
	return false
}

func (m *Mutator) DropWidget(refs []backend.WidgetRef) error {
	for _, ref := range refs {
		// Re-find widget each iteration because previous drops mutate the tree.
		var result *bsonWidgetResult
		if ref.IsColumn() {
			r, err := findBsonColumn(m.rawData, ref.Widget, ref.Column, m.widgetFinder)
			if err != nil {
				return err
			}
			result = r
		} else {
			result = m.widgetFinder(m.rawData, ref.Widget)
			if result == nil {
				return m.widgetNotFoundError(ref.Name())
			}
			if n := m.columnMatchCount(ref.Name()); n > 1 {
				return columnAmbiguityError(ref.Name(), n)
			}
		}
		newArr := make([]any, 0, len(result.parentArr)-1)
		newArr = append(newArr, result.parentArr[:result.index]...)
		newArr = append(newArr, result.parentArr[result.index+1:]...)
		bsonnav.DSetArray(result.parentDoc, result.parentKey, newArr)
	}
	return nil
}

func (m *Mutator) ReplaceWidget(widgetRef string, columnRef string, widgets []pages.Widget) error {
	if columnRef != "" {
		// Resolve first, so a mistyped column still reports "not found" (with the
		// available names) rather than the refusal below.
		if _, err := findBsonColumn(m.rawData, widgetRef, columnRef, m.widgetFinder); err != nil {
			return err
		}
		return refuseWidgetsAtColumnTarget(widgetRef, columnRef)
	}
	result := m.widgetFinder(m.rawData, widgetRef)
	if result == nil {
		return m.widgetNotFoundError(widgetRef)
	}
	if err := refuseObjectListItemTarget(result, widgetRef); err != nil {
		return err
	}
	if n := m.columnMatchCount(widgetRef); n > 1 {
		return columnAmbiguityError(widgetRef, n)
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
	result, err := findBsonColumn(m.rawData, gridRef, afterColumnRef, m.widgetFinder)
	if err != nil {
		return err
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
	result, err := findBsonColumn(m.rawData, gridRef, columnRef, m.widgetFinder)
	if err != nil {
		return err
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

// BoundPlaceholders returns the placeholder names this page binds content to.
//
// A FormCallArgument's Parameter is the placeholder's *qualified* name
// (Atlas_Core.Atlas_Default.Main), so the layout's own qualified name is
// stripped off the front — the last dot-separated segment is the placeholder.
func (m *Mutator) BoundPlaceholders() []string {
	formCall := bsonnav.DGetDoc(m.rawData, "FormCall")
	if formCall == nil {
		return nil
	}
	var out []string
	for _, item := range bsonnav.DGetArrayElements(bsonnav.DGet(formCall, "Arguments")) {
		doc, ok := item.(bson.D)
		if !ok {
			continue
		}
		p := bsonnav.DGetString(doc, "Parameter")
		if p == "" {
			continue
		}
		if i := strings.LastIndex(p, "."); i >= 0 {
			p = p[i+1:]
		}
		out = append(out, p)
	}
	return out
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

// EnclosingEntity returns the entity context that applies to a widget's
// SIBLINGS — what REPLACE and INSERT BEFORE/AFTER build against — i.e. the
// entity of the nearest enclosing data source.
func (m *Mutator) EnclosingEntity(widgetRef string) string {
	ds, _ := findNearestDataSourceDoc(m.rawData, widgetRef)
	return resolveSourceScope(m.rawData, ds).Entity
}

// EnclosingDataSourceFlow returns the microflow/nanoflow qualified name of the
// datasource that governs widgetRef's context, or "","" when that source is not
// a flow (database/association — EnclosingEntity/EnclosingEntityForChildren
// already resolve those, and a nearer non-flow source shadows an outer flow).
// A microflow/nanoflow datasource's entity is its RETURN type, which lives in
// the flow document rather than the datasource BSON, so the caller resolves the
// returned qualified name to an entity via the model. When forChildren is true
// the widget's OWN datasource is consulted (INSERT INTO / column inserts);
// otherwise the nearest ENCLOSING datasource (sibling INSERT BEFORE/AFTER,
// REPLACE). Without this a widget inserted into a flow-sourced list bound its
// attribute to nothing (CE0402/CE1613). (FINDINGS #55)
func (m *Mutator) EnclosingDataSourceFlow(widgetRef string, forChildren bool) (microflow, nanoflow string) {
	if forChildren {
		if result := m.widgetFinder(m.rawData, widgetRef); result != nil {
			if ds := widgetOwnDataSourceDoc(result.widget); ds != nil {
				scope := resolveSourceScope(m.rawData, ds)
				return scope.Microflow, scope.Nanoflow
			}
		}
	}
	ds, ok := findNearestDataSourceDoc(m.rawData, widgetRef)
	if !ok {
		return "", ""
	}
	scope := resolveSourceScope(m.rawData, ds)
	return scope.Microflow, scope.Nanoflow
}

// flowFromDataSourceDoc extracts the microflow/nanoflow qualified name from a
// widget's "DataSource" sub-document, or "","" when it is not a flow source.
func flowFromDataSourceDoc(ds bson.D) (microflow, nanoflow string) {
	if ds == nil {
		return "", ""
	}
	if s := bsonnav.DGetDoc(ds, "MicroflowSettings"); s != nil {
		return bsonnav.DGetString(s, "Microflow"), ""
	}
	// A Forms$NanoflowSource names its nanoflow directly (Studio Pro's shape);
	// the nested NanoflowSettings is what mxcli wrote before CE2633 was fixed,
	// and such pages are still out there.
	if nf := bsonnav.DGetString(ds, "Nanoflow"); nf != "" {
		return "", nf
	}
	if s := bsonnav.DGetDoc(ds, "NanoflowSettings"); s != nil {
		return "", bsonnav.DGetString(s, "Nanoflow")
	}
	return "", ""
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
	// A widget that declares a source of its own governs its children, even when
	// that source names no entity: a flow-sourced or selection-bound list whose
	// scope cannot be read here must SHADOW the enclosing entity rather than let
	// it be inherited — inheriting it is how a binding silently re-scoped to the
	// outer data view (#1076).
	if ds := widgetOwnDataSourceDoc(result.widget); ds != nil {
		return resolveSourceScope(m.rawData, ds).Entity
	}
	return m.EnclosingEntity(widgetRef)
}

// ---------------------------------------------------------------------------
// Data source resolution
// ---------------------------------------------------------------------------

// sourceScope is what one data source hands to the widgets under it: the entity
// it names, or the qualified name of the microflow/nanoflow whose RETURN type is
// that entity — which lives in the flow document, so only a caller holding the
// model can finish that half.
//
// Mendix has ten data source kinds and they divide exactly three ways: seven
// carry an EntityRef (Association, CustomWidgetXPath, DataView, GridXPath,
// ImageViewer, ListViewXPath, ReferenceSet — see the `DataSource is implemented
// by` list in generated/metamodel/types.go), two are flows (Microflow,
// Nanoflow), and one — ListenTargetSource — carries neither and borrows the
// scope of the widget it listens to. Every kind is therefore resolved here, and
// a source that still resolves to nothing means the model names nothing, not
// that this walk has another shape left to learn. Each time one kind was handled
// somewhere and not elsewhere, the result was a binding written against the
// wrong entity: association and flow sources (FINDINGS #55), pluggable widgets
// (#935), selection sources and pluggable flow sources (#1076).
type sourceScope struct {
	Entity    string
	Microflow string
	Nanoflow  string
}

// resolveSourceScope reads one widget's "DataSource" document. root is the whole
// unit, needed only to follow a selection source to its listen target.
func resolveSourceScope(root bson.D, ds bson.D) sourceScope {
	return resolveSourceScopeVia(root, ds, nil)
}

// resolveSourceScopeVia carries the listen targets already followed, so a
// selection chain that loops back on itself terminates instead of recursing
// forever. Studio Pro will not author that, but a hand-written ALTER can.
func resolveSourceScopeVia(root bson.D, ds bson.D, seen map[string]bool) sourceScope {
	if ds == nil {
		return sourceScope{}
	}
	if entity := entityFromEntityRef(bsonnav.DGetDoc(ds, "EntityRef")); entity != "" {
		return sourceScope{Entity: entity}
	}
	if mf, nf := flowFromDataSourceDoc(ds); mf != "" || nf != "" {
		return sourceScope{Microflow: mf, Nanoflow: nf}
	}
	// Forms$ListenTargetSource — `dataview dv (datasource: selection lv)`. It
	// stores the target widget's NAME and nothing else, so its scope is whatever
	// the target's own source resolves to, which may in turn be an association,
	// a flow or another selection.
	target := bsonnav.DGetString(ds, "ListenTarget")
	if target == "" || seen[target] {
		return sourceScope{}
	}
	if seen == nil {
		seen = make(map[string]bool, 2)
	}
	seen[target] = true
	return resolveSourceScopeVia(root, listenTargetDataSource(root, target), seen)
}

// listenTargetDataSource returns the data source of the widget a selection
// source names. A listen target is addressed by name and need not be a sibling
// of the listening widget, so it is searched for over the whole unit; keying the
// search on "carries this Name and a data source" rather than on a list of
// container shapes is what keeps it working for a pluggable list, whose source
// sits three levels inside its Object.
func listenTargetDataSource(root bson.D, name string) bson.D {
	var found bson.D
	var walk func(v any)
	walk = func(v any) {
		if found != nil {
			return
		}
		switch node := v.(type) {
		case bson.D:
			if bsonnav.DGetString(node, "Name") == name {
				if ds := widgetOwnDataSourceDoc(node); ds != nil {
					found = ds
					return
				}
			}
			for _, kv := range node {
				walk(kv.Value)
			}
		case bson.A:
			for _, elem := range node {
				walk(elem)
			}
		}
	}
	walk(root)
	return found
}

// widgetOwnDataSourceDoc returns the widget's own "DataSource" document,
// wherever its kind keeps it: at the top level for a plain Forms$ widget, and
// under Object.Properties[datasource].Value for a pluggable one (Gallery,
// DataGrid 2). Reading only the first is what made a flow-sourced gallery
// contribute nothing, so a widget replaced inside its template was written with
// no binding at all (#1076).
func widgetOwnDataSourceDoc(wDoc bson.D) bson.D {
	if ds := bsonnav.DGetDoc(wDoc, "DataSource"); ds != nil {
		return ds
	}
	return pluggableDataSourceDoc(wDoc)
}

// pluggableDataSourceDoc walks a CustomWidget's Object.Properties[] for the
// property the widget's schema keys "datasource", and returns its DataSource
// document.
func pluggableDataSourceDoc(widgetDoc bson.D) bson.D {
	obj := bsonnav.DGetDoc(widgetDoc, "Object")
	if obj == nil {
		return nil
	}
	propKeyMap := buildPropKeyMap(widgetDoc)
	if len(propKeyMap) == 0 {
		return nil
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
		if dsDoc := bsonnav.DGetDoc(valDoc, "DataSource"); dsDoc != nil {
			return dsDoc
		}
	}
	return nil
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
	if m.probe {
		return fmt.Errorf("refusing to save a dry-run copy of this %s", m.containerType)
	}
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
	// colPropKinds maps the same TypePointer ids to the value kind the schema
	// declares (Expression, TextTemplate, Boolean, …). Without it a setter
	// cannot tell which field of a WidgetValue to write — see columnValueField.
	colPropKinds map[string]string
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

// findBsonWidgetInLayout searches a Forms$Layout for a widget by name.
//
// A layout's tree hangs off Content — a Forms$WebLayoutContent (or
// Forms$NativeLayoutContent) — never off the layout element and never off a
// FormCall, so the page finder returns nil for every layout.
func findBsonWidgetInLayout(rawData bson.D, widgetName string) *bsonWidgetResult {
	content := bsonnav.DGetDoc(rawData, "Content")
	if content == nil {
		return nil
	}
	return findInWidgetArray(content, "Widgets", widgetName)
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

	// ScrollContainer: five named slots, not a list — Top, Right, Bottom, Left
	// and CenterRegion, the last spelled unlike its siblings. Without this a
	// layout's topbar and navigation are unreachable (they live in Top and
	// Left), and so is anything inside a scroll container a page places itself.
	for _, slot := range ScrollRegionSlots {
		region := bsonnav.DGetDoc(wDoc, slot)
		if region == nil {
			continue
		}
		// findInWidgetArray already descends through findInWidgetChildren, so
		// this reaches arbitrarily deep inside a region.
		if result := findInWidgetArray(region, "Widgets", widgetName); result != nil {
			return result
		}
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
				colPropKeyMap, colPropKindMap := buildColumnPropKeyMap(wDoc, typePointerID)
				columns := bsonnav.DGetArrayElements(bsonnav.DGet(valDoc, "Objects"))
				for i, colItem := range columns {
					colDoc, ok := colItem.(bson.D)
					if !ok {
						continue
					}
					if deriveColumnNameBson(colDoc, colPropKeyMap, i) == widgetName {
						return &bsonWidgetResult{
							widget:       colDoc,
							parentArr:    columns,
							parentKey:    "Objects",
							parentDoc:    valDoc,
							index:        i,
							colPropKeys:  colPropKeyMap,
							colPropKinds: colPropKindMap,
						}
					}
					// Descend into the column's OWN content widgets. A column
					// rendered as customContent holds a widget tree at
					// Properties[content].Value.Widgets — one level deeper than the
					// pluggable search above, which only reaches the grid's own
					// Object.Properties[].Value.Widgets. Without this, a widget
					// inside a customContent column was unreachable by ALTER PAGE
					// and the only remedy was rewriting the page (issue #834).
					for _, cProp := range bsonnav.DGetArrayElements(bsonnav.DGet(colDoc, "Properties")) {
						cPropDoc, ok := cProp.(bson.D)
						if !ok {
							continue
						}
						cValDoc := bsonnav.DGetDoc(cPropDoc, "Value")
						if cValDoc == nil {
							continue
						}
						if result := findInWidgetArray(cValDoc, "Widgets", widgetName); result != nil {
							return result
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
// findBsonColumn locates a DataGrid2 column inside a grid by its *derived* name.
//
// DataGrid2 columns carry no stored name in the Mendix model — mxcli addresses
// them by a name derived from their content: the bound attribute for an
// attribute column, the caption otherwise, falling back to col{N}
// (deriveColumnNameBson). Two consequences the caller must surface rather than
// paper over (ledger #78):
//
//   - the authored MDL name (`column colFoo (...)`) never survives a write, so
//     addressing a column by it fails — report the derived names that DO work;
//   - duplicate captions derive the same name, so `ON "Amount"` can match more
//     than one column. Silently mutating the first is a data hazard; reject the
//     ambiguity instead.
//
// Returns a non-nil error (and nil result) when the grid/column can't be
// resolved unambiguously; the error is actionable (lists available columns, or
// names the ambiguity).
func findBsonColumn(rawData bson.D, gridName, columnName string, find widgetFinder) (*bsonWidgetResult, error) {
	gridResult := find(rawData, gridName)
	if gridResult == nil {
		return nil, fmt.Errorf("widget %q not found", gridName)
	}

	gridPropKeyMap := buildPropKeyMap(gridResult.widget)

	obj := bsonnav.DGetDoc(gridResult.widget, "Object")
	if obj == nil {
		return nil, fmt.Errorf("widget %q has no columns (not a DataGrid2)", gridName)
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
			return nil, fmt.Errorf("column %q on grid %q not found", columnName, gridName)
		}

		colPropKeyMap, colPropKindMap := buildColumnPropKeyMap(gridResult.widget, typePointerID)

		columns := bsonnav.DGetArrayElements(bsonnav.DGet(valDoc, "Objects"))
		var matches []*bsonWidgetResult
		available := make([]string, 0, len(columns))
		for i, colItem := range columns {
			colDoc, ok := colItem.(bson.D)
			if !ok {
				continue
			}
			derived := deriveColumnNameBson(colDoc, colPropKeyMap, i)
			available = append(available, derived)
			if derived == columnName {
				matches = append(matches, &bsonWidgetResult{
					widget:       colDoc,
					parentArr:    columns,
					parentKey:    "Objects",
					parentDoc:    valDoc,
					index:        i,
					colPropKeys:  colPropKeyMap,
					colPropKinds: colPropKindMap,
				})
			}
		}
		switch len(matches) {
		case 1:
			return matches[0], nil
		case 0:
			return nil, fmt.Errorf(
				"column %q on grid %q not found — columns are addressed by a derived name "+
					"(the bound attribute for an attribute column, the caption otherwise), "+
					"not the name written in MDL; available columns: %s",
				columnName, gridName, formatColumnNameList(available))
		default:
			return nil, fmt.Errorf(
				"column %q on grid %q is ambiguous: %d columns derive that name. "+
					"Dynamic-text and custom-content columns have no stored name and are keyed by "+
					"their caption, so identical captions collide — give them distinct captions to "+
					"address them individually",
				columnName, gridName, len(matches))
		}
	}
	return nil, fmt.Errorf("widget %q has no columns (not a DataGrid2)", gridName)
}

// columnAmbiguityError builds the error for a bare `ON <name>` that resolves to
// more than one DataGrid2 column. Shared by the explicit-column path
// (findBsonColumn, within-grid) and the bare-name path (columnMatchCount,
// page-wide — covers duplicates across two grids too).
func columnAmbiguityError(name string, count int) error {
	return fmt.Errorf(
		"column %q is ambiguous: %d columns on this page derive that name — either "+
			"identical captions in one grid (dynamic-text/custom columns are keyed by "+
			"caption), or a same-named column in more than one grid. Give the columns "+
			"distinct captions, or qualify the reference as `ON gridName.%s`, to address one",
		name, count, name)
}

// collectColumnNamesBson walks the raw page/snippet tree and appends the derived
// name of every DataGrid2 column it finds, so a "not found" error can list the
// names that actually work (the authored MDL name never survives a write).
func collectColumnNamesBson(node any, out *[]string) {
	switch v := node.(type) {
	case bson.D:
		*out = append(*out, gridColumnNames(v)...)
		for _, e := range v {
			collectColumnNamesBson(e.Value, out)
		}
	case bson.A:
		for _, e := range v {
			collectColumnNamesBson(e, out)
		}
	}
}

// columnMatchCount counts how many DataGrid2 columns anywhere on the page derive
// the given name. >1 means a bare `ON <name>` is ambiguous — whether the
// duplicates sit in the same grid (identical captions) or in two different grids
// on the page — and mutating the first silently is a data hazard (ledger #78).
func (m *Mutator) columnMatchCount(name string) int {
	var cols []string
	collectColumnNamesBson(m.rawData, &cols)
	n := 0
	for _, c := range cols {
		if c == name {
			n++
		}
	}
	return n
}

// gridColumnNames returns the derived names of a DataGrid2's columns, or nil if
// the node is not a grid with a columns property.
func gridColumnNames(wDoc bson.D) []string {
	obj := bsonnav.DGetDoc(wDoc, "Object")
	if obj == nil {
		return nil
	}
	propKeyMap := buildPropKeyMap(wDoc)
	for _, prop := range bsonnav.DGetArrayElements(bsonnav.DGet(obj, "Properties")) {
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
			return nil
		}
		colPropKeyMap, _ := buildColumnPropKeyMap(wDoc, typePointerID)
		var names []string
		for i, colItem := range bsonnav.DGetArrayElements(bsonnav.DGet(valDoc, "Objects")) {
			if colDoc, ok := colItem.(bson.D); ok {
				names = append(names, deriveColumnNameBson(colDoc, colPropKeyMap, i))
			}
		}
		return names
	}
	return nil
}

// widgetNotFoundError builds a "not found" error for a bare widget/column
// reference. When the page carries DataGrid2 columns, it adds the addressable
// column names — columns are keyed by a derived name (attribute or caption), not
// the authored MDL name, which is the usual cause of the miss (ledger #78).
func (m *Mutator) widgetNotFoundError(name string) error {
	var cols []string
	collectColumnNamesBson(m.rawData, &cols)
	if len(cols) > 0 {
		return fmt.Errorf(
			"widget %q not found. DataGrid2 columns are addressed by a derived name "+
				"(the bound attribute, or the caption), not the name written in MDL — "+
				"available columns: %s (run DESCRIBE PAGE to confirm)",
			name, formatColumnNameList(cols))
	}
	return fmt.Errorf("widget %q not found", name)
}

// formatColumnNameList renders derived column names for an error message: each
// unique name once, in first-seen order, quoted when it isn't a bare identifier
// (so a caption-derived name with spaces reads as the `ON "..."` form the user
// must type).
func formatColumnNameList(names []string) string {
	seen := make(map[string]bool, len(names))
	parts := make([]string, 0, len(names))
	for _, n := range names {
		if seen[n] {
			continue
		}
		seen[n] = true
		if n == sanitizeColumnName(n) && n != "" {
			parts = append(parts, n)
		} else {
			parts = append(parts, fmt.Sprintf("%q", n))
		}
	}
	if len(parts) == 0 {
		return "(none)"
	}
	return strings.Join(parts, ", ")
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

// buildPropKindMap builds a TypePointer ID -> declared value kind map
// (PropertyTypes[].ValueType.Type) for a widget's top-level properties. Kept
// apart from buildPropKeyMap, which has many callers that want only the key.
func buildPropKindMap(widgetDoc bson.D) map[string]string {
	m := make(map[string]string)
	objType := bsonnav.DGetDoc(bsonnav.DGetDoc(widgetDoc, "Type"), "ObjectType")
	if objType == nil {
		return m
	}
	for _, pt := range bsonnav.DGetArrayElements(bsonnav.DGet(objType, "PropertyTypes")) {
		ptDoc, ok := pt.(bson.D)
		if !ok {
			continue
		}
		id := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(ptDoc, "$ID"))
		if id == "" {
			continue
		}
		if kind := bsonnav.DGetString(bsonnav.DGetDoc(ptDoc, "ValueType"), "Type"); kind != "" {
			m[id] = kind
		}
	}
	return m
}

// buildColumnPropKeyMap builds a TypePointer ID -> PropertyKey map for column properties.
func buildColumnPropKeyMap(widgetDoc bson.D, columnsTypePointerID string) (map[string]string, map[string]string) {
	m := make(map[string]string)
	kinds := make(map[string]string)
	widgetType := bsonnav.DGetDoc(widgetDoc, "Type")
	if widgetType == nil {
		return m, kinds
	}
	objType := bsonnav.DGetDoc(widgetType, "ObjectType")
	if objType == nil {
		return m, kinds
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
			return m, kinds
		}
		colObjType := bsonnav.DGetDoc(valType, "ObjectType")
		if colObjType == nil {
			return m, kinds
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
				if cvt := bsonnav.DGetDoc(cptDoc, "ValueType"); cvt != nil {
					kinds[cid] = bsonnav.DGetString(cvt, "Type")
				}
			}
		}
		return m, kinds
	}
	return m, kinds
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

// findNearestDataSourceDoc returns the "DataSource" sub-document of the NEAREST
// container enclosing widgetName that declares one, and whether widgetName was
// found at all. Unlike findEnclosingEntityContext — which resolves to an entity
// NAME and so cannot distinguish "found, but the source has no directly-readable
// entity" (a flow source) from "not found in this branch" — this returns the raw
// source doc, letting the caller resolve flow sources via the model. curDS
// carries the nearest enclosing DataSource seen so far, so a nearer non-flow
// source correctly shadows an outer flow.
func findNearestDataSourceDoc(rawData bson.D, widgetName string) (bson.D, bool) {
	if formCall := bsonnav.DGetDoc(rawData, "FormCall"); formCall != nil {
		for _, arg := range bsonnav.DGetArrayElements(bsonnav.DGet(formCall, "Arguments")) {
			argDoc, ok := arg.(bson.D)
			if !ok {
				continue
			}
			if ds, found := findNearestDSInWidgets(argDoc, "Widgets", widgetName, nil); found {
				return ds, true
			}
		}
	}
	if ds, found := findNearestDSInWidgets(rawData, "Widgets", widgetName, nil); found {
		return ds, true
	}
	if widgetContainer := bsonnav.DGetDoc(rawData, "Widget"); widgetContainer != nil {
		if ds, found := findNearestDSInWidgets(widgetContainer, "Widgets", widgetName, nil); found {
			return ds, true
		}
	}
	return nil, false
}

func findNearestDSInWidgets(parentDoc bson.D, key string, widgetName string, curDS bson.D) (bson.D, bool) {
	for _, elem := range bsonnav.DGetArrayElements(bsonnav.DGet(parentDoc, key)) {
		wDoc, ok := elem.(bson.D)
		if !ok {
			continue
		}
		if bsonnav.DGetString(wDoc, "Name") == widgetName {
			return curDS, true
		}
		childDS := curDS
		if ds := widgetOwnDataSourceDoc(wDoc); ds != nil {
			childDS = ds
		}
		if ds, found := findNearestDSInChildren(wDoc, widgetName, childDS); found {
			return ds, true
		}
	}
	return nil, false
}

func findNearestDSInChildren(wDoc bson.D, widgetName string, curDS bson.D) (bson.D, bool) {
	typeName := bsonnav.DGetString(wDoc, "$Type")
	if ds, found := findNearestDSInWidgets(wDoc, "Widgets", widgetName, curDS); found {
		return ds, true
	}
	if ds, found := findNearestDSInWidgets(wDoc, "FooterWidgets", widgetName, curDS); found {
		return ds, true
	}
	if strings.Contains(typeName, "LayoutGrid") {
		for _, row := range bsonnav.DGetArrayElements(bsonnav.DGet(wDoc, "Rows")) {
			rowDoc, ok := row.(bson.D)
			if !ok {
				continue
			}
			for _, col := range bsonnav.DGetArrayElements(bsonnav.DGet(rowDoc, "Columns")) {
				colDoc, ok := col.(bson.D)
				if !ok {
					continue
				}
				if ds, found := findNearestDSInWidgets(colDoc, "Widgets", widgetName, curDS); found {
					return ds, true
				}
			}
		}
	}
	for _, tp := range bsonnav.DGetArrayElements(bsonnav.DGet(wDoc, "TabPages")) {
		tpDoc, ok := tp.(bson.D)
		if !ok {
			continue
		}
		if ds, found := findNearestDSInWidgets(tpDoc, "Widgets", widgetName, curDS); found {
			return ds, true
		}
	}
	if controlBar := bsonnav.DGetDoc(wDoc, "ControlBar"); controlBar != nil {
		if ds, found := findNearestDSInWidgets(controlBar, "Items", widgetName, curDS); found {
			return ds, true
		}
	}
	if strings.Contains(typeName, "CustomWidget") {
		if obj := bsonnav.DGetDoc(wDoc, "Object"); obj != nil {
			for _, prop := range bsonnav.DGetArrayElements(bsonnav.DGet(obj, "Properties")) {
				propDoc, ok := prop.(bson.D)
				if !ok {
					continue
				}
				valDoc := bsonnav.DGetDoc(propDoc, "Value")
				if valDoc == nil {
					continue
				}
				if ds, found := findNearestDSInWidgets(valDoc, "Widgets", widgetName, curDS); found {
					return ds, true
				}
				// One level deeper: an object-list item (a DataGrid 2 column, an
				// Accordion group) keeps its widgets at Objects[].Properties[]
				// .Value.Widgets. The entity walk gained this descent in #935 and
				// this one did not, so a widget in a customContent cell under a
				// FLOW-sourced grid was never reached at all.
				for _, item := range bsonnav.DGetArrayElements(bsonnav.DGet(valDoc, "Objects")) {
					itemDoc, ok := item.(bson.D)
					if !ok {
						continue
					}
					for _, itemProp := range bsonnav.DGetArrayElements(bsonnav.DGet(itemDoc, "Properties")) {
						itemPropDoc, ok := itemProp.(bson.D)
						if !ok {
							continue
						}
						itemValDoc := bsonnav.DGetDoc(itemPropDoc, "Value")
						if itemValDoc == nil {
							continue
						}
						if ds, found := findNearestDSInWidgets(itemValDoc, "Widgets", widgetName, curDS); found {
							return ds, true
						}
					}
				}
			}
		}
	}
	return nil, false
}

// entityFromEntityRef resolves a datasource's EntityRef to an entity name,
// covering both shapes Mendix stores. Shared by the plain-widget and pluggable
// readers: the pluggable one handled only the direct form, so an
// association-bound DataGrid2/Gallery reported no entity at all.
func entityFromEntityRef(entityRef bson.D) string {
	if entityRef == nil {
		return ""
	}
	// DirectEntityRef (database source): the entity is named directly.
	if entity := bsonnav.DGetString(entityRef, "Entity"); entity != "" {
		return entity
	}
	// IndirectEntityRef (association source, e.g. a ListView bound
	// `from association`): the destination entity lives on the LAST
	// EntityRefStep, not at EntityRef.Entity. Without this, descending into
	// an association-bound list left the context entity unchanged, so a
	// bare attribute inserted via ALTER PAGE resolved against the wrong
	// (outer) entity and failed the build with CE1613 (FINDINGS #55).
	return lastStepDestinationEntity(entityRef)
}

// lastStepDestinationEntity returns the DestinationEntity of the final
// DomainModels$EntityRefStep in an IndirectEntityRef's Steps array (the entity
// an association path ultimately lands on). The Steps array is
// `[<count>, step, step, ...]` — a leading numeric marker followed by the step
// documents — so non-document elements (the marker) are skipped. Returns "" if
// there are no step documents.
func lastStepDestinationEntity(entityRef bson.D) string {
	dest := ""
	for _, elem := range bsonnav.DGetArrayElements(bsonnav.DGet(entityRef, "Steps")) {
		stepDoc, ok := elem.(bson.D)
		if !ok {
			continue
		}
		if d := bsonnav.DGetString(stepDoc, "DestinationEntity"); d != "" {
			dest = d
		}
	}
	return dest
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
				colPropKeyMap, _ := buildColumnPropKeyMap(wDoc, typePointerID)
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

// resolveColumnPropertyKey maps a user-facing MDL property name onto the schema
// key the column document actually declares.
//
// It resolves against the keys in propKeyMap — read from the widget's own Type
// document — rather than a hand-written list. That is the whole point: the list
// this replaced was a second copy of the create path's alias table, and the two
// drifted (mendixlabs/mxcli#919). Matching what the document declares means a
// property the create path can write is one ALTER can write, by construction.
//
// Two ways to match, in order: the schema key itself, case-insensitively
// (`Sortable` → `sortable`), then the genuine renames in types.ItemPropertyAliases
// (`DynamicCellClass` → `columnClass`).
func resolveColumnPropertyKey(propName string, propKeyMap map[string]string) string {
	want := strings.ToLower(propName)

	declared := make(map[string]bool, len(propKeyMap))
	for _, key := range propKeyMap {
		declared[key] = true
		if strings.EqualFold(key, propName) {
			return key
		}
	}

	for schemaKey, aliases := range types.ItemPropertyAliasesFor(types.DataGridWidgetID, types.DataGridColumnsKey) {
		if !declared[schemaKey] {
			continue
		}
		for _, alias := range aliases {
			if strings.ToLower(alias) == want {
				return schemaKey
			}
		}
	}
	return ""
}

// settableColumnProperties lists what ALTER can set on this column, for an error
// message. Derived from the document, so it is accurate for the widget version
// actually installed rather than for the one mxcli was built against.
func settableColumnProperties(propKeyMap map[string]string) string {
	seen := make(map[string]bool, len(propKeyMap))
	names := make([]string, 0, len(propKeyMap))
	for _, key := range propKeyMap {
		if seen[key] {
			continue
		}
		seen[key] = true
		names = append(names, key)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// columnValueField decides which field of a WidgetValue a property's value
// belongs in, from the value kind the widget schema declares for it.
//
// This has to come from the schema and cannot be inferred from the stored
// document: a WidgetValue carries *every* field at once — Expression,
// PrimitiveValue, TextTemplate, AttributeRef and the rest — with the unused ones
// empty. So "which key is present" says nothing, and the previous code, which
// always wrote PrimitiveValue, silently put expression values in the wrong field.
// `SET DynamicCellClass` and `SET Visible` both reported success, wrote a value
// Studio Pro does not read, did not survive a DESCRIBE round trip, and left
// `mx check` at 0 errors.
//
// Shared by setPluggableWidgetPropertyMut, which had the same always-
// PrimitiveValue bug for a widget's own properties (mendixlabs/mxcli#1201).
//
// The second return reports whether the kind is settable at all. Attribute,
// datasource, action and widget-valued properties need a structured value, not a
// string, so ALTER refuses them rather than writing a plausible-looking wrong one.
func columnValueField(kind string) (string, bool) {
	switch kind {
	case "Expression":
		return "Expression", true
	case "TextTemplate":
		return "TextTemplate", true
	case "Attribute", "Association", "DataSource", "Action", "Widgets", "Object", "Form", "Image", "Icon", "Microflow", "Nanoflow", "Selection":
		return "", false
	default:
		// Boolean, String, Integer, Decimal, Enumeration, … — the primitives.
		return "PrimitiveValue", true
	}
}

func setColumnPropertyMut(colDoc bson.D, propKeyMap map[string]string, propKindMap map[string]string, propName string, value any) error {
	// No column property takes a list. A bracketed value arrives as a []string,
	// and the %v below turned it into `[if$x/Ythen'a'else'b']` — the tokens
	// already fused by the visitor — written into Expression as success.
	if _, isList := value.([]string); isList {
		return errExpressionNotAString(propName, value)
	}
	internalKey := resolveColumnPropertyKey(propName, propKeyMap)
	if internalKey == "" {
		return fmt.Errorf("column property %q not found — settable column properties on this grid are: %s",
			propName, settableColumnProperties(propKeyMap))
	}

	props := bsonnav.DGetArrayElements(bsonnav.DGet(colDoc, "Properties"))
	for _, prop := range props {
		propDoc, ok := prop.(bson.D)
		if !ok {
			continue
		}
		typePointerID := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(propDoc, "TypePointer"))
		if propKeyMap[typePointerID] != internalKey {
			continue
		}
		valDoc := bsonnav.DGetDoc(propDoc, "Value")
		if valDoc == nil {
			return fmt.Errorf("column property %q has no Value", propName)
		}

		field, settable := columnValueField(propKindMap[typePointerID])
		if !settable {
			return fmt.Errorf(
				"column property %q holds a value of kind %s, which ALTER cannot set from a plain value — "+
					"rewrite the column with CREATE OR REPLACE PAGE instead",
				propName, propKindMap[typePointerID])
		}

		strVal := fmt.Sprintf("%v", value)
		if field == "TextTemplate" {
			// The text lives inside Forms$ClientTemplate → Texts$Text →
			// Items[Translation].Text, not in the field itself.
			if textTemplate := bsonnav.DGetDoc(valDoc, "TextTemplate"); textTemplate != nil {
				if updateClientTemplateText(textTemplate, strVal) {
					return nil
				}
			}
			return fmt.Errorf("column property %q has no text template to update", propName)
		}
		bsonnav.DSet(valDoc, field, strVal)
		return nil
	}
	return fmt.Errorf("column property %q not found on this column", propName)
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
		{Key: "LanguageCode", Value: model.AuthoringLanguage()},
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
	case "Documentation":
		// A plain top-level string, the same shape as Url, and declared on
		// Page, Layout and Snippet alike — all three reach this function
		// through SetWidgetProperty(""), so one case covers them.
		//
		// Without it, documenting an existing page meant re-running its CREATE
		// (the doc comment is the only other source), which for a real page
		// means re-emitting its whole widget tree through a describe → exec
		// round trip that is only as complete as what MDL can spell
		// (ako/mxcli#527).
		//
		// An empty string is stored rather than rejected: removing a doc
		// comment from a script has to be expressible, and the property is a
		// bare string with no unset value.
		strVal, ok := value.(string)
		if !ok {
			return rawData, fmt.Errorf("Documentation value must be a string")
		}
		rawData = dSetOrAppend(rawData, "Documentation", strVal)
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
			"(supported: Title, Url, Documentation, PopupWidth, PopupHeight, PopupResizable, "+
			"PopupCloseAction, Class, Style)", prop)
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
		{Key: "LanguageCode", Value: model.AuthoringLanguage()},
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
		// Attribute is a BY_NAME AttributeIdentifier: unset is the empty string,
		// NOT null. A null fails the reader with StorageLoadException "…has an
		// invalid value '' for property Attribute" and the project will not open,
		// while `mx check` still passes. The CREATE path encodes it the same way
		// via the Forms$Conditional{Visibility,Editability}Settings TypeDefaults
		// (mdl/backend/modelsdk/widget_write.go, EmptyStringFields); this ALTER
		// path builds the node by hand and has to match. Issue #851.
		{Key: "Attribute", Value: ""},
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
	// Property names arrive verbatim from MDL (any case) — `set class on …` is as
	// valid as `set Class on …`, and `create page` reads them case-insensitively
	// (WidgetV3.GetStringProp). Match the first-class properties case-insensitively
	// so the ALTER path behaves the same. The pluggable fallback (default) passes
	// the author's spelling through and resolves it case-insensitively against the
	// widget's own template keys — see setPluggableWidgetPropertyMut.
	switch strings.ToLower(propName) {
	case "caption":
		return setWidgetCaptionMut(widget, value)
	case "content":
		return setWidgetContentMut(widget, value)
	case "label":
		return setWidgetLabelMut(widget, value)
	case "buttonstyle":
		if s, ok := value.(string); ok {
			bsonnav.DSet(widget, "ButtonStyle", s)
		}
		return nil
	case "class":
		if appearance := bsonnav.DGetDoc(widget, "Appearance"); appearance != nil {
			if s, ok := value.(string); ok {
				bsonnav.DSet(appearance, "Class", s)
			}
		}
		return nil
	case "style":
		if appearance := bsonnav.DGetDoc(widget, "Appearance"); appearance != nil {
			if s, ok := value.(string); ok {
				bsonnav.DSet(appearance, "Style", s)
			}
		}
		return nil
	case "dynamicclasses":
		// One expression. A bracketed `[ … ]` — the spelling mendixlabs/mxcli#750
		// proposes — arrives as a []string; it used to fall past the type check
		// and return nil, reporting success with nothing written.
		s, ok := value.(string)
		if !ok {
			return errExpressionNotAString("DynamicClasses", value)
		}
		if appearance := bsonnav.DGetDoc(widget, "Appearance"); appearance != nil {
			bsonnav.DSet(appearance, "DynamicClasses", s)
		}
		return nil
	case "editable":
		if s, ok := value.(string); ok {
			bsonnav.DSet(widget, "Editable", s)
		}
		return nil
	case "visible":
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
	case "visibleif":
		// Conditional visibility expression (issue #627): replace the widget's
		// ConditionalVisibilitySettings node (null when unset) with one carrying
		// the rooted expression the visitor produced.
		expr, _ := value.(string)
		if !setWidgetConditionalSettingMut(widget, "ConditionalVisibilitySettings",
			"Forms$ConditionalVisibilitySettings", expr, true) {
			return fmt.Errorf("widget does not support conditional visibility")
		}
		return nil
	case "editableif":
		expr, _ := value.(string)
		if !setWidgetConditionalSettingMut(widget, "ConditionalEditabilitySettings",
			"Forms$ConditionalEditabilitySettings", expr, false) {
			return fmt.Errorf("widget does not support conditional editability (only input widgets are editable)")
		}
		return nil
	case "name":
		if s, ok := value.(string); ok {
			bsonnav.DSet(widget, "Name", s)
		}
		return nil
	case "attribute":
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
		return writeDesignProperties(widget, key, elements)
	}

	entry := bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: designPropertyEntryType},
		{Key: "Key", Value: key},
		{Key: "Value", Value: buildDesignPropertyValueDoc(valueType, option)},
	}
	return writeDesignProperties(widget, key, append(elements, entry))
}

// designPropertiesMarker is the typed-array marker Studio Pro writes on a
// Forms$Appearance's DesignProperties list — measured on every page of a blank
// 11.13 app, empty lists included.
const designPropertiesMarker = 3

// writeDesignProperties stores the widget's design-property entries, creating the
// Appearance.DesignProperties array when the widget does not carry one yet.
//
// It goes through DSetArrayIn rather than DSetArray because a widget mxcli
// authored may have no DesignProperties key at all, and DSet cannot add one: the
// write was a silent no-op while ALTER STYLING still reported success
// (upstream #931).
func writeDesignProperties(widget bson.D, key string, elements []any) error {
	if !bsonnav.DSetArrayIn(widget, "Appearance", "DesignProperties", elements, designPropertiesMarker) {
		return fmt.Errorf("could not store design property %q on this widget", key)
	}
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
	return writeDesignProperties(widget, key, kept)
}

// clearDesignPropertiesMut removes all design properties from the widget,
// leaving an empty (marker-only) array.
func clearDesignPropertiesMut(widget bson.D) error {
	appearance := bsonnav.DGetDoc(widget, "Appearance")
	if appearance == nil {
		return nil
	}
	return writeDesignProperties(widget, "", nil)
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
	if caption := bsonnav.DGetDoc(widget, "Caption"); caption != nil {
		setTranslatableText(caption, "", value)
		return nil
	}
	// An ActionButton has no `Caption` document: its caption is a
	// Forms$ClientTemplate stored under `CaptionTemplate` (Template → Items[] →
	// Translation.Text), the same shape setWidgetContentMut walks. Without this
	// branch `alter page … set Caption = '…' on <button>` failed with "widget has
	// no Caption property" for EVERY action button, nested or top-level.
	if tmpl := bsonnav.DGetDoc(widget, "CaptionTemplate"); tmpl != nil {
		return setClientTemplateText(tmpl, "CaptionTemplate", value)
	}
	return mdlerrors.NewValidation("widget has no Caption property")
}

// setClientTemplateText writes the literal text of a Forms$ClientTemplate
// (Template → Items[] → Translation.Text). Shared by the caption and content
// setters, which store their text in the same structure under different keys.
func setClientTemplateText(clientTemplate bson.D, label string, value any) error {
	strVal, ok := value.(string)
	if !ok {
		return fmt.Errorf("%s value must be a string", label)
	}
	template := bsonnav.DGetDoc(clientTemplate, "Template")
	if template == nil {
		return fmt.Errorf("%s has no Template", label)
	}
	items := bsonnav.DGetArrayElements(bsonnav.DGet(template, "Items"))
	if len(items) > 0 {
		if itemDoc, ok := items[0].(bson.D); ok {
			bsonnav.DSet(itemDoc, "Text", strVal)
			return nil
		}
	}
	return fmt.Errorf("%s.Template has no Items with Text", label)
}

func setWidgetContentMut(widget bson.D, value any) error {
	content := bsonnav.DGetDoc(widget, "Content")
	if content == nil {
		return fmt.Errorf("widget has no Content property")
	}
	return setClientTemplateText(content, "Content", value)
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

// setPluggableWidgetPropertyMut writes one property of a pluggable widget's
// Object, resolving the author's spelling against the widget's template keys
// CASE-INSENSITIVELY.
//
// That last part is the fix for #1069. A pluggable property key is lowerCamel in
// the template (`pageSize`), while DESCRIBE PAGE prints it capitalised
// (`PageSize:`) and CREATE accepts either — the widget engine resolves it with
// lookupProperty, which lowercases both sides. Comparing byte-for-byte here made
// `alter page … set PageSize = 10` fail with `pluggable property "PageSize" not
// found` on a grid that `create page … (PageSize: 20)` had just written, so the
// tool refused to execute its own DESCRIBE output.
//
// It is unambiguous, not merely convenient: this searches one object type's
// PropertyTypes, and no shipped template or definition has two keys in the same
// list differing only in case (96 scopes, 1208 keys, 0 collisions — held by
// TestPluggablePropertyKeysAreUniqueIgnoringCase). An unknown property still
// errors — and `mxcli check --references` now reaches that error before the
// script runs, by dry-running this setter against a copy of the document rather
// than re-deriving what it accepts (see probe.go).
func setPluggableWidgetPropertyMut(widget bson.D, propName string, value any) error {
	obj := bsonnav.DGetDoc(widget, "Object")
	if obj == nil {
		return noPluggableObjectError(widget, propName)
	}

	// The same derivation buildPropKeyMap does, and it used to be spelled out a
	// second time here. One of the two copies was #1069's bug site, so they are
	// now one function — a resolver that disagrees with itself is the failure
	// this whole area keeps producing.
	propTypeKeyMap := buildPropKeyMap(widget)
	propKindMap := buildPropKindMap(widget)

	// No pluggable property takes a list. A bracketed value arrives as a
	// []string, and %v fused its tokens into one string written as success.
	if _, isList := value.([]string); isList {
		return errExpressionNotAString(propName, value)
	}

	props := bsonnav.DGetArrayElements(bsonnav.DGet(obj, "Properties"))
	for _, prop := range props {
		propDoc, ok := prop.(bson.D)
		if !ok {
			continue
		}
		typePointerID := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(propDoc, "TypePointer"))
		propKey := propTypeKeyMap[typePointerID]
		if propKey == "" || !strings.EqualFold(propKey, propName) {
			continue
		}
		valDoc := bsonnav.DGetDoc(propDoc, "Value")
		if valDoc == nil {
			return fmt.Errorf("property %q has no Value map", propName)
		}
		// Which field the value belongs in is the schema's to say, exactly as
		// for a DataGrid 2 column (columnValueField). Writing PrimitiveValue for
		// every kind made `SET ImageUrl` on an image report "Altered page" and
		// change nothing: imageUrl is a TextTemplate, and DESCRIBE, mx check and
		// the runtime all read the template (mendixlabs/mxcli#1201). An empty
		// kind — a document with no ValueType — stays primitive, as before.
		kind := propKindMap[typePointerID]
		field, settable := columnValueField(kind)
		if !settable {
			return fmt.Errorf(
				"pluggable property %q holds a value of kind %s, which ALTER cannot set from a plain value — "+
					"rewrite the widget with CREATE OR REPLACE PAGE (or ALTER PAGE REPLACE) instead",
				propName, kind)
		}
		switch field {
		case "TextTemplate":
			// A null template is how #574 stores one its condition hides. ALTER
			// does not re-run visibility, so building one here would put text in
			// a pruned slot — refused, not created.
			textTemplate := bsonnav.DGetDoc(valDoc, "TextTemplate")
			if textTemplate == nil || !updateClientTemplateText(textTemplate, fmt.Sprintf("%v", value)) {
				return fmt.Errorf(
					"pluggable property %q has no text template to update — it is hidden by the widget's "+
						"current configuration; set the property that enables it with CREATE OR REPLACE PAGE",
					propName)
			}
			return nil
		case "Expression":
			bsonnav.DSet(valDoc, "Expression", fmt.Sprintf("%v", value))
			return nil
		}
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
	return fmt.Errorf("pluggable property %q not found", propName)
}

// noPluggableObjectError explains a SET that reached the pluggable fallback on a
// widget that has no pluggable Object — i.e. a built-in one, whose vocabulary is
// setRawWidgetPropertyMut's switch and nothing else.
//
// The message used to be "property %q not found (widget has no pluggable
// Object)". That is true and unusable: "pluggable Object" is not something the
// author wrote, and it was not the whole truth either — an Atlas design property
// on a built-in widget (the case reported as mendixlabs/mxcli#1135) is writable.
//
// It named ALTER STYLING as the route until ako/mxcli#515 taught `set` to write
// design properties itself. Reaching here now means the key is neither a
// first-class property NOR a design property the theme declares for this
// widget's type, so the message says that and points at the command that lists
// the ones it does declare — sending the reader to a second statement would be
// stale advice for a route that no longer differs.
//
// A Forms$Appearance and no Object is exactly a built-in widget, which is when
// the advice applies. A pluggable widget keeps the error that names its own
// declared keys — sending a mistyped pluggable key to ALTER STYLING would point
// at a command that cannot write it either.
func noPluggableObjectError(widget bson.D, propName string) error {
	if bsonnav.DGetDoc(widget, "Appearance") == nil {
		return fmt.Errorf("property %q not found on this widget", propName)
	}
	return fmt.Errorf("property %q is not a property of this built-in widget, and not an Atlas "+
		"design property your theme declares for it — `set` writes its own properties "+
		"(Caption, Class, Style, DynamicClasses, Visible, Editable, …) and any design "+
		"property of this widget's type. Run `mxcli show design properties for <widget type>` "+
		"to see which those are",
		propName)
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
	// A *pages.DatabaseSource is deliberately absent: it has no single stored
	// shape, so there is nothing to map it to here. databaseSourceRefusal
	// above turns it away before this is reached.
	case *pages.DataViewSource:
		// "Data from context": the widget binds to a page/snippet parameter. The
		// EntityRef names the parameter's entity and the SourceVariable points at
		// the parameter itself — both BY_NAME, so both must resolve or Mendix
		// stores null and the project will not open (#854).
		var entityRef any
		if d.EntityName != "" {
			entityRef = bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "DomainModels$DirectEntityRef"},
				{Key: "Entity", Value: d.EntityName},
			}
		}
		var sourceVariable any
		if d.ParameterName != "" {
			// A snippet parameter and a page parameter are the same shape under
			// different keys; writing the wrong one is a ref that resolves to null.
			paramKey := "PageParameter"
			if d.IsSnippetParameter {
				paramKey = "SnippetParameter"
			}
			sourceVariable = bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Forms$PageVariable"},
				{Key: paramKey, Value: d.ParameterName},
			}
		}
		return bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$DataViewSource"},
			{Key: "EntityRef", Value: entityRef},
			{Key: "ForceFullObjects", Value: false},
			{Key: "SourceVariable", Value: sourceVariable},
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
		// Flat, unlike the microflow source: no settings child. Measured on 5 of
		// 5 Studio Pro-authored nanoflow sources (Feedback v4.0.2, 11.13.0); the
		// nested Forms$NanoflowSettings shape is CE2633 "No nanoflow configured".
		return bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$NanoflowSource"},
			{Key: "ForceFullObjects", Value: false},
			{Key: "Nanoflow", Value: d.Nanoflow},
			{Key: "ParameterMappings", Value: bson.A{int32(2)}},
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

// lookupParameter resolves a page/snippet parameter by name to the entity it is
// typed with, reporting whether it is a snippet parameter.
//
// The snippet/page distinction is taken from the STORED parameter's own $Type
// rather than from the mutator's container kind: it is the same fact, read from
// the document that has to agree with it.
func (m *Mutator) lookupParameter(name string) (entity string, isSnippetParam bool, found bool) {
	for _, item := range bsonnav.DGetArrayElements(bsonnav.DGet(m.rawData, "Parameters")) {
		// Element 0 of a Mendix array is the version marker, not a parameter.
		paramDoc, ok := item.(bson.D)
		if !ok {
			continue
		}
		if bsonnav.DGetString(paramDoc, "Name") != name {
			continue
		}
		isSnippetParam = strings.Contains(bsonnav.DGetString(paramDoc, "$Type"), "Snippet")
		if pt := bsonnav.DGetDoc(paramDoc, "ParameterType"); pt != nil {
			entity = bsonnav.DGetString(pt, "Entity")
		}
		return entity, isSnippetParam, true
	}
	return "", false, false
}

// errExpressionNotAString refuses a bracketed list where a property takes a
// single value. Returned rather than skipped: ALTER's check dry-runs the setter
// (validateAlterSetProperties) and reports this error, and exec stops on it,
// instead of either reporting success for a value that was never written.
//
// The list itself is not echoed: the visitor has already fused its tokens
// (`if1>0then…`), which reads as a second, unrelated problem.
func errExpressionNotAString(propName string, _ any) error {
	return fmt.Errorf(
		"property %q takes a single value, but was given a bracketed list — "+
			"write the expression itself, without brackets: "+
			"set %s = if $currentObject/Featured then 'a' else 'b'",
		propName, propName)
}
