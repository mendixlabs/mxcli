// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
)

func TestRewriteWithErrorHandling_OnlyWrapsCalls(t *testing.T) {
	lines := []string{
		"$p1 = CALL MICROFLOW MfTest.M012_CreateEntity(Name = 'A', Code = '1');",
		"CHANGE $p1 (IsActive = false);",
		"COMMIT $p1;",
		"DELETE $p1;",
	}

	got := rewriteWithErrorHandling(lines, "test_1")
	joined := strings.Join(got, "\n")

	if strings.Contains(joined, "CHANGE $p1 (IsActive = false) ON ERROR CONTINUE;") {
		t.Fatalf("CHANGE statement must not get ON ERROR CONTINUE:\n%s", joined)
	}
	if strings.Contains(joined, "COMMIT $p1 ON ERROR CONTINUE;") {
		t.Fatalf("COMMIT statement must not get ON ERROR CONTINUE:\n%s", joined)
	}
	if strings.Contains(joined, "DELETE $p1 ON ERROR CONTINUE;") {
		t.Fatalf("DELETE statement must not get ON ERROR CONTINUE:\n%s", joined)
	}
}

func TestGenerateTestRunner_ParsesWhenTestUsesChangeAndListOps(t *testing.T) {
	suite := &TestSuite{
		Name: "microflow-spec",
		Tests: []TestCase{
			{
				ID:   "test_1",
				Name: "List FILTER by IsActive",
				MDL: strings.Join([]string{
					"$p1 = CALL MICROFLOW MfTest.M012_CreateEntity(Name = 'Active', Code = 'FI-001');",
					"$p2 = CALL MICROFLOW MfTest.M012_CreateEntity(Name = 'Inactive', Code = 'FI-002');",
					"CHANGE $p2 (IsActive = false);",
					"$list = CREATE LIST OF MfTest.Product;",
					"ADD $p1 TO $list;",
					"ADD $p2 TO $list;",
					"$filtered = CALL MICROFLOW MfTest.M043_ListFilter(ProductList = $list);",
					"$count = CALL MICROFLOW MfTest.M051_AggregateCount(ProductList = $filtered);",
				}, "\n"),
				Expects: []Expect{
					{Variable: "$count", Operator: "=", Value: "1"},
				},
			},
		},
	}

	mdl := GenerateTestRunner(suite)
	if strings.Contains(mdl, "CHANGE $p2 (IsActive = false) ON ERROR CONTINUE;") {
		t.Fatalf("generated runner must not add ON ERROR to CHANGE:\n%s", mdl)
	}

	_, errs := visitor.Build(mdl)
	if len(errs) > 0 {
		t.Fatalf("generated runner should parse, got error: %v\n%s", errs[0], mdl)
	}
}

func TestGenerateTestRunner_RenamesAllAssignmentsInTestBlock(t *testing.T) {
	suite := &TestSuite{
		Name: "rename-spec",
		Tests: []TestCase{
			{
				ID:   "test_11",
				Name: "Multiple assignments",
				MDL: strings.Join([]string{
					"$product = CALL MICROFLOW MfTest.M012_CreateEntity(Name = 'ToUpdate', Code = 'TP-002');",
					"COMMIT $product;",
					"$result = CALL MICROFLOW MfTest.M015_UpdateEntity(Product = $product, NewName = 'UpdatedProduct');",
					"$list = CREATE LIST OF MfTest.Product;",
					"ADD $product TO $list;",
				}, "\n"),
				Expects: []Expect{
					{Variable: "$result", Operator: "=", Value: "true"},
				},
			},
		},
	}

	mdl := GenerateTestRunner(suite)
	for _, want := range []string{
		"$product_1 = CALL MICROFLOW",
		"COMMIT $product_1;",
		"$result_1 = CALL MICROFLOW",
		"$list_1 = CREATE LIST OF MfTest.Product;",
		"ADD $product_1 TO $list_1;",
		"$result_1 = true",
	} {
		if !strings.Contains(mdl, want) {
			t.Fatalf("generated runner is missing renamed fragment %q:\n%s", want, mdl)
		}
	}
}
