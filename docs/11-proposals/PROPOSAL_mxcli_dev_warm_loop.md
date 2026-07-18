---
title: warm dev loop — Docker-free run and iPad split-screen preview
status: draft
date: 2026-07-17
---

# Proposal: Warm dev loop (`mxbuild --serve` + `reload_model`)

**Status:** Draft
**Date:** 2026-07-17
**Relates to:** `PROPOSAL_check_mxbuild_gap_heuristics.md` (the static-check gate that
runs *before* this loop ever builds), `.claude/skills/mendix/docker-workflow.md`
(the loop this replaces for inner-loop iteration).

## Problem Statement

The current way to run and test a Mendix app with mxcli is `mxcli docker build` /
`mxcli docker run`. It has two costs that dominate the edit→test loop:

1. **Full build every time.** The one-shot `mxbuild` CLI recompiles the whole
   deployment on each invocation. Measured on a one-entity/one-page/one-microflow
   app: **~30–60 s per build** (Java compile + full model export + package).
2. **Docker.** The runtime and database run in containers, adding image pulls and
   container lifecycle to every cycle — and Docker is **unavailable** in important
   contexts: Claude Code on the web, and iPad. In those environments the current
   loop cannot run at all.

Two primitives, both verified to exist and work in Mendix **11.6.3+**, make a far
tighter loop possible:

- **`mxbuild --serve`** — an HTTP build server (default port 6543) that keeps the
  model loaded and rebuilds **incrementally**, skipping unchanged work (including
  Java compilation when no Java changed). Each build returns `restartRequired`.
- **`reload_model`** — a runtime admin-port action that swaps the model into the
  **running** JVM in-place, draining in-flight actions first (near-zero-downtime).
  No process restart.

This proposal wires those two primitives into mxcli to serve **two scenarios**:

- **Scenario A — a fast, Docker-free `mxcli dev` loop** to replace `mxcli docker`
  for inner-loop iteration.
- **Scenario B — an iPad split-screen workflow**: Claude Code on the web in one
  pane, a browser preview in the other, prompt → build → test, all on the iPad,
  **without committing to the repo** before testing.

## End-to-end workflow: Claude design prototype → secured production Mendix app (on iPad)

Scenario B is one leg of a larger journey this loop is meant to unlock — taking an
idea all the way to a **secured, production-grade Mendix app entirely on an iPad**,
with no desktop Studio Pro in the loop:

1. **Prototype in Claude.** A Claude design Artifact (a clickable HTML/React mockup)
   captures the app's screens, data, and flows — fast and throwaway, no Mendix yet.
2. **Translate to Mendix in Claude Code (web).** In the same iPad session, mxcli/MDL
   turns the prototype into a real Mendix model: `create entity` / `create page` /
   `create microflow` / navigation. The prototype's structure becomes actual `.mpr`
   documents — a running Mendix app, not a mockup.
3. **Secure it for production.** Raise the app from prototype to production security
   and make it pass the real Mendix consistency rules — the exact path proven during
   the investigation behind this proposal:
   - `ALTER PROJECT SECURITY LEVEL PRODUCTION` — required before `DTAPMode=P` will
     even boot (otherwise `start` returns `result:11 — security must be
     CHECKEVERYTHING`).
   - `grant` / `revoke` page, entity, and microflow access per module role; define
     user roles and their mappings; wire the login and anonymous navigation profiles.
   - `mxcli check` (instant) plus the warm `mxbuild` build surface the
     production-only errors up front (e.g. *"page needs an allowed role"*, which we
     hit and fixed with a single `grant`) instead of discovering them at deploy time.
4. **Test each change live, on the iPad.** The warm loop (§Proposed CLI) previews
   every edit in ~1 s in the Safari pane, under **real production security** (login
   enforced, anonymous blocked) — so the author iterates prototype → model → secured
   app → test without ever leaving the tablet.

The deliverable is not a throwaway preview but a **secured, production-representative
Mendix app** — the same artifact you would deploy. The warm dev loop is what makes
step 4 fast enough for this to be a genuine authoring experience rather than a batch
process, and Scenario B is what makes it work on a device that has neither Docker nor
Studio Pro.

## BSON Structure

Not applicable — this is a build/runtime orchestration feature. It touches no
Mendix document serialization. It drives two already-existing interfaces: the
mxbuild HTTP build API and the runtime M2EE admin API.

## Measured evidence

All numbers below were measured directly (Mendix 11.6.3, Linux x86-64, JDK 21) on
a one-entity/one-page/one-microflow app during the investigation that motivated
this proposal.

| Step | Time | Notes |
|------|------|-------|
| `mxbuild` one-shot CLI (full) | **~30–60 s** | current loop; no incremental mode |
| `mxbuild --serve`, cold (first build) | ~13.7 s | loads model into the server |
| `mxbuild --serve`, warm, **no change** | ~1.1 s | model cached |
| `mxbuild --serve`, warm, **microflow change** | **~0.8 s** | Java **not** recompiled |
| `mxbuild --serve`, warm, **entity/attribute change** | ~7.3 s | `restartRequired: true` |
| `reload_model` (hot, no restart) | **~0.07–0.27 s** | `"reload": true`, JVM pid unchanged |
| **Full warm loop** (exec → serve build → reload) | **~1 s** | microflow/page change, no restart |

