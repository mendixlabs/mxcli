// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// `run clear-sessions` connects to the database the recorded run uses, so it
// reads the --db-* flags back from the run's stored arguments, with the run's
// own defaults (ako/mxcli#2129).
func TestRunDatabaseFromArgs(t *testing.T) {
	args := []string{"run", "--local", "--db-host", "127.0.0.1:55432", "--db-name=sessapp",
		"--db-user", "dev", "--db-password", "@env:MXCLI_TEST_DBPW", "-p", "/w/App.mpr"}
	t.Setenv("MXCLI_TEST_DBPW", "s3cret")
	db, err := runDatabaseFromArgs("/w/App.mpr", args, "")
	if err != nil {
		t.Fatal(err)
	}
	if db.Host != "127.0.0.1:55432" || db.Name != "sessapp" || db.User != "dev" || db.Password != "s3cret" {
		t.Errorf("db = %+v", db)
	}
}

func TestRunDatabaseFromArgs_Defaults(t *testing.T) {
	db, err := runDatabaseFromArgs("/w/App.mpr", []string{"run", "--local", "-p", "/w/App.mpr"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if db.Host != "127.0.0.1:5432" || db.User != "mendix" || db.Password != "mendix" || db.Name == "" {
		t.Errorf("defaults not applied: %+v", db)
	}
}

// A password given literally is not stored, so the command needs it again
// rather than connecting with the placeholder.
func TestRunDatabaseFromArgs_RedactedPasswordNeedsTheFlag(t *testing.T) {
	args := []string{"run", "--local", "--db-password", redactedValue}
	if _, err := runDatabaseFromArgs("/w/App.mpr", args, ""); err == nil || !strings.Contains(err.Error(), "--db-password") {
		t.Fatalf("err = %v, want one asking for --db-password", err)
	}
	db, err := runDatabaseFromArgs("/w/App.mpr", args, "given")
	if err != nil || db.Password != "given" {
		t.Fatalf("override: db=%+v err=%v", db, err)
	}
}

func TestRunDatabaseFromArgs_FileDatabase(t *testing.T) {
	db, err := runDatabaseFromArgs("/w/App.mpr", []string{"run", "--local", "--db-type", "hsqldb"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !db.IsFileBased() {
		t.Errorf("db = %+v, want the file database", db)
	}
}
