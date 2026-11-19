// SPDX-License-Identifier: Apache-2.0

// Package projectreport builds the `mxcli report <kind>` family: architecture,
// tests, security, performance and traceability (ako/mxcli#2221).
//
// Every kind works in two steps, and the split is the point:
//
//  1. Collect measures the project (catalog, test results, spans, brain) and
//     produces a typed data value. It is written as JSON inside an Envelope
//     that also carries how each number was obtained and what it does not
//     cover.
//  2. Render turns that envelope — plus an optional annotations file a person
//     writes (cluster names, verdicts, statuses) — into markdown or a
//     standalone HTML page.
//
// Because the page is rendered from the JSON, a team can add its judgement and
// re-render without re-measuring (`--from report.json --annotations notes.yaml`),
// and two runs of the same kind are comparable field by field.
package projectreport

import (
	"fmt"
	"html"
	"io"
	"strings"
)

// Doc is the rendered shape every kind produces: a title, headline numbers and
// sections of blocks. The markdown and HTML renderers only know this shape, so
// a kind never formats output itself and every page reads the same way.
type Doc struct {
	Title    string
	Subtitle string
	// Lede is the page's verdict in a sentence or three, shown under the
	// title in HTML (the subtitle when empty). An annotations summary wins.
	Lede     string
	Stats    []Stat
	Sections []Section

	// Filled by Frame, for the HTML header: the line above the title
	// (project · report · date) and the footer.
	Project, Kind, Generated, Mxcli string
	// Description is the kind's own subtitle, before Frame appends the
	// project and date for markdown.
	Description string
	// Nav links the pages of a report folder (report all); empty for a
	// single page.
	Nav []NavLink
}

// NavLink is one report page in the folder navigation.
type NavLink struct {
	Label, Href string
	Current     bool
}

// Stat is one headline number. Status ("good", "warn", "bad") colours the
// tile's edge; Note is the sentence that says what the number means.
type Stat struct {
	Label  string
	Value  string
	Note   string
	Status string
}

// Section is a titled part of the page.
type Section struct {
	ID     string
	Title  string
	Intro  string
	Blocks []Block
}

// Block is one piece of section content.
type Block interface{ isBlock() }

// Para is a paragraph of plain text. Backticks mark code in both renderers.
type Para struct{ Text string }

// Bullets is an unordered list.
type Bullets struct{ Items []string }

// Callout is a highlighted note: "warn" for a precondition that was not met or
// a measurement that is known to be off, "info" otherwise.
type Callout struct {
	Kind string
	Text string
}

// Table is a grid. Classes, when set, holds a CSS class per cell (badge
// colours in HTML; ignored in markdown). Heat, when set, shades a cell by a
// value in [0,1] (the coupling matrix).
type Table struct {
	Caption string
	Columns []string
	Rows    [][]string
	Classes [][]string
	Heat    [][]float64
	// Bars, when set, draws a bar in a cell in HTML (a score, a cohesion, a
	// share); the cell's text is its label, and all markdown shows.
	Bars [][]*Bar
}

// Bar is an inline bar: Pct in [0,100], Status "good", "warn", "bad" or ""
// (the accent colour).
type Bar struct {
	Pct    float64
	Status string
}

// Stacked is a set of stacked bars, one per row, with a legend: built,
// tested and not per milestone; reached and not per module.
type Stacked struct {
	Caption string
	Legend  []StackKey
	Rows    []StackRow
}

// StackKey names one part of the stack and its colour: a status ("good",
// "warn", "bad", "info", "none") or a series slot ("s1".."s8").
type StackKey struct {
	Label string
	Color string
}

// StackRow is one bar: its parts in legend order, and the text after it.
type StackRow struct {
	Label string
	Parts []int
	Value string
}

// Card is a finding as a card: a tag (severity or id), a status line, a
// title and paragraphs — triage, the report's advice.
type Card struct {
	Tag        string
	Status     string // "good", "warn", "bad", "none": colours tag and status
	StatusText string // "fixed · high", "open", "accepted"
	Title      string
	Paras      []string
}

