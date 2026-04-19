// SPDX-License-Identifier: Apache-2.0

// Example: Creating a Page with DataGrid2 using MDL
//
// This example demonstrates how to create a page with a DataGrid2 widget
// using the MDL (Mendix Definition Language) approach. DataGrid2 is a
// modern pluggable widget that replaces the deprecated DataGrid.
//
// Why MDL for DataGrid2?
// DataGrid2 is a complex pluggable widget with 50+ PropertyTypes that requires
// special template handling. The MDL executor uses embedded templates extracted
// from Studio Pro to ensure all property values are correctly initialized.
// This approach is recommended over trying to build the widget structure manually.
//
// There are two ways to create pages with DataGrid2:
// 1. Using MDL via the mxcli command line (recommended for scripts)
// 2. Using MDL programmatically via the executor (shown here)
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/backend"
	mprbackend "github.com/mendixlabs/mxcli/mdl/backend/mpr"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: create_datagrid2_page <path-to-mpr-file>")
		fmt.Println()
		fmt.Println("This example creates a Product Overview page with a DataGrid2 widget.")
		fmt.Println("WARNING: This will modify the MPR file! Make a backup first.")
		fmt.Println()
		fmt.Println("Prerequisites:")
		fmt.Println("  - A module named 'MyModule' (or modify the code)")
		fmt.Println("  - An entity named 'MyModule.Product' with attributes:")
		fmt.Println("    - Name (String)")
		fmt.Println("    - Code (String)")
		fmt.Println("    - Price (Decimal)")
		os.Exit(1)
	}

	mprPath := os.Args[1]

	// =========================================================================
	// Method 1: Using MDL programmatically via the executor
	// =========================================================================
	fmt.Println("Creating page with DataGrid2 using MDL executor...")

	// Create the MDL executor with stdout for output
	exec := executor.New(os.Stdout)
	exec.SetBackendFactory(func() backend.FullBackend { return mprbackend.New() })

	// Define the MDL script to create a page with DataGrid2
	// Note: Adjust module name, entity name, and attributes to match your project
	// The script includes CONNECT to open the project
	mdlScript := fmt.Sprintf(`
-- Connect to the Mendix project
CONNECT '%s';

-- Create a Product Overview page with DataGrid2
CREATE PAGE MyModule.Product_Overview
TITLE 'Product Overview'
LAYOUT Atlas_Core.Atlas_Default
BEGIN
  DATAGRID ProductGrid
    SOURCE DATABASE MyModule.Product
  BEGIN
    COLUMN Name AS 'Product Name';
    COLUMN Code AS 'SKU';
    COLUMN Price AS 'Unit Price';
  END
END;

-- Disconnect when done
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

	// Execute the MDL program
	fmt.Println("\nExecuting MDL:")
	fmt.Println(strings.TrimSpace(mdlScript))
	fmt.Println()

	err := exec.ExecuteProgram(prog)
	if err != nil {
		fmt.Printf("Error executing MDL: %v\n", err)
		fmt.Println("\nTip: Make sure the module and entity exist in your project.")
		os.Exit(1)
	}

	fmt.Println("Page created successfully!")

	// =========================================================================
	// Method 2: Using mxcli command line (alternative approach)
	// =========================================================================
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Alternative: Using mxcli command line")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()
	fmt.Println("You can also create the same page using mxcli:")
	fmt.Println()
	fmt.Printf("  mxcli -p %s -c \"\n", mprPath)
	fmt.Println("  CREATE PAGE MyModule.Product_Overview")
	fmt.Println("  TITLE 'Product Overview'")
	fmt.Println("  LAYOUT Atlas_Core.Atlas_Default")
	fmt.Println("  BEGIN")
	fmt.Println("    DATAGRID ProductGrid")
	fmt.Println("      SOURCE DATABASE MyModule.Product")
	fmt.Println("    BEGIN")
	fmt.Println("      COLUMN Name AS 'Product Name';")
	fmt.Println("      COLUMN Code AS 'SKU';")
	fmt.Println("      COLUMN Price AS 'Unit Price';")
	fmt.Println("    END")
	fmt.Println("  END;")
	fmt.Println("  \"")

	// =========================================================================
	// Verifying the page
	// =========================================================================
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Verification")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()
	fmt.Println("To verify the page was created correctly:")
	fmt.Println()
	fmt.Println("1. Check for errors:")
	fmt.Printf("   mx check %s\n", mprPath)
	fmt.Println()
	fmt.Println("2. View the page definition:")
	fmt.Printf("   mxcli -p %s -c \"DESCRIBE PAGE MyModule.Product_Overview\"\n", mprPath)
	fmt.Println()
	fmt.Println("3. Open in Mendix Studio Pro to verify visually")
}
