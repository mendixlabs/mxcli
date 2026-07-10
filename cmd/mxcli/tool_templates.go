// SPDX-License-Identifier: Apache-2.0

// tool_templates.go - Templates for multi-tool AI assistant support
package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ToolConfig defines configuration for an AI tool
type ToolConfig struct {
	Name        string
	Description string
	Files       []ToolFile
}

// ToolFile defines a configuration file to create
type ToolFile struct {
	Path     string
	Content  func(projectName, mprPath string) string
	Optional bool
}

// SupportedTools defines all AI tools that can be initialized
var SupportedTools = map[string]ToolConfig{
	"claude": {
		Name:        "Claude Code",
		Description: "Claude Code with skills and commands",
		Files: []ToolFile{
			{
				Path:    ".claude/settings.json",
				Content: generateClaudeSettings,
			},
			{
				Path:    "CLAUDE.md",
				Content: generateClaudeMD,
			},
		},
	},
	"cursor": {
		Name:        "Cursor",
		Description: "Cursor AI with MDL rules",
		Files: []ToolFile{
			{
				Path:    ".cursorrules",
				Content: generateCursorRules,
			},
		},
	},
	"continue": {
		Name:        "Continue.dev",
		Description: "Continue.dev with custom commands",
		Files: []ToolFile{
			{
				Path:    ".continue/config.json",
				Content: generateContinueConfig,
			},
		},
	},
	"windsurf": {
		Name:        "Windsurf",
		Description: "Windsurf (Codeium) with MDL rules",
		Files: []ToolFile{
			{
				Path:    ".windsurfrules",
				Content: generateWindsurfRules,
			},
		},
	},
	"aider": {
		Name:        "Aider",
		Description: "Aider with project configuration",
		Files: []ToolFile{
			{
				Path:    ".aider.conf.yml",
				Content: generateAiderConfig,
			},
		},
	},
	"opencode": {
		Name:        "OpenCode",
		Description: "OpenCode AI agent with MDL commands and skills",
		Files: []ToolFile{
			{
				Path:    "opencode.json",
				Content: generateOpenCodeConfig,
			},
		},
	},
	"vibe": {
		Name:        "Mistral Vibe",
		Description: "Mistral Vibe CLI agent with skills",
		Files: []ToolFile{
			{
				Path:    ".vibe/config.toml",
				Content: generateVibeConfig,
			},
			{
				Path:    ".vibe/prompts/mendix-mdl.md",
				Content: generateVibeSystemPrompt,
			},
		},
		// Skills are synced dynamically from the embedded skillsFS in init.go
	},
	"copilot": {
		Name:        "GitHub Copilot",
		Description: "GitHub Copilot with project-level instructions",
		Files: []ToolFile{
			{
				Path:    ".github/copilot-instructions.md",
				Content: generateCopilotInstructions,
			},
		},
	},
}

// Universal files created for all tools
var UniversalFiles = []ToolFile{
	{
		Path:    "AGENTS.md",
		Content: generateProjectAIMD,
	},
}

func generateClaudeSettings(projectName, mprPath string) string {
	return settingsJSON
}

