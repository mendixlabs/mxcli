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
	Stats    []Stat
	Sections []Section
}

// Stat is one headline number.
type Stat struct {
	Label string
	Value string
	Note  string
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
	}
}

func mdCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.ReplaceAll(s, "\n", " ")
}

// ---------------------------------------------------------------- html

// RenderHTML writes the document as one standalone page: embedded CSS, no
// scripts, no external requests, light and dark.
func RenderHTML(w io.Writer, d Doc) error {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	fmt.Fprintf(&b, "<title>%s</title>\n<style>%s</style>\n</head>\n<body>\n<main>\n", esc(d.Title), pageCSS)
	fmt.Fprintf(&b, "<header><h1>%s</h1>", esc(d.Title))
	if d.Subtitle != "" {
		fmt.Fprintf(&b, "<p class=\"sub\">%s</p>", inline(d.Subtitle))
	}
	b.WriteString("</header>\n")
	if len(d.Stats) > 0 {
		b.WriteString("<section class=\"stats\">")
		for _, s := range d.Stats {
			fmt.Fprintf(&b, "<div class=\"stat\"><div class=\"v\">%s</div><div class=\"l\">%s</div>", esc(s.Value), esc(s.Label))
			if s.Note != "" {
				fmt.Fprintf(&b, "<div class=\"n\">%s</div>", inline(s.Note))
			}
			b.WriteString("</div>")
		}
		b.WriteString("</section>\n")
	}
	if len(d.Sections) > 1 {
		b.WriteString("<nav><ol>")
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
	b.WriteString("</main>\n</body>\n</html>\n")
	_, err := io.WriteString(w, b.String())
	return err
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
		for _, c := range v.Columns {
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
				fmt.Fprintf(b, "<td%s>%s</td>", attrs, inline(cell))
			}
			b.WriteString("</tr>")
		}
		b.WriteString("</tbody></table></div>\n")
	case Figure:
		b.WriteString("<figure>")
		b.WriteString(v.SVG)
		if v.Caption != "" {
			fmt.Fprintf(b, "<figcaption>%s</figcaption>", inline(v.Caption))
		}
		b.WriteString("</figure>\n")
	}
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

const pageCSS = `
:root{--bg:#fbfbfa;--fg:#1d1f21;--muted:#62666d;--line:#e2e2df;--card:#fff;--accent:#2f5bd3;--heat:#2f5bd3;
--ok:#1f7a4a;--okbg:#e3f3ea;--warn:#8a5a00;--warnbg:#fbf0d9;--bad:#a12a2a;--badbg:#f8e1e1;--info:#2f5bd3;--infobg:#e6ecfb}
@media (prefers-color-scheme:dark){:root{--bg:#141517;--fg:#e6e6e3;--muted:#9a9ea5;--line:#2b2d31;--card:#1b1c1f;--accent:#8aa8ff;--heat:#5b7fe8;
--ok:#7fd3a3;--okbg:#17301f;--warn:#f0c46a;--warnbg:#33290f;--bad:#f39a9a;--badbg:#3a1a1a;--info:#9db5ff;--infobg:#1c2440}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--fg);font:15px/1.55 system-ui,-apple-system,"Segoe UI",sans-serif}
main{max-width:1180px;margin:0 auto;padding:24px 16px 64px}header h1{margin:0 0 4px;font-size:26px}.sub{color:var(--muted);margin:0 0 20px}
.stats{display:grid;grid-template-columns:repeat(auto-fill,minmax(160px,1fr));gap:10px;margin:0 0 24px}
.stat{background:var(--card);border:1px solid var(--line);border-radius:8px;padding:10px 12px}.stat .v{font-size:22px;font-weight:600}
.stat .l{color:var(--muted);font-size:13px}.stat .n{color:var(--muted);font-size:12px;margin-top:4px}
nav ol{columns:2;color:var(--muted);font-size:14px;margin:0 0 24px}nav a{color:var(--accent);text-decoration:none}
section{margin:0 0 28px}h2{font-size:19px;border-bottom:1px solid var(--line);padding-bottom:4px}.intro{color:var(--muted)}
.tw{overflow-x:auto;margin:8px 0 14px}table{border-collapse:collapse;font-size:13.5px;min-width:50%}
caption{text-align:left;font-weight:600;padding:4px 0}th,td{border:1px solid var(--line);padding:4px 8px;text-align:left;vertical-align:top}
th{background:var(--card);position:sticky;top:0}td.none{color:var(--muted);font-style:italic}
code{font:12.5px ui-monospace,SFMono-Regular,Menlo,monospace;background:var(--card);border:1px solid var(--line);border-radius:4px;padding:0 3px}
.callout{border-left:4px solid var(--info);background:var(--infobg);padding:8px 12px;border-radius:4px;margin:8px 0}
.callout.warn{border-color:var(--warn);background:var(--warnbg)}
td.ok{background:var(--okbg);color:var(--ok)}td.warn{background:var(--warnbg);color:var(--warn)}td.bad{background:var(--badbg);color:var(--bad)}
td.info{background:var(--infobg);color:var(--info)}td.muted{color:var(--muted)}td.num{text-align:right;font-variant-numeric:tabular-nums}
figure{margin:10px 0 16px;overflow-x:auto}figcaption{color:var(--muted);font-size:13px}
svg text{fill:var(--fg);font:11px system-ui,sans-serif}svg .muted{fill:var(--muted)}
`
