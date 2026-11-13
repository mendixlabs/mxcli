# ALTER MICROFLOW / NANOFLOW

## Synopsis

```text
mdl 1;
ALTER MICROFLOW module.Name {
  INSERT AFTER | BEFORE <target> BEGIN <statements> END;
  REPLACE <target> WITH BEGIN <statements> END;
  REPLACE BODY OF <loop> WITH BEGIN <statements> END;
  DROP <target>;
};

ALTER NANOFLOW module.Name { … };
```

## Description

Patches a stored microflow or nanoflow in place: the operations are spliced
into the stored graph rather than rebuilding it. Only the new activities, the
flows rewired around them and the nodes moved to make room change; every other
element keeps its element ID (`$ID`), position and curve, so Studio Pro shows
the change and nothing else.

A target addresses one stored activity by content, as
`DESCRIBE MICROFLOW module.Name WITH HANDLES` prints it above each activity:
`$Var` for the activity that outputs `$Var`, `'Caption'` for a decision or an
activity with a custom caption, or a statement pattern in which `*` matches any
run of tokens — followed by `@n` when several match. Every target is resolved
against the flow as stored before any operation runs.

A fragment is written exactly as the same statements are in `CREATE
MICROFLOW`, between `BEGIN` and `END`.

### Editing an activity

`REPLACE` by one activity of the same kind — the same action, with a
filter's or find's expression, a retrieve's `WHERE`, a member's value or a
call's argument changed — edits the stored activity rather than putting a new
one in its place. The activity keeps its element ID, its place and its size,
and so do its action and every element under it that pairs up: a member of a
change or create by its attribute or association, an argument by its
parameter, anything else by its property and type. A member taken out goes
with its ID, which no other member inherits; a part of another type (a `FIND`
where a `FILTER` was) is new. Studio Pro, which compares and merges by element
ID, then shows the activity as changed rather than one removed and another
added. An activity replaced by another kind of activity, or by several, is new.

### Inside a loop body

Inside a loop body, plain activities are inserted, replaced and dropped the
same way — the body's first and last activity included. The new activities are
laid out among the body's nodes; when the body no longer fits, the loop box
grows on its far side, and what is drawn past the loop moves along. The loop's
iterator is in scope.

What would change the body's *shape* — a decision, a nested loop, a `RETURN`,
`BREAK` or `CONTINUE`, a `RAISE ERROR` or an activity with a custom error
handler put in, changed or taken out — is refused there, and the refusal names
`REPLACE BODY OF`.

### REPLACE BODY OF

`REPLACE BODY OF <loop>` keeps the stored loop itself — its element ID, its
position, its iterator and list, the flows into and out of it and the notes on
it — and replaces only what is inside it. The new body is laid out as `CREATE
MICROFLOW` lays out a loop body; the loop box keeps its size when the new body
fits it, and grows on its far side when it does not.

`<loop>` is the loop's handle: `loop $It in $Items`, or `while $N < 3` for a
while loop, at any depth of nesting. The new body is scope-checked like any
fragment: it may read the iterator, what it declares itself and what is
declared before the loop, and it may not declare a variable the flow already
has outside the loop.

`REPLACE <loop handle> WITH BEGIN LOOP … END LOOP; END;` also works, and
rebuilds the loop as well — its element ID and position included.

### Decisions

`DROP` and `REPLACE` take a decision out whole when it has the shape `INSERT`
produces: two paths of plain activities, or none, that rejoin at one merge.
The flow into the decision is joined to the node after its merge (or to the
replacing fragment), and the decision, the activities on its paths and the
merge go; every other element keeps its element ID. A decision is addressed by
its handle — its caption, `'$Order/Total > 100'`. Any other decision — a path
that returns or raises an error, a decision or loop on a path, an activity
with an error handler, a path from elsewhere joining it — is refused with the
part of its shape that is out of reach.

### CREATE OR MODIFY

`CREATE OR MODIFY MICROFLOW` of a flow that exists derives the same operations
from the difference between the script and the stored flow. A plain activity
added, removed, reordered or changed inside a loop body is spliced; under
`mdl 1;` a change the splice cannot make is refused with the operation that
makes it — for a decision inside a loop body, `REPLACE BODY OF` that loop.
Re-running the script that created a flow after another script spliced a
decision into it takes that decision out again, when it has the shape above.

A statement changed where it stands edits its stored activity, as above, and
so does each of several adjacent statements changed in their own places. The
report says which: `Modified microflow: M.F (spliced: 1 edited in place)` for
a node that kept its element ID — an activity of the same kind, a decision's
condition, a return's value — and `replaced with new element IDs` for one
that did not.

An activity edited in place keeps the stored value of every property MDL has
no spelling for — its documentation, and a REST call's proxy setting (`Override`
with its host, port and credential expressions, or `NoProxy`). The statement
cannot state them, so it cannot change them either; a `drop` + `create`
rebuild still writes the defaults.

## Examples

Insert, replace and drop at top level:

```sql
mdl 1;
ALTER MICROFLOW FeedbackModule.VAL_Feedback {
  INSERT AFTER $IsValidEmail BEGIN LOG INFO NODE 'Feedback' 'Email checked'; END;
  REPLACE COMMIT $Order WITH BEGIN COMMIT $Order WITH EVENTS; END;
  DROP LOG DEBUG NODE 'Feedback' *;
};
```

Add an activity inside a loop body:

```sql
mdl 1;
ALTER MICROFLOW Shop.ACT_ProcessLines {
  INSERT AFTER COMMIT $Line BEGIN LOG INFO NODE 'Shop' 'line saved'; END;
};
```

Put a decision inside a loop body, keeping the loop:

```sql
mdl 1;
ALTER MICROFLOW Shop.ACT_ProcessLines {
  REPLACE BODY OF LOOP $Line IN $Lines WITH BEGIN
    IF $Line/Quantity > 0 THEN
      CHANGE $Line (Processed = true);
    END IF;
    COMMIT $Line;
  END;
};
```

Take out a decision another script put in:

```sql
mdl 1;
ALTER MICROFLOW Shop.ACT_Order {
  DROP '$Order/Total > 100';
};
```

## See Also

[CREATE MICROFLOW](create-microflow.md), [DESCRIBE MICROFLOW](/reference/query/describe-microflow.md)
