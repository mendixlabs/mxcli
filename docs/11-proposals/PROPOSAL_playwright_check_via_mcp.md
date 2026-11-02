---
title: Playwright Check Through Studio Pro's MCP Browser
status: draft
date: 2026-11-01
related:
  - PROPOSAL_mcp_backend
  - PROPOSAL_playwright_session_reuse
  - proposal-playwright-cli
---

# Proposal: Playwright Check Through Studio Pro's MCP Browser

**Status:** Draft
**Date:** 2026-11-01

## Problem Statement

`mxcli playwright check` gives a one-line text verdict per page of a running app.
It covers rendering, rows, error banners, console errors, failed requests, the
sign-in form, access denied and wrong-page redirects. To get there it launches a
**local** headless Chromium through Node and `playwright` (`cmd/mxcli/docker/pagecheck.go`).
That only works when three things are true of the machine mxcli runs on:

1. Node, Playwright and a Chromium are installed there.
2. The app is reachable from there. For `run --local` that is localhost. For an
   app that **Studio Pro** runs on the host, a devcontainer has to forward the
   port and the check has to be pointed at it with `--base-url`.
3. The app it checks is the one the model edits went into.

Under `--mcp`, all three fail together. mxcli writes the model into a live Studio
Pro, and from 11.16 `--mcp-run` starts the app there too, through the
`mendix_app` handler. The app then runs **on Studio Pro's machine**, and the
local check either cannot reach it or has no browser to reach it with. So the
loop "edit through MCP → run in Studio Pro → check the page" breaks at the last
step. The agent falls back to driving the `browser_*` handlers by hand. That is
the hand-rolled navigate / snapshot / read sequence `playwright check` was built
to replace, and here every accessibility snapshot lands in the agent's context.

Studio Pro 11.16 ships its own Playwright behind `execute_handler`. On
2026-11-01, `playwright_status` on 11.16.0 answered *"Playwright is available;
use the browser_\* handlers to check the UI."* Its browser runs next to the app,
so neither the reachability problem nor the install problem comes up.

**Who benefits:** agents and contributors working with `--mcp` from a
devcontainer or another machine, which is the setup the MCP backend exists for.

## Decision 1: A Mode of `playwright check`, Not a Flag on `--mcp-run`

Recommendation: **`mxcli playwright check --mcp <url>`**. It is the existing
command with the existing assertions and verdict format, and the browser is
Studio Pro's. It is **not** a `--mcp-check-page` added to `--mcp-run`.

- **Building and checking are separate steps with separate failure modes.**
  `--mcp-run` answers "did the app start" with exit 0 or an error. A check
  answers "is this page right" with exit 0 for pass, 1 for fail and 2 for could
  not check. One exit code cannot carry both answers, and agents read the exit
  code.
- **A check is repeated far more often than a run.** The usual loop is: run
  once, then check, fix through MCP, check again, and so on. Tied to `--mcp-run`,
  every check would restart the app (`mendix_app run` restarts a running app),
  which costs a minute each time and loses the session.
- **One command, one verdict.** Agents and `verify-in-runtime.md` already know
  `playwright check`. A second command with the same output would split the
  docs, the tests and the habits built on them. The browser is a transport
  detail, the same way `--mcp` is for model writes.
- **They compose.** `mxcli exec fix.mdl --mcp URL --mcp-run` followed by
  `mxcli playwright check /p/orders --mcp URL` is two commands with two clear
  results.

**Spelling.** `--mcp` is already a root **persistent string flag**: the server
URL, with `--mcp-dial` beside it (`cmd/mxcli/main.go`). A boolean `--mcp` on
`playwright check` would collide with it. So the mode is selected by the existing
flag carrying its URL, exactly as model writes are:

```bash
mxcli playwright check /p/customers /p/orders -p app.mpr \
    --mcp http://localhost:7782/mcp --mcp-dial host.docker.internal:7782
```

The `--mcp` help text ("Route model writes…") is widened to cover this use.