func generateCursorRules(projectName, mprPath string) string {
	mprFile := filepath.Base(mprPath)
	return fmt.Sprintf(`# Mendix MDL Project: %s

You are working on a Mendix project with MDL (Mendix Definition Language) support via mxcli.

## Important: mxcli Location

The mxcli tool is in the PROJECT ROOT, not in system PATH. Always use:
- ./mxcli (correct)
- NOT mxcli (will fail)

## Quick Reference

### Project Connection
`+"```bash"+`
./mxcli -p %s -c "SHOW MODULES"
`+"```"+`

### Validate MDL Scripts
`+"```bash"+`
./mxcli check script.mdl                    # Syntax only
./mxcli check script.mdl -p %s --references  # With refs
`+"```"+`

### Execute MDL Scripts
`+"```bash"+`
./mxcli exec script.mdl -p %s
`+"```"+`

### Code Search (requires REFRESH CATALOG FULL)
`+"```bash"+`
./mxcli search -p %s "pattern"
./mxcli callers -p %s Module.Microflow
./mxcli refs -p %s Module.Entity
`+"```"+`

## MDL Syntax Quick Guide

### Microflows
- Variable: `+"`DECLARE $var Type = value;`"+`
- Entity: `+"`DECLARE $entity Module.Entity;`"+` (no AS, no = empty)
- Loop: `+"`LOOP $item IN $list BEGIN ... END LOOP;`"+`
- Change: `+"`CHANGE $obj (Attr = value);`"+`
- If: `+"`IF condition THEN ... END IF;`"+` (not END)
- Log: `+"`LOG WARNING NODE 'Name' 'Message';`"+`

### Pages
- Properties: `+"(Title: 'value', Layout: 'value')"+`
- Widget nesting: curly braces `+"`{ }`"+`
- Widget properties: `+"(Label: 'Name', Attribute: AttrName)"+`

## Documentation

See AGENTS.md for complete documentation and .ai-context/skills/ for patterns.

## Before Writing MDL

1. Read relevant skill file: .ai-context/skills/write-microflows.md or create-page.md
2. Validate: ./mxcli check script.mdl -p %s --references
3. Execute: ./mxcli exec script.mdl -p %s
`, projectName, mprFile, mprFile, mprFile, mprFile, mprFile, mprFile, mprFile, mprFile)
}

func generateWindsurfRules(projectName, mprPath string) string {
	// Windsurf uses same format as Cursor
	return generateCursorRules(projectName, mprPath)
}

func generateContinueConfig(projectName, mprPath string) string {
	mprFile := filepath.Base(mprPath)
	return fmt.Sprintf(`{
  "name": "%s - Mendix MDL",
  "systemMessage": "You are helping with Mendix development using MDL (Mendix Definition Language). The mxcli tool is located in the project root - always use './mxcli' not 'mxcli'.",
  "docs": [
    "AGENTS.md",
    ".ai-context/skills/"
  ],
  "customCommands": [
    {
      "name": "check-mdl",
      "description": "Check MDL script syntax",
      "prompt": "Run: ./mxcli check {filename}"
    },
    {
      "name": "check-mdl-refs",
      "description": "Check MDL with reference validation",
      "prompt": "Run: ./mxcli check {filename} -p %s --references"
    },
    {
      "name": "execute-mdl",
      "description": "Execute MDL script",
      "prompt": "Run: ./mxcli exec {filename} -p %s"
    },
    {
      "name": "show-entities",
      "description": "Show all entities in project",
      "prompt": "Run: ./mxcli -p %s -c \"SHOW ENTITIES\""
    },
    {
      "name": "search-project",
      "description": "Search project with catalog",
      "prompt": "Run: ./mxcli search -p %s \"{query}\""
    }
  ],
  "slashCommands": [
    {
      "name": "mdl-syntax",
      "description": "Show MDL syntax reference",
      "prompt": "Read and summarize: .ai-context/skills/write-microflows.md"
    },
    {
      "name": "page-syntax",
      "description": "Show page creation syntax",
      "prompt": "Read and summarize: .ai-context/skills/create-page.md"
    }
  ]
}
`, projectName, mprFile, mprFile, mprFile, mprFile)
}

func generateAiderConfig(projectName, mprPath string) string {
	mprFile := filepath.Base(mprPath)
	return fmt.Sprintf(`# Mendix MDL Project: %s
# Configuration for Aider AI coding assistant

# Files to read for context
read-files:
  - AGENTS.md
  - .ai-context/skills/*.md

# Project description
description: |
  Mendix project with MDL (Mendix Definition Language) support.
  Use ./mxcli for all project operations.

# Custom commands
commands:
  check: "./mxcli check {file}"
  check-refs: "./mxcli check {file} -p %s --references"
  execute: "./mxcli exec {file} -p %s"
  search: "./mxcli search -p %s {query}"

# Patterns to recognize
recognize:
  - "*.mdl files use MDL syntax (see .ai-context/skills/)"
  - "Always use ./mxcli (local binary) not mxcli"
  - "Microflows: LOOP BEGIN/END LOOP, CHANGE (attr=val)"
  - "Pages: { } blocks, (Prop: value)"
`, projectName, mprFile, mprFile, mprFile)
}

