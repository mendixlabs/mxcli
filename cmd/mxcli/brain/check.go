// SPDX-License-Identifier: Apache-2.0

// check.go - does the store still describe the project?
//
// Two independent questions, and conflating them is the mistake this file
// exists to avoid:
//
//   - Does each anchor still resolve? Four states, and only one is a failure.
//     An anchor to a document type the catalog does not index resolves as
//     *missing* through the catalog alone, which is a false staleness signal —
//     so a second, type-agnostic lookup separates "gone" from "not indexed".
//     With no model at all, nothing can be checked, which is its own state.
//   - Is the entry in the right shard? A separate axis, not a fourth state: an
//     anchor can resolve perfectly and the entry still sit in the wrong file.
package brain

import (
	"encoding/json"
	"fmt"
	"sort"
)

// AnchorState is the outcome of resolving one anchor.
type AnchorState int

const (
	// Resolved: the anchor names something the catalog knows.
	Resolved AnchorState = iota
	// NotFound: the anchor names nothing in the project. The only failure.
	NotFound
	// NotIndexable: the target exists but is of a type the catalog's objects
	// view does not cover. Reported, never failed — treating it as missing
	// would make `check` demand edits to entries that are perfectly current.
	NotIndexable
	// Unverifiable: there is no model to check against — the brain was started
	// before the project (ako/mxcli#1948). Neither a failure nor "built": a
	// requirement counts as planned, a decision is simply not checked.
	Unverifiable
)

// MarshalJSON writes the state's name. The zero value of an int is a state
// here, so an ordinal contract would make "0" mean `resolved` today and
// something else the moment a state is inserted above it — a change nothing
// downstream could notice. The three states are the substance of the check, so
// they travel as names.
func (s AnchorState) MarshalJSON() ([]byte, error) { return json.Marshal(s.String()) }

// UnmarshalJSON accepts what MarshalJSON writes, so a report round-trips.
func (s *AnchorState) UnmarshalJSON(b []byte) error {
	var name string
	if err := json.Unmarshal(b, &name); err != nil {
		return err
	}
	switch name {
	case "resolved":
		*s = Resolved
	case "not found":
		*s = NotFound
	case "not indexable":
		*s = NotIndexable
	case "unverifiable":
		*s = Unverifiable
	default:
		return fmt.Errorf("unknown anchor state %q", name)
	}
	return nil
}

func (s AnchorState) String() string {
	switch s {
	case Resolved:
		return "resolved"
	case NotFound:
		return "not found"
	case Unverifiable:
		return "unverifiable"
	default:
		return "not indexable"
	}
}

// Resolution is what a Resolver reports about one anchor.
type Resolution struct {
	State AnchorState
	// Module is the module the target actually lives in, which is what the
	// misfiling check compares against. Empty unless State is Resolved.
	Module string
	// Kind is the target's type, for the report ("entity", "microflow").
	Kind string
}

// Resolver answers anchors against a project. It is an interface so the check
// logic is testable without a .mpr — the states that matter are the awkward
// ones, and a fixture project cannot readily produce a NotIndexable.
type Resolver interface {
	Resolve(Anchor) (Resolution, error)
}

// NoModel is the Resolver for a brain whose project does not exist yet. It
// answers every anchor Unverifiable rather than NotFound: a null resolver that
// said NotFound would report the whole store as stale when the truth is that
// nothing could be checked.
type NoModel struct{}

// Resolve reports that there is nothing to resolve against.
func (NoModel) Resolve(Anchor) (Resolution, error) {
	return Resolution{State: Unverifiable}, nil
}

// AnchorFinding is one anchor's outcome.
type AnchorFinding struct {
	Shard   string      `json:"shard"`
	EntryID string      `json:"entry_id"`
	Title   string      `json:"title"`
	Anchor  string      `json:"anchor"`
	State   AnchorState `json:"state"`
	Kind    string      `json:"kind,omitempty"`
}

// OpenQuestion is something the project has not decided yet.
type OpenQuestion struct {
	Shard   string `json:"shard"`
	EntryID string `json:"entry_id"`
	Title   string `json:"title"`
}

// MisfiledFinding is an entry sitting in a shard none of its anchors belong to.
type MisfiledFinding struct {
	Shard   string `json:"shard"`
	EntryID string `json:"entry_id"`
	Title   string `json:"title"`
	// Belongs is the shard it should be in, from its first resolved anchor.
	Belongs string `json:"belongs,omitempty"`
}

