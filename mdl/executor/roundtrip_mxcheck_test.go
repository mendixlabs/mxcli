// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
)

// --- MX Check Integration Tests ---
// These tests verify that created documents pass Mendix's validation.

// mxCheckAvailable checks if the mx command is available.
func mxCheckAvailable() bool {
	return findMxBinary() != ""
}

// runMxCheck runs mx check on the given project and returns any errors.
func runMxCheck(t *testing.T, projectPath string) (string, error) {
	t.Helper()

	mxPath := findMxBinary()
	if mxPath == "" {
		return "", fmt.Errorf("mx binary not found")
	}

	cmd := exec.Command(mxPath, "check", projectPath)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// runMxUpdateWidgets synchronizes widget definitions with mpk files.
// Call before runMxCheck on tests that create pluggable widgets.
func runMxUpdateWidgets(t *testing.T, projectPath string) {
	t.Helper()
	mxPath := findMxBinary()
	if mxPath == "" {
		return
	}
	exec.Command(mxPath, "update-widgets", projectPath).CombinedOutput()
}

// TestMxCheck_Entity creates an entity and verifies mx check passes.
func TestMxCheck_Entity(t *testing.T) {
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	env := setupTestEnv(t)
	defer env.teardown()

	entityName := testModule + ".MxCheckEntity"
	env.registerCleanup("entity", entityName)

	// Create entity
	createMDL := `create or modify persistent entity ` + entityName + ` (
		Code: String(50) not null,
		Description: String(500),
		Count: Integer default 0
	);`

	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	// Disconnect to flush changes
	env.executor.Execute(&ast.DisconnectStmt{})

	// Run mx check
	output, err := runMxCheck(t, env.projectPath)
	if err != nil {
		// mx check returns non-zero exit code if there are errors
		if strings.Contains(output, "error") || strings.Contains(output, "Error") {
			t.Errorf("mx check found errors:\n%s", output)
		} else {
			t.Logf("mx check output:\n%s", output)
		}
	} else {
		t.Logf("mx check passed:\n%s", output)
	}
}

// TestMxCheck_Enumeration creates an enumeration and verifies mx check passes.
func TestMxCheck_Enumeration(t *testing.T) {
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	env := setupTestEnv(t)
	defer env.teardown()

	enumName := testModule + ".MxCheckPriority"
	env.registerCleanup("enumeration", enumName)

	// Create enumeration
	createMDL := `create enumeration ` + enumName + ` (
		Low 'Low Priority',
		Medium 'Medium Priority',
		High 'High Priority',
		Critical 'Critical'
	);`

	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("Failed to create enumeration: %v", err)
	}

	// Disconnect to flush changes
	env.executor.Execute(&ast.DisconnectStmt{})

	// Run mx check
	output, err := runMxCheck(t, env.projectPath)
	if err != nil {
		if strings.Contains(output, "error") || strings.Contains(output, "Error") {
			t.Errorf("mx check found errors:\n%s", output)
		} else {
			t.Logf("mx check output:\n%s", output)
		}
	} else {
		t.Logf("mx check passed:\n%s", output)
	}
}

// TestMxCheck_RetrieveWithLimit validates that RETRIEVE with LIMIT produces
// BSON that passes mx check (Studio Pro validation).
func TestMxCheck_RetrieveWithLimit(t *testing.T) {
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`create or modify persistent entity RoundtripTest.MxCheckItem (Name: String(100));`); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	mfName := testModule + ".MxCheck_RetrieveLimit"
	env.registerCleanup("microflow", mfName)

	createMDL := `create microflow ` + mfName + ` () returns Boolean
begin
  retrieve $Item from RoundtripTest.MxCheckItem
    limit 1;
  return true;
end;`

	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("Failed to create microflow: %v", err)
	}

	env.executor.Execute(&ast.DisconnectStmt{})

	output, err := runMxCheck(t, env.projectPath)
	if err != nil {
		if strings.Contains(output, "error") || strings.Contains(output, "Error") {
			t.Errorf("mx check found errors:\n%s", output)
		} else {
			t.Logf("mx check output:\n%s", output)
		}
	} else {
		t.Logf("mx check passed:\n%s", output)
	}
}

