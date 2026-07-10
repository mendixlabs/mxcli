// SPDX-License-Identifier: Apache-2.0

//go:build integration

// Tests for catalog reference extraction and SHOW CALLERS/CALLEES/REFERENCES/IMPACT commands.
// These verify that cross-references between microflows, entities, pages, and associations
// are correctly registered in the catalog refs table.
package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

// --- Helpers ---

// buildCatalogFull triggers a full catalog rebuild with references.
func buildCatalogFull(t *testing.T, env *testEnv) {
	t.Helper()
	if err := env.executor.Execute(&ast.RefreshCatalogStmt{Full: true, Force: true}); err != nil {
		t.Fatalf("refresh catalog full force failed: %v", err)
	}
}

// countRefs returns the number of refs matching the given source, target, and kind.
// Pass empty string for any parameter to skip that filter.
func countRefs(t *testing.T, env *testEnv, sourceName, targetName, refKind string) int {
	t.Helper()
	if env.executor.catalog == nil {
		t.Fatal("catalog not built")
	}

	conditions := []string{"1=1"}
	if sourceName != "" {
		conditions = append(conditions, fmt.Sprintf("SourceName = '%s'", sourceName))
	}
	if targetName != "" {
		conditions = append(conditions, fmt.Sprintf("TargetName = '%s'", targetName))
	}
	if refKind != "" {
		conditions = append(conditions, fmt.Sprintf("RefKind = '%s'", refKind))
	}

	query := "select count(*) as cnt from refs where " + strings.Join(conditions, " and ")
	result, err := env.executor.catalog.Query(query)
	if err != nil {
		t.Fatalf("refs query failed: %v", err)
	}
	if result.Count == 0 || len(result.Rows) == 0 {
		return 0
	}
	// Parse count from first row
	cntStr := fmt.Sprintf("%v", result.Rows[0][0])
	var cnt int
	fmt.Sscanf(cntStr, "%d", &cnt)
	return cnt
}

// assertRefExists verifies that at least one ref row exists matching the criteria.
func assertRefExists(t *testing.T, env *testEnv, sourceName, targetName, refKind string) {
	t.Helper()
	cnt := countRefs(t, env, sourceName, targetName, refKind)
	if cnt == 0 {
		t.Errorf("expected ref (source=%q, target=%q, kind=%q) but found none", sourceName, targetName, refKind)
	}
}

// assertNoRef verifies that no ref row exists matching the criteria.
func assertNoRef(t *testing.T, env *testEnv, sourceName, targetName, refKind string) {
	t.Helper()
	cnt := countRefs(t, env, sourceName, targetName, refKind)
	if cnt > 0 {
		t.Errorf("expected no ref (source=%q, target=%q, kind=%q) but found %d", sourceName, targetName, refKind, cnt)
	}
}

// --- Tier 1: Direct refs table verification ---

func TestCatalogRefs_MicroflowCallsMicroflow(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	mod := testModule

	// Create target microflow
	if err := env.executeMDL(fmt.Sprintf(`create microflow %s.TargetMf () returns Boolean
begin
  return true;
end;`, mod)); err != nil {
		t.Fatalf("Failed to create target microflow: %v", err)
	}

	// Create caller microflow
	if err := env.executeMDL(fmt.Sprintf(`create microflow %s.CallerMf () returns Boolean
begin
  $Result = call microflow %s.TargetMf ();
  return $Result;
end;`, mod, mod)); err != nil {
		t.Fatalf("Failed to create caller microflow: %v", err)
	}

	buildCatalogFull(t, env)
	assertRefExists(t, env, mod+".CallerMf", mod+".TargetMf", "call")
}

func TestCatalogRefs_MicroflowCreatesEntity(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	mod := testModule

	if err := env.executeMDL(fmt.Sprintf(`create or modify persistent entity %s.RefCustomer (Name: String(100));`, mod)); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	if err := env.executeMDL(fmt.Sprintf(`create microflow %s.CreatorMf () returns Boolean
begin
  $Obj = create %s.RefCustomer;
  return true;
end;`, mod, mod)); err != nil {
		t.Fatalf("Failed to create microflow: %v", err)
	}

	buildCatalogFull(t, env)
	assertRefExists(t, env, mod+".CreatorMf", mod+".RefCustomer", "create")
}

