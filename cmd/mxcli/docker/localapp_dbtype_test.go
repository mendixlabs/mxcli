// SPDX-License-Identifier: Apache-2.0

// LocalAppOptions.applyDefaults filled in PostgreSQL's host, user, password and
// database name whenever they were empty, with no look at which database was
// asked for. So a file database came out of it carrying a host it has no use for,
// and StartLocalApp — which pings the host whenever EnsureDB is false — then
// dialled 127.0.0.1:5432 for an app that was going to open a file.
//
// The `run --local` path never showed this because LocalRunOptions carries its own
// applyDatabaseDefaults, which does guard on IsFileBased and clears those fields.
// The guard existed; it was simply on one of the two option structs, so the path
// that does NOT build a LocalRunOptions had none.
package docker

import (
	"io"
	"strings"
	"testing"
)

func TestLocalAppOptionsApplyDefaults_FileDatabaseKeepsNoConnectionSettings(t *testing.T) {
	var o LocalAppOptions
	o.DB.Type = "HSQLDB"
	o.applyDefaults()

	if o.DB.Host != "" {
		t.Errorf("DB.Host = %q; the file database has no host to reach, and StartLocalApp "+
			"pings it whenever EnsureDB is false — so this value turns a working boot into "+
			"\"database not reachable at 127.0.0.1:5432\"", o.DB.Host)
	}
	if o.DB.User != "" || o.DB.Password != "" {
		t.Errorf("DB.User = %q, DB.Password = %q; the file database has no credentials",
			o.DB.User, o.DB.Password)
	}
	// The NAME is still required, which an end-to-end boot is what proved: with it
	// cleared the runtime refused to start with "DatabaseJdbcUrl or DatabaseName has
	// no value". The file database has no server to name, but it still has a file,
	// and that file's name is what the runtime opens. LocalRunOptions
	// .applyDatabaseDefaults clears the three connection fields and leaves this one
	// for the same reason.
	if o.DB.Name == "" {
		t.Error("DB.Name is empty; the runtime needs a name even for the file database")
	}
}

// The control: an ordinary run still gets the PostgreSQL defaults this function has
// always applied. Guarding on the database type must not change the default path.
func TestLocalAppOptionsApplyDefaults_DefaultsToPostgres(t *testing.T) {
	var o LocalAppOptions
	o.applyDefaults()

	if o.DB.Type != "PostgreSQL" {
		t.Errorf("DB.Type = %q, want PostgreSQL", o.DB.Type)
	}
	if o.DB.Host != "127.0.0.1:5432" {
		t.Errorf("DB.Host = %q, want 127.0.0.1:5432", o.DB.Host)
	}
	if o.DB.User != "mendix" || o.DB.Password != "mendix" {
		t.Errorf("DB.User = %q, DB.Password = %q, want mendix/mendix", o.DB.User, o.DB.Password)
	}
	if o.DB.Name == "" {
		t.Error("DB.Name is empty; the derived name is what the default path has always used")
	}
}

// StartLocalApp gated the boot on `if EnsureDB { provision } else { ping }`, which
// is the wrong pair of branches for a file database: there is nothing to provision
// AND nothing to ping, so it fell into the ping with an empty host and failed. The
// error named a host and a database the user never asked for.
func TestCheckDatabase_FileDatabaseIsNeitherProvisionedNorPinged(t *testing.T) {
	opts := LocalAppOptions{ProjectPath: "/tmp/app/App.mpr"}
	opts.DB.Type = "HSQLDB"
	opts.applyDefaults()

	if err := checkDatabase(&opts, io.Discard); err != nil {
		t.Errorf("checkDatabase on a file database returned %v; there is no server to reach, "+
			"so this can only ever fail", err)
	}
}

// The control for the branch above: a PostgreSQL run with nothing listening must
// still be stopped, and the message must still name the host and the database —
// that message is the only thing telling a user which server was unreachable.
func TestCheckDatabase_UnreachablePostgresIsStillReported(t *testing.T) {
	opts := LocalAppOptions{ProjectPath: "/tmp/app/App.mpr"}
	opts.applyDefaults()
	// A port nothing listens on, so the check does not depend on what else the
	// machine happens to be running.
	opts.DB.Host = "127.0.0.1:1"

	err := checkDatabase(&opts, io.Discard)
	if err == nil {
		t.Fatal("checkDatabase returned nil for an unreachable PostgreSQL")
	}
	if !strings.Contains(err.Error(), "127.0.0.1:1") {
		t.Errorf("error does not name the unreachable host: %v", err)
	}
}
