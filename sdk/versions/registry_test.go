// SPDX-License-Identifier: Apache-2.0

package versions

import (
	"testing"
)

func TestLoad(t *testing.T) {
	r, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(r.specs) != 3 {
		t.Fatalf("expected 3 specs, got %d", len(r.specs))
	}
	if len(r.index) == 0 {
		t.Fatal("index is empty after load")
	}
}

func TestParseSemVer(t *testing.T) {
	tests := []struct {
		input   string
		want    SemVer
		wantErr bool
	}{
		{"10.18.0", SemVer{10, 18, 0}, false},
		{"11.0.0", SemVer{11, 0, 0}, false},
		{"9.24", SemVer{9, 24, 0}, false},
		{"bad", SemVer{}, true},
	}
	for _, tt := range tests {
		v, err := ParseSemVer(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseSemVer(%q) error=%v, wantErr=%v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && v != tt.want {
			t.Errorf("ParseSemVer(%q) = %v, want %v", tt.input, v, tt.want)
		}
	}
}

func TestSemVerAtLeast(t *testing.T) {
	tests := []struct {
		v, other SemVer
		want     bool
	}{
		{SemVer{10, 18, 0}, SemVer{10, 18, 0}, true},
		{SemVer{11, 0, 0}, SemVer{10, 18, 0}, true},
		{SemVer{10, 17, 0}, SemVer{10, 18, 0}, false},
		{SemVer{10, 18, 1}, SemVer{10, 18, 0}, true},
		{SemVer{10, 18, 0}, SemVer{10, 18, 1}, false},
		{SemVer{9, 0, 0}, SemVer{10, 0, 0}, false},
	}
	for _, tt := range tests {
		got := tt.v.AtLeast(tt.other)
		if got != tt.want {
			t.Errorf("%v.AtLeast(%v) = %v, want %v", tt.v, tt.other, got, tt.want)
		}
	}
}

func TestIsAvailable(t *testing.T) {
	r, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	tests := []struct {
		area, name string
		version    SemVer
		want       bool
	}{
		// View entities require 10.21+: the source document mxcli writes is
		// a 10.21 type (ako/mxcli#1811), though Mendix had them from 10.18.
		{"domain_model", "view_entities", SemVer{10, 21, 0}, true},
		{"domain_model", "view_entities", SemVer{10, 20, 0}, false},
		{"domain_model", "view_entities", SemVer{11, 0, 0}, true},
		// Basic entities available in 9.x+
		{"domain_model", "entities", SemVer{9, 0, 0}, true},
		// Page parameters require 9.4+ (see page_parameter_floor_test.go)
		{"pages", "page_parameters", SemVer{9, 3, 0}, false},
		{"pages", "page_parameters", SemVer{10, 24, 0}, true},
		{"pages", "page_parameters", SemVer{11, 0, 0}, true},
		// Unknown feature
		{"domain_model", "teleportation", SemVer{11, 0, 0}, false},
	}
	for _, tt := range tests {
		got := r.IsAvailable(tt.area, tt.name, tt.version)
		if got != tt.want {
			t.Errorf("IsAvailable(%s, %s, %v) = %v, want %v",
				tt.area, tt.name, tt.version, got, tt.want)
		}
	}
}

func TestFeaturesForVersion(t *testing.T) {
	r, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	features := r.FeaturesForVersion(SemVer{10, 24, 0})
	if len(features) == 0 {
		t.Fatal("expected features for 10.24.0, got none")
	}

	// Check that view_entities is available at 10.24
	found := false
	for _, f := range features {
		if f.Area == "domain_model" && f.Name == "view_entities" {
			found = true
			if !f.Available {
				t.Error("view_entities should be available at 10.24")
			}
		}
	}
	if !found {
		t.Error("view_entities not found in features list")
	}

	// Check that design_properties_v3 is NOT available at 10.24 (Atlas v3 is 11.0+)
	for _, f := range features {
		if f.Area == "pages" && f.Name == "design_properties_v3" {
			if f.Available {
				t.Error("design_properties_v3 should NOT be available at 10.24")
			}
		}
	}
}

func TestFeaturesAddedSince(t *testing.T) {
	r, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	added := r.FeaturesAddedSince(SemVer{10, 24, 0})
	if len(added) == 0 {
		t.Fatal("expected features added since 10.24.0, got none")
	}

	// design_properties_v3 (11.0+) should be in the list
	found := false
	for _, f := range added {
		if f.Name == "design_properties_v3" {
			found = true
			break
		}
	}
	if !found {
		t.Error("design_properties_v3 should appear in features added since 10.24")
	}

	// entities (10.0+) should NOT be in the list
	for _, f := range added {
		if f.Name == "entities" && f.Area == "domain_model" {
			t.Error("entities should not appear in features added since 10.24")
		}
	}
}

func TestFeaturesInArea(t *testing.T) {
	r, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	features := r.FeaturesInArea("domain_model", SemVer{10, 24, 0})
	if len(features) == 0 {
		t.Fatal("expected domain_model features, got none")
	}

	for _, f := range features {
		if f.Area != "domain_model" {
			t.Errorf("expected area domain_model, got %s", f.Area)
		}
	}
}

func TestAreas(t *testing.T) {
	r, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	areas := r.Areas()
	if len(areas) == 0 {
		t.Fatal("expected areas, got none")
	}

	// Should contain at least domain_model and microflows
	areaSet := make(map[string]bool)
	for _, a := range areas {
		areaSet[a] = true
	}
	for _, want := range []string{"domain_model", "microflows", "pages", "security"} {
		if !areaSet[want] {
			t.Errorf("expected area %q in list", want)
		}
	}
}

func TestDeprecatedPatterns(t *testing.T) {
	r, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	deps := r.DeprecatedPatterns(SemVer{10, 24, 0})
	if len(deps) == 0 {
		t.Fatal("expected deprecated patterns for 10.24, got none")
	}
	if deps[0].ID != "DEP001" {
		t.Errorf("expected DEP001, got %s", deps[0].ID)
	}
}

func TestUpgradeOpportunities(t *testing.T) {
	r, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	ups := r.UpgradeOpportunities(10, 11)
	if len(ups) == 0 {
		t.Fatal("expected upgrade opportunities from 10 to 11, got none")
	}

	// Should not find upgrades for nonexistent path
	ups = r.UpgradeOpportunities(9, 10)
	if len(ups) != 0 {
		t.Errorf("expected no upgrade opportunities from 9 to 10, got %d", len(ups))
	}
}

func TestDisplayName(t *testing.T) {
	e := FeatureEntry{Name: "view_entities"}
	if got := e.DisplayName(); got != "view entities" {
		t.Errorf("DisplayName() = %q, want %q", got, "view entities")
	}
}

// TestAgentDocumentsAreGated is the guard for the version gap reported in
// mxcli-formula1 FINDINGS §53: `show features` listed nothing for agents or MCP,
// so there was no checkFeature() gate and an older project got no actionable
// error.
//
// This matters more than a normal version gate because nothing downstream
// catches it. Agent Editor documents are stored as custom blobs, mxbuild does
// not validate them (it contains no agent-editor strings at all), so an agent
// authored against a pre-11.9 project builds green and simply cannot be opened.
func TestAgentDocumentsAreGated(t *testing.T) {
	reg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	docs := []string{"agent_model", "agent_knowledge_base", "agent_consumed_mcp_service", "agent"}
	for _, name := range docs {
		t.Run(name, func(t *testing.T) {
			// Available on a project new enough for the agent editor...
			if !reg.IsAvailable("agent_documents", name, SemVer{Major: 11, Minor: 9, Patch: 0}) {
				t.Errorf("%s should be available on 11.9.0", name)
			}
			// ...and refused below it, which is the half that was missing.
			if reg.IsAvailable("agent_documents", name, SemVer{Major: 11, Minor: 8, Patch: 0}) {
				t.Errorf("%s must not be reported available on 11.8.0", name)
			}
			if reg.IsAvailable("agent_documents", name, SemVer{Major: 10, Minor: 24, Patch: 0}) {
				t.Errorf("%s must not be reported available on 10.24.0", name)
			}
		})
	}
}

// TestWorkflowGroupsFloorIs11_2 pins the workflow-group gate to the metamodel,
// not to the release notes.
//
// mendixlabs/mxcli#272 states "Workflow Groups are GA from Mendix 11.6". The
// arbiter for whether the document loads is the Model SDK's own
// StructureVersionInfo, and there both Settings$WorkflowGroup and
// WorkflowsProjectSettingsPart.groups read `introduced: "11.2.0"`
// (mendixmodelsdk 4.115.0, package/src/gen/settings.js). Gating on 11.6 would
// refuse four minors of projects that store the property perfectly well.
func TestWorkflowGroupsFloorIs11_2(t *testing.T) {
	reg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cases := []struct {
		v    SemVer
		want bool
	}{
		{SemVer{Major: 10, Minor: 24, Patch: 0}, false},
		{SemVer{Major: 11, Minor: 1, Patch: 0}, false},
		{SemVer{Major: 11, Minor: 2, Patch: 0}, true},
		{SemVer{Major: 11, Minor: 13, Patch: 0}, true},
	}
	for _, tc := range cases {
		if got := reg.IsAvailable("workflows", "groups", tc.v); got != tc.want {
			t.Errorf("IsAvailable(workflows, groups, %v) = %v, want %v", tc.v, got, tc.want)
		}
	}
}

// TestStringAggIsOnTwoReleaseLines: Mendix added STRING_AGG to OQL in 11.2.0
// and, separately, in 10.24.22 on the 10.24 line (ako/mxcli#1650). Two entries
// with a max_version on the first; every view of the registry has to agree on
// what that means — IsAvailable alone did, while `show features` took the
// lowest floor and reported the feature missing on every 11.x.
func TestStringAggIsOnTwoReleaseLines(t *testing.T) {
	reg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cases := []struct {
		v         SemVer
		available bool
		since     string // the floor shown for v
	}{
		{SemVer{10, 24, 21}, false, "10.24.22"},
		{SemVer{10, 24, 22}, true, "10.24.22"},
		{SemVer{11, 0, 0}, false, "11.2.0"},
		{SemVer{11, 1, 0}, false, "11.2.0"},
		{SemVer{11, 2, 0}, true, "11.2.0"},
		{SemVer{11, 6, 6}, true, "11.2.0"},
	}
	for _, c := range cases {
		if got := reg.IsAvailable("oql_functions", "string_agg", c.v); got != c.available {
			t.Errorf("IsAvailable(%s) = %v, want %v", c.v, got, c.available)
		}
		for _, list := range [][]FeatureStatus{reg.FeaturesForVersion(c.v), reg.FeaturesInArea("oql_functions", c.v)} {
			found := false
			for _, f := range list {
				if f.Area != "oql_functions" || f.Name != "string_agg" {
					continue
				}
				found = true
				if f.Available != c.available || f.MinVersion.String() != c.since {
					t.Errorf("at %s: listed available=%v since %s, want %v since %s",
						c.v, f.Available, f.MinVersion, c.available, c.since)
				}
			}
			if !found {
				t.Errorf("at %s: string_agg not listed", c.v)
			}
		}
	}

	// Upgrading from 11.0 adds it at 11.2.0; from 10.24.22 there is nothing to add.
	added := func(since SemVer) string {
		for _, f := range reg.FeaturesAddedSince(since) {
			if f.Area == "oql_functions" && f.Name == "string_agg" {
				return f.MinVersion.String()
			}
		}
		return ""
	}
	if got := added(SemVer{11, 0, 0}); got != "11.2.0" {
		t.Errorf("added since 11.0.0 = %q, want 11.2.0", got)
	}
	if got := added(SemVer{10, 24, 22}); got != "" {
		t.Errorf("added since 10.24.22 = %q, want nothing: it is there already", got)
	}
	if got := added(SemVer{10, 24, 0}); got != "10.24.22" {
		t.Errorf("added since 10.24.0 = %q, want 10.24.22", got)
	}
}

// ako/mxcli#2140: mendix-10.yaml and mendix-11.yaml both list
// message_definition_collection from 10.0.0, and only the 11 file closes it at
// 11.14.99 (Mendix 11.15 removed the collection). dedup kept the first of the
// two — the one without the ceiling — so the registry said a collection is
// available on 11.16, and `mxcli syntax message-definition` printed "10.0.0+".
func TestMessageDefinitionCollectionEndsAt1114(t *testing.T) {
	reg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, c := range []struct {
		v         SemVer
		available bool
	}{
		{SemVer{10, 24, 0}, true},
		{SemVer{11, 14, 0}, true},
		{SemVer{11, 15, 0}, false},
		{SemVer{11, 16, 0}, false},
	} {
		if got := reg.IsAvailable("integration", "message_definition_collection", c.v); got != c.available {
			t.Errorf("IsAvailable(%s) = %v, want %v", c.v, got, c.available)
		}
	}
	e, ok := reg.Lookup("integration", "message_definition_collection")
	if !ok || e.MaxVersion == nil || e.MaxVersion.String() != "11.14.99" {
		t.Errorf("Lookup = %+v, want the 11.14.99 ceiling", e)
	}
}
