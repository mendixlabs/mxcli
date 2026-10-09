---
name: docker-workflow
description: "Build, run and validate a Mendix app in Docker from inside a devcontainer. Use when asked to run the app in Docker or produce a deployable image. For the faster Docker-free loop, prefer run-local."
---

# Docker Build & Run Skill

This skill guides you through building, running, and testing a Mendix application using Docker inside a devcontainer.

## When to Use This Skill

Use this when:
- The user wants to run their Mendix app locally in Docker
- The user wants to build a deployable container image
- The user asks about testing or validating their app
- The user needs to set up the Docker workflow for the first time

## Prerequisites

The devcontainer created by `mxcli init` includes:
- **JDK 21** (Adoptium temurin-21-jdk) — required by MxBuild
- **Docker-in-Docker** (or **Podman-in-Podman**) — container runtime inside the devcontainer
- **Port forwarding** — ports 8080 (app) and 8090 (admin) auto-forwarded

### Podman Support

mxcli auto-detects Docker or Podman. To force Podman:
```bash
export MXCLI_CONTAINER_CLI=podman
```

When using `mxcli init`, pass `--container-runtime podman` to generate a devcontainer with Podman-in-Podman instead of Docker-in-Docker. Requires Podman 4.7+ (ships `podman compose` natively).

## Architecture

```
host machine (browser at localhost:8080)
  └── Docker or Podman (host daemon)
      └── Devcontainer (VS Code)
          ├── mxcli, JDK 21, project files
          └── Docker/Podman daemon (docker-in-docker or podman-in-podman)
              └── docker/podman compose stack
                  ├── mendix container (8080, 8090)
                  │     └── /mendix ← volume mount from .docker/build/
                  └── postgres container (5432)
```

The Mendix container uses a **volume mount** (not COPY) — `.docker/build/` is mounted directly into the container at `/mendix`. This means after rebuilding the PAD, you only need to restart the container (no Docker image rebuild). The generated Dockerfile is kept in the build output for production image builds.

## Quick Start: One Command

The easiest way to get a Mendix app running in Docker:

```bash
# Setup, build, and start in one command
mxcli docker run -p app.mpr

# with startup confirmation (waits for "runtime successfully started")
mxcli docker run -p app.mpr --wait

# Fresh start (removes database volumes first)
mxcli docker run -p app.mpr --fresh --wait
```

`docker run` handles everything: downloads MxBuild and runtime (if not cached), initializes the Docker stack (if needed), builds the PAD package, starts the containers, and optionally waits for the runtime to report successful startup.

## Creating an Empty Mendix App

**Recommended:** Use `mxcli new` to create a project with all tooling in one step:

```bash
mxcli new MyApp --version 11.8.0
```

This downloads MxBuild, creates a blank project, sets up AI tooling and Dev Container, and installs the correct Linux mxcli binary. Open the resulting folder in VS Code and reopen in Dev Container.

**Manual approach** (if you need more control):

```bash
# Download mxbuild if not already cached
mxcli setup mxbuild --version 11.6.4

# create a blank project
mkdir -p /path/to/my-app
~/.mxcli/mxbuild/{version}/modeler/mx create-project --app-name MyApp --output-dir /path/to/my-app

# Initialize AI tooling
mxcli init /path/to/my-app
```

The `mx create-project` command creates an MPR v2 project with the standard Mendix module structure. You can then use the Docker workflow to build and run it.

**Caveat:** Blank projects have no demo users — login will fail until you configure security via MDL or Studio Pro. See `manage-security` for setting up demo users.

## Step-by-Step Workflow

If you prefer more control, use the individual commands:

### 1. Setup MxBuild and Runtime (first time only)

MxBuild and the Mendix runtime are auto-downloaded when needed, but you can pre-download:

```bash
# Download MxBuild matching the project version
mxcli setup mxbuild -p app.mpr

# Download Mendix runtime matching the project version
mxcli setup mxruntime -p app.mpr

# or specify version explicitly (without a project)
mxcli setup mxbuild --version 11.6.4
mxcli setup mxruntime --version 11.6.4

# or preview what would be downloaded
mxcli setup mxbuild -p app.mpr --dry-run
mxcli setup mxruntime -p app.mpr --dry-run
```

MxBuild is cached at `~/.mxcli/mxbuild/{version}/` and the runtime at `~/.mxcli/runtime/{version}/`. Both are reused across builds.

#### Runtime-to-MxBuild Copying (PAD Build Prerequisite)

MxBuild 11.6.3+ expects runtime files (`pad/`, `lib/`, `launcher/`, `agents/`) inside its own `runtime/` directory, but `mxcli setup mxbuild` only downloads the build tools (not the full runtime). If the PAD build fails with `StudioPro.conf.hbs does not exist` or `ClassNotFoundException`, copy the runtime directories into mxbuild:

