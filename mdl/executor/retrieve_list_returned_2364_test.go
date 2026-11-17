// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ako/mxcli#2364: under `mdl 1;` `retrieve … limit 1` binds a list of one, and
// `return $Item` from a microflow that returns `Lm.Item` passed check ("Expression
// types OK") while mx check reported CE0117 at the End event (measured by the
// reporter on 11.16.1).

func returnFlow(rng, returns string) string {
	return `mdl 1;
create microflow Lm.GetItem ($Name: String) returns ` + returns + ` begin
  retrieve $Item from Lm.Item where Name = $Name ` + rng + `;
  return $Item;
end;`
}

func TestRetrieveListReturnedAsObject_Issue2364(t *testing.T) {
	v := findViolation(violationsForMicroflowSource(t, returnFlow("limit 1", "Lm.Item")), retrieveListAsObjectRule)
	if v == nil {
		t.Fatal("returning a `limit 1` list from a microflow that returns Lm.Item is not reported")
	}
	for _, want := range []string{"$Item", "list", "first", "CE0117"} {
		if !strings.Contains(v.Message+v.Suggestion, want) {
			t.Errorf("violation lacks %q: %s / %s", want, v.Message, v.Suggestion)
		}
	}
}

// CONTROLS: the object range returned as the object, and the list returned
// from a microflow that returns a list, are what mxbuild accepts.
func TestRetrieveListReturnedAsObject_Controls_Issue2364(t *testing.T) {
	for name, src := range map[string]string{
		"first as object": returnFlow("first", "Lm.Item"),
		"list as list":    returnFlow("limit 1", "list of Lm.Item"),
	} {
		if v := findViolation(violationsForMicroflowSource(t, src), retrieveListAsObjectRule); v != nil {
			t.Errorf("%s: unexpected %s: %s", name, v.RuleID, v.Message)
		}
	}
}
