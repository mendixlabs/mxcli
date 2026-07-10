// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
)

// --- Security Roundtrip Tests ---

// securityTestModule uses MyFirstModule which has module security.
const securityTestModule = "MyFirstModule"

// securityTestEnv wraps testEnv with module role setup/teardown.
type securityTestEnv struct {
	*testEnv
	roles []string // qualified role names to clean up
}

// setupSecurityTestEnv creates module roles for security tests.
func setupSecurityTestEnv(t *testing.T) *securityTestEnv {
	t.Helper()
	env := setupTestEnv(t)
	senv := &securityTestEnv{testEnv: env}

	// Create two module roles for testing
	for _, role := range []string{"SecTestAdmin", "SecTestViewer"} {
		mdl := "create module role " + securityTestModule + "." + role + ";"
		if err := env.executeMDL(mdl); err != nil {
			// Ignore if already exists
			if !strings.Contains(err.Error(), "already exists") {
				t.Fatalf("Failed to create module role %s: %v", role, err)
			}
		}
		senv.roles = append(senv.roles, securityTestModule+"."+role)
	}

	return senv
}

// teardown disconnects from the project. No cleanup needed since each test
// uses a fresh project copy that is automatically deleted.
func (s *securityTestEnv) teardown() {
	s.testEnv.teardown()
}

func TestRoundtripSecurity_EntityAccessGrant(t *testing.T) {
	env := setupSecurityTestEnv(t)
	defer env.teardown()

	entityName := securityTestModule + ".SecTestEntity"
	env.registerCleanup("entity", entityName)

	// Create entity
	createMDL := `create persistent entity ` + entityName + ` (
		Name: String(100),
		Email: String(200)
	);`
	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	// Grant access
	grantMDL := `grant ` + securityTestModule + `.SecTestAdmin on ` + entityName + ` (create, delete, read *, write *);`
	if err := env.executeMDL(grantMDL); err != nil {
		t.Fatalf("Failed to grant entity access: %v", err)
	}

	// Describe and verify
	output, err := env.describeMDL(`describe entity ` + entityName + `;`)
	if err != nil {
		t.Fatalf("Failed to describe entity: %v", err)
	}

	// Verify GRANT line is present
	if !strings.Contains(output, "grant") {
		t.Errorf("Expected grant statement in describe output.\nActual:\n%s", output)
	}
	if !strings.Contains(output, securityTestModule+".SecTestAdmin") {
		t.Errorf("Expected role %s.SecTestAdmin in grant statement.\nActual:\n%s", securityTestModule, output)
	}
	if !strings.Contains(output, "create") || !strings.Contains(output, "delete") {
		t.Errorf("Expected create, delete in grant rights.\nActual:\n%s", output)
	}
	if !strings.Contains(output, "read *") || !strings.Contains(output, "write *") {
		t.Errorf("Expected read * and write * in grant rights.\nActual:\n%s", output)
	}

	t.Logf("Entity security roundtrip successful:\n%s", output)

	// Round-trip: parse the DESCRIBE output and verify it's valid MDL
	_, parseErrs := visitor.Build(output)
	if len(parseErrs) > 0 {
		t.Errorf("describe output is not valid MDL: %v\nOutput:\n%s", parseErrs[0], output)
	}
}

func TestRoundtripSecurity_EntityAccessMultipleRoles(t *testing.T) {
	env := setupSecurityTestEnv(t)
	defer env.teardown()

	entityName := securityTestModule + ".SecTestMultiRole"
	env.registerCleanup("entity", entityName)

	// Create entity
	createMDL := `create persistent entity ` + entityName + ` (
		DisplayName: String(200)
	);`
	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	// Grant full access to Admin
	grantAdmin := `grant ` + securityTestModule + `.SecTestAdmin on ` + entityName + ` (create, delete, read *, write *);`
	if err := env.executeMDL(grantAdmin); err != nil {
		t.Fatalf("Failed to grant Admin access: %v", err)
	}

	// Grant read-only access to Viewer
	grantViewer := `grant ` + securityTestModule + `.SecTestViewer on ` + entityName + ` (read *);`
	if err := env.executeMDL(grantViewer); err != nil {
		t.Fatalf("Failed to grant Viewer access: %v", err)
	}

	// Describe and verify
	output, err := env.describeMDL(`describe entity ` + entityName + `;`)
	if err != nil {
		t.Fatalf("Failed to describe entity: %v", err)
	}

	// Should contain two GRANT statements
	grantCount := strings.Count(output, "grant")
	if grantCount < 2 {
		t.Errorf("Expected at least 2 grant statements, got %d.\nOutput:\n%s", grantCount, output)
	}
	if !strings.Contains(output, securityTestModule+".SecTestAdmin") {
		t.Errorf("Expected SecTestAdmin role in output.\nActual:\n%s", output)
	}
	if !strings.Contains(output, securityTestModule+".SecTestViewer") {
		t.Errorf("Expected SecTestViewer role in output.\nActual:\n%s", output)
	}

	t.Logf("Entity multi-role security roundtrip successful:\n%s", output)

	// Round-trip parse check
	_, parseErrs := visitor.Build(output)
	if len(parseErrs) > 0 {
		t.Errorf("describe output is not valid MDL: %v\nOutput:\n%s", parseErrs[0], output)
	}
}

