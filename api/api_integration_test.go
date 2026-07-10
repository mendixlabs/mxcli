// SPDX-License-Identifier: Apache-2.0

package api

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr"
)

// sourceProject is the pristine source project directory.
const sourceProject = "../mx-test-projects/test-source-app"

// sourceProjectMPR is the MPR filename inside the source project.
const sourceProjectMPR = "test-source.mpr"

// copyTestProject copies the source project to a temp directory and returns the MPR path.
func copyTestProject(t *testing.T) string {
	t.Helper()

	srcDir, err := filepath.Abs(sourceProject)
	if err != nil {
		t.Fatalf("Failed to resolve source project path: %v", err)
	}
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		t.Skipf("Source project not found: %s", srcDir)
	}

	destDir := t.TempDir()

	// Copy the MPR file
	srcMPR := filepath.Join(srcDir, sourceProjectMPR)
	destMPR := filepath.Join(destDir, sourceProjectMPR)
	if err := copyFile(srcMPR, destMPR); err != nil {
		t.Fatalf("Failed to copy MPR file: %v", err)
	}

	// Copy the mprcontents directory tree
	srcContents := filepath.Join(srcDir, "mprcontents")
	destContents := filepath.Join(destDir, "mprcontents")
	if err := copyDir(srcContents, destContents); err != nil {
		t.Fatalf("Failed to copy mprcontents: %v", err)
	}

	return destMPR
}

// copyFile copies a single file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// copyDir recursively copies a directory tree.
func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// TestIntegration_OpenProject tests opening a real Mendix project
func TestIntegration_OpenProject(t *testing.T) {
	projectPath := copyTestProject(t)

	reader, err := mpr.Open(projectPath)
	if err != nil {
		t.Fatalf("Failed to open project: %v", err)
	}
	defer reader.Close()

	t.Logf("Opened project: %s (Mendix %s)", reader.Path(), reader.ProjectVersion())
}

// TestIntegration_ListModules tests listing modules from a real project
func TestIntegration_ListModules(t *testing.T) {
	projectPath := copyTestProject(t)

	reader, err := mpr.Open(projectPath)
	if err != nil {
		t.Fatalf("Failed to open project: %v", err)
	}
	defer reader.Close()

	writer, err := mpr.NewWriter(projectPath)
	if err != nil {
		t.Fatalf("Failed to open project for writing: %v", err)
	}
	defer writer.Close()

	api := New(writer)

	modules, err := api.ListModules()
	if err != nil {
		t.Fatalf("Failed to list modules: %v", err)
	}

	if len(modules) == 0 {
		t.Fatal("Expected at least one module")
	}

	t.Logf("Found %d modules:", len(modules))
	for _, m := range modules {
		t.Logf("  - %s (ID: %s)", m.Name, m.ID)
	}
}

// TestIntegration_GetModule tests retrieving a specific module
func TestIntegration_GetModule(t *testing.T) {
	projectPath := copyTestProject(t)

	writer, err := mpr.NewWriter(projectPath)
	if err != nil {
		t.Fatalf("Failed to open project: %v", err)
	}
	defer writer.Close()

	api := New(writer)

	// First list modules to find one
	modules, err := api.ListModules()
	if err != nil {
		t.Fatalf("Failed to list modules: %v", err)
	}

	if len(modules) == 0 {
		t.Skip("No modules found in test project")
	}

	// Get the first module by name
	moduleName := modules[0].Name
	module, err := api.GetModule(moduleName)
	if err != nil {
		t.Fatalf("Failed to get module %s: %v", moduleName, err)
	}

	if module.Name != moduleName {
		t.Errorf("Module name mismatch: got %s, want %s", module.Name, moduleName)
	}

	t.Logf("Retrieved module: %s", module.Name)
}

