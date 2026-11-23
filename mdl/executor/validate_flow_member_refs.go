// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// flowMemberRefs is what check --references knows while it walks a script's
// flows in order, for the member questions member_verdict.go answers: the
// project, and what the script declares.
//
// Entities are taken from the whole script — a flow's parameters name them, so
// exec cannot reach the flow before they exist. Associations only from the
// statements BEFORE the flow, because that is all exec's builder sees when it
// resolves a bare association name: one declared further down resolves at
// check time and is refused at exec time otherwise.
type flowMemberRefs struct {
	ctx      *ExecContext
	sc       *scriptContext
	declared map[string]assocShape // associations of the statements so far
	project  *associationCatalog   // read on first use
	merged   *associationCatalog   // project + declared, rebuilt when declared grows
}

func newFlowMemberRefs(ctx *ExecContext, sc *scriptContext) *flowMemberRefs {
	return &flowMemberRefs{ctx: ctx, sc: sc, declared: map[string]assocShape{}}
}

// declare records the associations stmt creates, for the flows after it.
func (r *flowMemberRefs) declare(stmt ast.Statement) {
	switch stmt.(type) {
	case *ast.CreateAssociationStmt, *ast.CreateViewEntityStmt:
	default:
		return
	}
	one := newScriptContext()
	one.collectSingle(stmt)
	for k, v := range one.associationShapes {
		r.declared[k] = v
		r.merged = nil
	}
}

// catalog is the project's associations overlaid with the declared ones.
func (r *flowMemberRefs) catalog() associationCatalog {
	if r.merged != nil {
		return *r.merged
	}
	if r.project == nil {
		cat := checkAssociationCatalog(r.ctx, nil)
		r.project = &cat
	}
	merged := associationCatalog{shapes: make(map[string]assocShape, len(r.project.shapes)+len(r.declared)),
		unnamed: r.project.unnamed, complete: r.project.complete}
	for k, v := range r.project.shapes {
		merged.shapes[k] = v
	}
	for k, v := range r.declared {
		merged.shapes[k] = v
	}
	r.merged = &merged
	return merged
}

// chain is entityQN's generalization chain through the script's declarations
// and then the project.
func (r *flowMemberRefs) chain(entityQN string) ([]string, bool) {
	return scriptGeneralizationChain(r.ctx, r.sc, entityQN)
}

// scriptGeneralizationChain is entityQN's generalization chain, itself first,
// through the entities the script declares and then the project's: a check
// runs before the script's entities exist, so the project alone does not
// know a specialization the same script creates.
func scriptGeneralizationChain(ctx *ExecContext, sc *scriptContext, entityQN string) ([]string, bool) {
	var chain []string
	seen := map[string]bool{}
	for qn := entityQN; qn != ""; {
		if seen[qn] {
			return chain, false
		}
		seen[qn] = true
		if sc != nil && sc.entities[qn] {
			chain = append(chain, qn)
			qn = sc.entityGeneralizations[strings.ToLower(qn)]
			continue
		}
		rest, complete := generalizationChain(ctx, qn)
		return append(chain, rest...), complete
	}
	return chain, true
}

// bareRetrieveAssociation reports `retrieve $X from $V/Name` when Name, a bare
// association name, does not resolve to exactly one association from the
// entity $V holds (ako/mxcli#1690). It is silent where the entity is not known
// or the model could not be read: exec decides those, and refuses them too,
// since a bare name cannot be written.
func (r *flowMemberRefs) bareRetrieveAssociation(s *ast.RetrieveStmt, vars map[string]string) string {
	if s.StartVariable == "" || s.Source.Module != "" || s.Source.Name == "" {
		return ""
	}
	entity := vars[strings.TrimPrefix(s.StartVariable, "$")]
	if entity == "" {
		return ""
	}
	chain, complete := r.chain(entity)
	_, matches, verdict := resolveBareAssociation(s.Source.Name, chain, complete, r.catalog())
	if verdict != bareAssocMissing && verdict != bareAssocAmbiguous {
		return ""
	}
	return bareRetrieveAssociationRefusal(s.Variable, s.StartVariable, s.Source.Name, entity, matches, verdict)
}