// SliceProgress is a slice's requirements counted against the model. Every
// figure is derived from resolving anchors, so nothing here is self-reported
// and no one has to maintain a status column that will go stale.
type SliceProgress struct {
	Slice string `json:"slice"`
	// Built is requirements whose anchors all resolve — the thing exists.
	Built int `json:"built"`
	// Planned is requirements with at least one anchor that does not resolve
	// yet. Not a failure: that is what a requirement is until it is built.
	Planned int `json:"planned"`
	// Questions is open questions filed against this slice — scope that is not
	// settled. They are not requirements and are not counted as either built
	// or planned; counting an unanswered question as outstanding work would
	// overstate the slice.
	Questions int `json:"questions"`
	// Unanchored is requirements with no anchor at all. They cannot be
	// measured, and are counted apart rather than silently called planned.
	Unanchored int `json:"unanchored"`
}

// Total is every requirement in the slice. Open questions are excluded: they
// are not scope until they are answered.
func (p SliceProgress) Total() int { return p.Built + p.Planned + p.Unanchored }

// Report is what `brain check` prints and exits on.
type Report struct {
	Shards    []string `json:"shards"`
	Entries   int      `json:"entries"`
	Anchors   int      `json:"anchors"`
	ResolvedN int      `json:"resolved"`
	// Unverifiable counts anchors that could not be checked because there is
	// no model. Counted rather than listed: with no model every anchor is in
	// this state, and listing them all would bury the one line that matters.
	Unverifiable int `json:"unverifiable"`
	// ModelAbsent says the check ran with no project to resolve against, so a
	// consumer does not have to infer it from a count.
	ModelAbsent bool `json:"model_absent,omitempty"`
	// Findings carries NotFound and NotIndexable anchors only; a resolved or
	// unverifiable anchor is not a finding.
	Findings []AnchorFinding   `json:"findings"`
	Misfiled []MisfiledFinding `json:"misfiled"`
	// Malformed names entry blocks whose metadata line could not be read.
	Malformed []string `json:"malformed"`
	// MissingSources names entries whose source document is not there. It
	// fails the check: provenance pointing at a file nobody has is a
	// reference to nothing, the same defect as an anchor that names nothing.
	MissingSources []SourceFinding `json:"missing_sources,omitempty"`
	Slices         []SliceProgress `json:"slices"`
	Open           []OpenQuestion  `json:"open"`
	// Requirements is each checked requirement with each anchor's
	// resolution: the per-requirement form of Slices, so a consumer (`brain
	// plan --json`, `report traceability`) reads "built" from the one place it
	// is defined instead of re-deriving it (ako/mxcli#2226).
	Requirements []RequirementResult `json:"requirements,omitempty"`
	// UnknownRequirementRefs are decisions whose verdict line names a
	// requirement id the plan does not have. It fails the check: a verdict on
	// nothing is a reference to nothing.
	UnknownRequirementRefs []VerdictRef `json:"unknown_requirement_refs,omitempty"`
}

// RequirementResult is one requirement and how each of its anchors resolved.
type RequirementResult struct {
	ID      string         `json:"id"`
	Slice   string         `json:"slice"`
	Title   string         `json:"title"`
	Built   bool           `json:"built"`
	Anchors []AnchorResult `json:"anchors"`
}

// AnchorResult is one anchor's resolution.
type AnchorResult struct {
	Anchor string      `json:"anchor"`
	State  AnchorState `json:"state"`
	Kind   string      `json:"kind,omitempty"`
}

// MarshalJSON adds a derived "failed" alongside the report's contents. A
// consumer deciding whether to advance should not have to re-implement which of
// these states are defects and which are information — that rule lives in
// Failed() and is easy to get subtly wrong from outside (a not-indexable anchor
// and an open question both look like problems and neither is one).
//
// It is computed here rather than stored, so it cannot disagree with Failed().
func (r Report) MarshalJSON() ([]byte, error) {
	type report Report // shed the method, keep the tags
	return json.Marshal(struct {
		report
		Failed bool `json:"failed"`
	}{report(r), r.Failed()})
}

// Failed reports whether the check should exit non-zero.
//
// Only two things fail: an anchor that names nothing at all, and an entry in
// the wrong shard. A NotIndexable anchor is information, not a defect — see the
// constant's comment.
func (r Report) Failed() bool {
	if len(r.Misfiled) > 0 || len(r.Malformed) > 0 || len(r.MissingSources) > 0 || len(r.UnknownRequirementRefs) > 0 {
		return true
	}
	for _, f := range r.Findings {
		if f.State == NotFound {
			return true
		}
	}
	return false
}

