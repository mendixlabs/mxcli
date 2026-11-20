// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#2239: `alter module … add jar dependency if not exists (…)` makes a
// script declaring a module's Java dependency re-runnable. It follows the other
// IF NOT EXISTS forms (ADD ATTRIBUTE, ADD VALUE, CREATE ENTITY): an existing
// coordinate is left untouched with a "skipped" notice — even when the stored
// version differs — and a statement whose every action skipped writes nothing.
func TestAddJarDependencyIfNotExists(t *testing.T) {
	type result struct {
		written *types.ModuleSettings
		out     string
		err     error
	}
	run := func(t *testing.T, stored []*types.JarDependency, actions ...ast.JarDepAction) result {
		t.Helper()
		var written *types.ModuleSettings
		mb := &mock.MockBackend{
			IsConnectedFunc: func() bool { return true },
			GetModuleByNameFunc: func(name string) (*model.Module, error) {
				return &model.Module{BaseElement: model.BaseElement{ID: "mod-1"}, Name: name}, nil
			},
			GetModuleSettingsFunc: func(model.ID) (*types.ModuleSettings, error) {
				return &types.ModuleSettings{ID: "ms-1", ContainerID: "mod-1", JarDependencies: stored}, nil
			},
			UpdateModuleSettingsFunc: func(ms *types.ModuleSettings) error {
				c := *ms
				written = &c
				return nil
			},
		}
		ctx, buf := newMockCtx(t, withBackend(mb))
		err := execAlterModuleJarDep(ctx, &ast.AlterModuleJarDepStmt{ModuleName: "ServiceDesk", Actions: actions})
		return result{written, buf.String(), err}
	}
	duckdb := func(version string, guard bool) *ast.AddJarDepAction {
		return &ast.AddJarDepAction{Group: "org.duckdb", Artifact: "duckdb_jdbc", Version: version, Included: true, IfNotExists: guard}
	}
	stored := func(version string) []*types.JarDependency {
		return []*types.JarDependency{{GroupID: "org.duckdb", ArtifactID: "duckdb_jdbc", Version: version, IsIncluded: true}}
	}

	t.Run("absent: creates it", func(t *testing.T) {
		r := run(t, nil, duckdb("2.0.0.1", true))
		if r.err != nil {
			t.Fatal(r.err)
		}
		if r.written == nil || len(r.written.JarDependencies) != 1 || r.written.JarDependencies[0].Version != "2.0.0.1" {
			t.Fatalf("want the dependency written, got %+v", r.written)
		}
		if !strings.Contains(r.out, "Updated jar dependencies for module 'ServiceDesk'") {
			t.Errorf("a write must say so:\n%s", r.out)
		}
	})
	t.Run("present, same values: writes nothing, reports Unchanged", func(t *testing.T) {
		r := run(t, stored("2.0.0.1"), duckdb("2.0.0.1", true))
		if r.err != nil {
			t.Fatal(r.err)
		}
		if r.written != nil {
			t.Errorf("an all-skipped statement must not write, wrote %+v", r.written)
		}
		if !strings.Contains(r.out, "already exists in module 'ServiceDesk' — skipped") ||
			!strings.Contains(r.out, "Unchanged jar dependencies for module 'ServiceDesk'") {
			t.Errorf("want the skip notice and Unchanged:\n%s", r.out)
		}
		if strings.Contains(r.out, "differs") {
			t.Errorf("identical values must not report a difference:\n%s", r.out)
		}
	})
	t.Run("present, other version: left untouched, the difference named", func(t *testing.T) {
		r := run(t, stored("1.4.0"), duckdb("2.0.0.1", true))
		if r.err != nil {
			t.Fatal(r.err)
		}
		if r.written != nil {
			t.Errorf("if not exists must leave the stored dependency alone, wrote %+v", r.written)
		}
		if !strings.Contains(r.out, "stored version '1.4.0'") || !strings.Contains(r.out, "set jar dependency 'org.duckdb:duckdb_jdbc' version '2.0.0.1'") {
			t.Errorf("want the stored version and the SET that would change it named:\n%s", r.out)
		}
	})
	t.Run("a skip beside a real change still writes the change", func(t *testing.T) {
		r := run(t, stored("2.0.0.1"), duckdb("2.0.0.1", true),
			&ast.AddJarDepAction{Group: "org.example", Artifact: "other", Version: "1.0", Included: true, IfNotExists: true})
		if r.err != nil {
			t.Fatal(r.err)
		}
		if r.written == nil || len(r.written.JarDependencies) != 2 {
			t.Fatalf("want both dependencies written, got %+v", r.written)
		}
	})
	t.Run("control: the bare form still refuses a present coordinate", func(t *testing.T) {
		r := run(t, stored("2.0.0.1"), duckdb("2.0.0.1", false))
		if r.err == nil || !strings.Contains(r.err.Error(), "jar dependency 'org.duckdb:duckdb_jdbc' already exists in module 'ServiceDesk'; use SET to update version") {
			t.Errorf("want the existing refusal, got %v", r.err)
		}
		if r.written != nil {
			t.Errorf("a refused statement must not write")
		}
	})
}

// On real storage: the second run of the issue's script leaves every project
// file byte-identical and says Unchanged (ADR-0008), on the Studio Pro-authored
// PedApp fixture.
func TestAddJarDependencyIfNotExists_RerunWritesNothing(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	script := func(version string) string {
		return `mdl 1;
alter module MyFirstModule
  add jar dependency if not exists (
    group = 'org.duckdb',
    artifact = 'duckdb_jdbc',
    version = '` + version + `',
    included = true,
  );`
	}
	if err := afRun(t, exec, script("2.0.0.1")); err != nil {
		t.Fatalf("run 1: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Updated jar dependencies for module 'MyFirstModule'") {
		t.Fatalf("run 1 is a write and must say so:\n%s", out.String())
	}

	for _, version := range []string{"2.0.0.1", "2.1.0"} {
		before := projectFiles(t, dir)
		out.Reset()
		if err := afRun(t, exec, script(version)); err != nil {
			t.Fatalf("rerun with %s: %v\n%s", version, err, out.String())
		}
		if changed := diffProjectFiles(before, projectFiles(t, dir)); len(changed) != 0 {
			t.Errorf("rerun with %s wrote %v", version, changed)
		}
		if got := out.String(); !strings.Contains(got, "Unchanged jar dependencies") || strings.Contains(got, "Updated jar") {
			t.Errorf("rerun with %s must report Unchanged:\n%s", version, got)
		}
	}

	// Control: the bare form still refuses the second run.
	out.Reset()
	err := afRun(t, exec, strings.Replace(script("2.0.0.1"), " if not exists", "", 1))
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("control: bare add must still refuse, got %v", err)
	}
}

// check -p must agree with exec: the guarded add of a present coordinate is
// accepted by both, the bare one refused by both.
func TestCheckExecAgree_JarDependencyIfNotExists(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	const add = `mdl 1;
alter module MyFirstModule
  add jar dependency ( group = 'org.example', artifact = 'agree-guard', version = '1.0.0', included = true );`
	if err := agreeExec(t, exec, add); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	assertAgree(t, exec, dir, add, "jar dependency 'org.example:agree-guard' already exists")
	assertAgree(t, exec, dir, `mdl 1;
alter module MyFirstModule
  add jar dependency if not exists ( group = 'org.example', artifact = 'agree-guard', version = '1.0.0', included = true );`, "")
}
