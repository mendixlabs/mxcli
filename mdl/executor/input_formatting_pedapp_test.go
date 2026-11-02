// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#968 (mendixlabs/mxcli#1263): a date picker's DateFormat /
// CustomDateFormat — and a text box's DecimalPrecision / GroupDigits — passed
// check, were written as the defaults, and were never described. Run end to
// end on PedApp: exec, read the stored unit, describe, re-execute the
// description, alter.

const bookingScript = `create non-persistent entity MyFirstModule.Booking968 (
  Moment: DateTime,
  Amount: Decimal
);
create page MyFirstModule.Booking968_Edit (
  Title: 'Booking',
  Layout: Atlas_Core.Atlas_Default,
  Params: ( $Booking: MyFirstModule.Booking968 )
) {
  dataview dv (DataSource: $Booking) {
    datepicker dpDefault (Label: 'Default', Attribute: Moment)
    datepicker dpTime (Label: 'Time', Attribute: Moment, DateFormat: Time)
    datepicker dpDateTime (Label: 'DateTime', Attribute: Moment, DateFormat: DateTime)
    datepicker dpCustom (Label: 'Custom', Attribute: Moment, DateFormat: Custom, CustomDateFormat: 'dd-MM-yyyy H:mm')
    datepicker dpStale (Label: 'Stale', Attribute: Moment, DateFormat: DateTime, CustomDateFormat: 'dd/MM/yyyy HH:mm')
    textbox tbAmount (Label: 'Amount', Attribute: Amount, DecimalPrecision: 4, GroupDigits: true)
  }
};`

// storedFormatting returns each named widget's FormattingInfo fields from the
// stored page unit.
func storedFormatting(t *testing.T, exec *Executor, page string) map[string]map[string]any {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	unit, err := ctx.Backend.GetRawUnitByName("page", page)
	if err != nil || unit == nil {
		t.Fatalf("raw unit of %s: %v", page, err)
	}
	var doc bson.D
	if err := bson.Unmarshal(unit.Contents, &doc); err != nil {
		t.Fatalf("unmarshal %s: %v", page, err)
	}
	out := map[string]map[string]any{}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case bson.D:
			var name string
			var fi bson.D
			for _, e := range x {
				switch e.Key {
				case "Name":
					name, _ = e.Value.(string)
				case "FormattingInfo":
					fi, _ = e.Value.(bson.D)
				}
			}
			if name != "" && fi != nil {
				m := map[string]any{}
				for _, e := range fi {
					m[e.Key] = e.Value
				}
				out[name] = m
			}
			for _, e := range x {
				walk(e.Value)
			}
		case bson.A:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(doc)
	return out
}

