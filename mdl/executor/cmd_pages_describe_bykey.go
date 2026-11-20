// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"sort"
	"strings"
	"sync"

	mwidgets "github.com/mendixlabs/mxcli/modelsdk/widgets"
)

// Settings addressed by the widget's own property key.
//
// A pluggable widget has far more settings than MDL gives a keyword. The write
// side takes every one of them under the key the widget's package declares:
//
//	datagrid dg (DataSource: database Shop.Ord, configurationStorageType: localStorage)
//
// DESCRIBE read only the handful it has keywords for (PageSize, Pagination,
// Selection, …), so a grid authored with the rest described back without them
// and a describe -> exec cycle put every one back to the widget's default —
// silently, with `mx check` at 0 errors.
//
// # What is printed
//
// A setting is printed when its stored value differs from what exec writes for
// a property left out, and only then. That keeps every grid that sets none of
// them describing byte for byte as it did.
//
// "What exec writes" is not always the schema's DefaultValue. A text template's
// DefaultValue is '' while the widget ships text for it ("Export progress"), and
// the writer stores that text when the property is absent; comparing against ''
// would print four labels on every grid. The schema is the widget's own, read
// from the document's stored Type — the definition the value was written
// against — so nothing here is a list of widgets or of defaults.

// widgetPropSchema is one property type of a stored widget, as far as DESCRIBE
// needs it.
type widgetPropSchema struct {
	Key  string
	Type string // "Boolean", "Integer", "Enumeration", "TextTemplate", …
	// Default is the schema's DefaultValue.
	Default string
	// DataSourceProperty is the key of the datasource property this one is
	// linked to; "" when it binds to the context object (an attribute property
	// resolves against the datasource's entity when linked, the enclosing data
	// container's when not).
	DataSourceProperty string
	// Translations is a text template's shipped default text per language code.
	Translations map[string]string
	// FirstTranslation is the text of the first shipped translation, the
	// fallback for a language the widget ships no text for.
	FirstTranslation string
}

// propertySchemasByID reads the property types of a stored widget (or of a
// nested object type) into a map keyed by the property type's $ID, which is
// what a stored property's TypePointer holds.
func propertySchemasByID(propTypes []any) map[string]widgetPropSchema {
	out := make(map[string]widgetPropSchema, len(propTypes))
	for _, pt := range propTypes {
		ptMap, ok := pt.(map[string]any)
		if !ok {
			continue
		}
		id := extractBinaryID(ptMap["$ID"])
		key := extractString(ptMap["PropertyKey"])
		if id == "" || key == "" {
			continue
		}
		sc := widgetPropSchema{Key: key}
		if vt, ok := ptMap["ValueType"].(map[string]any); ok {
			sc.Type = extractString(vt["Type"])
			sc.Default = extractString(vt["DefaultValue"])
			sc.DataSourceProperty = extractString(vt["DataSourceProperty"])
			for _, tr := range getBsonArrayElements(vt["Translations"]) {
				trMap, ok := tr.(map[string]any)
				if !ok {
					continue
				}
				text := extractString(trMap["Text"])
				if sc.Translations == nil {
					sc.Translations = map[string]string{}
					sc.FirstTranslation = text
				}
				sc.Translations[extractString(trMap["LanguageCode"])] = text
			}
		}
		out[id] = sc
	}
	return out
}

// widgetPropertySchemas is propertySchemasByID for a custom widget's own
// property types.
func widgetPropertySchemas(w map[string]any) map[string]widgetPropSchema {
	widgetType, ok := w["Type"].(map[string]any)
	if !ok {
		return nil
	}
	objType, ok := widgetType["ObjectType"].(map[string]any)
	if !ok {
		return nil
	}
	return propertySchemasByID(getBsonArrayElements(objType["PropertyTypes"]))
}

