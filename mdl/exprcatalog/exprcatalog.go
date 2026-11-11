// SPDX-License-Identifier: Apache-2.0

// Package exprcatalog implements exprcheck.CatalogReader over mxcli's catalog.
//
// exprcheck ships a complete expression type checker whose semantic rules —
// enum-value comparisons, attribute/operand type mismatches, function argument
// types — all run through the CatalogReader seam. Until this package existed
// nothing implemented that seam, so every real invocation ran with a nil Catalog
// and every semantic rule was silently skipped. The checker was in the tree and
// checked nothing.
//
// The whole index is loaded once, in four queries, rather than answering each
// lookup with SQL. A project has thousands of expressions and each one asks
// several questions; per-question round trips would dominate the run even
// against SQLite. This is the memoized reader PROPOSAL_expression_type_checking
// § 4 asks for.
//
// # Failure mode
//
// Every method returns (zero, false) for anything the catalog cannot answer, and
// exprcheck reads that as KindUnknown, which suppresses the downstream rule. A
// stale or partial catalog therefore makes the checker *catch less*, never raise
// a false positive on valid code — the correct direction for an advisory gate.
// A mutating consumer must not inherit that reading; see the same proposal's
// § Fourth consumer.
//
// One known blind spot lands in that same bucket, and is worth knowing before
// wondering why a check did not fire: **the System module contributes no
// enumerations**. mxcli reads zero of them from a project (`show enumerations in
// System` is empty on a stock 11.13 app) because they are platform metadata
// rather than stored units, so an attribute typed by e.g.
// System.WorkflowEventType resolves its enum name but not its cases, and the
// enum-value rule is skipped for it. Attributes typed by a project's or a
// marketplace module's own enumerations resolve fully.
package exprcatalog

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/exprcheck"
)

// Querier is the slice of the catalog this package needs: *sql.DB satisfies it,
// which is what catalog.Catalog.CatalogDB() returns.
type Querier interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

// Reader answers exprcheck's type lookups from a loaded index.
type Reader struct {
	// attrKind is keyed "Module.Entity.Attr" — the same shape callers already
	// hold, so a lookup is one map hit rather than a nested one.
	attrKind map[string]exprcheck.TypeKind
	attrEnum map[string]string
	enumCase map[string][]string
	mfReturn map[string]exprcheck.TypeKind
	mfParam  map[string]exprcheck.TypeKind
	// mfParamRef is the element a parameter's type names — the enumeration of
	// an Enumeration parameter, the entity of an Object or List one — keyed
	// like mfParam. A call argument of the wrong enumeration is only
	// detectable with it (ako/mxcli#2096).
	mfParamRef map[string]string
	// assoc maps an association's qualified name to its two ends, FROM first.
	assoc map[string][2]string
}

var _ exprcheck.CatalogReader = (*Reader)(nil)

