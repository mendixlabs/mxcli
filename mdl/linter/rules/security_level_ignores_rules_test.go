// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/security"
)

// ako/mxcli#2027: a blank app ships with security level Off. A project then
// wrote module roles, XPath-constrained entity access, page grants and demo
// users, and check, lint, mx check and test --local all passed — at runtime a
// customer saw every row and every page, and no demo user existed. At
// Prototype the pages were guarded but every row was still readable: only
// Production applies entity access. SEC010–SEC012 stay silent below
// Production, so nothing said so.

func sec013(f *readAccessFixture) []string {
	f.t.Helper()
	var msgs []string
	for _, v := range NewSecurityLevelIgnoresRulesRule().Check(f.ctx()) {
		if v.RuleID != "SEC013" {
			f.t.Errorf("RuleID = %q, want SEC013", v.RuleID)
		}
		msgs = append(msgs, v.Message+" | "+v.Suggestion)
	}
	return msgs
}

func TestSEC013_OffIgnoresAccessRulesAndDemoUsers(t *testing.T) {
	f := newReadAccessFixture(t, security.SecurityLevelOff)
	f.read("App.Ticket", "App.Customer", "[Owner = '[%CurrentUser%]']")
	f.page("App.Ticket_Overview", "App.Customer")
	f.sec.DemoUsers = []*security.DemoUser{{UserName: "demo_customer"}}
	got := sec013(f)
	if len(got) != 1 {
		t.Fatalf("want one finding, got %d: %q", len(got), got)
	}
	for _, want := range []string{"Off", "1 entity", "1 page", "1 demo user", "Production"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("message lacks %q:\n%s", want, got[0])
		}
	}

	// Control: Production applies all of it.
	f.sec.SecurityLevel = security.SecurityLevelProduction
	if got := sec013(f); len(got) != 0 {
		t.Errorf("Production: want no finding, got %q", got)
	}
}

// A blank app at Off with nothing of the project's own is not reported: only
// a marketplace module's rules exist, and nothing the author wrote is ignored.
func TestSEC013_OffWithoutOwnRulesIsQuiet(t *testing.T) {
	f := newReadAccessFixture(t, security.SecurityLevelOff)
	f.exec(`UPDATE modules_data SET Source = 'marketplace' WHERE Name = 'Administration'`)
	f.read("Administration.Account", "Administration.Administrator", "")
	if got := sec013(f); len(got) != 0 {
		t.Errorf("want no finding, got %q", got)
	}
}

func TestSEC013_PrototypeIgnoresXPathConstraints(t *testing.T) {
	f := newReadAccessFixture(t, security.SecurityLevelPrototype)
	f.read("App.Ticket", "App.Customer", "[Owner = '[%CurrentUser%]']")
	f.read("App.Product", "App.Customer", "")
	f.page("App.Ticket_Overview", "App.Customer")
	got := sec013(f)
	if len(got) != 1 || !strings.Contains(got[0], "Prototype") || !strings.Contains(got[0], "App.Ticket") {
		t.Fatalf("want one Prototype finding naming App.Ticket, got %q", got)
	}
	if strings.Contains(got[0], "App.Product") {
		t.Errorf("an unconstrained rule is not affected by the level:\n%s", got[0])
	}

	// Control: no XPath constraint, nothing Prototype ignores.
	g := newReadAccessFixture(t, security.SecurityLevelPrototype)
	g.read("App.Product", "App.Customer", "")
	if got := sec013(g); len(got) != 0 {
		t.Errorf("want no finding, got %q", got)
	}
}
