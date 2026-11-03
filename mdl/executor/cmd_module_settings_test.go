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

// Studio Pro's stored settings, measured on ako/TestApp (11.16): an app module
// as every untouched one is, and the probe set to Add-on 2.3.1. Every module
// stores ProtectedModuleType AddOn, so it is in the app module too.
func appModuleSettings() *types.ModuleSettings {
	return &types.ModuleSettings{ID: "ms-app", ContainerID: "mod-1", ExportLevel: "Source", ProtectedModuleType: "AddOn", Version: "1.0.0"}
}

func addOnModuleSettings() *types.ModuleSettings {
	return &types.ModuleSettings{ID: "ms-addon", ContainerID: "mod-1", ExportLevel: "Protected", ProtectedModuleType: "AddOn", Version: "2.3.1"}
}

// ako/mxcli#1921: the module type is ExportLevel, not ProtectedModuleType.
// Reading the latter would make every app module an add-on.
func TestModuleTypeOf(t *testing.T) {
	for _, c := range []struct {
		ms   *types.ModuleSettings
		want string
	}{
		{appModuleSettings(), "app"},
		{addOnModuleSettings(), "addon"},
		{&types.ModuleSettings{ExportLevel: "Protected", ProtectedModuleType: "Solution"}, ""},
	} {
		if got := moduleTypeOf(c.ms); got != c.want {
			t.Errorf("%+v: type %q, want %q", c.ms, got, c.want)
		}
	}
}

// describe prints only what differs from a new module (App, 1.0.0), and what it
// prints parses back to the same settings.
func TestModuleSettingsMDL(t *testing.T) {
	ctx, _ := newMockCtx(t)
	if got := moduleSettingsMDL(ctx, appModuleSettings()); got != "" {
		t.Errorf("an untouched app module prints settings: %q", got)
	}
	out := "create or modify module Probe" + moduleSettingsMDL(ctx, addOnModuleSettings()) + ";"
	prog := parseMDL(t, out)
	s, ok := prog.Statements[0].(*ast.CreateModuleStmt)
	if !ok || s.Settings.Version == nil || *s.Settings.Version != "2.3.1" ||
		s.Settings.ModuleType == nil || *s.Settings.ModuleType != "addon" {
		t.Errorf("describe output does not read back as the stored settings:\n%s\n%+v", out, prog.Statements[0])
	}
	// A solution module has no MDL spelling: no ModuleType, so a rerun keeps it.
	sol := &types.ModuleSettings{ExportLevel: "Protected", ProtectedModuleType: "Solution", Version: "1.0.0"}
	if got := moduleSettingsMDL(ctx, sol); got != "" {
		t.Errorf("a solution module prints %q", got)
	}
}

// alter module … set writes exactly what it names, onto the stored settings;
// a statement that restates them writes nothing.
func TestAlterModuleSet(t *testing.T) {
	str := func(s string) *string { return &s }
	run := func(t *testing.T, stored *types.ModuleSettings, c ast.ModuleSettingsClause) (*types.ModuleSettings, string) {
		t.Helper()
		var written *types.ModuleSettings
		mb := &mock.MockBackend{
			IsConnectedFunc: func() bool { return true },
			GetModuleByNameFunc: func(name string) (*model.Module, error) {
				return &model.Module{BaseElement: model.BaseElement{ID: "mod-1"}, Name: name}, nil
			},
			GetModuleSettingsFunc: func(model.ID) (*types.ModuleSettings, error) { return stored, nil },
			UpdateModuleSettingsFunc: func(ms *types.ModuleSettings) error {
				c := *ms
				written = &c
				return nil
			},
		}
		ctx, buf := newMockCtx(t, withBackend(mb))
		if err := execAlterModuleSet(ctx, &ast.AlterModuleSetStmt{ModuleName: "Probe", Settings: c}); err != nil {
			t.Fatal(err)
		}
		return written, buf.String()
	}

	t.Run("app to add-on", func(t *testing.T) {
		w, _ := run(t, appModuleSettings(), ast.ModuleSettingsClause{ModuleType: str("addon")})
		if w == nil || w.ExportLevel != "Protected" || w.ProtectedModuleType != "AddOn" || w.Version != "1.0.0" {
			t.Errorf("written %+v, want Protected/AddOn with the version kept", w)
		}
	})
	t.Run("add-on to app", func(t *testing.T) {
		w, _ := run(t, addOnModuleSettings(), ast.ModuleSettingsClause{ModuleType: str("app")})
		if w == nil || w.ExportLevel != "Source" || w.Version != "2.3.1" {
			t.Errorf("written %+v, want Source with the version kept", w)
		}
	})
	t.Run("version only", func(t *testing.T) {
		w, _ := run(t, addOnModuleSettings(), ast.ModuleSettingsClause{Version: str("2.4.0")})
		if w == nil || w.Version != "2.4.0" || w.ExportLevel != "Protected" {
			t.Errorf("written %+v, want 2.4.0 with the type kept", w)
		}
	})
	t.Run("control: restated settings write nothing", func(t *testing.T) {
		w, out := run(t, addOnModuleSettings(), ast.ModuleSettingsClause{Version: str("2.3.1"), ModuleType: str("addon")})
		if w != nil || !strings.Contains(out, "unchanged") {
			t.Errorf("restating the stored settings wrote %+v (output %q)", w, out)
		}
	})
}
