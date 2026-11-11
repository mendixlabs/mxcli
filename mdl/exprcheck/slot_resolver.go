// SPDX-License-Identifier: Apache-2.0

package exprcheck

// staticExpectations maps MDL slot paths to their expected expression type.
// Add a new entry whenever a new MDL statement slot is added to the executor.
// Slot paths mirror the AST node + field name, e.g. "IfStmt.Condition".
var staticExpectations = map[string]SlotConstraint{
	"IfStmt.Condition":    {Kind: KindBoolean},
	"WhileStmt.Condition": {Kind: KindBoolean},
	// A FIND/FILTER predicate is a Boolean expression over the item under test,
	// the same shape as a WHILE condition. Mendix reports a non-Boolean one as
	// CE0117 on the list-operation activity.
	"ListOperation.Condition": {Kind: KindBoolean},
	"RetrieveStmt.LimitExpr":  {Kind: KindInteger},
	"RetrieveStmt.OffsetExpr": {Kind: KindInteger},
	"ChangeItem.Value":        {Kind: KindUnknown, ResolveBy: "AttributeOf:Parent"},
	"CreateItem.Value":        {Kind: KindUnknown, ResolveBy: "AttributeOf:Parent"},
	"ReturnStmt.Value":        {Kind: KindUnknown, ResolveBy: "MicroflowReturn"},
	// Resolved per call site: the adapter qualifies the slot with the called
	// flow and parameter (CallArgumentSlot) and checkCallArgumentKind looks up
	// the parameter's type (ako/mxcli#2096). Nothing resolved it before.
	"CallArgument.Value": {Kind: KindUnknown, ResolveBy: "TargetParameter"},
	"LogStmt.Message":    {Kind: KindString},
	// A log message's template parameter must be a String — Mendix does not
	// coerce here, and a non-String one is CE0117 "Error(s) in expression" on
	// the activity. Measured on 11.13.0: an Integer attribute, an integer
	// literal and an object each fail; a String attribute is clean; and
	// toString(...) around any of them is clean (mendixlabs/mxcli#1043).
	"LogStmt.TemplateParam": {Kind: KindString},
	// Send Email (11.13+). Measured on mxbuild 11.15.0-rc.4: an Integer for
	// From, Server Host or Username is CE9528 "The expression for '…' should
	// be of type String."; a String port is CE9528 "should be of type
	// Integer/Long"; an Integer template parameter is CE0117, as for LOG.
	"SendEmailStmt.Address":       {Kind: KindString, Mxbuild: "CE9528 \"The expression for '…' should be of type String\""},
	"SendEmailStmt.Host":          {Kind: KindString, Mxbuild: "CE9528 \"The expression for '…' should be of type String\""},
	"SendEmailStmt.Port":          {Kind: KindInteger, AlsoAccepts: []TypeKind{KindLong}, Mxbuild: "CE9528 \"The expression for 'Server Port' should be of type Integer/Long\""},
	"SendEmailStmt.Credential":    {Kind: KindString, Mxbuild: "CE9528 \"The expression for '…' should be of type String\""},
	"SendEmailStmt.TemplateParam": {Kind: KindString},
	"MfSetStmt.Value":             {Kind: KindUnknown, ResolveBy: "TargetVariable"},
	"DeclareStmt.InitialValue":    {Kind: KindUnknown, ResolveBy: "DeclareType"},
}

type defaultSlotResolver struct{}

func DefaultSlotResolver() SlotResolver { return &defaultSlotResolver{} }

func (r *defaultSlotResolver) Expect(path string) (SlotConstraint, bool) {
	sc, ok := staticExpectations[path]
	return sc, ok
}
