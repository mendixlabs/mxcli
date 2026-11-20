// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#2329 (mendixlabs/mxcli#1872): `drop microflow` + `create
// microflow` of a flow Studio Pro authored lost its parameters' documentation.
// The carry for it (unstatedFlowDefaults) puts the stored value back on the
// element of the same $ID, so it depends on TransplantIDs pairing the
// parameter. It did not: Studio Pro stores a flow's objects in the order they
// were drawn, mxcli writes the parameters and the start event first, and the
// alignment anchored on the longest common run of activities — leaving the
// parameter and the start event in a gap with nothing opposite. A flow mxcli
// had created was written in mxcli's order, so it kept its documentation; one
// from Studio Pro did not, whatever its content.

func flowObj(id byte, typ string, props ...bson.E) bson.D {
	return append(bson.D{{Key: "$ID", Value: id16(id)}, {Key: "$Type", Value: typ}}, props...)
}

func activity(id byte, action string) bson.D {
	return flowObj(id, "Microflows$ActionActivity", bson.E{Key: "Documentation", Value: ""},
		bson.E{Key: "Action", Value: flowObj(id+0x80, action, bson.E{Key: "ErrorHandlingType", Value: "Rollback"})})
}

func parameter(id byte, doc string) bson.D {
	return flowObj(id, "Microflows$MicroflowParameter", bson.E{Key: "Documentation", Value: doc},
		bson.E{Key: "Name", Value: "AccountPasswordData"},
		bson.E{Key: "VariableType", Value: flowObj(id+0x80, "DataTypes$ObjectType",
			bson.E{Key: "Entity", Value: "Administration.AccountPasswordData"})})
}

func flowWithObjects(objs ...bson.D) []byte {
	list := bson.A{int32(3)}
	for _, o := range objs {
		list = append(list, o)
	}
	b, _ := bson.Marshal(bson.D{
		{Key: "$ID", Value: id16(0xAA)}, {Key: "$Type", Value: "Microflows$Microflow"},
		{Key: "ObjectCollection", Value: bson.D{{Key: "$ID", Value: id16(0xAB)},
			{Key: "$Type", Value: "Microflows$MicroflowObjectCollection"}, {Key: "Objects", Value: list}}},
	})
	return b
}

func parameterOf(t *testing.T, unit []byte) bson.D {
	t.Helper()
	var d bson.D
	if err := bson.Unmarshal(unit, &d); err != nil {
		t.Fatal(err)
	}
	var p bson.D
	walkElements(d, func(e bson.D) {
		if docLookup(e, "$Type") == "Microflows$MicroflowParameter" {
			p = e
		}
	})
	if p == nil {
		t.Fatal("no parameter in the unit")
	}
	return p
}

// Studio Pro's order: the activities first, the parameter and the start event
// last. mxcli's: the parameter and the start event first.
func storedStudioProOrder(doc string) []byte {
	return flowWithObjects(
		activity(0x10, "Microflows$RetrieveAction"),
		activity(0x11, "Microflows$ChangeAction"),
		activity(0x12, "Microflows$ShowMessageAction"),
		parameter(0x13, doc),
		flowObj(0x14, "Microflows$StartEvent"))
}

func writtenMxcliOrder(doc string) []byte {
	return flowWithObjects(
		parameter(0x23, doc),
		flowObj(0x24, "Microflows$StartEvent"),
		activity(0x20, "Microflows$RetrieveAction"),
		activity(0x21, "Microflows$ChangeAction"),
		activity(0x22, "Microflows$ShowMessageAction"))
}

func TestReconcile_ParameterDocSurvivesAReorderedRebuild_Issue2329(t *testing.T) {
	out, _ := Reconcile(writtenMxcliOrder(""), storedStudioProOrder("X DB"))
	p := parameterOf(t, out)
	if got, _ := docLookup(p, "Documentation").(string); got != "X DB" {
		t.Errorf("parameter documentation = %q, want the stored %q", got, "X DB")
	}
	if id, _ := docLookup(p, "$ID").(bson.Binary); string(id.Data) != string(id16(0x13).Data) {
		t.Errorf("parameter $ID was not carried from the stored one")
	}
}

