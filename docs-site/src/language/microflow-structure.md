# Structure

This page covers the structural elements of a microflow definition: the `CREATE MICROFLOW` syntax, parameters, variable declarations, return values, and annotations.

## Full CREATE MICROFLOW Syntax

```sql
CREATE [OR REPLACE] MICROFLOW <Module.Name>
  [FOLDER '<path>']
BEGIN
  [<declarations>]
  [<activities>]
  [RETURN <value>;]
END;
```

The `OR REPLACE` modifier overwrites an existing microflow of the same name. The `FOLDER` clause organizes the microflow within the module's folder structure.

## Parameters

Parameters are declared with `DECLARE` at the top of the `BEGIN...END` block. They define the inputs to the microflow.

### Primitive Parameters

```sql
DECLARE $Name String;
DECLARE $Count Integer;
DECLARE $IsActive Boolean;
DECLARE $Amount Decimal;
DECLARE $StartDate DateTime;
```

Supported primitive types: `String`, `Integer`, `Long`, `Boolean`, `Decimal`, `DateTime`.

### Entity Parameters

Entity parameters receive a single object:

```sql
DECLARE $Customer MyModule.Customer;
```

> **Important:** Do not use `= empty` or `AS` with entity declarations. The correct syntax is simply `DECLARE $Var Module.Entity;`.

### List Parameters

List parameters receive a list of objects:

```sql
DECLARE $Orders List of Sales.Order = empty;
```

The `= empty` initializer creates an empty list. This is required for list declarations.

### Parameter vs. Local Variable

In MDL, all `DECLARE` statements at the top of a microflow are treated as parameters. Variables created by activities (such as `$Var = CREATE ...` or `RETRIEVE $Var ...`) are local variables. If you need a local variable with a default value, use `DECLARE` followed by `SET`:

```sql
DECLARE $Counter Integer = 0;
SET $Counter = 10;
```

## Variables and Assignment

### Variable Declaration

```sql
DECLARE $Message String = 'Hello';
DECLARE $Total Decimal = 0;
DECLARE $Found Boolean = false;
```

### Assignment with SET

Change the value of an already-declared variable:

```sql
SET $Counter = $Counter + 1;
SET $FullName = $FirstName + ' ' + $LastName;
SET $IsValid = $Amount > 0;
```

The variable must be declared before it can be assigned.

### Variables from Activities

Activities like CREATE and RETRIEVE produce result variables:

```sql
$Order = CREATE Sales.Order (Status = 'New');
RETRIEVE $Customer FROM Sales.Customer WHERE Email = $Email FIRST;
$Result = CALL MICROFLOW Sales.CalculateTotal (Order = $Order);
```

## Return Values

Every flow path should end with a `RETURN` statement. The return type is inferred from the returned value.

### Returning a Primitive

```sql
RETURN true;
RETURN $Total;
RETURN 'Success';
```

### Returning an Object

```sql
RETURN $Order;
```

### Returning a List

```sql
RETURN $FilteredOrders;
```

### Returning an Enumeration

Declare the type in the signature with `returns enum Module.Name`; `returns Enumeration(Module.Name)` is the same. A bare `returns Module.Name` is also accepted when no entity has that name and an enumeration does. If an entity and an enumeration share the name, the entity wins, so write `enum` when a name could be either. Nanoflows and rules take the same spellings, and a rule must return a Boolean or an enumeration.

```sql
CREATE MICROFLOW Shop.SUB_Pick () RETURNS enum Shop.Variant AS $Variant
BEGIN
  RETURN Shop.Variant.Original;
END;
```

### Returning Nothing

If the microflow returns nothing (void), you can omit the `RETURN` or use:

```sql
RETURN;
```

## Annotations

Annotations are metadata decorators placed **before** an activity. They control visual layout and documentation in Mendix Studio Pro.

### Position

Set the canvas position of the next activity:

```sql
@position(200, 100)
$Order = CREATE Sales.Order (Status = 'New');
```

Positions are optional. A microflow written without them is laid out by mxcli: the
main line runs left to right and wraps onto a new row once it passes two canvas
widths (2880 px); a guard — `if … then …; return; end if` — drops its branch into the
lane below while the main line carries on above it; and a `case` of four or more
branches leaves the decision in three groups (top, right, bottom) so its lines do not
cross. A statement that carries `@position` is never moved, and becomes the start of
the row for the statements after it — so either place everything or nothing: a few
hand-placed statements are not measured against what is laid out around them.

