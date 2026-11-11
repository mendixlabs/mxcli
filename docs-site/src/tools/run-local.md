# mxcli run --local

`mxcli run --local` runs a Mendix app in a **warm, Docker-free dev loop**. It keeps
a `mxbuild --serve` process and a standalone Mendix runtime hot, so after the first
(cold) build a model change rebuilds incrementally and is hot-applied — without a
full rebuild-image-and-restart cycle.

```bash
mxcli run --local -p app.mpr
mxcli run --local -p app.mpr --watch
mxcli run --local -p app.mpr --watch --detach   # in the background
```

## Running it as a background service

A coding agent's tool call has to return, so the loop can run detached and be
managed in one call per question — no `nohup`, no `sleep`/`grep`/`curl` polling
loops, no `pkill`:

```console
$ mxcli run --local --watch --detach -p app.mpr
running: http://127.0.0.1:8080/ (pid 30133, watch, log .mxcli/run.log)

$ mxcli exec add-page.mdl -p app.mpr && mxcli run wait -p app.mpr
applied: build #2 via reload in 4.9s

$ mxcli run status -p app.mpr
running: http://127.0.0.1:8080/ (pid 30133, up 22s, watch, build #2 applied via reload 0s ago)

$ mxcli run stop -p app.mpr
stopped: pid 30133 in 300ms
```

| Command | Does | Exit |
|---|---|---|
| `run --local … --detach` | starts the loop in its own session (it survives the calling shell), writes `.mxcli/run-state.json` and `.mxcli/run.log`, returns once the app serves. A failed boot prints the reason and the last log lines. A second run for the same project is refused. | 0 / 1 |
| `run status` | one line: URL, pid, uptime, last build and how it was applied, a build in progress, a change still pending. A state file whose process is gone is reported as stopped, with any leftover processes. A run whose runtime stopped on its own says why when its log does — `stopped: trial licence time limit reached at 18:43:16 (runtime.log); restart with mxcli run restart -p …`. | 0 running, 3 stopped, 4 starting |
| `run wait [--timeout 5m]` | blocks until the watch loop has applied the model as it is now, or failed to; prints `applied: build #N via reload\|restart in …` or `failed:` with the CE codes / SCSS error lines. `--ready` waits for the boot instead; `--since N` for any build newer than N. | 0 applied, 1 failed, 2 timeout, 3 not running |
| `run stop` | asks the run to shut down, waits until it is gone, then stops anything left in its process session — so no orphaned `runtimelauncher` or mxbuild JVM, even from a run killed with `-9`. Nothing running is a no-op. | 0 |
| `run restart` | `run stop`, then `--detach` again with the arguments recorded in the state file. | as `--detach` |
| `run clear-sessions` | Deletes every session in the run's database, so the unlicensed runtime's 6 sign-in slots are free again without a restart (a 7th sign-in otherwise fails with HTTP 560, shown as "Sign in failed"). Signed-in browsers sign in again. Uses the run's recorded `--db-*` flags; pass `--db-password` when the run was given one literally. Not for `--db-type hsqldb`: restart instead. | 0 cleared, 1 failed, 3 no run recorded |