func TestCatalogRefs_MicroflowRetrievesEntity(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	mod := testModule

	if err := env.executeMDL(fmt.Sprintf(`create or modify persistent entity %s.RefProduct (Code: String(50));`, mod)); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	if err := env.executeMDL(fmt.Sprintf(`create microflow %s.RetrieverMf () returns Boolean
begin
  retrieve $Items from %s.RefProduct;
  return true;
end;`, mod, mod)); err != nil {
		t.Fatalf("Failed to create microflow: %v", err)
	}

	buildCatalogFull(t, env)
	assertRefExists(t, env, mod+".RetrieverMf", mod+".RefProduct", "retrieve")
}

// A microflow that only takes/returns an entity (never create/retrieve) still
// references it via its parameter and return type.
func TestCatalogRefs_MicroflowParameterAndReturn(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	mod := testModule

	if err := env.executeMDL(fmt.Sprintf(`create or modify persistent entity %s.RefAcct (Name: String(50));`, mod)); err != nil {
		t.Fatal(err)
	}
	if err := env.executeMDL(fmt.Sprintf(`create microflow %s.PassThru ($Acct: %s.RefAcct) returns %s.RefAcct
begin
  return $Acct;
end;`, mod, mod, mod)); err != nil {
		t.Fatal(err)
	}

	buildCatalogFull(t, env)
	assertRefExists(t, env, mod+".PassThru", mod+".RefAcct", "parameter")
	assertRefExists(t, env, mod+".PassThru", mod+".RefAcct", "return")
}

func TestCatalogRefs_Association(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	mod := testModule

	if err := env.executeMDL(fmt.Sprintf(`create or modify persistent entity %s.RefParent (Name: String(100));`, mod)); err != nil {
		t.Fatal(err)
	}
	if err := env.executeMDL(fmt.Sprintf(`create or modify persistent entity %s.RefChild (Label: String(100));`, mod)); err != nil {
		t.Fatal(err)
	}
	if err := env.executeMDL(fmt.Sprintf(`create association %s.RefChild_RefParent from %s.RefChild to %s.RefParent;`, mod, mod, mod)); err != nil {
		t.Fatal(err)
	}

	buildCatalogFull(t, env)
	// An association emits an `associate` ref to each endpoint (FROM and TO).
	assertRefExists(t, env, mod+".RefChild_RefParent", mod+".RefParent", "associate")
	assertRefExists(t, env, mod+".RefChild_RefParent", mod+".RefChild", "associate")
}

func TestCatalogRefs_MultipleRefKindsToSameTarget(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	mod := testModule

	if err := env.executeMDL(fmt.Sprintf(`create or modify persistent entity %s.RefOrder (OrderNum: String(50));`, mod)); err != nil {
		t.Fatal(err)
	}

	// Microflow that both creates and retrieves the same entity
	if err := env.executeMDL(fmt.Sprintf(`create microflow %s.MultiRefMf () returns Boolean
begin
  $Obj = create %s.RefOrder;
  retrieve $List from %s.RefOrder;
  return true;
end;`, mod, mod, mod)); err != nil {
		t.Fatal(err)
	}

	buildCatalogFull(t, env)
	assertRefExists(t, env, mod+".MultiRefMf", mod+".RefOrder", "create")
	assertRefExists(t, env, mod+".MultiRefMf", mod+".RefOrder", "retrieve")
}

func TestCatalogRefs_NoReferences(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	mod := testModule

	if err := env.executeMDL(fmt.Sprintf(`create or modify persistent entity %s.Orphan (Name: String(100));`, mod)); err != nil {
		t.Fatal(err)
	}

	buildCatalogFull(t, env)

	// No microflow or page references this entity
	cnt := countRefs(t, env, "", mod+".Orphan", "")
	if cnt > 0 {
		t.Errorf("expected no references to orphan entity, found %d", cnt)
	}
}

// --- Tier 2: SHOW command output verification ---

func TestCatalogRefs_ShowCallersOf(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	mod := testModule

	// Create target
	if err := env.executeMDL(fmt.Sprintf(`create microflow %s.CalleeA () returns Boolean
begin
  return true;
end;`, mod)); err != nil {
		t.Fatal(err)
	}

	// Create caller
	if err := env.executeMDL(fmt.Sprintf(`create microflow %s.CallerA () returns Boolean
begin
  $R = call microflow %s.CalleeA ();
  return $R;
end;`, mod, mod)); err != nil {
		t.Fatal(err)
	}

	buildCatalogFull(t, env)

	// Execute SHOW CALLERS OF
	env.output.Reset()
	err := env.executor.Execute(&ast.ShowStmt{
		ObjectType: ast.ShowCallers,
		Name:       parseQualifiedName(mod + ".CalleeA"),
	})
	if err != nil {
		t.Fatalf("show callers failed: %v", err)
	}

	output := env.output.String()
	if !strings.Contains(output, mod+".CallerA") {
		t.Errorf("expected output to contain %s.CallerA, got:\n%s", mod, output)
	}
}

