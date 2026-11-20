// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// mendixlabs/mxcli#1733 (ako/mxcli#1935): `check --references` passed a message
// definition member naming an attribute the entity no longer has, and exec then
// refused it — `MyFirstModule.Booking has no attribute "Reference"; available:
// Code, CustomerReference`. check is meant to be the stronger gate.
func messageMemberCtx(t *testing.T) *ExecContext {
	t.Helper()
	mod := mkModule("MyFirstModule")
	booking := mkEntity(mod.ID, "Booking")
	booking.Attributes = []*domainmodel.Attribute{{Name: "CustomerReference"}, {Name: "Code"}}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{mkDomainModel(mod.ID, booking)}, nil
		},
		GetModuleByNameFunc: func(string) (*model.Module, error) { return mod, nil },
		GetDomainModelFunc: func(model.ID) (*domainmodel.DomainModel, error) {
			return mkDomainModel(mod.ID, booking), nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	return ctx
}

func checkMessageMembers(t *testing.T, src string) string {
	t.Helper()
	ctx := messageMemberCtx(t)
	prog := parseMDL(t, src)
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	var out []string
	for _, stmt := range prog.Statements {
		if err := validateWithContext(ctx, stmt, sc); err != nil {
			out = append(out, err.Error())
		}
	}
	return strings.Join(out, "\n")
}

func TestMessageDefinitionUnknownAttributeIsReportedByCheck(t *testing.T) {
	got := checkMessageMembers(t, `create or modify message definition collection MyFirstModule.MD_Booking
{
  definition BookingMessage for MyFirstModule.Booking {
    Reference,
    Code
  }
};`)
	for _, want := range []string{`"Reference"`, "MyFirstModule.Booking", "CustomerReference"} {
		if !strings.Contains(got, want) {
			t.Errorf("check should refuse the unknown member and name %s; got:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"Code"`) {
		t.Errorf("Code is an attribute and must not be reported:\n%s", got)
	}
}

func TestMessageDefinitionKnownAttributesPassCheck(t *testing.T) {
	for name, src := range map[string]string{
		"renamed attribute with stored exposed name": `create or modify message definition collection MyFirstModule.MD_Booking
{
  definition BookingMessage for MyFirstModule.Booking {
    CustomerReference as 'Reference',
    Code
  }
};`,
		// the script adds the attribute itself: exec runs the alter first
		"attribute added earlier in the script": `alter entity MyFirstModule.Booking add attribute Extra: String(10);
create message definition collection MyFirstModule.MD_B2
{
  definition B for MyFirstModule.Booking {
    Extra
  }
};`,
		"11.15 document form": `create message definition MyFirstModule.MD_Doc for MyFirstModule.Booking {
    Code
};`,
	} {
		if got := checkMessageMembers(t, src); got != "" {
			t.Errorf("%s: check refused a valid member:\n%s", name, got)
		}
	}
}
