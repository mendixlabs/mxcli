# Design New MDL Syntax

This skill provides guardrails for designing new MDL statements. Read this **before** writing grammar rules, AST types, or executor code for any new MDL feature.

## When to Use This Skill

- Adding a new document type to MDL (e.g., scheduled events, message definitions, REST services)
- Adding a new action type to microflows (e.g., new activity, new operation)
- Extending existing syntax with new clauses or keywords
- Reviewing a PR that adds or modifies MDL syntax
- Resolving a syntax design disagreement

## Core Principles (Priority Order)

When principles conflict, higher-priority ones win.

### 1. Read Like English

MDL targets citizen developers and business analysts, not software engineers. Statements should read as natural English sentences.

- Use keyword words (`from`, `where`, `in`), not symbols (`->`, `|>`, `=>`)
- Spell out full words (`microflow`, `association`), not abbreviations (`MF`, `ASSOC`)
- Use prepositions to clarify relationships: `grant read on entity to role`

**Test**: Read it aloud. A business analyst should understand on first hearing.

### 2. One Way to Do Each Thing

Reuse existing patterns. Never create a second syntax for the same concept.

| Operation | Pattern | Example |
|-----------|---------|---------|
| Create | `create [MODIFIERS] <type> Module.Name (...)` | `create persistent entity Shop.Product (...)` |
| Modify | `alter <type> Module.Name <operation>` | `alter entity Shop.Product add (...)` |
| Remove | `drop <type> Module.Name` | `drop entity Shop.Product` |
| List | `list <type>S [in module]` | `list entities in Shop` |
| Inspect | `describe <type> Module.Name` | `describe entity Shop.Product` |
| Security | `grant/revoke <perm> on <kind> <target> to/from <role>` | `grant read on entity Shop.Product to Shop.User` |

Do NOT use alternative verbs: `add` instead of `create`, `remove` instead of `drop`, `show` instead of `list`, `view` instead of `describe`. `show` is dropped (ADR-0010 R6): plurals and relationship queries are `list`, single things are `describe`. Inside `alter`, children are added and removed with `add`/`drop`.

### 3. Optimize for LLMs

- Keep patterns regular so one example is sufficient for generation
- Statements must be self-contained (no implicit state from prior statements)
- Use consistent keyword order: `<VERB> [MODIFIERS] <type> <NAME> [CLAUSES] [body]`
- Prefer flat statement sequences over deeply nested structures

### 4. Make Diffs Reviewable

- One property per line in multi-property constructs
- Allow trailing commas
- `describe` output uses deterministic property order
- Default values omitted unless non-obvious

### 5. Token Efficiency (Without Sacrificing Clarity)

- Omit noise words: `create entity` not `create A NEW entity`
- Support `create or modify` (the one idempotent create, ADR-0010 R1) to avoid check-then-create
- Allow type inference for obvious cases: `declare $count = 0`
- Do NOT use symbols to save tokens at the cost of readability

## Design Workflow

Follow these steps when designing syntax for a new MDL feature.

### Step 1: Check Existing Patterns

Read the MDL Quick Reference: `docs/01-project/MDL_QUICK_REFERENCE.md`

Does an existing pattern cover this? If yes, extend it. Don't invent new syntax.

```
New feature: "image collections"
Existing pattern: create/alter/drop/list/describe
design: create image collection Module.Name (...)
        describe image collection Module.Name
        list image COLLECTIONS [in module]
```

### Step 2: Pick the Statement Shape

Every MDL statement fits one of these shapes:

```
DDL:   <VERB> [MODIFIERS] <type> <QualifiedName> [CLAUSES] [body];
DML:   <action> <TARGET> [CLAUSES];
DQL:   <query-VERB> <type>S [FILTERS];
```

If your feature doesn't fit any shape, it may belong as a CLI command (`mxcli <subcommand>`) rather than MDL syntax.

### Step 3: Choose Keywords

