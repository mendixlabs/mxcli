// SPDX-License-Identifier: Apache-2.0

// Package mpk parses Mendix .mpk widget packages to extract widget property definitions.
// An .mpk file is a ZIP archive containing package.xml (manifest) and a widget XML file
// that defines the widget's properties, types, and metadata.
package mpk

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
)

// PropertyDef describes a single property from a widget XML definition.
type PropertyDef struct {
	Key         string // e.g. "staticDataSourceCaption"
	Type        string // XML type: "attribute", "expression", "textTemplate", "widgets", etc.
	Caption     string
	Description string
	Category    string // from enclosing propertyGroup captions, joined with "::"
	Required    bool
	// OnChange names the sibling action property Studio Pro runs when this
	// property changes. Part of the widget DEFINITION, so a stale value makes
	// Mendix report CE0463 "definition of this widget has changed" (#716).
	OnChange     string
	DefaultValue string // for enumeration/boolean/integer types
	// DefaultType is the KIND of the default — `defaultType` in the widget XML,
	// e.g. an action defaulting to a nanoflow call ("CallNanoflow") or a
	// datasource defaulting to an association ("Association"). Like OnChange it
	// is part of the widget DEFINITION, so storing "None" where the package
	// declares one is CE0463 (mendixlabs/mxcli#956, File Uploader 2.5.0).
	DefaultType    string
	IsList         bool
	Multiline      bool     // for string/textTemplate: multiline="true"
	SelectionTypes []string // for selection properties: <selectionType name="..."/>
	IsSystem       bool     // true for <systemProperty> elements
	DataSource     string   // dataSource attribute reference
	ReturnType     string   // for expression properties: the <returnType type="..."/> Mendix type
	// ReturnTypeAssignableTo is the <returnType assignableTo="..."/> reference (e.g.
	// "../staticAttribute"), when the expression's return type is derived from another
	// property rather than a concrete type. mxbuild emits Type "None" with this set.
	ReturnTypeAssignableTo string
	AllowedTypes           []string      // for attribute properties: Mendix type names ("String", "Decimal", etc.)
	EnumValues             []EnumValue   // for enumeration properties: the declared options (key + caption)
	Translations           []Translation // widget-shipped caption/template translations (<translations>)
	// ActionVariables are the variables an action property passes to the flow
	// it calls (`<actionVariables>`). Part of the widget DEFINITION — Studio Pro
	// stores them on the ValueType whether or not an action is configured — so
	// writing an empty list where the package declares some is CE0463
	// (mendixlabs/mxcli#1200, Signature 2.1.0, Calendar 2.6.0).
	ActionVariables []ActionVariable
	Children        []PropertyDef // nested properties for object-type properties
}

// ActionVariable is one `<actionVariable key type caption/>` of an action property.
type ActionVariable struct {
	Key     string
	Type    string // Mendix type name as the XML spells it: "String", "DateTime", …
	Caption string
}

// EnumValue is one option of an enumeration-typed widget property.
type EnumValue struct {
	Key     string
	Caption string
}

// Translation is one localized caption/template string of a widget property.
type Translation struct {
	Lang string
	Text string
}

// WidgetDefinition holds the parsed definition of a pluggable widget from an .mpk file.
type WidgetDefinition struct {
	ID                 string        // e.g. "com.mendix.widget.web.combobox.Combobox"
	Name               string        // e.g. "Combo box"
	Description        string        // widget description from <description> element
	Version            string        // from package.xml clientModule version
	IsPluggable        bool          // true if pluginWidget="true" (React), false for legacy Dojo
	OfflineCapable     bool          // true if offlineCapable="true"
	NeedsEntityContext bool          // true if needsEntityContext="true"
	SupportedPlatform  string        // "Web", "Native", "All" (empty = Web)
	HelpURL            string        // helpUrl attribute
	StudioCategory     string        // studioCategory attribute
	StudioProCategory  string        // studioProCategory attribute
	Properties         []PropertyDef // regular <property> elements
	SystemProps        []PropertyDef // <systemProperty> elements
	// AllTopLevel is the top-level properties (regular and system interleaved) in
	// the widget XML's declared document order. It is the authoritative order for
	// the emitted WidgetType's PropertyTypes — mxbuild's update-widgets uses it and
	// CE0463 checks it. Regular entries carry their full PropertyDef (incl.
	// Children); system entries carry only Key with IsSystem=true.
	AllTopLevel []PropertyDef
}

