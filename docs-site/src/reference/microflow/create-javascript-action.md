# CREATE JAVASCRIPT ACTION

## Synopsis

```sql
CREATE [ OR MODIFY ] JAVASCRIPT ACTION module.Name [ FOLDER 'folder_path' ] ( parameters )
    RETURNS type
    [ EXPOSED AS 'caption' IN 'category' ]
    [ PLATFORM Web | Native | Hybrid | All ]
    AS $$ javascript_code $$

DROP JAVASCRIPT ACTION module.Name;
```

## Description

Creates a JavaScript action with inline JavaScript code. JavaScript actions hold
custom client-side logic and are callable from **nanoflows**.

Two things are written: the model unit
(`JavaScriptActions$JavaScriptAction`), and the source file
`javascriptsource/<Module>/actions/<Name>.js`. The `.js` file is generated with
the JSDoc header, the exported `async function`, and the supplied body placed
between the `BEGIN USER CODE` / `END USER CODE` markers.

> **The `AS $$ ... $$` body is mandatory.** Like Java actions, every `CREATE`
> needs a code body; omitting it reports `no viable alternative at input '...'`.
> Use a stub such as `AS $$ return Promise.resolve(false); $$` when the
> implementation is not yet written.

If `OR MODIFY` is specified and the action already exists, its parameters, return
type, exposed-as settings, platform, and body are updated in place; the UUID is
preserved.

### Parameter names in the body

The model keeps a parameter's name as declared, but the generated `.js` file uses
the name Studio Pro and mxbuild generate: the **first character lower-cased**,
nothing else. The body is written against that name. `ReadOnly` is `readOnly`,
`Url` is `url`, `URL` is `uRL` (not `url`), `MyID` is `myID`, `_Lead` stays
`_Lead`. mxcli writes the signature this way and does not rewrite the body, so a
body that uses the declared spelling (`ReadOnly`) throws a `ReferenceError` when
the action runs. `DESCRIBE` still prints the declared names.

### Platform

The optional `PLATFORM` clause restricts where the action runs: `Web`, `Native`,
`Hybrid`, or `All`. When omitted it defaults to **Web**.

### Exposed Actions

The optional `EXPOSED AS` clause makes the action visible in the Studio Pro
toolbox under the given category.

## Parameters

`module.Name`
:   The qualified name of the JavaScript action.

`FOLDER 'folder_path'`
:   Optional. Places the document in the named module folder, creating missing folders in the path. On `CREATE OR MODIFY` this **moves** an existing document; omitting the clause leaves placement alone rather than returning the document to the module root. See [MOVE](../organization/move.md).

`parameters`
:   Comma-separated parameter declarations (name, colon, type). Supported types:
    - Primitives: `String`, `Integer`, `Long`, `Decimal`, `Boolean`, `DateTime`
    - Entity: `Module.EntityName`
    - List: `List of Module.EntityName`
    - Enumeration: `ENUM Module.EnumName` or `Enumeration(Module.EnumName)`
    - Type parameter declaration: `ENTITY <pEntity>`; reference: bare `pEntity`
    - Add `NOT NULL` after the type to mark a parameter as required.

`RETURNS type`
:   The return type. Same type options as parameters.

`EXPOSED AS 'caption' IN 'category'`
:   Optional. Makes the action visible in the toolbox.

`PLATFORM Web | Native | Hybrid | All`
:   Optional. Target client platform; defaults to `Web`.

`AS $$ javascript_code $$`
:   The JavaScript body, enclosed in `$$` delimiters. **Mandatory.** The code
    becomes the body of an exported `async function`, so it typically returns a
    `Promise`.

`OR MODIFY`
:   Makes the statement idempotent — updates an existing action in place
    (UUID preserved) instead of erroring on a duplicate.

## Examples

Simple action (defaults to the Web platform):

```sql
CREATE JAVASCRIPT ACTION MyModule.JSA_IsOnline () RETURNS Boolean
AS $$
    return Promise.resolve(navigator.onLine);
$$;
```

Action with parameters:

```sql
CREATE JAVASCRIPT ACTION MyModule.JSA_Add (
    A: Integer NOT NULL,
    B: Integer NOT NULL
) RETURNS Integer
AS $$
    return Promise.resolve(a + b);
$$;
```

Exposed, native-only action:

```sql
CREATE JAVASCRIPT ACTION MyModule.JSA_ShowToast (
    Message: String NOT NULL,
    Duration: Integer
) RETURNS Boolean
EXPOSED AS 'Show Toast' IN 'UI'
PLATFORM Native
AS $$
    console.log(message);
    return Promise.resolve(true);
$$;
```

Idempotent upsert with `OR MODIFY` (UUID preserved):

```sql
CREATE OR MODIFY JAVASCRIPT ACTION MyModule.JSA_Add (
    A: Integer NOT NULL,
    B: Integer NOT NULL,
    C: Integer
) RETURNS Integer
AS $$
    return Promise.resolve(a + b + (c || 0));
$$;
```

Drop a JavaScript action (removes the unit and the `.js` source file):

```sql
DROP JAVASCRIPT ACTION MyModule.JSA_IsOnline;
```

## See Also

[CREATE JAVA ACTION](create-java-action.md), [CREATE MICROFLOW](create-microflow.md)
