// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#2121: a long-lived session read the project once and went on
// answering from that state, so a write by another process never showed up:
// `show microflows` kept listing the old set, `describe microflow` of a flow
// created elsewhere said "microflow not found", and on MPR v2 the session's own
// `create microflow` of that name wrote a SECOND document with it, because the
// stale unit index said the name was free. On v1 the executor's module cache
// refused `create entity` in a module made elsewhere ("module not found").
//
// The "other process" is a second executor with its own backend: its own
// SQLite connection to the .mpr, which is what a second mxcli, Studio Pro or a
// git checkout is to the session.
func TestSessionSeesAnotherProcessWrite_Issue2121(t *testing.T) {
	for _, f := range []pkgHostFixture{v1Fixture, pedAppFixture} {
		t.Run(f.name, func(t *testing.T) {
			path := copyHostProject(t, f)
			session, sessionOut := connectHost(t, path)
			run := func(src string) (string, error) {
				t.Helper()
				sessionOut.Reset()
				err := runPkgMDL(t, session, "mdl 1;\n"+src)
				return sessionOut.String(), err
			}

			// The session reads first, so whatever it caches is warm.
			if _, err := run("create module Lab;"); err != nil {
				t.Fatalf("create module: %v", err)
			}
			if out, err := run("show modules; show microflows in Lab; show entities in Lab;"); err != nil {
				t.Fatalf("warm-up reads: %v\n%s", err, out)
			}

			other, otherOut := connectHost(t, path)
			if err := runPkgMDL(t, other, `mdl 1;
create module ExtMod;
create microflow Lab.StaleProbe ()
begin
  log info node 'probe' 'one';
end;
`); err != nil {
				t.Fatalf("other process write: %v\n%s", err, otherOut.String())
			}

			out, err := run("show microflows in Lab;")
			if err != nil || !strings.Contains(out, "Lab.StaleProbe") {
				t.Errorf("show microflows after another process created Lab.StaleProbe: err=%v, output lacks it:\n%s", err, out)
			}
			out, err = run("describe microflow Lab.StaleProbe;")
			if err != nil || !strings.Contains(out, "'one'") {
				t.Errorf("describe microflow created elsewhere: err=%v\n%s", err, out)
			}

			if err := runPkgMDL(t, other, `mdl 1;
create or modify microflow Lab.StaleProbe ()
begin
  log info node 'probe' 'two';
end;
`); err != nil {
				t.Fatalf("other process modify: %v\n%s", err, otherOut.String())
			}
			out, err = run("describe microflow Lab.StaleProbe;")
			if err != nil || !strings.Contains(out, "'two'") {
				t.Errorf("describe after another process modified it shows the old content: err=%v\n%s", err, out)
			}

			// A session write must not act on the stale state either.
			out, err = run(`create microflow Lab.StaleProbe () begin log info node 'probe' 'session'; end;`)
			if err == nil {
				t.Errorf("create microflow of a name another process took succeeded — the session wrote a duplicate document:\n%s", out)
			}
			if out, err := run("create persistent entity ExtMod.E1 (A: Integer);"); err != nil {
				t.Errorf("create entity in a module another process created: %v\n%s", err, out)
			}

			_ = other.Execute(&ast.DisconnectStmt{})
			_ = session.Execute(&ast.DisconnectStmt{})
			check, checkOut := connectHost(t, path)
			if err := runPkgMDL(t, check, "show microflows in Lab;"); err != nil {
				t.Fatal(err)
			}
			if n := strings.Count(checkOut.String(), "| Lab.StaleProbe "); n != 1 {
				t.Errorf("the project holds %d microflows named Lab.StaleProbe, want 1:\n%s", n, checkOut.String())
			}
		})
	}
}

// A git checkout replaces the .mpr with a new file rather than writing into
// it. The session's SQLite connection still holds the old file open and would
// read it forever; it has to notice the replacement and reopen. v1 only: on v2
// the documents are separate files, and the replacement of the .mpr alone is
// the case that matters for the connection.
func TestSessionSeesReplacedProjectFile_Issue2121(t *testing.T) {
	path := copyHostProject(t, v1Fixture)
	session, sessionOut := connectHost(t, path)
	if err := runPkgMDL(t, session, "mdl 1;\nshow modules;"); err != nil {
		t.Fatal(err)
	}

	// Another checkout of the project gets a module, then is moved over the
	// session's file, as git does: write a new file, rename it into place.
	elsewhere := filepath.Join(t.TempDir(), "App.mpr")
	if err := copyPedAppFile(path, elsewhere); err != nil {
		t.Fatal(err)
	}
	other, otherOut := connectHost(t, elsewhere)
	if err := runPkgMDL(t, other, "mdl 1;\ncreate module Checkout;"); err != nil {
		t.Fatalf("%v\n%s", err, otherOut.String())
	}
	_ = other.Execute(&ast.DisconnectStmt{})
	staged := path + ".new"
	if err := copyPedAppFile(elsewhere, staged); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(staged, path); err != nil {
		t.Fatal(err)
	}

	sessionOut.Reset()
	if err := runPkgMDL(t, session, "mdl 1;\nshow modules;"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sessionOut.String(), "Checkout") {
		t.Errorf("show modules after the .mpr was replaced still reads the old file:\n%s", sessionOut.String())
	}
}

// The session's own writes must not be taken for another process's: that
// would throw the caches away after every statement and undo #2094.
func TestOwnWriteIsNotAnExternalChange_Issue2121(t *testing.T) {
	for _, f := range []pkgHostFixture{v1Fixture, pedAppFixture} {
		t.Run(f.name, func(t *testing.T) {
			path := copyHostProject(t, f)
			session, out := connectHost(t, path)
			if err := runPkgMDL(t, session, "mdl 1;\ncreate module Own;\ncreate persistent entity Own.E (A: Integer);\nshow entities in Own;"); err != nil {
				t.Fatalf("%v\n%s", err, out.String())
			}
			syncer, ok := session.backend.(interface{ SyncWithDisk() (bool, error) })
			if !ok {
				t.Fatalf("the modelsdk backend has no SyncWithDisk")
			}
			if changed, err := syncer.SyncWithDisk(); err != nil || changed {
				t.Errorf("after the session's own writes SyncWithDisk = %v, %v; want false", changed, err)
			}
		})
	}
}
