// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// The figures are hand-built SVG so the HTML page stays one self-contained
// file. Colours are CSS variables from the page stylesheet, set through
// style= (presentation attributes cannot read var()), so dark mode follows.

// layeredMapSVG draws modules on their layers, highest at the top, with
// domain edges as arrows. Cycle edges are dashed red; fill shows cohesion.
func layeredMapSVG(mods []ModuleHealth, edges []ModuleEdge) string {
	if len(mods) == 0 {
		return "<p>No app modules.</p>"
	}
	byLayer := map[int][]string{}
	maxLayer := 0
	for _, m := range mods {
		byLayer[m.Layer] = append(byLayer[m.Layer], m.Name)
		maxLayer = max(maxLayer, m.Layer)
	}
	widest := 1
	for _, ns := range byLayer {
		widest = max(widest, len(ns))
	}
	const boxW, boxH, gapX, gapY, pad = 150, 34, 30, 70, 20
	width := pad*2 + widest*boxW + (widest-1)*gapX + 70
	height := pad*2 + (maxLayer+1)*boxH + maxLayer*gapY
	pos := map[string][2]float64{}
	for l := maxLayer; l >= 0; l-- {
		ns := byLayer[l]
		sort.Strings(ns)
		rowW := len(ns)*boxW + (len(ns)-1)*gapX
		x0 := float64(pad+70) + float64((widest*boxW+(widest-1)*gapX)-rowW)/2
		y := float64(pad + (maxLayer-l)*(boxH+gapY))
		for i, n := range ns {
			pos[n] = [2]float64{x0 + float64(i*(boxW+gapX)), y}
		}
	}
	health := map[string]ModuleHealth{}
	for _, m := range mods {
		health[m.Name] = m
	}
	maxRefs := 1
	for _, e := range edges {
		maxRefs = max(maxRefs, e.Refs)
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="Layered module map">`, width, height, width, height)
	b.WriteString(`<defs><marker id="arr" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M0,0L10,5L0,10z" style="fill:var(--muted)"/></marker>` +
		`<marker id="arrc" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M0,0L10,5L0,10z" style="fill:var(--bad)"/></marker></defs>`)
	for l := maxLayer; l >= 0; l-- {
		y := float64(pad+(maxLayer-l)*(boxH+gapY)) + boxH/2 + 4
		fmt.Fprintf(&b, `<text x="%d" y="%.0f" class="muted">layer %d</text>`, pad, y, l)
	}
	for _, e := range edges {
		p, ok1 := pos[e.From]
		q, ok2 := pos[e.To]
		if !ok1 || !ok2 {
			continue
		}
		x1, y1 := p[0]+boxW/2, p[1]+boxH/2
		x2, y2 := q[0]+boxW/2, q[1]+boxH/2
		// Clip to the box edges so the arrowhead is visible.
		x1, y1 = clipToBox(x1, y1, x2, y2, boxW/2, boxH/2)
		x2, y2 = clipToBox(x2, y2, x1, y1, boxW/2, boxH/2)
		sw := 1 + 3*math.Log1p(float64(e.Refs))/math.Log1p(float64(maxRefs))
		style := "stroke:var(--muted);fill:none;opacity:.75"
		marker := "arr"
		if e.InCycle {
			style = "stroke:var(--bad);fill:none;stroke-dasharray:5 3"
			marker = "arrc"
		}
		// Same-layer edges bow so a pair in a cycle shows two arrows.
		cx, cy := (x1+x2)/2, (y1+y2)/2
		if math.Abs(y1-y2) < 1 {
			cy -= 24
			if e.From > e.To {
				cy += 48
			}
		}
		fmt.Fprintf(&b, `<path d="M%.1f,%.1f Q%.1f,%.1f %.1f,%.1f" style="%s;stroke-width:%.1f" marker-end="url(#%s)"><title>%s → %s: %d refs (%s)</title></path>`,
			x1, y1, cx, cy, x2, y2, style, sw, marker, esc(e.From), esc(e.To), e.Refs, esc(strings.Join(e.Kinds, ", ")))
	}
	for _, m := range mods {
		p := pos[m.Name]
		fill := "var(--okbg)"
		switch {
		case m.Cohesion < 0:
			fill = "var(--card)"
		case m.Cohesion < 40 && m.Role == "":
			fill = "var(--warnbg)"
		case m.Cohesion < 40:
			fill = "var(--infobg)"
		}
		stroke := "var(--line)"
		if m.InCycle {
			stroke = "var(--bad)"
		}
		fmt.Fprintf(&b, `<g><title>%s: cohesion %.0f%%, %d documents%s</title><rect x="%.0f" y="%.0f" width="%d" height="%d" rx="6" style="fill:%s;stroke:%s"/>`,
			esc(m.Name), m.Cohesion, m.Documents, roleSuffix(m.Role), p[0], p[1], boxW, boxH, fill, stroke)
		fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" text-anchor="middle">%s</text></g>`, p[0]+boxW/2, p[1]+boxH/2+4, esc(truncate(m.Name, 22)))
	}
	b.WriteString("</svg>")
	return b.String()
}

