// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#2477: an enum-typed OData property was dropped at import. It now
// becomes an attribute of an enumeration generated from the EnumType — the
// shape mx check 11.12.6 builds at 0 errors (a plain enumeration is CE4597).

func enumImportCtx2477(t *testing.T, stored ...*model.Enumeration) (*ExecContext, *[]*model.Enumeration) {
	t.Helper()
	mod := mkModule("Shop")
	h := mkHierarchy(mod)
	for _, e := range stored {
		e.ContainerID = mod.ID
	}
	var created []*model.Enumeration
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) {
			return append(append([]*model.Enumeration{}, stored...), created...), nil
		},
		CreateEnumerationFunc: func(e *model.Enumeration) error {
			created = append(created, e)
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, &created
}

var orderStatus2477 = &types.EdmEnumType{Name: "OrderStatus", Members: []*types.EdmEnumMember{{Name: "Open", Value: "0"}, {Name: "Shipped", Value: "1"}}}

func TestImportCreatesRemoteEnumeration_2477(t *testing.T) {
	ctx, created := enumImportCtx2477(t)
	o := &odataEnumerations{byRemote: map[string]string{}}
	qn, why := o.ensure(ctx, "mod1", "Shop", "Shop.Sales", "Vendor.Sales.OrderStatus", orderStatus2477)
	if why != "" || qn != "Shop.OrderStatus" {
		t.Fatalf("ensure = %q, %q", qn, why)
	}
	if len(*created) != 1 {
		t.Fatalf("created %d enumerations, want 1", len(*created))
	}
	e := (*created)[0]
	if e.RemoteService != "Shop.Sales" || e.RemoteName != "Vendor.Sales.OrderStatus" ||
		len(e.Values) != 2 || e.Values[0].Name != "Open" || e.Values[0].RemoteName != "Open" || e.Values[1].RemoteName != "Shipped" {
		t.Errorf("enumeration = %+v", e)
	}
	// A second property of the same type reuses it.
	if qn2, _ := o.ensure(ctx, "mod1", "Shop", "Shop.Sales", "Vendor.Sales.OrderStatus", orderStatus2477); qn2 != qn || len(*created) != 1 {
		t.Errorf("second property: %q, %d created", qn2, len(*created))
	}
}

func TestImportReusesOrRefusesStoredEnumeration_2477(t *testing.T) {
	// One generated from the same type, by an earlier import or Studio Pro: reused.
	ctx, created := enumImportCtx2477(t, &model.Enumeration{Name: "OrderStatus", RemoteService: "Shop.Sales", RemoteName: "Vendor.Sales.OrderStatus"})
	if qn, why := (&odataEnumerations{byRemote: map[string]string{}}).ensure(ctx, "mod1", "Shop", "Shop.Sales", "Vendor.Sales.OrderStatus", orderStatus2477); qn != "Shop.OrderStatus" || why != "" || len(*created) != 0 {
		t.Errorf("reuse: %q %q, %d created", qn, why, len(*created))
	}
	// An ordinary enumeration of that name: the property is not imported, and
	// the message says why (it would be CE4597).
	ctx, created = enumImportCtx2477(t, &model.Enumeration{Name: "OrderStatus"})
	if _, why := (&odataEnumerations{byRemote: map[string]string{}}).ensure(ctx, "mod1", "Shop", "Shop.Sales", "Vendor.Sales.OrderStatus", orderStatus2477); !strings.Contains(why, "not generated from") || len(*created) != 0 {
		t.Errorf("plain enumeration: why %q, %d created", why, len(*created))
	}
	// A member that is no value name.
	ctx, _ = enumImportCtx2477(t)
	bad := &types.EdmEnumType{Name: "Kind", Members: []*types.EdmEnumMember{{Name: "In-Progress"}}}
	if _, why := (&odataEnumerations{byRemote: map[string]string{}}).ensure(ctx, "mod1", "Shop", "Shop.Sales", "Vendor.Kind", bad); !strings.Contains(why, "In-Progress") {
		t.Errorf("bad member: why %q", why)
	}
}

// `create or modify enumeration` over a generated enumeration keeps its source
// and each kept value's member; without it, mx check 11.12.6 reported CE4597 on
// the external entity's attribute after the rewrite.
func TestCreateOrModifyKeepsRemoteSource_2477(t *testing.T) {
	stored := &model.Enumeration{RemoteService: "Shop.Sales", RemoteName: "Vendor.Sales.OrderStatus",
		Values: []model.EnumerationValue{{Name: "Open", RemoteName: "Open"}, {Name: "Shipped", RemoteName: "Shipped"}}}
	rebuilt := &model.Enumeration{Values: []model.EnumerationValue{{Name: "Open"}, {Name: "Shipped"}, {Name: "Added"}}}
	carryRemoteEnumeration(rebuilt, stored)
	if rebuilt.RemoteService != "Shop.Sales" || rebuilt.RemoteName != "Vendor.Sales.OrderStatus" ||
		rebuilt.Values[0].RemoteName != "Open" || rebuilt.Values[1].RemoteName != "Shipped" || rebuilt.Values[2].RemoteName != "" {
		t.Errorf("rebuilt = %+v", rebuilt)
	}
	// Control: an ordinary enumeration stays ordinary.
	plain := &model.Enumeration{Values: []model.EnumerationValue{{Name: "A"}}}
	carryRemoteEnumeration(plain, &model.Enumeration{Values: []model.EnumerationValue{{Name: "A"}}})
	if plain.RemoteName != "" || plain.Values[0].RemoteName != "" {
		t.Errorf("plain = %+v", plain)
	}
}