// objectListPropertySchemas is propertySchemasByID for the item type of the
// object-list property listKey (a data grid's `columns`).
func objectListPropertySchemas(w map[string]any, listKey string) map[string]widgetPropSchema {
	widgetType, ok := w["Type"].(map[string]any)
	if !ok {
		return nil
	}
	objType, ok := widgetType["ObjectType"].(map[string]any)
	if !ok {
		return nil
	}
	for _, pt := range getBsonArrayElements(objType["PropertyTypes"]) {
		ptMap, ok := pt.(map[string]any)
		if !ok || extractString(ptMap["PropertyKey"]) != listKey {
			continue
		}
		vt, ok := ptMap["ValueType"].(map[string]any)
		if !ok {
			return nil
		}
		itemType, ok := vt["ObjectType"].(map[string]any)
		if !ok {
			return nil
		}
		return propertySchemasByID(getBsonArrayElements(itemType["PropertyTypes"]))
	}
	return nil
}

// byKeyOptions steers nonDefaultPropsByKey.
type byKeyOptions struct {
	// Skip names the keys MDL already spells another way (PageSize, Selection,
	// onClick, …) or that have a body of their own (columns, slots).
	Skip map[string]bool
	// AttrEntity returns the entity a bare attribute name of property sc
	// resolves against on re-execution; "" when there is none.
	AttrEntity func(sc widgetPropSchema) string
	// WrittenDefault, when set, returns the primitive or expression value exec
	// writes for property key left out, where that is not the schema's
	// DefaultValue (templateWrittenDefaults).
	WrittenDefault func(key string) (string, bool)
}

// defaultFor is the value a stored primitive or expression is compared with.
func (o byKeyOptions) defaultFor(sc widgetPropSchema) string {
	if o.WrittenDefault != nil {
		if v, ok := o.WrittenDefault(sc.Key); ok {
			return v
		}
	}
	return sc.Default
}

// nonDefaultPropsByKey returns, in key order, the properties of a stored widget
// object (or object-list item) whose value differs from what exec writes for a
// property left out — each as the `key: value` it is written back with.
//
// omitted names the properties that differ and that no MDL spelling carries, so
// the caller can say so rather than lose them quietly.
func nonDefaultPropsByKey(ctx *ExecContext, objProps []any, schemas map[string]widgetPropSchema, opts byKeyOptions) (props []rawExplicitProp, omitted []string) {
	for _, prop := range objProps {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		sc, ok := schemas[extractBinaryID(propMap["TypePointer"])]
		if !ok || opts.Skip[sc.Key] {
			continue
		}
		value, ok := propMap["Value"].(map[string]any)
		if !ok {
			continue
		}
		switch sc.Type {
		case "Action", "DataSource", "Widgets", "Object", "Selection":
			// Carried elsewhere: an action by onClick / its own key, a datasource
			// by DataSource:, a widget slot or an object list by its body, and a
			// selection by Selection:.
			continue
		case "Boolean", "Integer", "Decimal", "String", "Enumeration":
			pv := extractString(value["PrimitiveValue"])
			if pv == "" || pv == opts.defaultFor(sc) {
				continue
			}
			props = append(props, rawExplicitProp{Key: sc.Key, Value: pv, ValueType: sc.Type})
		case "Expression":
			expr := extractString(value["Expression"])
			if expr == "" || expr == opts.defaultFor(sc) {
				continue
			}
			props = append(props, expressionByKeyProp(ctx, sc.Key, expr))
		case "TextTemplate":
			text, tt := extractTextTemplateText(value)
			params := []string(nil)
			if tt != nil {
				params = extractTextTemplateParameters(ctx, tt)
			}
			if len(params) == 0 && textTemplateIsDefault(value, sc) {
				continue
			}
			props = append(props, rawExplicitProp{Key: sc.Key, Value: text, ValueType: sc.Type, Params: params})
		case "Attribute":
			attrRef, ok := value["AttributeRef"].(map[string]any)
			if !ok || attrRef == nil {
				continue
			}
			attr := extractString(attrRef["Attribute"])
			if attr == "" {
				continue
			}
			if entityRef, ok := attrRef["EntityRef"].(map[string]any); ok && len(entityRef) > 0 {
				// A path over associations, `Assoc/Attr`, which the explicit-key
				// pass resolves back into the same steps (ako/mxcli#2279,
				// ako/mxcli#2295).
				if path := columnAttributeFromRef(ctx, attrRef); strings.Contains(path, "/") {
					props = append(props, rawExplicitProp{Key: sc.Key, Value: path, IsRef: true})
				} else {
					omitted = append(omitted, sc.Key)
				}
				continue
			}
			scope := ""
			if opts.AttrEntity != nil {
				scope = opts.AttrEntity(sc)
			}
			props = append(props, rawExplicitProp{Key: sc.Key, Value: byKeyAttributeName(ctx, attr, scope), IsRef: true})
		default:
			// A kind this does not know (an association, an icon, an image, a
			// file): if it holds a value, say it was left out.
			if valueHoldsSomething(value, "EntityRef", "Icon", "Image", "File", "AttributeRef", "Expression", "PrimitiveValue") {
				omitted = append(omitted, sc.Key)
			}
		}
	}
	sort.Slice(props, func(i, j int) bool { return props[i].Key < props[j].Key })
	sort.Strings(omitted)
	return props, omitted
}