// The start event is the other element the reorder left unpaired; it keeps its
// identity too.
func TestTransplant_StartEventPairsAcrossAReorder_Issue2329(t *testing.T) {
	out := TransplantIDs(writtenMxcliOrder(""), storedStudioProOrder(""))
	var d bson.D
	_ = bson.Unmarshal(out, &d)
	found := false
	walkElements(d, func(e bson.D) {
		if docLookup(e, "$Type") == "Microflows$StartEvent" {
			id, _ := docLookup(e, "$ID").(bson.Binary)
			found = string(id.Data) == string(id16(0x14).Data)
		}
	})
	if !found {
		t.Error("start event did not keep the stored $ID")
	}
}

// CONTROL: a documentation the write states is not replaced.
func TestReconcile_WrittenParameterDocLands_Issue2329(t *testing.T) {
	out, _ := Reconcile(writtenMxcliOrder("new"), storedStudioProOrder("X DB"))
	if got, _ := docLookup(parameterOf(t, out), "Documentation").(string); got != "new" {
		t.Errorf("parameter documentation = %q, want the written %q", got, "new")
	}
}

// CONTROL: an entry whose key occurs twice among the leftovers is ambiguous
// and pairs with nothing, rather than with a guess.
func TestTransplant_AmbiguousLeftoversStayUnpaired_Issue2329(t *testing.T) {
	stored := flowWithObjects(
		activity(0x10, "Microflows$RetrieveAction"),
		activity(0x11, "Microflows$ChangeAction"),
		activity(0x12, "Microflows$ShowMessageAction"),
		flowObj(0x13, "Microflows$EndEvent"),
		flowObj(0x14, "Microflows$EndEvent"))
	written := flowWithObjects(
		flowObj(0x23, "Microflows$EndEvent"),
		flowObj(0x24, "Microflows$EndEvent"),
		activity(0x20, "Microflows$RetrieveAction"),
		activity(0x21, "Microflows$ChangeAction"),
		activity(0x22, "Microflows$ShowMessageAction"))
	var d bson.D
	_ = bson.Unmarshal(TransplantIDs(written, stored), &d)
	walkElements(d, func(e bson.D) {
		if docLookup(e, "$Type") == "Microflows$EndEvent" {
			id, _ := docLookup(e, "$ID").(bson.Binary)
			if s := string(id.Data); s == string(id16(0x13).Data) || s == string(id16(0x14).Data) {
				t.Errorf("an end event took a stored $ID by guess")
			}
		}
	})
}

// The same reorder with the show messages' translations (#2329's second
// symptom). The translation carry runs before the transplant and pairs texts by
// place, by owning element or by source string. Reordered, the places differ;
// an activity has no name; and two messages with one default text but
// different translations make the source ambiguous, so nothing was carried and
// the rebuild kept only the language MDL writes.
func message(id byte, place string, texts ...[2]string) bson.D {
	items := bson.A{int32(3)}
	for i, l := range texts {
		items = append(items, flowObj(id+0x40+byte(i), "Texts$Translation",
			bson.E{Key: "LanguageCode", Value: l[0]}, bson.E{Key: "Text", Value: l[1]}))
	}
	return flowObj(id, "Microflows$ActionActivity", bson.E{Key: "Documentation", Value: ""},
		bson.E{Key: "RelativeMiddlePoint", Value: place},
		bson.E{Key: "Action", Value: flowObj(id+0x80, "Microflows$ShowMessageAction",
			bson.E{Key: "Template", Value: flowObj(id+0x20, "Microflows$TextTemplate",
				bson.E{Key: "Text", Value: flowObj(id+0x30, "Texts$Text", bson.E{Key: "Items", Value: items})})})})
}

func messageLanguages(t *testing.T, unit []byte) []map[string]string {
	t.Helper()
	var d bson.D
	if err := bson.Unmarshal(unit, &d); err != nil {
		t.Fatal(err)
	}
	var out []map[string]string
	walkElements(d, func(e bson.D) {
		if docLookup(e, "$Type") == "Texts$Text" {
			out = append(out, textTranslations(e))
		}
	})
	return out
}

