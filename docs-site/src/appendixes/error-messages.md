# Error Messages Reference

Common error messages from Studio Pro, mxcli, and the MDL parser, with explanations and solutions.

## Studio Pro Errors

These errors appear when opening a project in Studio Pro after modification by mxcli.

### TypeCacheUnknownTypeException

```
TypeCacheUnknownTypeException: The type cache does not contain a type
with qualified name DomainModels$Index
```

**Cause:** The BSON `$Type` field uses the **qualifiedName** instead of the **storageName**. These are often identical, but not always.

**Solution:** Use the correct storage name:

| qualifiedName (wrong) | storageName (correct) |
|-----------------------|----------------------|
| `DomainModels$Entity` | `DomainModels$EntityImpl` |
| `DomainModels$Index` | `DomainModels$EntityIndex` |

Look up the type's `SetTypeName(...)` call in `modelsdk/gen/<domain>/types.go`, or dump a Studio Pro-authored document.

### CE0463: Widget definition changed

```
CE0463: The widget definition of 'DataGrid2' has changed.
```

**Cause:** The widget's `WidgetObject` properties do not match its `PropertyTypes` schema. This happens when:

- A widget template is missing properties
- Property values have incorrect types
- The template was extracted from a different Mendix version

**Solution:**
1. Create the same widget manually in Studio Pro
2. Extract its BSON from the saved project
3. Compare the template's `object` section against the Studio Pro version
4. Update `sdk/widgets/templates/<version>/<Widget>.json` to match

See the debug workflow in `.claude/skills/debug-bson.md` for step-by-step instructions.

### CE0066: Entity access is out of date

```
CE0066: Entity access for 'MyModule.Customer' is out of date.
```

**Cause (1): the rules no longer cover the entity's members.** An access rule
names the members it governs, so an entity that has gained an attribute or an
association — or lost one — leaves every rule on it out of date. This is the
usual cause for a model that arrived from somewhere else: a module imported or
updated outside Studio Pro whose author never pressed **Update security**, a
hand-edited `.mpr`, a merge.

**Solution:** run the headless equivalent of that button:

```bash
mxcli -p app.mpr -c "update security MyModule"
```

It reports what it changed (`Reconciled 3 access rule(s) in module MyModule`) and
writes nothing when the rules already match. See
[UPDATE SECURITY](../reference/security/update-security.md). `mxcli marketplace
install` and `mxcli marketplace update` run it for the module they copy in.

**Cause (2): association MemberAccess entries on the wrong entity.** In Mendix,
association access rules must only be on the **FROM** entity (the one stored in
`ParentPointer`), not the TO entity.

**Solution:** Ensure `MemberAccess` entries for associations are added only to the
entity that owns the foreign key (the FROM side of the association). Remove any
association MemberAccess entries from the TO entity.

### System.ArgumentNullException (ValidationRule)

```
System.ArgumentNullException: Value cannot be null.
```

**Cause:** A validation rule's `Attribute` field uses a binary UUID instead of a qualified name string. The metamodel specifies `BY_NAME_REFERENCE` for this field.

**Solution:** Use a qualified name string (e.g., `"Module.Entity.Attribute"`) for the `Attribute` field in ValidationRule BSON, not a binary UUID.

## mxcli Check Errors

These rule IDs appear in `mxcli check` output and in LSP diagnostics in VS Code. They fire before MxBuild runs, so most pluggable-widget mistakes are caught at authoring time.

### MDL-WIDGET01: Unknown pluggable widget property

```
page MyModule.OrderList: widget `cb1` (combobox) has no property
`optionsSourcType` — did you mean `optionsSourceType`? [MDL-WIDGET01]
```

**Cause:** The property key written on a pluggable widget is not declared in the widget's `.def.json` (the extracted schema from its `.mpk`). Usually a typo; sometimes a property that exists in a different widget but not this one.

**Solution:**
1. Compare the key against the widget's known properties — `mxcli widget describe <name>` lists them (or `describe widget type <name>;` in MDL).
2. Use the suggested replacement if one is offered (Levenshtein-nearest match).
3. If the property genuinely doesn't exist on this widget version, check that `.mxcli/widgets/` has the latest schema: `mxcli refresh catalog -p app.mpr` re-extracts any `.mpk` whose mtime changed.
4. If the property was just added by a `.mpk` upgrade, make sure `mxcli init` or `widget init` was run after the upgrade.

This rule also fires in the LSP — the property key shows as a red squiggle while you type.