// --- XML structures for parsing ---

// xmlPackage represents <package> root element.
type xmlPackage struct {
	ClientModule xmlClientModule `xml:"clientModule"`
}

// xmlClientModule represents <clientModule> element.
type xmlClientModule struct {
	Name        string          `xml:"name,attr"`
	Version     string          `xml:"version,attr"`
	WidgetFiles []xmlWidgetFile `xml:"widgetFiles>widgetFile"`
}

// xmlWidgetFile represents <widgetFile path="..."/> element.
type xmlWidgetFile struct {
	Path string `xml:"path,attr"`
}

// xmlWidget represents <widget> root element in widget XML.
type xmlWidget struct {
	ID                 string `xml:"id,attr"`
	PluginWidget       string `xml:"pluginWidget,attr"`
	OfflineCapable     string `xml:"offlineCapable,attr"`
	NeedsEntityContext string `xml:"needsEntityContext,attr"`
	SupportedPlatform  string `xml:"supportedPlatform,attr"`
	// helpUrl, studioCategory, studioProCategory are child ELEMENTS in Mendix
	// widget XML (e.g. <studioProCategory>Charts</studioProCategory>), not attributes.
	HelpURL           string         `xml:"helpUrl"`
	StudioCategory    string         `xml:"studioCategory"`
	StudioProCategory string         `xml:"studioProCategory"`
	Name              string         `xml:"name"`
	Description       string         `xml:"description"`
	PropertyGroups    []xmlPropGroup `xml:"properties>propertyGroup"`
}

// xmlPropGroup represents <propertyGroup caption="..."> element.
//
// It has a custom UnmarshalXML (below) so that, in addition to the split
// Properties/SystemProps/SubGroups slices existing code reads, it records the
// children in document order (Children). The declared order matters: a widget
// may interleave <systemProperty> (Label/Visibility/Editability) among regular
// <property> elements (ComboBox declares them mid-list, and its Editability group
// even mixes a systemProperty ahead of a regular property). mxbuild's
// update-widgets emits the WidgetType's PropertyTypes in that declared order, and
// CE0463 checks the Type's PropertyType order — so we must reproduce it, not push
// system properties to the end.
type xmlPropGroup struct {
	Caption     string
	Properties  []xmlProperty
	SystemProps []xmlSystemProp
	SubGroups   []xmlPropGroup
	Children    []xmlPropGroupChild // document-ordered union of the three above
}

// xmlPropGroupChild is one child of a propertyGroup in document order: exactly one
// of Property/SystemProp/SubGroup is non-nil.
type xmlPropGroupChild struct {
	Property   *xmlProperty
	SystemProp *xmlSystemProp
	SubGroup   *xmlPropGroup
}

// UnmarshalXML decodes a <propertyGroup>, populating both the split slices
// (Properties/SystemProps/SubGroups) that existing walkers read and the
// document-ordered Children list used to reproduce the .mpk's declared property
// order. Unknown child elements are skipped.
func (pg *xmlPropGroup) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	for _, a := range start.Attr {
		if a.Name.Local == "caption" {
			pg.Caption = a.Value
		}
	}
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "property":
				p := new(xmlProperty)
				if err := d.DecodeElement(p, &t); err != nil {
					return err
				}
				pg.Properties = append(pg.Properties, *p)
				pg.Children = append(pg.Children, xmlPropGroupChild{Property: p})
			case "systemProperty":
				sp := new(xmlSystemProp)
				if err := d.DecodeElement(sp, &t); err != nil {
					return err
				}
				pg.SystemProps = append(pg.SystemProps, *sp)
				pg.Children = append(pg.Children, xmlPropGroupChild{SystemProp: sp})
			case "propertyGroup":
				sub := new(xmlPropGroup)
				if err := d.DecodeElement(sub, &t); err != nil {
					return err
				}
				pg.SubGroups = append(pg.SubGroups, *sub)
				pg.Children = append(pg.Children, xmlPropGroupChild{SubGroup: sub})
			default:
				if err := d.Skip(); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if t.Name.Local == start.Name.Local {
				return nil
			}
		}
	}
}

// xmlAttributeType represents <attributeType name="..."/> element.
type xmlAttributeType struct {
	Name string `xml:"name,attr"`
}

