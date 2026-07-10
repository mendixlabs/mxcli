// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/widgets/definitions"
)

func TestRegistryLoadsAllEmbeddedDefinitions(t *testing.T) {
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("NewWidgetRegistry() error: %v", err)
	}

	// We expect 9 embedded definitions (combobox, gallery, image, barcodescanner, 4 filters, dropdownsort)
	if got := reg.Count(); got != 9 {
		t.Errorf("registry count = %d, want 9", got)
	}
}

func TestRegistryGetByMDLName(t *testing.T) {
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("NewWidgetRegistry() error: %v", err)
	}

	tests := []struct {
		mdlName  string
		widgetID string
	}{
		{"combobox", "com.mendix.widget.web.combobox.Combobox"},
		{"gallery", "com.mendix.widget.web.gallery.Gallery"},
	}

	for _, tt := range tests {
		t.Run(tt.mdlName, func(t *testing.T) {
			def, ok := reg.Get(tt.mdlName)
			if !ok {
				t.Fatalf("Get(%q) not found", tt.mdlName)
			}
			if def.WidgetID != tt.widgetID {
				t.Errorf("WidgetID = %q, want %q", def.WidgetID, tt.widgetID)
			}
		})
	}
}

func TestRegistryGetCaseInsensitive(t *testing.T) {
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("NewWidgetRegistry() error: %v", err)
	}

	// Should work with any case
	for _, name := range []string{"combobox", "ComboBox", "combobox", "Combobox"} {
		def, ok := reg.Get(name)
		if !ok {
			t.Errorf("Get(%q) not found", name)
			continue
		}
		if def.MDLName != "combobox" {
			t.Errorf("Get(%q).MDLName = %q, want combobox", name, def.MDLName)
		}
	}
}

func TestRegistryGetUnknownWidget(t *testing.T) {
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("NewWidgetRegistry() error: %v", err)
	}

	_, ok := reg.Get("NONEXISTENT")
	if ok {
		t.Error("Get(NONEXISTENT) should return false")
	}
}

func TestRegistryGetByWidgetID(t *testing.T) {
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("NewWidgetRegistry() error: %v", err)
	}

	def, ok := reg.GetByWidgetID("com.mendix.widget.web.gallery.Gallery")
	if !ok {
		t.Fatal("GetByWidgetID(Gallery) not found")
	}
	if def.MDLName != "gallery" {
		t.Errorf("MDLName = %q, want gallery", def.MDLName)
	}
}

func TestAllEmbeddedDefinitionsAreValidJSON(t *testing.T) {
	entries, err := definitions.EmbeddedFS.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir error: %v", err)
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".def.json") {
			continue
		}

		t.Run(entry.Name(), func(t *testing.T) {
			data, err := definitions.EmbeddedFS.ReadFile(entry.Name())
			if err != nil {
				t.Fatalf("ReadFile error: %v", err)
			}

			var def WidgetDefinition
			if err := json.Unmarshal(data, &def); err != nil {
				t.Fatalf("json unmarshal error: %v", err)
			}

			// Validate required fields
			if def.WidgetID == "" {
				t.Error("widgetId is empty")
			}
			if def.MDLName == "" {
				t.Error("mdlName is empty")
			}
			if def.TemplateFile == "" {
				t.Error("templateFile is empty")
			}

			// Template-only widgets (e.g., DROPDOWNSORT) may have no mappings — that's valid
		})
	}
}

func TestRegistryLoadUserDefinitions(t *testing.T) {
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("NewWidgetRegistry() error: %v", err)
	}

	// Create a temp directory with a custom definition
	tmpDir := t.TempDir()
	widgetsDir := filepath.Join(tmpDir, ".mxcli", "widgets")
	if err := os.MkdirAll(widgetsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}

	customDef := `{
		"widgetId": "com.example.custom.MyWidget",
		"mdlName": "MYWIDGET",
		"templateFile": "mywidget.json",
		"defaultEditable": "Always",
		"propertyMappings": [
			{"propertyKey": "value", "source": "Attribute", "operation": "attribute"}
		]
	}`

	defPath := filepath.Join(widgetsDir, "mywidget.def.json")
	if err := os.WriteFile(defPath, []byte(customDef), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	// Create a fake project file in the temp directory
	projectPath := filepath.Join(tmpDir, "App.mpr")

	// Load user definitions
	if err := reg.LoadUserDefinitions(projectPath); err != nil {
		t.Fatalf("LoadUserDefinitions error: %v", err)
	}

	// The custom widget should now be found
	def, ok := reg.Get("MYWIDGET")
	if !ok {
		t.Fatal("custom widget MYWIDGET not found after LoadUserDefinitions")
	}
	if def.WidgetID != "com.example.custom.MyWidget" {
		t.Errorf("WidgetID = %q, want com.example.custom.MyWidget", def.WidgetID)
	}

	// Built-in widgets should still be available
	_, ok = reg.Get("combobox")
	if !ok {
		t.Error("built-in combobox lost after LoadUserDefinitions")
	}
}

