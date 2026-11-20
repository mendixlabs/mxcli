// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"html"
	"math"
	"strings"

	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// cmd_domainmodel_svg.go draws a module's domain model as a finished SVG, for
// clients with no layout engine of their own (the Claude Code companion mod has
// no DOM, so the ELK graph the VS Code extension lays out in a browser is no
// use to it). No layout is computed: entities are drawn where the model stores
// them, which is where Studio Pro shows them.

// Palette, per entity kind (classifyEntity), close to Studio Pro's.
var svgEntityHeader = map[string]string{
	"persistent":    "#2B5FD9",
	"nonpersistent": "#E07B2E",
	"external":      "#8A4FD6",
	"view":          "#2E9E6A",
}

const (
	svgPad        = 24.0
	svgHeaderH    = 24.0
	svgExtendsH   = 12.0
	svgLineH      = 16.0
	svgTextPad    = 16.0
	svgMinW       = 90.0
	svgMinGap     = 8.0
	svgFont       = `font-family="Segoe UI,Helvetica Neue,Arial,sans-serif"`
	svgMaxNameLen = 60
	// svgMaxSpread caps how far the stored layout is pulled apart to fit full
	// names, so one outlier cannot blow a diagram up to several screens.
	svgMaxSpread = 2.5
)

// textPx estimates a string's width in a proportional sans-serif at `size` px.
// Studio Pro draws entities in such a font and places them for its own box
// widths; sizing for a monospace (as the ELK graph does) made boxes wider than
// the room Studio Pro left, and they overlapped.
func textPx(s string, size float64, bold bool) float64 {
	em := 0.0
	for _, r := range s {
		switch {
		case strings.ContainsRune("il.,:;'|!tfjrI ()[]", r):
			em += 0.32
		case strings.ContainsRune("mwMW", r):
			em += 0.86
		case r >= 'A' && r <= 'Z':
			em += 0.66
		case r >= '0' && r <= '9':
			em += 0.56
		default:
			em += 0.54
		}
	}
	if bold {
		em *= 1.06
	}
	return em * size
}

type svgBox struct {
	x, y, w, h float64
}

func (b svgBox) center() (float64, float64) { return b.x + b.w/2, b.y + b.h/2 }

// edgePoint is where the line from the box's centre towards (tx, ty) leaves the box.
func (b svgBox) edgePoint(tx, ty float64) (float64, float64) {
	cx, cy := b.center()
	dx, dy := tx-cx, ty-cy
	if dx == 0 && dy == 0 {
		return cx, cy
	}
	scale := math.Inf(1)
	if dx != 0 {
		scale = math.Min(scale, (b.w/2)/math.Abs(dx))
	}
	if dy != 0 {
		scale = math.Min(scale, (b.h/2)/math.Abs(dy))
	}
	return cx + dx*scale, cy + dy*scale
}

// edgePointFrom is where the line from (sx, sy) — a point inside the box,
// shifted off its centre for a parallel association — towards (tx, ty) leaves
// the box.
func (b svgBox) edgePointFrom(sx, sy, tx, ty float64) (float64, float64) {
	dx, dy := tx-sx, ty-sy
	if dx == 0 && dy == 0 {
		return sx, sy
	}
	scale := math.Inf(1)
	if dx > 0 {
		scale = math.Min(scale, (b.x+b.w-sx)/dx)
	} else if dx < 0 {
		scale = math.Min(scale, (b.x-sx)/dx)
	}
	if dy > 0 {
		scale = math.Min(scale, (b.y+b.h-sy)/dy)
	} else if dy < 0 {
		scale = math.Min(scale, (b.y-sy)/dy)
	}
	if scale < 0 || math.IsInf(scale, 1) {
		return sx, sy
	}
	return sx + dx*scale, sy + dy*scale
}

// anchorPoint is where a stored association anchor sits on the drawn box.
//
// Mendix stores an association's anchors (ParentConnection on the FROM entity,
// ChildConnection on the TO entity) as "x;y", a PERCENTAGE of the entity box
// (see sdk/domainmodel/connection.go for the measurement). The box here is this
// renderer's estimate, not Studio Pro's, so the percentage is applied to the
// box as drawn — the line attaches to the same edge, the same fraction along.
// Mendix's loader does not range-check, so a value outside 0..100 is clamped
// onto the box: a connector attaches to its entity. nil means nothing (or
// nothing readable) was stored, and the caller keeps the centre-line drawing.
func (b svgBox) anchorPoint(p *model.Point) (x, y float64, ok bool) {
	if p == nil {
		return 0, 0, false
	}
	fx := math.Max(0, math.Min(100, float64(p.X))) / 100
	fy := math.Max(0, math.Min(100, float64(p.Y))) / 100
	return b.x + fx*b.w, b.y + fy*b.h, true
}

