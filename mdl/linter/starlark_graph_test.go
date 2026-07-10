// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/catalog"
	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
)

const graphTestRule = `
RULE_ID = "TEST001"
RULE_NAME = "graph builtins"
DESCRIPTION = "exercises the graph-analysis builtins"
CATEGORY = "test"
SEVERITY = "info"

def check():
    out = []
    for d in module_dependencies():
        out.append(violation(message = "dep %s->%s %s x%d" % (d.source_module, d.target_module, d.ref_kind, d.edges)))
    c = community_of("A.Foo")
    if c != None:
        out.append(violation(message = "community %d label %s" % (c.id, c.label)))
    l = layer_of("A.Foo")
    if l != None:
        out.append(violation(message = "layer %d" % l))
    for n in god_nodes(metric = "degree", min = 1):
        out.append(violation(message = "god %s deg %d" % (n.asset, n.degree)))
    return out
`

func TestStarlarkGraphBuiltins(t *testing.T) {
	// Use a file-based catalog: an in-memory one pools separate connections, so
	// inserts on one aren't visible to queries on another.
	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatalf("catalog.NewFromFile: %v", err)
	}
	defer cat.Close()
	db := cat.CatalogDB()

	// One cross-module call edge A.Foo -> B.Bar (drives module_dependencies,
	// god_nodes via the refs-backed views).
	if _, err := db.Exec(
		`INSERT INTO refs (SourceType, SourceId, SourceName, TargetType, TargetId, TargetName, RefKind, ModuleName, ProjectId, SnapshotId)
		 VALUES ('MICROFLOW','','A.Foo','ENTITY','','B.Bar','call','A','default','s1')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO communities_data (AssetName, ModuleName, CommunityId, ProjectId, SnapshotId)
		 VALUES ('A.Foo','A',0,'default','s1')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO graph_layers_data (AssetName, ModuleName, Layer, ProjectId, SnapshotId)
		 VALUES ('A.Foo','A',2,'default','s1')`); err != nil {
		t.Fatal(err)
	}

	// Write the rule to a temp file and load it.
	dir := t.TempDir()
	path := filepath.Join(dir, "graph_test.star")
	if err := os.WriteFile(path, []byte(graphTestRule), 0644); err != nil {
		t.Fatal(err)
	}
	rule, err := linter.LoadStarlarkRule(path)
	if err != nil {
		t.Fatalf("LoadStarlarkRule: %v", err)
	}

	ctx := linter.NewLintContext(cat, nil)
	violations := rule.Check(ctx)

	var msgs []string
	for _, v := range violations {
		msgs = append(msgs, v.Message)
	}
	joined := strings.Join(msgs, "\n")

	for _, want := range []string{
		"dep A->B call x1",    // module_dependencies
		"community 0 label A", // community_of + community_summary Label
		"layer 2",             // layer_of
		"god A.Foo deg 1",     // god_nodes (degree centrality from refs)
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in violations:\n%s", want, joined)
		}
	}
}
