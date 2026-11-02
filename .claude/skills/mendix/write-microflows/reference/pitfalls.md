# Pitfalls, unsupported syntax and build errors

Supporting reference for [write-microflows](../SKILL.md).

## Common Pitfalls

### 1. Object (Entity) Variables Cannot Be Declared

**Error**: CE0053 - "Selected type is not allowed" (+ CE0038, CE7247) — passes
`mxcli check` only until MDL043 was added; previously surfaced at mxbuild.

❌ **INCORRECT:**
```mdl
declare $Product Test.Product;             -- object Create Variable — rejected
declare $Product Test.Product = $In;       -- aliasing a parameter — rejected
declare $Product as Test.Product;          -- AS keyword also not supported
```

✅ **CORRECT** — get the object from a source that produces one:
```mdl
-- as a microflow parameter
create microflow Test.Save ($Product: Test.Product) returns boolean as $ok ...

-- from a retrieve (single object)
retrieve $Product from Test.Product where Code = $Code first;

-- from a create object
$Product = create Test.Product (Name = $Name);

-- from a loop iterator
loop $Product in $Products
begin
  change $Product (Processed = true);
end
```

**Explanation**: `declare` becomes a *Create Variable* activity, which in Mendix
only holds primitive types. Objects (and lists) have no Create Variable form — use
the parameter/retrieve/create/loop sources above. To "keep a reference" to an
existing object, just use the variable you already have (`$In`, the loop variable,
the retrieve/create output); Mendix has no aliasing activity.

### 2. XPath Association Navigation

**Error**: CE0117 - "Error in expression"

❌ **INCORRECT:**
```mdl
-- Using simple association name
declare $CustomerName string = $Order/Customer/Name;
set $Name = $Product/Category/Name;
```

✅ **CORRECT:**
```mdl
-- Use fully qualified association name: Module.AssociationName
declare $CustomerName string = $Order/Shop.Order_Customer/Name;
set $Name = $Product/Shop.Product_Category/Name;
```

**Explanation**: XPath navigation requires the full qualified association name in the format `Module.AssociationName`.

A path that **ends** on the association is not a value at all, in either
direction: `return $Parent/Shop.Order_Customer;` is CE0117 at the end event
(MDL049, as for a call argument), and `loop $o in $Customer/Shop.Order_Customer`
has no list to store, CE0099 (MDL094). Retrieve over the association — that is
the activity both mean:

```mdl
mdl 1;
create microflow Shop.CustomerOrders ($Customer: Shop.Customer)
returns list of Shop.Order as $Orders
begin
  retrieve $Orders from $Customer/Shop.Order_Customer;
  loop $o in $Orders begin
    log info node 'Shop' $o/OrderNumber;
  end loop;
  return $Orders;
end;
```

