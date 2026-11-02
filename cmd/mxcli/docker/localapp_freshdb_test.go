// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// `mxcli test --local --db-type hsqldb` booted on the file database the
// previous run left, so data an after-startup flow seeded under an earlier
// model stayed, an idempotent init skipped seeding, and tests failed on stale
// data (ako/mxcli#1853). FreshFileDB removes the run's database before boot —
// and only that one: the dev loop's database beside it is not the run's.
func TestPrepareLocalAppDatabase_FreshFileDBRemovesOnlyItsOwnDatabase(t *testing.T) {
	proj := t.TempDir()
	o := LocalAppOptions{
		ProjectPath: filepath.Join(proj, "App.mpr"),
		DB:          DBConfig{Type: RuntimeDatabaseType(DBTypeHSQLDB), Name: "app_test"},
		FreshFileDB: true,
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

// Without FreshFileDB — `run --local --db-type hsqldb`, the developer's own
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
		t.Errorf("the database was removed without FreshFileDB: %v", err)
	}
}

// The name becomes a path under the deployment directory. One that is not a
// single path element is refused rather than resolved, so a reset can never
// reach above the hsqldb directory.
func TestPrepareLocalAppDatabase_FreshFileDBRefusesAPathName(t *testing.T) {
	for _, name := range []string{"", ".", "..", "../app", "a/b"} {
		o := LocalAppOptions{
			ProjectPath: filepath.Join(t.TempDir(), "App.mpr"),
			DB:          DBConfig{Type: RuntimeDatabaseType(DBTypeHSQLDB), Name: name},
			FreshFileDB: true,
		}
		o.applyDefaults()
		o.DB.Name = name // applyDefaults may fill an empty name; test the raw value
		if err := prepareLocalAppDatabase(&o, io.Discard); err == nil {
			t.Errorf("name %q: reset accepted, want a refusal", name)
		}
	}
}