// statementMembers is what one flow statement names that its entity does not
// have, each report prefixed with the flow's name.
func (r *flowMemberRefs) statementMembers(flowQN string, st ast.MicroflowStatement, vars map[string]string) []string {
	var out []string
	report := func(msg string) {
		if msg != "" {
			out = append(out, flowQN+": "+msg)
		}
	}
	seen := map[string]bool{}
	for _, p := range statementAttributePaths(st) {
		if msg := r.expressionPath(p, vars); !seen[msg] {
			seen[msg] = true
			report(msg)
		}
	}
	switch s := st.(type) {
	case *ast.RetrieveStmt:
		report(r.bareRetrieveAssociation(s, vars))
	case *ast.ListOperationStmt:
		for _, m := range listOperationMembers(s) {
			report(r.listMember(m.verb, s.InputVariable, vars[s.InputVariable], m.name))
		}
	case *ast.AggregateListStmt:
		if !s.IsExpression && s.Attribute != "" {
			verb := strings.ToLower(s.Operation.String())
			msg := r.listMember(verb, s.InputVariable, vars[s.InputVariable], s.Attribute)
			if msg == "" {
				msg = r.aggregateAttributeType(verb, s.InputVariable, vars[s.InputVariable], s.Attribute)
			}
			report(msg)
		}
	}
	return out
}

type listOpMember struct{ verb, name string }

// listOperationMembers are the members a find, filter or sort names: the left
// of a by-member find / filter (the by-expression form is an expression), and
// each sort key.
func listOperationMembers(s *ast.ListOperationStmt) []listOpMember {
	switch s.Operation {
	case ast.ListOpFind, ast.ListOpFilter:
		if s.ByExpression || !ast.IsMemberEquality(s.Condition) {
			return nil
		}
		name, ok := listOperationFieldName(s.Condition.(*ast.BinaryExpr).Left)
		if !ok {
			return nil
		}
		return []listOpMember{{strings.ToLower(s.Operation.String()), name}}
	case ast.ListOpSort:
		var out []listOpMember
		for _, spec := range s.SortSpecs {
			out = append(out, listOpMember{"sort", spec.Attribute})
		}
		return out
	}
	return nil
}

// listMember reports a list operation's or aggregate's bare member that the
// list's entity does not have (ako/mxcli#1646), in the words exec's builder
// refuses it with (refuseMissingListMember). A qualified member is a different
// question — validateListOperationSystemMembers and the builder's association
// lookup answer it — and an untyped list is not judged.
func (r *flowMemberRefs) listMember(verb, list, entityQN, name string) string {
	if entityQN == "" || name == "" || strings.Contains(name, ".") {
		return ""
	}
	if res, _ := r.member(entityQN, name, !isAggregateVerb(verb)); res != memberMissing {
		return ""
	}
	return listMemberRefusal(listMemberWhere(verb, list, name), entityQN, name, r.nearMembers(entityQN))
}

// aggregateAttributeType refuses an aggregate by an attribute that is not a
// number. mxbuild reports CE0016 "Attribute must be of type Autonumber,
// Decimal, Integer, or Long." for minimum, maximum, sum and average over a
// DateTime or String attribute — measured on 11.12.6, each of the eight; the
// same four over an Integer build (ako/mxcli#2528). A type that cannot be
// read here (an inherited attribute, an unknown list) is not judged.
func (r *flowMemberRefs) aggregateAttributeType(verb, list, entityQN, name string) string {
	if !isAggregateVerb(verb) || entityQN == "" || name == "" || strings.Contains(name, ".") {
		return ""
	}
	t, ok := retrieveAttributeType(r.ctx, r.sc, entityQN, name)
	if !ok {
		return ""
	}
	if t.Kind == ast.TypeUnknown {
		return ""
	}
	return aggregateTypeRefusal(listMemberWhere(verb, list, name), entityQN+"."+name, t.Kind.String())
}

