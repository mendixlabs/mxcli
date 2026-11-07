// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
	_ "modernc.org/sqlite"
)

// microflows() yields microflows, nanoflows and rules, because all three share
// one catalog table. A shipped rule that loops over it and hardcodes
// document_type="Microflow" and "Microflow '…'" reports a nanoflow or a rule
// under the wrong doctype -- in the message and in the documentType field of
// the JSON and report output. The Go rules fixed this with
// Microflow.DocumentNoun(); these tests hold the Starlark rules to the same.

// shippedFlowRules are the shipped rules that walk every flow flavour. A rule
// that filters to microflow_type == "MICROFLOW" never reports a nanoflow and is
// not listed; TestEveryFlowWalkingRuleIsCovered keeps this list complete.
const flowRulesDir = "../../.claude/lint-rules"

var shippedFlowRules = []string{
	"conv009_max_microflow_objects.star",
	"conv010_act_microflow_content.star",
	"example_microflow.star",
	"long_microflows.star",
	"mccabe_complexity.star",
	"orphaned_elements.star",
}

func TestShippedFlowRulesNameNanoflowsAndRulesCorrectly(t *testing.T) {
	db := flowKindsFixtureDB(t)
	wantType := map[string]string{"MF": "Microflow", "NF": "Nanoflow", "RU": "Rule"}

	for _, name := range shippedFlowRules {
		t.Run(name, func(t *testing.T) {
			r, err := linter.LoadStarlarkRule(filepath.Join(flowRulesDir, name))
			if err != nil {
				t.Fatalf("LoadStarlarkRule: %v", err)
			}
			seen := map[string]bool{}
			for _, v := range r.Check(linter.NewLintContextFromDB(db)) {
				doc := v.Location.DocumentName
				kind := doc[strings.LastIndex(doc, "_")+1:]
				want, ok := wantType[kind]
				if !ok {
					continue
				}
				seen[kind] = true
				if v.Location.DocumentType != want {
					t.Errorf("%s: document_type %q, want %q", doc, v.Location.DocumentType, want)
				}
				if kind != "MF" && strings.Contains(strings.ToLower(v.Message), "microflow '") {
					t.Errorf("%s: message calls a %s a microflow: %q", doc, strings.ToLower(want), v.Message)
				}
			}
			// Without a finding on each flavour the assertions above are vacuous.
			for kind := range wantType {
				if !seen[kind] {
					t.Errorf("no finding on the %s fixture flow -- the fixture no longer triggers this rule", kind)
				}
			}
		})
	}
}