func TestNewWidgetRegistryWithOps_ExtendsKnownOperations(t *testing.T) {
	// A definition with a custom operation should fail with default ops
	customDef := &WidgetDefinition{
		WidgetID: "com.example.Custom",
		MDLName:  "CUSTOM",
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "prop", Source: "Attribute", Operation: "customOp"},
		},
	}

	// Default registry should reject custom operation
	defaultReg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("NewWidgetRegistry() error: %v", err)
	}
	if err := defaultReg.validateDefinitionOperations(customDef, "custom.def.json"); err == nil {
		t.Error("expected error for unknown operation 'customOp' with default ops, got nil")
	}

	// Extended registry should accept custom operation
	extReg, err := NewWidgetRegistryWithOps(map[string]bool{"customOp": true})
	if err != nil {
		t.Fatalf("NewWidgetRegistryWithOps() error: %v", err)
	}
	if err := extReg.validateDefinitionOperations(customDef, "custom.def.json"); err != nil {
		t.Errorf("unexpected error with extended ops: %v", err)
	}
}

func TestValidateDefinitionOperations_MappingOrderDependency(t *testing.T) {
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("NewWidgetRegistry() error: %v", err)
	}

	// Association before DataSource should fail validation
	badDef := &WidgetDefinition{
		WidgetID: "com.example.Bad",
		MDLName:  "BAD",
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "assocProp", Source: "Association", Operation: "association"},
			{PropertyKey: "dsProp", Source: "DataSource", Operation: "datasource"},
		},
	}
	if err := reg.validateDefinitionOperations(badDef, "bad.def.json"); err == nil {
		t.Error("expected error for Association before DataSource, got nil")
	}

	// DataSource before Association should pass
	goodDef := &WidgetDefinition{
		WidgetID: "com.example.Good",
		MDLName:  "GOOD",
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "dsProp", Source: "DataSource", Operation: "datasource"},
			{PropertyKey: "assocProp", Source: "Association", Operation: "association"},
		},
	}
	if err := reg.validateDefinitionOperations(goodDef, "good.def.json"); err != nil {
		t.Errorf("unexpected error for DataSource before Association: %v", err)
	}

	// Association in mode should also validate order
	modeDef := &WidgetDefinition{
		WidgetID: "com.example.Mode",
		MDLName:  "MODE",
		Modes: []WidgetMode{
			{
				Name: "bad",
				PropertyMappings: []PropertyMapping{
					{PropertyKey: "assocProp", Source: "Association", Operation: "association"},
					{PropertyKey: "dsProp", Source: "DataSource", Operation: "datasource"},
				},
			},
		},
	}
	if err := reg.validateDefinitionOperations(modeDef, "mode.def.json"); err == nil {
		t.Error("expected error for Association before DataSource in mode, got nil")
	}
}

func TestValidateDefinitionOperations_SourceOperationCompatibility(t *testing.T) {
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("NewWidgetRegistry() error: %v", err)
	}

	// Source "Attribute" with Operation "association" should fail
	badDef := &WidgetDefinition{
		WidgetID: "com.example.Bad",
		MDLName:  "BAD",
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "prop", Source: "Attribute", Operation: "association"},
		},
	}
	if err := reg.validateDefinitionOperations(badDef, "bad.def.json"); err == nil {
		t.Error("expected error for Source='Attribute' with Operation='association', got nil")
	}

	// Source "Association" with Operation "attribute" should fail
	badDef2 := &WidgetDefinition{
		WidgetID: "com.example.Bad2",
		MDLName:  "BAD2",
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "prop", Source: "Association", Operation: "attribute"},
		},
	}
	if err := reg.validateDefinitionOperations(badDef2, "bad2.def.json"); err == nil {
		t.Error("expected error for Source='Association' with Operation='attribute', got nil")
	}
}

func TestEmbeddedDefinitionsValidateRequiredFields(t *testing.T) {
	// All embedded definitions must have non-empty WidgetID and MDLName
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("NewWidgetRegistry() error: %v", err)
	}

	for _, def := range reg.All() {
		if def.WidgetID == "" {
			t.Errorf("embedded definition with MDLName=%q has empty WidgetID", def.MDLName)
		}
		if def.MDLName == "" {
			t.Errorf("embedded definition with WidgetID=%q has empty MDLName", def.WidgetID)
		}
	}
}

