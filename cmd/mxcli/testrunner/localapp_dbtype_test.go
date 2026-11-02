// SPDX-License-Identifier: Apache-2.0

// `mxcli test --local` could only boot against a scratch PostgreSQL, so on a
// machine without one it stopped with "ensuring database: no local PostgreSQL
// superuser available to create the role/database" — while `run --local
// --db-type hsqldb` served the same project fine. mendixlabs/mxcli#1274.
package testrunner

import (
	"io"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

func TestLocalAppOptions_HSQLDBBootsOnTheFileDatabase(t *testing.T) {
	opts := RunOptions{ProjectPath: "/tmp/app/App.mpr", DBType: docker.DBTypeHSQLDB}

	got := localAppOptions(opts, "log", nil, io.Discard)

	// IsFileBased keys on the runtime spelling, so the canonical "hsqldb" has to
	// be translated here or every file-based branch downstream is skipped.
	if !got.DB.IsFileBased() {
		t.Errorf("DB.Type = %q, want the runtime's file database (%q)",
			got.DB.Type, docker.RuntimeDatabaseType(docker.DBTypeHSQLDB))
	}
	if got.EnsureDB {
		t.Error("EnsureDB is true; it provisions PostgreSQL and fails with \"no local " +
			"PostgreSQL superuser\" on exactly the machines --db-type hsqldb is for")
	}
	if got.DB.Host != "" || got.DB.User != "" || got.DB.Password != "" {
		t.Errorf("DB = %+v, want no host or credentials for a file database", got.DB)
	}
}

// The scratch name is what keeps the suite off the dev loop's data. For HSQLDB
// the name selects the database files, so dropping the suffix would point the
// tests at the very files a concurrent `run --local --db-type hsqldb` holds open
// (and writes into its data). Same suffix for both types.
func TestLocalAppOptions_HSQLDBKeepsTheScratchName(t *testing.T) {
	got := localAppOptions(RunOptions{ProjectPath: "/tmp/app/App.mpr", DBType: docker.DBTypeHSQLDB},
		"log", nil, io.Discard)
	if want := "app" + localTestDBSuffix; got.DB.Name != want {
		t.Errorf("DB.Name = %q, want %q — the dev loop's database is %q", got.DB.Name, want, "app")
	}
}

func TestLocalAppOptions_DefaultStaysPostgreSQL(t *testing.T) {
	for _, dbType := range []string{"", docker.DBTypePostgreSQL} {
		got := localAppOptions(RunOptions{ProjectPath: "/tmp/app/App.mpr", DBType: dbType}, "log", nil, io.Discard)
		if got.DB.IsFileBased() || !got.EnsureDB {
			t.Errorf("DBType %q: DB=%+v EnsureDB=%v, want the scratch PostgreSQL as before",
				dbType, got.DB, got.EnsureDB)
		}
	}
}

func TestResolveTestDBType(t *testing.T) {
	cases := []struct {
		raw           string
		local, attach bool
		want          string
		wantErr       string
	}{
		{raw: "", local: false, want: ""},
		{raw: "", local: true, want: docker.DBTypePostgreSQL},
		{raw: "", local: true, attach: true, want: ""},
		{raw: "hsqldb", local: true, want: docker.DBTypeHSQLDB},
		{raw: "HSQLDB", local: true, want: docker.DBTypeHSQLDB},
		{raw: "postgres", local: true, want: docker.DBTypePostgreSQL},
		// A typo is refused before anything is built or booted.
		{raw: "mysql", local: true, wantErr: "unknown --db-type"},
		// Only a --local run boots a database of its own; accepting the flag
		// elsewhere and ignoring it is how a user ends up testing on the wrong one.
		{raw: "hsqldb", local: false, wantErr: "--db-type applies only to a --local test run"},
		{raw: "hsqldb", local: true, attach: true, wantErr: "--db-type applies only to a --local test run"},
	}
	for _, c := range cases {
		got, err := ResolveTestDBType(c.raw, c.local, c.attach)
		if c.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("ResolveTestDBType(%q, local=%v, attach=%v) err = %v, want %q",
					c.raw, c.local, c.attach, err, c.wantErr)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("ResolveTestDBType(%q, local=%v, attach=%v) = %q, %v; want %q",
				c.raw, c.local, c.attach, got, err, c.want)
		}
	}
}

// A test run's file database is its own scratch, so it starts empty: kept
// between runs, data an earlier run's after-startup seeded made an idempotent
// init skip seeding and the tests fail on it (ako/mxcli#1853).
func TestLocalAppOptions_HSQLDBStartsFresh(t *testing.T) {
	got := localAppOptions(RunOptions{ProjectPath: "/tmp/app/App.mpr", DBType: docker.DBTypeHSQLDB},
		"log", nil, io.Discard)
	if !got.FreshFileDB {
		t.Error("FreshFileDB is false; the run boots on the previous run's data")
	}
	if pg := localAppOptions(RunOptions{ProjectPath: "/tmp/app/App.mpr"}, "log", nil, io.Discard); pg.FreshFileDB {
		t.Error("FreshFileDB set for PostgreSQL, where it means nothing")
	}
}
