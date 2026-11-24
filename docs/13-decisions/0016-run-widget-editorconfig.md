# ADR-0016: Which widget properties are hidden is answered by running the widget's editorConfig

- **Status**: Proposed
- **Date**: 2026-11-24
- **Related**: ako/mxcli#1776 (the gap), ako/mxcli#2507 (a widget with its own hide helper), ako/mxcli#1762 (hidden enumeration), [ADR-0009](0009-tunnel-is-linux-only.md) (what a dependency costs in detection terms)

## Context

A pluggable widget hides properties depending on its settings, and mxbuild
requires a hidden property to hold its default: a hidden enumeration at another
value, or a hidden text template that is not null, is CE0463. The logic is the
widget's `getProperties(values, defaultProperties, target)` in its compiled
`editorConfig.js`, which Studio Pro runs.

mxcli read that file as text instead, recognising the toolkit's hide helpers and
the guard shapes around them, one shape per fix. Measured over the 123 distinct
packages on the development machine: 96 of 170 widget versions had hide calls
the reader could not state. A widget that hides through its own helper had no
rule at all (ChessClock, #2507). And a guard on `target` was read as if it held
in Studio Pro: TreeNode, Maps and every chart hide one set of properties in
Studio (`"web"`) and another in Studio Pro (`"desktop"`), so mxcli reported
Studio Pro-visible properties as hidden. Each miss is a script that passes
`check` and `exec` and fails the build.

Running the function over the same packages (goja, a pure-Go engine) loaded
all 106 editorConfig bundles and answered all 84 that define `getProperties` (the
other 22 define none, so they hide nothing), at 1–4 ms a call.

## Decision

For a widget's own (top-level) properties, mxcli answers "is it hidden?" by
running the installed package's `getProperties` with the widget's actual values
and the `"desktop"` target, in goja. The answer replaces the rules read from the
source for those properties, for that configuration. The read rules remain the
fallback when the bundle cannot be run, and the only source for the properties
of object-list items until those are evaluated too.

## Consequences

- The coverage of the read rules no longer limits `check`, `exec` and `widget
  describe` for top-level properties, and no per-shape work is needed for new
  widgets or new bundler output.
- The answer is per configuration, so it is computed at each consumer from the
  values that consumer has. It is expressed as rules whose condition always or
  never holds (`types.OperatorAlways` / `OperatorNever`), so every consumer of
  rules works unchanged. Such rules are never stored in a `.def.json`.
- mxcli now executes third-party code from Marketplace packages. goja gives a
  script no file, network or process access. Each call gets a fresh runtime, a
  one-second limit and a recovered panic, and `require` is refused. A bundle
  that fails costs its widget the evaluated answer, nothing more.
- The `values` mxcli passes are its reconstruction of Studio Pro's: booleans as
  booleans, numbers as numbers, an expression or text template as its text, an
  unset action, data source, icon or image as `null`, an object list as an empty
  array. A bundle that reads a value shape mxcli renders differently can get a
  different answer from Studio Pro's. That is the risk this trades the static
  reader's for, and the round-trip measurements (every TestApp and PedApp page
  rebuilt byte-equal, the same CE0463 set on every generated example) are its
  regression guard.
- A new dependency (goja, MIT, pure Go) and an indirect `regexp2` bump. It is
  not network code, so ADR-0009's detection concern does not apply.

## Alternatives considered

- **Keep extending the static reader** (the original plan in #1776). Each shape
  is a fix, each fix covers one bundler's output, and a `target` guard or a
  widget's own helper is out of its reach by construction.
- **Synthesise rules by probing** (flip one property at a time from its default
  and record what disappears). It produces value-independent rules that could
  be stored, but a conjunction comes out as a single-term rule that hides a
  property in configurations where Studio Pro shows it, the over-firing the
  rule type was built to avoid.
- **A JavaScript engine through CGO (V8, QuickJS).** Faster and complete, but
  mxcli builds without a C compiler, and that matters more than speed for a
  function that runs in milliseconds.
