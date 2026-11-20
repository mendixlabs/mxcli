// SPDX-License-Identifier: Apache-2.0

// cmd_add_tool.go - Add AI tool support to existing project
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var addToolCmd = &cobra.Command{
	Use:   "add-tool <tool-name> [project-directory]",
	Short: "Add AI tool support to an existing project",
	Long: `Add configuration for an AI coding assistant to an existing Mendix project.

This command adds tool-specific configuration files without affecting existing setup.

Examples:
  # Add Cursor support to current directory
  mxcli add-tool cursor

  # Add Continue.dev support to specific project
  mxcli add-tool continue /path/to/project

  # List available tools
  mxcli add-tool --list

Supported Tools:
  - claude      Claude Code with skills and commands
  - cursor      Cursor AI with MDL rules
  - continue    Continue.dev with custom commands
  - windsurf    Windsurf (Codeium) with MDL rules
  - aider       Aider with project configuration
  - opencode    OpenCode AI agent with MDL commands and skills
  - vibe        Mistral Vibe CLI agent with skills
  - copilot     GitHub Copilot with project-level instructions
`,
	Args: cobra.RangeArgs(0, 2),
	Run: func(cmd *cobra.Command, args []string) {
		// List tools if requested
		if listTools, _ := cmd.Flags().GetBool("list"); listTools {
			fmt.Println("Supported AI Tools:")
			fmt.Println()
			for key, tool := range SupportedTools {
				fmt.Printf("  %-12s %s\n", key, tool.Description)
			}
			return
		}

		// Validate args
		if len(args) == 0 {
			fmt.Fprintf(os.Stderr, "Error: tool name required\n")
			fmt.Fprintf(os.Stderr, "Usage: mxcli add-tool <tool-name> [project-directory]\n")
			fmt.Fprintf(os.Stderr, "Run 'mxcli add-tool --list' to see available tools\n")
			os.Exit(1)
		}

		toolName := args[0]
		projectDir := "."
		if len(args) > 1 {
			projectDir = args[1]
		}

		// Validate tool name
		toolConfig, ok := SupportedTools[toolName]
		if !ok {
			fmt.Fprintf(os.Stderr, "Error: unknown tool '%s'\n", toolName)
			fmt.Fprintf(os.Stderr, "Run 'mxcli add-tool --list' to see available tools\n")
			os.Exit(1)
		}

		// Make path absolute
		absDir, err := filepath.Abs(projectDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving path: %v\n", err)
			os.Exit(1)
		}

		// Check if directory exists
		info, err := os.Stat(absDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: directory does not exist: %s\n", absDir)
			os.Exit(1)
		}
		if !info.IsDir() {
			fmt.Fprintf(os.Stderr, "Error: not a directory: %s\n", absDir)
			os.Exit(1)
		}

		// Find .mpr file
		mprFile := findMprFile(absDir)
		if mprFile == "" {
			mprFile = "project.mpr" // Default if not found
		}
		projectName := projectDisplayName(absDir, findMprFile(absDir))

		fmt.Printf("Adding %s support to: %s\n", toolConfig.Name, absDir)

		// Create tool-specific configuration files
		for _, file := range toolConfig.Files {
			filePath := filepath.Join(absDir, file.Path)

			// Check if file already exists
			if _, err := os.Stat(filePath); err == nil {
				fmt.Printf("  Skipping %s (already exists)\n", file.Path)
				continue
			}

			// Create parent directory if needed
			if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
				fmt.Fprintf(os.Stderr, "  Error creating directory for %s: %v\n", file.Path, err)
				continue
			}

			// Generate content
			content := file.render(absDir, projectName, mprFile)

			// Write file
			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "  Error writing %s: %v\n", file.Path, err)
				continue
			}

			fmt.Printf("  Created %s\n", file.Path)
		}

		// Ensure universal files exist
		aiContextDir := filepath.Join(absDir, ".ai-context")
		if _, err := os.Stat(aiContextDir); os.IsNotExist(err) {
			fmt.Println("\n  Note: .ai-context/ directory not found.")
			fmt.Println("  Run 'mxcli init' first to create universal documentation.")
		}

		// OpenCode sidecar: commands, skills, lint-rules (same as mxcli init)
		if toolName == "opencode" {
			opencodeDir := filepath.Join(absDir, ".opencode")
			opencodeCommandsDir := filepath.Join(opencodeDir, "commands")
			opencodeSkillsDir := filepath.Join(opencodeDir, "skills")
			lintRulesDir := filepath.Join(absDir, ".claude", "lint-rules")

			for _, dir := range []string{opencodeCommandsDir, opencodeSkillsDir, lintRulesDir} {
				if err := os.MkdirAll(dir, 0755); err != nil {
					fmt.Fprintf(os.Stderr, "  Error creating directory %s: %v\n", dir, err)
				}
			}

			cmdCount := 0
			if err := fs.WalkDir(commandsFS, "commands", func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return err
				}
				content, err := commandsFS.ReadFile(path)
				if err != nil {
					return err
				}
				targetPath := filepath.Join(opencodeCommandsDir, d.Name())
				if _, statErr := os.Stat(targetPath); statErr == nil {
					return nil // skip existing
				}
				if err := os.WriteFile(targetPath, content, 0644); err != nil {
					return err
				}
				cmdCount++
				return nil
			}); err != nil {
				fmt.Fprintf(os.Stderr, "  Error writing OpenCode commands: %v\n", err)
			} else if cmdCount > 0 {
				fmt.Printf("  Created %d command files in .opencode/commands/\n", cmdCount)
			}

			initBundledLintRules(lintRulesDir)

			skillCount := 0
			if err := fs.WalkDir(skillsFS, "skills", func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return err
				}
				if d.Name() == "README.md" {
					return nil
				}
				content, err := skillsFS.ReadFile(path)
				if err != nil {
					return err
				}
				skillName := strings.TrimSuffix(d.Name(), ".md")
				skillDir := filepath.Join(opencodeSkillsDir, skillName)
				if err := os.MkdirAll(skillDir, 0755); err != nil {
					return err
				}
				targetPath := filepath.Join(skillDir, "SKILL.md")
				if _, statErr := os.Stat(targetPath); statErr == nil {
					return nil // skip existing
				}
				wrapped := wrapSkillContent(skillName, content)
				if err := os.WriteFile(targetPath, wrapped, 0644); err != nil {
					return err
				}
				skillCount++
				return nil
			}); err != nil {
				fmt.Fprintf(os.Stderr, "  Error writing OpenCode skills: %v\n", err)
			} else if skillCount > 0 {
				fmt.Printf("  Created %d skill directories in .opencode/skills/\n", skillCount)
			}
		}

		// Vibe-specific: sync all skills
		if toolName == "vibe" {
			vibeSkillsDir := filepath.Join(projectDir, ".vibe", "skills")
			vibeSkillCount := 0
			if err := fs.WalkDir(skillsFS, "skills", func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return err
				}
				if d.Name() == "README.md" {
					return nil
				}
				content, err := skillsFS.ReadFile(path)
				if err != nil {
					return err
				}
				skillName := strings.TrimSuffix(d.Name(), ".md")
				skillDir := filepath.Join(vibeSkillsDir, skillName)
				if err := os.MkdirAll(skillDir, 0755); err != nil {
					return err
				}
				targetPath := filepath.Join(skillDir, "SKILL.md")
				if _, statErr := os.Stat(targetPath); statErr == nil {
					return nil // skip existing
				}
				wrapped := wrapSkillForVibe(skillName, content)
				if err := os.WriteFile(targetPath, wrapped, 0644); err != nil {
					return err
				}
				vibeSkillCount++
				return nil
			}); err != nil {
				fmt.Fprintf(os.Stderr, "  Error writing Vibe skills: %v\n", err)
			} else if vibeSkillCount > 0 {
				fmt.Printf("  Created %d skill directories in .vibe/skills/\n", vibeSkillCount)
			}
		}

		fmt.Println("\n✓ Tool support added!")
		fmt.Printf("\nNext steps:\n")
		fmt.Printf("  1. Open project in %s\n", toolConfig.Name)
		fmt.Printf("  2. Read AGENTS.md for MDL commands\n")
		fmt.Printf("  3. Use './mxcli -p %s' to work with the project\n", mprFile)
	},
}

func init() {
	rootCmd.AddCommand(addToolCmd)
	addToolCmd.Flags().Bool("list", false, "List supported AI tools")
}
