---
title: MDL Language Critique and Beta Syntax Freeze
status: accepted
date: 2026-09-26
decisions: [ADR-0010, ADR-0011, ADR-0012]
---

# Proposal: MDL Language Critique and Beta Syntax Freeze

**Status:** Accepted (ADR-0010, ADR-0011 and ADR-0012 accepted 2026-09-26)
**Date:** 2026-09-26
**Related:** [ADR-0003](../13-decisions/0003-mdl-is-sql-shaped.md); resulting decisions [ADR-0010](../13-decisions/0010-mdl-canonical-syntax-rules.md), [ADR-0011](../13-decisions/0011-mdl-language-versioning.md), [ADR-0012](../13-decisions/0012-mdl-first-and-data-first-editing.md); [design-mdl-syntax skill](../../.claude/skills/design-mdl-syntax.md), [PROPOSAL_workflow_microflow_syntax_alignment.md](PROPOSAL_workflow_microflow_syntax_alignment.md)

## Summary

Once MDL leaves alpha, breaking syntax changes get expensive. This document reviews the whole language against its own stated principles (ADR-0003 and the `design-mdl-syntax` skill) and lists what should change before beta.

**Verdict.** The foundation is sound: SQL-shaped verbs, `Module.Name` everywhere, `/** */` documentation, `from … to …` associations and `extends`. The language does not need redesigning. It needs **consolidating**, for two reasons:

1. Each document type grew its own local conventions: how arguments are bound, how children nest, where metadata lives, how blocks close. Knowing one area of MDL does not teach you the next.
2. Almost every construct has picked up aliases. `delete behavior` alone has 16 surface spellings for 3 values. Every alias still in the grammar at beta becomes a permanent compatibility obligation.

Most of the findings collapse into **twelve rules** (§3). Adopt the rules, have `describe` emit only the canonical form, and leave the old forms parsing with deprecation warnings. That fixes most of this cheaply. A short list of changes **cannot** be bridged by aliases because they change the meaning of text that already parses (§5). Those are the true must-do-before-beta items.

The review also found **silent-loss bugs**: input that parses and is then dropped or mis-stored. They are listed first (§2) because they matter more than any syntax question.

**Brownfield editing (§8).** Most agent work will be on large Studio Pro apps that have no MDL source. For that work, the decisive questions are whether MDL can *read, locate and patch* reliably, not whether it reads well as source. Measured on two real apps:
- Surgical `alter` preserves stored state.
- Round-tripping `describe` output through `create or modify` silently lost data in 7 of 12 cases.
- Microflows, the most-edited type, have no `alter` at all.

§8 argues that MDL needs two first-class modes that share one syntax:
- **MDL-first:** declarative `create or modify` scripts as the source of truth. This is the efficient mode for new apps.
- **Data-first:** content-addressed `alter` patches that splice into the stored graph. This is the efficient mode for changes to existing Studio Pro documents.

Drift detection chooses between them safely, and two round-trip laws make moving between them lossless.

### Method

Six parallel reviews covered:
- the domain model;
- microflows and nanoflows;
- pages, snippets, layouts and navigation;
- security, workflows and settings;
- integration and agents;
- cross-cutting grammar and lexer.

Every "parses" or "rejected" claim was run through `mxcli check`. Round-trip claims come from `exec` followed by `describe` on a scratch copy of a real project. Grammar references are to `mdl/grammar/` at commit `5dc51ceb`.

---

## 1. What is good and should stay

- The statement shape `create [or modify] <type> Module.Name (props) body`, plus `alter`/`drop`/`describe`/`list`.
- Qualified names everywhere, and no `use module`.
- `/** */` doc comments as documentation, and `--` / `/* */` as comments. `//` is rejected, which keeps the comment syntax clean.
- `from Child to Parent` on associations, `on delete cascade|restrict|set null`, `extends`, `rename … to`, `move … to`.
- `alter page … set … on widget`, `insert after`, `replace … with`.
- The widget form `widget name (props) { children }`.
- `if … then … elsif … else … end if`, `loop $x in $L begin … end loop`, `retrieve $x from … where [xpath]`.
- `$$ … $$` for raw foreign content.

## 2. Bugs found during the review: fix regardless of syntax decisions

| # | Symptom | Where | Consequence |
|---|---|---|---|
| 1 | *(Withdrawn. The review claimed `create or replace entity` deletes and recreates the entity. It does not: the visitor folds `REPLACE` into `MODIFY` for persistent entities precisely to keep the GUID (`visitor_entity.go:36`). Delete-and-recreate happens only for **view** entities (`cmd_entities.go:913`), which have no stored rows. See R1 for the naming question that remains.)* | | |
| 2 | `throw <expr>` parses, but no visitor handles it | `MDLMicroflow.g4:280,724`; no Go reference | The statement disappears silently. `raise error` is the working form. |
| 3 | `float` and `currency` attribute types become `String` | `mdl/visitor/visitor_helpers.go:420` (falls through) | The wrong type is stored with no warning. `date` silently becomes `DateTime`. |
| 4 | The parenthesised association form `association X (from … to …, type: …, storage: …)` drops all options | `MDLDomainModel.g4:174-177`; the visitor reads only `AssociationOptions` | A `ReferenceSet` is stored as a `Reference`. |
| 5 | `$x = find($S, 'abc')` is either a string function or a list FIND, depending on whether `$x` was declared earlier | `setStatement` (optional `set`) vs `listOperationStatement` | The same text means two different things. |
| 6 | Unknown property keys are accepted and ignored (`Pathh:`, `Temprature:`, `Versoin:`) on the REST client, agent, model, published REST and business events | `visitor_rest.go:33-75`, `MDLAgent.g4:23-31` | Typos are silently discarded. Only the OData client refuses them (MDL-ODATA01). |
| 7 | Some statements parse but are always refused: `grant execute on workflow`, `case … else`, `declare … list of … = empty`, and widgets `text`/`statictext`/`legacydatagrid` | `MDLSecurity.g4:79-85`, `MDLMicroflow.g4:315`, `cmd_pages_builder_v3_widgets.go:728` | Grammar surface with no meaning. Remove it now while that costs nothing. |
| 8 | Describe output that does not re-parse or loses data: string defaults containing `'`; module-role, OData, published-REST and REST-client strings; agent MCP blocks missing a comma; REST header expressions re-emitted as literals; demo-user `password '***'`; user-role description; workflow captions and annotations emitted as `--` comments | `cmd_entities_describe.go:321`, `cmd_security.go:787,826,865`, `cmd_agenteditor_agents.go:179`, `cmd_rest_clients.go:180`, `cmd_workflows.go:263,832` | Describe does not round-trip. |
| 9 | Configuration describe prints `DatabasePassword = '<plaintext>'` | `cmd_settings.go:976` | Credential leak into PR diffs. |
| 10 | Image-collection describe emits `/tmp/mxcli-preview/...` file paths | `cmd_imagecollections.go:158` | Output is neither portable nor reviewable. |
| 11 | Items that are accepted and then dropped: enumeration-value doc comments, index names, and the declared name on `AutoOwner`-style attributes | `MDLDomainModel.g4:151,385` | The source says something the model does not store. |

## 3. The twelve rules

Each rule is stated so it can be added to the `design-mdl-syntax` skill as a checklist item. The findings each rule resolves follow it.

**About the examples.**
- Every **Before** block was run through `mxcli check` at `5dc51ceb` and parses today. The `describe` output in R12 is real output from a scratch project.
- Every **After** block is *proposed* syntax and does not parse yet. They follow the decisions recorded in §7.

### R1. One idempotent create, with one defined meaning

Today `create or modify` and `create or replace` are **one operation with two names**:
- The AST folds the two into one flag for every document type.
- The entity visitor folds `REPLACE` into `MODIFY` explicitly, to keep the GUID.
- The only difference is view entities, where `replace` deletes and recreates. That is safe only because they store no rows.

The semantics are also already declarative, not a merge:
- The statement is the **whole** definition.
- Members matched **by name** keep their stored `$ID`/GUID.
- Members the statement omits are dropped, and MDL087 reports them.

That settles renames. A declarative definition cannot tell a rename from a drop plus an add, so under it a rename destroys the column. **Renames are therefore only expressible as `alter entity … rename attribute A to B` or `rename entity … to …`**, which carry the identity across. The same holds for any document whose members have identity.

Rule (**decided**, §7):
- **`create or modify` is the one keyword.** It reads like the author's intent: *make the stored document match this definition*. `create or replace` becomes a deprecated alias.
- **Minimal change is part of the meaning.** `modify` changes only what differs, so re-running a statement whose definition already matches the stored document **writes nothing**, and Studio Pro shows no change. That covers byte-identical units, no `$ID` churn and no version-control noise. This is the GetPut law of §8.4, promoted from a goal to the defining property of the keyword. It is **not** true today: an unchanged microflow round trip rewrote the unit and moved 51 of 161 element `$ID`s (§8.1). The implementation consequence is in §8.3 and plan item 4.2.
- The other semantics, written into the skill and the ADR: the statement is the full definition; members are matched by name and keep their identity; omitted members are dropped and reported (MDL087); renames need `alter`/`rename`.
- Make view entities follow the same identity-carrying rewrite instead of delete and recreate, so the keyword means one thing on every type.
- Keep `if not exists` as a separate, genuinely different operation (leave an existing element untouched). Extend it to every type, not just entities and associations.
- Move `create module role` into `createStatement` (`MDLSecurity.g4:17`), so it gets the same prefix as every other type.
- Every `describe` emits `create or modify`. Today association, layout, user role, demo user, REST client, OData, agent and database connection emit a plain `create`, and navigation emits `create or replace`.

**Before** — two keywords for one operation, and a rename that looks harmless:

```mdl
create or modify persistent entity Shop.Order (Number: integer, Note: string(200));
create or replace persistent entity Shop.Order (Number: integer, Note: string(200));  -- identical effect

-- Editing the name in the definition is NOT a rename: Note is dropped (with its data),
-- and an empty Remarks column is added. MDL087 warns; nothing refuses.
create or modify persistent entity Shop.Order (Number: integer, Remarks: string(200));
```

**After** — one keyword that says what it does, a no-op when nothing differs, and renames as a separate statement:

```mdl
create or modify persistent entity Shop.Order (
  Number: integer,
  Note: string(200),
);

alter entity Shop.Order rename attribute Note to Remarks;   -- keeps the column and its data
create persistent entity if not exists Shop.Audit (At: datetime);  -- leaves an existing entity alone
```

### R2. Three bracket kinds, three meanings

| Bracket | Meaning | Used for |
|---|---|---|
| `( Key: value, … )` | the properties of *this* element | every document header and every child's properties |
| `{ … }` | declarative children | page widgets, service operations and resources, mapping trees, workflow activities, agent tools, menu items |
| `begin … end <keyword>` | imperative flow | microflow bodies, `if`, `loop`, `while`, error handlers |

Every child inside `{ }` has the shape `<kind> <Name> ( Key: value, ) [ { grandchildren } ]`. A child always ends in `)` or `}`, so, as with page widgets today, children need no separator.

What would change:
- REST client operations and agent attachments put properties inside `{}`. They become `operation GetUser ( Method: get, Path: '/u/{id}' )`.
- Image collections put children in `()`. They become `{ image Logo ( File: 'x.png' ) }`.
- The database connection is the only declarative document using `begin…end`, and it has no property list. It becomes `( Type: postgresql, ConnectionString: @M.Url, … ) { query Q ( Sql: $$…$$, Returns: M.E ) }`.
- Message definitions use `()` for member trees. They switch to `{}`, the same as mappings.
- Menus and navigation use `( item; item; )`. They switch to `{ }`, the same as widgets.
- Maps written as `Params: { $T: M.E }`, `DesignProperties: ['k': 'v']` or `ContentParams: [{1} = expr]` all become `( key: value, )`.
- `on error { … }` is the only brace block inside a microflow. It becomes `on error [without rollback] begin … end error;`.
- `while` makes both `begin` and `end while` optional. Make both required, the same as `loop`.
- The `split type` body becomes mandatory.

