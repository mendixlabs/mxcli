// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

var regexpLines = regexp.MustCompile(`<g class="association"><title>[^<]*</title>(<line [^>]*>)`)

func svgTestModel() ([]*domainmodel.Entity, []*domainmodel.Association) {
	customer := &domainmodel.Entity{Name: "Customer", Persistable: true, Location: model.Point{X: 100, Y: 100}}
	customer.ID = "c"
	customer.Attributes = []*domainmodel.Attribute{{Name: "Name", Type: &domainmodel.StringAttributeType{Length: 200}}}
	order := &domainmodel.Entity{Name: "Order", Persistable: true, Location: model.Point{X: 400, Y: 120}}
	order.ID = "o"
	filter := &domainmodel.Entity{Name: "OrderFilter", Persistable: false, Location: model.Point{X: 100, Y: 300}}
	filter.ID = "f"
	account := &domainmodel.Entity{Name: "Account", Persistable: true, Location: model.Point{X: 400, Y: 300}, GeneralizationRef: "System.User"}
	account.ID = "a"
	// Order_Customer: from Order (owns the reference) to Customer.
	assoc := &domainmodel.Association{Name: "Order_Customer", ParentID: "o", ChildID: "c", Type: domainmodel.AssociationTypeReference}
	tags := &domainmodel.Association{Name: "Order_Tag", ParentID: "o", ChildID: "elsewhere", Type: domainmodel.AssociationTypeReferenceSet}
	return []*domainmodel.Entity{customer, order, filter, account}, []*domainmodel.Association{assoc, tags}
}

// The SVG has to be well-formed: the mod hands it to the surface as is, and a
// broken document draws nothing.
func TestDomainModelSVG_IsWellFormed(t *testing.T) {
	entities, assocs := svgTestModel()
	svg := renderDomainModelSVG("Sales", entities, assocs)
	dec := xml.NewDecoder(strings.NewReader(svg))
	for {
		_, err := dec.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("not well-formed XML: %v\n%s", err, svg)
		}
	}
	if !strings.HasPrefix(svg, "<svg ") || !strings.Contains(svg, `viewBox="`) {
		t.Errorf("want an <svg> root with a viewBox, got %.80s", svg)
	}
}

// Entities sit where Studio Pro put them, with their attributes, and the box
// colour says what kind of entity it is.
func TestDomainModelSVG_DrawsEntitiesAtTheirStoredPositions(t *testing.T) {
	entities, assocs := svgTestModel()
	svg := renderDomainModelSVG("Sales", entities, assocs)
	for _, want := range []string{
		`<g class="entity persistent" transform="translate(100 100)">`,
		`<g class="entity persistent" transform="translate(400 120)">`,
		`<g class="entity nonpersistent" transform="translate(100 300)">`,
		">Customer<", ">Name: String<", ">OrderFilter<",
		">extends System.User<",
	} {
		if !strings.Contains(svg, want) {
			t.Errorf("missing %q in\n%s", want, svg)
		}
	}
}

// An association between two drawn entities is a line with its multiplicities;
// one to an entity of another module has nowhere to go and is left out rather
// than drawn to the origin.
func TestDomainModelSVG_DrawsAssociationsBetweenDrawnEntities(t *testing.T) {
	entities, assocs := svgTestModel()
	svg := renderDomainModelSVG("Sales", entities, assocs)
	if strings.Count(svg, `class="association"`) != 1 {
		t.Errorf("want exactly one association drawn (Order_Customer), got\n%s", svg)
	}
	if !strings.Contains(svg, "Order_Customer") {
		t.Error("the association's name is not in the drawing")
	}
	if strings.Contains(svg, "Order_Tag") {
		t.Error("an association to an entity outside the module was drawn")
	}
}

func TestDomainModelSVG_EscapesNames(t *testing.T) {
	e := &domainmodel.Entity{Name: "A<B>&C", Persistable: true}
	e.ID = "x"
	svg := renderDomainModelSVG("M", []*domainmodel.Entity{e}, nil)
	if strings.Contains(svg, "A<B>") || !strings.Contains(svg, "A&lt;B&gt;&amp;C") {
		t.Errorf("names are not escaped:\n%s", svg)
	}
}

func TestDomainModelSVG_EmptyModuleSaysSo(t *testing.T) {
	svg := renderDomainModelSVG("Empty", nil, nil)
	if !strings.Contains(svg, "no entities") {
		t.Errorf("an empty domain model should say so, got\n%s", svg)
	}
}

// Two associations between the same pair of entities are drawn side by side,
// not on top of each other (MicroflowSelection_Microflow and _Active in a real
// model printed their labels over one another).
func TestDomainModelSVG_SeparatesParallelAssociations(t *testing.T) {
	entities, _ := svgTestModel()
	assocs := []*domainmodel.Association{
		{Name: "Order_Customer", ParentID: "o", ChildID: "c", Type: domainmodel.AssociationTypeReference},
		{Name: "Order_Customer_Billing", ParentID: "o", ChildID: "c", Type: domainmodel.AssociationTypeReference},
	}
	svg := renderDomainModelSVG("Sales", entities, assocs)
	m := regexpLines.FindAllStringSubmatch(svg, -1)
	if len(m) < 2 {
		t.Fatalf("want two association lines, got %d", len(m))
	}
	if m[0][1] == m[1][1] {
		t.Errorf("parallel associations drawn on the same line: %s", m[0][1])
	}
}