// xmlProperty represents <property key="..." type="..." ...> element.
type xmlProperty struct {
	Key            string             `xml:"key,attr"`
	Type           string             `xml:"type,attr"`
	DefaultValue   string             `xml:"defaultValue,attr"`
	DefaultType    string             `xml:"defaultType,attr"`
	Required       string             `xml:"required,attr"`
	OnChange       string             `xml:"onChange,attr"`
	IsList         string             `xml:"isList,attr"`
	Multiline      string             `xml:"multiline,attr"`
	DataSource     string             `xml:"dataSource,attr"`
	Caption        string             `xml:"caption"`
	Description    string             `xml:"description"`
	AttributeTypes []xmlAttributeType `xml:"attributeTypes>attributeType"`
	EnumValues     []xmlEnumValue     `xml:"enumerationValues>enumerationValue"`
	SelectionTypes []xmlSelectionType `xml:"selectionTypes>selectionType"`
	ReturnType     xmlReturnType      `xml:"returnType"`
	Translations   []xmlTranslation   `xml:"translations>translation"`
	// ActionVariables of an action property (<actionVariables><actionVariable …/>).
	ActionVariables []xmlActionVariable `xml:"actionVariables>actionVariable"`
	// Nested properties for object type
	NestedProps []xmlPropGroup `xml:"properties>propertyGroup"`
}

// xmlActionVariable represents <actionVariable key="…" caption="…" type="…"/>.
type xmlActionVariable struct {
	Key     string `xml:"key,attr"`
	Caption string `xml:"caption,attr"`
	Type    string `xml:"type,attr"`
}

// xmlSelectionType represents <selectionType name="..."/> on a selection property.
type xmlSelectionType struct {
	Name string `xml:"name,attr"`
}

// xmlReturnType represents <returnType type="..."/> or
// <returnType assignableTo="../otherProp"/> on an expression property. A widget may
// declare either a concrete Mendix type or an assignableTo reference (the expression
// must be assignable to another property's type); mxbuild emits a WidgetReturnType in
// both cases, with Type "None" when only assignableTo is given.
type xmlReturnType struct {
	Type         string `xml:"type,attr"`
	AssignableTo string `xml:"assignableTo,attr"`
}

// xmlTranslation represents <translation lang="...">Text</translation> — a widget-shipped
// caption/template translation.
type xmlTranslation struct {
	Lang string `xml:"lang,attr"`
	Text string `xml:",chardata"`
}

// xmlEnumValue represents <enumerationValue key="...">Caption</enumerationValue>.
type xmlEnumValue struct {
	Key     string `xml:"key,attr"`
	Caption string `xml:",chardata"`
}

// xmlSystemProp represents <systemProperty key="..."/> element.
type xmlSystemProp struct {
	Key string `xml:"key,attr"`
}

// Zip extraction limits to prevent zip-bomb attacks.
const (
	maxFileSize  = 50 << 20  // 50MB per individual file
	maxTotalSize = 200 << 20 // 200MB total extracted
)

// --- Caching ---

var (
	defCache     = make(map[string]*WidgetDefinition) // mpkPath -> definition
	defCacheLock sync.RWMutex

	dirCache     = make(map[string]map[string]string) // projectDir -> (widgetID -> mpkPath)
	dirCacheLock sync.RWMutex
)

