# mxcli - Mendix CLI for AI-Assisted Development

> **WARNING: Alpha-quality software.** This project is in early stages and has been largely vibe-engineered with AI coding assistants. Expect bugs, missing features, and rough edges. **mxcli can corrupt your Mendix project files** — always work on a copy or use version control. Use at your own risk. This has been developed and tested against Mendix 11.6, other versions are currently not validated.
>
> **Do not edit a project with mxcli while it is open in Studio Pro.** Studio Pro maintains in-memory caches that cannot be updated externally. Close the project in Studio Pro first, run mxcli, then re-open the project.

A command-line tool that enables AI coding assistants ([Claude Code](https://claude.ai/claude-code), GitHub Copilot, OpenCode, Cursor, Continue.dev, Windsurf, Aider, and others) to read, understand, and modify Mendix application projects.

**[Read the documentation](https://mendixlabs.github.io/mxcli/)** | **[Try it in the Playground](https://codespaces.new/mendixlabs/mxcli-playground)** -- no install needed, runs in your browser

## Why mxcli?

Mendix projects are stored in binary `.mpr` files that AI agents can't read directly. `mxcli` bridges this gap by providing:

- **MDL (Mendix Definition Language)** - A SQL-like syntax for querying and modifying Mendix models
- **Multi-tool support** - Works with Claude Code, GitHub Copilot, OpenCode, Cursor, Continue.dev, Windsurf, Aider, and more
- **Full-text search** - Search across all strings, messages, and source definitions
- **Code navigation** - Find callers, callees, references, and impact analysis
- **Catalog queries** - SQL-based querying of project metadata
- **Linting** - Check projects for common issues
- **Unix pipe support** - Output formats designed for scripting and chaining

### Design principles

mxcli is built on a single bet: the agent writes a compact, human-readable DSL, and mxcli's deterministic layer turns it into the many low-level mutations, validations, and tool calls a real model change requires. The differentiators that follow:

- **Token efficiency** - the agent emits one larger MDL script instead of many small, chatty tool calls. MDL is 5-10x denser than raw model JSON, and one script is one model turn -- less orchestration overhead means lower cost and faster runs.
- **Complexity in the deterministic layer** - one MDL statement maps deterministically to multiple documents (or MCP tool calls). The LLM states intent; mxcli carries the fan-out, ordering, and read-modify-write bookkeeping.
- **Human-readable DSL** - MDL is inspired by SQL, Ada, and PL/SQL. LLM-generated scripts are easy for humans to review, audit, and author. Readable code is reviewable code.
- **Headless agentic loop** - build and test Mendix applications without ever opening Studio Pro.
- **Understanding large applications** - a queryable catalog and dependency-graph tables let agents explore complex apps through straightforward SQL.
- **Precise skills** - a concrete DSL lets skill files embed specific, copyable examples of exactly the MDL to produce, for more reliable output.
- **Power with safety** - `mxcli init` provisions a Dev Container that scopes exactly what the agent can see, do, and modify.
- **Deterministic verification** - `mxcli check` and `mxcli lint` verify generated projects against deterministic rules -- no LLM judgement -- keeping the feedback loop fast and reliable.
- **Longer-term: smaller local models** - generating a constrained DSL is simpler for an LLM than orchestrating tool calls, and a grammar can be enforced via grammar-constrained decoding.

See [Design Principles](https://mendixlabs.github.io/mxcli/preface/design-principles.html) in the documentation for the full rationale.

## What is mxcli?

Mxcli is a tool that enables some of the following use cases.

### A textual DSL for mendix models

MDL, Mendix Definition Language, is a DSL that provides textual models at the same abstraction level as the visual models in Studio Pro. 

![Mxcli MDL](docs/images/mxcli-mdl-dsl.png)

### Command line tool to work with Mendix projects

Mxcli command line tool allows you to run commands against your project to investigate your project and make changes.

![Mxcli](docs/images/mxcli-cli.png)

### A REPL to work with Mendix projects

In repl mode mxcli allows you to interactively work with a Mendix project. This is similar to psql or sqlplus when working with databases. You can list the available Mendix documents, view the MDL source, and make changes.

![Mxcli repl](docs/images/mxcli-repl.png)

### Skills and configuration to enable Agentic Coding on Mendix projects

Running *mxcli init* will install configuration files for agentic coding tools like AGENTS.md, CLAUDE.md, and Mendix specific skills. It will also configure a devcontainer that you can use when opening the project in Vscode, so you limit what your agentic coder can impact and see. 

![Mxcli skills](docs/images/mxcli-init-claud.png)

This screenshot shows how Claude uses mxcli command to do agentic search on your Mendix project to understand what is available. It gets a list of pages that are in the specified module, it uses structure to get an overview of all the documents in the module, and then it describes the soure of a specifc page. Based on this info it can make a plan how to modify your project.

![Mxcli claude](docs/images/mxcli-claude-add-page.png)

### A set of extensible skills

The skills documents teach agentic coding tools how to build Mendix projects. You can add your own skills with design patterns and best practices. Using MDL you can be very specific how the agent should generate the required Mendix documents.

![Mxcli skills](docs/images/mxcli-skills.png)

### Metadata Catalog 

Mxcli builds up a set of database tables with information about your project. This allows for flexible agentic search on your project documents.

![Mxcli catalog](docs/images/mxcli-catalog.png)

### A Mendix project linter

The catalog tables are exposed as Starlark APIs so you can use the available data in custom Mendix linter rules.

![Mxcli lint](docs/images/mxcli-lint.png)

### VSCode for Mendix projects

The easiest way to use mxcli is in vscode. You can run Claude Code inside vscode, mxcli installs a Mendix vscode extension that helps you review and understand your Mendix project. 

![mxcli vscode claude code](docs/images/mxcli-vscode-claude.png)

The project structure shows you all modules with document, similar to the app explorer in Mendix Studio Pro. The VSCode extension also provides visualizations for some Mendix document types, ensuring you can review the generated documents without leaving VSCode.

![mxcli vscode mendix extions](docs/images/mxcli-vscode-ext.png)

### Run and test your Mendix projects

Claude code can start your Mendix project using PAD (portable application distribution). This will run the Mendix runtime in a docker container, and postgres in another docker container. This allows you to test your Mendix project without leaving vscode.

![mxcli docker portable application distribution](docs/images/mxcli-docker-run.png)

### Automated Playwright-cli testing for Mendix projects

The devcontainer is configured for use with playwright-cli so Claude Code can test your running application.

### Data migration for Mendix projects

Claude code can migrate existing data, or generate demo data in the postgres container when you run your application.

### Edit your Mendix Project in the browser with GitHub Codespaces

![GitHub codespaces](docs/images/mxcli-github-codespaces.png)

## Quick Start

### Starting from scratch

Create a new Mendix project with everything configured in one command:

```bash
mxcli new MyApp --version 11.8.0
```

This downloads MxBuild, creates a blank Mendix project, sets up AI tooling and a Dev Container, and installs the correct mxcli binary. Open the resulting folder in VS Code and reopen in the Dev Container — you're ready to go.

### From the web or an iPad (empty repo, no local install)

No machine with a CLI? Open an **empty repo** in [Claude Code on the web](https://claude.ai/code) (works on an iPad) and paste the **bootstrap prompt** — the agent provisions the whole project (creates the app, wires the Dev Container + AI tooling, provisions the database) and commits it so future sessions self-bootstrap. This is the recommended web/iPad path; you don't need to pick a GitHub template.

> See **[Bootstrap Prompt](https://mendixlabs.github.io/mxcli/tools/bootstrap-prompt.html)** for the exact copy-paste text. In short: it runs `mxcli new` → `mxcli init` → commits the config → `mxcli run --local --setup --ensure-db` so the app comes up testable.

Then iterate with the **warm local dev loop** — a Docker-free ~1-second edit→test cycle:

```bash
mxcli run --local -p app.mpr --watch --screenshot   # hot-reload + auto screenshots
```

`mxcli run --local` keeps `mxbuild --serve` and a standalone runtime hot: a page/microflow edit is hot-applied in seconds (a hot `reload_model`, or a restart + DDL for entity changes), and `--screenshot` captures each page with Playwright. See **[Local Dev Loop](https://mendixlabs.github.io/mxcli/tools/run-local.html)**.

### Existing project

For an existing Mendix project, use `mxcli init` to add AI tooling and a Dev Container:

```bash
mxcli init /path/to/my-mendix-project

# or specify your tool(s)
mxcli init --tool cursor /path/to/my-mendix-project
mxcli init --tool claude --tool cursor /path/to/my-mendix-project
```

Both approaches create `AGENTS.md`, `.ai-context/` with skills, `.devcontainer/` for sandboxed development, and tool-specific config files. Open the project in VS Code / Cursor and reopen in Dev Container, then start your AI assistant:

```bash
claude  # or use Cursor, Continue.dev, etc.
```

### Supported AI Tools

| Tool | Config File | Description |
|------|------------|-------------|
| **Claude Code** | `.claude/`, `CLAUDE.md` | Full integration with skills and commands |
| **OpenCode** | `.opencode/`, `opencode.json` | Skills, commands, and lint rules |
| **Cursor** | `.cursorrules` | Compact MDL reference and command guide |
| **Continue.dev** | `.continue/config.json` | Custom commands and slash commands |
| **Windsurf** | `.windsurfrules` | Codeium's AI with MDL rules |
| **Aider** | `.aider.conf.yml` | Terminal-based AI pair programming |
| **Universal** | `AGENTS.md` | Works with all tools |

```bash
# list supported tools
mxcli init --list-tools

# add tool to existing project
mxcli add-tool cursor
```

## Installation

Download a pre-built binary from the [releases page](https://github.com/mendixlabs/mxcli/releases) — the assets are raw binaries named `mxcli-<os>-<arch>` (nothing to extract). While mxcli is fast-moving alpha, the rolling `nightly` build is recommended (new features land there first); pin a `vX.Y.Z` release for reproducibility:

```bash
# Linux/macOS — nightly
curl -fsSL -o mxcli \
  https://github.com/mendixlabs/mxcli/releases/download/nightly/mxcli-linux-amd64
chmod +x mxcli && sudo mv mxcli /usr/local/bin/
```

Or build from source (Go + Make — `make build` runs the ANTLR parser generation that `go install` can't):

```bash
git clone https://github.com/mendixlabs/mxcli.git
cd mxcli
make build
# binary is at ./bin/mxcli
```

> `go install …@latest` is not supported: the generated ANTLR parser isn't committed, so a module-source build fails. Use a pre-built binary or `make build`.

## Core Features

### Explore Project Structure

```bash
# list all modules
mxcli -p app.mpr -c "show modules"

# list entities in a module
mxcli -p app.mpr -c "show entities in MyModule"

# describe any element (module, entity, microflow, nanoflow, page, etc.)
mxcli describe -p app.mpr module MyModule
mxcli describe -p app.mpr entity MyModule.Customer
mxcli describe -p app.mpr microflow MyModule.ProcessOrder
mxcli describe -p app.mpr nanoflow MyModule.ValidateInput
mxcli describe -p app.mpr page MyModule.CustomerOverview
mxcli describe -p app.mpr json structure MyModule.CustomerResponse
```

### Full-Text Search

Search across validation messages, log messages, captions, labels, and MDL source:

```bash
# search for validation-related content
mxcli search -p app.mpr "validation"

# Pipe-friendly output (type<TAB>name per line)
mxcli search -p app.mpr "error" -q --format names

# json output for processing with jq
mxcli search -p app.mpr "Customer" -q --format json
```

Pipe to describe:
```bash
# describe the first matching microflow
mxcli search -p app.mpr "validation" -q --format names | head -1 | awk '{print $2}' | \
  xargs mxcli describe -p app.mpr microflow

# Process all matches
mxcli search -p app.mpr "error" -q --format names > results.txt
while IFS=$'\t' read -r type name; do
  mxcli describe -p app.mpr "$type" "$name"
done < results.txt
```

### Code Navigation

```bash
# find what calls a microflow
mxcli callers -p app.mpr MyModule.ProcessOrder
mxcli callers -p app.mpr MyModule.ProcessOrder --transitive

# find what a microflow calls
mxcli callees -p app.mpr MyModule.ProcessOrder

# find all references to an element
mxcli refs -p app.mpr MyModule.Customer

# Analyze impact of changing an element
mxcli impact -p app.mpr MyModule.Customer

# Assemble context for understanding code
mxcli context -p app.mpr MyModule.ProcessOrder --depth 3
```

### Widget Discovery and Bulk Updates

> **EXPERIMENTAL**: These commands are an untested proof-of-concept.
> Always use `dry run` first and backup your project before applying changes.

Find and update widget properties across pages and snippets:

```bash
# Discover widgets by type
mxcli -p app.mpr -c "show widgets where widgettype like '%combobox%'"

# filter by module
mxcli -p app.mpr -c "show widgets in MyModule"

# Preview changes (dry run)
mxcli -p app.mpr -c "update widgets set 'showLabel' = false where widgettype like '%DataGrid%' dry run"

# apply changes
mxcli -p app.mpr -c "update widgets set 'showLabel' = false, 'labelWidth' = 4 where widgettype like '%combobox%' in MyModule"
```

Requires `refresh catalog full` to populate the widgets table.

### Catalog Queries

SQL-based querying of project metadata:

```bash
# find microflows with many activities
mxcli -p app.mpr -c "select Name, ActivityCount from CATALOG.MICROFLOWS where ActivityCount > 10 ORDER by ActivityCount desc"

# find all entity usages
mxcli -p app.mpr -c "refresh catalog full; select SourceName, RefKind, TargetName from CATALOG.REFS where TargetName = 'MyModule.Customer'"

# search strings table directly
mxcli -p app.mpr -c "select * from CATALOG.STRINGS where strings match 'error' limit 10"
```

Available tables: `modules`, `entities`, `microflows`, `nanoflows`, `pages`, `snippets`, `enumerations`, `workflows`, `ACTIVITIES`, `widgets`, `REFS`, `PERMISSIONS`, `STRINGS`, `source`

### Linting

```bash
# lint a project
mxcli lint -p app.mpr

# sarif output for CI/GitHub integration
mxcli lint -p app.mpr --format sarif > results.sarif

# list available rules
mxcli lint -p app.mpr --list-rules

# Exclude modules
mxcli lint -p app.mpr --exclude System --exclude Administration
```

14 built-in Go rules (MPR001-MPR007, SEC001-SEC003, CONV011-CONV014) plus 27 bundled Starlark rules covering security (SEC004-SEC009), architecture (ARCH001-003), quality (QUAL001-004), design (DESIGN001), and Mendix best practice conventions (CONV001-CONV010, CONV015-CONV017). Custom `.star` rules in `.claude/lint-rules/` are loaded automatically.

### Best Practices Report

```bash
# generate a scored report (Markdown)
mxcli report -p app.mpr

# HTML report
mxcli report -p app.mpr --format html --output report.html

# json report for CI
mxcli report -p app.mpr --format json
```

The report evaluates the project across 6 categories (Security, Quality, Architecture, Performance, Naming, Design) with a 0-100 score per category and overall.

### Testing

Test microflows using MDL syntax with javadoc-style annotations:

```bash
# run tests
mxcli test tests/microflows.test.mdl -p app.mpr

# list tests without executing
mxcli test tests/ --list

# JUnit xml output for CI
mxcli test tests/ -p app.mpr --junit results.xml
```

Tests use `@test` and `@expect` annotations in `.test.mdl` or `.test.md` files. See `mxcli help test` for full syntax.

### Create and Modify

```bash
# execute MDL commands
mxcli -p app.mpr -c "create entity MyModule.Product (Name: string(200) not null, Price: decimal)"

# execute an MDL script file
mxcli -p app.mpr -c "execute script 'setup.mdl'"

# check MDL syntax before executing
mxcli check script.mdl

# check syntax and validate references
mxcli check script.mdl -p app.mpr --references

# Preview changes (diff against current state)
mxcli diff -p app.mpr changes.mdl
```

## MDL Language

MDL (Mendix Definition Language) is a SQL-like syntax for working with Mendix models:

```sql
-- Show project structure
show modules;
show entities in MyModule;
describe entity MyModule.Customer;
describe microflow MyModule.ProcessOrder;
describe page MyModule.CustomerOverview;

-- Create entities
create entity MyModule.Product (
  Name: string(200) not null,
  Price: decimal(10,2),
  IsActive: boolean default true
);

-- Create associations
create association MyModule.Order_Product
  from MyModule.Order to MyModule.Product
  type ReferenceSet;

-- Create pages
create page MyModule.Product_Edit
(
  params: { $Product: MyModule.Product },
  title: 'Edit Product',
  layout: Atlas_Core.PopupLayout
)
{
  dataview dvProduct (datasource: $Product) {
    textbox txtName (label: 'Name', attribute: Name)
    textbox txtPrice (label: 'Price', attribute: Price)
    checkbox cbActive (label: 'Active', attribute: IsActive)

    footer footer1 {
      actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: primary)
      actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
    }
  }
}

-- Security management
create module role MyModule.Admin description 'Full access';
create module role MyModule.Viewer description 'Read-only';
grant execute on microflow MyModule.ProcessOrder to MyModule.Admin;
grant view on page MyModule.Product_Edit to MyModule.Admin, MyModule.Viewer;
grant MyModule.Admin on MyModule.Product (create, delete, read *, write *);
grant MyModule.Viewer on MyModule.Product (read *);
create user role AppAdmin (MyModule.Admin) manage all roles;
alter project security level production;
show security matrix in MyModule;

-- Search
search 'validation';

-- Code navigation
show callers of MyModule.ProcessOrder transitive;
show references to MyModule.Customer;
show impact of MyModule.Customer;
show context of MyModule.ProcessOrder depth 3;

-- Widget discovery and bulk updates
show widgets where widgettype like '%combobox%';
update widgets set 'showLabel' = false where widgettype like '%DataGrid%' dry run;
```

Run `mxcli syntax` for MDL syntax reference, or `mxcli syntax <topic>` for specific topics:
- `mxcli syntax keywords` - Reserved keywords
- `mxcli syntax entity` - Entity creation syntax
- `mxcli syntax microflow` - Microflow creation syntax
- `mxcli syntax page` - Page creation syntax
- `mxcli syntax search` - Full-text search syntax

## AI Assistant Integration

The `mxcli init` command sets up a Mendix project for AI-assisted development:

```bash
# default: Claude Code + universal docs
mxcli init /path/to/my-mendix-project

# Specify tool(s)
mxcli init --tool cursor /path/to/my-mendix-project
mxcli init --tool claude --tool cursor /path/to/my-mendix-project

# all tools
mxcli init --all-tools /path/to/my-mendix-project

# add tool to existing project
mxcli add-tool cursor
```

### What Gets Created

**Universal (all tools):**
- `AGENTS.md` - Comprehensive guide for AI assistants
- `.ai-context/skills/` - MDL pattern guides (write-microflows.md, create-page.md, etc.)
- `.ai-context/examples/` - Example MDL scripts

**Tool-Specific:**
- **Claude Code**: `.claude/settings.json`, `CLAUDE.md`, commands, lint-rules, skills
- **Cursor**: `.cursorrules` - Compact MDL reference
- **Continue.dev**: `.continue/config.json` - Custom commands and slash commands
- **Windsurf**: `.windsurfrules` - MDL rules for Codeium
- **Aider**: `.aider.conf.yml` - YAML config for Aider

**VS Code Extension** (auto-installed with Claude):
- Syntax highlighting and diagnostics
- Hover and go-to-definition
- Code completion
- Context menu commands

### Dev Container

`mxcli init` generates a `.devcontainer/` configuration that provides a sandboxed development environment. This is the recommended way to run AI coding agents — it limits their access to just the project files.

**What's installed in the dev container:**

| Component | Purpose |
|-----------|---------|
| **mxcli** | Mendix CLI for AI-assisted development (copied into project) |
| **MxBuild / mx** | Mendix project validation and building (`~/.mxcli/mxbuild/`) |
| **JDK 21** (Adoptium) | Required by MxBuild |
| **Docker-in-Docker** | Running Mendix apps locally with `mxcli docker` |
| **Node.js** | Playwright testing support |
| **PostgreSQL client** | Database connectivity |
| **Claude Code** | AI coding assistant (auto-installed on container creation) |

**Key paths inside the dev container:**

```
~/.mxcli/mxbuild/{version}/modeler/mx    # mx check / mx build
~/.mxcli/runtime/{version}/               # Mendix runtime (auto-downloaded)
./mxcli                                    # project-local mxcli binary
```

MxBuild is auto-downloaded on first use (via `mxcli setup mxbuild -p app.mpr` or `mxcli docker build`). To validate a project:

```bash
# Auto-download mxbuild and check project
mxcli setup mxbuild -p app.mpr
~/.mxcli/mxbuild/*/modeler/mx check app.mpr

# or use the integrated command
mxcli docker check -p app.mpr
```

### Usage

After initialization, open the project in VS Code or Cursor and **reopen in Dev Container**, then start your AI assistant:
```bash
claude              # Claude Code
# or use Cursor, Continue.dev, Windsurf, etc.
```

The AI assistant will have access to:
- MDL command reference in `AGENTS.md`
- Pattern guides in `.ai-context/skills/`
- Tool-specific configuration
- Full project context via `mxcli` commands

## VS Code Extension

The MDL extension for VS Code provides a rich editing experience for `.mdl` files:

- **Syntax highlighting** and **parse diagnostics** as you type
- **Semantic diagnostics** on save (validates references against the project)
- **Code completion** with context-aware keyword and snippet suggestions
- **Hover** over `Module.Name` references to see their MDL definition
- **Go-to-definition** (Ctrl+click) to open element source as a virtual document
- **Document outline** and **folding** for MDL statements and blocks
- **Context menu** commands: Run File, Run Selection, Check File

The extension is automatically installed by `mxcli init`. To install manually:
```bash
make vscode-install
# or: code --install-extension vscode-mdl/vscode-mdl-*.vsix
```

**Settings:**
- `mdl.mxcliPath` - Path to the mxcli executable (default: `mxcli`)
- `mdl.mprPath` - Path to `.mpr` file (auto-discovered if empty)

## Code Quality Monitoring

The `source_tree` tool provides a visual overview of the codebase, showing file sizes, dependency tiers, and optional quality metrics:

```bash
# basic source tree (file sizes + dependency depth)
go run ./cmd/source_tree

# with function metrics (count, longest function per file)
go run ./cmd/source_tree --fn

# with intra-file duplication detection
go run ./cmd/source_tree --dup

# with churn (commit frequency per file)
go run ./cmd/source_tree --churn

# all metrics at once
go run ./cmd/source_tree --all

# with test coverage (runs tests first if no coverage.out exists)
go run ./cmd/source_tree --cover
```

Output is color-coded by severity (green/yellow/orange/red) for each metric, making it easy to spot files that need attention.

## Building from Source

```bash
# Prerequisites: Go 1.24+, Make

# build
make build

# run tests
make test

# build release binaries for all platforms
make release

# Regenerate parser (requires ANTLR4)
make grammar
```

## Compatibility

- **Mendix versions**: Studio Pro 8.x, 9.x, 10.x, 11.x
- **MPR formats**: Both v1 and v2 (with mprcontents folder)
- **Platforms**: Linux, macOS, Windows (amd64 and arm64)

## Go Library

`mxcli` is built on a Go library for reading and modifying Mendix projects. If you want to use the library directly:

```go
import "github.com/mendixlabs/mxcli"

// read a project
reader, _ := modelsdk.Open("/path/to/app.mpr")
modules, _ := reader.ListModules()
dm, _ := reader.GetDomainModel(modules[0].ID)

// write to a project
writer, _ := modelsdk.OpenForWriting("/path/to/app.mpr")
entity := modelsdk.NewEntity("Customer")
writer.CreateEntity(dm.ID, entity)
```

See [docs/GO_LIBRARY.md](docs/GO_LIBRARY.md) for full API documentation.

## License

Apache License 2.0 - See [LICENSE](LICENSE) for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