func TestRoundtripSecurity_EntityAccessWithXPath(t *testing.T) {
	env := setupSecurityTestEnv(t)
	defer env.teardown()

	entityName := securityTestModule + ".SecTestXPath"
	env.registerCleanup("entity", entityName)

	// Create entity. OwnerName (not Owner) — Owner is reserved on persistent
	// entities (CE7247 / MDL020): it is auto-managed by Mendix via AutoOwner.
	createMDL := `create persistent entity ` + entityName + ` (
		OwnerName: String(100)
	);`
	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	// Grant with XPath constraint referencing the entity attribute.
	grantMDL := `grant ` + securityTestModule + `.SecTestViewer on ` + entityName + ` (read *) where '[%CurrentUser%] = OwnerName';`
	if err := env.executeMDL(grantMDL); err != nil {
		t.Fatalf("Failed to grant entity access with XPath: %v", err)
	}

	// Describe and verify
	output, err := env.describeMDL(`describe entity ` + entityName + `;`)
	if err != nil {
		t.Fatalf("Failed to describe entity: %v", err)
	}

	if !strings.Contains(output, "where") {
		t.Errorf("Expected where clause in grant statement.\nActual:\n%s", output)
	}
	if !strings.Contains(output, "[%CurrentUser%] = OwnerName") {
		t.Errorf("Expected XPath constraint in output.\nActual:\n%s", output)
	}

	t.Logf("Entity XPath security roundtrip successful:\n%s", output)

	// Round-trip parse check
	_, parseErrs := visitor.Build(output)
	if len(parseErrs) > 0 {
		t.Errorf("describe output is not valid MDL: %v\nOutput:\n%s", parseErrs[0], output)
	}
}

func TestRoundtripSecurity_EntityNoAccess(t *testing.T) {
	env := setupSecurityTestEnv(t)
	defer env.teardown()

	entityName := securityTestModule + ".SecTestNoAccess"
	env.registerCleanup("entity", entityName)

	// Create entity without granting any access
	createMDL := `create persistent entity ` + entityName + ` (
		Value: Integer
	);`
	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	// Describe — should NOT contain any GRANT
	output, err := env.describeMDL(`describe entity ` + entityName + `;`)
	if err != nil {
		t.Fatalf("Failed to describe entity: %v", err)
	}

	if strings.Contains(output, "grant") {
		t.Errorf("Expected NO grant statement for entity without access rules.\nActual:\n%s", output)
	}

	t.Logf("Entity no-access roundtrip successful:\n%s", output)
}