Naming the target entity after a reference followed from its owner,
`$Order/Shop.Order_Customer/Shop.Customer`, is the one path form that is an
object value (Studio Pro's own spelling).

### 3. Missing Attributes

**Error**: Attribute references must exist in entity definition

❌ **INCORRECT:**
```mdl
-- Referencing Status when it doesn't exist in Order entity
change $Order (
  status = 'PROCESSING',
  ProcessedDate = [%CurrentDateTime%]);
```

✅ **CORRECT:**
```mdl
-- First, ensure entity has the attributes
create persistent entity Shop.Order (
  OrderNumber: string(50),
  status: string(50),          -- ← Must be defined
  ProcessedDate: datetime       -- ← Must be defined
);

-- Then reference them
change $Order (
  status = 'PROCESSING',
  ProcessedDate = [%CurrentDateTime%]);
```

### 4. Flow Must End with RETURN

**Error**: CE0105 - "Activity cannot be the last object of a flow"

❌ **INCORRECT:**
```mdl
begin
  declare $success boolean = true;
  log info 'Done';
  -- Missing RETURN!
end;
```

✅ **CORRECT:**
```mdl
begin
  declare $success boolean = true;
  log info 'Done';
  return $success;  -- ← Always required
end;
```

### 5. Unreachable Code After RETURN

**Error**: CE0104 - "Action activity is unreachable"

❌ **INCORRECT:**
```mdl
if $value < 0 then
  return false;
  log info 'This will never execute';  -- ← Unreachable!
end if;
```

✅ **CORRECT:**
```mdl
if $value < 0 then
  log info 'Value is negative';
  return false;
end if;
```

### 6. Unused Variables

**Warning**: CW0094 - "Variable 'X' is never used"

```mdl
mdl 1;
-- Studio Pro will warn if parameters/variables are declared but never used
create microflow Test.Example (
  $ProductCode: string  -- ← Warning if never referenced
)
returns boolean as $success
begin
  set $success = true;  -- ProductCode never used
  return $success;
end;
```

### 7. Using SET on Undeclared Variables

**Error**: MDL executor validates that all variables used with `set` are declared first.

❌ **INCORRECT:**
```mdl
begin
  if $value > 10 then
    set $message = 'High';  -- ERROR: $Message not declared!
  end if;
  return true;
end;
```

✅ **CORRECT:**
```mdl
begin
  declare $message string = '';  -- Declare first
  if $value > 10 then
    set $message = 'High';  -- Now SET works
  end if;
  return true;
end;
```

**Note**: Parameters are automatically declared by the parameter list. The `returns type as $Var` syntax names the return variable but does NOT declare it - you must still use `declare $Var type = value;` if you want to use SET on it.

### 8. RETURN Inside a Loop

**Error**: CE0068 - "End events cannot be placed inside a loop." (MDL062)

A `return` builds an End event, and Mendix does not allow one inside a loop —
whether the return sits in the loop body directly or inside a branch within it.

❌ **INCORRECT:**
```mdl
loop $Part in $PartList
begin
  if $Part/IsMatch then
    return true;   -- End event inside the loop
  end if;
end loop;
```

✅ **CORRECT** — leave the loop with `break`, and return once after it:
```mdl
declare $Found boolean = false;
loop $Part in $PartList
begin
  if $Part/IsMatch then
    set $Found = true;
    break;
  end if;
end loop;
return $Found;
```

### 9. Two Activities Creating the Same Variable

**Error**: CE0111 - "Duplicate variable name 'X'." (MDL063)

A microflow's (and a nanoflow's) variable names are unique **flow-wide**.
Branches and loop bodies do not open a scope, and parameters and loop iterators
share the same namespace.
The trap is that every activity with an output **creates** its variable — there
is no form in which a call, a retrieve, an aggregate or an import mapping writes
into one that already exists.

❌ **INCORRECT:**
```mdl
declare $Session string = '';
$Session = call microflow Mod.Login();   -- the call creates $Session too
```

✅ **CORRECT** — let the activity create it:
```mdl
$Session = call microflow Mod.Login();
```

Assigning to an existing variable is fine, because `set` is a *Change Variable*
activity and creates nothing:

```mdl
declare $Session string = '';
set $Session = 'anonymous';   -- valid, any number of times
```

