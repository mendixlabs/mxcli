// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
)

// ako/mxcli#2454: the first lint after a model change reported "MPR017 could not
// run: read N entities but could not read their attributes", and the next run was
// right. Lint reads the catalog while a cursor is open (ctx.Entities() yields rows
// and the loop calls ctx.AttributesFor()), which needs a second pooled connection.
// An incremental build loaded the cached catalog into ONE connection, so a second
// connection saw the same database's tables with no rows: no error, just nothing.
// #2417 fixed this for the full build; TestCatalog_IncrementalBuildEqualsFullBuild
// could not see it because it reads one statement at a time.

// buildCatalogOpen is catalogRows for a caller that needs the catalog itself.
func buildCatalogOpen(t *testing.T, exec *Executor, full, force bool) (*catalog.Catalog, catalog.BuildStats) {
	t.Helper()
	var stats catalog.BuildStats
	catalogBuildHook = func(s catalog.BuildStats) { stats = s }
	defer func() { catalogBuildHook = nil }()
	ctx := exec.newExecContext(context.Background())
	var out bytes.Buffer
	ctx.Output = &out
	ctx.catalogForce = force
	if err := buildCatalog(ctx, full, false, false, 0); err != nil {
		t.Fatalf("build: %v\n%s", err, out.String())
	}
	return ctx.Catalog, stats
}

// readsWhileACursorIsOpen counts attribute rows the way lint's rules do: one query
// per entity, issued while the entities cursor is still open. readsOneAtATime
// counts the same rows with every cursor closed before the next query.
func readsWhileACursorIsOpen(t *testing.T, cat *catalog.Catalog) int {
	t.Helper()
	db := cat.CatalogDB()
	rows, err := db.Query(`SELECT QualifiedName FROM entities`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var qn string
		if err := rows.Scan(&qn); err != nil {
			t.Fatal(err)
		}
		inner, err := db.Query(`SELECT Name FROM attributes WHERE EntityQualifiedName = ?`, qn)
		if err != nil {
			t.Fatalf("nested read for %s: %v", qn, err)
		}
		for inner.Next() {
			n++
		}
		inner.Close()
	}
	return n
}

func readsOneAtATime(t *testing.T, cat *catalog.Catalog) int {
	t.Helper()
	res, err := cat.Query(`SELECT COUNT(*) FROM attributes`)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if _, err := fmt.Sscan(fmt.Sprint(res.Rows[0][0]), &n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCatalog_IncrementalBuildAnswersReadsMadeWhileACursorIsOpen(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	setupIncrementalProbes(t, exec, out)

	// The control: a full build answers a nested read with every attribute.
	full, _ := buildCatalogOpen(t, exec, true, true)
	want := readsOneAtATime(t, full)
	if want < 20 {
		t.Fatalf("control: the fixture has %d attributes", want)
	}
	if got := readsWhileACursorIsOpen(t, full); got != want {
		t.Fatalf("control: a nested read of a full build saw %d of %d attributes", got, want)
	}
	full.Close()

	// Change the model the way an agent does between two lint runs, then build
	// again: the cache the full build wrote is carried from.
	if err := agreeExec(t, exec, "alter entity FeedbackModule.Feedback add attribute NestedReadProbe: String;"); err != nil {
		t.Fatalf("alter: %v\n%s", err, out.String())
	}
	inc, stats := buildCatalogOpen(t, exec, true, false)
	defer inc.Close()
	if !stats.Incremental {
		t.Fatalf("control: the build was full (%s), so this proves nothing about the incremental path", stats.Reason)
	}
	want = readsOneAtATime(t, inc)
	if got := readsWhileACursorIsOpen(t, inc); got != want {
		t.Errorf("a read made while a cursor is open saw %d of %d attributes after an incremental build", got, want)
	}
}