// TestMxCheck_RetrieveWithLimitOffset validates that RETRIEVE with LIMIT and OFFSET
// produces BSON that passes mx check. Regression guard for LimitExpression/OffsetExpression
// being stored in the correct BSON fields within Microflows$ConstantRange.
func TestMxCheck_RetrieveWithLimitOffset(t *testing.T) {
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`create or modify persistent entity RoundtripTest.MxCheckItem (Name: String(100));`); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	mfName := testModule + ".MxCheck_RetrieveLimOff"
	env.registerCleanup("microflow", mfName)

	createMDL := `create microflow ` + mfName + ` () returns Boolean
begin
  retrieve $Items from RoundtripTest.MxCheckItem
    limit 2
    offset 3;
  return true;
end;`

	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("Failed to create microflow: %v", err)
	}

	env.executor.Execute(&ast.DisconnectStmt{})

	output, err := runMxCheck(t, env.projectPath)
	if err != nil {
		if strings.Contains(output, "error") || strings.Contains(output, "Error") {
			t.Errorf("mx check found errors:\n%s", output)
		} else {
			t.Logf("mx check output:\n%s", output)
		}
	} else {
		t.Logf("mx check passed:\n%s", output)
	}
}

// TestMxCheck_RetrieveWithSortBy validates that RETRIEVE with SORT BY produces
// BSON that passes mx check. Regression guard for sort items being stored under
// the correct BSON key (NewSortings vs sortItemList).
func TestMxCheck_RetrieveWithSortBy(t *testing.T) {
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`create or modify persistent entity RoundtripTest.MxCheckItem (Name: String(100));`); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	mfName := testModule + ".MxCheck_RetrieveSort"
	env.registerCleanup("microflow", mfName)

	createMDL := `create microflow ` + mfName + ` () returns Boolean
begin
  retrieve $Items from RoundtripTest.MxCheckItem
    sort by RoundtripTest.MxCheckItem.Name asc;
  return true;
end;`

	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("Failed to create microflow: %v", err)
	}

	env.executor.Execute(&ast.DisconnectStmt{})

	output, err := runMxCheck(t, env.projectPath)
	if err != nil {
		if strings.Contains(output, "error") || strings.Contains(output, "Error") {
			t.Errorf("mx check found errors:\n%s", output)
		} else {
			t.Logf("mx check output:\n%s", output)
		}
	} else {
		t.Logf("mx check passed:\n%s", output)
	}
}

// TestMxCheck_RetrieveWithWhereSortLimitOffset validates the full RETRIEVE pattern
// (WHERE + SORT BY + LIMIT + OFFSET) passes mx check. This matches the
// M028_DataForm_Getter microflow pattern.
func TestMxCheck_RetrieveWithWhereSortLimitOffset(t *testing.T) {
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`create or modify persistent entity RoundtripTest.MxCheckItem (Name: String(100));`); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	mfName := testModule + ".MxCheck_RetrieveFull"
	env.registerCleanup("microflow", mfName)

	createMDL := `create microflow ` + mfName + ` () returns Boolean
begin
  retrieve $Items from RoundtripTest.MxCheckItem
    where (starts-with(Name, 'a'))
    sort by RoundtripTest.MxCheckItem.Name asc
    limit 2
    offset 3;
  return true;
end;`

	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("Failed to create microflow: %v", err)
	}

	env.executor.Execute(&ast.DisconnectStmt{})

	output, err := runMxCheck(t, env.projectPath)
	if err != nil {
		if strings.Contains(output, "error") || strings.Contains(output, "Error") {
			t.Errorf("mx check found errors:\n%s", output)
		} else {
			t.Logf("mx check output:\n%s", output)
		}
	} else {
		t.Logf("mx check passed:\n%s", output)
	}
}

