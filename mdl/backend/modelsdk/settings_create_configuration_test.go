// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"path/filepath"
	"testing"

	"github.com/mendixlabs/mxcli/internal/testutil"
	"github.com/mendixlabs/mxcli/model"
)

// `create configuration 'Staging'` on a Mendix 11.14 project was refused with
// "3 element id(s) are used more than once": the new configuration copies the
// Default one as its shape template, and on 11.12+ that one carries a nested
// OpenTelemetry part (and its Logs / Traces children) with $IDs of its own. The
// 11.6 and 11.13 fixtures have no such part, which is why it went unnoticed.
func TestUpdateProjectSettings_CreateConfigurationOn1114(t *testing.T) {
	dst := t.TempDir()
	if err := testutil.CopyFixture(dst, "../../../testdata/testapp-views"); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	proj := filepath.Join(dst, "TestApp.mpr")

	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	ps, err := b.GetProjectSettings()
	if err != nil {
		t.Fatalf("GetProjectSettings: %v", err)
	}
	staging := &model.ServerConfiguration{
		Name: "Staging", DatabaseType: "Hsqldb", HttpPortNumber: 8080, ServerPortNumber: 8090,
	}
	staging.TypeName = "Settings$ServerConfiguration"
	ps.Configuration.Configurations = append(ps.Configuration.Configurations, staging)
	if err := b.UpdateProjectSettings(ps); err != nil {
		t.Fatalf("UpdateProjectSettings refused a new configuration: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })
	ps2, err := b2.GetProjectSettings()
	if err != nil {
		t.Fatalf("GetProjectSettings(2): %v", err)
	}
	var names []string
	for _, c := range ps2.Configuration.Configurations {
		names = append(names, c.Name)
	}
	if len(names) != 2 || names[1] != "Staging" {
		t.Errorf("configurations read back as %v, want [Default Staging]", names)
	}
}