### MDL-WIDGET02: Legacy native widget on upgraded project

```
page MyModule.OrderList: widget `OrdersGrid` uses deprecated native
`Forms$DataGrid` (deprecated from Mendix 11.0.0) — migrate to pluggable
Datagrid 2.x (`DATAGRID` keyword resolves to this on 11.0+) (project is
on 11.9.0) [MDL-WIDGET02]
```

**Cause:** Studio Pro does not auto-migrate native-stack widgets (e.g. `Forms$DataGrid`) to their pluggable replacements during a Mendix major-version upgrade. After upgrading from Mendix 10.x to 11.x, your pages can still contain the native widget unless you opened them and migrated by hand.

**Solution:**
1. Run `mxcli check --post-migration -p app.mpr` to get the full list of pages and widgets to migrate.
2. Open each reported page in Studio Pro and replace the native widget with its pluggable equivalent. For DataGrid, this means using the `DATAGRID` MDL keyword (which resolves to pluggable Datagrid 2.x on Mendix 11.0+) or `pluggablewidget 'com.mendix.widget.web.datagrid.Datagrid'`.
3. Re-run the scan to confirm the page is clean.

The deprecated-widget catalog is hand-maintained in `mdl/executor/keyword_dispatch.go` (`LegacyWidgets`). Open an issue if you spot a native widget that should be on the list.

### MDL-LISTOP01: Undefined iterator in a FILTER/FIND predicate

```
filter($L, …): '$item' is not defined in this microflow. A filter predicate is
evaluated once per item and Mendix binds the item to '$currentObject' — no other
iterator name exists, so mxbuild rejects this with CE0109 "Undefined variable
'item'". [MDL-LISTOP01]
```

**Cause:** A `FILTER`/`FIND` predicate navigates from a variable the microflow never binds. Mendix evaluates the predicate once per item with the item bound to `$currentObject`, and defines no other iterator name — so `filter($L, $item/Amount > 0)` compiles to a reference to nothing.

**Solution:**
1. Use `$currentObject/` for the item under test: `filter($L, $currentObject/Amount > 0)`.
2. Or write the attribute bare — `filter($L, Amount > 0)` — and mxcli resolves it against the list's entity and stores `$currentObject/Amount`.

The rule keys on **scope, not on the name**. `$item` is perfectly valid in a predicate when it is the enclosing loop's iterator, which is how the O(N) lookup idiom is written — inside `loop $item in $L`, `find($Others, Key = $item/Key)` navigates the loop's variable and is not flagged.

### MDL-STUB01: One flow declared twice in a script set

```
MyFirstModule.Export is declared by 2 `create or modify` statements in this script
set: 13-actions.mdl:40 (mdl 0), 30-export.mdl:12 (mdl 1). Run in order, the first
replaces what the last one stored, on every run; ... A self-recursive flow no
longer needs a placeholder (#843): drop the stub and keep the real statement [MDL-STUB01]
```

