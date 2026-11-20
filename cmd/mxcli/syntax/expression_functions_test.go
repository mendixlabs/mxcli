// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/exprcheck"
)

// mendixlabs/mxcli#1708 (ako/mxcli#1866): "the expression reference has no
// entries for isNew, the addDays/addMonths family or trimToDays/trimToMonths".
// `mxcli syntax expression` must reach a page that names them, with types.
func TestExpressionReferenceNamesReportedFunctions(t *testing.T) {
	m := Lookup([]string{"expression"})
	var text strings.Builder
	for _, f := range m.Features {
		text.WriteString(f.Syntax)
	}
	for _, want := range []string{
		"isNew(Any) -> Boolean",
		"addDays(DateTime, Integer) -> DateTime",
		"addMonths(DateTime, Integer) -> DateTime",
		"trimToDays(DateTime) -> DateTime",
		"trimToMonths(DateTime) -> DateTime",
	} {
		if !strings.Contains(text.String(), want) {
			t.Errorf("`mxcli syntax expression` (topics %v) does not list %q", paths(m.Features), want)
		}
	}
}

// Every function the type checker accepts is in the reference, and nothing it
// does not accept: the page is rendered from exprcheck's table, so this guards
// the rendering, not a hand-kept list.
func TestExpressionFunctionReferenceMatchesChecker(t *testing.T) {
	f := ByPath("microflow.expressions.functions")
	if f == nil {
		t.Fatal("no microflow.expressions.functions topic")
	}
	fns := exprcheck.Functions()
	if len(fns) < 80 {
		t.Fatalf("exprcheck.Functions() returned %d rows — expected the full built-in table", len(fns))
	}
	for _, fn := range fns {
		if !strings.Contains(f.Syntax, "  "+fn.Name+"(") {
			t.Errorf("checker accepts %s() but the reference does not list it", fn.Name)
		}
	}
	// Rows nobody has built against mxbuild are CE0117 when measured; listing
	// them as functions would send readers straight into a build failure.
	if strings.Contains(f.Syntax, "  dayOfYear(") || strings.Contains(f.Syntax, "  hour(") {
		t.Error("reference lists an unverified, build-rejected function (dayOfYear/hour)")
	}
}
