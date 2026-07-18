// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// init_hook.go emits a Claude Code SessionStart hook into .claude/settings.json so
// a fresh (or reaped-and-resumed) session self-bootstraps: it caches MxBuild +
// runtime and provisions the local database, leaving the session ready to
// `mxcli run --local`. Background processes (Postgres, the JVM) are reaped on
// idle, so this idempotent bring-up must run on every session start.
//
// The hook is setup-only (non-blocking): it must return, so it prepares
// prerequisites and exits rather than booting the long-lived warm loop.

// sessionStartHookMarker identifies our hook command so re-running init is
// idempotent and we never clobber a user's own SessionStart hooks.
const sessionStartHookMarker = "run --local --setup"

// sessionStartHookCommand is the shell command the hook runs. It is guarded so a
// missing ./mxcli (or a setup hiccup) never blocks the session from starting.
func sessionStartHookCommand(mprFile string) string {
	return fmt.Sprintf("test -x ./mxcli && ./mxcli run --local --setup --ensure-db -p %s || true", mprFile)
}

// ensureSessionStartHook adds (idempotently) the mxcli bring-up to
// .claude/settings.json, preserving any existing settings/hooks. It returns
// whether a change was written. If settings.json exists but is not valid JSON it
// is left untouched (changed=false) with an error, so we never destroy content.
func ensureSessionStartHook(claudeDir, mprFile string) (changed bool, err error) {
	path := filepath.Join(claudeDir, "settings.json")

	settings := map[string]any{}
	if data, readErr := os.ReadFile(path); readErr == nil {
		if json.Unmarshal(data, &settings) != nil {
			return false, fmt.Errorf("%s exists but is not valid JSON; leaving it untouched — add a SessionStart hook manually", path)
		}
	}

	if updated := addSessionStartHook(settings, sessionStartHookCommand(mprFile)); !updated {
		return false, nil // already present
	}

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return false, err
	}
	out = append(out, '\n')
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// addSessionStartHook inserts a SessionStart command hook into a parsed settings
// map, preserving existing keys and hooks. It returns false if a SessionStart
// hook whose command contains sessionStartHookMarker already exists (idempotent).
// Exported-for-test via the package; operates on the generic JSON shape so it
// never drops unknown settings.
func addSessionStartHook(settings map[string]any, command string) bool {
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	sessionStart, _ := hooks["SessionStart"].([]any)

	for _, entry := range sessionStart {
		em, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		inner, _ := em["hooks"].([]any)
		for _, h := range inner {
			hm, ok := h.(map[string]any)
			if !ok {
				continue
			}
			if c, _ := hm["command"].(string); strings.Contains(c, sessionStartHookMarker) {
				return false // already configured
			}
		}
	}

	sessionStart = append(sessionStart, map[string]any{
		"hooks": []any{
			map[string]any{"type": "command", "command": command},
		},
	})
	hooks["SessionStart"] = sessionStart
	settings["hooks"] = hooks
	return true
}
