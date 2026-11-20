// SPDX-License-Identifier: Apache-2.0

// entry.go - the unit the brain stores.
//
// An entry is deliberately small: a title, an optional body, and the anchors
// that make it routable and checkable. Anything derivable from the model is not
// an entry — mxcli answers that with a query, and a store that transcribes it
// is a store that will disagree with the project.
package brain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Kind is what an entry records, and it decides what a failed anchor MEANS.
//
// This is the whole reason requirements are not simply more decisions. A
// decision's anchor points backward at something that exists, so an anchor that
// no longer resolves means the decision is stale. A requirement's anchor points
// forward at something intended, so an anchor that does not resolve means "not
// built yet" — the normal state of a requirement, and measured to fail the
// check outright when requirements were tried as ordinary entries.
type Kind string

const (
	KindDecision    Kind = "decision"
	KindRequirement Kind = "requirement"
)

// Entry is one recorded piece of project knowledge.
type Entry struct {
	ID string `json:"id"`
	// Kind is empty for a decision, so an entry written before requirements
	// existed still reads correctly. Use EntryKind rather than this field.
	Kind Kind `json:"kind,omitempty"`
	// Slice is the deliverable a requirement belongs to. Empty for a decision.
	Slice string `json:"slice,omitempty"`
	// Open marks an unresolved question: a decision that has not been made
	// yet. It lives beside the decisions it will join rather than in a file of
	// its own, because the moment you need to see it is when you are reading
	// what that module already decided.
	//
	// Unlike Kind for a requirement — which is implied by the file — this has
	// to travel with the entry, since a question shares its shard with settled
	// decisions. There is still only ONE copy of the fact, so nothing can
	// drift; what the store forbids is two copies, not one in an odd place.
	Open    bool     `json:"open,omitempty"`
	Title   string   `json:"title"`
	Body    string   `json:"body,omitempty"`
	Anchors []string `json:"anchors,omitempty"`
	// Source is where the entry came from: a document path relative to the
	// project directory, with / separators, plus an optional #locator
	// ("docs/prd.md#L18-21"). Provenance, not content — it is not part of the
	// content key, so the same fact found in two places is still one fact.
	Source string `json:"source,omitempty"`
	Date   string `json:"date"`
}

// EntryKind normalises the zero value: an entry with no kind is a decision.
func (e Entry) EntryKind() Kind {
	if e.Kind == KindRequirement {
		return KindRequirement
	}
	return KindDecision
}

// NewEntry builds an entry from captured text. The first line is the title and
// the remainder is the body, so `brain capture` can take one argument and still
// produce something that renders as a heading plus prose.
func NewEntry(text string, anchors []string, now time.Time) (Entry, error) {
	title, body := splitTitle(text)
	if title == "" {
		return Entry{}, fmt.Errorf("an entry needs at least a title line")
	}
	if _, err := ParseAnchors(anchors); err != nil {
		return Entry{}, err
	}
	e := Entry{
		Title:   title,
		Body:    body,
		Anchors: anchors,
		Date:    now.Format("2006-01-02"),
	}
	e.ID = e.computeID()
	return e, nil
}

// computeID derives the id from the content rather than from a random source.
// That is what makes the duplicate check of A5 free: capturing the same fact
// twice produces the same id, so the queue can refuse it without comparing
// prose. Date is deliberately excluded — the same fact captured on two days is
// the same fact.
func (e Entry) computeID() string {
	h := sha256.New()
	h.Write([]byte(strings.ToLower(strings.Join(strings.Fields(e.Title), " "))))
	if e.Slice != "" {
		h.Write([]byte("\x01" + e.Slice))
	}
	for _, a := range e.Anchors {
		h.Write([]byte("\x00" + a))
	}
	return hex.EncodeToString(h.Sum(nil))[:6]
}

// ParsedAnchors returns the entry's anchors, ignoring any that do not parse.
// Callers that need to report a bad anchor use ParseAnchors directly; this is
// for the paths where a malformed anchor must not stop the rest of the work.
func (e Entry) ParsedAnchors() []Anchor {
	out := make([]Anchor, 0, len(e.Anchors))
	for _, s := range e.Anchors {
		if a, err := ParseAnchor(s); err == nil {
			out = append(out, a)
		}
	}
	return out
}

// NewRequirement builds a requirement: a piece of intended scope belonging to a
// deliverable slice. Its anchors are forward references — they may name things
// that do not exist yet, and usually do.
func NewRequirement(text string, anchors []string, slice string, now time.Time) (Entry, error) {
	e, err := NewEntry(text, anchors, now)
	if err != nil {
		return Entry{}, err
	}
	if !sliceName.MatchString(slice) {
		return Entry{}, fmt.Errorf("slice %q: use letters, digits, '-' and '_' (a leading number orders it: 01-accounts)", slice)
	}
	if err := requirementAnchorsArePlannable(anchors); err != nil {
		return Entry{}, err
	}
	e.Kind, e.Slice = KindRequirement, slice
	// The id folds in the slice, so the same sentence can legitimately appear
	// as a requirement of two slices without the second being refused as a
	// duplicate.
	e.ID = e.computeID()
	return e, nil
}