```bash
version=11.6.4  # replace with your version

# after downloading both mxbuild and mxruntime:
cp -r ~/.mxcli/runtime/$version/runtime/pad ~/.mxcli/mxbuild/$version/runtime/pad
cp -r ~/.mxcli/runtime/$version/runtime/lib ~/.mxcli/mxbuild/$version/runtime/lib
cp -r ~/.mxcli/runtime/$version/runtime/launcher ~/.mxcli/mxbuild/$version/runtime/launcher
cp -r ~/.mxcli/runtime/$version/runtime/agents ~/.mxcli/mxbuild/$version/runtime/agents
```

**Important:** The PAD build output may only include partial runtime bundles (5 jars instead of 354). If the runtime fails to start with `ClassNotFoundException: com.mendix.container.support.EventProcessor`, copy the full runtime into the PAD build output:

```bash
rm -rf /path/to/project/.docker/build/lib/runtime
cp -r ~/.mxcli/runtime/$version/runtime /path/to/project/.docker/build/lib/runtime
```

### 2. Initialize Docker stack (first time only)

```bash
# generate docker-compose.yml, .env, Dockerfile in .docker/
mxcli docker init -p app.mpr
```

This creates a `.docker/` directory with Docker Compose configuration for the Mendix app + PostgreSQL.

**Stack name:** `docker init` writes a Compose project name into `.docker/docker-compose.yml` (`name: "mxcli-<app folder>"`), so containers and volumes of different apps are separate (`mxcli-myapp-db-1`, `mxcli-myapp_postgres-data`). Never change `name:` on a stack that holds data you need: Compose then starts new containers with an empty database and leaves the old volume behind. Projects initialised by older mxcli have no `name:`, so Compose calls them `docker` (`docker-db-1`, `docker_postgres-data`), shared with every other such project; `docker init --force` keeps that name rather than stranding the data.

**Port conflicts:** If default ports (8080/8090/5432) are already in use, check with `ss -tlnp | grep -E '808|809|543'` and use `--port-offset N` to shift all ports:

```bash
# check which ports are occupied
ss -tlnp | grep -E '808[0-9]|809[0-9]|543[0-9]'

# use offset to avoid conflicts (e.g., offset 5 → 8085/8095/5437)
mxcli docker init -p app.mpr --port-offset 5
```

### 3. Check project for errors

```bash
# Quick validation using mx check
mxcli docker check -p app.mpr
```

### 4. Build the Portable App Distribution (PAD) package

```bash
# build (auto-downloads MxBuild if not cached)
mxcli docker build -p app.mpr

# Preview what would happen
mxcli docker build -p app.mpr --dry-run

# Skip pre-build check
mxcli docker build -p app.mpr --skip-check
```

This:
1. Detects the Mendix version (requires >= 11.6.1)
2. Locates or downloads MxBuild and JDK 21
3. Runs MxBuild to produce a PAD package (ZIP)
4. Extracts the PAD ZIP (cleans up old ZIPs after extraction)
5. Generates a Dockerfile if MxBuild didn't produce one (11.6.3+)
6. Downloads and injects the Mendix runtime if not in PAD output
7. Applies version-aware patches (CMD fix, base image, healthcheck, etc.)

### 5. Start the application

```bash
# Start in foreground (see logs directly)
mxcli docker up -p app.mpr

# Start in background
mxcli docker up -p app.mpr --detach

# Start in background and wait for runtime startup confirmation
mxcli docker up -p app.mpr --detach --wait

# Fresh start (removes database volumes)
mxcli docker up -p app.mpr --fresh

# Custom wait timeout (default: 300 seconds)
mxcli docker up -p app.mpr --detach --wait --wait-timeout 600
```

The `--wait` flag (requires `--detach`) tails the container logs and waits until the Mendix runtime reports "Runtime successfully started" or a timeout/failure is detected.

The app is available at:
- **http://localhost:8080** — application
- **http://localhost:8090** — admin console (password in `.docker/.env`)

### 6. Query data with OQL

```bash
# run OQL queries against the live runtime (read-only preview mode)
mxcli oql -p app.mpr "select Name, Email from MyModule.Customer"

# json output for piping
mxcli oql -p app.mpr --json "SELECT count(c.ID) FROM MyModule.Order AS c" | jq '.[0]'

# Test a view entity query before embedding it in MDL
mxcli oql -p app.mpr "select datepart(YEAR, o.Date) as Year, sum(o.Total) as Revenue from Sales.Order as o GROUP by datepart(YEAR, o.Date)"
```

### 7. Monitor and manage

```bash
# view container status
mxcli docker status -p app.mpr

# view logs
mxcli docker logs -p app.mpr
mxcli docker logs -p app.mpr --follow
mxcli docker logs -p app.mpr --tail 50

# open a shell in the container
mxcli docker shell -p app.mpr
mxcli docker shell -p app.mpr --exec "ls -la /mendix"
```