func TestRoundtripSecurity_MicroflowAccess(t *testing.T) {
	env := setupSecurityTestEnv(t)
	defer env.teardown()

	mfName := securityTestModule + ".SecTestMicroflow"
	env.registerCleanup("microflow", mfName)

	// Create microflow
	createMDL := `create microflow ` + mfName + ` ()
	begin
		log info 'test';
	end;`
	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("Failed to create microflow: %v", err)
	}

	// Grant execute access to both roles
	grantMDL := `grant execute on microflow ` + mfName + ` to ` + securityTestModule + `.SecTestAdmin, ` + securityTestModule + `.SecTestViewer;`
	if err := env.executeMDL(grantMDL); err != nil {
		t.Fatalf("Failed to grant microflow access: %v", err)
	}

	// Describe and verify
	output, err := env.describeMDL(`describe microflow ` + mfName + `;`)
	if err != nil {
		t.Fatalf("Failed to describe microflow: %v", err)
	}

	if !strings.Contains(output, "grant execute on microflow") {
		t.Errorf("Expected grant execute statement.\nActual:\n%s", output)
	}
	if !strings.Contains(output, securityTestModule+".SecTestAdmin") {
		t.Errorf("Expected SecTestAdmin role in output.\nActual:\n%s", output)
	}
	if !strings.Contains(output, securityTestModule+".SecTestViewer") {
		t.Errorf("Expected SecTestViewer role in output.\nActual:\n%s", output)
	}

	t.Logf("Microflow security roundtrip successful:\n%s", output)

	// Round-trip parse check
	_, parseErrs := visitor.Build(output)
	if len(parseErrs) > 0 {
		t.Errorf("describe output is not valid MDL: %v\nOutput:\n%s", parseErrs[0], output)
	}
}

func TestRoundtripSecurity_MicroflowNoAccess(t *testing.T) {
	env := setupSecurityTestEnv(t)
	defer env.teardown()

	mfName := securityTestModule + ".SecTestMfNoAccess"
	env.registerCleanup("microflow", mfName)

	// Create microflow without granting access
	createMDL := `create microflow ` + mfName + ` ()
	begin
		log info 'test';
	end;`
	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("Failed to create microflow: %v", err)
	}

	// Describe — should NOT contain GRANT
	output, err := env.describeMDL(`describe microflow ` + mfName + `;`)
	if err != nil {
		t.Fatalf("Failed to describe microflow: %v", err)
	}

	if strings.Contains(output, "grant") {
		t.Errorf("Expected NO grant statement for microflow without access.\nActual:\n%s", output)
	}

	t.Logf("Microflow no-access roundtrip successful:\n%s", output)
}

func TestRoundtripSecurity_PageAccess(t *testing.T) {
	env := setupSecurityTestEnv(t)
	defer env.teardown()

	pageName := securityTestModule + ".SecTestPage"
	env.registerCleanup("page", pageName)

	// Create page
	createMDL := `create page ` + pageName + ` (Title: 'Security Test', Layout: Atlas_Core.Atlas_Default) {
		container container1 {
			dynamictext text1 (Content: 'Hello')
		}
	}`
	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("Failed to create page: %v", err)
	}

	// Grant view access
	grantMDL := `grant view on page ` + pageName + ` to ` + securityTestModule + `.SecTestAdmin;`
	if err := env.executeMDL(grantMDL); err != nil {
		t.Fatalf("Failed to grant page access: %v", err)
	}

	// Describe and verify
	output, err := env.describeMDL(`describe page ` + pageName + `;`)
	if err != nil {
		t.Fatalf("Failed to describe page: %v", err)
	}

	if !strings.Contains(output, "grant view on page") {
		t.Errorf("Expected grant view statement.\nActual:\n%s", output)
	}
	if !strings.Contains(output, securityTestModule+".SecTestAdmin") {
		t.Errorf("Expected SecTestAdmin role in output.\nActual:\n%s", output)
	}

	t.Logf("Page security roundtrip successful:\n%s", output)

	// Round-trip parse check
	_, parseErrs := visitor.Build(output)
	if len(parseErrs) > 0 {
		t.Errorf("describe output is not valid MDL: %v\nOutput:\n%s", parseErrs[0], output)
	}
}

func TestRoundtripSecurity_PageNoAccess(t *testing.T) {
	env := setupSecurityTestEnv(t)
	defer env.teardown()

	pageName := securityTestModule + ".SecTestPageNoAccess"
	env.registerCleanup("page", pageName)

	// Create page without granting access
	createMDL := `create page ` + pageName + ` (Title: 'No Access', Layout: Atlas_Core.Atlas_Default) {
		container container1 {
			dynamictext text1 (Content: 'Hello')
		}
	}`
	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("Failed to create page: %v", err)
	}

	// Describe — should NOT contain GRANT
	output, err := env.describeMDL(`describe page ` + pageName + `;`)
	if err != nil {
		t.Fatalf("Failed to describe page: %v", err)
	}

	if strings.Contains(output, "grant") {
		t.Errorf("Expected NO grant statement for page without access.\nActual:\n%s", output)
	}

	t.Logf("Page no-access roundtrip successful:\n%s", output)
}
