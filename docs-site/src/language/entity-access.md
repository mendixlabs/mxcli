# Entity Access

Entity access rules control which module roles can create, read, write, and delete objects of a given entity. Rules can restrict access to specific attributes and apply XPath constraints to limit which rows are visible.

## GRANT on Entities

```sql
GRANT <rights> ON ENTITY <Module>.<Entity> TO <Module>.<Role> [, ...] [WHERE [<xpath>]];
```

The `<rights>` list is a comma-separated combination of:

| Right | Syntax | Description |
|-------|--------|-------------|
| Create | `CREATE` | Allow creating new objects |
| Delete | `DELETE` | Allow deleting objects |
| Read all | `READ *` | Read access to all attributes and associations |
| Read specific | `READ (<attr>, ...)` | Read access to listed members only |
| Write all | `WRITE *` | Write access to all attributes and associations |
| Write specific | `WRITE (<attr>, ...)` | Write access to listed members only |

## Examples

### Full Access

Grant all operations on all members:

```sql
GRANT CREATE, DELETE, READ *, WRITE * ON ENTITY Shop.Customer TO Shop.Admin;
```

### Read-Only Access

```sql
GRANT READ * ON ENTITY Shop.Customer TO Shop.Viewer;
```

### Selective Member Access

Restrict read and write to specific attributes:

```sql
GRANT READ (Name, Email, Status), WRITE (Email) ON ENTITY Shop.Customer TO Shop.User;
```

### Members Added Later

A rule also carries a **default for members added after it was written**, and MDL
derives that default from the grant rather than stating it: `WRITE *` gives
ReadWrite, `READ *` gives ReadOnly, and a grant written **purely as member
lists** leaves it at **None**.

So an attribute added later is granted None on a member-listed rule. The model
is complete and correct — every rule gets an entry for the new member, and the
build reports no errors — but the attribute renders blank for that role:

```sql
GRANT READ (Name, Email) ON ENTITY Shop.Customer TO Shop.User;
-- later:
--   alter entity Shop.Customer add attribute Phone: String;
-- Phone is granted None to Shop.User. Nothing is broken; it is simply not visible.
```

`alter entity … add attribute` reports this and prints the GRANT that fixes it:

```
Added attribute 'Phone' to entity Shop.Customer
Warning: Shop.Customer.Phone is not readable by Shop.User
  ...
    grant read (Phone) on entity Shop.Customer to Shop.User;
```

What decides this is the rule's **default**, not how narrow its member list is. A
rule granted `READ *, WRITE (Email)` is narrower than `READ *, WRITE *` and still
sees new members, because `READ *` set its default to ReadOnly. If you want a
role to pick up future members automatically, give its rule a `READ *` or
`WRITE *` and narrow from there with [REVOKE](#revoke-on-entities).

### XPath Constraints

Limit which objects a role can see or modify using an XPath expression in the `WHERE` clause:

```sql
mdl 1;
-- Users can only access their own orders (Shop.Order stores its owner)
GRANT READ *, WRITE * ON ENTITY Shop.Order TO Shop.User
  WHERE [System.owner = '[%CurrentUser%]'];

-- Only open orders are editable
GRANT READ *, WRITE * ON ENTITY Shop.Order TO Shop.User
  WHERE [Status = 'Open'];
```

An access rule has no variables: `$currentUser` (or any `$var`) is CE0161 in a
constraint, so write the user as the `'[%CurrentUser%]'` token, compared with
`System.owner` on an entity that stores its owner or with an association to
`System.User`. `mxcli check` reports a variable as MDL-XPATH04.

Note that single quotes inside XPath expressions must be doubled (`''`), since the entire expression is wrapped in single quotes.

### Additive Behavior

GRANT is **additive**. If a role already has an access rule on the entity, the new rights are merged in without removing existing permissions:

```sql
mdl 1;
-- Initial grant
GRANT READ (Name, Email) ON ENTITY Shop.Customer TO Shop.User;

-- Add Notes access — Name and Email are preserved
GRANT READ (Notes) ON ENTITY Shop.Customer TO Shop.User;
-- Result: READ (Name, Email, Notes)

-- Upgrade Email to writable — existing reads preserved
GRANT WRITE (Email) ON ENTITY Shop.Customer TO Shop.User;
-- Result: READ (Name, Notes), WRITE (Email)
```

Merging only ever widens access — rights rank `None < ReadOnly < ReadWrite`, and a
GRANT takes whichever is higher. To take access away, use [REVOKE](#revoke-on-entities);
that is what makes the two commands inverses rather than two spellings of "set".

### One Rule per Constraint

The constraint is part of a rule's identity. Two GRANTs for the same role with the
**same** `WHERE` update one rule; with **different** constraints they produce two,
which is how row-level security is normally written:

```sql
mdl 1;
-- Two separate rules for one role
GRANT READ * ON ENTITY Shop.Order TO Shop.User WHERE [Status = 'Open'];
GRANT READ (Total) ON ENTITY Shop.Order TO Shop.User WHERE [System.owner = '[%CurrentUser%]'];
```

Mendix combines them at runtime: a role's effective access is the union of every
rule naming it. An unconstrained rule is distinct from a constrained one, so
adding a `WHERE` to an existing GRANT creates a second rule rather than narrowing
the first — and because a rule is matched on its constraint, re-running a script
updates the same rules instead of accumulating new ones.

### Multiple Roles on the Same Entity

Each GRANT creates a separate access rule. An entity can have rules for multiple roles:

```sql
mdl 1;
GRANT CREATE, DELETE, READ *, WRITE * ON ENTITY Shop.Order TO Shop.Admin;
GRANT READ *, WRITE * ON ENTITY Shop.Order TO Shop.User WHERE [Status = 'Open'];
GRANT READ * ON ENTITY Shop.Order TO Shop.Viewer;
```

## REVOKE on Entities

Remove an entity access rule entirely or revoke specific rights:

```sql
-- Full revoke (removes entire rule)
REVOKE <Module>.<Role> ON <Module>.<Entity>;

-- Partial revoke (downgrades specific rights)
REVOKE <Module>.<Role> ON <Module>.<Entity> (<rights>);
```

Examples:

```sql
mdl 1;
-- Remove all access for Viewer
REVOKE ALL ON ENTITY Shop.Customer FROM Shop.Viewer;

-- Remove read access on a specific attribute
REVOKE READ (Notes) ON ENTITY Shop.Customer FROM Shop.User;

-- Downgrade write to read-only on Email
REVOKE WRITE (Email) ON ENTITY Shop.Customer FROM Shop.User;
```

A full `REVOKE` (without rights list) removes the entire access rule. A partial `REVOKE` downgrades specific rights: `REVOKE READ (x)` sets member x to no access, `REVOKE WRITE (x)` downgrades from ReadWrite to ReadOnly.

## Viewing Entity Access

```sql
-- See which roles have access to an entity (the two spellings are synonyms)
LIST ACCESS ON ENTITY Shop.Customer;
LIST ACCESS ON Shop.Customer;

-- Full matrix across a module
DESCRIBE SECURITY MATRIX IN Shop;
```

## See Also

- [Security](./security.md) -- overview of the security model
- [Module Roles and User Roles](./roles.md) -- defining the roles referenced in access rules
- [Document Access](./document-access.md) -- microflow and page access
- [GRANT / REVOKE](./grant-revoke.md) -- complete GRANT and REVOKE reference