// byKeyAttributeName is the attribute as the explicit-key pass reads it back: a
// bare name, as the rest of describe writes one, when exec resolves that against
// the entity the property binds to; the qualified name when it would not — no
// data container to resolve against, or an attribute of another entity (one
// inherited from a generalization). The writer takes any name with two dots as
// qualified.
//
// The scope is a qualified entity name when the container's source is an
// entity, and only a name (a page parameter's) when it is not; the latter cannot
// be compared, and Mendix binds such a property to an attribute of that
// container's entity anyway.
func byKeyAttributeName(ctx *ExecContext, attr, scopeEntity string) string {
	if ctx != nil && ctx.describeQualifyAttrs {
		return attr
	}
	if scopeEntity == "" {
		return attr
	}
	if strings.Contains(scopeEntity, ".") && !strings.EqualFold(attributeEntity(attr), scopeEntity) {
		return attr
	}
	return shortAttributeName(attr)
}

// attributeEntity is Module.Entity of Module.Entity.Attribute.
func attributeEntity(attr string) string {
	if idx := strings.LastIndex(attr, "."); idx >= 0 {
		return attr[:idx]
	}
	return ""
}

// expressionByKeyProp renders an Expression-typed property. The explicit-key
// pass takes the unquoted form as the expression itself, so that is what is
// printed — except for an expression that is nothing but a string literal
// (`'cls'`), which unquoted would be read as the string `cls` and stored
// without its quotes; it is printed as the quoted text of the expression.
func expressionByKeyProp(ctx *ExecContext, key, expr string) rawExplicitProp {
	if _, isLiteral := mendixStringLiteral(expr); isLiteral {
		return rawExplicitProp{Key: key, Value: mdlQuote(ctx, expr), IsRef: true}
	}
	return rawExplicitProp{Key: key, Value: expr, IsRef: true}
}

// textTemplateIsDefault reports whether a stored text template is what exec
// writes for the property left out: the text the widget ships for it, or no text
// at all. Compared per language, falling back to the first shipped text for a
// language it ships none for.
func textTemplateIsDefault(value map[string]any, sc widgetPropSchema) bool {
	stored := map[string]string{}
	if tt, ok := value["TextTemplate"].(map[string]any); ok && tt != nil {
		if template, ok := tt["Template"].(map[string]any); ok && template != nil {
			for _, item := range getBsonArrayElements(template["Items"]) {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if text := extractString(m["Text"]); text != "" {
					stored[extractString(m["LanguageCode"])] = text
				}
			}
		}
	}
	if len(stored) == 0 {
		// Nothing stored: a null template, which is what a property the widget's
		// editor config hides is written with, or an empty one. Neither is a
		// setting MDL can restate — `key: ''` is written as the widget's text,
		// the same as leaving the property out — so printing it would only
		// change the description of every grid whose property is hidden.
		return true
	}
	for lang, text := range stored {
		shipped, ok := sc.Translations[lang]
		if !ok {
			shipped = sc.FirstTranslation
		}
		if text != shipped {
			return false
		}
	}
	return true
}

