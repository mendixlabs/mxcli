// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#2200 (second half): once the .mpr changed — after any exec that
// writes — the cached catalog was invalid, and a catalog query rebuilt it in
// FAST mode in memory. CATALOG.ACTIVITIES (a full-mode table) then answered
// zero rows, with only a progress-line warning to say why. A query now builds
// the catalog its tables need, incrementally, and the cache stays current.
func TestCatalogQuery_RebuildsTheModeItsTablesNeed(t *testing.T) {
	exec := typeCheckFixture(t)
	run(t, exec, `CREATE MICROFLOW MyFirstModule.ACT_Log () BEGIN LOG INFO NODE 'N' 'hello'; END;`)
	run(t, exec, "REFRESH CATALOG FULL FORCE")
	ctx := exec.newExecContext(context.Background())
	cachePath := getCachePath(ctx)
	want := cachedRowCount(t, cachePath, "activities")
	if want == 0 {
		t.Fatal("fixture: the full catalog has no activities, so their loss would prove nothing")
	}

	// A write changes the .mpr: the cache is stale.
	touchProject(t, exec)

	var out bytes.Buffer
	exec.output = &out
	run(t, exec, "SELECT count(*) AS n FROM CATALOG.ACTIVITIES")
	if strings.Contains(out.String(), "requires refresh catalog full") {
		t.Errorf("the query warned instead of building what it needs:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "| "+strconv.Itoa(want)+" ") {
		t.Errorf("CATALOG.ACTIVITIES count is not %d after the project changed:\n%s", want, out.String())
	}
	if valid, reason := isCacheValid(exec.newExecContext(context.Background()), cachePath, "full"); !valid {
		t.Errorf("the rebuilt catalog was not saved as the current full cache: %s", reason)
	}

	// The next mxcli process: no catalog in memory, the cache stale again.
	// This is where the old path built FAST and CATALOG.ACTIVITIES was empty.
	touchProject(t, exec)
	run(t, exec, "CONNECT LOCAL '"+visitor.QuoteString(ctx.MprPath)+"'")
	out.Reset()
	run(t, exec, "SELECT count(*) AS n FROM CATALOG.ACTIVITIES")
	if !strings.Contains(out.String(), "| "+strconv.Itoa(want)+" ") {
		t.Errorf("a new session's CATALOG.ACTIVITIES count is not %d:\n%s", want, out.String())
	}

	// Control: a fast table still builds only fast.
	if got := requiredCatalogMode([]string{"entities"}); got != "fast" {
		t.Errorf("requiredCatalogMode(entities) = %q, want fast", got)
	}
	if got := requiredCatalogMode([]string{"entities", "source"}); got != "source" {
		t.Errorf("requiredCatalogMode(entities, source) = %q, want source", got)
	}
}
