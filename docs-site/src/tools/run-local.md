# mxcli run --local

`mxcli run --local` runs a Mendix app in a **warm, Docker-free dev loop**. It keeps
a `mxbuild --serve` process and a standalone Mendix runtime hot, so after the first
(cold) build a model change rebuilds incrementally and is hot-applied — without a
full rebuild-image-and-restart cycle.

```bash
mxcli run --local -p app.mpr
mxcli run --local -p app.mpr --watch
```

## Why

The Docker path (`mxcli docker run`) rebuilds a full deployment package and restarts
the container on every change (~30–60 s). `run --local` instead:

- keeps the model loaded in `mxbuild --serve` — a warm rebuild is ~1 s;
- keeps the runtime process up and applies each change over the M2EE admin API;
- chooses the cheapest apply automatically from the build's `restartRequired` flag:

| Change | Apply | Cost |
|--------|-------|------|
| page / microflow / nanoflow / text | hot `reload_model` (no restart) | ~1 s |
| entity / view entity / association | runtime restart + DDL | ~9 s |

The metamodel catalog (entities/associations) is reconciled only at runtime startup,
so structural changes need a restart; behavioural changes do not.

## What it does

1. Detects the project's Mendix version.
2. Ensures MxBuild and the runtime are cached (downloads once, reused after).
3. Checks the database is reachable (it does **not** provision it — see below).
4. Starts `mxbuild --serve` and does the first (cold) build into `deployment/`.
5. Bundles the browser client (`web/dist/`) with mxbuild's rollup tooling — the
   serve Deploy target writes client *source* but not the bundle, so this step is
   what makes pages render.
6. Boots a standalone runtime against that deployment and serves it.
7. With `--watch`, rebuilds, re-bundles the client, and hot-applies on every
   project change until `Ctrl-C`.

## Requirements

- **Mendix 11.x** project. The runtime is launched under **JDK 21**; version-aware
  JDK selection for Mendix 9/10 is a follow-up.
- A **PostgreSQL** database. Defaults: `127.0.0.1:5432`, user `mendix`, database
  derived from the project file name (`App1112.mpr` → `app1112`). Two ways to have it:
  - **`--ensure-db`** (recommended for a fresh session) provisions it: starts the
    local Postgres service if the port is down, and creates the app role + database
    if missing (via a local `sudo -u postgres` superuser). For a non-local `--db-host`
    it only verifies reachability — mxcli won't provision a remote database.
  - Otherwise create it once yourself; without `--ensure-db`, `run --local` stops with
    an actionable message if the DB is unreachable:

    ```bash
    createdb -h 127.0.0.1 -U mendix app1112
    ```

## Flags

| Flag | Default | Purpose |
|------|---------|---------|
| `--local` | — | Required; run without Docker |
| `--watch` | off | Rebuild + hot-apply on every project change |
| `--ensure-db` | off | Provision local Postgres + the app database if missing (fresh-session bootstrap) |
| `--setup` | off | Prepare prerequisites (cache MxBuild+runtime, ensure DB) and exit without booting — for a SessionStart hook |
| `--app-port` | 8080 | App HTTP port |
| `--admin-port` | 8090 | M2EE admin API port |
| `--serve-port` | 6543 | `mxbuild --serve` port |
| `--db-host` | 127.0.0.1:5432 | Database `host:port` |
| `--db-name` | derived from project | Database name |
| `--db-user` / `--db-password` | mendix / mendix | Database credentials |
| `--screenshot` | off | Capture a Playwright PNG after boot and each applied change |
| `--screenshot-path` | `<projectDir>/.mxcli/run-local.png` | Screenshot output PNG |
| `--screenshot-url` | app root | Page to shoot: full URL, or a path relative to the app root (e.g. `/p/customers`). Repeat for a multi-page set. |
| `--screenshot-user` / `--screenshot-password` | — | Log in once (Mendix form auth) and reuse the session, so pages behind login render authenticated |

## The change signal

`--watch` watches the model **source** — the `.mpr` file and the `mprcontents/`
document tree (v2) — not the whole project dir. This is deliberate: the serve/mxbuild
build rewrites `deployment/`, `theme-cache/`, and `.mendix-cache/` on every run, and
screenshots land in `.mxcli/`; watching only the source keeps that build-output churn
from re-triggering the loop. The intended cycle: an agent (or you) edits the model
with `mxcli exec`/MDL, and the running `run --local` picks it up and hot-applies it.

## Pages render in the browser

`run --local` bundles the browser client (`web/dist/`) so the app renders in a real
browser — verified by driving the pre-installed Chromium with Playwright (the Mendix
homepage renders fully). This makes it usable for **visual page-design iteration**,
not just headless checks.

- **Non-`--watch`**: a one-shot rollup bundle after the deploy build (~7 s cold).
- **`--watch`**: a long-lived incremental bundler stays hot (the client-side mirror
  of `mxbuild --serve`), so a page/widget edit re-bundles in ~3–4 s. It runs with
  `CHOKIDAR_USEPOLLING` because inotify does not fire on container overlay
  filesystems — without it, change detection takes tens of seconds. The loop
  re-bundles **only when the edit touched client source**: a microflow/entity edit
  skips the bundle and just hot-reloads.

## Pixel-perfect page loop

Pass `--screenshot` and each applied change is captured to a PNG (default
`<projectDir>/.mxcli/run-local.png`) using Playwright's built-in `screenshot`
command (Chromium from `PLAYWRIGHT_BROWSERS_PATH` — no `playwright-cli` needed):

```bash
mxcli run --local -p app.mpr --watch --screenshot
# edit a page with mxcli exec/MDL -> auto rebuild -> re-bundle -> reload -> new PNG
```

**Deep links.** `--screenshot-url /p/customers` shoots a specific page instead of the
app root (a bare path is resolved against the app URL; a full `http(s)://…` is used
as-is).

**Multi-page sets.** Repeat `--screenshot-url` to shoot several pages after every
change — a visual-regression sheet. Each page gets its own PNG, named from the page
(`run-local-p-customers.png`, `run-local-home.png`):

```bash
mxcli run --local -p app.mpr --watch --screenshot \
  --screenshot-url / --screenshot-url /p/customers --screenshot-url /p/orders
```

**Pages behind login.** `--screenshot-user`/`--screenshot-password` log in once via
the Mendix login form (Playwright drives `#usernameInput`/`#passwordInput`/
`#loginButton`), save the session as a Playwright storage state, and reuse it for
every screenshot — so authenticated pages render. Login is best-effort: if no login
form appears (anonymous app) it proceeds unauthenticated.

```bash
mxcli run --local -p app.mpr --watch --screenshot \
  --screenshot-user demo_admin --screenshot-password '<pw>' \
  --screenshot-url /p/customer_overview
```

## Fresh sessions (Claude Code Web)

Background processes (Postgres, the JVM) are reaped on idle, so a resumed web session
needs to bring prerequisites back up. `mxcli init` emits a **SessionStart hook** into
`.claude/settings.json` that runs `./mxcli run --local --setup --ensure-db -p <app.mpr>`
on every session start — the non-blocking `--setup` mode caches MxBuild+runtime and
provisions the database, then exits, leaving the session ready to `run --local`.

To start from an **empty repo** on the web or an iPad, use the
[bootstrap prompt](bootstrap-prompt.md) instead of a GitHub template.

See also: [PROPOSAL_mxcli_dev_warm_loop](../../../docs/11-proposals/PROPOSAL_mxcli_dev_warm_loop.md),
[mxcli docker run](docker-run.md), [Playwright Testing](playwright.md).
