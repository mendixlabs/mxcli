// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
)

// settingsReader serves the project settings; everything else is empty.
type settingsReader struct {
	rawUnitReader
	ps *model.ProjectSettings
}

func (r settingsReader) GetProjectSettings() (*model.ProjectSettings, error) { return r.ps, nil }

// CE0582 is the React client's error. A Mendix 11 app can still run the classic
// client (Web UI → UseOptimizedClient = No), and there mxbuild builds the legacy
// image widgets clean: 57 MPR012 warnings, each saying "mxbuild reports CE0582",
// on an app at 0 errors (mendixlabs/mxcli#1226, ako/mxcli#1287). Measured on 11.14.0 for the same CE0582 on a classic
// drop-down: an error under "Yes", clean under "No" and "MigrationMode"
// (usesReactClient in mdl/executor).
func TestReactUnsupportedWidgetRule_FollowsTheProjectsWebClient(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	for _, stmt := range []string{
		`INSERT INTO modules_data (Id, Name, QualifiedName, ModuleName, Source) VALUES
			('m1', 'MyFirstModule', 'MyFirstModule', 'MyFirstModule', '')`,
		`INSERT INTO pages_data (Id, Name, QualifiedName, ModuleName, LayoutRef) VALUES
			('p1', 'P_Images', 'MyFirstModule.P_Images', 'MyFirstModule', 'Atlas_Core.Atlas_Default')`,
		`INSERT INTO widgets_data (Id, Name, WidgetType, ContainerId, ContainerQualifiedName, ContainerType, ModuleName) VALUES
			('w1', 'img1', 'Forms$StaticImageViewer', 'p1', 'MyFirstModule.P_Images', 'PAGE', 'MyFirstModule')`,
	} {
		if _, err := cat.CatalogDB().Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	check := func(reader linter.LintReader) linter.Violation {
		t.Helper()
		vs := NewReactUnsupportedWidgetRule().Check(linter.NewLintContext(cat, reader))
		if len(vs) != 1 {
			t.Fatalf("want one MPR012 for img1, got %d: %v", len(vs), vs)
		}
		return vs[0]
	}
	client := func(v string) linter.LintReader {
		return settingsReader{ps: &model.ProjectSettings{WebUI: &model.WebUISettings{UseOptimizedClient: v}}}
	}

	// The React client: CE0582 is real, so the warning stands.
	if v := check(client("Yes")); v.Severity != linter.SeverityWarning || !strings.Contains(v.Message, "CE0582") {
		t.Errorf("React client: got %s %q, want a warning naming CE0582", v.Severity, v.Message)
	}

	// The classic client and migration mode build clean: an advisory, and no
	// claim that mxbuild reports anything.
	for _, setting := range []string{"No", "MigrationMode"} {
		v := check(client(setting))
		if v.Severity != linter.SeverityInfo {
			t.Errorf("UseOptimizedClient %s: severity %s, want info", setting, v.Severity)
		}
		if strings.Contains(v.Message, "reports CE0582") || strings.Contains(v.Message, "only client") {
			t.Errorf("UseOptimizedClient %s: message claims an error mxbuild does not report: %q", setting, v.Message)
		}
		if !strings.Contains(v.Message, "React client") {
			t.Errorf("UseOptimizedClient %s: message no longer says why it matters: %q", setting, v.Message)
		}
	}

	// Settings that cannot be read leave the rule as it was.
	if v := check(nil); v.Severity != linter.SeverityWarning {
		t.Errorf("no reader: severity %s, want the warning unchanged", v.Severity)
	}
}