// ParseMPK opens an .mpk ZIP archive, finds the widget XML, and parses it.
func ParseMPK(mpkPath string) (*WidgetDefinition, error) {
	// Check cache
	defCacheLock.RLock()
	if def, ok := defCache[mpkPath]; ok {
		defCacheLock.RUnlock()
		return def, nil
	}
	defCacheLock.RUnlock()

	r, err := zip.OpenReader(mpkPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open mpk: %w", err)
	}
	defer r.Close()

	// Parse package.xml to find widget file path and version
	var pkg xmlPackage
	var widgetFilePath string
	var version string
	var totalExtracted uint64

	for _, f := range r.File {
		if f.Name == "package.xml" {
			if f.UncompressedSize64 > maxFileSize {
				return nil, fmt.Errorf("package.xml exceeds max file size (%d > %d)", f.UncompressedSize64, maxFileSize)
			}
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("failed to open package.xml: %w", err)
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("failed to read package.xml: %w", err)
			}
			totalExtracted += uint64(len(data))
			if totalExtracted > maxTotalSize {
				return nil, fmt.Errorf("total extracted size exceeds limit (%d > %d)", totalExtracted, maxTotalSize)
			}
			if err := xml.Unmarshal(data, &pkg); err != nil {
				return nil, fmt.Errorf("failed to parse package.xml: %w", err)
			}
			version = pkg.ClientModule.Version
			if len(pkg.ClientModule.WidgetFiles) > 0 {
				widgetFilePath = pkg.ClientModule.WidgetFiles[0].Path
			}
			break
		}
	}

	if widgetFilePath == "" {
		return nil, fmt.Errorf("no widget file path found in package.xml")
	}

	// Parse widget XML
	for _, f := range r.File {
		if f.Name == widgetFilePath {
			if f.UncompressedSize64 > maxFileSize {
				return nil, fmt.Errorf("%s exceeds max file size (%d > %d)", widgetFilePath, f.UncompressedSize64, maxFileSize)
			}
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("failed to open %s: %w", widgetFilePath, err)
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("failed to read %s: %w", widgetFilePath, err)
			}
			totalExtracted += uint64(len(data))
			if totalExtracted > maxTotalSize {
				return nil, fmt.Errorf("total extracted size exceeds limit (%d > %d)", totalExtracted, maxTotalSize)
			}

			var widget xmlWidget
			if err := xml.Unmarshal(data, &widget); err != nil {
				return nil, fmt.Errorf("failed to parse %s: %w", widgetFilePath, err)
			}

			def := buildDefinition(&widget, version)

			// Cache
			defCacheLock.Lock()
			defCache[mpkPath] = def
			defCacheLock.Unlock()

			return def, nil
		}
	}

	return nil, fmt.Errorf("widget file %s not found in mpk", widgetFilePath)
}

// walkPropertyGroup recursively walks property groups to collect properties.
func walkPropertyGroup(pg xmlPropGroup, parentCategory string, def *WidgetDefinition) {
	category := pg.Caption
	if parentCategory != "" && category != "" {
		category = parentCategory + "::" + category
	} else if parentCategory != "" {
		category = parentCategory
	}

	// Collect regular properties
	for _, p := range pg.Properties {
		var allowedTypes []string
		for _, at := range p.AttributeTypes {
			if at.Name != "" {
				allowedTypes = append(allowedTypes, at.Name)
			}
		}
		var enumValues []EnumValue
		for _, ev := range p.EnumValues {
			enumValues = append(enumValues, EnumValue{Key: ev.Key, Caption: strings.TrimSpace(ev.Caption)})
		}
		prop := PropertyDef{
			Key:         p.Key,
			Type:        p.Type,
			Caption:     p.Caption,
			Description: p.Description,
			Category:    category,
			// Mendix pluggable-widget spec: `required` defaults to true when the
			// attribute is absent (mxbuild's update-widgets emits Required=true for
			// every property that omits required=, e.g. DataGrid2 showContentAs). Only
			// an explicit required="false" is optional. Defaulting missing→false here
			// caused within-key CE0463 drift on augment-added keys (issue #600).
			Required:               p.Required != "false",
			OnChange:               p.OnChange,
			DefaultValue:           p.DefaultValue,
			DefaultType:            p.DefaultType,
			IsList:                 p.IsList == "true",
			Multiline:              p.Multiline == "true",
			SelectionTypes:         toSelectionTypes(p.SelectionTypes),
			DataSource:             p.DataSource,
			ReturnType:             p.ReturnType.Type,
			ReturnTypeAssignableTo: p.ReturnType.AssignableTo,
			AllowedTypes:           allowedTypes,
			EnumValues:             enumValues,
			Translations:           toTranslations(p.Translations),
			ActionVariables:        toActionVariables(p.ActionVariables),
		}

		// Parse nested properties for object-type properties
		if p.Type == "object" && len(p.NestedProps) > 0 {
			for _, npg := range p.NestedProps {
				collectNestedProperties(npg, &prop, "")
			}
		}

		def.Properties = append(def.Properties, prop)
	}

	// Collect system properties
	for _, sp := range pg.SystemProps {
		def.SystemProps = append(def.SystemProps, PropertyDef{
			Key:      sp.Key,
			IsSystem: true,
			Category: category,
		})
	}

	// Recurse into subgroups
	for _, sub := range pg.SubGroups {
		walkPropertyGroup(sub, category, def)
	}
}