// valueHoldsSomething reports whether any of the named fields of a widget value
// is set to something other than null, "" or an empty map.
func valueHoldsSomething(value map[string]any, fields ...string) bool {
	for _, f := range fields {
		switch v := value[f].(type) {
		case nil:
		case string:
			if v != "" {
				return true
			}
		case map[string]any:
			if len(v) > 0 {
				return true
			}
		default:
			return true
		}
	}
	return false
}

// dataGridByKeySkip are the grid-level keys MDL spells another way or carries in
// a body: the paging keywords, the selection, the datasource, the columns, the
// slot widgets, and the action slots. (Action-, DataSource-, Widgets-, Object-
// and Selection-typed properties are skipped by type; these are the scalar ones
// with a keyword of their own.)
var dataGridByKeySkip = map[string]bool{
	"pageSize": true, "pagination": true, "pagingPosition": true, "showPagingButtons": true,
}

// dataGridColumnKeySkip are the column keys extractDataGrid2Column already
// reads under their MDL names.
var dataGridColumnKeySkip = map[string]bool{
	"showContentAs": true, "attribute": true, "header": true, "dynamicText": true,
	"tooltip": true, "visible": true, "sortable": true, "resizable": true,
	"draggable": true, "hidable": true, "width": true, "size": true,
	"alignment": true, "columnClass": true, "wrapText": true,
}

// galleryByKeySkip is dataGridByKeySkip for a Gallery: the column counts are
// DesktopColumns / TabletColumns / PhoneColumns, and the click trigger has its
// own line.
var galleryByKeySkip = map[string]bool{
	"desktopItems": true, "tabletItems": true, "phoneItems": true, "onClickTrigger": true,
}

// sharedBranchByKeySkip are, per widget, the keys the combo box's and the data
// grid filters' readers already print under an MDL keyword (Attribute:,
// DataSource:, CaptionAttribute:, Attributes:, FilterType:, the expression
// caption) or that select the mode those keywords imply. Every other scalar
// setting is printed by its own key.
var sharedBranchByKeySkip = map[string]map[string]bool{
	"combobox": {
		"source": true, "optionsSourceType": true,
		"attributeEnumeration": true, "attributeBoolean": true, "attributeAssociation": true,
		"optionsSourceAssociationCaptionType": true, "optionsSourceAssociationCaptionAttribute": true,
		"optionsSourceAssociationCaptionExpression": true,
	},
	"textfilter":   {"attrChoice": true, "defaultFilter": true},
	"numberfilter": {"attrChoice": true, "defaultFilter": true},
	"datefilter":   {"attrChoice": true, "defaultFilter": true},
	"dropdownfilter": {
		"baseType": true, "attrChoice": true, "attr": true, "refEntity": true, "refCaption": true,
	},
}

// comboBoxStaticModePrefixes and comboBoxDatabaseModePrefixes are the keys of
// the combo box's option sources DESCRIBE does not spell in the mode being
// described (withoutUndescribedComboBoxModes): the static
// source always, and the database source outside database mode. Those settings
// are named as not reproduced rather than printed: on their own, without the
// mode and its datasource, they would be written into a combo box in another
// mode — an attribute resolved against the wrong entity.
var (
	comboBoxStaticModePrefixes   = []string{"optionsSourceStatic", "staticDataSource", "staticAttribute"}
	comboBoxDatabaseModePrefixes = []string{"optionsSourceDatabase", "databaseAttributeString"}
)

// comboBoxDatabaseByKeySkip is sharedBranchByKeySkip for a database-mode combo
// box: the association and enumeration keys stay unprinted, and so do the
// database keys the reader spells as keywords — the target attribute as
// `Attribute:`, the option caption as `CaptionAttribute:` — and `source`,
// which the datasource's key selects on exec.
var comboBoxDatabaseByKeySkip = func() map[string]bool {
	skip := map[string]bool{"databaseAttributeString": true, "optionsSourceDatabaseCaptionAttribute": true}
	for k, v := range sharedBranchByKeySkip["combobox"] {
		skip[k] = v
	}
	return skip
}()

// comboBoxIsDatabaseMode reports whether a stored combo box takes its options
// from a database source of its own (`source` "database").
func comboBoxIsDatabaseMode(ctx *ExecContext, w map[string]any) bool {
	return extractCustomWidgetPropertyString(ctx, w, "source") == "database"
}