### 8. Stop the application

```bash
# Stop containers
mxcli docker down -p app.mpr

# Stop and remove database volumes
mxcli docker down -p app.mpr --volumes
```

## Common Workflow: Edit, Rebuild, Test

After making MDL changes:

```bash
# 1. apply MDL changes
mxcli exec changes.mdl -p app.mpr

# 2. Rebuild and restart (one command)
mxcli docker run -p app.mpr --fresh --wait
```

Or step by step:

```bash
# 1. apply MDL changes
mxcli exec changes.mdl -p app.mpr

# 2. Validate
mxcli docker check -p app.mpr

# 3. Rebuild
mxcli docker build -p app.mpr

# 4. restart with fresh database and wait for startup
mxcli docker up -p app.mpr --fresh --detach --wait

# 5. check it's running
mxcli docker status -p app.mpr
```

## Hot Reload

The Mendix runtime supports hot reloading via the M2EE admin API. Because mxcli's Docker setup uses a **bind mount** (`.docker/build/` → `/mendix/`), rebuilt PAD output is immediately visible to the running runtime — no Docker image rebuild or container restart needed.

### How It Works

1. `mxcli docker build` compiles the project into `.docker/build/` (the bind-mounted directory)
2. `mxcli docker reload` rebuilds the PAD (same as `docker build`) **then** calls the M2EE `reload_model` action on port 8090
3. The runtime re-reads the model from disk in ~100ms
4. Connected browsers auto-refresh via the `/mxdevtools/` WebSocket

**Note:** By default, `docker reload` includes a full build step (~55s). Use `--model-only` to skip the build when you've already run `docker build` separately:

**Typical hot reload cycle (with separate build):**
```
mxcli exec script.mdl -p app.mpr                   # ~1s   — update model
mxcli docker build -p app.mpr                       # ~55s  — compile PAD
mxcli docker reload -p app.mpr --model-only         # ~100ms — reload only
# Total: ~56s (vs ~75s with full container restart)
```

**One-step reload (build + reload combined):**
```
mxcli exec script.mdl -p app.mpr                    # ~1s   — update model
mxcli docker reload -p app.mpr                      # ~56s  — build + reload
```

### Model Reload

For logic changes (microflows, nanoflows, pages, security):

```bash
# 1. apply changes
mxcli exec changes.mdl -p app.mpr

# 2. Rebuild PAD (skip pre-check for speed)
mxcli docker build -p app.mpr --skip-check

# 3. Hot reload the runtime (--model-only skips redundant rebuild)
mxcli docker reload -p app.mpr --model-only
```

Or combine build + reload in one command (no `--model-only`):

```bash
# 1. apply changes
mxcli exec changes.mdl -p app.mpr

# 2. build and reload in one step
mxcli docker reload -p app.mpr --skip-check
```

The `--direct` flag connects to the admin API via HTTP instead of routing through `docker compose exec` (faster, requires the `admin.addresses = ["*"]` build patch which is applied automatically):

```bash
mxcli docker reload -p app.mpr --direct
```

### CSS-Only Reload

For theme/styling changes, SCSS must first be compiled by MxBuild into the PAD output. The `--css` flag skips the model reload step — it only pushes already-compiled CSS to browsers via WebSocket.

```bash
# Correct workflow for SCSS/theme changes:
mxcli docker build -p app.mpr            # compile SCSS into PAD (~55s)
mxcli docker reload -p app.mpr --css     # push compiled CSS to browsers (~instant)
```

> **Note:** `--css` does NOT compile SCSS. If you skip the build step, the browser will not reflect your SCSS changes. The "instant" refers to the browser update via WebSocket, not end-to-end time.

This calls the M2EE `update_styling` action, which pushes CSS changes to all connected browsers via the `/mxdevtools/` WebSocket. Browsers update their stylesheets without a full page reload.

### The mxdevtools WebSocket

The Mendix runtime exposes a WebSocket at `ws://localhost:8080/mxdevtools/` (on the app port). This is a **server-push-only** channel — the runtime pushes instructions to connected browsers:

| Instruction | Effect |
|-------------|--------|
| `set_deployment_id` | If the ID changes, triggers full browser reload |
| `reload` | Forces full browser reload |
| `update_styling` | Hot-reloads CSS without page reload |

When `reload_model` is called, a new `set_deployment_id` is pushed to browsers, triggering automatic refresh. This is why you don't need to manually refresh the browser after `docker reload`.

### When to Use `reload` vs `run`

