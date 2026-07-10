// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestRoundtripExportMapping_NoSchema(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`create entity ` + testModule + `.EMPet (
  PetId: Integer,
  Name: String(200)
);`); err != nil {
		t.Fatalf("create entity failed: %v", err)
	}

	createMDL := `create export mapping ` + testModule + `.ExportPetBasic {
  ` + testModule + `.EMPet {
    id = PetId,
    name = Name
  }
};`

	env.assertContains(createMDL, []string{
		"export mapping",
		"ExportPetBasic",
		"EMPet",
	})
}

func TestRoundtripExportMapping_WithJsonStructureRef(t *testing.T) {
	t.Skip("TODO: fix describe output for ExposedName vs original json key")
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`create entity ` + testModule + `.EMOrder (
  OrderId: Integer,
  Total: Decimal
);`); err != nil {
		t.Fatalf("create entity failed: %v", err)
	}

	if err := env.executeMDL(`create json structure ` + testModule + `.EMOrderJS
snippet '{"orderId": 1, "total": 99.99}';`); err != nil {
		t.Fatalf("create json structure failed: %v", err)
	}

	createMDL := `create export mapping ` + testModule + `.ExportOrder
  with json structure ` + testModule + `.EMOrderJS
{
  ` + testModule + `.EMOrder {
    orderId = OrderId,
    total = Total
  }
};`

	env.assertContains(createMDL, []string{
		"export mapping",
		"ExportOrder",
		"with json structure",
		"EMOrder",
		"orderId",
		"total",
	})
}

func TestRoundtripExportMapping_NullValueOption(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`create entity ` + testModule + `.EMNullPet (
  PetId: Integer
);`); err != nil {
		t.Fatalf("create entity failed: %v", err)
	}

	if err := env.executeMDL(`create export mapping ` + testModule + `.ExportNullPet
  null values SendAsNil
{
  ` + testModule + `.EMNullPet {
    id = PetId
  }
};`); err != nil {
		t.Fatalf("create export mapping failed: %v", err)
	}

	out, err := env.describeMDL(`describe export mapping ` + testModule + `.ExportNullPet;`)
	if err != nil {
		t.Fatalf("describe failed: %v", err)
	}

	if !strings.Contains(out, "SendAsNil") {
		t.Errorf("expected 'SendAsNil' in describe output:\n%s", out)
	}
}

func TestRoundtripExportMapping_Drop(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`create entity ` + testModule + `.EMDropPet (
  PetId: Integer
);`); err != nil {
		t.Fatalf("create entity failed: %v", err)
	}

	if err := env.executeMDL(`create export mapping ` + testModule + `.ToDropEM {
  ` + testModule + `.EMDropPet {
    id = PetId
  }
};`); err != nil {
		t.Fatalf("create export mapping failed: %v", err)
	}

	if _, err := env.describeMDL(`describe export mapping ` + testModule + `.ToDropEM;`); err != nil {
		t.Fatalf("export mapping should exist before drop: %v", err)
	}

	if err := env.executeMDL(`drop export mapping ` + testModule + `.ToDropEM;`); err != nil {
		t.Fatalf("drop export mapping failed: %v", err)
	}

	if _, err := env.describeMDL(`describe export mapping ` + testModule + `.ToDropEM;`); err == nil {
		t.Error("export mapping should not exist after drop")
	}
}

func TestRoundtripExportMapping_ShowAppearsInList(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`create entity ` + testModule + `.EMListPet (
  PetId: Integer
);`); err != nil {
		t.Fatalf("create entity failed: %v", err)
	}

	if err := env.executeMDL(`create export mapping ` + testModule + `.ListableEM {
  ` + testModule + `.EMListPet {
    id = PetId
  }
};`); err != nil {
		t.Fatalf("create export mapping failed: %v", err)
	}

	env.output.Reset()
	if err := env.executeMDL(`show export mappings in ` + testModule + `;`); err != nil {
		t.Fatalf("show failed: %v", err)
	}

	if !strings.Contains(env.output.String(), "ListableEM") {
		t.Errorf("expected 'ListableEM' in show output:\n%s", env.output.String())
	}
}

// --- MX Check ---

func TestMxCheck_ExportMapping_Basic(t *testing.T) {
	t.Skip("TODO: fix mapping BSON alignment with json structure schema (ExposedName/MaxOccurs)")
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`create entity ` + testModule + `.MxCheckEMPet (
  PetId: Integer,
  Name: String(200)
);`); err != nil {
		t.Fatalf("create entity failed: %v", err)
	}

	if err := env.executeMDL(`create json structure ` + testModule + `.MxCheckEMJS
snippet '{"id": 1, "name": "Fido"}';`); err != nil {
		t.Fatalf("create json structure failed: %v", err)
	}

	if err := env.executeMDL(`create export mapping ` + testModule + `.MxCheckExportPet
  with json structure ` + testModule + `.MxCheckEMJS
{
  ` + testModule + `.MxCheckEMPet {
    id = PetId,
    name = Name
  }
};`); err != nil {
		t.Fatalf("create export mapping failed: %v", err)
	}

	env.executor.Execute(&ast.DisconnectStmt{})

	output, err := runMxCheck(t, env.projectPath)
	assertMxCheckPassed(t, output, err)
}
