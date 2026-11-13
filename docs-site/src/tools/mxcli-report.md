# mxcli report

The `mxcli report` command generates a scored best practices report for a Mendix project. It runs all lint rules and aggregates findings into a category-based scorecard.

## Basic Usage

```bash
# Generate a report (text output to terminal)
mxcli report -p app.mpr
```

## Scoring Selected Modules

```bash
# score only your own modules
mxcli report -p app.mpr --modules Sales,Orders
mxcli report -p app.mpr -m Sales -m Orders
```

`--modules` (`-m`, as on `mxcli lint`) scores the named modules and nothing else:
the rules skip every other module, and only findings **located in a selected
module** count toward the score. Findings that belong to no module — project
security, user-role mappings — are left out, because they would weigh the same
on every module's score. The report names the selection under the date, so a
module score is not read as the project's. `--exclude` and `excludeModules` in
`lint-config.yaml` still win over `--modules`; a module named by both is warned
about and not scored.

## Output Formats

### Markdown

```bash
mxcli report -p app.mpr --format markdown
mxcli report -p app.mpr --format markdown --output report.md
```

### HTML

Visual report with color-coded scoring:

```bash
mxcli report -p app.mpr --format html --output report.html
```

### JSON

Machine-readable report for CI pipelines:

```bash
mxcli report -p app.mpr --format json
```

## Scoring Categories

The report scores the project across 6 categories on a 0-100 scale:

| Category | What It Measures |
|----------|-----------------|
| **Security** | Access rules, password policy, demo users, PII exposure |
| **Quality** | Documentation coverage, complexity, orphaned elements |
| **Architecture** | Module coupling, data access patterns, business keys |
| **Performance** | Commit-in-loop, query patterns |
| **Naming** | Entity, attribute, microflow, and page naming conventions |
| **Design** | Entity size, attribute counts, association patterns |

Each category shows:
- A score from 0 to 100
- Number of findings in that category
- Specific rule violations with affected elements

## Rules That Could Not Run

A lint rule that fails is a problem with the tooling, not the project, so it is
**not counted** in the score, the summary or any category. The report lists it
in its own section ("Rules That Could Not Run"; `ruleFailures` in JSON).

The common case is a rule written for a newer mxcli — one that reads a field
this binary's catalog does not expose. It is reported at info level as
`rule QUAL004 needs a newer mxcli ("microflow" struct has no .document_noun_title attribute)`.
Any other failure is reported as a `Starlark rule error`. If the project's
tooling was written by a newer mxcli, `mxcli` warns about it on every command;
see [Syncing with Updates](../ide/syncing.md#the-version-stamp).

## Writing Reports to Files

```bash
# Write HTML report
mxcli report -p app.mpr --format html --output report.html

# Write Markdown report
mxcli report -p app.mpr --format markdown --output report.md

# Write JSON report
mxcli report -p app.mpr --format json --output report.json
```

## CI Integration

Use the JSON format to fail CI pipelines when scores drop below a threshold:

```bash
# Check if overall score is above 70
SCORE=$(mxcli report -p app.mpr --format json | jq '.overallScore')
if [ "$SCORE" -lt 70 ]; then
  echo "Quality score $SCORE is below threshold 70"
  exit 1
fi
```

For reports that answer one question instead of scoring — architecture, tests,
security, performance and traceability — see [Project Reports](project-reports.md).
