// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ako/mxcli#2099 (mendixlabs/mxcli#1803): an access rule's WHERE whose members
// all resolve still failed the build —
//
//	[error] [CE0161] "Error(s) in XPath constraint." at Access rule of entity 'M.E'
//
// — while `check -p` and exec passed it. Measured on mxbuild 11.14.0 at security
// level Production, one entity per constraint (see validate_access_rule_xpath.go
// for the table).

// accessRuleDecls is the issue's entity, declared by the script.
const accessRuleDecls = `create enumeration M.Color ( Red 'Red', Grn 'Green' );
create persistent entity M.Kid ( Name: String(100) );
create persistent entity M.E ( Title: String(100), Cnt: Integer, Big: Long, Amt: Decimal, Flag: Boolean, D: DateTime, Status: Enumeration(M.Color) );
create persistent entity M.O ( Title: String(100), owner: AutoOwner );
create association M.E_Kid from M.E to M.Kid;
create association M.E_User from M.E to System.User;
create module role M.User;
`

func accessRuleCtx(t *testing.T) *ExecContext {
	t.Helper()
	mod := mkModule("M")
	dm := &domainmodel.DomainModel{BaseElement: model.BaseElement{ID: model.ID("dm-m")}, ContainerID: mod.ID}
	h := mkHierarchy(mod)
	withContainer(h, dm.ID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc:  func(string) (*model.Module, error) { return mod, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx
}

// accessRuleProblems runs both tiers a grant goes through — the no-project
// rules (ValidateProgram) and the project's reference pass — and returns every
// message, errors only.
func accessRuleProblems(t *testing.T, ctx *ExecContext, src string) []string {
	t.Helper()
	prog := parseMDL(t, src)
	var out []string
	for _, v := range ValidateProgram(prog, "") {
		if v.Severity.String() == "error" {
			out = append(out, v.RuleID+": "+v.Message)
		}
	}
	for _, e := range validateGrantReferences(ctx, prog) {
		out = append(out, e.Error())
	}
	return out
}

func TestAccessRuleXPath_ReportsWhatMxbuildRejects(t *testing.T) {
	cases := []struct{ name, where, want string }{
		// the issue's seven
		{"a variable", "[Title = $x]", "$x"},
		{"empty on an association", "[M.E_Kid = empty]", "M.E_Kid = empty"},
		{"user token on a String", "[Title = '[%CurrentUser%]']", "[%CurrentUser%]"},
		{"quoted value on an Integer", "[Cnt = '5']", "Integer"},
		{"enumeration key is Red", "[Status = 'red']", "Red, Grn"},
		{"entity stores no owner", "[System.owner = '[%CurrentUser%]']", "does not store owner"},
		{"association without its module", "[E_Kid/M.Kid/Name = 'a']", "M.E_Kid"},
		// their measured siblings
		{"$currentUser", "[M.E_User = $currentUser]", "$currentUser"},
		{"a variable path", "[Title = $currentObject/Title]", "$currentObject"},
		{"!= empty on an association", "[M.E_Kid != empty]", "M.E_Kid != empty"},
		{"user token on an Integer", "[Cnt = '[%CurrentUser%]']", "[%CurrentUser%]"},
		{"user token on an enumeration", "[Status = '[%CurrentUser%]']", "[%CurrentUser%]"},
		{"quoted value, reversed", "['5' = Cnt]", "Integer"},
		{"quoted value on a Long", "[Big = 'abc']", "Long"},
		{"quoted value on a Decimal", "[Amt = '5.5']", "Decimal"},
		{"quoted value in an ordering", "[Cnt > '5']", "Integer"},
		{"boolean on an Integer", "[Cnt = true()]", "Integer"},
		{"number on a String", "[Title = 5]", "String"},
		{"boolean on a String", "[Title = true()]", "String"},
		{"non-boolean string on a Boolean", "[Flag = 'yes']", "Boolean"},
		{"number on a Boolean", "[Flag = 1]", "Boolean"},
		{"number on a DateTime", "[D = 5]", "DateTime"},
		{"caption, not key", "[Status = 'Green']", "Red, Grn"},
		{"empty key", "[Status = '']", "Red, Grn"},
		{"number on an enumeration", "[Status = 5]", "M.Color"},
		{"inside an and", "[Title = 'a' and Cnt = '5']", "Integer"},
	}
	ctx := accessRuleCtx(t)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := accessRuleDecls + "grant read * on entity M.E to M.User where " + c.where + ";"
			got := strings.Join(accessRuleProblems(t, ctx, src), "\n")
			if !strings.Contains(got, c.want) || !strings.Contains(got, "CE0161") {
				t.Errorf("%s: want a CE0161 report naming %q, got:\n%s", c.where, c.want, got)
			}
		})
	}
}

