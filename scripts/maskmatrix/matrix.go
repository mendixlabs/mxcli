// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Defect is one way a microflow can be wrong, written so that any two can share a
// microflow: each fragment owns its variable names and its erroneous statement carries
// a caption, so a finding can be traced back to it.
type Defect struct {
	Name     string
	Fragment string
	NoReturn bool // the flow has no final return (the defect is the omission)
}

// Defects are the document-level defects the matrix combines. A reference defect (a
// call that names a parameter that does not exist) is not among them: MxBuild
// reports it first and it hides every other error in every document, so one such
// document would blank the whole matrix. ReferenceProbe covers it instead.
var Defects = []Defect{
	{Name: "UNDEF", Fragment: "@caption 'T_UNDEF' declare $u1 Integer = $Nope;"},
	{Name: "DUP", Fragment: "declare $d1 Integer = 1; @caption 'T_DUP' declare $d1 Integer = 2;"},
	{Name: "TYPE", Fragment: "@caption 'T_TYPE' declare $t1 Integer = 'abc';"},
	{Name: "RETLOOP", Fragment: "loop $i1 in $L begin return 1; end loop;"},
	{Name: "NOVALUE", Fragment: "@caption 'T_NOVALUE' declare $n1 Integer;"},
	{Name: "NORETURN", NoReturn: true},
}

// Module is the module the generated documents live in.
const Module = "Mk"

// Generated is the script and the defects each document holds.
type Generated struct {
	Script string
	Docs   map[string][]string // document name -> the defects in it, in order
}

// Generate writes a script with one document per defect (S_<d>) and one per ordered
// pair (X_<a>__<b>), so the order within a document can be compared too, and a clean
// baseline (Z_baseline) as the control.
func Generate(defects []Defect) Generated {
	g := Generated{Docs: map[string][]string{}}
	var b strings.Builder
	fmt.Fprintf(&b, "mdl 1;\ncreate module %s;\n", Module)
	fmt.Fprintf(&b, "create persistent entity %s.Item ( Name: String(50) );\n", Module)
	fmt.Fprintf(&b, "create microflow %s.Z_baseline ($L: list of %s.Item) returns Integer begin return 1; end;\n", Module, Module)
	emit := func(name string, ds ...Defect) {
		var body strings.Builder
		ret := " return 1;"
		var names []string
		for _, d := range ds {
			names = append(names, d.Name)
			if d.Fragment != "" {
				body.WriteString(d.Fragment + " ")
			}
			if d.NoReturn {
				ret = ""
			}
		}
		fmt.Fprintf(&b, "create microflow %s.%s ($L: list of %s.Item) returns Integer begin %s%s end;\n",
			Module, name, Module, body.String(), strings.TrimSpace(ret))
		g.Docs[name] = names
	}
	for _, d := range defects {
		emit("S_"+d.Name, d)
	}
	for _, a := range defects {
		for _, c := range defects {
			if a.Name != c.Name {
				emit(fmt.Sprintf("X_%s__%s", a.Name, c.Name), a, c)
			}
		}
	}
	g.Script = b.String()
	return g
}

// refusedRE finds the defect a write-time rule refused: exec enforces some rules even
// under --no-check and rolls the whole script back.
var refusedRE = regexp.MustCompile(Module + `\.S_([A-Z]+)' has validation errors`)

// Refused returns the defect exec refused to write, or "".
func Refused(execOutput string) string {
	if m := refusedRE.FindStringSubmatch(execOutput); m != nil {
		return m[1]
	}
	return ""
}

// Finding is one error of a checker, on one document.
type Finding struct{ Code, Element string }

// Findings maps a document name to what a checker reported on it.
type Findings map[string][]Finding

func (f Findings) codes(doc string) map[string]bool {
	out := map[string]bool{}
	for _, x := range f[doc] {
		out[x.Code] = true
	}
	return out
}

// Pair is what a checker reported for a document holding two defects, in one order.
type Pair string

const (
	Both    Pair = "both"
	OnlyA   Pair = "only first"
	OnlyB   Pair = "only second"
	Neither Pair = "neither"
)

