// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"bytes"
	"strings"
	"testing"
)

func TestClassifyXPath(t *testing.T) {
	cases := map[string]string{
		"":                                 AccessAll,
		"[System.owner='[%CurrentUser%]']": AccessOwn,
		"[ id = '[%CurrentUser%]' ]":       AccessOwn,
		"[Shop.Order_Customer/Shop.Customer/Shop.Customer_User = '[%CurrentUser%]']": AccessViaUser,
		"[Status = 'Open']": AccessFiltered,
	}
	for x, want := range cases {
		if got := ClassifyXPath(x); got != want {
			t.Errorf("ClassifyXPath(%q) = %q, want %q", x, got, want)
		}
	}
}

func TestLooksLikeKeyMaterial(t *testing.T) {
	for _, v := range []string{
		"q8Zr0d5m3l9xS2tVb6wYcA==",                     // base64, 16 bytes
		"3q2+7wAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", // base64, 32 bytes
		"00112233445566778899aabbccddeeff",             // hex, 128 bits
	} {
		if !LooksLikeKeyMaterial(v) {
			t.Errorf("%q should look like key material", v)
		}
	}
	for _, v := range []string{"", "mxfeedback-form-data", "Feedback Module 4.0.2", "https://example.com"} {
		if LooksLikeKeyMaterial(v) {
			t.Errorf("%q should not look like key material", v)
		}
	}
}

func securityFixture(t *testing.T) *fixture {
	f := newFixture(t)
	f.module("Shop", "")
	f.module("System", "")
	f.entity("Shop.Customer", "PERSISTENT", "System.User")
	f.entity("Shop.Order", "PERSISTENT", "")
	f.roleMapping("User", "Shop.User")
	f.roleMapping("Admin", "Shop.Admin")
	f.permission("Shop.User", "ENTITY", "Shop.Order", "READ", "[Shop.Order_Customer = '[%CurrentUser%]']")
	f.permission("Shop.Admin", "ENTITY", "Shop.Order", "READ", "")
	f.permission("Shop.Admin", "ENTITY", "Shop.Order", "WRITE", "")
	f.permission("Shop.Admin", "ENTITY", "Shop.Order", "DELETE", "")
	f.permission("Shop.User", "ENTITY", "Shop.Customer", "CREATE", "")
	// Acts for "me", taken from the page: the substitution the prototype proved.
	f.microflow("Shop.ACT_Subscribe", true, "")
	f.param("Shop.ACT_Subscribe", "Customer", "Object:Shop.Customer")
	f.permission("Shop.User", "MICROFLOW", "Shop.ACT_Subscribe", "EXECUTE", "")
	f.source("Shop.ACT_Subscribe", "create or modify microflow Shop.ACT_Subscribe ($Customer: Shop.Customer) begin commit $Customer; end")
	// Checks the caller itself.
	f.microflow("Shop.ACT_Cancel", true, "")
	f.param("Shop.ACT_Cancel", "Order", "Object:Shop.Order")
	f.permission("Shop.User", "MICROFLOW", "Shop.ACT_Cancel", "EXECUTE", "")
	f.source("Shop.ACT_Cancel", "if $Order/Shop.Order_Customer = $currentUser then ... end")
	// Entity access off and it writes.
	f.microflow("Shop.ACT_Delete", false, "")
	f.param("Shop.ACT_Delete", "Order", "Object:Shop.Order")
	f.permission("Shop.User", "MICROFLOW", "Shop.ACT_Delete", "EXECUTE", "")
	f.ref("Shop.ACT_Delete", "MICROFLOW", "Shop.Order", "ENTITY", "delete")

	f.constant("Shop.EncryptionKey", "String", "q8Zr0d5m3l9xS2tVb6wYcA==", false)
	f.constant("Shop.LocalStorageKey", "String", "shop-cart", true)
	f.constant("Shop.MockDataOn", "Boolean", "true", false)
	return f
}

