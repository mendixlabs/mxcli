// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#2297: two ways the platform reaches a flow that no call names left
// no refs row, so the flow read as dead. A constant whose value is a flow's
// qualified name (OIDC.GlobalUserProvisioningMicroflow names the app's
// provisioning microflow), and a call placed in a task queue (visible only as
// activities.QueueRef).
func TestByNameAndQueueRefs(t *testing.T) {
	queued := &microflows.Microflow{ContainerID: loopTestModule, Name: "Act_Enqueue",
		ObjectCollection: &microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{
			loopAction("q-mf", &microflows.MicroflowCallAction{MicroflowCall: &microflows.MicroflowCall{
				Microflow: "Loops.Sub_Background", QueueSettings: &microflows.QueueSettings{Queue: "Loops.Background"}}}),
			loopAction("q-ja", &microflows.JavaActionCallAction{JavaAction: "Loops.JA_Export",
				QueueSettings: &microflows.QueueSettings{Queue: "Loops.Background"}}),
			loopAction("plain", &microflows.MicroflowCallAction{MicroflowCall: &microflows.MicroflowCall{Microflow: "Loops.Sub_Background"}}),
		}}}
	queued.ID = "mf-enqueue"
	var flows []*microflows.Microflow
	flows = append(flows, queued)
	for _, n := range []string{"Sub_Background", "Provision", "Configured"} {
		mf := &microflows.Microflow{ContainerID: loopTestModule, Name: n, ObjectCollection: &microflows.MicroflowObjectCollection{}}
		mf.ID = model.ID("mf-" + n)
		flows = append(flows, mf)
	}
	cat := buildFlowsForTest(t, flows, nil, nil)
	db := cat.CatalogDB()
	for _, c := range [][3]string{
		{"OIDC.ProvisioningMicroflow", "OIDC", " Loops.Provision "}, // trimmed
		{"OIDC.Url", "OIDC", "https://x/Loops.Provision"},           // contains a name: not a call
		{"OIDC.Hook", "OIDC", ""},
	} {
		if _, err := db.Exec(`INSERT INTO constants_data (Id, Name, QualifiedName, ModuleName, DataType, DefaultValue)
			VALUES (?, ?, ?, ?, 'String', ?)`, c[0], c[0], c[0], c[1], c[2]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO constant_values (ConstantName, ConfigurationName, Value) VALUES ('OIDC.Hook', 'Prod', 'Loops.Configured')`); err != nil {
		t.Fatal(err)
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	b := &Builder{catalog: cat, snapshot: &Snapshot{ID: "snap"}, fullMode: true}
	b.tx = wrapTx(tx)
	stmt, err := b.tx.Prepare(`INSERT INTO refs (SourceType, SourceId, SourceName, TargetType, TargetId, TargetName, RefKind, ModuleName, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	n := b.extractByNameRefs(stmt, "p", "snap") + b.extractQueueRefs(stmt, "p", "snap")
	stmt.Close()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Errorf("emitted %d refs, want 4", n)
	}
	res, err := cat.Query(`SELECT SourceType, SourceName, TargetType, TargetName, RefKind, ModuleName FROM refs ORDER BY RefKind, SourceName, TargetName`)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]any{
		{"CONSTANT", "OIDC.Hook", "MICROFLOW", "Loops.Configured", "by_name", "OIDC"},
		{"CONSTANT", "OIDC.ProvisioningMicroflow", "MICROFLOW", "Loops.Provision", "by_name", "OIDC"},
		{"TASK_QUEUE", "Loops.Background", "JAVA_ACTION", "Loops.JA_Export", "queue", "Loops"},
		{"TASK_QUEUE", "Loops.Background", "MICROFLOW", "Loops.Sub_Background", "queue", "Loops"},
	}
	if len(res.Rows) != len(want) {
		t.Fatalf("refs = %v", res.Rows)
	}
	for i, w := range want {
		for j := range w {
			if res.Rows[i][j] != w[j] {
				t.Errorf("row %d = %v, want %v", i, res.Rows[i], w)
				break
			}
		}
	}
}
