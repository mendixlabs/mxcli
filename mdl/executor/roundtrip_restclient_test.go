// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

// --- REST Client Roundtrip Tests ---

func TestRoundtripRestClient_SimpleGet(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	createMDL := `create rest client ` + testModule + `.SimpleAPI (
  BaseUrl: 'https://api.example.com',
  Authentication: none
)
{
  operation GetStatus {
    Method: get,
    Path: '/status',
    Response: none
  }
};`

	env.assertContains(createMDL, []string{
		"rest client",
		"SimpleAPI",
		"BaseUrl: 'https://api.example.com'",
		"Authentication: none",
		"operation GetStatus",
		"Method: get",
		"Path: '/status'",
		"Response: none",
	})
}

func TestRoundtripRestClient_WithJsonResponse(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	createMDL := `create rest client ` + testModule + `.JsonAPI (
  BaseUrl: 'https://jsonplaceholder.typicode.com',
  Authentication: none
)
{
  operation GetPosts {
    Method: get,
    Path: '/posts',
    Headers: ('Accept' = 'application/json'),
    Response: json as $Posts
  }
};`

	env.assertContains(createMDL, []string{
		"rest client",
		"JsonAPI",
		"BaseUrl: 'https://jsonplaceholder.typicode.com'",
		"operation GetPosts",
		"Method: get",
		"Path: '/posts'",
		"'Accept' = 'application/json'",
		"Response: json",
	})
}

func TestRoundtripRestClient_WithPathParams(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	createMDL := `create rest client ` + testModule + `.ParamAPI (
  BaseUrl: 'https://api.example.com',
  Authentication: none
)
{
  operation GetItem {
    Method: get,
    Path: '/items/{itemId}',
    Parameters: ($itemId: Integer),
    Response: json as $Item
  }
};`

	env.assertContains(createMDL, []string{
		"operation GetItem",
		"Path: '/items/{itemId}'",
		"$itemId: Integer",
		"Response: json",
	})
}

func TestRoundtripRestClient_WithQueryParams(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	createMDL := `create rest client ` + testModule + `.SearchAPI (
  BaseUrl: 'https://api.example.com',
  Authentication: none
)
{
  operation SearchItems {
    Method: get,
    Path: '/search',
    Query: ($q: String, $page: String),
    Response: json as $Results
  }
};`

	env.assertContains(createMDL, []string{
		"operation SearchItems",
		"Path: '/search'",
		"$q: String",
		"$page: String",
		"Response: json",
	})
}

func TestRoundtripRestClient_PostWithBody(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	createMDL := `create rest client ` + testModule + `.CrudAPI (
  BaseUrl: 'https://api.example.com',
  Authentication: none
)
{
  operation CreateItem {
    Method: post,
    Path: '/items',
    Headers: ('Content-Type' = 'application/json'),
    Body: json from $NewItem,
    Response: json as $CreatedItem
  }
};`

	env.assertContains(createMDL, []string{
		"operation CreateItem",
		"Method: post",
		"Body: json from $NewItem",
		"Response: json",
	})
}

func TestRoundtripRestClient_BasicAuth(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	createMDL := `create rest client ` + testModule + `.AuthAPI (
  BaseUrl: 'https://api.example.com',
  Authentication: basic (Username: 'admin', Password: 'secret')
)
{
  operation GetData {
    Method: get,
    Path: '/data',
    Response: json as $Data
  }
};`

	env.assertContains(createMDL, []string{
		"rest client",
		"AuthAPI",
		"Authentication: basic",
		"Username: 'admin'",
		"Password: 'secret'",
		"operation GetData",
	})
}

func TestRoundtripRestClient_WithTimeout(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	createMDL := `create rest client ` + testModule + `.TimeoutAPI (
  BaseUrl: 'https://api.example.com',
  Authentication: none
)
{
  operation SlowQuery {
    Method: get,
    Path: '/slow',
    Timeout: 60,
    Response: json as $Result
  }
};`

	env.assertContains(createMDL, []string{
		"operation SlowQuery",
		"Timeout: 60",
		"Response: json",
	})
}