// outward is the unit normal of the box edge nearest to (x, y), a point on it.
func (b svgBox) outward(x, y float64) (float64, float64) {
	nx, ny, best := -1.0, 0.0, x-b.x
	if d := b.x + b.w - x; d < best {
		nx, ny, best = 1, 0, d
	}
	if d := y - b.y; d < best {
		nx, ny, best = 0, -1, d
	}
	if d := b.y + b.h - y; d < best {
		nx, ny = 0, 1
	}
	return nx, ny
}

// selfLoop routes a self-association from anchor (x1, y1) back to anchor
// (x2, y2) of the same box: a short stub straight out of each anchor's edge,
// joined the shorter way round a rectangle `d` outside the box, so the line
// never crosses the entity it belongs to.
func (b svgBox) selfLoop(x1, y1, x2, y2 float64) [][2]float64 {
	const d = 20.0
	r := svgBox{x: b.x - d, y: b.y - d, w: b.w + 2*d, h: b.h + 2*d}
	n1x, n1y := b.outward(x1, y1)
	n2x, n2y := b.outward(x2, y2)
	s1 := [2]float64{x1 + n1x*d, y1 + n1y*d}
	s2 := [2]float64{x2 + n2x*d, y2 + n2y*d}
	// Position along r's perimeter, clockwise from its top-left corner.
	per := 2 * (r.w + r.h)
	at := func(p [2]float64) float64 {
		switch {
		case p[1] <= r.y:
			return p[0] - r.x
		case p[0] >= r.x+r.w:
			return r.w + p[1] - r.y
		case p[1] >= r.y+r.h:
			return r.w + r.h + r.x + r.w - p[0]
		default:
			return 2*r.w + r.h + r.y + r.h - p[1]
		}
	}
	corners := []struct {
		t float64
		p [2]float64
	}{
		{0, [2]float64{r.x, r.y}},
		{r.w, [2]float64{r.x + r.w, r.y}},
		{r.w + r.h, [2]float64{r.x + r.w, r.y + r.h}},
		{2*r.w + r.h, [2]float64{r.x, r.y + r.h}},
	}
	t1, t2 := at(s1), at(s2)
	cw := math.Mod(t2-t1+per, per)
	clockwise := cw <= per-cw
	walk := cw
	if !clockwise {
		walk = per - cw
	}
	// The corners passed on the way, each with its distance from s1 along the
	// walk; at most four, so sorting them by insertion is enough.
	type passed struct {
		p    [2]float64
		dist float64
	}
	var via []passed
	for _, c := range corners {
		dc := math.Mod(c.t-t1+per, per)
		if !clockwise {
			dc = math.Mod(t1-c.t+per, per)
		}
		if dc <= 0 || dc >= walk {
			continue
		}
		i := len(via)
		via = append(via, passed{c.p, dc})
		for ; i > 0 && via[i-1].dist > via[i].dist; i-- {
			via[i-1], via[i] = via[i], via[i-1]
		}
	}
	pts := [][2]float64{{x1, y1}, s1}
	for _, v := range via {
		pts = append(pts, v.p)
	}
	return append(pts, s2, [2]float64{x2, y2})
}

func svgNum(f float64) string { return fmt.Sprintf("%.0f", f) }

func svgText(s string) string {
	if len(s) > svgMaxNameLen {
		s = s[:svgMaxNameLen-1] + "…"
	}
	return html.EscapeString(s)
}

