// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func mapFixture(t *testing.T) *fixture {
	f := newFixture(t)
	for _, m := range []string{"Core", "Sales", "Billing", "Admin"} {
		f.module(m, "")
	}
	f.module("OIDC", "Marketplace v3.0.0")
	f.exec(`UPDATE modules_data SET DomainModelDocumentation = 'Orders and customers. Owns the order lifecycle.' WHERE Name = 'Sales'`)
	f.entity("Core.Account", "PERSISTENT", "System.User")
	f.entity("Sales.Order", "PERSISTENT", "")
	f.exec(`INSERT INTO attributes_data (Id, Name, EntityQualifiedName, ModuleName, DataType) VALUES ('a1', 'Total', 'Sales.Order', 'Sales', 'Decimal')`)
	f.exec(`INSERT INTO associations_data (Id, Name, QualifiedName, ModuleName, FromEntity, ToEntity, AssociationType)
		VALUES ('as1', 'Order_Account', 'Sales.Order_Account', 'Sales', 'Sales.Order', 'Core.Account', 'Reference')`)
	for _, m := range []string{"Sales.ACT_Place", "Sales.SUB_Price", "Core.SUB_Log", "Billing.ACT_Invoice", "Billing.SE_Nightly",
		"Core.SSO_Provision", "Billing.Worker", "Admin.ACT_Purge", "Sales.REST_GetOrder", "Admin.SUB_Audit"} {
		f.microflow(m, true, "")
	}
	f.page("Sales.Order_Overview")
	f.page("Sales.Order_Edit")
	f.page("Admin.Purge")
	f.page("Sales.Task_Approve") // allowed, reached by no click path
	f.exec(`INSERT INTO layouts_data (Id, Name, QualifiedName, ModuleName, LayoutType) VALUES ('l1', 'Popup', 'Core.Popup', 'Core', 'ModalPopup')`)
	f.exec(`UPDATE pages_data SET LayoutRef = 'Core.Popup' WHERE QualifiedName = 'Sales.Order_Edit'`)

	f.ref("Sales.Order_Account", "ASSOCIATION", "Sales.Order", "ENTITY", "associate")
	f.ref("Sales.Order_Account", "ASSOCIATION", "Core.Account", "ENTITY", "associate")
	f.call("Sales.ACT_Place", "Sales.SUB_Price")
	f.call("Sales.ACT_Place", "Core.SUB_Log")
	f.call("Billing.ACT_Invoice", "Core.SUB_Log")
	f.call("Admin.ACT_Purge", "Core.SUB_Log")
	f.call("Sales.REST_GetOrder", "Sales.SUB_Price")
	f.call("Admin.ACT_Purge", "Admin.SUB_Audit")
	f.ref("Sales.ACT_Place", "MICROFLOW", "Sales.Order", "ENTITY", "create")
	f.ref("Sales.ACT_Place", "MICROFLOW", "Sales.Order.Total", "ATTRIBUTE", "member")
	f.ref("Billing.ACT_Invoice", "MICROFLOW", "Sales.Order", "ENTITY", "retrieve")
	f.ref("Billing.SE_Nightly", "SCHEDULED_EVENT", "Billing.ACT_Invoice", "MICROFLOW", "schedule")
	f.ref("Billing.Nightly", "SCHEDULED_EVENT", "Billing.SE_Nightly", "MICROFLOW", "schedule")
	f.ref("OIDC.ProvisioningMicroflow", "CONSTANT", "Core.SSO_Provision", "MICROFLOW", "by_name")
	f.ref("Billing.Queue", "TASK_QUEUE", "Billing.Worker", "MICROFLOW", "queue")
	f.ref("Sales.API.GetOrder", "PUBLISHED_REST_OPERATION", "Sales.REST_GetOrder", "MICROFLOW", "publish")
	f.ref("Navigation.Responsive", "NAVIGATION", "Sales.Order_Overview", "PAGE", "home_page")
	f.ref("Sales.Order_Overview", "PAGE", "Sales.Order_Edit", "PAGE", "show_page")
	f.ref("Sales.Order_Overview", "PAGE", "Sales.ACT_Place", "MICROFLOW", "action")
	f.ref("Sales.Order_Overview", "PAGE", "Admin.ACT_Purge", "MICROFLOW", "action")
	f.ref("Admin.ACT_Purge", "MICROFLOW", "Admin.Purge", "PAGE", "show_page")
	f.ref("Sales.Order_Overview", "PAGE", "Core.Atlas_Default", "LAYOUT", "layout")
	f.exec(`INSERT INTO published_rest_operations_data (Id, ServiceQualifiedName, HttpMethod, Path, Microflow, ModuleName)
		VALUES ('op', 'Sales.API', 'GET', '/orders/{id}', 'Sales.REST_GetOrder', 'Sales')`)

	f.exec(`INSERT INTO user_roles (Name, IsAnonymous) VALUES ('User', 0), ('Admin', 0)`)
	f.roleMapping("User", "Sales.User")
	f.roleMapping("Admin", "Sales.User")
	f.roleMapping("Admin", "Admin.Admin")
	for _, p := range []string{"Sales.Order_Overview", "Sales.Order_Edit", "Sales.Task_Approve"} {
		f.permission("Sales.User", "PAGE", p, "VIEW", "")
	}
	f.permission("Admin.Admin", "PAGE", "Admin.Purge", "VIEW", "")
	// User may view Admin.Purge but not run the flow that opens it.
	f.permission("Sales.User", "PAGE", "Admin.Purge", "VIEW", "")
	f.permission("Sales.User", "MICROFLOW", "Sales.ACT_Place", "EXECUTE", "")
	f.permission("Admin.Admin", "MICROFLOW", "Admin.ACT_Purge", "EXECUTE", "")
	return f
}

