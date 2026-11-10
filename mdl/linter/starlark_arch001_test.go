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

// ako/mxcli#2069: ARCH001 flagged the data view on ConversationalUI.ChatContext
// that Conversational UI's chat snippet needs — the snippet takes that entity as
// its parameter, so the module's own entity is the only thing that can feed it,
// and its own full-screen chat page does the same. A cross-module entity is
// exempt when the page calls a snippet (or links to a page) of that entity's
// module that takes the entity as a parameter. Controls: the same entity on a
// page that calls no such snippet, and a snippet of that module whose parameter
// is a different entity.
func TestARCH001ExemptsAnEntityFedToItsOwnModulesSnippet(t *testing.T) {
	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cat.Close() })
	db := cat.CatalogDB()
	for _, q := range []string{
		`INSERT INTO modules_data (Id, Name, Source, ProjectId, SnapshotId) VALUES ('m', 'ServiceDesk', '', 'p', 's')`,
		`INSERT INTO widgets_data (Id, Name, WidgetType, ContainerId, ContainerQualifiedName, ContainerType, ModuleName, EntityRef, ProjectId, SnapshotId) VALUES
			('w1', 'dvChat', 'Forms$DataView', 'pg1', 'ServiceDesk.Chat', 'PAGE', 'ServiceDesk', 'ConversationalUI.ChatContext', 'p', 's'),
			('w2', 'dvLinked', 'Forms$DataView', 'pg2', 'ServiceDesk.ChatLink', 'PAGE', 'ServiceDesk', 'ConversationalUI.ChatContext', 'p', 's'),
			('w3', 'dvBare', 'Forms$DataView', 'pg3', 'ServiceDesk.ChatBare', 'PAGE', 'ServiceDesk', 'ConversationalUI.ChatContext', 'p', 's'),
			('w4', 'dvOther', 'Forms$DataView', 'pg4', 'ServiceDesk.Msg', 'PAGE', 'ServiceDesk', 'ConversationalUI.Message', 'p', 's')`,
		`INSERT INTO refs (SourceType, SourceId, SourceName, TargetType, TargetName, RefKind) VALUES
			('PAGE', 'pg1', 'ServiceDesk.Chat', 'SNIPPET', 'ConversationalUI.Snippet_ChatContext_ConversationalUI', 'snippet_call'),
			('SNIPPET', 'sn1', 'ConversationalUI.Snippet_ChatContext_ConversationalUI', 'ENTITY', 'ConversationalUI.ChatContext', 'parameter'),
			('PAGE', 'pg2', 'ServiceDesk.ChatLink', 'PAGE', 'ConversationalUI.ConversationalUI_FullScreenChat', 'action'),
			('PAGE', 'pgx', 'ConversationalUI.ConversationalUI_FullScreenChat', 'ENTITY', 'ConversationalUI.ChatContext', 'parameter'),
			('PAGE', 'pg4', 'ServiceDesk.Msg', 'SNIPPET', 'ConversationalUI.Snippet_ChatContext_ConversationalUI', 'snippet_call')`,
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
			flagged := map[string]bool{}
			var msgs []string
			for _, v := range got {
				msgs = append(msgs, v.Message)
				for _, w := range []string{"dvChat", "dvLinked", "dvBare", "dvOther"} {
					if strings.Contains(v.Message, "'"+w+"'") {
						flagged[w] = true
					}
				}
			}
			if flagged["dvChat"] || flagged["dvLinked"] {
				t.Errorf("an entity fed to its own module's snippet or page was flagged:\n%s", strings.Join(msgs, "\n"))
			}
			if !flagged["dvBare"] || !flagged["dvOther"] {
				t.Errorf("controls not flagged (want dvBare and dvOther):\n%s", strings.Join(msgs, "\n"))
			}
		})
	}
}