| Scenario | Command | Why |
|----------|---------|-----|
| Microflow/nanoflow logic | `docker reload` | No schema change, keeps data |
| Page layout or widget changes | `docker reload` | Runtime reloads pages from model |
| CSS/theme changes only | `docker reload --css` | Instant, no MxBuild needed |
| Security rule changes | `docker reload` | Runtime reloads security config |
| New entity or attribute (additive) | `docker reload` | Runtime applies DDL on reload |
| Destructive schema change (drop column, type change) | `docker up --fresh` | Runtime can't apply destructive DDL |
| First-time setup | `docker run` | Need containers + database |
| Database corruption or reset | `docker run --fresh` | Recreates volumes |

### When Reload is NOT Sufficient

`reload_model` cannot handle destructive database schema changes. Signs you need a full restart with `--fresh`:

- **Removed an entity** — the runtime won't drop the table
- **Removed an attribute** — the runtime won't drop the column
- **Changed an attribute type** (e.g., String → Integer) — the runtime can't ALTER COLUMN type
- **Runtime logs show DDL errors** — look for "Could not execute DDL" or schema mismatch messages

In these cases:

```bash
mxcli docker up -p app.mpr --fresh --detach --wait
```

## Environment Variables

The `docker-compose.yml` generated by `mxcli docker init` sets these environment variables for the Mendix container:

| Variable | Default | Description |
|----------|---------|-------------|
| `ADMIN_ADMINPASSWORD` | `AdminPassword1!` | Admin console password (read by runtime) |
| `RUNTIME_DEBUGGER_PASSWORD` | `AdminPassword1!` | Debugger password (required in non-Development mode) |
| `RUNTIME_PARAMS_DATABASETYPE` | `POSTGRESQL` | Database type |
| `RUNTIME_PARAMS_DATABASEHOST` | `db:5432` | Hostname and port of the `db` service |
| `RUNTIME_PARAMS_DATABASENAME` | `mendix` | Database name |
| `RUNTIME_PARAMS_DATABASEUSERNAME` | `mendix` | Database user |
| `RUNTIME_PARAMS_DATABASEPASSWORD` | `mendix` | Database password |
| `MX_LOG_LEVEL` | `info` | Log level |

All defaults can be overridden in `.docker/.env`.

**Note**: The runtime reads `ADMIN_ADMINPASSWORD`, not `M2EE_ADMIN_PASS`. The `.env` file defines `M2EE_ADMIN_PASS` as the single source, and `docker-compose.yml` maps it to both `ADMIN_ADMINPASSWORD` and `RUNTIME_DEBUGGER_PASSWORD`.

## Troubleshooting

| Problem | Solution |
|---------|----------|
| `docker: command not found` | Rebuild devcontainer — docker-in-docker feature needs rebuild to activate. Or use Podman: `export MXCLI_CONTAINER_CLI=podman` |
| `mxbuild not found` | Run `mxcli setup mxbuild -p app.mpr` to download from CDN |
| `JDK 21 not found` | Rebuild devcontainer — JDK 21 should be pre-installed |
| Build fails with version error | Requires Mendix >= 11.6.1 for PAD support |
| No Dockerfile in PAD output | Normal for MxBuild 11.6.3+ — `mxcli docker build` auto-generates one |
| Runtime not found / runtimelauncher.jar missing | Run `mxcli setup mxruntime -p app.mpr` or let `docker build` auto-download |
| `StudioPro.conf.hbs does not exist` | Runtime not linked into mxbuild — see "Runtime-to-MxBuild Copying" above |
| `ClassNotFoundException: EventProcessor` | PAD has partial runtime bundles — copy full runtime into `.docker/build/lib/runtime/` (see above) |
| Port already allocated | Check ports with `ss -tlnp \| grep 808` and use `docker init --port-offset N --force` |
| `' etc/default' is not a file` | Dockerfile CMD passes config arg — `docker build` patches this automatically |
| `DatabasePassword has no value` | Ensure `RUNTIME_PARAMS_DATABASE*` env vars are in docker-compose.yml — re-run `mxcli docker init --force` |
| `password should not be empty (debugger)` | Add `RUNTIME_DEBUGGER_PASSWORD` — re-run `mxcli docker init --force` |
| `security level should be set to CHECKEVERYTHING` | App in Production mode without security — set to Development mode or configure security |
| Port 8080 not accessible | Check `forwardPorts` in devcontainer.json includes 8080 |
| Two projects overwrite each other's containers or share one database (both show as project `docker`) | Old compose file without `name:`. Back up the data, set a unique `name:` at the top of `.docker/docker-compose.yml` (e.g. `mxcli-myapp`), then `docker compose up`; the new stack starts with an empty database |
| Database errors on startup | Try `mxcli docker up -p app.mpr --fresh` to reset volumes |
| OQL: "Action not found: preview_execute_oql" | Runtime needs `-Dmendix.live-preview=enabled` JVM flag — re-run `mxcli docker init --force` to get the updated docker-compose.yml |