// aggregateTypeRefusal is the CE0016 refusal check and exec share, or "" for a
// type an aggregate takes. typeName is the attribute type as MDL spells it.
func aggregateTypeRefusal(where, attrQN, typeName string) string {
	switch typeName {
	case "Integer", "Long", "Decimal", "AutoNumber", "", "Unknown":
		return ""
	}
	return fmt.Sprintf(
		"%s: %s is %s, and an aggregate takes a number — mxbuild reports CE0016 "+
			"\"Attribute must be of type Autonumber, Decimal, Integer, or Long.\"",
		where, attrQN, typeName)
}

// isAggregateVerb reports an aggregate list function: one that cannot read a
// system date (listMemberRefusal).
func isAggregateVerb(verb string) bool {
	switch verb {
	case "sum", "average", "minimum", "maximum":
		return true
	}
	return false
}

// member is flowMemberVerdict through the script: the same walk
// (memberVerdictOver), reading each link from the script where the script
// declares it — exec's builder will find exactly that in the project — and
// from the project otherwise. A link the script alters is not judged: the
// alter may add the member. An association the statements before the flow
// create counts as a member of its module, as a stored one does.
func (r *flowMemberRefs) member(entityQN, name string, systemDates bool) (memberResolution, memberKind) {
	return memberVerdictOver(scriptMemberModel{r}, entityQN, name, systemDates)
}

type scriptMemberModel struct{ r *flowMemberRefs }

func (m scriptMemberModel) memberLink(entityQN, name string) (memberLinkFacts, bool) {
	r := m.r
	if r.sc != nil && r.sc.alteredEntities[entityQN] {
		return memberLinkFacts{}, false
	}
	var decl *ast.CreateEntityStmt
	if r.sc != nil {
		decl = r.sc.entityDecls[entityQN]
		// A view entity the script declares has the columns it lists, the
		// stored one's not: a column added and aggregated over in one script
		// was refused (mendixlabs/mxcli#1908).
		if v := r.sc.viewEntityDecls[entityQN]; decl == nil && v != nil {
			var facts memberLinkFacts
			for _, a := range v.Attributes {
				if a.Name == name {
					facts.attribute = true
				}
			}
			facts.association = r.declaredAssociationInModule(entityQN, name) || r.projectAssociationInModule(entityQN, name)
			return facts, true
		}
	}
	if decl == nil {
		facts, ok := backendMemberModel{r.backend()}.memberLink(entityQN, name)
		if ok && !facts.association {
			facts.association = r.declaredAssociationInModule(entityQN, name)
		}
		return facts, ok
	}
	var facts memberLinkFacts
	if decl.Generalization != nil {
		facts.generalization = decl.Generalization.String()
	}
	for _, a := range decl.Attributes {
		switch {
		case isAutoSystemMemberType(a.Type.Kind):
			if systemMemberOfPseudoType(a.Type.Kind) == name {
				facts.storesDate = true
			}
		case a.Name == name:
			facts.attribute = true
		}
	}
	facts.association = r.declaredAssociationInModule(entityQN, name) || r.projectAssociationInModule(entityQN, name)
	return facts, true
}

func (r *flowMemberRefs) backend() entityLookupBackend {
	if r.ctx == nil {
		return nil
	}
	b, _ := r.ctx.Backend.(entityLookupBackend)
	return b
}

// declaredAssociationInModule reports an association of this name that a
// statement before the flow creates in entityQN's module.
func (r *flowMemberRefs) declaredAssociationInModule(entityQN, name string) bool {
	module, _, _ := strings.Cut(entityQN, ".")
	_, ok := r.declared[strings.ToLower(module+"."+name)]
	return ok
}

// projectAssociationInModule reports a stored association of this name in
// entityQN's module, for an entity the script (re)declares.
func (r *flowMemberRefs) projectAssociationInModule(entityQN, name string) bool {
	module, _, _ := strings.Cut(entityQN, ".")
	_, ok := r.catalog().shapes[strings.ToLower(module+"."+name)]
	return ok
}

