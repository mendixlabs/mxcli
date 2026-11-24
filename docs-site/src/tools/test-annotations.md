# Test Annotations

Annotations describe what a test is and what must be true when it finishes. They
live in the javadoc comment above a test's statements, one per line, each opening
its line:

```mdl
/**
 * @test dealing a board writes 81 cells
 * @cleanup none
 * @expect $result = 'ok'
 * @verify select count(*) as n from Sudoku.Cell = 81
 */
$result = call microflow Sudoku.ACT_DealGame();
/
```

| Tag | Purpose |
|-----|---------|
| `@test <name>` | Names the test. Required — a doc comment without it is not a test. |
| `@expect <condition>` | A Mendix expression over the body's variables that must be true. Repeatable. |
| `@verify <oql> <op> <value>` | An OQL post-condition on the database. Repeatable. |
| `@throws ['<message>']` | The body is expected to raise an error; with a message, one whose text contains it. |
| `@setup <Module.Microflow>` | A microflow to call before the body. Repeatable. |
| `@cleanup rollback\|none` | Whether the test's writes survive it. `rollback` is the default. |

A tag is read only when it **opens its line** (after the javadoc `*` and its
indentation). Quoting one inside a sentence — `` `@expect $x = 1` `` — is
documentation, not an assertion.

## `@test`

Names the test, and marks the doc comment as one. Everything between that comment
and the `/` that ends the block is the test's body.

A file may open with a header comment, in either spelling, and it is not part of
the first test. Exactly one `@test` may appear per block: the `/` is what ends a
test, and leaving it out is refused by name rather than silently running one of
the two tests it merges.

## `@expect`

Any Mendix expression that must evaluate to true — not a fixed `$var = value`
shape:

```mdl
@expect $result = 'John Doe'                    -- equality
@expect $product/Name != 'Widget'               -- inequality (<> is accepted too)
@expect length($result) = 81                    -- built-in functions
@expect find($result, '0') >= 0 and $count > 3  -- and / or / not(...)
@expect $status = MyModule.Status.Open          -- enumeration values
@expect count($Customers) = 5                   -- how many rows a list holds
```

`count($list)` is the one aggregate an assertion can make. Counting a list is a
Mendix Aggregate list **activity**, not an expression function, so mxcli lifts it
into that activity ahead of the decision that evaluates the condition. `sum`,
`average`, `minimum` and `maximum` aggregate an *attribute* over the list, which
an assertion has no way to supply — call a microflow that returns the figure and
assert on its result.

**An assertion the runner cannot compile is an ERROR, never a pass.** Unknown
functions, wrong arity, unbalanced parentheses, and expressions that evaluate to
a value rather than a condition are all rejected by name:

```
ERROR  a self-evident falsehood
       @expect randomInt($result) = 1: randomInt() is not a Mendix expression
       function at column 1 ("randomInt")
```

A failing assertion reports what came back, whenever the observed value's type is
pinned down by the assertion itself:

```
FAIL  the board is 81 squares
      expected length($result) = 81, actual: 27
```

A test with several `@expect` lines passes only when every one of them holds, and
it reports the first that does not. The Docker / `--legacy-runner` path evaluates
them all, and adds how many more failed — `(and 1 more failure in this test)`.

`@expect` cannot be combined with `@throws`: a body expected to fail produces no
result to assert on, so the combination is refused rather than ignored.

## `@verify`

`@expect` only sees what a microflow returned. Most Mendix microflows are side
effects, so `@verify` asserts on the rows one left behind — an OQL query, a
comparison operator, and the value it must satisfy:

```mdl
@verify select count(*) as n from Sudoku.Cell = 81
@verify select count(*) as n from Sudoku.Cell where Value = 0 > 0
```

The query runs after the microflow returns, over the same admin API `mxcli oql`
uses. Three rules follow:

- The result must be **one row and one column** (`select count(*) as n …`, or one
  attribute of one row). OQL requires the column to be named, so write `as n`.
- `@cleanup rollback` — the default — is **refused** with `@verify`: the writes
  would be undone before the query could see them. Add `@cleanup none`.
- The **legacy after-startup runner refuses the whole suite**, because its tests
  run during boot with nothing to query yet. Use `--local` or `--attach`.

A query that cannot be evaluated is an ERROR, distinct from one that returns the
wrong value (FAIL).

## `@throws`

Marks a body that is expected to raise an error. The verdict starts as a failure,
so a body that completes normally fails the test; an error that leaves the test
microflow is compared with the message.