// --- CE0066 "Entity access is out of date" scenario tests ---
//
// These tests enumerate mutation orderings that might trigger CE0066 when
// entity structure changes after access rules have been written.
//
// Scenarios to test:
//   S1: CREATE ENTITY (with attrs) → GRANT                      (baseline)
//   S2: CREATE ENTITY → GRANT → ALTER ADD ATTRIBUTE             (attribute added after security)
//   S3: CREATE ENTITY → GRANT → CREATE ASSOCIATION              (association added after security)
//   S4: CREATE ENTITY → GRANT → ALTER DROP ATTRIBUTE            (attribute removed after security)
//   S5: CREATE ENTITY → ALTER ADD ATTRIBUTE → GRANT             (security after mutation — should always work)
//   S6: CREATE ENTITY → GRANT(READ *) → ALTER → GRANT(READ *)  (re-grant after mutation)
//   S7: CREATE ENTITY + GRANT in single script                  (typical user script pattern)
//   S8: CREATE ENTITY → GRANT multiple roles → ALTER            (multiple rules + mutation)
//   S9: CREATE ENTITY → GRANT → ALTER + CREATE ASSOC (combined) (both attrs and assocs change)

// ce0066Scenario describes a single CE0066 test case.
type ce0066Scenario struct {
	name  string   // subtest name
	steps []string // MDL statements to execute in order
}