**Base URL.** Under `--mcp`, the pages are resolved against `mendix_app status`'s
`appUrl` (measured as `http://localhost:8080/`), not `.docker/.env` or a
`run --local` handshake. That `localhost` is Studio Pro's, which is the point.
An explicit `--base-url` still wins. The check **never starts the app**. If
status reports not running, it exits 2 with "start it with `--mcp-run`".

## What the Check Needs from `execute_handler`

### Handlers, Measured 2026-11-01 on 11.16.0

`query_handlers {}` (captured earlier the same day in
`mx-test-projects/runs/mcp-surface/query_handlers.txt`) lists these browser
handlers:

| Handler (`mcp_studio-pro-playwright_` prefix omitted) | Description as served | Used by the check |
|---|---|---|
| `playwright_status` (no prefix) | whether Playwright is available, and why not | **gate**: first call |
| `browser_navigate` | Navigate to a URL | each page |
| `browser_snapshot` | accessibility snapshot of the current page | each page: the main signal source |
| `browser_find` | search the snapshot for text or a regexp, with context | `--assert-text`, login-form and alert detection without a whole snapshot |
| `browser_wait_for` | wait for text to appear or disappear, or for a time | settling |
| `browser_console_messages` | all console messages | `console-errors=` |
| `browser_network_requests` / `browser_network_request` | numbered list of requests since load / one request's details | `http=`, failed same-origin requests, settling |
| `browser_fill_form`, `browser_type`, `browser_click`, `browser_press_key` | form input | sign-in |
| `browser_handle_dialog` | handle a dialog | a native dialog blocking the page |
| `browser_close` | close the page | `--sign-out` cleanup |
| `browser_navigate_back`, `browser_hover`, `browser_select_option` | — | not needed |

**Absent from the list, and load-bearing for the design:**
`browser_evaluate`, `browser_take_screenshot`, `browser_tabs`, and any
storage-state handler. Upstream `@playwright/mcp` has these. Studio Pro serves a
curated subset (and adds `browser_find`, which upstream does not have).

### Input Schemas: Not Yet Captured

`getDetails` is the only source of the payload shapes, and `execute_handler`'s
description requires it before a call. It **could not be read**. From 15:3x to
15:58 UTC on 2026-11-01, every `query_handlers` call on the 11.16.0 server
(`host.docker.internal:7785`, Host: localhost) failed with JSON-RPC `-32000
"Request timed out"` after 30 s. That includes `{operation: list}`, a
`keyword: browser` list, and `getDetails` for `mendix_app` alone, which had
answered earlier that day. Other tools on the same connection (`tools/list`,
`glob`) answered in about 0.2 s. So the handler registry, not the connection,
was stuck. My guess, not measured, is that it waits on the Playwright subserver
while listing.

Two consequences for the design, whatever the schemas turn out to be:

1. **A `query_handlers` timeout is "could not check" (exit 2)**, with that
   cause named. `requireHandler` currently caches only answers. It must not
   cache a timeout as "no such handler", and it must not retry it the way
   mutation timeouts are retried (`timeout.go`).
2. The schemas are **re-measured before implementation starts** (Open
   Question 1). The payload field names below follow upstream `@playwright/mcp`
   and are placeholders until then: `navigate {url}`, `click {element, ref}`,
   `type {element, ref, text, submit?}`, `fill_form {fields[]}`,
   `wait_for {text?, textGone?, time?}`, `console_messages {level?}`. The
   implementation reads nothing from them until measured. Each call site cites
   the captured `getDetails` reply, the way `AppStatus` cites its measured reply.

## How Each Verdict Signal Maps

The verdict is built from `PageSignals` (`pagecheck.go`), which `EvaluatePage`
and `formatPageVerdict` turn into the line. The MCP mode must **produce the same
`PageSignals`** so that evaluation and formatting are shared, not reimplemented.
The local probe collects them with `page.evaluate` and CSS selectors. Neither is
available here, so each signal comes from a different source:

| Signal | Local probe | Through MCP | Fidelity |
|---|---|---|---|
| `finalUrl`, `title` | `p.url()`, `p.title()` | the "Page URL" / "Page Title" lines of the navigate/snapshot reply | same, if the reply carries them (measure) |
| `status` | `goto` response | the document entry in `browser_network_requests` | same |
| `headings` | `h1,h2` outside the demo-user switcher | snapshot `heading [level=1/2]` nodes outside the switcher's subtree | same |
| `loginForm` | `#usernameInput` | snapshot textbox named like the sign-in field, or `browser_find` | same, keyed on the accessible name (measure on 11.16's login.html) |
| `alerts`, `errorAlerts` | `.alert-danger`, `.mx-dialog-error`, … | snapshot `alert` / `dialog` nodes | **weaker**: the danger/warning/info split relies on classes the tree may not show. Measure, and fall back to "any alert fails" if no split survives |
| `rows`, `dataWidgets` | grid/list-view selectors | `row` nodes with `gridcell` children; `listitem` under a list | close for Data Grid 2 (role=row); **list views to measure** |
| `textLen`, `textFound` | `body.innerText` | the snapshot's text content; `--assert-text` via `browser_find` | close |
| `consoleErrors` | `console` + `pageerror` events | `browser_console_messages`, errors only, minus same-origin "Failed to load resource" | same, if level is reported |
| `httpErrors` | `response` events ≥ 400 | `browser_network_requests` entries ≥ 400, same filters | same |
| `unsettled`, `pendingRequests` | in-flight map + text stable ×3 | poll `browser_network_requests` and `browser_find` until both are unchanged across polls, within `--wait` | **weaker**: requests still open may not be visible (#845 is the case where it matters). Measure whether the list shows pending entries |
| `counts` (`--assert-count`) | `locator(css).count()` | **CSS cannot be evaluated** | see below |
| `screenshot` | `p.screenshot` | **no handler** | refused |

**`--assert-count`.** With no evaluate, a CSS selector such as
`.mx-name-dgCustomers [role=row]>=2` has nothing to run against. In MCP mode the
check accepts an **accessibility selector**, which is `role` with an optional
`"name"`, counted in the snapshot: `row>=2`, `button "Save"==1`. It refuses a CSS
selector with exit 2 and names the role form. It never silently passes. Widget
names (`mx-name-*`) do not appear in the accessibility tree. Whether a scoped
form (`within grid "Customers"`) is worth adding is Open Question 3.

**`--screenshot`** is refused under `--mcp` with exit 2: "Studio Pro's browser
has no screenshot handler; drop --mcp for a visual check". `verify-in-runtime.md`
already makes a screenshot the exception, so this costs least where it matters
least.

## Sign-in and Sessions

- **Credentials are resolved as today**: `--user/--password`, `--role R` and the
  demo-user fallback read project security from `-p`. The sign-in itself is
  `navigate login.html` → fill the two fields → click Sign in, and the result is
  confirmed by the next snapshot showing no sign-in form. A refusal still reads
  the runtime log for the cause, but through `read_file /app-logs/app.log`
  (`appLogPath` in `app.go`), not `<project>/.mxcli/runtime.log`.
- **No storage-state file.** The session lives in Studio Pro's browser context,
  so it persists across checks only as long as that browser does. Without a
  storage handler, mxcli cannot save it or restore it. The check therefore
  starts each run by asking whether the first page already shows the sign-in
  form. If it does not, it treats the browser as signed in, and the verdict
  names the user only when this run signed in. `--fresh-login` signs out first.
  `--sign-out` signs out and closes the page at the end.
- **The shared browser is the user's too.** An agent working alongside a human in
  Studio Pro drives the same browser. The check navigates away from whatever was
  open, and the help text says so.
- **Passwords cross the MCP connection** into Studio Pro, the same connection
  model writes use. Demo-user passwords are generated per project, but a
  `--password` for a real account may land in a Studio Pro log. This goes in the
  help text, measured if possible (Open Question 4).

## Fit with `verify-in-runtime.md` and `playwright check`

- **`playwright check`**: one command, two probes. `RunPageCheck` already
  isolates its browser behind seams (`probePagesFn`, `loginFn`, `signOutFn` in
  `pagecheck_check.go`). The MCP mode becomes a second implementation of those
  three, selected when `--mcp` is set. Everything above them is unchanged and
  shared: credential resolution, the access-denied and wrong-page rules,
  `EvaluatePage` and the verdict line. A new failure rule then reaches both
  modes at once.
- **`verify-in-runtime.md`**: the table "Symptom lives in → Sufficient proof"
  gains no row. The tier is the same, and it is still the running app. The
  procedure gains one branch at step 4/5 ("Provision and boot", "Assert in the
  browser"): *when the model was written with `--mcp` into Studio Pro 11.16+,
  boot with `--mcp-run` and assert with `playwright check --mcp`, not
  `run --local`*. Step 6, the A/B against the broken build, works the same way.
  Two limits are stated there: no screenshot, and `--assert-count` takes the
  role form. The skill's "assert in text" rule fits better here than locally,
  because the alternative under MCP is snapshots in the agent's context.
- **`live-edit-with-studio-pro` skill**: its "after `--mcp-run`" step points to
  `playwright check --mcp` instead of browser handlers called by hand.
- **`PROPOSAL_playwright_session_reuse`**: does not apply to this mode. There is
  no session file to reuse, and the browser's lifetime belongs to Studio Pro.

## BSON Structure

Not applicable. The check reads nothing from the model and writes nothing to
it. It talks only to the running app, through Studio Pro's browser.

## Proposed CLI

No MDL syntax. The command, flags and exit codes are those of
`playwright check`, with:

| Flag | Under `--mcp` |
|---|---|
| `--mcp URL`, `--mcp-dial ADDR` | select Studio Pro's browser (root flags, existing) |
| `--base-url` | defaults to `mendix_app status` `appUrl` |
| `--assert-text`, `--wait`, `--user/--password/--role`, `--fresh-login`, `--sign-out`, `--allow-console-errors` | same meaning |
| `--assert-count` | accessibility selector (`row>=2`); a CSS selector is exit 2 |
| `--screenshot` | exit 2 |

Exit 2 is also returned when the server lacks `execute_handler` (before 11.16),
when `playwright_status` reports unavailable (its reason is printed verbatim), when
`query_handlers` times out, and when the app is not running.

## Implementation Plan

**Prerequisite:** `mdl/backend/mcp/app.go` (`requireHandler`, `executeHandler`,
`GetAppStatus`) exists today only on branch `claude/festive-driscoll-aa5aa4`,
commit 41698502e ("drop Concord"), and is not on `main`. This work starts after
that lands.

### Files to modify/create

| File | Change |
|---|---|
| `mdl/backend/mcp/browser.go` (new) | `PlaywrightStatus()`, and thin wrappers over `executeHandler` for the handlers in the table above, using the measured payload shapes, with the reply returned as text |
| `mdl/backend/mcp/app.go` | `requireHandler`: a `-32000` timeout is returned as its own error (`ErrHandlersUnavailable`), never cached as "absent" |
| `cmd/mxcli/docker/pagecheck_snapshot.go` (new) | pure parser from snapshot text, console text and request-list text to `PageSignals`, plus the accessibility-selector counter. It has no I/O and is where most of the tests go |
| `cmd/mxcli/docker/pagecheck_mcp.go` (new) | an MCP implementation of probe, login and sign-out behind a small interface, with no import of `mdl/backend/mcp`. `cmd/mxcli` passes in an adapter, so `docker` stays free of the backend |
| `cmd/mxcli/docker/pagecheck_check.go` | `PageCheckOptions` gains `Browser` (nil means the local probe). The three seams dispatch on it |
| `cmd/mxcli/docker/pagecheck_assert.go` | `ParseCountAssertion` gains the role form, tagged so each mode refuses the other's form |
| `cmd/mxcli/cmd_playwright.go` | read `--mcp`/`--mcp-dial`, build the backend and adapter, resolve the base URL from status, refuse `--screenshot`, extend the help text |
| `cmd/mxcli/main.go` | widen the `--mcp` flag description |
| `.claude/skills/verify-in-runtime.md`, `.claude/skills/mendix/live-edit-with-studio-pro/SKILL.md`, `docs-site/src/tools/mcp-connect.md` | the branch and limits described above |

Order: measure (Open Question 1) → snapshot parser with fixtures → `browser.go` →
dispatch in `RunPageCheck` → CLI → docs.

## Version Compatibility

Studio Pro **11.16+**, gated on **presence, not version**: the `execute_handler`
and `query_handlers` tools, then `playwright_status` by `getDetails`, and then
`playwright_status` itself must answer "available". Presence also depends on
Studio Pro's preferences and the servers connected to it
(`mcp-1116-surface.md`), so a version number alone would be wrong in both
directions. Before 11.16, the error names the version, as `--mcp-run` does.

## Test Plan

- **Snapshot parser (unit, the bulk).** Fixtures are snapshot, console and
  request-list replies **captured from 11.16.0**, not hand-written, saved under
  `cmd/mxcli/docker/testdata/mcp-snapshots/`. They cover: a page with a Data
  Grid 2 that has rows; an empty list view; the sign-in form; an
  `alert-danger`; an error dialog; the 403 landing; an unknown `/p/` that
  redirects home with an info dialog (#998); and the demo-user switcher's
  heading. Each asserts the `PageSignals` and the verdict line.
- **Backend wrappers (unit).** `newFakePED` in `mdl/backend/mcp/client_test.go`
  answers `query_handlers` and `execute_handler`. The cases: the handler is
  absent; `playwright_status` reports unavailable, and its reason is surfaced; a
  `-32000` timeout gives `ErrHandlersUnavailable` and is not cached;
  "Error executing tool handler: …" with `isError:false` is an error.
- **`RunPageCheck` dispatch (unit).** A fake `Browser` covers the sign-in
  sequence, `--fresh-login`, `--sign-out`, an already-signed-in browser, CSS
  `--assert-count` refused under MCP, and `--screenshot` refused.
- **Parity control (live, manual or `-tags integration` with an env-provided
  URL).** One app, run by Studio Pro, with a passing page and a page that has a
  seeded error banner. Check both pages with both probes, the local one through
  a forwarded port. The verdicts must agree: the passing page must pass under
  both and the banner page must fail under both. This is the control. A mode
  that passes everything cannot pass it.
- **Fix-proven step.** For the settling rule, revert it and confirm that a page
  whose list-view microflow sleeps 3 s reports `rows=0` instead of
  STILL LOADING or its real rows. This is the #845 symptom, made to happen
  through MCP.

## Open Questions

1. **Handler input schemas.** Re-run `query_handlers {operation: getDetails,
   names: [playwright_status, mcp_studio-pro-playwright_browser_*]}` once the
   registry answers again (a Studio Pro restart may be needed), and commit the
   reply to `mx-test-projects/runs/mcp-surface/`. Also measure: does the navigate
   or snapshot reply carry URL and title? Does `browser_network_requests` show
   pending requests and the status of the document request? Does
   `browser_console_messages` report the level?
2. **Why `query_handlers` hung.** Is it reproducible, does it follow the
   Playwright subserver's start, and does it recover without a restart? If it is
   the subserver, an exit 2 "could not check" is right, but the message should
   say to restart Studio Pro.
3. **A scoped count.** Is `row>=2` enough without a "within grid X" scope? Pages
   with two grids are common.
4. **Where the password goes.** Does Studio Pro log `execute_handler` payloads?
5. **Concurrent users of the browser.** Does Studio Pro's Maia chat share this
   browser? If it does, a check run while Maia is browsing interleaves with it,
   and the help text should say so.
