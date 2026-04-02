// SPDX-License-Identifier: Apache-2.0

//go:build integration

// Package executor provides roundtrip tests for MDL commands.
// These tests verify that creating a document and describing it back
// produces semantically equivalent results.
//
// Test categories:
// - Roundtrip tests: Create document → Describe → Verify semantic properties
// - MxCheck tests: Create document → Run mx check → Verify no errors
package executor

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/pmezard/go-difflib/difflib"
)

// sourceProject is the committed source project path (fast path).
const sourceProject = "../../mx-test-projects/test-source-app"

// sourceProjectMPR is the MPR filename inside the source project.
const sourceProjectMPR = "test-source.mpr"

// testModule is the module name used for test entities.
const testModule = "RoundtripTest"

// sharedSourceProject is set once by TestMain to the directory containing the
// pristine source project. All tests copy from this directory.
var sharedSourceProject string

// sharedSourceMPR is the MPR filename inside sharedSourceProject.
var sharedSourceMPR string

// TestMain creates or locates the source project once, then runs all tests.
// This avoids running `mx create-project` per test (~29s each).
func TestMain(m *testing.M) {
	// 1. Try the committed source project
	srcDir, err := filepath.Abs(sourceProject)
	if err == nil {
		if _, err := os.Stat(filepath.Join(srcDir, sourceProjectMPR)); err == nil {
			sharedSourceProject = srcDir
			sharedSourceMPR = sourceProjectMPR
			os.Exit(m.Run())
		}
	}

	// 2. Create a project once using mx create-project
	mxPath := findMxBinary()
	if mxPath == "" {
		fmt.Fprintln(os.Stderr, "SKIP: mx binary not available and source project not found at", sourceProject)
		os.Exit(0)
	}

	tmpDir, err := os.MkdirTemp("", "roundtrip-source-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: could not create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	fmt.Fprintf(os.Stderr, "TestMain: creating shared source project with %s ...\n", mxPath)
	cmd := exec.Command(mxPath, "create-project")
	cmd.Dir = tmpDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "SKIP: mx create-project failed: %v\n%s\n", err, output)
		os.Exit(0)
	}

	mprPath := filepath.Join(tmpDir, "App.mpr")
	if _, err := os.Stat(mprPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "SKIP: mx create-project did not produce App.mpr in %s\n", tmpDir)
		os.Exit(0)
	}

	sharedSourceProject = tmpDir
	sharedSourceMPR = "App.mpr"
	fmt.Fprintf(os.Stderr, "TestMain: shared source project ready at %s\n", tmpDir)
	os.Exit(m.Run())
}

// testEnv holds the test environment for roundtrip tests.
type testEnv struct {
	t           *testing.T
	executor    *Executor
	output      *bytes.Buffer
	projectPath string // path to the copied MPR file
}

// copyTestProject copies the shared source project to a temp directory and returns the MPR path.
// The temp directory is automatically cleaned up when the test finishes.
func copyTestProject(t *testing.T) string {
	t.Helper()

	if sharedSourceProject == "" {
		t.Fatal("sharedSourceProject not set — TestMain did not run")
	}

	destDir := t.TempDir()

	// Copy the MPR file
	srcMPR := filepath.Join(sharedSourceProject, sharedSourceMPR)
	destMPR := filepath.Join(destDir, sharedSourceMPR)
	if err := copyFile(srcMPR, destMPR); err != nil {
		t.Fatalf("Failed to copy MPR file: %v", err)
	}

	// Copy required directories
	for _, dir := range []string{"mprcontents", "widgets", "themesource", "theme", "javascriptsource"} {
		srcSub := filepath.Join(sharedSourceProject, dir)
		if _, err := os.Stat(srcSub); err == nil {
			if err := copyDir(srcSub, filepath.Join(destDir, dir)); err != nil {
				t.Fatalf("Failed to copy %s: %v", dir, err)
			}
		}
	}

	return destMPR
}

