# mxcli verify

`mxcli verify` is the one command that answers "did my change break anything?". It runs the
checks in cost order and reports only the findings that are **new** against a baseline, so on
a mature project the result is about the change just made, not the backlog.

| Stage | What it runs | Against |
|---|---|---|
| `check` | `mxcli check` on the script files you pass | nothing: every finding is the script's |
| `exec` | `mxcli exec` on those scripts; only with `--apply`, and only if `check` passed | nothing: it applies the change |
| `lint` | `mxcli lint` on the project | `verify-baseline/lint.json` |
| `mxcheck` | `mx check` on a copy of the project | `verify-baseline/mxcheck.json` |

`mx check` is the authority. `mxcli check` cannot yet predict everything it rejects, so a project
that passes `check` and `lint` can still fail `mxcheck`; that is why the stage is part of the gate.

## Usage

```bash
# Record the current state as the baseline (do this once, on a project you accept as it is)
mxcli verify -p app.mpr --update-baseline

# After a change you already applied
mxcli verify -p app.mpr
mxcli verify -p app.mpr --stages lint --format json

# Check, apply and verify a change in one call
mxcli verify -p app.mpr --apply change.mdl
mxcli verify -p app.mpr --apply change.mdl --through exec   # check and exec only
```

## Which gate list a project gets

The generated `CLAUDE.md` lists `verify` as a gate (in place of `mxcli lint` and `mxcli docker check`,
which it runs) only once the project has recorded its baselines, that is once `verify-baseline/lint.json`
exists. Without baselines `verify` reports every finding the project already had as new, and on a project
with errors it fails on the whole backlog, so a project keeps the separate gates until it has them.
The list follows the project and not the mxcli version: the session-start sync regenerates it every
session, so recording the baselines moves a project onto `verify` at the next session start
(`mxcli init --sync-skills .` does it at once), and removing them moves it back. Upgrading mxcli alone
changes nothing.

## How far it runs

You say how far. `--through <stage>` runs every stage up to and including that one, in pipeline
order `check`, `exec`, `lint`, `mxcheck`; `--stages` names an explicit set. Without either, `verify`
runs `lint` and `mxcheck` (and `check` when scripts are given), and with `--apply` it runs through
`mxcheck`. Iterate with `--through exec`, finish a change with the default.

Scripts are applied only with `--apply`. Without it they are checked and not applied, so `lint` and
`mxcheck` describe the project **without** them, and the result says so. With `--apply`:

- a failed `check` writes nothing and skips everything after it;
- a script `exec` refuses skips `lint` and `mxcheck`, which would otherwise judge a project without the change;
- several scripts are checked as one ordered set (each sees what the earlier ones create) and applied in
  order. `exec` is not transactional across files, so a failure part-way leaves the earlier ones applied
  and the finding says how far it got.

Commit `verify-baseline/`. It lives next to the `.mpr`, not under `.mxcli/` (which `mxcli init`
git-ignores): a baseline that is not committed is gone with the next fresh clone, and every run
then reports every finding as new. Without a baseline a stage reports every finding as new and
says so.

Every selected stage runs, so one result carries every stage's findings. `--fail-fast` stops at
the first stage that fails. `--fail-on warning` makes a new warning fail a stage (the default is
errors). `--format json` prints one document with each stage's status, duration and every new finding.

The text output lists only the new findings that would fail the gate at `--fail-on`, and counts the
rest by severity. A baseline taken on a blank app leaves most of a built app's lint findings "new"
(documentation, access-rule notes); listing them would bury the one that matters. When there are
any, the output ends with the command that accepts them, scoped to the stages that have them
(`--stages lint --update-baseline`), because `--update-baseline` records every selected stage.
Review them first: re-recording is how a finding becomes accepted.

## When lint cannot vouch for its result

The lint baseline records which rules ran and a fingerprint of the lint configuration. If a later run
did not run a rule the baseline holds findings for (a CI checkout without `.claude/lint-rules/`, a
disabled rule, a rule that failed with "could not run"), or the configuration differs, `lint` reports
those findings as **not checked**, never as fixed, and `verify` stops with an error (exit 2) naming
the rules instead of passing. A finding about a rule that could not run is not a finding about the
project and is not counted as new. Run `lint` where the project's rules and config are installed, or
re-record with `--update-baseline` if the change is intended. A baseline recorded before this was added
still works: the rules it holds findings for must have run.

The advice follows the cause. A rule that did not run needs the project's rules and config where `lint`
runs. A rule that ran into an error (a built-in rule can) usually runs on a second attempt, and the
stage shows the rule's own message. `--update-baseline` refuses (exit 2) while a rule cannot run, because
the baseline would hold none of that rule's findings and a re-record is what the failing run suggests:
run it again first.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | nothing new |
| 1 | usage error |
| 2 | a stage could not run (no `mx`, ...) and nothing failed |
| 16 + bits | a stage failed: 1 `check`, 2 `lint`, 4 `mxcheck`, 8 `exec` (18 = lint, 22 = lint and mxcheck, 24 = exec refused) |

A stage that could not run is never reported as passed.

## Why mx check needs special handling

`mx check` reports errors in stages: an earlier-stage error hides the later ones. A plain diff
would blame a change for an error it only uncovered, or call an error resolved because a new
earlier one hides it. So `mxcheck` sorts findings into:

- **new** — not in the baseline. This includes a later error uncovered in a microflow whose
  earlier baseline error is gone, because the change edited that flow and must leave it clean.
- **unmasked** — not in the baseline, but a baseline unresolved-reference error was hiding the
  whole project. Reported, and does not fail the gate unless `--strict`.
- **resolved** — in the baseline, gone, and nothing hides it.
- **hidden** — in the baseline, gone only because a new earlier-stage error hides it. Not resolved.

See the [proposal](https://github.com/mendixlabs/mxcli/blob/main/docs/11-proposals/PROPOSAL_verify_gate.md)
for the measurements behind this.
