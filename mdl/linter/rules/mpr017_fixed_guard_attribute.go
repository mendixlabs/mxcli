// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// FixedGuardAttributeRule flags a Boolean attribute that a decision tests but
// that can only ever hold one value, so the decision's outcome is fixed.
//
// From the benchmark that motivated ako/mxcli#2403, found in all three apps read:
//
//	"The tick, the housekeeping and the scheduled reset check a flag so that
//	 they do not work during the data reset, but no flow ever sets it to true;
//	 the reset only sets it to false. So the tick can work on data while the
//	 reset deletes and reloads it."
//
// Nothing in the toolchain reports this. The model is valid, the build is
// clean, and the flow reads as though it has a safety interlock. It does not:
// the branch it guards is unreachable.
//
// # Why "one reachable value" and not "never written"
//
// #2403 described this as "read in conditions but never written", which is the
// shape it takes when a flag is forgotten entirely. But the case above is NOT
// that — the flag IS written, to false, by the reset. A "never written" rule
// would miss the very defect it was raised for.
//
// So the test is the attribute's set of reachable values: the default, plus
// every literal any Change activity assigns. One value means the condition's
// outcome cannot vary. That subsumes "never written" (a value set of one, or of
// none) and catches the real case. The control the other way matters just as
// much: a default of true with the only write false is TWO values, so the guard
// can change state once and is not dead.
//
// # What makes a finding safe to report
//
// A write the rule cannot see reads as "no write", so every source of writes
// has to be accounted for or the rule reports working flags as dead. Four
// exclusions, each measured as a real false-positive source:
//
//   - bound to a page widget (linter.Widget.AttributeRef): a user can type into
//     it and the model has no idea what they will type. #2403 called this the
//     decisive one, and it is — without it the rule fires across a normal app.
//   - written from an expression rather than a literal: the value set is
//     unknown, so nothing follows.
//   - calculated: Mendix computes it, the model never assigns it.
//   - not read in any condition: then there is no guard, dead or otherwise.
//     (The mirror case, written but never read, is a separate verdict and is
//     deliberately not in this rule.)
//
// Writes are collected from EVERY module, including Marketplace ones, while
// only the project's own attributes are reported. A Marketplace flow writing an
// app's attribute is rare, but counting it as no write would be a false
// positive, and the asymmetry costs nothing.
//
// Boolean only for now. A String or Integer read in a condition and assigned
// one literal is the same shape, but enumerating "the literals I could see" is
// a much weaker claim for those types, so they are left out rather than
// reported at low confidence.
type FixedGuardAttributeRule struct{}

func NewFixedGuardAttributeRule() *FixedGuardAttributeRule {
	return &FixedGuardAttributeRule{}
}

func (r *FixedGuardAttributeRule) ID() string                       { return "MPR017" }
func (r *FixedGuardAttributeRule) Name() string                     { return "FixedGuardAttribute" }
func (r *FixedGuardAttributeRule) Category() string                 { return "correctness" }
func (r *FixedGuardAttributeRule) DefaultSeverity() linter.Severity { return linter.SeverityWarning }

// RequiredCatalogMode declares that this rule needs a FULL catalog.
//
// It reads activities (decision expressions), widgets (AttributeRef) and
// xpath_expressions, all of which REFRESH CATALOG FULL writes and a fast
// catalog does not. Without this declaration the lint entry point builds a
// fast catalog and the rule finds no conditions at all — so it reports
// NOTHING on a project that has the defect, which is worse than a wrong
// finding because it looks like a pass.
//
// Measured: on a freshly built catalog the rule found nothing, while a second
// run against the cached catalog (built in full mode by an earlier command)
// reported correctly. That difference is this declaration.
func (r *FixedGuardAttributeRule) RequiredCatalogMode() linter.CatalogMode {
	return linter.CatalogFull
}

func (r *FixedGuardAttributeRule) Description() string {
	return "A Boolean attribute a decision tests but which can only hold one value, so the branch it guards is unreachable"
}

// attrFacts is what the project says about one attribute.
type attrFacts struct {
	// readInCondition: some decision or XPath consults it.
	readInCondition bool
	// widgetBound: a page widget is bound to it, so a user may write it.
	widgetBound bool
	// unknownWrite: assigned from something other than a literal, so the
	// reachable set cannot be enumerated.
	unknownWrite bool
	// literals are the distinct literal values assigned to it.
	literals map[string]bool
}

// guardFinding is one reportable attribute and why.
type guardFinding struct {
	attr   linter.Attribute
	values int    // size of the reachable value set
	only   string // the one value, for the message
}