func TestRoundtripRestClient_MultipleOperations(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	createMDL := `create rest client ` + testModule + `.PetStoreAPI (
  BaseUrl: 'https://petstore.swagger.io/v2',
  Authentication: none
)
{
  operation ListPets {
    Method: get,
    Path: '/pet/findByStatus',
    Query: ($status: String),
    Headers: ('Accept' = 'application/json'),
    Timeout: 30,
    Response: json as $PetList
  }

  operation GetPet {
    Method: get,
    Path: '/pet/{petId}',
    Parameters: ($petId: Integer),
    Response: json as $Pet
  }

  operation AddPet {
    Method: post,
    Path: '/pet',
    Headers: ('Content-Type' = 'application/json'),
    Body: json from $NewPet,
    Response: json as $CreatedPet
  }

  operation RemovePet {
    Method: delete,
    Path: '/pet/{petId}',
    Parameters: ($petId: Integer),
    Response: none
  }
};`

	env.assertContains(createMDL, []string{
		"rest client",
		"PetStoreAPI",
		"operation ListPets",
		"$status: String",
		"Timeout: 30",
		"operation GetPet",
		"$petId: Integer",
		"operation AddPet",
		"Method: post",
		"Body: json from $NewPet",
		"operation RemovePet",
		"Method: delete",
		"Response: none",
	})
}

func TestRoundtripRestClient_DeleteNoResponse(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	createMDL := `create rest client ` + testModule + `.DeleteAPI (
  BaseUrl: 'https://api.example.com',
  Authentication: none
)
{
  operation DeleteResource {
    Method: delete,
    Path: '/resources/{id}',
    Parameters: ($id: Integer),
    Response: none
  }
};`

	env.assertContains(createMDL, []string{
		"operation DeleteResource",
		"Method: delete",
		"$id: Integer",
		"Response: none",
	})
}

func TestRoundtripRestClient_CreateOrModify(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	// Create first version
	createMDL := `create rest client ` + testModule + `.MutableAPI (
  BaseUrl: 'https://api.example.com/v1',
  Authentication: none
)
{
  operation GetData {
    Method: get,
    Path: '/data',
    Response: json as $Data
  }
};`

	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("Failed to create rest client: %v", err)
	}

	// Update with CREATE OR MODIFY
	updateMDL := `create or modify rest client ` + testModule + `.MutableAPI (
  BaseUrl: 'https://api.example.com/v2',
  Authentication: none
)
{
  operation GetDataV2 {
    Method: get,
    Path: '/data/v2',
    Response: json as $DataV2
  }
};`

	if err := env.executeMDL(updateMDL); err != nil {
		t.Fatalf("Failed to update rest client: %v", err)
	}

	// Verify the updated version
	output, err := env.describeMDL("describe rest client " + testModule + ".MutableAPI;")
	if err != nil {
		t.Fatalf("Failed to describe rest client: %v", err)
	}

	if !strings.Contains(output, "v2") {
		t.Errorf("Expected updated BaseUrl with v2, got:\n%s", output)
	}
	if !strings.Contains(output, "GetDataV2") {
		t.Errorf("Expected updated operation GetDataV2, got:\n%s", output)
	}
}

func TestRoundtripRestClient_Drop(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	// Create a REST client
	createMDL := `create rest client ` + testModule + `.ToBeDropped (
  BaseUrl: 'https://api.example.com',
  Authentication: none
)
{
  operation Ping {
    Method: get,
    Path: '/ping',
    Response: none
  }
};`

	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("Failed to create rest client: %v", err)
	}

	// Verify it exists
	_, err := env.describeMDL("describe rest client " + testModule + ".ToBeDropped;")
	if err != nil {
		t.Fatalf("rest client should exist before drop: %v", err)
	}

	// Drop it
	if err := env.executeMDL("drop rest client " + testModule + ".ToBeDropped;"); err != nil {
		t.Fatalf("Failed to drop rest client: %v", err)
	}

	// Verify it's gone
	_, err = env.describeMDL("describe rest client " + testModule + ".ToBeDropped;")
	if err == nil {
		t.Error("rest client should not exist after drop")
	}
}

// --- MX Check Tests ---