A call to a Java or JavaScript action that returns **Void** creates nothing
either, whatever output name it carries — Studio Pro keeps one on such calls
(a JavaScript action's is named after the action, e.g. `$RefreshEntity`), and
two of them in one flow build clean. `describe` keeps printing the stored name
so a round trip does not change the model. The name is not a variable: using
`$RefreshEntity` afterwards is CE0109 "Undefined variable" (MDL093 — `check`
reports it when the script or, with `-p`, the project says the action is void).

### 10. Calling a Rule or Microflow Inside an Expression

**Error**: CE0117 - "Error(s) in expression." (MDL066)

A Mendix **expression** has no user-callable functions. Its library is built-in
and unqualified (`length`, `toString`, `contains`, ...); microflows, rules and
Java actions are called by **activities**. So a qualified call in a value
position is not an expression at all — mxbuild rejects it whichever document it
names.

❌ **INCORRECT:**
```mdl
declare $Active Boolean = Sample.Rule_IsActive(IsActive = $IsActive);
declare $Next Integer = Sample.MF_Increment(N = $N);   -- a microflow is no better
```

✅ **CORRECT** — a microflow or Java action is an activity:
```mdl
$Next = call microflow Sample.MF_Increment(N = $N);
```

A **rule** has no call activity at all: Mendix can only evaluate one as a
decision's condition, and that is the single position where a bare qualified
call is valid MDL.

```mdl
if Sample.Rule_IsActive(IsActive = $IsActive) then
  ...
end if;
```

The name in that position must resolve to a real **rule** — a microflow there is
the same CE0117, and mxcli refuses the statement rather than writing it.

### 11. Changing Something Inside a Loop Body

**What is spliced** (both modes): **plain activities** inside a loop body — one
inserted, dropped, moved (dropped and inserted again) or replaced by one or more
others (ako/mxcli#1693), and one replaced by one of the same kind in place
(ako/mxcli#1106). The new activities are laid out among the body's nodes; when the
body no longer fits, the loop box grows on its far side and what is drawn past the
loop moves along. Only the new activities get new `$ID`s — one replaced by the same
kind of activity is the stored one edited and keeps its `$ID` and its action's
(ako/mxcli#1761); the loop, the rest of its body and everything outside it keep theirs. The loop's iterator is in scope, and
a nested loop's body is edited the same way. `create or modify` does this for a body
whose statements change; `alter` for an activity of the body addressed by its handle.

**Refusal** for a change of the body's *shape* — a decision, a nested loop, a
`return`, `break`/`continue`, a `raise error` or an activity with a custom error
handler added, changed or taken out inside the body, or a body node drawn elsewhere:
`create or modify` under `mdl 1;` — "the Loop at (x, y) changes inside its body: a
decision is added to it; inside a loop body the splice inserts, drops and replaces
plain activities only …"; `alter` aimed at an activity in the loop — "… is inside the
body of loop $Car in $Cars, and the fragment holds a decision …". Both refusals spell
out the statement below, addressed to the innermost loop around the change.

✅ **SPLICED** — a plain activity added to the body:
```mdl
mdl 1;
alter microflow MyModule.ACT_SaveAll {
  insert after commit $Car begin log info node 'App' 'saved'; end;
};
```

❌ **REFUSED** (either way) — a decision changes the body's shape:
```mdl
mdl 1;
alter microflow MyModule.ACT_SaveAll {
  insert after commit $Car begin if $Car/Price > 0 then log info node 'App' 'priced'; end if; end;
};
```

✅ **CORRECT** — replace the loop's body:
```mdl
mdl 1;
alter microflow MyModule.ACT_SaveAll {
  replace body of loop $Car in $Cars with begin
    commit $Car;
    if $Car/Price > 0 then
      log info node 'App' 'priced';
    end if;
  end;
};
```

`replace body of` keeps the loop itself — its `$ID`, position, iterator and list, the
flows into and out of it and the notes on it — and rebuilds only what is inside it
(new IDs, laid out by mxcli). The box keeps its size when the new body fits, and grows
when it does not. A `while` loop is addressed by its handle the same way —
`replace body of while $N < 3 with begin … end;` — and so is a loop nested in another
loop. The new body is scope-checked like any fragment: a variable it reads must be the
iterator, declared in the body, or declared before the loop; one it declares must not
be declared outside the loop. (`replace loop … with begin loop … end loop; end;` still
works, and rebuilds the loop as well — its `$ID` and position included.)

Under `mdl 1;` a body that states its nodes' `@position` (a `describe` restated) is
pinned like the top level (§13): an insert that would have to move one of them, or
grow the loop past a node the script draws where it is, is refused.

### 12. One Alter Block Is One Change

Every operation of an `alter microflow` block is aimed at the flow **as stored**, but
the scope checks judge the flow the **whole block** writes:

- a fragment may read a variable an earlier operation of the block declares, when that
  declaration lands upstream of it (an `insert after`/`replace` of a node before it, or
  an `insert before` the node it replaces or inserts after) — and may `set` it or name
  its members, since the fragment is built knowing what the earlier operations declare
  (ako/mxcli#1399). Declare first, use after: a declaration made by a *later*
  operation of the block is not known to an earlier one's fragment;
- an activity whose output is taken away is still "used" only by readers the block
  leaves in place, and not by a reader that another operation's fragment declares the
  variable for upstream of it — so a producer can be moved up the flow (insert it
  where it goes, drop it where it was; `create or modify` does this for a reordered
  body), and a rename (the producer, every reader and the `return`) goes in **one**
  block:

```mdl
mdl 1;
alter microflow MyModule.GetCode {
  replace $Code = call microflow MyModule.Lookup() with begin $Code2 = call microflow MyModule.Lookup(); end;
  replace log node 'App' * with begin log info node 'App' $Code2; end;
  replace return $Code with begin return $Code2; end;
};
```

### 13. Inserting Where the Script States Every Position

`describe` output states `@position` on every statement. Under `mdl 1;`, `create or
modify` never moves a node the script draws at a stated position: a statement inserted
**without** `@position` into a gap too small for it is refused, naming the room it needs
and the node in the way (mendixlabs/mxcli#1598). Moving the rest of the row along would
leave the stored flow saying something other than the script, so the next run would move
it back. Either:

- give the new statements an `@position` where there is room for them, or
- leave the positions of what comes after them unstated (`@position`, `@merge`, a note's
  `position:`), so the splice can move it along — the second run of that script is then
  Unchanged.

A headerless (mdl 0) script still moves the row along, as it always did.

### 14. Attaching an Inserted Statement to a Note the Flow Already Has

A fragment of `alter microflow … insert|replace` is built on its own, so it cannot see
the `id:` labels `describe` prints — and those are re-derived on every describe and
stored nowhere, so a saved one could name a different note next time. Name the stored
note by its **text**, exactly as `describe` prints it (ako/mxcli#1695):

❌ **REFUSED** — "refers to a note that has not been declared above it":
```mdl
mdl 1;
-- ❌ WRONG: a describe-time id: label names no stored note
alter microflow MyModule.ACT_Process {
  insert after log node 'App' 'start' begin
    @annotation(id: n1)
    log info node 'App' 'middle';
  end;
};
```

✅ **CORRECT** — the new activity joins the note; the note keeps its `$ID`, text and place:
```mdl
mdl 1;
alter microflow MyModule.ACT_Process {
  insert after log node 'App' 'start' begin
    @annotation(existing: 'Validates the order')
    log info node 'App' 'middle';
  end;
};
```

- The text must be the text of **exactly one** note of the flow. None, or several, is
  refused by `check` and `exec` alike, and the refusal lists the flow's notes (with where
  each is drawn and what it is attached to). Two notes with the same text cannot be told
  apart by text: reword one first.
- `existing:` stands alone. Beside `text:`, `id:`, `position:` or `size:` it is refused
  (MDL079) — `@annotation 'text'` is how a statement gets a note of its own.
- Only an alter fragment takes it: `create`/`create or modify` state the whole flow and
  declare every note, so `existing:` there is refused (MDL079). `describe` never prints it.
- Re-running the alter inserts the statement again, as any `insert` does; the note is
  attached to both.

### 15. Taking Out a Decision Another Script Put In

A decision spliced in by `alter microflow … insert` — `if … then … [else …] end if` whose
paths hold plain activities (or nothing) and rejoin at one merge — is taken out whole
(ako/mxcli#1694): re-running the script that created the flow drops it, and so does
`alter … drop` or `alter … replace` addressed by its handle (its caption, e.g.
`'$Order/Total > 100'`). The flow into the decision is joined to the node after its
merge; the decision, the activities on its paths (and notes on them) and the merge go,
and every other element keeps its `$ID`. A variable a path declares that the rest of
the flow still reads is refused like any dropped producer.

```mdl
mdl 1;
alter microflow MyModule.ACT_Order {
  drop '$Order/Total > 100';
};
```

Any other decision is refused, by `check` and `exec` alike, naming the part of the
shape that is out of reach — "its true path ends in a return", "its false path holds a
decision", "… on its true path has an error handler", "3 flows join at its merge", "it
has 3 paths". Those change where the flow's paths end or meet; rebuild the flow
deliberately (drop the microflow and create it in the same script) or edit it in Studio
Pro. Inside a loop body a decision is the loop's to replace (§11).

### 16. A Message Template Is a String, Not an Expression With `with (…)`

The text of a `log` message, a `send email` subject or body, a `show message` and a
`validation feedback` message is a **template**: a string with `{n}` placeholders that
`with (…)` fills. An expression is the whole text — stored as the template `'{1}'`
with the expression as its parameter — so there is nothing left for `with (…)` to
fill, and Mendix cannot store an expression as a template's text. An expression
followed by `with (…)` is refused by `check` and `exec` (MDL-TEMPLATE01,
ako/mxcli#1683); it used to log the characters `$Msg`.

```mdl
-- ❌ WRONG: $Msg is not a template; this is refused (MDL-TEMPLATE01)
log info node 'Orders' $Msg with ({1} = $Order/Number);

-- ✅ CORRECT: the template is a string, or the expression stands alone
log info node 'Orders' 'Order {1} shipped' with ({1} = $Order/Number);
log info node 'Orders' 'Order ' + $Order/Number + ' shipped';
```

A template kept in a variable cannot be filled by `with (…)` at run time: write the
template in the statement, or build the whole text in the expression.

### 17. A String Function Never Returns `empty`

`trim`, `toLowerCase`, `toUpperCase`, `substring`, `replaceAll` and `toString` return
`''` for an `empty` input, never `empty` (measured on the Mendix 10.18.5 runtime). So
comparing their result with `empty` is constant: `!= empty` is always true, and a
blank value goes down the "has text" branch. `check` warns (E015).

```mdl
-- ❌ WRONG: always true — trim(empty) and trim('   ') are both ''
declare $HasText boolean = trim($Value) != empty;

-- ✅ CORRECT: trim(empty) is '', so this covers empty, '' and blanks
declare $HasText boolean = trim($Value) != '';
```

### 18. Concatenating `empty` Gives the Text `null`

`+` turns an `empty` String operand into the four characters `null`, not `''`:
`'[' + $Value + ']'` with an empty `$Value` is `[null]` (measured on the Mendix 10.18.5
runtime). A label built from an attribute that may be empty shows "null" to the user.
`check` cannot know whether a value will be empty, so it does not warn — guard it:

```mdl
-- ❌ WRONG: shows 'Customer: null' when Name is empty
declare $Label string = 'Customer: ' + $Customer/Name;

-- ✅ CORRECT
declare $Label string = if $Customer/Name != empty then 'Customer: ' + $Customer/Name else 'Customer: ';
```

### 19. A Brace in a Template Is a Placeholder or `{{`

In a template's text a `{` opens a numbered placeholder or is escaped as `{{`; a `}`
needs no escape. `{number}`, `{}`, `{ 1 }` and an unescaped JSON object fail the build
with CE0712 (CE0713/0714/0716/0717/0720 for the other malformed placeholders), in every
template slot — log, show message, validation feedback, send email, a REST call's URL and
body, a Java action's text template, workflow texts, a dynamic text and a button caption.
`check` reports it as MDL-TEMPLATE02 with mxbuild's code (ako/mxcli#1397).

```mdl
-- ❌ WRONG: CE0712 — a name is not a placeholder, and the JSON brace is not escaped
log info node 'Orders' 'Item {number} not found';
$R = call rest service post 'https://example.com/x' (Body: template '{"name": "{1}"}' with ({1} = $Name)) returns String;

-- ✅ CORRECT
log info node 'Orders' 'Item {1} not found' with ({1} = $Number);
$R = call rest service post 'https://example.com/x' (Body: template '{{"name": "{1}"}' with ({1} = $Name)) returns String;
```

An expression is not a template: in `'{name} = ' + $Name` the brace is plain text.

### 20. `replaceAll`'s Pattern Is a Regex; Its Replacement Is Plain Text

The second argument of `replaceAll`, `replaceFirst` and `isMatch` is a regular
expression; the replacement is inserted as written — a `\` or `$` in it is literal
(measured on the Mendix runtime and the nanoflow client). Under `mdl 1;` a string
literal is stored as written; without the header `\\` is one backslash.

```mdl
mdl 1;
-- escape a text for JSON — backslashes first, then quotes
create microflow Mod.ToJsonString ($S: String) returns String
begin
  declare $T String = replaceAll($S, '\\', '\\');  -- regex \\ matches one \, inserts \\
  return replaceAll($T, '"', '\"');                 -- inserts \"
end;
```

A regex that ends in a lone backslash (`'\'`) builds and fails every time it runs
(PatternSyntaxException); `check` warns MDL-REGEX01. The `*Between` functions
(`secondsBetween`, `minutesBetween`, …) return the absolute difference, whichever date
comes first — compare the dates for the direction.

## Implicit Variable Creation (CE0111 Duplicate Variable)

These statements **implicitly create a new variable** with the name on the left side:

- `$Var = call microflow ...`
- `$Var = call java action ...`
- `$Var = call nanoflow ...`
- `$Var = create Module.Entity (...)`
- `retrieve $Var from Module.Entity ...`

**Do NOT use `declare` before these** — it creates a duplicate variable (CE0111):

```mdl
-- WRONG: Duplicate variable — DECLARE + CALL both create $Result
declare $Result boolean = false;
$Result = call java action Module.DoSomething();  -- CE0111!

-- CORRECT: Let CALL create the variable, use a different name if you need a default
declare $success boolean = false;
$CallResult = call java action Module.DoSomething();
set $success = $CallResult;

-- CORRECT: Simple pass-through (no default needed)
$Result = call java action Module.DoSomething();
return $Result;
```

The same applies to RETRIEVE:

```mdl
-- WRONG: declaring a list at all (CE0053/CE0038, MDL040) — and duplicate variable
declare $Items list of Module.Entity = empty;
retrieve $Items from Module.Entity where Active = true;  -- CE0111!

-- CORRECT: Let RETRIEVE create the variable
retrieve $Items from Module.Entity where Active = true;
```

**Note**: `returns type as $Var` in the microflow signature does NOT create an activity variable — it only names the return value. So `$Var = call java action ...` after `returns as $Var` is fine (one creation).

**Variables are scoped to the branch that creates them.** A variable first created
inside an `if`/`else` arm (including by `$Var = call ...`) is not visible outside
that arm. To use one value after a conditional, `declare` it *before* the
conditional and assign in every branch:

```mdl
-- WRONG: $GTotalText is created only in the `then` arm → not declared in `else`
if $HasVariance then
  $GTotalText = call microflow Module.FMT_Variance(Value = $v);   -- created here only
else
  set $GTotalText = 'n/a';                                 -- error: not declared
end if;

-- CORRECT: declare before, then set in each branch (call into a temp, then set)
declare $GTotalText string = '';
if $HasVariance then
  $Tmp = call microflow Module.FMT_Variance(Value = $v);
  set $GTotalText = $Tmp;
else
  set $GTotalText = 'n/a';
end if;
```

**Fallback chains reuse the same name = CE0111.** Because each `$Var = call microflow …` (or retrieve/create) is a *fresh* variable creation, the natural "try A, else try B" shape is invalid — even when the retry is inside an `if`:

```mdl
-- WRONG: the second call re-creates $Summary — CE0111
$Summary = call microflow M.Inner(Tag = 'description');
if trim($Summary) = '' then
  $Summary = call microflow M.Inner(Tag = 'summary');   -- CE0111!
end if;

-- CORRECT: one variable per call, then a plain `set` picks the winner
$Summary = call microflow M.Inner(Tag = 'description');
if trim($Summary) = '' then
  $SummaryFallback = call microflow M.Inner(Tag = 'summary');
  set $Summary = $SummaryFallback;
end if;
```

`mxcli check --references` catches this (CE0111) before the build.
## UNSUPPORTED Syntax (Will Cause Parse Errors)

**CRITICAL**: The following syntax is NOT implemented and will cause parse errors. Do NOT use these patterns:

### ROLLBACK Statement (Supported!)

```mdl
-- CORRECT: ROLLBACK is now supported
rollback $Order;

-- With REFRESH to update client UI
rollback $Order refresh;
```

**Use Case**: Revert uncommitted changes to an object. Useful when validation fails and you want to restore the object to its database state.

### RETRIEVE: `first` for an object, `limit` for a list

```mdl
-- FIRST binds a single object (Mendix's "First object" range)
retrieve $Product from Module.Product where IsActive = true first;

-- Without a range, or with LIMIT/OFFSET, it binds a list
retrieve $ProductList from Module.Product where IsActive = true;
retrieve $Page from Module.Product sort by Name asc limit 20 offset 40;
```

`limit 1` without `offset` changes meaning with the language version: a list of
one under `mdl 1;`, but in a script without the header the object, with warning
`MDL-V1-LIMIT1`. Write `first` whenever you mean the object.

### WHILE Loop

```mdl
-- WHILE loops iterate while a condition is true
while $Counter < 10
begin
  set $Counter = $Counter + 1;
end while;

-- FOR EACH loops iterate over a list
loop $item in $ItemList
begin
  -- Process each item
end loop;
```

### CASE with string values, `else`, or an alias

`case … end case` **is supported** — see [CASE Statements (Enum Split)](#case-statements-enum-split)
above for the correct form. What is not supported is the SQL-flavoured spelling of
it: quoted values, an `else` fallback, and an `AS` alias all fail.

```text
-- WRONG: case values are not string literals (parse error)
case $Order/Status
  when 'Active' then set $Result = 1;
end case;

-- WRONG: case values are not qualified (parse error)
case $Order/Status
  when MyModule.Status.Active then set $Result = 1;
end case;

-- WRONG: no AS alias (parse error: mismatched input 'as' expecting WHEN)
case $Order/Status as s
  when Active then set $Result = 1;
end case;

-- WRONG: no else branch — an enumeration split has no default flow (parse error)
case $Order/Status
  when Active then set $Result = 1;
  else set $Result = 0;
end case;
```

```mdl
-- CORRECT: bare enum values, one branch per value, including (empty)
case $Order/Status
  when Active then set $Result = 1;
  when Inactive then set $Result = 2;
  when (empty) then set $Result = 0;
end case;
```

An enum split is the *only* thing `case` does — it branches on an enumeration, not
on arbitrary expressions. For anything else (a string comparison, a numeric range),
use nested `if … else … end if`.

### TRY/CATCH Block

```mdl
-- WRONG: TRY/CATCH not supported
TRY
  commit $Order;
CATCH
  log error 'Commit failed';
end TRY;

-- CORRECT: Use ON ERROR on specific activities
commit $Order on error begin
  log error 'Commit failed';
end error;
```

### BREAK/CONTINUE in Loops

```mdl
-- WRONG: BREAK/CONTINUE not supported
loop $item in $ItemList
begin
  if $item/Skip = true then
    continue;  -- NOT SUPPORTED
  end if;
  if $item/Stop = true then
    break;     -- NOT SUPPORTED
  end if;
end loop;

-- CORRECT: Use conditional logic
loop $item in $ItemList
begin
  if $item/Skip = false and $item/Stop = false then
    -- Process item
  end if;
end loop;
```

### Reserved Words as Identifiers

**Best practice: Always quote all identifiers** (attribute names, parameter names, entity names) with double quotes. This eliminates all reserved keyword conflicts and is always safe — quotes are stripped automatically by the parser.

> **Exception — never quote `$`-prefixed variable/parameter references.** The quote
> rule is for *bare* names (entities, attributes, associations, declared parameter
> names). Variable/parameter **references** in expressions stay **unquoted**:
> `$Customer/Name`, `$currentObject`, `retrieve … from $List`. Quoting the `$` token
> (`"$Customer"`) breaks resolution.

> **In an XPath, quote names, never values.** `["Status" = 'Accepted']` is fine — the
> quotes come off the name, in a retrieve, a grant and a workflow targeting alike. A
> string is always single-quoted: `[Name = "Admin"]` is refused, because a name can
> never be the right-hand side of an XPath comparison (write `[Name = 'Admin']`).

```mdl
mdl 1;
create persistent entity Module."item" (
  "check": boolean default false,
  "text": string(500),
  "format": string(50),
  "value": decimal,
  "create": datetime,
  "delete": datetime
);
```

Quoted identifiers also work for microflow parameter names:
```mdl
mdl 1;
create microflow Module."Process" ("select": string, "type": integer)
begin
  log info 'Processing';
  return;
end;
```
## Common Studio Pro Errors

| Error Code | Message | Fix |
|------------|---------|-----|
| CE0053 | Selected type is not allowed | Don't `declare` an object/list — get it from a parameter, retrieve, create, or loop (MDL043/MDL040) |
| CE0117 | Error in expression | Use qualified association names; use `not(expr)` not bare `not expr` |
| CE0104 | Action activity is unreachable | Remove code after RETURN |
| CE0105 | Must end with end event | Add RETURN statement |
| CE0008 | No action defined | Define action for activity |
| CW0094 | Variable never used | Remove unused variables or use them |
| MDL | Variable not declared | Use `declare $var type = value;` before SET |

## Apply entity access

`@applyentityaccess` before a `create microflow` (or `create rule`) sets Studio
Pro's **"Apply entity access"** checkbox: the flow runs under the **current
user's** entity access rules instead of with full access.

```mdl
mdl 1;
@applyentityaccess
create microflow MyModule.ReadOwnOrders ()
returns list of MyModule.Order
begin
  retrieve $Orders from MyModule.Order;
  return $Orders;
end;
```

It is a **security** setting and it only ever narrows, so the rules mirror
`@excluded`:

- **An absent `@applyentityaccess` never turns it off.** It means "the script does
  not say", so a `create or modify` that omits it preserves whatever is stored.
  Before this was carried, every rewrite cleared the flag — *widening* what the
  microflow could read and write, with `mxcli check`, mxbuild and the model all
  perfectly happy. Measured across 342 microflows in 4 projects: every microflow
  storing the flag came back without it, and nothing anywhere reported it.
- **Turning it off is explicit**: `@applyentityaccess(false)`.
- **A flow without it cannot call a flow with it.** A microflow that does not apply
  entity access may only call microflows — and decide on rules — that do not apply
  it either: mxbuild CE0114, `check -p` MDL-SEC22. The reverse (on calling off) is
  fine. Fix the caller, not the callee: turning it off on a shared sub-flow widens
  it for everyone.
- **Not available on a nanoflow.** A nanoflow runs in the client and Mendix stores
  no such property. Writing it there is **MDL059**, not a silent no-op —
  the same rule that catches `@applyentityacces` and any other annotation the
  document does not read. The message names what that document does accept.

## Document properties: authorable, and omitted still preserves

Four microflow properties live in the header rather than the body:

```mdl
create or modify microflow Shop.ACT_ShowOrder ($Order: Shop.Order, $Tab: String)
url 'order/{Order}'
url search parameters ($Tab)
export level api
disallow concurrent execution error message 'This order is already being processed'
begin
  ...
end;
```

- **`url`** is the deep link (Mendix **10.6+**), Studio Pro's URL field. `url search
  parameters (...)` names the parameters passed as query arguments; `drop url`
  removes both.
- **`export level api | hidden`** decides whether the microflow is part of the
  module's public surface when the module is exported. It bites in a
  **Protected (add-on) module** — `describe module` prints
  `-- export level: Protected …` and `show modules` says `Protected AddOn`:
  another module or the navigation referencing a Hidden microflow, entity,
  attribute, association or any page there is **CE1406**, and an `api`
  microflow whose parameter or return type is a Hidden entity or enumeration of
  that module is **CE5400**. `check -p --references` reports both
  (mendixlabs/mxcli#1498), including a flow's use of a Hidden attribute or
  association — a change / create member, an association retrieve, an XPath
  step or a `$v/…` path, on a parameter, a retrieved or created object, a loop
  iterator or a `call microflow` result (ako/mxcli#1701) — and the Hidden
  entity a database `retrieve`, a
  `create` or a `create list of` names (ako/mxcli#1699), with the `alter`
  statement that makes it usable. Inside the Protected module itself every one
  of these is fine.
  Page widgets binding a Hidden member are not checked yet. A page there cannot
  be made usable (mxbuild refuses it even when stored as API) — reach it through
  an `api` microflow in the module that shows it. Entities, attributes,
  associations and enumerations take the same clause: `alter entity M.E set
  export level api`, `Attr: Type export level api`, `create association … export
  level api`, `alter enumeration M.K set export level api`
  (`mxcli syntax domain-model.export-level`).
- **`disallow concurrent execution`** takes `error message 'text'` or
  `error microflow Mod.Name`; `allow concurrent execution` is the default.

**An omitted clause preserves what is stored.** Same rule as `@excluded`,
`@applyentityaccess` and `EXPOSED AS`: a `create or modify` that only edits the
body leaves all of them alone. That is the fix for #1120, and the clauses are the
way to opt out of it deliberately — `drop url`, `export level hidden`,
`allow concurrent execution`.

### Four rules that used to surface only at build time

| | Rule | Mendix reports |
|---|---|---|
| **MDL-MF01** | every `{Name}` must name a parameter of this microflow | — |
| **MDL-MF02** | a parameter in the PATH may **not** also be a search parameter | CE5612 |
| **MDL-MF03** | `disallow` needs an error message or microflow | CE4899 |
| — | a URL another microflow already owns (needs `-p`) | CE0570 |

MF02 is the one to remember: path parameters and query parameters are disjoint
sets. `url 'item/{Key}'` with `url search parameters ($Key)` is rejected — use a
different parameter for the query argument.

A segment may carry an attribute path — `{Customer/Name}` binds the **Customer**
parameter by one of its attributes — so the leading identifier is what must match.

### Two things that do NOT get cleared

- **`allow concurrent execution` leaves a stored error message.** Studio Pro greys
  those fields rather than erasing them, so re-disallowing restores the message —
  and `canon.CarryTranslations` would put it back regardless, because a rebuild
  cannot distinguish "cleared on purpose" from "the statement could not say it".
  An inert stored message breaks nothing: Mendix reads it only when execution is
  disallowed.
- **Other languages of an error message.** MDL states one string, but a rewrite
  keeps the rest: measured, restating an English message left its Dutch
  translation untouched. `describe` flags the languages a **copy** would not carry.

### `Mark as used` still has no clause

It is carried across a rewrite like the others were, and there is no way to set
it from MDL. Nothing is lost by that — it only suppresses an editor warning.

## `drop` + `create` is still a new document

**Do the drop and the create in ONE script.** The drop's module-role grants (and
the unit's ID and folder) carry only to a create later in the same script or REPL
session. A drop in one run and a create in the next loses every `grant execute` —
CE0106 on the pages that call the flow (`check -p` reports it as MDL-SEC21).
`drop` prints the roles it removed and the `grant` that restores them. A `drop
page` never carries its view grants.

`drop microflow` followed by `create microflow` starts from nothing, so it keeps
none of these unless the script restates them. Use `create or modify` to edit a
microflow that carries any of them — and note that `describe` now emits all
four clauses, so **describe → rename → exec copies them faithfully**. Give the
copy a different `url`, though: two microflows may not share one (CE0570).

The flow's `/** … */` doc comment goes directly above the `create`, **after** the
`drop`. A doc comment belongs to the statement right after it, and a `drop` (like
a `grant`, `revoke`, `set` or `alter`) cannot store one, so a comment above the
drop is silently lost — mxcli-rest lost six flows' documentation that way.
`check` warns **MDL089** and names the statement that would have taken it:

```sql
mdl 1;
drop microflow if exists Sales.ACT_CreateOrder;

/** Creates an order for the current customer. */
create microflow Sales.ACT_CreateOrder ($Customer: Sales.Customer)
returns Boolean as $Created
begin
  declare $Created Boolean = true;
  return $Created;
end;
```