// findMxBinary searches for the mx command in known locations.
// Search order: MX_BINARY env var, reference/mxbuild/modeler/mx (repo-local),
// ~/.mxcli/mxbuild/*/modeler/mx (cached downloads), PATH lookup.
func findMxBinary() string {
	// 0. Explicit override via environment variable
	if p := os.Getenv("MX_BINARY"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// 1. Repo-local reference path
	repoPath, err := filepath.Abs("../../reference/mxbuild/modeler/mx")
	if err == nil {
		if _, err := os.Stat(repoPath); err == nil {
			return repoPath
		}
	}

	// 2. Cached downloads (~/.mxcli/mxbuild/*/modeler/mx)
	if home, err := os.UserHomeDir(); err == nil {
		pattern := filepath.Join(home, ".mxcli", "mxbuild", "*", "modeler", "mx")
		if matches, _ := filepath.Glob(pattern); len(matches) > 0 {
			return matches[len(matches)-1]
		}
	}

	// 3. PATH lookup
	if p, err := exec.LookPath("mx"); err == nil {
		return p
	}

	return ""
}

// copyFile copies a single file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// copyDir recursively copies a directory tree.
func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// setupTestEnv creates a new test environment with a fresh copy of the source project.
func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()

	projectPath := copyTestProject(t)

	output := &bytes.Buffer{}
	exec := New(output)

	// Connect to project
	connectStmt := &ast.ConnectStmt{
		Path: projectPath,
	}
	if err := exec.Execute(connectStmt); err != nil {
		t.Fatalf("Failed to connect to project: %v", err)
	}

	// Ensure test module exists
	env := &testEnv{
		t:           t,
		executor:    exec,
		output:      output,
		projectPath: projectPath,
	}
	env.ensureTestModule()

	return env
}

// ensureTestModule creates the test module if it doesn't exist.
func (e *testEnv) ensureTestModule() {
	e.t.Helper()

	// Try to create module (ignore error if already exists)
	createModuleStmt := &ast.CreateModuleStmt{
		Name: testModule,
	}
	_ = e.executor.Execute(createModuleStmt)
}

// teardown disconnects from the project. No cleanup of created artifacts is needed
// since each test uses a fresh copy that is automatically deleted.
func (e *testEnv) teardown() {
	if e.executor != nil {
		e.executor.Execute(&ast.DisconnectStmt{})
	}
}

// registerCleanup is a no-op since each test uses a fresh project copy.
// Kept for API compatibility with existing test code.
func (e *testEnv) registerCleanup(docType, qualifiedName string) {
	// No-op: temp directory is automatically cleaned up
}

// executeMDL parses and executes MDL commands.
func (e *testEnv) executeMDL(mdl string) error {
	e.t.Helper()
	e.output.Reset()

	prog, errs := visitor.Build(mdl)
	if len(errs) > 0 {
		return errs[0]
	}

	for _, stmt := range prog.Statements {
		if err := e.executor.Execute(stmt); err != nil {
			return err
		}
	}
	return nil
}

// describeMDL executes a DESCRIBE command and returns the output.
func (e *testEnv) describeMDL(describeCmd string) (string, error) {
	e.t.Helper()
	e.output.Reset()

	prog, errs := visitor.Build(describeCmd)
	if len(errs) > 0 {
		return "", errs[0]
	}

	for _, stmt := range prog.Statements {
		if err := e.executor.Execute(stmt); err != nil {
			return "", err
		}
	}
	return e.output.String(), nil
}

// parseQualifiedName parses "Module.Name" into ast.QualifiedName.
func parseQualifiedName(name string) *ast.QualifiedName {
	parts := strings.SplitN(name, ".", 2)
	if len(parts) == 2 {
		return &ast.QualifiedName{Module: parts[0], Name: parts[1]}
	}
	return &ast.QualifiedName{Name: name}
}

// --- Diff-Based Roundtrip Helpers ---
//
// These helpers use the diff infrastructure for more precise comparison
// and better error messages when roundtrip tests fail.

// RoundtripOption configures roundtrip verification behavior.
type RoundtripOption func(*roundtripConfig)

type roundtripConfig struct {
	ignorePatterns   []string        // Lines matching these patterns are ignored
	ignoreAttributes map[string]bool // Attribute names to ignore (e.g., "changedDate")
	allowNewLines    bool            // Allow DESCRIBE output to have additional lines
	entityType       string          // "entity", "enumeration", "page", "microflow", etc.
}

// IgnorePattern returns an option to ignore lines containing the given pattern.
func IgnorePattern(pattern string) RoundtripOption {
	return func(c *roundtripConfig) {
		c.ignorePatterns = append(c.ignorePatterns, pattern)
	}
}

