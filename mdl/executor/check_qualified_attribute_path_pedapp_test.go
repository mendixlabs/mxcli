// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// Follow-up to ako/mxcli#2252. A page attribute path written fully qualified —
// the way a microflow writes `$X/Mod.Assoc/Mod.Entity/Attr` — is normalised by
// exec (resolveAssociationPathFrom) to the short form describe prints, while
// `check -p` passed it without a word. And the one shape exec does NOT
// normalise, `Mod.Entity/Attr` (the entity the path is read from, then the
// attribute), passed check as well and was written as one flat attribute name:
// the widget was left unbound, and mxbuild 11.16.0 reported CE0402 on a text's
// parameter, CE7005 on a text box and "An attribute is required" on a Data
// grid 2 column.
//
// check now says what exec will store for the first (MDL-ATTRPATH01, a
// warning) and refuses the second with the attribute to write
// (MDL-ATTRPATH02). Controls: the short form says nothing, and a microflow's
// qualified `$X/…` path is not a page attribute path at all.

// qualifiedPathCheck runs the reference pass of `check -p` over src and
// returns its errors and warnings.
func qualifiedPathCheck(t *testing.T, src string) (errs, warns string) {
	t.Helper()
	exec, _, _ := openPedAppCopy(t)
	prog, perrs := visitor.Build(src)
	if len(perrs) > 0 {
		t.Fatalf("parse: %v\n%s", perrs[0], src)
	}
	refErrs, warnings := exec.ValidateProgramWithWarnings(prog)
	var es []string
	for _, e := range refErrs {
		es = append(es, e.Error())
	}
	return strings.Join(es, "\n"), strings.Join(warnings, "\n")
}

func TestCheckQualifiedAttributePath_NormalisedShapesWarn(t *testing.T) {
	const src = `mdl 1;
create page Administration.QP_Normalised (title: 'QP', layout: Atlas_Core.Atlas_Default, params: ( $Pwd: Administration.AccountPasswordData )) {
  dataview dv (datasource: $Pwd) {
    dynamictext t1 (content: '{1}', contentparams: ({1} = "Administration"."AccountPasswordData_Account"/"Administration"."Account"/"FullName"))
    textbox tb1 (label: 'L', attribute: Administration.AccountPasswordData/Administration.AccountPasswordData_Account/FullName)
  }
};
create snippet Administration.QP_Snippet (params: ( $Acc: Administration.Account )) {
  dataview dv (datasource: $Acc) {
    dynamictext t2 (content: '{1}', contentparams: ({1} = Administration.Account/System.User_Language/System.Language/Code))
  }
};`
	errs, warns := qualifiedPathCheck(t, src)
	if errs != "" {
		t.Fatalf("a shape exec normalises must not be refused:\n%s", errs)
	}
	for _, want := range []string{
		"`Administration.AccountPasswordData_Account/FullName`",
		"`System.User_Language/Code`",
	} {
		if !strings.Contains(warns, want) || !strings.Contains(warns, "MDL-ATTRPATH01") {
			t.Errorf("check must say exec stores %s [MDL-ATTRPATH01]; warnings:\n%s", want, warns)
		}
	}
	if n := strings.Count(warns, "MDL-ATTRPATH01"); n != 3 {
		t.Errorf("want one MDL-ATTRPATH01 per qualified path (3), got %d:\n%s", n, warns)
	}
}

func TestCheckQualifiedAttributePath_LeadingEntityAttributeRefused(t *testing.T) {
	const src = `mdl 1;
create page Administration.QP_Flat (title: 'QP', layout: Atlas_Core.Atlas_Default, params: ( $Acc: Administration.Account )) {
  dataview dv (datasource: $Acc) {
    textbox tb1 (label: 'L', attribute: "Administration"."Account"/"FullName")
    dynamictext t1 (content: '{1}', contentparams: ({1} = System.User/Name))
  }
  datagrid dg (datasource: database Administration.Account) {
    column (attribute: Administration.Account/Email, caption: 'E')
  }
};`
	errs, _ := qualifiedPathCheck(t, src)
	for _, want := range []string{
		"`Administration.Account/FullName`", "write `FullName`",
		"`System.User/Name`", "write `Name`",
		"`Administration.Account/Email`", "write `Email`",
	} {
		if !strings.Contains(errs, want) {
			t.Errorf("check must refuse with %q; errors:\n%s", want, errs)
		}
	}
	if n := strings.Count(errs, "MDL-ATTRPATH02"); n != 3 {
		t.Errorf("want three MDL-ATTRPATH02, got %d:\n%s", n, errs)
	}
}

// Controls: the short forms exec stores, and a microflow path written fully
// qualified, say nothing.
func TestCheckQualifiedAttributePath_Controls(t *testing.T) {
	const src = `mdl 1;
create page Administration.QP_Short (title: 'QP', layout: Atlas_Core.Atlas_Default, params: ( $Pwd: Administration.AccountPasswordData, $Acc: Administration.Account )) {
  dataview dv (datasource: $Pwd) {
    dynamictext t1 (content: '{1}', contentparams: ({1} = AccountPasswordData_Account/FullName))
    textbox tb1 (label: 'L', attribute: Administration.AccountPasswordData_Account/FullName)
  }
  dataview dv2 (datasource: $Acc) {
    textbox tb2 (label: 'L', attribute: FullName)
    dynamictext t2 (content: '{1}', contentparams: ({1} = System.User_Language/Code))
  }
};
create microflow Administration.QP_Flow ($Pwd: Administration.AccountPasswordData) returns String
begin
  return $Pwd/Administration.AccountPasswordData_Account/Administration.Account/FullName;
end;`
	errs, warns := qualifiedPathCheck(t, src)
	if strings.Contains(errs, "ATTRPATH") || strings.Contains(warns, "ATTRPATH") {
		t.Errorf("control: the short form and a microflow path must say nothing\nerrors:\n%s\nwarnings:\n%s", errs, warns)
	}
}
