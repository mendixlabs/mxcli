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
	Key          string // e.g. "staticDataSourceCaption"
	Type         string // XML type: "attribute", "expression", "textTemplate", "widgets", etc.
	Caption      string
	Description  string
	Category     string // from enclosing propertyGroup captions, joined with "::"
	Required     bool
	DefaultValue string // for enumeration/boolean/integer types
	IsList       bool
	IsSystem     bool          // true for <systemProperty> elements
	DataSource   string        // dataSource attribute reference
	EnumValues   []string      // enumeration member keys (for type="enumeration")
	Children     []PropertyDef // nested properties for object-type properties
}

// WidgetDefinition holds the parsed definition of a pluggable widget from an .mpk file.
type WidgetDefinition struct {
	ID          string        // e.g. "com.mendix.widget.web.combobox.Combobox"
	Name        string        // e.g. "Combo box"
	Version     string        // from package.xml clientModule version
	IsPluggable bool          // true if pluginWidget="true" (React), false for legacy Dojo
	Properties  []PropertyDef // regular <property> elements
	SystemProps []PropertyDef // <systemProperty> elements
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
	ID             string         `xml:"id,attr"`
	PluginWidget   string         `xml:"pluginWidget,attr"`
	Name           string         `xml:"name"`
	PropertyGroups []xmlPropGroup `xml:"properties>propertyGroup"`
}

// xmlPropGroup represents <propertyGroup caption="..."> element.
type xmlPropGroup struct {
	Caption     string          `xml:"caption,attr"`
	Properties  []xmlProperty   `xml:"property"`
	SystemProps []xmlSystemProp `xml:"systemProperty"`
	SubGroups   []xmlPropGroup  `xml:"propertyGroup"`
}

// xmlProperty represents <property key="..." type="..." ...> element.
type xmlProperty struct {
	Key          string `xml:"key,attr"`
	Type         string `xml:"type,attr"`
	DefaultValue string `xml:"defaultValue,attr"`
	Required     string `xml:"required,attr"`
	IsList       string `xml:"isList,attr"`
	DataSource   string `xml:"dataSource,attr"`
	Caption      string `xml:"caption"`
	Description  string `xml:"description"`
	// Enumeration member keys for type="enumeration" properties.
	EnumValues []xmlEnumValue `xml:"enumerationValues>enumerationValue"`
	// Nested properties for object type — two XML shapes:
	// (a) <properties><propertyGroup><property>...</property></propertyGroup></properties>
	// (b) <properties><property>...</property></properties>  (no group wrapper)
	NestedProps       []xmlPropGroup `xml:"properties>propertyGroup"`
	NestedDirectProps []xmlProperty  `xml:"properties>property"`
}

// xmlEnumValue represents <enumerationValue key="...">Caption</enumerationValue>.
type xmlEnumValue struct {
	Key string `xml:"key,attr"`
}

// enumValueKeys extracts the enumeration member keys from an XML property.
func enumValueKeys(vals []xmlEnumValue) []string {
	if len(vals) == 0 {
		return nil
	}
	keys := make([]string, 0, len(vals))
	for _, v := range vals {
		keys = append(keys, v.Key)
	}
	return keys
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
	allDefCache     = make(map[string][]*WidgetDefinition) // mpkPath -> all widget definitions in the package
	allDefCacheLock sync.RWMutex

	dirCache     = make(map[string]map[string]string) // projectDir -> (widgetID -> mpkPath)
	dirCacheLock sync.RWMutex
)

// readZipEntry reads a single zip entry with the package size guards applied.
func readZipEntry(f *zip.File, total *uint64) ([]byte, error) {
	if f.UncompressedSize64 > maxFileSize {
		return nil, fmt.Errorf("%s exceeds max file size (%d > %d)", f.Name, f.UncompressedSize64, maxFileSize)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	*total += uint64(len(data))
	if *total > maxTotalSize {
		return nil, fmt.Errorf("total extracted size exceeds limit (%d > %d)", *total, maxTotalSize)
	}
	return data, nil
}

// ParseMPKAll opens an .mpk ZIP archive and parses every widget it bundles.
// A single .mpk can ship many widgets (e.g. Charts.mpk → AreaChart, BarChart,
// PieChart, …); each widgetFile listed in package.xml becomes one definition.
func ParseMPKAll(mpkPath string) ([]*WidgetDefinition, error) {
	allDefCacheLock.RLock()
	if defs, ok := allDefCache[mpkPath]; ok {
		allDefCacheLock.RUnlock()
		return defs, nil
	}
	allDefCacheLock.RUnlock()

	r, err := zip.OpenReader(mpkPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open mpk: %w", err)
	}
	defer r.Close()

	files := make(map[string]*zip.File, len(r.File))
	for _, f := range r.File {
		files[f.Name] = f
	}

	pkgFile, ok := files["package.xml"]
	if !ok {
		return nil, fmt.Errorf("package.xml not found in mpk")
	}
	var totalExtracted uint64
	data, err := readZipEntry(pkgFile, &totalExtracted)
	if err != nil {
		return nil, fmt.Errorf("package.xml: %w", err)
	}
	var pkg xmlPackage
	if err := xml.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("failed to parse package.xml: %w", err)
	}
	version := pkg.ClientModule.Version
	if len(pkg.ClientModule.WidgetFiles) == 0 {
		return nil, fmt.Errorf("no widget file path found in package.xml")
	}

	defs := make([]*WidgetDefinition, 0, len(pkg.ClientModule.WidgetFiles))
	for _, wf := range pkg.ClientModule.WidgetFiles {
		wfile, ok := files[wf.Path]
		if !ok {
			// package.xml may reference a path not actually bundled; skip it
			// rather than failing the whole package.
			continue
		}
		wdata, err := readZipEntry(wfile, &totalExtracted)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", wf.Path, err)
		}
		var widget xmlWidget
		if err := xml.Unmarshal(wdata, &widget); err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", wf.Path, err)
		}
		def := &WidgetDefinition{
			ID:          widget.ID,
			Name:        widget.Name,
			Version:     version,
			IsPluggable: widget.PluginWidget == "true",
		}
		for _, pg := range widget.PropertyGroups {
			walkPropertyGroup(pg, "", def)
		}
		defs = append(defs, def)
	}
	if len(defs) == 0 {
		return nil, fmt.Errorf("no parseable widget files in mpk")
	}

	allDefCacheLock.Lock()
	allDefCache[mpkPath] = defs
	allDefCacheLock.Unlock()
	return defs, nil
}

