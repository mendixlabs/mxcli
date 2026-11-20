// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

// Two configurations may share a name (ako/mxcli#2184). A statement that names
// one cannot say which it means, so it is refused with a message that says so,
// instead of being applied to whichever comes first. The control is the same
// statement against a project whose names are unique.
func TestConfigurationByName_RefusesADuplicateName(t *testing.T) {
	run := func(names ...string) (error, bool) {
		ps := &model.ProjectSettings{Configuration: &model.ConfigurationSettings{}}
		for _, n := range names {
			ps.Configuration.Configurations = append(ps.Configuration.Configurations,
				&model.ServerConfiguration{Name: n, HttpPortNumber: 8080})
		}
		wrote := false
		mb := &mock.MockBackend{
			IsConnectedFunc:           func() bool { return true },
			GetProjectSettingsFunc:    func() (*model.ProjectSettings, error) { return ps, nil },
			UpdateProjectSettingsFunc: func(*model.ProjectSettings) error { wrote = true; return nil },
		}
		ctx, _ := newMockCtx(t, withBackend(mb))
		err := alterSettingsConfiguration(ctx, ps, &ast.AlterSettingsStmt{
			Section: "configuration", ConfigName: "Default",
			Properties: map[string]any{"HttpPortNumber": "8081"},
		})
		if err == nil {
			err = dropConfiguration(ctx, &ast.DropConfigurationStmt{Name: "Default"})
		}
		return err, wrote
	}

	if err, wrote := run("Default", "Other"); err != nil || !wrote {
		t.Fatalf("control: unique name refused or not written: err=%v wrote=%v", err, wrote)
	}
	err, wrote := run("Default", "default")
	if err == nil || !strings.Contains(err.Error(), "not unique") {
		t.Fatalf("duplicate name: err = %v, want a 'not unique' refusal", err)
	}
	if wrote {
		t.Error("duplicate name: settings were written")
	}
}
