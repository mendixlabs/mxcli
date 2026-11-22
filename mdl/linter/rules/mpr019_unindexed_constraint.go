// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// UnindexedConstraintRule flags a persistable attribute an XPath constraint
// filters on when no index leads with it.
//
// From the benchmark that raised ako/mxcli#2407:
//
//	"Database indexes appear in five apps: all three built with MxCraft (Opus
//	 16, Sonnet 8, Haiku 4), and Sonnet (5) and Opus (3) with MXCLI. No Concord
//	 app has one."
//
// Four of nine apps shipped with no index at all, and the spread among the rest
// was 16 against 3 for the same task. It is the least dramatic of that report's
// findings and the most ordinary: a standard review item, free to check, and
// invisible to mx check and to any amount of testing on demo data.
//
// # What it reports, and what it deliberately does not
//
// Severity is info. Whether an index earns its place depends on table size and
// write rate, which the model does not know, so this names a candidate rather
// than asserting a defect — and an index on a small reference table is not
// worth adding.
//
// An index covers a query on its FIRST attribute only, so coverage is tested
// against the leading attribute, not membership. A composite index on
// (Status, Date) does nothing for a query on Date alone; treating membership as
// coverage would make the rule wrong rather than merely quiet.
//
// Only attributes of the entity the XPath TARGETS. An attribute reached across
// an association belongs to the other entity, and indexing it is that entity's
// business; reporting it here would blame the wrong document.
//
// Sort orders are NOT covered, though ako/mxcli#2407 lists them. They are not
// in the catalog — there is no sort column on any table — so they would need
// the flow and page models read for retrieve and data-source sort orders. That
// is worth doing and is a bigger change than this; XPath constraints are the
// richest source and are catalogued, so they are v1 on their own.
type UnindexedConstraintRule struct{}

func NewUnindexedConstraintRule() *UnindexedConstraintRule {
	return &UnindexedConstraintRule{}
}

func (r *UnindexedConstraintRule) ID() string                       { return "MPR019" }
func (r *UnindexedConstraintRule) Name() string                     { return "UnindexedConstraint" }
func (r *UnindexedConstraintRule) Category() string                 { return "performance" }
func (r *UnindexedConstraintRule) DefaultSeverity() linter.Severity { return linter.SeverityInfo }
func (r *UnindexedConstraintRule) Description() string {
	return "An attribute an XPath constraint filters on with no index leading with it, so every query scans the table"
}

// RequiredCatalogMode declares that this rule needs a FULL catalog: it reads
// xpath_expressions, which only REFRESH CATALOG FULL writes.
//
// Without this, lint builds a fast catalog, that table is empty, and the rule
// reports NOTHING on a project full of unindexed constraints — which looks
// exactly like a pass. MPR017 shipped that way until it was measured
// (ako/mxcli#2403), and linter.CatalogRequirer's own doc warns of it.
func (r *UnindexedConstraintRule) RequiredCatalogMode() linter.CatalogMode {
	return linter.CatalogFull
}

func (r *UnindexedConstraintRule) Check(ctx *linter.LintContext) []linter.Violation {
	if ctx == nil || ctx.Reader() == nil {
		return nil
	}
	entities := indexableEntityIndex(ctx)
	if len(entities) == 0 {
		return nil
	}

	// One finding per entity+attribute, however many constraints filter on it:
	// the fix is one index, not one per XPath.
	type key struct{ entity, attr string }
	seen := map[key]bool{}
	type finding struct {
		entity, attr, module, where string
	}
	var findings []finding

	for x := range ctx.XPathExpressions() {
		e := entities[strings.ToLower(x.TargetEntity)]
		if e == nil {
			continue
		}
		for _, attr := range constraintAttributeNames(x.XPathExpression, e.attrNames) {
			if e.leading[attr] {
				continue
			}
			k := key{strings.ToLower(x.TargetEntity), attr}
			if seen[k] {
				continue
			}
			seen[k] = true
			findings = append(findings, finding{
				entity: x.TargetEntity, attr: e.attrNames[attr], module: e.module,
				where: xpathWhere(x),
			})
		}
	}

	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].entity != findings[j].entity {
			return findings[i].entity < findings[j].entity
		}
		return findings[i].attr < findings[j].attr
	})

	var violations []linter.Violation
	for _, f := range findings {
		violations = append(violations, linter.Violation{
			RuleID:   r.ID(),
			Severity: r.DefaultSeverity(),
			Message: fmt.Sprintf("'%s.%s' is filtered on by an XPath constraint (%s) and no index leads with it, so each query scans the table.",
				f.entity, f.attr, f.where),
			Location: linter.Location{
				Module:       f.module,
				DocumentType: "entity",
				DocumentName: entityName(f.entity),
			},
			Suggestion: fmt.Sprintf("Add an index to '%s' whose FIRST attribute is '%s' — an index covers a query on its leading attribute only. "+
				"On a small reference table the scan is cheaper than the index; this is a candidate, not a defect.", f.entity, f.attr),
		})
	}
	return violations
}