// nearMembers is nearMembersFor through the script's declarations.
func (r *flowMemberRefs) nearMembers(entityQN string) string {
	if r.sc != nil {
		if v := r.sc.viewEntityDecls[entityQN]; v != nil && r.sc.entityDecls[entityQN] == nil {
			var names []string
			for _, a := range v.Attributes {
				names = append(names, a.Name)
			}
			return nearMemberList(names)
		}
		if decl := r.sc.entityDecls[entityQN]; decl != nil {
			var names []string
			for _, a := range decl.Attributes {
				if !isAutoSystemMemberType(a.Type.Kind) {
					names = append(names, a.Name)
				}
			}
			return nearMemberList(names)
		}
	}
	return nearMembersFor(r.backend(), entityQN)
}

// expressionPath reports a `$Var/<member>` path in an expression that names a
// member the variable's entity does not have (ako/mxcli#1697): an attribute
// that is not there (the system `id` above all, which no expression can read),
// an association written by its bare name, or one that does not exist or does
// not start at the entity. mxbuild rejects each with CE0117 "Error(s) in
// expression." (measured on 11.13.0). Each step is resolved through
// flowMemberRefs.member — the verdict exec's builder asks for a list member —
// and the path is followed across association hops as far as it resolves;
// anything not established is silence.
func (r *flowMemberRefs) expressionPath(p *ast.AttributePathExpr, vars map[string]string) string {
	cur := vars[strings.TrimPrefix(p.Variable, "$")]
	steps := expressionPathSteps(p)
	if cur == "" || len(steps) == 0 {
		return ""
	}
	text := "$" + strings.TrimPrefix(p.Variable, "$") + "/" + strings.Join(steps, "/")
	const rejected = " — mxbuild rejects the expression with CE0117 \"Error(s) in expression.\""
	for i := 0; i < len(steps); i++ {
		step := steps[i]
		switch strings.Count(step, ".") {
		case 0:
			res, kind := r.member(cur, step, true)
			switch {
			case res == memberMissing:
				msg := fmt.Sprintf("%s: %s has no attribute %q%s%s", text, cur, step, flowSystemMemberHint(step), rejected)
				if strings.EqualFold(step, "id") {
					return msg + " An object's id is not readable in an expression: compare the objects " +
						"themselves (`$A = $B`), or read a GUID with CommunityCommons.getGUID(item = $X)"
				}
				return msg + r.nearMembers(cur)
			case res == memberFound && kind == memberIsAssociation:
				qualified := "Module." + step
				chain, complete := r.chain(cur)
				if qn, _, v := resolveBareAssociation(step, chain, complete, r.catalog()); v == bareAssocResolved {
					qualified = qn
				}
				return fmt.Sprintf("%s: %s is an association: an expression names it qualified, `$%s/%s/…`%s",
					text, step, strings.TrimPrefix(p.Variable, "$"), qualified, rejected)
			}
			// An attribute: whatever follows it is not a member question.
			return ""
		case 1:
			if _, ok := builtinSystemMemberAssociation(step); ok {
				return "" // System.owner / System.changedBy: no domain model lists them
			}
			if r.isEntity(step) {
				return "" // an entity step that is not the hop's target: not modelled
			}
			to, res := r.associationHop(step, cur)
			switch res {
			case assocMissing:
				return fmt.Sprintf("%s: there is no association %s%s", text, step, rejected)
			case assocNotAnEnd:
				return fmt.Sprintf("%s: %s is not an end of %s, nor of anything it extends%s", text, cur, step, rejected)
			case assocResolved:
			default:
				return ""
			}
			cur = to
			// `$O/Mod.Assoc/Mod.Target/Attr` names the target after the hop,
			// and the builder qualifies a bare `Target` in place (#829).
			if i+1 < len(steps) && (steps[i+1] == to || strings.HasSuffix(to, "."+steps[i+1])) {
				i++
			}
		default:
			return ""
		}
	}
	return ""
}