func TestMxCheck_CE0066_Scenarios(t *testing.T) {
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	mod := testModule
	scenarios := []ce0066Scenario{
		{
			name: "S1_CreateEntity_Grant",
			steps: []string{
				`create module role ` + mod + `.S1Admin;`,
				`create or modify persistent entity ` + mod + `.S1Entity (Name: String(100), Email: String(200));`,
				`grant ` + mod + `.S1Admin on ` + mod + `.S1Entity (create, delete, read *, write *);`,
			},
		},
		{
			name: "S2_Grant_ThenAddAttribute",
			steps: []string{
				`create module role ` + mod + `.S2Admin;`,
				`create or modify persistent entity ` + mod + `.S2Entity (Name: String(100));`,
				`grant ` + mod + `.S2Admin on ` + mod + `.S2Entity (create, delete, read *, write *);`,
				`alter entity ` + mod + `.S2Entity add attribute Email: String(200);`,
				`alter entity ` + mod + `.S2Entity add attribute Active: Boolean default false;`,
			},
		},
		{
			name: "S3_Grant_ThenAddAssociation",
			steps: []string{
				`create module role ` + mod + `.S3Admin;`,
				`create or modify persistent entity ` + mod + `.S3Parent (Name: String(100));`,
				`create or modify persistent entity ` + mod + `.S3Child (Label: String(100));`,
				`grant ` + mod + `.S3Admin on ` + mod + `.S3Parent (create, delete, read *, write *);`,
				`grant ` + mod + `.S3Admin on ` + mod + `.S3Child (create, delete, read *, write *);`,
				`create association ` + mod + `.S3Child_S3Parent from ` + mod + `.S3Child to ` + mod + `.S3Parent;`,
			},
		},
		{
			name: "S4_Grant_ThenDropAttribute",
			steps: []string{
				`create module role ` + mod + `.S4Admin;`,
				`create or modify persistent entity ` + mod + `.S4Entity (Name: String(100), Temp: String(50));`,
				`grant ` + mod + `.S4Admin on ` + mod + `.S4Entity (create, delete, read *, write *);`,
				`alter entity ` + mod + `.S4Entity drop attribute Temp;`,
			},
		},
		{
			name: "S5_AddAttribute_ThenGrant",
			steps: []string{
				`create module role ` + mod + `.S5Admin;`,
				`create or modify persistent entity ` + mod + `.S5Entity (Name: String(100));`,
				`alter entity ` + mod + `.S5Entity add attribute Email: String(200);`,
				`grant ` + mod + `.S5Admin on ` + mod + `.S5Entity (create, delete, read *, write *);`,
			},
		},
		{
			name: "S6_Grant_Alter_Regrant",
			steps: []string{
				`create module role ` + mod + `.S6Admin;`,
				`create or modify persistent entity ` + mod + `.S6Entity (Name: String(100));`,
				`grant ` + mod + `.S6Admin on ` + mod + `.S6Entity (read *);`,
				`alter entity ` + mod + `.S6Entity add attribute Code: String(50);`,
				`grant ` + mod + `.S6Admin on ` + mod + `.S6Entity (create, delete, read *, write *);`,
			},
		},
		{
			name: "S7_SingleScript_CreateAndGrant",
			steps: []string{
				`create module role ` + mod + `.S7Admin;`,
				`create or modify persistent entity ` + mod + `.S7Entity (
					Code: String(50) not null,
					Description: String(500),
					Count: Integer default 0
				);` + "\n" +
					`grant ` + mod + `.S7Admin on ` + mod + `.S7Entity (create, delete, read *, write *);`,
			},
		},
		{
			name: "S8_MultipleRoles_ThenAlter",
			steps: []string{
				`create module role ` + mod + `.S8Admin;`,
				`create module role ` + mod + `.S8Viewer;`,
				`create or modify persistent entity ` + mod + `.S8Entity (Name: String(100));`,
				`grant ` + mod + `.S8Admin on ` + mod + `.S8Entity (create, delete, read *, write *);`,
				`grant ` + mod + `.S8Viewer on ` + mod + `.S8Entity (read *);`,
				`alter entity ` + mod + `.S8Entity add attribute Status: String(50);`,
			},
		},
		{
			name: "S9_Grant_ThenAlterAndAssoc",
			steps: []string{
				`create module role ` + mod + `.S9Admin;`,
				`create or modify persistent entity ` + mod + `.S9Main (Name: String(100));`,
				`create or modify persistent entity ` + mod + `.S9Related (Value: Integer);`,
				`grant ` + mod + `.S9Admin on ` + mod + `.S9Main (create, delete, read *, write *);`,
				`grant ` + mod + `.S9Admin on ` + mod + `.S9Related (create, delete, read *, write *);`,
				`alter entity ` + mod + `.S9Main add attribute Extra: String(200);`,
				`create association ` + mod + `.S9Related_S9Main from ` + mod + `.S9Related to ` + mod + `.S9Main;`,
			},
		},
		{
			name: "S10_DropIndexedAttribute",
			steps: []string{
				`create module role ` + mod + `.S10Admin;`,
				`create or modify persistent entity ` + mod + `.S10Entity (Name: String(100), Code: String(50));`,
				`alter entity ` + mod + `.S10Entity add index (Code);`,
				`grant ` + mod + `.S10Admin on ` + mod + `.S10Entity (create, delete, read *, write *);`,
				`alter entity ` + mod + `.S10Entity drop attribute Code;`,
			},
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			env := setupTestEnv(t)
			defer env.teardown()

			// Combine all steps into a single script so ExecuteProgram
			// runs finalizeProgramExecution (ReconcileMemberAccesses).
			allMDL := strings.Join(sc.steps, "\n")
			prog, errs := visitor.Build(allMDL)
			if len(errs) > 0 {
				t.Fatalf("Parse failed: %v\nMDL:\n%s", errs[0], allMDL)
			}
			if err := env.executor.ExecuteProgram(prog); err != nil {
				if !strings.Contains(err.Error(), "already exists") {
					t.Fatalf("ExecuteProgram failed: %v\nMDL:\n%s", err, allMDL)
				}
			}

			env.executor.Execute(&ast.DisconnectStmt{})

			output, err := runMxCheck(t, env.projectPath)
			if err != nil {
				if strings.Contains(output, "CE0066") || strings.Contains(output, "out of date") {
					t.Errorf("CE0066 entity access out of date:\n%s", output)
				} else if strings.Contains(output, "error") || strings.Contains(output, "Error") {
					t.Errorf("mx check found errors:\n%s", output)
				} else {
					t.Logf("mx check output (non-zero exit but no errors):\n%s", output)
				}
			} else {
				t.Logf("mx check passed")
			}
		})
	}
}