// xpathWhere names where the constraint came from, so a finding can be traced
// without opening every access rule on the entity.
func xpathWhere(x linter.XPathExpressionEntry) string {
	kind := strings.ToLower(strings.ReplaceAll(x.ComponentType, "_", " "))
	if kind == "" {
		kind = "constraint"
	}
	if x.DocumentQualifiedName != "" {
		return kind + " in " + x.DocumentQualifiedName
	}
	return kind
}

// indexableEntityInfo is what the rule needs to know about one entity.
type indexableEntityInfo struct {
	module string
	// attrNames maps a lower-cased attribute name to its stored spelling.
	attrNames map[string]string
	// leading holds the lower-cased names that LEAD an index.
	leading map[string]bool
}

// indexableEntityIndex reads every owned module's domain model and returns the
// persistable entities, keyed by lower-cased qualified name because Mendix
// resolves names case-insensitively.
//
// From the MODEL, not the catalog: an entity's indexes are not catalogued (no
// index table exists), while domainmodel.Entity carries them with their
// attribute ORDER, which is what coverage depends on. ako/mxcli#2407 assumed
// catalog extraction had to come first; it does not, though it would make this
// expressible as a Starlark rule and is useful beyond it.
func indexableEntityIndex(ctx *linter.LintContext) map[string]*indexableEntityInfo {
	out := map[string]*indexableEntityInfo{}
	moduleNames := ctx.OwnedModuleNamesByID()
	for _, dm := range ctx.DomainModels() {
		if dm == nil {
			continue
		}
		module := moduleNames[dm.ContainerID]
		if module == "" || ctx.IsExcluded(module) {
			continue
		}
		for _, e := range dm.Entities {
			if !indexableEntity(e) {
				continue
			}
			info := &indexableEntityInfo{
				module:    module,
				attrNames: map[string]string{},
				leading:   leadingIndexAttributes(e),
			}
			// A unique attribute counts as covered. See uniqueAttributes for
			// why this is an exclusion rather than a finding.
			for name := range uniqueAttributes(e) {
				info.leading[name] = true
			}
			for _, a := range e.Attributes {
				if a != nil && a.Name != "" {
					info.attrNames[strings.ToLower(a.Name)] = a.Name
				}
			}
			out[strings.ToLower(module+"."+e.Name)] = info
		}
	}
	return out
}

// indexableEntity reports whether an entity has a table to index at all.
//
// An external entity is stored persistable but its table lives in the app that
// publishes it, so there is no index here to add (ako/mxcli#2502: all 39 findings
// on a frontend app were external entities).
func indexableEntity(e *domainmodel.Entity) bool {
	return e != nil && e.Persistable && e.Name != "" && e.Source != externalEntitySource
}

// externalEntitySource is the stored Source of an external (OData remote) entity.
const externalEntitySource = "Rest$ODataRemoteEntitySource"

// attributeRefIndex returns an entity's attributes by UUID and by lower-cased
// name, the two keys an attribute reference can arrive as.
func attributeRefIndex(e *domainmodel.Entity) (byID map[model.ID]string, names map[string]bool) {
	byID, names = map[model.ID]string{}, map[string]bool{}
	if e == nil {
		return byID, names
	}
	for _, a := range e.Attributes {
		if a != nil && a.Name != "" {
			lower := strings.ToLower(a.Name)
			byID[a.ID] = lower
			names[lower] = true
		}
	}
	return byID, names
}

// resolveAttributeRef maps an attribute reference to a lower-cased attribute
// name, accepting EITHER form the model uses.
//
// The two carriers disagree, measured on testdata/testapp:
// ValidationRule.AttributeID holds a qualified name ("Mappings.Order.OrderId"),
// while an index segment written by mxcli holds the attribute's UUID. Assuming
// the UUID made the unique-attribute exclusion silently match nothing, which
// looked exactly like the exclusion not being needed.
//
// No Studio Pro-authored index was available to measure (every indexed entity
// to hand was created by mxcli), so which form Studio Pro writes for an index
// segment is unverified — hence both forms here rather than one.
func resolveAttributeRef(ref model.ID, byID map[model.ID]string, names map[string]bool) string {
	if ref == "" {
		return ""
	}
	if name := byID[ref]; name != "" {
		return name
	}
	s := string(ref)
	if i := strings.LastIndex(s, "."); i >= 0 {
		s = s[i+1:]
	}
	if s = strings.ToLower(s); names[s] {
		return s
	}
	return ""
}

// leadingIndexAttributes returns the lower-cased names of the attributes that
// LEAD one of the entity's indexes.
//
// Leading only, because that is what an index covers: a composite index on
// (Status, Date) serves a query on Status and does nothing for one on Date
// alone. An index whose first segment is a system member (CreatedDate /
// ChangedDate) has no AttributeID behind it (mendixlabs/mxcli#1401), so it
// contributes nothing rather than resolving to the empty name.
func leadingIndexAttributes(e *domainmodel.Entity) map[string]bool {
	out := map[string]bool{}
	if !indexableEntity(e) {
		return out
	}
	byID, names := attributeRefIndex(e)
	for _, ix := range e.Indexes {
		if ix == nil || len(ix.Attributes) == 0 {
			continue
		}
		first := ix.Attributes[0]
		if first == nil {
			continue
		}
		if name := resolveAttributeRef(first.AttributeID, byID, names); name != "" {
			out[name] = true
		}
	}
	return out
}

