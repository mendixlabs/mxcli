# Marketplace Content

How to discover, download, and install Mendix Marketplace content (modules, widgets) from the command line with `mxcli marketplace`.

## Authenticate first

Marketplace access needs a Mendix **Personal Access Token (PAT)**. Create one at
[user-settings.mendix.com](https://user-settings.mendix.com/) (Developer Settings → Personal Access Tokens), then:

```bash
mxcli auth login                 # prompts for the PAT
# or, non-interactively (CI):
mxcli auth login --token <PAT>
# or via the environment:
export MENDIX_PAT=<PAT>

mxcli auth status                # confirm it validates
```

Credentials are stored at `~/.mxcli/auth.json` (mode `0600`).

## Discover content

```bash
# search by name/publisher
mxcli marketplace search "database connector"

# show one item's details (by content id)
mxcli marketplace info 2888

# list available versions, optionally filtered by Mendix compatibility
mxcli marketplace versions 2888
mxcli marketplace versions 2888 --min-mendix 10.24.0
```

Each item has a numeric **content id** (shown by `search`/`info`); you pass it to `download` and `install`.

> **Search caching.** The marketplace Content API has no server-side search, so the first `search` fetches the full catalog listing and caches it under `~/.mxcli/marketplace-catalog-<profile>.json` for a day. After that, searches (for any keyword) are served from the cache instantly. Pass `--refresh` to bypass the cache and re-fetch (e.g. to pick up a brand-new module).

## Download a `.mpk`

```bash
# latest version, into the current directory under its CDN filename
mxcli marketplace download 2888

# a specific version, to a chosen path
mxcli marketplace download 2888 --version 7.0.2 -o ./mods/dbc.mpk
```

The download is atomic (written to a temp file and renamed), so a cancelled run never leaves a truncated `.mpk`.

## Install into a project

`install` downloads the content and places it according to its type:

```bash
mxcli marketplace install 20 -p app.mpr               # a widget
mxcli marketplace install 2888 --version 7.0.3 -p app.mpr   # a module
```

| Content type | What `install` does |
|---|---|
| **Widget** | Copies the `.mpk` into the project's `widgets/` folder (overwrites on update). Reload in Studio Pro or run `mx update-widgets` to pick it up. |
| **Module** (new) | Copies the module in with mxcli's own writer, preserving the project's MPR format, plus everything the package ships (widgets, themesource, ...). Requires a matching mxbuild — run `mxcli setup mxbuild -p app.mpr` if missing. |
| **Module** (already present) | **Reported, not modified** — see below. |
| Theme / Starter App / Sample | Downloaded to disk with import instructions (import via Studio Pro). |

### Installing a `.mpk` you already have (`--file`)

Not every package comes from the marketplace: an internal or company-standard module handed
over as a file, a theme module, a package fetched earlier with `download`, or a CI job with no
token. `--file` installs such a package through the same writer as an online install, with no
marketplace lookup and no PAT:

```bash
mxcli marketplace install --file ./CompanyTheme.mpk -p app.mpr
```

The kind is read from the package's own `package.xml` — a module goes through the transplant
writer (storage format preserved, theme modules included), a widget lands in `widgets/`.
Anything else is refused rather than guessed at. A module installed this way carries **no
marketplace version stamp**, because it has none: `update` and `diff` will report that no
marketplace content is installed under it, which is the truth.

The reference project the writer builds is still created with `mx` at the project's Mendix
version, so a package that `mx` itself cannot open — one exported from a Studio Pro further back
than the version window `mx module-import` accepts — is refused with `mx`'s own message. Convert
it once with `mx convert` on a toolset whose window covers it, then `--file` the result.

### The latest version is often not installable

New releases are published against the newest Studio Pro patch within days of it shipping, and `install` with no `--version` resolves to the latest — so on a project that is not on the very newest patch, the default is routinely the one version that cannot be imported. Measured on an 11.12.1 project: the latest release of all six agent-editor stack modules required 11.12.2, published five days earlier.

`install` and `update` check the version's published minimum before downloading anything, and name the version to use instead:

```text
Agent Commons 4.2.0 requires Mendix 11.12.2, and the project is 11.12.1
  hint: install --version 4.1.0 (the newest release built for 11.12.1 or older)
```

`mxcli marketplace versions <id>` shows the same information as a `MIN MENDIX` column.

### Dependencies are not resolved

`install` installs exactly the content you name. Its dependencies are neither fetched nor named — read the check errors after each install, which identify what is missing by qualified name. The error count is **not monotonic**, because each new module brings its own unmet dependencies: installing the agent-editor stack into a vanilla 11.12.1 app went 0 → 15 → 0 → 18 → 1 → 22 → 1 → 1. Dependencies include widget content as well as modules (`CE0462 "Could not find widget ..."`).

## Exporting a module (`marketplace package`)

The reverse of `install --file`: write a module of a project out as a `.mpk`, for a company-standard
module, a CI artifact, or to move a module to another app.

```bash
mxcli marketplace package MyModule -p app.mpr                 # ./MyModule.mpk
mxcli marketplace package MyModule -p app.mpr -o ./dist --json
```

It wraps Mendix's own `mx create-module-package` at the project's Mendix version, so the package is
the one Studio Pro's *Export module package* makes. The module name is checked against the project
first (a typo lists the modules instead of failing with `mx` exit code 3), and `mx`'s exit codes 1–4
are reported with what they mean and what `mx` printed. Whatever `mx` prints on a successful export
goes to stderr, so `--json` output stays clean.

**The `mx` must be the project's Mendix version.** `mx` is found from an `mx` on PATH first, then a
Studio Pro install, then the `mxbuild` cache, falling back to any cached one when the project's version
is missing, so the first match can belong to another Mendix version than the app. mxcli reads the
version of the `mx` it found (from the `mx.deps.json` beside it, else its version-named folder),
compares major.minor.patch with the project's, and **refuses** a mismatch — or an `mx` whose version
cannot be told — naming both versions. The fix is `mxcli setup mxbuild --version <version>` (take an
older `mx` off PATH, which is used before it). `--mxbuild-path` is the deliberate override: with it
a mismatch only warns on stderr and the export goes on. A path that does not exist is an error.

| Flag | Purpose |
|---|---|
| `-p, --project` | The project holding the module (required). |
| `-o, --output-dir` | Where `<Module>.mpk` goes (default: the current directory; created if missing). |
| `--all-libs` | Bundle every `userlib/` JAR. By default only the module's own required libs go in (`mx -l`): 12 KB against 24 MB on an app with Business Events installed. |
| `--exclude <regex>` | Leave out files whose path inside the package matches. `mx` takes **one** expression, so join alternatives with `\|`; write the folder separator as `.`. |
| `--exclude-managed-dependencies` | Leave out managed dependencies (`mx -m`). |
| `--mxbuild-path` | Take `mx` from this Mendix installation (or `mxbuild`) instead of resolving it by the project's version. Overrides the version check: a mismatch warns instead of refusing. |
| `--json` | Print `{path, sizeBytes, files, foreignEntries}`. |

**Files of other modules.** `-l` filters only `userlib/` JARs. The app's whole `resources/` folder is
still exported (measured on 11.12.5: another module's `resources/EventLog/` came along; its `javasource/` and
`themesource/` did not). After the export mxcli opens the package and warns on stderr about every `resources/<X>/…`, `javasource/<x>/…` or
`themesource/<x>/…` entry where `X` is not the exported module (the last two are a defensive check),
and prints the pattern that drops them:

```text
warning: the package holds 1 file(s) that belong to other modules, not to MyModule:
  resources/EventLog/x.txt
To leave them out, export again with:
  --exclude '^resources.EventLog.'
```

The pattern is quoted for a POSIX shell (a `'` in your own `--exclude` is written `'''`), so it can be
pasted as it stands; in PowerShell or `cmd` quote it for that shell.

The pattern uses `.` for the separator because a character class or a backslash (`[/\]`) makes `mx`
print its usage and write nothing. `.` matches any character, so a pattern that would also match a file
of the module itself is not suggested.

**The project is not changed.** `mx create-module-package` writes into the project it is given:
measured on a blank 11.12.5 app, one run rewrote `theme-cache/web/theme.compiled.css` and its map,
added `deployment/sass/main.scss`, and created the module's `themesource/<module>/` folder; the model
files were untouched. mxcli therefore runs it on a temporary copy, as `docker check` does, and a
before/after listing of the project (sizes and modification times) was identical. An output directory
inside the project (`-o ./dist`) is left out of that copy, so earlier packages are not copied along.

The package installs back with `marketplace install --file`; the round trip into a fresh 11.12.5 app
checks with 0 errors.

## Why installs do not use `mx module-import`

`mx module-import` rewrites an MPR v2 project as v1. Measured on a blank Mendix 11.12.1 app, a single import turned a 69 KB `.mpr` plus 341 `.mxunit` files into one 14 MB SQLite blob with no `mprcontents/` — and the same was observed independently on 11.13.0, so it is not version-specific:

```text
before   .mpr     69,632 bytes  +  341 .mxunit   tables: Unit, _MetaData, _Transaction
after    .mpr 14,295,040 bytes  +    0 .mxunit   tables: Unit, _MetaData
```

That is not cosmetic. The v2 layout is what makes the model diffable and mergeable per document: it is what [`mxcli diff-local`](../tools/diff.md) reads, and what makes an idempotent re-run observable as "no files changed". The conversion is **one-way** — `mx convert` targets Mendix *versions*, not storage formats.

So `install` copies the module's units directly instead, which keeps the project in whatever format it already uses and also works for theme modules (`module-import` refuses those outright). Measured: CommunityCommons 11.5.1 into a vanilla 11.12.1 app — 128 units and 126 bundled files, `mprcontents/` grew from 369 to 497 `.mxunit` files, and `mx check` reports 0 errors.

`--allow-format-change` selects the legacy `module-import` path, which still refuses to run silently:

```text
refusing to import: app.mpr uses the MPR v2 storage format, and 'mx module-import'
would rewrite it as v1.
...
  - Import the module in Studio Pro, which preserves the format; or
  - pass --allow-format-change to accept the conversion to MPR v1.
```

If you take that route the command states plainly that the project is now v1.

## Updating an existing module

Updating a module that is **already in the project is not done by `install`**. `install` detects it, reports the installed and target versions, names the command that does the update, and stops:

```text
Module "DatabaseConnector" is already installed (version 7.0.1).
Target: version 7.0.3.
It is not updated by install (an update can discard local edits). To update it,
review what would change, then apply it with entity identity and role grants preserved:
  mxcli marketplace diff 2888 -p app.mpr --to 7.0.3
  mxcli marketplace update 2888 -p app.mpr --to 7.0.3
```

Before you update, the question worth answering is **whether anyone has edited the module since it was installed** — because the update will not ask. That is what `marketplace diff` is for; see below.

Two reasons make automatic in-place module updates unsafe:

1. **Local edits.** Teams sometimes modify a marketplace module after importing it; a blind re-import would discard those changes.
2. **Persistent-entity IDs.** A fresh import assigns new entity `$ID`s. The runtime database keys data by entity ID, so re-importing a module with persistent entities would make the runtime treat them as *different* entities — **losing data**.

Studio Pro's Marketplace **Update** performs an ID-preserving merge that the `mx` CLI does not expose; `marketplace update` (below) is mxcli's equivalent, as a separate command so that `install` never updates implicitly.

## Updating a module (`marketplace update`)

`update` replaces an installed module with another published version, preserving the two things a plain replace destroys.

```bash
# Refuses if you have edited the module, naming what it would discard
mxcli marketplace update 23513 -p app.mpr --to 4.5.0

# Park those edits as re-executable MDL, then update over them
mxcli marketplace update 23513 -p app.mpr --to 4.5.0 --save-edits ./local-edits
mxcli marketplace update 23513 -p app.mpr --to 4.5.0 --force
mxcli exec ./local-edits/entity-Account.mdl -p app.mpr
```

```text
Administration updated 4.3.2 → 4.5.0
  28 units copied, 9 element identities preserved, 2 role grant(s) restored.

  Removed in 4.5.0 (1) — their database columns or tables will go on the next deploy:
    Account/MyLocalEdit
```

### What it preserves, and why

- **Element identity.** The runtime keys entities and their attributes on the model's `GUID`, so a module whose documents are replaced without carrying the old GUIDs is a *different* module to the database and its tables are dropped on the next deploy. Studio Pro transplants them; so does this.
- **Access.** A user role's grant of a module role lives in the project's security document, not the module, so removing the module takes the grants with it and putting it back does not return them.
- **Everything else the package ships.** A module is not only its model: the `.mpk` carries widget binaries under `widgets/`, styling and design-property declarations under `themesource/`, and whatever else it needs. All of it is replaced — only `project.mpr` and `package.xml` are manifest rather than payload. DataWidgets 3.11.3 replaces 49 such files, and skipping them leaves the app running old widget code and reporting `CE6083` for design properties the module itself declares.

It does not use `mx module-import`, which would rewrite an MPR v2 project as v1 and refuses theme modules. Units are copied with mxcli's own writer, so the project keeps its format.


### Local edits

Local edits are **not** preserved. `update` refuses when it finds any, `--save-edits` writes them out first, and `--force` proceeds. Two limits on the saved files:

- They are the element's **resulting state, not a diff**, so replaying restores additions and changes but not removals.
- An element that could not be described has nothing to save, and is reported rather than skipped.

### Afterwards

```bash
mxcli fix widgets -p app.mpr
mxcli fix design-properties -p app.mpr
mxcli docker check -p app.mpr
mxcli diff-local -p app.mpr
```

A headless install or update leaves two repairs for Mendix's own tools: **CE0463** (the project's stored widget instances are older than the widget packages beside them) and **CE6087** (a module references design properties an older Atlas spelled differently). Both are expected, not faults in the install. See [`mxcli fix`](#repairing-the-model-mxcli-fix) below.

A third repair is **not** left to you: **CE0066** ("Entity access is out of date"). A transplant copies the incoming module's units in verbatim, so a package whose access rules do not cover every member of their entities would land that error in your project with nothing said about it (mendixlabs/mxcli#1085). Install and update now reconcile the module's rules for themselves and report the count:

```
  3 entity access rule(s) reconciled — the package's rules did not cover every
  member of their entities, which Mendix reports as CE0066.
```

Nothing is written when the rules already match, so a well-formed package comes through untouched. When the line does appear, the module's domain model differs from the package — the same change Studio Pro's **Update security** button makes — and `mxcli marketplace diff` will read it as a local edit. To run the repair by hand, or on a project updated by an older mxcli, use [`UPDATE SECURITY`](../reference/security/update-security.md):

```bash
mxcli -p app.mpr -c "update security UserCommons"
```

Measured: Administration 4.3.2 → 4.5.0 and DataWidgets 3.5.0 → 3.11.3 both reach **0 errors** afterwards.

## Bundled widgets and install order

A module package carries a copy of every widget its pages use, pinned at the module author's release time — and different modules pin different versions of the same widget. Atlas_Web_Content 4.3.0 ships five Data Widgets at 3.4.0 that DataWidgets 3.11.3 ships at 3.11.3.

`install` and `update` keep the newer copy and say so:

```text
  Kept 5 newer widget(s) the package would have rolled back:
    widgets/com.mendix.widget.web.Datagrid.mpk — kept 3.11.3, package ships 3.4.0
```

Without this, module install order silently decided which widget versions the project ended up with, and nothing reported it — an out-of-date widget is not a check error. A package that ships the same widget both as a `.mpk` and as an unpacked tree (FeedbackModule 5.0.0) installs only the `.mpk`.

## When the installed version is no longer published

`diff` and `update` both download the installed version to establish the local-edit baseline, so both fail when it has been unpublished — as NanoflowCommons 6.0.0 has, while a blank 11.13 app still ships it. `--force` does not help: it overrides a finding, and there is no finding to override. `mxcli marketplace update … --no-baseline` accepts that the question cannot be answered and updates anyway, discarding any local edits to that module without naming them.

## Why `diff` and `update` are slow, and what is cached

Both commands answer their question by building a **reference project**: a blank
Mendix app of the consuming project's version with the published module imported
into it, so the comparison runs against a real project rather than against a
`.mpk`. That costs a download plus two `mx` invocations, and `update` builds
two references — the installed version, to answer "has anyone edited this?", and
the version being moved to.

Two caches under `~/.mxcli/marketplace-refs/` keep that off the clock:

| Cache | Keyed by | Holds | Saves |
|---|---|---|---|
| `blank/` | Mendix version | a whole blank app (~37 MB) | the `mx create-project` in **every** reference build |
| `ref/` | published version UUID + Mendix version | the model only (~14 MB) | the whole reference, on a repeat |

Measured on Administration (content 23513, 4.3.2 → 4.5.0, Mendix 11.12.1):

```text
mxcli marketplace diff 23513 -p app.mpr --to 4.5.0

  no cache      66s
  cold cache    47s     (blank app built once, reused by the second reference)
  warm           9s
```

A `ref/` entry stores the `.mpr` (and `mprcontents/`) and nothing else. A
reference project is a whole blank Mendix app, but only its model is ever read —
`widgets/`, `themesource/`, `theme-cache/` and `javascriptsource/` account for
20 MB of 34 MB and are never opened, because an update takes the module's
bundled files from the `.mpk` rather than from the reference. Storing the model
alone made entries 58% smaller and warm runs faster, since less is copied.

The Mendix version is part of both keys, because a reference built at a
different version reports Mendix's own conversions as local edits. An entry is
only used once its completion marker is present and the project's version stamp
has been re-checked on the way out, so a half-written or mislabelled entry is
rebuilt rather than trusted.

`ref/` keeps the 12 most recently used entries and evicts the rest — twelve is
what a six-module update sweep builds, so a whole sweep stays cached at ~170 MB.
It is bounded at all because running out of disk part way through an update is
worse than rebuilding an entry: `update` does not roll back, so a failed write
leaves the module already dropped. `blank/` is not bounded — one entry per
Mendix version.

```bash
MXCLI_REF_CACHE_MAX=20 mxcli marketplace diff …   # keep more (0 = keep everything)
MXCLI_NO_REF_CACHE=1   mxcli marketplace diff …   # build everything from scratch
```

Reach for `MXCLI_NO_REF_CACHE=1` before deleting anything: it answers "is this a
stale-cache problem?" while leaving the evidence in place.

Note that `--force` still builds the baseline. It is not wasted work — under
`--force` the command prints the locally changed elements it is about to
replace, and that list is the whole point of the check. Use `--no-baseline` when
you genuinely want to skip it.

## Repairing the model (`mxcli fix`)

`mx update-widgets` and `mx rename-design-properties` each fix something only Mendix can fix, and each rewrites an MPR v2 project into the single-file v1 format while doing it. Measured on 11.12.1: `update-widgets` took 369 `.mxunit` files to 0 and a 69,632-byte index to 14,405,632 bytes; `rename-design-properties` took 1,865 files to 0 and a 249,856-byte index to 39,895,040 bytes, having renamed 149 design properties across 41 documents. The conversion is one-way.

```bash
mxcli fix widgets -p app.mpr             # CE0463
mxcli fix design-properties -p app.mpr   # CE6087
```

`marketplace install` runs both itself after a module install (a widget install runs
the widget resync only) when `mx` for the project's version is available, and falls
back to printing these two commands with a warning when it is not.

```text
Updated design properties: 42 unit(s) changed.
  Storage: 1868 .mxunit file(s), unchanged from 1868 before (MPR v2 preserved).
```

Each runs the same Mendix tool, reads every unit back out of the converted file, restores the v2 storage, and writes the changed units into it through mxcli's own writer. The storage count is printed before and after because that is where the failure this exists to prevent would show up — as a zero.

Measured end to end on a vanilla 11.12.1 app carrying the agent-editor stack: `mx check` reported **203 errors** (202 × CE0463 + 1 × CE6087), and **0** after the two commands, with the project still MPR v2 (1,868 `.mxunit` files) — reproduced from a restored pre-fix snapshot.

Re-running is free: a second run reports 0 units changed, because [idempotent writes](../internals/idempotent-writes.md) elide a unit whose content did not really change. An MPR v1 project is passed straight through, since these tools write v1 natively.

| Command | Persists? | Use |
|---|---|---|
| `mxcli fix widgets` | **yes** | the fix — after any headless install |
| `mxcli fix design-properties` | **yes** | the fix — after any headless install |
| `mxcli docker check` | no | runs the widget resync on a temporary copy so the *check* is not tripped by CE0463; the stored model stays stale (`--no-update-widgets` checks it as stored) |
| `mxcli widget sync` | yes, partial | reconciles widget schemas in mxcli's own code; clears 7 of 40 on the reference fixture |

CE6087 is distinct from `CE6083`, which is a *missing* design-property declaration and is fixed by installing everything the package ships — something `install` and `update` already do.

`update` does **not** roll back. Work on a copy or have the project in version control.

## Has this module been edited? (`marketplace diff`)

Studio Pro's Marketplace **Update** replaces the module and discards local edits without asking. `marketplace diff` answers the question that decides whether that is safe:

```bash
# What have I changed in this module since installing it?
mxcli marketplace diff 23513 -p app.mpr
```

```text
Administration — installed 4.3.2 (Mendix 11.12.1)

  Locally modified (1 of 21 elements):
    changed   ENTITY Account
```

An untouched module reports how much was actually checked, not just a verdict:

```text
  No local modifications: 21 of 21 elements verified unchanged.
```

### What it does

1. Reads which marketplace version each module in the project records. The project stores the marketplace **version UUID** per module, so the module and the exact release it came from are both identified without guessing — the listing name does not help here (content 23513 is listed as "Administration module" and installs a module called `Administration`; "Data Widgets" installs `DataWidgets`).
2. Downloads that version's `.mpk` and imports it into a throwaway reference project built **at the project's own Mendix version**, so the package goes through the same conversion the installed copy did.
3. Describes every element of the module on both sides and compares the descriptions.

Comparison is on `DESCRIBE` output rather than raw storage: an *untouched* module differs from its own published package in thousands of BSON paths, because the installed copy carries subtrees the package does not.

Requires the mxbuild toolchain for the project's Mendix version — `mxcli setup mxbuild -p app.mpr`. Building the reference at a *different* version is refused rather than warned about, because Mendix's own conversions would then show up as your edits.

### Theme modules

`mx module-import` refuses a theme module outright ("Importing theme module is not supported"), which would take Atlas_Core, Atlas_Web_Content and Conversational UI off the table. The refusal is gated on a single flag on the module document inside the package, so `diff` clears it on **its own throwaway copy** before importing — the published package and your project are untouched.

Atlas modules are among the most-edited in real projects, so this matters more than the module count suggests:

```text
Atlas_Web_Content — installed 4.1.0 (Mendix 11.12.1)

  No local modifications found, but 46 of 89 elements could not be read —
  this is not a clean bill of health.

  Not comparable (46) — reported as unknown, never as unchanged:
    unknown   PAGE_TEMPLATE Blank (no DESCRIBE support for PAGE_TEMPLATE)
    ...
```

The 46 are page templates, which have no `DESCRIBE` handler yet. They are reported rather than quietly counted as unchanged — see [`CATALOG.PAGE_TEMPLATES`](../tools/catalog-tables.md).

### What an upgrade would touch

`--to` adds the other half of the question: what the module's author changed, and whether it collides with what you changed.

```bash
mxcli marketplace diff 23513 -p app.mpr --to 4.5.0
```

```text
  Upgrading to 4.5.0 would touch 5 element(s), 1 of which you have modified:
    CONFLICT  ENTITY Account

  Studio Pro's update would discard those local edits without asking.
```

### In CI

`--json` emits the machine-readable form, for a build gate that fails when a marketplace module has been edited:

```bash
mxcli marketplace diff 23513 -p app.mpr --json
```

```json
{
  "module": "Administration",
  "installedVersion": "4.3.2",
  "mendixVersion": "11.12.1",
  "locallyModified": true,
  "verified": true,
  "modified": ["ENTITY Account"],
  "unchangedCount": 20
}
```

Read **both** `locallyModified` and `verified`. `verified: false` means at least one element could not be described, so "no modifications found" is not a conclusion you can act on — an element that cannot be read is reported as `unknown`, never as unchanged:

```text
  No local modifications found, but 1 of 21 elements could not be read —
  this is not a clean bill of health.
```

### Flags

| Flag | Purpose |
|---|---|
| `-p, --project` | The project holding the installed module (required). |
| `--to <version>` | Also report what upgrading to this version would touch, and which of those you have modified. |
| `--module <name>` | Name the module explicitly, when the project records no marketplace version for it (a hand-imported copy) or several modules match. |
| `--json` | Emit JSON instead of text. |
