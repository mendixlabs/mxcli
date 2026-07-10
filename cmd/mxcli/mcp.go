// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	mcpbackend "github.com/JordtenBulte-OLC/mxcli/mdl/backend/mcp"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Inspect the live Studio Pro MCP backend",
	Long: `Commands for the MCP backend, which routes model writes to a running
Studio Pro via its embedded MCP server (see --mcp).`,
}

var mcpCapabilitiesCmd = &cobra.Command{
	Use:   "capabilities",
	Short: "Report what the connected Studio Pro can author over MCP",
	Long: `Connect to the Studio Pro MCP server (--mcp) and print what the MCP backend
can and cannot author against that server's version — so an agent knows, before
generating MDL, which operations are possible. Reads come from -p (the same local
.mpr Studio Pro has open).

Example:
  mxcli mcp capabilities -p app.mpr --mcp http://localhost/mcp --mcp-dial host.docker.internal:7784`,
	Run: func(cmd *cobra.Command, _ []string) {
		if globalMCPURL == "" {
			fmt.Fprintln(os.Stderr, "Error: mcp capabilities requires --mcp <url> (the connected Studio Pro MCP server)")
			os.Exit(1)
		}
		projectPath, _ := cmd.Flags().GetString("project")
		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: mcp capabilities requires -p <project.mpr> (reads come from the local project)")
			os.Exit(1)
		}
		b := mcpbackend.New(globalMCPURL, globalMCPDial)
		if globalMCPConcord != "" {
			b = b.WithConcord(mcpbackend.ConcordConfig{URL: globalMCPConcord, Dial: globalMCPConcordDial})
		}
		if err := b.Connect(projectPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = b.Disconnect() }()
		fmt.Print(b.CapabilityReport())
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
	mcpCmd.AddCommand(mcpCapabilitiesCmd)
}
