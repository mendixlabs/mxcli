// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Compose project names (mendixlabs/mxcli#1364).
//
// mxcli runs `docker compose` with the project's .docker/ folder as the working
// directory, and Compose names a stack after that folder. Every project's folder
// is called ".docker", so every project was the Compose project "docker" and
// shared container names (docker-db-1) and volumes (docker_postgres-data): two
// apps' stacks replaced each other and their databases collided.
//
// New compose files therefore carry a top-level `name:` derived from the app.
// The name lives only in docker-compose.yml (not in .env) because that file is
// the one stamped with the template version and the one `docker init --force`
// regenerates deliberately; a .env that gets recreated by itself must never be
// able to rename a stack. Precedence in Compose is `-p` > COMPOSE_PROJECT_NAME
// (shell or .env) > `name:` > folder name, so users can still override it.
//
// The name is part of a stack's identity: a different name makes Compose create
// new, EMPTY volumes and leaves the old database volume behind. So a name is only
// ever chosen when a compose file is created from nothing. Regenerating an
// existing file (--force) carries over whatever name the stack already had.

const (
	// composeNamePlaceholder is the token in templates/docker-compose.yml that
	// is replaced by the project name when the file is written.
	composeNamePlaceholder = "__COMPOSE_PROJECT_NAME__"

	// composeNamePrefix marks stacks created by mxcli so they are recognisable
	// in `docker ps` / `docker volume ls`.
	composeNamePrefix = "mxcli-"

	// legacyComposeProjectName is what Compose called every stack before the
	// template had a name: the normalised ".docker" folder name.
	legacyComposeProjectName = "docker"

	// maxComposeNameBase bounds the app-derived part so container, network and
	// volume names stay well inside the 63-character DNS label limit.
	maxComposeNameBase = 48
)

var (
	invalidComposeNameChars = regexp.MustCompile(`[^a-z0-9_-]+`)
	repeatedDashes          = regexp.MustCompile(`-{2,}`)
	// topLevelNameRegex matches an unindented `name:` line of a compose file and
	// captures its value up to whitespace or a trailing comment.
	topLevelNameRegex = regexp.MustCompile(`(?m)^name:[ \t]*([^\s#]+)`)
	envProjectNameRe  = regexp.MustCompile(`(?m)^COMPOSE_PROJECT_NAME=[ \t]*([^\r\n#]*)`)
)

// sanitizeComposeName reduces s to what a Compose project name may contain
// (lowercase letters, digits, '-' and '_', starting with a letter or digit).
// Anything else becomes a dash; runs of dashes collapse; at most
// maxComposeNameBase characters are kept. Returns "" when nothing usable is left.
func sanitizeComposeName(s string) string {
	s = invalidComposeNameChars.ReplaceAllString(strings.ToLower(s), "-")
	s = repeatedDashes.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-_")
	if len(s) > maxComposeNameBase {
		s = strings.Trim(s[:maxComposeNameBase], "-_")
	}
	return s
}

// ComposeProjectNameFor returns the Compose project name for a new stack of the
// app whose .mpr is projectPath: "mxcli-" plus the app folder's name, so
// ~/work/Customer Portal/app.mpr becomes "mxcli-customer-portal". The folder is
// preferred over the .mpr name because projects commonly call the file "app.mpr".
// If the folder gives nothing usable the .mpr base name is used, then "app".
func ComposeProjectNameFor(projectPath string) string {
	abs, err := filepath.Abs(projectPath)
	if err != nil {
		abs = projectPath
	}
	base := sanitizeComposeName(filepath.Base(filepath.Dir(abs)))
	if base == "" {
		file := filepath.Base(abs)
		base = sanitizeComposeName(strings.TrimSuffix(file, filepath.Ext(file)))
	}
	if base == "" {
		base = "app"
	}
	return composeNamePrefix + base
}

// normalizeLikeCompose applies Docker Compose's own rule for deriving a project
// name from a directory name: lowercase, drop every character outside
// [a-z0-9_-], then trim leading '_' and '-'. ".docker" -> "docker".
func normalizeLikeCompose(s string) string {
	s = invalidComposeNameChars.ReplaceAllString(strings.ToLower(s), "")
	return strings.TrimLeft(s, "_-")
}

// existingComposeName returns the raw value of the top-level `name:` of a
// compose file (quotes and interpolation untouched) and whether there was one.
func existingComposeName(data []byte) (string, bool) {
	m := topLevelNameRegex.FindSubmatch(data)
	if m == nil {
		return "", false
	}
	return string(m[1]), true
}

// envComposeProjectName returns COMPOSE_PROJECT_NAME from dockerDir/.env, as
// Compose normalises it, if the user set it (the workaround for #1364).
func envComposeProjectName(dockerDir string) (string, bool) {
	env, err := os.ReadFile(filepath.Join(dockerDir, ".env"))
	if err != nil {
		return "", false
	}
	m := envProjectNameRe.FindSubmatch(env)
	if m == nil {
		return "", false
	}
	v := normalizeLikeCompose(strings.Trim(strings.TrimSpace(string(m[1])), `"'`))
	return v, v != ""
}

// legacyComposeName returns the project name that a compose file with no
// `name:` in dockerDir has been getting: the folder name as Compose normalises
// it (".docker" -> "docker").
func legacyComposeName(dockerDir string) string {
	abs, err := filepath.Abs(dockerDir)
	if err != nil {
		abs = dockerDir
	}
	if n := normalizeLikeCompose(filepath.Base(abs)); n != "" {
		return n
	}
	return legacyComposeProjectName
}

// composeNameDecision is the name written into a (re)generated compose file and
// where it came from.
type composeNameDecision struct {
	// Value is the YAML scalar to write after `name:` (already quoted when generated).
	Value string
	// Name is the bare project name, for messages.
	Name string
	// Source is "env" (COMPOSE_PROJECT_NAME in the old .env), "kept" (an existing
	// name: line), "legacy" (an existing file with no name, pinned to the name
	// Compose was already using) or "new".
	Source string
}

// decideComposeName chooses the project name for the compose file about to be
// written at composePath. See the package comment above for why an existing
// file's name always wins over a freshly derived one.
func decideComposeName(projectPath, dockerDir, composePath string) composeNameDecision {
	// COMPOSE_PROJECT_NAME in .env beats `name:` in Compose's precedence, so it
	// is the stack's real name wherever it is set. `--force` rewrites .env
	// without it, so it must be carried into the compose file now or the stack
	// is renamed (and its volumes stranded) by the rewrite.
	if v, ok := envComposeProjectName(dockerDir); ok {
		return composeNameDecision{Value: fmt.Sprintf("%q", v), Name: v, Source: "env"}
	}
	if data, err := os.ReadFile(composePath); err == nil {
		if raw, ok := existingComposeName(data); ok {
			return composeNameDecision{Value: raw, Name: strings.Trim(raw, `"'`), Source: "kept"}
		}
		legacy := legacyComposeName(dockerDir)
		return composeNameDecision{Value: fmt.Sprintf("%q", legacy), Name: legacy, Source: "legacy"}
	}
	name := ComposeProjectNameFor(projectPath)
	return composeNameDecision{Value: fmt.Sprintf("%q", name), Name: name, Source: "new"}
}

// renderComposeTemplate fills the project name into the compose template.
func renderComposeTemplate(data []byte, d composeNameDecision) ([]byte, error) {
	if !strings.Contains(string(data), composeNamePlaceholder) {
		return nil, fmt.Errorf("compose template has no %s placeholder", composeNamePlaceholder)
	}
	return []byte(strings.Replace(string(data), composeNamePlaceholder, d.Value, 1)), nil
}