func TestInputFormatting_WrittenDescribedAndRoundTripped(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	if err := afRun(t, exec, bookingScript); err != nil {
		t.Fatalf("exec: %v\n%s", err, out.String())
	}
	const page = "MyFirstModule.Booking968_Edit"

	got := storedFormatting(t, exec, page)
	for name, want := range map[string]map[string]any{
		"dpDefault":  {"DateFormat": "Date", "CustomDateFormat": ""},
		"dpTime":     {"DateFormat": "Time", "CustomDateFormat": ""},
		"dpDateTime": {"DateFormat": "DateTime", "CustomDateFormat": ""},
		"dpCustom":   {"DateFormat": "Custom", "CustomDateFormat": "dd-MM-yyyy H:mm"},
		"dpStale":    {"DateFormat": "DateTime", "CustomDateFormat": "dd/MM/yyyy HH:mm"},
		// int64: the width Studio Pro stores DecimalPrecision at, and the
		// codec's since ako/mxcli#1264 (the comparison is on an interface, so
		// it pins the width as well as the value).
		"tbAmount": {"DecimalPrecision": int64(4), "GroupDigits": true},
	} {
		for k, v := range want {
			if got[name][k] != v {
				t.Errorf("%s: stored FormattingInfo.%s = %#v, want %#v", name, k, got[name][k], v)
			}
		}
	}

	described := describeOn(t, exec, out, page)
	for _, want := range []string{
		"DateFormat: Time",
		"DateFormat: DateTime",
		"DateFormat: Custom",
		"CustomDateFormat: 'dd-MM-yyyy H:mm'",
		"CustomDateFormat: 'dd/MM/yyyy HH:mm'",
		"DecimalPrecision: 4",
		"GroupDigits: true",
	} {
		if !strings.Contains(described, want) {
			t.Errorf("describe should print %q:\n%s", want, described)
		}
	}
	// The default is not printed: an unformatted picker describes as before.
	if strings.Contains(described, "DateFormat: Date,") || strings.Contains(described, "DateFormat: Date\n") {
		t.Errorf("describe printed the default DateFormat:\n%s", described)
	}

	// GetPut: executing the description writes nothing.
	before := projectFiles(t, dir)
	out.Reset()
	if err := afRun(t, exec, described); err != nil {
		t.Fatalf("re-exec describe output: %v\n%s", err, out.String())
	}
	if changed := diffProjectFiles(before, projectFiles(t, dir)); len(changed) != 0 {
		t.Errorf("executing the describe output wrote %v", changed)
	}

	// Control: an edited description IS a write, and lands.
	edited := strings.Replace(described, "DateFormat: Time", "DateFormat: DateTime", 1)
	if edited == described {
		t.Fatal("control edit did not apply")
	}
	if err := afRun(t, exec, edited); err != nil {
		t.Fatalf("exec edited describe output: %v", err)
	}
	if df := storedFormatting(t, exec, page)["dpTime"]["DateFormat"]; df != "DateTime" {
		t.Errorf("edited dpTime DateFormat = %v, want DateTime", df)
	}
}

func TestInputFormatting_AlterPageSet(t *testing.T) {
	exec, out := openPedAppFixture(t)
	if err := afRun(t, exec, bookingScript); err != nil {
		t.Fatalf("exec: %v\n%s", err, out.String())
	}
	const page = "MyFirstModule.Booking968_Edit"

	if err := afRun(t, exec, `alter page `+page+` { set (DateFormat: Custom, CustomDateFormat: 'HH:mm') on dpDefault; set (DecimalPrecision: 0, GroupDigits: false) on tbAmount; set DateFormat = Time on dpDateTime; };`); err != nil {
		t.Fatalf("alter page set: %v", err)
	}
	got := storedFormatting(t, exec, page)
	if got["dpDefault"]["DateFormat"] != "Custom" || got["dpDefault"]["CustomDateFormat"] != "HH:mm" {
		t.Errorf("dpDefault = %v, want Custom / HH:mm", got["dpDefault"])
	}
	if got["dpDateTime"]["DateFormat"] != "Time" {
		t.Errorf("dpDateTime = %v, want Time", got["dpDateTime"])
	}
	// The page was created by the codec, which writes Integer properties as
	// int64 like Studio Pro (ako/mxcli#1264); ALTER keeps the stored width.
	if got["tbAmount"]["DecimalPrecision"] != int64(0) || got["tbAmount"]["GroupDigits"] != false {
		t.Errorf("tbAmount = %v, want precision 0 (same int64 width), no grouping", got["tbAmount"])
	}

	for _, c := range []struct{ stmt, want string }{
		{`set DateFormat = Weekly on dpTime`, "DateFormat is one of Date, Time, DateTime, Custom"},
		// mxbuild CE0493 "Date format is custom but no format string is specified."
		{`set DateFormat = Custom on dpTime`, "CE0493"},
		{`set DecimalPrecision = 3 on dpTime`, "no DecimalPrecision property"},
		{`set DateFormat = Time on tbAmount`, "no DateFormat property"},
	} {
		err := afRun(t, exec, `alter page `+page+` { `+c.stmt+`; };`)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want an error containing %q, got %v", c.stmt, c.want, err)
		}
	}
	if got := storedFormatting(t, exec, page)["dpTime"]["DateFormat"]; got != "Time" {
		t.Errorf("a refused set changed dpTime to %v", got)
	}
}