**`run wait` is race-free.** It reads the model's source time when it starts and
returns the first build made from that source or newer — so a change the loop
applied *before* `wait` was called is reported at once rather than waited for, and
`mxcli exec x.mdl && mxcli run wait` is a correct one-call chain. With nothing
changed it reports the last build. On Mendix versions whose serve process only
builds completely once ([below](#--watch-when-mxbuild-writes-the-client-11125-1114)),
the loop restarts that process per change; if even the fresh one fails, `wait`
reports the failure with its explanation — it never hangs on it. A run started without `--watch` never applies
a change, and `wait` says so (exit 1).

`run status|wait|stop` also work on a foreground `run --local` started in another
terminal — every run publishes its state. On Windows, `run stop` ends the process
tree without a graceful shutdown.

### `.mxcli/run-local.json`

Besides `run-state.json`, a run publishes **`.mxcli/run-local.json`** (mode 0600): the
app and admin ports, the admin password and the boot configuration — how `mxcli oql`,
`mxcli log`, `constant set --apply`, and any tool that calls the admin API, reach the
app.

- **Written** once the app serves, carrying the run's pid.
- **Removed** when that run ends — a requested stop, or the runtime stopping on its own
  (the trial licence's time limit, a crash) — immediately after `run-state.json`
  records `"phase": "stopped"`, `"ended"` and a one-line `"reason"`. `run stop`
  removes it for a run that was killed outright. No other process removes it: a
  `run --local --setup` (the SessionStart hook) or a refused second run leaves it alone.
- **Missing?** Check `mxcli run status -p app.mpr`, not the credentials. It reports a
  stopped run with its reason, or a live run whose handshake is gone (`mxcli run
  restart` republishes it). A 401/403 from the admin API without the handshake means
  the connection fell back to default settings; `oql` and `log` say so.

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
so structural changes need a restart; behavioural changes do not. A restart drops
every browser session — `--watch` prints `browser sessions were dropped; log in
again` when it happens, so a browser test knows to sign in before its next step.

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
    local Postgres server if the port is down, and creates the app role + database
    if missing. It uses a service manager, or a user-owned `initdb`/`pg_ctl` cluster
    under `~/.mxcli/postgres` when no service becomes ready (e.g. Arch) — no
    `postgres` OS account or `sudo` required. For a non-local `--db-host` it only verifies
    reachability — mxcli won't provision a remote database.
    The user-owned cluster persists across sessions; its server log is
    `~/.mxcli/postgres/server.log`. Stop it with
    `pg_ctl -D "$HOME/.mxcli/postgres/data" stop`. To remove it, stop it first and
    then delete `~/.mxcli/postgres` (this permanently deletes its databases).
    The fallback never replaces a **stopped system cluster** for the same port
    (`/var/lib/postgresql/<ver>/<name>`, `/var/lib/pgsql/data`,
    `/var/lib/postgres/data`, Homebrew's): a new cluster there would hold none of the
    app's data while looking healthy, so `--ensure-db` stops, names the data
    directory and prints the command that starts it (e.g.
    `sudo pg_ctlcluster 16 main start`). If that cluster is not this app's and cannot
    be started, `MXCLI_ENSURE_DB_NEW_CLUSTER=1` creates the user-owned cluster anyway.
    Whenever a new cluster is initialized, the output says
    `WARNING: created a NEW, EMPTY PostgreSQL cluster`; an existing user-owned cluster
    is restarted without it.
    A server whose `postgres` superuser has a password — **every Windows install**,
    where the installer always sets one — needs that login:
    `--ensure-db --db-admin-user postgres --db-admin-password <password>`, or set
    `PGPASSWORD` (or a `.pgpass` entry) for `postgres`, which keeps the password off
    the command line. The admin login only creates the role and database; the app
    still connects as `--db-user`.
  - Otherwise create it once yourself; without `--ensure-db`, `run --local` stops with
    an actionable message if the DB is unreachable:

    ```bash
    createdb -h 127.0.0.1 -U mendix app1112
    ```

- **Or no database server at all**: `--db-type hsqldb` runs on the runtime's
  built-in file database. Use it where PostgreSQL is not installed or cannot be
  provisioned — e.g. `--ensure-db` failing with *no local PostgreSQL superuser
  available to create the role/database*. See the `--db-type` row below.

At start, `run --local` warns (without stopping) when the project's git state
would make Studio Pro 11.13 fail to open it — a branch with no upstream, or
"dubious ownership". See [Working Outside Studio Pro](outside-studio-pro.md).

## Flags

| Flag | Default | Purpose |
|------|---------|---------|
| `--local` | — | Required; run without Docker (implied by `--hub`) |
| `--hub` | — | Expose the running app in a browser at a tunnel-hub URL (see [External browser preview](#external-browser-preview---hub)) |
| `--hub-secret` | — | Shared auth (`user:pass`) matching an **open** hub's `--secret` |
| *(hub API key)* | — | For a **GitHub-authenticated** hub: get one from `https://<hub>/cli`, set `MXCLI_HUB_KEY` (see below) |
| `--watch` | off | Rebuild + hot-apply on every project change |
| `--web-client-timeout` | `$MXCLI_WEB_CLIENT_TIMEOUT`, else `5m` | Limit for one web client bundle build (e.g. `10m`); on timeout the tail of `deployment/log/web-client-build.log` is printed |
| `--build-timeout` | `$MXCLI_BUILD_TIMEOUT`, else `30m` | Limit for one `mxbuild --serve` build. The cold first build of a large app (thousands of documents) can take ten minutes or more |
| `--start-timeout` | `$MXCLI_START_TIMEOUT`, else `15m` | Limit for the runtime start, after-startup microflow and schema update included. Running out says "did not answer", not "is the app running?" |
| `--ensure-db` | off | Provision local Postgres + the app database if missing (fresh-session bootstrap) |
| `--db-admin-user` / `--db-admin-password` | postgres / — | Superuser login `--ensure-db` creates the role and database with when it has a password (Windows). Only with `--ensure-db`; never passed to the runtime |
| `--setup` | off | Prepare prerequisites (cache MxBuild+runtime, ensure DB) and exit without booting — for a SessionStart hook |
| `--app-port` | 8080 | App HTTP port |
| `--admin-port` | 8090 | M2EE admin API port |
| `--serve-port` | 6543 | `mxbuild --serve` port |
| `--listen-addr` | `127.0.0.1` | IP address the app binds to (not a host name). `0.0.0.0` binds all IPv4 interfaces, `::` all interfaces, so other machines on the network can open the app; the URLs it is reachable at are printed after boot. The runtime does not derive an application root URL from this address, so if the app builds absolute URLs (SSO redirects, links in emails) pass `--runtime-setting ApplicationRootUrl=http://<host>:<port>/` or set a real host in the configuration (a warning says so). The M2EE admin API is unaffected and always binds `127.0.0.1` |
| `--mxbuild-path` | resolved for this host | The mxbuild to build with, overriding resolution (Studio Pro on macOS/Windows, the cached CDN download on Linux). `MXCLI_MXBUILD_PATH` sets the same override from the environment; the flag wins |
| `--db-host` | 127.0.0.1:5432 | Database `host:port`; bracket IPv6 endpoints (`[::1]:5432`) |
| `--db-name` | derived from project | Database name |
| `--db-user` / `--db-password` | mendix / mendix | Database credentials; the password takes `@env:VAR` / `@file:path` — see [Secrets off the command line](#secrets-off-the-command-line) |
| `--db-type` | `postgresql` | `postgresql` or `hsqldb`. `hsqldb` boots on the runtime's built-in file database: no database server, no provisioning, works offline. Its data lives in `<project>/deployment/data/database/hsqldb/`. Cannot be combined with `--db-host`, `--db-user`, `--db-password` or `--ensure-db` (refused, not ignored). For local development only |
| `--screenshot` | off | Capture a Playwright PNG after boot and each applied change |
| `--screenshot-path` | `<projectDir>/.mxcli/run-local.png` | Screenshot output PNG |
| `--screenshot-url` | app root | Page to shoot: full URL, or a path relative to the app root (e.g. `/p/customers`). Repeat for a multi-page set. |
| `--screenshot-user` / `--screenshot-password` | — | Log in once (Mendix form auth) and reuse the session, so pages behind login render authenticated |
| `--runtime-log` | `<projectDir>/.mxcli/runtime.log` | Runtime log file — JVM stdout/stderr **and** the application log (server stack traces + microflow `LOG` output); `-` disables |
| `--test-endpoint` | off | Host mxcli's token-guarded test endpoint so [`mxcli test … --attach`](running-tests.md) runs a suite against this app with no boot of its own. Installed before the boot (the handler registers from after-startup, so it cannot be added to a running app); your project's own after-startup microflow is chained, not displaced. Removed on exit; one left behind by a run that was killed (with its container, say) is removed by the next `run`, and `run` and `check -p` warn while the project's after-startup microflow is still `MxTest.RegisterEndpoint` with no run hosting it, so the swap is not committed. Tests then use **this app's database** |
| `--debug` | off | Enable the microflow debugger at boot; then use [`mxcli debug`](debug-microflows.md) from another terminal. Behaviour-neutral until a breakpoint is set |
| `--debug-pass` | `mxdebug` | Debugger password when `--debug` is set |
| `--metrics` | off | Register a Prometheus meter registry; metrics served at `http://127.0.0.1:<admin-port>/prometheus` |
| `--trace` | off | Enable OpenTelemetry tracing (bundled agent → runtime log) with default span filters |
| `--trace-service` | `.mpr` name | `OTEL_SERVICE_NAME` under `--trace` |
| `--trace-otlp` | off (console) | Export traces to this OTLP collector endpoint (e.g. `http://127.0.0.1:4318`) instead of the console; implies `--trace` (needed for flame charts) |
| `--runtime-setting Key=Value` | — | Merge an extra runtime setting into the boot config (Value parsed as JSON when possible); repeatable |

## Secrets off the command line

A value typed on the command line is visible in `ps` for as long as the app runs and
stays in shell history. The secret flags — `--db-password`, `--db-admin-password`,
`--screenshot-password`, `--hub-secret`, `--debug-pass` — and every `--constant` value
can name where the secret is instead:

```bash
export MX_DB_PASSWORD=…        # from your secret store, a .env loader, CI secrets
mxcli run --local --watch --detach -p app.mpr --db-password @env:MX_DB_PASSWORD
mxcli run --local -p app.mpr --db-password @file:$HOME/.config/mxcli/db-password
```

| Form | Reads | Refused when |
|------|-------|--------------|
| `@env:VAR` | the environment variable `VAR` | it is unset or empty |
| `@file:path` | the file, minus one trailing newline | it is empty, not a regular file, or readable by its group or others (Unix; Windows is not checked) |
| `@@text` | the literal `@text` | — |

Any other value starting with `@` is refused, so a mistyped `@evn:VAR` is not used as
the password. Errors name the flag and the variable or file, never the value.

Only the reference is stored in `.mxcli/run-state.json`, so a run started this way can
be detached and restarted: `run restart` resolves it again, **in the environment of the
shell that runs the restart** (a relative `@file:` path is stored as an absolute one). A
secret written literally is not stored, and `run restart` refuses that run and says so.
The resolved database password is still in the owner-only `.mxcli/run-local.json` while
the app runs, because re-applying constants needs it.

`PGPASSWORD` is not read for `--db-password`: it is what `--ensure-db` hands `psql` for
the superuser, which is usually a different account. Use `--db-password @env:PGPASSWORD`
if they really are the same.

A password with leading or trailing whitespace may not work whatever the source: the
Mendix 11.16 beta runtime was observed to trim `DatabasePassword` (reported in
mendixlabs/mxcli#1761). mxcli passes the value unchanged.

## Metrics and OpenTelemetry

`--metrics` registers a **Prometheus** meter registry at boot, so
`http://127.0.0.1:8090/prometheus` (the admin port) serves the runtime's Micrometer
metrics (`connectionbus_*`, `handler_requests_total`, `sessions_*`, `taskqueue_*`, …).
For another registry, use `--runtime-setting`:

```bash
mxcli run --local -p app.mpr --metrics
mxcli run --local -p app.mpr --runtime-setting 'Metrics.Registries=[{"type":"otlp"}]'
```

These are flags rather than a post-boot call because the admin `update_configuration`
action **replaces** the whole config (no read-back), so a separate call would wipe the
DB/BasePath settings — `--metrics`/`--runtime-setting` merge into mxcli's single boot
`update_configuration`.

**Traces (`--trace`):** attaches the bundled OpenTelemetry Java agent to the runtime
JVM (console exporter → `runtime.log`) and applies default span filters — unfiltered
per-activity tracing is ~10× slower, so `--trace` ships
`OpenTelemetry._RuntimeSpanFilters=["CreateOrChangeVariable","Loop","Gateway","RetrieveFromCache"]`
(override via `--runtime-setting`).

```bash
mxcli run --local -p app.mpr --trace
tail -f .mxcli/runtime.log     # microflow spans: mx.microflow.name / mx.microflow.depth
```

The console exporter omits start/end timestamps and parent span IDs, so call trees
and durations can't be reconstructed from it — for flame charts, export to an OTLP
collector with `--trace-otlp <endpoint>` (implies `--trace`; sets
`OTEL_TRACES_EXPORTER=otlp`, `OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf`, and the
endpoint for you):

```bash
mxcli run --local -p app.mpr --trace-otlp http://127.0.0.1:4318
```

You can still set the OTEL env yourself for full control (`--trace` / `--trace-otlp`
won't override an exporter you've set):

```bash
export OTEL_TRACES_EXPORTER=otlp OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
mxcli run --local -p app.mpr --trace
```

## Debugging a server-side error

When a page action throws, the browser shows the generic Mendix error dialog with no
detail. The runtime log is written to `<projectDir>/.mxcli/runtime.log` (the path is
printed at boot) — `tail -f .mxcli/runtime.log` while you reproduce the action to see the
server stack trace and your microflow `LOG` output.

A standalone runtime attaches **no** log subscriber by default (a Studio Pro / m2ee run
does), so mxcli wires two sources into that file: it tees the runtime JVM's stdout/stderr,
and after start it attaches a Mendix **file log subscriber** so the application log
(microflow `LOG` output and server-side stack traces) lands there too — otherwise the file
would be nearly empty. The file is appended across restarts, each marked with
`=== runtime start … ===`; the subscriber is re-attached on each restart and never rotates
the file. Use `--runtime-log <path>` to relocate it or `--runtime-log -` to turn it off.

## External browser preview (`--hub`)

> **Linux builds only.** `--hub` and `mxcli tunnel-hub` are available in the
> **Linux** build of mxcli only.
>
> The tunnel exists to get a preview *out of a Linux container*, which is the only
> place it ever ran. Shipping it in the Windows and macOS binaries meant those
> binaries embedded a general-purpose tunnelling tool they could never use — and
> endpoint security noticed: Microsoft Defender flagged the Windows binary, and
> enterprise EDR (Defender for Endpoint, CrowdStrike, SentinelOne) flags this class
> of payload harder still. That blocked mxcli on exactly the managed corporate
> laptops most Mendix developers work on.
>
> So it is built for Linux only. On Windows and macOS the commands still exist and
> still show help, but fail with a message pointing you here. To use `--hub`, run
> mxcli **inside the project's devcontainer** (or any Linux container) — which is
> where the warm loop already runs. Everything else in `mxcli run --local` is
> unaffected.
>
> We deliberately did **not** hide the dependency to dodge the scanners; that would
> be dishonest and would make the binary less trustworthy, not more. The fix is not
> shipping the capability where it is not used. See
> [ADR-0009](https://github.com/mendixlabs/mxcli/blob/main/docs/13-decisions/0009-tunnel-is-linux-only.md).


`--hub <url>` makes the running app reachable **in a browser at a public URL** — without
the app leaving this machine and without committing. It's for reviewing work-in-progress
from a phone or tablet, or from an egress-only environment such as Claude Code on the web.
The app stays local and a **reverse tunnel** dials *out* to a hub over 443; the hub
proxies browser requests back down the tunnel. Nothing is pushed — only live HTTP — and
because everything rides a single 443 connection, it works even from an egress-only proxy.

**You run your own hub — there is no hosted service.** Stand up `mxcli tunnel-hub` once on
a host you control (a small VPS with a domain), then point apps at it.

```bash
# on your VPS: *.example.com + hub.example.com -> this host, inbound 80+443 open
mxcli tunnel-hub --domain example.com --secret alice:s3cret

# where the app runs:
mxcli run --hub https://hub.example.com --hub-secret alice:s3cret -p app.mpr
#   -> registers and prints e.g. "Preview available at https://app.example.com"
```

The hub is **multi-tenant**: it fronts many previews at per-preview subdomains
(`<project>-<branch>.example.com`; `main`/`master` collapses to `<project>`) — across
projects, solutions, branches, and worktrees — with a sortable overview at
`https://hub.example.com/`. Each `run --hub` self-registers:

- **Project** and **branch** auto-detect from the `.mpr` name and git; override with
  `--hub-project`/`--hub-branch`, and `--hub-worktree` separates worktrees of one branch.
- **`--hub-prefix`** namespaces the hostname (org/solution/team/env) →
  `<prefix>-<project>-<branch>`; **`--hub-solution`** groups a solution's apps in the overview.
- The overview **groups previews by Claude Code session** (agent): each session lists the
  endpoints it exposed, links back to its `claude.ai/code` conversation, and shows its
  availability — a reaped/idle container turns **stale**, then **offline**. `run --hub`
  auto-detects the session from `CLAUDE_CODE_REMOTE_SESSION_ID` (override with
  `--hub-session` / `MXCLI_HUB_SESSION`). Past sessions are **retained** so you can see
  older ones: the hub persists a per-session endpoint history to `--sessions-file` (default
  `~/.mxcli/hub-sessions.json`, survives restarts) and prunes it after `--session-retention`
  (default 30 days). Re-registering keeps a **stable URL**.
- Each endpoint shows **first seen** alongside last-seen, last-used, and uptime. First seen
  is read from that persisted history, so it is the first time the endpoint was *ever*
  exposed — unlike uptime, it does not reset when an idle container is reaped and the
  preview reconnects.
- `--hub` **implies `--local`**, boots the runtime with `ApplicationRootUrl` set to the
  assigned URL (so the SPA and `originURI` cookie work), and the tunnel reconnects forever.
  Combine with `--watch` for the full remote loop: edit here → hot-apply → refresh the tab.

**Hub setup:** a wildcard `*.example.com` A record (and `hub.example.com`) pointed at the
VPS; inbound 80 + 443 open — a Let's Encrypt cert is issued per subdomain on demand.

### Authenticated hub (GitHub)

A hub started **without** the GitHub flags is open — the shared `--secret` gates
registration, so keep it to people you trust. A hub started **with** GitHub OAuth adds
per-user isolation: viewers sign in with GitHub and see only their own previews, and each
preview is owned by whoever registered it.

```bash
# create a GitHub OAuth App (callback https://hub.example.com/auth/github/callback), then:
mxcli tunnel-hub --domain example.com --secret alice:s3cret \
  --github-oauth-client-id <id> --github-oauth-client-secret <secret> \
  --session-secret "$(openssl rand -hex 32)" --audit-log ~/.mxcli/hub-audit.jsonl
```

With auth on, `--require-auth` (default) 302s an unauthenticated viewer to GitHub and
returns a 403 to a non-owner; `--require-auth=false` filters the admin listing but leaves
previews open. Hub API keys are stored durably (`--keys-file`, default
`~/.mxcli/hub-keys.json`) so they survive restarts.

**Get a key (any device, including Claude Code web/mobile):** open `https://hub.example.com/cli`,
sign in with GitHub, click **Create a hub key**, and set it as an environment/repo secret:

```bash
export MXCLI_HUB_KEY=<key>
mxcli run --hub https://hub.example.com -p app.mpr   # registers previews as you
```

The key is durable and does not expire — set it once. Manage it from the same page ("Revoke
all keys"), or headless with `mxcli auth hub login --token <github-pat>`. If registration
fails (unreachable hub, stale key), `run --hub` warns and continues as a normal local run
instead of aborting.

## The change signal

`--watch` watches two **source** trees and rebuilds when either changes:

- the **model source** — the `.mpr` file and the `mprcontents/` document tree (v2); and
- the **theme source** — `theme/` (app-level `main.scss`, `custom-variables.scss`, …)
  and `themesource/<module>/web/` (per-module SCSS/CSS/JS).

It does **not** watch the whole project dir. This is deliberate: the serve/mxbuild
build rewrites `deployment/`, `theme-cache/`, and `.mendix-cache/` on every run, and
screenshots land in `.mxcli/`; watching only the source keeps that build-output churn
from re-triggering the loop. Both signals are **mtime polling** (default 1 s), so they
work on container filesystems where inotify does not fire — no watcher fd is involved.

Each applied change is logged with a **build generation** counter (`build #2`,
`build #3`, …; the boot build is `#1`), so "did my change take?" is answerable from
the log instead of guessed.

The intended cycle: an agent (or you) edits the model with `mxcli exec`/MDL — or edits
a theme `.scss` — and the running `run --local` picks it up and hot-applies it.

**A rebuild starts once the source stops changing, not on the first change.** An
`mxcli exec` of a real script rewrites the `.mpr` and many `mprcontents/*.mxunit` files
over several seconds; building on the first change would deploy whatever was on disk at
that instant — a half-applied model. The watcher therefore waits for a couple of quiet
polls before it builds, so a long `exec` produces one build of the finished model
rather than a build of the first file it touched.

This matters more than it used to. The old escape hatch was "run the script again", and
byte-idempotent `exec` closed it: re-running an already-applied script writes nothing,
so nothing re-triggers the watcher and the stale build has no way out. If you do need to
force a rebuild without changing anything, `touch` the `.mpr` — the signal is mtime.

## Editing themes (SCSS): rebuild, don't clear caches

A theme edit (e.g. `theme/web/main.scss`) needs a **rebuild**, not a cache-clear.
`mxbuild --serve` recompiles the theme on its next `/build`, and that recompile
correctly picks up SCSS **content** changes — there is no incremental-theme cache to
clear (verified: one `/build` after an `main.scss` content edit changes
`theme-cache/web/theme.compiled.css`). So:

- **With `--watch`** — just save the `.scss`; the theme source is watched and the loop
  rebuilds and hot-applies automatically.
  One exception, in `mxbuild --serve` itself: after a theme **error**, the same serve
  process keeps reporting that first Sass error — as `[CE6093] The theme could not be
  compiled`, quoting a line no longer in the file — on every later build, even once the
  SCSS compiles (measured on 11.12.5; the bundled Dart Sass compiles the same source
  cleanly). So when a theme failure follows another one, the loop restarts
  `mxbuild --serve` and builds once more, printing *"the theme error may be stale …
  restarting mxbuild --serve"*. A theme error that survives the restart is the source's
  own, and is the one reported. The fresh build cannot tell a theme-only change from a
  model change, so that apply restarts the runtime.
- **Without `--watch`** — nothing watches anything, so a save changes nothing in the
  running app. Trigger a rebuild: restart `run --local`, or use `--watch`.

Do **not** `rm -rf theme-cache/ .mendix-cache/ deployment/` — clearing caches is a red
herring. If a theme edit "won't show up", the cause is that no rebuild ran (Problem
above) or a **stale process is still serving** (below), never a stale compiled-CSS
cache.

## "My edit didn't show up" — it's usually a stale process, not a cache

`run --local` refuses to boot if its ports (`8080` app, `8090` admin, `6543` serve)
are already answering — because a previous `run --local` (or a stray `mxbuild --serve`
/ runtime) left alive would otherwise be **silently adopted**: the startup readiness
probes only check that the port answers, so a fresh run would attach to the old
process and keep serving old output. That reads exactly like a stale cache but is a
stale **process**.

If you started `run --local` in the background and the wrapping shell exited non-zero
(e.g. a chained `sleep`/`curl` that failed), the `run --local` process can die while
its `mxbuild --serve` + runtime keep serving on `:8080`. Launch `run --local` as the
**sole** command in its own invocation — don't chain a `sleep`/status check after it in
the same shell — and poll separately.

**The refusal names the offending process.** On Linux mxcli resolves the port's
listener through `/proc` and prints its pid and command line, so recovery is one
command rather than a `pgrep` hunt:

```
port 8080 (app) is already in use.
  A stale process is silently adopted otherwise, so edits appear to do nothing
  (looks like a stale cache — it isn't).
  Held by pid 11893: /root/.mxcli/mxbuild/11.13.0/modeler/mxbuild --serve …
  That is a leftover from an earlier run that did not shut down cleanly
  (a kill -9 or a reaped container skips mxcli's own teardown).
    kill 11893
    # confirm it is gone: curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080   (want 000)
  Or run on different ports with --app-port (and --admin-port/--serve-port).
```

The two cases need opposite remedies, so they are reported differently. A **leftover
of a previous run** is safe to kill, as above. A **foreign** listener — anything mxcli
did not start — is not, and mxcli says so instead of offering a `kill`:

```
  Held by pid 4820: python3 -m http.server 8080
  That is not a process mxcli started, so it is not a leftover run —
  pick another port rather than killing it.
```

Off Linux, or when the listener belongs to another user (so `/proc/<pid>/fd` cannot be
read), it falls back to a generic hint.

A *graceful* stop needs none of this: each child — mxbuild's JVM, the runtime, the
rollup bundler — is started in its own process group and the whole group is killed on
Ctrl-C/SIGTERM. Seeing this error means the previous run never got to run its teardown:
`kill -9`, a crash, or a reaped container. Avoid `pkill -f 'mxcli run'` — that pattern
also matches the shell you type it into.

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

**Mendix 11.14+ bundles the client itself**, so there is no rollup step to run and
no bundler to keep hot — `run --local` prints a line saying so and skips it. Both
paths work; the bundle is mxbuild's rather than mxcli's.

**Classic-client apps need no bundling at all.** With **Web UI Settings >
OptimizedClient = No**, mxbuild puts Mendix's classic (Dojo) client in
`deployment/web/` — loaded from `mxclientsystem` and served as-is — and parks the
React client in `deployment/react-web/`. (With `Yes` it is the other way round:
React in `web/`, Dojo in `dojo-web/`.) There is no rollup step and no `web/dist`,
so `run --local` says so and skips it, under `--watch` too. mxcli reads this from
the deployment's own `web/index.html` rather than from the model setting, since
the deployment is what gets served and the two disagree right after the setting
is changed.

### `--watch` when mxbuild writes the client (11.12.5, 11.14)

Where mxbuild bundles the client itself, the **first** build in an `mxbuild --serve`
process is the only complete one: it does not leave behind the client source
(`web/pages/`, `web/layouts/`) or the bundler config its own incremental build
needs. Every later build in the same process then goes one of two ways:

- **A change that regenerates a page or layout fails** inside mxbuild:

  ```
  Could not find a part of the path '…/deployment/web/pages/MyModule.Home_Web.js'
  ```

  (on 11.14 a change that touches no page fails too, with
  `Cannot find module '…/deployment/web/rollup.config.mjs'`).
- **Any other change — even no change at all — succeeds with an incomplete
  bundle** (11.12.5): `deployment/web/dist` is replaced by a monolithic `index.js`
  and one chunk, with no `pages/` and no `widgets.css`. The reload is reported as
  applied and every page fails to load.

Measured against mxbuild 11.12.5 over its own HTTP API, no watch loop involved:

| Build | `restartRequired` | `dist/` after it |
|---|---|---|
| 1, cold | true | 13 pages, `widgets.css`, 5.6 KB `index.js` |
| 2, same process, model unchanged | false | 0 pages, no `widgets.css`, 683 KB `index.js` |
| 3, same process, after `rm -rf web/dist` | false | same as 2 |
| 4, same process, after `rm -rf web/` | build fails | — |
| 5, fresh process | true | complete again |

Nothing done to the deployment helps (also measured on 11.14: switching the app
bundler to Rspack, deleting `deployment/`, restoring the bundler config), and a
fresh process always does — a one-shot `mxbuild --target=deploy` run twice into
the same directory succeeds both times. So under `--watch` mxcli **restarts
`mxbuild --serve` and builds again** when a build fails this way or leaves
`dist/` without its pages, and says so in one line:

```
mxbuild --serve's incremental build left web/dist without dist/pages (13 page modules) and dist/widgets.css (…); restarting mxbuild --serve for a full build...
```

A change then costs ~25–30 s instead of ~5 s. It is still hot-applied: the
incremental build's `restartRequired` is kept, because a fresh process's first
build reports `true` even for an unchanged model, and taking it would restart the
runtime — dropping every browser session — on every change. Measured end to end
on 11.12.5; on 11.14 the same restart applies to the same failure shape.

**Which versions.** The two halves have different fates (ako/mxcli#606). The
bundler-config failure (`Cannot find module '…/rollup.config.mjs'`) is **fixed in
Mendix 11.15.0**: a project converted from 11.14.0 to 11.15.0 rebuilt warm in
2.8–3.9 s per change, where 11.14.0 failed every build after the first — so on
11.14 the remedy is to upgrade. The per-document client failure (`web/pages/…`)
was still seen on macOS 11.15.0, where mxcli's serve restart is what keeps the
loop going. The hint mxcli prints offers the upgrade only for the half it fixes.
`mxcli test --attach` rebuilds through the hosting app's own serve process, so it
meets the same second-build failure on the same versions (read off the code, not
measured).

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
