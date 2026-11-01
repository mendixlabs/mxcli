# Connecting to a Running Studio Pro (MCP)

> **Experimental.** The MCP backend targets Studio Pro's embedded MCP server,
> introduced in **Studio Pro 11.11**. Tooling and transport details may change
> between Studio Pro versions — verify with `mxcli mcp capabilities` (below).

Normally mxcli edits a project by writing its `.mpr` file directly. With `--mcp`
it instead routes every **model write** to a *running* Studio Pro over that
editor's embedded MCP server ("PED"), so your changes appear in the open model
**immediately — no save, no reload**. Reads (`SHOW`/`DESCRIBE`) still come from the
local `.mpr` you pass with `-p` (the same file Studio Pro has open).

This is the live-editing counterpart to the file backend. For the cost trade-offs
of routing through MCP, see
[MCP Backend: Script-to-Tool-Call Cost](../internals/mcp-backend-cost.md).

## What you need

1. **Studio Pro 11.11 or later**, running, with the project **open**.
2. The **same project file** passed to mxcli via `-p` — reads come from there, so
   it must match what Studio Pro has open.
3. A reachable MCP endpoint. The server binds **IPv6 loopback only** (`[::1]:7782`
   observed) and enforces a DNS-rebinding guard: the `/mcp` route only accepts
   requests whose HTTP `Host` header is exactly `localhost`. mxcli handles the Host
   header for you; you just have to make the TCP port reachable (see below).

> The listening **port can change between Studio Pro sessions**. Confirm it on the
> machine running Studio Pro, e.g. `lsof -nP -iTCP -sTCP:LISTEN | grep -i studio`.

## The flags

| Flag | Purpose |
|---|---|
| `--mcp <url>` | Enable the MCP backend. The URL is what the **server** must see; its host drives the `Host` header and must stay `localhost` — e.g. `http://localhost/mcp`. |
| `--mcp-dial <addr>` | The TCP address mxcli actually connects to, independent of the URL host. From a devcontainer this is typically `host.docker.internal:<port>`. |
| `--mcp-check` | After the command, run Studio Pro's project-wide error check (`ped_check_errors`) and print the report. Needs Studio Pro 11.15+. |
| `--mcp-run` | After the command, start the app in Studio Pro (the `mendix_app` handler) and print its URL. Needs Studio Pro 11.16+. |
| `--mcp-verbose` | Print each PED tool call the backend makes (to stderr). |
| `--mcp-trace` | Print each MDL command with the PED tool calls it makes (implies `--mcp-verbose`). |

If `--mcp-dial` is omitted, mxcli connects to the URL's own host first. For a
`localhost`/`127.0.0.1` URL it then falls back to `host.docker.internal:<port>`
when that name resolves — the Docker host, which is where Studio Pro is when
mxcli runs in a devcontainer. Loopback comes first because the Docker name
resolves on more machines than containers: Docker Desktop for Windows adds it to
the hosts file, pointing at the LAN address, where Studio Pro does not listen.
With `--mcp-dial` given, that address is the only one tried.

## From a devcontainer (the common case)

Studio Pro runs on your host; mxcli runs in the container. Because PED listens on
**IPv6 loopback** on the host, the cleanest path is a tiny IPv4→IPv6 bridge on the
**host**, which `host.docker.internal` can then reach.

**Step 1 — bridge the port on the host** (where Studio Pro runs). Forward an IPv4
port (here `7784`) to PED's IPv6 loopback (`7782`):

```bash
# Run this on the HOST, not in the container.
socat TCP4-LISTEN:7784,reuseaddr,fork 'TCP6:[::1]:7782'
```

**Step 2 — verify the connection from the container** with the capability report.
This is the fastest way to confirm the handshake works and to see what the
connected Studio Pro version can author:

```bash
mxcli mcp capabilities -p app.mpr \
  --mcp http://localhost/mcp \
  --mcp-dial host.docker.internal:7784
```