func TestRegistryUserDefinitionOverrideLogsWarning(t *testing.T) {
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("NewWidgetRegistry() error: %v", err)
	}

	// Create a user definition that overrides the built-in COMBOBOX
	tmpDir := t.TempDir()
	widgetsDir := filepath.Join(tmpDir, ".mxcli", "widgets")
	if err := os.MkdirAll(widgetsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}

	overrideDef := `{
		"widgetId": "com.mendix.widget.web.combobox.Combobox",
		"mdlName": "combobox",
		"templateFile": "combobox.json",
		"defaultEditable": "Always",
		"propertyMappings": [
			{"propertyKey": "value", "source": "Attribute", "operation": "attribute"}
		]
	}`

	defPath := filepath.Join(widgetsDir, "combobox-override.def.json")
	if err := os.WriteFile(defPath, []byte(overrideDef), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	projectPath := filepath.Join(tmpDir, "App.mpr")
	if err := reg.LoadUserDefinitions(projectPath); err != nil {
		t.Fatalf("LoadUserDefinitions error: %v", err)
	}

	if !strings.Contains(buf.String(), "combobox") {
		t.Errorf("expected warning log about overriding combobox, got: %q", buf.String())
	}
}

func TestRegistryComboboxModes(t *testing.T) {
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("NewWidgetRegistry() error: %v", err)
	}

	def, ok := reg.Get("combobox")
	if !ok {
		t.Fatal("combobox not found")
	}

	if len(def.Modes) != 2 {
		t.Fatalf("modes count = %d, want 2", len(def.Modes))
	}

	// First mode: association (conditional)
	if def.Modes[0].Name != "association" {
		t.Errorf("first mode name = %q, want association", def.Modes[0].Name)
	}
	if def.Modes[0].Condition != "hasDataSource" {
		t.Errorf("association mode condition = %q, want hasDataSource", def.Modes[0].Condition)
	}
	if len(def.Modes[0].PropertyMappings) != 4 {
		t.Errorf("association mode mappings = %d, want 4", len(def.Modes[0].PropertyMappings))
	}

	// Second mode: default (no condition)
	if def.Modes[1].Name != "default" {
		t.Errorf("second mode name = %q, want default", def.Modes[1].Name)
	}
	if len(def.Modes[1].PropertyMappings) != 1 {
		t.Errorf("default mode mappings = %d, want 1", len(def.Modes[1].PropertyMappings))
	}
}

func TestRegistryGalleryChildSlots(t *testing.T) {
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("NewWidgetRegistry() error: %v", err)
	}

	def, ok := reg.Get("gallery")
	if !ok {
		t.Fatal("gallery not found")
	}

	if len(def.ChildSlots) != 3 {
		t.Fatalf("childSlots count = %d, want 3", len(def.ChildSlots))
	}

	// Verify slot mappings
	slotsByContainer := make(map[string]ChildSlotMapping)
	for _, slot := range def.ChildSlots {
		slotsByContainer[slot.MDLContainer] = slot
	}

	contentSlot, ok := slotsByContainer["template"]
	if !ok {
		t.Fatal("template slot not found")
	}
	if contentSlot.PropertyKey != "content" {
		t.Errorf("template slot propertyKey = %q, want content", contentSlot.PropertyKey)
	}

	emptySlot, ok := slotsByContainer["emptyplaceholder"]
	if !ok {
		t.Fatal("emptyplaceholder slot not found")
	}
	if emptySlot.PropertyKey != "emptyPlaceholder" {
		t.Errorf("emptyplaceholder slot propertyKey = %q, want emptyPlaceholder", emptySlot.PropertyKey)
	}

	// FILTER must match what DESCRIBE outputs ("FILTER"), not the BSON property name
	filterSlot, ok := slotsByContainer["filter"]
	if !ok {
		t.Fatal("filter slot not found — mdlContainer must be 'filter' to match describe output")
	}
	if filterSlot.PropertyKey != "filtersPlaceholder" {
		t.Errorf("filter slot propertyKey = %q, want filtersPlaceholder", filterSlot.PropertyKey)
	}
}

func TestGallerySelectionDefaultIsSingle(t *testing.T) {
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("NewWidgetRegistry() error: %v", err)
	}

	def, ok := reg.Get("gallery")
	if !ok {
		t.Fatal("gallery not found")
	}

	// Find itemSelection mapping
	for _, m := range def.PropertyMappings {
		if m.PropertyKey == "itemSelection" {
			if m.Default != "Single" {
				t.Errorf("itemSelection default = %q, want %q", m.Default, "Single")
			}
			return
		}
	}
	t.Fatal("itemSelection mapping not found in gallery definition")
}

func TestComboboxAssociationModeUsesAssociationSource(t *testing.T) {
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("NewWidgetRegistry() error: %v", err)
	}

	def, ok := reg.Get("combobox")
	if !ok {
		t.Fatal("combobox not found")
	}

	// Find association mode
	for _, mode := range def.Modes {
		if mode.Name != "association" {
			continue
		}
		for _, m := range mode.PropertyMappings {
			if m.PropertyKey == "attributeAssociation" {
				if m.Source != "Association" {
					t.Errorf("attributeAssociation source = %q, want %q — 'Attribute' source populates AttributePath but opAssociation reads AssocPath", m.Source, "Association")
				}
				if m.Operation != "association" {
					t.Errorf("attributeAssociation operation = %q, want %q", m.Operation, "association")
				}
				return
			}
		}
	}
	t.Fatal("attributeAssociation mapping not found in combobox association mode")
}
