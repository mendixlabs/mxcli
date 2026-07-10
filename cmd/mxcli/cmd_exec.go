// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/JordtenBulte-OLC/mxcli/mdl/executor"
	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec <file>",
	Short: "Execute an MDL script file",
	Long: `Execute an MDL script file containing MDL commands.

Example:
  mxcli exec setup.mdl
  mxcli exec -p app.mpr script.mdl
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filePath := args[0]
		projectPath, _ := cmd.Flags().GetString("project")

		// Read the file
		content, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
			os.Exit(1)
		}

		exec, logger := newLoggedExecutor("exec")
		defer logger.Close()
		defer exec.Close()

		// Auto-connect if project specified
		if projectPath != "" {
			connectCmd := fmt.Sprintf("CONNECT LOCAL '%s';", visitor.QuoteString(projectPath))
			prog, _ := visitor.Build(connectCmd)
			for _, stmt := range prog.Statements {
				if err := exec.Execute(stmt); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
			}
		}

		// Parse and execute the file
		prog, errs := visitor.Build(string(content))
		if len(errs) > 0 {
			for _, err := range errs {
				fmt.Fprintf(os.Stderr, "Parse error: %v\n", err)
			}
			os.Exit(1)
		}

		if err := exec.ExecuteProgram(prog); err != nil {
			if errors.Is(err, executor.ErrExit) {
				return
			}
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}
