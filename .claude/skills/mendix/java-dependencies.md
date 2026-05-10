# Java Dependencies for Custom Java Actions

This skill explains how Maven / JAR dependencies are added to a Mendix module
when a custom Java action needs an external library (Jackson, Apache Commons,
HTTP clients, etc.), and what to do because **mxcli cannot perform this step**.

## When to Use This Skill

Use this skill **before** writing or modifying any custom Java action that
imports a non-Mendix package — i.e. anything outside `com.mendix.*`,
`java.*`, `javax.*`, or what is already present in the project's `vendorlib/`
or `userlib/` folders.

Concrete triggers:
- Adding a new `import` statement in a `javasource/<module>/**/*.java` file
  whose package is not already on the classpath.
- Compilation fails with `package X does not exist` or `cannot find symbol`
  pointing to a third-party class.
- Migrating a Java action that previously depended on a marketplace module
  whose JARs are no longer present.
- Updating an existing Java action to a newer library version.

If the new code only uses classes from packages already present in
`vendorlib/` JARs (run `ls vendorlib/`), no dependency change is needed —
the Gradle build picks those up automatically.

## Critical Constraint

**mxcli does NOT support adding Maven dependencies to a module.**

Verified surface area (all rejected by the parser):

| Tried | Result |
|---|---|
| `ALTER MODULE Mod ...` | Parse error — `ALTER MODULE` isn't a recognized statement |
| `CREATE OR MODIFY MODULE Mod (JarDependencies = [...])` | Parse error — `(` not allowed after module name |
| `CREATE OR MODIFY MODULE Mod DEPENDS ON / REQUIRES / INCLUDES / DEPENDENCY / JAR ...` | Parse error — no such keyword |
| `IMPORT JAR ... INTO Mod;` / `ADD DEPENDENCY ... TO Mod;` | Parse error — wrong grammar branch |
| `mxcli bson dump --type module-settings` | Not a supported BSON type |

The grammar has a hidden `moduleOption` rule, but its usable surface only
accepts `CREATE MODULE Name;` (no options). There is no MDL syntax to write
a `Projects$JarDependency` entry into a module's `Projects$ModuleSettings`
unit, and no `mxcli bson` command exposes module-settings BSON for editing.

## Why Studio Pro Is the Only Supported Path

Studio Pro adds a Maven dependency by:

1. Writing a `Projects$JarDependency` (`{GroupId, ArtifactId, Version,
   Exclusions, IsIncluded}`) into the module's `Projects$ModuleSettings.JarDependencies[]`
   array (BSON in `mprcontents/.../*.mxunit`).
2. Triggering Gradle to resolve and download the JAR into `vendorlib/`,
   updating `vendorlib/vendorlib-sbom.json`.

Step 1 is what mxcli cannot do. Step 2 alone (dropping a JAR into `vendorlib/`
without the matching ModuleSettings entry) **does work for the build** —
`deployment/build.gradle` reads the compile classpath from
`fileTree(vendorlibDir) + fileTree(userlibDir)`. But it leaves the project
in an inconsistent state: Studio Pro's UI doesn't show the dependency,
Gradle won't manage updates, the SBOM can drift, and a future Studio Pro
"clean vendorlib" can wipe the JAR. Treat that as an emergency workaround
only when Studio Pro is unavailable, and always tell the user about it so
they can reconcile the module settings later.

## Workflow When a Java Action Needs a New Library

Follow this every time, in order:

### 1. Identify the dependency

Before writing code, decide on the exact Maven coordinate. **Pin a specific
version** — never tell the user "the latest" because Studio Pro's dialog
requires concrete `GroupId`, `ArtifactId`, `Version` fields.

Beware of namespace renames. The most common gotcha:

- Jackson 2.x → group `com.fasterxml.jackson.core`, packages `com.fasterxml.jackson.*`.
- Jackson 3.x → group `tools.jackson.core`, packages `tools.jackson.*`.

If the Java code imports `com.fasterxml.jackson.databind.*`, you need
**Jackson 2.x**, not 3.x — even though 3.x is newer, the package paths don't
match.

