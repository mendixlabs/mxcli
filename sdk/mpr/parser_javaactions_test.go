// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/javaactions"
)

func TestBuildSystemJavaActions_VerifyPassword(t *testing.T) {
	actions := BuildSystemJavaActions()

	var found bool
	for _, a := range actions {
		if a.Name == "VerifyPassword" && string(a.ContainerID) == SystemModuleID {
			found = true
			break
		}
	}
	if !found {
		t.Error("BuildSystemJavaActions: System.VerifyPassword not present")
	}
}

func TestBuildSystemJavaActionsFull_VerifyPassword(t *testing.T) {
	actions := BuildSystemJavaActionsFull()

	var found bool
	for _, a := range actions {
		if a.Name != "VerifyPassword" || string(a.ContainerID) != SystemModuleID {
			continue
		}
		found = true
		if len(a.Parameters) != 2 {
			t.Errorf("VerifyPassword: want 2 parameters, got %d", len(a.Parameters))
		}
		if _, ok := a.ReturnType.(*javaactions.BooleanType); !ok {
			t.Errorf("VerifyPassword: want BooleanType return, got %T", a.ReturnType)
		}
	}
	if !found {
		t.Error("BuildSystemJavaActionsFull: System.VerifyPassword not present")
	}
}

func TestBuildSystemJavaActions_DeterministicIDs(t *testing.T) {
	a1 := BuildSystemJavaActions()
	a2 := BuildSystemJavaActions()
	for i := range a1 {
		if a1[i].ID != a2[i].ID {
			t.Errorf("non-deterministic ID for %s", a1[i].Name)
		}
	}
}

func TestParseCodeActionParameterType_JavaActionMicroflowParameter(t *testing.T) {
	value := parseCodeActionParameterType(map[string]any{
		"$ID":   "type-1",
		"$Type": "JavaActions$MicroflowJavaActionParameterType",
	})

	if _, ok := value.(*javaactions.MicroflowType); !ok {
		t.Fatalf("value = %T, want *MicroflowType", value)
	}
}