// TestMxCheck_DropAddEnumAttribute validates that dropping and re-adding an attribute
// with an enumeration default value does not corrupt the MPR (GitHub issue #4).
// Before the fix, the StoredValue $ID was lost during parsing and a new GUID was
// generated on write, leaving dangling references that caused KeyNotFoundException.
func TestMxCheck_DropAddEnumAttribute(t *testing.T) {
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	env := setupTestEnv(t)
	defer env.teardown()

	mod := testModule

	// Step 1: Create an enumeration and an entity with an enumeration attribute
	setupMDL := strings.Join([]string{
		`create enumeration ` + mod + `.SubmissionStatus (StatusNew 'New', StatusInProgress 'In Progress', StatusDone 'Done');`,
		`create or modify persistent entity ` + mod + `.Issue4Entity (
			Name: String(100),
			Status: Enumeration(` + mod + `.SubmissionStatus) default ` + mod + `.SubmissionStatus.StatusNew
		);`,
	}, "\n")

	prog, errs := visitor.Build(setupMDL)
	if len(errs) > 0 {
		t.Fatalf("Parse failed: %v\nMDL:\n%s", errs[0], setupMDL)
	}
	if err := env.executor.ExecuteProgram(prog); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Step 2: Drop the attribute and re-add it with a different default
	alterMDL := strings.Join([]string{
		`alter entity ` + mod + `.Issue4Entity drop attribute Status;`,
		`alter entity ` + mod + `.Issue4Entity add attribute Status: Enumeration(` + mod + `.SubmissionStatus) default ` + mod + `.SubmissionStatus.StatusInProgress;`,
	}, "\n")

	prog, errs = visitor.Build(alterMDL)
	if len(errs) > 0 {
		t.Fatalf("Parse failed: %v\nMDL:\n%s", errs[0], alterMDL)
	}
	if err := env.executor.ExecuteProgram(prog); err != nil {
		t.Fatalf("Alter failed: %v", err)
	}

	// Step 3: Flush to disk and validate with mx check
	env.executor.Execute(&ast.DisconnectStmt{})

	output, err := runMxCheck(t, env.projectPath)
	if err != nil {
		if strings.Contains(output, "KeyNotFoundException") || strings.Contains(output, "not present in the dictionary") {
			t.Errorf("Issue #4 regression: dangling GUID reference after drop/add enum attribute:\n%s", output)
		} else if strings.Contains(output, "error") || strings.Contains(output, "Error") {
			t.Errorf("mx check found errors:\n%s", output)
		} else {
			t.Logf("mx check output (non-zero exit but no errors):\n%s", output)
		}
	} else {
		t.Logf("mx check passed")
	}
}

// TestMxCheck_RetrieveWithDateTimeToken validates that RETRIEVE with [%CurrentDateTime%]
// in a WHERE clause produces correctly quoted XPath (GitHub issue #1).
// The token must be quoted as '[%CurrentDateTime%]' in XPath constraints.
func TestMxCheck_RetrieveWithDateTimeToken(t *testing.T) {
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`create or modify persistent entity RoundtripTest.MxCheckDated (
		Name: String(100),
		DueDate: DateTime
	);`); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	mfName := testModule + ".MxCheck_DateTimeToken"
	env.registerCleanup("microflow", mfName)

	createMDL := `create microflow ` + mfName + ` () returns Boolean
begin
  retrieve $Items from RoundtripTest.MxCheckDated
    where DueDate < [%CurrentDateTime%];
  return true;
end;`

	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("Failed to create microflow: %v", err)
	}

	env.executor.Execute(&ast.DisconnectStmt{})

	output, err := runMxCheck(t, env.projectPath)
	if err != nil {
		if strings.Contains(output, "error") || strings.Contains(output, "Error") {
			t.Errorf("mx check found errors:\n%s", output)
		} else {
			t.Logf("mx check output:\n%s", output)
		}
	} else {
		t.Logf("mx check passed:\n%s", output)
	}
}