// Figure is an inline SVG for the HTML page. Markdown has no place for it and
// prints Fallback instead (usually "see the HTML report").
type Figure struct {
	SVG      string
	Caption  string
	Fallback string
}

func (Para) isBlock()    {}
func (Bullets) isBlock() {}
func (Callout) isBlock() {}
func (Table) isBlock()   {}
func (Figure) isBlock()  {}
func (Stacked) isBlock() {}
func (Card) isBlock()    {}
func (Cards) isBlock()   {}

// Cards is a grid of linked panels: the overview's one per report.
type Cards struct{ Items []LinkCard }

// LinkCard is one report on the overview: where it is, its verdict, its
// headline tiles. Missing says why it was not generated.
type LinkCard struct {
	Title, Href, Lede, Missing string
	Stats                      []Stat
}

// ---------------------------------------------------------------- markdown

// RenderMarkdown writes the document as GitHub-flavoured markdown.
func RenderMarkdown(w io.Writer, d Doc) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", d.Title)
	if d.Subtitle != "" {
		fmt.Fprintf(&b, "%s\n\n", d.Subtitle)
	}
	if len(d.Stats) > 0 {
		b.WriteString("| | |\n|---|---|\n")
		for _, s := range d.Stats {
			v := s.Value
			if s.Note != "" {
				v += " — " + s.Note
			}
			fmt.Fprintf(&b, "| %s | %s |\n", mdCell(s.Label), mdCell(v))
		}
		b.WriteString("\n")
	}
	for _, s := range d.Sections {
		fmt.Fprintf(&b, "## %s\n\n", s.Title)
		if s.Intro != "" {
			fmt.Fprintf(&b, "%s\n\n", s.Intro)
		}
		for _, bl := range s.Blocks {
			writeMarkdownBlock(&b, bl)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func writeMarkdownBlock(b *strings.Builder, bl Block) {
	switch v := bl.(type) {
	case Para:
		fmt.Fprintf(b, "%s\n\n", v.Text)
	case Bullets:
		for _, it := range v.Items {
			fmt.Fprintf(b, "- %s\n", it)
		}
		b.WriteString("\n")
	case Callout:
		label := "Note"
		if v.Kind == "warn" {
			label = "Warning"
		}
		fmt.Fprintf(b, "> **%s:** %s\n\n", label, v.Text)
	case Table:
		if v.Caption != "" {
			fmt.Fprintf(b, "**%s**\n\n", v.Caption)
		}
		if len(v.Rows) == 0 {
			b.WriteString("_(none)_\n\n")
			return
		}
		cols := make([]string, len(v.Columns))
		for i, c := range v.Columns {
			cols[i] = mdCell(c)
		}
		b.WriteString("| " + strings.Join(cols, " | ") + " |\n")
		b.WriteString("|" + strings.Repeat("---|", len(v.Columns)) + "\n")
		for _, r := range v.Rows {
			cells := make([]string, len(v.Columns))
			for i := range cells {
				if i < len(r) {
					cells[i] = mdCell(r[i])
				}
			}
			b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
		}
		b.WriteString("\n")
	case Figure:
		if v.Fallback != "" {
			fmt.Fprintf(b, "%s\n\n", v.Fallback)
		}
	case Stacked:
		if v.Caption != "" {
			fmt.Fprintf(b, "**%s**\n\n", v.Caption)
		}
		cols := []string{""}
		for _, k := range v.Legend {
			cols = append(cols, mdCell(k.Label))
		}
		cols = append(cols, "")
		b.WriteString("| " + strings.Join(cols, " | ") + " |\n")
		b.WriteString("|" + strings.Repeat("---|", len(cols)) + "\n")
		for _, r := range v.Rows {
			cells := []string{mdCell(r.Label)}
			for i := range v.Legend {
				n := 0
				if i < len(r.Parts) {
					n = r.Parts[i]
				}
				cells = append(cells, fmt.Sprint(n))
			}
			cells = append(cells, mdCell(r.Value))
			b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
		}
		b.WriteString("\n")
	case Cards:
		for _, c := range v.Items {
			if c.Missing != "" {
				fmt.Fprintf(b, "### %s\n\nNot generated: %s\n\n", c.Title, c.Missing)
				continue
			}
			fmt.Fprintf(b, "### [%s](%s)\n\n", c.Title, c.Href)
			if c.Lede != "" {
				fmt.Fprintf(b, "%s\n\n", c.Lede)
			}
			for _, s := range c.Stats {
				fmt.Fprintf(b, "- %s: %s\n", s.Label, s.Value)
			}
			b.WriteString("\n")
		}
	case Card:
		head := "**" + v.Title + "**"
		if v.Tag != "" {
			head = "`" + v.Tag + "` " + head
		}
		if v.StatusText != "" {
			head += " — " + v.StatusText
		}
		fmt.Fprintf(b, "%s\n\n", head)
		for _, p := range v.Paras {
			fmt.Fprintf(b, "%s\n\n", p)
		}
	}
}

func mdCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.ReplaceAll(s, "\n", " ")
}

// ---------------------------------------------------------------- html

// RenderHTML writes the document as one standalone page: embedded CSS, no
// scripts, no external requests, light and dark (the system's, or forced with
// data-theme="light|dark" on <html>).
//
// The look follows the reports ako/PizzaV1 wrote by hand (ako/mxcli#2443):
// a line naming project, report and date above the title, the verdict as a
// lede, tiles with a status edge and a sentence, bars in tables, findings as
// cards, numbers in a fixed-width face, prose at a reading width.
func RenderHTML(w io.Writer, d Doc) error {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	fmt.Fprintf(&b, "<title>%s</title>\n<style>%s</style>\n</head>\n<body>\n<main>\n", esc(d.Title), pageCSS)
	b.WriteString("<header class=\"top\">")
	if len(d.Nav) > 0 {
		b.WriteString("<nav class=\"reports\">")
		for _, n := range d.Nav {
			cur := ""
			if n.Current {
				cur = " aria-current=\"page\""
			}
			fmt.Fprintf(&b, "<a href=\"%s\"%s>%s</a>", esc(n.Href), cur, esc(n.Label))
		}
		b.WriteString("</nav>")
	}
	if eb := eyebrow(d); eb != "" {
		fmt.Fprintf(&b, "<span class=\"eyebrow\">%s</span>", esc(eb))
	}
	fmt.Fprintf(&b, "<h1>%s</h1>", esc(d.Title))
	lede := d.Lede
	if lede == "" {
		lede = d.Description
	}
	if lede == "" && d.Project == "" {
		lede = d.Subtitle // a Doc that was not framed
	}
	if lede != "" {
		fmt.Fprintf(&b, "<p class=\"lede\">%s</p>", inline(lede))
	}
	b.WriteString("</header>\n")
	if len(d.Stats) > 0 {
		b.WriteString("<section class=\"kpis\">")
		for _, s := range d.Stats {
			cls := "kpi"
			if s.Status != "" {
				cls += " " + esc(s.Status)
			}
			fmt.Fprintf(&b, "<div class=\"%s\"><span class=\"v\">%s</span><span class=\"l\">%s</span>", cls, esc(s.Value), esc(s.Label))
			if s.Note != "" {
				fmt.Fprintf(&b, "<span class=\"n\">%s</span>", inline(s.Note))
			}
			b.WriteString("</div>")
		}
		b.WriteString("</section>\n")
	}
	if len(d.Sections) > 1 {
		b.WriteString("<nav class=\"toc\"><ol>")
		for i, s := range d.Sections {
			fmt.Fprintf(&b, "<li><a href=\"#%s\">%s</a></li>", sectionID(s, i), esc(s.Title))
		}
		b.WriteString("</ol></nav>\n")
	}
	for i, s := range d.Sections {
		fmt.Fprintf(&b, "<section id=\"%s\"><h2>%s</h2>\n", sectionID(s, i), esc(s.Title))
		if s.Intro != "" {
			fmt.Fprintf(&b, "<p class=\"intro\">%s</p>\n", inline(s.Intro))
		}
		for _, bl := range s.Blocks {
			writeHTMLBlock(&b, bl)
		}
		b.WriteString("</section>\n")
	}
	if d.Generated != "" {
		foot := "Generated " + d.Generated
		if d.Mxcli != "" {
			foot += " by mxcli " + d.Mxcli
		}
		fmt.Fprintf(&b, "<footer>%s</footer>\n", esc(foot))
	}
	b.WriteString("</main>\n</body>\n</html>\n")
	_, err := io.WriteString(w, b.String())
	return err
}

// eyebrow is the line above the title: project, report and date.
func eyebrow(d Doc) string {
	var parts []string
	for _, p := range []string{d.Project, d.Kind, dateOf(d.Generated)} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}

// dateOf keeps the date of a "2026-11-19 10:00"-style timestamp.
func dateOf(ts string) string {
	if i := strings.IndexAny(ts, " T"); i > 0 {
		return ts[:i]
	}
	return ts
}

func sectionID(s Section, i int) string {
	if s.ID != "" {
		return s.ID
	}
	return fmt.Sprintf("s%d", i+1)
}

func writeHTMLBlock(b *strings.Builder, bl Block) {
	switch v := bl.(type) {
	case Para:
		fmt.Fprintf(b, "<p>%s</p>\n", inline(v.Text))
	case Bullets:
		b.WriteString("<ul>")
		for _, it := range v.Items {
			fmt.Fprintf(b, "<li>%s</li>", inline(it))
		}
		b.WriteString("</ul>\n")
	case Callout:
		kind := "info"
		if v.Kind == "warn" {
			kind = "warn"
		}
		fmt.Fprintf(b, "<div class=\"callout %s\">%s</div>\n", kind, inline(v.Text))
	case Table:
		b.WriteString("<div class=\"tw\"><table>")
		if v.Caption != "" {
			fmt.Fprintf(b, "<caption>%s</caption>", inline(v.Caption))
		}
		b.WriteString("<thead><tr>")
		for ci, c := range v.Columns {
			if numericColumn(v, ci) {
				fmt.Fprintf(b, "<th class=\"num\">%s</th>", esc(c))
				continue
			}
			fmt.Fprintf(b, "<th>%s</th>", esc(c))
		}
		b.WriteString("</tr></thead><tbody>")
		if len(v.Rows) == 0 {
			fmt.Fprintf(b, "<tr><td colspan=\"%d\" class=\"none\">none</td></tr>", max(1, len(v.Columns)))
		}
		for ri, r := range v.Rows {
			b.WriteString("<tr>")
			for ci := range v.Columns {
				cell := ""
				if ci < len(r) {
					cell = r[ci]
				}
				attrs := ""
				if v.Classes != nil && ri < len(v.Classes) && ci < len(v.Classes[ri]) && v.Classes[ri][ci] != "" {
					attrs += fmt.Sprintf(" class=\"%s\"", esc(v.Classes[ri][ci]))
				}
				if v.Heat != nil && ri < len(v.Heat) && ci < len(v.Heat[ri]) && v.Heat[ri][ci] > 0 {
					attrs += fmt.Sprintf(" style=\"background:color-mix(in srgb, var(--heat) %d%%, transparent)\"", int(10+v.Heat[ri][ci]*70))
				}
				if v.Bars != nil && ri < len(v.Bars) && ci < len(v.Bars[ri]) && v.Bars[ri][ci] != nil {
					fmt.Fprintf(b, "<td%s>%s</td>", attrs, barHTML(*v.Bars[ri][ci], cell))
					continue
				}
				fmt.Fprintf(b, "<td%s>%s</td>", attrs, inline(cell))
			}
			b.WriteString("</tr>")
		}
		b.WriteString("</tbody></table></div>\n")
	case Stacked:
		writeStackedHTML(b, v)
	case Cards:
		b.WriteString("<div class=\"cards\">")
		for _, c := range v.Items {
			if c.Missing != "" {
				fmt.Fprintf(b, "<article class=\"rcard missing\"><h3>%s</h3><p class=\"intro\">Not generated: %s</p></article>", esc(c.Title), inline(c.Missing))
				continue
			}
			fmt.Fprintf(b, "<article class=\"rcard\"><h3><a href=\"%s\">%s</a></h3>", esc(c.Href), esc(c.Title))
			if c.Lede != "" {
				fmt.Fprintf(b, "<p>%s</p>", inline(c.Lede))
			}
			if len(c.Stats) > 0 {
				b.WriteString("<div class=\"minis\">")
				for _, s := range c.Stats {
					cls := "mini"
					if s.Status != "" {
						cls += " " + esc(s.Status)
					}
					fmt.Fprintf(b, "<div class=\"%s\"><span class=\"v\">%s</span><span class=\"l\">%s</span></div>", cls, esc(s.Value), esc(s.Label))
				}
				b.WriteString("</div>")
			}
			fmt.Fprintf(b, "<a class=\"open\" href=\"%s\">Open report →</a></article>", esc(c.Href))
		}
		b.WriteString("</div>\n")
	case Card:
		st := v.Status
		if st == "" {
			st = "none"
		}
		fmt.Fprintf(b, "<article class=\"finding\"><span class=\"tag %s\">%s</span><div>", esc(st), esc(v.Tag))
		if v.StatusText != "" {
			fmt.Fprintf(b, "<span class=\"status %s\">%s</span>", esc(st), esc(v.StatusText))
		}
		fmt.Fprintf(b, "<h3>%s</h3>", inline(v.Title))
		for _, p := range v.Paras {
			fmt.Fprintf(b, "<p>%s</p>", inline(p))
		}
		b.WriteString("</div></article>\n")
	case Figure:
		b.WriteString("<figure>")
		b.WriteString(v.SVG)
		if v.Caption != "" {
			fmt.Fprintf(b, "<figcaption>%s</figcaption>", inline(v.Caption))
		}
		b.WriteString("</figure>\n")
	}
}

// numericColumn reports whether every row marks column ci "num", so its
// heading is right-aligned over its numbers.
func numericColumn(t Table, ci int) bool {
	if len(t.Rows) == 0 || len(t.Classes) < len(t.Rows) {
		return false
	}
	for ri := range t.Rows {
		if ci >= len(t.Classes[ri]) || t.Classes[ri][ci] != "num" {
			return false
		}
	}
	return true
}

// barHTML is an inline bar with its label: inside the fill when there is
// room, after it otherwise.
func barHTML(v Bar, label string) string {
	pct := v.Pct
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	pos := "left:6px"
	if pct < 30 {
		pos = fmt.Sprintf("left:calc(%.0f%% + 6px)", pct)
	}
	cls := "meter"
	if v.Status != "" {
		cls += " " + esc(v.Status)
	}
	return fmt.Sprintf("<div class=\"%s\"><i style=\"width:%.1f%%\"></i><span style=\"%s\">%s</span></div>", cls, pct, pos, inline(label))
}

func writeStackedHTML(b *strings.Builder, v Stacked) {
	b.WriteString("<div class=\"stacked\">")
	if v.Caption != "" {
		fmt.Fprintf(b, "<div class=\"cap\">%s</div>", inline(v.Caption))
	}
	for _, r := range v.Rows {
		total := 0
		for _, n := range r.Parts {
			total += n
		}
		fmt.Fprintf(b, "<div class=\"srow\"><span class=\"sl\">%s</span><span class=\"stack\">", inline(r.Label))
		for i, n := range r.Parts {
			if n <= 0 || i >= len(v.Legend) {
				continue
			}
			fmt.Fprintf(b, "<i style=\"flex:%d;background:%s\" title=\"%s: %d\"></i>", n, colorVar(v.Legend[i].Color), esc(v.Legend[i].Label), n)
		}
		if total == 0 {
			b.WriteString("<i style=\"flex:1;background:var(--none-bg)\"></i>")
		}
		fmt.Fprintf(b, "</span><span class=\"sv\">%s</span></div>", inline(r.Value))
	}
	b.WriteString("<div class=\"legend\">")
	for _, k := range v.Legend {
		fmt.Fprintf(b, "<span><i style=\"background:%s\"></i>%s</span>", colorVar(k.Color), esc(k.Label))
	}
	b.WriteString("</div></div>\n")
}

// colorVar maps a status or series name to its CSS variable.
func colorVar(c string) string {
	switch c {
	case "good", "ok":
		return "var(--good)"
	case "warn":
		return "var(--warn)"
	case "bad":
		return "var(--bad)"
	case "info":
		return "var(--s1)"
	case "none", "":
		return "var(--none-bg)"
	}
	if len(c) == 2 && c[0] == 's' && c[1] >= '1' && c[1] <= '8' {
		return "var(--" + c + ")"
	}
	return "var(--accent)"
}

func esc(s string) string { return html.EscapeString(s) }

// inline escapes text and turns `code` spans into <code>. Nothing else is
// interpreted: annotation text is written by people and must not be able to
// inject markup into the page.
func inline(s string) string {
	parts := strings.Split(s, "`")
	var b strings.Builder
	for i, p := range parts {
		if i%2 == 1 && i < len(parts)-1 {
			b.WriteString("<code>" + esc(p) + "</code>")
			continue
		}
		if i%2 == 1 {
			b.WriteString("`")
		}
		b.WriteString(esc(p))
	}
	return b.String()
}

// pageCSS is the shared look. Colours are tokens, redefined for dark (the
// system's, or data-theme="dark" on <html>); the series slots s1..s8 are a
// fixed categorical palette, used in order, never cycled.
const pageCSS = `
:root{--bg:#f6f5f1;--card:#fff;--fg:#1b1b1c;--muted:#5e6066;--line:#dedcd5;--accent:#2a78d6;--heat:#2a78d6;--hit:#fff1d9;
--s1:#2a78d6;--s2:#eb6834;--s3:#1baf7a;--s4:#eda100;--s5:#e87ba4;--s6:#008300;--s7:#6250d6;--s8:#e34948;
--good:#127a3a;--good-bg:#e6f4ec;--warn:#9a6b00;--warn-bg:#fdf3dc;--bad:#b3261e;--bad-bg:#fbe7e6;--none-bg:#eeede8;
--ok:var(--good);--okbg:var(--good-bg);--warnbg:var(--warn-bg);--badbg:var(--bad-bg);--info:var(--s1);--infobg:#e7f0fb;
--mono:ui-monospace,"JetBrains Mono","SF Mono",Menlo,Consolas,monospace;--body:system-ui,-apple-system,"Segoe UI",sans-serif}
@media (prefers-color-scheme:dark){:root:not([data-theme="light"]){--bg:#131313;--card:#1b1b1c;--fg:#ececec;--muted:#a2a4aa;--line:#2f2f31;--accent:#3987e5;--heat:#3987e5;--hit:#2c2210;color-scheme:dark;
--s1:#3987e5;--s2:#d95926;--s3:#199e70;--s4:#c98500;--s5:#d55181;--s6:#0ca30c;--s7:#9085e9;--s8:#e66767;
--good:#5cc98e;--good-bg:#15291e;--warn:#e0b54a;--warn-bg:#2d2510;--bad:#f07575;--bad-bg:#321615;--none-bg:#242426;--infobg:#16233a}}
:root[data-theme="dark"]{--bg:#131313;--card:#1b1b1c;--fg:#ececec;--muted:#a2a4aa;--line:#2f2f31;--accent:#3987e5;--heat:#3987e5;--hit:#2c2210;color-scheme:dark;
--s1:#3987e5;--s2:#d95926;--s3:#199e70;--s4:#c98500;--s5:#d55181;--s6:#0ca30c;--s7:#9085e9;--s8:#e66767;
--good:#5cc98e;--good-bg:#15291e;--warn:#e0b54a;--warn-bg:#2d2510;--bad:#f07575;--bad-bg:#321615;--none-bg:#242426;--infobg:#16233a}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--fg);font:15px/1.55 var(--body)}
main{max-width:1180px;margin:0 auto;padding:0 16px 64px}main>*{min-width:0}main>header:not(.top){padding-top:28px}
header.top{border-bottom:6px solid var(--accent);padding:32px 0 22px;margin-bottom:28px;display:grid;gap:10px}
.eyebrow{font:600 12px/1 var(--body);letter-spacing:.08em;text-transform:uppercase;color:var(--muted)}
h1{font:700 clamp(28px,5vw,40px)/1.1 var(--mono);margin:0;letter-spacing:-.02em}
h2{font:700 21px/1.25 var(--mono);margin:0 0 10px;letter-spacing:-.01em}h3{font:600 16px/1.3 var(--body);margin:0 0 6px}
p,.lede,.intro,ul{max-width:76ch}p{margin:0 0 10px}.lede{font-size:17px;margin:0}.intro{color:var(--muted)}
a{color:var(--accent)}code{font:12.5px var(--mono);background:var(--card);border:1px solid var(--line);border-radius:4px;padding:0 3px}
nav.reports{display:flex;flex-wrap:wrap;gap:4px 14px;font:500 13px var(--body)}nav.reports a{color:var(--muted);text-decoration:none}
nav.reports a[aria-current]{color:var(--fg);font-weight:600}
nav.toc ol{columns:2;color:var(--muted);font-size:14px;margin:0 0 28px}nav.toc a{color:var(--accent);text-decoration:none}
.kpis{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(220px,100%),1fr));gap:14px;margin:0 0 28px}
.kpi{background:var(--card);border:1px solid var(--line);border-radius:6px;padding:14px 16px;display:grid;gap:5px;align-content:start}
.kpi .v{font:600 24px/1.1 var(--mono);font-variant-numeric:tabular-nums}.kpi .l{color:var(--muted);font-size:13.5px}
.kpi .n{color:var(--muted);font-size:13px}
.kpi.good{border-left:4px solid var(--good)}.kpi.warn{border-left:4px solid var(--warn)}.kpi.bad{border-left:4px solid var(--bad)}
section{margin:0 0 36px}
.tw{overflow-x:auto;margin:8px 0 14px;background:var(--card);border:1px solid var(--line);border-radius:6px;padding:4px 10px}
table{border-collapse:collapse;width:100%;font-size:14px;font-variant-numeric:tabular-nums}
caption{text-align:left;font-weight:600;padding:8px 0 4px}
th,td{text-align:left;padding:7px 9px;border-bottom:1px solid var(--line);vertical-align:top}tbody tr:last-child td{border-bottom:0}
thead th{font:600 11.5px/1.2 var(--body);letter-spacing:.04em;text-transform:uppercase;color:var(--muted);border-bottom:2px solid var(--fg);white-space:nowrap}
td.none{color:var(--muted);font-style:italic}td.num{text-align:right;font:13px var(--mono);white-space:nowrap}th.num{text-align:right}
td.ok,td.good{background:var(--good-bg);color:var(--good)}td.warn{background:var(--warn-bg);color:var(--warn)}td.bad{background:var(--bad-bg);color:var(--bad)}
td.info{background:var(--infobg);color:var(--info)}td.muted{color:var(--muted)}tr.hit{background:var(--hit)}
.callout{border-left:4px solid var(--info);background:var(--infobg);padding:10px 14px;border-radius:4px;margin:8px 0;max-width:96ch}
.callout.warn{border-color:var(--warn);background:var(--warn-bg)}
.meter{position:relative;width:150px;height:18px;background:var(--bg);border:1px solid var(--line);border-radius:3px;overflow:hidden}
.meter i{position:absolute;inset:0 auto 0 0;background:var(--accent)}
.meter.good i{background:var(--good)}.meter.warn i{background:var(--warn)}.meter.bad i{background:var(--bad)}
.meter span{position:absolute;top:0;font:600 11.5px/16px var(--mono);color:var(--fg)}
.meter.good i+span,.meter.warn i+span,.meter.bad i+span{mix-blend-mode:normal}
.stacked{background:var(--card);border:1px solid var(--line);border-radius:6px;padding:12px 14px;margin:8px 0 14px;display:grid;gap:7px}
.stacked .cap{font-weight:600}.srow{display:grid;grid-template-columns:minmax(120px,220px) 1fr auto;gap:12px;align-items:center;font-size:13.5px}
.srow .sl{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.srow .sv{font:12.5px var(--mono);color:var(--muted);white-space:nowrap}
.stack{display:flex;height:16px;border-radius:3px;overflow:hidden;gap:2px}.stack i{display:block;height:100%}
.legend{display:flex;flex-wrap:wrap;gap:6px 18px;font-size:13px;color:var(--muted)}
.legend i{display:inline-block;width:12px;height:12px;border-radius:2px;margin-right:6px;vertical-align:-1px}
.finding{display:grid;grid-template-columns:72px 1fr;gap:16px;margin:0 0 20px}
.tag{display:inline-block;font:700 12px/1 var(--mono);padding:8px 4px;border-radius:3px;text-align:center;height:fit-content;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.tag.good{background:var(--good-bg);color:var(--good)}.tag.warn{background:var(--warn-bg);color:var(--warn)}
.tag.bad{background:var(--bad-bg);color:var(--bad)}.tag.none{background:var(--none-bg);color:var(--muted)}
.status{display:block;font:600 12px/1 var(--body);letter-spacing:.05em;text-transform:uppercase;margin-bottom:6px}
.status.good{color:var(--good)}.status.warn{color:var(--warn)}.status.bad{color:var(--bad)}.status.none{color:var(--muted)}
figure{margin:10px 0 16px;overflow-x:auto}figcaption{color:var(--muted);font-size:13px}
.cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(340px,100%),1fr));gap:18px}
.rcard{background:var(--card);border:1px solid var(--line);border-radius:6px;padding:16px 18px;display:grid;gap:10px;align-content:start}
.rcard h3{font:700 19px/1.2 var(--mono);margin:0}.rcard h3 a{color:var(--fg);text-decoration:none}.rcard p{margin:0;color:var(--muted)}
.rcard.missing{border-style:dashed}.rcard .open{font-weight:600;font-size:14px}
.minis{display:grid;grid-template-columns:repeat(auto-fit,minmax(130px,1fr));gap:8px}
.mini{border:1px solid var(--line);border-radius:4px;padding:8px 10px;display:grid;gap:2px}
.mini .v{font:600 17px/1.15 var(--mono);font-variant-numeric:tabular-nums}.mini .l{color:var(--muted);font-size:12.5px}
.mini.good{border-left:3px solid var(--good)}.mini.warn{border-left:3px solid var(--warn)}.mini.bad{border-left:3px solid var(--bad)}
footer{color:var(--muted);font-size:13px;border-top:1px solid var(--line);padding-top:12px}
svg text{fill:var(--fg);font:11px var(--mono)}svg .muted{fill:var(--muted)}
`
