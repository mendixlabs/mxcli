// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// strictModeReader is a minimalReader that answers project security and the
// Web UI client setting.
type strictModeReader struct {
	minimalReader
	ps     *security.ProjectSecurity
	client string
}

func (r *strictModeReader) GetProjectSecurity() (*security.ProjectSecurity, error) { return r.ps, nil }
func (r *strictModeReader) GetProjectSettings() (*model.ProjectSettings, error) {
	return &model.ProjectSettings{WebUI: &model.WebUISettings{UseOptimizedClient: r.client}}, nil
}

// SEC005 explains strict mode as Mendix does — it restricts the client API —
// suggests MDL that is current, and stays quiet where strict mode cannot be
// turned on: the classic web client (ako/mxcli#2550).
func TestSEC005StrictMode(t *testing.T) {
	r, err := linter.LoadStarlarkRule(filepath.Join("..", "..", ".claude", "lint-rules", "sec_strict_mode.star"))
	if err != nil {
		t.Fatal(err)
	}
	run := func(level string, strict bool, client string) []linter.Violation {
		reader := &strictModeReader{ps: &security.ProjectSecurity{SecurityLevel: level, StrictMode: strict}, client: client}
		return r.Check(linter.NewLintContext(newSingleModuleCatalog(t, model.ID("mod-1")), reader))
	}

	got := run("CheckEverything", false, "Yes")
	if len(got) != 1 {
		t.Fatalf("production, React client, strict mode off: want 1 violation, got %v", got)
	}
	v := got[0]
	if !strings.Contains(v.Message, "client API") || strings.Contains(v.Message, "XPath") || strings.Contains(v.Message, "CVE") {
		t.Errorf("the message says what strict mode restricts, not an unrelated CVE: %q", v.Message)
	}
	if !strings.Contains(v.Suggestion, "alter app security ( StrictMode: true );") || strings.Contains(strings.ToLower(v.Suggestion), "project security") {
		t.Errorf("the suggestion is current MDL: %q", v.Suggestion)
	}

	for _, c := range []struct {
		level  string
		strict bool
		client string
		why    string
	}{
		{"CheckEverything", true, "Yes", "strict mode is on"},
		{"Prototype", false, "Yes", "strict mode applies at Production only"},
		{"CheckEverything", false, "No", "the classic web client cannot enable strict mode"},
	} {
		if got := run(c.level, c.strict, c.client); len(got) != 0 {
			t.Errorf("%s: want no violation, got %v", c.why, got)
		}
	}
}
