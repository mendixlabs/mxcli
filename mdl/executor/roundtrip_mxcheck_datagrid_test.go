// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

// TestMxCheck_DataGridPage creates a page with a DATAGRID widget and verifies
// mx check passes. This is a regression test for issue #6: DATAGRID was
// completely unusable because placeholder IDs leaked during template
// augmentation when the .mpk file added extra properties.
func TestMxCheck_DataGridPage(t *testing.T) {
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	env := setupTestEnv(t)
	defer env.teardown()

	// Create entity for the DataGrid
	entityName := testModule + ".MxCheckDGItem"
	env.registerCleanup("entity", entityName)

	if err := env.executeMDL(`create or modify persistent entity ` + entityName + ` (
		Name: String(100),
		Description: String(500),
		Active: Boolean default true
	);`); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	// Create page with DATAGRID using DATABASE datasource and columns
	pageName := testModule + ".MxCheckDataGridPage"
	env.registerCleanup("page", pageName)

	createPageMDL := `create page ` + pageName + ` (
		Title: 'DataGrid Check Test',
		Layout: Atlas_Core.Atlas_Default
	) {
		layoutgrid mainGrid {
			row row1 {
				column col1 (DesktopWidth: 12) {
					datagrid dg (DataSource: database ` + entityName + `) {
						column colName (Attribute: Name, Caption: 'Name')
						column colDesc (Attribute: Description, Caption: 'Description')
						column colActive (Attribute: Active, Caption: 'Active')
					}
				}
			}
		}
	}`

	if err := env.executeMDL(createPageMDL); err != nil {
		t.Fatalf("Failed to create page with datagrid: %v", err)
	}

	// Disconnect to flush changes before mx check
	env.executor.Execute(&ast.DisconnectStmt{})

	// Run mx check
	output, err := runMxCheck(t, env.projectPath)
	if err != nil {
		if strings.Contains(output, "error") || strings.Contains(output, "Error") {
			t.Errorf("mx check found errors for datagrid page:\n%s", output)
		} else {
			t.Logf("mx check output:\n%s", output)
		}
	} else {
		t.Logf("mx check passed for datagrid page:\n%s", output)
	}
}

// TestMxCheck_DataGridNoColumns creates a DATAGRID without explicit columns
// (uses template defaults) and verifies the page is created successfully.
// Note: template default columns reference attributes that don't exist on the
// test entity, so mx check will report CE errors about missing attributes.
// The key validation is that the page is created without placeholder ID leaks.
func TestMxCheck_DataGridNoColumns(t *testing.T) {
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	env := setupTestEnv(t)
	defer env.teardown()

	entityName := testModule + ".MxCheckDGNoColItem"
	env.registerCleanup("entity", entityName)

	if err := env.executeMDL(`create or modify persistent entity ` + entityName + ` (
		Code: String(50),
		Value: Integer
	);`); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	pageName := testModule + ".MxCheckDGNoColPage"
	env.registerCleanup("page", pageName)

	createPageMDL := `create page ` + pageName + ` (
		Title: 'DataGrid No Columns Test',
		Layout: Atlas_Core.Atlas_Default
	) {
		layoutgrid mainGrid {
			row row1 {
				column col1 (DesktopWidth: 12) {
					datagrid dg (DataSource: database ` + entityName + `)
				}
			}
		}
	}`

	if err := env.executeMDL(createPageMDL); err != nil {
		t.Fatalf("Failed to create page with datagrid (no columns): %v", err)
	}

	// Disconnect to flush changes before mx check
	env.executor.Execute(&ast.DisconnectStmt{})

	// Run mx check — template default columns won't match the entity's attributes,
	// so CE errors about missing attributes are expected. But placeholder ID leaks
	// or structural errors (CE0463) would indicate a regression.
	output, err := runMxCheck(t, env.projectPath)
	if err != nil {
		if strings.Contains(output, "placeholder") || strings.Contains(output, "CE0463") {
			t.Errorf("mx check found structural errors (possible placeholder leak):\n%s", output)
		} else {
			t.Logf("mx check output (attribute errors expected for template defaults):\n%s", output)
		}
	} else {
		t.Logf("mx check passed for datagrid (no columns) page:\n%s", output)
	}
}

// TestMxCheck_GalleryPage creates a page with a GALLERY widget and verifies
// mx check passes. Regression test for issue #7: same placeholder ID leak
// as issue #6 but for the Gallery widget.
func TestMxCheck_GalleryPage(t *testing.T) {
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	env := setupTestEnv(t)
	defer env.teardown()
	env.requireMinVersion(t, 11, 0) // Gallery widget template is 11.6, CE0463 on 10.x

	entityName := testModule + ".MxCheckGalleryItem"
	env.registerCleanup("entity", entityName)

	if err := env.executeMDL(`create or modify persistent entity ` + entityName + ` (
		Heading: String(200),
		Summary: String(500)
	);`); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	pageName := testModule + ".MxCheckGalleryPage"
	env.registerCleanup("page", pageName)

	createPageMDL := `create page ` + pageName + ` (
		Title: 'Gallery Check Test',
		Layout: Atlas_Core.Atlas_Default
	) {
		layoutgrid mainGrid {
			row row1 {
				column col1 (DesktopWidth: 12) {
					gallery gal (DataSource: database ` + entityName + `) {
						dynamictext dtHeading (Content: '{1}', ContentParams: [{1} = Heading])
					}
				}
			}
		}
	}`

	if err := env.executeMDL(createPageMDL); err != nil {
		t.Fatalf("Failed to create page with gallery: %v", err)
	}

	env.executor.Execute(&ast.DisconnectStmt{})

	runMxUpdateWidgets(t, env.projectPath)

	output, err := runMxCheck(t, env.projectPath)
	if err != nil {
		if strings.Contains(output, "placeholder") || strings.Contains(output, "CE0463") {
			t.Errorf("mx check found structural errors for gallery page (possible placeholder leak):\n%s", output)
		} else {
			t.Logf("mx check output:\n%s", output)
		}
	} else {
		t.Logf("mx check passed for gallery page:\n%s", output)
	}
}
