// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"bytes"
	"strings"
	"testing"
)

func configurationFixture(t *testing.T) *fixture {
	f := newFixture(t)
	f.module("Sales", "")
	f.module("Mkt", "Marketplace v1.0.0")
	f.constant("Sales.ApiBaseUrl", "String", "", false)                      // must be set
	f.constant("Sales.PartnerUrl", "String", "http://localhost:8080", false) // wrong unless overridden
	f.constant("Sales.ApiKey", "String", "q8Zr0d5m3l9xS2tVb6wYcA==", false)  // secret in the model
	f.constant("Sales.PageSize", "Integer", "50", false)                     // default applies
	f.constant("Mkt.Endpoint", "String", "", false)                          // outside the app
	f.exec(`INSERT INTO rest_clients_data (Id, Name, QualifiedName, ModuleName, BaseUrl, BaseUrlConstant)
		VALUES ('rc1', 'Rates', 'Sales.Rates', 'Sales', 'http://10.0.0.5/rates', ''),
		       ('rc2', 'Orders', 'Sales.Orders', 'Sales', '', 'Sales.ApiBaseUrl')`)
	f.microflow("Sales.ACT_Fixed", true, "")
	f.source("Sales.ACT_Fixed", `$R = call rest service get 'https://staging.partner.com/api' (
    Authentication: basic (Username: 'svc', Password: 'Winter2026!')
  ) returns string;`)
	f.microflow("Sales.ACT_Configured", true, "")
	f.source("Sales.ACT_Configured", `$R = call rest service get '{1}' with ({1} = @Sales.PartnerUrl + '/x') returns string;`)
	f.microflow("Sales.ASU_Start", true, "")
	f.call("Sales.ASU_Start", "Sales.ACT_Fixed")
	f.ref("Sales.ASU_Start", "MICROFLOW", "Sales.Order", "ENTITY", "commit")
	f.exec(`INSERT INTO scheduled_events_data (Id, Name, QualifiedName, ModuleName, Microflow, RepeatDescription, Enabled)
		VALUES ('se', 'Nightly', 'Sales.SE_Nightly', 'Sales', 'Sales.ACT_Fixed', 'daily at 02:00', 1)`)
	f.exec(`INSERT INTO published_rest_services_data (Id, Name, QualifiedName, ModuleName, Path, OperationCount, AuthenticationTypes)
		VALUES ('ps', 'OrdersApi', 'Sales.OrdersApi', 'Sales', 'rest/orders/v1', 3, 'Basic')`)
	return f
}

// The configuration report says what each environment must supply and what
// the model fixes (ako/mxcli#2426).
func TestConfigurationReport(t *testing.T) {
	f := configurationFixture(t)
	d, err := CollectConfiguration(f.cat, ConfigurationInput{AfterStartup: "Sales.ASU_Start"})
	if err != nil {
		t.Fatal(err)
	}
	need := map[string]ConfigConstant{}
	for _, c := range d.Constants {
		need[c.Name] = c
	}
	for name, want := range map[string]string{
		"Sales.ApiBaseUrl": NeedMustSet, "Sales.PartnerUrl": NeedOverride, "Sales.ApiKey": NeedSecret, "Sales.PageSize": NeedDefault,
	} {
		if need[name].Need != want {
			t.Errorf("%s needs %q, want %q (%s)", name, need[name].Need, want, need[name].Reason)
		}
	}
	if _, ok := need["Mkt.Endpoint"]; ok {
		t.Error("a Marketplace module's constant is outside the app")
	}
	if c := need["Sales.ApiKey"]; !c.Masked || strings.Contains(c.Default, "q8Zr") {
		t.Errorf("a secret-looking default is masked: %q", c.Default)
	}
	if got := need["Sales.ApiBaseUrl"].UsedBy; len(got) != 1 || got[0] != "Sales.Orders (base URL)" {
		t.Errorf("ApiBaseUrl is the Orders client's base URL: used by %v", got)
	}
	if got := need["Sales.PartnerUrl"].UsedBy; len(got) != 1 || got[0] != "Sales.ACT_Configured" {
		t.Errorf("PartnerUrl is used in ACT_Configured's call: used by %v", got)
	}

	ep := map[string]ConfigEndpoint{}
	for _, e := range d.Endpoints {
		ep[e.Where] = e
	}
	for where, want := range map[string]struct {
		fixed bool
		host  string
		con   string
	}{
		"Sales.Rates":          {true, "private", ""},
		"Sales.Orders":         {false, "", "Sales.ApiBaseUrl"},
		"Sales.ACT_Fixed":      {true, "test", ""},
		"Sales.ACT_Configured": {false, "", "Sales.PartnerUrl"},
	} {
		e := ep[where]
		if e.Fixed != want.fixed || e.Host != want.host || e.Constant != want.con {
			t.Errorf("%s: fixed %v host %q constant %q, want %+v", where, e.Fixed, e.Host, e.Constant, want)
		}
	}
	if len(d.Credentials) != 1 || d.Credentials[0].Where != "Sales.ACT_Fixed" {
		t.Errorf("the literal password in ACT_Fixed is a credential in the model: %+v", d.Credentials)
	}
	if len(d.Lifecycle) != 1 || d.Lifecycle[0].Reaches != 2 || len(d.Lifecycle[0].Writes) != 1 ||
		len(d.Lifecycle[0].Calls) != 1 || d.Lifecycle[0].Calls[0] != "https://staging.partner.com/api" {
		t.Errorf("after startup reaches ACT_Fixed, commits Order and calls the partner: %+v", d.Lifecycle)
	}

	env, _ := NewEnvelope("configuration", "Sales", "now", "test", d, ConfigurationMethod, ConfigurationLimits, nil)
	doc, err := RenderConfiguration(env, &Annotations{Items: map[string]Note{
		"scheduled:Sales.SE_Nightly": {Status: "done", Note: "enabled in production"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var md bytes.Buffer
	_ = RenderMarkdown(&md, doc)
	out := md.String()
	mustContain(t, out,
		"| Set `Sales.ApiBaseUrl` |", "| Set `Sales.PartnerUrl` |", "Set `Sales.ApiKey` and rotate the committed value",
		"| Enable or disable `Sales.SE_Nightly` | daily at 02:00, runs `Sales.ACT_Fixed` | done: enabled in production |",
		"| Set up callers of `Sales.OrdersApi` |",
		"| `Sales.Rates` | REST client base URL | `http://10.0.0.5/rates` | a private address |",
		"basic-authentication password is a literal",
		"does not read the deployment portal")
	if strings.Contains(out, "Set `Sales.PageSize`") {
		t.Error("a constant whose default applies is not a checklist item")
	}
	if strings.Contains(out, "q8Zr0d5m3l9xS2tVb6wYcA==") || strings.Contains(out, "Winter2026!") {
		t.Error("the page must not print a secret")
	}
}

func TestHostClass(t *testing.T) {
	for loc, want := range map[string]string{
		"http://localhost:8080/":          "localhost",
		"https://127.0.0.1/x":             "localhost",
		"http://192.168.1.4":              "private",
		"https://api-test.example.org":    "test",
		"https://staging.partner.com/api": "test",
		"https://acc.shop.nl":             "test",
		"https://api.partner.com/v1":      "external",
		"https://contest.partner.com":     "external", // "test" inside a word is not a test host
		"{1}":                             "",
		"":                                "",
	} {
		if got := hostClass(loc); got != want {
			t.Errorf("hostClass(%q) = %q, want %q", loc, got, want)
		}
	}
}
