# ALTER MODULE … SET

## Synopsis

    ALTER MODULE module_name SET ( Version: 'x.y.z', ModuleType: app | addon )

## Description

Changes a module's settings, the General tab of Studio Pro's *Module settings* dialog. It takes the same keys as [CREATE MODULE](create-module.md), and changes only the settings it names. The rest of the module's settings, including its *Module ID* and an imported module's checksum, keep their stored values. A statement that restates the stored settings writes nothing.

| Key | Values | Studio Pro field |
|---|---|---|
| `Version` | a SemVer string, `'2.3.1'` | Module version |
| `ModuleType` | `app` \| `addon` | Module type |

An unknown key, a key named twice, a version that is not SemVer, or a type other than `app`/`addon` is an error from both `check` and `exec`. `ModuleType: addon` needs Mendix 9.12 or later.

Making a module an add-on hides every element not marked *Usable* from the rest of the app: a reference from another module becomes CE1406. Run `mxcli check script.mdl -p app.mpr --references` to see which references that affects.

## Examples

```sql
mdl 1;
-- Bump the version before exporting the module
alter module Inventory set (Version: '2.4.0');

-- Make it an add-on module
alter module Inventory set (ModuleType: addon);

-- Both at once
alter module Inventory set (Version: '3.0.0', ModuleType: app);
```

## See Also

[CREATE MODULE](create-module.md), [ALTER MODULE … JAR DEPENDENCY](../jar-dependency/alter-module-jar-dependency.md)
