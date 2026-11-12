// SPDX-License-Identifier: Apache-2.0

package exprcheck

// Context carries per-call metadata into the parser.
//
// Two usage modes:
//   - Syntax-only: Catalog and Slots are nil; semantic hint rules (E001, E002,
//     E009, E011, …) are silently skipped. Use NewSyntaxContext.
//   - Semantic: Catalog and Slots are non-nil; all hint rules are active.
//     Use NewSemanticContext.
//
// Scope is always optional: when nil, variable type inference falls back to KindUnknown.
type Context struct {
	SlotPath  string
	Microflow string
	File      string
	Line      int
	Column    int

	Scope    Scope
	Catalog  CatalogReader // nil → semantic checks disabled
	Slots    SlotResolver  // nil → slot-kind checks disabled
	Entities EntityScope   // nil → attribute paths stay KindUnknown

	// XPath parses the source as an XPath constraint body rather than a
	// microflow expression: a path may start at the queried entity
	// (`Mod.Assoc/Mod.Entity/Attr`, no variable) and any segment may carry a
	// `[...]` predicate. Neither is valid in an expression, so it is off
	// everywhere but parse_xpath (ako/mxcli#1439).
	XPath bool
}

// IsSemanticEnabled reports whether Catalog and Slots are both wired so that
// semantic hint rules can run. Callers can gate expensive lookups behind this
// check instead of repeating nil guards.
func (c Context) IsSemanticEnabled() bool {
	return c.Catalog != nil && c.Slots != nil
}

// NewSyntaxContext creates a Context for syntax-only parsing (E003, E007, E011
// structurally wired; semantic rules that need Catalog/Slots are inactive).
func NewSyntaxContext(slotPath, microflow string) Context {
	return Context{SlotPath: slotPath, Microflow: microflow}
}

// NewSemanticContext creates a Context with full semantic checking enabled.
// Both slots and catalog must be non-nil.
func NewSemanticContext(slotPath, microflow string, slots SlotResolver, catalog CatalogReader) Context {
	return Context{
		SlotPath:  slotPath,
		Microflow: microflow,
		Slots:     slots,
		Catalog:   catalog,
	}
}

type Parser interface {
	Parse(source string, ctx Context) (RobustExpr, []Hint)
}

type SlotResolver interface {
	Expect(slotPath string) (SlotConstraint, bool)
}

type CatalogReader interface {
	AttributeKind(entityQN, attrName string) (TypeKind, bool)
	AttributeEnumQN(entityQN, attrName string) (string, bool)
	EnumCases(enumQN string) ([]string, bool)
	MicroflowReturn(qn string) (TypeKind, bool)
	MicroflowParam(qn, paramName string) (TypeKind, bool)
}

// FlowParamRefReader is the optional half of a CatalogReader that names what a
// flow parameter's type refers to — the enumeration of an Enumeration
// parameter, the entity of an Object or List one. MicroflowParam answers only
// the kind, which cannot tell one enumeration from another; a call argument of
// the wrong enumeration needs this (ako/mxcli#2096). It is optional so that
// CatalogReader's shape stays as it was: a reader without it leaves
// enumeration identity unchecked.
type FlowParamRefReader interface {
	MicroflowParamRef(qn, paramName string) (string, bool)
}

type Scope interface {
	Lookup(name string) (TypeKind, bool)
}

// EntityScope resolves the object side of an expression: which entity a
// variable holds, and where an association leads from there.
//
// It is separate from Scope because Scope speaks TypeKind, which can say "$P is
// an Object" but not *which* entity — and without that, `$P/Status` cannot be
// resolved to an attribute at all. Keeping it out of CatalogReader too: the
// variable half is per-microflow rather than per-project, and leaving
// CatalogReader's shape untouched keeps it re-syncable from the upstream fork.
//
// A nil Context.Entities is the pre-existing behaviour: attribute paths infer
// KindUnknown and every rule that depends on them stays quiet.
type EntityScope interface {
	// VariableEntity returns the qualified entity name a variable holds. The
	// name is passed without the leading '$'.
	VariableEntity(name string) (string, bool)
	// AssociationTarget returns the entity at the other end of association
	// assocQN traversed from fromEntityQN, and whether assocQN is an
	// association at all. Both directions resolve — a Mendix expression can
	// follow an association from either end.
	AssociationTarget(assocQN, fromEntityQN string) (string, bool)
}

type SlotConstraint struct {
	Kind TypeKind
	// AlsoAccepts lists further kinds the slot takes besides Kind — a Send
	// Email port is "Integer/Long" in mxbuild's own message (CE9528).
	AlsoAccepts []TypeKind
	// Mxbuild is what mxbuild reports for a mismatch in this slot, when it is
	// not the generic CE0117 "Error(s) in expression".
	Mxbuild   string
	ResolveBy string
	Frequency int
	Samples   []string
}

type TypeKind int

const (
	KindUnknown TypeKind = iota
	KindAny
	KindBoolean
	KindString
	KindInteger
	KindLong
	KindDecimal
	KindDateTime
	KindBinary
	KindObject
	KindList
	KindEnumeration
	KindEmpty
)

// Hint is now defined as a type alias to hints.Hint in hint.go (P1.6).
// RobustExpr lives in ast.go (P1.2).