// renderDomainModelSVG draws the entities, the associations between them and
// the generalizations of one module. An association or generalization whose
// other end is outside the module has no position to draw to: the association
// is left out, the generalization is written under the entity's name.
func renderDomainModelSVG(moduleName string, entities []*domainmodel.Entity, assocs []*domainmodel.Association) string {
	if len(entities) == 0 {
		return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 320 60" width="320" height="60">` +
			`<text x="16" y="34" ` + svgFont + ` font-size="13" fill="#6B7684">` +
			svgText(moduleName) + ` has no entities</text></svg>`
	}

	boxes := map[model.ID]svgBox{}
	byName := map[string]model.ID{}
	for _, e := range entities {
		w := textPx(e.Name, 12, true)
		if e.GeneralizationRef != "" {
			w = math.Max(w, textPx("extends "+e.GeneralizationRef, 10, false))
		}
		for _, at := range e.Attributes {
			w = math.Max(w, textPx(at.Name+": "+at.Type.GetTypeName(), 11, false))
		}
		h := svgHeaderH + svgLineH*math.Max(1, float64(len(e.Attributes))) + 4
		if e.GeneralizationRef != "" {
			h += svgExtendsH
		}
		boxes[e.ID] = svgBox{x: float64(e.Location.X), y: float64(e.Location.Y), w: math.Max(svgMinW, w+svgTextPad), h: h}
		byName[moduleName+"."+e.Name] = e.ID
	}
	// Studio Pro draws entities at a fixed width and cuts long names off, a
	// common complaint. Keep its arrangement, but pull it apart (x first, then
	// y at the new x) just enough that every box fits its longest line, up to
	// svgMaxSpread; past that the clamp below takes over.
	spread(entities, boxes, true)
	spread(entities, boxes, false)

	// A box never runs into the entity to its right, nor into the one below:
	// widths first (against neighbours sharing its rows), then heights (against
	// neighbours sharing its columns, at the clamped width). The text is
	// clipped to the box; the tooltip keeps all of it.
	clamped := map[model.ID]bool{}
	for _, e := range entities {
		a := boxes[e.ID]
		for _, o := range entities {
			b := boxes[o.ID]
			if o.ID == e.ID || b.x <= a.x || a.y >= b.y+b.h || b.y >= a.y+a.h {
				continue
			}
			if room := b.x - a.x - svgMinGap; room < a.w {
				a.w, clamped[e.ID] = math.Max(60, room), true
			}
		}
		boxes[e.ID] = a
	}
	for _, e := range entities {
		a := boxes[e.ID]
		for _, o := range entities {
			b := boxes[o.ID]
			if o.ID == e.ID || b.y <= a.y || a.x >= b.x+b.w || b.x >= a.x+a.w {
				continue
			}
			if room := b.y - a.y - svgMinGap; room < a.h {
				a.h, clamped[e.ID] = math.Max(svgHeaderH, room), true
			}
		}
		boxes[e.ID] = a
	}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, b := range boxes {
		minX, minY = math.Min(minX, b.x), math.Min(minY, b.y)
		maxX, maxY = math.Max(maxX, b.x+b.w), math.Max(maxY, b.y+b.h)
	}
	minX, minY, maxX, maxY = minX-svgPad, minY-svgPad, maxX+svgPad, maxY+svgPad
	w, h := maxX-minX, maxY-minY

	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="%s %s %s %s" width="%s" height="%s" %s font-size="11">`,
		svgNum(minX), svgNum(minY), svgNum(w), svgNum(h), svgNum(w), svgNum(h), svgFont)
	fmt.Fprintf(&sb, `<rect x="%s" y="%s" width="%s" height="%s" rx="8" fill="#F7F8FA"/>`, svgNum(minX), svgNum(minY), svgNum(w), svgNum(h))

	// Associations between the same two entities are spread side by side, so
	// their lines and labels do not print over one another.
	pairKey := func(a *domainmodel.Association) string {
		if a.ParentID < a.ChildID {
			return string(a.ParentID) + "|" + string(a.ChildID)
		}
		return string(a.ChildID) + "|" + string(a.ParentID)
	}
	pairTotal := map[string]int{}
	for _, a := range assocs {
		pairTotal[pairKey(a)]++
	}
	pairSeen := map[string]int{}

	// Lines first, so the boxes sit on top of them.
	for _, a := range assocs {
		from, okFrom := boxes[a.ParentID] // ParentPointer is the FROM side: it owns the reference
		to, okTo := boxes[a.ChildID]
		if !okFrom || !okTo {
			continue
		}
		key := pairKey(a)
		offset := (float64(pairSeen[key]) - float64(pairTotal[key]-1)/2) * 16
		pairSeen[key]++
		tcx, tcy := to.center()
		fcx, fcy := from.center()
		// Shift both aim points perpendicular to the centre line by the offset.
		if l := math.Hypot(tcx-fcx, tcy-fcy); l > 0 && offset != 0 {
			px, py := -(tcy-fcy)/l*offset, (tcx-fcx)/l*offset
			tcx, tcy, fcx, fcy = tcx+px, tcy+py, fcx+px, fcy+py
		}
		x1, y1 := from.edgePointFrom(fcx, fcy, tcx, tcy)
		x2, y2 := to.edgePointFrom(tcx, tcy, fcx, fcy)
		// The stored anchors say where Studio Pro attaches each end; an end
		// with none stored keeps the centre-line point. (ako/mxcli#1841)
		ax1, ay1, okA1 := from.anchorPoint(a.ParentConnection)
		ax2, ay2, okA2 := to.anchorPoint(a.ChildConnection)
		if okA1 {
			x1, y1 = ax1, ay1
		}
		if okA2 {
			x2, y2 = ax2, ay2
		}
		fromMult, toMult := "*", "1"
		if a.Type == domainmodel.AssociationTypeReferenceSet {
			toMult = "*"
		}
		fmt.Fprintf(&sb, `<g class="association"><title>%s</title>`, svgText(a.Name))
		if a.ParentID == a.ChildID && okA1 && okA2 {
			// A self-association leaves its box and comes back in, round
			// the outside; a straight line would cross the entity.
			route := from.selfLoop(x1, y1, x2, y2)
			sb.WriteString(`<path d="`)
			for i, p := range route {
				if i == 0 {
					sb.WriteString("M")
				} else {
					sb.WriteString("L")
				}
				sb.WriteString(svgNum(p[0]) + " " + svgNum(p[1]))
			}
			sb.WriteString(`" fill="none" stroke="#6B7684" stroke-width="1.5"/>`)
			n := len(route)
			writeEndLabel(&sb, x1, y1, route[1][0], route[1][1], fromMult)
			writeEndLabel(&sb, x2, y2, route[n-2][0], route[n-2][1], toMult)
			mid := route[n/2]
			fmt.Fprintf(&sb, `<text x="%s" y="%s" font-size="10" fill="#6B7684" text-anchor="middle">%s</text>`,
				svgNum(mid[0]), svgNum(mid[1]-4+offset/16*12), svgText(a.Name))
			sb.WriteString(`</g>`)
			continue
		}
		fmt.Fprintf(&sb, `<line x1="%s" y1="%s" x2="%s" y2="%s" stroke="#6B7684" stroke-width="1.5"/>`, svgNum(x1), svgNum(y1), svgNum(x2), svgNum(y2))
		writeEndLabel(&sb, x1, y1, x2, y2, fromMult)
		writeEndLabel(&sb, x2, y2, x1, y1, toMult)
		// Labels of parallel associations stack, one line each.
		fmt.Fprintf(&sb, `<text x="%s" y="%s" font-size="10" fill="#6B7684" text-anchor="middle">%s</text>`,
			svgNum((x1+x2)/2), svgNum((y1+y2)/2-4+offset/16*12), svgText(a.Name))
		sb.WriteString(`</g>`)
	}
	for _, e := range entities {
		parentID, ok := byName[e.GeneralizationRef]
		if !ok {
			continue
		}
		child, parent := boxes[e.ID], boxes[parentID]
		pcx, pcy := parent.center()
		ccx, ccy := child.center()
		x1, y1 := child.edgePoint(pcx, pcy)
		x2, y2 := parent.edgePoint(ccx, ccy)
		fmt.Fprintf(&sb, `<g class="generalization"><line x1="%s" y1="%s" x2="%s" y2="%s" stroke="#6B7684" stroke-width="1.5"/>`, svgNum(x1), svgNum(y1), svgNum(x2), svgNum(y2))
		writeArrowHead(&sb, x1, y1, x2, y2)
		sb.WriteString(`</g>`)
	}

	for i, e := range entities {
		b := boxes[e.ID]
		kind := classifyEntity(e)
		var tip strings.Builder
		tip.WriteString(e.Name)
		if e.GeneralizationRef != "" {
			fmt.Fprintf(&tip, " (extends %s)", e.GeneralizationRef)
		}
		for _, at := range e.Attributes {
			fmt.Fprintf(&tip, "\n%s: %s", at.Name, at.Type.GetTypeName())
		}
		fmt.Fprintf(&sb, `<g class="entity %s" transform="translate(%s %s)"><title>%s</title>`, kind, svgNum(b.x), svgNum(b.y), html.EscapeString(tip.String()))
		headerH := svgHeaderH
		if e.GeneralizationRef != "" {
			headerH += svgExtendsH
		}
		headerH = math.Min(headerH, b.h)
		fmt.Fprintf(&sb, `<rect width="%s" height="%s" rx="4" fill="#FFFFFF" stroke="#B8C0CC"/>`, svgNum(b.w), svgNum(b.h))
		fmt.Fprintf(&sb, `<path d="M4 0H%sA4 4 0 0 1 %s 4V%sH0V4A4 4 0 0 1 4 0Z" fill="%s"/>`,
			svgNum(b.w-4), svgNum(b.w), svgNum(headerH), svgEntityHeader[kind])
		textOpen := "<g>"
		if clamped[e.ID] {
			fmt.Fprintf(&sb, `<clipPath id="clip%d"><rect width="%s" height="%s"/></clipPath>`, i, svgNum(b.w-4), svgNum(b.h))
			textOpen = fmt.Sprintf(`<g clip-path="url(#clip%d)">`, i)
		}
		sb.WriteString(textOpen)
		fmt.Fprintf(&sb, `<text x="8" y="16" fill="#FFFFFF" font-size="12" font-weight="600">%s</text>`, svgText(e.Name))
		if e.GeneralizationRef != "" {
			fmt.Fprintf(&sb, `<text x="8" y="30" fill="#FFFFFF" font-size="10" font-style="italic">extends %s</text>`, svgText(e.GeneralizationRef))
		}
		for j, at := range e.Attributes {
			fmt.Fprintf(&sb, `<text x="8" y="%s" fill="#1F2933">%s</text>`,
				svgNum(headerH+svgLineH*float64(j)+13), svgText(at.Name+": "+at.Type.GetTypeName()))
		}
		sb.WriteString(`</g></g>`)
	}
	sb.WriteString(`</svg>`)
	return sb.String()
}

