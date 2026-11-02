// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ADR-0013 moved call rest service's dialog settings from clauses into one
// property list; the clauses stay as the alias MDL-DEPR720. A respelling must
// not change what is stored, so this execs both forms of every body and every
// result handling into the Studio Pro-authored PedApp fixture and compares the
// stored Microflows$RestCallAction field by field, ignoring only $ID. It then
// describes the new-form activity, execs that text under a third name, and
// requires the same activity again: describe -> exec is the identity.
//
// The mapping cases use the fixture's own FeedbackModule.EXM_PostFeedback and
// IMM_PostResponse, so a mapping reference resolves as it does in a real app.
func TestRestCallSettingsList_StoresTheSameActivityAsTheClauses(t *testing.T) {
	exec, out := openPedAppFixture(t)
	if err := afRun(t, exec, "create module RestEq;\n"+
		"create persistent entity RestEq.Download extends System.FileDocument ();\n"); err != nil {
		t.Fatalf("setup: %v", err)
	}

	const params = "($T: String, $U: String, $Doc: RestEq.Download, $Fb: FeedbackModule.Feedback)"
	cases := []struct{ name, old, canon string }{
		{"HeadersTimeout",
			"$R = call rest service get 'https://x.org/{1}' with ({1} = $T) header 'Accept' = 'text/html' header XKey = $U timeout 300 returns String",
			"$R = call rest service get 'https://x.org/{1}' with ({1} = $T) (Headers: ('Accept': 'text/html', 'XKey': $U), Timeout: 300) returns String"},
		{"AuthTemplate",
			"$R = call rest service post 'https://x.org' auth basic $U password $T body '{{\"a\": \"{1}\"}' with ({1} = $T) returns response",
			"$R = call rest service post 'https://x.org' (Authentication: basic (Username: $U, Password: $T), Body: template '{{\"a\": \"{1}\"}' with ({1} = $T)) returns response"},
		{"TemplateNoParams",
			"call rest service post 'https://x.org' body 'ping' returns nothing",
			"call rest service post 'https://x.org' (Body: template 'ping') returns nothing"},
		{"ExpressionBody",
			"call rest service put 'https://x.org' body $T returns nothing",
			"call rest service put 'https://x.org' (Body: $T) returns nothing"},
		{"BinaryFileDocument",
			"$R = call rest service post 'https://x.org' body binary $Doc/Contents returns RestEq.Download",
			"$R = call rest service post 'https://x.org' (Body: binary $Doc/Contents) returns RestEq.Download"},
		{"MappingBodyMappingResult",
			"$R = call rest service post 'https://x.org' body mapping FeedbackModule.EXM_PostFeedback from $Fb returns mapping FeedbackModule.IMM_PostResponse as FeedbackModule.ResponseHelper",
			"$R = call rest service post 'https://x.org' (Body: mapping FeedbackModule.EXM_PostFeedback from $Fb) returns mapping FeedbackModule.IMM_PostResponse as FeedbackModule.ResponseHelper"},
		{"ListResultOnError",
			"$R = call rest service delete 'https://x.org' timeout 5 returns mapping FeedbackModule.IMM_PostResponse as list of FeedbackModule.ResponseHelper on error continue",
			"$R = call rest service delete 'https://x.org' (Timeout: 5) returns mapping FeedbackModule.IMM_PostResponse as list of FeedbackModule.ResponseHelper on error continue"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			flow := func(name, stmt string) string {
				return fmt.Sprintf("create microflow RestEq.%s %s begin %s; end;\n", name, params, stmt)
			}
			if err := afRun(t, exec, flow("Old"+c.name, c.old)); err != nil {
				t.Fatalf("exec clause form: %v", err)
			}
			if err := afRun(t, exec, flow("New"+c.name, c.canon)); err != nil {
				t.Fatalf("exec settings list: %v", err)
			}
			old := storedRestCallAction(t, exec, "Old"+c.name)
			canon := storedRestCallAction(t, exec, "New"+c.name)
			if o, n := canonicalBSON(t, old), canonicalBSON(t, canon); o != n {
				t.Fatalf("clause form and settings list stored different activities:\n old: %s\n new: %s", o, n)
			}

			// describe -> exec -> the same activity.
			out.Reset()
			if err := afRun(t, exec, "describe microflow RestEq.New"+c.name+";"); err != nil {
				t.Fatalf("describe: %v", err)
			}
			described := out.String()
			if strings.Contains(described, "\n    header ") || strings.Contains(described, "\n    timeout ") ||
				strings.Contains(described, "\n    body ") || strings.Contains(described, "\n    auth ") {
				t.Errorf("describe printed the clause form:\n%s", described)
			}
			again := strings.Replace(described, "RestEq.New"+c.name, "RestEq.RT"+c.name, 1)
			if err := afRun(t, exec, again); err != nil {
				t.Fatalf("exec of describe output: %v\n%s", err, again)
			}
			rt := storedRestCallAction(t, exec, "RT"+c.name)
			if n, r := canonicalBSON(t, canon), canonicalBSON(t, rt); n != r {
				t.Fatalf("describe -> exec changed the activity:\n describe: %s\n before: %s\n after:  %s", described, n, r)
			}
		})
	}
}

// storedRestCallAction is the Microflows$RestCallAction stored in RestEq.<name>.
func storedRestCallAction(t *testing.T, exec *Executor, name string) bson.D {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	h, err := getHierarchy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	all, err := ctx.Backend.ListMicroflows()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range all {
		if m.Name != name || h.GetModuleName(h.FindModuleID(m.ContainerID)) != "RestEq" {
			continue
		}
		raw, err := ctx.Backend.GetRawUnitBytes(m.ID)
		if err != nil {
			t.Fatal(err)
		}
		var d bson.D
		if err := bson.Unmarshal(raw, &d); err != nil {
			t.Fatal(err)
		}
		if a := findBSONType(d, "Microflows$RestCallAction"); a != nil {
			return a
		}
		t.Fatalf("RestEq.%s stores no Microflows$RestCallAction", name)
	}
	t.Fatalf("RestEq.%s not found", name)
	return nil
}

// findBSONType is the first document of $Type typ in v, depth first.
func findBSONType(v any, typ string) bson.D {
	switch x := v.(type) {
	case bson.D:
		if afGet(x, "$Type") == typ {
			return x
		}
		for _, e := range x {
			if d := findBSONType(e.Value, typ); d != nil {
				return d
			}
		}
	case bson.A:
		for _, e := range x {
			if d := findBSONType(e, typ); d != nil {
				return d
			}
		}
	}
	return nil
}

// canonicalBSON renders d as extended JSON without its $ID fields, which are
// minted per write and carry no meaning of the activity.
func canonicalBSON(t *testing.T, d bson.D) string {
	t.Helper()
	b, err := bson.MarshalExtJSON(restEqWithoutIDs(d), true, false)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func restEqWithoutIDs(v any) any {
	switch x := v.(type) {
	case bson.D:
		out := bson.D{}
		for _, e := range x {
			if e.Key == "$ID" {
				continue
			}
			out = append(out, bson.E{Key: e.Key, Value: restEqWithoutIDs(e.Value)})
		}
		return out
	case bson.A:
		out := bson.A{}
		for _, e := range x {
			out = append(out, restEqWithoutIDs(e))
		}
		return out
	}
	return v
}
