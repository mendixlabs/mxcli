# mxcli report

The `mxcli report` command generates a scored best practices report for a Mendix project. It runs all lint rules and aggregates findings into a category-based scorecard.

## Basic Usage

```bash
# Generate a report (text output to terminal)
mxcli report -p app.mpr
```

## Does It Build?

The report runs Mendix's own consistency check (`mx check`) on a temporary copy of
the project and puts its verdict above the score:

| Builds | Meaning |
|---|---|
| `yes` | `mx check` found no errors |
| `no — N consistency error(s)` | the project cannot be deployed, whatever it scores |
| `not checked` | no mx for the project's Mendix version, or `--no-mx-check` |
| `unknown` | mx's output could not be read in full |

The check's errors, warnings and deprecations are listed under **Consistency
Check**, grouped by code with the documents they are in (a stale theme or widget
set gives hundreds of one code), and are not part of the score. They cover the
whole project, also with `--modules`. `mx` writes into the project it checks, so
the report runs it on a temporary copy; run `mx check` yourself only on a copy
too.

```bash
mxcli setup mxbuild -p app.mpr                          # mx for the project's version, once
mxcli report -p app.mpr --mx-check-results check.json   # the JSON 'mx check <app.mpr> -j check.json -w -d' wrote
mxcli report -p app.mpr --mxbuild-path /opt/mxbuild     # a specific mxbuild
mxcli report -p app.mpr --no-mx-check                   # lint only
```

## Reading the HTML Page

`--format html` explains the number rather than only showing it:

- **Categories:** each one's weight, the points it costs the overall score, and its
  penalty: how many points are left before the floor, or how far past it the findings
  go. Past the floor, fixing findings in that category does not move the score.
- **Rules:** every rule that found something, with how many of its findings brain
  decisions accept, the points fixing the rest would gain (scored again without them),
  and a verdict. Give a verdict with `--annotations`:

  ```yaml
  items:
    "rule:MPR006": {status: by design, note: "CSS-drawn dots, not empty containers"}
  ```
- **Recommendations:** the five rules whose fixes gain the most, rules with a verdict
  left out.

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
# Fail when the project does not build, or when the overall score is below 70. The score has one decimal (e.g. 82.5),
# so compare in jq: the shell's -lt only takes integers.
mxcli report -p app.mpr --format json -o quality.json
jq -e '.builds == true' quality.json > /dev/null || {
  echo "The project does not build, or mx check did not run: $(jq -r '.consistency.notRun // .consistency.incomplete // "consistency errors"' quality.json)"
  exit 1
}
jq -e '.overallScore >= 70' quality.json > /dev/null || {
  echo "Quality score $(jq .overallScore quality.json) is below threshold 70"
  exit 1
}
```

`.builds` is absent when the check was not run, so `== true` also fails a CI run
that has no mx for the project's version: run `mxcli setup mxbuild -p app.mpr`
first in the pipeline.

The `report-quality` skill covers how the score is computed, what it leaves out (the
bundled Starlark rules when the project has no `.claude/lint-rules/`), and how to
accept an intended finding in the project brain.

For reports that answer one question instead of scoring — architecture, tests,
security, performance, traceability and the software map — see
[Project Reports](project-reports.md).