var regexpEntity = regexp.MustCompile(`<g class="entity [a-z]+" transform="translate\((-?\d+) (-?\d+)\)"><title>[^<]*</title><rect width="(\d+)" height="(\d+)"`)

// Studio Pro places entities just far enough apart for its own box widths; a
// box estimated wider must not run into its neighbour (seen on a real
// Teamcenter model: names like TeamcenterConfigurationHelper overlapped the
// next entity). Boxes are clamped to the gap, the text clipped.
func TestDomainModelSVG_BoxesDoNotOverlapNeighbours(t *testing.T) {
	long := &domainmodel.Entity{Name: "TeamcenterConfigurationHelperWithAVeryLongName", Persistable: true, Location: model.Point{X: 0, Y: 0}}
	long.ID = "l"
	long.Attributes = []*domainmodel.Attribute{{Name: "EncryptedPasswordForTheConfiguration", Type: &domainmodel.StringAttributeType{}}}
	// Too close for even the largest spread to make room: the clamp must hold.
	right := &domainmodel.Entity{Name: "Next", Persistable: true, Location: model.Point{X: 60, Y: 10}}
	right.ID = "r"
	below := &domainmodel.Entity{Name: "Below", Persistable: true, Location: model.Point{X: 0, Y: 15}}
	below.ID = "b"
	svg := renderDomainModelSVG("M", []*domainmodel.Entity{long, right, below}, nil)

	type box struct{ x, y, w, h int }
	var boxes []box
	for _, m := range regexpEntity.FindAllStringSubmatch(svg, -1) {
		var b box
		fmt.Sscanf(m[1]+" "+m[2]+" "+m[3]+" "+m[4], "%d %d %d %d", &b.x, &b.y, &b.w, &b.h)
		boxes = append(boxes, b)
	}
	if len(boxes) != 3 {
		t.Fatalf("want 3 entity boxes, got %d:\n%s", len(boxes), svg)
	}
	for i, a := range boxes {
		for j, b := range boxes {
			if i < j && a.x < b.x+b.w && b.x < a.x+a.w && a.y < b.y+b.h && b.y < a.y+a.h {
				t.Errorf("boxes %d %+v and %d %+v overlap", i, a, j, b)
			}
		}
	}
	if !strings.Contains(svg, "<clipPath") {
		t.Error("a clamped box's text is not clipped")
	}
	if !strings.Contains(svg, "TeamcenterConfigurationHelperWithAVeryLongName") {
		t.Error("the full name should stay in the tooltip")
	}
}

// Studio Pro draws entities at a fixed width and cuts long names off, a
// common complaint. The diagram keeps Studio Pro's arrangement but spreads it
// apart (up to a cap) so each box is as wide as its longest line: nothing is
// clipped when spreading can make room.
func TestDomainModelSVG_SpreadsTheLayoutSoNamesFit(t *testing.T) {
	long := &domainmodel.Entity{Name: "TeamcenterConfigurationHelper", Persistable: true, Location: model.Point{X: 0, Y: 0}}
	long.ID = "l"
	long.Attributes = []*domainmodel.Attribute{{Name: "EncryptedPassword", Type: &domainmodel.StringAttributeType{}}}
	right := &domainmodel.Entity{Name: "Next", Persistable: true, Location: model.Point{X: 150, Y: 0}}
	right.ID = "r"
	svg := renderDomainModelSVG("M", []*domainmodel.Entity{long, right}, nil)
	if strings.Contains(svg, "<clipPath") {
		t.Errorf("text was clipped although the layout could spread to fit it:\n%s", svg)
	}
	m := regexpEntity.FindAllStringSubmatch(svg, -1)
	if len(m) != 2 || m[0][1] != "0" || m[1][1] == "150" {
		t.Errorf("want the left entity kept at 0 and the right one moved out from 150, got %v", m)
	}
}

// assocLine returns the endpoints of the one association line drawn for name.
func assocLine(t *testing.T, svg, name string) (x1, y1, x2, y2 int) {
	t.Helper()
	re := regexp.MustCompile(`<g class="association"><title>` + regexp.QuoteMeta(name) +
		`</title><line x1="(-?\d+)" y1="(-?\d+)" x2="(-?\d+)" y2="(-?\d+)"`)
	m := re.FindStringSubmatch(svg)
	if m == nil {
		t.Fatalf("no line drawn for %s in\n%s", name, svg)
	}
	fmt.Sscanf(m[1]+" "+m[2]+" "+m[3]+" "+m[4], "%d %d %d %d", &x1, &y1, &x2, &y2)
	return
}