Match the bundled runtime version when one is available. Mendix runtimes
ship many common libraries inside
`~/.mxcli/mxbuild/<runtime-version>/runtime/bundles/`; pinning to the same
version that the runtime bundles avoids classloader surprises at runtime.
Inspect that directory with `ls ~/.mxcli/mxbuild/<runtime-version>/runtime/bundles/ | grep <library>`
to see what's already loaded by the runtime. Read the project's runtime
version from `.mendix-cache/base_ver` or from the `_MetaData._ProductVersion`
row of the `.mpr` SQLite.

### 2. Write and validate the Java code

Implement the Java action as usual. Validate compilation locally with
`javac` against `vendorlib/*` and the Mendix runtime API JARs:

```bash
javac -cp "vendorlib/*:$HOME/.mxcli/mxbuild/<runtime-version>/runtime/bundles/com.mendix.public-api.jar:$HOME/.mxcli/mxbuild/<runtime-version>/runtime/bundles/com.mendix.json.jar:$HOME/.mxcli/mxbuild/<runtime-version>/runtime/bundles/com.mendix.logging-api.jar" \
  -d /tmp/javacheck \
  javasource/<module>/**/*.java
```

If `vendorlib/` is missing the JAR you need, fail-fast here rather than
shipping uncompilable code to the user.

### 3. Update `.classpath`

Add a `<classpathentry kind="lib" path="vendorlib/<artifactId>-<version>.jar"/>`
line for each new JAR so Eclipse and `mx check` see them. Place new
entries alphabetically inside the existing `vendorlib/...` block; do not
disturb the runtime-API entries above.

### 4. Prompt the user — every single time

**This step is mandatory.** When a Java action requires a dependency mxcli
cannot register, you MUST stop after finishing your work and tell the user
exactly what to add in Studio Pro. The build may pass locally because
`vendorlib/` is sufficient for compile, but the project state is
inconsistent until the user reconciles it.

Use this template — fill in the bracketed fields with the specific
coordinate you pinned in step 1, and list every artifact (databind +
core + annotations etc., not just the top-level one):

> **Studio Pro action required**
>
> mxcli cannot add Maven dependencies to a module. Please open the project
> in Studio Pro and add the following dependency to the **`<ModuleName>`**
> module:
>
> 1. Right-click the **`<ModuleName>`** module → **Settings** → **Java
>    dependencies** tab.
> 2. Click **Add** and enter:
>    - **Group ID:** `<groupId>`
>    - **Artifact ID:** `<artifactId>`
>    - **Version:** `<version>`
> 3. Repeat for transitive coordinates if needed:
>    - `<groupId>:<artifactId2>:<version>`
>    - `<groupId>:<artifactId3>:<version>`
> 4. Click **OK** and let Gradle resolve. Studio Pro will populate
>    `vendorlib/` and update `vendorlib/vendorlib-sbom.json`.
>
> I have already placed compatible JARs in `vendorlib/` and added them to
> `.classpath` so the build works now, but the module settings won't reflect
> these dependencies until you complete the steps above.

Do not collapse this into a one-liner like "add Jackson via Studio Pro" — the
user needs the literal coordinates. The dialog has separate fields, not a
single GAV string.

## Worked Example: Adding Jackson Databind to a Module

Suppose Java actions in a module named `MyModule` import
`com.fasterxml.jackson.databind.JsonNode` and `ObjectMapper`. Walk through:

1. **Identify** — these imports are Jackson 2.x. Check
   `~/.mxcli/mxbuild/<runtime-version>/runtime/bundles/` for
   `com.fasterxml.jackson.core.jackson-databind.*.jar`. If the runtime ships,
   for example, `2.21.2`, pin to that.
