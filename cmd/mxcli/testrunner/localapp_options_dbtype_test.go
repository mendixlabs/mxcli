// SPDX-License-Identifier: Apache-2.0

// `test --local` hardcoded a scratch PostgreSQL (localAppOptions set EnsureDB and a
// derived name, and no flag could change either), while `run --local` has had
// `--db-type hsqldb` — the runtime's built-in file database, no server — since
// local DB support landed. So a project with no reachable PostgreSQL, or a
// developer who simply does not have one running, could `run --local` but could
// not `test --local` at all. The file database needs no provisioning, so
// EnsureDB has to go with it: leaving it on asks for a role and a database that
// the file database does not have.
package testrunner

import (
	"io"
	"testing"
)

// The flag has to reach the boot for a `--db-type hsqldb` suite to run at all, and
// the value has to be the runtime's spelling rather than the flag's. This is
// spelled the same way `run --local` spells it, so a test script written against
// one works against the other.
func TestLocalAppOptions_CarriesTheDbTypeToTheBoot(t *testing.T) {
	opts := RunOptions{ProjectPath: "/tmp/app/App.mpr", DBType: "hsqldb"}

	got := localAppOptions(opts, "log", nil, io.Discard)

	if got.DB.Type != "HSQLDB" {
		t.Errorf("DB.Type = %q, want HSQLDB — the runtime enum is upper case, and a "+
			"lower-case value boots a runtime that does not recognise its own database",
			got.DB.Type)
	}
}

// The scratch database is what lets a `run --local` dev loop keep serving the same
// project while tests run. Under the file database there is nothing to keep
// separate — it is a file under the project's own deployment directory — so the
// scratch NAME and the provisioning both have to go, or --db-type hsqldb fails on
// a --ensure-db that can never succeed.
func TestLocalAppOptions_FileDatabaseNeedsNoScratchDatabaseNorProvisioning(t *testing.T) {
	opts := RunOptions{ProjectPath: "/tmp/app/App.mpr", DBType: "hsqldb"}

	got := localAppOptions(opts, "log", nil, io.Discard)

	if got.EnsureDB {
		t.Error("EnsureDB is true under --db-type hsqldb; the file database has no role " +
			"and no database to create, so provisioning it cannot succeed")
	}
	if got.DB.Host != "" || got.DB.User != "" || got.DB.Password != "" {
		t.Errorf("the file database has no host and no credentials, but got host=%q user=%q password=%q",
			got.DB.Host, got.DB.User, got.DB.Password)
	}
}

// The control for the two above: without the flag nothing changes. A test run on
// a machine that never passes --db-type must keep getting the scratch PostgreSQL
// it has always got, or this change breaks every existing suite.
func TestLocalAppOptions_DefaultIsUnchangedPostgresScratchDatabase(t *testing.T) {
	got := localAppOptions(RunOptions{ProjectPath: "/tmp/app/App.mpr"}, "log", nil, io.Discard)

	if want := "app" + localTestDBSuffix; got.DB.Name != want {
		t.Errorf("DB.Name = %q, want %q", got.DB.Name, want)
	}
	if !got.EnsureDB {
		t.Error("EnsureDB is false by default; the scratch database would have to exist already")
	}
	if got.DB.Type != "PostgreSQL" {
		t.Errorf("DB.Type = %q, want PostgreSQL by default", got.DB.Type)
	}
}

// An unknown value is a user error and must be reported rather than normalised
// away — `run --local` already refuses one (docker.NormalizeDBType), and a test
// run that silently fell back to PostgreSQL would fail much later, on a
// connection error that says nothing about the typo.
func TestResolveTestDBType_RejectsAnUnknownDbType(t *testing.T) {
	if _, err := ResolveTestDBType("mysql"); err == nil {
		t.Error("ResolveTestDBType(\"mysql\") returned no error; a typo would silently " +
			"fall back to PostgreSQL and surface as an unrelated connection failure")
	}
}

// The control for the rejection above: the two supported spellings and the empty
// default all resolve, and to the runtime's upper-case enum rather than the
// flag's spelling.
func TestResolveTestDBType_AcceptedSpellings(t *testing.T) {
	for raw, want := range map[string]string{
		"":           "PostgreSQL",
		"postgresql": "PostgreSQL",
		"postgres":   "PostgreSQL",
		"hsqldb":     "HSQLDB",
		" HSQLDB ":   "HSQLDB",
	} {
		got, err := ResolveTestDBType(raw)
		if err != nil {
			t.Errorf("ResolveTestDBType(%q) returned %v, want %q", raw, err, want)
			continue
		}
		if got != want {
			t.Errorf("ResolveTestDBType(%q) = %q, want %q", raw, got, want)
		}
	}
}