func TestCatalogRefs_ShowCalleesOf(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	mod := testModule

	// Create callee
	if err := env.executeMDL(fmt.Sprintf(`create microflow %s.CalleeB () returns Boolean
begin
  return true;
end;`, mod)); err != nil {
		t.Fatal(err)
	}

	// Create caller
	if err := env.executeMDL(fmt.Sprintf(`create microflow %s.CallerB () returns Boolean
begin
  $R = call microflow %s.CalleeB ();
  return $R;
end;`, mod, mod)); err != nil {
		t.Fatal(err)
	}

	buildCatalogFull(t, env)

	env.output.Reset()
	err := env.executor.Execute(&ast.ShowStmt{
		ObjectType: ast.ShowCallees,
		Name:       parseQualifiedName(mod + ".CallerB"),
	})
	if err != nil {
		t.Fatalf("show callees failed: %v", err)
	}

	output := env.output.String()
	if !strings.Contains(output, mod+".CalleeB") {
		t.Errorf("expected output to contain %s.CalleeB, got:\n%s", mod, output)
	}
}

func TestCatalogRefs_ShowCallersTransitive(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	mod := testModule

	// Create chain: CallerC1 -> CallerC2 -> CalleeC
	if err := env.executeMDL(fmt.Sprintf(`create microflow %s.CalleeC () returns Boolean
begin
  return true;
end;`, mod)); err != nil {
		t.Fatal(err)
	}

	if err := env.executeMDL(fmt.Sprintf(`create microflow %s.CallerC2 () returns Boolean
begin
  $R = call microflow %s.CalleeC ();
  return $R;
end;`, mod, mod)); err != nil {
		t.Fatal(err)
	}

	if err := env.executeMDL(fmt.Sprintf(`create microflow %s.CallerC1 () returns Boolean
begin
  $R = call microflow %s.CallerC2 ();
  return $R;
end;`, mod, mod)); err != nil {
		t.Fatal(err)
	}

	buildCatalogFull(t, env)

	// Non-transitive: only direct callers
	env.output.Reset()
	err := env.executor.Execute(&ast.ShowStmt{
		ObjectType: ast.ShowCallers,
		Name:       parseQualifiedName(mod + ".CalleeC"),
	})
	if err != nil {
		t.Fatalf("show callers failed: %v", err)
	}
	output := env.output.String()
	if !strings.Contains(output, mod+".CallerC2") {
		t.Errorf("expected direct caller CallerC2 in output:\n%s", output)
	}

	// Transitive: should include CallerC1 too
	env.output.Reset()
	err = env.executor.Execute(&ast.ShowStmt{
		ObjectType: ast.ShowCallers,
		Name:       parseQualifiedName(mod + ".CalleeC"),
		Transitive: true,
	})
	if err != nil {
		t.Fatalf("show callers transitive failed: %v", err)
	}
	output = env.output.String()
	if !strings.Contains(output, mod+".CallerC2") {
		t.Errorf("expected CallerC2 in transitive output:\n%s", output)
	}
	if !strings.Contains(output, mod+".CallerC1") {
		t.Errorf("expected CallerC1 in transitive output:\n%s", output)
	}
}

func TestCatalogRefs_ShowReferencesTo(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	mod := testModule

	if err := env.executeMDL(fmt.Sprintf(`create or modify persistent entity %s.RefTarget (Name: String(100));`, mod)); err != nil {
		t.Fatal(err)
	}

	// Microflow creates the entity
	if err := env.executeMDL(fmt.Sprintf(`create microflow %s.RefCreator () returns Boolean
begin
  $Obj = create %s.RefTarget;
  return true;
end;`, mod, mod)); err != nil {
		t.Fatal(err)
	}

	// Another microflow retrieves it
	if err := env.executeMDL(fmt.Sprintf(`create microflow %s.RefRetriever () returns Boolean
begin
  retrieve $List from %s.RefTarget;
  return true;
end;`, mod, mod)); err != nil {
		t.Fatal(err)
	}

	buildCatalogFull(t, env)

	env.output.Reset()
	err := env.executor.Execute(&ast.ShowStmt{
		ObjectType: ast.ShowReferences,
		Name:       parseQualifiedName(mod + ".RefTarget"),
	})
	if err != nil {
		t.Fatalf("show references to failed: %v", err)
	}

	output := env.output.String()
	if !strings.Contains(output, mod+".RefCreator") {
		t.Errorf("expected RefCreator in references output:\n%s", output)
	}
	if !strings.Contains(output, mod+".RefRetriever") {
		t.Errorf("expected RefRetriever in references output:\n%s", output)
	}
}