func generateDevcontainerJSON(projectName, mprPath, containerRuntime string) string {
	features := `"features": {
    "ghcr.io/devcontainers/features/docker-in-docker:2": {}
  },`
	runArgs := ""
	containerEnv := `"PLAYWRIGHT_CLI_SESSION": "mendix-app"`
	if containerRuntime == "podman" {
		// The devcontainers registry publishes no podman feature (issue #653),
		// so Podman is installed and configured in the Dockerfile instead.
		// label=disable eases SELinux hosts; nested rootless Podman otherwise
		// runs without extra privileges (vfs storage, configured in the image).
		features = `"features": {},`
		runArgs = `
  "runArgs": ["--security-opt", "label=disable"],`
		containerEnv = `"PLAYWRIGHT_CLI_SESSION": "mendix-app",
    "MXCLI_CONTAINER_CLI": "podman"`
	}

	return fmt.Sprintf(`{
  "name": "%s",
  "build": {
    "dockerfile": "Dockerfile"
  },
  %s%s
  "forwardPorts": [8080, 8090, 5432],
  "portsAttributes": {
    "8080-8099": { "onAutoForward": "silent" },
    "5432-5499": { "onAutoForward": "silent" }
  },
  "containerEnv": {
    %s
  },
  "postCreateCommand": "curl -fsSL https://claude.ai/install.sh | bash && if [ -f ./mxcli ] && file ./mxcli | grep -q Linux; then echo 'mxcli binary OK'; else ./mxcli setup mxcli --output ./mxcli 2>/dev/null || { ARCH=$(uname -m); [ \"$ARCH\" = x86_64 ] && ARCH=amd64; [ \"$ARCH\" = aarch64 ] && ARCH=arm64; curl -fsSL https://github.com/JordtenBulte-OLC/mxcli/releases/latest/download/mxcli-linux-${ARCH} -o ./mxcli && chmod +x ./mxcli; }; fi",
  "customizations": {
    "vscode": {
      "extensions": [
        "anthropic.claude-code"
      ],
      "settings": {
        "mdl.mxcliPath": "./mxcli"
      }
    }
  },
  "remoteUser": "vscode"
}
`, projectName, features, runArgs, containerEnv)
}

func generateDockerfile(projectName, mprPath, containerRuntime string) string {
	podmanSetup := ""
	if containerRuntime == "podman" {
		// No devcontainers feature provides Podman (issue #653), so install and
		// configure rootless Podman directly. The dev container has no systemd,
		// so the cgroupfs manager + file event logger are required; vfs storage
		// avoids needing /dev/fuse; the search registry lets unqualified image
		// names (e.g. postgres:16) resolve.
		podmanSetup = `
# Install rootless Podman for nested in-container container management.
RUN apt-get update && apt-get install -y --no-install-recommends \
       podman uidmap slirp4netns fuse-overlayfs \
    && apt-get clean && rm -rf /var/lib/apt/lists/* \
    && echo 'vscode:100000:65536' > /etc/subuid \
    && echo 'vscode:100000:65536' > /etc/subgid \
    && mkdir -p /etc/containers \
    && printf '[storage]\ndriver = "vfs"\n' > /etc/containers/storage.conf \
    && printf 'unqualified-search-registries = ["docker.io"]\n' > /etc/containers/registries.conf \
    && printf '[engine]\ncgroup_manager = "cgroupfs"\nevents_logger = "file"\n' > /etc/containers/containers.conf
`
	}

	return `FROM mcr.microsoft.com/devcontainers/base:bookworm

# Install Adoptium JDK 21 (required by MxBuild), Node.js 22, and utility tools
RUN apt-get update && apt-get install -y --no-install-recommends wget apt-transport-https gpg ca-certificates curl && \
    wget -qO - https://packages.adoptium.net/artifactory/api/gpg/key/public | gpg --dearmor -o /etc/apt/keyrings/adoptium.gpg && \
    echo "deb [signed-by=/etc/apt/keyrings/adoptium.gpg] https://packages.adoptium.net/artifactory/deb bookworm main" > /etc/apt/sources.list.d/adoptium.list && \
    curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && \
    apt-get update && \
    apt-get install -y --no-install-recommends \
       temurin-21-jdk \
       nodejs \
       postgresql-client \
       kafkacat \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Install playwright-cli plus the bundled Chromium (headless shell).
# @playwright/cli ships its OWN playwright-core, so browsers must be installed
# via that bundled core (matching revision) — NOT via a transient "npx
# playwright", which resolves a different package and cache. Browsers go under a
# world-readable PLAYWRIGHT_BROWSERS_PATH so the non-root "vscode" user finds
# them (a root-owned ~/.cache/ms-playwright would be invisible to it).
#
# On Linux arm64 the chrome/msedge *channels* have no distribution and the alpha
# CLI's browserName:chromium resolution is unreliable, so we pin the bundled
# Chromium headless-shell through a stable symlink referenced by
# .playwright/cli.config.json (executablePath). The symlink keeps the config
# revision-proof as the headless-shell build number changes.
# @playwright/cli is pinned (not @latest): it is a young package whose CLI
# surface still shifts between releases, and an unexpected bump has silently
# broken browser provisioning before. Bump deliberately after re-verifying the
# flow below. NOTE: the npm package version (0.1.x) is independent of the
# playwright-core version the CLI's --version reports (e.g. 1.59.0-alpha).
ENV PLAYWRIGHT_BROWSERS_PATH=/opt/ms-playwright
RUN npm install -g @playwright/cli@0.1.15 && \
    CORE="$(npm root -g)/@playwright/cli/node_modules/playwright-core/cli.js" && \
    node "$CORE" install --with-deps chromium chromium-headless-shell && \
    chmod -R a+rX "$PLAYWRIGHT_BROWSERS_PATH" && \
    ln -sf "$PLAYWRIGHT_BROWSERS_PATH"/chromium_headless_shell-*/chrome-linux/headless_shell /usr/local/bin/mx-headless-shell
` + podmanSetup
}

