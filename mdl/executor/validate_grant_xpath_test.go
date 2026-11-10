// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// The access-rule constraints xpathFormProblems refuses, each measured with mx
// check on a fresh 11.14.0 project at security level Production as CE0161
// "Error(s) in XPath constraint." at the access rule while check passed, and
// the controls that built clean. Run on the real path against the PedApp
// fixture, as the other grant checks are.
const grantXPathFormSetup = "create enumeration MyFirstModule.Lvl ( Lo 'Lo', Hi 'Hi' );\n" +
	"create persistent entity MyFirstModule.Kid ( Name: string(50) );\n" +
	"create persistent entity MyFirstModule.Doc ( Title: string(100), Cnt: integer, Big: long, Price: decimal, " +
	"Status: enumeration(MyFirstModule.Lvl), Created: datetime, Flag: boolean );\n" +
	"create association MyFirstModule.Doc_Kid from MyFirstModule.Doc to MyFirstModule.Kid;\n" +
	"create association MyFirstModule.Doc_Account from MyFirstModule.Doc to Administration.Account;\n"

func grantXPathFormScript(where string) string {
	return "mdl 1;\n" + grantXPathFormSetup +
		"grant read * on entity MyFirstModule.Doc to MyFirstModule.User where " + where + ";"
}

func TestCheckGrantXPathForm_RefusedShapes(t *testing.T) {
	cases := []struct{ name, where, want string }{
		{"a variable", "[Title = $x]", "uses the variable $x"},
		{"empty on an association", "[MyFirstModule.Doc_Kid = empty]", "tests association `MyFirstModule.Doc_Kid = empty`"},
		{"not empty on an association", "[MyFirstModule.Doc_Kid != empty]", "tests association `MyFirstModule.Doc_Kid != empty`"},
		{"owner the entity does not store", "[System.owner = '[%CurrentUser%]']", "does not store owner"},
		{"an unqualified association", "[Doc_Kid/MyFirstModule.Kid/Name = 'a']", "names the association Doc_Kid unqualified"},
		{"the user token with a String", "[Title = '[%CurrentUser%]']", "compares Title (String) with '[%CurrentUser%]'"},
		{"a date token with a String", "[Title = '[%BeginOfCurrentDay%]']", "compares Title (String)"},
		{"a number with a String", "[Title = 5]", "compares Title (String) with 5"},
		{"true() with a String", "[Title = true()]", "compares Title (String) with true()"},
		{"a quoted number with an Integer", "[Cnt = '5']", "compares Cnt (Integer) with '5'"},
		{"a string with a Long", "[Big = 'abc']", "compares Big (Long)"},
		{"a string with a Decimal", "[Price = '1.5']", "compares Price (Decimal)"},
		{"the user token with a DateTime", "[Created = '[%CurrentUser%]']", "compares Created (DateTime)"},
		{"a string with a Boolean", "[Flag = 'yes']", "compares Flag (Boolean) with 'yes'"},
		{"not a key of the enumeration", "[Status = 'Bogus']", "not a key of MyFirstModule.Lvl"},
		{"a key in the wrong case", "[Status = 'lo']", "not a key of MyFirstModule.Lvl"},
		{"a number with an enumeration", "[Status = 5]", "compares Status (Enumeration) with 5"},
		{"the user with an association to a non-user", "[MyFirstModule.Doc_Kid = '[%CurrentUser%]']", "MyFirstModule.Kid is not one"},
		{"inside and/not", "[Cnt > 1 and not(Flag = 'yes')]", "compares Flag (Boolean)"},
	}
	exec, _, dir := openPedAppCopy(t)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := strings.Join(agreeCheck(t, exec, dir, grantXPathFormScript(c.where)), "\n")
			if !strings.Contains(got, c.want) || !strings.Contains(got, "CE0161") {
				t.Errorf("check must report %q as CE0161 at the access rule, reported:\n%s", c.want, got)
			}
		})
	}
}

func TestCheckGrantXPathForm_BuildableShapesPass(t *testing.T) {
	cases := []struct{ name, where string }{
		{"Boolean literal", "[Flag = 'true']"},
		{"Boolean function", "[Flag = true()]"},
		{"Integer", "[Cnt = 5]"},
		{"decimal number with an Integer", "[Cnt = 3.5]"},
		{"date string with a DateTime", "[Created = '2020-01-01']"},
		{"date token with a DateTime", "[Created < '[%CurrentDateTime%]']"},
		{"enumeration key", "[Status = 'Lo' or Status = 'Hi']"},
		{"negated enumeration key", "[not(Status = 'Hi')]"},
		{"a dollar inside a string", "[Title = '$x']"},
		{"empty on an attribute", "[Title != empty and Cnt = empty]"},
		{"the user with an association to a user specialization", "[MyFirstModule.Doc_Account = '[%CurrentUser%]']"},
		{"an association path", "[MyFirstModule.Doc_Kid/MyFirstModule.Kid]"},
	}
	exec, _, dir := openPedAppCopy(t)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := agreeCheck(t, exec, dir, grantXPathFormScript(c.where)); len(got) != 0 {
				t.Fatalf("a constraint mxbuild builds was refused:\n%s", strings.Join(got, "\n"))
			}
		})
	}
}

// An unknown attribute is CE0161 on 11.14.0; only an unknown association is
// CE1613.
func TestCheckGrantXPath_UnknownAttributeIsCE0161(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	got := strings.Join(agreeCheck(t, exec, dir, grantXPathFormScript("[NoSuch = 'x']")), "\n")
	if !strings.Contains(got, `attribute "NoSuch"`) || !strings.Contains(got, "CE0161") || strings.Contains(got, "CE1613") {
		t.Errorf("an unknown attribute must be reported as CE0161, reported:\n%s", got)
	}
}

func TestXPathVariableNames(t *testing.T) {
	cases := map[string][]string{
		"[Title = $x]":                       {"x"},
		"[contains(Title, $b) and Cnt = $a]": {"a", "b"},
		"[Title = '$x']":                     nil,
		"[MyFirstModule.Doc_Kid/MyFirstModule.Kid[Name = $n]]": {"n"},
	}
	for in, want := range cases {
		e, ok := visitor.ParseXPathConstraint(in)
		if !ok {
			t.Fatalf("parse %s", in)
		}
		got := xpathVariableNames([]ast.Expression{e})
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("xpathVariableNames(%s) = %v, want %v", in, got, want)
		}
	}
}