// TestCatalogRefs_ShowContextResolvesTypes guards against the case-mismatch that
// silently broke SHOW CONTEXT's relationship sections: they filter on
// TargetType/SourceType, but those literals were lowercase ('entity') while the
// refs builder stores uppercase ('ENTITY'), and SQLite '=' is case-sensitive — so
// "Entities Used" / "Microflows Using This Entity" always rendered empty.
func TestCatalogRefs_ShowContextResolvesTypes(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	mod := testModule

	if err := env.executeMDL(fmt.Sprintf(`create or modify persistent entity %s.CtxEntity (Name: String(100));`, mod)); err != nil {
		t.Fatal(err)
	}
	if err := env.executeMDL(fmt.Sprintf(`create microflow %s.CtxCreator () returns Boolean
begin
  $Obj = create %s.CtxEntity;
  return true;
end;`, mod, mod)); err != nil {
		t.Fatal(err)
	}

	buildCatalogFull(t, env)

	// Microflow context must list the entity it uses (TargetType = 'ENTITY').
	env.output.Reset()
	if err := env.executor.Execute(&ast.ShowStmt{
		ObjectType: ast.ShowContext,
		Name:       parseQualifiedName(mod + ".CtxCreator"),
	}); err != nil {
		t.Fatalf("show context of microflow failed: %v", err)
	}
	if mfCtx := env.output.String(); !strings.Contains(mfCtx, mod+".CtxEntity") {
		t.Errorf("microflow context should list the entity it uses:\n%s", mfCtx)
	}

	// Entity context must list the microflow that uses it (SourceType = 'MICROFLOW').
	env.output.Reset()
	if err := env.executor.Execute(&ast.ShowStmt{
		ObjectType: ast.ShowContext,
		Name:       parseQualifiedName(mod + ".CtxEntity"),
	}); err != nil {
		t.Fatalf("show context of entity failed: %v", err)
	}
	if entCtx := env.output.String(); !strings.Contains(entCtx, mod+".CtxCreator") {
		t.Errorf("entity context should list the microflow using it:\n%s", entCtx)
	}
}

func TestCatalogRefs_ShowReferencesNoResults(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	mod := testModule

	if err := env.executeMDL(fmt.Sprintf(`create or modify persistent entity %s.Unreferenced (Name: String(100));`, mod)); err != nil {
		t.Fatal(err)
	}

	buildCatalogFull(t, env)

	env.output.Reset()
	err := env.executor.Execute(&ast.ShowStmt{
		ObjectType: ast.ShowReferences,
		Name:       parseQualifiedName(mod + ".Unreferenced"),
	})
	if err != nil {
		t.Fatalf("show references to failed: %v", err)
	}

	output := env.output.String()
	if !strings.Contains(output, "no references found") {
		t.Errorf("expected 'no references found' in output:\n%s", output)
	}
}

func TestCatalogRefs_ShowImpactOf(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	mod := testModule

	if err := env.executeMDL(fmt.Sprintf(`create or modify persistent entity %s.ImpactEntity (Name: String(100));`, mod)); err != nil {
		t.Fatal(err)
	}

	if err := env.executeMDL(fmt.Sprintf(`create microflow %s.ImpactMf () returns Boolean
begin
  $Obj = create %s.ImpactEntity;
  return true;
end;`, mod, mod)); err != nil {
		t.Fatal(err)
	}

	buildCatalogFull(t, env)

	env.output.Reset()
	err := env.executor.Execute(&ast.ShowStmt{
		ObjectType: ast.ShowImpact,
		Name:       parseQualifiedName(mod + ".ImpactEntity"),
	})
	if err != nil {
		t.Fatalf("show impact failed: %v", err)
	}

	output := env.output.String()
	if !strings.Contains(output, mod+".ImpactMf") {
		t.Errorf("expected ImpactMf in impact output:\n%s", output)
	}
}