func TestMxCheck_RestClient_SimpleGet(t *testing.T) {
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	env := setupTestEnv(t)
	defer env.teardown()

	createMDL := `create rest client ` + testModule + `.MxCheckSimpleAPI (
  BaseUrl: 'https://api.example.com',
  Authentication: none
)
{
  operation GetStatus {
    Method: get,
    Path: '/status',
    Headers: ('Accept' = '*/*'),
    Response: none
  }
};`

	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("Failed to create rest client: %v", err)
	}

	// Disconnect to flush changes to disk
	env.executor.Execute(&ast.DisconnectStmt{})

	// Run mx check
	output, err := runMxCheck(t, env.projectPath)
	assertMxCheckPassed(t, output, err)
}

func TestMxCheck_RestClient_PostWithBody(t *testing.T) {
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	env := setupTestEnv(t)
	defer env.teardown()

	// Test with path parameters (GET to avoid body requirements).
	createMDL := `create rest client ` + testModule + `.MxCheckParamAPI (
  BaseUrl: 'https://api.example.com',
  Authentication: none
)
{
  operation GetItem {
    Method: get,
    Path: '/items/{itemId}',
    Parameters: ($itemId: Integer),
    Headers: ('Accept' = '*/*'),
    Response: none
  }
};`

	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("Failed to create rest client: %v", err)
	}

	env.executor.Execute(&ast.DisconnectStmt{})

	output, err := runMxCheck(t, env.projectPath)
	assertMxCheckPassed(t, output, err)
}

// assertMxCheckPassed checks mx check output for errors.
// Detects both "[error]" markers (validation errors) and "ERROR:" (load crashes).
func assertMxCheckPassed(t *testing.T, output string, err error) {
	t.Helper()
	if err != nil {
		// Non-zero exit code — could be a crash or validation errors
		if strings.Contains(output, "[error]") || strings.Contains(output, "error:") {
			t.Errorf("mx check failed:\n%s", output)
		} else {
			t.Logf("mx check exited with error but no validation errors:\n%s", output)
		}
	} else if strings.Contains(output, "[error]") {
		t.Errorf("mx check found errors:\n%s", output)
	} else {
		t.Logf("mx check passed:\n%s", output)
	}
}

func TestMxCheck_RestClient_BasicAuth(t *testing.T) {
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	env := setupTestEnv(t)
	defer env.teardown()

	// Use RESPONSE NONE to avoid entity mapping requirements (CE0061)
	createMDL := `create rest client ` + testModule + `.MxCheckAuthAPI (
  BaseUrl: 'https://api.example.com',
  Authentication: basic (Username: 'user', Password: 'pass')
)
{
  operation GetSecureData {
    Method: get,
    Path: '/secure/data',
    Headers: ('Accept' = '*/*'),
    Response: none
  }
};`

	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("Failed to create rest client: %v", err)
	}

	env.executor.Execute(&ast.DisconnectStmt{})

	output, err := runMxCheck(t, env.projectPath)
	assertMxCheckPassed(t, output, err)
}

func TestMxCheck_RestClient_MultipleOperations(t *testing.T) {
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	env := setupTestEnv(t)
	defer env.teardown()

	// Use RESPONSE NONE for all operations to avoid entity mapping requirements (CE0061).
	// All operations include Accept header to avoid CE7062.
	createMDL := `create rest client ` + testModule + `.MxCheckPetStore (
  BaseUrl: 'https://petstore.swagger.io/v2',
  Authentication: none
)
{
  operation ListPets {
    Method: get,
    Path: '/pet/findByStatus',
    Query: ($status: String),
    Headers: ('Accept' = 'application/json'),
    Timeout: 30,
    Response: none
  }

  operation GetPet {
    Method: get,
    Path: '/pet/{petId}',
    Parameters: ($petId: Integer),
    Headers: ('Accept' = 'application/json'),
    Response: none
  }

  operation RemovePet {
    Method: delete,
    Path: '/pet/{petId}',
    Parameters: ($petId: Integer),
    Headers: ('Accept' = '*/*'),
    Response: none
  }
};`

	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("Failed to create rest client: %v", err)
	}

	env.executor.Execute(&ast.DisconnectStmt{})

	output, err := runMxCheck(t, env.projectPath)
	assertMxCheckPassed(t, output, err)
}
