# 5-Minute Quickstart

Get from zero to modifying a Mendix project in 5 minutes. No prior MDL knowledge needed.

## 1. Install mxcli

**Option A: Zero install (Playground)**

[![Open in GitHub Codespaces](https://github.com/codespaces/badge.svg)](https://codespaces.new/mendixlabs/mxcli-playground)

Open the [mxcli Playground](https://github.com/mendixlabs/mxcli-playground) in a Codespace -- mxcli, a sample project, and example scripts are pre-installed. Skip to step 3.

**Option B: Binary download**

Release assets are raw binaries named `mxcli-<os>-<arch>` (nothing to extract). Use the rolling `nightly` build while mxcli is alpha:

```bash
# macOS / Linux
curl -fsSL -o mxcli \
  https://github.com/mendixlabs/mxcli/releases/download/nightly/mxcli-linux-amd64
chmod +x mxcli && sudo mv mxcli /usr/local/bin/
```

**Option C: Build from source** (Go + Make)

```bash
git clone https://github.com/mendixlabs/mxcli.git && cd mxcli && make build
# binary at ./bin/mxcli  (go install @latest doesn't work — see Installation)
```

Verify: `mxcli --version` should print the version number.

See [Installation](installation.md) for all options including Dev Containers.

## 2. Open your project

```bash
mxcli -p /path/to/your-app.mpr
```

You'll see:
```
Connected to: your-app.mpr (Mendix 11.6.3)
MDL REPL - Mendix Definition Language
mdl>
```

> **Don't have a Mendix project?** Create one with `mxcli new test-app --version 11.8.0` — this downloads MxBuild, creates a blank project, and sets up all tooling automatically.

## 3. Explore what's there

```sql
-- List all modules
LIST MODULES;

-- List entities in a module
LIST ENTITIES IN MyFirstModule;

-- See the full structure at a glance
DESCRIBE STRUCTURE;
```

## 4. Create something

```sql
-- Create a new entity
CREATE PERSISTENT ENTITY MyFirstModule.Customer (
  Name: String(200) NOT NULL,
  Email: String(200),
  IsActive: Boolean DEFAULT true
);
```

Expected output:
```
Created entity: MyFirstModule.Customer
```

## 5. Verify it works

```sql
-- See what you created
DESCRIBE ENTITY MyFirstModule.Customer;
```

Output:
```
@Position(100, 100)
create or modify persistent entity MyFirstModule.Customer (
  Name: String(200) not null,
  Email: String(200),
  IsActive: Boolean default true
);
```

Open the project in Studio Pro -- your entity is there.

## 6. Create a microflow

```sql
CREATE MICROFLOW MyFirstModule.ACT_DeactivateCustomer (
  $Customer: MyFirstModule.Customer
)
BEGIN
  CHANGE $Customer (IsActive = false);
  COMMIT $Customer;
END;
```

## 7. Validate

```bash
# Check your project for errors (from a separate terminal)
mxcli check -p /path/to/your-app.mpr
```

No errors? You're done. Open in Studio Pro and everything is there.

## What's next?

| I want to... | Read... |
|---|---|
| Explore my project deeper | [SHOW Commands](list-commands.md) |
| Create pages with widgets | [Creating a Page](create-page.md) |
| Use AI to generate code | [Claude Code Integration](claude-code.md) |
| Set up for a team | [Skills and CLAUDE.md](skills.md) |
| See everything mxcli can do | [Capabilities Overview](../reference/capabilities.md) |
| Customize AI generation | [Customizing AI Generation](../guides/customizing-ai.md) |

## Common issues

**"mx command not available"** -- Install mxbuild for validation:
```bash
mxcli setup mxbuild -p your-app.mpr
```

**"CGO not available"** -- mxcli uses pure Go SQLite. No C compiler needed. If you see CGO errors, ensure you're using the official pre-built binary or a `make build` from source.

**"refusing to write …: Studio Pro has this project open"** -- Close the project in Studio Pro before running mxcli write commands, then reopen it. Studio Pro does not reload the model from disk, so its next save would silently discard mxcli's changes. If Studio Pro is not running, the `.mpr.lock` was left behind by a crash: delete it, or pass `--force` to `exec`.

**Studio Pro converts a project made with `mxcli new` the first time it opens it** -- Expected, once. The template `mx create-project` ships can be one Studio Pro conversion behind the Studio Pro of the same version (11.16.1 adds a setup conversion and a `SetupSettings` unit that `mx` does not write), so the first open prompts and converts. Let it, and commit the result on its own before other changes, so it does not land in the diff of your first real one.