// Check validates the given shards. Passing a subset is how `--changed` avoids
// paying for shards a diff did not touch.
func Check(s *Store, r Resolver, shards []string) (Report, error) {
	_, noModel := r.(NoModel)
	rep := Report{Shards: shards, ModelAbsent: noModel}
	for _, shard := range shards {
		entries, malformed, err := s.LoadShard(shard)
		if err != nil {
			return rep, err
		}
		for _, m := range malformed {
			rep.Malformed = append(rep.Malformed, shard+": "+m)
		}
		rep.MissingSources = append(rep.MissingSources, s.missingSources(shard, entries)...)
		if IsPlanShard(shard) {
			progress, err := checkSlice(r, shard, entries)
			if err != nil {
				return rep, err
			}
			rep.Entries += len(entries)
			rep.Anchors += progress.anchors
			rep.ResolvedN += progress.resolved
			rep.Unverifiable += progress.unverifiable
			rep.Slices = append(rep.Slices, progress.SliceProgress)
			rep.Open = append(rep.Open, progress.open...)
			rep.Requirements = append(rep.Requirements, progress.requirements...)
			continue
		}
		unknown, err := s.unknownVerdictRefs(shard, entries)
		if err != nil {
			return rep, err
		}
		rep.UnknownRequirementRefs = append(rep.UnknownRequirementRefs, unknown...)
		for _, e := range entries {
			rep.Entries++
			if e.Open {
				// A question's anchors are not checked. It may name something
				// that does not exist — often the question IS whether it
				// should — so the staleness rule that keeps decisions honest
				// would report every question as a defect.
				rep.Open = append(rep.Open, OpenQuestion{Shard: shard, EntryID: e.ID, Title: e.Title})
				continue
			}
			var resolvedModules []string
			for _, a := range e.ParsedAnchors() {
				rep.Anchors++
				res, err := r.Resolve(a)
				if err != nil {
					return rep, err
				}
				switch res.State {
				case Resolved:
					rep.ResolvedN++
					resolvedModules = append(resolvedModules, res.Module)
					continue
				case Unverifiable:
					rep.Unverifiable++
					continue
				}
				rep.Findings = append(rep.Findings, AnchorFinding{
					Shard: shard, EntryID: e.ID, Title: e.Title,
					Anchor: a.String(), State: res.State, Kind: res.Kind,
				})
			}
			if MisfiledIn(shard, resolvedModules) {
				rep.Misfiled = append(rep.Misfiled, MisfiledFinding{
					Shard: shard, EntryID: e.ID, Title: e.Title,
					Belongs: belongsIn(resolvedModules),
				})
			}
		}
	}
	return rep, nil
}

type sliceCounts struct {
	SliceProgress
	anchors, resolved, unverifiable int
	open                            []OpenQuestion
	requirements                    []RequirementResult
}

// checkSlice counts a slice's requirements against the model. It records no
// findings and no misfiling, and that is the point rather than an omission:
//
//   - A requirement's anchor points FORWARD. Not resolving means not built,
//     which is the normal state of a requirement and must never fail a check —
//     measured: filed as an ordinary entry, one unbuilt requirement took
//     `brain check` to exit 1.
//   - A slice spans modules by design ("approvals" touches Sales and Finance),
//     so the misfiling rule that keeps decisions honest does not apply.
func checkSlice(r Resolver, shard string, entries []Entry) (sliceCounts, error) {
	out := sliceCounts{SliceProgress: SliceProgress{Slice: SliceOf(shard)}}
	for _, e := range entries {
		if e.Open {
			out.Questions++
			out.open = append(out.open, OpenQuestion{Shard: shard, EntryID: e.ID, Title: e.Title})
			continue
		}
		anchors := e.ParsedAnchors()
		if len(anchors) == 0 {
			out.Unanchored++
			out.requirements = append(out.requirements, RequirementResult{ID: e.ID, Slice: out.Slice, Title: e.Title})
			continue
		}
		built := true
		rr := RequirementResult{ID: e.ID, Slice: out.Slice, Title: e.Title}
		for _, a := range anchors {
			out.anchors++
			res, err := r.Resolve(a)
			if err != nil {
				return out, err
			}
			rr.Anchors = append(rr.Anchors, AnchorResult{Anchor: a.String(), State: res.State, Kind: res.Kind})
			// NotIndexable counts as built: the thing is there, the catalog
			// simply does not index its type. Calling it planned would report
			// finished work as outstanding.
			//
			// Unverifiable does NOT: with no model nothing is built, and
			// letting it fall through to the built branch with NotIndexable
			// reports a project that does not exist yet as finished.
			switch res.State {
			case NotFound:
				built = false
				continue
			case Unverifiable:
				built = false
				out.unverifiable++
				continue
			}
			out.resolved++
		}
		rr.Built = built
		out.requirements = append(out.requirements, rr)
		if built {
			out.Built++
		} else {
			out.Planned++
		}
	}
	return out, nil
}

// belongsIn names the shard an entry should have gone to. With no resolved
// anchor at all there is nothing to suggest, and the empty string says so
// rather than guessing.
func belongsIn(resolved []string) string {
	if len(resolved) == 0 {
		return ""
	}
	uniq := map[string]bool{}
	for _, m := range resolved {
		uniq[m] = true
	}
	keys := make([]string, 0, len(uniq))
	for k := range uniq {
		keys = append(keys, k)
	}
	sort.Strings(keys) // deterministic output; maps iterate randomly
	return keys[0]
}