func (r *FixedGuardAttributeRule) Check(ctx *linter.LintContext) []linter.Violation {
	if ctx == nil || ctx.Reader() == nil {
		return nil
	}
	facts, why := collectAttributeFacts(ctx)
	if why != "" {
		// The writes could not be read. "I could not see the writes" must never
		// be reported as "there are no writes", so the rule reports its own
		// failure rather than a silently empty result (linter.RuleFailure is
		// kept out of the score and listed separately).
		return []linter.Violation{{
			RuleID:      r.ID(),
			Severity:    r.DefaultSeverity(),
			Message:     "MPR017 could not run: " + why,
			RuleFailure: true,
		}}
	}

	var owned []linter.Attribute
	for e := range ctx.Entities() {
		if ctx.IsExcluded(e.ModuleName) {
			continue
		}
		for a := range ctx.AttributesFor(e.QualifiedName) {
			owned = append(owned, a)
		}
	}

	var violations []linter.Violation
	for _, f := range fixedGuards(owned, facts) {
		qn := f.attr.EntityQualifiedName + "." + f.attr.Name
		msg := fmt.Sprintf("Attribute '%s' is tested by a decision but can only ever be %s, "+
			"so the branch it guards is unreachable.", qn, f.only)
		if f.values == 0 {
			msg = fmt.Sprintf("Attribute '%s' is tested by a decision but nothing ever assigns it "+
				"and it has no default, so the branch it guards is unreachable.", qn)
		}
		violations = append(violations, linter.Violation{
			RuleID:   r.ID(),
			Severity: r.DefaultSeverity(),
			Message:  msg,
			Location: linter.Location{
				Module:       f.attr.ModuleName,
				DocumentType: "entity",
				DocumentName: entityName(f.attr.EntityQualifiedName),
				DocumentID:   f.attr.EntityID,
			},
			Suggestion: "Either assign the other value where the guarded work should be skipped, " +
				"or remove the flag and the decision that reads it. A flag with one reachable " +
				"value reads as an interlock and is not one.",
		})
	}
	return violations
}

func entityName(qualified string) string {
	if i := strings.LastIndex(qualified, "."); i >= 0 {
		return qualified[i+1:]
	}
	return qualified
}

// fixedGuards is the decision, kept pure so it can be tested without a project.
// Deterministic: findings are ordered by qualified name.
func fixedGuards(attrs []linter.Attribute, facts map[string]*attrFacts) []guardFinding {
	var out []guardFinding
	for _, a := range attrs {
		if !strings.EqualFold(a.DataType, "Boolean") || a.IsCalculated {
			continue
		}
		f := facts[a.EntityQualifiedName+"."+a.Name]
		if f == nil || !f.readInCondition || f.widgetBound || f.unknownWrite {
			continue
		}
		reachable := map[string]bool{}
		for v := range f.literals {
			reachable[v] = true
		}
		if a.DefaultValue != "" {
			reachable[strings.ToLower(strings.TrimSpace(a.DefaultValue))] = true
		}
		if len(reachable) > 1 {
			continue
		}
		only := ""
		for v := range reachable {
			only = v
		}
		out = append(out, guardFinding{attr: a, values: len(reachable), only: only})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].attr.EntityQualifiedName+"."+out[i].attr.Name <
			out[j].attr.EntityQualifiedName+"."+out[j].attr.Name
	})
	return out
}

// attributeReadInExpression reports whether expr reads the member `name`.
//
// Matched as `/name` followed by a non-identifier character, which is how a
// member read is spelled in a Mendix expression ($Var/Attr, and across an
// association $Var/Mod.Assoc/Attr). It cannot resolve WHICH entity $Var holds,
// so the match is by name alone and therefore over-broad: a read of another
// entity's same-named attribute marks this one read too. That direction is
// deliberate — it costs findings, not correctness.
func attributeReadInExpression(expr, name string) bool {
	if expr == "" || name == "" {
		return false
	}
	needle := "/" + name
	for i := 0; ; {
		j := strings.Index(expr[i:], needle)
		if j < 0 {
			return false
		}
		end := i + j + len(needle)
		if end >= len(expr) || !isIdentRune(expr[end]) {
			return true
		}
		i = end
	}
}