// collectNestedProperties extracts child properties from nested propertyGroups
// within an object-type property and appends them to the parent PropertyDef. The
// property group's caption chain becomes each child's Category (joined with "::",
// mirroring walkPropertyGroup) — mxbuild derives nested categories the same way, so a
// missing category here is a within-key CE0463 drift on augment-added nested props.
func collectNestedProperties(pg xmlPropGroup, parent *PropertyDef, parentCategory string) {
	category := pg.Caption
	if parentCategory != "" && category != "" {
		category = parentCategory + "::" + category
	} else if parentCategory != "" {
		category = parentCategory
	}
	for _, p := range pg.Properties {
		var allowedTypes []string
		for _, at := range p.AttributeTypes {
			if at.Name != "" {
				allowedTypes = append(allowedTypes, at.Name)
			}
		}
		var enumValues []EnumValue
		for _, ev := range p.EnumValues {
			enumValues = append(enumValues, EnumValue{Key: ev.Key, Caption: strings.TrimSpace(ev.Caption)})
		}
		child := PropertyDef{
			Key:         p.Key,
			Type:        p.Type,
			Caption:     p.Caption,
			Description: p.Description,
			Category:    category,
			// Mendix pluggable-widget spec: `required` defaults to true when the
			// attribute is absent (mxbuild's update-widgets emits Required=true for
			// every property that omits required=, e.g. DataGrid2 showContentAs). Only
			// an explicit required="false" is optional. Defaulting missing→false here
			// caused within-key CE0463 drift on augment-added keys (issue #600).
			Required:               p.Required != "false",
			OnChange:               p.OnChange,
			DefaultValue:           p.DefaultValue,
			DefaultType:            p.DefaultType,
			IsList:                 p.IsList == "true",
			Multiline:              p.Multiline == "true",
			SelectionTypes:         toSelectionTypes(p.SelectionTypes),
			DataSource:             p.DataSource,
			ReturnType:             p.ReturnType.Type,
			ReturnTypeAssignableTo: p.ReturnType.AssignableTo,
			AllowedTypes:           allowedTypes,
			EnumValues:             enumValues,
			Translations:           toTranslations(p.Translations),
			ActionVariables:        toActionVariables(p.ActionVariables),
		}
		// Nested object-type properties can themselves contain object lists.
		if p.Type == "object" && len(p.NestedProps) > 0 {
			for _, npg := range p.NestedProps {
				collectNestedProperties(npg, &child, "")
			}
		}
		parent.Children = append(parent.Children, child)
	}

	for _, sub := range pg.SubGroups {
		collectNestedProperties(sub, parent, category)
	}
}

// toSelectionTypes converts parsed <selectionType> XML elements to a name list
// (e.g. ["None","Single"]), the ValueType.SelectionTypes a selection property carries.
func toSelectionTypes(xs []xmlSelectionType) []string {
	if len(xs) == 0 {
		return nil
	}
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		if x.Name != "" {
			out = append(out, x.Name)
		}
	}
	return out
}

// toTranslations converts parsed <translation> XML elements to Translation records,
// trimming caption whitespace (the XML pretty-prints chardata with indentation).
func toTranslations(xts []xmlTranslation) []Translation {
	if len(xts) == 0 {
		return nil
	}
	out := make([]Translation, 0, len(xts))
	for _, xt := range xts {
		if xt.Lang == "" {
			continue
		}
		out = append(out, Translation{Lang: xt.Lang, Text: strings.TrimSpace(xt.Text)})
	}
	return out
}

func toActionVariables(xavs []xmlActionVariable) []ActionVariable {
	if len(xavs) == 0 {
		return nil
	}
	out := make([]ActionVariable, 0, len(xavs))
	for _, x := range xavs {
		if x.Key == "" {
			continue
		}
		out = append(out, ActionVariable{Key: x.Key, Type: x.Type, Caption: x.Caption})
	}
	return out
}

