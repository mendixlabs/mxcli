// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ako/mxcli#2189 (mendixlabs/mxcli#1843): access-rule constraints comparing a
// token with a member of the wrong type passed check and exec, and mx check
// (11.14.0, security level Production) reported CE0161 "Error(s) in XPath
// constraint." at the access rule. Every case below is the issue's measurement.
const accessRuleTokenDecls = accessRuleDecls + `create persistent entity M.Acc extends System.User ( Nick: String(50) );
create association M.E_Acc from M.E to M.Acc;
`

func TestAccessRuleXPath_TokenTypes(t *testing.T) {
	ctx := accessRuleCtx(t)
	problems := func(where string) string {
		src := accessRuleTokenDecls + "grant read * on entity M.E to M.User where " + where + ";"
		return strings.Join(accessRuleProblems(t, ctx, src), "\n")
	}
	for _, c := range []struct{ where, want string }{
		{"[Title = '[%CurrentDateTime%]']", "[%CurrentDateTime%]"},     // date token on a String
		{"[Title = '[%BeginOfCurrentDay%]']", "[%BeginOfCurrentDay%]"}, // date token on a String
		{"[Title = '[%UserRole_User%]']", "[%UserRole_User%]"},         // role token on a String
		{"[D = '[%UserRole_User%]']", "[%UserRole_User%]"},             // role token on a DateTime
		{"[Cnt = '[%CurrentDateTime%]']", "[%CurrentDateTime%]"},       // quoted token on an Integer
		{"[Cnt = '[%DayLength%]']", "[%DayLength%]"},                   // quoted token on an Integer
		{"[M.E_Kid = '[%CurrentUser%]']", "M.Kid"},                     // leads to M.Kid, not a user
	} {
		if got := problems(c.where); !strings.Contains(got, c.want) || !strings.Contains(got, "CE0161") {
			t.Errorf("%s: want a CE0161 report naming %q, got:\n%s", c.where, c.want, got)
		}
	}
	// Measured legal, and to stay silent.
	for _, where := range []string{
		"[D = '[%CurrentDateTime%]']",
		"[D > '[%BeginOfCurrentDay%]']",
		"[M.E_User = '[%CurrentUser%]']", // System.User
		"[M.E_Acc = '[%CurrentUser%]']",  // a specialization of System.User
	} {
		if got := problems(where); got != "" {
			t.Errorf("%s: legal, but reported:\n%s", where, got)
		}
	}
}