// TestIntegration_DomainModels_GetEntity tests retrieving entities
func TestIntegration_DomainModels_GetEntity(t *testing.T) {
	projectPath := copyTestProject(t)

	writer, err := mpr.NewWriter(projectPath)
	if err != nil {
		t.Fatalf("Failed to open project: %v", err)
	}
	defer writer.Close()

	api := New(writer)

	// List modules first
	modules, err := api.ListModules()
	if err != nil {
		t.Fatalf("Failed to list modules: %v", err)
	}

	// Find entities in the domain model
	for _, module := range modules {
		dm, err := api.Reader().GetDomainModel(module.ID)
		if err != nil {
			continue
		}

		if len(dm.Entities) > 0 {
			// Try to get the first entity via the API
			entityName := module.Name + "." + dm.Entities[0].Name
			entity, err := api.DomainModels.GetEntity(entityName)
			if err != nil {
				t.Fatalf("Failed to get entity %s: %v", entityName, err)
			}

			t.Logf("Retrieved entity: %s.%s with %d attributes",
				module.Name, entity.Name, len(entity.Attributes))

			// Log attributes
			for _, attr := range entity.Attributes {
				t.Logf("  - %s: %s", attr.Name, attr.Type.GetTypeName())
			}
			return
		}
	}

	t.Skip("No entities found in test project")
}

// TestIntegration_Pages_GetPage tests retrieving pages
func TestIntegration_Pages_GetPage(t *testing.T) {
	projectPath := copyTestProject(t)

	writer, err := mpr.NewWriter(projectPath)
	if err != nil {
		t.Fatalf("Failed to open project: %v", err)
	}
	defer writer.Close()

	api := New(writer)

	// List all pages
	pages, err := api.Reader().ListPages()
	if err != nil {
		t.Fatalf("Failed to list pages: %v", err)
	}

	if len(pages) == 0 {
		t.Skip("No pages found in test project")
	}

	t.Logf("Found %d pages", len(pages))

	// Find a page with a known module
	modules, _ := api.ListModules()
	moduleIDs := make(map[string]string)
	for _, m := range modules {
		moduleIDs[string(m.ID)] = m.Name
	}

	for _, page := range pages {
		moduleName, ok := moduleIDs[string(page.ContainerID)]
		if !ok {
			continue
		}

		qualifiedName := moduleName + "." + page.Name
		retrievedPage, err := api.Pages.GetPage(qualifiedName)
		if err != nil {
			t.Fatalf("Failed to get page %s: %v", qualifiedName, err)
		}

		t.Logf("Retrieved page: %s (Title: %v, Parameters: %d)",
			qualifiedName,
			retrievedPage.Title,
			len(retrievedPage.Parameters))
		return
	}

	t.Skip("Could not find a page in a known module")
}

// TestIntegration_Microflows_GetMicroflow tests retrieving microflows
func TestIntegration_Microflows_GetMicroflow(t *testing.T) {
	projectPath := copyTestProject(t)

	writer, err := mpr.NewWriter(projectPath)
	if err != nil {
		t.Fatalf("Failed to open project: %v", err)
	}
	defer writer.Close()

	api := New(writer)

	// List all microflows
	microflows, err := api.Reader().ListMicroflows()
	if err != nil {
		t.Fatalf("Failed to list microflows: %v", err)
	}

	if len(microflows) == 0 {
		t.Skip("No microflows found in test project")
	}

	t.Logf("Found %d microflows", len(microflows))

	// Find a microflow with a known module
	modules, _ := api.ListModules()
	moduleIDs := make(map[string]string)
	for _, m := range modules {
		moduleIDs[string(m.ID)] = m.Name
	}

	for _, mf := range microflows {
		moduleName, ok := moduleIDs[string(mf.ContainerID)]
		if !ok {
			continue
		}

		qualifiedName := moduleName + "." + mf.Name
		retrievedMf, err := api.Microflows.GetMicroflow(qualifiedName)
		if err != nil {
			t.Fatalf("Failed to get microflow %s: %v", qualifiedName, err)
		}

		t.Logf("Retrieved microflow: %s (Parameters: %d)",
			qualifiedName,
			len(retrievedMf.Parameters))
		return
	}

	t.Skip("Could not find a microflow in a known module")
}