**Cause:** A script set — `mxcli check` or `mxcli fmt --upgrade` given several files, read as one run in the order given — declares one microflow or nanoflow with two `create or modify` statements: a placeholder ("stub") that earlier scripts can reference, followed by the real flow. Run in order, the stub replaces the stored real flow and the real statement restores it, on every run. Under different language headers it is worse: the mdl 0 stub rebuilds the real flow, the mdl 1 real statement can be refused (its change cannot be spliced into the stub), and the project keeps running the placeholder with `mx check` reporting nothing (ako/mxcli#905).

**Solution:** Drop the stub. Since #843 a self-recursive flow is created in one statement, and a script that only references the flow does not need it to exist until it runs. If the stub has to stay, give both files the same header: `mxcli fmt --upgrade -w -p app.mpr` over **all** the files decides the header for the pair together — added to both or to neither — where upgrading the files one at a time could split it.

It is a warning. One file checked alone is not read as a set.

### MDL-SEC21: A used flow left without access (CE0106)

```
✗ microflow Shop.MF_Button has no allowed role but is used from page Shop.P_Main —
MxBuild reports CE0106 "At least one allowed role must be selected if the microflow
is used from navigation, a page, a nanoflow or a published service" at security
level Production. This script creates it as a NEW microflow, ... [MDL-SEC21]
  → add grant execute on microflow Shop.MF_Button to Shop.Admin; — or, to rebuild a
    flow that already exists without losing its access, use `create or modify
    microflow` instead of drop + create
```

**Cause:** With a project (`check -p`), the script leaves a microflow or nanoflow with no allowed module role while a page, snippet, layout, nanoflow, menu document or navigation profile names it. Measured on Mendix 11.14, MxBuild reports CE0106 for each of those uses (even from an unused snippet, menu document or nanoflow), but not for a published REST operation, a microflow called only from another microflow, or an excluded page. The usual way to get there is `drop microflow` in one run and `create microflow` in a later one: the later create is a *new* flow, and a new document gets no access. A new flow in a script that creates it and a page using it, with no grant, is the other. Dropping and recreating in the same run, or using `create or modify`, keeps the stored roles.

**Solution:** Add the `grant execute on microflow …` (or `… on nanoflow …`) the suggestion names, or rebuild the flow with `create or modify` instead of dropping it.

It is an **error at security level Prototype or Production** (the stored level, or the one the script sets) and a warning at Off, where MxBuild does not check it. Only what the script changes is reported: a flow that already had the problem before the script, and that the script does not touch, is left to `mxcli docker check`.

### MDL-SEC22: A call across entity access (CE0114)

```
✗ microflow MyModule.DS_Tile_Count does not apply entity access (@applyentityaccess(false)
in this script) but calls microflow MyModule.SUB_Scoped_List, which does
(@applyentityaccess(true) as stored) — MxBuild reports CE0114 "A microflow that does not
apply entity access can only call microflows that also do not apply entity access." [MDL-SEC22]
```

**Cause:** With a project (`check -p`), a microflow without "Apply entity access" calls a microflow — or decides on a rule (`if Mod.Rule(…)`) — that has it. Measured on Mendix 11.13 and 11.14, MxBuild refuses only that direction, at every security level; on -> off, on -> on, off -> off, a nanoflow caller and an excluded caller all build. Both settings are read from the project and simulated through the script (`@applyentityaccess`, `@applyentityaccess(false)`, and an absent annotation on `create or modify` keeping what is stored).

**Solution:** Put `@applyentityaccess` on the caller, or call a flow that does not apply entity access. Turning it off on the callee also builds, but widens what that flow may read and write for every caller.

Only what the script changes is reported, and a callee neither the project nor the script holds is not judged.

### MDL-I18N01: A tab page caption without the default language (CE4899)

```
✗ Administration.Account_Overview: tab page caption tabPage2 ("Local Users") has no
de_DE text once this script makes de_DE the default language — mxbuild reports
CE4899 "Empty caption" [MDL-I18N01]
  → change the default language before creating the document, or give it a de_DE
    text after the change: `alter page Administration.Account_Overview { set (Caption: '…') on tabPage2; };`
```

**Cause:** With a project (`check -p`), the script changes `DefaultLanguageCode` while a tab page caption has no text in the new default: one stored in the project, or one a `create page` / `create snippet` earlier in the script wrote in the old default. Mendix has no language-neutral text — a caption is stored per language, and a new one under the default at the time. Measured on Mendix 11.14, a tab page caption is the one caption kind MxBuild refuses when the default has no text (CE4899 "Empty caption. [German, Germany]"); a page title, button, label or menu item caption builds and shows the placeholder in Studio Pro instead. A stock app's `Administration.Account_Overview` has an en_US-only `tabPage2`, so switching such an app to another default fails the build.

**Solution:** Change the default language before the script creates its pages, or give each caption a text in the new default after the change — `alter page … { set (Caption: '…') on <tab>; };` (or `alter snippet`) writes the default language. `alter settings language (DefaultLanguageCode: …)` prints the list when it runs, and `mxcli lint` reports the project's existing ones as **QUAL006**. A page the script creates or rewrites *after* the change is written in the new default and is not reported.

### MDL-DUPDEF: Element defined twice in one script

```
microflow already defined in this script: Sales.ACT_Submit (first defined at
statement 4) [MDL-DUPDEF]
```

**Cause:** Two plain `create` statements in one script make the same element — same kind, same qualified name — and nothing between them drops or renames the first. `exec` would create the first and refuse the second as "already exists", after everything before it had been written.

**Solution:** Keep one of the two. To change an element the script already made, use `create or modify` (it is never reported), or `drop` it before re-creating it. A `rename` frees the old name and takes the new one, so `create` after `rename … to New` is reported for `New` and not for the old name.

`check --references` (`-p app.mpr`) adds the project side: a plain `create` of an element the project already has is reported as "already exists in project", for every create `exec` refuses that way — documents, module and user roles, demo users, configurations.

### MDL-DUPNAME: Name already taken in the module

```
cannot create nanoflow Sales.ACT_Submit: microflow Sales.ACT_Submit already has
that name — microflows, nanoflows and rules share one name space per module,
whatever their folder, and names compare case-insensitively (Mendix CE0122)
(statement 3) [MDL-DUPNAME]
```

**Cause:** The statement gives an element a name Mendix will not let it share. Measured with `mx check`, three groups of kinds each share one name space per module:

| Kinds | Mendix error |
|-------|--------------|
| microflows, nanoflows, rules | CE0122 "Duplicate document name" |
| pages, snippets, layouts | CE0122 "Duplicate document name" |
| entities (incl. view and external), associations, enumerations | CE0065 "Entities, associations and enumerations cannot share names" |

Every other kind is a name space of its own. A microflow may share its name with a page, constant, enumeration, java action or workflow. Folders do not separate names, and names compare **case-insensitively**: `Sales.act_submit` next to microflow `Sales.ACT_Submit` is a duplicate of the same kind (ako/mxcli#806).

`check` reports the clash between two statements of one script. `check --references` and `exec`'s pre-flight also report a statement that clashes with what the project already holds — a `create`, a `rename … to` or a cross-module `move … to Module`. `exec` refuses it before writing anything. The model it would write does not build, so this holds under every language version, `create or modify` included. `create or modify` of the element that already has the name, spelled as it is stored, still modifies it.

**Solution:** Pick another name, or rename the other element first. To change an existing element, spell its name as it is stored. A different folder does not help.

### MDL-DEPRnnn: Deprecated spelling

```
line 1: `create or replace …` (enumeration) is deprecated; write `create or modify …`
— same meaning. Refused from `mdl 2`. (mxcli help MDL-DEPR001) [MDL-DEPR001]
```

**Cause:** The script uses an alias that MDL is consolidating away (ADR-0010). The alias means exactly what the canonical form means, so the statement runs unchanged. The warning names the canonical form, and the language version whose header (`mdl <n>;`) will refuse the alias (ADR-0011).

**Solution:** Write the canonical form the warning names. The rewrite is a mechanical keyword swap, and the suggestion line says which one.

`check` and `exec` report these as **warnings**. To fail the run on one, for example in CI over documentation and examples, pass `--deprecations=error`.

`--deprecations=error` does not gate the `MDL-V1-*` warnings — what a headerless (`mdl 0`) script still has to change before it can carry `mdl 1;` — so a script can pass it and still not parse under `mdl 1`. `mxcli check --v1=error` fails the run on those; `--deprecations=error --v1=error` together is the full `mdl 1` readiness gate, and `mxcli fmt --upgrade --header` is the fix.

`mxcli help <code>` prints the entry for any `MDL-DEPRnnn` or `MDL-V1-*` code, `mxcli syntax <topic> --deprecated` lists a topic's old spellings, and [Language Versions and Migration](../language/versions.md) tabulates them all.

The registry of deprecated spellings is `mdl/deprecation/deprecation.go`. It holds the code, old form, canonical form, rewrite and removal version of each entry. A spelling is only registered where it means exactly the same as its canonical form. `create or replace view entity`, for example, drops and recreates the view entity, so it is not reported. Likewise `show` is reported as `MDL-DEPR002` only where its canonical form is `list` (plurals, relationship queries and the summary tables), and as `MDL-DEPR090` where it is `describe` with the same output. `show entity X` and `show version` are neither: the first has no mdl 1 statement (`MDL-V1-SHOWSUMMARY`), the second is a session command (`MDL-V1-SESSION`).

## mxcli Parser Errors


### MDL-LISTOP02: A list operation nested inside another one

```
count(…): the list argument is `filter($reqs, $currentObject/Status = Mod.E.Approved)`,
which is not a variable. A Mendix aggregate list activity stores its list as a
variable reference and has no slot for a nested computation, so the argument is
dropped and the activity is written with an empty list — mxbuild then rejects it
with CE0012 "The 'List' property is required.". [MDL-LISTOP02]
```

**Cause:** Every list operation and aggregate — `HEAD`, `TAIL`, `FIND`, `FILTER`, `SORT`, `UNION`, `INTERSECT`, `SUBTRACT`, `RANGE`, `COUNT`, `SUM`, `AVERAGE`, `MINIMUM`, `MAXIMUM`, `REDUCE`, `ALL`, `ANY` — is a separate **activity** in Mendix, and an activity stores its list as a **variable reference**. MDL's expression grammar makes them look composable, but there is nowhere in the model to put a nested call.

Before this rule existed the inner call was dropped, list and predicate together, and the activity was written with an empty list. That passes `mxcli check`, execs with `Created microflow`, and fails only at build time — `CE0012 "The 'List' property is required."` for an aggregate, `CE0096` for a list operation. `sort(filter(…), Attr)` was worse: with the list gone the sort attribute has no entity to resolve against, and mxbuild aborts with an `InvalidOperationException` instead of reporting an error.

The rule keys on the operand not reducing to a variable, so it also covers a non-list argument: `count('nonsense')` failed the same way.

**Solution:** Give the inner operation its own statement and pass the variable.

```text
-- WRONG
$n = count(filter($Requests, $currentObject/Status = Module.ENUM_Status.Approved));
```

```mdl
-- RIGHT
$Approved = filter $Requests where $currentObject/Status = Module.ENUM_Status.Approved;
$n        = count $Approved;
```

The same applies to both operands of `union`/`intersect`/`subtract`.

### MDL-SET01: `set` on an object variable

```
cannot set object variable '$Cursor' (G46.Group): Mendix has no action that reassigns an
object variable — Change variable takes only a primitive, and mxbuild rejects it with
CE7247 "Variable 'Cursor' does not have a primitive type". [MDL-SET01]
```

**Cause:** `set $Var = …` is a Change variable activity, which takes only a primitive
variable; on a list it is a Change list Replace. Mendix has no activity that reassigns an
object variable, so the statement used to be written as a Change variable and fail the build
with CE7247 (mendixlabs/mxcli#1323). It comes up naturally when walking a parent chain with
`set $Cursor = $Next` in a `while` loop. On a parameter mxbuild words the same code
`"Parameter 'X' cannot be changed."`.

Plain `check` reports it for the objects it can see without a project — entity parameters,
`create`, `retrieve … first`, loop iterators, casts, `head`. Whether an association retrieve
is an object or a list depends on the association, so that case is reported by
`check --references` and `exec`.

**Solution:** Return the new object from a sub-microflow (recursion for a chain walk),
retrieve it into a new variable, or change the object's members with `change $Obj (…)`.

The same rule refuses `set` on a **parameter** of any type but a list, in a microflow, a
nanoflow or a rule:

```
cannot set parameter '$N' (Integer): a Change variable cannot target a parameter, and
mxbuild rejects it with CE7247 "Parameter 'N' cannot be changed". [MDL-SET01]
```

Copy the parameter into a variable and change that (`declare $Value Integer = $N;`). A list
parameter is not refused — `set $L = $M` on one is a Change list Replace, which builds — and
neither is a member change, `set $Param/Attr = …`.

### MDL-RETRIEVE02: Retrieve by association over `System.owner` / `System.changedBy`

```
retrieve $u from $task/System.owner: System.owner is not a modelled association, and a
retrieve by association over it has no entity — mxbuild rejects it with CE0136 "Retrieve
object must specify the 'Entity' property." [MDL-RETRIEVE02]
```

**Cause:** `owner` and `changedBy` are not associations in any domain model — an entity
carries them as the *Store owner* / *Store changedBy* flags (`AutoOwner`, `AutoChangedBy`).
XPath constraints and page paths resolve them, but a retrieve by association does not:
mxbuild finds the name and derives no entity from it, whether or not the entity stores the
member (measured on 11.12.2, mendixlabs/mxcli#1358). `check` and `exec` both refuse it.

**Solution:** Retrieve the user from the database by id:

```mdl
retrieve $u from System.User where [id = $task/System.owner] first;
retrieve $u from System.User where [id = $task/System.changedBy] first;
```

That constraint reads the member off `$task`, so it builds only when `$task`'s entity stores
it — `owner: AutoOwner` / `changedBy: AutoChangedBy` on the root of its generalization chain
(a `System.FileDocument` specialization stores both). On an entity that does not, mxbuild
rejects it with CE0161 "Error(s) in XPath constraint." (measured on 11.12.5, ako/mxcli#1306),
and `check -p` and `exec` refuse it, naming the `alter entity` that stores the member.

### MDL-CHANGE01: Setting `System.owner` / `System.changedBy` in a create or change

```
member "System.owner" on M.Entity: System.owner is kept by the runtime, not set by a
microflow — a create or change object cannot set it, and mxbuild rejects the member with
CE1050 "Unknown association 'System.owner'." (whether or not the entity stores it) [MDL-CHANGE01]
```

**Cause:** The owner and last editor of an object are system members, not associations a
project models. Written as a create or change object member, `System.owner` and
`System.changedBy` are CE1050 for every entity — one that stores its owner, one that does not,
a `System.FileDocument` specialization — in a microflow and a nanoflow alike (measured on
11.12.5, mendixlabs/mxcli#1612). `check` needs no project to know it; `check` and `exec` both
refuse it. A list operation is different: `find $l by System.owner = $u` (and `filter`, and
`System.changedBy`) builds when the list's entity stores the member — on one that does not it
is CE0091 "No member selected.", which `check -p` and `exec` report with the `alter entity`
that stores it (ako/mxcli#1629).

**Solution:** To record a user on an object — for example to own seeded data — model the
association and set that:

```mdl
create association M.Entity_User from M.Entity to System.User;

$S = create M.Entity (Name = 'x', M.Entity_User = $User);
```

Constrain on `M.Entity_User` where you would have used `System.owner`.

### MDL-RETRIEVE03: `change` over a list retrieve

```
$E was retrieved as a list (under `mdl 1;` `limit 1` is a list of one), not a single
object, so `change $E` cannot take it — mxbuild rejects the stored member with CE1613
"The selected attribute 'List of …' no longer exists". [MDL-RETRIEVE03]
```

**Cause:** The reverse of MDL-RETRIEVE01. A database retrieve with no range, with
`limit n [offset n]`, or — under the `mdl 1;` header — with a bare `limit 1` binds a
**list**; a Change object activity takes one object. Before mendixlabs/mxcli#1422, `exec`
wrote such a change with the member qualified through the list type, which `mx check`
reported as CE1613 (measured on 11.6.6). A member path `$E/Attr` in any later expression is
the same mistake and is CE0117 "Error(s) in expression.". `check` and `exec` both refuse
both. `set $E = empty` over the list is a Change list Replace and builds, so it is not
reported.

**Solution:** Retrieve the object with `first`, or change the items inside a `loop`:

```mdl
retrieve $E from Mod.Ent first;
change $E (Attr = 'x');
```

**Over an association** (`check --references`, mendixlabs/mxcli#1516). Whether
`retrieve $L from $h/Mod.Assoc` binds a list depends on the association: from the FROM
entity of a Reference it is one object; from its TO entity (the "one" side) it is a
**list**, unless the owner is Both; a ReferenceSet is a list from either end. The project
knows which, so with `-p` / `--references` a member path over that list (CE0117), a
`change` of it (CE7247 "Variable 'L' is of type List of …, but should be of type
Object."), and passing it to a parameter that takes one object (CE0117) are reported too
— measured identically on 11.6.6 and 11.15.0. The bare list retrieve, `count`, a `loop`
and a list parameter build. For one object, retrieve it from the database:

```mdl
retrieve $L from Mod.Line where [Mod.Line_Header = $h] first;
```

The reverse is reported as **MDL-RETRIEVE01** (ako/mxcli#1793): the one object a
Reference gives from its FROM entity, taken as a list. An aggregate of it (`count`, `sum`,
…) is CE0013 "Input variable 'P' must be of type 'List'.", a list operation (`head`,
`filter`, …) CE0097, a `loop` CE0100 and a list parameter CE0117 — measured on 11.13.0.
A **self-reference** (`Mod.Order_Parent` from `Mod.Order` to `Mod.Order`) is always
followed from its FROM end, from an `Order` and from a specialization of it alike: a
Reference gives the one object, a Reference set the list. `range`, `add`, `remove` and
`clear` over the object are not reported — mx check does not type their variable.

```
$P = retrieve over Mod.Order_Parent from $Order (a Mod.Order) is one Mod.Order — a
Reference followed from its FROM end, which is how Mendix follows a self-reference — so
`count` cannot take it: mxbuild rejects it with CE0013 "Input variable 'P' must be of
type 'List'." [MDL-RETRIEVE01]
```

A flow already stored with such a member describes with a `-- WARNING` on the change
line. `create or modify` will not splice over it: without the header it rebuilds the flow
(which repairs it), and under `mdl 1;` it refuses with the drop-and-create advice.

### MDL-EMAIL02 / 03: A `send email` setting Studio Pro would not allow

```
send email: CheckServerIdentity has no effect without SecurityType: ssl (it is TLS) [MDL-EMAIL02]
send email: header name "Bad Name!" may contain only letters, digits and hyphens [MDL-EMAIL03]
```

**Cause:** Studio Pro enables *Check Server Identity* only for SSL (MDL-EMAIL02),
and restricts a custom header to a name of letters, digits and hyphens and a
single-line, non-empty value (MDL-EMAIL03). mxbuild 11.15 accepts both, which is
why they are warnings: a script is the only way to store them, and the flag is
ignored or the header fails only when a mail is sent.

**Solution:** `SecurityType: ssl, CheckServerIdentity: true`, or drop
`CheckServerIdentity`; rename the header, e.g.
`Headers: ('X-Correlation-Id': 'value')`.

### MDL-TEMPLATE01: A message written as an expression followed by `with (…)`

```
log: the message is an expression, so `with (…)` has no template to fill — the
parameters fill the {n} placeholders of a message written as a string. [MDL-TEMPLATE01]
```

**Cause:** The text of a `log` message, a `send email` subject or body, a `show message`
and a `validation feedback` message is a template: a string whose `{n}` placeholders
`with (…)` fills. An expression is the whole text, stored as the template `'{1}'` with the
expression as its parameter, so it has no placeholder for further parameters, and Mendix
cannot store an expression as a template's text. Before ako/mxcli#1683, `log` and
`send email` stored the expression's source as the template — the log line read `$Msg` —
and `show message` / `validation feedback` stored the parameters where no placeholder read
them. `check`, `exec` and `mx check` passed all four. `check` and `exec` now refuse it with
one shared function, as MDL-REST05 does for `call rest service`.

**Solution:** Write the template as a string, or the expression without `with (…)`:

```mdl
log info node 'Orders' 'Order {1} shipped' with ({1} = $Order/Number);
log info node 'Orders' $Msg;
```

### MDL-TEMPLATE02: A template's braces that MxBuild refuses

```
log: the message template has `{number}` — MxBuild rejects it (CE0712 "Brace should be
followed by a number of digits or another brace.") [MDL-TEMPLATE02]
```

**Cause:** Every Mendix text template — a `log`, `show message` or `validation feedback`
message, a `send email` subject or body, a `call rest service` URL or body, a Java action's
text-template argument, a workflow's display, description and task texts, a dynamic text's
content and a button's caption — reads its text with one grammar. A `{` opens a numbered
placeholder (`{1}`, `{2}`, …) or is escaped as `{{`; a `}` needs no escape. Anything else is a
build error, with the code `check` reports in its message:

| Text | MxBuild |
|------|---------|
| `{number}`, `{}`, `{ 1 }`, `{-1}`, `{"a": 1}` | CE0712 |
| `{1` at the end of the text | CE0713 |
| `{1x}`, `{1 }` | CE0714 |
| `{0}` | CE0716 |
| `{2147483648}` (past 32 bits) | CE0717 |
| `{2}` with one parameter | CE0720 |

Measured on mxbuild 10.24.24, 11.13.0 and 11.15.0, the same in every slot (ako/mxcli#1397).
`check` and `exec` passed all of them before; `exec` now refuses a flow with one, `--no-check`
included.

**Solution:** Number the placeholders and fill them with `with (…)`; escape a literal brace:

```mdl
log info node 'Orders' 'Item {1} not found' with ({1} = $Number);
$R = call rest service post 'https://example.com/items' (
  Body: template '{{"name": "{1}"}' with ({1} = $Name)
) returns String;
```

### MDL-REGEX01: A regular expression that ends in a lone backslash

```
return: the regular expression of replaceAll, '\', ends in an unescaped backslash — it
builds, and the call fails each time it runs [MDL-REGEX01]
```

**Cause:** The second argument of `replaceAll`, `replaceFirst` and `isMatch` is a regular
expression. One that ends in a lone `\` builds — MxBuild does not read the pattern — and
fails every time it runs: the runtime throws `PatternSyntaxException "Unescaped trailing
backslash"`, a nanoflow `"\ at end of pattern"`. A warning, since the build accepts it. The
replacement (third argument) is literal text on both sides, so a backslash or `$` in it is
inserted as written and is never reported.

**Solution:** Match a backslash with `\\` — `'\\'` under `mdl 1;`, `'\\\\'` without the
header, where `\\` is one backslash. See `mxcli syntax microflow expressions`.

### MDL-JSONNUM01: A locale-dependent number in hand-built JSON

```
set '$Json' builds JSON with formatDecimal(…) and no locale: it formats in the user's
language, so a Dutch user gets '12,50' and the JSON is invalid for them only [MDL-JSONNUM01]
```

**Cause:** `formatDecimal(x, '0.00')` without a locale argument formats in the *current user's* language. The microflow writes `12.50` for an English user and `12,50` for a Dutch one, so JSON built by concatenation (a chart's data, an API payload) is invalid only for some users — typically not the author. An underscore locale tag (`'nl_NL'`, `'en_US'`) is silently ignored and falls back to the user's language; only the hyphenated form applies. Measured on Mendix 11.13.

The rule is info and heuristic: it fires when the same `+` concatenation has a string literal containing `{` or `":`. A display string such as `'Total: ' + formatDecimal(…)` is not flagged.

**Solution:** Use `toString(round(x, 2))`, which always writes a `.` and never an exponent, or pass a hyphenated locale.

```text
-- WRONG
set $Json = $Json + ',"v":' + formatDecimal($R/Total, '0.00') + '}';
-- RIGHT
set $Json = $Json + ',"v":' + toString(round($R/Total, 2)) + '}';
set $Json = $Json + ',"v":' + formatDecimal($R/Total, '0.00', 'en-US') + '}';
```

### Mismatched input

```
Line 3, Col 42: mismatched input ')' expecting ','
```

**Cause:** Syntax error in the MDL statement -- typically a missing comma, semicolon, or unmatched bracket.

**Solution:** Check the MDL syntax at the reported line and column. Common issues:
- Missing commas between attribute definitions
- Missing semicolons at the end of statements
- Unmatched parentheses or curly braces

### No viable alternative

```
Line 1, Col 0: no viable alternative at input 'CREAT'
```

**Cause:** Unrecognized keyword or misspelling.

**Solution:** Check the keyword spelling. MDL keywords are case-insensitive but must be valid. Run `mxcli syntax keywords` for the full keyword list.

## mxcli Execution Errors

### Module not found

```
Error: module 'MyModule' not found in project
```

**Cause:** The referenced module does not exist in the `.mpr` file.

**Solution:** Check the module name with `LIST MODULES` and verify the spelling. Module names are case-sensitive.

### Entity not found

```
Error: entity 'MyModule.Customer' not found
```

**Cause:** The referenced entity does not exist in the specified module.

**Solution:** Check with `LIST ENTITIES IN MyModule`. If the entity was just created, ensure the create statement executed successfully before referencing it.

### Reference validation failed

```
Error: unresolved reference 'MyModule.NonExistent' at line 5
```

**Cause:** A qualified name references an element that does not exist in the project. This error appears with `mxcli check script.mdl -p app.mpr --references`.

**Solution:** Verify the referenced element exists, or create it before the referencing statement.

### Bare name is not a member of the list's entity

```
Error: filter($L, …): "Nonexistent" is not an attribute or association of
Shop.Order — a filter predicate is evaluated once per item, so a bare name must
be a member of the list's entity (mxbuild reports CE0117 otherwise)
```

**Cause:** A bare name in a `FILTER`/`FIND` predicate can only mean a member of the item being tested. mxcli resolves it against the list's element entity and rewrites it to `$currentObject/<member>`; a name that does not resolve cannot be rewritten into anything valid.

**Solution:** Check the spelling against `DESCRIBE ENTITY Shop.Order`. If you meant a variable rather than a member, write it with its `$` — a bare word is always read as a member of the iterated item.

Before mxcli qualified these, the unresolvable name was written into the expression verbatim and surfaced at build time as `CE0117 "Error(s) in expression."`, with `mxcli check` reporting nothing.

## BSON Serialization Errors

### Wrong array prefix

**Symptom:** Studio Pro fails to load the project or shows garbled data.

**Cause:** Missing or incorrect integer prefix in BSON arrays. Mendix BSON arrays require a count/type prefix as the first element:

```json
{
  "Attributes": [3, { ... }, { ... }]
}
```

**Solution:** Ensure all arrays include the correct prefix value (typically `2` or `3`). Check existing BSON output for the correct prefix for each array property.

### Wrong reference format

**Symptom:** Studio Pro crashes or shows null reference errors.

**Cause:** Using `BY_ID_REFERENCE` (binary UUID) where `BY_NAME_REFERENCE` (qualified string) is expected, or vice versa.

**Solution:** Check the metamodel reflection data for the property's `kind` field:
- `BY_ID_REFERENCE` -> use binary UUID
- `BY_NAME_REFERENCE` -> use qualified name string (e.g., `"Module.Entity.Attribute"`)
