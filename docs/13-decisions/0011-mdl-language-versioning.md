# ADR-0011: MDL evolves through deprecation aliases and a language header; meaning changes only across versions

- **Status**: Accepted
- **Date**: 2026-09-26
- **Related**: [ADR-0010](0010-mdl-canonical-syntax-rules.md); [PROPOSAL_mdl_beta_syntax_freeze.md](../11-proposals/PROPOSAL_mdl_beta_syntax_freeze.md) §5, §6, §9, §10; PR ako/mxcli#702

## Context

ADR-0010 consolidates MDL onto one canonical form. That change comes in two kinds:

- **Respellings.** Most changes give an existing meaning a new spelling: `SHOW_PAGE` becomes `show page`, `generalization` becomes `extends`. The old form can keep parsing.
- **Changes of meaning.** A few change what existing text *means*, or reject text that is accepted today:
  - `retrieve … limit 1` binds an object today; under R11 it will be a list of one.
  - `$x = find(…)` means different things depending on earlier statements.
  - `'it\'s'` is an escape today.
  - Unknown property keys are silently ignored today.

  Changing their meaning in place would silently alter scripts that users have committed.

MDL has no version marker. The proposed `set version '10.18'` (`version-aware-mdl.md`) names the **Mendix target** version, a different axis. mxcli releases weekly, so "warn for one release, then flip" gives users one week to notice. After beta, the same mechanism will be needed for every later change.

## Decision

1. **Respellings become deprecated aliases.**
   - Each alias is an entry in one registry: code `MDL-DEPRnnn`, old form, canonical form, and the version that removes it.
   - `check` and `exec` warn on it, and `describe` never emits it.
   - `mxcli fmt --upgrade` rewrites every registered alias mechanically.
   - An alias without a registry entry fails a test.
2. **A script may start with the header `mdl <n>;`**, which declares the language version it is written in. Meaning changes and new rejections apply **only under the version that introduces them**:
   - `mdl 1;` gets beta semantics.
   - A script with no header is treated as `mdl 0` (the alpha meaning) and warns on every construct whose meaning differs.
   - `fmt --upgrade` adds the header and rewrites those constructs.
   - **A version is a preview until it is frozen.** Before beta, `mdl 1;` parses but warns "preview: may still change", and `describe`/`fmt` do not emit it. At beta, `mdl 1` is frozen and `describe`/`fmt` always emit it. After that, any change of meaning needs a new version.
   - Aliases can be removed only at a version boundary: under the version that deprecates them they warn; under the next one they are refused.
3. **The header is independent of the Mendix target version.**

## Consequences

**Positive**

- A script's meaning never depends on which mxcli release runs it. No committed script changes behaviour silently.
- Beta is not a flag day, and it needs no warning releases. Because no script changes meaning without opting in, beta is a single release, cut when `mdl 1` is complete. Existing scripts keep running with warnings, and each user migrates when they choose, in one command.
- The same mechanism carries every future change, so after beta, "can we change this?" becomes "which version does it belong to?".
- Documentation, skills and examples can be held to the canonical form automatically, with deprecation warnings treated as errors in CI.

**Negative**

- **Two language versions must be maintained side by side.** The visitor must implement both meanings wherever they differ, and the tests must cover both.
- **A script with no header gets the *old* meaning.** New users, and LLMs that omit the header, get alpha semantics plus warnings until they add it. This is the price of never changing meaning silently, and the same price Rust and Go pay.
- **`fmt --upgrade` is a code generator that must be exactly right.** A wrong rewrite is itself a silent change of meaning. Every rewrite rule needs a test that the AST is equivalent after the rewrite.
- The registry is a new place every syntax change has to be recorded.

**Neutral**

- `mdl 0` is only ever implicit. Nobody writes it.
- Freezing, not the release number, is what makes a version a contract. A preview version may change between weekly releases, and says so.
- Removing a deprecated form requires both a new language version and the passage of time, never only time.

## Alternatives considered

- **Warn for one release, then change meaning in place.** This was the proposal's first plan. Rejected: with weekly releases, the warning window is one week, and a user who skipped a release gets a silent change.
- **A script with no header gets the latest version.** Rejected: its meaning would then depend on the mxcli release that runs it. Rust (a missing `edition` means 2015) and Go (a missing `go` line means the oldest version) chose the oldest for this reason.
- **Tie the language version to the mxcli version.** Rejected: mxcli releases weekly, while the language should change rarely.
- **Tie the language version to the Mendix version.** Rejected: that is a different axis, and a project upgrading Mendix must not have its scripts change meaning.
- **No versioning; aliases forever.** Rejected: aliases cannot express a change of meaning, so R11's changes would be impossible.