// TestIntegration_Enumerations_GetEnumeration tests retrieving enumerations
func TestIntegration_Enumerations_GetEnumeration(t *testing.T) {
	projectPath := copyTestProject(t)

	writer, err := mpr.NewWriter(projectPath)
	if err != nil {
		t.Fatalf("Failed to open project: %v", err)
	}
	defer writer.Close()

	api := New(writer)

	// List all enumerations
	enums, err := api.Reader().ListEnumerations()
	if err != nil {
		t.Fatalf("Failed to list enumerations: %v", err)
	}

	if len(enums) == 0 {
		t.Skip("No enumerations found in test project")
	}

	t.Logf("Found %d enumerations", len(enums))

	// Find an enumeration with a known module
	modules, _ := api.ListModules()
	moduleIDs := make(map[string]string)
	for _, m := range modules {
		moduleIDs[string(m.ID)] = m.Name
	}

	for _, enum := range enums {
		moduleName, ok := moduleIDs[string(enum.ContainerID)]
		if !ok {
			continue
		}

		qualifiedName := moduleName + "." + enum.Name
		retrievedEnum, err := api.Enumerations.GetEnumeration(qualifiedName)
		if err != nil {
			t.Fatalf("Failed to get enumeration %s: %v", qualifiedName, err)
		}

		t.Logf("Retrieved enumeration: %s (Values: %d)",
			qualifiedName,
			len(retrievedEnum.Values))

		for _, v := range retrievedEnum.Values {
			t.Logf("  - %s", v.Name)
		}
		return
	}

	t.Skip("Could not find an enumeration in a known module")
}

// TestIntegration_CreateEntity tests creating a new entity
func TestIntegration_CreateEntity(t *testing.T) {
	projectPath := copyTestProject(t)

	writer, err := mpr.NewWriter(projectPath)
	if err != nil {
		t.Fatalf("Failed to open temp project: %v", err)
	}
	defer writer.Close()

	api := New(writer)

	// Get the first module
	modules, err := api.ListModules()
	if err != nil {
		t.Fatalf("Failed to list modules: %v", err)
	}

	if len(modules) == 0 {
		t.Skip("No modules found")
	}

	module := modules[0]
	api.SetModule(module)

	// Create a new entity
	entity, err := api.DomainModels.CreateEntity("TestAPIEntity").
		Persistent().
		WithStringAttribute("Name", 100).
		WithIntegerAttribute("Age").
		WithBooleanAttribute("IsActive").
		WithDateTimeAttribute("CreatedAt").
		Build()

	if err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	t.Logf("Created entity: %s.%s (ID: %s)", module.Name, entity.Name, entity.ID)
	t.Logf("  Attributes: %d", len(entity.Attributes))
	for _, attr := range entity.Attributes {
		t.Logf("    - %s: %s", attr.Name, attr.Type.GetTypeName())
	}

	// Verify the entity was created by retrieving it
	qualifiedName := module.Name + "." + entity.Name
	retrieved, err := api.DomainModels.GetEntity(qualifiedName)
	if err != nil {
		t.Fatalf("Failed to retrieve created entity: %v", err)
	}

	if retrieved.Name != entity.Name {
		t.Errorf("Retrieved entity name mismatch: got %s, want %s", retrieved.Name, entity.Name)
	}

	t.Logf("Successfully verified entity creation")
}

