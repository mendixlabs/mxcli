// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#2106: values describe → exec of FactoryManagement's pages rewrote
// although nothing changed, measured with a value-only diff of each page's
// unit before and after the first exec (Mendix 11.16).

func pageWithElement(el bson.D) []byte {
	b, _ := bson.Marshal(bson.D{{Key: "$ID", Value: id16(0xAA)}, {Key: "$Type", Value: "Forms$Page"}, {Key: "Widgets", Value: bson.A{int32(2), el}}})
	return b
}

func appearance(dynamicClasses string) []byte {
	return pageWithElement(bson.D{{Key: "$ID", Value: id16(0x10)}, {Key: "$Type", Value: "Forms$Appearance"}, {Key: "DynamicClasses", Value: dynamicClasses}})
}

func xpathSource(constraint string) []byte {
	return pageWithElement(bson.D{{Key: "$ID", Value: id16(0x10)}, {Key: "$Type", Value: "Forms$ListViewXPathSource"}, {Key: "XPathConstraint", Value: constraint}})
}

func assertElided(t *testing.T, what string, written, stored []byte) {
	t.Helper()
	if eq, _ := Equal(written, stored); eq {
		t.Fatalf("%s: control: the documents should differ before the carries", what)
	}
	if _, unchanged := Reconcile(written, stored); !unchanged {
		t.Errorf("%s: a round trip that changed nothing was not elided", what)
	}
}

func assertLands(t *testing.T, what string, written, stored []byte) {
	t.Helper()
	if _, unchanged := Reconcile(written, stored); unchanged {
		t.Errorf("%s: a real change was elided", what)
	}
}

// DynamicClasses is an expression: MDL writes it bare, so a stored trailing
// newline (DigitalTwin.TechnicianTicket_Overview, 3 pages) was dropped.
func TestUnexpressed_DynamicClassesWhitespace(t *testing.T) {
	stored := "if $currentObject/Status = M.S.A\nthen 'badge-danger'\nelse 'badge-info'\n\n"
	assertElided(t, "trailing newlines", appearance("if $currentObject/Status = M.S.A\nthen 'badge-danger'\nelse 'badge-info'"), appearance(stored))
	assertLands(t, "another class", appearance("if $currentObject/Status = M.S.A\nthen 'badge-warning'\nelse 'badge-info'"), appearance(stored))
}

// An XPath constraint with a newline between predicates and no space before
// `=` came back reformatted (AgentCommons.TestCase_Overview, Version_Overview).
func TestUnexpressed_XPathWhitespace(t *testing.T) {
	stored := "[AgentCommons.TestCase_Agent= $currentObject]\n[not(IsDraftTestCase)]"
	assertElided(t, "reformatted", xpathSource("[AgentCommons.TestCase_Agent = $currentObject][not(IsDraftTestCase)]"), xpathSource(stored))
	assertLands(t, "a predicate removed", xpathSource("[AgentCommons.TestCase_Agent = $currentObject]"), xpathSource(stored))
	// Whitespace inside a string literal is part of the value.
	assertLands(t, "a literal changed", xpathSource("[Name = 'ab']"), xpathSource("[Name = 'a b']"))
}

func condition(id byte, value string, visible bool) bson.D {
	return bson.D{{Key: "$ID", Value: id16(id)}, {Key: "$Type", Value: "Enumerations$Condition"}, {Key: "AttributeValue", Value: value}, {Key: "EditableVisible", Value: visible}}
}

func visibility(conds ...bson.D) []byte {
	a := bson.A{int32(2)}
	for _, c := range conds {
		a = append(a, c)
	}
	return pageWithElement(bson.D{{Key: "$ID", Value: id16(0x10)}, {Key: "$Type", Value: "Forms$ConditionalVisibilitySettings"},
		{Key: "Attribute", Value: "M.E.Kind"}, {Key: "Conditions", Value: a}})
}

// The writer lists an enumeration condition's values in enumeration order;
// Studio Pro keeps the order they were added in
// (FactoryManagement.ConversationalUI_PopUp_Custom). Each value keeps its flag,
// so only the order differs.
func TestUnexpressed_ConditionOrder(t *testing.T) {
	stored := visibility(condition(0x20, "AWS", false), condition(0x21, "(empty)", false), condition(0x22, "GraphStudio", true))
	// The rebuild takes the stored ids by position, as TransplantIDs pairs a list.
	written := visibility(condition(0x20, "AWS", false), condition(0x21, "GraphStudio", true), condition(0x22, "(empty)", false))
	assertElided(t, "reordered", written, stored)
	// Control: a flag moved to another value is a different condition.
	assertLands(t, "flag moved", visibility(condition(0x20, "AWS", true), condition(0x21, "GraphStudio", false), condition(0x22, "(empty)", false)), stored)
}
