// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func TestRoundtripImportMapping_NoSchema(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	// First create the entity that will be mapped to
	if err := env.executeMDL(`CREATE ENTITY ` + testModule + `.IMPet {
  Id: Integer;
  Name: String(200);
};`); err != nil {
		t.Fatalf("CREATE ENTITY failed: %v", err)
	}

	createMDL := `CREATE IMPORT MAPPING ` + testModule + `.ImportPetBasic {
  "" -> ` + testModule + `.IMPet (Create) {
    id -> Id (Integer, KEY);
    name -> Name (String);
  };
};`

	env.assertContains(createMDL, []string{
		"IMPORT MAPPING",
		"ImportPetBasic",
		"IMPet",
		"(Create)",
	})
}

func TestRoundtripImportMapping_WithJsonStructureRef(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	// Create required entity and JSON structure first
	if err := env.executeMDL(`CREATE ENTITY ` + testModule + `.IMOrder {
  OrderId: Integer;
  Total: Decimal(10,2);
};`); err != nil {
		t.Fatalf("CREATE ENTITY failed: %v", err)
	}

	if err := env.executeMDL(`CREATE JSON STRUCTURE ` + testModule + `.OrderJS
FROM '{"orderId": 1, "total": 99.99}';`); err != nil {
		t.Fatalf("CREATE JSON STRUCTURE failed: %v", err)
	}

	createMDL := `CREATE IMPORT MAPPING ` + testModule + `.ImportOrder
  FROM JSON STRUCTURE ` + testModule + `.OrderJS
{
  "" -> ` + testModule + `.IMOrder (Create) {
    orderId -> OrderId (Integer, KEY);
    total -> Total (Decimal);
  };
};`

	env.assertContains(createMDL, []string{
		"IMPORT MAPPING",
		"ImportOrder",
		"FROM JSON STRUCTURE",
		"IMOrder",
		"orderId",
		"total",
	})
}

func TestRoundtripImportMapping_ValueTypes(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`CREATE ENTITY ` + testModule + `.IMAllTypes {
  IntVal: Integer;
  DecVal: Decimal(10,2);
  BoolVal: Boolean DEFAULT false;
  DateVal: DateTime;
};`); err != nil {
		t.Fatalf("CREATE ENTITY failed: %v", err)
	}

	createMDL := `CREATE IMPORT MAPPING ` + testModule + `.ImportAllTypes {
  "" -> ` + testModule + `.IMAllTypes (Create) {
    intVal   -> IntVal (Integer, KEY);
    decVal   -> DecVal (Decimal);
    boolVal  -> BoolVal (Boolean);
    dateVal  -> DateVal (DateTime);
  };
};`

	env.assertContains(createMDL, []string{
		"Integer",
		"Decimal",
		"Boolean",
		"DateTime",
	})
}

func TestRoundtripImportMapping_Drop(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`CREATE ENTITY ` + testModule + `.IMDropPet {
  Id: Integer;
};`); err != nil {
		t.Fatalf("CREATE ENTITY failed: %v", err)
	}

	if err := env.executeMDL(`CREATE IMPORT MAPPING ` + testModule + `.ToDropIM {
  "" -> ` + testModule + `.IMDropPet (Create) {
    id -> Id (Integer, KEY);
  };
};`); err != nil {
		t.Fatalf("CREATE IMPORT MAPPING failed: %v", err)
	}

	if _, err := env.describeMDL(`DESCRIBE IMPORT MAPPING ` + testModule + `.ToDropIM;`); err != nil {
		t.Fatalf("import mapping should exist before DROP: %v", err)
	}

	if err := env.executeMDL(`DROP IMPORT MAPPING ` + testModule + `.ToDropIM;`); err != nil {
		t.Fatalf("DROP IMPORT MAPPING failed: %v", err)
	}

	if _, err := env.describeMDL(`DESCRIBE IMPORT MAPPING ` + testModule + `.ToDropIM;`); err == nil {
		t.Error("import mapping should not exist after DROP")
	}
}

func TestRoundtripImportMapping_ShowAppearsInList(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`CREATE ENTITY ` + testModule + `.IMListPet {
  Id: Integer;
};`); err != nil {
		t.Fatalf("CREATE ENTITY failed: %v", err)
	}

	if err := env.executeMDL(`CREATE IMPORT MAPPING ` + testModule + `.ListableIM {
  "" -> ` + testModule + `.IMListPet (Create) {
    id -> Id (Integer, KEY);
  };
};`); err != nil {
		t.Fatalf("CREATE IMPORT MAPPING failed: %v", err)
	}

	env.output.Reset()
	if err := env.executeMDL(`SHOW IMPORT MAPPINGS IN ` + testModule + `;`); err != nil {
		t.Fatalf("SHOW failed: %v", err)
	}

	if !strings.Contains(env.output.String(), "ListableIM") {
		t.Errorf("expected 'ListableIM' in SHOW output:\n%s", env.output.String())
	}
}

// --- MX Check ---

func TestMxCheck_ImportMapping_Basic(t *testing.T) {
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`CREATE ENTITY ` + testModule + `.MxCheckIMPet {
  Id: Integer;
  Name: String(200);
};`); err != nil {
		t.Fatalf("CREATE ENTITY failed: %v", err)
	}

	if err := env.executeMDL(`CREATE IMPORT MAPPING ` + testModule + `.MxCheckImportPet {
  "" -> ` + testModule + `.MxCheckIMPet (Create) {
    id -> Id (Integer, KEY);
    name -> Name (String);
  };
};`); err != nil {
		t.Fatalf("CREATE IMPORT MAPPING failed: %v", err)
	}

	env.executor.Execute(&ast.DisconnectStmt{})

	output, err := runMxCheck(t, env.projectPath)
	assertMxCheckPassed(t, output, err)
}
