# ADR-0010: MDL has one canonical form, governed by twelve syntax rules

- **Status**: Accepted
- **Date**: 2026-09-26
- **Related**: extends [ADR-0003](0003-mdl-is-sql-shaped.md); [PROPOSAL_mdl_beta_syntax_freeze.md](../11-proposals/PROPOSAL_mdl_beta_syntax_freeze.md) §3, §4, §7, §10; [ADR-0011](0011-mdl-language-versioning.md), [ADR-0012](0012-mdl-first-and-data-first-editing.md); PR ako/mxcli#702

## Context

ADR-0003 fixed MDL's *shape*: SQL-like verbs, qualified names, English keywords, property lists. It did not fix MDL's *conventions*, and in their absence each document type grew its own. A whole-language review before beta (the proposal, §3–§4) found:

- **Argument binding has five spellings** across microflow calls, `show page`, button actions, workflows and REST.
- **Children nest by five different conventions** across REST clients, agents, published services, image collections and database connections.
- **Metadata has three homes.** Documentation, folder and layout each have up to three spellings.
- **Page actions are SCREAMING_SNAKE**: `SHOW_PAGE`, `SAVE_CHANGES`. Nothing else in MDL is.
- **Keywords have aliases everywhere.** `delete behavior` alone has 16 surface spellings for 3 values.
- **`describe` emits defaults, derived layout and invented names**, so its output reads worse than what the author wrote.

After beta, every alias still in the grammar becomes a permanent compatibility obligation. Consolidating is cheap now and expensive later. MDL is authored increasingly by LLMs and reviewed in PR diffs, which strengthens both of ADR-0003's goals: regular patterns and one way to say each thing.

## Decision

Every MDL construct has **exactly one canonical form**, defined by the twelve rules below. `describe` emits only that form. Other forms survive only as deprecated aliases (ADR-0011).

| # | Rule |
|---|---|
| R1 | **One idempotent create: `create or modify`.** It makes the stored document match the definition, and writes nothing when nothing differs (ADR-0012). `if not exists` is the separate "leave it alone" operation. Renames go through `alter`/`rename`, never through editing a definition. |
| R2 | **Three brackets, three meanings.** `( Key: value, … )` holds properties; `{ … }` holds declarative children, each shaped `<kind> [Name] ( props ) [ { children } ]`; `begin … end <keyword>` holds imperative flow. |
| R3 | **`:` sets a model property, `=` binds a runtime value.** `alter` uses the same property list as `create`: `set ( Key: value )`. |
| R4 | **One argument form everywhere: `Param = expression`**, with no `$` on the parameter name. Text-template parameters are always `with ({1} = …)`. |
| R5 | **Expressions are bare, and XPath is always in `[ … ]`.** Neither is ever written inside a string literal. There is one way to refer to a constant: `@Module.Const`. |
| R6 | **The verbs are `list` (plurals and relationship queries), `describe` (one thing), and `create`/`alter`/`drop`.** `show` is dropped. Element `describe` emits runnable MDL. *Definition* `describe` (`describe widget type`, `describe catalog.X`, …) emits a report marked "not executable". No `add`/`remove`/`define`/`update` verbs, and no synonyms such as `column` for `attribute`. |
| R7 | **MDL is what belongs in a checked-in `.mdl` file.** Session commands (`connect`, `set format`, `status`, `help`, …) are REPL commands, not grammar. |
| R8 | **Keywords are words, one spelling each.** No SCREAMING_SNAKE, no optional underscores. Lowercase is canonical. Property keys are case-preserved identifiers, never keywords. |
| R9 | **Each kind of metadata has one home.** Documentation is a `/** */` doc comment. Folder is a `folder '…'` clause. Canvas layout is `@position`/`@anchor`/`@curve`. Everything else is a property. |
| R10 | **Document types use Studio Pro's names**, with consistent `consumed`/`published` prefixes. |
| R11 | **Strict parsing.** `;` terminates every statement. Trailing commas are allowed in every list. `''` is the only string escape. Unknown or mis-shaped property keys are errors. Grammar that can never succeed is removed. |
| R12 | **`describe` emits the canonical form and nothing else.** It omits defaults, layout the tool derived itself, and names Mendix does not store. |

Microflow list operations follow R2 and R12 in a way that is specific to Studio Pro: **one statement per List operation or Aggregate list activity**, named after the operation, taking the inputs its dialog asks for. As in the diagram, they cannot nest.

## Consequences

**Positive**

- One example of a construct generalises to every document type. This is ADR-0003's LLM-fitness argument, now actually true.
- `describe` output becomes the reference implementation of the language. A fragment from it can be pasted into `create` or `alter` unchanged (R2, R3).
- R2's uniform node shape is what makes a single generic `alter` possible (ADR-0012). The rule has structural value, not only a cosmetic one.
- Several defect classes disappear at the grammar level: nested list operations, the `find` ambiguity, typos in property keys that are silently dropped, and XPath quoting that turns into runs of six quote characters.

**Negative**

- **Every existing script, skill, example and syntax page must migrate.** `fmt --upgrade` (ADR-0011) makes most of this mechanical, but not all.
- **`describe` output changes, and that output has been committed by users.** They will see one large diff when it switches to the canonical form.
- **R12's "omit derived layout" needs a reliable way to tell authored layout from derived layout.** That distinction exists today only for `@start`.
- The grammar temporarily grows, because every consolidated form keeps its alias until it is removed.
- R6 draws a line some users will find arbitrary: element describe must round-trip, definition describe need not.

**Neutral**

- ADR-0003's verb list (`SHOW / LIST`) and its note on the legacy `show` verb are replaced by R6. The rest of ADR-0003 stands.
- The rules become checklist items in the `design-mdl-syntax` skill, so new syntax is reviewed against them.

## Alternatives considered

- **Keep the aliases indefinitely** (be liberal in what you accept). Rejected: every alias is a second way to say something. It doubles what LLMs and reviewers must recognise and freezes accidental history into the post-beta language.
- **`Param: expr` for arguments.** It matches today's page `describe` output. Rejected in favour of R3's semantic split: `:` sets model properties, `=` binds runtime values, which also covers `change` and `set`.
- **SQL-style `alter X set Key = value`.** Rejected because the same key would take `:` in `create` and `=` in `alter`, which breaks copying a fragment between them.
- **`create or replace` as the idempotent keyword** (it matches SQL's `CREATE OR REPLACE VIEW`). Rejected because `modify` states the intent, *make the stored document match this*, and R1 adds minimal-change semantics that "replace" would contradict.
- **Keep `show` for non-element state.** Rejected in favour of a single `list`/`describe` split, which leaves no third verb to choose from.
- **Microflow list operations as nestable functions** (today's form). Rejected: the diagram has no nesting. The function form invited `count(filter(…))`, which Mendix cannot store, and it collides with the string functions `find` and `contains`.
- **Earlier proposals: `:=`, braces for blocks, lambdas, dropping `call`** (`PROPOSAL_mdl_syntax_improvements*.md`). Rejected as contrary to ADR-0003.

Prior art for each rule is tabulated in the proposal's §10.
