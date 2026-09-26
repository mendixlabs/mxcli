# ADR-0003: MDL is SQL-shaped

- **Status**: Accepted. The verb inventory (`SHOW / LIST`) is amended by [ADR-0010](0010-mdl-canonical-syntax-rules.md) (R6: `show` is dropped); the rest stands.
- **Date**: 2026-05-24
- **Related**: [PROPOSAL_mdl_syntax_design_guidelines.md](../11-proposals/PROPOSAL_mdl_syntax_design_guidelines.md); [`design-mdl-syntax` skill](../../.claude/skills/design-mdl-syntax.md)

## Context

Mendix's core audience is **citizen developers and business analysts** — people who are not software engineers and who reject syntax that looks cryptic or mathematical. A DSL for programmatic Mendix project manipulation has to land in a design tradition aimed at non-technical users, not one borrowed from systems programming or functional languages.

That tradition is well-established. SQL was deliberately shaped for business analysts — SEQUEL originally stood for "Structured English Query Language", and its keyword-heavy, declarative shape was a calculated trade of concision for readability. BASIC was deliberately shaped for people learning programming, with the same trade-offs. Both succeeded because they read like English, used full words instead of symbols, and made the common operations obvious. MDL inherits that goal directly: a language non-technical users can read aloud and reason about, not a terse DSL optimized for programmers.

A secondary context: MDL is increasingly authored by **LLMs** (inside Claude Code workflows and in scripts checked into projects) and reviewed by humans in **PR diffs**. Both pull in the same direction as the primary citizen-developer goal: regular patterns, self-contained statements, one-property-per-line diffs. The audiences reinforce each other rather than competing, but only because the citizen-developer constraint is treated as primary — optimising first for LLM token efficiency or operator concision would have produced a different and less readable language.

## Decision

MDL is shaped like SQL — specifically, like a SQL dialect extended with imperative flow constructs (`loop`, `if`, `retrieve where`) for microflow bodies, in the same family as PL/SQL, T-SQL, and PL/pgSQL. Concretely:

- **Standard verbs**: `CREATE`, `ALTER`, `DROP`, `SHOW` / `LIST`, `DESCRIBE`, `GRANT` / `REVOKE`, `SELECT`. No alternative verbs (`add`, `remove`, `view`).
- **Qualified names everywhere**: `Module.Element` — never implicit module context.
- **Property format**: `( Key: value, Key2: value2 )` with colon separators, one per line in multi-property forms, trailing commas allowed.
- **Keywords over symbols**: `from`, `where`, `in` — never `->`, `|>`, `=>`.
- **Self-contained statements**: each statement carries full context; no implicit state from prior statements.
- **One way to do each thing**: a single canonical pattern per operation, enforced by the `design-mdl-syntax` skill's checklist.

Full design principles and the contributor-facing checklist live in [`.claude/skills/design-mdl-syntax.md`](../../.claude/skills/design-mdl-syntax.md). This ADR records the underlying decision; the skill records the rules.

## Consequences

**Positive:**

- **Aligned with the target audience.** Citizen developers and business analysts read MDL as English. "create persistent entity Shop.Product" is a sentence; verifiable by reading statements aloud. This is the primary goal and the choice that drives everything else.
- **Well-trodden precedent for imperative extension.** Microflows, workflows, and nanoflows are control-flow artifacts. PL/SQL has demonstrated for decades that wrapping SQL in an Ada-derived imperative shell works well for non-technical users — MDL's `loop`, `if`, `retrieve where` constructs follow the same composition. T-SQL, PostgreSQL's PL/pgSQL, and Mendix's own microflow-as-flowchart shape are all in this lineage. The pattern is not novel; it has stress-tested itself for forty years.
- **Familiarity.** Anyone who knows SQL can read MDL on first encounter. The verbs (`CREATE`, `ALTER`, `SELECT`, `GRANT`) map to existing intuitions.
- **LLM fitness.** SQL is one of the most heavily-represented syntaxes in LLM training corpora. One example is generally enough for an LLM to generalise to variants — confirmed in practice across 50+ statement types.
- **Diff-friendly.** One-property-per-line and trailing commas make property additions a one-line diff, which makes PRs reviewable.
- **A clear design checklist exists.** New syntax is gated by the priority-ordered principles, which prevents drift as the language grows.

**Negative:**

- **Verbosity.** SQL-shape produces longer statements than a JSON DSL or symbolic Go fluent API. Token counts are higher, on-screen real estate is larger. This is the deliberate trade — concision sacrificed for readability — but it remains a real cost when authoring long scripts.
- **Not idiomatic for software engineers.** Contributors used to JSON, YAML, or code DSLs may find MDL's verbosity uncomfortable initially. The design skill's checklist mitigates this but doesn't eliminate it.
- **Parser complexity.** SQL-style grammar requires ANTLR4 or a comparable parser generator; a JSON DSL would have parsed with the standard library.

**Neutral:**

- ANTLR4 grammar is a sibling decision worth its own ADR when back-filled; the SQL-shape doesn't require ANTLR specifically, but they reinforce each other.
- The legacy `show` verb coexists with the newer `list` verb during transition; this is documented in the design skill.

## Alternatives considered

- **JSON DSL.** Rejected: terrible readability for non-programmers (`{"create": "entity", ...}`), comma-sensitive (no trailing commas in strict JSON), poor diffs (any change can reflow), no natural place for imperative flow constructs. JSON is good for machine-to-machine; bad for human-to-machine.
- **YAML DSL.** Rejected: indentation-sensitive, which LLMs handle poorly (off-by-one indents are a known generation failure mode), and the imperative microflow constructs don't compose with YAML's declarative shape without ugly workarounds (`type: loop`, `body: [ ... ]`).
- **Go-native fluent API only (no DSL).** Rejected: requires Go knowledge, excludes citizen developers entirely, can't be scripted from non-Go contexts, and produces poor diffs (Go code formatters reflow on trivial changes). The fluent API exists in `api/` for programmatic use but is not the primary user interface.
- **Custom Mendix-specific syntax.** Rejected: no LLM training-data benefit (model would need many examples to internalise), no audience familiarity, and new vocabulary creates a learning curve where SQL provides a free one.
- **Adopt an existing schema/DSL language (Cue, Dhall, Jsonnet).** Rejected: each carries its own conceptual overhead, none is widely known to citizen developers, and the operational benefit (type-checking) is partly served by `mxcli check`'s reference validation.
