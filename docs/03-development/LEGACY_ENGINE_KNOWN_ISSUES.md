# Legacy Engine — Known Issues

**Status:** tracking list (living document)
**Related:** [ADR-0004: Route all document types through the codec engine](../13-decisions/0004-full-codec-engine.md), [ADR-0002: Backend abstraction](../13-decisions/0002-backend-abstraction.md), [`MODELSDK_ENGINE_ARCHITECTURE.md`](MODELSDK_ENGINE_ARCHITECTURE.md)

## Purpose

The hand-written `sdk/mpr` serializer (the **legacy** engine, selected via
`MXCLI_ENGINE=legacy`) predates the `modelsdk` codec engine, which is now the
default. **The direction is to remove legacy once modelsdk is mature — not to fix
legacy.** This file tracks known legacy-only defects so they are not mistaken for
modelsdk regressions and so the removal can be done with eyes open.

**Do not fix these in `sdk/mpr`.** If a construct is broken on legacy but correct
on modelsdk, that is expected — record it here and move on. Only touch legacy if
it is the *only* engine that can produce a construct at all (see ADR-0004: legacy
remains the fallback for a shrinking set of writes the codec can't yet reproduce)
**and** the breakage blocks that fallback.

When a legacy issue is encountered:

1. Confirm the construct is **correct on modelsdk** (the default) — verify the
   BSON against Studio Pro or the canonical serializer.
2. Add a row below with the symptom, the legacy code path, and how modelsdk
   differs.
3. Do **not** open a fix on the legacy path.

## Known issues

### L1 — Widget association data source writes `EntityRef: nil`

- **Symptom:** a DataView/ListView (and other built-in widgets) with an
  association data source (`datasource: $currentObject/Module.Assoc`) is written
  as `Forms$AssociationSource{ EntityRef: nil }` — the association is dropped, so
  the page loads but the widget has no data source. Legacy does not error at exec
  (that is why it was the "escape hatch" for the modelsdk Bug 4), but the result
  is broken. On a **DataView** the same wrong source type is rejected harder — `mx
  check` fails with **CE6705** "Data view cannot have a data source of type
  association" (a DataView requires `Forms$DataViewSource`, not
  `Forms$AssociationSource`). This is why the `legacy/03-page-examples.mdl` doctype
  variant is in `engineScriptSkip` (the `P_OrderWithCustomer` example uses a
  "data from context" association DataView).
- **Legacy code path:** `sdk/mpr/writer_widgets_display.go` — the
  `*pages.AssociationSource` cases in `serializeListViewDataSource` (and the sibling
  DataView path) return a stub with `{Key: "EntityRef", Value: nil}` instead of
  building the `IndirectEntityRef` + `EntityRefStep{Association, DestinationEntity}`.
  The *correct* serializer, `serializeAssociationSource`
  (`sdk/mpr/writer_widgets.go`), is only reached via the generic
  `serializeDataSource` dispatch (used by pluggable widgets), not the built-in
  DataView/ListView widget paths.
- **modelsdk (correct):** `associationSourceToGen`
  (`mdl/backend/modelsdk/widget_write.go`) emits the full
  `Forms$AssociationSource` with the `IndirectEntityRef` of steps, verified against
  Studio Pro. See the Bug 4 fix and `mdl-examples/bug-tests/assoc-datasource-modelsdk.mdl`.
- **Discovered:** 2026-07 (while fixing the modelsdk Bug 4).

<!--
Template for new entries:

### L<n> — <one-line title>

- **Symptom:** …
- **Legacy code path:** `sdk/mpr/…` (`function`) — what it does wrong.
- **modelsdk (correct):** `mdl/backend/modelsdk/…` (`function`) — how it differs; how verified.
- **Discovered:** <YYYY-MM>.
-->
