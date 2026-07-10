// SPDX-License-Identifier: Apache-2.0

// Example: Creating Nanoflows using MDL
//
// This example demonstrates how to create nanoflows programmatically
// using the MDL (Mendix Definition Language) executor. Nanoflows run
// client-side and support JavaScript action calls, show page/message
// actions, and other client-compatible activities.
//
// There are two ways to create nanoflows:
// 1. Using MDL via the mxcli command line (recommended for scripts)
// 2. Using MDL programmatically via the executor (shown here)
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	mprbackend "github.com/JordtenBulte-OLC/mxcli/mdl/backend/mpr"
	"github.com/JordtenBulte-OLC/mxcli/mdl/executor"
	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: create_nanoflow <path-to-mpr-file>")
		fmt.Println()
		fmt.Println("This example creates several nanoflows demonstrating different patterns.")
		fmt.Println("WARNING: This will modify the MPR file! Make a backup first.")
		fmt.Println()
		fmt.Println("Prerequisites:")
		fmt.Println("  - A module named 'MyModule' (or modify the code)")
		os.Exit(1)
	}

	mprPath := os.Args[1]

	// Create the MDL executor with stdout for output
	exec := executor.New(os.Stdout)
	exec.SetBackendFactory(func() backend.FullBackend { return mprbackend.New() })

	// Define MDL script with several nanoflow examples
	mdlScript := fmt.Sprintf(`
-- Connect to the Mendix project
CONNECT '%s';

-- 1. Minimal nanoflow (no parameters, no return)
create nanoflow MyModule.NF_HelloWorld ()
begin
  show message 'Hello from a nanoflow!';
end;

-- 2. Nanoflow with parameters and return type
create nanoflow MyModule.NF_IsValidInput (Input : String) returns Boolean
begin
  if length($Input) > 0 then
    return true;
  else
    return false;
  end if;
end;

-- 3. Nanoflow calling a JavaScript action
create nanoflow MyModule.NF_CheckConnectivity () returns Boolean
begin
  $IsOnline = call javascript action NanoflowCommons.HasConnectivity ();
  return $IsOnline;
end;

-- 4. Nanoflow calling another nanoflow
create nanoflow MyModule.NF_ValidateAndCheck (Input : String) returns Boolean
begin
  $IsValid = call nanoflow MyModule.NF_IsValidInput (Input = $Input);
  if $IsValid then
    $IsOnline = call nanoflow MyModule.NF_CheckConnectivity ();
    return $IsOnline;
  else
    return false;
  end if;
end;

-- 5. Grant access to a role
grant execute on nanoflow MyModule.NF_HelloWorld to MyModule.User;
grant execute on nanoflow MyModule.NF_ValidateAndCheck to MyModule.User;

-- Verify: list all nanoflows in the module
show nanoflows in MyModule;

-- Verify: describe a nanoflow
describe nanoflow MyModule.NF_ValidateAndCheck;

DISCONNECT;
`, mprPath)

	// Parse the MDL script
	fmt.Println("Parsing MDL script...")
	prog, errs := visitor.Build(mdlScript)
	if len(errs) > 0 {
		fmt.Printf("Parse errors:\n")
		for _, err := range errs {
			fmt.Printf("  - %v\n", err)
		}
		os.Exit(1)
	}

	// Execute
	fmt.Println("\nExecuting MDL:")
	fmt.Println(strings.TrimSpace(mdlScript))
	fmt.Println()

	err := exec.ExecuteProgram(prog)
	if err != nil {
		fmt.Printf("Error executing MDL: %v\n", err)
		fmt.Println("\nTip: Make sure the module exists in your project.")
		os.Exit(1)
	}

	fmt.Println("\nNanoflows created successfully!")

	// =========================================================================
	// Alternative: Using mxcli command line
	// =========================================================================
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Alternative: Using mxcli command line")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()
	fmt.Printf("  echo 'create nanoflow MyModule.NF_Test () begin show message '\"'\"'Hello!'\"'\"'; end;' | mxcli exec -p %s /dev/stdin\n", mprPath)
	fmt.Println()
	fmt.Println("Or with a script file:")
	fmt.Println()
	fmt.Printf("  mxcli exec -p %s nanoflows.mdl\n", mprPath)
}