// TestMxCheck_MicroflowWithCallParams tests microflow with CALL unified param syntax.
func TestMxCheck_MicroflowWithCallParams(t *testing.T) {
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	env := setupTestEnv(t)
	defer env.teardown()

	// Create a helper microflow
	helperName := testModule + ".MxCheckCallHelper"
	env.registerCleanup("microflow", helperName)

	createHelperMDL := `create microflow ` + helperName + ` ($InputValue: String) returns String
	begin
		return $InputValue;
	end;`

	if err := env.executeMDL(createHelperMDL); err != nil {
		t.Fatalf("Failed to create helper microflow: %v", err)
	}

	// Create caller microflow with unified param syntax
	callerName := testModule + ".MxCheckCallCaller"
	env.registerCleanup("microflow", callerName)

	createCallerMDL := `create microflow ` + callerName + ` () returns String
	begin
		$Result = call microflow ` + helperName + ` (InputValue = 'TestValue');
		return $Result;
	end;`

	if err := env.executeMDL(createCallerMDL); err != nil {
		t.Fatalf("Failed to create caller microflow: %v", err)
	}

	// Disconnect to flush changes
	env.executor.Execute(&ast.DisconnectStmt{})

	// Run mx check
	output, err := runMxCheck(t, env.projectPath)
	if err != nil {
		if strings.Contains(output, "error") || strings.Contains(output, "Error") {
			t.Errorf("mx check found errors:\n%s", output)
		} else {
			t.Logf("mx check output:\n%s", output)
		}
	} else {
		t.Logf("mx check passed:\n%s", output)
	}
}

// TestMxCheck_ViewEntitySimple creates a simple VIEW entity (no aggregates)
// and verifies mx check passes.
func TestMxCheck_ViewEntitySimple(t *testing.T) {
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	env := setupTestEnv(t)
	defer env.teardown()
	env.requireMinVersion(t, 10, 18) // VIEW ENTITY requires 10.18+

	mod := testModule

	entityName := mod + ".MxCheckProduct"
	env.registerCleanup("entity", entityName)

	if err := env.executeMDL(`create or modify persistent entity ` + entityName + ` (
		Name: String(100),
		Price: Decimal
	);`); err != nil {
		t.Fatalf("Failed to create source entity: %v", err)
	}

	viewName := mod + ".MxCheckProductView"
	env.registerCleanup("entity", viewName)

	viewMDL := `create view entity ` + viewName + ` (
		Name: String(100),
		Price: Decimal
	) as (
		select p.Name as Name, p.Price as Price
		from ` + entityName + ` as p
	);`

	if err := env.executeMDL(viewMDL); err != nil {
		t.Fatalf("Failed to create view entity: %v", err)
	}

	env.executor.Execute(&ast.DisconnectStmt{})

	output, err := runMxCheck(t, env.projectPath)
	if err != nil {
		if strings.Contains(output, "out of sync") {
			t.Errorf("mx check reports view entity out of sync with OQL:\n%s", output)
		} else if strings.Contains(output, "error") || strings.Contains(output, "Error") {
			t.Errorf("mx check found errors:\n%s", output)
		} else {
			t.Logf("mx check output:\n%s", output)
		}
	} else {
		t.Logf("mx check passed:\n%s", output)
	}
}

