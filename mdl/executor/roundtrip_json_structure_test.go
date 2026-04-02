// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func TestRoundtripJsonStructure_Simple(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	createMDL := `CREATE JSON STRUCTURE ` + testModule + `.PetResponse
FROM '{"id": 1, "name": "Fido", "active": true}';`

	env.assertContains(createMDL, []string{
		"JSON STRUCTURE",
		"PetResponse",
		"FROM",
	})
}

func TestRoundtripJsonStructure_ElementTreeCommented(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`CREATE JSON STRUCTURE ` + testModule + `.OrderResponse
FROM '{"orderId": 1, "total": 99.99, "items": [{"sku": "ABC", "qty": 2}]}';`); err != nil {
		t.Fatalf("CREATE failed: %v", err)
	}

	out, err := env.describeMDL(`DESCRIBE JSON STRUCTURE ` + testModule + `.OrderResponse;`)
	if err != nil {
		t.Fatalf("DESCRIBE failed: %v", err)
	}

	// DESCRIBE emits the element tree as comments
	if !strings.Contains(out, "-- Element tree:") {
		t.Errorf("expected element tree comment in output:\n%s", out)
	}
	if !strings.Contains(out, "OrderId") {
		t.Errorf("expected 'OrderId' element in output:\n%s", out)
	}
	if !strings.Contains(out, "Items") {
		t.Errorf("expected 'Items' array element in output:\n%s", out)
	}
}

func TestRoundtripJsonStructure_Drop(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`CREATE JSON STRUCTURE ` + testModule + `.ToDropJS
FROM '{"id": 1}';`); err != nil {
		t.Fatalf("CREATE failed: %v", err)
	}

	// Verify it exists
	if _, err := env.describeMDL(`DESCRIBE JSON STRUCTURE ` + testModule + `.ToDropJS;`); err != nil {
		t.Fatalf("JSON structure should exist before DROP: %v", err)
	}

	// Drop it
	if err := env.executeMDL(`DROP JSON STRUCTURE ` + testModule + `.ToDropJS;`); err != nil {
		t.Fatalf("DROP failed: %v", err)
	}

	// Verify it's gone
	if _, err := env.describeMDL(`DESCRIBE JSON STRUCTURE ` + testModule + `.ToDropJS;`); err == nil {
		t.Error("JSON structure should not exist after DROP")
	}
}

func TestRoundtripJsonStructure_ShowAppearsInList(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`CREATE JSON STRUCTURE ` + testModule + `.ListableJS
FROM '{"value": "hello"}';`); err != nil {
		t.Fatalf("CREATE failed: %v", err)
	}

	env.output.Reset()
	if err := env.executeMDL(`SHOW JSON STRUCTURES IN ` + testModule + `;`); err != nil {
		t.Fatalf("SHOW failed: %v", err)
	}

	if !strings.Contains(env.output.String(), "ListableJS") {
		t.Errorf("expected 'ListableJS' in SHOW output:\n%s", env.output.String())
	}
}

// --- MX Check ---

func TestMxCheck_JsonStructure_Simple(t *testing.T) {
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`CREATE JSON STRUCTURE ` + testModule + `.MxCheckPetJS
FROM '{"id": 1, "name": "Fido"}';`); err != nil {
		t.Fatalf("CREATE failed: %v", err)
	}

	env.executor.Execute(&ast.DisconnectStmt{})

	output, err := runMxCheck(t, env.projectPath)
	assertMxCheckPassed(t, output, err)
}
