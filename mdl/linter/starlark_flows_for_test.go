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

// ako/mxcli#1299 (mendixlabs/mxcli#1308): the issue's three checks, written
// against flows_for() and the activity box. With only activities_for(), a
// rule could not tell which log belongs to which error handler, which
// activity an annotation is attached to, or which way a split's branches go.
func TestFlowsFor_Issue1299(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	db := cat.CatalogDB()
	for _, q := range []string{
		`INSERT INTO microflows_data (Id, Name, QualifiedName, ModuleName, MicroflowType, ActivityCount, TotalActivityCount)
		 VALUES ('mf', 'Act_Store', 'M.Act_Store', 'M', 'MICROFLOW', 8, 8)`,
		`INSERT INTO activities_data (Id, ActivityType, ActionType, Sequence, MicroflowId, MicroflowQualifiedName,
			ModuleName, ErrorHandlingType, LogLevel, WithEvents, Caption, X, Y, Width, Height)
		 VALUES
		 ('note',    'Annotation',     '',                    1, 'mf', 'M.Act_Store', 'M', '',                      '',        0, 'Reset first', 360, 100, 200, 50),
		 ('change',  'ActionActivity', 'ChangeObjectAction',  2, 'mf', 'M.Act_Store', 'M', 'Rollback',              '',        0, '', 360, 200, 120, 60),
		 ('commit1', 'ActionActivity', 'CommitObjectsAction', 3, 'mf', 'M.Act_Store', 'M', 'CustomWithoutRollBack', '',        0, '', 500, 200, 120, 60),
		 ('log1',    'ActionActivity', 'LogMessageAction',    4, 'mf', 'M.Act_Store', 'M', 'Rollback',              'Warning', 0, '', 500, 330, 120, 60),
		 ('split',   'ExclusiveSplit', '',                    5, 'mf', 'M.Act_Store', 'M', 'Rollback',              '',        0, '', 700, 200, 120, 60),
		 ('commit2', 'ActionActivity', 'CommitObjectsAction', 6, 'mf', 'M.Act_Store', 'M', 'CustomWithoutRollBack', '',        1, '', 700, 330, 120, 60),
		 ('log2',    'ActionActivity', 'LogMessageAction',    7, 'mf', 'M.Act_Store', 'M', 'Rollback',              'Error',   0, '', 700, 460, 120, 60),
		 ('merge',   'ExclusiveMerge', '',                    8, 'mf', 'M.Act_Store', 'M', '',                      '',        0, '', 900, 200, 30, 30)`,
		`INSERT INTO flows_data (Id, FlowType, Sequence, MicroflowId, MicroflowQualifiedName, ModuleName,
			OriginId, DestinationId, IsErrorHandler, CaseValue, OriginConnectionIndex, DestinationConnectionIndex)
		 VALUES
		 ('f1', 'SequenceFlow',   1, 'mf', 'M.Act_Store', 'M', 'change',  'commit1', 0, '',      1, 3),
		 ('f2', 'SequenceFlow',   2, 'mf', 'M.Act_Store', 'M', 'commit1', 'log1',    1, '',      2, 0),
		 ('f3', 'SequenceFlow',   3, 'mf', 'M.Act_Store', 'M', 'commit1', 'split',   0, '',      1, 3),
		 ('f4', 'SequenceFlow',   4, 'mf', 'M.Act_Store', 'M', 'split',   'commit2', 0, 'true',  2, 0),
		 ('f5', 'SequenceFlow',   5, 'mf', 'M.Act_Store', 'M', 'split',   'merge',   0, 'false', 1, 3),
		 ('f6', 'SequenceFlow',   6, 'mf', 'M.Act_Store', 'M', 'commit2', 'log2',    1, '',      2, 0),
		 ('a1', 'AnnotationFlow', 7, 'mf', 'M.Act_Store', 'M', 'note',    'change',  0, '',      2, 0)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}

	rule := `RULE_ID = "TEST01"
RULE_NAME = "Flows"
DESCRIPTION = "probe"
CATEGORY = "quality"
SEVERITY = "info"
REQUIRES = ["full"]

def check():
    out = []
    for mf in microflows():
        acts = {a.id: a for a in activities_for(mf.qualified_name, nested = True)}
        flows = flows_for(mf.qualified_name)
        for f in flows:
            if f.is_error_handler:
                handler = acts[f.destination_id]
                if handler.action_type == "LogMessageAction" and handler.log_level != "Error":
                    out.append(violation(message = "handler of %s logs at %s" % (f.origin_id, handler.log_level)))
            if f.flow_type == "AnnotationFlow":
                out.append(violation(message = "%r annotates %s" % (acts[f.origin_id].caption, acts[f.destination_id].action_type)))
            if f.case_value in ("true", "false"):
                o, d = acts[f.origin_id], acts[f.destination_id]
                way = "down" if d.y > o.y else "right" if d.x > o.x else "other"
                out.append(violation(message = "%s branch goes %s" % (f.case_value, way)))
    return out
`
	path := filepath.Join(t.TempDir(), "flows.star")
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
		`"Reset first" annotates ChangeObjectAction`,
		"false branch goes right",
		"handler of commit1 logs at Warning",
		"true branch goes down",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("findings:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
