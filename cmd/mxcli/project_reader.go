// SPDX-License-Identifier: Apache-2.0

package main

// How this package opens a project for reading.
//
// Phase 4a of docs/plans/2026-09-14-retire-legacy-engine.md: these commands
// held a concrete *sdk/mpr.Reader. Everything they call — GetMendixVersion,
// ProjectVersion, ListPages, ListSnippets, ListModuleSettings,
// FindCustomWidgetType — is on backend.FullBackend, so the only real change is
// where the reader comes from.
//
// Read-only for the reason it always is here: `new`, `setup`, `serve`,
// `sync-java-deps` and the two check commands run against a developer's working
// copy, which Studio Pro may have open. None of them writes through this reader.

import (
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
)

// openProjectReadOnly connects to projectPath read-only.
//
// The caller owns the returned backend and must Disconnect it.
func openProjectReadOnly(projectPath string) (backend.FullBackend, error) {
	b := modelsdkbackend.New()
	if err := b.ConnectReadOnly(projectPath); err != nil {
		return nil, err
	}
	return b, nil
}
