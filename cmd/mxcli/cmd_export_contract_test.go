// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#2384 part E. `export contract` is what lets two separate apps
// exchange business events without Studio Pro: the producing app mints the
// channel id, and it reaches the consumer inside the exported contract.

type fakeModuleLister struct {
	modules []*model.Module
	err     error
}

func (f fakeModuleLister) ListModules() ([]*model.Module, error) { return f.modules, f.err }

func TestBusinessEventQualifiedName(t *testing.T) {
	modA := &model.Module{BaseElement: model.BaseElement{ID: "mod-a"}, Name: "CdcPublish"}
	modB := &model.Module{BaseElement: model.BaseElement{ID: "mod-b"}, Name: "Other"}
	svc := &model.BusinessEventService{ContainerID: "mod-a", Name: "OrdersEventService"}

	if got := businessEventQualifiedName(fakeModuleLister{modules: []*model.Module{modB, modA}}, svc); got != "CdcPublish.OrdersEventService" {
		t.Errorf("qualified name = %q, want CdcPublish.OrdersEventService", got)
	}

	// A service whose module cannot be resolved is still worth naming: the
	// lookup failing is no reason to drop it from the "known services" list the
	// not-found error prints.
	orphan := &model.BusinessEventService{ContainerID: "mod-missing", Name: "Stray"}
	if got := businessEventQualifiedName(fakeModuleLister{modules: []*model.Module{modA}}, orphan); got != "Stray" {
		t.Errorf("unresolvable module: got %q, want the bare name Stray", got)
	}
	if got := businessEventQualifiedName(fakeModuleLister{err: errors.New("boom")}, svc); got != "OrdersEventService" {
		t.Errorf("module listing failed: got %q, want the bare name", got)
	}
}

// The command takes a qualified name. A bare one is the likely slip — `export
// contract OrdersEventService` — and it has to say what the right shape is
// rather than report "not found", which would send the reader looking for a
// missing service instead of a missing module prefix.
func TestExportContractCmd_RejectsUnqualifiedName(t *testing.T) {
	cmd := exportContractCmd
	if err := cmd.Flags().Set("output", ""); err != nil {
		t.Fatal(err)
	}
	// -p is a persistent flag of the root command, absent on a bare invocation.
	err := cmd.RunE(cmd, []string{"OrdersEventService"})
	if err == nil {
		t.Fatal("accepted a command with no project")
	}
	if !strings.Contains(err.Error(), "project") {
		t.Errorf("error does not name the missing project flag: %v", err)
	}
}