// The legal twins: each measured at 0 errors on the same app.
func TestAccessRuleXPath_LegalTwinsPass(t *testing.T) {
	cases := []struct{ entity, where string }{
		{"M.E", "[M.E_User = '[%CurrentUser%]']"},
		{"M.E", "[not(M.E_Kid/M.Kid)]"},
		{"M.E", "[M.E_Kid/M.Kid/Name = 'a']"},
		{"M.O", "[System.owner = '[%CurrentUser%]']"},
		{"M.E", "[Cnt = 5]"},
		{"M.E", "[Cnt = 5.5]"},
		{"M.E", "[Cnt = -5]"},
		{"M.E", "[Cnt < 5 and Cnt >= -3]"},
		{"M.E", "[Big = 5.5]"},
		{"M.E", "[Amt = 5]"},
		{"M.E", "[Title = '5']"},
		{"M.E", "[Title = '']"},
		{"M.E", "[Flag = 'true']"},
		{"M.E", "[Flag = 'TRUE']"},
		{"M.E", "[Flag = 'false']"},
		{"M.E", "[Flag = true]"},
		{"M.E", "[Flag = false()]"},
		{"M.E", "[D = '2020-01-01']"},
		{"M.E", "[D = 'abc']"},
		{"M.E", "[D = '[%CurrentDateTime%]']"},
		{"M.E", "[D > '[%BeginOfCurrentDay%]']"},
		{"M.E", "[Status = 'Red']"},
		{"M.E", "[Status = 'Red' or Status = 'Grn']"},
		{"M.E", "[Title = empty]"},
		{"M.E", "[Cnt = empty]"},
		{"M.E", "[Status = empty]"},
		{"M.E", "[Status = 'Red' and Cnt = 5 and Flag = 'false' and D = '2020-01-01' and Title = 'x']"},
		// Not established, so not judged: a token other than CurrentUser on a
		// number (measured CE0161, but the token vocabulary is not modelled),
		// and an attribute reached over a path.
		{"M.E", "[Cnt = '[%DayLength%]']"},
		{"M.E", "[M.E_Kid/M.Kid/Name = 5]"},
	}
	ctx := accessRuleCtx(t)
	for _, c := range cases {
		t.Run(c.where, func(t *testing.T) {
			src := accessRuleDecls + "grant read * on entity " + c.entity + " to M.User where " + c.where + ";"
			if got := accessRuleProblems(t, ctx, src); len(got) != 0 {
				t.Errorf("%s: a constraint mxbuild accepts was refused:\n%s", c.where, strings.Join(got, "\n"))
			}
		})
	}
}

// The same rules against a STORED entity and enumeration, which the project
// answers for rather than the script.
func TestAccessRuleXPath_StoredEntity(t *testing.T) {
	mod := mkModule("M")
	enum := &model.Enumeration{BaseElement: model.BaseElement{ID: model.ID("en-color")}, ContainerID: mod.ID, Name: "Color",
		Values: []model.EnumerationValue{{Name: "Red"}, {Name: "Grn"}}}
	e := &domainmodel.Entity{BaseElement: model.BaseElement{ID: model.ID("e-e")}, Name: "E", Persistable: true,
		Attributes: []*domainmodel.Attribute{
			{Name: "Cnt", Type: &domainmodel.IntegerAttributeType{}},
			{Name: "Status", Type: &domainmodel.EnumerationAttributeType{EnumerationRef: "M.Color"}},
		}}
	dm := &domainmodel.DomainModel{BaseElement: model.BaseElement{ID: model.ID("dm-m")}, ContainerID: mod.ID,
		Entities: []*domainmodel.Entity{e}}
	h := mkHierarchy(mod)
	withContainer(h, dm.ID, mod.ID)
	withContainer(h, enum.ID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc:  func(string) (*model.Module, error) { return mod, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return []*model.Enumeration{enum}, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	for where, want := range map[string]string{
		"[Cnt = '5']":      "Integer",
		"[Status = 'red']": "Red, Grn",
		"[Status = 'Red']": "",
		"[Cnt = 5]":        "",
	} {
		t.Run(where, func(t *testing.T) {
			got := strings.Join(accessRuleProblems(t, ctx,
				"create module role M.User;\ngrant read * on entity M.E to M.User where "+where+";"), "\n")
			if want == "" && got != "" {
				t.Errorf("%s: refused a legal constraint:\n%s", where, got)
			}
			if want != "" && (!strings.Contains(got, want) || !strings.Contains(got, "CE0161")) {
				t.Errorf("%s: want a CE0161 report naming %q, got:\n%s", where, want, got)
			}
		})
	}
}
