// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// SchemaVersion is the version of the envelope and of every kind's data. A
// renderer refuses a newer one rather than drop fields it does not know.
const SchemaVersion = 1

// Kinds lists the report kinds, in the order the help text names them.
var Kinds = []string{"architecture", "tests", "security", "performance", "traceability"}

// Envelope is what `--format json` writes and `--from` reads back: the
// collected data plus everything a reader needs to trust it. Method and Limits
// travel with the data rather than being added at render time, because they
// describe how THIS run measured — a page re-rendered next month must still
// say what the measurement could not see.
type Envelope struct {
	Kind      string `json:"kind"`
	Schema    int    `json:"schema"`
	Project   string `json:"project"`
	Generated string `json:"generated"`
	Mxcli     string `json:"mxcli,omitempty"`
	// Method is how each number on the page was obtained.
	Method []string `json:"method"`
	// Limits is what the report does not cover, and where a number is known
	// to under- or over-count.
	Limits []string `json:"limits"`
	// Warnings are preconditions this run did not meet (an unrecorded
	// database state, leftover test-runner files). They are shown at the top
	// of the page, not buried in the method.
	Warnings []string        `json:"warnings,omitempty"`
	Data     json.RawMessage `json:"data"`
}

// NewEnvelope wraps a kind's data.
func NewEnvelope(kind, project, generated, version string, data any, method, limits, warnings []string) (*Envelope, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return &Envelope{
		Kind: kind, Schema: SchemaVersion, Project: project, Generated: generated, Mxcli: version,
		Method: method, Limits: limits, Warnings: warnings, Data: raw,
	}, nil
}

// ReadEnvelope loads a report written by `--format json`, checking it is the
// kind the caller is about to render.
func ReadEnvelope(path, wantKind string) (*Envelope, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var env Envelope
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, fmt.Errorf("%s: not a report JSON file: %w", path, err)
	}
	if env.Kind != wantKind {
		return nil, fmt.Errorf("%s holds a %q report, not %q", path, env.Kind, wantKind)
	}
	if env.Schema > SchemaVersion {
		return nil, fmt.Errorf("%s was written by a newer mxcli (schema %d, this one reads %d)", path, env.Schema, SchemaVersion)
	}
	return &env, nil
}

// Decode unmarshals the envelope's data into v.
func (e *Envelope) Decode(v any) error { return json.Unmarshal(e.Data, v) }

// WriteJSON writes the envelope, indented.
func (e *Envelope) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(e)
}

// ---------------------------------------------------------------- annotations

// Annotations is the part of a report a person writes: names for clusters,
// verdicts on findings and gaps, notes and recommendations. It is keyed by the
// stable key the page prints next to each annotatable item ("cluster:…",
// "finding:…", "gap:…", "rule:…"), so it survives re-measuring as long as the
// thing it names still exists — and an entry that no longer matches anything
// is listed on the page rather than silently dropped.
type Annotations struct {
	Title           string          `json:"title,omitempty" yaml:"title,omitempty"`
	Summary         string          `json:"summary,omitempty" yaml:"summary,omitempty"`
	Items           map[string]Note `json:"items,omitempty" yaml:"items,omitempty"`
	Recommendations []string        `json:"recommendations,omitempty" yaml:"recommendations,omitempty"`

	used map[string]bool
}

// Note annotates one item. Name renames it (a cluster); Status is a verdict
// (open, fixed, accepted, intended, real — each kind says which apply); Note is
// free text shown next to it.
type Note struct {
	Name   string `json:"name,omitempty" yaml:"name,omitempty"`
	Status string `json:"status,omitempty" yaml:"status,omitempty"`
	Note   string `json:"note,omitempty" yaml:"note,omitempty"`
}

// LoadAnnotations reads a JSON or YAML annotations file. An empty path is no
// annotations, which every kind handles.
func LoadAnnotations(path string) (*Annotations, error) {
	a := &Annotations{}
	if path == "" {
		return a, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		err = yaml.Unmarshal(b, a)
	default:
		err = json.Unmarshal(b, a)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return a, nil
}

// Get returns the note for key and records that the key matched something.
func (a *Annotations) Get(key string) (Note, bool) {
	if a == nil || a.Items == nil {
		return Note{}, false
	}
	n, ok := a.Items[key]
	if ok {
		if a.used == nil {
			a.used = map[string]bool{}
		}
		a.used[key] = true
	}
	return n, ok
}

// Peek returns the note for key without marking it used.
func (a *Annotations) Peek(key string) (Note, bool) {
	if a == nil || a.Items == nil {
		return Note{}, false
	}
	n, ok := a.Items[key]
	return n, ok
}

// Unused lists the annotation keys no item on the page matched, sorted. They
// are shown, because a verdict that silently stops applying is how a fixed
// finding reappears as open without anyone noticing.
func (a *Annotations) Unused() []string {
	if a == nil {
		return nil
	}
	var out []string
	for k := range a.Items {
		if !a.used[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------- page frame

// Frame adds the sections every kind ends with — the person's summary and
// recommendations, the preconditions not met, the method, the limits and any
// annotation key that matched nothing — around a kind's own sections.
func Frame(env *Envelope, ann *Annotations, d Doc) Doc {
	if ann != nil && ann.Title != "" {
		d.Title = ann.Title
	}
	var head []Section
	if len(env.Warnings) > 0 {
		var bl []Block
		for _, w := range env.Warnings {
			bl = append(bl, Callout{Kind: "warn", Text: w})
		}
		head = append(head, Section{ID: "preconditions", Title: "Preconditions not met", Blocks: bl})
	}
	if ann != nil && ann.Summary != "" {
		head = append(head, Section{ID: "summary", Title: "Summary", Blocks: []Block{Para{Text: ann.Summary}}})
	}
	d.Sections = append(head, d.Sections...)
	if ann != nil && len(ann.Recommendations) > 0 {
		d.Sections = append(d.Sections, Section{ID: "recommendations", Title: "Recommendations",
			Blocks: []Block{Bullets{Items: ann.Recommendations}}})
	}
	d.Sections = append(d.Sections,
		Section{ID: "method", Title: "How this was measured", Blocks: []Block{Bullets{Items: env.Method}}},
		Section{ID: "limits", Title: "What this does not cover", Blocks: []Block{Bullets{Items: env.Limits}}},
	)
	if unused := ann.Unused(); len(unused) > 0 {
		d.Sections = append(d.Sections, Section{ID: "stale-annotations", Title: "Annotations that matched nothing",
			Intro:  "These keys are in the annotations file but name nothing on this page. The item was renamed, fixed or removed — update or drop the entry.",
			Blocks: []Block{Bullets{Items: codeList(unused)}}})
	}
	sub := fmt.Sprintf("%s · generated %s", env.Project, env.Generated)
	if env.Mxcli != "" {
		sub += " · mxcli " + env.Mxcli
	}
	if d.Subtitle != "" {
		sub = d.Subtitle + " · " + sub
	}
	d.Subtitle = sub
	return d
}

// Render writes the framed document in the given format ("markdown" or "html").
func Render(w io.Writer, format string, d Doc) error {
	switch format {
	case "html":
		return RenderHTML(w, d)
	case "markdown", "md", "":
		return RenderMarkdown(w, d)
	default:
		return fmt.Errorf("unknown format %q (want html, markdown or json)", format)
	}
}

func codeList(items []string) []string {
	out := make([]string, len(items))
	for i, s := range items {
		out[i] = "`" + s + "`"
	}
	return out
}
