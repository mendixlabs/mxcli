// SPDX-License-Identifier: Apache-2.0

package exprcheck

import (
	"sort"
	"strconv"
	"strings"
)

// FunctionSignature is one built-in expression function as the checker knows
// it, for documentation. `mxcli syntax microflow expressions functions` is
// rendered from these, so the reference and the checker cannot disagree about
// which functions exist or what they take (mendixlabs/mxcli#1708).
type FunctionSignature struct {
	Name string
	// Params holds each parameter's kind name; a parameter that accepts more
	// than one kind is written "Decimal|DateTime".
	Params []string
	// MinArgs is how many leading Params are required.
	MinArgs  int
	Variadic bool // the last parameter repeats
	Return   string
	// ReturnNote explains a result kind that is not fixed: round()'s depends on
	// its arity, abs/max/min follow their operands.
	ReturnNote string
}

// String renders the signature as `name(A, B[, C]) -> R`.
func (s FunctionSignature) String() string {
	var b strings.Builder
	b.WriteString(s.Name)
	b.WriteByte('(')
	for i, p := range s.Params {
		switch {
		case i == 0:
		case i >= s.MinArgs:
			b.WriteString("[, ")
		default:
			b.WriteString(", ")
		}
		b.WriteString(p)
	}
	if s.Variadic {
		b.WriteString(", …")
	}
	for i := len(s.Params) - 1; i >= s.MinArgs && i > 0; i-- {
		b.WriteByte(']')
	}
	b.WriteString(") -> ")
	b.WriteString(s.Return)
	return b.String()
}

// Functions returns every verified built-in, sorted by name. Rows marked
// unverified are left out: each was measured as CE0117 (mendixlabs/mxcli#1515)
// and is only kept in the table so MDL044 does not change behaviour by itself.
func Functions() []FunctionSignature {
	out := make([]FunctionSignature, 0, len(funcTable))
	for name, sig := range funcTable {
		if sig.unverified {
			continue
		}
		fs := FunctionSignature{
			Name:     name,
			MinArgs:  sig.min(),
			Variadic: sig.variadic,
			Return:   typeKindName(sig.ret),
		}
		for i := range sig.args {
			fs.Params = append(fs.Params, strings.Join(typeKindNames(sig.accepts(i)), "|"))
		}
		switch {
		case sig.retFromArgs:
			fs.ReturnNote = "Integer when every operand is Integer, else Decimal"
		case len(sig.retByArity) > 0:
			var notes []string
			for argc, k := range sig.retByArity {
				notes = append(notes, typeKindName(k)+" with "+strconv.Itoa(argc)+" argument(s)")
			}
			sort.Strings(notes)
			fs.ReturnNote = strings.Join(notes, "; ")
		}
		out = append(out, fs)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
