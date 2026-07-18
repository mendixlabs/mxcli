// SPDX-License-Identifier: Apache-2.0

// init.go - Initialize Mendix project for Claude Code integration
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

var (
	initTools            []string
	initAllTools         bool
	initListTools        bool
	initContainerRuntime string
)

const mendixGitignore = `# Mendix project
/**/node_modules/
!/javascriptsource/**/node_modules/
/*.launch
/.classpath
/.mendix-cache/
/.project
/deployment/
/javasource/*/proxies/
/javasource/system/
/modeler-merge-marker
/nativemobile/builds/
/packages/
/project-settings.user.json
/releases/
*.mpr.lock
*.mpr.bak
/vendorlib/temp/
/.svn/

# MPR v2 journal files
/mprcontents/mprjournal*

# OS
.DS_Store

# mxcli
.claude/settings.local.json
mxcli
mxcli.exe
.mxcli/
`

var initCmd = &cobra.Command{
	Use:   "init [project-directory]",
	Short: "Initialize a Mendix project for AI-assisted development",
	Long: `Initialize a Mendix project for AI-assisted development using mxcli.

This command creates configuration for AI coding assistants and MDL development.

Default: Claude Code + universal documentation

Examples:
  # Initialize with Claude Code (default)
  mxcli init

  # Initialize for Cursor
  mxcli init --tool cursor

  # Initialize for multiple tools
  mxcli init --tool claude --tool cursor --tool continue

  # Initialize for all supported tools
  mxcli init --all-tools

  # List supported tools
  mxcli init --list-tools

Supported Tools:
  - claude      Claude Code with skills and commands
  - cursor      Cursor AI with MDL rules
  - continue    Continue.dev with custom commands
  - windsurf    Windsurf (Codeium) with MDL rules
  - aider       Aider with project configuration
  - opencode    OpenCode AI agent with MDL commands and skills
  - vibe        Mistral Vibe CLI agent with skills
  - copilot     GitHub Copilot with project-level instructions

All tools receive universal documentation in AGENTS.md and .ai-context/

Container Runtime:
  --container-runtime docker   Use Docker-in-Docker (default)
  --container-runtime podman   Use Podman-in-Podman
`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// List tools if requested
		if initListTools {
			fmt.Println("Supported AI Tools:")
			fmt.Println()
			for key, tool := range SupportedTools {
				fmt.Printf("  %-12s %s\n", key, tool.Description)
			}
			return
		}
		projectDir := "."
		if len(args) > 0 {
			projectDir = args[0]
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
		projectName := filepath.Base(absDir)

		// Determine which tools to initialize
		tools := initTools
		if initAllTools {
			tools = []string{}
			for key := range SupportedTools {
				tools = append(tools, key)
			}
		}
		if len(tools) == 0 {
			// Default to Claude + universal
			tools = []string{"claude"}
		}

		fmt.Printf("Initializing AI-assisted development for: %s\n", absDir)
		fmt.Printf("Tools: %v\n", tools)

		// Create .ai-context directory for universal content
		aiContextDir := filepath.Join(absDir, ".ai-context")
		skillsDir := filepath.Join(aiContextDir, "skills")
		examplesDir := filepath.Join(aiContextDir, "examples")

		if err := os.MkdirAll(skillsDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating .ai-context/skills directory: %v\n", err)
			os.Exit(1)
		}
		if err := os.MkdirAll(examplesDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating .ai-context/examples directory: %v\n", err)
			os.Exit(1)
		}

		// Create .claude directory for Claude-specific content (if Claude is selected)
		var claudeDir, commandsDir, lintRulesDir string
		if slices.Contains(tools, "claude") {
			claudeDir = filepath.Join(absDir, ".claude")
			commandsDir = filepath.Join(claudeDir, "commands")
			lintRulesDir = filepath.Join(claudeDir, "lint-rules")

			if err := os.MkdirAll(commandsDir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating .claude/commands directory: %v\n", err)
				os.Exit(1)
			}
			if err := os.MkdirAll(lintRulesDir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating .claude/lint-rules directory: %v\n", err)
				os.Exit(1)
			}
		}

		// Create .opencode directory for OpenCode-specific content (if OpenCode is selected)
		var opencodeCommandsDir, opencodeSkillsDir string
		if slices.Contains(tools, "opencode") {
			opencodeDir := filepath.Join(absDir, ".opencode")
			opencodeCommandsDir = filepath.Join(opencodeDir, "commands")
			opencodeSkillsDir = filepath.Join(opencodeDir, "skills")

			if err := os.MkdirAll(opencodeCommandsDir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating .opencode/commands directory: %v\n", err)
				os.Exit(1)
			}
			if err := os.MkdirAll(opencodeSkillsDir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating .opencode/skills directory: %v\n", err)
				os.Exit(1)
			}

			// Lint rules stay in .claude/lint-rules/ (read by mxcli lint).
			// Ensure that directory exists even when claude tool is not selected.
			if !slices.Contains(tools, "claude") {
				if lintRulesDir == "" {
					lintRulesDir = filepath.Join(absDir, ".claude", "lint-rules")
				}
				if err := os.MkdirAll(lintRulesDir, 0755); err != nil {
					fmt.Fprintf(os.Stderr, "Error creating .claude/lint-rules directory: %v\n", err)
					os.Exit(1)
				}
			}
		}

		// Write universal skills to .ai-context/skills/
		skillCount := 0
		err = fs.WalkDir(skillsFS, "skills", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			// Read from embedded FS
			content, err := skillsFS.ReadFile(path)
			if err != nil {
				return err
			}
			// Write to target directory
			targetPath := filepath.Join(skillsDir, d.Name())
			if err := os.WriteFile(targetPath, content, 0644); err != nil {
				return err
			}
			skillCount++
			return nil
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing skills: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("  Created %d skill files in .ai-context/skills/\n", skillCount)

		// Write tool-specific configurations
		for _, toolName := range tools {
			toolConfig, ok := SupportedTools[toolName]
			if !ok {
				fmt.Fprintf(os.Stderr, "Warning: unknown tool '%s', skipping\n", toolName)
				continue
			}

			fmt.Printf("\nConfiguring %s:\n", toolConfig.Name)

			for _, file := range toolConfig.Files {
				filePath := filepath.Join(absDir, file.Path)

				// Create parent directory if needed
				if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
					fmt.Fprintf(os.Stderr, "  Error creating directory for %s: %v\n", file.Path, err)
					continue
				}

				// Generate content
				content := file.Content(projectName, mprFile)

				// Write file
				if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
					fmt.Fprintf(os.Stderr, "  Error writing %s: %v\n", file.Path, err)
					continue
				}

				fmt.Printf("  Created %s\n", file.Path)
			}

			// Claude-specific: write commands and lint rules
			if toolName == "claude" && commandsDir != "" {
				cmdCount := 0
				err = fs.WalkDir(commandsFS, "commands", func(path string, d fs.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if d.IsDir() {
						return nil
					}
					content, err := commandsFS.ReadFile(path)
					if err != nil {
						return err
					}
					targetPath := filepath.Join(commandsDir, d.Name())
					if err := os.WriteFile(targetPath, content, 0644); err != nil {
						return err
					}
					cmdCount++
					return nil
				})
				if err != nil {
					fmt.Fprintf(os.Stderr, "  Error writing commands: %v\n", err)
				} else {
					fmt.Printf("  Created %d command files in .claude/commands/\n", cmdCount)
				}

				lintRuleCount := 0
				err = fs.WalkDir(lintRulesFS, "lint-rules", func(path string, d fs.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if d.IsDir() {
						return nil
					}
					content, err := lintRulesFS.ReadFile(path)
					if err != nil {
						return err
					}
					targetPath := filepath.Join(lintRulesDir, d.Name())
					if err := os.WriteFile(targetPath, content, 0644); err != nil {
						return err
					}
					lintRuleCount++
					return nil
				})
				if err != nil {
					fmt.Fprintf(os.Stderr, "  Error writing lint rules: %v\n", err)
				} else {
					fmt.Printf("  Created %d lint rule files in .claude/lint-rules/\n", lintRuleCount)
				}
			}

			// OpenCode-specific: write commands, lint rules, and skills
			if toolName == "opencode" && opencodeCommandsDir != "" {
				cmdCount := 0
				err = fs.WalkDir(commandsFS, "commands", func(path string, d fs.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if d.IsDir() {
						return nil
					}
					content, err := commandsFS.ReadFile(path)
					if err != nil {
						return err
					}
					targetPath := filepath.Join(opencodeCommandsDir, d.Name())
					if err := os.WriteFile(targetPath, content, 0644); err != nil {
						return err
					}
					cmdCount++
					return nil
				})
				if err != nil {
					fmt.Fprintf(os.Stderr, "  Error writing OpenCode commands: %v\n", err)
				} else {
					fmt.Printf("  Created %d command files in .opencode/commands/\n", cmdCount)
				}

				lintRuleCount := 0
				// Only write lint rules from the OpenCode path when Claude is not also
				// being initialised — the Claude path already writes the same files to
				// .claude/lint-rules/ and we don't want duplicate log output or writes.
				if !slices.Contains(tools, "claude") {
					err = fs.WalkDir(lintRulesFS, "lint-rules", func(path string, d fs.DirEntry, err error) error {
						if err != nil {
							return err
						}
						if d.IsDir() {
							return nil
						}
						content, err := lintRulesFS.ReadFile(path)
						if err != nil {
							return err
						}
						targetPath := filepath.Join(lintRulesDir, d.Name())
						if err := os.WriteFile(targetPath, content, 0644); err != nil {
							return err
						}
						lintRuleCount++
						return nil
					})
					if err != nil {
						fmt.Fprintf(os.Stderr, "  Error writing lint rules: %v\n", err)
					} else {
						fmt.Printf("  Created %d lint rule files in .claude/lint-rules/\n", lintRuleCount)
					}
				}

				skillCount2 := 0
				err = fs.WalkDir(skillsFS, "skills", func(path string, d fs.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if d.IsDir() {
						return nil
					}
					// Skip README
					if d.Name() == "README.md" {
						return nil
					}
					content, err := skillsFS.ReadFile(path)
					if err != nil {
						return err
					}
					// Derive skill name from filename (strip .md)
					skillName := strings.TrimSuffix(d.Name(), ".md")
					// Create per-skill subdirectory
					skillDir := filepath.Join(opencodeSkillsDir, skillName)
					if err := os.MkdirAll(skillDir, 0755); err != nil {
						return err
					}
					// Wrap content with OpenCode frontmatter
					wrapped := wrapSkillContent(skillName, content)
					targetPath := filepath.Join(skillDir, "SKILL.md")
					if err := os.WriteFile(targetPath, wrapped, 0644); err != nil {
						return err
					}
					skillCount2++
					return nil
				})
				if err != nil {
					fmt.Fprintf(os.Stderr, "  Error writing OpenCode skills: %v\n", err)
				} else {
					fmt.Printf("  Created %d skill directories in .opencode/skills/\n", skillCount2)
				}
			}
			// Vibe-specific: write all skills as .vibe/skills/<name>/SKILL.md
			if toolName == "vibe" {
				vibeSkillsDir := filepath.Join(absDir, ".vibe", "skills")
				vibeSkillCount := 0
				err = fs.WalkDir(skillsFS, "skills", func(path string, d fs.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if d.IsDir() {
						return nil
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
					wrapped := wrapSkillForVibe(skillName, content)
					targetPath := filepath.Join(skillDir, "SKILL.md")
					if err := os.WriteFile(targetPath, wrapped, 0644); err != nil {
						return err
					}
					vibeSkillCount++
					return nil
				})
				if err != nil {
					fmt.Fprintf(os.Stderr, "  Error writing Vibe skills: %v\n", err)
				} else {
					fmt.Printf("  Created %d skill directories in .vibe/skills/\n", vibeSkillCount)
				}
			}
		}

		// Write universal AGENTS.md
		fmt.Println("\nCreating universal documentation:")
		for _, file := range UniversalFiles {
			filePath := filepath.Join(absDir, file.Path)
			content := file.Content(projectName, mprFile)

			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "  Error writing %s: %v\n", file.Path, err)
				os.Exit(1)
			}

			fmt.Printf("  Created %s\n", file.Path)
		}

		// Create or update .devcontainer/ configuration
		devcontainerDir := filepath.Join(absDir, ".devcontainer")
		devcontainerJSON := filepath.Join(devcontainerDir, "devcontainer.json")
		dcExisted := false
		if _, err := os.Stat(devcontainerJSON); err == nil {
			dcExisted = true
		}
		if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating .devcontainer directory: %v\n", err)
		} else {
			dcJSON := generateDevcontainerJSON(projectName, mprFile, initContainerRuntime)
			if err := os.WriteFile(devcontainerJSON, []byte(dcJSON), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "  Error writing devcontainer.json: %v\n", err)
			}
			dockerfile := filepath.Join(devcontainerDir, "Dockerfile")
			dcDockerfile := generateDockerfile(projectName, mprFile, initContainerRuntime)
			if err := os.WriteFile(dockerfile, []byte(dcDockerfile), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "  Error writing Dockerfile: %v\n", err)
			}
			if dcExisted {
				fmt.Println("\nUpdated .devcontainer/ configuration")
			} else {
				fmt.Println("\nCreated .devcontainer/ configuration")
			}
			if runtime.GOOS == "windows" {
				fmt.Println("\n⚠  You are running on Windows. The devcontainer is Linux-based,")
				fmt.Println("   so the Windows mxcli.exe will not work inside it.")
				fmt.Println("   The devcontainer will auto-download the correct Linux binary on first start.")
				fmt.Println("   Or run: mxcli setup mxcli --os linux --output ./mxcli")
			}
		}

		// Emit a Claude Code SessionStart hook so a fresh/reaped web session
		// self-bootstraps (cache MxBuild+runtime, provision the DB) before use.
		if slices.Contains(tools, "claude") && claudeDir != "" {
			if changed, err := ensureSessionStartHook(claudeDir, mprFile); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: SessionStart hook: %v\n", err)
			} else if changed {
				fmt.Println("\nAdded SessionStart hook to .claude/settings.json (self-bootstrap on session start)")
			}
		}

		// Create .gitignore if it doesn't exist
		gitignorePath := filepath.Join(absDir, ".gitignore")
		if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
			if err := os.WriteFile(gitignorePath, []byte(mendixGitignore), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "  Error writing .gitignore: %v\n", err)
			} else {
				fmt.Println("\nCreated .gitignore")
			}
		}

		// Create .playwright/cli.config.json for playwright-cli
		playwrightDir := filepath.Join(absDir, ".playwright")
		playwrightConfig := filepath.Join(playwrightDir, "cli.config.json")
		if _, err := os.Stat(playwrightConfig); os.IsNotExist(err) {
			if err := os.MkdirAll(playwrightDir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating .playwright directory: %v\n", err)
			} else {
				configContent := generatePlaywrightConfig()
				if err := os.WriteFile(playwrightConfig, []byte(configContent), 0644); err != nil {
					fmt.Fprintf(os.Stderr, "  Error writing playwright config: %v\n", err)
				} else {
					fmt.Println("\nCreated .playwright/cli.config.json")
				}
			}
		}

		// Install VS Code extension if Claude or OpenCode is selected
		if slices.Contains(tools, "claude") || slices.Contains(tools, "opencode") {
			installVSCodeExtension(absDir)
		}

		// Extract pluggable widget definitions from the project's widgets/
		// folder. Skip silently if there's no .mpr or no widgets/. Auto-
		// refresh keeps definitions in sync when mxcli has been upgraded.
		if mprPath := filepath.Join(absDir, mprFile); statExists(mprPath) {
			if err := ExtractWidgetDefinitions(mprPath, false, false); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: widget definition extraction failed: %v\n", err)
			}
		}

		fmt.Println("\n✓ Initialization complete!")
		fmt.Println("\nWhat was created:")
		fmt.Println("  • .gitignore - Mendix project ignore patterns")
		fmt.Println("  • AGENTS.md - Universal AI assistant guide")
		fmt.Println("  • .ai-context/skills/ - MDL pattern guides")
		fmt.Println("  • .devcontainer/ - Dev container configuration")
		for _, toolName := range tools {
			if config, ok := SupportedTools[toolName]; ok {
				fmt.Printf("  • %s configuration\n", config.Name)
			}
		}

		fmt.Println("\nNext steps:")
		fmt.Println("  1. Open this project in your AI coding assistant")
		fmt.Println("  2. Read AGENTS.md for command reference")
		fmt.Println("  3. Use './mxcli -p " + mprFile + "' to work with the project")
		fmt.Println("\nAvailable commands:")
		fmt.Println("  ./mxcli check <script>         - Validate MDL")
		fmt.Println("  ./mxcli exec <script>          - Execute MDL")
		fmt.Println("  ./mxcli search \"pattern\"       - Search project")
		fmt.Println("  ./mxcli lint                   - Check for issues")
	},
}

func findMprFile(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".mpr") {
			return e.Name()
		}
	}
	return ""
}

func statExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func init() {
	rootCmd.AddCommand(initCmd)

	// Add flags for tool selection
	initCmd.Flags().StringSliceVar(&initTools, "tool", []string{}, "AI tool(s) to configure (claude, opencode, cursor, continue, windsurf, aider)")
	initCmd.Flags().BoolVar(&initAllTools, "all-tools", false, "Initialize for all supported AI tools")
	initCmd.Flags().BoolVar(&initListTools, "list-tools", false, "List supported AI tools and exit")
	initCmd.Flags().StringVar(&initContainerRuntime, "container-runtime", "docker", "Container runtime for devcontainer (docker or podman)")
}
