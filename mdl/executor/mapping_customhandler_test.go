// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
)

// Issue #264: custom object handling — a microflow resolves the element's object
// instead of Create/Find. 56 of the 327 mapping documents in the demo apps use
// it, and it was unauthorable: the writers hardcoded CustomHandlerCall to nil.
//
// MDL spells it as a modifier on `find`, which is what it means:
//
//	find M.E by M.Microflow ( Param: parent ) = member { ... }
//
// The four parameter sources and their stored shapes are pinned below. They are
// not distinguished by a kind field in the document — the reader has to recover
// them from the path marker plus LevelOfParent, so a test that only checked the
// microflow name would miss a source being written as the wrong shape.

const customHandlerSetup = `create entity ` + testModule + `.CHRoot ( Name: String(100) );
create entity ` + testModule + `.CHChild ( Code: String(100) );
create association ` + testModule + `.CHChild_CHRoot
  from ` + testModule + `.CHChild to ` + testModule + `.CHRoot
  type Reference;
create json structure ` + testModule + `.JSON_CH
  snippet '{"name": "n", "items": [{"code": "c", "idx": 1}]}';
create microflow ` + testModule + `.CH_Resolve ( $Obj: ` + testModule + `.CHRoot )
returns ` + testModule + `.CHChild
begin
  @start(100, 200)
  @position(400, 200)
  return empty;
end;
/
create microflow ` + testModule + `.CH_ByIndex ( $Index: Integer )
returns ` + testModule + `.CHChild
begin
  @start(100, 200)
  @position(400, 200)
  return empty;
end;
/
`

func newCustomHandlerEnv(t *testing.T) *testEnv {
	t.Helper()
	// Named explicitly rather than via setupTestEnv, because this construct is
	// the reason the engine mattered: the legacy serializers wrote
	// CustomHandlerCall as nil, so that backend refused the statement instead of
	// dropping the microflow silently. That engine is gone
	// (docs/plans/2026-09-14-retire-legacy-engine.md) and the refusal test with
	// it; the naming stays because it says which writer these assertions pin.
	env := setupTestEnvWithBackend(t, func() backend.FullBackend { return modelsdkbackend.New() })
	if err := env.executeMDL(customHandlerSetup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	return env
}

func TestCustomHandlerParameterSources(t *testing.T) {
	env := newCustomHandlerEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`create import mapping ` + testModule + `.IM_CH
  with json structure ` + testModule + `.JSON_CH
{
  create ` + testModule + `.CHRoot {
    Name = name,
    find ` + testModule + `.CHChild_CHRoot/` + testModule + `.CHChild
      by ` + testModule + `.CH_ByIndex ( Index: idx )
      = items {
        Code = code
      }
  }
};`); err != nil {
		t.Fatalf("custom handler rejected (#264): %v", err)
	}

	root := rootElement(t, env, "ImportMappings$ImportMapping", "IM_CH")
	var obj map[string]string
	for _, child := range childElements(root) {
		if strings.HasSuffix(lookupString(child, "$Type"), "ObjectMappingElement") {
			obj = map[string]string{
				"handling": lookupString(child, "ObjectHandling"),
				"backup":   lookupString(child, "ObjectHandlingBackup"),
			}
		}
	}
	if obj == nil {
		t.Fatal("no nested object element")
	}
	if obj["handling"] != "Custom" {
		t.Errorf("ObjectHandling = %q, want Custom — `by` is what makes it custom", obj["handling"])
	}
	// "Custom" is not in the ObjectHandlingBackup enum {Create, Error, Ignore};
	// letting it inherit the handling would write an illegal value (#261).
	if obj["backup"] != "Create" {
		t.Errorf("ObjectHandlingBackup = %q, want Create", obj["backup"])
	}

	// A value-path parameter needs the value it keys on to exist as an
	// attribute-less value element, or mxbuild reports CE0281 — and that
	// element's type must MIRROR the schema, or CE5015. Both were found by
	// running mx check, not by reading the document.
	var idxElem map[string]string
	for _, child := range childElements(root) {
		if !strings.HasSuffix(lookupString(child, "$Type"), "ObjectMappingElement") {
			continue
		}
		for _, gc := range childElements(child) {
			if lookupString(gc, "JsonPath") == "(Object)|items|(Object)|idx" {
				idxElem = map[string]string{
					"attr": lookupString(gc, "Attribute"),
					"type": lookupString(gc, "ElementType"),
				}
			}
		}
	}
	if idxElem == nil {
		t.Fatal("no value element for the handler's parameter — mxbuild reports CE0281")
	}
	if idxElem["attr"] != "" {
		t.Errorf("handler value element has Attribute %q, want none", idxElem["attr"])
	}

	// The whole point: the mapping must round-trip, converter clause and all.
	checkMappingRoundTrip(t, env, fixtureMapping{
		Module: testModule, Name: "IM_CH", Type: "ImportMappings$ImportMapping",
	})
}

// TestCustomHandlerEmptyParameterList: a microflow with no parameters still
// stores ParameterMappings, as the bare typed-array marker. Studio Pro always
// writes it (KrogerAPI.IM_AccessToken is the demo-app case), and dropping it is
// a diff against every stored document that has one.
func TestCustomHandlerEmptyParameterList(t *testing.T) {
	env := newCustomHandlerEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`create microflow ` + testModule + `.CH_NoArgs ()
returns ` + testModule + `.CHRoot
begin
  @start(100, 200)
  @position(400, 200)
  return empty;
end;
/
create import mapping ` + testModule + `.IM_CHNoArgs
  with json structure ` + testModule + `.JSON_CH
{
  find ` + testModule + `.CHRoot by ` + testModule + `.CH_NoArgs() {
    Name = name
  }
};`); err != nil {
		t.Fatalf("parameterless custom handler rejected: %v", err)
	}
	checkMappingRoundTrip(t, env, fixtureMapping{
		Module: testModule, Name: "IM_CHNoArgs", Type: "ImportMappings$ImportMapping",
	})
}

// TestCustomHandlerRefusals: the microflow reference is resolved, not written
// through — mxbuild reports an unresolvable one as CE1613 and the handler is
// silently gone (#259's rule).
func TestCustomHandlerRefusals(t *testing.T) {
	env := newCustomHandlerEnv(t)
	defer env.teardown()

	err := env.executeMDL(`create import mapping ` + testModule + `.IM_CHBad
  with json structure ` + testModule + `.JSON_CH
{
  find ` + testModule + `.CHRoot by ` + testModule + `.NoSuchMicroflow() { Name = name }
};`)
	if err == nil {
		t.Fatal("accepted an unknown handler microflow")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error %q does not say the microflow was not found", err)
	}
}
