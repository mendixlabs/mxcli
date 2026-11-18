// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// CE0243 "The mapping used to return a value of type 'List of X', but now
// returns a value of type 'X'." Measured on MxBuild 11.16.1, one microflow per
// case, `rest call … returns mapping M as …`:
//
//	object-rooted mapping   as Entity          clean
//	object-rooted mapping   as list of Entity  CE0243   <- accepted by check and exec
//	array-rooted mapping    as Entity          clean   (binds one out of a list)
//	array-rooted mapping    as list of Entity  clean
//	mapping rooted below an array, either form clean
//
// So only an object-rooted mapping declared as a list is wrong; binding one
// object out of a list-rooted mapping is a legitimate pattern.
const restMapScript = `mdl 1;
create non-persistent entity Rm.Item (Rid: integer, Name: string(50));
create json structure Rm.JS_Obj sample '{"id": 1, "name": "a"}';
create json structure Rm.JS_List sample '[{"id": 1, "name": "a"}]';
create import mapping Rm.IMM_Obj with json structure Rm.JS_Obj {
  create Rm.Item { Rid = id, Name = name }
};
create import mapping Rm.IMM_List with json structure Rm.JS_List {
  create Rm.Item { Rid = id, Name = name }
};
create import mapping Rm.IMM_Below with json structure Rm.JS_Obj root name {
  create Rm.Item { Rid = id, Name = name }
};
`

func restMapFlow(kind, call string) string {
	return restMapScript + "create " + kind + " Rm.F ($Url: string)\nbegin\n" + call + "\nend;\n"
}

func restCall(mapping, as string) string {
	return "  $R = call rest service get $Url (Timeout: 30) returns mapping " + mapping + " as " + as + " on error rollback;"
}

func TestValidateRestMappingResult(t *testing.T) {
	cases := []struct {
		name, call string
		want       int
	}{
		{"object-rooted mapping as list of", restCall("Rm.IMM_Obj", "list of Rm.Item"), 1},
		{"object-rooted mapping as the entity (control)", restCall("Rm.IMM_Obj", "Rm.Item"), 0},
		{"list-rooted mapping as list of (control)", restCall("Rm.IMM_List", "list of Rm.Item"), 0},
		{"list-rooted mapping as the entity: binds one out of a list (control, measured clean)",
			restCall("Rm.IMM_List", "Rm.Item"), 0},
		{"a mapping with a root path is not judged (control)", restCall("Rm.IMM_Below", "list of Rm.Item"), 0},
		{"a mapping that is neither the script's nor known (control)", restCall("Other.IMM_Elsewhere", "list of Rm.Item"), 0},
		{"inside an if branch", "  if $Url != empty then\n" + restCall("Rm.IMM_Obj", "list of Rm.Item") + "\n  end if;", 1},
	}
	for _, kind := range []string{"microflow", "nanoflow"} {
		for _, c := range cases {
			t.Run(kind+": "+c.name, func(t *testing.T) {
				got := ValidateRestMappingResult(mustBuild(t, restMapFlow(kind, c.call)), "")
				if len(got) != c.want {
					t.Fatalf("violations=%v, want %d", rulesOf(got), c.want)
				}
				if c.want > 0 {
					v := got[0]
					if v.RuleID != restMappingResultRule || !strings.Contains(v.Message, "CE0243") ||
						!strings.Contains(v.Message, "Rm.IMM_Obj") || v.Location.DocumentName != "F" {
						t.Errorf("rule, CE code, mapping or location wrong: %+v", v)
					}
				}
			})
		}
	}
}