// FindMPK looks in the project's widgets/ directory for an .mpk matching the widgetID.
// Returns the path to the .mpk file, or empty string if not found.
func FindMPK(projectDir string, widgetID string) (string, error) {
	// Check directory cache
	dirCacheLock.RLock()
	if dirMap, ok := dirCache[projectDir]; ok {
		if mpkPath, ok := dirMap[widgetID]; ok {
			dirCacheLock.RUnlock()
			return mpkPath, nil
		}
		dirCacheLock.RUnlock()
		// Already scanned this dir, widget not found
		return "", nil
	}
	dirCacheLock.RUnlock()

	// Scan widgets/ directory
	widgetsDir := filepath.Join(projectDir, "widgets")
	matches, err := filepath.Glob(filepath.Join(widgetsDir, "*.mpk"))
	if err != nil {
		return "", fmt.Errorf("failed to scan widgets directory: %w", err)
	}

	// Build mapping by parsing each .mpk's package.xml and widget XML.
	// Multi-widget MPKs list multiple widget IDs; map each one to this file.
	dirMap := make(map[string]string)
	for _, mpkPath := range matches {
		wids, err := getWidgetIDsFromMPK(mpkPath)
		if err != nil {
			continue // Skip unparseable files
		}
		for _, wid := range wids {
			if wid != "" {
				dirMap[wid] = mpkPath
			}
		}
	}

	// Cache the mapping
	dirCacheLock.Lock()
	dirCache[projectDir] = dirMap
	dirCacheLock.Unlock()

	return dirMap[widgetID], nil
}

// getWidgetIDsFromMPK returns ALL widget IDs declared in an .mpk package.xml.
// Multi-widget MPKs (e.g. CrusherWidgets.mpk) list multiple <widgetFile> entries.
func getWidgetIDsFromMPK(mpkPath string) ([]string, error) {
	r, err := zip.OpenReader(mpkPath)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	var widgetFilePaths []string
	var totalExtracted uint64
	for _, f := range r.File {
		if f.Name == "package.xml" {
			if f.UncompressedSize64 > maxFileSize {
				return nil, fmt.Errorf("package.xml exceeds max file size (%d > %d)", f.UncompressedSize64, maxFileSize)
			}
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, err
			}
			totalExtracted += uint64(len(data))
			if totalExtracted > maxTotalSize {
				return nil, fmt.Errorf("total extracted size exceeds limit")
			}
			var pkg xmlPackage
			if err := xml.Unmarshal(data, &pkg); err != nil {
				return nil, err
			}
			for _, wf := range pkg.ClientModule.WidgetFiles {
				widgetFilePaths = append(widgetFilePaths, wf.Path)
			}
			break
		}
	}

	var ids []string
	for _, wfPath := range widgetFilePaths {
		for _, f := range r.File {
			if f.Name != wfPath {
				continue
			}
			if f.UncompressedSize64 > maxFileSize {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				continue
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				continue
			}
			totalExtracted += uint64(len(data))
			if totalExtracted > maxTotalSize {
				return ids, fmt.Errorf("total extracted size exceeds limit")
			}
			var widget struct {
				ID string `xml:"id,attr"`
			}
			if err := xml.Unmarshal(data, &widget); err != nil {
				continue
			}
			if widget.ID != "" {
				ids = append(ids, widget.ID)
			}
		}
	}
	return ids, nil
}

// ReadEditorConfig returns the compiled editorConfig.js source for the given
// widgetID inside an .mpk, or "" if the widget ships none. The editor config is
// the sibling `<WidgetFile-basename>.editorConfig.js` of the widget's XML
// definition. It carries the widget's property-applicability logic
// (hidePropertyIn / hidePropertiesIn) that mxcli lifts into WidgetVisibilityRules.
func ReadEditorConfig(mpkPath, widgetID string) (string, error) {
	r, err := zip.OpenReader(mpkPath)
	if err != nil {
		return "", fmt.Errorf("open mpk: %w", err)
	}
	defer r.Close()

	// Find the widget-file path whose XML declares widgetID.
	var pkg xmlPackage
	for _, f := range r.File {
		if f.Name != "package.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return "", err
		}
		if err := xml.Unmarshal(data, &pkg); err != nil {
			return "", err
		}
		break
	}

	fileByName := make(map[string]*zip.File, len(r.File))
	for _, f := range r.File {
		fileByName[f.Name] = f
	}

	for _, wf := range pkg.ClientModule.WidgetFiles {
		xf := fileByName[wf.Path]
		if xf == nil {
			continue
		}
		rc, err := xf.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		var widget struct {
			ID string `xml:"id,attr"`
		}
		if err := xml.Unmarshal(data, &widget); err != nil || widget.ID != widgetID {
			continue
		}
		ecPath := strings.TrimSuffix(wf.Path, ".xml") + ".editorConfig.js"
		ec := fileByName[ecPath]
		if ec == nil {
			return "", nil // widget ships no editor config
		}
		if ec.UncompressedSize64 > maxFileSize {
			return "", fmt.Errorf("editorConfig %s exceeds max file size", ecPath)
		}
		erc, err := ec.Open()
		if err != nil {
			return "", err
		}
		defer erc.Close()
		body, err := io.ReadAll(erc)
		if err != nil {
			return "", err
		}
		return string(body), nil
	}
	return "", nil
}

