# Skills and CLAUDE.md

Skills teach AI assistants how to write correct MDL. Each skill covers a specific topic -- creating pages, writing microflows, managing security -- and contains syntax references, examples, and validation checklists. When an AI assistant needs to generate MDL, it reads the relevant skill first, which dramatically improves output quality.

## The format

Skills follow the [Agent Skills](https://agentskills.io) standard: each one is a
directory holding a `SKILL.md`, whose YAML frontmatter says what it covers and
when to use it.

```
.ai-context/skills/
└── write-microflows/
    └── SKILL.md
```

```markdown
---
name: write-microflows
description: Microflow syntax reference in MDL -- every activity type, control flow,
  expressions, and the mistakes that fail `mxcli check`. Use before writing any
  CREATE MICROFLOW, and when debugging a microflow syntax error.
---

# Mendix Microflow Skill
...
```

The `description` is what makes a skill findable. A tool that reads skills loads
every description up front -- a line or two each -- and pulls in the full body
only when one is actually needed. So the assistant knows the whole set exists
without any of them costing context until used, and without a hand-maintained
index that can fall out of step.

## Where skills live

`mxcli init` installs the same set in two places:

| Location | Used by |
|----------|---------|
| `.ai-context/skills/<name>/SKILL.md` | All tools -- the vendor-neutral copy, referenced by the generated OpenCode, Cursor, Continue, Windsurf and Aider configs |
| `.claude/skills/<name>/SKILL.md` | Claude Code, which discovers skills from this path automatically |

The `.claude/skills/` copy exists because that is the only directory Claude Code
scans, one level deep. Both copies are refreshed from the mxcli binary on every
session by `mxcli init --sync-skills`, so they cannot drift apart.

If you are upgrading a project created by an older mxcli, the flat `<name>.md`
files it used to write are retired automatically on the next refresh. Skill files
you added yourself are left alone.

## Available skills

`mxcli init` installs the following skill files:

| Skill File | Topic |
|------------|-------|
| `generate-domain-model/SKILL.md` | Entity, attribute, and association syntax |
| `write-microflows/SKILL.md` | Microflow syntax, activities, common mistakes |
| `create-page/SKILL.md` | Page and widget syntax reference |
| `alter-page/SKILL.md` | ALTER PAGE/SNIPPET for modifying existing pages |
| `overview-pages/SKILL.md` | CRUD page patterns (overview + edit) |
| `master-detail-pages/SKILL.md` | Master-detail page patterns |
| `manage-security/SKILL.md` | Module roles, user roles, access control, GRANT/REVOKE |
| `manage-navigation/SKILL.md` | Navigation profiles, home pages, menus |
| `demo-data/SKILL.md` | Mendix ID system, association storage, demo data insertion |
| `xpath-constraints/SKILL.md` | XPath syntax in WHERE clauses, nested predicates |
| `database-connections/SKILL.md` | External database connections from microflows |
| `check-syntax/SKILL.md` | Pre-flight validation checklist |
| `organize-project/SKILL.md` | Folders, MOVE command, project structure conventions |
| `test-microflows/SKILL.md` | Test annotations, file formats, Docker setup |
| `patterns-data-processing/SKILL.md` | Delta merge, batch processing, list operations |

## What a skill file contains

A typical skill file has four sections:

### 1. Syntax reference

The core MDL syntax for the topic, with all available options and keywords:

```sql
-- From write-microflows.md:
CREATE MICROFLOW Module.Name(
    $param: Module.Entity
) RETURNS Boolean AS $result
BEGIN
    -- activities here
END;
```

### 2. Examples

Complete, working MDL examples that the AI can use as templates:

```sql
-- From create-page.md:
CREATE PAGE Sales.Customer_Overview
(
  Title: 'Customers',
  Layout: Atlas_Core.Atlas_Default
)
{
  DATAGRID dgCustomers (
    DataSource: DATABASE FROM Sales.Customer SORT BY Name ASC
  ) {
    COLUMN (Attribute: Name, Caption: 'Name')
  }
};
```

### 3. Common mistakes

A list of errors the AI should avoid. For example, `write-microflows.md` warns against creating empty list variables as loop sources, and `create-page.md` documents required widget properties that are easy to forget.

### 4. Validation checklist

Steps the AI should follow after writing MDL to confirm correctness:

```bash
# Syntax check only (--no-project: do not auto-detect an .mpr in this directory)
./mxcli check script.mdl --no-project

# Syntax + reference validation
./mxcli check script.mdl -p app.mpr --references
```

## CLAUDE.md

The `CLAUDE.md` file is specific to Claude Code. It sits in the project root and is automatically read by Claude when it starts. It provides:

- **Project overview** -- what the project is, which modules exist, and what mxcli is
- **Available commands** -- a summary of mxcli commands Claude can use
- **Rules** -- instructions like "always read the relevant skill file before writing MDL" and "always validate with `mxcli check` before executing"
- **Conventions** -- project-specific naming conventions, module structure, etc.

Think of `CLAUDE.md` as the "system prompt" for Claude Code in the context of your project. It sets the tone and establishes guardrails.

## AGENTS.md

`AGENTS.md` serves the same purpose as `CLAUDE.md` but in a universal format. It is always created by `mxcli init`, regardless of which tool you selected. AI tools that don't have their own config format (or that read markdown files from the project root) will pick up `AGENTS.md` automatically.

## Adding custom skills

You can create your own skills to teach the AI about your project's patterns and conventions. Add a directory with a `SKILL.md` to `.ai-context/skills/` (and `.claude/skills/` if you use Claude Code):

```
.ai-context/skills/
├── write-microflows/SKILL.md         # Built-in (installed by mxcli init)
├── create-page/SKILL.md              # Built-in
├── our-naming-conventions/SKILL.md   # Custom: your team's naming rules
├── order-processing-pattern/SKILL.md # Custom: how orders work in your app
└── api-integration-guide/SKILL.md    # Custom: how to call external APIs
```

`mxcli init --sync-skills` only rewrites the skills mxcli itself ships, so your
own directories survive every upgrade. It also refreshes the bundled lint rules
and the mxcli section of `CLAUDE.md` — see [Syncing with Updates](../ide/syncing.md).

A custom skill is a markdown document with two lines of frontmatter. Write the
body the way you would explain something to a new team member, and write the
`description` so an assistant can tell from it alone whether this is the skill
for the job:

```markdown
---
name: order-processing-pattern
description: How orders are processed in this application -- validation rules,
  status enumeration, logging node and confirmation email. Use when writing or
  changing any microflow that touches an Order.
---

# Order Processing Pattern

When creating microflows that process orders in our application, follow these rules:

1. Always validate the order has at least one OrderLine
2. Use the Sales.OrderStatus enumeration for status tracking
3. Log to the 'OrderProcessing' node at INFO level
4. Send a confirmation email via Sales.SendNotification

## Example

\```sql
CREATE MICROFLOW Sales.ACT_Order_Submit($order: Sales.Order)
RETURNS Boolean AS $success
BEGIN
    -- validation, processing, notification...
END;
\```
```

The AI will read your custom skills alongside the built-in ones, learning your project's specific patterns.

## How the AI uses skills

The workflow is straightforward:

1. You ask for a change: "Create a page that shows all orders"
2. The AI determines which skill is relevant (in this case, `create-page.md` and `overview-pages.md`)
3. The AI reads the skill files
4. The AI writes MDL following the syntax and patterns in the skill
5. The AI validates with `mxcli check`
6. The AI executes the script

Skills act as a guardrail. Without them, AI assistants tend to guess at MDL syntax and get details wrong. With skills, the AI has a precise reference to follow, and the output is correct far more often.

## Next steps

Now that you understand how skills guide AI behavior, see [The MDL + AI Workflow](mdl-ai-workflow.md) for the complete recommended workflow from project initialization to review in Studio Pro.