// Result is the analysis of one checker's findings.
type Result struct {
	Defects        []string
	Signature      map[string][]string // defect -> the codes its own document raises
	Silent         []string            // defects whose own document the checker leaves clean
	SharedCodes    []string            // codes two defects raise: they cannot be told apart
	Hides          map[string][]string // defect -> defects it hides, in both orders
	OrderDependent [][2]string
	Stages         [][]string // defects layered: everything in a stage hides the next ones
}

// Analyze reads a checker's findings over the generated documents. A defect is visible
// in a document when a code its own single-defect document raises appears there.
func Analyze(defects []Defect, docs map[string][]string, f Findings) Result {
	r := Result{Signature: map[string][]string{}, Hides: map[string][]string{}}
	owners := map[string][]string{}
	for _, d := range defects {
		r.Defects = append(r.Defects, d.Name)
		sig := sortedCodes(f.codes("S_" + d.Name))
		r.Signature[d.Name] = sig
		if len(sig) == 0 {
			r.Silent = append(r.Silent, d.Name)
		}
		for _, c := range sig {
			owners[c] = append(owners[c], d.Name)
		}
	}
	for c, o := range owners {
		if len(o) > 1 {
			r.SharedCodes = append(r.SharedCodes, c)
		}
	}
	sort.Strings(r.SharedCodes)
	visible := func(doc, d string) bool {
		got := f.codes(doc)
		for _, c := range r.Signature[d] {
			if got[c] {
				return true
			}
		}
		return false
	}
	outcome := func(a, b string) Pair {
		va, vb := visible(fmt.Sprintf("X_%s__%s", a, b), a), visible(fmt.Sprintf("X_%s__%s", a, b), b)
		switch {
		case va && vb:
			return Both
		case va:
			return OnlyA
		case vb:
			return OnlyB
		}
		return Neither
	}
	hides := map[string]map[string]bool{}
	for i, a := range r.Defects {
		for _, b := range r.Defects[i+1:] {
			if contains(r.Silent, a) || contains(r.Silent, b) {
				continue
			}
			ab, ba := outcome(a, b), outcome(b, a)
			// ab is the pair (a first, b second); ba is (b first, a second): swap the
			// roles in the second to compare which defect survives.
			survivorAB, survivorBA := survivor(ab, a, b), survivor(ba, b, a)
			if survivorAB != survivorBA {
				r.OrderDependent = append(r.OrderDependent, [2]string{a, b})
				continue
			}
			if survivorAB == "" || survivorAB == "both" {
				continue
			}
			hider, hidden := survivorAB, a
			if hider == a {
				hidden = b
			}
			if hides[hider] == nil {
				hides[hider] = map[string]bool{}
			}
			hides[hider][hidden] = true
		}
	}
	for h, set := range hides {
		for x := range set {
			r.Hides[h] = append(r.Hides[h], x)
		}
		sort.Strings(r.Hides[h])
	}
	r.Stages = layer(r.Defects, r.Silent, r.Hides)
	return r
}

// survivor names the defect a checker still reports for a pair (first, second), "both"
// when it reports both and "" when neither.
func survivor(p Pair, first, second string) string {
	switch p {
	case Both:
		return "both"
	case OnlyA:
		return first
	case OnlyB:
		return second
	}
	return ""
}

// layer orders defects into stages: a defect that nobody hides is stage 1, and one
// hidden by a stage n defect is at least stage n+1. Defects the checker leaves clean
// are not placed.
func layer(defects, silent []string, hides map[string][]string) [][]string {
	level := map[string]int{}
	var depth func(d string, seen map[string]bool) int
	hiddenBy := map[string][]string{}
	for h, xs := range hides {
		for _, x := range xs {
			hiddenBy[x] = append(hiddenBy[x], h)
		}
	}
	depth = func(d string, seen map[string]bool) int {
		if v, ok := level[d]; ok {
			return v
		}
		if seen[d] {
			return 0
		}
		seen[d] = true
		n := 0
		for _, h := range hiddenBy[d] {
			if v := depth(h, seen) + 1; v > n {
				n = v
			}
		}
		level[d] = n
		return n
	}
	var stages [][]string
	for _, d := range defects {
		if contains(silent, d) {
			continue
		}
		n := depth(d, map[string]bool{})
		for len(stages) <= n {
			stages = append(stages, nil)
		}
		stages[n] = append(stages[n], d)
	}
	return stages
}

func sortedCodes(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for c := range m {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}
