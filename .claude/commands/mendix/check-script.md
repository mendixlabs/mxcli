---
description: Validate an MDL script before executing it
argument-hint: <script.mdl>
---

# Check Script

Validate an MDL script before executing it.

## Commands

```bash
# Syntax check only (--no-project: do not auto-detect an .mpr in this directory)
mxcli check script.mdl --no-project

# Syntax + reference validation (checks entities/modules exist)
mxcli check script.mdl -p app.mpr --references
```

A script starts with `mdl 1;`. A headerless script is checked as the older `mdl 0`;
upgrade it with `mxcli fmt --upgrade --header -p app.mpr -w script.mdl` before editing it.

## What It Checks

**Syntax check (always runs):**
- MDL grammar correctness
- Proper statement structure
- Valid keywords and types

**Microflow body validation (always runs, no project needed):**
- RETURN must provide a value when microflow declares a return type
- RETURN must not provide a value on void microflows (except `RETURN empty`)
- Scalar literals (string, integer, boolean) cannot be returned from entity-typed microflows
- All code paths must end with RETURN for non-void microflows
- Variables declared inside IF/ELSE branches or ON ERROR bodies cannot be used outside the branch
- VALIDATION FEEDBACK must have a non-empty message (CE0091)

**Reference check (with --references, requires -p):**
- Module references exist
- Entity references exist
- Association references exist
- Skips references to objects created in the same script
- Variables must be declared before use with SET
- Validates all branches (IF/ELSE) and loops
- Tracks variable declarations from parameters, DECLARE, CREATE, RETRIEVE, CALL

## Example Output

```
$ mxcli check domain-model.mdl -p app.mpr --references
Checking: domain-model.mdl
✓ Syntax OK (15 statements)
✓ References OK

$ mxcli check broken.mdl
Checking: broken.mdl
✗ Line 5: missing ';' at 'CREATE'
✗ Line 12: unknown type 'Strin'
Found 2 errors

$ mxcli check microflow.mdl
Checking: microflow.mdl
✓ Syntax OK (3 statements)
Validation errors:
  - statement 1 (Module.MyMicroflow): RETURN requires a value because microflow returns Boolean
  - statement 2 (Module.OtherFlow): microflow returns String but not all code paths have a RETURN statement
  - statement 3 (Module.ScopeIssue): variable '$X' is declared inside IF branch but used outside

$ mxcli check microflow.mdl -p app.mpr --references
Checking: microflow.mdl
✓ Syntax OK (1 statements)
Reference errors:
  statement 1: microflow 'Module.Name' has validation errors:
  - variable 'IsValid' is not declared. Use DECLARE IsValid: <Type> before using SET
✗ 1 reference error(s) found
```

## Tips

- Always check scripts before executing on production projects
- Use --references to catch typos in entity/module names
- The checker is smart about script-internal references
