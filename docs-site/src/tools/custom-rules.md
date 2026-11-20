# Writing Custom Rules

You can extend the linter with your own rules written in Starlark, a Python-like
language. A rule is a `.star` file in the project's `.claude/lint-rules/`
directory; `mxcli lint` (and the `lint` statement) load every file there next to
the built-in rules.

The complete rule API — every query builtin, every field of the structs they
return and the values those fields take — is documented in the
**`write-lint-rules` skill**, `.claude/skills/mendix/write-lint-rules/SKILL.md`,
which `mxcli init` installs into the project. That file is the reference; this
page shows the shape of a rule.

## Rule file structure

A rule file defines its metadata as module-level constants and a `check()`
function that takes no arguments and returns a list of violations:

```python
# .claude/lint-rules/custom001_entity_prefix.star

RULE_ID = "CUSTOM001"
RULE_NAME = "EntityPrefix"
DESCRIPTION = "Entity names start with their module's abbreviation"
CATEGORY = "naming"
SEVERITY = "warning"            # error, warning, info or hint

def check():
    violations = []
    for entity in entities():
        prefix = entity.module_name[:3]
        if not entity.name.startswith(prefix):
            violations.append(violation(
                message = "Entity '%s' should start with '%s'" % (entity.name, prefix),
                location = location(
                    module = entity.module_name,
                    document_type = "entity",
                    document_name = entity.name,
                ),
                suggestion = "Rename to %s%s" % (prefix, entity.name),
            ))
    return violations
```

The project is read through builtins rather than a context argument:
`entities()`, `microflows()`, `pages()`, `widgets()`, `attributes_for(name)`,
`permissions_for(name)`, `refs_to(name)`, `xpath_expressions()` and more — the
skill lists them all. `violation(...)` and `location(...)` build the result.

## Catalog depth

Some builtins read data that only a full catalog build produces (`widgets()`,
`xpath_expressions()`, `activities_for()`, `flows_for()`, `permissions()`,
`permissions_for()`, `refs_to()` / `refs_from()`, a page's `widget_count`), and
the graph builtins (`cycles()`, `layer_of()`, …) need the communities build.
`mxcli lint` detects them in the rule's source and builds the catalog deep
enough. If the scan cannot see the call, declare it:

```python
REQUIRES = ["full"]             # or ["communities"]
```

## Severity levels

| Severity | Description |
|----------|-------------|
| `error` | Must be fixed; fails CI pipelines |
| `warning` | Should be fixed; potential issue (the default) |
| `info` | Informational; suggestion for improvement |
| `hint` | Lowest level; a nudge |

## Testing custom rules

```bash
# list the rules, including yours
mxcli lint -p app.mpr --list-rules

# run them
mxcli lint -p app.mpr
```

A rule file that fails to load is reported as skipped with the reason, and a
rule that raises an error at run time is reported as an `error` violation
carrying the Starlark message.

## Best practices

- Use a unique rule ID prefix (e.g. `CUSTOM001`) to avoid clashing with the
  built-in rules.
- Write messages that say what to fix, and set `suggestion` where there is one.
- Compare fields against the values the skill documents — a comparison against a
  value the API never returns matches nothing and reports a clean pass.
- Try the rule on a real project before relying on it.
