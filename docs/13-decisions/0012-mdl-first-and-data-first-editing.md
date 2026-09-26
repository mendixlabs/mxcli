# ADR-0012: MDL-first and data-first editing share one syntax and one patch engine

- **Status**: Accepted
- **Date**: 2026-09-26
- **Related**: builds on [ADR-0008](0008-identity-and-idempotence.md); [ADR-0010](0010-mdl-canonical-syntax-rules.md) (R1, R2, R12); [PROPOSAL_mdl_beta_syntax_freeze.md](../11-proposals/PROPOSAL_mdl_beta_syntax_freeze.md) §8, §9, §10; PR ako/mxcli#702

## Context

mxcli is used in two ways that pull in opposite directions:

- **New apps and modules.** MDL scripts are the source. Declarative definitions are the most efficient way to write them: each element is written once, in its final shape, with nothing to read first.
- **Existing Studio Pro apps.** This is where most agent work happens. The stored model is the truth, and MDL is a view of it. The efficient change is a small patch.

Measured on two real apps (the proposal, §8.1), the second way does not work today:

- **Re-running a document's own `describe` output as `create or modify`**, with no edit at all, silently lost data in **7 of 12** Studio Pro documents:
  - association storage changed from table to column (a schema change);
  - page translations were lost;
  - annotation links were lost;
  - export levels changed.
- **Microflows have no `alter`.** A one-line insert into a 16-activity flow meant re-sending all 107 lines. The whole-document rebuild (`UpdateMicroflow`, followed by re-pairing IDs by type and position in `canon/transplant.go`) then:
  - deleted 5 merges;
  - reset 11 of 15 curves;
  - changed 51 of 161 element `$ID`s, some onto *different* nodes.
- **Surgical `alter page … set` preserved everything.**

ADR-0008 made storage skip unchanged units. It cannot help when the rebuilt document is genuinely different from the stored one, and here it is.

## Decision

1. **Two modes, both first-class, one syntax.**
   - **MDL-first** is declarative: `create or modify`, where the script is the source of truth.
   - **Data-first** is a patch: `alter <type> X { set / insert / replace / drop }`, where the stored model is the truth.
   - A fragment inside `alter` is written exactly as in `create`, and `describe` output is valid declarative source.
2. **One generic `alter`, addressed by content where elements have no names.**
   - It covers every document type, including microflows and nanoflows.
   - Microflow activities are addressed by output variable, then by caption, then by statement pattern with wildcards.
   - When an address matches more than one element, that is an error that lists the matches. mxcli never guesses.
3. **Both modes write through one patch engine** that splices into the stored document.
   - Untouched elements stay byte-identical; only new nodes are placed.
   - `create or modify` is diff-then-patch: compare the definition with the stored document, derive the minimal patch, apply it. An empty patch writes nothing.
4. **Two round-trip laws hold for every document type** that has element `describe`, and CI enforces them against a Studio Pro-authored fixture:
   - **GetPut:** executing `describe` output unchanged writes nothing.
   - **PutGet:** describing what was written returns what was written.

   Content that MDL cannot express is carried through by a `preserved` placeholder, or the write is refused. It is never silently dropped.
5. **Concurrent change is detected by optional optimistic locking.**
   - For mixed projects: `describe` emits `@base '<fingerprint>'`, `create or modify` refuses when that fingerprint no longer matches the stored document, and `exec` updates the stamps it finds.
   - For projects driven entirely by MDL: `@base` is not used. A dry run of all scripts reporting no changes is the drift check.
   - There is no separate state file.

## Consequences

**Positive**

- The MDL-first and data-first cases each get the efficient form, and moving between them needs no second language.
- Agents can change existing apps at a cost proportional to the change, not to the document, without disturbing what they did not touch. This is the beta goal (§7 of the proposal).
- "No changes" becomes true in Studio Pro and in version control, not only in storage, which extends ADR-0008 from storage up to the language.
- The laws turn silent loss into either a test failure or a refusal.

**Negative**

- **The graph splice is new.** Patching a flowchart graph (sequence flows, merges, layout) has no textual precedent we know of. It is the riskiest code in the plan, and it must be tested on Studio Pro-authored flows, because mxcli-authored ones cannot show identity loss.
- **Diff-then-patch needs a stable way to match elements.** Where an activity has no output variable or caption, matching falls back to its statement signature. That is the same weakness React has for list items without `key` props.
- **`@base` means `exec` rewrites source files**, which is unusual (most tools keep this state separately), and some users will object. It is optional and only updates stamps already present.
- **CI/CD pipelines may not support it.** Pipelines usually check out read-only and do not commit back. So a stamp that `exec` updates in CI is lost, and a stale stamp could make the next apply refuse. In pipelines, the dry-run check (decision 5, second bullet) is the drift guard, and `exec` must be able to skip rewriting stamps.
- **A `preserved` placeholder in `describe` output is opaque to readers and to LLMs.** Each occurrence is a gap in what MDL can express, and must be tracked down.
- The whole-document rebuild paths (`UpdateMicroflow` and others) have to be replaced, not patched.

**Neutral**

- The guidance for agents is "choose the mode by who owns the document". It becomes skill text, and eventually an enforced check through `@base`.
- Three-way merge and per-element locking are natural extensions, deferred until after beta.
- **The `@base` mechanism (decision 5, first bullet) is explicitly provisional.** It was accepted with the reservation that users may dislike stamps in their scripts and that CI/CD pipelines may not support them. It is not on the beta gate. Before it is implemented, it is validated with users and in a pipeline. If it fails that test, a new ADR supersedes this decision with an alternative that keeps the lock outside the script. The candidates are a committed state file (Terraform) and the base stored alongside the model (Kubernetes' last-applied annotation). The rest of this ADR does not depend on the choice.

## Alternatives considered

- **Declarative only: always describe → edit → replace.** Rejected on the measurements above. Its cost is proportional to the document, and on Studio Pro content it is lossy.
- **Patch only.** Rejected for new apps: it forces building from empty shells, and scripts then describe a history rather than a design.
- **Keep the whole-document rebuild and improve ID re-pairing.** Rejected: re-pairing by type and position is structurally unable to be a fixed point once the builder emits objects in a different order from Studio Pro's.
- **A per-doctype `alter` grammar for microflows**, in the style of today's `alter workflow`. Rejected in favour of one generic `alter` over R2's uniform node shape, which leaves one grammar to learn instead of about 15.
- **Positional addressing** (the nth activity), as in JSON Patch's array indexes. Rejected as brittle under any unrelated edit.
- **A sidecar state file** for drift detection, as Terraform uses. Rejected in favour of the stamp in the statement, which travels with the script through git, copy-paste and review, and keeps statements self-contained (ADR-0003). This follows the HTTP `If-Match` model.