// TestIntegration_CreateEnumeration tests creating a new enumeration
func TestIntegration_CreateEnumeration(t *testing.T) {
	projectPath := copyTestProject(t)

	writer, err := mpr.NewWriter(projectPath)
	if err != nil {
		t.Fatalf("Failed to open temp project: %v", err)
	}
	defer writer.Close()

	api := New(writer)

	modules, err := api.ListModules()
	if err != nil {
		t.Fatalf("Failed to list modules: %v", err)
	}

	if len(modules) == 0 {
		t.Skip("No modules found")
	}

	module := modules[0]
	api.SetModule(module)

	// Create a new enumeration
	enum, err := api.Enumerations.CreateEnumeration("TestAPIStatus").
		WithValue("Pending", "Pending").
		WithValue("InProgress", "In Progress").
		WithValue("Completed", "Completed").
		WithValue("Cancelled", "Cancelled").
		Build()

	if err != nil {
		t.Fatalf("Failed to create enumeration: %v", err)
	}

	t.Logf("Created enumeration: %s.%s (ID: %s)", module.Name, enum.Name, enum.ID)
	t.Logf("  Values: %d", len(enum.Values))
	for _, v := range enum.Values {
		t.Logf("    - %s", v.Name)
	}

	// Verify by retrieving
	qualifiedName := module.Name + "." + enum.Name
	retrieved, err := api.Enumerations.GetEnumeration(qualifiedName)
	if err != nil {
		t.Fatalf("Failed to retrieve created enumeration: %v", err)
	}

	if len(retrieved.Values) != 4 {
		t.Errorf("Expected 4 values, got %d", len(retrieved.Values))
	}

	t.Logf("Successfully verified enumeration creation")
}

// TestIntegration_CreateMicroflow tests creating a new microflow
func TestIntegration_CreateMicroflow(t *testing.T) {
	projectPath := copyTestProject(t)

	writer, err := mpr.NewWriter(projectPath)
	if err != nil {
		t.Fatalf("Failed to open temp project: %v", err)
	}
	defer writer.Close()

	api := New(writer)

	modules, err := api.ListModules()
	if err != nil {
		t.Fatalf("Failed to list modules: %v", err)
	}

	if len(modules) == 0 {
		t.Skip("No modules found")
	}

	module := modules[0]
	api.SetModule(module)

	// Create a new microflow
	mf, err := api.Microflows.CreateMicroflow("ACT_TestAPI_DoSomething").
		WithStringParameter("Message").
		WithBooleanParameter("IsEnabled").
		ReturnsBoolean().
		Build()

	if err != nil {
		t.Fatalf("Failed to create microflow: %v", err)
	}

	t.Logf("Created microflow: %s.%s (ID: %s)", module.Name, mf.Name, mf.ID)
	t.Logf("  Parameters: %d", len(mf.Parameters))
	for _, p := range mf.Parameters {
		t.Logf("    - %s: %s", p.Name, p.Type.GetTypeName())
	}

	// Verify by retrieving
	qualifiedName := module.Name + "." + mf.Name
	retrieved, err := api.Microflows.GetMicroflow(qualifiedName)
	if err != nil {
		t.Fatalf("Failed to retrieve created microflow: %v", err)
	}

	if retrieved.Name != mf.Name {
		t.Errorf("Retrieved microflow name mismatch: got %s, want %s", retrieved.Name, mf.Name)
	}

	t.Logf("Successfully verified microflow creation")
}

// TestIntegration_EntityBuilder_WithModule tests the fluent API with explicit module
func TestIntegration_EntityBuilder_WithModule(t *testing.T) {
	projectPath := copyTestProject(t)

	writer, err := mpr.NewWriter(projectPath)
	if err != nil {
		t.Fatalf("Failed to open temp project: %v", err)
	}
	defer writer.Close()

	api := New(writer)

	modules, err := api.ListModules()
	if err != nil {
		t.Fatalf("Failed to list modules: %v", err)
	}

	if len(modules) == 0 {
		t.Skip("No modules found")
	}

	module := modules[0]

	// Create entity using InModule() instead of SetModule()
	entity, err := api.DomainModels.CreateEntity("TestEntityWithModule").
		InModule(module).
		Persistent().
		WithStringAttribute("Title", 200).
		WithDecimalAttribute("Amount").
		Build()

	if err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	t.Logf("Created entity with InModule(): %s.%s", module.Name, entity.Name)
}