// describeComboBoxDatabaseMode reads a database-mode combo box (mendixlabs/
// mxcli#1583): its option list under the datasource's own key, which is what
// selects the mode on exec — the generic `DataSource:` clause selects the
// association mode — the attribute it stores into as `Attribute:`, the option
// caption as `CaptionAttribute:`, and the change action from the mode's own
// slot. The value attribute and the caption type and expression are settings by
// key (comboBoxDatabaseByKeySkip).
func describeComboBoxDatabaseMode(ctx *ExecContext, w map[string]any, widget *rawWidget) {
	if ds := extractCustomWidgetPropertyDataSource(ctx, w, "optionsSourceDatabaseDataSource"); ds != nil {
		widget.NamedDataSources = []rawNamedDataSource{{Key: "optionsSourceDatabaseDataSource", DataSource: ds}}
	}
	widget.Content = extractCustomWidgetPropertyAttributeRef(ctx, w, "databaseAttributeString")
	if param := customWidgetPropertyParameter(w, "databaseAttributeString"); param != "" && widget.Content != "" {
		widget.Content = parameterAttributeMDL(param, widget.Content)
	}
	if extractCustomWidgetPropertyString(ctx, w, "optionsSourceDatabaseCaptionType") != "expression" {
		widget.CaptionAttribute = extractCustomWidgetPropertyAttributeRef(ctx, w, "optionsSourceDatabaseCaptionAttribute")
	}
	widget.OnChange = renderClientActionMDL(ctx, customWidgetPropertyActionMap(ctx, w, "onChangeDatabaseEvent"))
}

// comboBoxDatabaseSelection is the item selection of a database-mode combo
// box, by key, when it is not the Single exec writes for it left out. The
// by-key reader skips every selection-typed property, because a data grid's
// and a gallery's have the `Selection:` keyword.
func comboBoxDatabaseSelection(ctx *ExecContext, w map[string]any) []rawExplicitProp {
	sel := extractCustomWidgetPropertySelection(ctx, w, "optionsSourceDatabaseItemSelection")
	if sel == "" || sel == "Single" {
		return nil
	}
	return []rawExplicitProp{{Key: "optionsSourceDatabaseItemSelection", Value: sel, IsRef: true}}
}

// withoutUndescribedComboBoxModes moves the settings of a combo box's
// undescribed option-source modes from props to omitted. database says the
// combo box is described in database mode, whose own settings are kept.
func withoutUndescribedComboBoxModes(props []rawExplicitProp, omitted []string, database bool) ([]rawExplicitProp, []string) {
	prefixes := comboBoxStaticModePrefixes
	if !database {
		prefixes = append(append([]string{}, prefixes...), comboBoxDatabaseModePrefixes...)
	}
	kept := props[:0:0]
	for _, p := range props {
		undescribed := false
		for _, prefix := range prefixes {
			if strings.HasPrefix(p.Key, prefix) {
				undescribed = true
				break
			}
		}
		if undescribed {
			omitted = append(omitted, p.Key)
			continue
		}
		kept = append(kept, p)
	}
	sort.Strings(omitted)
	return kept, omitted
}

var templateWrittenDefaultsCache sync.Map // widgetID → map[key]string

// templateWrittenDefaults returns, per property key, the primitive or
// expression value the widget's embedded template stores — which is what exec
// writes for a property left out. It is not always the schema's DefaultValue:
// the combo box template stores `readOnlyStyle: bordered` against a default of
// `text`, so comparing with the default printed `readOnlyStyle: 'bordered'` on
// every combo box mxcli wrote, and left a Studio Pro one's `text` unprinted —
// which re-executed as `bordered`. Empty for a widget with no embedded template.
func templateWrittenDefaults(widgetID string) map[string]string {
	if v, ok := templateWrittenDefaultsCache.Load(widgetID); ok {
		return v.(map[string]string)
	}
	out := map[string]string{}
	if tmpl, err := mwidgets.GetTemplate(widgetID); err == nil && tmpl != nil {
		keys := map[string]string{}
		for id, sc := range widgetPropertySchemas(map[string]any{"Type": tmpl.Type}) {
			keys[id] = sc.Key
		}
		for _, prop := range getBsonArrayElements(tmpl.Object["Properties"]) {
			propMap, ok := prop.(map[string]any)
			if !ok {
				continue
			}
			key := keys[extractBinaryID(propMap["TypePointer"])]
			value, ok := propMap["Value"].(map[string]any)
			if key == "" || !ok {
				continue
			}
			if pv := extractString(value["PrimitiveValue"]); pv != "" {
				out[key] = pv
			} else if expr := extractString(value["Expression"]); expr != "" {
				out[key] = expr
			}
		}
	}
	templateWrittenDefaultsCache.Store(widgetID, out)
	return out
}