// buildDefinition constructs a WidgetDefinition from a parsed xmlWidget and version string.
func buildDefinition(widget *xmlWidget, version string) *WidgetDefinition {
	platform := widget.SupportedPlatform
	if platform == "" {
		platform = "Web"
	}
	def := &WidgetDefinition{
		ID:                 widget.ID,
		Name:               widget.Name,
		Description:        widget.Description,
		Version:            version,
		IsPluggable:        widget.PluginWidget == "true",
		OfflineCapable:     widget.OfflineCapable == "true",
		NeedsEntityContext: widget.NeedsEntityContext == "true",
		SupportedPlatform:  platform,
		HelpURL:            widget.HelpURL,
		StudioCategory:     widget.StudioCategory,
		StudioProCategory:  widget.StudioProCategory,
	}
	for _, pg := range widget.PropertyGroups {
		walkPropertyGroup(pg, "", def)
	}
	// Build the document-ordered top-level property list (regular + system
	// interleaved) from the ordered Children, reusing the fully-built regular
	// PropertyDefs so they keep Category/Children/etc.
	byKey := make(map[string]*PropertyDef, len(def.Properties))
	for i := range def.Properties {
		byKey[def.Properties[i].Key] = &def.Properties[i]
	}
	for _, pg := range widget.PropertyGroups {
		collectTopLevelOrder(pg, "", def, byKey)
	}
	return def
}

// collectTopLevelOrder walks a property group's document-ordered Children and
// appends each top-level property (regular or system) to def.AllTopLevel in the
// order declared in the widget XML. Regular entries reuse the already-built
// PropertyDef (via byKey); system entries are recorded as Key + IsSystem. It does
// not descend into an object property's nested properties — only the top-level
// PropertyType order is reproduced here.
func collectTopLevelOrder(pg xmlPropGroup, parentCategory string, def *WidgetDefinition, byKey map[string]*PropertyDef) {
	category := pg.Caption
	if parentCategory != "" && category != "" {
		category = parentCategory + "::" + category
	} else if parentCategory != "" {
		category = parentCategory
	}
	for _, c := range pg.Children {
		switch {
		case c.Property != nil:
			if pd := byKey[c.Property.Key]; pd != nil {
				def.AllTopLevel = append(def.AllTopLevel, *pd)
			}
		case c.SystemProp != nil:
			// Category is the enclosing group chain (e.g. "General::Common"),
			// mirroring walkPropertyGroup; GenerateFromMPK emits it on the System
			// PropertyType so a generated widget matches mxbuild's definition.
			def.AllTopLevel = append(def.AllTopLevel, PropertyDef{Key: c.SystemProp.Key, IsSystem: true, Category: category})
		case c.SubGroup != nil:
			collectTopLevelOrder(*c.SubGroup, category, def, byKey)
		}
	}
}

