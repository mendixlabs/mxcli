// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ako/mxcli#2034: ARCH001 flagged the System.WorkflowUserTask data view of
// every workflow task page and a task inbox over System.WorkflowUserTask —
// both what the write-workflows skill builds. Mendix requires them (a task
// page takes a System.WorkflowUserTask parameter, CE7410/CE7412; `complete
// task` / `open user task` only work inside one, CE1843), so there is no view
// entity or microflow to use instead. The System workflow entities are exempt;
// a cross-module reference to an app module (Sales.Order) is the control.
func TestARCH001ExemptsSystemWorkflowEntities(t *testing.T) {
	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cat.Close() })
	db := cat.CatalogDB()
	for _, q := range []string{
		`INSERT INTO modules_data (Id, Name, Source, ProjectId, SnapshotId) VALUES ('m', 'ServiceDesk', '', 'p', 's')`,
		`INSERT INTO widgets_data (Id, Name, WidgetType, ContainerId, ContainerQualifiedName, ContainerType, ModuleName, EntityRef, ProjectId, SnapshotId) VALUES
			('w1', 'dvTask', 'Forms$DataView', 'pg1', 'ServiceDesk.WF_ResolveTicket', 'PAGE', 'ServiceDesk', 'System.WorkflowUserTask', 'p', 's'),
			('w2', 'dgTasks', 'DataGrid2', 'pg2', 'ServiceDesk.WF_TaskInbox', 'PAGE', 'ServiceDesk', 'System.WorkflowUserTask', 'p', 's'),
			('w3', 'dvWorkflow', 'Forms$DataView', 'pg3', 'ServiceDesk.WF_Admin', 'PAGE', 'ServiceDesk', 'System.Workflow', 'p', 's'),
			('w4', 'dgDefs', 'DataGrid2', 'pg3', 'ServiceDesk.WF_Admin', 'PAGE', 'ServiceDesk', 'System.WorkflowDefinition', 'p', 's'),
			('w5', 'dvOrder', 'Forms$DataView', 'pg4', 'ServiceDesk.Order_View', 'PAGE', 'ServiceDesk', 'Sales.Order', 'p', 's')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	for _, dir := range []string{".claude/lint-rules", "cmd/mxcli/lint-rules"} {
		t.Run(dir, func(t *testing.T) {
			r, err := linter.LoadStarlarkRule(filepath.Join("..", "..", dir, "cross_module_data.star"))
			if err != nil {
				t.Fatal(err)
			}
			got := r.Check(linter.NewLintContext(cat, &minimalReader{}))
			var msgs []string
			for _, v := range got {
				msgs = append(msgs, v.Message)
			}
			if len(got) != 1 || !strings.Contains(got[0].Message, "Sales.Order") {
				t.Fatalf("want only the Sales.Order control flagged, got %d:\n%s", len(got), strings.Join(msgs, "\n"))
			}
		})
	}
}
