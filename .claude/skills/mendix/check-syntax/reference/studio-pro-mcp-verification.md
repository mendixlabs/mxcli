# Studio Pro MCP — Verification Only

When Studio Pro's embedded MCP server is available **alongside** mxcli (i.e. you are
*not* using mxcli's own `--mcp` backend, but mxcli writes the `.mpr` directly while
Studio Pro is open), use Studio Pro MCP **only for reading and verification**. Never
author with `ped_create_document` / `ped_update_document` — mxcli owns the `.mpr`, and
mixing authoring tools corrupts intent and diverges UUIDs.

## Role split

| Task | Tool |
|------|------|
| Create/modify entities, microflows, pages, nanoflows, nav | mxcli MDL |
| Verify CE errors after exec | `ped_check_errors` |
| Inspect widget tree / microflow body detail | `ped_read_document` |
| Check if a document exists before creating | `ped_find_document` |
| Full model validation | `./mxcli docker check` |

## Studio Pro reads its in-memory model, not the file — a flush IS needed

This is the opposite of what you might expect. mxcli writes directly to the `.mpr`
SQLite file, but **Studio Pro serves `ped_*` reads from its in-memory model**, which
does not hot-reload when an external process changes the file. So after `mxcli exec`:

- `ped_read_document` / `ped_check_errors` will show the **stale** pre-exec model until
  Studio Pro re-scans — call `refresh_project` first (or reload the project in the UI).
- **Hazard:** if Studio Pro later saves on its own, it overwrites mxcli's disk write with
  its in-memory copy, silently discarding your MDL changes. So a file-based write is
  **refused** while Studio Pro's `<project>.mpr.lock` is beside the `.mpr`; `exec --force`
  (or `MXCLI_ALLOW_STUDIO_PRO_OPEN=1`) overrides it, e.g. for a lock left by a crash.

**Safest practice:** don't keep the same project open-and-saving in Studio Pro while
mxcli writes it. Either close (or don't save in) Studio Pro during MDL authoring, or
`refresh_project` after every `mxcli exec` before verifying. If you need both writing
*and* a live Studio Pro, use mxcli's `--mcp` backend (which authors *through* Studio
Pro) instead of writing the file directly.

## Post-execution verification (step 6 of the validation workflow in SKILL.md)

After `./mxcli exec script.mdl -p app.mpr` succeeds:

1. `refresh_project` (Studio Pro MCP) so the in-memory model reflects the new file.
2. `ped_check_errors` on each created/modified document for CE errors. The argument
   depends on the Studio Pro release — read the tool's input schema:
   - up to 11.14: `{"documents": [{"documentType": "Microflows$Microflow", "documentName": "Mod.Name"}]}`
   - 11.15+: `{"filters": {"documentType": "Microflows$Microflow", "documentNamePrefix": "Mod.Name"}}`.
     11.15 **silently ignores** `documents` and checks the whole project. `documentNamePrefix`
     is a prefix (`Mod.Order` also matches `Mod.OrderLine`), so read the `'Mod.Name' (Type):`
     header above each problem. More than 100 problems come in pages: repeat the same
     `filters` with `pagination: {checkId, offset, size}` using the `Check ID` from the first answer.

> **Do not treat an empty `DESCRIBE` as proof of a dropped construct.** `DESCRIBE`
> renders from the MDL emitter, which does not yet render every activity/widget type
> (e.g. Java/JavaScript action calls, exclusive splits, some nanoflow buttons). The
> construct may be present in the model even when `DESCRIBE` omits it. To tell a real
> write-drop from an emitter gap, confirm with `ped_read_document` (the live model) or
> `./mxcli docker check` — only flag an engine bug once the live model is also missing it.
