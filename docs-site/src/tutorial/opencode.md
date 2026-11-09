# OpenCode Integration

OpenCode is a fully supported AI integration for mxcli. It gets deep support: a dedicated configuration directory, project-level context in `AGENTS.md`, skill files that teach the AI MDL patterns, slash commands for common operations, and Starlark lint rules.

## Initializing a project

Use the `--tool opencode` flag:

```bash
mxcli init --tool opencode /path/to/my-mendix-project
```

To set up both OpenCode and Claude Code together:

```bash
mxcli init --tool opencode --tool claude /path/to/my-mendix-project
```

## What gets created

After running `mxcli init --tool opencode`, your project directory gains:

```
my-mendix-project/
├── AGENTS.md                    # Universal AI assistant guide (OpenCode reads this)
├── opencode.json                # OpenCode configuration file
├── .opencode/
│   ├── commands/                # Slash commands (/create-entity, etc.)
│   └── skills/                  # MDL pattern guides (OpenCode skill format)
│       ├── write-microflows/
│       │   └── SKILL.md
│       ├── create-page/
│       │   └── SKILL.md
│       └── ...
├── .claude/
│   └── lint-rules/              # Starlark lint rules (shared with mxcli lint)
├── .ai-context/
│   ├── skills/                  # Shared skill files (universal copy)
│   └── examples/                # Example MDL scripts
├── .devcontainer/
│   ├── devcontainer.json        # Dev container configuration
│   └── Dockerfile               # Container image with mxcli, JDK, Docker
├── mxcli                        # CLI binary (copied into project)
└── app.mpr                      # Your Mendix project (already existed)
```

### opencode.json

The `opencode.json` file is OpenCode's primary configuration. Its instructions are `AGENTS.md` alone:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "instructions": [
    "AGENTS.md"
  ]
}
```

The skills are not listed there. OpenCode inlines every instruction file's whole body into the system prompt, so listing the skill trees put about 2.7 MB of skill text in front of every session. OpenCode discovers the skills in `.opencode/skills/` itself and loads a skill's body only when its description matches the task.

### Skills

Skills live in `.opencode/skills/<name>/SKILL.md` and use YAML frontmatter that OpenCode understands. The description is the skill's own, with its "Use when…" clause, since that is what OpenCode matches a task against:

```markdown
---
name: write-microflows
description: 'Microflow syntax reference in MDL — … Use before writing any CREATE MICROFLOW …'
compatibility: opencode
---

# Writing Microflows
...
```

Each skill covers a specific area: microflow syntax, page patterns, security setup, and so on. OpenCode reads the relevant skill before generating MDL, which significantly improves output quality.

### Commands

The `.opencode/commands/` directory contains slash commands available inside OpenCode. These mirror the Claude Code commands: `/create-entity`, `/create-microflow`, `/create-page`, `/lint`, and others.

### Lint rules

Lint rules live in `.claude/lint-rules/` regardless of which tool is selected — this is where `mxcli lint` looks for custom Starlark rules. OpenCode init writes the rules there so `mxcli lint` works the same way for both tool choices.

## Setting up the dev container

The dev container setup is identical to the Claude Code workflow. Open your project folder in VS Code, click **"Reopen in Container"** when prompted, and wait for the container to build.

### What's installed in the container

| Component | Purpose |
|-----------|---------|
| **mxcli** | Mendix CLI (copied into project root) |
| **MxBuild / mx** | Mendix project validation and building |
| **JDK 21** (Adoptium) | Required by MxBuild |
| **Docker-in-Docker** | Running Mendix apps locally with `mxcli docker` |
| **Node.js** | Playwright testing support |
| **PostgreSQL client** | Database connectivity |

## Starting OpenCode

With the dev container running, open a terminal in VS Code and start OpenCode:

```bash
opencode
```

OpenCode now has access to your project files, the mxcli binary, the skill files in `.opencode/skills/`, and the commands in `.opencode/commands/`.

## How OpenCode works with your project

The workflow mirrors Claude Code exactly:

### 1. Explore

OpenCode uses mxcli commands to understand the project before making changes:

```sql
-- What modules exist?
LIST MODULES;

-- What entities are in this module?
LIST ENTITIES IN Sales;

-- What does this entity look like?
DESCRIBE ENTITY Sales.Customer;

-- What microflows exist?
LIST MICROFLOWS IN Sales;

-- Search for something specific
SEARCH 'validation';
```

### 2. Read the relevant skill

Before writing MDL, OpenCode reads the appropriate skill file from `.opencode/skills/`. If you ask for a microflow, it reads the `write-microflows` skill. If you ask for a page, it reads `create-page`.

### 3. Write MDL

OpenCode generates an MDL script based on the project context and skill guidance:

```sql
/** Customer master data */
@Position(100, 100)
CREATE PERSISTENT ENTITY Sales.Customer (
    Name: String(200) NOT NULL,
    Email: String(200) NOT NULL,
    Phone: String(50),
    IsActive: Boolean DEFAULT true
);
```

### 4. Validate

```bash
./mxcli check script.mdl
./mxcli check script.mdl -p app.mpr --references
```

### 5. Execute

```bash
./mxcli -p app.mpr -c "EXECUTE SCRIPT 'script.mdl'"
```

### 6. Verify

```bash
./mxcli docker check -p app.mpr
```

## Adding OpenCode to an existing project

If you already ran `mxcli init` for another tool and want to add OpenCode support:

```bash
mxcli add-tool opencode
```

This creates `.opencode/`, `opencode.json`, and the lint rules without touching any existing configuration.

## Tips for working with OpenCode

- **Be specific about module names.** Say "Create a Customer entity in the Sales module" rather than just "Create a Customer entity."
- **Mention existing elements.** If you want an association to an existing entity, name it: "Link Order to the existing Sales.Customer entity."
- **Let the AI explore first.** OpenCode will run SHOW and DESCRIBE commands to understand what's already in the project. This leads to better results.
- **Review in Studio Pro.** After changes are applied, open the project in Studio Pro to verify the result visually.
- **Use `mxcli docker check`** to catch issues that `mxcli check` alone might miss.

## Next steps

To understand what the skill files contain and how they guide AI behavior, see [Skills and CLAUDE.md](skills.md). For other supported tools, see [Other AI tools (Cursor, Continue.dev, Windsurf, OpenCode)](other-ai-tools.md).