// TestMxCheck_ViewEntityWithAggregates creates a VIEW entity with aggregate OQL
// (COUNT, SUM, AVG, GROUP BY) and verifies mx check passes.
// Regression test for GitHub issue: COUNT must return Long, not Integer.
func TestMxCheck_ViewEntityWithAggregates(t *testing.T) {
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	env := setupTestEnv(t)
	defer env.teardown()
	env.requireMinVersion(t, 10, 18) // VIEW ENTITY requires 10.18+

	mod := testModule

	// Create source entity with numeric fields for aggregation
	entityName := mod + ".MxCheckDeal"
	env.registerCleanup("entity", entityName)

	if err := env.executeMDL(`create or modify persistent entity ` + entityName + ` (
		Stage: String(50),
		Amount: Decimal
	);`); err != nil {
		t.Fatalf("Failed to create source entity: %v", err)
	}

	// Create VIEW entity with aggregate OQL
	// Note: COUNT returns Long in Mendix OQL, not Integer
	viewName := mod + ".MxCheckDealsByStage"
	env.registerCleanup("entity", viewName)

	viewMDL := `create view entity ` + viewName + ` (
		Stage: String(50),
		DealCount: Integer,
		TotalAmount: Decimal,
		AvgAmount: Decimal
	) as (
		select
			d.Stage as Stage,
			count(d.ID) as DealCount,
			sum(d.Amount) as TotalAmount,
			avg(d.Amount) as AvgAmount
		from ` + entityName + ` as d
		GROUP by d.Stage
	);`

	if err := env.executeMDL(viewMDL); err != nil {
		t.Fatalf("Failed to create view entity: %v", err)
	}

	// Disconnect to flush changes
	env.executor.Execute(&ast.DisconnectStmt{})

	// Run mx check
	output, err := runMxCheck(t, env.projectPath)
	if err != nil {
		if strings.Contains(output, "out of sync") {
			t.Errorf("mx check reports view entity out of sync with OQL:\n%s", output)
		} else if strings.Contains(output, "error") || strings.Contains(output, "Error") {
			t.Errorf("mx check found errors:\n%s", output)
		} else {
			t.Logf("mx check output:\n%s", output)
		}
	} else {
		t.Logf("mx check passed:\n%s", output)
	}
}

// TestMxCheck_ComboBoxWithAssociation creates a page with a COMBOBOX widget that
// uses an association attribute and verifies mx check passes.
// Regression test for: COMBOBOX Attribute should resolve as association path (2-part),
// not regular attribute path (3-part).
func TestMxCheck_ComboBoxWithAssociation(t *testing.T) {
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	env := setupTestEnv(t)
	defer env.teardown()
	env.requireMinVersion(t, 11, 0) // Widget template is 11.6, CE0463 on 10.x

	mod := testModule

	// Create target entity (for the association)
	companyEntity := mod + ".MxCheckCompany"
	env.registerCleanup("entity", companyEntity)

	if err := env.executeMDL(`create or modify persistent entity ` + companyEntity + ` (
		Name: String(100)
	);`); err != nil {
		t.Fatalf("Failed to create Company entity: %v", err)
	}

	// Create source entity (with association to Company)
	contactEntity := mod + ".MxCheckContact"
	env.registerCleanup("entity", contactEntity)

	if err := env.executeMDL(`create or modify persistent entity ` + contactEntity + ` (
		FullName: String(100),
		Email: String(200)
	);`); err != nil {
		t.Fatalf("Failed to create Contact entity: %v", err)
	}

	// Create association
	assocName := mod + ".MxCheckContact_MxCheckCompany"

	if err := env.executeMDL(`create association ` + assocName + ` from ` + contactEntity + ` to ` + companyEntity + `;`); err != nil {
		t.Fatalf("Failed to create association: %v", err)
	}

	// Create a microflow that returns a Contact (for dataview source)
	mfName := mod + ".MxCheckGetContact"
	env.registerCleanup("microflow", mfName)

	mfMDL := `create microflow ` + mfName + ` () returns ` + contactEntity + `
begin
  retrieve $Contact from ` + contactEntity + ` limit 1;
  return $Contact;
end;`

	if err := env.executeMDL(mfMDL); err != nil {
		t.Fatalf("Failed to create microflow: %v", err)
	}

	// Create page with COMBOBOX using association attribute
	pageName := mod + ".MxCheckContactEdit"
	env.registerCleanup("page", pageName)

	pageMDL := `create page ` + pageName + ` (
		Title: 'Contact Edit',
		Layout: Atlas_Core.Atlas_Default
	) {
		dataview dvContact (DataSource: microflow ` + mfName + `) {
			layoutgrid lgMain {
				row r1 {
					column c1 (DesktopWidth: 12) {
						textbox txtName (Attribute: FullName, Label: 'Full Name')
						combobox cmbCompany (
							Label: 'Company',
							Attribute: MxCheckContact_MxCheckCompany,
							DataSource: database ` + companyEntity + `,
							CaptionAttribute: Name
						)
					}
				}
			}
		}
	};`

	if err := env.executeMDL(pageMDL); err != nil {
		t.Fatalf("Failed to create page with ComboBox: %v", err)
	}

	// Disconnect to flush changes
	env.executor.Execute(&ast.DisconnectStmt{})

	runMxUpdateWidgets(t, env.projectPath)

	// Run mx check
	output, err := runMxCheck(t, env.projectPath)
	if err != nil {
		if strings.Contains(output, "no longer exists") {
			t.Errorf("mx check reports attribute no longer exists (association not resolved correctly):\n%s", output)
		} else if strings.Contains(output, "CE0642") {
			t.Errorf("mx check reports Entity property missing on ComboBox (association EntityRef not set):\n%s", output)
		} else if strings.Contains(output, "CE8812") {
			t.Errorf("mx check reports association path missing on ComboBox:\n%s", output)
		} else if strings.Contains(output, "error") || strings.Contains(output, "Error") {
			t.Errorf("mx check found errors:\n%s", output)
		} else {
			t.Logf("mx check output:\n%s", output)
		}
	} else {
		t.Logf("mx check passed:\n%s", output)
	}
}

