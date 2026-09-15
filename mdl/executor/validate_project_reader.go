// SPDX-License-Identifier: Apache-2.0

package executor

// How a check-time validator reads the project.
//
// The executor must not import sdk/mpr (CLAUDE.md, "Backend abstraction
// compliance"); two validators did, which is what this replaces
// (docs/plans/2026-09-14-retire-legacy-engine.md, Phase 4a).
//
// These open their OWN connection rather than using ctx.Backend, because
// ValidateProgram takes a project PATH, not a backend: `mxcli check
// --references` validates a script against a project it never connects an
// executor to. Threading a backend down would change a public signature and
// every caller for no gain, since the connection here is read-only, short and
// discarded.
//
// Read-only for the obvious reason — a validator must never lock or touch the
// project it is inspecting, and `check` runs against a developer's working copy
// while Studio Pro may have it open.

import (
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
)

// openProjectForValidation connects read-only, or returns nil.
//
// A nil return is the fail-open contract these validators already had: a
// project that cannot be opened silences the rule rather than failing the check
// on something it could not inspect. Callers branch on nil, not on an error,
// which is why this returns no error to ignore.
func openProjectForValidation(projectPath string) backend.FullBackend {
	if projectPath == "" {
		return nil
	}
	b := modelsdkbackend.New()
	if err := b.ConnectReadOnly(projectPath); err != nil {
		return nil
	}
	return b
}
