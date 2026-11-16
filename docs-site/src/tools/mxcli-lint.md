# mxcli lint

The `mxcli lint` command runs all lint rules (built-in Go rules and Starlark rules) against a Mendix project and reports findings.

## Basic Usage

```bash
# Lint a project
mxcli lint -p app.mpr

# With colored terminal output
mxcli lint -p app.mpr --color
```

## Output Formats

### Text (Default)

Human-readable output with rule IDs, severity, and messages:

```bash
mxcli lint -p app.mpr
```

### JSON

Machine-readable JSON output for CI integration:

```bash
mxcli lint -p app.mpr --format json
```

### SARIF

SARIF (Static Analysis Results Interchange Format) output for GitHub Code Scanning and other SARIF-compatible tools:

```bash
mxcli lint -p app.mpr --format sarif > results.sarif
```

This file can be uploaded to GitHub as a code scanning result.

## Options

### List Available Rules

```bash
mxcli lint -p app.mpr --list-rules
```

Shows all available rules with their IDs, names, severity, and source (built-in or Starlark).

### Exclude Modules

System and marketplace modules often trigger false positives. Exclude them:

```bash
mxcli lint -p app.mpr --exclude System --exclude Administration
```

### Colored Output

```bash
mxcli lint -p app.mpr --color
```

## Baseline: Report Only New Findings

On a mature app the existing backlog buries the findings about the change you
just made. Record a baseline once, commit it, and compare against it afterwards:

```bash
# Record today's findings
mxcli lint -p app.mpr --baseline lint-baseline.json --update-baseline

# Later: report only what is not in the baseline
mxcli lint -p app.mpr --baseline lint-baseline.json
```

The comparison prints a summary on stderr, so `--format json` and `--format sarif`
stay clean:

```
Lint baseline lint-baseline.json: 1 new, 1518 already known, 0 fixed
```

- Only the **new** findings are reported, and the exit code follows them alone.
- A finding is matched on its **rule and document** (the document's id where the
  rule gives one, so renaming a document keeps its findings), not on its message.
  Rewording a rule therefore does not make its findings new, while a further
  finding of the same rule in the same document is.
- A rule that **fails to run** is never absorbed by the baseline.
- A run scoped with `--modules`, `--documents` or `--rules` sees only part of the
  project: it reports new findings but not how many were fixed, and
  `--update-baseline` refuses it, since recording it would drop every finding
  outside the scope.
- A missing or unreadable baseline file is an error, not "everything is new".

The file is sorted JSON, so re-recording an unchanged project rewrites the same
bytes and a committed baseline diffs only where findings changed.

## Accepted Findings: Decisions in the Project Brain

Accepting a finding is a project decision, so it is recorded in the
[project brain](project-brain.md) with its reason, on the element it is about:

```bash
mxcli brain capture "Beperking has no entity access rules on purpose" \
  --accepts "SEC001:read only through DS_Beperkingen_Inzien, which logs every view" \
  -a @Dossier.Beperking -p app.mpr
mxcli brain capture "Demo users stay on in the demo app" --accepts "SEC003:never deployed" -p app.mpr
```

That writes ``Accepts lint `SEC001` on `@Dossier.Beperking`: …`` in the decision.
An unanchored decision accepts a project-wide finding (``on `project` ``); a module
anchor accepts the rule anywhere in that module.

`mxcli lint` then leaves accepted findings out of the open findings, the summary
and the exit code, and says how many there are on stderr; `--show-accepted` lists
each with the decision that accepts it. An acceptance whose rule ran but no longer
fires on its element is reported as **stale**, so the decision does not outlive the
finding. The scored `mxcli report` counts accepted findings apart and does not score
them, and `mxcli report security` shows a rule's verdict as *accepted (brain …)*.
`brain check` refuses an acceptance on an open question or requirement, on an element
that does not resolve, or naming a rule lint does not have.

## CI Integration

### GitHub Actions

```yaml
- name: Lint Mendix project
  run: |
    mxcli lint -p app.mpr --format sarif > results.sarif

- name: Upload SARIF
  uses: github/codeql-action/upload-sarif@v3
  with:
    sarif_file: results.sarif
```

### JSON Processing

```bash
# Count findings by severity
mxcli lint -p app.mpr --format json | jq 'group_by(.severity) | map({severity: .[0].severity, count: length})'

# Filter for errors only
mxcli lint -p app.mpr --format json | jq '[.[] | select(.severity == "error")]'
```

## Rules That Fail

A Starlark rule that reads a struct field this mxcli does not expose — usually a
rule written for a newer mxcli — is reported at **info** level as
`rule <ID> needs a newer mxcli (<detail>)`, not as an error. Any other rule
failure is an error (`Starlark rule error: …`). A rule's configured severity in
`lint-config.yaml` applies to its findings, never to its failure. `mxcli report`
keeps all rule failures out of the score.

An option in `lint-config.yaml` that a rule cannot use — for example a number
where the rule expects a list of prefixes — is reported at **warning** level as
`lint-config.yaml: rules.<ID>.options.<key> … the rule ran with its default`.
The rule still runs, on its built-in default, and the warning is kept out of the
score like any other rule failure.

A rule file that does not load at all (for example, one calling a builtin this
mxcli lacks) is skipped with a warning on stderr.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | No findings |
| 1 | Findings reported |
| 2 | Error running linter |
