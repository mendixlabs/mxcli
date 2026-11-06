# mxcli init & mxcli new

## mxcli new — Create a project from scratch

If you don't have a Mendix project yet, `mxcli new` creates one with everything configured in a single command:

```bash
mxcli new MyApp --version 11.8.0
mxcli new MyApp --version 10.24.0 --output-dir ./projects/my-app
mxcli new MyApp --version 11.8.0 --skip-init   # Skip AI tooling setup
```

This downloads MxBuild, creates a blank Mendix project via `mx create-project`, runs `mxcli init` to set up AI tooling and Dev Container, and downloads the correct Linux mxcli binary for the container. The result is a ready-to-open folder.

Placing that Linux binary is best-effort. If the download fails (no network, a proxy, or a development build with no matching release), `mxcli new` prints a warning and still exits 0, because the project is already complete. Add the binary afterwards with `mxcli setup mxcli --output ./mxcli` in the project directory, adding `--tag <release>` when you're running a development build.

| Flag | Description |
|------|-------------|
| `--version` | Mendix version (required, e.g., `11.8.0`) |
| `--mxbuild-path` | Select an installed mxbuild executable, its directory, or an installation root containing `modeler/mxbuild`. Uses its sibling `mx` for creation and this mxbuild for the first build |
| `--output-dir` | Output directory (default: `./<app-name>`) |
| `--skip-init` | Skip AI tooling initialization |

To choose a specific installed toolchain, including a beta or a second installation of the same release:

```bash
mxcli new MyApp --version 11.8.0 --mxbuild-path /path/to/modeler/mxbuild
```

The selected tools must match `--version`. This bypasses CDN version resolution and native tool autodetection. An invalid selection fails without switching to another installation, PATH or the download cache. A three-part version accepts the selected installation's build; a four-part version must match the created project's full version. Without the flag, tool resolution is unchanged. AI tooling and Linux devcontainer binary setup still run as usual.

## mxcli init — Add tooling to an existing project

The `mxcli init` command prepares an existing Mendix project for AI-assisted development. It creates configuration files, installs skills, sets up a dev container, and optionally installs the VS Code extension.

### Basic Usage

```bash
# Initialize with Claude Code (default)
mxcli init /path/to/my-mendix-project

# Initialize for a specific AI tool
mxcli init --tool cursor /path/to/my-mendix-project

# Initialize for multiple tools
mxcli init --tool claude --tool cursor /path/to/my-mendix-project

# Initialize for all supported tools
mxcli init --all-tools /path/to/my-mendix-project

# List supported tools
mxcli init --list-tools
```

## Supported AI Tools

| Tool | Flag | Config Files Created |
|------|------|---------------------|
| **Claude Code** (default) | `--tool claude` | `.claude/settings.json`, `CLAUDE.md`, commands, lint rules |
| **OpenCode** | `--tool opencode` | `.opencode/`, `opencode.json`, commands, skills, lint rules |
| **Cursor** | `--tool cursor` | `.cursorrules` |
| **Continue.dev** | `--tool continue` | `.continue/config.json` |
| **Windsurf** | `--tool windsurf` | `.windsurfrules` |
| **Aider** | `--tool aider` | `.aider.conf.yml` |
| **Mistral Vibe** | `--tool vibe` | `.vibe/config.toml`, `.vibe/prompts/`, `.vibe/skills/` |
| **GitHub Copilot** | `--tool copilot` | `.github/copilot-instructions.md` |

All tools also receive the universal files (`AGENTS.md`, `.ai-context/`).

## What Happens During Init

1. **Detect the Mendix project** -- finds the `.mpr` file in the target directory
2. **Create universal AI context** -- `AGENTS.md` and `.ai-context/skills/` with MDL pattern guides
3. **Create tool-specific configuration** -- based on the selected `--tool` flag(s)
4. **Set up dev container** -- `.devcontainer/` with Dockerfile and configuration
5. **Copy mxcli binary** -- places the mxcli executable in the project root
6. **Install VS Code extension** -- copies and installs the bundled `.vsix` file
7. **Create `mdlsource/`** -- the directory the generated `CLAUDE.md` tells the assistant to keep MDL scripts in, with a short README (`mxcli new` creates it even with `--skip-init`)
8. **Stamp the version** -- `.ai-context/mxcli-tooling.json` records which mxcli wrote the tooling; see [Syncing with Updates](./syncing.md#the-version-stamp)

`CLAUDE.md` and `AGENTS.md` are written between `<!-- mxcli:begin … -->` and
`<!-- mxcli:end -->` markers. Notes you add outside the markers survive a
re-run of `mxcli init` and every `mxcli init --sync-skills`.

## Adding a Tool Later

To add support for an additional AI tool to an existing project:

```bash
mxcli add-tool cursor
```

This creates only the tool-specific files without overwriting existing configuration.

## Related Pages

- [What Gets Created](./init-output.md) -- detailed directory listing
- [Customizing Skills](./customizing-skills.md) -- modifying generated skill files
- [Syncing with Updates](./syncing.md) -- keeping files current after mxcli upgrades
