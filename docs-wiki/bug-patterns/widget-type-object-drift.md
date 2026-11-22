---
title: Widget Type / Object Drift (CE0463)
category: bug-pattern
last-synced: 392cacd6
covers:
  - cmd/mxcli
  - mdl/backend
  - mdl/executor
sources:
  - .claude/skills/fix-issue/findings/mdl-executor/
  - .claude/skills/fix-issue/findings/cmd-mxcli/
  - .claude/skills/fix-issue/findings/sdk/
  - .claude/skills/fix-issue/findings/mdl-backend/
  - .claude/skills/diagnose-ce0463.md
  - .claude/skills/debug-bson.md
  - sdk/widgets/templates/README.md
  - mdl/backend/modelsdk/widget_write.go
---

> **Do not duplicate**: the elimination order, the two controls and the
> measurement traps are canonical in `.claude/skills/diagnose-ce0463.md`; the
> general BSON diff workflow is in `debug-bson.md`; template extraction is in
> `sdk/widgets/templates/README.md`; the per-instance fixes are in the findings.
> This page describes why the class behaves the way it does.

## What this is

A pluggable widget stores two coupled structures: the **type** (the PropertyTypes
schema — what properties exist) and the **object** (the WidgetObject — the values).
Studio Pro requires them to correspond exactly: every PropertyType needs its
WidgetProperty, every `TypePointer` must resolve, ordering must match, and no
field may appear that the reflection schema does not declare. Any deviation is
CE0463 *"the definition of this widget has changed"* — which on a master-detail
page cascades into CE3637 on the dependent DataView.

Nearly fifty findings carry this code, which makes it the most-reported single
error in the repo. It is expensive out of proportion to its frequency, and the
reason is in the wording: **CE0463 names the widget *version*, and is caused by
almost anything in the widget's stored BSON.** The error points at the one thing
that is usually fine.

## How it fits

**Two unrelated bugs wear this error, and separating them is the first move.**
Either the widget *package* changed after the widgets were authored — a stored
instance now carries a property the new definition dropped, which is what Studio
Pro's "Update all widgets" exists for and is **not an mxcli defect** — or the
widgets were authored against the current package and mxcli emitted something it
does not accept. The two are indistinguishable in `mx check` output. Two controls
tell them apart: whether Studio Pro's *own* template widgets fail alongside yours,
and whether `mx update-widgets` clears it. Skipping that step produces a confident
wrong answer, and has.

**The default measurement hid the class.** Until ako/mxcli#646, `mxcli docker
check` ran `mx update-widgets` on its copy first, to suppress exactly the
package-drift noise above — so it reported 0 errors on a project that genuinely had
a CE0463, which then failed `run --local`. A gate that normalises its input before
judging it answers a different question from the one asked. It now checks the
stored project and runs the second control itself, labelling each CE0463 *stale*
(the resync clears it) or *persists*; `--update-widgets` is the old behaviour, so
"docker check passed" in an older finding meant the resynced copy passed.
(`mxcli check` is not an alternative: it validates an MDL script and never invokes
mxbuild, so it cannot produce CE0463 at all.)

**The cause is usually a value, not the schema** — four of five fixes in one
release cycle were value-shaped while the error named the version: an empty
`TextTemplate` where Studio Pro stores the attribute name, an empty
`Forms$ClientTemplate` where Studio Pro stores `null`, a placeholder `" "` for an
unset string. Property-set drift against the package is real but a weak predictor:
one widget needs nineteen additions and passes, while another is byte-for-byte in
sync and fails.

**A difference from the reference is not a cause.** With one grid's type
byte-identical to `mx update-widgets` output, twenty-five value-level differences
remained; three were patched in isolation and none moved the error count. The diff
*bounds* the search, it does not rank it — budget for that rather than
pattern-matching the first plausible entry.

**A visibility rule must be evaluated against the configuration that will be
written, never the one the package declares.** The two diverge precisely on
unmapped properties, which is the only place the rule matters — which is how every
mxcli-authored Image widget failed a build while the package's own defaults said
it should not. The corollary for fixtures: a test whose helper restates the
default under test is not a fixture, it is the hypothesis wearing a test's
clothes.

**A rule the extractor cannot read fails silently, and only in the configuration
it governs.** An unread editorConfig hide becomes "always visible", so the
template's value is written wherever the property should be null — or the reverse.
The value Studio Pro stores also depends on the configuration: one text property
held `null` on the filters where it was hidden and the shipped default text on the
others. A fix that copies one Studio Pro instance's value just moves the failures
to the other filters (#2518). `mxcli widget describe` prints "N of M editor
hide-rules recognized", and each one it misses is a candidate. The fastest way to
find the property is to diff mxcli's widget against **the same widget after
`mx update-widgets`**, not against a Studio Pro reference. That diff contains only
what the package rejects; the reference diff also carries order and copy
differences that do not matter.

**Where a property is an enumeration, compare against the package's
`enumerationValue` *keys*, not its captions.** Both sides are plain strings, so
nothing else compares them and a caption-shaped value is stored and built without
complaint until Studio Pro reads it. A wrong value hidden inside the default
configuration will also not reproduce at all — vary the property that unhides it,
or the measurement is of nothing.

**The drift recurs because the two structures are written from different code
paths**, and the definitions themselves exist in more than one copy (the embedded
set, the modelsdk set, and the project's own `.mxcli/widgets/`). A fix applied to
one copy is latent in the others — the same [[duplicate-resolver-drift]] shape,
here in data rather than code.

**A rewrite must not rebuild a stored widget from the template.** The template
is one package version in one configuration; a stored widget carries its own
schema version, property order, translations and every value MDL does not map.
Rebuilding it on `describe` → `exec` reset all of that on every Studio Pro
data grid, combo box and image while `mx check` stayed green (#721 L4). The
writer now keeps the stored Type and Object when the statement's widget equals
what describe prints for it and binds nothing new, and rebuilds only a widget
whose statement changed. Two measurement traps from that fix: diff a widget's
properties **by key** (TypePointer → PropertyKey), never by index, or a pure
reorder reads as a thousand value changes; and a carry that re-sets an equal
value dirties the kept Object, which re-encodes its lists with today's
typed-array markers where Studio Pro stored older ones.

**`mx check` is the wrong gate for this class.** It is tolerant of both extra and
missing properties; `mx diff` and Studio Pro are strict, and Studio Pro runs
`mx diff` internally when opening a project with uncommitted changes. A green
check does not mean the project opens.

## See also

- [`diagnose-ce0463.md`](../../.claude/skills/diagnose-ce0463.md) — the
  elimination order, the controls, and what each normalisation hides
- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the
  per-instance CE0463 / CE3637 fixes
- [[architecture/widget-engine]] — how `.def.json` definitions and templates
  produce widget BSON
- [[duplicate-resolver-drift]] — why a definition kept in three places drifts
- [[models/version-gating]] — why a field can be valid in one Mendix minor and
  drift in another