func TestReconcile_MessageTranslationsSurviveAReorderedRebuild_Issue2329(t *testing.T) {
	stored := flowWithObjects(
		message(0x01, "100;100", [2]string{"en_US", "Done."}, [2]string{"nl_NL", "Klaar."}, [2]string{"en_GB", "Done, mate."}),
		message(0x02, "300;100", [2]string{"en_US", "Done."}, [2]string{"nl_NL", "Gereed."}),
		parameter(0x13, ""),
		flowObj(0x14, "Microflows$StartEvent"))
	written := flowWithObjects(
		parameter(0x23, ""),
		flowObj(0x24, "Microflows$StartEvent"),
		message(0x05, "100;100", [2]string{"en_US", "Done."}),
		message(0x06, "300;100", [2]string{"en_US", "Done."}))
	out, _ := Reconcile(written, stored)
	got := messageLanguages(t, out)
	if len(got) != 2 {
		t.Fatalf("texts = %v, want two", got)
	}
	if got[0]["nl_NL"] != "Klaar." || got[0]["en_GB"] != "Done, mate." {
		t.Errorf("first message = %v, want its stored nl_NL and en_GB back", got[0])
	}
	if got[1]["nl_NL"] != "Gereed." || got[1]["en_GB"] != "" {
		t.Errorf("second message = %v, want its own stored nl_NL and no en_GB", got[1])
	}
}

// CONTROL: a language the write states is the write's.
func TestReconcile_WrittenLanguageLandsAfterReorder_Issue2329(t *testing.T) {
	stored := flowWithObjects(
		message(0x01, "100;100", [2]string{"en_US", "Done."}, [2]string{"nl_NL", "Klaar."}),
		activity(0x10, "Microflows$RetrieveAction"),
		parameter(0x13, ""))
	written := flowWithObjects(
		parameter(0x23, ""),
		message(0x05, "100;100", [2]string{"en_US", "Finished."}),
		activity(0x20, "Microflows$RetrieveAction"))
	out, _ := Reconcile(written, stored)
	got := messageLanguages(t, out)
	if len(got) != 1 || got[0]["en_US"] != "Finished." || got[0]["nl_NL"] != "Klaar." {
		t.Errorf("message = %v, want the written en_US and the stored nl_NL", got)
	}
}

// The crossing the place breaks: Studio Pro stored the two messages in one
// order, the rebuild writes them in the other. Without the place, the
// alignment paired each with the other, and the other's translation was
// carried onto it.
func TestReconcile_CrossedMessagesPairByPlace_Issue2329(t *testing.T) {
	stored := flowWithObjects(
		message(0x01, "100;100", [2]string{"en_US", "No match."}, [2]string{"nl_NL", "Geen match."}),
		activity(0x10, "Microflows$RetrieveAction"),
		message(0x02, "300;100", [2]string{"en_US", "Updated."}, [2]string{"nl_NL", "Bijgewerkt."}))
	written := flowWithObjects(
		message(0x06, "300;100", [2]string{"en_US", "Updated!"}),
		activity(0x20, "Microflows$RetrieveAction"),
		message(0x05, "100;100", [2]string{"en_US", "No match."}))
	out, _ := Reconcile(written, stored)
	got := messageLanguages(t, out)
	if len(got) != 2 || got[0]["nl_NL"] != "Bijgewerkt." || got[1]["nl_NL"] != "Geen match." {
		t.Errorf("messages = %v, want each its own stored nl_NL", got)
	}
}

// CONTROL: a text whose pairing is unconfirmed — moved, and its source
// changed — takes nothing rather than a guess.
func TestReconcile_UnconfirmedPairingCarriesNothing_Issue2329(t *testing.T) {
	stored := flowWithObjects(
		message(0x01, "100;100", [2]string{"en_US", "Done."}, [2]string{"nl_NL", "Klaar."}),
		parameter(0x13, ""))
	written := flowWithObjects(
		parameter(0x23, ""),
		message(0x05, "500;500", [2]string{"en_US", "Finished."}))
	out, _ := Reconcile(written, stored)
	if got := messageLanguages(t, out); len(got) != 1 || len(got[0]) != 1 {
		t.Errorf("message = %v, want only the written en_US", got)
	}
}
