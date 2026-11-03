# CREATE MODULE

## Synopsis

    CREATE [OR MODIFY] MODULE module_name [ ( Version: 'x.y.z', ModuleType: app | addon ) ]

## Description

Creates a new module in the Mendix project. Modules are the top-level organizational unit for all project documents including entities, microflows, pages, and enumerations. The module is created with an empty domain model and Studio Pro's defaults for a new module: an **app module**, version **1.0.0**.

The optional property list sets the module's settings, the General tab of Studio Pro's *Module settings* dialog. `CREATE OR MODIFY` applies them to a module that already exists, changing only the settings it names. A plain `CREATE` of an existing module leaves it unchanged. To change one setting, use [ALTER MODULE … SET](alter-module-set.md).

## Parameters

**module_name**
: The name of the module to create. Must be a valid identifier (letters, digits, underscores; cannot start with a digit). If the name conflicts with a reserved keyword, enclose it in double quotes.

**Version**
: The *Module version*, a SemVer string `'major.minor.patch'` with an optional pre-release or build suffix (`'1.0.0-rc.1'`). It is the version an exported `.mpk` carries. Anything else is refused, as the dialog refuses it.

**ModuleType**
: `app` (an *App module*, the default) or `addon` (an *Add-on module*). An add-on hides every element not marked *Usable* from the rest of the app: referencing one from another module is CE1406, which `check -p --references` reports. Add-on modules need Mendix 9.12 or later. Solution modules have no MDL spelling.

## Settings shown by DESCRIBE MODULE

`DESCRIBE MODULE` prints the module's `CREATE OR MODIFY` statement with the settings that differ from a new module's, so its output re-runs:

```sql
create or modify module Inventory (
  Version: '2.3.1',
  ModuleType: addon,
);
-- location: Marketplace modules (set in Studio Pro)
-- add-on module: elements not marked Usable cannot be referenced from other modules
```

The *Location* (Module list or Marketplace modules) is printed as a comment when it is *Marketplace modules*; it cannot be set from MDL yet. The *Module ID* and the checksum of an imported module are not shown: Studio Pro owns them, and setting a module's settings keeps them.

`LIST MODULES` shows each module's version and type in its `Version` and `Type` columns.

## Examples

### Create a module

```sql
CREATE MODULE OrderManagement;
```

### Create multiple modules for a layered architecture

```sql
mdl 1;
CREATE MODULE Sales;
CREATE MODULE Inventory;
CREATE MODULE Reporting;
```

### Create a versioned add-on module

```sql
mdl 1;
create or modify module Inventory (
  Version: '2.3.1',
  ModuleType: addon,
);
```

## See Also

[ALTER MODULE … SET](alter-module-set.md), [CREATE FOLDER](create-folder.md), [MOVE](move.md), [DROP MODULE](../domain-model/drop-entity.md)
