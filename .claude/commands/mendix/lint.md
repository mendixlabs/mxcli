---
description: Check the project for naming, security and best-practice issues
argument-hint: [--format json|sarif]
---

# Lint Project

Check a Mendix project for common issues like naming convention violations, security gaps, and best practice deviations.

## Commands

```bash
# Basic lint
mxcli lint -p app.mpr

# With colored output
mxcli lint -p app.mpr --color

# JSON output
mxcli lint -p app.mpr --format json

# SARIF output (for CI/GitHub)
mxcli lint -p app.mpr --format sarif > results.sarif

# List available rules
mxcli lint -p app.mpr --list-rules

# Exclude modules
mxcli lint -p app.mpr --exclude System --exclude Administration

# Only findings not in a recorded baseline (record it once with --update-baseline)
mxcli lint -p app.mpr --baseline lint-baseline.json --update-baseline
mxcli lint -p app.mpr --baseline lint-baseline.json
```

## Built-in Go Rules

| Rule | Category | Description |
|------|----------|-------------|
| MPR001 | naming | NamingConvention - PascalCase with 21 microflow prefixes (ACT_, SUB_, DS_, VAL_, SCH_, IVK_, BCO_, ACO_, etc.) |
| MPR002 | quality | EmptyMicroflow - Microflows should have at least one activity |
| MPR003 | design | DomainModelSize - Max persistent entities per domain model |
| MPR004 | quality | ValidationFeedback - Validation feedback with empty message |
| MPR005 | quality | ImageSource - IMAGE widgets with no source configured |
| MPR006 | quality | EmptyContainer - Empty layout containers |
| MPR007 | security | PageNavigationSecurity - Navigation pages need allowed roles (CE0557); every user role must be able to open its home page (at Production security, else CE2729 per widget) |
| MPR012 | correctness | ReactUnsupportedWidget - static/dynamic image, drop-down, legacy data grid and reference selector are unsupported by the React client (CE0582; info on the classic client) |
| MPR013 | correctness | SnippetCallAppearance - Class/Style/DynamicClasses/design properties on a snippet call never reach the page (it renders no element) |
| SEC001 | security | NoEntityAccessRules - Persistent entities need access rules |
| SEC002 | security | WeakPasswordPolicy - Password minimum length should be 8+ |
| SEC003 | security | DemoUsersActive - Demo users should be off at Production security |
| SEC010 | security | PageEntityNotReadable - A page data widget lists an entity the user roles that open the page cannot read; at Production it renders empty, silently (info when only row-scoped) |
| SEC011 | security | XPathAssociationNotReadable - A page data source's XPath crosses an association a role that opens the page may not read; at Production the page fails to open ("No access rights for …") |
| CONV011 | performance | NoCommitInLoop - Commits inside loops (commit actions, or create/change with commit) cause N+1 issues |
| CONV012 | quality | ExclusiveSplitCaption - Exclusive splits need meaningful captions |
| CONV013 | quality | ErrorHandlingOnCalls - External calls (REST/WS/Java) need custom error handling |
| CONV014 | quality | NoContinueErrorHandling - Don't silently swallow errors with Continue |
| CONV020 | quality | CreatedObjectNeverPersisted - A microflow creates a persistent object without commit and never commits, returns or passes it on, so it is discarded |
| CONV021 | quality | CreateChangeThenCommit - A create/change without commit directly followed by a Commit of the same object; set Commit on the create/change instead |

## Bundled Starlark Rules

These are loaded automatically from `.claude/lint-rules/`:

| Rule | Category | Description |
|------|----------|-------------|
| SEC004 | security | GuestAccessEnabled - Review anonymous user entity access |
| SEC005 | security | StrictModeDisabled - XPath constraint enforcement off |
| SEC006 | security | PIIAttributesExposed - PII-sounding attributes need access rules |
| SEC007 | security | UnconstrainedAnonymousEntityRead - DIVD-2022-00019 detection |
| SEC008 | security | PIIUnconstrainedRead - PII entities readable without row scoping |
| SEC009 | security | MissingMemberRestriction - Large entities without attribute-level access |
| ARCH001 | architecture | CrossModuleDataAccess - Pages should use same-module entities |
| ARCH002 | architecture | DataChangesThroughMicroflows - Enforce microflow-based writes |
| ARCH003 | architecture | EntityBusinessKey - Persistent entities need a unique key |
| QUAL001 | quality | McCabeComplexity - Microflow cyclomatic complexity threshold |
| QUAL002 | quality | MissingDocumentation - Entities/microflows need documentation |
| QUAL003 | quality | LongMicroflows - Microflows with >25 activities |
| QUAL004 | quality | OrphanedElements - Unreferenced elements in the project |
| DESIGN001 | design | EntityAttributeCount - Entities with too many attributes |
| CONV001 | naming | BooleanNaming - Boolean attributes start with Is/Has/Can/Should/Was/Will |
| CONV002 | naming | NoEntityDefaultValues - Avoid attribute defaults, use microflows |
| CONV003 | naming | PageNamingSuffix - Pages end with _NewEdit/_View/_Overview/etc. |
| CONV004 | naming | EnumerationPrefix - Enumerations start with ENUM_ |
| CONV005 | naming | SnippetPrefix - Snippets start with SNIPPET_ |
| CONV006 | security | NoCreateDeleteRights - Use microflows for create/delete |
| CONV007 | security | XPathOnAllAccess - Entity access should have XPath constraints |
| CONV008 | security | ModuleRoleMapping - Each module role maps to one user role |
| CONV009 | quality | MaxMicroflowObjects - Microflows should have <= 15 activities |
| CONV010 | architecture | ACTMicroflowContent - ACT_ microflows should only have UI actions |
| CONV015 | quality | NoEntityValidationRules - Use VAL_ microflows instead |
| CONV016 | performance | NoEventHandlers - Use explicit microflow calls instead |
| CONV017 | performance | NoCalculatedAttributes - Use stored attributes updated by microflows |

## Best Practices Report

Generate a scored report with category breakdown:

```bash
# Markdown report (default)
mxcli report -p app.mpr

# HTML report
mxcli report -p app.mpr --format html --output report.html

# JSON report
mxcli report -p app.mpr --format json
```

The report scores the project across 6 categories: Security, Quality, Architecture, Performance, Naming, Design (0-100 each).

## Custom Starlark Rules

Place `.star` files in `.claude/lint-rules/` to add project-specific rules. They run alongside built-in rules. See the skill file `write-lint-rules.md` for the full API reference.

## Output Formats

- **text** (default) - Human-readable, grouped by module
- **json** - Machine-readable JSON
- **sarif** - SARIF 2.1.0 for GitHub Actions and IDE integration

## Example Output

```
Sales
-----
  ⚠ Entity name 'customer_info' should use PascalCase [MPR001]
      at Sales.customer_info
      → CustomerInfo

  ⚠ Microflow 'test' has no activities [MPR002]
      at Sales.test
      → Add activities or remove unused microflow

2 issues: 0 errors, 2 warnings, 0 info
```

## Tips

- The linter automatically builds the catalog if needed
- Use --exclude to skip system/generated modules
- SARIF output integrates with GitHub code scanning
- Exit code is 1 if any errors are found (not warnings)
- Use `mxcli report` for a comprehensive scored assessment