1. Reuse existing keywords first (check reserved words in grammar)
2. Use SQL/DDL verbs: `create`, `alter`, `drop`, `show`, `describe`, `grant`, `revoke`, `set`
3. Use Mendix terminology: `entity` not `table`, `microflow` not `FUNCTION`, `page` not `view`
4. Prepositions clarify structure: `from`, `to`, `in`, `on`, `by`, `with`, `as`, `where`, `into`

### Step 4: Write the Property List

All property-bearing constructs use this format:

```mdl
create <type> Module.Name (
    Property1: value,
    Property2: value,
);
```

Rules:
- Parentheses `()` delimit property lists
- Colon `:` separates key from value
- Comma `,` separates properties
- Trailing comma allowed
- One property per line (single line acceptable for 1-2 properties)

#### Colon `:`, equals `=` and `as` — When to Use Each

ADR-0010 R3: **`:` sets a model property; `=` binds a runtime value** (call arguments `M.F(Order = $Order)`, `change $o (Attr = v)`, `set $x = …`, text-template parameters `with ({1} = …)`). `alter` uses the same property list as `create`: `set ( Key: value )`.

Use **colon** for property definitions (assigning a value to a named property):

```mdl
create entity Shop.Product (
    Name: string(200),          -- property: type/value
    Price: decimal,
);
textbox txtName (label: 'Name', attribute: title)
```

Use **`as`** for name-to-name mappings (renaming, aliasing, mapping one name to another):

```mdl
CUSTOM NAME map (
    'kvkNummer' as 'ChamberOfCommerceNumber',   -- old name AS new name
    'naam' as 'CompanyName',
)
```

Renames use `to`, like top-level `rename … to`: `alter entity Shop.Product rename attribute Code to ProductCode`.

**Rule of thumb**: if the left side is a *fixed property key* defined by the syntax, use `:`. If the left side is a *user-provided name* being mapped to another name, use `as`.

### Step 5: Validate

Run these checks before finalizing syntax design:

1. **Read aloud test** — Does it read as English? Can a business analyst understand it?
2. **LLM generation test** — Give one example to an LLM, ask for a variant. Does it get it right?
3. **Diff test** — Change one property. Is the diff exactly one line?
4. **Pattern test** — Does it follow CREATE/ALTER/DROP/SHOW/DESCRIBE? If not, why?
5. **Roundtrip test** — Can `describe` output be fed back as input?

## Anti-Patterns (DO NOT)

### Custom Verbs for Standard Operations

```mdl
-- WRONG: custom verb
SCHEDULE event Shop.Cleanup ...
REGISTER WEBHOOK Shop.OnOrder ...

-- RIGHT: standard CREATE
create SCHEDULED event Shop.Cleanup (...)
create WEBHOOK Shop.OnOrder (...)
```

### Implicit Module Context

```mdl
-- WRONG: implicit state
use module Shop;
create entity Customer (...);

-- RIGHT: explicit qualified name
create entity Shop.Customer (...);
```

### Symbolic Syntax

```mdl
-- WRONG: requires learning symbol meanings
$items |> filter($.active) |> map($.name)

-- RIGHT: keyword-based
$Active = filter $Items by Active = true;   -- one statement per Studio Pro list-operation activity
```

### Positional Arguments

```mdl
-- WRONG: meaning unclear without docs
create rule Shop Process Order ACT_ProcessOrder

-- RIGHT: labeled properties
create rule Shop.ProcessOrder (
    type: validation,
    microflow: Shop.ACT_ProcessOrder,
);
```

### Keyword Overloading

```mdl
-- CAUTION: SET already means variable assignment in microflows
-- Don't reuse it to mean property modification elsewhere unless established
```

## Canonical Rules (ADR-0010)

These are the canonical rules. Each PR that adds or changes syntax is checked against them. The rationale is in [ADR-0010](../../docs/13-decisions/0010-mdl-canonical-syntax-rules.md); examples are in `docs/11-proposals/PROPOSAL_mdl_beta_syntax_freeze.md` §3.