func TestSecurityReview(t *testing.T) {
	f := securityFixture(t)
	d, err := CollectSecurity(f.cat, SecurityInput{Settings: SecuritySettings{Known: true, Level: "CheckEverything",
		DemoUsersEnabled: true, DemoUsers: []DemoUser{{Name: "demo_administrator", Roles: []string{"Admin"}}}, AdminRoles: []string{"Admin"}},
		Lint: []LintFinding{{Rule: "CONV007", Severity: "info", Message: "unconstrained", Where: "Shop.Order"}}})
	if err != nil {
		t.Fatal(err)
	}
	risk := map[string]string{}
	for _, fl := range d.Flows {
		risk[fl.Name] = fl.Risk
	}
	for name, want := range map[string]string{"Shop.ACT_Subscribe": "high", "Shop.ACT_Cancel": "ok", "Shop.ACT_Delete": "high"} {
		if risk[name] != want {
			t.Errorf("%s risk = %q, want %q (%v)", name, risk[name], want, risk)
		}
	}
	ids := map[string]string{}
	for _, fd := range d.Findings {
		ids[fd.ID] = fd.Severity
	}
	for id, sev := range map[string]string{
		"constant-secret:Shop.EncryptionKey": "high",
		"constant-testmode:Shop.MockDataOn":  "medium",
		"setting:demo-users":                 "high",
	} {
		if ids[id] != sev {
			t.Errorf("finding %s = %q, want %q (all %v)", id, ids[id], sev, ids)
		}
	}
	if _, ok := ids["constant-secret:Shop.LocalStorageKey"]; ok {
		t.Error("a storage key name is not a secret")
	}
	grid := map[string]GridRow{}
	for _, g := range d.Grid {
		grid[g.Entity] = g
	}
	// Create and delete are their own letters, not folded into W.
	if order := grid["Shop.Order"]; order.Access["User"] != "R via user" || order.Access["Admin"] != "RWD all" {
		t.Errorf("grid for Shop.Order = %v", order.Access)
	}
	if c := grid["Shop.Customer"]; c.Access["User"] != "C" {
		t.Errorf("grid for Shop.Customer = %v, want User C (create only)", c.Access)
	}

	// A fixed finding no longer counts as open; its status shows.
	env, _ := NewEnvelope("security", "Shop", "now", "test", d, SecurityMethod, SecurityLimits, nil)
	doc, err := RenderSecurity(env, &Annotations{Items: map[string]Note{
		"finding:constant-secret:Shop.EncryptionKey": {Status: "fixed", Note: "rotated, now per environment"},
		"rule:CONV007": {Status: "intended", Note: "admin sees all orders"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var md bytes.Buffer
	_ = RenderMarkdown(&md, doc)
	mustContain(t, md.String(), "fixed: rotated, now per environment", "intended: admin sees all orders", "not a penetration test")
	if !strings.Contains(md.String(), "| open high findings | 3 |") {
		t.Errorf("want 3 open high findings (2 flows + demo users) after the key was fixed:\n%s", md.String()[:600])
	}
}

// Annotation text is written by people and must not inject markup.
func TestHTMLEscapesAnnotationText(t *testing.T) {
	var b bytes.Buffer
	_ = RenderHTML(&b, Doc{Title: "x", Sections: []Section{{Title: "s", Blocks: []Block{Para{Text: "<script>alert(1)</script> `<b>`"}}}}})
	if strings.Contains(b.String(), "<script>") || strings.Contains(b.String(), "<b>") {
		t.Errorf("unescaped markup:\n%s", b.String())
	}
	mustContain(t, b.String(), "<code>&lt;b&gt;</code>")
}

// ako/mxcli#2299: OIDC.CustomATPTokenType — a Marketplace module's constant,
// and a token TYPE — was flagged as secret-like.
func TestSecurityConstantsScopeAndDescriptorNames(t *testing.T) {
	f := newFixture(t)
	f.module("Shop", "")
	f.module("OIDC", "Marketplace v3.0.0")
	f.constant("OIDC.CustomATPTokenType", "String", "urn:custom:token", false)
	f.constant("OIDC.ClientSecret", "String", "s3cr3t-value", false)
	f.constant("Shop.ApiTokenHeader", "String", "X-Api-Token", false)
	f.constant("Shop.ApiToken", "String", "abc-123-live", false)
	ids := func(include bool) map[string]bool {
		d, err := CollectSecurity(f.cat, SecurityInput{Scope: ScopeOptions{IncludeMarketplace: include}})
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, fd := range d.Findings {
			out[fd.ID] = true
		}
		return out
	}
	got := ids(false)
	if !got["constant-secret:Shop.ApiToken"] {
		t.Errorf("the app's own token constant is a finding: %v", got)
	}
	for _, id := range []string{"constant-secret:Shop.ApiTokenHeader", "constant-secret:OIDC.ClientSecret", "constant-secret:OIDC.CustomATPTokenType"} {
		if got[id] {
			t.Errorf("unexpected %s (a header name, or a module outside the app)", id)
		}
	}
	got = ids(true)
	if !got["constant-secret:OIDC.ClientSecret"] || got["constant-secret:OIDC.CustomATPTokenType"] {
		t.Errorf("with --include-marketplace: %v", got)
	}
}

// ako/mxcli#2304: a lint rule whose findings brain decisions accept shows as
// accepted with the decision ids, unless an annotation says otherwise.
func TestSecurityLintVerdictComesFromTheBrain(t *testing.T) {
	f := newFixture(t)
	f.module("Shop", "")
	d, err := CollectSecurity(f.cat, SecurityInput{Lint: []LintFinding{
		{Rule: "SEC001", Severity: "warning", Message: "no rules", Where: "Shop.A", AcceptedBy: "d1"},
		{Rule: "SEC001", Severity: "warning", Message: "no rules", Where: "Shop.B", AcceptedBy: "d2"},
		{Rule: "SEC010", Severity: "warning", Message: "x", Where: "Shop.P"},
		{Rule: "SEC010", Severity: "warning", Message: "x", Where: "Shop.Q", AcceptedBy: "d3"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	env, _ := NewEnvelope("security", "Shop", "now", "t", d, SecurityMethod, SecurityLimits, nil)
	doc, _ := RenderSecurity(env, nil)
	var md bytes.Buffer
	_ = RenderMarkdown(&md, doc)
	mustContain(t, md.String(), "| SEC001 | warning | 2 |  | accepted (brain d1, d2) |", "1 of 2 accepted (brain d3)")
}

// What a flow does through its sub-microflows counts: a check on the caller in
// a sub is still a check, and a commit in a sub is still a write
// (ako/mxcli#2443 — Shop.DS_Info_Line was flagged high on ako/PizzaV1 though it
// takes the caller's own cart through a sub).
func TestCallableFlowFollowsSubMicroflows(t *testing.T) {
	f := securityFixture(t)
	// Checks the caller through a sub.
	f.microflow("Shop.DS_Info", true, "")
	f.param("Shop.DS_Info", "Order", "Object:Shop.Order")
	f.permission("Shop.User", "MICROFLOW", "Shop.DS_Info", "EXECUTE", "")
	f.source("Shop.DS_Info", "call microflow Shop.SUB_Current();")
	f.ref("Shop.DS_Info", "MICROFLOW", "Shop.SUB_Current", "MICROFLOW", "call")
	f.ref("Shop.DS_Info", "MICROFLOW", "Shop.Order", "ENTITY", "change")
	f.microflow("Shop.SUB_Current", true, "")
	f.source("Shop.SUB_Current", "retrieve $Cart from Shop.Order where Shop.Order_Customer = '[%CurrentUser%]';")
	// Entity access off, writes only through a sub.
	f.microflow("Shop.ACT_Archive", false, "")
	f.param("Shop.ACT_Archive", "Order", "Object:Shop.Order")
	f.permission("Shop.User", "MICROFLOW", "Shop.ACT_Archive", "EXECUTE", "")
	f.ref("Shop.ACT_Archive", "MICROFLOW", "Shop.SUB_Commit", "MICROFLOW", "call")
	f.microflow("Shop.SUB_Commit", false, "")
	f.ref("Shop.SUB_Commit", "MICROFLOW", "Shop.Order", "ENTITY", "commit")

	d, err := CollectSecurity(f.cat, SecurityInput{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]CallableFlow{}
	for _, fl := range d.Flows {
		got[fl.Name] = fl
	}
	if fl := got["Shop.DS_Info"]; fl.Risk != "ok" || fl.CheckedBy != "Shop.SUB_Current" {
		t.Errorf("DS_Info checks the caller through SUB_Current: risk %q, checkedBy %q (%s)", fl.Risk, fl.CheckedBy, fl.Triage)
	}
	if fl := got["Shop.ACT_Archive"]; fl.Risk != "high" || !fl.Writes || fl.WritesVia != "Shop.SUB_Commit" {
		t.Errorf("ACT_Archive writes through SUB_Commit: risk %q, writes %v via %q (%s)", fl.Risk, fl.Writes, fl.WritesVia, fl.Triage)
	}
}

// Callable flows are grouped by what they take, the riskiest group first.
func TestCallableFlowsGroupedByWhatTheyTake(t *testing.T) {
	f := securityFixture(t)
	d, err := CollectSecurity(f.cat, SecurityInput{})
	if err != nil {
		t.Fatal(err)
	}
	env, _ := NewEnvelope("security", "Shop", "now", "test", d, SecurityMethod, SecurityLimits, nil)
	doc, err := RenderSecurity(env, nil)
	if err != nil {
		t.Fatal(err)
	}
	var md bytes.Buffer
	_ = RenderMarkdown(&md, doc)
	out := md.String()
	order := strings.Index(out, "**Takes `Shop.Order` · 2 flows · 1 high**")
	customer := strings.Index(out, "**Takes `Shop.Customer` · 1 flow · 1 high**")
	if order < 0 || customer < 0 {
		t.Fatalf("want one captioned table per parameter entity:\n%s", out[strings.Index(out, "Client-callable"):])
	}
	if customer > order {
		t.Error("groups with the same worst risk are ordered by entity")
	}
}
