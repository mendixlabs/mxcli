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
	Children     []PropertyDef // nested properties for object-type properties
}

// WidgetDefinition holds the parsed definition of a pluggable widget from an .mpk file.
type WidgetDefinition struct {
	ID          string        // e.g. "com.mendix.widget.web.combobox.Combobox"
	Name        string        // e.g. "Combo box"
	Version     string        // from package.xml clientModule version
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
	// Nested properties for object type
	NestedProps []xmlPropGroup `xml:"properties>propertyGroup"`
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

			def := &WidgetDefinition{
				ID:      widget.ID,
				Name:    widget.Name,
				Version: version,
			}

			// Walk property groups to collect properties
			for _, pg := range widget.PropertyGroups {
				walkPropertyGroup(pg, "", def)
			}

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
		}

		// Parse nested properties for object-type properties
		if p.Type == "object" && len(p.NestedProps) > 0 {
			for _, npg := range p.NestedProps {
				collectNestedProperties(npg, &prop)
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

	// Build mapping by parsing each .mpk's package.xml and widget XML
	dirMap := make(map[string]string)
	for _, mpkPath := range matches {
		wid, err := getWidgetIDFromMPK(mpkPath)
		if err != nil {
			continue // Skip unparseable files
		}
		if wid != "" {
			dirMap[wid] = mpkPath
		}
	}

	// Cache the mapping
	dirCacheLock.Lock()
	dirCache[projectDir] = dirMap
	dirCacheLock.Unlock()

	return dirMap[widgetID], nil
}

// getWidgetIDFromMPK extracts the widget ID from an .mpk file without fully parsing it.
func getWidgetIDFromMPK(mpkPath string) (string, error) {
	r, err := zip.OpenReader(mpkPath)
	if err != nil {
		return "", err
	}
	defer r.Close()

	// Find package.xml to get widget file path
	var widgetFilePath string
	var totalExtracted uint64
	for _, f := range r.File {
		if f.Name == "package.xml" {
			if f.UncompressedSize64 > maxFileSize {
				return "", fmt.Errorf("package.xml exceeds max file size (%d > %d)", f.UncompressedSize64, maxFileSize)
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
			totalExtracted += uint64(len(data))
			if totalExtracted > maxTotalSize {
				return "", fmt.Errorf("total extracted size exceeds limit (%d > %d)", totalExtracted, maxTotalSize)
			}
			var pkg xmlPackage
			if err := xml.Unmarshal(data, &pkg); err != nil {
				return "", err
			}
			if len(pkg.ClientModule.WidgetFiles) > 0 {
				widgetFilePath = pkg.ClientModule.WidgetFiles[0].Path
			}
			break
		}
	}

	if widgetFilePath == "" {
		return "", nil
	}

	// Read widget XML to get the id attribute
	for _, f := range r.File {
		if f.Name == widgetFilePath {
			if f.UncompressedSize64 > maxFileSize {
				return "", fmt.Errorf("%s exceeds max file size (%d > %d)", widgetFilePath, f.UncompressedSize64, maxFileSize)
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
			totalExtracted += uint64(len(data))
			if totalExtracted > maxTotalSize {
				return "", fmt.Errorf("total extracted size exceeds limit (%d > %d)", totalExtracted, maxTotalSize)
			}

			// Quick XML parse to just get the id attribute
			var widget struct {
				ID string `xml:"id,attr"`
			}
			if err := xml.Unmarshal(data, &widget); err != nil {
				return "", err
			}
			return widget.ID, nil
		}
	}

	return "", nil
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