// A new shipped rule that walks microflows() without filtering to MICROFLOW
// must join shippedFlowRules, or it can hardcode "Microflow" unnoticed.
func TestEveryFlowWalkingRuleIsCovered(t *testing.T) {
	loop := regexp.MustCompile(`for \w+ in microflows\(\)`)
	paths, err := filepath.Glob(filepath.Join(flowRulesDir, "*.star"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no shipped rules under %s: %v", flowRulesDir, err)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		src := string(data)
		if !loop.MatchString(src) || strings.Contains(src, `microflow_type != "MICROFLOW"`) {
			continue
		}
		name := filepath.Base(path)
		found := false
		for _, n := range shippedFlowRules {
			found = found || n == name
		}
		if !found {
			t.Errorf("%s walks microflows() (which yields nanoflows and rules too) but is not in shippedFlowRules", name)
		}
	}
}

// Two flows per flavour, each shaped to trip every rule in shippedFlowRules:
// Big_<K> has no naming prefix, and ACT_<K> contains a forbidden action; both
// are large, complex and unreferenced.
func flowKindsFixtureDB(t *testing.T) catalog.CatalogDB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	stmts := []string{
		`CREATE TABLE modules (Id TEXT, Name TEXT, Source TEXT)`,
		`INSERT INTO modules VALUES ('m1', 'Sales', '')`,
		`CREATE TABLE microflows (
			Id TEXT, Name TEXT, QualifiedName TEXT, ModuleName TEXT, Folder TEXT,
			MicroflowType TEXT, Description TEXT, ReturnType TEXT,
			ParameterCount INTEGER, ActivityCount INTEGER, TotalActivityCount INTEGER DEFAULT 0, Complexity INTEGER,
			Excluded INTEGER DEFAULT 0, ApplyEntityAccess INTEGER DEFAULT 0)`,
		`INSERT INTO microflows (Id, Name, QualifiedName, ModuleName, Folder, MicroflowType, Description, ReturnType, ParameterCount, ActivityCount, TotalActivityCount, Complexity) VALUES
			('f1','Big_MF','Sales.Big_MF','Sales','','MICROFLOW','','',0,100,100,50),
			('f2','Big_NF','Sales.Big_NF','Sales','','NANOFLOW','','',0,100,100,50),
			('f3','Big_RU','Sales.Big_RU','Sales','','RULE','','',0,100,100,50),
			('f4','ACT_MF','Sales.ACT_MF','Sales','','MICROFLOW','','',0,100,100,50),
			('f5','ACT_NF','Sales.ACT_NF','Sales','','NANOFLOW','','',0,100,100,50),
			('f6','ACT_RU','Sales.ACT_RU','Sales','','RULE','','',0,100,100,50)`,
		`CREATE TABLE activities (
			Id TEXT, Name TEXT, Caption TEXT, ActivityType TEXT, ActionType TEXT,
			MicroflowId TEXT, MicroflowQualifiedName TEXT, ModuleName TEXT, EntityRef TEXT,
			ServiceRef TEXT, ActionRef TEXT, UseRequestTimeout INTEGER, TimeoutExpression TEXT,
			Sequence INTEGER, ParentLoopId TEXT, LoopDepth INTEGER,
			Description TEXT, AutoGenerateCaption INTEGER, ConditionExpression TEXT,
			ConditionRule TEXT, ErrorHandlingType TEXT, LogLevel TEXT, LogNodeExpression TEXT,
			LogMessage TEXT, CommitType TEXT, WithEvents INTEGER, RetrieveSource TEXT,
			QueueRef TEXT)`,
		`INSERT INTO activities (Id, Name, Caption, ActivityType, ActionType, MicroflowId,
			MicroflowQualifiedName, ModuleName, EntityRef, ServiceRef, ActionRef,
			UseRequestTimeout, TimeoutExpression, Sequence, ParentLoopId, LoopDepth) VALUES
			('a1','','','ActionActivity','ChangeObjectAction','f4','Sales.ACT_MF','Sales','','','',0,'',1,'',0),
			('a2','','','ActionActivity','ChangeObjectAction','f5','Sales.ACT_NF','Sales','','','',0,'',1,'',0),
			('a3','','','ActionActivity','ChangeObjectAction','f6','Sales.ACT_RU','Sales','','','',0,'',1,'',0)`,
		// One unrelated row, so the refs table reads as populated.
		`CREATE TABLE refs (
			SourceType TEXT, SourceId TEXT, SourceName TEXT, TargetType TEXT,
			TargetId TEXT, TargetName TEXT, RefKind TEXT, ModuleName TEXT)`,
		`INSERT INTO refs VALUES ('PAGE','p1','Sales.P','ENTITY','e1','Sales.E','parameter','Sales')`,
		`CREATE TABLE pages (Id TEXT, Name TEXT, QualifiedName TEXT, ModuleName TEXT, Folder TEXT,
			Title TEXT, URL TEXT, Description TEXT, WidgetCount INTEGER,
			Class TEXT, Style TEXT, DynamicClasses TEXT)`,
		`CREATE TABLE entities (
			Id TEXT, Name TEXT, QualifiedName TEXT, ModuleName TEXT, Folder TEXT,
			EntityType TEXT, Description TEXT, Generalization TEXT,
			AttributeCount INTEGER, AccessRuleCount INTEGER, ValidationRuleCount INTEGER,
			HasEventHandlers INTEGER, IsExternal INTEGER,
			HasCreatedDate INTEGER, HasChangedDate INTEGER,
			HasOwner INTEGER, HasChangedBy INTEGER)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("fixture %q: %v", s, err)
		}
	}
	return catalog.WrapSqlDB(db)
}