func generatePlaywrightConfig() string {
	return `{
  "browser": {
    "browserName": "chromium",
    "isolated": true,
    "launchOptions": {
      "headless": true,
      "executablePath": "/usr/local/bin/mx-headless-shell"
    }
  },
  "timeouts": {
    "action": 10000,
    "navigation": 30000
  },
  "network": {
    "allowedOrigins": [
      "http://localhost:8079",
      "http://localhost:8080",
      "http://localhost:8081",
      "http://localhost:8082",
      "http://localhost:8083",
      "http://localhost:8084",
      "http://localhost:8085"
    ]
  }
}
`
}

func generateProjectAIMD(projectName, mprPath string) string {
	return generateClaudeMD(projectName, mprPath)
}

func generateVibeConfig(projectName, mprPath string) string {
	return fmt.Sprintf(`# Mistral Vibe configuration for Mendix project: %s
# See: https://docs.mistral.ai/mistral-vibe/introduction/configuration

# Use the project AGENTS.md as system prompt context
system_prompt_id = "mendix-mdl"

# Skills from .vibe/skills/ are auto-discovered
# Additional context files
# skill_paths = [".ai-context/skills"]

# Tool permissions for MDL workflow
[tools.bash]
permission = "ask"

[tools.read_file]
permission = "always"

[tools.write_file]
permission = "ask"

[tools.search_replace]
permission = "ask"
`, projectName)
}

func generateVibeSystemPrompt(projectName, mprPath string) string {
	mprFile := filepath.Base(mprPath)
	return fmt.Sprintf(`You are helping with a Mendix project using MDL (Mendix Definition Language) via mxcli.

## Project: %s

MPR file: %s

## Key Rules

- The mxcli tool is in the project root. Always use ./mxcli, not mxcli.
- Read AGENTS.md for full project documentation.
- Read .ai-context/skills/ for MDL syntax patterns before writing scripts.
- Always validate MDL scripts: ./mxcli check script.mdl -p %s --references
- Microflow variables start with $. Entity declarations have no AS keyword.
- Page widgets nest with curly braces { }. Properties use (Key: value).
- Single quotes in expressions are escaped by doubling: 'it''s here'

## Quick Commands

- Explore: ./mxcli -p %s -c "SHOW STRUCTURE"
- Check: ./mxcli check script.mdl -p %s --references
- Execute: ./mxcli exec script.mdl -p %s
- Search: ./mxcli search -p %s "keyword"
`, projectName, mprFile, mprFile, mprFile, mprFile, mprFile, mprFile)
}