// spread scales the boxes' positions along one axis, from the smallest
// coordinate, by the least factor (1 to svgMaxSpread) that leaves every box
// clear of the next one along that axis among those it shares the other axis
// with.
func spread(entities []*domainmodel.Entity, boxes map[model.ID]svgBox, horizontal bool) {
	k := 1.0
	origin := math.Inf(1)
	for _, e := range entities {
		a := boxes[e.ID]
		if horizontal {
			origin = math.Min(origin, a.x)
		} else {
			origin = math.Min(origin, a.y)
		}
		for _, o := range entities {
			b := boxes[o.ID]
			if o.ID == e.ID {
				continue
			}
			var gap, need float64
			if horizontal {
				if b.x <= a.x || a.y >= b.y+b.h || b.y >= a.y+a.h {
					continue
				}
				gap, need = b.x-a.x, a.w+svgMinGap
			} else {
				if b.y <= a.y || a.x >= b.x+b.w || b.x >= a.x+a.w {
					continue
				}
				gap, need = b.y-a.y, a.h+svgMinGap
			}
			if need > gap {
				k = math.Max(k, need/gap)
			}
		}
	}
	k = math.Min(k, svgMaxSpread)
	if k == 1 {
		return
	}
	for id, b := range boxes {
		if horizontal {
			b.x = origin + (b.x-origin)*k
		} else {
			b.y = origin + (b.y-origin)*k
		}
		boxes[id] = b
	}
}