// ParseMPK returns the first widget definition in an .mpk. Prefer ParseMPKAll
// (all widgets) or ParseMPKWidget (a specific widget id) for bundled packages.
func ParseMPK(mpkPath string) (*WidgetDefinition, error) {
	defs, err := ParseMPKAll(mpkPath)
	if err != nil {
		return nil, err
	}
	return defs[0], nil
}

// ParseMPKWidget returns the definition of a specific widget id within an .mpk.
// Needed for bundled packages, where the first widget is not the one wanted.
func ParseMPKWidget(mpkPath, widgetID string) (*WidgetDefinition, error) {
	defs, err := ParseMPKAll(mpkPath)
	if err != nil {
		return nil, err
	}
	for _, d := range defs {
		if d.ID == widgetID {
			return d, nil
		}
	}
	return nil, fmt.Errorf("widget %s not found in mpk %s", widgetID, mpkPath)
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
		prop := PropertyDef{
			Key:          p.Key,
			Type:         p.Type,
			Caption:      p.Caption,
			Description:  p.Description,
			Category:     category,
			Required:     p.Required == "true",
			DefaultValue: p.DefaultValue,
			IsList:       p.IsList == "true",
			DataSource:   p.DataSource,
			EnumValues:   enumValueKeys(p.EnumValues),
		}

		// Parse nested properties for object-type properties.
		// Two XML shapes coexist across Mendix widgets:
		//   (a) <properties><propertyGroup><property>...</property></propertyGroup></properties>
		//       (e.g. Accordion groups, DataGrid columns)
		//   (b) <properties><property>...</property></properties>
		//       (e.g. PopupMenu basicItems, Maps markers)
		if p.Type == "object" {
			for _, npg := range p.NestedProps {
				collectNestedProperties(npg, &prop)
			}
			for _, np := range p.NestedDirectProps {
				prop.Children = append(prop.Children, PropertyDef{
					Key:          np.Key,
					Type:         np.Type,
					Caption:      np.Caption,
					Description:  np.Description,
					Required:     np.Required == "true",
					DefaultValue: np.DefaultValue,
					IsList:       np.IsList == "true",
					DataSource:   np.DataSource,
					EnumValues:   enumValueKeys(np.EnumValues),
				})
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
// within an object-type property and appends them to the parent PropertyDef.
func collectNestedProperties(pg xmlPropGroup, parent *PropertyDef) {
	for _, p := range pg.Properties {
		child := PropertyDef{
			Key:          p.Key,
			Type:         p.Type,
			Caption:      p.Caption,
			Description:  p.Description,
			Required:     p.Required == "true",
			DefaultValue: p.DefaultValue,
			IsList:       p.IsList == "true",
			DataSource:   p.DataSource,
			EnumValues:   enumValueKeys(p.EnumValues),
		}
		parent.Children = append(parent.Children, child)
	}

	for _, sub := range pg.SubGroups {
		collectNestedProperties(sub, parent)
	}
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

	// Build mapping by parsing each .mpk's package.xml and widget XMLs. A
	// single .mpk can bundle many widgets (e.g. Charts.mpk), so register every
	// widget id in the package, not just the first.
	dirMap := make(map[string]string)
	for _, mpkPath := range matches {
		defs, err := ParseMPKAll(mpkPath)
		if err != nil {
			continue // Skip unparseable files
		}
		for _, d := range defs {
			if d.ID != "" {
				dirMap[d.ID] = mpkPath
			}
		}
	}

	// Cache the mapping
	dirCacheLock.Lock()
	dirCache[projectDir] = dirMap
	dirCacheLock.Unlock()

	return dirMap[widgetID], nil
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
	allDefCacheLock.Lock()
	allDefCache = make(map[string][]*WidgetDefinition)
	allDefCacheLock.Unlock()

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
