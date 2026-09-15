// SPDX-License-Identifier: Apache-2.0

package executor

// Both validators that read a project FAIL OPEN: an unreadable project silences
// the rule rather than failing the check on something it could not inspect.
// That is the right behaviour and it makes a broken reader SILENT — the rule
// simply stops firing and every existing test still passes.
//
// When these moved off sdk/mpr (Phase 4a of
// docs/plans/2026-09-14-retire-legacy-engine.md), coverage of offlineProfilesIn,
// projectEntityFacts and openProjectForValidation was 0.0%. These exercise them
// against a real project, so the port is measured rather than assumed.

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// projectFixture copies the shared fixture and returns its .mpr path.
func projectFixture(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	if err := os.CopyFS(dst, os.DirFS("../../testdata/expr-checker")); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	return filepath.Join(dst, "minimal.mpr")
}

// execAgainst runs MDL against a project, for tests that need to seed state.
func execAgainst(t *testing.T, projectPath, mdl string) {
	t.Helper()
	exec := New(&bytes.Buffer{})
	exec.SetQuiet(true)
	exec.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	t.Cleanup(func() { exec.Close() })
	run(t, exec, "CONNECT LOCAL '"+visitor.QuoteString(projectPath)+"'")
	run(t, exec, mdl)
}

func TestOpenProjectForValidation_ConnectsAndRefusesJunk(t *testing.T) {
	if b := openProjectForValidation(projectFixture(t)); b == nil {
		t.Fatal("a real project did not open")
	} else {
		_ = b.Disconnect()
	}
	// The fail-open contract: no path, and a path that is not a project, both
	// yield nil rather than an error the callers would have to handle.
	if b := openProjectForValidation(""); b != nil {
		t.Error("an empty path opened something")
	}
	if b := openProjectForValidation(filepath.Join(t.TempDir(), "nope.mpr")); b != nil {
		t.Error("a nonexistent project opened something")
	}
}

// projectEntityFacts is the half of MDL-MAP03 that needs a project: without it
// every persistability question is unanswerable and the rule goes quiet.
func TestProjectEntityFacts_ReadsTheDomainModel(t *testing.T) {
	facts, reader := projectEntityFacts(projectFixture(t))
	if reader == nil {
		t.Fatal("projectEntityFacts returned no reader — the project did not open")
	}
	t.Cleanup(func() { _ = reader.Disconnect() })

	if len(facts) == 0 {
		t.Fatal("no entity facts read from a project that has entities — " +
			"MDL-MAP03's persistability half would be silently inert")
	}
	// Qualified, and carrying the persistable flag: a map keyed on bare names
	// would collide across modules, and the flag is the thing being asked for.
	var sawQualified, sawPersistable bool
	for name, f := range facts {
		if containsDot(name) {
			sawQualified = true
		}
		if f.persistable {
			sawPersistable = true
		}
	}
	if !sawQualified {
		t.Errorf("entity names are not qualified; got e.g. %v", firstKey(facts))
	}
	if !sawPersistable {
		t.Error("no entity reported persistable — the fixture has persistent entities")
	}
}

// offlineProfilesIn gates MDL-OFFLINE01 entirely: an empty result disables the
// rule, so a broken reader is indistinguishable from a project with no offline
// profile. The fixture ships only an online one, which is why this seeds one.
func TestOfflineProfilesIn_FindsASeededOfflineProfile(t *testing.T) {
	p := projectFixture(t)

	// Control first: the stock fixture has no offline profile, so a reader that
	// invented one would be caught here rather than passing the assertion below.
	if got := offlineProfilesIn(p); len(got) != 0 {
		t.Fatalf("stock fixture reports offline profiles %v, want none", got)
	}

	// The profile NAME is not free: Mendix fixes the set, and the offline ones
	// are the *Offline variants. An invented name is refused by the executor.
	execAgainst(t, p, `create or replace navigation "PhoneOffline"
  home page "MyFirstModule"."Home_Web";`)

	got := offlineProfilesIn(p)
	if len(got) != 1 || got[0] != "PhoneOffline" {
		t.Fatalf("offline profiles = %v, want [PhoneOffline] — with this empty, "+
			"MDL-OFFLINE01 is disabled and a broken reader is indistinguishable "+
			"from a project that has no offline profile", got)
	}
}

func containsDot(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			return true
		}
	}
	return false
}

func firstKey(m map[string]entityFacts) string {
	for k := range m {
		return k
	}
	return ""
}