func TestSoftwareMap(t *testing.T) {
	f := mapFixture(t)
	d, err := CollectMap(f.cat, ScopeOptions{}, map[string]string{"Billing": "Invoices and dunning."})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Scope.App, []string{"Admin", "Billing", "Core", "Sales"}) {
		t.Fatalf("app = %v", d.Scope.App)
	}
	if d.Hub != "Core" {
		t.Errorf("hub = %q, want Core (every other module uses it)", d.Hub)
	}
	purpose := map[string]string{}
	for _, m := range d.Modules {
		purpose[m.Name] = m.Purpose
	}
	if purpose["Sales"] != "Orders and customers." || purpose["Billing"] != "Invoices and dunning." {
		t.Errorf("purposes = %v", purpose)
	}

	trig := map[string]string{}
	for _, fl := range d.Flows {
		for _, tr := range fl.Triggers {
			trig[fl.Name] = tr.Kind
		}
	}
	for flow, kind := range map[string]string{
		"Core.SSO_Provision": "constant", "Billing.Worker": "task queue", "Billing.ACT_Invoice": "schedule",
		"Sales.REST_GetOrder": "REST / service", "Sales.ACT_Place": "page button",
	} {
		if trig[flow] != kind {
			t.Errorf("%s started by %q, want %q (all %v)", flow, trig[flow], kind, trig)
		}
	}
	if _, ok := trig["Sales.SUB_Price"]; ok {
		t.Error("a sub-microflow is not an entry point")
	}

	roles := map[string]MapRole{}
	for _, r := range d.Roles {
		roles[r.Name] = r
	}
	// User may view Admin.Purge but may not run ACT_Purge, the only flow that
	// opens it, so no click path reaches it; Task_Approve is allowed and
	// reached by nothing. Admin may run ACT_Purge and reaches the page.
	if !reflect.DeepEqual(roles["User"].Reachable, []string{"Sales.Order_Edit", "Sales.Order_Overview"}) ||
		!reflect.DeepEqual(roles["User"].Unreached, []string{"Admin.Purge", "Sales.Task_Approve"}) {
		t.Errorf("User: reachable %v, unreached %v", roles["User"].Reachable, roles["User"].Unreached)
	}
	if !containsStr(roles["Admin"].Reachable, "Admin.Purge") {
		t.Errorf("Admin reaches %v", roles["Admin"].Reachable)
	}

	var edit MapPage
	for _, p := range d.Pages {
		if p.Name == "Sales.Order_Edit" {
			edit = p
		}
	}
	if !edit.Popup {
		t.Error("a page on a ModalPopup layout is a pop-up (from layouts.LayoutType, not the name)")
	}
	var order MapEntity
	for _, e := range d.Entities {
		if e.Name == "Sales.Order" {
			order = e
		}
	}
	if !reflect.DeepEqual(order.WrittenBy, []string{"Sales.ACT_Place"}) || !reflect.DeepEqual(order.Attributes, []string{"Total: Decimal"}) {
		t.Errorf("order = %+v", order)
	}
	if len(d.Published) != 1 || d.Published[0].Closure != 2 {
		t.Errorf("published = %+v (closure of REST_GetOrder is itself + SUB_Price)", d.Published)
	}
	if len(d.SharedFlows) != 1 || d.SharedFlows[0].Flow != "Core.SUB_Log" {
		t.Errorf("shared = %+v", d.SharedFlows)
	}
	idx := map[string]MapIndexDoc{}
	for _, x := range d.Index {
		idx[x.Name] = x
	}
	if !reflect.DeepEqual(idx["Sales.Order"].UsedBy, []string{"Billing.ACT_Invoice", "Sales.ACT_Place", "Sales.Order_Account"}) {
		t.Errorf("Sales.Order used by %v (the attribute reference folds into the entity)", idx["Sales.Order"].UsedBy)
	}
	foundSystems := map[string]bool{}
	for _, s := range d.Systems {
		foundSystems[s.Kind+":"+s.Name] = true
	}
	if !foundSystems["identity:OIDC"] || !foundSystems["published REST:Sales.API"] {
		t.Errorf("systems = %+v", d.Systems)
	}

	// Rendering: annotations overrule purposes; the page embeds the data safely.
	env, _ := NewEnvelope("map", "Shop", "now", "test", d, MapMethod, MapLimits, nil)
	ann := &Annotations{Items: map[string]Note{"module:Admin": {Note: "Back office </script><b>x</b>"}}}
	var html bytes.Buffer
	if err := RenderMapHTML(&html, env, ann); err != nil {
		t.Fatal(err)
	}
	page := html.String()
	if strings.Count(page, "</script>") != 2 {
		t.Error("annotation text closed the data script element")
	}
	mustContain(t, page, "Back office \\u003c/script\\u003e", "Software map — Shop")
	doc, err := RenderMap(env, ann)
	if err != nil {
		t.Fatal(err)
	}
	var md bytes.Buffer
	_ = RenderMarkdown(&md, doc)
	mustContain(t, md.String(), "| constant | `OIDC.ProvisioningMicroflow` | `Core.SSO_Provision` |", "Sales → Core", "Back office")
}
