# Language Versions and Migration

An MDL script is written in one **language version**, declared by its first
statement:

```sql
mdl 1;

create persistent entity Shop.Customer (
  Name: String(200)
);
```

A script file with no header is **mdl 0**, the alpha language. The rest of
this manual documents `mdl 1`; this page is the one place that documents what
`mdl 0` means differently, and how to move a script from one to the other.

Two rules make the move safe ([ADR-0011](https://github.com/mendixlabs/mxcli/blob/main/docs/13-decisions/0011-mdl-language-versioning.md)):

- **A script never changes meaning because a newer mxcli runs it.** A change of
  meaning, or a new rejection, applies only under the header that introduces
  it. Without the header the old meaning is kept, and the construct warns with
  an `MDL-V1-*` code.
- **A deprecated spelling is a respelling.** It means exactly what its new form
  means, warns with an `MDL-DEPR*` code, and `mxcli fmt --upgrade` rewrites it.
  It is refused only from the version named in its entry.

Every warning that carries one of these codes ends with a pointer such as
`(mxcli help MDL-V1-LIMIT1)`. That command prints the code's entry: the old
form, the new form, whether `fmt --upgrade` rewrites it, and the version that
refuses it. `mxcli syntax <topic> --deprecated` lists the old spellings of one
topic.

## mdl 1 is frozen

`mdl 1` was frozen at beta. It is a contract: a later change of meaning needs a
new version, `mdl 2`, and a script headed `mdl 1;` means the same under every
later mxcli release. What that changes in practice:

| Where | Since the freeze | To get `mdl 0` |
|---|---|---|
| `describe` (statement and `mxcli describe`), `mxcli context`, `mxcli diff-local` | Writes `mdl 1`, and every description starts with `mdl 1;`, so it runs as it is. Descriptions concatenated into one file repeat the header; a repeated header naming the same version is allowed. Describe never writes a deprecated spelling, in either version: a List operation or Aggregate list activity is its statement (`$n = count $L;`), never the call (`count($L)`). | `--mdl 0` on the command (no header is written) |
| `mxcli fmt --upgrade` | Adds the `mdl 1;` header by default, after the rewrites below. | `--header=false` upgrades the spellings alone |
| `mxcli fmt` (without `--upgrade`) | Keeps the script's header, or its lack of one: formatting never changes a script's language. | — |
| The REPL and `mxcli -c "…"` | Input without a header is read as `mdl 1`. Session commands (`connect`, `set format`, `status`, …) and a missing `;` after the last statement stay allowed: this is the session they belong to. | `--mdl 0` on the command line; in the REPL an `mdl 0;` statement switches the session (and `mdl 1;` back). The REPL is the only place a header may change the language part-way |
| A script file run with `exec`, `check` or `fmt` | Unchanged: read by its own header, and as `mdl 0` without one ([ADR-0011](https://github.com/mendixlabs/mxcli/blob/main/docs/13-decisions/0011-mdl-language-versioning.md)). | — |

Before the freeze `mdl 1;` warned "preview: may still change" (`MDL-LANG01`);
that warning is gone. A header after the first statement must name the same
version as the first: `mdl 0;` after `mdl 1;` is an error, and so is `mdl 1;`
part-way through a script that started without one.

## What a script without the header means

Each construct below is refused under `mdl 1;` and accepted without the header,
where it keeps its old meaning and warns with the code shown.

| Under `mdl 1;` | Without the header |
|---|---|
| A statement without `;` is an error. | Accepted; `MDL-V1-SEMI`. |
| A `/` terminator line is an error. | Accepted; `MDL-V1-SLASH`. |
| `''` is the only string escape; a backslash is an ordinary character, so `'C:\temp'` is that path. | `\n`, `\r`, `\t`, `\\` and `\'` are escapes; `MDL-V1-ESCAPE` for each literal whose value would change. |
| A text template (`log`, `show message`, `validation feedback`) written as one literal is the template text even when it spans lines, which is how a template holds a line break. | A template literal spanning lines with no `with ({n} = …)` parameters is an expression: the template is `{1}`, the literal its parameter; `MDL-V1-TEMPLATE`. |
| In a REST client, published REST service, business event service, model, knowledge base, consumed MCP service or agent, an unknown property key is an error that names the key it most likely meant, and so is a value its key does not take (`Response: json from $X`). | The property is ignored, or read by its shape as before; `MDL-V1-PROP` / `MDL-V1-PROPVALUE`. |
| A `while` loop is `while <condition> begin … end while;`; leaving out `begin`, or the `while` after `end`, is an error. | Accepted; `MDL-V1-WHILE`. |
| `show entity X` / `show association X` is an error: `describe entity X` prints the definition, `list entities in M` the summary columns. | Prints the summary; `MDL-V1-SHOWSUMMARY`. |
| A session command — `connect`, `disconnect`, `use`, `set format = …`, `status`, `show version`, `show status`, `show connections`, `show catalog status`, `check`, `build`, `lint`, `debug`, `execute script`, `execute runtime`, `help`, `introspect api` — is an error in a script. Type it at the REPL, or use the command-line flag (`mxcli exec script.mdl -p app.mpr --json`). The REPL keeps accepting them. | Runs as before; `MDL-V1-SESSION`. |
| A quoted value of `DynamicClasses`, `DynamicCellClass` or an OData client's credential or header value is a Mendix string, so the pre-#750 spelling — the expression's text in quotes, `'if $currentObject/X then ''on'' else '''''`, `'''admin'''`, `'@Mod.C'` — is an error (MDL-WIDGET33, MDL-ODATA07). Write the expression bare. | The quoted text is the expression it holds; `MDL-V1-QUOTEDEXPR`. A quoted class name or plain credential is the string under both. |

### Strings without the header

A script without the header (`mdl 0`) still reads a backslash before `n`, `r`, `t`, `\` or `'` as an escape — `'C:\temp'` holds a tab — and `check` warns `MDL-V1-ESCAPE` for each literal whose value changes under `mdl 1`. Write the backslash-free form (`'it''s'`) to mean the same under both.

In a microflow or nanoflow expression, without the header `'C:\\temp'` stores `'C:\temp'` and `'it\'s'` stores `'it''s'`: the stored expression is the same value, spelled as Studio Pro spells it.

`describe --mdl 0` keeps the `mdl 0` escapes (`\n`, `\\`): in a flow's expression and an XPath constraint it writes a stored backslash as `\\`, so the description reads back as the stored value under `mdl 0`. `mxcli fmt --upgrade` rewrites an `mdl 0` script's escaped strings to the `mdl 1` form. An expression whose string holds an escaped line break is written as the expression Mendix stores for it, since an expression that spans lines is stored exactly as written.

Without the header a template literal that spans lines and has no parameters is an expression instead — the template is `{1}` and the literal its parameter — and `check` warns `MDL-V1-TEMPLATE`. With `with ({n} = …)` parameters it is the template text under both versions.

## How to migrate a script

1. **Commit the script first**, so the upgrade is one reviewable diff.

2. **Upgrade it with the project it runs against**, and let the upgrade add the
   header. Never type `mdl 1;` by hand: the upgrade adds it only after
   rewriting every construct whose meaning the header would change, and refuses
   when one has no rewrite.

   ```bash
   mxcli fmt --upgrade -w -p app.mpr script.mdl
   ```

   The header is the default since the freeze; `--header` still says so
   explicitly, and `--header=false` upgrades the spellings alone.

   The project (`-p`) answers what the script alone cannot: whether
   `find(…)` over a flow's result is the string function or the list
   operation, which bare `commit` statements the stored flows run without
   events, and which `create or modify` statements exec would refuse under
   the header. The project is only read.

3. **Read what it reports** on stderr. A deprecated spelling with no mechanical
   rewrite is left in place and listed as "not upgraded"; a header-gated
   construct with no rewrite blocks the header, and the file keeps its version.
   Fix those by hand (`mxcli help <code>` says how) and upgrade again.

4. **Check it against the project**, the way exec will read it:

   ```bash
   mxcli check script.mdl -p app.mpr --references
   ```

   `check -p` reports everything exec would refuse under the header, computed
   by the same code exec uses, so a refusal shows up here and not halfway
   through a run.

   To gate a script that is meant to be ready for the header — in CI, say —
   pass `--deprecations=error --v1=error`: the first fails on a deprecated
   spelling (`MDL-DEPRnnn`), the second on an `MDL-V1-*` warning, which only a
   headerless script reports. Either alone lets the other through.

5. **Run it twice.** The first `exec` applies the script; the second must write
   nothing, and report every document unchanged. A second run that writes is a
   script that does not say what the project holds — the upgrade's job is to
   make it say so — and is worth reporting.

   ```bash
   mxcli exec script.mdl -p app.mpr
   mxcli exec script.mdl -p app.mpr   # expect: nothing written
   ```

**Test files** (`.test.mdl`, `.test.md`) are upgraded the same way: the
statements in their blocks are rewritten, and doc comments (`@test`,
`@expect`, …), separators and prose are kept byte for byte. The header goes
on the first line of a `.test.mdl` file and on the first line inside each
`mdl-test` block of a `.test.md` file; `--header=false` declines it, as for a
script. The runner and `check` read it, so a test body means under the header
what it means in a script (see [Test Formats](../tools/test-formats.md)).

**One dialect per file.** Upgrade a headerless file before editing it; do not
add `mdl 1` statements to an `mdl 0` file.

### What the upgrade does, in order

`mxcli fmt --upgrade` changes nothing but the constructs below; comments,
layout and keyword case survive, and the result is re-parsed before it is
written.

1. **Deprecated spellings** become their new form (the `MDL-DEPR*` table).
   This step runs with or without the header.
2. **Bare commits are pinned** (with `-p`, with or without the header). A bare
   `commit $X;` means *with* events, Studio Pro's default; mxcli releases
   before that change stored it *without*. In a `create or modify` flow whose
   stored commit has no events, the upgrade writes `commit $X without events;`
   so that re-running the script keeps what is stored. Without `-p` the script
   is left as written and fmt prints a note (`MDL067`) per flow.
3. **Header-gated constructs** are rewritten to the spelling that keeps their
   `mdl 0` meaning under the header (not with `--header=false`): the missing `;` is
   added, `/` lines are deleted, a backslash escape becomes the character it
   stood for, `retrieve … limit 1` (an object under `mdl 0`) becomes
   `retrieve … first`, a reassignment gets `set`, and a list operation becomes
   one statement per activity. One change is version-neutral and is rewritten
   without the header too (the table says which).
4. **The header is added** — unless a construct in step 3 has no rewrite, or,
   with `-p`, exec would refuse a statement under the header (a `create or
   modify` of a stored flow whose change cannot be spliced in, `MDL-V1-REBUILD`).
   The file then keeps its version and the rest of the upgrade still applies;
   `--force-header` adds the header anyway.

### The rewrites and refusals in detail

`mxcli fmt --upgrade` rewrites every deprecated spelling (the `MDL-DEPRnnn` warnings) to its canonical form — `create or replace` becomes `create or modify`, `list entities` becomes `list entities`, `on error { … }` becomes `on error begin … end error` — and changes nothing else: comments, layout and keyword case are kept. A deprecated use with no mechanical rewrite is reported and left in place.

```bash
mxcli fmt --upgrade script.mdl                  # print the upgraded script
mxcli fmt --upgrade -w script.mdl               # upgrade in place, adding `mdl 1;`
mxcli fmt --upgrade --header=false -w script.mdl  # the spellings alone, no header
```

The header is added after rewriting every construct whose meaning it would change, so the script keeps doing what it did:

| Code | Rewrite |
|---|---|
| `MDL-V1-SEMI` | adds the missing `;` |
| `MDL-V1-SLASH` | deletes the `/` line |
| `MDL-V1-ESCAPE` | writes the string's value with `''` as the only escape (`'it\'s\t'` becomes `'it''s` + a tab + `'`) |
| `MDL-V1-LIMIT1` | `retrieve … limit 1` (one object) becomes `retrieve … first` |
| `MDL-V1-SET` | `$x = …` becomes `set $x = …` |
| `MDL-V1-LIST`, `MDL-DEPR003`, `MDL-DEPR004` | a list operation or aggregate call becomes its statement form (`$x = filter($L, …)` → `$x = filter $L where …`); `find`/`contains` on a declared String keeps the call and gains `set` |
| `MDL-V1-REPLACE02` | `create or replace user role` / `demo user` becomes a plain `create` |
| `MDL-V1-WHILE` | inserts the missing `begin` after a `while` condition and `while` after its `end` |
| `MDL-V1-TEMPLATE` | a template literal spanning lines becomes the parameter it was: `log info 'a⏎b';` becomes `log info '{1}' with ({1} = 'a⏎b');` |
| `MDL-V1-QUOTEDEXPR` | a quoted expression becomes the expression bare: `dynamicclasses: 'if $currentObject/X then ''on'' else '''''` becomes `dynamicclasses: if $currentObject/X then 'on' else ''`. The bare form means the same without the header, so `fmt --upgrade` applies this one with `--header=false` too |

A construct with no mechanical rewrite is reported with the reason, and `fmt` refuses to add the header rather than change the script's meaning: an unknown or mis-shaped property (`MDL-V1-PROP`, `MDL-V1-PROPVALUE`), `create or replace view entity` (`MDL-V1-REPLACE01`), a session command in a script (`MDL-V1-SESSION`: move it to the command line or the REPL), a nested list operation such as `count(filter(…))`, `find`/`contains` on a variable whose type the script does not state (when the variable holds a microflow or nanoflow call's result, the called flow's return type decides: a flow the script creates earlier is read from the script, and any other from the project given with `-p app.mpr`, so `fmt --upgrade -p app.mpr` rewrites it), and an escaped line break (`\n`) inside an expression. An escaped line break in a text template's literal is rewritten: the break is written into the literal, which under `mdl 1` is still the template text. Running `fmt --upgrade` on its own output changes nothing.

`-p app.mpr` also settles a bare `commit $X;` in a `create or modify microflow|nanoflow`, with or without the header. Since mendixlabs/mxcli#895 a bare commit means *with* events, Studio Pro's default; an older mxcli stored the same statement *without* events. Where the stored flow commits the variable without events, `fmt --upgrade -p app.mpr` writes `commit $X without events;`, so re-running the script keeps what is stored instead of turning the event handlers on (or, inside a loop under `mdl 1`, being refused). A new flow, a new commit, or a stored commit with events is left as written; where the stored flow commits the variable both ways, the statement is left and reported. Without `-p`, `fmt` prints a note (`MDL067`) for each flow with a bare commit.

`-p app.mpr` also tells the upgrade which statements `exec` would refuse once the header is there. Under `mdl 1` a `create or modify microflow|nanoflow` of a stored flow is applied as a patch, and a change the patch cannot make (inside a loop body anything but an activity replaced by one of the same kind, an error handler, a redrawn connector, a `return` added, a statement inserted without `@position` that needs room past a node the script places) is refused with nothing written; without the header the same statement rebuilds the whole flow (`MDL-V1-REBUILD`). No rewrite keeps that meaning, so `fmt` names each such statement and leaves the header off that file, applying the rest of the upgrade. Change those flows with `alter`, or drop and create them, and upgrade again; `--force-header` adds the header regardless. `mxcli check script.mdl -p app.mpr` reports the same statements (an `MDL-V1-REBUILD` error under `mdl 1`, the warning without the header), computed by the same code as `exec` and `diff`.

## Behaviour changes to know about

Besides the generated tables, a few changes alter what an existing script
**writes**. Most apply under **both** versions, because the old behaviour was a
silent wrong write rather than a meaning a script could rely on (ADR-0011
allows fixing those everywhere); the others are gated like any `MDL-V1-*`
change. They are listed because a script that ran before may write something
different now. Where the old output can still be asked for, the entry says how.

| Change | What a script writes now | Applies under | Code |
|---|---|---|---|
| String escapes in flow expressions | A string literal in a flow expression stores its value as Studio Pro spells it: `''` doubled, never a backslash escape in the stored expression. Only how a `\` in the MDL literal is *read* depends on the header (`MDL-V1-ESCAPE`). | both | [visitor_string_escapes.go](https://github.com/mendixlabs/mxcli/blob/main/mdl/visitor/visitor_string_escapes.go), [mendixexpr.go](https://github.com/mendixlabs/mxcli/blob/main/mdl/mendixexpr/mendixexpr.go) |
| Pluggable widget schema kept | An unchanged pluggable widget keeps its stored `Type` and `Object` (the widget's schema) instead of being rebuilt from mxcli's template, so a widget upgraded in Studio Pro stays upgraded. A changed one keeps its stored schema and every property the statement does not change — a caption edit no longer resets a column's attribute or a row click describe cannot print. Layouts included. A rewrite does not upgrade a widget to the installed package (only one whose data context the statement changes is rebuilt): use `mxcli widget sync`. | both | [cmd_pages_pluggable_passthrough.go](https://github.com/mendixlabs/mxcli/blob/main/mdl/executor/cmd_pages_pluggable_passthrough.go), [cmd_pages_pluggable_patch.go](https://github.com/mendixlabs/mxcli/blob/main/mdl/executor/cmd_pages_pluggable_patch.go) |
| Text-template attribute binding | `{1} = $Order.Total` binds the attribute and renders with its formatting, as Studio Pro stores it, instead of `toString($Order/Total)`. Write `{1} = toString($Order/Total)` to keep the old output. Without the header the binding warns (`MDL-V1-TEMPLATEATTR`). | both | [cmd_pages_template_attr.go](https://github.com/mendixlabs/mxcli/blob/main/mdl/executor/cmd_pages_template_attr.go) |
| Gallery row click | A row click on a gallery with single-click selection — by `create page`, or by an `alter page … set` that leaves the gallery so — is refused (`MDL-WIDGET36`), with nothing written, because `mx check` fails such a page; without the header it is written and warns (`MDL-V1-GALLERYCLICK`). | `mdl 1` | [validate_widget_language.go](https://github.com/mendixlabs/mxcli/blob/main/mdl/executor/validate_widget_language.go) |
| `create constant … private` | Was never stored. Without the header it parses, does nothing and warns (`MDL-DEPR138`); `fmt --upgrade` deletes it; `mdl 1` refuses it. Set a private value with `mxcli constant set`. | both | [visitor_deprecations.go](https://github.com/mendixlabs/mxcli/blob/main/mdl/visitor/visitor_deprecations.go) |
| Commit events | A bare `commit $X;` commits *with* events, Studio Pro's default; older releases stored it *without*. `check` names the affected flows (`MDL067`; `exec -p` leaves out a flow the project already stores with events), and `fmt --upgrade -p` writes `without events` where the stored flow has it. | both | [commit_events.go](https://github.com/mendixlabs/mxcli/blob/main/mdl/upgrade/commit_events.go), [flow_commit_events.go](https://github.com/mendixlabs/mxcli/blob/main/mdl/executor/flow_commit_events.go) |
| `date` as a type | Mendix has no date-only type: `date` is an alias of `DateTime` and builds exactly what `DateTime` builds. Without the header it warns (`MDL-DEPR160`); `fmt --upgrade` writes `DateTime`; `mdl 1` refuses `date`. | both | [deprecation.go](https://github.com/mendixlabs/mxcli/blob/main/mdl/deprecation/deprecation.go) |

## Reference

The tables below are generated from the registries in the code
(`visitor.LanguageChanges`, `executor.LanguageChanges` and `mdl/deprecation`)
by `make gen-migration-reference`; CI fails when they are stale. A version
above the newest one this mxcli knows (`mdl 2` today) is the version that will
refuse the spelling; until then it only warns.

<!-- BEGIN GENERATED: mxcli migration reference (make gen-migration-reference) -->
<!-- Do not edit by hand: generated from visitor.LanguageChanges, executor.LanguageChanges
     and mdl/deprecation by cmd/gen-migration-reference. -->

### Changes of meaning (`MDL-V1-*`)

25 constructs mean something different under the `mdl 1;` header. Without the header each keeps the meaning in the second column and warns with its code.

| Code | Without the header (mdl 0) | Under `mdl 1;` | Decided at | Rewritten by `fmt --upgrade` |
|---|---|---|---|---|
| `MDL-V1-ACTIONSLOT` | An `onClick:` or `OnChange:` on a widget with no slot for it is dropped on write | a refusal (MDL-WIDGET37), with nothing written | exec | no: decided by exec against the project; `check -p` reports it |
| `MDL-V1-BOUNDARYDROP` | `drop <activity> boundary event` on an activity with several boundary events drops the first one `describe workflow` prints | a refusal, with nothing written | exec | no: decided by exec against the project; `check -p` reports it |
| `MDL-V1-ESCAPE` | a backslash in a string literal is an escape (`\n` is a newline, `\t` a tab, `\'` an apostrophe, `\\` one backslash) | an ordinary character, as in a Mendix expression; `''` is the only escape | parse | yes: `fmt --upgrade --header` |
| `MDL-V1-GALLERYCLICK` | A gallery row click on a single-click selection is written, and `mx check` fails the page ("The item click action is ambiguous") | a refusal (MDL-WIDGET36), with nothing written | exec | no: decided by exec against the project; `check -p` reports it |
| `MDL-V1-LIMIT1` | `retrieve … limit 1` binds a single object (Mendix's "First object" range, so a loop over it is CE0100 and count() or head() CE0097); write `retrieve … first` to say so | a list of one (a Custom range), and the object is written `first` | parse | yes: `fmt --upgrade --header` |
| `MDL-V1-LIST` | a list operation or aggregate written as a call — `find(…)` or `contains(…)`, a call after `set`, or one call nested in another — is turned into a List operation or Aggregate list activity, guessing from its arguments whether a string function was meant, | an error: a List operation or Aggregate list is one statement per activity whose operand is a variable (`$x = find $L where …;`, `$n = count $L;`), and after `set` a call is always the expression function | parse | yes: `fmt --upgrade --header` |
| `MDL-V1-PROP` | an unknown property key is accepted and ignored | an error | parse | no: blocks `--header`; an unknown property key is ignored under mdl 0 and an error under mdl 1; which key was meant cannot be guessed, so correct or delete it by hand |
| `MDL-V1-PROPVALUE` | a value its key does not take is accepted, and ignored or read by its shape | an error | parse | no: blocks `--header`; a value its key does not take is ignored or read by its shape under mdl 0; what was meant cannot be guessed, so correct it by hand |
| `MDL-V1-QUOTEDEXPR` | a quoted value holding expression text in an expression property (DynamicClasses, DynamicCellClass, an OData client's HttpUsername / HttpPassword / ClientCertificate or header value) is that expression: the outer quotes are dropped and the doubled ones undone | a Mendix string, so the old spelling is refused (MDL-WIDGET33, MDL-ODATA07); write the expression itself, without the outer quotes and with its own quotes single | parse | yes: `fmt --upgrade`, with or without `--header` |
| `MDL-V1-REBUILD` | `create or modify microflow\|nanoflow` rebuilds the whole stored flow when the change cannot be spliced in, which resets curves, drops merges and renumbers element IDs | a refusal that names the change the splice cannot make, with nothing written | exec | no: decided by exec against the project; `check -p` reports it |
| `MDL-V1-REMOTETYPE` | `create or modify external entity` writes a mapped attribute with the declared type and keeps the service's RemoteType, which mx check reports as CE6616 | a refusal that names the attribute and the type the service publishes, with nothing written | exec | no: decided by exec against the project; `check -p` reports it |
| `MDL-V1-REPLACE01` | `create or replace view entity` deletes the view entity and creates a new one (a new identity; its access rules and associations are lost) | `create or modify view entity`, which rewrites it in place and keeps its identity | parse | no: blocks `--header`; `create or replace view entity` drops and recreates the view entity under mdl 0, which no mdl 1 statement does: write `create or modify view entity` to keep its identity, or `drop entity` then `create view entity` to discard it |
| `MDL-V1-REPLACE02` | `create or replace` on a user role or demo user is a plain create, which fails when it already exists | `create or modify` | parse | yes: `fmt --upgrade --header` |
| `MDL-V1-SEMI` | a statement without a terminating `;` is accepted | an error: every statement ends with `;` | parse | yes: `fmt --upgrade --header` |
| `MDL-V1-SESSION` | a session command in a script runs as if typed at the REPL | an error: a script holds model statements only; type the command at the REPL, or use the command-line flag (`-p app.mpr` to connect, `--json` for the output format) | parse | no: blocks `--header`; a session command (`connect`, `set format`, `status`, `help`, …) is refused in an mdl 1 script, and no model statement does what it does: move it out of the script, to the command line (`-p app.mpr` to connect, `--json` for the output format) or the REPL |
| `MDL-V1-SET` | `$x = <expression>` without `set` changes the variable | an error: a reassignment is written `set $x = <expression>;` | parse | yes: `fmt --upgrade --header` |
| `MDL-V1-SHOWSUMMARY` | `show entity X` / `show association X` prints a summary of the element | an error: `show` is dropped (R6) and the summary has no mdl 1 statement; write `describe entity X` for the definition, or `list entities in M` / `list associations in M` for the summary | parse | no: blocks `--header`; `show entity X` / `show association X` print a summary no mdl 1 statement prints: `describe` prints the definition as MDL and `list entities` / `list associations` the summary columns, so either would change the script's output; choose one by hand |
| `MDL-V1-SLASH` | a `/` after a statement is accepted as a terminator (SQL*Plus style) | an error: `;` is the only statement terminator | parse | yes: `fmt --upgrade --header` |
| `MDL-V1-TABPAGEPROP` | A property a tab page cannot store (CaptionParams, Class, Style, …) is accepted and dropped on write | a refusal (MDL-WIDGET44), with nothing written | exec | no: decided by exec against the project; `check -p` reports it |
| `MDL-V1-TEMPLATE` | a message template written as one string literal that spans lines is an expression: the template is `{1}` and the literal its parameter | the template text, as a literal on one line is; a line break in a template is written into the literal | parse | yes: `fmt --upgrade --header` |
| `MDL-V1-TEMPLATEATTR` | a text-template parameter bound to a non-String attribute (`{1} = $Order.Total`) was stored as `toString($Order/Total)` by earlier mxcli releases | an attribute reference, rendered with the attribute's formatting, as Studio Pro stores it | exec | no: decided by exec against the project; `check -p` reports it |
| `MDL-V1-VIEWAUTONUMBER` | a view entity attribute declared `autonumber` over an AutoNumber column passes check, and mxbuild reports CE6770 "View Entity is out of sync with the OQL Query" | a check error that names the attribute and suggests `long`, the type the view gives the column | exec | no: decided by exec against the project; `check -p` reports it |
| `MDL-V1-WHILE` | `while <condition>` accepts a body without `begin` and an `end` without `while` | an error: a while loop is `while <condition> begin … end while;`, like `loop … begin … end loop;` | parse | yes: `fmt --upgrade --header` |
| `MDL-V1-WIDGETENUM` | A value an enumeration property of a pluggable widget declares no key for (`onClickTrigger: triple`) is written as given, and the widget ignores it | a refusal (MDL-WIDGET08) that names the keys, with nothing written | exec | no: decided by exec against the project; `check -p` reports it |
| `MDL-V1-WIDGETPROP` | A property a built-in page element does not take (`Caption:` on a textbox, `Label:` on a button, a misspelt key) is accepted and dropped on write | a refusal (MDL-WIDGET52), with nothing written | exec | no: decided by exec against the project; `check -p` reports it |

### Deprecated spellings (`MDL-DEPR*`)

90 old spellings mean exactly what their new form means. They warn with their code under every version before the one in the last column, which refuses them.

| Code | Old form | New form | Rewritten by `fmt --upgrade` | Refused from |
|---|---|---|---|---|
| `MDL-DEPR001` | `create or replace …` | `create or modify …` | yes: `replace` → `modify` | mdl 2 |
| `MDL-DEPR002` | `show …` | `list …` | yes: `show` → `list` | mdl 2 |
| `MDL-DEPR003` | `$x = <operation>($List, …)` | `$x = <operation> $List …` | yes: call form to statement form: head/tail $L; filter/find $L by Member = v (when the condition has that shape) or where <expr>; sort $L by …; union/intersect $A with $B; subtract($A, $B) -> subtract $B from $A; equals $A and $B; range($L, o, n) -> range $L offset o limit n | mdl 2 |
| `MDL-DEPR004` | `$n = <function>($List, …)` | `$n = <function> $List …` | yes: call form to statement form: count $L; sum\|average\|minimum\|maximum $L by Attr (for $L.Attr) or of <expr>; all\|any $L where <expr>; reduce $L from <initial> as <type> using <expr> | mdl 2 |
| `MDL-DEPR005` | `row row1 { … } / column Name (…) — a name on an element Mendix stores none for` | `row { … } / column (…)` | yes: name out of the element: `row row1 {` becomes `row {` | mdl 2 |
| `MDL-DEPR006` | `call microflow M.F($Param = expr)` | `call microflow M.F(Param = expr)` | yes: parameter name without its `$` (quoted when it is not an identifier or keyword) | mdl 2 |
| `MDL-DEPR007` | `show page M.P(Param: expr)` | `show page M.P(Param = expr)` | yes: colon as `=`: `Param: expr` -> `Param = expr` | mdl 2 |
| `MDL-DEPR008` | `call microflow M.F with (Param = '<expression>')` | `call microflow M.F(Param = <expression>)` | yes: string list as a list after the callee, each string's content written as the bare expression | mdl 2 |
| `MDL-DEPR009` | `objects [$a, $b] / parameters ['a', 'b']` | `with ({1} = $a, {2} = $b)` | yes: positional list as numbered placeholders: `objects [a, b]` -> `with ({1} = a, {2} = b)` | mdl 2 |
| `MDL-DEPR020` | `show_page, save_changes, close_page, microflow M.F, …` | `show page, save changes, close page, call microflow M.F, …` | yes: page action as words: the underscore becomes a space (`show_page` -> `show page`, also `save_changes`, `cancel_changes`, `close_page`, `create_object`, `open_link`, `sign_out`, `complete_task`); `delete_object` -> `delete`; `microflow M.F` / `nanoflow M.F` -> `call microflow M.F` / `call nanoflow M.F` | mdl 2 |
| `MDL-DEPR021` | `error '…' / feedback '…' / error_message '…'` | `error message '…'` | yes: message keyword as `error message`: `not null error '…'` (and after `unique` or `required`), a validation rule's `feedback '…'`, and `error_message` / `errormessage` | mdl 2 |
| `MDL-DEPR022` | `delete_behavior …` | `on delete cascade\|restrict\|set null` | yes: delete behaviour as the SQL referential action: `delete_behavior cascade` / `delete_and_references` -> `on delete cascade`; `prevent` / `delete_if_no_references` -> `on delete restrict`; `delete_but_keep_references` -> `on delete set null` | mdl 2 |
| `MDL-DEPR023` | `type reference_set` | `type ReferenceSet` | yes: type name as Mendix writes it: `reference_set` -> `ReferenceSet` | mdl 2 |
| `MDL-DEPR024` | `call rest service … returns none` | `call rest service … returns nothing` | yes: `none` → `nothing` | mdl 2 |
| `MDL-DEPR030` | `grant M.Role on M.E (rights) where '[xpath]'` | `grant rights on entity M.E to M.Role where [xpath]` | yes: rights before `on entity`, roles after `to`, and the XPath out of its string: `grant R on M.E (read *) where '[A = ''x'']'` becomes `grant read * on entity M.E to R where [A = 'x']` | mdl 2 |
| `MDL-DEPR031` | `targeting [users\|groups] xpath '[xpath]'` | `targeting [users\|groups] xpath [xpath]` | yes: XPath out of its string: `xpath '[Name = ''Admin'']'` becomes `xpath [Name = 'Admin']` | mdl 2 |
| `MDL-DEPR060` | `alter settings runtime Key = value, … / create configuration 'X' Key = value, …` | `alter settings runtime ( Key: value, … ) / create configuration 'X' ( Key: value, … )` | yes: assignments in parentheses, each `=` as `:` | mdl 2 |
| `MDL-DEPR061` | `alter consumed\|published odata service X set Key = value, …` | `alter consumed\|published odata service X set ( Key: value, … )` | yes: assignments in parentheses, each `=` as `:` | mdl 2 |
| `MDL-DEPR062` | `alter styling on page P widget w set Class = 'x', 'Full width' = on` | `alter styling on page P widget w set ( Class: 'x', 'Full width': on )` | yes: assignments in parentheses, each `=` as `:` | mdl 2 |
| `MDL-DEPR063` | `alter entity M.E set allow_create_change_locally = true` | `alter entity M.E set ( AllowCreateChangeLocally: true )` | yes: property as create's list: `set ( AllowCreateChangeLocally: <value> )` | mdl 2 |
| `MDL-DEPR064` | `type: Reference / owner: Both / storage: Table` | `type Reference / owner Both / storage Table` | yes: clause without its colon: `type: Reference` as `type Reference` (also `owner`, `storage`) | mdl 2 |
| `MDL-DEPR065` | `alter entity M.E modify attribute A Type` | `alter entity M.E modify attribute A: Type` | yes: attribute definition with its colon: `A Type` as `A: Type` | mdl 2 |
| `MDL-DEPR070` | `operation X { Method: get, … }` | `operation X ( Method: get, … )` | yes: operation's braces: `operation X { … }` becomes `operation X ( … )` | mdl 2 |
| `MDL-DEPR071` | `tool X { … } / mcp service M.S { … } / knowledge base K { … }` | `tool X ( … ) / mcp service M.S ( … ) / knowledge base K ( … )` | yes: attachment's braces: `tool X { … }` becomes `tool X ( … )` | mdl 2 |
| `MDL-DEPR072` | `image collection M.C ( image X from file '…', … )` | `image collection M.C { image X ( File: '…' ) … }` | yes: image list: the images move into { } without commas, and `from file '…'` becomes `( File: '…' )` | mdl 2 |
| `MDL-DEPR073` | `message definition collection M.C ( definition D for M.E ( A, M.E_B/M.B ( C ) ) )` | `message definition collection M.C { definition D for M.E { A, M.E_B/M.B { C } } }` | yes: message trees: each parenthesised definition list and member tree moves into { } | mdl 2 |
| `MDL-DEPR074` | `alter microflow M.F { insert after $X { … } }` | `alter microflow M.F { insert after $X begin … end; }` | yes: fragment's braces: `{` becomes `begin` and `}` becomes `end` | mdl 2 |
| `MDL-DEPR075` | `message X (…) publish\|subscribe entity M.E microflow M.H;` | `message X (…) { publish\|subscribe (Entity: M.E, Microflow: M.H); };` | yes: message's operation clauses: `publish entity M.E` becomes `{ publish (Entity: M.E); }` | mdl 2 |
| `MDL-DEPR076` | `consume X entity M.E microflow M.H;` | `consume X (Entity: M.E, Microflow: M.H);` | yes: subscription's clauses: `consume X entity M.E microflow M.H` becomes `consume X (Entity: M.E, Microflow: M.H)` | mdl 2 |
| `MDL-DEPR080` | `decision '<expression>' / timer '<expression>' / due date '<expression>'` | `decision <expression> / timer <expression> / due date <expression>` | yes: expression out of its string: `decision '$Ctx/Total > 1000'` becomes `decision $Ctx/Total > 1000` | mdl 2 |
| `MDL-DEPR081` | `Visible: [<expression>] / Editable: [<expression>]` | `Visible: <expression> / Editable: <expression>, each attribute as $currentObject/Attr` | yes: brackets into the expression they store: `Visible: [Active]` becomes `Visible: $currentObject/Active` | mdl 2 |
| `MDL-DEPR082` | `revoke M.Role on M.E [(rights)]` | `revoke rights\|all on entity M.E from M.Role` | yes: rights (or `all` when none are listed) before `on entity`, roles after `from`: `revoke R on M.E (write *)` becomes `revoke write * on entity M.E from R`, `revoke R on M.E` becomes `revoke all on entity M.E from R` | mdl 2 |
| `MDL-DEPR083` | `Username: $Const` | `Username: @Module.Const` | yes: `$Const` becomes `@<the service's module>.Const` | mdl 2 |
| `MDL-DEPR084` | `Key: Module.Const` | `Key: @Module.Const` | yes: `@` before the constant's name | mdl 2 |
| `MDL-DEPR085` | `alter settings constant 'Module.Const' …` | `alter settings constant @Module.Const …` | yes: constant's name out of its string, with `@`: `constant 'M.ApiUrl'` becomes `constant @M.ApiUrl` | mdl 2 |
| `MDL-DEPR086` | `Variables: ( $name: Type = '<expression>' ) / add variables $name: Type = '<expression>'` | `Variables: ( $name: Type = <expression> ) / add variables $name: Type = <expression>` | yes: default out of its string: `$show: boolean = 'true'` becomes `$show: boolean = true` | mdl 2 |
| `MDL-DEPR090` | `show page\|project security\|security matrix\|structure\|context of …` | `describe page\|app security\|security matrix\|structure\|context of …` | yes: verb as `describe`: `show page X` -> `describe page X`, `show project security` -> `describe app security`; the same for `list` on these forms | mdl 2 |
| `MDL-DEPR091` | `alter user role R remove module roles (…)` | `alter user role R drop module roles (…)` | yes: `remove` → `drop` | mdl 2 |
| `MDL-DEPR092` | `alter settings language remove '…' / alter settings workflows remove group '…'` | `alter settings language drop '…' / alter settings workflows drop group '…'` | yes: `remove` → `drop` | mdl 2 |
| `MDL-DEPR093` | `alter entity E add\|rename\|modify\|drop column …` | `alter entity E add\|rename\|modify\|drop attribute …` | yes: `column` → `attribute` | mdl 2 |
| `MDL-DEPR094` | `rest call get\|post\|… 'url' …` | `call rest service get\|post\|… 'url' …` | yes: statement keyword: `rest call` becomes `call rest service` | mdl 2 |
| `MDL-DEPR095` | `describe widget <name>` | `describe widget type <name>` | yes: `type` after `widget`: `describe widget combobox` -> `describe widget type combobox` | mdl 2 |
| `MDL-DEPR096` | `define fragment F as { … }` | `create fragment F as { … }` | yes: `define` → `create` | mdl 2 |
| `MDL-DEPR100` | `create constant\|association\|json structure\|image collection … comment '…'` | `/** … */ before the statement` | yes: documentation as a doc comment: the clause is deleted and its text written as `/** … */` before the statement | mdl 2 |
| `MDL-DEPR101` | `set Key = value [on target] / set (Key = value, …) [on target]` | `set (Key: value, …) [on target]` | yes: each `=` as `:`, and the assignments in parentheses when they are not | mdl 2 |
| `MDL-DEPR102` | `set Key: value [on target]` | `set (Key: value) [on target]` | yes: assignment in parentheses | mdl 2 |
| `MDL-DEPR103` | `drop widget a, b` | `drop a, b` | yes: `drop widget a` as `drop a` | mdl 2 |
| `MDL-DEPR104` | `<workflow activity> comment '…'` | `<workflow activity> caption '…'` | yes: `comment` → `caption` | mdl 2 |
| `MDL-DEPR105` | `create page\|snippet\|consumed rest service\|… M.N (…, Folder: '…', …)` | `create page\|snippet\|consumed rest service\|… M.N folder '…' (…)` | yes: folder as a clause: the property is deleted from the list and written as `folder '…'` after the name | mdl 2 |
| `MDL-DEPR106` | `Documentation: '…' in a regular expression, task queue or scheduled event` | `/** … */ before the statement` | yes: documentation as a doc comment: the property is deleted from the list and its text written as `/** … */` before the statement | mdl 2 |
| `MDL-DEPR120` | `Headers: ( 'Name' = value )` | `Headers: ( 'Name': value )` | yes: header list: `'Name' = value` becomes `'Name': value` | mdl 2 |
| `MDL-DEPR121` | `navigation P menu ( menu item …; menu 'X' ( … ); ) / create menu M.M ( … )` | `navigation P { menu item … menu 'X' { … } } / create menu M.M { … }` | yes: menu items: `menu (` becomes `{`, a menu's or sub-menu's ( ) become { }, and the `;` after each item is dropped | mdl 2 |
| `MDL-DEPR122` | `menu item 'X' page M.P icon I / microflow M.F / sign out` | `menu item 'X' ( OnClick: show page M.P, Icon: I ) / call microflow M.F / sign out` | yes: item clauses: `page M.P` becomes `( OnClick: show page M.P )`, `microflow M.F` `OnClick: call microflow M.F`, `sign out` `OnClick: sign out`, and `icon I` `Icon: I` | mdl 2 |
| `MDL-DEPR123` | `Params: { $P: M.E } / Variables: { $v: Boolean = 'true' }` | `Params: ( $P: M.E ) / Variables: ( $v: Boolean = 'true' )` | yes: header map: the braces become ( ) | mdl 2 |
| `MDL-DEPR124` | `ContentParams: [{1} = expr]` | `ContentParams: ({1} = expr)` | yes: template parameters: the brackets become ( ) | mdl 2 |
| `MDL-DEPR125` | `DesignProperties: ['Key': 'Value', 'Group': ['k': on]]` | `DesignProperties: ('Key': 'Value', 'Group': ('k': on))` | yes: design properties: each bracketed list becomes ( ) | mdl 2 |
| `MDL-DEPR126` | `snippetcall s (Snippet: M.S, Params: {$Asset: $var})` | `snippetcall s (Snippet: M.S, Params: (Asset = $var))` | yes: snippet call arguments: `{$P: $v}` becomes `(P = $v)` | mdl 2 |
| `MDL-DEPR127` | `database connection M.Db type '…' connection string @M.C … begin query Q sql '…' returns M.E map (c as A); end` | `database connection M.Db ( Type: '…', ConnectionString: @M.C, … ) { query Q ( Sql: '…', Returns: M.E, Map: ( A = c ) ) }` | yes: clauses become the property list: `type` `Type:`, `connection string` `ConnectionString:`, `host` `Host:`, `port` `Port:`, `database` `DatabaseName:`, `username` `Username:`, `password` `Password:`; `begin … end` becomes { }; a query's `sql`, `parameter`, `returns` and `map` become Sql, Parameters, Returns and Map, and `column as Attr` becomes `Attr = column` | mdl 2 |
| `MDL-DEPR130` | `list image\|icon\|message definition collection [in M]` | `list image\|icon\|message definition collections [in M]` | yes: singular as the plural: `collection` -> `collections` after `list` | mdl 2 |
| `MDL-DEPR131` | `create\|alter\|drop\|describe\|move model M.X / list models` | `create\|alter\|drop\|describe\|move ai model M.X / list ai models` | yes: document name: `model` becomes `ai model`, `models` becomes `ai models` | mdl 2 |
| `MDL-DEPR132` | `create json structure M.J snippet '…'` | `create json structure M.J sample '…'` | yes: `snippet` → `sample` | mdl 2 |
| `MDL-DEPR133` | `alter app security level …\|demo users on\|off\|guest access on [role R]\|off\|strict mode on\|off` | `alter app security ( SecurityLevel: …, EnableDemoUsers: …, EnableGuestAccess: …, GuestUserRole: R, StrictMode: … )` | yes: clause as a property list: `level production` -> `( SecurityLevel: production )` | mdl 2 |
| `MDL-DEPR134` | `create constant M.C ( … ) folder '…' / create snippet M.S (…) folder '…' { … }` | `create constant M.C folder '…' ( … ) / create snippet M.S folder '…' (…) { … }` | yes: clause moved: `folder '…'` goes right after the name | mdl 2 |
| `MDL-DEPR135` | `alter entity\|association\|enumeration … set comment '…'` | `alter entity\|association\|enumeration … set documentation '…'` | yes: `comment` → `documentation` | mdl 2 |
| `MDL-DEPR136` | `create constant M.C type T default v [exposed to client]` | `create constant M.C ( Type: T, DefaultValue: v, ExposedToClient: true )` | yes: constant properties: `type T default v exposed to client` becomes `( Type: T, DefaultValue: v, ExposedToClient: true )` | mdl 2 |
| `MDL-DEPR137` | `create demo user 'u' password 'p' [entity M.E] (Role, …)` | `create demo user 'u' ( Password: 'p', Entity: M.E, UserRoles: (Role, …) )` | yes: demo user properties: `password 'p' entity M.E (R1, R2)` becomes `( Password: 'p', Entity: M.E, UserRoles: (R1, R2) )` | mdl 2 |
| `MDL-DEPR138` | `create constant M.C … private` | `create constant M.C …` | yes: constant's `private` modifier away: it is deleted | mdl 1 |
| `MDL-DEPR139` | `create published odata service M.S ( … ) authentication basic, session, microflow M.F` | `create published odata service M.S ( …, Authentication: (basic, session, microflow M.F) )` | yes: published OData authentication: the trailing `authentication m1, m2` clause becomes the last property, `Authentication: (m1, m2)` | mdl 2 |
| `MDL-DEPR140` | `alter workflow M.W set display 'x' / set description … / set export level … / set due date … / set overview page … / set parameter $P: M.E` | `alter workflow M.W { set ( Display: 'x', Description: …, ExportLevel: …, DueDate: …, OverviewPage: …, Parameter: $P: M.E ); }` | yes: property as `set ( Key: value )`, inside the statement's { } | mdl 2 |
| `MDL-DEPR141` | `alter workflow M.W set activity X page M.P / description … / targeting … / due date …` | `alter workflow M.W { set ( Page: M.P, Description: …, Targeting: …, DueDate: … ) on X; }` | yes: `set activity X <prop> v` as `set ( Key: v ) on X`, inside the statement's { } | mdl 2 |
| `MDL-DEPR142` | `alter workflow M.W insert after X <activity>;` | `alter workflow M.W { insert after X { <activity>; } }` | yes: inserted activity in { }, inside the statement's { } | mdl 2 |
| `MDL-DEPR143` | `alter workflow M.W drop activity X` | `alter workflow M.W { drop X; }` | yes: `drop activity X` as `drop X`, inside the statement's { } | mdl 2 |
| `MDL-DEPR144` | `alter workflow M.W replace activity X with <activity>;` | `alter workflow M.W { replace X with { <activity>; } }` | yes: `replace activity X with a;` as `replace X with { a; }`, inside the statement's { } | mdl 2 |
| `MDL-DEPR145` | `alter workflow M.W insert outcome 'N' on X { … }` | `alter workflow M.W { insert into X { outcomes 'N' { … } } }` | yes: `insert outcome 'N' on X { … }` as `insert into X { outcomes 'N' { … } }` | mdl 2 |
| `MDL-DEPR146` | `alter workflow M.W insert path on X { … }` | `alter workflow M.W { insert into X { path { … } } }` | yes: `insert path on X { … }` as `insert into X { path { … } }` | mdl 2 |
| `MDL-DEPR147` | `alter workflow M.W insert condition 'V' on X { … }` | `alter workflow M.W { insert into X { outcomes 'V' -> { … } } }` | yes: `insert condition 'V' on X { … }` as `insert into X { outcomes 'V' -> { … } }` | mdl 2 |
| `MDL-DEPR148` | `alter workflow M.W insert boundary event on X <event>` | `alter workflow M.W { insert into X { boundary event <event> } }` | yes: `insert boundary event on X e` as `insert into X { boundary event e }` | mdl 2 |
| `MDL-DEPR149` | `alter workflow M.W drop outcome 'N' on X / drop condition 'V' on X / drop path 'Path n' on X / drop boundary event on X` | `alter workflow M.W { drop X outcome 'N'; drop X outcome true; drop X path n; drop X boundary event; }` | yes: member after the activity it belongs to: `drop X outcome 'N'`, `drop X path n`, `drop X boundary event` | mdl 2 |
| `MDL-DEPR160` | `date` | `DateTime` | yes: type `date` as the type it was stored as: `DateTime` | mdl 1 |
| `MDL-DEPR161` | `create regular expression M.R ( …, ExportLevel: Public )` | `create regular expression M.R ( …, ExportLevel: API )` | yes: regular expression's export level value `Public` as `API` | mdl 2 |
| `MDL-DEPR540` | `on error [without rollback] { … }` | `on error [without rollback] begin … end error` | yes: `{` becomes `begin` and the closing `}` becomes `end error` | mdl 2 |
| `MDL-DEPR550` | `rest client / rest clients` | `consumed rest service / consumed rest services` | yes: document type name: `rest client` becomes `consumed rest service`, `rest clients` `consumed rest services` | mdl 2 |
| `MDL-DEPR551` | `odata client / odata clients` | `consumed odata service / consumed odata services` | yes: document type name: `odata client` becomes `consumed odata service`, `odata clients` `consumed odata services` | mdl 2 |
| `MDL-DEPR552` | `odata service / odata services` | `published odata service / published odata services` | yes: document type name: `odata service` becomes `published odata service` | mdl 2 |
| `MDL-DEPR553` | `queue / queues` | `task queue / task queues` | yes: document type name: `queue` becomes `task queue`, `queues` `task queues` | mdl 2 |
| `MDL-DEPR554` | `alter project security …` | `alter app security …` | yes: security name: `project security` becomes `app security` | mdl 2 |
| `MDL-DEPR555` | `alter settings model …` | `alter settings runtime …` | yes: section name: `model` becomes `runtime` | mdl 2 |
| `MDL-DEPR710` | `create user role R (M.A, …) [manage all roles]` | `create user role R ( ModuleRoles: (M.A, …), ManageAllRoles: true, … )` | yes: user role properties: the role list becomes `ModuleRoles: (…)` in a ( Key: value ) list, and `manage all roles` becomes `ManageAllRoles: true` | mdl 2 |
| `MDL-DEPR711` | `Headers: ('Authorization': 'Bearer ' + $Token) / ('X-Key': $Key)` | `Headers: ('Authorization': 'Bearer {Token}') / ('X-Key': '{Key}')` | yes: header value as a template: `'text' + $P` becomes `'text{P}'` | mdl 2 |
| `MDL-DEPR720` | `call rest service <method> <url> [header N = v …] [auth basic $u password $p] [body …] [timeout n] returns …` | `call rest service <method> <url> (Headers: ('N': v), Authentication: basic (Username: $u, Password: $p), Body: …, Timeout: n) returns …` | yes: activity settings as one property list (ADR-0013): `header N = v` becomes `Headers: ('N': v)`, `auth basic $u password $p` becomes `Authentication: basic (Username: $u, Password: $p)`, `body '…'` becomes `Body: template '…'`, `body binary\|mapping …` becomes `Body: binary\|mapping …`, `timeout n` becomes `Timeout: n` | mdl 2 |

<!-- END GENERATED: mxcli migration reference -->
