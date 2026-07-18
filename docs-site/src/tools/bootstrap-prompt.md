# Bootstrap prompt (empty repo → running Mendix app)

The **primary** way to start a Mendix + mxcli project from the web or an iPad — no
local CLI, no GitHub template to pick from a (short) mobile list. Open an **empty
repo** in Claude Code Web and paste the prompt below; the agent provisions
everything and commits the result so future sessions self-bootstrap.

Why a prompt instead of a GitHub template repo: the mobile "New repository" template
dropdown shows only a small subset of templates, and a template repo needs per-Mendix-
version upkeep. A prompt starts from a *truly empty* repo, runs *current* mxcli, and
can seed the model from a design prototype in the same session — nothing to maintain.

## The prompt

```text
This is an empty repo. Provision it as a Mendix app developed with mxcli:

1. Ensure `mxcli` is available. It should be pre-installed by the environment; if
   not, download a prebuilt binary for your OS/arch and put it at `./mxcli`, e.g.:

   ```bash
   curl -fsSL -o ./mxcli \
     https://github.com/mendixlabs/mxcli/releases/download/nightly/mxcli-linux-amd64
   chmod +x ./mxcli
   ```

   Use the **`nightly`** build: this is fast-moving alpha software and the warm-loop
   commands used below (`run --local`, `--setup`, `--ensure-db`) land in `nightly`
   before they appear in a tagged release. For a reproducible setup, pin a specific
   release instead (`.../releases/download/vX.Y.Z/mxcli-<os>-<arch>`). Note: `go
   install …@latest` does **not** work — the generated ANTLR parser isn't committed,
   so use the prebuilt binary (a from-source build needs `make grammar`).
2. Create the app at the repo root: `mxcli new App --version 11.6.3`
   (or `mxcli init` if an .mpr already exists).
3. Ensure the Claude tooling is set up: `mxcli init --tool claude`. This adds a
   SessionStart hook to `.claude/settings.json` that self-bootstraps future sessions.
4. Bring prerequisites up: `./mxcli run --local --setup --ensure-db -p App.mpr`
   (caches MxBuild + runtime, starts Postgres, creates the app database).
5. COMMIT everything now — `App.mpr`, `.devcontainer/`, and `.claude/` (including the
   SessionStart hook) — so that after idle reaping the next session bootstraps from
   files, not from re-running this prompt.
6. Boot and verify: `./mxcli run --local -p App.mpr` in the background, then confirm
   the app answers HTTP 200 at http://localhost:8080/ and report.

(Optional) Seed the domain model, pages, and microflows from this prototype: <paste or link a design here>.
```

## Which mxcli version gets installed

Prebuilt binaries are the working install path. CI publishes them on every `vX.Y.Z`
tag (latest is v0.16.0) **and** as a rolling `nightly` pre-release, with assets named
`mxcli-<os>-<arch>`.

- **`nightly` — recommended while mxcli is fast-moving alpha.** New features (the whole
  warm-loop surface: `run --local`, `--watch`, `--ensure-db`, `--setup`, screenshots)
  land in `nightly` before they reach a tagged release, so the bootstrap flow above
  needs it. Download `.../releases/download/nightly/mxcli-<os>-<arch>`, or once mxcli is
  present, `mxcli setup mxcli --tag nightly`.
- **`vX.Y.Z` — pin for reproducibility / stability.** The CI marks nightly a
  pre-release ("use tagged releases for production"). Download
  `.../releases/download/vX.Y.Z/mxcli-<os>-<arch>` or `mxcli setup mxcli --tag vX.Y.Z`.
  With no `--tag`, `mxcli setup mxcli` matches the mxcli already running it (nightly →
  `nightly`, `vX.Y.Z` → that release) — mainly useful for replicating a version onto
  another OS/arch (e.g. the Linux binary in a Dev Container), not the first install.
- **Environment pre-install** (the robust path) installs whatever the Claude Code Web
  image bakes in — the way to pin a known-good version fleet-wide.
- **`go install …@latest` does not work yet.** The module *is* public (tags v0.1.0–
  v0.16.0), but the generated ANTLR parser (`mdl/grammar/parser/`) is gitignored and
  not committed, so a `go install` from the tagged source fails on the missing package.
  Building from source works only via `make build`/`make release` (which run
  `make grammar` first). Enabling `go install` would require committing the generated
  parser (or generating it during module build) — a maintainer decision.

## Two rules that make this robust

- **Committing the config (step 5) is mandatory.** The prompt is a *one-time seed*.
  Its output — `.mpr` + `.devcontainer/` + `.claude/` with the SessionStart hook — must
  be committed so the steady state is file-driven and deterministic. After that, every
  new session runs the hook (`run --local --setup --ensure-db`) automatically; you
  never re-paste the prompt.
- **mxcli delivery is an environment concern, not the prompt's.** Step 1 is the fragile
  part in a gated web session (a GitHub release `curl` may be blocked). The robust fix
  is for the Claude Code Web **environment image / setup script to pre-install mxcli**
  (and pre-cache MxBuild + runtime); `go install` via `proxy.golang.org` is the fallback
  and needs mxcli published as a public Go module.

## After bootstrap — the inner loop

```bash
./mxcli run --local -p App.mpr --watch --screenshot   # warm dev loop + screenshots
./mxcli exec change.mdl -p App.mpr                     # edit the model; the loop hot-applies
```

See [mxcli run --local](run-local.md) for the warm loop, `--watch`, `--ensure-db`, and
the screenshot flags.
