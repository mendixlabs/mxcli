# Installation

Pick whichever method suits your situation. If you just want to try mxcli without installing anything, start with the [Playground](#playground-zero-install). If you're planning to use mxcli on your own project with an AI coding assistant, skip to the [Dev Container](#dev-container-recommended) section.

## Playground (zero install)

The fastest way to try mxcli. The [mxcli Playground](https://github.com/mendixlabs/mxcli-playground) is a GitHub repository with a pre-configured Mendix project, example scripts, and tutorials. Open it in a Codespace and start using mxcli immediately -- nothing to install on your machine.

[![Open in GitHub Codespaces](https://github.com/codespaces/badge.svg)](https://codespaces.new/mendixlabs/mxcli-playground)

The Codespace comes with mxcli, a JDK, Docker-in-Docker, Claude Code, and a sample Mendix 11.x project ready to explore and modify. It includes:

- **5 example scripts** -- explore, create entities, microflows, pages, and security
- **Step-by-step tutorials** -- from first steps through linting and testing
- **AI tool configs** -- pre-configured for Claude Code, GitHub Copilot, OpenCode, Cursor, Windsurf, Continue.dev, and Aider

Once the Codespace is running:

```bash
./mxcli -p App.mpr -c "SHOW STRUCTURE"          # Explore the project
./mxcli exec scripts/01-explore.mdl -p App.mpr   # Run an example script
./mxcli                                           # Start interactive REPL
```

When you're ready to work on your own Mendix project, use one of the installation methods below.

## Binary download

Pre-built binaries are published for Linux, macOS, and Windows on both amd64 and arm64. Release assets are **raw binaries** named `mxcli-<os>-<arch>` (Windows: `.exe`) — there is nothing to extract.

While mxcli is fast-moving alpha, use the rolling **`nightly`** build (new features land there before a tagged release); pin a `vX.Y.Z` release for reproducibility.

```bash
# Linux/macOS — nightly (recommended for now)
curl -fsSL -o mxcli \
  https://github.com/mendixlabs/mxcli/releases/download/nightly/mxcli-linux-amd64
chmod +x mxcli && sudo mv mxcli /usr/local/bin/

# ...or pin a specific release
curl -fsSL -o mxcli \
  https://github.com/mendixlabs/mxcli/releases/download/v0.16.0/mxcli-darwin-arm64
chmod +x mxcli && sudo mv mxcli /usr/local/bin/
```

Assets: `mxcli-linux-amd64`, `mxcli-linux-arm64`, `mxcli-darwin-amd64`, `mxcli-darwin-arm64`, `mxcli-windows-amd64.exe`, `mxcli-windows-arm64.exe`. On Windows, download the `.exe` and add its folder to your PATH. You can browse them on the [GitHub Releases page](https://github.com/mendixlabs/mxcli/releases).

> `go install github.com/mendixlabs/mxcli/cmd/mxcli@latest` does **not** work: the ANTLR parser is generated at build time and isn't committed, so a `go install` from the module source fails. Use a pre-built binary, or **Build from source** below (which runs `make grammar`).

## Build from source

Building from source requires **Go 1.24 or later** and **Make**. No C compiler is needed -- mxcli uses a pure-Go SQLite driver.

```bash
git clone https://github.com/mendixlabs/mxcli.git
cd mxcli
make build
```

The binary lands at `./bin/mxcli`. You can copy it to a directory on your PATH or run it directly:

```bash
./bin/mxcli --version
```

## New project from scratch

If you don't have a Mendix project yet, `mxcli new` creates one with everything configured:

```bash
mxcli new MyApp --version 11.8.0
```

This single command:
1. Downloads MxBuild for the specified Mendix version
2. Creates a blank Mendix project (`App.mpr`)
3. Sets up AI tooling (`.claude/`, skills, `AGENTS.md`)
4. Configures a Dev Container (`.devcontainer/`)
5. Downloads the correct Linux mxcli binary for the container

Open the resulting `MyApp/` folder in VS Code and click **"Reopen in Container"** — you're ready to go.

Options:
```bash
mxcli new MyApp --version 10.24.0 --output-dir ./projects/my-app
mxcli new MyApp --version 11.8.0 --skip-init   # Skip AI tooling setup
```

## Dev Container for existing projects

For an existing Mendix project, `mxcli init` adds AI tooling and a Dev Container configuration.

Here's how to set it up:

**Step 1:** Install mxcli using one of the methods above (you need it locally to run `init`).

**Step 2:** Run `mxcli init` on your Mendix project:

```bash
mxcli init /path/to/my-mendix-project
```

This creates a `.devcontainer/` folder (along with skill files, agent configs, and other goodies) inside your project directory.

**Step 3:** Open the project folder in VS Code and click **"Reopen in Container"** when prompted (or use the Command Palette: `Dev Containers: Reopen in Container`).

VS Code will build and start the container. This takes a minute or two the first time.

### What's inside the Dev Container

The container comes with everything you need pre-installed:

| Component | What it's for |
|-----------|---------------|
| **mxcli** | The CLI itself, copied into the project |
| **JDK 21** (Adoptium) | Required by MxBuild for project validation |
| **Docker-in-Docker** | Running Mendix apps locally with `mxcli docker run` |
| **Node.js** | Playwright testing support |
| **PostgreSQL client** | Database connectivity for demo data |
| **Claude Code** | AI coding assistant (auto-installed on container creation) |

Once the container is running, mxcli is ready to use -- no further setup needed.

### Specifying AI tools

By default, `mxcli init` configures for Claude Code. You can target other tools too:

```bash
# Cursor only
mxcli init --tool cursor /path/to/my-mendix-project

# Multiple tools
mxcli init --tool claude --tool cursor /path/to/my-mendix-project

# Everything
mxcli init --all-tools /path/to/my-mendix-project
```

Run `mxcli init --list-tools` to see all supported tools.

## Verify your installation

Whichever method you used, confirm that mxcli is working:

```bash
mxcli --version
```

You should see version and build information printed to the terminal. If you get a "command not found" error, double-check that the binary is on your PATH.

You're ready to open a project.
