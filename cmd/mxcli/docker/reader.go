// SPDX-License-Identifier: Apache-2.0

package docker

// openReadOnly is this package's one way to read a project.
//
// It replaces mpr.Open: every caller here holds the result only to read
// (project security, settings, module settings, unit lists, the MPR format
// version), and all of those are FullBackend methods. Read-only because nothing
// in this package should lock a project Studio Pro may have open — the build
// and check paths run against a developer's working copy.
//
// Callers must Disconnect, not Close; the backend has no Close.

import (
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
)

func openReadOnly(projectPath string) (backend.FullBackend, error) {
	b := modelsdkbackend.New()
	if err := b.ConnectReadOnly(projectPath); err != nil {
		return nil, err
	}
	return b, nil
}

// openForWriting is openReadOnly's counterpart for the two paths that mutate:
// the demo-user bootstrap in build.go and the widget harvest in harvest.go.
func openForWriting(projectPath string) (backend.FullBackend, error) {
	b := modelsdkbackend.New()
	if err := b.Connect(projectPath); err != nil {
		return nil, err
	}
	return b, nil
}