// IgnoreAttribute returns an option to ignore a specific attribute in comparison.
func IgnoreAttribute(attrName string) RoundtripOption {
	return func(c *roundtripConfig) {
		if c.ignoreAttributes == nil {
			c.ignoreAttributes = make(map[string]bool)
		}
		c.ignoreAttributes[attrName] = true
	}
}

// AllowNewLines returns an option to allow DESCRIBE output to have additional lines.
func AllowNewLines() RoundtripOption {
	return func(c *roundtripConfig) {
		c.allowNewLines = true
	}
}

// RoundtripResult contains the result of a roundtrip test.
type RoundtripResult struct {
	Expected string   // Normalized MDL from script
	Actual   string   // Normalized MDL from DESCRIBE
	Diff     string   // Unified diff if there are differences
	Changes  []string // List of structural changes
	Success  bool     // Whether roundtrip passed
}

// assertRoundtrip executes MDL, describes the result, and verifies they match.
// It uses the diff infrastructure for comparison and provides clear error output.
func (e *testEnv) assertRoundtrip(createMDL string, opts ...RoundtripOption) RoundtripResult {
	e.t.Helper()

	config := &roundtripConfig{
		ignorePatterns: []string{"@Position"}, // Default: ignore position annotations
	}
	for _, opt := range opts {
		opt(config)
	}

	result := RoundtripResult{}

	// Parse MDL to get statement type and qualified name
	prog, errs := visitor.Build(createMDL)
	if len(errs) > 0 {
		e.t.Fatalf("Failed to parse MDL: %v", errs[0])
		return result
	}
	if len(prog.Statements) == 0 {
		e.t.Fatal("No statements in MDL")
		return result
	}

	// Execute CREATE statement
	e.output.Reset()
	for _, stmt := range prog.Statements {
		if err := e.executor.Execute(stmt); err != nil {
			e.t.Fatalf("Failed to execute MDL: %v", err)
			return result
		}
	}

	// Determine object type and name for DESCRIBE
	var describeCmd string
	var qualifiedName string
	switch s := prog.Statements[0].(type) {
	case *ast.CreateEntityStmt:
		qualifiedName = s.Name.String()
		e.registerCleanup("entity", qualifiedName)
		describeCmd = "DESCRIBE ENTITY " + qualifiedName + ";"
	case *ast.CreateViewEntityStmt:
		qualifiedName = s.Name.String()
		e.registerCleanup("entity", qualifiedName)
		describeCmd = "DESCRIBE ENTITY " + qualifiedName + ";"
	case *ast.CreateEnumerationStmt:
		qualifiedName = s.Name.String()
		e.registerCleanup("enumeration", qualifiedName)
		describeCmd = "DESCRIBE ENUMERATION " + qualifiedName + ";"
	case *ast.CreatePageStmtV3:
		qualifiedName = s.Name.String()
		e.registerCleanup("page", qualifiedName)
		describeCmd = "DESCRIBE PAGE " + qualifiedName + ";"
	case *ast.CreateSnippetStmtV3:
		qualifiedName = s.Name.String()
		e.registerCleanup("snippet", qualifiedName)
		describeCmd = "DESCRIBE SNIPPET " + qualifiedName + ";"
	case *ast.CreateMicroflowStmt:
		qualifiedName = s.Name.String()
		e.registerCleanup("microflow", qualifiedName)
		describeCmd = "DESCRIBE MICROFLOW " + qualifiedName + ";"
	case *ast.CreateAssociationStmt:
		qualifiedName = s.Name.String()
		describeCmd = "DESCRIBE ASSOCIATION " + qualifiedName + ";"
	case *ast.CreateDatabaseConnectionStmt:
		qualifiedName = s.Name.String()
		describeCmd = "DESCRIBE DATABASE CONNECTION " + qualifiedName + ";"
	case *ast.CreateRestClientStmt:
		qualifiedName = s.Name.String()
		describeCmd = "DESCRIBE REST CLIENT " + qualifiedName + ";"
	case *ast.CreateJsonStructureStmt:
		qualifiedName = s.Name.String()
		describeCmd = "DESCRIBE JSON STRUCTURE " + qualifiedName + ";"
	case *ast.CreateImportMappingStmt:
		qualifiedName = s.Name.String()
		describeCmd = "DESCRIBE IMPORT MAPPING " + qualifiedName + ";"
	case *ast.CreateExportMappingStmt:
		qualifiedName = s.Name.String()
		describeCmd = "DESCRIBE EXPORT MAPPING " + qualifiedName + ";"
	default:
		e.t.Fatalf("Unsupported statement type for roundtrip: %T", prog.Statements[0])
		return result
	}

	// Execute DESCRIBE
	describeOutput, err := e.describeMDL(describeCmd)
	if err != nil {
		e.t.Fatalf("Failed to describe %s: %v", qualifiedName, err)
		return result
	}

	// Normalize both MDL strings for comparison
	result.Expected = normalizeMDL(createMDL, config)
	result.Actual = normalizeMDL(describeOutput, config)

	// Compare and generate diff
	if result.Expected != result.Actual {
		diff := difflib.UnifiedDiff{
			A:        difflib.SplitLines(result.Expected),
			B:        difflib.SplitLines(result.Actual),
			FromFile: "expected (script)",
			ToFile:   "actual (describe)",
			Context:  3,
		}
		result.Diff, _ = difflib.GetUnifiedDiffString(diff)

		// Extract structural changes
		result.Changes = extractStructuralChanges(result.Expected, result.Actual)
		result.Success = false
	} else {
		result.Success = true
	}

	return result
}