// The stored anchors (ParentConnection/ChildConnection, "x;y" as a percentage
// of the FROM and TO entity boxes) say where Studio Pro attaches the line. The
// SVG drew every association centre to centre whatever they held, so it could
// not be used to check a "corner to corner" layout (ako/mxcli#1841, part 1 of
// mendixlabs/mxcli#1692). In svgTestModel Order is drawn at (400,120) 90x44 and
// Customer at (100,100) 90x44.
func TestDomainModelSVG_AssociationUsesStoredAnchors(t *testing.T) {
	cases := []struct {
		name           string
		from, to       *model.Point
		x1, y1, x2, y2 int
	}{
		// Corner to corner: Order's bottom-right to Customer's top-left.
		{"corners", &model.Point{X: 100, Y: 100}, &model.Point{X: 0, Y: 0}, 490, 164, 100, 100},
		// The other two corners, the extreme the report tested with.
		{"other corners", &model.Point{X: 0, Y: 0}, &model.Point{X: 100, Y: 100}, 400, 120, 190, 144},
		// Part-way along an edge: Order's top centre, Customer's left at 54%
		// (Studio Pro's own Administration.AccountPasswordData_Account stores 0;54).
		{"along an edge", &model.Point{X: 50, Y: 0}, &model.Point{X: 0, Y: 54}, 445, 120, 100, 124},
		// Mendix's loader does not range-check, so a stored value can lie off
		// the box; the line still attaches to the box, at the nearest edge.
		{"out of range", &model.Point{X: -20, Y: 500}, &model.Point{X: 250, Y: -5}, 400, 164, 190, 100},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			entities, _ := svgTestModel()
			a := &domainmodel.Association{Name: "Order_Customer", ParentID: "o", ChildID: "c",
				Type: domainmodel.AssociationTypeReference, ParentConnection: c.from, ChildConnection: c.to}
			svg := renderDomainModelSVG("Sales", entities, []*domainmodel.Association{a})
			x1, y1, x2, y2 := assocLine(t, svg, "Order_Customer")
			if x1 != c.x1 || y1 != c.y1 || x2 != c.x2 || y2 != c.y2 {
				t.Errorf("line (%d,%d)-(%d,%d), want (%d,%d)-(%d,%d) from anchors %v / %v",
					x1, y1, x2, y2, c.x1, c.y1, c.x2, c.y2, *c.from, *c.to)
			}
		})
	}
}

// With nothing stored (or nothing readable) the line is drawn as before:
// along the centre line, from where it leaves one box to where it enters the
// other. These are the endpoints the renderer drew before #1841.
func TestDomainModelSVG_UnsetAnchorsKeepCentreLine(t *testing.T) {
	entities, assocs := svgTestModel()
	svg := renderDomainModelSVG("Sales", entities, assocs)
	x1, y1, x2, y2 := assocLine(t, svg, "Order_Customer")
	if x1 != 400 || y1 != 139 || x2 != 190 || y2 != 125 {
		t.Errorf("line (%d,%d)-(%d,%d), want the centre line (400,139)-(190,125)", x1, y1, x2, y2)
	}
}

var regexpSelfPath = regexp.MustCompile(`<g class="association"><title>Customer_Parent</title><path d="M([^"]*)"`)

// A self-association with anchors leaves the box at one stored anchor and
// comes back in at the other, routed around the outside of the box rather than
// through it (centre to centre it was a zero-length line).
func TestDomainModelSVG_SelfAssociationUsesStoredAnchors(t *testing.T) {
	entities, _ := svgTestModel()
	a := &domainmodel.Association{Name: "Customer_Parent", ParentID: "c", ChildID: "c",
		Type:             domainmodel.AssociationTypeReference,
		ParentConnection: &model.Point{X: 100, Y: 30}, ChildConnection: &model.Point{X: 50, Y: 100}}
	svg := renderDomainModelSVG("Sales", entities, []*domainmodel.Association{a})
	m := regexpSelfPath.FindStringSubmatch(svg)
	if m == nil {
		t.Fatalf("no routed path for the self-association in\n%s", svg)
	}
	var pts [][2]int
	for _, seg := range strings.Split(m[1], "L") {
		var x, y int
		if _, err := fmt.Sscanf(strings.TrimSpace(seg), "%d %d", &x, &y); err != nil {
			t.Fatalf("unreadable path %q: %v", m[1], err)
		}
		pts = append(pts, [2]int{x, y})
	}
	// Customer is drawn at (100,100) 90x44: right edge at 30% is (190,113),
	// bottom edge at 50% is (145,144).
	if first, last := pts[0], pts[len(pts)-1]; first != [2]int{190, 113} || last != [2]int{145, 144} {
		t.Errorf("path runs %v to %v, want (190,113) to (145,144): %s", first, last, m[1])
	}
	if len(pts) < 4 {
		t.Fatalf("want a routed loop with at least two bends, got %v", pts)
	}
	for i := 1; i+1 < len(pts); i++ {
		if p := pts[i]; p[0] > 100 && p[0] < 190 && p[1] > 100 && p[1] < 144 {
			t.Errorf("bend %v lies inside the entity box", p)
		}
	}
}
