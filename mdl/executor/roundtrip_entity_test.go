// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
)

// --- Roundtrip Tests ---

func TestRoundtripEntity_Simple(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	entityName := testModule + ".TestEntitySimple"

	// Create entity (Boolean auto-defaults to false if no DEFAULT specified)
	createMDL := `create or modify persistent entity ` + entityName + ` (
		Name: String(100),
		Age: Integer,
		Active: Boolean default false
	);`

	// Use diff-based helper to verify roundtrip
	env.assertContains(createMDL, []string{
		"persistent entity",
		"Name:",
		"String(100)",
		"Age:",
		"Integer",
		"Active:",
		"Boolean",
	})
}

func TestRoundtripEntity_WithConstraints(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	entityName := testModule + ".TestEntityConstraints"

	// Create entity with constraints
	createMDL := `create or modify persistent entity ` + entityName + ` (
		Code: String(50) not null,
		Email: String(200) unique
	);`

	// Use diff-based helper - NOT NULL may be output as REQUIRED
	env.assertContains(createMDL, []string{
		"persistent entity",
		"Code:",
		"String(50)",
		"Email:",
		"String(200)",
		"unique",
	})
}

func TestRoundtripEntity_WithIndex(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	entityName := testModule + ".TestEntityIndex"

	// Create entity with index
	createMDL := `create or modify persistent entity ` + entityName + ` (
		Code: String(50),
		Name: String(100)
	)
	index (Code);`

	// Use diff-based helper to verify roundtrip
	env.assertContains(createMDL, []string{
		"persistent entity",
		"Code:",
		"String(50)",
		"Name:",
		"String(100)",
		"index",
	})
}

func TestRoundtripEntity_WithEventHandler(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	entityName := testModule + ".TestEntityEventHandler"
	mfName := testModule + ".ACT_ValidateTestEntity"

	// Create a microflow first (event handler references it)
	if err := env.executeMDL(`create or modify microflow ` + mfName + ` ()
begin
  log info 'validating';
end;`); err != nil {
		t.Fatalf("failed to create microflow: %v", err)
	}

	// Create entity with event handler
	createMDL := `create or modify persistent entity ` + entityName + ` (
		Name: String(100)
	)
	on before commit call ` + mfName + ` raise error;`

	// Verify roundtrip preserves the event handler
	env.assertContains(createMDL, []string{
		"persistent entity",
		"Name:",
		"String(100)",
		"on before commit call",
		mfName,
		"raise error",
	})
}

func TestRoundtripEntity_AlterAddDropEventHandler(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	entityName := testModule + ".TestAlterEventHandler"
	mfName := testModule + ".ACT_AlterEventTest"

	// Create microflow
	if err := env.executeMDL(`create or modify microflow ` + mfName + ` ()
begin
  log info 'test';
end;`); err != nil {
		t.Fatalf("failed to create microflow: %v", err)
	}

	// Create entity without handlers
	if err := env.executeMDL(`create or modify persistent entity ` + entityName + ` (
		Code: String(50)
	);`); err != nil {
		t.Fatalf("failed to create entity: %v", err)
	}

	// Add event handler via ALTER
	if err := env.executeMDL(`alter entity ` + entityName + `
		add event handler on after create call ` + mfName + `;`); err != nil {
		t.Fatalf("failed to add event handler: %v", err)
	}

	// Verify handler appears in DESCRIBE
	out, err := env.describeMDL(`describe entity ` + entityName + `;`)
	if err != nil {
		t.Fatalf("describe failed: %v", err)
	}
	if !strings.Contains(out, "on after create call") {
		t.Errorf("expected on after create call in describe output, got:\n%s", out)
	}
	if !strings.Contains(out, mfName) {
		t.Errorf("expected microflow name %q in describe output, got:\n%s", mfName, out)
	}

	// Drop the event handler
	if err := env.executeMDL(`alter entity ` + entityName + `
		drop event handler on after create;`); err != nil {
		t.Fatalf("failed to drop event handler: %v", err)
	}

	// Verify handler is gone
	out, err = env.describeMDL(`describe entity ` + entityName + `;`)
	if err != nil {
		t.Fatalf("describe after drop failed: %v", err)
	}
	if strings.Contains(out, "on after create call") {
		t.Errorf("event handler should be removed but still in describe output:\n%s", out)
	}
}

func TestRoundtripEnumeration(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	enumName := testModule + ".TestStatus"

	// Create enumeration
	createMDL := `create enumeration ` + enumName + ` (
		Active 'Active',
		Inactive 'Inactive',
		Pending 'Pending Review'
	);`

	// Use diff-based helper to verify roundtrip
	env.assertContains(createMDL, []string{
		"enumeration",
		"Active",
		"Inactive",
		"Pending",
	})
}

// --- Benchmark Tests ---

func BenchmarkRoundtripEntity(b *testing.B) {
	// Skip if source project doesn't exist
	srcDir, _ := filepath.Abs(sourceProject)
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		b.Skip("Source project not found")
	}

	// Copy source project for benchmark (read-only use, but keeps pattern consistent)
	destDir := b.TempDir()
	srcMPR := filepath.Join(srcDir, sourceProjectMPR)
	destMPR := filepath.Join(destDir, sourceProjectMPR)
	if err := copyFile(srcMPR, destMPR); err != nil {
		b.Fatalf("Failed to copy MPR: %v", err)
	}
	for _, dir := range []string{"mprcontents", "widgets", "themesource", "theme", "javascriptsource"} {
		srcSub := filepath.Join(srcDir, dir)
		if _, serr := os.Stat(srcSub); serr == nil {
			if err := copyDir(srcSub, filepath.Join(destDir, dir)); err != nil {
				b.Fatalf("Failed to copy %s: %v", dir, err)
			}
		}
	}

	output := &bytes.Buffer{}
	exec := New(output)

	// Connect once
	exec.Execute(&ast.ConnectStmt{
		Path: destMPR,
	})
	defer exec.Execute(&ast.DisconnectStmt{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		output.Reset()

		// Create and describe entity
		prog, _ := visitor.Build(`describe entity MyFirstModule.MyEntity;`)
		for _, stmt := range prog.Statements {
			exec.Execute(stmt)
		}
	}
}