`restartRequired` from the serve API cleanly separates the two change classes:

- **page / microflow change** → `restartRequired: false` → `reload_model` (~0.1 s)
- **entity / domain change** → `restartRequired: true` → `execute_ddl_commands`
  + runtime restart (the DDL gate is real and unavoidable — the runtime refuses a
  stale schema)

`mxbuild --serve` and `reload_model` are present in both **11.6.3** and **11.12.1**
(verified). The 11.12 CLI adds no build-skip flags (only `--export-secrets`), so the
incremental path is `--serve`, not a new one-shot flag.

## Hot-reload scope: what `reload_model` can and cannot do (verified)

`reload_model` reloads exactly three subsystems — the model store
(`ProjectModelStoreLoader.load`), the microflow engine
(`MicroflowEngineModule.reload`), and translations (`I18NProcessor.reload`) — after
draining in-flight actions. It does **not** touch the datastorage / entity layer.

The runtime maintains a **metamodel catalog inside the app database** —
`mendixsystem$entity`, `mendixsystem$entityidentifier`, `mendixsystem$attribute`,
`mendixsystem$association` — and reconciles it **at startup**, not during a reload.
This was verified directly: a view entity added via `reload_model` (`TaskView`)
returned `Success` but **never appeared in `mendixsystem$entity`**, while the base
`Task` (present at the last startup) has its row (`entity_name` → `table_name` → a
stable identifier GUID). So any change that needs a catalog row — a new/changed
entity, **including OQL view entities**, and associations in
`mendixsystem$association` (e.g. a view entity referencing a persistent entity) —
requires a **restart**, because that is when the catalog is reconciled.

This makes the apply-decision **two-dimensional**: `restartRequired` (from the serve
build) and `get_ddl_commands` (from the runtime) are **independent** signals.

| Change class | `restartRequired` | `get_ddl_commands` | Apply via |
|---|---|---|---|
| microflow / page / text | `false` | 0 | `reload_model` (~0.1 s, hot) |
| **OQL view entity** | **`true`** | **0** | **restart** — catalog sync, **no DDL** |
| persistable entity / attribute | `true` | > 0 | `execute_ddl_commands` → restart |
| association (adds FK) | `true` | > 0 | `execute_ddl_commands` → restart |

The loop must branch on **both** signals: `restartRequired == false` → `reload_model`;
`restartRequired == true` → restart, running `execute_ddl_commands` **only if**
`get_ddl_commands > 0`. Treating "restart" as implying "DDL" is wrong — **OQL view
entities are the counterexample**: they need a restart (to write the
`mendixsystem$entity` catalog row) but no DDL (they are query-time, not materialized —
confirmed: no view/table for `TaskView` in Postgres).