// writeEndLabel puts a multiplicity just inside the line's end at (x, y), set
// off to one side so it does not sit on the line.
func writeEndLabel(sb *strings.Builder, x, y, ox, oy float64, label string) {
	dx, dy := ox-x, oy-y
	l := math.Hypot(dx, dy)
	if l == 0 {
		return
	}
	ux, uy := dx/l, dy/l
	lx, ly := x+ux*12-uy*8, y+uy*12+ux*8
	fmt.Fprintf(sb, `<text x="%s" y="%s" font-size="11" fill="#1F2933" text-anchor="middle" dominant-baseline="middle">%s</text>`, svgNum(lx), svgNum(ly), label)
}

// writeArrowHead draws a hollow triangle at (x2, y2), pointing along the line.
func writeArrowHead(sb *strings.Builder, x1, y1, x2, y2 float64) {
	dx, dy := x2-x1, y2-y1
	l := math.Hypot(dx, dy)
	if l == 0 {
		return
	}
	ux, uy := dx/l, dy/l
	bx, by := x2-ux*12, y2-uy*12
	fmt.Fprintf(sb, `<path d="M%s %sL%s %sL%s %sZ" fill="#FFFFFF" stroke="#6B7684" stroke-width="1.5"/>`,
		svgNum(x2), svgNum(y2), svgNum(bx-uy*7), svgNum(by+ux*7), svgNum(bx+uy*7), svgNum(by-ux*7))
}

// domainModelSVG writes the SVG of a module's domain model.
func domainModelSVG(ctx *ExecContext, moduleName string) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	module, err := findModule(ctx, moduleName)
	if err != nil {
		return err
	}
	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}
	fmt.Fprint(ctx.Output, renderDomainModelSVG(moduleName, dm.Entities, dm.Associations))
	return nil
}

// DomainModelSVG is the exported Executor method, called from outside the package.
func (e *Executor) DomainModelSVG(moduleName string) error {
	return domainModelSVG(e.newExecContext(context.Background()), moduleName)
}
