// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func TestRoundtripExportMapping_NoSchema(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	// First create the entity that will be exported
	if err := env.executeMDL(`CREATE ENTITY ` + testModule + `.EMPet {
  Id: Integer;
  Name: String(200);
};`); err != nil {
		t.Fatalf("CREATE ENTITY failed: %v", err)
	}

	createMDL := `CREATE EXPORT MAPPING ` + testModule + `.ExportPetBasic {
  ` + testModule + `.EMPet -> Root {
    Id -> id (Integer);
    Name -> name (String);
  };
};`

	env.assertContains(createMDL, []string{
		"EXPORT MAPPING",
		"ExportPetBasic",
		"EMPet",
		"Root",
	})
}

func TestRoundtripExportMapping_WithJsonStructureRef(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`CREATE ENTITY ` + testModule + `.EMOrder {
  OrderId: Integer;
  Total: Decimal(10,2);
};`); err != nil {
		t.Fatalf("CREATE ENTITY failed: %v", err)
	}

	if err := env.executeMDL(`CREATE JSON STRUCTURE ` + testModule + `.EMOrderJS
FROM '{"orderId": 1, "total": 99.99}';`); err != nil {
		t.Fatalf("CREATE JSON STRUCTURE failed: %v", err)
	}

	createMDL := `CREATE EXPORT MAPPING ` + testModule + `.ExportOrder
  TO JSON STRUCTURE ` + testModule + `.EMOrderJS
{
  ` + testModule + `.EMOrder -> Root {
    OrderId -> orderId (Integer);
    Total -> total (Decimal);
  };
};`

	env.assertContains(createMDL, []string{
		"EXPORT MAPPING",
		"ExportOrder",
		"TO JSON STRUCTURE",
		"EMOrder",
		"orderId",
		"total",
	})
}

func TestRoundtripExportMapping_NullValueOption(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`CREATE ENTITY ` + testModule + `.EMNullPet {
  Id: Integer;
};`); err != nil {
		t.Fatalf("CREATE ENTITY failed: %v", err)
	}

	if err := env.executeMDL(`CREATE EXPORT MAPPING ` + testModule + `.ExportNullPet
  NULL VALUES SendAsNil
{
  ` + testModule + `.EMNullPet -> Root {
    Id -> id (Integer);
  };
};`); err != nil {
		t.Fatalf("CREATE EXPORT MAPPING failed: %v", err)
	}

	out, err := env.describeMDL(`DESCRIBE EXPORT MAPPING ` + testModule + `.ExportNullPet;`)
	if err != nil {
		t.Fatalf("DESCRIBE failed: %v", err)
	}

	if !strings.Contains(out, "SendAsNil") {
		t.Errorf("expected 'SendAsNil' in DESCRIBE output:\n%s", out)
	}
}

func TestRoundtripExportMapping_Drop(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`CREATE ENTITY ` + testModule + `.EMDropPet {
  Id: Integer;
};`); err != nil {
		t.Fatalf("CREATE ENTITY failed: %v", err)
	}

	if err := env.executeMDL(`CREATE EXPORT MAPPING ` + testModule + `.ToDropEM {
  ` + testModule + `.EMDropPet -> Root {
    Id -> id (Integer);
  };
};`); err != nil {
		t.Fatalf("CREATE EXPORT MAPPING failed: %v", err)
	}

	if _, err := env.describeMDL(`DESCRIBE EXPORT MAPPING ` + testModule + `.ToDropEM;`); err != nil {
		t.Fatalf("export mapping should exist before DROP: %v", err)
	}

	if err := env.executeMDL(`DROP EXPORT MAPPING ` + testModule + `.ToDropEM;`); err != nil {
		t.Fatalf("DROP EXPORT MAPPING failed: %v", err)
	}

	if _, err := env.describeMDL(`DESCRIBE EXPORT MAPPING ` + testModule + `.ToDropEM;`); err == nil {
		t.Error("export mapping should not exist after DROP")
	}
}

func TestRoundtripExportMapping_ShowAppearsInList(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`CREATE ENTITY ` + testModule + `.EMListPet {
  Id: Integer;
};`); err != nil {
		t.Fatalf("CREATE ENTITY failed: %v", err)
	}

	if err := env.executeMDL(`CREATE EXPORT MAPPING ` + testModule + `.ListableEM {
  ` + testModule + `.EMListPet -> Root {
    Id -> id (Integer);
  };
};`); err != nil {
		t.Fatalf("CREATE EXPORT MAPPING failed: %v", err)
	}

	env.output.Reset()
	if err := env.executeMDL(`SHOW EXPORT MAPPINGS IN ` + testModule + `;`); err != nil {
		t.Fatalf("SHOW failed: %v", err)
	}

	if !strings.Contains(env.output.String(), "ListableEM") {
		t.Errorf("expected 'ListableEM' in SHOW output:\n%s", env.output.String())
	}
}

// --- MX Check ---

func TestMxCheck_ExportMapping_Basic(t *testing.T) {
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`CREATE ENTITY ` + testModule + `.MxCheckEMPet {
  Id: Integer;
  Name: String(200);
};`); err != nil {
		t.Fatalf("CREATE ENTITY failed: %v", err)
	}

	if err := env.executeMDL(`CREATE EXPORT MAPPING ` + testModule + `.MxCheckExportPet {
  ` + testModule + `.MxCheckEMPet -> Root {
    Id -> id (Integer);
    Name -> name (String);
  };
};`); err != nil {
		t.Fatalf("CREATE EXPORT MAPPING failed: %v", err)
	}

	env.executor.Execute(&ast.DisconnectStmt{})

	output, err := runMxCheck(t, env.projectPath)
	assertMxCheckPassed(t, output, err)
}