func isIdentRune(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// collectAttributeFacts sweeps the project once. The returned string is empty
// on success and otherwise says why the sweep could not be trusted — the rule
// then reports that instead of findings, because "I could not see the writes"
// must never be reported as "there are no writes".
func collectAttributeFacts(ctx *linter.LintContext) (map[string]*attrFacts, string) {
	facts := map[string]*attrFacts{}
	get := func(qn string) *attrFacts {
		if f := facts[qn]; f != nil {
			return f
		}
		f := &attrFacts{literals: map[string]bool{}}
		facts[qn] = f
		return f
	}

	// Attribute names of interest, and the qualified names each maps to. Reads
	// are matched by name (see attributeReadInExpression), so one name can mark
	// several attributes read.
	byName := map[string][]string{}
	entities := 0
	for e := range ctx.Entities() {
		entities++
		for a := range ctx.AttributesFor(e.QualifiedName) {
			qn := a.EntityQualifiedName + "." + a.Name
			get(qn)
			byName[a.Name] = append(byName[a.Name], qn)
		}
	}
	// No attributes readable. A project with no entities is simply nothing to
	// check; entities WITH no readable attributes means the attribute read
	// failed, and this rule then says so rather than reporting a clean run.
	//
	// The specific bug this was written for is fixed (ako/mxcli#2417: an
	// in-memory catalog gave each pooled connection its own empty database, so
	// AttributesFor could fail while Entities succeeded in the same run), and
	// the advice it used to carry -- re-run to use the cached catalog -- is
	// gone with it. The check itself stays: it is the invariant, not the
	// workaround. A rule that cannot read its input must not be able to report
	// a pass, whatever the next reason for an unreadable catalog turns out to be.
	if len(byName) == 0 {
		if entities > 0 {
			return nil, fmt.Sprintf("read %d entities but could not read their attributes", entities)
		}
		return facts, ""
	}

	markRead := func(expr string) {
		if expr == "" {
			return
		}
		for name, qns := range byName {
			if attributeReadInExpression(expr, name) {
				for _, qn := range qns {
					get(qn).readInCondition = true
				}
			}
		}
	}

	// Reads: decision and other activity condition expressions, from the
	// catalog, and every XPath constraint. Neither is platform-filtered, which
	// is what we want for reads.
	// The `activities` VIEW, not activities_data: the _data tables are the
	// builders' storage and hold rows for every snapshot, while the view joins
	// the current one. Reading the table directly made this rule find no
	// conditions at all on the run right after a write, when the catalog was
	// rebuilt in-process — so it reported nothing on a project that did have
	// the defect.
	rows, err := ctx.Query(`SELECT COALESCE(ConditionExpression, '') FROM activities
		WHERE COALESCE(ConditionExpression, '') <> ''`)
	if err != nil {
		return nil, fmt.Sprintf("reading decision expressions: %v", err)
	}
	for rows.Next() {
		var expr string
		if rows.Scan(&expr) == nil {
			markRead(expr)
		}
	}
	rows.Close()
	for x := range ctx.XPathExpressions() {
		markRead(x.XPathExpression)
	}

	// A page widget bound to the attribute: a user may write it.
	for w := range ctx.Widgets() {
		if w.AttributeRef == "" {
			continue
		}
		if f := facts[w.AttributeRef]; f != nil {
			f.widgetBound = true
			continue
		}
		// Bound by bare name or an unexpected spelling: mark every attribute of
		// that name, erring towards not reporting.
		for _, qn := range byName[entityName(w.AttributeRef)] {
			get(qn).widgetBound = true
		}
	}

	// Writes: every Change/Create member assignment in every module. Uses the
	// reader rather than ctx.Microflows(), which is platform-filtered — a write
	// from a Marketplace flow is still a write.
	reader := ctx.Reader()
	flows, err := reader.ListMicroflows()
	if err != nil {
		return nil, fmt.Sprintf("reading microflows for member assignments: %v", err)
	}
	for _, mf := range flows {
		if mf == nil {
			continue
		}
		full := mf
		if mf.ObjectCollection == nil && mf.ID != "" {
			if loaded, err := ctx.FullMicroflow(model.ID(mf.ID)); err == nil && loaded != nil {
				full = loaded
			}
		}
		if full.ObjectCollection == nil {
			continue
		}
		walkMemberChanges(full.ObjectCollection.Objects, func(mc *microflows.MemberChange) {
			qn := mc.AttributeQualifiedName
			targets := []string{}
			if f := facts[qn]; f != nil {
				targets = append(targets, qn)
			} else if qn != "" {
				targets = append(targets, byName[entityName(qn)]...)
			}
			for _, t := range targets {
				f := get(t)
				if lit, ok := booleanLiteral(mc.Value); ok {
					f.literals[lit] = true
				} else {
					f.unknownWrite = true
				}
			}
			// The assigned value is itself an expression, so it reads whatever
			// it mentions.
			markRead(mc.Value)
		})
	}
	return facts, ""
}

// booleanLiteral normalises a value that is exactly a boolean literal.
// Anything else is an expression whose result is not known here.
func booleanLiteral(v string) (string, bool) {
	s := strings.ToLower(strings.TrimSpace(v))
	if s == "true" || s == "false" {
		return s, true
	}
	return "", false
}

// walkMemberChanges visits every member assignment, descending into loops the
// way CONV020 does — a change inside a loop is still a change.
func walkMemberChanges(objs []microflows.MicroflowObject, visit func(*microflows.MemberChange)) {
	for _, o := range objs {
		switch a := o.(type) {
		case *microflows.LoopedActivity:
			if a.ObjectCollection != nil {
				walkMemberChanges(a.ObjectCollection.Objects, visit)
			}
		case *microflows.ActionActivity:
			switch act := a.Action.(type) {
			case *microflows.ChangeObjectAction:
				for _, mc := range act.Changes {
					if mc != nil {
						visit(mc)
					}
				}
			case *microflows.CreateObjectAction:
				for _, mc := range act.InitialMembers {
					if mc != nil {
						visit(mc)
					}
				}
			}
		}
	}
}
