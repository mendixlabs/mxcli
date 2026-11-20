# mxcli docker check

The `mxcli docker check` command validates a Mendix project without building it. It runs the Mendix `mx check` tool against the project and reports errors, warnings, and deprecations.

## Usage

```bash
mxcli docker check -p app.mpr
```

## What It Does

1. **Resolves the `mx` binary** -- Takes the `mx` next to `--mxbuild-path`, else the first it finds on PATH, in a Studio Pro installation, or in the download cache at `~/.mxcli/mxbuild/<version>/modeler/mx`. It does not download one (see [Getting mx](#getting-mx))
2. **Runs `mx check`** -- Executes the Mendix project checker against a temporary copy of the project **as stored**, as a local process; despite the `docker` in its name, it does not use Docker
3. **Reports results** -- Outputs errors, warnings, and deprecations to the console
4. **Explains CE0463** -- Only when the check found a CE0463, re-checks the copy after `mx update-widgets` and labels each one (see [CE0463 and stale widget definitions](#ce0463-and-stale-widget-definitions))

If the stored project has errors, the command exits with a non-zero status code.

Before `mx check` it also warns, without failing, about two problems `mx check`
cannot see: an MPR v2 `ContentsHash` index that disagrees with the `.mxunit`
files (`mxcli fix hashes`), and git states that make Studio Pro 11.13 fail to
open the project. See [Working Outside Studio Pro](outside-studio-pro.md).

## Getting mx

`docker check` does not download MxBuild. With no `mx` to find it stops with
`mx not found` and names the command that sets one up:

```bash
# Once per Mendix version: download MxBuild (including mx) for the project's version
mxcli setup mxbuild -p app.mpr

# Then check
mxcli docker check -p app.mpr
```

The downloaded mxbuild is cached at `~/.mxcli/mxbuild/{version}/` for reuse.
`mxcli new` and `mxcli docker build` download it too. On macOS and Windows,
`setup mxbuild` prefers Studio Pro's bundled MxBuild, because the CDN only
publishes Linux binaries.

The `mx` found need not be the project's version: an `mx` on PATH, or any
cached version, is used when the project's own is not there. `mx check` accepts
another patch of the same minor; `mx update-widgets` does not and the check then
runs without it (see `mxcli docker check --help`). Pass `--mxbuild-path
~/.mxcli/mxbuild/<version>` to choose one.

## CE0463 and stale widget definitions

CE0463 ("The definition of this widget has changed") means a stored pluggable
widget instance does not match its widget package. Its commonest cause is
harmless to fix: the package was upgraded after the widget was placed (any
Marketplace or widget install does this), and Studio Pro's "Update all widgets"
resyncs it. But until it is resynced, MxBuild and `mxcli run --local` refuse
the project, so `docker check` reports it.

When the check of the stored project finds a CE0463, `docker check` runs
`mx update-widgets` on the same temporary copy, checks it again, and labels each
CE0463 of the stored project:

| Label | Meaning | Fix |
|---|---|---|
| `` stale widget definition — `mxcli fix widgets` fixes it `` | the resync clears it | `mxcli fix widgets -p app.mpr` updates the project's widgets and keeps MPR v2 |
| `persists after update-widgets` | the resync does not clear it: something in the stored widget is wrong in a way a resync does not repair (if mxcli wrote the widget, that is an mxcli defect worth reporting) | not a resync; see [Pluggable Widgets](../guides/pluggable-widgets.md) |

The verdict and the exit status are always the stored project's. The re-check
costs a second `mx check`, so it runs only when there is a CE0463 to explain.

`mx check` does not print the page a CE0463 is on, so widgets of the same name on
different pages print the same line; the labels count them (`(4×)`, or
`(2 of 3)` when a resync clears only some of them).

| Flag | Checks | Exit status | Labels |
|---|---|---|---|
| *(none)* | the project as stored | the stored project's | yes, when there is a CE0463 |
| `--no-update-widgets` | the project as stored | the stored project's | no: update-widgets never runs |
| `--update-widgets` | only a copy normalised by `mx update-widgets` first | the normalised copy's | no |

`--update-widgets` is how `docker check` behaved by default in earlier
releases. It reports 0 errors for a project whose only errors are stale widget
definitions, which then still fails to build: use it only when you mean to ask
"would this pass after `mxcli fix widgets`?". `--no-update-widgets` is kept for
scripts that pass it; its verdict is the default's.

If `mx update-widgets` fails during the re-check (it refuses a project whose
Mendix version is not exactly its own, `exit status 4`), the CE0463s are left
unlabelled and the output says so.

## Example Output

A clean project:

```
Using mx: /home/vscode/.mxcli/mxbuild/11.13.0/modeler/mx
Checking a temporary copy (mx writes into the project it checks; the project on disk is not changed).
Checking project /workspaces/app/app.mpr...
Checking app for errors...
The app contains: 0 errors.
Project check passed.
```

A project whose Data grid 2 package was upgraded after its grids were placed
(measured on Mendix 11.13.0; the long CE0463 message is shortened here):

```
Checking project /workspaces/app/app.mpr...
Checking app for errors...
[error] [CE0463] "The definition of this widget has changed. ..." at Data grid 2 'dataGrid2_1'
[error] [CE0463] "The definition of this widget has changed. ..." at Data grid 2 'dataGrid21'
...
The app contains: 11 errors.

The stored project has 11 CE0463 errors. Re-checking the temporary copy after
  `mx update-widgets` to tell stale widget definitions from other causes...
CE0463 in the stored project:
  at Data grid 2 'dataGrid2_1' (4×): stale widget definition — `mxcli fix widgets` fixes it
  at Data grid 2 'dataGrid21' (4×): stale widget definition — `mxcli fix widgets` fixes it
  at Data grid 2 'dataGrid2_2' (2×): stale widget definition — `mxcli fix widgets` fixes it
  at Data grid 2 'dataGrid2_3': stale widget definition — `mxcli fix widgets` fixes it
All 11 are stale widget definitions. This updates them in the project, as Studio
  Pro's "Update all widgets" does:
    mxcli fix widgets -p /workspaces/app/app.mpr
As stored: 11 errors. After update-widgets: 0 errors.
Error: project check failed: exit status 1
```

## When to Use

`mxcli docker check` is faster than a full build (`mxcli docker build`) because it only validates the project structure without compiling or packaging. This makes it useful for:

- **Quick validation** after making MDL changes
- **Pre-build gate** -- `mxcli docker build` runs the same check automatically before building, on the project as stored (use `--skip-check` to bypass; `--update-widgets` checks and builds a widget-normalised copy instead)
- **CI pipelines** -- fail fast on invalid projects before spending time on a full build
- **Iterative development** -- check after each MDL script execution to catch errors early

## Integration with mxcli docker build

The `mxcli docker build` command runs `mx check` automatically as a pre-build step. If the check fails, the build is aborted. You can skip this with:

```bash
mxcli docker build -p app.mpr --skip-check
```

## Integration with the TUI

When using mxcli in interactive REPL mode, the TUI can auto-check the project on file changes, giving immediate feedback on whether MDL modifications introduced errors. Like `docker check`, it runs `mx check` on a temporary copy, so the project's `theme-cache/` and `deployment/` are not touched; it does not run update-widgets, so it checks the project as stored. `mxcli eval`'s `mx_check` check does the same.

## Related Pages

- [Docker Build](docker-build.md) -- Building a Mendix application with PAD patching
- [Docker Run](docker-run.md) -- Building and running the app in Docker
- [Testing](testing.md) -- MDL test framework
- [Linting](linting.md) -- MDL lint rules
