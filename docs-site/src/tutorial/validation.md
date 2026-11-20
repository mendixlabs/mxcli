# Validating with mxcli check

You've created entities, microflows, and pages. Before opening your project in Studio Pro, you should validate that everything is correct. mxcli provides three levels of validation, each catching different kinds of problems.

## Level 1: Syntax check (no project needed)

The fastest check. It parses your MDL script and reports syntax errors without touching any `.mpr` file:

```bash
mxcli check script.mdl --no-project
```

Without `--no-project`, a plain `mxcli check script.mdl` run from a directory holding exactly one `.mpr` auto-detects it (it prints `Using project: ...`) and goes on to Level 2 against it. `--no-project` gives the syntax-only check wherever you run it.

This catches:

- Typos in keywords (`CRETE` instead of `CREATE`)
- Missing semicolons or parentheses
- Invalid attribute type syntax (`String` without a length)
- Anti-patterns like empty list variables used as loop sources
- Nested loops that should use `RETRIEVE` for lookups

You don't even need a project file for this. It's a pure syntax and structure check, so it runs instantly.

### Checking inline commands

You can also check a single statement:

```bash
mxcli check -c "CREATE PERSISTENT ENTITY MyModule.Product (Name: String(200));"
```

## Level 2: Syntax + reference validation

Add `-p` and `--references` to also verify that names in your script resolve to real elements in the project:

```bash
mxcli check script.mdl -p app.mpr --references
```

This catches everything Level 1 catches, plus:

- References to entities that don't exist (`MyModule.NonExistent`)
- References to attributes that don't exist on an entity
- Microflow calls to non-existent microflows
- Page layouts that don't exist in the project
- Association endpoints pointing to missing entities
- A plain `create` whose name already exists in the project
- A statement the project's Mendix version cannot store (`MDL-VERSION01`, e.g. `create view entity` below 10.21) — the same version gates `exec` refuses with

This is the check you should run before executing a script. It's fast (reads the project but doesn't modify it) and catches most mistakes.

`mxcli exec` runs the same semantic checks before it writes anything, and refuses a script with an error. It prints errors and warnings in full but only counts the info notes (`2 info notes not shown — run mxcli check script.mdl to see them`), so a re-run stays readable; `check` prints every note, and `exec --verbose` does too.

References inside an **excluded** document (`@excluded`, or a page or snippet
that stays excluded because its stored namesake is) are reported as
`Reference warnings` rather than errors, because Mendix does not validate
excluded documents. A missing *data-source flow* is a warning too, but only when
every binding inside that container is qualified (`Module.Entity.Attribute`) —
`describe page` writes them that way there. The widgets bind against the entity
the flow returns, so with the flow missing a bare binding cannot be resolved and
the check fails, naming the widget. A missing entity still fails the check.

### Name conflicts with the project

A plain `create` of something the project already has is reported here rather
than left to `exec`:

```
statement 4: association already exists in project: Sales.Order_Customer — use CREATE OR MODIFY to update it
```

This matters because `exec` stops at the *first* such statement, having already
written the ones before it — so a script that only fails at execution time
leaves the project half-modified. `--references` reports every conflict in the
script up front, before anything is written.

Three spellings say "it is fine if this already exists", and none of them is
reported as a conflict:

| Form | What exec does |
|------|----------------|
| `create or modify X` | rewrites the element |
| `create or replace X` | rewrites the element |
| `create X if not exists` | leaves the stored element untouched |

`create module M;` is also never reported: it is a no-op when the module exists,
which is why it can safely open every script.

## Level 3: Full project validation with mx check

After executing your script, validate the entire project using the Mendix toolchain:

```bash
mxcli setup mxbuild -p app.mpr   # once per Mendix version
mxcli docker check -p app.mpr
```

This runs Mendix's own `mx check` tool on a temporary copy of the project. It performs the same validation that Studio Pro does when you open a project, catching issues that mxcli's parser cannot detect:

- BSON serialization errors (malformed internal data)
- Security configuration problems (missing access rules, CE0066 errors)
- Widget definition mismatches (CE0463 errors), each labelled either a *stale widget definition*, which `mxcli fix widgets` clears, or one that *persists after update-widgets*
- Missing required properties on widgets
- Broken cross-references between documents

`docker check` does not download MxBuild, and does not use Docker: the `docker` in its name groups it with `docker build`. It runs the `mx` of an MxBuild already on the machine, as a local process, and stops with `mx not found` when there is none. `mxcli setup mxbuild -p app.mpr` downloads the project's version once and caches it under `~/.mxcli/mxbuild/`; `mxcli new` and `mxcli docker build` download it too.

### With a local Mendix installation

If you have `mx` installed locally (e.g., from a Mendix installation), the check finds it on PATH or in the Studio Pro installation, and `setup mxbuild` is not needed. To choose a specific one:

```bash
mxcli docker check -p app.mpr --mxbuild-path /path/to/mendix
```

## The recommended workflow

Here's the workflow you should follow for every change:

```
Write MDL --> Check syntax --> Execute --> Docker check --> Open in Studio Pro
```

In practice:

```bash
# 1. Write your MDL in a script file
cat > changes.mdl << 'EOF'
CREATE OR MODIFY PERSISTENT ENTITY MyModule.Product (
    Name: String(200) NOT NULL,
    Price: Decimal,
    IsActive: Boolean DEFAULT true
);

CREATE OR MODIFY MICROFLOW MyModule.CreateProduct(
    DECLARE $Name: String,
    DECLARE $Price: Decimal
)
RETURN MyModule.Product
BEGIN
    CREATE $Product: MyModule.Product (
        Name = $Name,
        Price = $Price,
        IsActive = true
    );
    COMMIT $Product;
    RETURN $Product;
END;
EOF

# 2. Check syntax (fast, no project used)
mxcli check changes.mdl --no-project

# 3. Check references (needs project, still fast)
mxcli check changes.mdl -p app.mpr --references

# 4. Execute against the project
mxcli exec changes.mdl -p app.mpr

# 5. Full validation
mxcli docker check -p app.mpr

# 6. Open in Studio Pro and verify visually
```

## Previewing changes with diff

Before executing, you can preview what a script would change:

```bash
mxcli diff -p app.mpr changes.mdl
```

This runs the script with exec's own code on a scratch copy of the project and shows every document exec would create, modify or remove, each as its `describe` before and after. It does not modify the project.

## What mxcli check catches automatically

Beyond basic syntax, `mxcli check` includes built-in anti-pattern detection:

| Pattern | Problem | What check reports |
|---------|---------|-------------------|
| `DECLARE $Items List of ... = empty` followed by `LOOP $Item IN $Items` | Looping over an empty list does nothing | Warning: empty list variable used as loop source |
| Nested `LOOP` inside `LOOP` for list matching | O(N^2) performance | Warning: use RETRIEVE from list instead |
| Missing `RETURN` at end of flow path | Microflow won't compile | Error: missing return statement |

## Linting for deeper analysis

For a broader set of checks across the entire project (not just a single script), use the linter:

```bash
mxcli lint -p app.mpr
```

This runs 19 built-in rules plus 31 Starlark rules covering security, architecture, quality, and naming conventions. See `mxcli lint --list-rules` for the full list.

For CI/CD integration, output in SARIF format:

```bash
mxcli lint -p app.mpr --format sarif > results.sarif
```
