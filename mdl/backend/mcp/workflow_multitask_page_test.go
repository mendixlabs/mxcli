// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"reflect"
	"testing"
)

// Studio Pro 11.14's multi-user task constructor takes the page as a `taskPage`
// element and rejects a bare `pageReference` (measured live). The shape is read
// off the live constructor schema, once.
func TestMultiUserTaskConstructorPageProbe(t *testing.T) {
	calls := 0
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		if name == "ped_get_schema" {
			calls++
			return `{"kind":"constructor","schema":"constructor type 'Workflows$MultiUserTaskActivity' = {\n name: string;\n outcomes: string[];\n taskPage: Element<'Workflows$PageReference'>;\n}"}`, false
		}
		return "SUCCESS", false
	})
	b := &Backend{client: f.connectClient(t)}
	if !b.multiUserTaskConstructorTakesTaskPage() || !b.multiUserTaskConstructorTakesTaskPage() {
		t.Fatal("an 11.14 constructor schema must select the taskPage shape")
	}
	if calls != 1 {
		t.Errorf("ped_get_schema called %d times, want 1 (cached)", calls)
	}
	old := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		return `{"schemas":[{"elementType":"Workflows$MultiUserTaskActivity","schema":{"properties":{"pageReference":{}}}}]}`, false
	})
	if (&Backend{client: old.connectClient(t)}).multiUserTaskConstructorTakesTaskPage() {
		t.Error("a constructor without taskPage must keep pageReference")
	}
}

func TestAdaptMultiUserTaskPages(t *testing.T) {
	nested := map[string]any{"$Type": "Workflows$MultiUserTaskActivity", "name": "inner", "pageReference": "M.Inner"}
	content := map[string]any{
		"flow": map[string]any{"$Type": "Workflows$Flow", "activities": []any{
			map[string]any{"$Type": "Workflows$MultiUserTaskActivity", "name": "top", "pageReference": "M.Top"},
			map[string]any{"$Type": "Workflows$SingleUserTaskActivity", "name": "single", "taskPage": map[string]any{"$Type": "Workflows$PageReference", "page": "M.Single"}},
			map[string]any{"$Type": "Workflows$CallMicroflowActivity", "outcomes": []any{
				map[string]any{"$Type": "Workflows$BooleanConditionOutcome", "flow": map[string]any{"activities": []any{nested}}},
			}},
		}},
	}
	adaptMultiUserTaskPages(content)
	top := content["flow"].(map[string]any)["activities"].([]any)[0].(map[string]any)
	for name, m := range map[string]map[string]any{"top": top, "inner": nested} {
		if _, ok := m["pageReference"]; ok {
			t.Errorf("%s still sends pageReference", name)
		}
	}
	if !reflect.DeepEqual(top["taskPage"], map[string]any{"$Type": "Workflows$PageReference", "page": "M.Top"}) {
		t.Errorf("top taskPage = %v", top["taskPage"])
	}
	if !reflect.DeepEqual(nested["taskPage"], map[string]any{"$Type": "Workflows$PageReference", "page": "M.Inner"}) {
		t.Errorf("nested taskPage = %v", nested["taskPage"])
	}
}