// TestMxCheck_WhileLoop creates a microflow with a WHILE loop and verifies
// mx check passes. Regression test for WHILE loop support.
func TestMxCheck_WhileLoop(t *testing.T) {
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	env := setupTestEnv(t)
	defer env.teardown()

	mod := testModule

	mfName := mod + ".MxCheck_WhileLoop"
	env.registerCleanup("microflow", mfName)

	createMDL := `create microflow ` + mfName + ` ($N: Integer) returns Integer
begin
  declare $Counter Integer = 0;
  declare $Sum Integer = 0;
  while $Counter < $N
  begin
    set $Counter = $Counter + 1;
    set $Sum = $Sum + $Counter;
  end while;
  return $Sum;
end;`

	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("Failed to create microflow with while loop: %v", err)
	}

	env.executor.Execute(&ast.DisconnectStmt{})

	output, err := runMxCheck(t, env.projectPath)
	if err != nil {
		if strings.Contains(output, "error") || strings.Contains(output, "Error") {
			t.Errorf("mx check found errors:\n%s", output)
		} else {
			t.Logf("mx check output:\n%s", output)
		}
	} else {
		t.Logf("mx check passed:\n%s", output)
	}
}

// TestMxCheck_DropModuleCleansUserRoles verifies that dropping a module removes
// its module roles from user roles in ProjectSecurity (prevents CE1613).
func TestMxCheck_DropModuleCleansUserRoles(t *testing.T) {
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	env := setupTestEnv(t)
	defer env.teardown()

	mod := testModule

	// Create module role and user role referencing it
	setupMDL := strings.Join([]string{
		`create module role ` + mod + `.TestAdmin;`,
		`create user role DropTestUser (System.User, ` + mod + `.TestAdmin);`,
	}, "\n")

	prog, errs := visitor.Build(setupMDL)
	if len(errs) > 0 {
		t.Fatalf("Parse failed: %v", errs[0])
	}
	if err := env.executor.ExecuteProgram(prog); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Drop the module — should cascade-remove module roles from user roles
	dropMDL := `drop module ` + mod + `;`
	if err := env.executeMDL(dropMDL); err != nil {
		t.Fatalf("Drop module failed: %v", err)
	}

	// Flush and validate
	env.executor.Execute(&ast.DisconnectStmt{})

	output, err := runMxCheck(t, env.projectPath)
	if err != nil {
		if strings.Contains(output, "CE1613") {
			t.Errorf("CE1613: dangling module role reference after drop module:\n%s", output)
		} else if strings.Contains(output, "[error]") {
			t.Errorf("mx check found errors:\n%s", output)
		} else {
			t.Logf("mx check output:\n%s", output)
		}
	} else {
		t.Logf("mx check passed:\n%s", output)
	}
}