// requirementAnchorsArePlannable refuses a requirement anchored at a bare
// MODULE.
//
// A requirement's anchors point FORWARD: not resolving means "not built yet",
// which is what lets `brain plan` derive progress from the model instead of from
// a status column somebody has to remember to update. A module anchor breaks
// that, because a module resolves the instant it exists — long before any of the
// work inside it. Measured on ako/ChipCoV1: two requirements anchored at
// `@Maintenance` (the brand theme, and the tablet/phone profiles) both reported
// BUILT after slice 01 with none of their work done.
//
// It is refused rather than warned about because the failure is silent and
// flattering: the plan reports progress that has not happened, and nothing else
// in the system disagrees. A decision anchored at a module is still perfectly
// fine — its anchor points BACKWARD, and "this module exists" is exactly the
// fact a cross-cutting decision about it wants to assert.
func requirementAnchorsArePlannable(anchors []string) error {
	parsed, err := ParseAnchors(anchors)
	if err != nil {
		return err
	}
	for i, a := range parsed {
		if a.Element != "" {
			continue
		}
		return fmt.Errorf(
			"requirement anchor %s names a module, which resolves as soon as the module exists — "+
				"so the requirement would report as built before any of its work is done. "+
				"Anchor at a document the slice actually creates (@%s.SomePage, @%s.ACT_Something) instead",
			anchors[i], a.Module, a.Module)
	}
	return nil
}

// sliceName is deliberately permissive about ordering: a slice is sorted by
// name, so a numeric prefix is how a roadmap gets its order, and that is the
// user's choice rather than a field mxcli maintains.
var sliceName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// NewQuestion builds an open question: something undecided, recorded so it is
// not silently forgotten and rediscovered expensively later.
//
// A question's anchors are not checked for staleness. It may name something
// that does not exist — the question is often precisely whether it should — so
// the rule that keeps decisions honest would report every question as a defect.
func NewQuestion(text string, anchors []string, slice string, now time.Time) (Entry, error) {
	var (
		e   Entry
		err error
	)
	if slice != "" {
		e, err = NewRequirement(text, anchors, slice, now)
	} else {
		e, err = NewEntry(text, anchors, now)
	}
	if err != nil {
		return Entry{}, err
	}
	e.Open = true
	return e, nil
}

// Resolve turns a question into the decision it was always going to become,
// keeping the entry's id so that anything referring to it still resolves, and
// keeping the question itself as the answer's context.
//
// The id therefore stops matching the title it was derived from. That is
// deliberate: an answered question is the same piece of knowledge as the
// question, and re-minting the id would make it a different one.
func (e Entry) Resolve(answer string, now time.Time) (Entry, error) {
	if !e.Open {
		return Entry{}, fmt.Errorf("entry %s is not an open question", e.ID)
	}
	title, body := splitTitle(answer)
	if title == "" {
		return Entry{}, fmt.Errorf("an answer needs at least one line")
	}
	out := e
	out.Open = false
	out.Title = title
	out.Date = now.Format("2006-01-02")
	out.Body = strings.TrimSpace("Resolves: " + e.Title + "\n\n" + strings.TrimSpace(body+"\n\n"+e.Body))
	return out, nil
}

// Shard is where this entry belongs. A requirement goes to its slice; a
// decision to the module of its first anchor.
func (e Entry) Shard() string {
	if e.EntryKind() == KindRequirement {
		return PlanShard(e.Slice)
	}
	return ShardFor(e.ParsedAnchors())
}

// MisfiledIn reports whether the entry sits in the wrong shard, given which of
// its anchors resolved and in which module.
//
// This is a second axis, not a fourth anchor state: an anchor can resolve
// perfectly and the entry still be in the wrong file. The rule is deliberately
// relaxed — *at least one* anchor must belong to the shard, and anchors into
// other modules are fine. A fact like "Sales.Order is committed by
// Finance.ACT_Post" is genuinely two-module, and forcing it into project.md
// would grow the one file that must stay small.
func MisfiledIn(shard string, resolvedModules []string) bool {
	if shard == ProjectShard {
		return false // the catch-all is never misfiled
	}
	if len(resolvedModules) == 0 {
		// Nothing resolved, so there is no evidence about where the entry
		// belongs — and an entry whose only anchor is NotIndexable would
		// otherwise be reported as misfiled, reintroducing through this axis
		// exactly the false staleness that A1's third state exists to prevent.
		// An anchor that names nothing is already a failure on its own axis.
		return false
	}
	for _, m := range resolvedModules {
		if m == shard {
			return false
		}
	}
	return true
}

func splitTitle(text string) (title, body string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", ""
	}
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		return strings.TrimSpace(text[:i]), strings.TrimSpace(text[i+1:])
	}
	return text, ""
}