func roleSuffix(r string) string {
	if r == "" {
		return ""
	}
	return " (" + r + ")"
}

func clipToBox(cx, cy, tx, ty, hw, hh float64) (float64, float64) {
	dx, dy := tx-cx, ty-cy
	if dx == 0 && dy == 0 {
		return cx, cy
	}
	s := math.Inf(1)
	if dx != 0 {
		s = math.Min(s, hw/math.Abs(dx))
	}
	if dy != 0 {
		s = math.Min(s, hh/math.Abs(dy))
	}
	return cx + dx*s, cy + dy*s
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// barSegment is one part of a stacked bar.
type barSegment struct {
	Label string
	Value int
	Color string // a CSS variable name, e.g. "--ok"
}

// stackedBarsSVG draws one horizontal stacked bar per row, with a legend.
func stackedBarsSVG(labels []string, rows [][]barSegment) string {
	if len(rows) == 0 {
		return ""
	}
	const labelW, barW, rowH, pad = 170, 520, 22, 10
	maxTotal := 1
	for _, r := range rows {
		t := 0
		for _, s := range r {
			t += s.Value
		}
		maxTotal = max(maxTotal, t)
	}
	height := pad*2 + len(rows)*rowH + 26
	width := labelW + barW + 80
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img">`, width, height, width, height)
	for i, r := range rows {
		y := pad + i*rowH
		fmt.Fprintf(&b, `<text x="%d" y="%d" text-anchor="end">%s</text>`, labelW-8, y+14, esc(truncate(labels[i], 26)))
		x := float64(labelW)
		total := 0
		for _, s := range r {
			total += s.Value
			if s.Value == 0 {
				continue
			}
			w := float64(barW) * float64(s.Value) / float64(maxTotal)
			fmt.Fprintf(&b, `<rect x="%.1f" y="%d" width="%.1f" height="%d" style="fill:var(%s)"><title>%s: %d</title></rect>`, x, y+3, w, rowH-6, s.Color, esc(s.Label), s.Value)
			x += w
		}
		fmt.Fprintf(&b, `<text x="%.1f" y="%d" class="muted">%d</text>`, x+6, y+14, total)
	}
	// legend from the first row's segment labels
	lx := labelW
	ly := pad + len(rows)*rowH + 16
	for _, s := range rows[0] {
		fmt.Fprintf(&b, `<rect x="%d" y="%d" width="10" height="10" style="fill:var(%s)"/><text x="%d" y="%d">%s</text>`, lx, ly-9, s.Color, lx+14, ly, esc(s.Label))
		lx += 14 + 7*len(s.Label) + 18
	}
	b.WriteString("</svg>")
	return b.String()
}

// flameSpan is a span placed for a flame chart.
type flameSpan struct {
	Name  string
	Start float64 // ms from the window start
	Dur   float64 // ms
	Depth int
	Kind  string // microflow | db | request | other
}

// flameSVG draws spans as a flame chart: time left to right, call depth top to
// bottom. Concurrent requests are packed into bands by the caller (Depth
// already includes the band offset).
func flameSVG(spans []flameSpan, totalMs float64) string {
	if len(spans) == 0 || totalMs <= 0 {
		return ""
	}
	const width, rowH, pad = 1000, 18, 6
	maxDepth := 0
	for _, s := range spans {
		maxDepth = max(maxDepth, s.Depth)
	}
	height := pad*2 + (maxDepth+1)*rowH + 16
	scale := float64(width-2*pad) / totalMs
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img">`, width, height, width, height)
	for _, s := range spans {
		x := pad + s.Start*scale
		w := math.Max(s.Dur*scale, 0.5)
		y := pad + s.Depth*rowH
		color := map[string]string{"microflow": "--infobg", "db": "--warnbg", "request": "--okbg"}[s.Kind]
		if color == "" {
			color = "--card"
		}
		fmt.Fprintf(&b, `<g><title>%s — %.1f ms</title><rect x="%.1f" y="%d" width="%.1f" height="%d" style="fill:var(%s);stroke:var(--line)"/>`, esc(s.Name), s.Dur, x, y, w, rowH-2, color)
		if w > 40 {
			fmt.Fprintf(&b, `<text x="%.1f" y="%d">%s</text>`, x+3, y+12, esc(truncate(s.Name, int(w/6.5))))
		}
		b.WriteString("</g>")
	}
	fmt.Fprintf(&b, `<text x="%d" y="%d" class="muted">0 ms</text><text x="%d" y="%d" class="muted" text-anchor="end">%.0f ms</text>`,
		pad, height-4, width-pad, height-4, totalMs)
	b.WriteString("</svg>")
	return b.String()
}
