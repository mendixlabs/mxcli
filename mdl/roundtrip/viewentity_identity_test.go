// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"bytes"
	"encoding/hex"
	"path/filepath"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/internal/testutil"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
)

// viewsFixtureDir holds MyFirstModule.VCar, a view entity authored in Studio
// Pro 11.14.0, so its GUIDs differ from its $IDs (testdata/testapp-views/README.md).
// PedApp has no view entity, and on one mxcli created GUID == $ID, so a
// re-minted GUID reproduces the same value and the test could not fail.
const (
	viewsFixtureDir = "../../testdata/testapp-views"
	viewsFixtureMPR = "TestApp.mpr"
)

const vcarBody = `view entity MyFirstModule.VCar (
  ModelID: Long,
  Brand: String(200)
) as (
  from MyFirstModule.Car as c
  select c.ModelId as ModelID
  ,      c.Brand as Brand
);
`

// R1 (#731): `create or replace` means `create or modify` on every document
// type, and for a view entity that is the identity-carrying rewrite rather than
// a delete and recreate. Under mdl 0 the old meaning is kept, which is the
// control: it re-mints the GUID, so the assertion can detect a lost one.
func TestViewEntityCreateOrModifyKeepsGUID(t *testing.T) {
	cases := []struct {
		name     string
		script   string
		keepGUID bool
	}{
		{"create or modify", "create or modify " + vcarBody, true},
		{"mdl 1 create or replace", "mdl 1;\ncreate or replace " + vcarBody, true},
		// Control: mdl 0 keeps delete-and-recreate, so the GUID changes.
		{"mdl 0 create or replace (control)", "create or replace " + vcarBody, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mpr := copyViewsFixture(t)
			before := vcarGUIDs(t, mpr)
			for name, id := range before {
				if id[0] == id[1] {
					t.Fatalf("fixture %s has GUID == $ID; the test cannot detect a re-minted GUID", name)
				}
			}

			runScript(t, mpr, c.script)

			after := vcarGUIDs(t, mpr)
			for name, want := range before {
				got, ok := after[name]
				kept := ok && got[1] == want[1]
				if c.keepGUID && !kept {
					t.Errorf("%s: GUID %s -> %s; the rewrite must keep the stored GUID", name, want[1], got[1])
				}
				if !c.keepGUID && kept {
					t.Errorf("%s: GUID kept under the control; the test cannot tell a carried GUID from a lost one", name)
				}
			}
		})
	}
}

// vcarGUIDs maps VCar and each of its attributes to [$ID, GUID] as hex.
func vcarGUIDs(t *testing.T, mprPath string) map[string][2]string {
	t.Helper()
	r, err := mmpr.Open(mprPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer r.Close()
	units, err := r.ListUnits()
	if err != nil {
		t.Fatalf("list units: %v", err)
	}
	for _, u := range units {
		b, err := r.GetRawUnitBytes(u.ID)
		if err != nil {
			t.Fatalf("read unit: %v", err)
		}
		if typ, _ := typeAndName(b); typ != "DomainModels$DomainModel" {
			continue
		}
		var dm bson.D
		if err := bson.Unmarshal(b, &dm); err != nil {
			t.Fatalf("decode: %v", err)
		}
		ents, _ := field(dm, "Entities").(bson.A)
		for _, e := range ents {
			ent, ok := e.(bson.D)
			if !ok || field(ent, "Name") != "VCar" {
				continue
			}
			out := map[string][2]string{"entity VCar": ids(ent)}
			attrs, _ := field(ent, "Attributes").(bson.A)
			for _, a := range attrs {
				if attr, ok := a.(bson.D); ok {
					name, _ := field(attr, "Name").(string)
					out["attribute "+name] = ids(attr)
				}
			}
			return out
		}
	}
	t.Fatal("MyFirstModule.VCar not found")
	return nil
}

func field(d bson.D, key string) any {
	for _, e := range d {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

func ids(el bson.D) [2]string {
	h := func(v any) string {
		if b, ok := v.(bson.Binary); ok {
			return hex.EncodeToString(b.Data)
		}
		return ""
	}
	return [2]string{h(field(el, "$ID")), h(field(el, "GUID"))}
}

func copyViewsFixture(t *testing.T) string {
	t.Helper()
	src, err := filepath.Abs(viewsFixtureDir)
	if err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	if err := testutil.CopyFixture(dst, src); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	return filepath.Join(dst, viewsFixtureMPR)
}

// runScript runs a script the way `mxcli exec` does after its preflight.
func runScript(t *testing.T, mprPath, script string) {
	t.Helper()
	var out bytes.Buffer
	exe := executor.New(&out)
	exe.SetQuiet(true)
	exe.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	if err := exe.Execute(&ast.ConnectStmt{Path: mprPath}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	if err := exe.ExecuteProgram(prog); err != nil {
		t.Fatalf("exec: %v\n%s", err, out.String())
	}
	_ = exe.Execute(&ast.DisconnectStmt{})
	_ = exe.Close()
}