To re-arrange a flow that already exists — one drawn in Studio Pro, or one whose
positions no longer fit after edits — run `mxcli layout flows`. It applies this
same layout to the stored flow and changes nothing but coordinates; see
[Microflow and Nanoflow Layout](../tools/flow-layout.md).

### Start event

The start event has no statement of its own, so `@start` goes on the **first**
statement — the one the start flows into:

```sql
@start(145, 200)
@position(260, 200)
$Order = CREATE Sales.Order (Status = 'New');
```

It is optional. Omitted, the start is placed one spacing unit left of the first
activity on that activity's centre line, and a rewrite re-derives it so the start
follows the activities when they move. A start that is *not* at that derived spot
was placed on purpose — in Studio Pro or with `@start` — so it survives a rewrite
that does not mention it, and `DESCRIBE` emits an `@start` line for it. An
explicit `@start` overrides both.

### Anchors

`@anchor(from: X, to: Y)` picks the side (`top`, `right`, `bottom`, `left`) each
end of a flow attaches to: `to` is the flow arriving at the statement, `from` the
flow leaving it. On an `if`, `true: (from: …, to: …)` and `false: (…)` are its
branches, and `from` is the flow leaving its closing merge — the merge has no
statement of its own, just as its position rides on the `if` as `@merge`:

```sql
create microflow Sales.ACT_CheckFactory ($Factory: Sales.Factory)
begin
  @anchor(from: bottom, to: top)
  @merge(2650, 200)
  if $Factory/Latitude = empty then
    log warning node 'Factory' 'No location';
  end if;
end;
```

This is how `DESCRIBE` writes a decision whose merge drops onto the next row.

### Layout in `DESCRIBE` output

`DESCRIBE MICROFLOW` and `DESCRIBE NANOFLOW` print a layout annotation —
`@position`, `@merge`, `@anchor`, `@curve`, `@start` — only where the layout
would not produce it on its own. A flow written without annotations describes
without them. A statement placed by hand keeps its `@position`, and the
statements the layout places after it follow from it without one.

Whether an annotation is needed is not guessed from the coordinates. `DESCRIBE`
builds the flow again from its own output, exactly as `CREATE OR MODIFY` would
(nothing is written), and every node that lands elsewhere keeps its annotation.
So re-executing a description never moves a node: a flow drawn in Studio Pro
comes back with its layout, because Studio Pro positions are not ones the layout
produces. `DESCRIBE … WITH HANDLES` and `DESCRIBE … NORMALIZED` still print every
annotation.

### Caption

Set a custom caption displayed on the activity in the canvas:

```sql
@caption 'Create new order'
$Order = CREATE Sales.Order (Status = 'New');
```

### Color

Set the background color of the activity:

```sql
@color Green
COMMIT $Order;
```

### Annotation (Visual Note)

Attach a visual annotation note to the next activity:

```sql
@annotation 'This step validates the input before saving'
VALIDATION FEEDBACK $Customer/Email MESSAGE 'Email is required';
```

### Combining Annotations

Multiple annotations can be stacked before a single activity:

```sql
@position(300, 200)
@caption 'Validate and save'
@color Green
@annotation 'Final step: commit the validated order'
COMMIT $Order;
```

## Complete Example

```sql
CREATE MICROFLOW Sales.ACT_ProcessOrder
FOLDER 'Orders/Processing'
BEGIN
  -- Parameters
  DECLARE $Order Sales.Order;
  DECLARE $ApplyDiscount Boolean;

  -- Local variable
  DECLARE $Total Decimal = 0;

  -- Retrieve order lines
  RETRIEVE $Lines FROM $Order/Sales.OrderLine_Order;

  -- Calculate total
  @caption 'Calculate total'
  $Total = CALL MICROFLOW Sales.SUB_CalculateTotal (
    OrderLines = $Lines
  );

  -- Apply discount if requested
  IF $ApplyDiscount THEN
    SET $Total = $Total * 0.9;
  END IF;

  -- Update order
  CHANGE $Order (
    TotalAmount = $Total,
    Status = 'Processed'
  );
  COMMIT $Order;

  RETURN $Order;
END;
```
