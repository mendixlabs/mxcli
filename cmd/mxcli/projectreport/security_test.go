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
	var order GridRow
	for _, g := range d.Grid {
		if g.Entity == "Shop.Order" {
			order = g
		}
	}
	if order.Access["User"] != "R via user" || order.Access["Admin"] != "RW all" {
		t.Errorf("grid for Shop.Order = %v", order.Access)
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
