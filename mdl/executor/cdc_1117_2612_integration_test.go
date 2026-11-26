//go:build integration

// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// cdc1117Script exercises every fact Mendix 11.17 records in a CDC baseline
// (ako/mxcli#2612): a String length, a required attribute, a Boolean and an
// AutoNumber (not nullable), a Decimal (the project decimal scale), a reference
// (OneToMany) and a reference set (ManyToMany).
const cdc1117Script = `mdl 1;
create module Shop;
create or modify persistent entity Shop.Customer ( Name: String(100) );
create or modify persistent entity Shop.Tag ( Label: String(30) );
create or modify persistent entity Shop."Order" (
  Number: String(20),
  Code: String(5) not null error message 'Code is required',
  Total: Decimal,
  Paid: Boolean,
  Seq: AutoNumber default 1
);
create or modify association Shop.Order_Customer from Shop."Order" to Shop.Customer;
create or modify association Shop.Order_Tag from Shop."Order" to Shop.Tag type ReferenceSet;
create or modify change data capture service Shop.OrdersCdc ( ServiceName: 'OrdersCdc' ) {
  entity Shop."Order" as 'Order' ( Number as 'Number', Code as 'Code', Total as 'Total', Paid as 'Paid', Seq as 'Seq',
    Order_Customer as 'Customer', Order_Tag as 'Tags' );
  entity Shop.Customer as 'Customer' ( Name as 'Name' );
  entity Shop.Tag as 'Tag' ( Label as 'Label' );
};`

// mx1117 is the 11.17.0 toolchain: MX_BINARY_1117, or the cached download.
// This test needs that exact line — the facts do not exist below it — so it
// does not fall back to whatever mx findMxBinary would pick.
func mx1117(t *testing.T) string {
	if p := os.Getenv("MX_BINARY_1117"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	p := filepath.Join(home, ".mxcli", "mxbuild", "11.17.0", "modeler", "mx")
	if _, err := os.Stat(p); err != nil {
		t.Skip("needs mx 11.17.0: mxcli setup mxbuild --version 11.17.0, or MX_BINARY_1117")
	}
	return p
}

func runMx1117(t *testing.T, mx, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(mx, args...)
	cmd.Dir = dir
	docker.PrepareMxCommand(cmd)
	out, _ := cmd.CombinedOutput()
	return string(out)
}

// A change data capture service written on an 11.17 project passes mx check —
// no CE1585 "There are changes in your … document" — and re-running the script
// writes nothing (ako/mxcli#2612). Before the fix the baseline lacked the five
// 11.17 fields, and mx check reported CE1585 after every write.
func TestCdcBaselineOn1117PassesMxCheck_2612(t *testing.T) {
	mx := mx1117(t)
	dir := t.TempDir()
	if out := runMx1117(t, mx, dir, "create-project", "--app-name", "App", "--output-dir", dir); !strings.Contains(out, "Done") {
		t.Fatalf("mx create-project:\n%s", out)
	}
	mpr := filepath.Join(dir, "App.mpr")

	run := func() string {
		var buf bytes.Buffer
		e := New(&buf)
		e.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
		if err := e.Execute(&ast.ConnectStmt{Path: mpr}); err != nil {
			t.Fatalf("connect: %v", err)
		}
		prog, errs := visitor.Build(cdc1117Script)
		if len(errs) > 0 {
			t.Fatalf("parse: %v", errs)
		}
		for _, s := range prog.Statements {
			if err := e.Execute(s); err != nil {
				t.Fatalf("exec: %v\n%s", err, buf.String())
			}
		}
		if err := e.Execute(&ast.DisconnectStmt{}); err != nil {
			t.Fatalf("disconnect: %v", err)
		}
		return buf.String()
	}
	run()
	second := run()
	if strings.Contains(second, "Modified change data capture service") {
		t.Errorf("a re-run rewrote the service:\n%s", second)
	}
	out := runMx1117(t, mx, dir, "check", mpr)
	if strings.Contains(out, "CE1585") || !strings.Contains(out, "The app contains: 0 errors") {
		t.Fatalf("mx check after create or modify change data capture service:\n%s", out)
	}
}
