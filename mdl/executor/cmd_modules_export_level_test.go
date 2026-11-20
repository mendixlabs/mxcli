// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1498: nothing mxcli printed said a module was Protected, nor
// what an entity's export level was, so the reporter found both by a raw read.
// A module's is `ModuleType: addon` since ako/mxcli#1921, with a comment on
// what it means; the output still re-runs. Only a Protected
// module's output changes; Open is the control.
func TestDescribe_ProtectedModuleExportLevels(t *testing.T) {
	ctx := protectedModuleFixture(t, nil)
	buf := ctx.Output.(*bytes.Buffer)

	describe := func(f func() error) string {
		t.Helper()
		buf.Reset()
		if err := f(); err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		parseMDL(t, out) // still re-runnable
		return out
	}

	out := describe(func() error { return describeModule(ctx, "MyAddOn", false) })
	// An add-on is a setting since ako/mxcli#1921, with the comment saying what
	// it means for references.
	if !strings.Contains(out, "ModuleType: addon") || !strings.Contains(out, "-- add-on module: elements not marked Usable") {
		t.Errorf("describe module of a protected module does not say so:\n%s", out)
	}
	out = describe(func() error { return describeModule(ctx, "Open", false) })
	if strings.Contains(out, "export level") {
		t.Errorf("describe module of an ordinary module mentions an export level:\n%s", out)
	}

	// An entity's level is a clause since MDL can state it (the #1498
	// follow-up): `export level api` when Usable, nothing for the default.
	for _, c := range []struct {
		entity string
		want   bool
	}{
		{"Request", false},
		{"Pub", true},
	} {
		out = describe(func() error { return describeEntity(ctx, ast.QualifiedName{Module: "MyAddOn", Name: c.entity}) })
		if got := strings.Contains(out, "export level api"); got != c.want {
			t.Errorf("describe entity MyAddOn.%s: `export level api` printed = %v, want %v:\n%s", c.entity, got, c.want, out)
		}
		if strings.Contains(out, "set in Studio Pro") {
			t.Errorf("describe entity MyAddOn.%s still says the level is set in Studio Pro:\n%s", c.entity, out)
		}
	}
	out = describe(func() error { return describeEntity(ctx, ast.QualifiedName{Module: "Open", Name: "E"}) })
	if strings.Contains(out, "export level") {
		t.Errorf("describe of a Hidden entity mentions an export level:\n%s", out)
	}
}

func TestShowModules_MarksProtectedModule(t *testing.T) {
	ctx := protectedModuleFixture(t, nil)
	buf := ctx.Output.(*bytes.Buffer)
	if err := listModules(ctx); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(buf.String(), "\n") {
		switch {
		case strings.Contains(line, "| MyAddOn "):
			if !strings.Contains(line, "Protected AddOn") {
				t.Errorf("MyAddOn row does not say Protected: %s", line)
			}
		case strings.Contains(line, "| Open "):
			if strings.Contains(line, "Protected") {
				t.Errorf("Open row says Protected: %s", line)
			}
		}
	}
}