// Load builds the index from a catalog database.
//
// A table that does not exist yet — an old cache file written before schema 10 —
// leaves its part of the index empty rather than failing the load: the caller
// gets a checker that catches less, which is the same degradation as a stale
// row and better than no checking at all.
func Load(db Querier) (*Reader, error) {
	if db == nil {
		return nil, fmt.Errorf("exprcatalog: nil catalog")
	}
	r := &Reader{
		attrKind:   map[string]exprcheck.TypeKind{},
		attrEnum:   map[string]string{},
		enumCase:   map[string][]string{},
		mfReturn:   map[string]exprcheck.TypeKind{},
		mfParam:    map[string]exprcheck.TypeKind{},
		mfParamRef: map[string]string{},
		assoc:      map[string][2]string{},
	}
	for _, load := range []func(Querier) error{
		r.loadAttributes, r.loadEnumValues, r.loadMicroflows, r.loadParameters,
		r.loadAssociations,
	} {
		if err := load(db); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *Reader) loadAttributes(db Querier) error {
	rows, err := db.Query(`SELECT EntityQualifiedName, Name, DataType, EnumerationQualifiedName FROM attributes`)
	if err != nil {
		return missingTableOK(err)
	}
	defer rows.Close()
	for rows.Next() {
		var entity, name, dataType, enumQN sql.NullString
		if err := rows.Scan(&entity, &name, &dataType, &enumQN); err != nil {
			return err
		}
		if entity.String == "" || name.String == "" {
			continue
		}
		key := entity.String + "." + name.String
		if k, ok := attributeKind(dataType.String); ok {
			r.attrKind[key] = k
		}
		if enumQN.String != "" {
			r.attrEnum[key] = enumQN.String
		}
	}
	return rows.Err()
}

func (r *Reader) loadEnumValues(db Querier) error {
	rows, err := db.Query(
		`SELECT EnumerationQualifiedName, Name FROM enumeration_values ORDER BY EnumerationQualifiedName, Ordinal`)
	if err != nil {
		return missingTableOK(err)
	}
	defer rows.Close()
	for rows.Next() {
		var enumQN, name sql.NullString
		if err := rows.Scan(&enumQN, &name); err != nil {
			return err
		}
		if enumQN.String == "" || name.String == "" {
			continue
		}
		r.enumCase[enumQN.String] = append(r.enumCase[enumQN.String], name.String)
	}
	return rows.Err()
}

func (r *Reader) loadMicroflows(db Querier) error {
	// The microflows view already covers nanoflows; MicroflowType only filters
	// them apart, and a caller naming a flow does not care which it is.
	rows, err := db.Query(`SELECT QualifiedName, ReturnType FROM microflows`)
	if err != nil {
		return missingTableOK(err)
	}
	defer rows.Close()
	for rows.Next() {
		var qn, returnType sql.NullString
		if err := rows.Scan(&qn, &returnType); err != nil {
			return err
		}
		if qn.String == "" {
			continue
		}
		if k, ok := flowKind(returnType.String); ok {
			r.mfReturn[qn.String] = k
		}
	}
	return rows.Err()
}

func (r *Reader) loadParameters(db Querier) error {
	rows, err := db.Query(`SELECT MicroflowQualifiedName, Name, ParameterType FROM microflow_parameters`)
	if err != nil {
		return missingTableOK(err)
	}
	defer rows.Close()
	for rows.Next() {
		var qn, name, paramType sql.NullString
		if err := rows.Scan(&qn, &name, &paramType); err != nil {
			return err
		}
		if qn.String == "" || name.String == "" {
			continue
		}
		if k, ok := flowKind(paramType.String); ok {
			key := qn.String + "(" + name.String
			r.mfParam[key] = k
			if _, ref, ok := strings.Cut(paramType.String, ":"); ok && ref != "" {
				r.mfParamRef[key] = ref
			}
		}
	}
	return rows.Err()
}

func (r *Reader) loadAssociations(db Querier) error {
	rows, err := db.Query(`SELECT QualifiedName, FromEntity, ToEntity FROM associations`)
	if err != nil {
		return missingTableOK(err)
	}
	defer rows.Close()
	for rows.Next() {
		var qn, from, to sql.NullString
		if err := rows.Scan(&qn, &from, &to); err != nil {
			return err
		}
		// An end that is not recorded leaves the association out entirely, so a
		// path through it reads as unresolved rather than half-resolved.
		if qn.String == "" || from.String == "" || to.String == "" {
			continue
		}
		r.assoc[qn.String] = [2]string{from.String, to.String}
	}
	return rows.Err()
}

// DeclareEnumeration records an enumeration a script creates, replacing what
// the catalog held under that name (a CREATE OR MODIFY redefines the cases).
//
// The catalog is built from the stored project, so without this an attribute
// or enumeration created earlier in the same script resolved to nothing, the
// kind came out Unknown, and every rule keyed on it stayed silent — a genuine
// `change $X (EnumAttr = 'NW')` passed check whenever the enum was new
// (ako/mxcli#969).
func (r *Reader) DeclareEnumeration(enumQN string, cases []string) {
	if enumQN == "" {
		return
	}
	r.enumCase[enumQN] = append([]string(nil), cases...)
}

// DeclareAttribute records an attribute a script creates or retypes. An
// unknown kind removes any stored entry rather than keeping a stale type.
func (r *Reader) DeclareAttribute(entityQN, attrName string, kind exprcheck.TypeKind, enumQN string) {
	if entityQN == "" || attrName == "" {
		return
	}
	key := entityQN + "." + attrName
	delete(r.attrKind, key)
	delete(r.attrEnum, key)
	if kind != exprcheck.KindUnknown {
		r.attrKind[key] = kind
	}
	if kind == exprcheck.KindEnumeration && enumQN != "" {
		r.attrEnum[key] = enumQN
	}
}

// FlowParam is one parameter of a flow a script creates, as DeclareFlow
// records it. Ref names the enumeration or entity the type refers to, when it
// has one.
type FlowParam struct {
	Name string
	Kind exprcheck.TypeKind
	Ref  string
}

// DeclareFlow records the parameters of a microflow or nanoflow a script
// creates, replacing whatever the catalog held for that name — a CREATE OR
// REPLACE can change the parameter list, and a parameter that is gone must not
// keep its stored type.
//
// Without it a call to a flow created in the same script had no parameter to
// compare its arguments with, and the issue's own reproduction — target and
// caller in one script — checked clean (ako/mxcli#2096). A parameter whose
// kind could not be established is left out, so its arguments stay unchecked.
func (r *Reader) DeclareFlow(qn string, params []FlowParam) {
	if qn == "" {
		return
	}
	prefix := qn + "("
	for k := range r.mfParam {
		if strings.HasPrefix(k, prefix) {
			delete(r.mfParam, k)
		}
	}
	for k := range r.mfParamRef {
		if strings.HasPrefix(k, prefix) {
			delete(r.mfParamRef, k)
		}
	}
	for _, p := range params {
		name := strings.TrimPrefix(p.Name, "$")
		if name == "" || p.Kind == exprcheck.KindUnknown {
			continue
		}
		r.mfParam[prefix+name] = p.Kind
		if p.Ref != "" {
			r.mfParamRef[prefix+name] = p.Ref
		}
	}
}

// KnowsEntity reports whether any attribute of entityQN is indexed. It is how
// a script's bare `Module.Name` parameter type — which the parser cannot tell
// from an enumeration — is settled as an entity; an entity with no attributes
// stays unsettled, and its arguments unchecked.
func (r *Reader) KnowsEntity(entityQN string) bool {
	if entityQN == "" {
		return false
	}
	prefix := entityQN + "."
	for k := range r.attrKind {
		if strings.HasPrefix(k, prefix) && !strings.Contains(k[len(prefix):], ".") {
			return true
		}
	}
	return false
}

// AssociationTarget returns the entity at the other end of an association.
//
// Both directions resolve: a Mendix expression follows an association from its
// FROM end and from its TO end alike, and the path looks the same either way.
// The signature matches xpathrefs.Model so the two resolvers can converge.
func (r *Reader) AssociationTarget(assocQN, fromEntityQN string) (string, bool) {
	ends, ok := r.assoc[assocQN]
	if !ok || fromEntityQN == "" {
		return "", false
	}
	switch fromEntityQN {
	case ends[0]:
		return ends[1], true
	case ends[1]:
		return ends[0], true
	}
	// A self-association resolves above; anything else means the path does not
	// start where it claims to.
	return "", false
}

// AttributeKind returns the kind of Module.Entity.Attr.
func (r *Reader) AttributeKind(entityQN, attrName string) (exprcheck.TypeKind, bool) {
	k, ok := r.attrKind[entityQN+"."+attrName]
	return k, ok
}

// AttributeEnumQN returns which enumeration an enumeration-typed attribute uses.
func (r *Reader) AttributeEnumQN(entityQN, attrName string) (string, bool) {
	qn, ok := r.attrEnum[entityQN+"."+attrName]
	return qn, ok
}

// EnumCases returns an enumeration's value names in model order.
func (r *Reader) EnumCases(enumQN string) ([]string, bool) {
	cases, ok := r.enumCase[enumQN]
	if !ok {
		return nil, false
	}
	// Copy: the index outlives any one check, and a caller that sorted or
	// truncated the slice in place would corrupt every later lookup.
	out := make([]string, len(cases))
	copy(out, cases)
	return out, true
}

// MicroflowReturn returns a microflow's or nanoflow's return kind.
func (r *Reader) MicroflowReturn(qn string) (exprcheck.TypeKind, bool) {
	k, ok := r.mfReturn[qn]
	return k, ok
}

// MicroflowParam returns the kind of one named parameter.
func (r *Reader) MicroflowParam(qn, paramName string) (exprcheck.TypeKind, bool) {
	k, ok := r.mfParam[qn+"("+strings.TrimPrefix(paramName, "$")]
	return k, ok
}

// MicroflowParamRef returns the enumeration or entity a parameter's type
// names: "Mod.Color" for an Enumeration(Mod.Color) parameter. It is the
// optional exprcheck.FlowParamRefReader half of the reader.
func (r *Reader) MicroflowParamRef(qn, paramName string) (string, bool) {
	ref, ok := r.mfParamRef[qn+"("+strings.TrimPrefix(paramName, "$")]
	return ref, ok && ref != ""
}

// attributeKind maps a domain-model attribute's stored type name.
//
// The names come from domainmodel's GetTypeName, which is the bare kind — an
// enumeration attribute reports "Enumeration" and says nothing about which one,
// hence the separate AttributeEnumQN lookup.
func attributeKind(name string) (exprcheck.TypeKind, bool) {
	switch name {
	case "String", "HashedString":
		// A hashed string is still a String everywhere an expression can touch
		// it; only the storage differs.
		return exprcheck.KindString, true
	case "Integer":
		return exprcheck.KindInteger, true
	case "Long", "AutoNumber":
		// AutoNumber is a Long that the runtime assigns.
		return exprcheck.KindLong, true
	case "Decimal":
		return exprcheck.KindDecimal, true
	case "Boolean":
		return exprcheck.KindBoolean, true
	case "DateTime", "Date":
		return exprcheck.KindDateTime, true
	case "Binary":
		return exprcheck.KindBinary, true
	case "Enumeration":
		return exprcheck.KindEnumeration, true
	}
	return exprcheck.KindUnknown, false
}

// flowKind maps a microflow parameter or return type as the catalog encodes it,
// which carries the referenced element after a colon ("Object:Mod.Entity").
func flowKind(name string) (exprcheck.TypeKind, bool) {
	base, _, _ := strings.Cut(name, ":")
	switch base {
	case "Object":
		return exprcheck.KindObject, true
	case "List":
		return exprcheck.KindList, true
	case "Enumeration":
		return exprcheck.KindEnumeration, true
	case "Void":
		// A void microflow has no value, and exprcheck has no kind that says so.
		// Reporting "not found" makes it unknown, which suppresses rules rather
		// than inventing a type — calling a void flow in a value position is a
		// real error, but it is not this seam's to diagnose.
		return exprcheck.KindUnknown, false
	}
	return attributeKind(base)
}

// missingTableOK turns "no such table" into a soft miss.
//
// NewFromFile applies the current schema to an old cache with CREATE TABLE IF
// NOT EXISTS, so this should not fire in practice — but a reader that hard-fails
// on one absent table would take the whole check down over a lookup it is
// designed to survive losing.
func missingTableOK(err error) error {
	if err != nil && strings.Contains(err.Error(), "no such table") {
		return nil
	}
	return err
}
