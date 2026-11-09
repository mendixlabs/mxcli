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
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/spf13/cobra"
)

var (
	initTools            []string
	initAllTools         bool
	initListTools        bool
	initContainerRuntime string
	initSyncSkills       bool
	initSync             bool
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
# Generated while 'mxcli run --test-endpoint' hosts the test endpoint (ako/mxcli#2033).
/javasource/mxtest/
/modeler-merge-marker
/nativemobile/builds/
/packages/
/project-settings.user.json
/releases/
# Compiled theme output. MxBuild regenerates it on every build, so tracking it
# means a fresh clone goes dirty the first time anyone builds. (mxcli-todo #7)
/theme-cache/
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

		// --sync-skills: refresh only the embedded skills and exit. The rest of
		// init is interactive-ish and writes tool configs; this is the part that
		// must follow a binary upgrade, and the SessionStart bootstrap runs it
		// unattended on every session (mxcli-formula1 §16).
		//
		// It also refreshes the bundled lint rules and the mxcli section of
		// CLAUDE.md / AGENTS.md, and stamps the version that did it — and
		// refuses when this binary is older than that stamp (ako/mxcli#952).
		if initSyncSkills || initSync {
			res, err := syncProjectTooling(absDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error syncing project tooling: %v\n", err)
				os.Exit(1)
			}
			reportToolingSync(os.Stdout, os.Stderr, res)
			return
		}

		// An explicit init from an older binary still runs — the user asked for
		// it — but not silently: it rewrites the tooling a newer mxcli wrote.
		if s := staleBinaryStamp(absDir); s != nil {
			writeStaleBinaryWarning(os.Stderr, *s)
			fmt.Fprintln(os.Stderr, "  Continuing: 'mxcli init' will rewrite that tooling with this older version.")
		}

		// Find .mpr file. With none here, look one level down: a solution repo
		// keeps each app in its own folder, and running `mxcli init` from the
		// root used to write everything at the root against an invented
		// `project.mpr` that does not exist — silently wrong, and in a two-app
		// repo the odds of it being what you meant are zero.
		// (mxcli-formula1 findings #3.)
		mprFile := findMprFile(absDir)
		mprPlaceholder := false
		if mprFile == "" {
			candidates := findMprFilesInSubdirs(absDir)
			switch len(candidates) {
			case 0:
				fmt.Fprintf(os.Stderr, "Warning: no .mpr file found in %s.\n", absDir)
				fmt.Fprintln(os.Stderr, "  Generated files will refer to 'project.mpr'; run this from the app folder to get real paths.")
				mprFile = "project.mpr"
				mprPlaceholder = true
			case 1:
				absDir = filepath.Dir(candidates[0])
				mprFile = filepath.Base(candidates[0])
				fmt.Printf("No .mpr here; initializing the project found below: %s\n", candidates[0])
			default:
				fmt.Fprintf(os.Stderr, "Error: %d Mendix projects found below %s:\n", len(candidates), absDir)
				for _, c := range candidates {
					fmt.Fprintf(os.Stderr, "  %s\n", c)
				}
				fmt.Fprintln(os.Stderr, "\nName the one you mean, so a solution repo does not get one app's tooling by coin flip:")
				fmt.Fprintf(os.Stderr, "  mxcli init %s\n", filepath.Dir(candidates[0]))
				os.Exit(1)
			}
		}
		// The app's name (from the .mpr), not the folder's: a cloud session
		// clones into a folder named after the repository. Every use below is
		// a display name; paths use absDir/mprFile directly.
		projectName := projectDisplayName(absDir, mprFile)
		if mprPlaceholder {
			projectName = filepath.Base(absDir)
		}

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
		var claudeDir, commandsDir string
		if slices.Contains(tools, "claude") {
			claudeDir = filepath.Join(absDir, ".claude")
			commandsDir = filepath.Join(claudeDir, "commands")

			if err := os.MkdirAll(commandsDir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating .claude/commands directory: %v\n", err)
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
		}

		// Seed the lint config. `mxcli lint` reads this regardless of which AI
		// tool was selected, so it is written outside the per-tool branches.
		if path, created := writeDefaultLintConfig(absDir); created {
			fmt.Printf("  Created %s (System module excluded from lint)\n", filepath.Base(path))
		}

		// The bundled Starlark rules, for the same reason. Only claude and
		// opencode wrote them, so `init --tool cursor` left lint running the Go
		// rules alone — 17 rules instead of 49 on one project, silently
		// (ako/mxcli#1292).
		lintRulesDir := filepath.Join(absDir, ".claude", "lint-rules")
		if err := os.MkdirAll(lintRulesDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating .claude/lint-rules directory: %v\n", err)
			os.Exit(1)
		}
		initBundledLintRules(lintRulesDir)

		// Write the skills. One code path with the SessionStart refresh, so a
		// fresh init and an upgraded project end up with byte-identical trees —
		// including the `.claude/skills/` copy Claude Code actually scans.
		skillRes, err := syncAIContextSkills(absDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing skills: %v\n", err)
			os.Exit(1)
		}
		for _, dest := range skillDests(absDir) {
			rel, relErr := filepath.Rel(absDir, dest)
			if relErr != nil {
				rel = dest
			}
			fmt.Printf("  Created %d skills in %s/\n", skillRes.Total, filepath.ToSlash(rel))
		}

		// The companion mod (a Claude Code pane), beside the skills. Same
		// code path as the SessionStart refresh, and only for Claude projects.
		if written, err := syncCompanion(absDir); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing the companion mod: %v\n", err)
			os.Exit(1)
		} else if len(written) > 0 {
			fmt.Printf("  Created the mxcli companion in .claude/skills/%s/ (/mxcli-project opens the Project pane)\n", companionFolder)
		}

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

				// Write file. CLAUDE.md goes through the mxcli markers so a
				// later sync can refresh it without touching project notes.
				if isOwnedDoc(file.Path) {
					if _, err := writeOwnedDoc(filePath, content, true); err != nil {
						fmt.Fprintf(os.Stderr, "  Error writing %s: %v\n", file.Path, err)
						continue
					}
				} else if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
					fmt.Fprintf(os.Stderr, "  Error writing %s: %v\n", file.Path, err)
					continue
				}

				fmt.Printf("  Created %s\n", file.Path)
			}

			// Claude-specific: write commands
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
			}

			// OpenCode-specific: write commands and skills
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

				skillCount2, err := writeWrappedSkills(opencodeSkillsDir, wrapSkillContent)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  Error writing OpenCode skills: %v\n", err)
				} else {
					fmt.Printf("  Created %d skill directories in .opencode/skills/\n", skillCount2)
				}
			}
			// Vibe-specific: write all skills as .vibe/skills/<name>/SKILL.md
			if toolName == "vibe" {
				vibeSkillsDir := filepath.Join(absDir, ".vibe", "skills")
				vibeSkillCount, err := writeWrappedSkills(vibeSkillsDir, wrapSkillForVibe)
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

			if _, err := writeOwnedDoc(filePath, content, true); err != nil {
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
		} else if mprFile != "" {
			// A project that already has a .gitignore keeps it — but .mxcli/ has to
			// be in there regardless, because it holds machine-local constant values
			// (which may be secrets), the run/test handshake and its token, and
			// runtime logs. Only the create-if-missing branch above covered it.
			// The same goes for the test endpoint's generated Java source.
			if err := ensureTestEndpointIgnored(filepath.Join(absDir, mprFile)); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: could not ensure .mxcli/ and javasource/mxtest/ are git-ignored: %v\n", err)
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

		// The directory the generated CLAUDE.md tells the agent to put scripts
		// in; it used to be named and never created (ako/mxcli#952).
		if created, err := ensureMdlsourceDir(absDir); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: creating mdlsource/: %v\n", err)
		} else if created {
			fmt.Println("\nCreated mdlsource/ (MDL scripts)")
		}

		// Record which mxcli wrote all of the above, so an older binary
		// opening the project later can say so (ako/mxcli#952).
		if _, err := writeToolingStamp(absDir); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: writing %s: %v\n", toolingStampRel, err)
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

// initBundledLintRules writes the bundled lint rules for an explicit init,
// keeping the ones the project edited or moved to a new name
// (mendixlabs/mxcli#1615), and says which.
func initBundledLintRules(lintRulesDir string) {
	res, err := writeBundledLintRules(lintRulesDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Error writing lint rules: %v\n", err)
	}
	printBundledLintRuleWrite(os.Stdout, res)
}

// defaultLintConfig is the lint configuration written into a freshly
// initialised project. System is excluded because its contents are Mendix's,
// not the developer's: you cannot document its entities, give them access
// rules, or rename their members. Linting it produced ~100 un-actionable
// findings on a blank app, which buried the handful about the developer's own
// code (issuetracker finding #9).
const defaultLintConfig = `# mxcli lint configuration.
# Docs: mxcli lint --help

# Modules that 'mxcli lint' skips entirely.
#
# System is Mendix's own platform module — its entities, members and access
# rules are not yours to change, so findings against it are noise. On a blank
# app it accounts for the large majority of all issues.
#
# Marketplace modules (Atlas_Core, Atlas_Web_Content, Administration, …) are
# equally read-only in practice; add them here if their findings distract you.
#
# NOTE: this list always wins. It is merged with '--exclude', and a module
# listed here is skipped even if you ask for it with '--modules'. To lint
# System, remove it from this list (or delete this file).
excludeModules:
  - System

# Per-rule overrides. Examples:
#
# rules:
#   QUAL002:            # missing documentation
#     enabled: false
#   CONV009:            # max microflow objects
#     severity: warning
#     options:
#       maxObjects: 20
#   QUAL004:            # orphaned elements: name prefixes of entry points
#     options:
#       entry_point_prefixes: [ACT_, SCH_, WS_, REST_, OData_]
rules: {}
`

// writeDefaultLintConfig creates .claude/lint-config.yaml unless the project
// already has a lint config in any of the locations linter.FindConfigFile
// searches. Never overwrites: init is re-runnable, and the config is meant to
// be edited.
func writeDefaultLintConfig(projectDir string) (string, bool) {
	if existing := linter.FindConfigFile(projectDir); existing != "" {
		return existing, false
	}
	claudeDir := filepath.Join(projectDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "  Error creating .claude directory for lint config: %v\n", err)
		return "", false
	}
	path := filepath.Join(claudeDir, "lint-config.yaml")
	if err := os.WriteFile(path, []byte(defaultLintConfig), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "  Error writing lint config: %v\n", err)
		return "", false
	}
	return path, true
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
	initCmd.Flags().BoolVar(&initSyncSkills, "sync-skills", false, "Refresh the project's mxcli tooling from this binary and exit: skills, bundled lint rules, the mxcli section of CLAUDE.md/AGENTS.md, and the version stamp (quiet when already current; refuses when this binary is older than the stamp)")
	initCmd.Flags().BoolVar(&initSync, "sync", false, "Same as --sync-skills")
}

// findMprFilesInSubdirs returns the .mpr files one level below dir, sorted, so
// the choice is deterministic and the error message lists them in a stable
// order. One level only: a Mendix app keeps its .mpr at its root, and walking
// deeper would find deployment copies and backups.
func findMprFilesInSubdirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		sub := filepath.Join(dir, e.Name())
		if mpr := findMprFile(sub); mpr != "" {
			out = append(out, filepath.Join(sub, mpr))
		}
	}
	sort.Strings(out)
	return out
}