**Before** — properties in braces, children in parentheses, menu items separated by `;`, braces inside a microflow:

```mdl
create rest client Shop.Api (
  BaseUrl: 'https://api.example.com',
  authentication: none
)
{
  operation GetUser {
    method: get,
    path: '/users/{id}',
    headers: ('Accept' = 'application/json'),
    response: none
  }
};

create image collection Shop.Icons (
  image Logo from file 'assets/logo.png'
);

create or replace navigation Responsive
  home page Shop.Home
  menu (
    menu item 'Home' page Shop.Home;
    menu 'Admin' (
      menu item 'Users' page Administration.Account_Overview;
    );
  );

commit $Order on error without rollback {
  log warning 'Save failed';
};
```

**After** — `()` properties, `{}` children, `begin … end` flow:

```mdl
create consumed rest service Shop.Api (
  BaseUrl: 'https://api.example.com',
  Authentication: none,
) {
  operation GetUser (
    Method: get,
    Path: '/users/{id}',
    Headers: ( 'Accept': 'application/json' ),
    Response: none,
  )
};

create image collection Shop.Icons {
  image Logo ( File: 'assets/logo.png' )
};

create or modify navigation Responsive (
  HomePage: Shop.Home,
) {
  menu item 'Home' ( OnClick: show page Shop.Home )
  menu 'Admin' {
    menu item 'Users' ( OnClick: show page Administration.Account_Overview )
  }
};

commit $Order on error without rollback begin
  log warning 'Save failed';
end error;
```

### R3. `:` vs `=`

- **`:` sets a model property.** This covers property lists, including `alter … set ( Key: value )`.
- **`=` binds a runtime value.** This covers call arguments, `change $o (Attr = v)`, `set $x = …`, text-template parameters and mapping sides.
- Today the same key uses `:` in `create` and `=` in `alter`, for example page `set Caption = 'x'`, `alter odata … set Version = '1'` and `alter settings` `k = v`.
- Make `alter <type> X set ( Key: value, … )` accept exactly the keys and value grammar of `create`. That way, turning a describe fragment into an `alter` is a copy and paste.
- Also:
  - Remove the optional colons, one way or the other. Clauses outside a property list take no colon (`type reference`, not `type: reference`). An attribute definition is always `Name: Type`, including `modify attribute A: T`.
  - Remove `set allow_create_change_locally = true`, the only snake_case `=` alter action.

**Before** — `create` uses `:`, but the same keys take `=` in `alter`, in settings and inside some parenthesised lists:

```mdl
alter page Shop.Product_Overview {
  set caption = 'Name' on dgProducts.Name;
  set (caption = 'Cost', alignment = right) on dgProducts.Price
};

alter settings model AfterStartupMicroflow = 'Shop.ASU_Startup';

alter module Ops add jar dependency (group = 'org.postgresql', artifact = 'postgresql', version = '42.7.4', included = true);
```

**After** — a model property is always `Key: value` in a `( … )` list, whether it is set by `create` or `alter`:

```mdl
alter page Shop.Product_Overview {
  set ( Caption: 'Name' ) on dgProducts.Name;
  set ( Caption: 'Cost', Alignment: right ) on dgProducts.Price;
};

alter settings runtime ( AfterStartupMicroflow: Shop.ASU_Startup );

alter module Ops add jar dependency (
  Group: 'org.postgresql',
  Artifact: 'postgresql',
  Version: '42.7.4',
  Included: true,
);
```

### R4. One argument-binding form everywhere

The same idea is spelled five ways today:
- `call microflow M.F($p = e)`, also accepting `p = e`;
- `show page M.P($P = e)` or `P: e`;
- page actions `(P: v)`;
- workflows `with (p = '…')`;
- `send rest request` accepts only `$p = e`.

Canonical form, following R3: `Name = expression`, with no `$` on the parameter name.

Text-template parameters get one form too: `with ({1} = e)`. It replaces `objects [e, …]` in `show message` and validation feedback, and the older `parameters [...]`.

**Before** — five ways to pass a value to a parameter:

```mdl
call microflow Shop.ACT_Approve($Order = $Order, Force = false);    -- `$` optional
show page Shop.Order_Edit(Order: $Order);                           -- `:` here, `$Order =` also accepted
actionbutton btnApprove (Caption: 'Approve', Action: microflow Shop.ACT_Approve(Order: $currentObject))
call microflow Shop.ACT_Process with (Order = '$WorkflowContext');  -- in a workflow: the expression is a string
show message 'Order {1} approved' type information objects [$Order/Number];
log info node 'Shop' 'Order {1} approved' with ({1} = $Order/Number);
```

**After** — one form, whatever is being called and wherever the call appears:

```mdl
call microflow Shop.ACT_Approve(Order = $Order, Force = false);
show page Shop.Order_Edit(Order = $Order);
actionbutton btnApprove (Caption: 'Approve', OnClick: call microflow Shop.ACT_Approve(Order = $currentObject))
call microflow Shop.ACT_Process(Order = $WorkflowContext);          -- in a workflow
show message 'Order {1} approved' type information with ({1} = $Order/Number);
log info node 'Shop' 'Order {1} approved' with ({1} = $Order/Number);
```

### R5. Expressions are bare, and XPath is in `[ ]`

