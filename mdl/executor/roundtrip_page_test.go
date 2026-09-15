// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// --- V3 Page Syntax Tests ---

// stripDescribeArtifacts removes the non-MDL lines from DESCRIBE output (the
// `-- Page:` comment header and the SQL*Plus `/` terminator) so the body can be
// fed back through the parser.
func stripDescribeArtifacts(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if t == "/" || strings.HasPrefix(t, "-- Page:") {
			continue
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

// TestRoundtripPage_DescribeReparses guarantees DESCRIBE PAGE output re-parses
// through the MDL parser — issue #626 (previously only substring-asserted). The
// empty page is the regression case: a widget-less page used to describe without
// a `{ }` body block, which the CREATE PAGE grammar rejects on re-parse.
func TestRoundtripPage_DescribeReparses(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	entityName := testModule + ".RtReparseEntity"
	env.registerCleanup("entity", entityName)
	if err := env.executeMDL(`create or modify persistent entity ` + entityName + ` ( Name: String(100) );`); err != nil {
		t.Fatalf("create entity: %v", err)
	}

	emptyPage := testModule + ".RtEmptyPage"
	widgetPage := testModule + ".RtWidgetPage"
	env.registerCleanup("page", emptyPage)
	env.registerCleanup("page", widgetPage)

	// A widget-less page must still supply the `{ }` body block — the grammar
	// requires it. This is exactly what DESCRIBE used to omit (#626).
	if err := env.executeMDL(`create page ` + emptyPage + ` ( Title: 'Empty', Layout: Atlas_Core.Atlas_Default ) { }`); err != nil {
		t.Fatalf("create empty page: %v", err)
	}
	if err := env.executeMDL(`create page ` + widgetPage + ` ( Title: 'Grid', Layout: Atlas_Core.Atlas_Default ) {
		datagrid dg (DataSource: database ` + entityName + `) {
			column colName (Attribute: Name, Caption: 'Name')
		}
	}`); err != nil {
		t.Fatalf("create widget page: %v", err)
	}

	for _, pg := range []string{emptyPage, widgetPage} {
		output, err := env.describeMDL(`describe page ` + pg + `;`)
		if err != nil {
			t.Fatalf("describe %s: %v", pg, err)
		}
		reparse := stripDescribeArtifacts(output)
		if _, errs := visitor.Build(reparse); len(errs) > 0 {
			t.Errorf("DESCRIBE PAGE %s does not re-parse (#626): %v\n--- output ---\n%s", pg, errs, reparse)
		}
	}
}

// TestRoundtripPage_V3DataGridColumns tests that DataGrid columns are correctly created with V3 syntax.
// This is a regression test for the bug where user-defined columns were ignored and template columns were used.
func TestRoundtripPage_V3DataGridColumns(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	// First create an entity to use with the DataGrid
	entityName := testModule + ".V3GridTestEntity"
	env.registerCleanup("entity", entityName)

	createEntityMDL := `create or modify persistent entity ` + entityName + ` (
		FirstName: String(100),
		LastName: String(100),
		Email: String(200)
	);`

	if err := env.executeMDL(createEntityMDL); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	// Create page with DataGrid using V3 syntax and custom columns
	pageName := testModule + ".V3DataGridColumnsPage"
	env.registerCleanup("page", pageName)

	createPageMDL := `create page ` + pageName + ` (
		Title: 'DataGrid Columns Test',
		Layout: Atlas_Core.Atlas_Default
	) {
		layoutgrid mainGrid {
			row row1 {
				column col1 (DesktopWidth: 12) {
					datagrid dg (DataSource: database ` + entityName + `) {
						column colFirst (Attribute: FirstName, Caption: 'First Name')
						column colLast (Attribute: LastName, Caption: 'Last Name')
						column colEmail (Attribute: Email, Caption: 'Email Address')
					}
				}
			}
		}
	}`

	if err := env.executeMDL(createPageMDL); err != nil {
		t.Fatalf("Failed to create page with V3 DataGrid: %v", err)
	}

	// Describe the page
	output, err := env.describeMDL(`describe page ` + pageName + `;`)
	if err != nil {
		t.Fatalf("Failed to describe page: %v", err)
	}

	// Verify that the user-defined columns are present (not template columns)
	expectedColumns := []string{"FirstName", "LastName", "Email"}
	unexpectedColumns := []string{"Code", "Price", "Stock", "IsActive"} // Template columns (excluding Name which might be in entity)

	for _, col := range expectedColumns {
		if !strings.Contains(output, col) {
			t.Errorf("Expected column '%s' not found in describe output.\nOutput:\n%s", col, output)
		}
	}

	for _, col := range unexpectedColumns {
		if strings.Contains(output, col) {
			t.Errorf("Unexpected template column '%s' found in describe output - user columns may have been ignored.\nOutput:\n%s", col, output)
		}
	}

	// Also verify column captions
	if !strings.Contains(output, "First Name") {
		t.Error("Expected caption 'First Name' not found in output")
	}
	if !strings.Contains(output, "Last Name") {
		t.Error("Expected caption 'Last Name' not found in output")
	}
	if !strings.Contains(output, "Email Address") {
		t.Error("Expected caption 'Email Address' not found in output")
	}

	t.Logf("V3 DataGrid columns roundtrip successful:\n%s", output)
}

// TestRoundtripPage_V3DataGridNoColumns tests DataGrid without columns uses template defaults.
func TestRoundtripPage_V3DataGridNoColumns(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	// First create an entity
	entityName := testModule + ".V3GridNoColEntity"
	env.registerCleanup("entity", entityName)

	createEntityMDL := `create or modify persistent entity ` + entityName + ` (
		Name: String(100)
	);`

	if err := env.executeMDL(createEntityMDL); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	// Create page with DataGrid without explicit columns
	pageName := testModule + ".V3DataGridNoColumnsPage"
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
		t.Fatalf("Failed to create page with V3 DataGrid (no columns): %v", err)
	}

	// Describe the page - should have template columns
	output, err := env.describeMDL(`describe page ` + pageName + `;`)
	if err != nil {
		t.Fatalf("Failed to describe page: %v", err)
	}

	// When no columns are specified, template columns should be used
	if !strings.Contains(output, "data GRID") && !strings.Contains(output, "datagrid") {
		t.Error("Expected datagrid in output")
	}

	t.Logf("V3 DataGrid (no columns) roundtrip successful:\n%s", output)
}

// TestRoundtripPage_V3DataGridWithOrderBy tests DataGrid with ORDER BY clause.
func TestRoundtripPage_V3DataGridWithOrderBy(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	// Create entity
	entityName := testModule + ".V3GridOrderEntity"
	env.registerCleanup("entity", entityName)

	createEntityMDL := `create or modify persistent entity ` + entityName + ` (
		Name: String(100),
		Price: Decimal,
		Stock: Integer
	);`

	if err := env.executeMDL(createEntityMDL); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	// Create page with DataGrid with ORDER BY
	pageName := testModule + ".V3DataGridOrderByPage"
	env.registerCleanup("page", pageName)

	createPageMDL := `create page ` + pageName + ` (
		Title: 'DataGrid OrderBy Test',
		Layout: Atlas_Core.Atlas_Default
	) {
		datagrid dg (DataSource: database from ` + entityName + ` sort by Name asc) {
			column colName (Attribute: Name, Caption: 'Product Name')
			column colPrice (Attribute: Price, Caption: 'Price')
			column colStock (Attribute: Stock, Caption: 'In Stock')
		}
	}`

	if err := env.executeMDL(createPageMDL); err != nil {
		t.Fatalf("Failed to create page with V3 DataGrid OrderBy: %v", err)
	}

	// Describe the page
	output, err := env.describeMDL(`describe page ` + pageName + `;`)
	if err != nil {
		t.Fatalf("Failed to describe page: %v", err)
	}

	// Verify columns are present
	if !strings.Contains(output, "Name") {
		t.Error("Expected column 'Name' in output")
	}
	if !strings.Contains(output, "Price") {
		t.Error("Expected column 'Price' in output")
	}
	if !strings.Contains(output, "Stock") {
		t.Error("Expected column 'Stock' in output")
	}

	t.Logf("V3 DataGrid with OrderBy roundtrip successful:\n%s", output)
}

// TestRoundtripPage_V3DataGridWithFilter tests DataGrid with WHERE filter.
func TestRoundtripPage_V3DataGridWithFilter(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	// Create entity
	entityName := testModule + ".V3GridFilterEntity"
	env.registerCleanup("entity", entityName)

	createEntityMDL := `create or modify persistent entity ` + entityName + ` (
		Name: String(100),
		IsActive: Boolean default true,
		Stock: Integer
	);`

	if err := env.executeMDL(createEntityMDL); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	// Create page with DataGrid with WHERE filter
	pageName := testModule + ".V3DataGridFilterPage"
	env.registerCleanup("page", pageName)

	createPageMDL := `create page ` + pageName + ` (
		Title: 'DataGrid Filter Test',
		Layout: Atlas_Core.Atlas_Default
	) {
		datagrid dg (
			DataSource: database from ` + entityName + ` where [IsActive = true] sort by Name asc
		) {
			column colName (Attribute: Name, Caption: 'Name')
			column colStock (Attribute: Stock, Caption: 'Stock')
			column colActive (Attribute: IsActive, Caption: 'Active')
		}
	}`

	if err := env.executeMDL(createPageMDL); err != nil {
		t.Fatalf("Failed to create page with V3 DataGrid filter: %v", err)
	}

	// Describe the page
	output, err := env.describeMDL(`describe page ` + pageName + `;`)
	if err != nil {
		t.Fatalf("Failed to describe page: %v", err)
	}

	// Verify columns are present
	expectedColumns := []string{"Name", "Stock", "IsActive"}
	for _, col := range expectedColumns {
		if !strings.Contains(output, col) {
			t.Errorf("Expected column '%s' in output", col)
		}
	}

	t.Logf("V3 DataGrid with filter roundtrip successful:\n%s", output)
}

// TestRoundtripPage_V3DataGridWithControlBar tests DataGrid with CONTROLBAR.
func TestRoundtripPage_V3DataGridWithControlBar(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()
	env.requireMinVersion(t, 11, 0) // CREATE PAGE with parameters requires 11.0+

	// Create entity
	entityName := testModule + ".V3GridControlEntity"
	env.registerCleanup("entity", entityName)

	createEntityMDL := `create or modify persistent entity ` + entityName + ` (
		Name: String(100),
		Code: String(50)
	);`

	if err := env.executeMDL(createEntityMDL); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	// Create a simple edit page for the control bar action
	editPageName := testModule + ".V3GridControlEditPage"
	env.registerCleanup("page", editPageName)

	createEditPageMDL := `create page ` + editPageName + ` (
		Params: { $Item: ` + entityName + ` },
		Title: 'Edit Item',
		Layout: Atlas_Core.PopupLayout
	) {
		dataview dv (DataSource: $Item) {
			textbox txtName (Label: 'Name', Attribute: Name)
		}
	}`

	if err := env.executeMDL(createEditPageMDL); err != nil {
		t.Fatalf("Failed to create edit page: %v", err)
	}

	// Create page with DataGrid with CONTROLBAR
	pageName := testModule + ".V3DataGridControlBarPage"
	env.registerCleanup("page", pageName)

	createPageMDL := `create page ` + pageName + ` (
		Title: 'DataGrid ControlBar Test',
		Layout: Atlas_Core.Atlas_Default
	) {
		layoutgrid mainGrid {
			row row1 {
				column col1 (DesktopWidth: 12) {
					datagrid dg (DataSource: database ` + entityName + `) {
						controlbar controlBar1 {
							actionbutton btnNew (
								Caption: 'New Item',
								Action: create_object ` + entityName + ` then show_page ` + editPageName + `,
								ButtonStyle: Primary
							)
						}
						column colName (Attribute: Name, Caption: 'Name')
						column colCode (Attribute: Code, Caption: 'Code')
					}
				}
			}
		}
	}`

	if err := env.executeMDL(createPageMDL); err != nil {
		t.Fatalf("Failed to create page with V3 DataGrid ControlBar: %v", err)
	}

	// Describe the page
	output, err := env.describeMDL(`describe page ` + pageName + `;`)
	if err != nil {
		t.Fatalf("Failed to describe page: %v", err)
	}

	// Verify columns are present (not template columns)
	if !strings.Contains(output, "Name") {
		t.Error("Expected column 'Name' in output")
	}
	if !strings.Contains(output, "Code") {
		t.Error("Expected column 'Code' in output")
	}

	// Verify template columns are NOT present
	if strings.Contains(output, "Price") || strings.Contains(output, "Stock") {
		t.Error("Unexpected template columns found - user columns may have been ignored")
	}

	t.Logf("V3 DataGrid with ControlBar roundtrip successful:\n%s", output)
}

// TestRoundtripPage_V3DataGridManyColumns tests DataGrid with many columns.
func TestRoundtripPage_V3DataGridManyColumns(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	// Create entity with many attributes
	entityName := testModule + ".V3GridManyColEntity"
	env.registerCleanup("entity", entityName)

	createEntityMDL := `create or modify persistent entity ` + entityName + ` (
		Field1: String(100),
		Field2: String(100),
		Field3: String(100),
		Field4: Integer,
		Field5: Decimal,
		Field6: Boolean default false,
		Field7: DateTime
	);`

	if err := env.executeMDL(createEntityMDL); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	// Create page with DataGrid with many columns
	pageName := testModule + ".V3DataGridManyColumnsPage"
	env.registerCleanup("page", pageName)

	createPageMDL := `create page ` + pageName + ` (
		Title: 'DataGrid Many Columns Test',
		Layout: Atlas_Core.Atlas_Default
	) {
		datagrid dg (DataSource: database ` + entityName + `) {
			column col1 (Attribute: Field1, Caption: 'First Field')
			column col2 (Attribute: Field2, Caption: 'Second Field')
			column col3 (Attribute: Field3, Caption: 'Third Field')
			column col4 (Attribute: Field4, Caption: 'Integer Field')
			column col5 (Attribute: Field5, Caption: 'Decimal Field')
			column col6 (Attribute: Field6, Caption: 'Boolean Field')
			column col7 (Attribute: Field7, Caption: 'DateTime Field')
		}
	}`

	if err := env.executeMDL(createPageMDL); err != nil {
		t.Fatalf("Failed to create page with many DataGrid columns: %v", err)
	}

	// Describe the page
	output, err := env.describeMDL(`describe page ` + pageName + `;`)
	if err != nil {
		t.Fatalf("Failed to describe page: %v", err)
	}

	// Verify all 7 columns are present
	expectedFields := []string{"Field1", "Field2", "Field3", "Field4", "Field5", "Field6", "Field7"}
	for _, field := range expectedFields {
		if !strings.Contains(output, field) {
			t.Errorf("Expected column '%s' not found in output", field)
		}
	}

	// Verify template columns are NOT present
	templateColumns := []string{"Code", "Price", "Stock", "IsActive"}
	for _, col := range templateColumns {
		if strings.Contains(output, col) {
			t.Errorf("Unexpected template column '%s' found - user columns may have been ignored", col)
		}
	}

	// Count columns in output (rough check)
	columnCount := strings.Count(output, "column ")
	if columnCount < 7 {
		t.Errorf("Expected at least 7 columns, found %d", columnCount)
	}

	t.Logf("V3 DataGrid with many columns roundtrip successful:\n%s", output)
}

// TestRoundtripPage_V3DataGridComplexFilter tests DataGrid with complex WHERE clause.
func TestRoundtripPage_V3DataGridComplexFilter(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	// Create entity
	entityName := testModule + ".V3GridComplexEntity"
	env.registerCleanup("entity", entityName)

	createEntityMDL := `create or modify persistent entity ` + entityName + ` (
		Name: String(100),
		Code: String(50),
		Price: Decimal,
		Stock: Integer,
		IsActive: Boolean default true
	);`

	if err := env.executeMDL(createEntityMDL); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	// Create page with DataGrid with complex WHERE and multiple OrderBy
	pageName := testModule + ".V3DataGridComplexFilterPage"
	env.registerCleanup("page", pageName)

	createPageMDL := `create page ` + pageName + ` (
		Title: 'Complex Filter Test',
		Layout: Atlas_Core.Atlas_Default
	) {
		datagrid dg (
			DataSource: database from ` + entityName + ` where [IsActive = true and Price > 10] or [Stock < 5] sort by Name asc, Price desc
		) {
			column colName (Attribute: Name, Caption: 'Product')
			column colCode (Attribute: Code, Caption: 'SKU')
			column colPrice (Attribute: Price, Caption: 'Price')
			column colStock (Attribute: Stock, Caption: 'Available')
			column colActive (Attribute: IsActive, Caption: 'Active')
		}
	}`

	if err := env.executeMDL(createPageMDL); err != nil {
		t.Fatalf("Failed to create page with complex filter: %v", err)
	}

	// Describe the page
	output, err := env.describeMDL(`describe page ` + pageName + `;`)
	if err != nil {
		t.Fatalf("Failed to describe page: %v", err)
	}

	// Verify all columns are present
	expectedColumns := []string{"Name", "Code", "Price", "Stock", "IsActive"}
	for _, col := range expectedColumns {
		if !strings.Contains(output, col) {
			t.Errorf("Expected column '%s' not found in output", col)
		}
	}

	t.Logf("V3 DataGrid with complex filter roundtrip successful:\n%s", output)
}

// TestRoundtripPage_MicroflowButtonWithParams tests that action buttons with microflow calls
// and parameters are correctly round-tripped.
func TestRoundtripPage_MicroflowButtonWithParams(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()
	env.requireMinVersion(t, 11, 0) // CREATE PAGE with parameters requires 11.0+

	// First create an entity to use as the page parameter and microflow parameter
	entityName := testModule + ".MfButtonEntity"
	env.registerCleanup("entity", entityName)

	createEntityMDL := `create or modify persistent entity ` + entityName + ` (
		Name: String(100),
		Status: String(50)
	);`

	if err := env.executeMDL(createEntityMDL); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	// Create a microflow that accepts the entity as parameter
	mfName := testModule + ".MfButtonProcess"
	env.registerCleanup("microflow", mfName)

	createMfMDL := `create microflow ` + mfName + ` (
		$Product: ` + entityName + `
	)
	returns Boolean
	begin
		log info node 'Test' 'Processing item';
		return true;
	end;`

	if err := env.executeMDL(createMfMDL); err != nil {
		t.Fatalf("Failed to create microflow: %v", err)
	}

	// Create page with action button that calls microflow with parameter
	pageName := testModule + ".MfButtonPage"
	env.registerCleanup("page", pageName)

	createPageMDL := `create page ` + pageName + ` (
		Params: { $Product: ` + entityName + ` },
		Title: 'Microflow Button Test',
		Layout: Atlas_Core.Atlas_Default
	) {
		dataview dv (DataSource: $Product) {
			textbox txt (Label: 'Name', Attribute: Name)
			actionbutton btnProcess (Caption: 'Process', Action: microflow ` + mfName + `(Product: $Product))
		}
	}`

	if err := env.executeMDL(createPageMDL); err != nil {
		t.Fatalf("Failed to create page with microflow button: %v", err)
	}

	// Describe the page
	output, err := env.describeMDL(`describe page ` + pageName + `;`)
	if err != nil {
		t.Fatalf("Failed to describe page: %v", err)
	}

	// Verify that the microflow call with parameter is in the output, in the
	// parser-accepted form (issue #634: no "call_" prefix on widget actions).
	if !strings.Contains(output, "microflow") || strings.Contains(output, "call_microflow") {
		t.Errorf("Expected re-parseable 'microflow' (not 'call_microflow') in describe output.\nOutput:\n%s", output)
	}
	if !strings.Contains(output, mfName) {
		t.Errorf("Expected microflow name '%s' in describe output.\nOutput:\n%s", mfName, output)
	}
	if !strings.Contains(output, "Product: $Product") {
		t.Errorf("Expected 'Product: $Product' parameter mapping in describe output.\nOutput:\n%s", output)
	}

	t.Logf("Microflow button with params roundtrip successful:\n%s", output)
}

// TestRoundtripPage_MicroflowButtonWithCurrentObject tests that action buttons in list widgets
// with $currentObject parameter are correctly round-tripped.
func TestRoundtripPage_MicroflowButtonWithCurrentObject(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	// Create an entity for the DataGrid
	entityName := testModule + ".MfCurrentObjEntity"
	env.registerCleanup("entity", entityName)

	createEntityMDL := `create or modify persistent entity ` + entityName + ` (
		Name: String(100)
	);`

	if err := env.executeMDL(createEntityMDL); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	// Create a microflow that accepts the entity as parameter
	mfName := testModule + ".MfCurrentObjProcess"
	env.registerCleanup("microflow", mfName)

	createMfMDL := `create microflow ` + mfName + ` (
		$Target: ` + entityName + `
	)
	begin
		log info node 'Test' 'Processing: ' + $Target/Name;
	end;`

	if err := env.executeMDL(createMfMDL); err != nil {
		t.Fatalf("Failed to create microflow: %v", err)
	}

	// Create page with DataGrid containing button that passes $currentObject
	pageName := testModule + ".MfCurrentObjPage"
	env.registerCleanup("page", pageName)

	createPageMDL := `create page ` + pageName + ` (
		Title: 'CurrentObject Button Test',
		Layout: Atlas_Core.Atlas_Default
	) {
		datagrid dg (DataSource: database ` + entityName + `) {
			column colName (Attribute: Name, Caption: 'Name')
			column colActions (Attribute: Name, Caption: 'Actions', ShowContentAs: customContent) {
				actionbutton btnProcess (Caption: 'Process', Action: microflow ` + mfName + `(Target: $currentObject))
			}
		}
	}`

	if err := env.executeMDL(createPageMDL); err != nil {
		t.Fatalf("Failed to create page with currentObject microflow button: %v", err)
	}

	// Describe the page
	output, err := env.describeMDL(`describe page ` + pageName + `;`)
	if err != nil {
		t.Fatalf("Failed to describe page: %v", err)
	}

	// Verify that the microflow call with $currentObject parameter is in the output,
	// in the parser-accepted form (issue #634: no "call_" prefix on widget actions).
	if !strings.Contains(output, "microflow") || strings.Contains(output, "call_microflow") {
		t.Errorf("Expected re-parseable 'microflow' (not 'call_microflow') in describe output.\nOutput:\n%s", output)
	}
	if !strings.Contains(output, mfName) {
		t.Errorf("Expected microflow name '%s' in describe output.\nOutput:\n%s", mfName, output)
	}
	// Quoting-agnostic on purpose. DESCRIBE emits an identifier through mdlIdent,
	// which quotes anything that does not LEX as a bare identifier — so the day
	// `TARGET` became a lexer token (the notify-workflow target clause, #476) this
	// output went from `Target:` to `"Target":` and an exact-substring assertion
	// started failing on a describe that had become MORE correct, not less.
	//
	// Both forms re-parse here (TARGET is non-reserved, so the unquoted spelling
	// this test's own input uses is still accepted); what the assertion is for is
	// the MAPPING surviving the roundtrip, which is independent of the quoting.
	if !strings.Contains(output, "Target: $currentObject") &&
		!strings.Contains(output, `"Target": $currentObject`) {
		t.Errorf("Expected a 'Target: $currentObject' parameter mapping (quoted or not) "+
			"in describe output.\nOutput:\n%s", output)
	}

	t.Logf("Microflow button with $currentObject roundtrip successful:\n%s", output)
}

// TestRoundtripPage_DataViewAttributeShortNames tests that DESCRIBE outputs short attribute names
// (not fully qualified Module.Entity.Attribute) for widgets inside a DATAVIEW.
func TestRoundtripPage_DataViewAttributeShortNames(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()
	env.requireMinVersion(t, 11, 0) // CREATE PAGE with parameters requires 11.0+

	entityName := testModule + ".AttrShortNameEntity"
	env.registerCleanup("entity", entityName)

	createEntityMDL := `create or modify persistent entity ` + entityName + ` (
		FirstName: String(100),
		Email: String(200),
		IsActive: Boolean default false,
		BirthDate: DateTime
	);`

	if err := env.executeMDL(createEntityMDL); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	pageName := testModule + ".AttrShortNamePage"
	env.registerCleanup("page", pageName)

	createPageMDL := `create page ` + pageName + ` (
		Params: { $Item: ` + entityName + ` },
		Title: 'Short Attribute Names Test',
		Layout: Atlas_Core.PopupLayout
	) {
		dataview dv (DataSource: $Item) {
			textbox txtFirst (Label: 'First Name', Attribute: FirstName)
			textbox txtEmail (Label: 'Email', Attribute: Email)
			checkbox cbActive (Label: 'Active', Attribute: IsActive)
			datepicker dpBirth (Label: 'Birthday', Attribute: BirthDate)
		}
	}`

	if err := env.executeMDL(createPageMDL); err != nil {
		t.Fatalf("Failed to create page: %v", err)
	}

	output, err := env.describeMDL(`describe page ` + pageName + `;`)
	if err != nil {
		t.Fatalf("Failed to describe page: %v", err)
	}

	// Verify attributes are short names, not qualified
	shortNames := []string{"Attribute: FirstName", "Attribute: Email", "Attribute: IsActive", "Attribute: BirthDate"}
	for _, expected := range shortNames {
		if !strings.Contains(output, expected) {
			t.Errorf("Expected short attribute '%s' not found in output.\nOutput:\n%s", expected, output)
		}
	}

	// Verify qualified names are NOT present
	qualifiedPrefix := testModule + ".AttrShortNameEntity."
	if strings.Contains(output, qualifiedPrefix) {
		t.Errorf("Found qualified attribute prefix '%s' in output — should use short names.\nOutput:\n%s", qualifiedPrefix, output)
	}

	t.Logf("DataView short attribute names roundtrip successful:\n%s", output)
}

// TestRoundtripPage_DataGridAttributeShortNames tests that DataGrid column attributes
// use short names in DESCRIBE output.
func TestRoundtripPage_DataGridAttributeShortNames(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	entityName := testModule + ".GridAttrEntity"
	env.registerCleanup("entity", entityName)

	createEntityMDL := `create or modify persistent entity ` + entityName + ` (
		Name: String(100),
		Code: String(50),
		Price: Decimal
	);`

	if err := env.executeMDL(createEntityMDL); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	pageName := testModule + ".GridAttrPage"
	env.registerCleanup("page", pageName)

	createPageMDL := `create page ` + pageName + ` (
		Title: 'Grid Attribute Names Test',
		Layout: Atlas_Core.Atlas_Default
	) {
		datagrid dg (DataSource: database ` + entityName + `) {
			column colName (Attribute: Name, Caption: 'Name')
			column colCode (Attribute: Code, Caption: 'Code')
			column colPrice (Attribute: Price, Caption: 'Price')
		}
	}`

	if err := env.executeMDL(createPageMDL); err != nil {
		t.Fatalf("Failed to create page: %v", err)
	}

	output, err := env.describeMDL(`describe page ` + pageName + `;`)
	if err != nil {
		t.Fatalf("Failed to describe page: %v", err)
	}

	// Verify column attributes are short names
	shortNames := []string{"Attribute: Name", "Attribute: Code", "Attribute: Price"}
	for _, expected := range shortNames {
		if !strings.Contains(output, expected) {
			t.Errorf("Expected short attribute '%s' not found in output.\nOutput:\n%s", expected, output)
		}
	}

	// Verify qualified names are NOT present
	qualifiedPrefix := testModule + ".GridAttrEntity."
	if strings.Contains(output, qualifiedPrefix) {
		t.Errorf("Found qualified attribute prefix '%s' in output — should use short names.\nOutput:\n%s", qualifiedPrefix, output)
	}

	t.Logf("DataGrid short attribute names roundtrip successful:\n%s", output)
}
