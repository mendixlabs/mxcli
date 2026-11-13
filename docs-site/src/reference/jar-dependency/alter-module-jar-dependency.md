# ALTER MODULE … JAR DEPENDENCY

## Synopsis

    ALTER MODULE module_name
      ADD JAR DEPENDENCY [IF NOT EXISTS] (
        group    = 'groupId',
        artifact = 'artifactId',
        version  = 'version',
        included = true|false,
      );

    ALTER MODULE module_name
      SET JAR DEPENDENCY 'group:artifact' VERSION 'new-version';

    ALTER MODULE module_name
      SET JAR DEPENDENCY 'group:artifact' INCLUDED true|false;

    ALTER MODULE module_name
      SET JAR DEPENDENCY 'group:artifact' ADD EXCLUSION 'group:artifact';

    ALTER MODULE module_name
      SET JAR DEPENDENCY 'group:artifact' DROP EXCLUSION 'group:artifact';

    ALTER MODULE module_name
      DROP JAR DEPENDENCY 'group:artifact';

## Description

Manages Maven/JAR dependencies stored in a module's `Projects$ModuleSettings`
document. These are the same dependencies shown in Studio Pro under
**Module Settings → Java dependencies**.

Multiple actions can be chained in a single `ALTER MODULE` statement.

A bare `ADD JAR DEPENDENCY` fails when the module already declares the same
`group:artifact`, so a script containing it cannot be executed twice.
`ADD JAR DEPENDENCY IF NOT EXISTS` is the re-runnable form, with the same
semantics as `ADD ATTRIBUTE IF NOT EXISTS` and `CREATE ENTITY IF NOT EXISTS`:

- When the coordinate is absent, it is added exactly as the bare form would.
- When it is present, it is **left untouched** and a `… already exists — skipped`
  notice is printed — even if the stored `version` or `included` differs from the
  statement. The notice then names the difference and the `SET JAR DEPENDENCY`
  that would change it; `IF NOT EXISTS` never updates.
- When every action of the statement was skipped, nothing is written and the
  statement reports `Unchanged jar dependencies for module '…'`.

To make a script both re-runnable and authoritative about the version, follow
the guarded `ADD` with a `SET … VERSION`: restating the stored version writes
nothing either.

## Parameters

**module_name**
: The name of the module to modify.

**'group:artifact'**
: The Maven coordinate identifying the dependency, in `groupId:artifactId`
  format (no version). Example: `'org.duckdb:duckdb_jdbc'`.

**group, artifact, version**
: The three components of a Maven coordinate. All three are required for `ADD`.

**included**
: When `false`, the dependency is declared but excluded from the classpath
  at build time. Defaults to `true`.

## Examples

### Add a new dependency

```sql
ALTER MODULE MyModule
  ADD JAR DEPENDENCY (
    group    = 'org.duckdb',
    artifact = 'duckdb_jdbc',
    version  = '1.1.3',
    included = true,
  );
```

### Add a dependency from a re-runnable script

```sql
ALTER MODULE MyModule
  ADD JAR DEPENDENCY IF NOT EXISTS (
    group    = 'org.duckdb',
    artifact = 'duckdb_jdbc',
    version  = '1.1.3',
    included = true,
  );
```

### Update the version

```sql
ALTER MODULE MyModule
  SET JAR DEPENDENCY 'org.duckdb:duckdb_jdbc' VERSION '1.2.0';
```

### Disable a dependency without removing it

```sql
ALTER MODULE MyModule
  SET JAR DEPENDENCY 'org.duckdb:duckdb_jdbc' INCLUDED false;
```

### Add a transitive exclusion

```sql
ALTER MODULE MyModule
  SET JAR DEPENDENCY 'org.duckdb:duckdb_jdbc'
    ADD EXCLUSION 'com.example:conflicting-lib';
```

### Remove a transitive exclusion

```sql
ALTER MODULE MyModule
  SET JAR DEPENDENCY 'org.duckdb:duckdb_jdbc'
    DROP EXCLUSION 'com.example:conflicting-lib';
```

### Remove a dependency entirely

```sql
ALTER MODULE MyModule
  DROP JAR DEPENDENCY 'org.duckdb:duckdb_jdbc';
```

### Chain multiple actions

```sql
ALTER MODULE MyModule
  ADD JAR DEPENDENCY (
    group    = 'com.fasterxml.jackson.core',
    artifact = 'jackson-databind',
    version  = '2.21.2',
    included = true,
  )
  ADD JAR DEPENDENCY (
    group    = 'com.fasterxml.jackson.core',
    artifact = 'jackson-core',
    version  = '2.21.2',
    included = true,
  );
```

## Notes

Writing the dependency updates the `.mpr` file immediately, but Gradle must
still resolve and download the JAR into `vendorlib/`. This happens automatically
when the project is opened in Studio Pro or built with `mxcli docker build`.

## See Also

[LIST / DESCRIBE JAR DEPENDENCY](list-describe-jar-dependency.md)