A successful run prints the server name/version, the authorable feature list, and
the live tool set. If you instead see a connection error, jump to
[Troubleshooting](#troubleshooting).

**Step 3 — apply a script against the live model:**

```bash
mxcli exec mdl-examples/doctype-tests/01-mcp-domain-model-examples.mdl \
  -p app.mpr \
  --mcp http://localhost/mcp \
  --mcp-dial host.docker.internal:7784
```

The new entities, enumerations, and associations appear in Studio Pro as the
script runs — no save or reload required.

The same `--mcp` / `--mcp-dial` flags work for a single statement (`-c "…"`) and in
the interactive REPL (`mxcli -p app.mpr --mcp …` with no subcommand) — in every
mode, writes go to Studio Pro while reads come from the local `.mpr`.

## Seeing the tool calls (`--mcp-verbose` / `--mcp-trace`)

Because each MDL statement expands into several PED tool calls, it's useful to
see what actually goes over the wire — for debugging, or just to understand the
[cost](../internals/mcp-backend-cost.md). Two levels, both printed to **stderr**
(so they don't pollute a piped stdout):

- **`--mcp-verbose`** — one line per PED tool call, with its target document:

  ```
  → ped_create_module
  → ped_get_schema
  → ped_update_document McpDm
  → ped_check_errors McpDm
  ```

- **`--mcp-trace`** — groups the calls under the MDL command that triggered them,
  and adds per-call detail (the update operations, schema types, read paths):

  ```
  ▸ create persistent entity McpDm.Customer
    → ped_read_document McpDm: /entities, /associations
    → ped_get_schema: DomainModels$Entity, DomainModels$Attribute
    → ped_update_document McpDm: add /entities
    → ped_get_schema: DomainModels$ValidationRule
    → ped_update_document McpDm: add /entities/0/validationRules ×2
    → ped_check_errors McpDm
  ```

  This makes the statement-to-tool-call mapping (and surprises like a read-back
  reconstructing a dirty module) visible at runtime.

## Delete, check and run

Studio Pro's own MCP server covers what used to need a second server:

- **Saving** needs nothing: PED writes each change to disk as it lands.
- **`DROP`** of a standalone document (enumeration, constant, microflow, nanoflow,
  page, workflow, business-event service) calls `ped_delete_document`, which
  Studio Pro 11.16 added. An older Studio Pro gets an error naming the version.
- **`--mcp-check`** runs `ped_check_errors` over the whole project (11.15+).
- **`--mcp-run`** starts the app through the `mendix_app` handler (11.16+), waits
  up to two minutes and prints its URL. A project with model errors is not run;
  the refusal is printed instead. The runtime log is `read_file /app-logs/app.log`
  on the same server.

```bash
mxcli exec changes.mdl -p app.mpr \
  --mcp http://localhost/mcp --mcp-dial host.docker.internal:7784 \
  --mcp-check --mcp-run
```

## Same machine as Studio Pro

If mxcli runs on the host itself, point `--mcp-dial` at the loopback bridge
directly and keep the `localhost` URL for the Host guard:

```bash
socat TCP4-LISTEN:7784,reuseaddr,fork 'TCP6:[::1]:7782'   # if PED is IPv6-only
mxcli exec changes.mdl -p app.mpr --mcp http://localhost/mcp --mcp-dial 127.0.0.1:7784
```

## What works over MCP

The MCP backend authors a subset of MDL — entities (incl. `extends`, `ALTER`, and
NOT NULL / UNIQUE), associations, enumerations, constants, microflows, pages
(`CREATE` + `ALTER`), and workflows. Some document types (nanoflows, Java actions,
business-event services, **view entities**), security, navigation, and `MOVE` are
**not** authorable over MCP today. Reads of any document type always work (they
come from the local `.mpr`).

`mxcli mcp capabilities -p app.mpr --mcp … --mcp-dial …` reports exactly what your
connected version supports. See also
[Capabilities Overview](../reference/capabilities.md).

## Troubleshooting

| Symptom | Likely cause / fix |
|---|---|
| `connection refused` on dial | The bridge isn't running, or the port is wrong. Re-run `socat` on the host and confirm PED's port with `lsof` (it changes per session). |
| Request rejected / 403-style error | The `Host` header guard. Keep the **URL host `localhost`** (`http://localhost/mcp`) and redirect the connection with `--mcp-dial` — never put the container/host address in the URL itself. |
| Reads don't reflect a change you just made | Expected for some edits until the project is saved — reads of an *unchanged* doc come from the local `.mpr`. A module you just wrote is reconstructed live. |
| `DROP` / `--mcp-check` / `--mcp-run` reports it needs a newer Studio Pro | `DROP` and `--mcp-run` need Studio Pro 11.16+, `--mcp-check` 11.15+. On 11.16.0, `ped_delete_document` fails with `Session 'mcp-server' not found` for callers outside Studio Pro's own agent session; delete the document in Studio Pro. |
| Intermittent timeouts | The transport can be flaky across a bridge; a timed-out write may still have applied. Re-run `mxcli mcp capabilities` to re-establish, and prefer idempotent scripts. |

## See also

- [MCP Backend: Script-to-Tool-Call Cost](../internals/mcp-backend-cost.md) — what
  each statement costs in tool calls, and why running through mxcli is far cheaper
  for an LLM than driving Studio Pro's MCP server directly.
- [Capabilities Overview](../reference/capabilities.md).