**Caveat (not fully closed):** `reload_model` *returns* `Success` for a view-entity
change (it doesn't reject it), and I could not run the functional tiebreaker —
querying `TaskView` post-reload — because the standalone hand-boot does not register
the `preview_execute_oql` admin action (it needs the dev-preview flag `mxcli docker`
sets). The conclusion rests on the authoritative `restartRequired` signal plus the
catalog evidence, which agree. Enabling the dev-preview flag on the standalone boot
(so the agent can verify via OQL) is itself a small item worth doing — see Open
Questions.

## Implementation status (slice 1 — `mxcli run --local`)

Shipped and verified end to end against a blank Mendix 11.12.1 app on a clean
Postgres: boot → HTTP 200; a microflow edit applies via `reload_model` in ~1.1 s (no
restart); an entity add applies via restart in ~9 s, runs `execute_ddl_commands`, and
returns HTTP 200 with the table present in Postgres. Pieces:
`docker.RuntimeController` (reload-vs-restart + DDL-aware start), `docker.LocalRuntime`
(standalone boot; constants lifted from `deployment/model/config.json`),
`docker.RunLocal` (+ `--watch`), and the `run --local` command.

**Browser client bundle — closed.** Initially a blocker: a browser loaded blank
because `index.html` requests `web/dist/index.js`, which neither the serve `Deploy`
target nor a one-shot `mxbuild` produces. The client bundle is built by a **rollup**
step (`web/rollup.config.mjs`, using mxbuild's bundled `modeler/tools/node` tooling)
that the serve/standalone path doesn't run — `web/index.js` (the rollup *input*) is
written but `web/dist/index.js` is not. Fixed by `docker.BuildWebClient`, which runs
mxbuild's `rollup-runner.mjs` (production, one-shot) over `deployment/web` after the
deploy build. Verified: driving the devcontainer's Chromium with Playwright renders
the Mendix homepage fully. `web/dist/index.js` now serves HTTP 200 out of the box.

**Incremental client bundler + screenshots — done.** `--watch` now keeps a
long-lived incremental bundler hot (`docker.WebClientWatcher`) — the client-side
mirror of `mxbuild --serve`, running the runner's watch branch and parsing its
`modern-web-bundler-protocol` stdout to count successful bundles. A page/widget edit
re-bundles in ~3–4 s (vs ~7 s cold); the loop re-bundles **only when the edit touched
web/ client source** (mtime gate), so a microflow/entity edit skips it and just
hot-reloads. `WaitForRebuild` settles out cleanly if the touched file isn't a rollup
input, so it never hangs.

Two findings from implementation:
- **inotify is silent on container overlay filesystems** — the rollup watcher's
  chokidar detected changes only after tens of seconds until `CHOKIDAR_USEPOLLING`
  (+`CHOKIDAR_INTERVAL`) was set, which drops detection to ~1 s. (Thanks to the polling
  tip.)
- **The change signal must watch model source, not the project dir.** The build
  rewrites `theme-cache/`, `.mendix-cache/`, and `deployment/` every run, and
  screenshots land in `.mxcli/`; an initial whole-dir watcher self-triggered an
  infinite rebuild loop. `projectSourceMTime` watches only `.mpr` + `mprcontents/`.

`--screenshot` captures a PNG after boot and each applied change via Playwright's
built-in `screenshot` command (Chromium from `PLAYWRIGHT_BROWSERS_PATH`; no
`playwright-cli` dependency), completing the pixel-perfect page loop.

**Deep-link + authenticated screenshots — done.** `--screenshot-url` accepts a page
path (resolved against the app root) or a full URL, so a specific page under edit is
shot rather than the app root. `--screenshot-user`/`--screenshot-password` log in
once through the Mendix login form (a headless Playwright script driving
`#usernameInput`/`#passwordInput`/`#loginButton`, run via the same Playwright install
resolved from the CLI's package dir — no hardcoded paths), save the session as a
Playwright storage state, and reuse it for every screenshot via
`screenshot --load-storage`. Login is best-effort: an anonymous app with no form
proceeds unauthenticated. Verified E2E: the saved storage state carries the Mendix
session cookies and the screenshot renders the authenticated page.

**Multi-page screenshot sets — done.** `--screenshot-url` is repeatable; more than one
target produces a screenshot *set* — one PNG per page, named from the page slug
(`run-local-p-customers.png`, `run-local-home.png`) — so every change yields a
visual-regression sheet across the key pages. Verified E2E: two targets produced two
distinct PNGs in one change cycle.

## Proposed CLI

> **Shipped name (slice 1):** the warm loop shipped as **`mxcli run --local`**, not
> `mxcli dev`. The separate `dev reload`/`dev exec`/`dev status`/`dev stop`
> subcommands envisioned below were folded into a single long-lived
> `mxcli run --local --watch` (it watches the model source and hot-applies each
> change itself), so no `reload`/`exec` subcommands were needed. The `mxcli dev …`
> names in the rest of this proposal refer to that command (and the not-yet-built
> `dev up`/`dev serve` bootstrap + preview pieces of slices 2–3).

### Scenario A: `mxcli dev` — Docker-free warm run loop

A long-lived local dev supervisor. Boots (or reuses) the standalone runtime +
Postgres, starts `mxbuild --serve`, and on each model change runs
`serve build (Deploy)` → branch on `restartRequired` → `reload_model` or
DDL+restart.

```bash
# start a warm dev session (no Docker)
mxcli dev -p app.mpr
#   → downloads/caches mxbuild + runtime (once), starts Postgres,
#     boots the runtime, starts `mxbuild --serve`, prints the local URL,
#     and holds everything warm.

# apply a change and hot-reload (from another shell, or driven by the agent)
mxcli dev reload -p app.mpr          # serve-build + restartRequired-aware reload
mxcli dev exec change.mdl -p app.mpr # exec MDL, then reload in one step
mxcli dev status                     # runtime status, warm-build server health
mxcli dev stop
```

`mxcli dev` **subsumes the inner loop of `mxcli docker run`** while keeping
`mxcli docker` for produced-artifact / container-parity builds.

Optionally a `--watch` mode: watch the `.mpr` (v2 `mprcontents/`) for changes and
auto-run the reload cycle.

### Scenario B: `mxcli dev serve` + a preview container (iPad)

The iPad has two panes: **Claude Code on the web** (prompt + edit) and **Safari**
(the running app). The catch, established during investigation: the Claude Code web
sandbox is **egress-only and cannot expose a port** (all reverse tunnels are blocked
by a 443-only + TLS-intercepting egress policy). So the *running* app must live in a
container that has real public ingress — a **Codespace** (public port forwarding →
`https://<name>-8080.app.github.dev`) is the natural fit.

Two containers, coupled **without a commit** (the explicit requirement — preview WIP
before committing):

```
┌─────────────────────────┐         push built deployment          ┌──────────────────────────┐
│  DEV: Claude Code Web    │  ── (443, authenticated, no git) ──▶   │  PREVIEW: Codespace       │
│  edits app.mpr           │                                        │  runtime + mxbuild --serve │
│  mxbuild --serve (build) │                                        │  Postgres                  │
│                          │  ◀── agent verifies via public URL ──  │  :8080 forwarded PUBLIC    │
└─────────────────────────┘         (curl / Playwright)             └──────────────────────────┘
                                                                      Safari tab ▲ (iPad)
```

- **DEV** (Claude Code Web): prompt Claude → `mxcli` edits `app.mpr` →
  `mxbuild --serve` produces the Deploy artifact → `mxcli dev push` uploads it to the
  Preview container over 443.
- **PREVIEW** (Codespace): receives the artifact, runs `reload_model` (or DDL+restart
  on `restartRequired`), serves the public URL. Booted by a `devcontainer.json` +
  `postStart` script that reproduces the standalone-boot recipe.
- **iPad UX:** prompt in the Claude pane → ~1–2 s → refresh the Safari tab → see the
  change. Nothing is committed until the user is happy.

Proposed commands:

```bash
# in the Preview container (Codespace), started by the devcontainer:
mxcli dev serve -p app.mpr --intake-port 9000 --intake-secret $TOKEN
#   → boots runtime + Postgres + mxbuild --serve, forwards 8080 public,
#     exposes an authenticated /deploy intake for artifact pushes.

# in the DEV container (Claude Code Web):
mxcli dev push --to https://<name>-9000.app.github.dev --secret $TOKEN -p app.mpr
#   → mxbuild --serve (Deploy) locally, then upload the deployment delta;
#     the Preview container reloads and returns the live status.
```

### Coupling: options considered

| Option | Preview WIP w/o commit? | Notes |
|--------|-------------------------|-------|
| **A. Push built deployment artifact over 443** (recommended) | ✅ | robust; runtime in a legit host; ~1–2 s + network |
| B. Push MDL/model, build in the Preview container | ✅ | keeps a synced `.mpr` in the Preview container; heavier transfer for MPR v2 |
| C. Git branch, Codespace pulls | ❌ | requires a commit — **rejected**, violates the core requirement |
| D. Live tunnel (app stays in DEV, static relay forwards) | ✅ | **viable — chisel verified** (see § Scaling). Cleanest dev loop: hot-reload stays local, no artifact sync, and the relay is a generic static component. Caveat: the DEV runtime is ephemeral. **Preferred for the multi-container dev loop; A remains the durable/persistent-preview fallback.** |

## Provisioning: from new Claude Code Web project to a running testable app

Three entry points, ordered by how little you need to start with:

- **Prompt template (primary for web/iPad — starts from a *truly empty* repo).** Open an
  empty repo in Claude Code Web and paste a fixed bootstrap prompt; the agent provisions
  the project. No local CLI and no pre-built template repo to maintain — the agent runs
  *current* mxcli and can seed the model from a Claude design prototype in the same
  prompt. Detailed below.
- **Repo template ("Use this template" on GitHub).** A `mendix-mxcli-starter` marked as a
  GitHub template → a new repo with the config already committed. Deterministic, but needs
  per-version upkeep; a good fixed, reviewed baseline. Could be generated by
  `mxcli new --emit-template` + CI per Mendix version so it is never hand-maintained.
- **Local CLI.** `mxcli new MyApp --version <X.Y.Z>` (new app: downloads MxBuild, runs
  `mx create-project`, scaffolds `.claude/` + a devcontainer, installs the Linux mxcli
  binary) or `mxcli init` on an existing repo with an `.mpr`. Requires a machine with the
  CLI — **not available on an iPad**.

### Prompt template (the web/iPad-native path)

The agent turns an empty repo into a self-sufficient Mendix project in one session:

```
This is an empty repo. Provision it as a Mendix app developed with mxcli:
1. Ensure `mxcli` is available (should be pre-installed by the environment; else
   `go install github.com/mendixlabs/mxcli/cmd/mxcli@latest`).
2. `mxcli new App --version 11.6.3` at the repo root (or `mxcli init` if an .mpr exists).
3. Add `.claude/settings.json` with a SessionStart hook running `./mxcli dev up -p App.mpr`,
   then COMMIT .mpr + .devcontainer/ + .claude/ so future sessions self-bootstrap.
4. `mxcli dev up -p App.mpr` — cache MxBuild+runtime, start Postgres, boot the runtime.
5. Confirm HTTP 200 locally and report.
(Optional) Seed the domain model, pages, and microflows from this prototype: <paste/link>.
```

Two rules make this robust:

- **Committing the config (step 3) is mandatory.** The prompt is a *one-time seed*; its
  output must be committed (`.mpr` + `.devcontainer/` + `.claude/` SessionStart hook) so
  that after idle **reaping** the next session self-bootstraps from files, not from
  re-prompting. Steady state is then file-driven and deterministic — the prompt template
  *produces* the repo-template end state without a maintained template.
- **mxcli delivery is an *environment*-layer concern, not the prompt's.** Step 1 is the
  fragile part: GitHub is gated in web sessions (a release-binary `curl` may be blocked),
  and `go install` via `proxy.golang.org` works **only if mxcli is a public Go module**
  (the mechanism was proven for chisel this session). Robust fix: the Claude Code Web
  **environment setup script / image pre-installs mxcli** (and pre-caches MxBuild+runtime),
  so the prompt only does logic (`new` → commit → `dev up`), never delivery. Publishing
  mxcli as a public Go module is the fallback.

### Initializing the Claude Code Web session

`mxcli init` / `new` today scaffold a **devcontainer + `.claude/` tooling** — enough for
Codespaces and local dev containers, but Claude Code Web needs one more piece: a
**SessionStart hook** (or devcontainer `postStart`) that idempotently brings the runtime
up on **every** session, because background processes (Postgres, the JVM) are **reaped
on idle** — observed repeatedly during this investigation.

**Shipped:** `mxcli init` now emits a Claude Code **SessionStart hook** into
`.claude/settings.json` (merged idempotently, preserving existing settings) that runs
`./mxcli run --local --setup --ensure-db -p <app.mpr>`. That `--setup` mode is the
non-blocking bring-up — it does steps 1–3 below and **returns** (a SessionStart hook
must not block), leaving the session ready to `mxcli run --local` on demand:

1. **Ensure mxcli** — prefer a prebuilt binary via `mxcli setup mxcli` (fast) over a
   from-source build (needs antlr4 + Go, ~70 s). *(mxcli delivery is the environment's
   job; the hook guards on `test -x ./mxcli`.)*
2. **Ensure MxBuild + runtime cached** — `run --local --setup` runs DownloadMxBuild +
   the runtime resolve. One-time ~700 MB / ~30–40 s; **bake into the devcontainer
   image** so the first session is instant.
3. **Start Postgres + create the app DB** — `--ensure-db`, re-run every session
   (survives reaping).

Step 4 — **booting the runtime + `mxbuild --serve`** — is deliberately *not* in the
hook (it is long-lived and would block session start). The agent runs the full
`mxcli run --local` when it wants to test; the heavy prerequisites are already warm.

End state: the session comes up **ready to prompt → build → test**. Then:

- `mxcli dev` — local testable app at `localhost:8080` (the ~1 s warm loop).
- `mxcli dev --hub <url>` — externally testable: self-registers with the hub and gets a
  public subdomain (§ Scaling).

### First-run vs warm

| Phase | One-time (first session / image build) | Per session (after reap) |
|-------|----------------------------------------|--------------------------|
| mxcli binary | `setup mxcli` download (or ~70 s build) | cached |
| MxBuild + runtime | ~700 MB, ~30–40 s | cached |
| Postgres + DB | — | start + createdb (~seconds) |
| runtime boot | — | ~40 s cold / seconds warm |

The heavy costs are one-time and **image-cacheable**; steady-state session startup is
just Postgres + a runtime boot. The only real gap is the **SessionStart bootstrap** —
mxcli already has every ingredient (`new`, `init`, `setup mx*`, the standalone boot);
this proposal wires them into one idempotent `mxcli dev up` that Claude Code Web runs on
session start.

## Scaling: tunnel hub with self-registration and admin overview

The two-container model generalizes to **one static hub fronting many dynamic dev
containers** — parallel agents on different features, or the several apps of one
distributed solution. Verified: a single `chisel server --reverse` fans in to multiple
clients, each on its own server-side port (`server:8001 → container A`,
`server:8002 → container B`), and `--authfile` (auto-reloaded on change) isolates them
per-agent.

**Why chisel and not the others** (all tested this session): it installs via
`go install` (through `proxy.golang.org`, bypassing the GitHub gate), multiplexes
everything over a **single 443 connection** (no second data port — the wall that killed
localtunnel and cloudflared), and its TLS **passes the sandbox's mandatory MITM**
because it uses the system trust store, which already contains the agent CA (ngrok
failed only because it pins its own roots). The one hop not provable without a live
host: the WebSocket upgrade through Codespaces' `app.github.dev` proxy — a **VPS relay
removes that unknown** and is cleaner AUP-wise for a pure relay.

### `mxcli tunnel-hub` (static) + `mxcli dev --hub` (dynamic)

- **`mxcli tunnel-hub`** — the always-on static container. Embeds a chisel server
  (`github.com/jpillora/chisel/server` as a library), a host-routing layer
  (Caddy/Traefik or built-in), a **registration API**, and an **admin overview page**.
  Runs once on a VPS (preferred) or a Codespace. It never changes when an app changes.
- **`mxcli dev --hub <url>`** — the dynamic side self-registers on startup.

Self-registration flow — chisel has **no native registration API**; this thin control
plane adds one on top of its two primitives (the auto-reloading `--authfile` and the
client-declared reverse remote):

```
mxcli dev --hub https://hub.example.com            (on container startup)
  → POST /register {app, feature, agent}           → hub allocates {port, subdomain, token}
                                                      appends users.json  (chisel reloads)
                                                      writes vhost subdomain→port (proxy reloads)
  ← {port, subdomain, token}
  → chisel client HUB R:<port>:localhost:8080 --auth <user:token>
  → set Mendix ApplicationRootUrl = https://<subdomain>.example.com
  → heartbeat POST /status {health, last_reload, change_summary}   (periodic)
on stop / TTL expiry → POST /deregister            (frees the slot)
```

### Admin overview page

The hub serves an authenticated dashboard — the single place to see and reach every
in-flight preview:

- **One row per dev container:** agent/feature name, app, **clickable public URL**,
  runtime health (from the heartbeat), last hot-reload time, uptime.
- **Pending changes per container:** each `mxcli dev` reports its delta — from
  `mxcli diff-local` against the committed baseline, or the running list of MDL
  statements applied this session — and the hub aggregates them, so a reviewer sees
  *what each agent has changed* without opening each session.
- **Use cases:** a PM/reviewer clicks through parallel feature previews; comparing two
  agents' takes on the same screen side by side; spotting a container that has drifted
  or broken; a distributed-solution operator seeing all constituent apps and their
  states at once.

The change list makes the hub more than a router — it is a **live index of work in
progress across the fleet**, which is exactly the "keep overview" need.

### Mendix wrinkles at fleet scale

- **Subdomain, not path**, per container — `ApplicationRootUrl` = the container's own
  subdomain. Path-prefix routing fights Mendix's SPA/XAS/cookies; subdomains give each
  app a clean origin and free **cookie/session isolation** between agents.
- **DB per container** — each dev container needs its own Postgres/schema + demo data,
  or agents overwrite each other. (Sandbox model: each has its own local Postgres.)
- **Slot assignment** — the hub owns the `{port, subdomain, token}` map via the
  registration API; `--authfile` pins each agent to its allowed remote.
- **Distributed solution** — constituent apps register as separate containers on
  separate subdomains and integrate via their public routes (published REST/OData),
  faithfully previewing the real multi-app topology.

## Implementation Plan

Reuse the existing `cmd/mxcli/docker/` plumbing wherever possible — it already
downloads/caches mxbuild + runtime and speaks the M2EE admin API.

### Files to modify/create

| File | Change |
|------|--------|
| `cmd/mxcli/dev.go` | New `mxcli dev` command tree (`dev`, `dev reload`, `dev exec`, `dev serve`, `dev push`, `dev status`, `dev stop`). |
| `cmd/mxcli/docker/mxserve.go` | New: `mxbuild --serve` client — start the daemon, `POST /build {target:Deploy}`, parse `status` + `restartRequired`. |
| `cmd/mxcli/docker/admin.go` | Extract the M2EE admin client (auth header `X-M2EE-Authentication: base64(pass)`, actions `update_appcontainer_configuration`, `update_configuration`, `start`, `execute_ddl_commands`, **`reload_model`**, `runtime_status`) out of `docker/oql.go` into a reusable client. |
| `cmd/mxcli/docker/localboot.go` | New: standalone runtime boot (see the config-set below), so `dev` can boot without Docker. |
| `cmd/mxcli/docker/download.go` | Reuse `DownloadMxBuild` / `DownloadRuntime` (no change). |
| `cmd/mxcli/docker/mxenv.go` | Reuse `PrepareMxCommand` (LD_PRELOAD FreeType fix) for the `mx`/`mxbuild` children (no change). |
| `.devcontainer/preview/devcontainer.json` + `boot.sh` | New: Codespace preview container — `forwardPorts: [8080]` public, Postgres, `postStart` → `mxcli dev serve`. |
| `.claude/skills/mendix/docker-workflow.md` | Add a "warm dev loop" section pointing at `mxcli dev`. |
| `cmd/mxcli/tunnelhub/server.go` | New: `mxcli tunnel-hub` — embedded chisel server (`chisel/server` lib), host routing, slot allocation, `--authfile` writer. |
| `cmd/mxcli/tunnelhub/register.go` | New: registration API (`/register`, `/status` heartbeat, `/deregister`); owns the `{port, subdomain, token}` map + TTL cleanup. |
| `cmd/mxcli/tunnelhub/admin.go` | New: authenticated admin overview page — fleet list (URL, health, last reload) + per-container change summaries. |
| `cmd/mxcli/dev.go` | Add `--hub <url>`: self-register on startup, run the chisel client, set `ApplicationRootUrl` from the assigned subdomain, push `change_summary` heartbeats, deregister on stop. |
| `cmd/mxcli/dev.go` (`dev up`) | Idempotent bootstrap for provisioning: `setup mxcli`/`mxbuild`/`mxruntime` if missing, start Postgres + create DB, boot runtime + `mxbuild --serve`. Safe to re-run every session (survives reaping). |
| `cmd/mxcli/init.go` | Emit a **SessionStart hook** (Claude Code Web) and/or devcontainer `postStart` that runs `mxcli dev up`; keep the existing devcontainer + `.claude/` scaffolding. |
| `cmd/mxcli/new.go` | Add `--emit-template`: write a GitHub-template-ready repo (config + starter `.mpr`), for CI-per-version publishing of `mendix-mxcli-starter`. |
| `docs-site/.../bootstrap-prompt.md` | Ship the canonical **prompt template** (the web/iPad seed prompt) as documented, copy-pasteable text. |
| release / go.mod | Ensure mxcli is deliverable into a gated web session. **Status:** the module is public (tags to v0.16.0) but **`go install` does not build** — the generated ANTLR parser (`mdl/grammar/parser/`) is gitignored/uncommitted, so the tagged source is missing the package. Working paths today: prebuilt release binaries (`mxcli-<os>-<arch>`, incl. a rolling `nightly`) via `mxcli setup mxcli` / direct download, or an **environment setup-script** pre-install. Enabling `go install` needs the parser committed (or generated during module build) — open decision. |
| `cmd/mxcli/tunnelhub/*_test.go` | Tests: slot allocation/isolation, authfile + vhost reload, register/heartbeat/deregister lifecycle, admin-page rendering. |
| `cmd/mxcli/dev_test.go` | Tests for the serve client and the `restartRequired` branch logic (mock the serve/admin HTTP endpoints). |

### The standalone-boot config set (discovered, must be encoded)

The standalone runtime launcher (`com.mendix.container.boot.Main`, driven via the
admin API) requires a specific config that Studio Pro / the buildpack normally
supply. `mxcli dev` must set all of it or `start` fails:

- Env: `MX_INSTALL_PATH`, `M2EE_ADMIN_PASS`, `M2EE_ADMIN_PORT`, and the FreeType
  `LD_PRELOAD` on the JVM.
- Ensure the `data/{files,tmp,model-upload}` dirs exist under the deployment.
- `update_configuration`: `BasePath` (deployment dir), `RuntimePath`
  (`<install>/runtime`), `DatabaseType/Host/Name/UserName/Password`
  (`DatabaseHost` includes the port, e.g. `127.0.0.1:5432`), `DTAPMode`, and
  **`MicroflowConstants`** (design-time default values are *not* auto-applied
  standalone — missing a constant surfaces at runtime as HTTP 530 → login bounce).
- Boot sequence: `update_appcontainer_configuration` (runtime port) →
  `update_configuration` → `start` → on `"database has to be updated"`:
  `execute_ddl_commands` → `start`.
- Reload cycle (two independent signals — see § Hot-reload scope): `mxbuild --serve`
  Deploy build → if `restartRequired == false` then `reload_model`; else restart,
  running `execute_ddl_commands` first **only if** `get_ddl_commands > 0`. Do **not**
  couple "restart" to "DDL" — OQL view entities need a restart with zero DDL.

## Delivery slices

This stays one proposal, but it should **ship as four independent slices** — each
valuable on its own, each a candidate for its own PR. Ordered by value-to-risk; each
builds on the previous.

| # | Slice | Delivers | Depends on | Size / risk |
|---|-------|----------|------------|-------------|
| 1 | **Warm local loop** — shipped as `mxcli run --local [--watch]` (serve daemon + M2EE admin client + `restartRequired` branching; + client bundling & Playwright screenshots) | Docker-free ~1 s edit→test loop, locally | nothing new | ✅ **shipped** |
| 2 | **Provisioning** — `run --local --ensure-db` (DB) + `run --local --setup` (non-blocking bring-up) + `mxcli init` SessionStart hook + bootstrap prompt template | a fresh Claude Code Web session comes up testable; iPad-native start | slice 1 | ✅ **shipped** |
| 3 | **Single-app external preview** — `mxcli dev serve` + chisel client → one static relay + `ApplicationRootUrl` wiring | a shareable live preview URL (the iPad two-container flow) | slice 1 | medium / medium — the `app.github.dev` WebSocket hop is unverified (a VPS relay avoids it) |
| 4 | **Tunnel hub** — `mxcli tunnel-hub` + `mxcli dev --hub` + registration API + admin overview | many dev containers behind one ingress; fleet overview + per-container change lists | slice 3 | large / higher — a product in its own right, with a multi-tenant auth surface |

Recommended sequencing: **1 → 2** delivers the complete solo dev experience (Scenario A
plus provisioning) with no external moving parts, and can ship first. **3** adds external
preview for a single app. **4** is the scale-out and deserves its own design/security
review — build it only once 1–3 are proven. Slices 1–2 stand alone if the tunnel/hub work
is deferred indefinitely.

Cross-cut: the instant `mxcli check` gate (`PROPOSAL_check_mxbuild_gap_heuristics.md`)
should front slice 1's build on every iteration, so most errors never reach even the
~0.8 s warm build.

## Version Compatibility

- `mxbuild --serve` and `reload_model`: **≥ 11.6.3** (verified on 11.6.3 and
  11.12.1). No new flag needed on 11.12 — the incremental path is `--serve` in both.
- Register the feature in `sdk/versions/mendix-11.yaml` (and 10 if we verify the
  admin action name `reload_model` there) with the correct `min_version`, and add a
  `checkFeature()` pre-check with an actionable hint.
- **Production-representative testing note:** running `DTAPMode=P` requires the app's
  security level to be `CHECKEVERYTHING` (Production) or `start` refuses
  (`result:11`). `mxcli dev` should default to `D` for the inner loop and expose a
  `--dtap P` flag; when `P` is set, surface the security-level requirement up front.

## Test Plan

- `cmd/mxcli/docker/mxserve_test.go` — mock the serve HTTP endpoint; assert request
  shape (`{target:Deploy, projectFilePath}`) and `restartRequired` parsing.
- `cmd/mxcli/docker/admin_test.go` — mock the admin endpoint; assert the boot
  sequence and the `reload_model` vs DDL+restart branch.
- Integration (gated, needs mxbuild + runtime + Postgres, like the existing docker
  tests): boot a fixture app, do a microflow change → assert warm reload with pid
  unchanged; do an entity change → assert `restartRequired` → DDL path.
- `mdl-examples/` — a small fixture app the dev-loop tests drive.

## Open Questions

1. **Process lifecycle.** Long-lived `mxbuild --serve` and the runtime JVM need
   supervision (restart on crash, port cleanup). PID files under `.mxcli/`? A
   `dev status`/`dev stop`? (In the sandbox, background JVMs were reaped on idle —
   the Codespace/preview host must keep them alive; document the idle-stop caveat.)
2. **Artifact transfer for Scenario B.** Push the whole Deploy `model/` each time, or
   compute a delta? MPR v2 is a folder; the *deployment* `model/` is more compact,
   but still worth measuring. What's the auth model for the intake endpoint (bearer
   secret over the public forward)?
3. **Codespaces AUP.** Running your own app for preview is intended use; the
   Codespace-as-relay variant (Option D) is a gray area. The proposal recommends A
   (artifact push) precisely to stay clearly on the "previewing your own project"
   side. Confirm this framing.
4. **Warm-serve memory.** `mxbuild --serve` holds the model in memory; quantify for
   large apps and document guidance.
5. **`reload_model` scope (largely resolved — see § Hot-reload scope).** Verified:
   microflow changes hot-reload; entity/view-entity/association changes flip
   `restartRequired` because the runtime reconciles its DB metamodel catalog
   (`mendixsystem$entity` / `entityidentifier` / `attribute` / `association`) at
   startup, not on reload. Open sub-items: (a) enable the `preview_execute_oql`
   dev flag on the standalone boot so the loop can *functionally* verify a change via
   OQL (needed to close the view-entity caveat and to power agentic verification);
   (b) exercise styling-only changes (`update_styling` is a distinct admin action —
   likely a third hot path); (c) multi-unit and mixed changes.
6. **Relationship to `check`.** The instant `mxcli check` gate (see
   `PROPOSAL_check_mxbuild_gap_heuristics.md`) should run *before* every warm build so
   most errors never reach even the ~0.8 s build. `mxcli dev` should call it first.
7. **Tunnel-hub auth & multi-tenancy.** The hub fronts multiple previews on one host —
   who may view the admin page, and how are per-container URLs protected (per-subdomain
   auth, shared secret, SSO)? Registration tokens must be scoped so one agent cannot
   claim another's slot (`--authfile` allowed-remote regex per user).
8. **Change-summary source & cadence.** `mxcli diff-local` (git baseline) vs a running
   MDL-statement log — which is the better per-container "changes" feed for the admin
   page, and how often to push it (every reload vs periodic)?
9. **WebSocket through `app.github.dev` unverified.** chisel's reverse tunnel is proven
   through the sandbox egress + MITM, but not through a live Codespace forward. Verify
   on a real Codespace, or default the hub to a VPS (also the cleaner AUP posture for a
   pure relay).
10. **Fleet lifecycle.** TTL/heartbeat-timeout cleanup of dead containers, port/subdomain
    reclamation, and what the admin page shows for a container whose sandbox was reaped
    (the ephemeral-runtime reality observed this session).
11. **Provisioning baseline.** Best mechanism to pre-bake MxBuild + runtime (~700 MB)
    into the Claude Code Web image so first session is instant, vs downloading on first
    `mxcli dev up`. And the exact SessionStart hook contract on Claude Code Web (does it
    run on every resume, with what timeout?) — determines whether `dev up` must be fully
    idempotent and how long it may take.
12. **mxcli delivery into a gated empty repo (the prompt-template blocker).** GitHub is
    gated in web sessions, so a release-binary `curl` may be blocked; `go install` via
    `proxy.golang.org` only works if mxcli is a **public** module. Decide the canonical
    delivery: publish a public Go module, pre-install mxcli in the environment image, or
    both. This determines whether the prompt-template path works unassisted or must lean
    on the environment layer.