```mdl
/**
 * @test rejects an empty order
 * @throws 'validation failed'
 */
$result = call microflow Sales.ACT_Submit(Order = $empty);
/
```

The message is **matched as a substring** of the error Mendix raised, so an
expectation names the part of the message the test can predict. A real message
routinely carries an activity name or an object id, which is why the match is not
equality:

```
FAIL  rejects an empty order (6ms, 1 assertion)
       expected an error containing 'validation failed',
       actual: Could not find object of type 'Sales.Order'
```

Write `@throws` **on its own** to expect any exception, when the message is not
the thing under test:

```mdl
/**
 * @test rejects an empty order
 * @throws
 */
$result = call microflow Sales.ACT_Submit(Order = $empty);
/
```

Anything after `@throws` that is not a single-quoted string is an error, not a
message — `@throws not found` is refused rather than read as an unquoted one.

An error raised by **any** statement in the body — a call, a `commit`, a
`change` — leaves the test microflow, and the test endpoint reports its message.
Without `@throws` that is a FAIL reading `exception during execution: <message>`;
with it, the message is compared as above. The runner adds no error handler of
its own to the body (mendixlabs/mxcli#1874: a generated handler that read the
message made mxbuild's incremental build report CE0108, so `test --attach` could
not run a test that called a microflow).

### A call with its own error handling

A call that has an `on error` clause keeps it, so a test runs the statement
exactly as production code does. What the
test then reports depends on the clause (measured on Mendix 11.13 with a Java
action that throws):

| Clause on the call | Without `@throws` | With `@throws` |
|---|---|---|
| none | FAIL `exception during execution: <message>` | PASS when the message matches |
| `on error rollback` | same as none — it is Mendix's default spelled out | same as none |
| `on error continue` | the error is swallowed; the assertions decide (`$r` is empty) | FAIL `expected an exception but none was thrown` |
| custom handler that ends normally | FAIL `not every assertion was evaluated` when an assertion reads the call's result; an assertion on what the handler did is evaluated | FAIL `expected an exception but none was thrown` |
| custom handler that re-raises (`raise error`) | FAIL `exception during execution: <message>` — the error leaves the test microflow | PASS when the message matches |

A custom handler that ends normally rejoins the flow after every statement that
reads the failed call's output, because that variable does not exist on the
error path. The assertions read exactly that, so the runner counts the
assertions it evaluated and fails the test when the count is short, instead of
reporting a pass for assertions that never ran. To test the handler's path,
assert on something the handler sets: `declare $Failed Boolean = false;` before
the call, `set $Failed = true;` inside its handler, and `@expect $Failed = true`.
That assertion does not read the call's result, so it is evaluated, and passes
only when the handler ran.

Under the Docker / `--legacy-runner` path an error that leaves the test — the
re-raising row above — ends the whole runner: that test is reported as an ERROR
that started and gave no verdict, and every later test as not executed.

## `@setup`

Names a **microflow** to call before the test's own statements — a fixture in a
Mendix app is a microflow, so there is nothing to declare:

```mdl
/**
 * @test the seed microflow writes five brands
 * @setup eShop.ACT_SeedCatalog
 * @cleanup none
 * @expect count($Brands) = 5
 */
retrieve $Brands from eShop.CatalogBrand;
/
```

Repeat it to compose fixtures; they run in the order written. Declare it once in
the file's header comment and every test in the file gets it, the file's
fixtures first:

```mdl
/**
 * Seeds every test below.
 * @setup eShop.ACT_SeedCatalog
 */
```

A header may carry only `@setup`. `@expect`, `@verify`, `@throws` and `@cleanup`
describe one test's execution, so a header carrying one is refused by name.

The setup runs **inside the test's transaction**, so under the `@cleanup
rollback` default it is undone with the test and every test starts from the same
state. A failing setup is an **ERROR** naming the microflow, not a FAIL: the test
never ran, and a broken fixture should not read as a broken feature.

`@setup` calls a microflow with no arguments — a fixture that needs arguments
gets a wrapper microflow. There is no `@teardown`; `@cleanup rollback` is the
teardown.

## `@cleanup`

`rollback` (the default) wraps the test in a transaction that is rolled back when
it returns, so its writes do not survive it. `none` leaves them in place — needed
whenever a later test, or a `@verify`, has to see them. An unknown strategy is a
parse error.

## Related Pages

- [Test Formats](test-formats.md) — the `.test.mdl` and `.test.md` file layouts
- [Running Tests](running-tests.md) — `mxcli test`, `--local`, `--watch`, `--attach`