- XPath is always bracketed: `where [Active = true()]`. Today entity-access `where '…'` and workflow `targeting xpath '…'` are quoted strings. Those compose into runs of six quote characters (#750); navigation already moved to brackets for exactly that reason.
- Client and microflow expressions are always bare, never in a string and never in brackets. This affects:
  - workflow `decision '$Ctx/Total > 1000'`, timers and due dates;
  - page `Visible: [expr]` and `Editable: [expr]` (brackets used for a non-XPath expression, `MDLPage.g4:198-206`);
  - page variable defaults `Boolean = 'true'`.
- Microflow `where` accepts both a bracketed and a bare expression (`MDLMicroflow.g4:425`). Only the bracketed form stays.
- There are four ways to refer to a constant: `@Module.Const`, legacy `$Const`, bare `Key: M.C`, and `CONSTANT 'M.C'` in settings. Choose one; the recommendation is `@Module.Const`, which is already the most common.

**Before** — XPath inside string literals with doubled quotes, and expressions in strings or in brackets:

```mdl
grant Shop.User on Shop.Order (read *, write *) where '[Status = ''Open'']';

user task ReviewOrder 'Review the order'
  targeting xpath '[Status = ''Draft'']'
  ...
decision '$WorkflowContext/Total > 1000'
  ...

textbox txtNote (Attribute: Note, Visible: [$currentObject/Status = 'Open'])
```

**After** — XPath in brackets, expressions bare, no quotes to double:

```mdl
grant read *, write * on entity Shop.Order to Shop.User where [Status = 'Open'];

user task ReviewOrder caption 'Review the order'
  targeting users xpath [Status = 'Draft']
  ...
decision $WorkflowContext/Total > 1000
  ...

textbox txtNote (Attribute: Note, Visible: $currentObject/Status = 'Open')
```

### R6. Verb inventory

| Verb | Meaning | Changes |
|---|---|---|
| `list …` | enumerate elements, and relationship queries | Replace `show <plural>`, which today leads 72 of the syntax lines versus 15 for `list`. Add plurals for `list image collection` / `icon collection` / `message definition collection`. |
| `describe <type> Name` | one element | Remove `show entity|association|page X` (`MDLCatalog.g4:62-64`). |
| ~~`show`~~ | **dropped** (decided, §7) | Every one of the 88 `show` forms maps to `list`, to `describe`, or to a session command:<br>• Plurals and relationship queries become `list`: `list callers of X`, `list callees of X`, `list references to X`, `list impact of X`, `list access on E`, `list widgets …`, `list design properties …`, `list catalog tables`.<br>• Single things become `describe`: `describe entity X`, `describe navigation`, `describe app security`, `describe security matrix`, `describe structure [in M]`, `describe context of X`.<br>• Session state (`version`, `catalog status`) becomes a REPL command (R7).<br>`show` stays as a deprecated alias for one release. |
| `create` / `alter` / `drop` | as in SQL | `alter` children use `add`/`drop`. Replace `remove` (user role), `modify`/`add or modify` (settings), `define fragment`, `update security` and `update widgets`. Add the missing `drop`s: database connection, validation rule, external entity. |

**`describe` does three jobs; keep them apart.** Every document type that has a `create` also has a `describe`. Validation rules, indexes and access rules come out as part of `describe entity`, and `describe Module.Name` detects the type itself. The only gaps, app security and the security matrix, are covered by the `show` mapping above. But the verb currently mixes three kinds of answer:

| Kind | Examples today | Output | Rule |
|---|---|---|---|
| **Model element** | `describe entity`, `describe page`, `describe microflow`, … | runnable MDL | round-trips (R12, §8.4) |
| **Part of a model element** | `describe fragment from page … widget`, `describe styling on page …`, `describe translations for nl_NL` | runnable MDL or a report | the same round-trip rules when the output is MDL |
| **Definition or lookup** | `describe widget combobox`, `describe glyph 'star'`, `describe catalog.entities`, `describe contract entity …`, `describe contract operation from openapi '…'` | a report, not MDL | starts with a `-- <kind> definition (not executable)` header; `--json` gives a documented shape |

How to tell `describe` and `syntax` apart: **does the answer depend on the project?**
- `describe widget combobox` reads the widget package installed in *this* project. On PedApp that is Combo box 2.5.0 with 58 properties; another project with another version gets other properties. It therefore stays under `describe`, and does not move to `syntax`.
- `syntax` is built into the binary, the same for every project, and answers "how do I write X in MDL?".
- The two point at each other. For example, `syntax page widgets` tells the reader to run `describe widget type <name> -p app.mpr` for the properties a given project actually has.
- `describe catalog.<table>` stays too: it is SQL's `DESCRIBE TABLE`.

**Rename:** `describe widget <name>` becomes **`describe widget type <name>`**. As it stands, it collides with describing a widget *instance* on a page (`describe fragment … widget`, and the proposed `describe page X widget Y`, §8.1). "Which kind of widget?" and "which widget on this page?" must not share a phrase. The old form stays as a deprecated alias.

Also:
- `column` survives as a synonym for `attribute` in four `alter entity` actions. Remove it.
- `rest call get …` breaks the `call <kind>` pattern. Use `call rest service …`, which is also Studio Pro's name for the activity, and fold `send rest request` into it where the semantics allow.

**Before:**

```mdl
show entities in Shop;
show entity Shop.Order;
alter user role AppAdmin remove module roles (Shop.Admin);
alter entity Shop.Order add column Note: string(200);
alter styling on page Shop.Home widget ctnHeader set 'Full width' = on;
$Html = rest call get 'https://example.com' header Accept = 'text/html' timeout 300 returns string;
```

**After:**

```mdl
list entities in Shop;
describe entity Shop.Order;
alter user role AppAdmin drop module roles (Shop.Admin);
alter entity Shop.Order add attribute Note: string(200);
alter page Shop.Home { set ( DesignProperties: ( 'Full width': true ) ) on ctnHeader; };
$Html = call rest service get 'https://example.com' (
  Headers: ( 'Accept': 'text/html' ),
  Timeout: 300,
) returns string;
```

### R7. The language vs the session

The line between MDL and session commands:
- **MDL** is what makes sense in a checked-in `.mdl` file applied to a model.
- **Session commands** are anything that needs a session or an environment: `connect`, `disconnect`, `use`, `set format = …`, `status`, `check`, `build`, `lint`, `debug`, `execute script`, `help`, `introspect`.

Those session commands live in `utilityStatement` (`MDLSettings.g4:93-112`) today. Move them out as REPL meta-commands, and have `exec` refuse them inside scripts.

Two concrete gains:
- `set` stops having three meanings.
- `helpStatement: IDENTIFIER …` stops swallowing typos. Today `craete entity …` reports its error at the `(`, not at the misspelt word.

**Before** — this script passes the syntax check. The only error is on line 3, at column 35 (the `(`), not at the typo:

```mdl
connect local 'app.mpr';
set format = json;
craete persistent entity Shop.Note (Text: string(200));
```

**After** — a `.mdl` file holds model statements only. The session is set up outside the script (`mxcli exec script.mdl -p app.mpr --json`, or at the REPL). The typo is reported where it is: `unknown statement 'craete' — did you mean 'create'?`.

### R8. Words, not SCREAMING_SNAKE; one spelling per keyword

- Page actions use snake-case tokens: `SHOW_PAGE`, `CLOSE_PAGE`, `SAVE_CHANGES`, `CALL_MICROFLOW`, `CREATE_OBJECT`, `DELETE_OBJECT`, `OPEN_LINK`, `SIGN_OUT`, `COMPLETE_TASK`. Replace them with the microflow words: `show page`, `close page`, `save changes`, `call microflow`, `sign out`. Menus use the same vocabulary.
- Several keywords have more than one spelling, and each needs a single form:
  - `DELETE_AND_REFERENCES` has three lexer spellings.
  - `REFERENCE_SET`, `DELETE_BEHAVIOR` and `ALLOW_CREATE_CHANGE_LOCALLY` take optional underscores.
  - `ELSEIF` exists alongside `ELSIF`.
  - `returns none` sits next to `returns nothing`.
  - `NOT_NULL` sits next to `NOT NULL`.
- Lowercase keywords are canonical. `describe`, `fmt` and the `mxcli syntax` examples should all agree; the examples currently use uppercase 528 times and lowercase 100 times. Today `fmt` upper-cases property keys that happen to be keywords (`FOLDER:`, `Enabled: TRUE`). Property keys must be case-preserved identifiers, never keywords.
- The same "error message" concept has three keywords: `not null error '…'`, validation-rule `feedback '…'` and association `error_message '…'`. All become `error message '…'`.

**Before:**

```mdl
actionbutton btnNew (Caption: 'New', Action: create_object Shop.Order then show_page Shop.Order_Edit)
actionbutton btnSave (Caption: 'Save', Action: save_changes)
actionbutton btnOut (Caption: 'Sign out', Action: sign_out)

create association Shop.Order_Customer from Shop.Order to Shop.Customer
  type reference delete_behavior prevent error_message 'Customer still has orders';
```

**After** — the same words a microflow uses, and one spelling for "error message":

```mdl
actionbutton btnNew (Caption: 'New', OnClick: create object Shop.Order then show page Shop.Order_Edit)
actionbutton btnSave (Caption: 'Save', OnClick: save changes)
actionbutton btnOut (Caption: 'Sign out', OnClick: sign out)

create association Shop.Order_Customer from Shop.Order to Shop.Customer
  type reference
  on delete restrict error message 'Customer still has orders';
```

### R9. Where document metadata lives

| Metadata | Canonical form | Remove |
|---|---|---|
| Documentation | `/** … */` doc comment only | the `comment '…'` clause on constant, association, JSON structure, image collection and workflow; `Documentation:`; `set comment` |
| Folder | `folder '…'` clause after the name | `Folder:` property on REST client, OData, published REST and page headers; `@folder`; double folder on snippets |
| Canvas layout | `@position`, `@anchor`, `@curve` annotations only | `set position` on alter stays as the alter form. Also: `@Position` vs `@position` casing, and the `Position: (x,y)` property |
| Export level, security flags | clauses (`export level api`) | the `@applyentityaccess` / `@excluded` annotations, which become clauses |

**Before** — documentation as a clause, folder as a property on one document and a clause on another:

```mdl
create constant Shop.ApiUrl type string default 'https://api.example.com' comment 'Base URL of the API' exposed to client;

create rest client Shop.Api (
  BaseUrl: 'https://api.example.com',
  Folder: 'Integration',
  authentication: none
) { };

create microflow Shop.Helper () folder 'Integration'
begin
end;
```

**After** — documentation is always a doc comment, folder is always a clause, everything else is a property:

```mdl
/** Base URL of the API */
create constant Shop.ApiUrl folder 'Integration' (
  Type: string,
  DefaultValue: 'https://api.example.com',
  ExposedToClient: true,
);

create consumed rest service Shop.Api folder 'Integration' (
  BaseUrl: 'https://api.example.com',
  Authentication: none,
) { };

create microflow Shop.Helper () folder 'Integration'
begin
end;
```

In workflows, `comment 'x'` sets the **caption** today. That misled the draft alignment proposal, which assumed `comment` was the annotation. Rename it to `caption '…'`.

Workflow activities should all be `<kind> [Name] … [caption '…']`, with the name always optional and derived one way. Today the name is positional-and-required for user tasks, positional-and-optional elsewhere, and `as name` for calls.

### R10. Document type names follow Studio Pro

| Today | Canonical |
|---|---|
| `rest client` | `consumed rest service` |
| `odata client` / `odata service` | `consumed odata service` / `published odata service` |
| `business event service` (covers both directions) | `consumed` / `published business event service` |
| `queue` | `task queue` |
| `database connection` | `external database connection` (or keep; less important) |
| `model` (agents) | `ai model`. `model` is too generic and collides with `alter settings model`. |
| `alter project security` | `alter app security ( Level: production, … )` |
| `alter settings model` | the Studio Pro tab names: `runtime`, … |
| `create json structure … snippet '…'` | `… sample $$…$$`. `snippet` is already a page document type. |

Also reserve `consumed web service`, `published web service` and `xml schema` now, even though they are not implemented.

**Before** → **After:**

```mdl
create rest client Shop.Api (…)       -- becomes:  create consumed rest service Shop.Api (…)
create odata client Shop.Crm (…)      -- becomes:  create consumed odata service Shop.Crm (…)
create odata service Shop.Api (…)     -- becomes:  create published odata service Shop.Api (…)
show project security;                -- becomes:  show app security;
alter settings model …                -- becomes:  alter settings runtime ( … )
```

### R11. Strictness: a typo or a meaningless form is an error, not a no-op

- Unknown or mis-shaped property keys are errors (§2 #6). Tightening this after beta would break scripts that were silently wrong, so it has to happen before.
- The value's shape must not decide its meaning. `Response: json from $X` vs `Body: status as $Y` is decided by `from`/`as`, not by the key.
- `;` is required on top-level statements (`MDLParser.g4:39` makes it optional). Drop the SQL*Plus `/` terminator: it adds a noise line to every describe hunk and the ADR does not mention it.
- Trailing commas are allowed in **every** list. ADR-0003 promises them, yet they are rejected in entity attributes, enumeration values, parameters and every integration list. Use one shared list rule.
- `''` is the only string escape. `STRING_LITERAL` also accepts `\'` (`MDLLexer.g4:895`), which contradicts Mendix and makes `'C:\temp'` ambiguous.
- Delete the grammar branches that can never succeed (§2 #7).

**Before** — all of this passes `mxcli check` today. The missing `;`, the `/`, the backslash escape and the misspelt `pathh` key are all accepted; the misspelt key is then silently ignored:

```mdl
create persistent entity Shop.Note (Text: string(200))
/
create persistent entity Shop.Note2 (Text: string default 'it\'s')
create rest client Shop.Api3 (
  BaseUrl: 'https://api.example.com',
  authentication: none
)
{
  operation GetUser {
    method: get,
    pathh: '/users',
    response: none
  }
};
```

**After:**

```mdl
create persistent entity Shop.Note (Text: string(200),);                   -- `;` required, trailing comma allowed
create persistent entity Shop.Note2 (Text: string default 'it''s');        -- `''` is the only escape
create consumed rest service Shop.Api3 (
  BaseUrl: 'https://api.example.com',
  Authentication: none,
) {
  operation GetUser ( Method: get, Pathh: '/users', Response: none )
  --                               ^ error: unknown property 'Pathh' — did you mean 'Path'?
};
```
- Ban bare `IDENTIFIER` in parser rules; use `identifierOrKeyword`, checked by a test like `keyword_coverage_test.go`. Today whether a keyword can be used as a name depends on where it appears.

### R12. `describe` emits the canonical form and nothing else

`describe` output is what reviewers read in PRs, so it defines the language in practice. This rule covers describing model elements. Definitions and lookups (R6) are reports, not MDL, and are marked as such.

- Emit only canonical forms, which makes `describe` the reference implementation of these rules.
- Omit defaults, because default noise inflates the output:
  - associations always print `owner Default storage column on delete set null`;
  - `log 'x'` comes back as `log info node 'Application' 'x'`;
  - `sort by` is fully qualified;
  - `commit … with events`.
- Omit **derived** layout. A microflow authored with no `@position` comes back with `@position` on every statement, about three layout lines per logic line. Re-executing that output pins the auto-layout. Extend the derived-vs-authored rule already used for `@start` to `@position`, `@anchor` and `@curve`.
- Do not invent names Mendix does not store. Layout-grid rows and columns and DataGrid 2 columns get synthetic names (`row1`, `col3`, or two columns both called `Name`). Those names churn when a column is inserted and make `alter … on dg.Name` ambiguous. Make the widget name optional in the grammar, and address grid columns explicitly.
- Fold structured control flow back into its source shape. Today `case … when A, B then` and fall-through error handlers come back as `join sharedN; … merge sharedN;` goto labels, and a lone-`if` else comes back nested instead of as `elsif`.
- Put long data sources on multiple lines (`where`, `sort by` and each XPath term on its own line).

**Before** — real `describe` output for an association written as `create association Crit.Order_Customer from Crit.Order to Crit.Customer;` and a microflow written with **no** layout annotations:

```mdl
create association Crit.Order_Customer
from Crit.Order to Crit.Customer
type Reference
owner Default
storage column
on delete set null;
/
create or modify microflow Crit.CountOpen (
  $Orders: List of Crit.Order
)
returns Integer
begin
  @position(360, 200)
  $Open = filter($Orders, Status = 'Open');
  @position(520, 200)
  $N = count($Open);
  @position(680, 200)
  @merge(970, 200)
  @caption '$N > 10'
  if $N > 10 then
    @position(850, 300)
    log warning node 'Crit' 'Many open orders';
  end if;
  @position(1090, 200)
  return $N;
end;
/
```

**After** — defaults, derived layout and a caption that repeats the condition are all omitted. What is left is what the author wrote:

```mdl
create or modify association Crit.Order_Customer
  from Crit.Order to Crit.Customer;

create or modify microflow Crit.CountOpen (
  $Orders: List of Crit.Order
)
returns Integer
begin
  $Open = filter $Orders by Status = 'Open';
  $N = count $Open;
  if $N > 10 then
    log warning node 'Crit' 'Many open orders';
  end if;
  return $N;
end;
```

## 4. Area-specific findings not covered by the rules

### Domain model
- **Aliases to retire:**
  - `generalization` → `extends`;
  - bare `entity` → `persistent entity`, which is what `describe` emits;
  - `enum M.E` / `Enumeration(M.E)` / bare `M.E` → pick one, the bare form being ambiguous with an entity reference;
  - `required` → `not null`;
  - `delete_behavior` (16 spellings) → `on delete …`;
  - `create index X on E` → `alter entity … add index (…)`;
  - the optional `by` in `calculated by`;
  - the optional parentheses on a view entity's `as ( … )`;
  - the enumeration caption, where `Open 'x'` and `Open caption 'x'` both work in create but alter requires `caption`.
- **Validation rules.** Required and unique are attribute constraints, but regex and range need a separate `create validation rule for …` that has no `drop` or `alter`. Make all four inline constraints: `Age: Integer range 0 to 150 error message '…'`.
- **`alter` separators.** Optional commas on `alter entity`, none on `alter association` or `alter enumeration`. Use commas everywhere.
- **`alter enumeration` value names.** They accept only `IDENTIFIER`, so `add value "Select"` fails.
- **`drop event handler on before commit`.** It can't tell apart several handlers on the same moment. Add `call M.Flow`.
- **Grammar and docs disagree.** The skill shows `rename Code as ProductCode`, but the grammar has `rename attribute Code to ProductCode`. Keep `to`.

**Before** — three enumeration spellings, two "required" spellings, `generalization`, and validation rules as a separate statement with no `drop`:

```mdl
create persistent entity Shop.Customer generalization Administration.Account (
  Name: string(100) required,
  Status: enum Shop.Status,
  Tier: Enumeration(Shop.Tier),
  Region: Shop.Region
);
create validation rule for Shop.Customer.Email
    regex Shop.EmailPattern
    feedback 'Enter a valid email address';
```

**After** — one spelling each, and every validation lives on its attribute:

```mdl
create or modify persistent entity Shop.Customer extends Administration.Account (
  Name: string(100) not null,
  Status: enum Shop.Status,
  Tier: enum Shop.Tier,
  Region: enum Shop.Region,
  Email: string(200) matches Shop.EmailPattern error message 'Enter a valid email address',
);
```

### Microflows
- **`$x = …` ambiguity (§2 #5).**
  - Make `set` mandatory for reassignment. `describe` already always emits it.
  - Make list operations and aggregates **statements that mirror Studio Pro's activities** (decided, §7; see *List operations mirror the diagram* below).
  - Nesting then no longer parses. The nesting trap in CLAUDE.md idiom 3 disappears at the grammar level instead of being caught by MDL-LISTOP02, and the `find`/`contains` collision with the string functions goes away.
  - This also fixes the `RANGE($L, offset, amount)` vs `limit … offset` order inversion.
- **List operations mirror the diagram** (decided, §7). A developer who knows the microflow editor should recognise each statement as the activity they would drag in.

  The rules:
  - **One statement per activity.** Each statement corresponds to one *List operation* or *Aggregate list* activity.
  - **The keyword is the operation's name.** These are the operations the Mendix metamodel defines for those activities (`Microflows$Filter`, `FilterByExpression`, `Find`, `FindByExpression`, `Sort`, `Head`, `Tail`, `ListRange`, `Union`, `Intersect`, `Subtract`, `Contains`, `ListEquals`, and aggregate functions `Sum`, `Average`, `Count`, `Minimum`, `Maximum`, `All`, `Any`, `Reduce`).
  - **The inputs are the ones the activity's dialog asks for.** The operand is always a variable, because the dialog selects a variable. So nesting cannot be written, just as it cannot be drawn.
  - **`by` picks a member** (the dialog's attribute or association selector). **`where` / `of` take an expression** (the "… by expression" variants).
  - `describe` prints the same form. `@caption` appears only when the caption was customised.

  | Studio Pro activity: operation | MDL statement |
  |---|---|
  | List operation: Filter | `$Open = filter $Orders by Status = Shop.Status.Open;` |
  | List operation: Filter by expression | `$Big = filter $Orders where $currentObject/Total > 1000;` |
  | List operation: Find | `$Order = find $Orders by Number = $Number;` |
  | List operation: Find by expression | `$Late = find $Orders where $currentObject/DueDate < [%CurrentDateTime%];` |
  | List operation: Sort | `$Sorted = sort $Orders by Date desc, Number asc;` |
  | List operation: Head / Tail | `$First = head $Orders;` / `$Rest = tail $Orders;` |
  | List operation: Range | `$Page = range $Orders offset 20 limit 10;` |
  | List operation: Union / Intersect / Subtract | `$All = union $A with $B;` / `$Both = intersect $A with $B;` / `$Left = subtract $B from $A;` |
  | List operation: Contains / Equals | `$Has = contains $Order in $Orders;` / `$Same = equals $A and $B;` |
  | Aggregate list: Count | `$N = count $Orders;` |
  | Aggregate list: Sum / Average / Minimum / Maximum | `$Total = sum $Orders by Amount;` or, by expression, `$Total = sum $Orders of $currentObject/Price * $currentObject/Quantity;` |
  | Aggregate list: All / Any | `$AllPaid = all $Orders where $currentObject/Paid;` / `$AnyLate = any $Orders where …;` |
  | Aggregate list: Reduce | `$Csv = reduce $Orders from '' as String using …;` |

  Before the grammar is fixed, check the keywords and connecting words (`by`, `where`, `of`, `with`, `from`, `in`) against the labels Studio Pro's dialogs actually show for the Mendix versions mxcli supports. The operation set above comes from the metamodel. The label wording is not verified.
- **`retrieve … limit 1`** — see §5.
- **Aliases to retire:**
  - `$o/A = v` and `set $o/A = v` → `change $o (A = v)`;
  - `/` vs `.` in attribute paths → `/`;
  - `SUM($L.attr)` vs `SUM($L, expr)`;
  - the `case`/`else` spellings inside `split type`.
- **`@anchor(false: (from: top, to: bottom))`** → `@anchor(branch: false, from: top, to: bottom)`. `@merge(x,y)` collides with the `merge` statement; rename it to `@endmerge`.
- **Keywords that are common attribute names.** `Count`, `Sum`, `Range`, `Filter`, `Sort` and `Head` are keywords, and `sortSpec` accepts only `IDENTIFIER`, so an attribute named `Count` must be quoted. The keyword-statement form above lets these fall back to identifiers.

**Before** — the same `$v = find(…)` shape means two things, list operations look like nestable functions, `limit 1` returns an object, and an attribute can be changed three ways:

```mdl
declare $x integer = 0;
$x = find($S, 'abc');                    -- $x was declared: the STRING function find()
$y = find($Orders, Number = 42);         -- $y was not: the LIST operation FIND

$Approved = filter($Orders, Status = Shop.Status.Approved);
$Count = count($Approved);               -- count(filter(…)) also parses (MDL-LISTOP02 catches it)
$Total = sum($Approved.Amount);          -- `.` here, `/` everywhere else
retrieve $First from Shop.Order where [Status = 'Open'] limit 1;   -- binds an OBJECT, not a list
$First/Note = 'first';                   -- also: set $First/Note = …; change $First (Note = …)
while $Count > 0
  set $Count = $Count - 1;
end while;                               -- begin and end while both optional
```

**After:**

```mdl
declare $x integer = 0;
set $x = find($S, 'abc');                -- assignment always says `set`; find() is the string function
$y = find $Orders by Number = 42;        -- a list operation is a statement, never a function call

$Approved = filter $Orders by Status = Shop.Status.Approved;
$Count = count $Approved;                -- count filter … cannot parse: the operand must be a variable
$Total = sum $Approved by Amount;
retrieve $First from Shop.Order where [Status = 'Open'] first;     -- an object
retrieve $Top from Shop.Order where [Status = 'Open'] limit 1;     -- a list of one
change $First (Note = 'first');
while $Count > 0 begin
  set $Count = $Count - 1;
end while;
```

### Pages
- **Widget-type keyword explosion.** `widgetTypeV3` has about 60 tokens, including object-list item words (`MARKER`, `SERIES`, `SCALECOLOR`, …) that then have to be quoted as names.
  - Make a widget type an identifier resolved through the widget registry.
  - Keep lexer tokens for structural words only, with `pluggablewidget '<id>' name` as the single escape hatch.
  - Delete the aliases: `container`/`customcontainer`, `button`/`actionbutton`, `pluggablewidget`/`customwidget`.
  - Remove the silent `image` fallback to a static image.
  - This is the most important change for keeping MDL stable across widget versions.
- **Property-key convention.**
  - A small fixed set of PascalCase common keys: `DataSource`, `Attribute`, `Label`, `Caption`, `OnClick`, `OnChange`, `Visible`, `Editable`, `Class`, `Style`, `DynamicClasses`, `DesignProperties`.
  - For every other property, the pluggable widget's XML key verbatim. That key is stable across versions; Studio Pro captions are not.
  - "On click" should have one key, `OnClick`. Today describe rewrites it to `Action:`.
  - Booleans are `true`/`false` only, not `on/off` or `yes/no`. Enum values are unquoted.
- **Five ways to set styling.** Inline properties, `alter page set`, `alter styling`, `alter pages … where widgettype =` and `update widgets … where WidgetType like`. Collapse them to inline properties plus `alter page`, and one bulk form with one `where` language.
- **Data sources.**
  - Make `database from M.E` require `from`, and require `and` between XPath terms; describe currently emits juxtaposed `[a] [b]`.
  - Use one association-path form.
  - Reference `Layout:`, `Icon:` and `Image:` by bare qualified name, not a string.
- **`define fragment`.** It is unqualified and session-scoped, which breaks the self-contained-statement rule. Either make it `create fragment Module.F` or remove it.

**Before** → **After**, a data source:

```mdl
listview lv (DataSource: database Shop.Order where [Status = 'Open'] [Total > 100]) { }
```

```mdl
listview lv (
  DataSource: database from Shop.Order
    where [Status = 'Open']
      and [Total > 100],
) { }
```

### Security, workflows, settings
- **Entity `grant`.** It is the only grant with reversed word order: `grant M.Role on M.E (read, write)`. Change it to `grant read *, write (Email), create on entity M.E to M.Role where [xpath];`. The skill's own example `grant read on Shop.Product to Shop.User` does not parse today. The two forms can both parse, because they start differently.
- **Workflow decision outcomes** use `->`, the only symbol operator in MDL, which the ADR bans explicitly. User-task outcomes already have no arrow. Use `outcomes true { … } false { … }`.
- **`alter settings` mixes three syntaxes.**
  - It has bare `k = v` lists, `(k: v)` items, and the verbs `add` / `modify` / `add or modify` / `remove`. It also duplicates `create or modify configuration`, and constant overrides can be dropped two ways.
  - Proposed forms:
    - `alter settings runtime ( AfterStartupMicroflow: M.F, … );`
    - `create or modify configuration 'Default' ( … );`
    - `alter configuration 'Default' set constant M.C = '…';`
- **Role, constant and demo-user headers use clauses, not property lists.**
  - `create user role N (Mod.R) manage all roles`, with a positional role list; `create user role N;` with no roles fails.
  - `create demo user 'u' password 'p' entity E (roles)`.
  - `create constant M.C type string default 'x' exposed to client`.
  - Convert them to `( Key: value )`, as scheduled events already are.
- **`alter workflow` vocabulary doesn't match create.**
  - `insert condition` vs `outcomes`.
  - `drop path 'Path 3'` vs `path 3`.
  - The `@1` ordinal reuses the annotation sigil.
- **Guest-access doc bug.** The page shows `grant Anonymous on …`, a user role, which is always refused (MDL-GRANT02).

**Before** — a workflow and settings as they are written today:

```mdl
create workflow Shop.OrderApproval
  parameter $WorkflowContext: Shop.OrderContext
  display 'Order Approval'
begin
  user task ReviewOrder 'Review the order'
    page Shop.TaskPage
    targeting xpath '[Status = ''Draft'']'
    outcomes
      'Approve' { call microflow Shop.ACT_Process with (Order = '$WorkflowContext'); }
      'Reject' { }
  ;
  decision '$WorkflowContext/Total > 1000' comment 'Large order?'   -- `comment` sets the CAPTION
    outcomes
      true -> { call microflow Shop.ACT_Escalate; }
      false -> { }
  ;
end workflow;

alter settings configuration 'Default'
  DatabaseType = 'PostgreSql',
  HttpPortNumber = 8080;
alter settings constant 'Shop.ApiUrl' value 'https://test.example.com' in configuration 'Default';
```

**After** — no arrows, no expressions in strings, `caption` says what it sets, and settings use property lists:

```mdl
create or modify workflow Shop.OrderApproval
  parameter $WorkflowContext: Shop.OrderContext
  display 'Order Approval'
begin
  user task ReviewOrder caption 'Review the order'
    page Shop.TaskPage
    targeting users xpath [Status = 'Draft']
    outcomes
      'Approve' { call microflow Shop.ACT_Process(Order = $WorkflowContext); }
      'Reject' { }
  ;
  decision $WorkflowContext/Total > 1000 caption 'Large order?'
    outcomes
      true { call microflow Shop.ACT_Escalate; }
      false { }
  ;
end workflow;

create or modify configuration 'Default' (
  DatabaseType: postgresql,
  HttpPortNumber: 8080,
);
alter configuration 'Default' set constant Shop.ApiUrl = 'https://test.example.com';
```

### Integration and agents
- **Three mapping dialects.**
  - Import is `Attr = json`.
  - Export uses `as` for objects and `json = Attr` for values.
  - The inline REST `Body: mapping` spells export objects with `=`.
  - Message definitions use `()` and `as 'String'`.
  - Adopt one rule, "the left side is the side being written", and make inline mappings reuse the standalone mapping grammar verbatim.

  **Before** — attribute lines already follow the rule; nested objects do not (`= json` on import, but `as json` on export):

  ```mdl
  create import mapping Shop.IMM_Order with json structure Shop.JSON_Order {
    create Shop.OrderResponse {
      OrderId = orderId,
      create Shop.CustomerInfo_OrderResponse/Shop.CustomerInfo = customer {
        Name = name
      }
    }
  };
  create export mapping Shop.EMM_Order with json structure Shop.JSON_Order {
    Shop.OrderResponse {
      orderId = OrderId,
      Shop.CustomerInfo_OrderResponse/Shop.CustomerInfo as customer {
        name = Name
      }
    }
  };
  ```

  **After** — the written side is always on the left, for objects as well as values:

  ```mdl
  create import mapping Shop.IMM_Order with json structure Shop.JSON_Order {
    create Shop.OrderResponse {
      OrderId = orderId,
      create Shop.CustomerInfo_OrderResponse/Shop.CustomerInfo = customer {
        Name = name,
      },
    }
  };
  create export mapping Shop.EMM_Order with json structure Shop.JSON_Order {
    Shop.OrderResponse {
      orderId = OrderId,
      customer = Shop.CustomerInfo_OrderResponse/Shop.CustomerInfo {
        name = Name,
      },
    }
  };
  ```
- **HTTP concepts spelled three or four ways.** Headers, basic auth, constants and timeout.
  - `Headers: ( 'Name': expr, )`.
  - `Authentication: basic ( Username: …, Password: … )` for the REST client, the OData client, the database connection and `call rest service`.
  - `Timeout:` in seconds everywhere.
- **`OpenAPI:` as a property** is a generation directive, not stored state, so it can't round-trip. Use `create consumed rest service X from openapi '…' ( … )`, the same shape as `create external entities from …`.
- **Foreign content.** Accept `$$…$$` wherever the content is foreign; `Body: template '…'` can't be multi-line today.
- **Agent attachments come in three shapes.** Use one: `<kind> <LocalName> ( Source: M.Doc, … )`.
- **Enum-like values.** They are quoted in some places (`type 'MSSQL'`, `export level 'Public'`) and bare in others. Make them bare. OData accepts both `Yes/No` and `true/false`; keep `true/false` only.

## 5. Changes that cannot be bridged by an alias

These change the meaning of text that parses today, or they reject text that is accepted today. They must land before beta or never.

**Changes of meaning and new rejections are tied to the language header, as Rust editions do** (§6, §10). A script that starts with `mdl 1;` gets the new meaning. A script without the header keeps the alpha meaning and gets a warning on every construct whose meaning differs. `fmt --upgrade` adds the header and rewrites those constructs. This applies to items 2–6 below. Existing scripts therefore never change meaning silently, and no user has to notice a one-week warning.

Items 7–8 (removing forms that never worked or mis-store) and item 9 (describe output) do not depend on the header. Item 1 is an alias, so it does not either.

1. **`create or modify` becomes the one idempotent create, with minimal-change semantics (R1).** `or replace` becomes an alias. Only the view-entity path gives the two names different behaviour, so aliasing is safe only once view entities also use the identity-carrying rewrite. The no-op guarantee changes behaviour for every document type that is rewritten today when nothing changed.
2. **`retrieve … limit 1`.**
   - Today it binds an **object** (the Mendix "first" range), so to any SQL reader it means something different from what it does.
   - Import mappings already solved this with `first | limit n`, and their own grammar comment argues that a list of one and an object "cannot share syntax".
   - Adopt `retrieve $x from … where … first;` for an object, so that `limit 1` means a list of one.
   - Tied to the header. Under `mdl 1;`, `limit 1` is a list of one. Without the header, it keeps its old meaning (an object) and warns. `describe` always emits `first` or `limit n` together with the header, so its output is never ambiguous.
3. **Make `set` mandatory, and turn list operations into statements that mirror Studio Pro's activities** (§4, Microflows), removing the `$x = find(...)` ambiguity.
4. **Unknown property keys become errors.**
5. **`;` becomes required**, and `/` is removed.
6. **`\` is no longer an escape character** in string literals.
7. **Remove the dead grammar**: `throw`, `grant … on workflow`, `case … else`, `text`/`statictext`, and the parenthesised association form.
8. **Remove `float`, `currency` and `date`** as attribute types. They are accepted and mis-stored today.
9. **Omit synthetic widget names and derived layout** from describe. This changes describe output, which people have already committed to repos.

Everything else in this document can land after an alias period. Prior art for the whole proposal, and where it is new, is in §10.

## 6. Migration mechanism

The language has no version marker today. `version-aware-mdl.md` proposes `set version '10.18'`, but that is the **Mendix target version**, a different axis. Proposal:

1. **Deprecated aliases keep parsing** and emit a warning with a stable code (`MDL-DEPR001`, …) that names the canonical form. `describe` never emits a deprecated form.
2. **`mxcli fmt --upgrade`** rewrites every deprecated form to its canonical form. This is mechanical for every alias in §3–§4, and it is what makes consolidation cheap for users.
3. **An optional language header**, `mdl 1;`, as the first statement (decided, §7).
   - `describe` and `fmt` emit it **from beta on**. Before beta, `mdl 1` is a preview that warns "may still change"; at beta it is frozen, and later changes of meaning need `mdl 2`.
   - **With no header, a script gets the alpha meaning (`mdl 0`) plus warnings**, never the latest. This matches the precedents: Rust treats a crate with no edition as the oldest edition, and Go treats a module with no `go` line as the oldest version. The meaning of a script never depends on which mxcli release happens to run it.
   - Under `mdl 1`, the §5 changes of meaning and new rejections apply. Under `mdl 0` they are warnings.
   - After beta, a change that is not backwards compatible bumps the number, and the visitor gates removed aliases on it: under `mdl 1` they warn, under `mdl 2` they are refused.
   - It is independent of the Mendix target version.
4. **Skills, `mxcli syntax` and the quick reference** are regenerated from, or checked against, describe output. They should not be hand-maintained. The skill examples already disagree with the grammar in several places, such as `DELETE_BEHAVIOR PREVENT` and `rename … as`.

### Suggested order

See the implementation plan in §9, which supersedes the short list that was here.

## 7. Decisions

### Decided (2026-09-26)

1. **R1, the idempotent-create keyword: `create or modify`.** It reads like the intent of the command. `modify` changes only what differs: re-running an unchanged definition writes nothing, and Studio Pro shows no change. `create or replace` becomes a deprecated alias.
2. **`show` is dropped in favour of `list`** (R6).
3. **Microflow list operations must feel intuitive to developers who work with microflow diagrams in Studio Pro.** Each statement mirrors one activity: one **List operation** or **Aggregate list** activity per statement, named after the operation Studio Pro offers, and taking the inputs its dialog asks for. Because a diagram cannot nest one activity inside another, the statements cannot nest either (§4, Microflows).
4. **PedApp may be committed** as the Studio Pro-authored round-trip fixture (plan item 0.2).

5. **R3/R4, argument binding: `Param = expr`.** `=` binds a runtime value, `:` sets a model property. There is no `$` on the parameter name. One form for every call site: microflow calls, `show page`, button and menu actions, workflows and REST.
6. **Alter form: `set ( Key: value, … )`.** It is the same property list as `create`, so a fragment from `describe` can be pasted straight into an `alter`.
7. **Brownfield agent editing is a beta goal.** `alter microflow`/`alter nanoflow` (plan item 4.2, at least insert, replace and drop) and the no-op `create or modify` for microflows join the beta gate.
8. **The `mdl 1;` language header is added now** (plan item 1.5). It is an optional first statement. It is a preview until beta, then frozen; `describe` and `fmt` emit it from beta on. After beta, removed aliases are gated by version.

9. **Cadence: one release per week; beta in about four weeks** (around 2026-10-24). Because changes of meaning are tied to the header, no change needs a warning release. Beta is a single release, cut when `mdl 1` is complete. The schedule is in §9.

10. **Drift detection is optimistic locking, and is optional** (§8.2). A `@base '<fingerprint>'` annotation on the statement is emitted by `describe`, checked by `create or modify`, and updated by `exec`. Projects driven entirely by MDL skip it and instead require a dry run of all scripts to report no changes. There is no sidecar state file.

### Still open

None. All decisions needed to start are recorded above.

## 8. Two ways of working: MDL-first and data-first

Most agent work will happen on large existing apps authored in Studio Pro. There, MDL is a *view* over a stored model, not the source of truth. §3–§7 judge MDL as a language for writing. This section judges it as a language for **reading, locating and patching**.

Three measurements were taken on two real apps: PedApp (Mendix 11.13) and Evora Factory Management (Mendix 10.24; 42 modules, 1765 microflows, 212 pages).

### 8.1 Findings

**Reading mostly works, but the answers it gets wrong are dangerous.**

What works:
- `structure -m Module` is a good signature view.
- `select … from CATALOG.*` is cheap once you know the schema.
- `search --format names` is excellent.

Orienting on a realistic task cost about 5–7k tokens. But:
- **`impact`/`refs` has no attribute, enumeration-value, `call workflow`, widget→association or mapping→entity edges.** So `impact Module.Entity.Status` answers "not referenced" for a used attribute. Only a source search (`select … from CATALOG.SOURCE where source match …`) found the real usages. That needs an 8-minute `refresh catalog full source`, and without it `search` returns empty results with no warning.
- **Too many overlapping read commands:** `show`/`describe`, `refs`/`impact`/`context`, `search`/`select … SOURCE`, `show modules`/`structure`. The overlap costs wrong turns more than tokens.
- **No partial reads.** There is no way to describe one widget or one microflow branch; `describe fragment from page … widget` can never succeed (bug). `--json` is not pure JSON on most commands.
- **Layout noise is 30–57% of microflow `describe` output**, depending on the flow. A 123-widget page is about 6k tokens.

**Modifying is safe only through `alter`, and the most-edited document type has no `alter`.**
- **The `alter` control preserved everything.** `alter page … set Caption` changed exactly one en_US string and byte-preserved every other translation and text.
- **The describe → edit → `create or modify` path lost data in 7 of the 12 Studio Pro documents it actually wrote, even with no edit at all:**
  - association storage Table→Column, which is a schema change, and `mxcli diff` reported "no changes";
  - page translations (113→102), and an empty English caption filled from the Dutch one;
  - nanoflow annotation links;
  - export levels on nanoflows and Java actions (the Java action became Public);
  - snippet `Type`.
- **There is no `alter microflow` or `alter nanoflow`.** A one-line insert into a 16-activity Studio Pro microflow required:
  - re-emitting all 107 lines;
  - deleting 5 merges and resetting 11 of 15 curves;
  - changing 51 of 161 element `$ID`s, with some reassigned to *different* nodes;
  - placing the new activity on top of an existing one.

  The root cause is the architecture: `UpdateMicroflow` rebuilds the whole document from the AST and re-pairs IDs afterwards by type and position (`modelsdk/canon/transplant.go`). No per-statement fix can make that a fixed point.
- **`mxcli diff` is not a dry run.** It renders both sides as text for five create statements only. It lists every `alter` as "not compared" and gives false results in both directions.

### 8.2 Two modes: MDL-first and data-first

Neither mode is better than the other in general. Each is the efficient one for a different situation, and MDL has to support both as first-class.

| | **MDL-first (declarative)** | **Data-first (patch)** |
|---|---|---|
| Source of truth | the `.mdl` scripts | the stored model (`.mpr`) |
| Typical use | new apps, new modules, generated scaffolding, documents mxcli owns | existing Studio Pro apps, documents people also edit in Studio Pro, marketplace modules |
| Main statement | `create or modify <type> X ( … ) { … }`, the whole definition (R1) | `alter <type> X { insert … / replace … / set … / drop … }` (§8.3) |
| What the author must know | only the intended end state; no read needed | the current state, well enough to address the target |
| Token cost of a change | proportional to the **document** | proportional to the **change** |
| Reviewability | the script *is* the design; a PR diff shows the new definition | the patch *is* the intent; a PR diff shows exactly what was changed and where |
| Identity | carried by name, so renames need `alter`/`rename` (R1) | untouched elements are byte-identical by construction |
| Risk | drops whatever the statement does not say, including content MDL cannot express | an ambiguous or stale address; needs strict matching (§8.3) |

**For a new app, declarative is clearly more efficient.** There is nothing to read, nothing to address and nothing to preserve. Each element is written once, in its final shape, in the order a reader wants to see it. A patch-only style would force an agent to create empty shells and then fill them in with a sequence of edits: more tokens, more statements, and a script that describes a history rather than a design. Declarative scripts are also what an LLM generates most reliably, since one example generalises (ADR-0003).

**For a small change to a large existing document, patching is clearly more efficient and safer.** It costs O(change) instead of O(document), and it cannot disturb what it does not mention (§8.1).

**The crossover** depends on the change and on who owns the document:
- A change that rewrites most of a document is cheaper declaratively, even on an existing app, *provided* the document round-trips (§8.4).
- A change that touches a small part is cheaper as a patch, even on an app built MDL-first.

**The modes are phases of one app's life, not a choice made once.** An app typically starts MDL-first. Then it is opened in Studio Pro, and from that point the stored model can drift away from the scripts. The practical question per document is therefore *"is the stored document still exactly what my MDL last produced?"*:
- **Yes:** declarative replace is safe and cheapest.
- **No** (someone edited it in Studio Pro, or it was never MDL): patch, or re-adopt it as MDL source first by running `describe`. Re-adopting is safe only for types whose round trip is proven.

That question can be answered mechanically, with **optimistic locking**: the same scheme as HTTP's `If-Match: <etag>`. There are two forms, one per kind of project.

**1. `@base`: a per-statement version stamp, for mixed projects.**

`describe` emits an annotation carrying the fingerprint (a hash of the canonical BSON) of the stored document it read:

```mdl
@base 'sha256:3f9a…'
create or modify persistent entity Shop.Order (
  Number: integer,
  Note: string(200),
);
```

- `create or modify` checks the annotation against the stored document and writes only if the two still match. Then `exec` rewrites the `@base` lines of the file it applied, so the next edit starts from the new version. It only updates annotations that are already there.
- The describe → edit → modify loop an agent uses on an existing app is therefore locked end to end, with no state file. The version travels with the statement through git, copy-paste and review. That keeps statements self-contained (ADR-0003).
- It is an **annotation, not a comment**. Formatters and LLMs strip or copy comments freely; an annotation has defined meaning and is validated. A statement copied to create another document carries a `@base` that does not match, and fails safely.
- The diff noise is small. The fingerprint changes only when the document changes, which happens because its statement was edited, so both land in the same hunk. When two developers apply different versions of one statement, the result is a git conflict on that line: a real conflict, shown where conflicts are already resolved.

| Statement | Stored document | Result |
|---|---|---|
| no `@base` | missing | create |
| no `@base` | exists | modify to match (a no-op if nothing differs), exactly as without locking |
| `@base` matches | exists | modify, and `exec` updates `@base` |
| `@base` differs | exists | refused: "changed since you read it"; use `alter`, re-`describe`, or `--force` |

**`@base` is optional, and provisional** (ADR-0012). A statement without it behaves exactly as it does today. Users may dislike stamps in their scripts, and CI/CD pipelines, which usually check out read-only and do not commit back, may not support a tool that rewrites them. It is therefore validated with users and in a pipeline before it is built. If it fails, it is replaced by a lock kept outside the script: a committed state file, or the base stored alongside the model.

**2. A dry run of all scripts must report no changes: for projects driven entirely by MDL.**

Nothing but the scripts writes to such a project, so no per-statement lock is needed. Drift still happens in three ways:
- someone opens the app in Studio Pro (for example, a Mendix version upgrade rewrites documents);
- a marketplace module is updated;
- a one-off `alter` runs against the model and is never added to the scripts. The next apply silently undoes it, which makes this the main risk.

Because `create or modify` writes nothing when nothing differs (R1), **any change reported by `mxcli exec --dry-run scripts/` is drift by definition**. Run in CI, that one check protects the whole project with no annotations. The rule for such projects: change the scripts, not the model.

| Project style | Protection |
|---|---|
| Entirely MDL-driven | no `@base`; CI requires a dry run of all scripts to report no changes |
| Mixed (Studio Pro users, or agents patching existing documents) | `@base` on the statements `describe` produced |

The two can be combined, for example scripts owning some modules and Studio Pro users owning others.

**After beta.**
- **Finer-grained locking.** A whole-document check refuses non-conflicting edits: someone else added `Discount` while you changed `Note`. A per-element check refuses only when an element this statement would change or drop was changed by someone else. `alter` is already this fine-grained: a patch needs only its target to exist and be unambiguous.
- **Three-way merge.** A hash only detects a conflict. A merge also needs the base content, and git already has it: the previous version of the same statement. With base, theirs and yours, non-conflicting edits merge automatically, and real conflicts are reported per element.
- **Check and write together.** With `--mcp` the project is open in Studio Pro, so the backend must compare and write in one step, or there is a window for a lost update.

This makes the choice between modes visible and safe, instead of something the agent has to remember.

**Language consequences.** The two modes must share one syntax, so that moving between them is free:
- A fragment inside `alter … { … }` is written exactly as in `create`.
- `describe` output is valid declarative source.
- An agent should never have to learn a second language to switch modes, and a reviewer should see the same constructs either way.

This is another reason for R2's uniform node shape and R12's canonical describe.

### 8.3 The model is data: edits are tree patches

MDL describes stored data, so it can be manipulated the way Lisp manipulates code: the text is a structure, and edits are structural operations on it. Five consequences follow.

1. **One generic `alter` for every document type.** Rule R2 gives every child the shape `<kind> [Name] ( props ) { children }`. That rule is what makes a single patch grammar possible, replacing about 15 bespoke `alter` forms:

   ```mdl
   alter <type> Module.Name {
     set ( Key: value ) on <target>;
     insert before|after|into <target> { <fragment> }
     replace <target> with { <fragment> }
     drop <target>;
   }
   ```

   Fragments are written in exactly the syntax `create` uses; `alter page` already works this way. This generic form is also the strongest argument for settling R2 before beta.

2. **Content addressing where there are no names.** Pages address widgets by name. Microflow activities have no names, so they are addressed by what they are, in this order of preference:
   1. by output variable: `after $Lines`;
   2. by caption: `before 'Email is valid?'`;
   3. by statement pattern with wildcards: `after commit $Order`, `drop log * node 'Debug' *`.

   A pattern that matches more than one activity is an error that lists the matches with an ordinal (`@2`); it is never a guess. `describe` can mark targets so an agent sees which handle to use.

   ```mdl
   alter microflow Shop.ACT_CancelOrder {
     insert after commit $Order {
       call microflow Shop.SUB_EmailCustomer(Order = $Order);
     }
     replace retrieve $Lines with {
       retrieve $Lines from Shop.OrderLine where [Shop.OrderLine_Order = $Order];
     }
   }
   ```

3. **A patch splices into the stored graph; it does not rebuild it.** The executor rewires sequence flows around the target and places only the new nodes, shifting downstream nodes rather than overlapping them. Every untouched activity, flow, curve and merge stays byte-identical. The microflow is a graph, not a tree:
   - Tree edits apply to its structured regions (`if`, `loop`, error handlers).
   - Unstructured regions are addressed through the `join`/`merge` labels that `describe` already prints.
   - An inserted fragment is checked in the scope of its insertion point, for macro hygiene: a variable it declares must not collide with one declared further down.

4. **Declarative `create or modify` is a diff that produces a patch.** Given the decided R1 semantics (change only what differs), `create or modify` is implemented as: compare the declared definition with the stored document, derive the minimal patch, and apply it with the same splice engine `alter` uses. An empty patch means no write. The two modes of §8.2 therefore share one engine, and MDL-first scripts gain the same no-churn guarantee as data-first patches. This is the reconciliation model of Terraform's plan/apply.

5. **Bulk patches are "macros":** a query plus a patch.

   ```mdl
   alter microflows in Shop where contains (commit $Order) {
     insert after commit $Order { call microflow Shop.SUB_Audit(Order = $Order); }
   }
   ```

### 8.4 The two round-trip laws

Code-as-data only works if printing and reading are inverse operations. Formally, `describe` and `create or …` form a *lens* over the stored model, and they must obey two laws:

- **GetPut:** executing `describe X` unchanged writes nothing. ADR-0008's write elision already covers this at the storage level, but today it is violated in 7 of 12 cases (§8.1).
- **PutGet:** executing a statement, then describing the result, returns what was written. This law is R12: no invented names, no derived layout, no defaults.

Content MDL cannot express must never be silently dropped. It needs one of two things:
- **An opaque passthrough placeholder** that `describe` emits and a replace carries through, e.g. `preserved activity 'a1b2…';`.
- **A refusal:** "this rebuild would drop 3 translations; use `alter`".

Silent loss is never acceptable.

### 8.5 Recommendations (in order)

1. **Guidance now: choose the mode by who owns the document (§8.2).**
   - New apps and modules: write declarative MDL.
   - Existing Studio Pro documents: change them with `alter`.
   - describe → replace only for documents whose stored state is still what MDL produced, and never on a Studio Pro-authored document of a type without a proven round trip.
2. **Drift detection** (§8.2): the optional `@base` annotation for mixed projects, and a no-changes dry run in CI for projects driven entirely by MDL. Until then, the guidance in step 1 is the only guard.
3. **A CI round-trip test** on a Studio Pro fixture: describe → exec → canonical BSON compared per unit, covering every document type. It would have caught every loss in §8.1. Fix those losses, or turn them into refusals.
4. **`alter microflow` / `alter nanoflow`** with content addressing and graph splicing (§8.3). This is the single largest gap for brownfield work.
5. **A real dry run.** Execute on an in-memory copy and diff canonical BSON per unit. The machinery exists in `canon.Reconcile`. Until then, `diff` saying "no changes" is not evidence of no change.
6. **Read side:**
   - Complete the refs graph (attributes, enum values, `call workflow`, widget bindings, mappings) and add a typed `usages <Entity.Attr | Enum.Value>`.
   - Add partial and outline reads: `describe … brief`, `describe microflow … without layout`, `describe page X widget Y`, `describe page X outline`.
   - Make `--json` strict and uniform: stdout is JSON only.
   - Make the source index incremental, or say it is missing.
   - Collapse the overlapping read commands along the R6 verb table.
7. **Settle R2 and R12 before beta.** Both are preconditions for the generic `alter`, not just style rules.

## 9. Implementation plan

The plan has six phases:
- Phases 0 and 1 are foundations: a safety net, and the machinery that makes syntax changes cheap.
- Phase 2 is the **beta gate**: the changes that cannot be bridged by an alias.
- Phases 3–5 can continue past beta, because every change in them keeps the old form parsing.
- Phase 4 (data-first editing) depends only on phases 0 and 1, so it can run in parallel with phases 2 and 3. Items 4.1 and 4.2 are on the beta gate, because brownfield editing is a beta goal (§7). Start them early: they are the longest items in the plan.

Every item follows the repo's working rules (CLAUDE.md):
- One concern per PR.
- The test is written first.
- The fix is proven by reverting it.
- Any "nothing changed" assertion comes with a control.

Sizes are rough: **S** is about 1 PR, **M** is 2–4 PRs, **L** is a series.

```
Phase 0  safety net + bugs ──┬──> Phase 1  decisions + deprecation machinery ──> Phase 2  non-bridgeable (BETA GATE) ──> Phase 3  aliases, area by area
                             └──> Phase 5  read side (any time)
                                  Phase 1 ──> Phase 4  data-first editing (alter microflow, drift, dry run)
```

### Schedule (weekly releases; beta when `mdl 1` is complete)

**No change needs a warning release.**
- Changes of meaning apply only under `mdl 1;` (§5), so a script without the header keeps its meaning on every release.
- Changes of spelling keep the old form as an alias.
- The old output of `describe` has no header, so it keeps running with the old meaning.

Beta can therefore be a single release, cut whenever `mdl 1` is complete and verified. The order below is **build order**: the registry (1.2) and the header (1.5) come first because everything else is built on them. Weekly releases ship progress safely along the way.

**`mdl 1` is a preview until beta, then frozen.** Parts of `mdl 1` ship in weekly releases before beta, so its meaning could still shift between those releases. To stop anyone depending on an unfinished `mdl 1`:
- Before beta, `mdl 1;` parses but warns "preview: may still change".
- `describe` and `fmt` do **not** emit the header before beta; `fmt --upgrade` adds it only on request.
- At beta, `mdl 1` is frozen, and `describe` and `fmt` start emitting it. From then on, any change of meaning needs `mdl 2`.

| Order | Lands | Parallel track (Phase 4) |
|---|---|---|
| **1** | ADR (1.1); deprecation registry (1.2); `mdl 1;` header as a preview (1.5); round-trip harness and PedApp fixture (0.1, 0.2); first bug fixes (0.3–0.5); interim skill guidance (0.6) | generic `alter` skeleton (4.1); `mfmutator` target resolver (4.2a) |
| **2** | `mdl 1` semantics behind the preview header (2.1, 2.2, 2.4, 2.5); `fmt --upgrade` (1.3) | graph splice and placement (4.2b, 4.2c) |
| **3** | Canonical `describe` (2.6); Phase 3 canonical forms added to the grammar, with the old forms as aliases; conformance gate (1.4); dead grammar removed (2.3) | `insert`, `replace`, `drop`; diff-then-patch `create or modify` for microflows (4.2e, 4.2g) |
| **Beta** | `mdl 1` is frozen, documented, and emitted by `describe`/`fmt`. Scripts without the header keep working, with warnings. | 4.2 acceptance test on `VAL_Feedback` passes |

At a weekly cadence this is still roughly four weeks, but no date is imposed by compatibility.

**Risk.** Item 4.2 is the largest and the riskiest code (the graph splice). If its acceptance test is not green when everything else is, slip beta. Slipping now costs nobody compatibility, so it is the cheap option; the alternative is shipping `alter microflow` as experimental. Phase 3's aliases and Phase 5 continue after beta; only the canonical *forms* have to be in the grammar and in `mdl 1` by then.

### Phase 0: safety net and bugs (no syntax change)

| # | Item | Size | Where | Done when |
|---|---|---|---|---|
| 0.1 | **Round-trip harness.** For every document in a Studio Pro-authored fixture: `describe` → `exec` → canonical BSON compared per unit. Known failures go in an allowlist that may only shrink. | M | integration test (`-tags integration`); compare with `canon` | runs in CI; the allowlist equals the §8.1 losses |
| 0.2 | **Fixture.** Commit PedApp, a Studio Pro-authored project (cleared for use, §7), which has microflows, nanoflows, pages, snippets, translations and table-stored associations. Add a workflow and REST documents in Studio Pro. Evora (`mx-test-projects/`) serves as a large, uncommitted read-side benchmark. | S | `testdata/` | fixture in the repo |
| 0.3 | **Silent drops from §2:** `throw`; `float`/`currency`/`date`; the parenthesised association form; enumeration-value doc comments and index names; describe output that doesn't re-parse (#8); the plaintext password (#9); `/tmp` image paths (#10). | M | visitor, executor describe | each has a failing test first; §2 closed |
| 0.4 | **Round-trip losses from §8.1:** association storage, page translations, nanoflow annotations and export level, Java action export level, snippet `Type`, describe emitting a plain `create`. Tracked as separate tasks. | M | executor create paths | removed from the 0.1 allowlist |
| 0.5 | **Read-side wrong answers from §8.1:** `describe fragment`, `structure` counts, silent empty `search`, strict `--json`. Tracked as a task. | S | executor | tests |
| 0.6 | **Interim agent guidance:** add §8.2 "choose the mode by owner" and "change existing documents with `alter`" to `write-microflows`, `alter-page` and the other doctype skills; run `make sync-skills`. | S | `.claude/skills/mendix/` | skills merged |

### Phase 1: decisions and infrastructure

| # | Item | Size | Where | Done when |
|---|---|---|---|---|
| 1.1 | **Resolve the remaining open questions in §7** and record all decisions. Write an ADR ("MDL canonical syntax: R1–R12, two modes") that extends ADR-0003, and turn R1–R12 into checklist items in `design-mdl-syntax.md`. Mark the v1/v2 syntax proposals **rejected**; fold the workflow alignment proposal into R4/R9. | S | `docs/13-decisions/`, skill | ADR accepted |
| 1.2 | **Deprecation registry.** Generalise the existing MDL065 pattern, where the AST records which spelling the source used (`ast_microflow.go:273`). Add a single table of `{code MDL-DEPRnnn, old form, canonical form, removed in}`. `check` and `exec` warn; a `--deprecations=error` flag fails instead. | M | `mdl/ast`, `mdl/linter`, new `deprecations.go` | a test fails if a grammar alternative marked as an alias has no registry entry |
| 1.3 | **`mxcli fmt --upgrade`.** Each registry entry carries a rewrite, built on `mdl/formatter` / `cmd_fmt.go`. The output must parse with zero deprecation warnings and give an identical AST. | M | `mdl/formatter` | a property test over all of `mdl-examples/`: upgrade, then zero warnings, then AST equal |
| 1.4 | **Canonical-form conformance gate.** Everything in `mxcli syntax`, the skills, `docs-site/` and `mdl-examples/` is parsed with `--deprecations=error`, starting with an allowlist. This stops the docs teaching old forms, a gap measured in §3. | S | `make lint` target | allowlist only shrinks |
| 1.5 | **Language header `mdl 1;`** (decided, §7). It is parsed and emitted by `describe`/`fmt`, and gates alias removal after beta. | S | grammar, visitor | round-trips |
| 1.6 | **Grammar hygiene** that makes later phases cheaper: one shared trailing-comma list rule; ban bare `IDENTIFIER` in parser rules with a test modelled on `keyword_coverage_test.go`; move session commands out of `utilityStatement` into the REPL (R7). | M | `mdl/grammar` | tests; `make grammar` clean |

### Phase 2: the beta gate (§5, cannot be aliased)

Each change lands with a registry entry, an `fmt --upgrade` rewrite where one is possible, and release notes. Changes of meaning and new rejections are tied to the `mdl 1` header, so none of them needs a warning release.

| # | Item | Size | Compatibility | Done when |
|---|---|---|---|---|
| 2.1 | **R1: `create or modify` is the keyword; `or replace` becomes an alias.** View entities get an identity-carrying rewrite instead of delete-and-recreate; `if not exists` works on every type. The no-op guarantee (an unchanged definition writes nothing) is enforced by the 0.1 harness for every type except microflows and nanoflows, which need 4.2. | M | `or replace` warns | GUID preserved on a view-entity replace (a test with a GUID != `$ID` control); re-running unchanged `describe` output writes no unit |
| 2.2 | **Strictness:** unknown or mis-shaped property keys are errors (REST, agents and business events first, then everywhere); `;` required; `/` removed; `\` is no longer an escape. | M | tied to the header: errors under `mdl 1`, warnings without it | parse tests under both versions |
| 2.3 | **Dead grammar removed:** `grant … on workflow`, `case … else`, `text`/`statictext`/`legacydatagrid`, `throw` (if not fixed in 0.3). | S | none (they never worked) | parse errors with a hint |
| 2.4 | **Microflow assignment and list operations.** `set` becomes mandatory. List operations and aggregates become one statement per Studio Pro activity (`$A = filter $L by …`, `$B = filter $L where …`, `$n = count $A`; the full table is in §4), so nesting no longer parses. The `find`/`contains` ambiguity disappears; MDL-LISTOP02 becomes unreachable and is retired. The first PR checks the keywords against Studio Pro's dialog labels. | L | old function form warns (except `find`/`contains`, which can't) | `write-microflows` skill and examples migrated by `fmt --upgrade` |
| 2.5 | **`retrieve … first` vs `limit 1`.** Under `mdl 1`, `limit 1` means a list and `first` an object. Without the header, the old meaning plus a warning. `describe` emits `first` together with the header. | M | tied to the header | MDL-RETRIEVE01 retired |
| 2.6 | **Canonical `describe` (R12), one area per PR:** omit derived layout (extend the `@start` derived-vs-authored rule to `@position`/`@curve`/`@anchor`); omit defaults; no synthetic widget names (make the name optional in `widgetV3`, address grid columns explicitly); fold `join`/`merge` back into `case` and fall-through handlers; emit `elsif`. | L | none (output change) | the harness in 0.1 stays green; PutGet holds per area |

**Beta gate:**
- Phases 0, 1 and 2 are complete.
- Phase 3's canonical forms are *decided and in the grammar*. Aliases may remain.
- Phase 4.2 has at least insert, replace and drop, and `create or modify` on a microflow is a no-op when nothing differs (decided, §7: brownfield agent editing is a beta goal). Phase 4.1 (the generic `alter`) comes before it.

### Phase 3: consistency through aliases (R2–R10, §4)

Every item follows the same recipe:
1. Add the canonical form to the grammar.
2. Register the old form as a deprecation (1.2) with an `fmt --upgrade` rewrite (1.3).
3. Switch `describe` to the canonical form.
4. Migrate examples, skills and `mxcli syntax` with `fmt --upgrade`.
5. Shrink the 1.4 allowlist.

Order, by how many existing scripts each item touches:

| # | Rule | Size |
|---|---|---|
| 3.1 | R3/R4: `:` vs `=`; one argument-binding form; `with ({1} = …)` for text templates | L |
| 3.2 | R8: page actions as words (`show page`, `save changes`); one spelling per keyword; `error message`; lowercase canonical (fixes `fmt` upper-casing keys) | M |
| 3.3 | R5: XPath in `[ ]`, bare expressions (grants, workflow targeting and decisions, `Visible`/`Editable`); one constant reference | M |
| 3.4 | R2: brackets (REST client, agents, image collection, database connection, message definitions, navigation/menus, `on error begin … end error`, `while`) | L |
| 3.5 | R6/R7: drop `show` in favour of `list`/`describe`; `describe widget type` rename, and a not-executable header plus `--json` for definition reports; cross-links between `syntax` and `describe`; `drop` instead of `remove`; missing `drop`s; `call rest service` | M |
| 3.6 | R9/R10: metadata placement (doc comment, `folder` clause); Studio Pro document names (`consumed rest service`, …); property lists for role, constant, demo user and settings headers | M |
| 3.7 | §4 area items: domain-model aliases and inline validation rules; widget types resolved through the registry and the property-key convention; unified mapping grammar; HTTP concepts; workflow `caption`/`->`/`alter workflow` vocabulary | L |

### Phase 4: data-first editing (§8)

| # | Item | Size | Where | Done when |
|---|---|---|---|---|
| 4.1 | **Generic `alter <type> X { set / insert / replace / drop }`.** One grammar rule plus a per-doctype *target resolver* interface. `alter page`/`snippet`/`layout` (`pagemutator`) and `alter workflow` (`wfmutator`) are ported onto it first, with their old forms as aliases. | M | grammar, `mdl/backend` | existing alter tests pass through the new path |
| 4.2 | **`alter microflow` / `alter nanoflow`**, built as an `mfmutator` alongside `pagemutator` and `wfmutator`. The same engine then implements `create or modify` for microflows (step g), which is what makes the R1 no-op guarantee hold for them: | L | `mdl/backend/modelsdk`, `mdl/microflowgraph` | see acceptance below |
| | a. **Target resolver.** Address by output `$var`, by caption, or by statement pattern with `*` wildcards; `@n` disambiguates, and ambiguity is an error listing the matches. Add `describe … with handles` to show the addresses. | | | |
| | b. **Graph splice** on the *stored* object collection. Rewire the incoming and outgoing sequence flows around the target; build only the fragment's objects; never call the whole-document `UpdateMicroflow` rebuild. The write goes through `canon.Reconcile` (CLAUDE.md rule 2). | | | |
| | c. **Placement.** Put the new node on the flow's midpoint and shift downstream nodes; never overlap. | | | |
| | d. **Hygiene.** Check the fragment in the scope of the insertion point; a variable collision is an error. | | | |
| | e. **Operations,** in order: `insert after`/`before` → `replace` → `drop` → `set` (expression, caption, `on error`) → `add`/`drop parameter`. | | | |
| | f. **Both backends:** the modelsdk engine and `--mcp` (the Studio Pro MCP backend), or an explicit "not supported by this backend" error. | | | |
| | g. **Declarative `create or modify` as diff-then-patch:** match the declared flow against the stored one (by statement signature and output variable, as in 4.2a), derive the minimal set of insert/replace/drop operations, and apply them with the splice from 4.2b. An unchanged definition yields an empty patch and no write. This replaces `UpdateMicroflow`'s whole-document rebuild. | | | |
| 4.3 | **Drift detection as optimistic locking (provisional; not on the beta gate).** Before building, validate `@base` with users and in a CI/CD pipeline (ADR-0012). `exec --no-stamp` skips rewriting stamps, for pipelines. `describe` emits `@base '<fingerprint>'` (canonical BSON hash per unit, computed with `canon`). `create or modify` refuses when a present `@base` does not match, with `--force` to override. `exec` rewrites existing `@base` lines after applying. The check and the write are atomic on both backends (modelsdk and `--mcp`). A statement without `@base` behaves exactly as before. | M | grammar (annotation), `modelsdk/canon`, executor | a Studio Pro edit between `describe` and `modify` is refused; a control with no edit applies; `exec` updates the stamp |
| 4.4 | **A real dry run** (also the drift check for projects driven entirely by MDL, §8.2): `exec --dry-run`, replacing today's `diff`. Execute on an in-memory copy, diff canonical BSON per unit, and render changed units as a `describe` diff. Covers `alter` and every document type. | M | executor, `canon` | the §8.1 false negative (association storage) and false positives disappear |
| 4.5 | **Opaque passthrough or refusal** for content MDL cannot express, per document type as the 0.1 harness finds it: `preserved <kind> '<id>'` in `describe` output, carried by replace. | M | per doctype | no silent loss remains in the harness |
| 4.6 | **Bulk patches:** `alter microflows|pages in M where contains (<pattern>) { … }`, building on 4.2a. `alter pages … where` and `update widgets` are folded into it as aliases. | M | grammar, executor | — |

**Acceptance for 4.2** on the Studio Pro fixture's `VAL_Feedback`:
- Insert one `log` after `$IsValidEmail`.
- Only the new activity, the two rewired flows and any shifted positions differ in canonical BSON. Every other element's `$ID`, curve and merge is byte-identical.
- Control: an empty `alter` changes nothing.
- `mx check` is unchanged from the baseline, and Studio Pro opens the project.
- An MDL script of at most 5 lines replaces today's 107.
- Re-running the unchanged `describe` output as `create or modify` writes nothing (GetPut), with an edited-flow control that does write.

### Phase 5: read side (§8.1; any time)

| # | Item | Size |
|---|---|---|
| 5.1 | **Complete the refs graph:** attribute and enumeration-value targets (expressions, XPath, widget bindings, `ContentParams`, change/create members, mappings); `call workflow` edges; widget→association; mapping, OData and REST→entity. `impact` groups by element and has a kind column. | L |
| 5.2 | **`usages <Entity.Attr \| Enum.Value>`**, and typed `callers`/`callees`. | S |
| 5.3 | **Partial and outline reads:** `describe … brief` (signatures only), `describe microflow … without layout`, `describe page X widget Y`, `describe page X outline`. | M |
| 5.4 | **Strict, uniform `--json`:** JSON only on stdout, status on stderr, one `{kind, qualifiedName, …}` shape. | M |
| 5.5 | **Incremental source index,** or a warning when it is missing. Uniform catalog column names (`QualifiedName` everywhere); hide snapshot columns from `select *`. | M |
| 5.6 | **Collapse overlapping read commands** per R6. Remove `show <element>` in favour of `describe`; merge `refs`/`impact`/`context`; document `search` as sugar over `CATALOG.SOURCE`. | M |

### Risks

- **`describe` output changes (2.6, Phase 3) churn MDL that users have committed.** Mitigation: ship them together in as few releases as possible before beta, with `fmt --upgrade` and a changelog entry per change.
- **Grammar changes ripple into generated artefacts:** LSP completions (`lsp_completions_gen.go`), `keyword_coverage_test.go`, the VS Code extension and the embedded skills. Each grammar PR runs `make build`, which regenerates them, and `make sync-skills`.
- **The graph splice (4.2b) is the riskiest code.** It must never rewrite an `$ID` without rewriting every reference to it (CLAUDE.md rule 1), and its tests must use a Studio Pro-authored flow, because an mxcli-created flow cannot show identity loss (GUID == `$ID`).
- **Scope creep in Phase 3.** Hold each rule to its recipe; anything beyond renaming belongs in its own proposal.

## 10. Prior art: what is proven and what is new

Nearly every construct in this proposal has a well-proven precedent. The ADR's *alternatives considered* can cite this table.

| Proposed construct | Precedent | How close |
|---|---|---|
| `create`/`alter`/`drop`/`list`/`describe` | SQL DDL | exact (ADR-0003) |
| `create or modify` that changes only what differs | `kubectl apply`, Terraform plan/apply, declarative schema tools (Atlas, Skeema) | close. The principle is proven; the difficulty is in the implementation (see below). |
| Declarative modify as diff-then-patch (§8.3) | React reconciliation, Kubernetes controllers, Terraform | close |
| Content addressing (`after $Lines`) | React's `key` prop | close. React needs keys because matching by position produces wrong pairs; `canon/transplant.go` measured exactly that (§8.1). The output variable is the key. |
| `@base` optimistic locking | HTTP `ETag`/`If-Match`, Kubernetes `resourceVersion` | exact in principle |
| Three-way merge (after beta) | `kubectl apply` (it keeps the last applied configuration as an annotation for this), git | very close |
| Dry run in CI as the drift check | `terraform plan -detailed-exitcode` | exact |
| `()` properties, `{}` children (R2) | QML, SwiftUI, Flutter, HCL | close; QML is almost the page syntax |
| `:` declares, `=` binds (R3/R4) | Kotlin and Swift (`name: Type` vs `= value`), Python keyword arguments | close |
| Pattern-based bulk patches (§8.3) | Coccinelle (Linux kernel), OpenRewrite (Java), ast-grep, codemods | proven at large scale on code |
| Deprecation aliases, `fmt --upgrade`, header-tied changes of meaning | `go fix` with the `go` line in go.mod; Rust editions with `cargo fix --edition` | exact |
| One canonical form, emitted by the tool (R12) | `gofmt`, `terraform fmt`, Prettier | exact |
| The two round-trip laws (§8.4) | *lenses*, i.e. bidirectional transformations (Foster, Pierce et al.; Boomerang); Terraform's "no changes after import" | exact in theory, well known in practice |

**What is new or less proven, and therefore where the risk sits:**

1. **Patching a flowchart, not a tree.** Pattern-based patching is proven on code syntax trees. Splicing into a diagram graph, with sequence flows, merges and layout, has no textual precedent we know of; BPMN modelers do it internally, behind a UI. This is plan item 4.2b, the riskiest code in the plan, and its acceptance test runs on a Studio Pro-authored flow.
2. **Writing the version stamp back into the source.** Most tools keep this state separately: Terraform's state file, lockfiles, the Kubernetes server. A tool that rewrites your script is unusual, and some users will object. The mitigations: `@base` is optional; `exec` only updates stamps that are already there; projects driven entirely by MDL never use it.
3. **Round-tripping a model that MDL cannot fully express.** The principle is proven, but Terraform's long history of "perpetual diff" provider bugs shows how hard the practice is. Lens theory says a view that does not capture everything needs a *complement*: the hidden part carried alongside. The proposed `preserved` placeholder (§8.4) is that complement. The 7 losses in 12 writes measured in §8.1 are this problem, so the round-trip harness (0.1) is a precondition, not an optional extra.
