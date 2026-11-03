// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// queueRetryProps lists, per retry kind, the properties the model stores
// (Queues$QueueFixedRetry / Queues$QueueExponentialRetry). Every one is
// required: the clause states the whole policy, so nothing is defaulted and a
// describe of it reads back the same.
var queueRetryProps = map[string][]string{
	"Fixed":       {"Retries", "Interval", "IntervalType"},
	"Exponential": {"Retries", "InitialInterval", "MaximumInterval", "IntervalType"},
}

// queueIntervalTypes are Queues$IntervalType's values.
var queueIntervalTypes = []string{"Seconds", "Minutes", "Hours"}

// queueRetryKind maps the word after RETRY to the kind, case-insensitively.
func queueRetryKind(word string) string {
	switch strings.ToLower(word) {
	case "fixed":
		return "Fixed"
	case "exponential":
		return "Exponential"
	}
	return ""
}

// buildQueueRetry reads the RETRY clause of an IN QUEUE clause, or nil. It is
// lenient — ExitQueueRetryClause reports what is wrong with it.
func buildQueueRetry(ctx parser.IQueueClauseContext) *ast.QueueRetry {
	qc, ok := ctx.(*parser.QueueClauseContext)
	if !ok || qc == nil || qc.QueueRetryClause() == nil {
		return nil
	}
	rc := qc.QueueRetryClause().(*parser.QueueRetryClauseContext)
	r := &ast.QueueRetry{}
	if k := rc.IdentifierOrKeyword(); k != nil {
		r.Kind = queueRetryKind(k.GetText())
	}
	for _, p := range rc.AllQueueRetryProperty() {
		pc := p.(*parser.QueueRetryPropertyContext)
		name, value := queueRetryProperty(pc)
		n, _ := strconv.Atoi(value)
		switch strings.ToLower(name) {
		case "retries":
			r.Retries = n
		case "interval":
			r.Interval = n
		case "initialinterval":
			r.InitialInterval = n
		case "maximuminterval":
			r.MaximumInterval = n
		case "intervaltype":
			r.IntervalType = canonicalIntervalType(value)
		}
	}
	return r
}

func queueRetryProperty(pc *parser.QueueRetryPropertyContext) (name, value string) {
	ids := pc.AllIdentifierOrKeyword()
	if len(ids) > 0 {
		name = ids[0].GetText()
	}
	if num := pc.NUMBER_LITERAL(); num != nil {
		value = num.GetText()
	} else if len(ids) > 1 {
		value = ids[1].GetText()
	}
	return name, value
}

func canonicalIntervalType(v string) string {
	for _, t := range queueIntervalTypes {
		if strings.EqualFold(t, v) {
			return t
		}
	}
	return ""
}

// ExitQueueRetryClause refuses a retry clause that does not state a whole,
// valid policy: an unknown kind, a property the kind does not have, a missing
// or repeated one, a count that is not a whole number, or an unknown unit.
func (b *Builder) ExitQueueRetryClause(ctx *parser.QueueRetryClauseContext) {
	tok := ctx.GetStart()
	at := fmt.Sprintf("line %d:%d", tok.GetLine(), tok.GetColumn())
	word := ""
	if k := ctx.IdentifierOrKeyword(); k != nil {
		word = k.GetText()
	}
	kind := queueRetryKind(word)
	if kind == "" {
		b.addError(fmt.Errorf("%s retry %q: a queued call's retry is `retry fixed (…)` or `retry exponential (…)`. "+
			"See: mxcli syntax microflow call", at, word))
		return
	}
	want := queueRetryProps[kind]
	seen := map[string]bool{}
	for _, p := range ctx.AllQueueRetryProperty() {
		pc := p.(*parser.QueueRetryPropertyContext)
		name, value := queueRetryProperty(pc)
		canon := ""
		for _, w := range want {
			if strings.EqualFold(w, name) {
				canon = w
			}
		}
		if canon == "" {
			b.addError(fmt.Errorf("%s retry %s has no property %q; it takes %s",
				at, strings.ToLower(kind), name, strings.Join(want, ", ")))
			continue
		}
		if seen[canon] {
			b.addError(fmt.Errorf("%s retry %s: %s is given twice", at, strings.ToLower(kind), canon))
		}
		seen[canon] = true
		if canon == "IntervalType" {
			if canonicalIntervalType(value) == "" {
				b.addError(fmt.Errorf("%s retry %s: IntervalType is %s, not %q",
					at, strings.ToLower(kind), strings.Join(queueIntervalTypes, ", "), value))
			}
			continue
		}
		n, err := strconv.Atoi(value)
		switch {
		case err != nil || n < 0:
			b.addError(fmt.Errorf("%s retry %s: %s must be a whole number of 0 or more, not %q",
				at, strings.ToLower(kind), canon, value))
		case canon == "Retries" && n < 1:
			// Measured on mxbuild 11.12.6: CE7044 "Retries should be at least 1."
			// on the call activity. An interval of 0 builds clean.
			b.addError(fmt.Errorf("%s retry %s: Retries must be at least 1 — mxbuild rejects 0 "+
				"(CE7044 \"Retries should be at least 1.\"); leave out the retry clause for no retries",
				at, strings.ToLower(kind)))
		}
	}
	var missing []string
	for _, w := range want {
		if !seen[w] {
			missing = append(missing, w)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		b.addError(fmt.Errorf("%s retry %s needs %s — the clause states the whole policy, so nothing is defaulted",
			at, strings.ToLower(kind), strings.Join(missing, ", ")))
	}
}
