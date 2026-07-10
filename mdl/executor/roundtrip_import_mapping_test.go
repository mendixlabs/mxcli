// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestRoundtripImportMapping_NoSchema(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`create entity ` + testModule + `.IMPet (
  PetId: Integer,
  Name: String(200)
);`); err != nil {
		t.Fatalf("create entity failed: %v", err)
	}

	createMDL := `create import mapping ` + testModule + `.ImportPetBasic {
  create ` + testModule + `.IMPet {
    PetId = id key,
    Name = name
  }
};`

	env.assertContains(createMDL, []string{
		"import mapping",
		"ImportPetBasic",
		"IMPet",
		"create",
	})
}

func TestRoundtripImportMapping_WithJsonStructureRef(t *testing.T) {
	t.Skip("TODO: fix describe output for ExposedName vs original json key")
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`create entity ` + testModule + `.IMOrder (
  OrderId: Integer,
  Total: Decimal
);`); err != nil {
		t.Fatalf("create entity failed: %v", err)
	}

	if err := env.executeMDL(`create json structure ` + testModule + `.OrderJS
snippet '{"orderId": 1, "total": 99.99}';`); err != nil {
		t.Fatalf("create json structure failed: %v", err)
	}

	createMDL := `create import mapping ` + testModule + `.ImportOrder
  with json structure ` + testModule + `.OrderJS
{
  create ` + testModule + `.IMOrder {
    OrderId = orderId key,
    Total = total
  }
};`

	env.assertContains(createMDL, []string{
		"import mapping",
		"ImportOrder",
		"with json structure",
		"IMOrder",
		"orderId",
		"total",
	})
}

func TestRoundtripImportMapping_ValueTypes(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`create entity ` + testModule + `.IMAllTypes (
  IntVal: Integer,
  DecVal: Decimal,
  BoolVal: Boolean default false,
  DateVal: DateTime
);`); err != nil {
		t.Fatalf("create entity failed: %v", err)
	}

	createMDL := `create import mapping ` + testModule + `.ImportAllTypes {
  create ` + testModule + `.IMAllTypes {
    IntVal = intVal key,
    DecVal = decVal,
    BoolVal = boolVal,
    DateVal = dateVal
  }
};`

	env.assertContains(createMDL, []string{
		"IntVal",
		"DecVal",
		"BoolVal",
		"DateVal",
	})
}

func TestRoundtripImportMapping_Drop(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`create entity ` + testModule + `.IMDropPet (
  PetId: Integer
);`); err != nil {
		t.Fatalf("create entity failed: %v", err)
	}

	if err := env.executeMDL(`create import mapping ` + testModule + `.ToDropIM {
  create ` + testModule + `.IMDropPet {
    PetId = id key
  }
};`); err != nil {
		t.Fatalf("create import mapping failed: %v", err)
	}

	if _, err := env.describeMDL(`describe import mapping ` + testModule + `.ToDropIM;`); err != nil {
		t.Fatalf("import mapping should exist before drop: %v", err)
	}

	if err := env.executeMDL(`drop import mapping ` + testModule + `.ToDropIM;`); err != nil {
		t.Fatalf("drop import mapping failed: %v", err)
	}

	if _, err := env.describeMDL(`describe import mapping ` + testModule + `.ToDropIM;`); err == nil {
		t.Error("import mapping should not exist after drop")
	}
}

func TestRoundtripImportMapping_ShowAppearsInList(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`create entity ` + testModule + `.IMListPet (
  PetId: Integer
);`); err != nil {
		t.Fatalf("create entity failed: %v", err)
	}

	if err := env.executeMDL(`create import mapping ` + testModule + `.ListableIM {
  create ` + testModule + `.IMListPet {
    PetId = id key
  }
};`); err != nil {
		t.Fatalf("create import mapping failed: %v", err)
	}

	env.output.Reset()
	if err := env.executeMDL(`show import mappings in ` + testModule + `;`); err != nil {
		t.Fatalf("show failed: %v", err)
	}

	if !strings.Contains(env.output.String(), "ListableIM") {
		t.Errorf("expected 'ListableIM' in show output:\n%s", env.output.String())
	}
}

// --- MX Check ---

func TestMxCheck_ImportMapping_Basic(t *testing.T) {
	t.Skip("TODO: fix mapping BSON alignment with json structure schema (ExposedName/MaxOccurs)")
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`create entity ` + testModule + `.MxCheckIMPet (
  PetId: Integer,
  Name: String(200)
);`); err != nil {
		t.Fatalf("create entity failed: %v", err)
	}

	if err := env.executeMDL(`create json structure ` + testModule + `.MxCheckIMJS
snippet '{"id": 1, "name": "Fido"}';`); err != nil {
		t.Fatalf("create json structure failed: %v", err)
	}

	if err := env.executeMDL(`create import mapping ` + testModule + `.MxCheckImportPet
  with json structure ` + testModule + `.MxCheckIMJS
{
  create ` + testModule + `.MxCheckIMPet {
    PetId = id,
    Name = name
  }
};`); err != nil {
		t.Fatalf("create import mapping failed: %v", err)
	}

	env.executor.Execute(&ast.DisconnectStmt{})

	output, err := runMxCheck(t, env.projectPath)
	assertMxCheckPassed(t, output, err)
}