// extractDataGrid2ByKeyProps returns the grid-level settings of a Data grid 2
// that DESCRIBE has no keyword for and that differ from their defaults, and the
// keys it could not print. enclosingEntity is the entity of the data container
// the grid sits in (what an attribute property linked to no datasource binds
// to), gridEntity the grid's own datasource entity.
func extractDataGrid2ByKeyProps(ctx *ExecContext, w map[string]any, enclosingEntity, gridEntity string) ([]rawExplicitProp, []string) {
	return extractWidgetByKeyProps(ctx, w, dataGridByKeySkip, enclosingEntity, gridEntity)
}

// extractGalleryByKeyProps is extractDataGrid2ByKeyProps for a Gallery, which
// takes its settings by key the same way and described back without them the
// same way (pagination, page size, empty placeholder, state storage, …).
func extractGalleryByKeyProps(ctx *ExecContext, w map[string]any, enclosingEntity, galleryEntity string) ([]rawExplicitProp, []string) {
	return extractWidgetByKeyProps(ctx, w, galleryByKeySkip, enclosingEntity, galleryEntity)
}

func extractWidgetByKeyProps(ctx *ExecContext, w map[string]any, skip map[string]bool, enclosingEntity, ownEntity string) ([]rawExplicitProp, []string) {
	return widgetByKeyProps(ctx, w, byKeyOptions{Skip: skip}, enclosingEntity, ownEntity)
}

// extractSharedBranchByKeyProps is extractWidgetByKeyProps for a combo box or
// a data grid filter, compared with what exec writes for a property left out
// (templateWrittenDefaults) rather than with the schema's DefaultValue.
func extractSharedBranchByKeyProps(ctx *ExecContext, w map[string]any, skip map[string]bool, enclosingEntity, ownEntity string) ([]rawExplicitProp, []string) {
	written := templateWrittenDefaults(extractCustomWidgetID(ctx, w))
	return widgetByKeyProps(ctx, w, byKeyOptions{
		Skip: skip,
		WrittenDefault: func(key string) (string, bool) {
			v, ok := written[key]
			return v, ok
		},
	}, enclosingEntity, ownEntity)
}

func widgetByKeyProps(ctx *ExecContext, w map[string]any, opts byKeyOptions, enclosingEntity, ownEntity string) ([]rawExplicitProp, []string) {
	obj, ok := w["Object"].(map[string]any)
	if !ok {
		return nil, nil
	}
	opts.AttrEntity = func(sc widgetPropSchema) string {
		if sc.DataSourceProperty != "" {
			return ownEntity
		}
		return enclosingEntity
	}
	return nonDefaultPropsByKey(ctx, getBsonArrayElements(obj["Properties"]), widgetPropertySchemas(w), opts)
}

// extractDataGrid2ColumnByKeyProps is extractDataGrid2ByKeyProps for one
// column's object.
func extractDataGrid2ColumnByKeyProps(ctx *ExecContext, colObj map[string]any, colSchemas map[string]widgetPropSchema, entityContext string) ([]rawExplicitProp, []string) {
	return nonDefaultPropsByKey(ctx, getBsonArrayElements(colObj["Properties"]), colSchemas, byKeyOptions{
		Skip:       dataGridColumnKeySkip,
		AttrEntity: func(widgetPropSchema) string { return entityContext },
	})
}