// uniqueAttributes returns the lower-cased names of attributes carrying a
// Unique validation rule.
//
// They are treated as covered, which is an admission rather than a claim.
// Mendix's "unique" is a validation rule the runtime enforces; whether the
// generated database schema also carries a unique INDEX for it could not be
// established here, and it decides the question — enforcing uniqueness on
// every insert essentially requires an index, so the platform plausibly makes
// one.
//
// Measured consequence of getting it wrong: without this exclusion the only
// findings on the 900-unit Studio Pro-authored fixture testdata/testapp were
// Mappings.Customer.CustomerId and Mappings.Order.OrderId, both declared
// `unique`. So the choice is between a rule whose every finding is true and one
// whose first two findings on a real app may be noise, at info severity, where
// a false candidate costs more trust than a missed one costs performance.
//
// To settle it: create an entity with a unique attribute, start the app against
// a real database (see .claude/skills/verify-in-runtime.md) and read the
// schema's indexes for that table. If Mendix does not index them, delete this
// function and its call — the test named after it records the question.
func uniqueAttributes(e *domainmodel.Entity) map[string]bool {
	out := map[string]bool{}
	if e == nil {
		return out
	}
	byID, names := attributeRefIndex(e)
	for _, vr := range e.ValidationRules {
		if vr == nil {
			continue
		}
		_, isUnique := vr.Rule.(*domainmodel.UniqueValidationRuleInfo)
		if !isUnique && !strings.EqualFold(vr.Type, "Unique") {
			continue
		}
		if name := resolveAttributeRef(vr.AttributeID, byID, names); name != "" {
			out[name] = true
		}
	}
	return out
}

// constraintAttributeNames returns the lower-cased names of this entity's own
// attributes that the constraint filters on, in the order first seen.
//
// Only single-step names count. A step that walks an association names another
// entity's member, and indexing that is the other entity's business. A group
// mxcli cannot parse is skipped rather than guessed at — the same choice SEC011
// makes for the hops it cannot place.
func constraintAttributeNames(constraint string, attrNames map[string]string) []string {
	var out []string
	seen := map[string]bool{}
	groups := visitor.SplitXPathPredicateGroups(constraint)
	if len(groups) == 0 && strings.TrimSpace(constraint) != "" {
		// SplitXPathPredicateGroups finds bracketed predicate groups and
		// returns none for a bare condition, while the catalog's
		// XPathExpression only "may include outer [ ]". Every value measured
		// in testdata/pedapp does carry them, so this is insurance rather than
		// the normal path.
		groups = []string{constraint}
	}
	for _, group := range groups {
		expr, ok := visitor.ParseXPathConstraint(group)
		if !ok || expr == nil {
			continue
		}
		walkConstraintNames(expr, attrNames, seen, &out)
	}
	return out
}

func walkConstraintNames(expr ast.Expression, attrNames map[string]string, seen map[string]bool, out *[]string) {
	switch e := expr.(type) {
	case *ast.XPathPathExpr:
		// One step is a member of the target entity; more than one walks an
		// association to somewhere else.
		if len(e.Steps) == 1 {
			if q, ok := e.Steps[0].Expr.(*ast.QualifiedNameExpr); ok {
				consider(q.QualifiedName.String(), attrNames, seen, out)
			}
		}
		for _, st := range e.Steps {
			for _, pred := range st.Predicates {
				walkConstraintNames(pred, attrNames, seen, out)
			}
		}
	case *ast.IdentifierExpr:
		// A bare member of the target entity parses as an identifier, NOT as a
		// one-step path: `[Status = 'Open']` is BinaryExpr{IdentifierExpr, =,
		// LiteralExpr}. Measured against the parser rather than assumed, after
		// the one-step-path version found nothing.
		consider(e.Name, attrNames, seen, out)
	case *ast.QualifiedNameExpr:
		consider(e.QualifiedName.String(), attrNames, seen, out)
	case *ast.BinaryExpr:
		walkConstraintNames(e.Left, attrNames, seen, out)
		walkConstraintNames(e.Right, attrNames, seen, out)
	case *ast.UnaryExpr:
		walkConstraintNames(e.Operand, attrNames, seen, out)
	case *ast.ParenExpr:
		walkConstraintNames(e.Inner, attrNames, seen, out)
	}
}

// consider records name when it is an attribute of the target entity. A dotted
// name is a qualified reference (an association, an enumeration value), never a
// bare member of this entity.
func consider(name string, attrNames map[string]string, seen map[string]bool, out *[]string) {
	if name == "" || strings.Contains(name, ".") {
		return
	}
	lower := strings.ToLower(name)
	if attrNames[lower] == "" || seen[lower] {
		return
	}
	seen[lower] = true
	*out = append(*out, lower)
}
