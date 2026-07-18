# Warm Local Dev Loop — `mxcli run --local`

## Overview

`mxcli run --local` runs a Mendix app **without Docker**, keeping a
`mxbuild --serve` process and a standalone runtime hot so model changes apply in
~1 s instead of a ~30–60 s rebuild-and-restart. Use it as the fast inner loop when
iterating on an app; use `mxcli docker run` when you need the fully-rendered browser
client (see the limitation below).

## When to Use This Skill

Use this when:
- You want the fastest edit → running-app loop for a Mendix 11.x project.
- You're driving the model programmatically (`mxcli exec`/MDL) and want each change
  live immediately.
- You're iterating on **page design** (the app renders in a real browser) or doing
  runtime/model/API/headless verification.

Prefer `mxcli docker run` when:
- The project is Mendix 9/10 (JDK 11/17 — not yet supported by `run --local`).
- You want a container-parity deployment rather than a standalone runtime.

## Usage

```bash
# boot once and keep serving (Ctrl-C to stop)
mxcli run --local -p app.mpr

# boot and hot-apply on every project change
mxcli run --local -p app.mpr --watch
```

## How apply is chosen

Every warm rebuild reports whether a restart is required; `run --local` applies the
cheapest action automatically:

| Change | Apply | Cost |
|--------|-------|------|
| page / microflow / nanoflow / text | hot `reload_model` (no restart) | ~1 s |
| entity / view entity / association | runtime restart + DDL | ~9 s |

Structural changes need a restart because the runtime reconciles its entity/
association catalog only at startup; behavioural changes are hot-reloaded.

## Prerequisites

- **Mendix 11.x** project (runtime launches under **JDK 21**).
- A **PostgreSQL** database (defaults: `127.0.0.1:5432`, user `mendix`, db derived
  from the project name; override with `--db-host/--db-name/--db-user/--db-password`).
  - **`--ensure-db`** provisions it for a fresh session: starts local Postgres if the
    port is down and creates the role + database if missing (local superuser via
    `sudo -u postgres`). Remote hosts are only checked, not provisioned.
  - Without `--ensure-db`, create it once and the command errors if it's unreachable:

    ```bash
    createdb -h 127.0.0.1 -U mendix "$(basename app.mpr .mpr | tr '[:upper:]' '[:lower:]')"
    ```

## The intended loop

```bash
# terminal 1: keep the app hot
mxcli run --local -p app.mpr --watch

# terminal 2 (or an agent): edit the model — the change hot-applies automatically
mxcli exec add-page.mdl -p app.mpr
```

`--watch` observes the project directory (MPR v1 and v2 layouts), ignoring
`deployment/`, `.git`, and `node_modules`.

## Flags

| Flag | Default | Purpose |
|------|---------|---------|
| `--local` | — | Required; run without Docker |
| `--watch` | off | Rebuild + hot-apply on each change |
| `--ensure-db` | off | Provision local Postgres + app database if missing |
| `--setup` | off | Cache MxBuild+runtime + ensure DB, then exit (SessionStart bring-up) |
| `--screenshot` | off | Playwright PNG after boot + each change |
| `--screenshot-path` / `--screenshot-url` | `.mxcli/run-local.png` / app root | Screenshot output / page (URL or `/path`) |
| `--screenshot-user` / `--screenshot-password` | — | Log in once, reuse session (pages behind login) |
| `--app-port` / `--admin-port` / `--serve-port` | 8080 / 8090 / 6543 | Ports |
| `--db-host` / `--db-name` / `--db-user` / `--db-password` | 127.0.0.1:5432 / derived / mendix / mendix | Database |

## Pages render in the browser

`run --local` bundles the browser client (`web/dist/`) with mxbuild's rollup tooling
after the deploy build, so the app renders in a real browser (verified with
Playwright + the devcontainer's Chromium).

- **`--watch`** keeps a long-lived incremental bundler hot (the client-side mirror of
  `mxbuild --serve`): a page/widget edit re-bundles in ~3–4 s; a microflow/entity edit
  skips the bundle and just hot-reloads. It uses `CHOKIDAR_USEPOLLING` because inotify
  is silent on container filesystems.
- Without `--watch`, a single one-shot bundle (~7 s) runs before boot.

## Pixel-perfect page loop

`--screenshot` captures a PNG (default `<projectDir>/.mxcli/run-local.png`) after boot
and after each applied change, via Playwright's built-in `screenshot` command
(Chromium from `PLAYWRIGHT_BROWSERS_PATH`):

```bash
mxcli run --local -p app.mpr --watch --screenshot
# edit a page -> auto rebuild -> re-bundle -> reload -> fresh screenshot
```

- `--screenshot-url /p/customers` shoots a specific page (bare path resolved against
  the app root; a full URL is used as-is). Repeat it for a multi-page set — each page
  gets its own PNG (`run-local-p-customers.png`, `run-local-home.png`).
- `--screenshot-user`/`--screenshot-password` log in once (Mendix form auth) and
  reuse the session, so pages behind login render authenticated. Best-effort: an
  anonymous app with no login form proceeds unauthenticated.

## Validation checklist

- [ ] Project is Mendix 11.x.
- [ ] Postgres is running and the target database exists.
- [ ] `curl -s -o /dev/null -w '%{http_code}' http://localhost:8080/` returns `200`,
      and `.../dist/index.js` also returns `200` (client bundle served).
- [ ] With `--watch`, editing a microflow logs `applied via reload`; adding an entity
      logs `applied via restart` and creates the table in Postgres.