// wrapSkillForVibe wraps a plain markdown skill file with Vibe SKILL.md frontmatter.
func wrapSkillForVibe(skillName string, content []byte) []byte {
	// Derive a description from the first markdown heading
	description := "MDL skill for Mendix development"
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "# ") {
			description = strings.TrimPrefix(line, "# ")
			break
		}
	}

	frontmatter := fmt.Sprintf(`---
name: %s
description: %s
user-invocable: true
allowed-tools:
  - read_file
  - write_file
  - bash
  - grep
---

`, skillName, description)

	return []byte(frontmatter + string(content))
}

// generateCopilotInstructions creates .github/copilot-instructions.md, the
// project-level instructions file automatically loaded by GitHub Copilot in VS Code.
// Kept compact since Copilot's instruction context window is smaller than Claude Code's.
func generateCopilotInstructions(projectName, mprPath string) string {
	mprFile := filepath.Base(mprPath)
	return fmt.Sprintf("# Mendix MDL Project: %s\n\n"+
		"This project uses **mxcli** to read and modify Mendix model files (`.mpr`) via MDL "+
		"(Mendix Definition Language) — a SQL-like text format for Mendix applications.\n\n"+
		"## Critical: mxcli Location\n\n"+
		"The mxcli binary is in the **project root**, not on PATH:\n"+
		"- ✅ `./mxcli ...`\n"+
		"- ❌ `mxcli ...` (will fail)\n\n"+
		"MPR file: `%s`\n\n"+
		"## Quick Commands\n\n"+
		"```bash\n"+
		"# Explore project structure\n"+
		"./mxcli -p %s -c \"SHOW STRUCTURE\"\n\n"+
		"# Validate an MDL script (always do this before executing)\n"+
		"./mxcli check script.mdl -p %s --references\n\n"+
		"# Execute an MDL script\n"+
		"./mxcli exec script.mdl -p %s\n\n"+
		"# Search the project\n"+
		"./mxcli search -p %s \"keyword\"\n"+
		"```\n\n"+
		"## Where to Look First\n\n"+
		"- **`AGENTS.md`** — full mxcli reference, MDL command list, workflow examples.\n"+
		"- **`.ai-context/skills/`** — focused syntax guides for microflows, pages, "+
		"domain models, security, etc. **Read the relevant skill before writing MDL.**\n\n"+
		"## MDL Syntax Reminders\n\n"+
		"- **Entities:** `CREATE PERSISTENT ENTITY Mod.Customer (Name: String(200), Age: Integer);`\n"+
		"- **Microflow variables:** start with `$` (e.g., `$Customer`, `$List`).\n"+
		"- **Page widgets:** nest with `{ }`, properties use `(Key: value, ...)`.\n"+
		"- **Single quotes in expressions:** escape by doubling — `'it''s here'`.\n"+
		"- **No `AS` keyword** in entity attribute declarations.\n"+
		"- **Statements** end with `;` or `/` on a line by itself.\n\n"+
		"## Workflow\n\n"+
		"1. Explore: read `AGENTS.md` and the relevant `.ai-context/skills/` file first.\n"+
		"2. Write MDL in a `.mdl` file.\n"+
		"3. Validate: `./mxcli check <file>.mdl -p %s --references`.\n"+
		"4. Execute: `./mxcli exec <file>.mdl -p %s`.\n"+
		"5. Verify: `./mxcli -p %s -c \"DESCRIBE ENTITY Mod.Name\"`.\n",
		projectName, mprFile, mprFile, mprFile, mprFile, mprFile, mprFile, mprFile, mprFile)
}

func generateOpenCodeConfig(projectName, mprPath string) string {
	return `{
  "$schema": "https://opencode.ai/config.json",
  "instructions": [
    "AGENTS.md",
    ".opencode/skills/**/SKILL.md",
    ".ai-context/skills/*.md"
  ]
}
`
}