// ParseMPKForWidget parses the widget XML for a specific widgetID from an .mpk file.
// Unlike ParseMPK (which reads only the first widget), this scans all widget files
// declared in package.xml to find the one whose ID matches widgetID.
// Needed for multi-widget .mpk packages (e.g. CrusherWidgets.mpk).
// Returns nil, nil when widgetID is not found in the MPK.
func ParseMPKForWidget(mpkPath string, widgetID string) (*WidgetDefinition, error) {
	cacheKey := mpkPath + "\x00" + widgetID
	defCacheLock.RLock()
	if def, ok := defCache[cacheKey]; ok {
		defCacheLock.RUnlock()
		return def, nil
	}
	defCacheLock.RUnlock()

	r, err := zip.OpenReader(mpkPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open mpk: %w", err)
	}
	defer r.Close()

	var pkg xmlPackage
	var version string
	var totalExtracted uint64
	for _, f := range r.File {
		if f.Name == "package.xml" {
			if f.UncompressedSize64 > maxFileSize {
				return nil, fmt.Errorf("package.xml exceeds max size")
			}
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("open package.xml: %w", err)
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("read package.xml: %w", err)
			}
			totalExtracted += uint64(len(data))
			if totalExtracted > maxTotalSize {
				return nil, fmt.Errorf("total extracted size exceeds limit (%d > %d)", totalExtracted, maxTotalSize)
			}
			if err := xml.Unmarshal(data, &pkg); err != nil {
				return nil, fmt.Errorf("parse package.xml: %w", err)
			}
			version = pkg.ClientModule.Version
			break
		}
	}

	for _, wf := range pkg.ClientModule.WidgetFiles {
		for _, f := range r.File {
			if f.Name != wf.Path {
				continue
			}
			if f.UncompressedSize64 > maxFileSize {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				continue
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				continue
			}
			totalExtracted += uint64(len(data))
			if totalExtracted > maxTotalSize {
				return nil, fmt.Errorf("total extracted size exceeds limit (%d > %d)", totalExtracted, maxTotalSize)
			}

			var widget xmlWidget
			if err := xml.Unmarshal(data, &widget); err != nil {
				continue
			}
			if widget.ID != widgetID {
				continue
			}

			def := buildDefinition(&widget, version)
			defCacheLock.Lock()
			defCache[cacheKey] = def
			defCacheLock.Unlock()
			return def, nil
		}
	}

	return nil, nil
}

// ParseAll parses every widget definition bundled in an MPK file and returns them all.
// For single-widget MPKs this returns a one-element slice. For multi-widget MPKs (where
// package.xml lists multiple <widgetFile> entries) every widget is returned. Errors for
// individual widgets are skipped; only fatal archive errors are returned.
func ParseAll(mpkPath string) ([]*WidgetDefinition, error) {
	ids, err := getWidgetIDsFromMPK(mpkPath)
	if err != nil {
		return nil, err
	}
	var result []*WidgetDefinition
	for _, id := range ids {
		def, err := ParseMPKForWidget(mpkPath, id)
		if err != nil || def == nil {
			continue
		}
		result = append(result, def)
	}
	return result, nil
}

// PropertyKeys returns a set of regular (non-system) property keys from the definition.
func (def *WidgetDefinition) PropertyKeys() map[string]bool {
	keys := make(map[string]bool, len(def.Properties))
	for _, p := range def.Properties {
		keys[p.Key] = true
	}
	return keys
}

// FindProperty returns the PropertyDef for the given key, or nil if not found.
func (def *WidgetDefinition) FindProperty(key string) *PropertyDef {
	for i := range def.Properties {
		if def.Properties[i].Key == key {
			return &def.Properties[i]
		}
	}
	return nil
}

// SystemPropertyKeys returns a set of system property keys from the definition.
func (def *WidgetDefinition) SystemPropertyKeys() map[string]bool {
	keys := make(map[string]bool, len(def.SystemProps))
	for _, p := range def.SystemProps {
		keys[p.Key] = true
	}
	return keys
}

// ClearCache clears all cached widget definitions and directory mappings.
// Useful for testing or when the project's widgets change.
func ClearCache() {
	defCacheLock.Lock()
	defCache = make(map[string]*WidgetDefinition)
	defCacheLock.Unlock()

	dirCacheLock.Lock()
	dirCache = make(map[string]map[string]string)
	dirCacheLock.Unlock()
}

// xmlPropertyTypeMapping maps lowercased XML property type names to their canonical camelCase forms.
var xmlPropertyTypeMapping = map[string]string{
	"attribute":    "attribute",
	"expression":   "expression",
	"texttemplate": "textTemplate",
	"widgets":      "widgets",
	"enumeration":  "enumeration",
	"boolean":      "boolean",
	"integer":      "integer",
	"datasource":   "datasource",
	"action":       "action",
	"selection":    "selection",
	"association":  "association",
	"object":       "object",
	"string":       "string",
	"decimal":      "decimal",
	"icon":         "icon",
	"image":        "image",
	"file":         "file",
}

// NormalizeType returns the canonical XML property type name.
func NormalizeType(xmlType string) string {
	lower := strings.ToLower(xmlType)
	if canonical, ok := xmlPropertyTypeMapping[lower]; ok {
		return canonical
	}
	return xmlType
}
