// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/marketplace"
)

// Package metadata is left out of every install (mendixlabs/mxcli#1487), so it
// is named on one line; a widget the package would have rolled back keeps its
// own line, because that one is a decision the user may want to revisit.
func TestReportSkippedFiles_PackageMetadataOnOneLine(t *testing.T) {
	var out bytes.Buffer
	reportSkippedFiles(&out, []marketplace.SkippedFile{
		{Path: "License.txt", Reason: marketplace.ReasonNotAProjectFile},
		{Path: "manifest.json", Reason: marketplace.ReasonNotAProjectFile},
		{Path: "widgets/Charts.mpk", Reason: "older", Kept: "3.11.3", Offered: "3.4.0"},
	})
	got := out.String()

	if n := strings.Count(got, "License.txt"); n != 1 {
		t.Errorf("License.txt named %d times, want once:\n%s", n, got)
	}
	if !strings.Contains(got, "Left out 2 package file(s) package.xml does not list as project files: License.txt, manifest.json.") {
		t.Errorf("metadata not reported on one line:\n%s", got)
	}
	if !strings.Contains(got, "widgets/Charts.mpk — kept 3.11.3, package ships 3.4.0") {
		t.Errorf("kept widget no longer reported:\n%s", got)
	}
}

// An add-on module package an install or update wrote is named, since it is
// the one bundled file with no unit in the .mpr (ako/mxcli#2205).
func TestReportAddOnModules_NamesTheMxmoduleWritten(t *testing.T) {
	var out bytes.Buffer
	reportAddOnModules(&out, []string{
		filepath.Join("modules", "AgentEditor.mxmodule"),
		filepath.Join("themesource", "agenteditorcommons", "settings.json"),
	})
	got := out.String()
	if !strings.Contains(got, "Add-on module package written: modules/AgentEditor.mxmodule") {
		t.Errorf("add-on not reported:\n%s", got)
	}
	if strings.Contains(got, "settings.json") {
		t.Errorf("an ordinary bundled file was reported as an add-on:\n%s", got)
	}
}