// normalizeMDL normalizes MDL for comparison by removing ignored patterns and whitespace variations.
func normalizeMDL(mdl string, config *roundtripConfig) string {
	lines := strings.Split(mdl, "\n")
	var normalized []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines
		if trimmed == "" {
			continue
		}

		// Skip ignored patterns
		skip := false
		for _, pattern := range config.ignorePatterns {
			if strings.Contains(trimmed, pattern) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		// Skip statement terminators on their own line
		if trimmed == "/" || trimmed == ";" {
			continue
		}

		normalized = append(normalized, trimmed)
	}

	return strings.Join(normalized, "\n")
}

// extractStructuralChanges extracts a list of high-level changes between two MDL strings.
func extractStructuralChanges(expected, actual string) []string {
	var changes []string

	expectedLines := strings.Split(expected, "\n")
	actualLines := strings.Split(actual, "\n")

	// Build maps for comparison
	expectedSet := make(map[string]bool)
	actualSet := make(map[string]bool)

	for _, line := range expectedLines {
		expectedSet[strings.TrimSpace(line)] = true
	}
	for _, line := range actualLines {
		actualSet[strings.TrimSpace(line)] = true
	}

	// Find lines only in expected (removed/missing)
	for _, line := range expectedLines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !actualSet[trimmed] {
			changes = append(changes, "- "+trimmed)
		}
	}

	// Find lines only in actual (added/extra)
	for _, line := range actualLines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !expectedSet[trimmed] {
			changes = append(changes, "+ "+trimmed)
		}
	}

	return changes
}

// assertRoundtripSuccess is a convenience method that asserts roundtrip passes.
func (e *testEnv) assertRoundtripSuccess(createMDL string, opts ...RoundtripOption) {
	e.t.Helper()

	result := e.assertRoundtrip(createMDL, opts...)
	if !result.Success {
		e.t.Errorf("Roundtrip failed.\n\nDiff:\n%s\n\nStructural changes:\n%s",
			result.Diff, strings.Join(result.Changes, "\n"))
	} else {
		e.t.Logf("Roundtrip successful.\nActual output:\n%s", result.Actual)
	}
}

// assertContains verifies that the roundtrip output contains expected properties.
// This is useful when exact matching isn't possible but key properties must be present.
func (e *testEnv) assertContains(createMDL string, expectedProps []string, opts ...RoundtripOption) {
	e.t.Helper()

	result := e.assertRoundtrip(createMDL, opts...)

	var missing []string
	for _, prop := range expectedProps {
		if !strings.Contains(result.Actual, prop) {
			missing = append(missing, prop)
		}
	}

	if len(missing) > 0 {
		e.t.Errorf("Missing expected properties: %v\n\nActual output:\n%s",
			missing, result.Actual)
	} else {
		e.t.Logf("Roundtrip contains all expected properties.\nActual output:\n%s", result.Actual)
	}
}

// --- Legacy Semantic Comparison Helpers (kept for backward compatibility) ---

// containsProperty checks if the MDL output contains a property.
func containsProperty(mdl, property string) bool {
	return strings.Contains(mdl, property)
}
