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

// mendixlabs/mxcli#1515 item 2 (ako/mxcli#1397): `toString` of a Decimal-valued
// expression (minutesBetween, div) "can produce a long fraction string (43
// characters seen at runtime) and overflow a bounded String attribute.
// Assigning a Decimal to an Integer is caught (MDL041), toString of one is not.
// Expected: a warning."
func toStringShortAttrWarnings(t *testing.T, src string) string {
	t.Helper()
	mod := mkModule("Tmp")
	item := mkEntity(mod.ID, "Item")
	item.Attributes = []*domainmodel.Attribute{
		{Name: "Lbl", Type: &domainmodel.StringAttributeType{Length: 10}},
		{Name: "Note", Type: &domainmodel.StringAttributeType{Length: 0}}, // unlimited
	}
	dm := mkDomainModel(mod.ID, item)
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc:      func() ([]*types.FolderInfo, error) { return nil, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetModuleByNameFunc:  func(string) (*model.Module, error) { return mod, nil },
		GetDomainModelFunc:   func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	prog := parseMDL(t, src)
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	validateToStringIntoShortAttribute(ctx, prog, sc)
	return strings.Join(sc.warnings, "\n")
}

func TestToStringOfDecimalIntoShortAttributeWarns(t *testing.T) {
	got := toStringShortAttrWarnings(t, `create microflow Tmp.F ($a: DateTime, $b: DateTime, $i: Tmp.Item)
begin
  change $i (Lbl = toString(minutesBetween($a, $b)));
end;`)
	for _, want := range []string{"Lbl", "String(10)", "toString(minutesBetween($a, $b))", "round"} {
		if !strings.Contains(got, want) {
			t.Errorf("want a warning naming %q; got:\n%s", want, got)
		}
	}
}

func TestToStringIntoAttributeStaysQuietWhenSafe(t *testing.T) {
	for name, body := range map[string]string{
		"unlimited attribute":    `change $i (Note = toString(minutesBetween($a, $b)));`,
		"rounded first":          `change $i (Lbl = toString(round(minutesBetween($a, $b))));`,
		"integer value":          `change $i (Lbl = toString($n));`,
		"create, unlimited attr": `$x = create Tmp.Item (Note = toString($d));`,
	} {
		got := toStringShortAttrWarnings(t, `create microflow Tmp.F ($a: DateTime, $b: DateTime, $n: Integer, $d: Decimal, $i: Tmp.Item)
begin
  `+body+`
end;`)
		if got != "" {
			t.Errorf("%s: unexpected warning:\n%s", name, got)
		}
	}
	// script-declared entity, create form, Decimal parameter
	got := toStringShortAttrWarnings(t, `create persistent entity Tmp.Other (Code: String(5));
create microflow Tmp.G ($d: Decimal)
begin
  $o = create Tmp.Other (Code = toString($d));
end;`)
	if !strings.Contains(got, "Code") || !strings.Contains(got, "String(5)") {
		t.Errorf("script-declared String(5) should warn; got:\n%s", got)
	}
}
