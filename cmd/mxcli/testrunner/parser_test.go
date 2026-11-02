// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseAnnotations(t *testing.T) {
	doc := `/**
 * @test String concatenation
 * @expect $result = 'John Doe'
 * @expect $product/Name = 'TestProduct'
 * @verify SELECT count(*) FROM MfTest.Product WHERE Code = 'X' = 1
 * @cleanup none
 */`

	a := parseAnnotations(doc)

	if a.Test != "String concatenation" {
		t.Errorf("Test name: got %q, want %q", a.Test, "String concatenation")
	}
	if len(a.Expects) != 2 {
		t.Fatalf("Expects count: got %d, want 2", len(a.Expects))
	}
	if a.Expects[0].Condition != "$result = 'John Doe'" {
		t.Errorf("Expect[0] condition: got %q, want %q", a.Expects[0].Condition, "$result = 'John Doe'")
	}
	if a.Expects[1].Condition != "$product/Name = 'TestProduct'" {
		t.Errorf("Expect[1] condition: got %q, want %q", a.Expects[1].Condition, "$product/Name = 'TestProduct'")
	}
	if len(a.Verify) != 1 {
		t.Fatalf("Verify count: got %d, want 1", len(a.Verify))
	}
	if a.Cleanup != "none" {
		t.Errorf("Cleanup: got %q, want %q", a.Cleanup, "none")
	}
}

func TestParseAnnotationsThrows(t *testing.T) {
	doc := `/**
 * @test Error handling on invalid input
 * @throws 'Validation failed'
 */`

	a := parseAnnotations(doc)

	if a.Test != "Error handling on invalid input" {
		t.Errorf("Test name: got %q, want %q", a.Test, "Error handling on invalid input")
	}
	if a.Throws != "Validation failed" {
		t.Errorf("Throws: got %q, want %q", a.Throws, "Validation failed")
	}
}

func TestParseMDLTests(t *testing.T) {
	content := `/**
 * @test First test
 * @expect $result = true
 */
$result = CALL MICROFLOW MfTest.M001_HelloWorld();
/

/**
 * @test Second test
 * @expect $result = 'John Doe'
 */
$result = CALL MICROFLOW MfTest.M003_StringOperations(
  FirstName = 'John', LastName = 'Doe'
);
/
`
	tests, err := parseMDLTests(content, "test.mdl")
	if err != nil {
		t.Fatalf("parseMDLTests: %v", err)
	}

	if len(tests) != 2 {
		t.Fatalf("Test count: got %d, want 2", len(tests))
	}

	if tests[0].Name != "First test" {
		t.Errorf("Test[0] name: got %q, want %q", tests[0].Name, "First test")
	}
	if tests[1].Name != "Second test" {
		t.Errorf("Test[1] name: got %q, want %q", tests[1].Name, "Second test")
	}
	if len(tests[1].Expects) != 1 {
		t.Errorf("Test[1] expects: got %d, want 1", len(tests[1].Expects))
	}
}

func TestParseMarkdownTests(t *testing.T) {
	content := "# Test Spec\n\nSome description.\n\n```mdl-test\n/** @test First test\n *  @expect $r = true\n */\n$r = CALL MICROFLOW MfTest.M001();\n```\n\nMore text.\n\n```mdl-test\n/** @test Second test */\n$r = CALL MICROFLOW MfTest.M002();\n```\n"

	tests, err := parseMarkdownTests(content, "test.md")
	if err != nil {
		t.Fatalf("parseMarkdownTests: %v", err)
	}

	if len(tests) != 2 {
		t.Fatalf("Test count: got %d, want 2", len(tests))
	}

	if tests[0].Name != "First test" {
		t.Errorf("Test[0] name: got %q, want %q", tests[0].Name, "First test")
	}
	if len(tests[0].Expects) != 1 {
		t.Errorf("Test[0] expects: got %d, want 1", len(tests[0].Expects))
	}
	if tests[1].Name != "Second test" {
		t.Errorf("Test[1] name: got %q, want %q", tests[1].Name, "Second test")
	}
}

func TestParseTestFileMDL(t *testing.T) {
	// Create a temp test file
	dir := t.TempDir()
	path := filepath.Join(dir, "example.test.mdl")
	content := `/**
 * @test Hello World
 * @expect $result = true
 */
$result = CALL MICROFLOW MfTest.M001_HelloWorld();
/
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	suite, err := ParseTestFile(path)
	if err != nil {
		t.Fatalf("ParseTestFile: %v", err)
	}

	if suite.Name != "example" {
		t.Errorf("Suite name: got %q, want %q", suite.Name, "example")
	}
	if len(suite.Tests) != 1 {
		t.Fatalf("Test count: got %d, want 1", len(suite.Tests))
	}
	if suite.Tests[0].Name != "Hello World" {
		t.Errorf("Test name: got %q, want %q", suite.Tests[0].Name, "Hello World")
	}
}

func TestGenerateTestRunner(t *testing.T) {
	suite := &TestSuite{
		Name: "example",
		Tests: []TestCase{
			{
				ID:   "test_1",
				Name: "Hello World",
				MDL:  "$result = CALL MICROFLOW MfTest.M001_HelloWorld();",
				Expects: []Expect{
					expectOf("$result = true"),
				},
			},
			{
				ID:   "test_2",
				Name: "String concat",
				MDL:  "$result = CALL MICROFLOW MfTest.M003(FirstName = 'John', LastName = 'Doe');",
				Expects: []Expect{
					expectOf("$result = 'John Doe'"),
				},
			},
		},
	}

	mdl := GenerateTestRunner(suite)

	// Check that it contains key patterns
	if !contains(mdl, "CREATE OR MODIFY MICROFLOW MxTest.TestRunner") {
		t.Error("Missing CREATE OR MODIFY MICROFLOW")
	}
	if !contains(mdl, "RETURNS Boolean") {
		t.Error("Missing RETURNS Boolean")
	}
	if !contains(mdl, "MXTEST:START:example") {
		t.Error("Missing MXTEST:START")
	}
	if !contains(mdl, "MXTEST:END:example") {
		t.Error("Missing MXTEST:END")
	}
	if !contains(mdl, "MXTEST:RUN:test_1:Hello World") {
		t.Error("Missing MXTEST:RUN for test_1")
	}
	if !contains(mdl, "ON ERROR") {
		t.Error("Missing ON ERROR clause")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
