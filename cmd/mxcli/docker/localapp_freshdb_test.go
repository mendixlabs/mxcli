// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// `mxcli test --local --db-type hsqldb` booted on the file database the
// previous run left, so data an after-startup flow seeded under an earlier
// model stayed, an idempotent init skipped seeding, and tests failed on stale
// data (ako/mxcli#1853). FreshDB removes the run's database before boot —
// and only that one: the dev loop's database beside it is not the run's.
func TestPrepareLocalAppDatabase_FreshDBRemovesOnlyItsOwnDatabase(t *testing.T) {
	proj := t.TempDir()
	o := LocalAppOptions{
		ProjectPath: filepath.Join(proj, "App.mpr"),
		DB:          DBConfig{Type: RuntimeDatabaseType(DBTypeHSQLDB), Name: "app_test"},
		FreshDB:     true,
	}
	o.applyDefaults()
	stale := filepath.Join(o.DeployDir, "data", "database", "hsqldb", "app_test", "app_test.script")
	devDB := filepath.Join(o.DeployDir, "data", "database", "hsqldb", "app", "app.script")
	for _, f := range []string{stale, devDB} {
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("INSERT INTO ..."), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := prepareLocalAppDatabase(&o, io.Discard); err != nil {
		t.Fatalf("prepareLocalAppDatabase: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(stale)); !os.IsNotExist(err) {
		t.Errorf("the previous run's database is still there (stat err %v); the runtime boots on its data", err)
	}
	if _, err := os.Stat(devDB); err != nil {
		t.Errorf("the dev loop's database was touched: %v", err)
	}
}

// Without FreshDB — `run --local --db-type hsqldb`, the developer's own
// data — nothing is removed. Also the control for the test above.
func TestPrepareLocalAppDatabase_FileDBKeptByDefault(t *testing.T) {
	proj := t.TempDir()
	o := LocalAppOptions{
		ProjectPath: filepath.Join(proj, "App.mpr"),
		DB:          DBConfig{Type: RuntimeDatabaseType(DBTypeHSQLDB), Name: "app"},
	}
	o.applyDefaults()
	f := filepath.Join(o.DeployDir, "data", "database", "hsqldb", "app", "app.script")
	if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := prepareLocalAppDatabase(&o, io.Discard); err != nil {
		t.Fatalf("prepareLocalAppDatabase: %v", err)
	}
	if _, err := os.Stat(f); err != nil {
		t.Errorf("the database was removed without FreshDB: %v", err)
	}
}

// The name becomes a path under the deployment directory. One that is not a
// single path element is refused rather than resolved, so a reset can never
// reach above the hsqldb directory.
func TestPrepareLocalAppDatabase_FreshDBRefusesAPathName(t *testing.T) {
	for _, name := range []string{"", ".", "..", "../app", "a/b"} {
		o := LocalAppOptions{
			ProjectPath: filepath.Join(t.TempDir(), "App.mpr"),
			DB:          DBConfig{Type: RuntimeDatabaseType(DBTypeHSQLDB), Name: name},
			FreshDB:     true,
		}
		o.applyDefaults()
		o.DB.Name = name // applyDefaults may fill an empty name; test the raw value
		if err := prepareLocalAppDatabase(&o, io.Discard); err == nil {
			t.Errorf("name %q: reset accepted, want a refusal", name)
		}
	}
}

// PostgreSQL's scratch database is emptied in place, as the app role, inside
// that database: `DROP OWNED BY CURRENT_USER` removes every table the runtime
// created there and nothing else — the database itself, and every other
// database the role owns (the dev loop's), are out of its reach. Dropping and
// recreating the database instead would need CREATEDB at the moment the data is
// already gone. Measured on PostgreSQL 15: 88 relations to 0, and the role can
// still create in public afterwards.
func TestEmptyPostgresDatabaseCommand(t *testing.T) {
	db := DBConfig{Type: "PostgreSQL", Host: "127.0.0.1:5433", Name: "app_test", User: "mendix", Password: "s3cret"}
	cmd, err := emptyPostgresDatabaseCommand(db)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(cmd.Args, " ")
	for _, want := range []string{"-d app_test", "-U mendix", "-h 127.0.0.1", "-p 5433", "ON_ERROR_STOP=1", "DROP OWNED BY CURRENT_USER"} {
		if !strings.Contains(args, want) {
			t.Errorf("command %q is missing %q", args, want)
		}
	}
	if strings.Contains(args, "s3cret") {
		t.Errorf("the password is on the command line, readable in the process list: %q", args)
	}
	if !slices.Contains(cmd.Env, "PGPASSWORD=s3cret") {
		t.Error("the password does not reach psql through PGPASSWORD")
	}
}

func TestEmptyPostgresDatabaseCommand_RefusesUnsafeNames(t *testing.T) {
	for _, db := range []DBConfig{
		{Name: "app_test; drop database app", User: "mendix", Host: "127.0.0.1:5432"},
		{Name: "app_test", User: "x y", Host: "127.0.0.1:5432"},
		{Name: "", User: "mendix", Host: "127.0.0.1:5432"},
	} {
		if _, err := emptyPostgresDatabaseCommand(db); err == nil {
			t.Errorf("%+v: accepted, want a refusal", db)
		}
	}
}