| # | Rule |
|---|---|
| R1 | One idempotent create: `create or modify`. It changes only what differs, and writes nothing when nothing differs. `if not exists` is the "leave it alone" operation. Renames go through `alter`/`rename`. |
| R2 | `( Key: value, )` holds properties. `{ }` holds declarative children, each shaped `<kind> [Name] ( props ) [ { … } ]`. `begin … end <keyword>` holds imperative flow. |
| R3 | `:` sets a model property; `=` binds a runtime value. `alter … set ( Key: value )` takes exactly the keys of `create`. |
| R4 | One argument form everywhere: `Param = expression` (no `$` on the parameter name). Text templates always use `with ({1} = …)`. |
| R5 | Expressions are bare; XPath is in `[ … ]`. Neither ever goes in a string literal. Constants are referred to as `@Module.Const`. |
| R6 | Verbs: `list`, `describe`, `create`, `alter`, `drop`. Element `describe` emits runnable MDL; definition `describe` emits a report marked "not executable". |
| R7 | Session commands are REPL commands, not grammar. |
| R8 | Keywords are words, one spelling each: lowercase, no SCREAMING_SNAKE, no optional underscores. Property keys are identifiers, never keywords. |
| R9 | Documentation is a `/** */` doc comment; folder is a `folder '…'` clause; canvas layout uses `@` annotations; everything else is a property. |
| R10 | Document types use Studio Pro's names, with consistent `consumed`/`published` prefixes. |
| R11 | `;` is required; trailing commas are allowed in every list; `''` is the only string escape; unknown property keys are errors. |
| R12 | `describe` emits the canonical form only: no defaults, no derived layout, no names Mendix does not store. |

A change of **meaning** to existing syntax is never made in place. It lands behind a language version (`mdl <n>;`, [ADR-0011](../../docs/13-decisions/0011-mdl-language-versioning.md)). A change of **spelling** keeps the old form as a registered deprecated alias.

## Checklist

Before merging any PR that adds new MDL syntax, verify:

- [ ] Conforms to R1–R12 above (and, where the construct exists in both modes, `alter` accepts the same fragment syntax as `create`, per ADR-0012)
- [ ] No new alias: any second spelling is a registered deprecation with an `fmt --upgrade` rewrite
- [ ] Any change of meaning is gated on the language header (ADR-0011)

- [ ] Follows `create`/`alter`/`drop`/`list`/`describe` pattern
- [ ] Uses `Module.Element` qualified names (no bare names)
- [ ] Property lists use `( key: value, ... )` format
- [ ] Keywords are full English words (no abbreviations)
- [ ] Statement reads as English (aloud test passed)
- [ ] One example sufficient for LLM generation
- [ ] Small change = one-line diff
- [ ] No new keyword overloading
- [ ] No implicit context dependency
- [ ] `describe` roundtrips to valid MDL
- [ ] Grammar regenerated (`make grammar`)
- [ ] Quick reference updated (`docs/01-project/MDL_QUICK_REFERENCE.md`)
- [ ] Full-stack wired: grammar, AST, visitor, executor, DESCRIBE

## Related Resources

- Decisions: [ADR-0003](../../docs/13-decisions/0003-mdl-is-sql-shaped.md), [ADR-0010](../../docs/13-decisions/0010-mdl-canonical-syntax-rules.md), [ADR-0011](../../docs/13-decisions/0011-mdl-language-versioning.md), [ADR-0012](../../docs/13-decisions/0012-mdl-first-and-data-first-editing.md)
- Beta syntax proposal (examples, plan): `docs/11-proposals/PROPOSAL_mdl_beta_syntax_freeze.md`
- Earlier design rationale: `docs/11-proposals/PROPOSAL_mdl_syntax_design_guidelines.md`
- MDL Quick Reference: `docs/01-project/MDL_QUICK_REFERENCE.md`
- Implementation workflow: `.claude/skills/implement-mdl-feature.md`
- Grammar file: `mdl/grammar/MDLParser.g4`
