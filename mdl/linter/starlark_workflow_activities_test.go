// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ako/mxcli#1295 (mendixlabs/mxcli#1269): "microflows used in workflows must
// use a prefix per role (user task, system task, ...)". A rule needs the kind
// of activity each microflow runs from, which refs (WORKFLOW -> MICROFLOW
// `call`) does not record.
func TestWorkflowActivities_PrefixPerRole_Issue1295(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	db := cat.CatalogDB()
	for _, q := range []string{
		`INSERT INTO modules_data (Id, Name, QualifiedName, ModuleName, Source) VALUES ('m', 'Sales', 'Sales', 'Sales', '')`,
		`INSERT INTO workflows_data (Id, Name, QualifiedName, ModuleName, ParameterEntity, ActivityCount)
		 VALUES ('wf', 'Approve', 'Sales.Approve', 'Sales', 'Sales.Order', 4)`,
		`INSERT INTO workflow_activities_data (Id, Sequence, WorkflowId, WorkflowQualifiedName, ModuleName, Name,
			ActivityType, Microflow, UserSourceMicroflow, OnCreatedMicroflow, Page)
		 VALUES
		 ('a1', 0, 'wf', 'Sales.Approve', 'Sales', 'Start',  'StartWorkflow', NULL, NULL, NULL, NULL),
		 ('a2', 1, 'wf', 'Sales.Approve', 'Sales', 'Review', 'UserTask',      NULL, 'Sales.WF_TGT_Managers', 'Sales.Notify', 'Sales.Review_Page'),
		 ('a3', 2, 'wf', 'Sales.Approve', 'Sales', 'Book',   'SystemTask',    'Sales.BookOrder', NULL, NULL, NULL),
		 ('a4', 3, 'wf', 'Sales.Approve', 'Sales', 'Mail',   'SystemTask',    'Sales.WF_SYS_Mail', NULL, NULL, NULL)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}

	rule := `RULE_ID = "TEST01"
RULE_NAME = "Workflow microflow prefixes"
DESCRIPTION = "probe"
CATEGORY = "naming"
SEVERITY = "info"

PREFIX = {"SystemTask": "WF_SYS_", "user_source": "WF_TGT_", "on_created": "WF_EVT_"}

def bad(mf, prefix):
    return mf != "" and not mf.split(".")[-1].startswith(prefix)

def check():
    out = []
    for wf in workflows():
        out.append(violation(message = "%s: %d activities, context %s" % (wf.qualified_name, wf.activity_count, wf.parameter_entity)))
    for a in workflow_activities():
        if a.activity_type == "SystemTask" and bad(a.microflow, PREFIX["SystemTask"]):
            out.append(violation(message = "%s/%s runs %s" % (a.workflow_qualified_name, a.name, a.microflow)))
        if bad(a.user_source_microflow, PREFIX["user_source"]):
            out.append(violation(message = "%s/%s targets with %s" % (a.workflow_qualified_name, a.name, a.user_source_microflow)))
        if bad(a.on_created_microflow, PREFIX["on_created"]):
            out.append(violation(message = "%s/%s on created %s" % (a.workflow_qualified_name, a.name, a.on_created_microflow)))
    return out
`
	path := filepath.Join(t.TempDir(), "wf.star")
	if err := os.WriteFile(path, []byte(rule), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := linter.LoadStarlarkRule(path)
	if err != nil {
		t.Fatalf("LoadStarlarkRule: %v", err)
	}
	ctx := linter.NewLintContextFromDB(db)
	var got []string
	for _, v := range r.Check(ctx) {
		got = append(got, v.Message)
	}
	if errs := ctx.QueryErrors(); len(errs) > 0 {
		t.Fatalf("query errors: %v -- the query names a column the catalog does not have", errs)
	}
	sort.Strings(got)
	want := []string{
		"Sales.Approve/Book runs Sales.BookOrder",
		"Sales.Approve/Review on created Sales.Notify",
		"Sales.Approve: 4 activities, context Sales.Order",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("findings:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