// associationHop follows a qualified association from an object of entityQN:
// an association the script creates by its declared ends, one the project has
// by resolveAssociationFrom — which also answers "missing" for an association
// in a module the script creates and does not declare it in. A module whose
// entities come from a service contract is not judged.
func (r *flowMemberRefs) associationHop(assocQN, entityQN string) (string, assocResolution) {
	if r.sc == nil {
		return resolveAssociationFrom(r.ctx, assocQN, entityQN)
	}
	chain, complete := r.chain(entityQN)
	if ends, ok := r.sc.associationEnds[strings.ToLower(assocQN)]; ok {
		for _, e := range chain {
			switch e {
			case ends[0]:
				return ends[1], assocResolved
			case ends[1]:
				return ends[0], assocResolved
			}
		}
		if complete {
			return "", assocNotAnEnd
		}
		return "", assocUnknown
	}
	if module, _, _ := strings.Cut(assocQN, "."); r.sc.externalEntityModules[module] {
		return "", assocUnknown
	}
	// A link the script declares is not in the project, so the project cannot
	// say whether it is an end; the first stored link answers for itself and
	// everything it extends.
	for _, e := range chain {
		to, res := resolveAssociationFrom(r.ctx, assocQN, e)
		if res == assocResolved || res == assocMissing || !r.sc.entities[e] {
			return to, res
		}
	}
	if complete {
		return "", assocNotAnEnd
	}
	return "", assocUnknown
}

// isEntity reports a qualified name the script or the project has as an entity.
func (r *flowMemberRefs) isEntity(qn string) bool {
	if r.sc != nil && r.sc.entities[qn] {
		return true
	}
	_, ok := findEntityByQN(r.backend(), qn)
	return ok
}

// expressionPathSteps are the member steps of `$v/A/B`.
func expressionPathSteps(p *ast.AttributePathExpr) []string {
	if len(p.Path) > 0 {
		return p.Path
	}
	steps := make([]string, 0, len(p.Segments))
	for _, sg := range p.Segments {
		steps = append(steps, sg.Name)
	}
	return steps
}

var (
	flowBodyType    = reflect.TypeOf([]ast.MicroflowStatement(nil))
	errorHandlerPtr = reflect.TypeOf((*ast.ErrorHandlingClause)(nil))
)

// statementAttributePaths collects the `$Var/…` paths in one statement's own
// expressions — every field, so a statement kind added later is covered
// without a case here. Nested bodies and error handlers are left out: the
// typed walk visits their statements itself, with the variables in scope
// there. A retrieve's constraint is XPath, judged by validateRetrieveMembers,
// and a validation feedback's target is a member reference, not an expression.
func statementAttributePaths(st ast.MicroflowStatement) []*ast.AttributePathExpr {
	if _, ok := st.(*ast.RetrieveStmt); ok {
		return nil
	}
	var out []*ast.AttributePathExpr
	var walk func(rv reflect.Value, top bool)
	walk = func(rv reflect.Value, top bool) {
		if rv.Type() == flowBodyType || rv.Type() == errorHandlerPtr {
			return
		}
		switch rv.Kind() {
		case reflect.Interface:
			if !rv.IsNil() {
				walk(rv.Elem(), false)
			}
		case reflect.Pointer:
			if rv.IsNil() {
				return
			}
			if p, ok := rv.Interface().(*ast.AttributePathExpr); ok {
				out = append(out, p)
				return
			}
			walk(rv.Elem(), top)
		case reflect.Struct:
			_, feedback := st.(*ast.ValidationFeedbackStmt)
			for i := 0; i < rv.NumField(); i++ {
				f := rv.Type().Field(i)
				if !f.IsExported() || (top && feedback && f.Name == "AttributePath") {
					continue
				}
				walk(rv.Field(i), false)
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < rv.Len(); i++ {
				walk(rv.Index(i), false)
			}
		}
	}
	walk(reflect.ValueOf(st), true)
	return out
}