2. **Locally bridge the build** — copy the matching JARs from the runtime
   bundles into `vendorlib/`:
   ```bash
   cp ~/.mxcli/mxbuild/<runtime-version>/runtime/bundles/com.fasterxml.jackson.core.jackson-databind.2.21.2.jar vendorlib/jackson-databind-2.21.2.jar
   cp ~/.mxcli/mxbuild/<runtime-version>/runtime/bundles/com.fasterxml.jackson.core.jackson-core.2.21.2.jar    vendorlib/jackson-core-2.21.2.jar
   cp ~/.mxcli/mxbuild/<runtime-version>/runtime/bundles/com.fasterxml.jackson.core.jackson-annotations.2.21.jar vendorlib/jackson-annotations-2.21.jar
   ```
3. **Update `.classpath`** — add three matching `<classpathentry kind="lib" .../>`
   lines.
4. **Verify** — `mx check App.mpr` reports 0 errors and a direct `javac` of
   the affected files succeeds.
5. **Prompt the user** with this concrete block:

   > **Studio Pro action required**
   >
   > mxcli cannot add Maven dependencies to a module. Please open the project
   > in Studio Pro and add these to the **`MyModule`** module:
   >
   > 1. Right-click **`MyModule`** → **Settings** → **Java dependencies**.
   > 2. Click **Add** and enter each of the following:
   >    - **Group ID:** `com.fasterxml.jackson.core` · **Artifact ID:** `jackson-databind` · **Version:** `2.21.2`
   >    - **Group ID:** `com.fasterxml.jackson.core` · **Artifact ID:** `jackson-core` · **Version:** `2.21.2`
   >    - **Group ID:** `com.fasterxml.jackson.core` · **Artifact ID:** `jackson-annotations` · **Version:** `2.21`
   > 3. Click **OK** and let Gradle resolve.
   >
   > I have already placed compatible JARs in `vendorlib/` and added them
   > to `.classpath` so the build works now, but the module settings won't
   > reflect these dependencies until you complete the steps above.

## Common Library → Coordinate Cheatsheet

Pin a known-good version per project; this list is a starting point only.

| Need | Group ID | Artifact ID | Notes |
|---|---|---|---|
| JSON (most common) | `com.fasterxml.jackson.core` | `jackson-databind` | Pulls `jackson-core` + `jackson-annotations`; pin all three. **Use 2.x for `com.fasterxml.jackson.*` imports**, 3.x only if you migrate code to `tools.jackson.*` |
| HTTP client (modern) | (none — use `java.net.http.HttpClient` from JDK 17+) | — | No dependency needed |
| HTTP client (Apache) | `org.apache.httpcomponents.client5` | `httpclient5` | Heavy; prefer JDK `HttpClient` |
| String / collection utils | `org.apache.commons` | `commons-lang3`, `commons-text` | Often already present in `vendorlib/` from CommunityCommons |
| File / IO utils | `commons-io` | `commons-io` | Often already present |
| Logging | (use `com.mendix.logging-api`) | — | Already on the classpath; do not add SLF4J/log4j |
| YAML | `com.fasterxml.jackson.dataformat` | `jackson-dataformat-yaml` | Pin to same version as `jackson-databind` |
| CSV | `com.fasterxml.jackson.dataformat` | `jackson-dataformat-csv` | Pin to same version as `jackson-databind` |

Always check `vendorlib/` first with `ls vendorlib/` — many libraries arrive
as transitive dependencies of marketplace modules and don't need to be
added again.

## Validation Checklist

Before reporting the Java action work as done:

- [ ] `ls vendorlib/` shows the JAR(s) for the new dependency.
- [ ] `.classpath` lists each new JAR as a `<classpathentry kind="lib" .../>`.
- [ ] `javac` compiles the affected files cleanly against
      `vendorlib/* + Mendix runtime API JARs`.
- [ ] `mx check App.mpr` reports 0 errors.
- [ ] **The user has been prompted with the exact `GroupId / ArtifactId /
      Version` coordinates to add in Studio Pro Module Settings.**

If the last box is unchecked, the work is **not** done — you've left the
project in an inconsistent state where the build works only because of an
out-of-band JAR drop.

## Related Skills

- [java-actions.md](./java-actions.md) — authoring Java actions and
  calling them from MDL.
- [write-microflows.md](./write-microflows.md) — invoking Java actions
  from microflows (`call java action ...`).
