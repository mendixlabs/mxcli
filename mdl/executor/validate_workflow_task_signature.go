// SPDX-License-Identifier: Apache-2.0

// Signature checks for the two documents a user task hands work to — its task
// page and its targeting microflow. Both were resolved by name only, so a page
// typed to the workflow's context entity, or a targeting microflow missing the
// System.Workflow parameter, passed `check --references`, was written by exec,
// and surfaced only under the native validator. A team building workflows from
// MDL reported both, and put them among the reasons "a green CLI check is not
// evidence for workflows".
//
// Every rule here is a row of an mxbuild measurement (11.13.0, one user task per
// shape, verdict = the literal `mx check` line):
//
//	task page, no parameters                             CE7410
//	task page, only a context-entity parameter           CE7412 (multi-user task too)
//	task page, WorkflowUserTask + an extra parameter     0 errors
//	targeting (System.Workflow, Ctx)                     0 errors
//	targeting (Ctx, System.Workflow)                     0 errors — order is free
//	targeting (System.Workflow) / () / (+ String)        CE6677 — exactly two
//	targeting (System.Workflow, <generalization of Ctx>) 0 errors
//	targeting (System.Workflow, <specialization of Ctx>) CE6677
//	targeting groups microflow                           the same rule
//
// Three of those rows are clean shapes a plausible reading of the error text
// would refuse — extra page parameters, reversed order, a generalization. They
// are the reason the rules are measured rather than read off the messages:
// exec refuses a script whose check reports an error, so a false refusal here
// is not a warning but a blocker.
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

const (
	workflowUserTaskEntity = "System.WorkflowUserTask"
	workflowEntity         = "System.Workflow"
)

// validateWorkflowTaskSignatures checks the task page and targeting microflow of
// every user task in activities, including nested ones. contextEntity is the
// workflow's context entity; with none known, only the page is checked.
func validateWorkflowTaskSignatures(ctx *ExecContext, activities []ast.WorkflowActivityNode, contextEntity string, sc *scriptContext) []string {
	c := newWorkflowTaskSignatureChecker(ctx, sc)
	if c == nil {
		return nil
	}
	return c.checkActivities(activities, contextEntity)
}

// validateAlterWorkflowTaskSignatures applies the same checks to ALTER WORKFLOW:
// user tasks an operation introduces, and SET ACTIVITY … PAGE / TARGETING
// MICROFLOW. The context entity is not in the statement, so it is read off the
// stored workflow; a workflow that is not stored yet leaves targeting unchecked
// rather than guessed at.
func validateAlterWorkflowTaskSignatures(ctx *ExecContext, s *ast.AlterWorkflowStmt, added []ast.WorkflowActivityNode, sc *scriptContext) []string {
	c := newWorkflowTaskSignatureChecker(ctx, sc)
	if c == nil {
		return nil
	}
	contextEntity := ""
	if wf := findStoredWorkflow(ctx, s.Name); wf != nil && wf.Parameter != nil {
		contextEntity = wf.Parameter.EntityRef
	}
	errs := c.checkActivities(added, contextEntity)
	for _, op := range s.Operations {
		o, ok := op.(*ast.SetActivityPropertyOp)
		if !ok {
			continue
		}
		label := fmt.Sprintf("activity '%s'", o.ActivityRef)
		errs = appendIfSet(errs, c.checkPage(label, o.PageName.String()))
		errs = appendIfSet(errs, c.checkTargeting(label, o.Microflow.String(), contextEntity))
	}
	return errs
}

// workflowTaskSignatureChecker resolves page and flow signatures lazily — from
// the script first, because a script that redefines a document is describing
// what will be written, then from the project.
type workflowTaskSignatureChecker struct {
	ctx   *ExecContext
	sc    *scriptContext
	pages map[string][]string
	flows map[string]*flowSignature
}

// newWorkflowTaskSignatureChecker returns nil when the checks do not apply.
//
// Gated on Mendix 11 because that is where the rules are measured. Whether 10.x
// agrees is unknown here, and applying a guess would be a false refusal on the
// versions it is wrong for — the same reasoning as MDL-WF07.
func newWorkflowTaskSignatureChecker(ctx *ExecContext, sc *scriptContext) *workflowTaskSignatureChecker {
	if ctx == nil || ctx.Backend == nil || !ctx.Connected() {
		return nil
	}
	pv := ctx.Backend.ProjectVersion()
	if pv == nil || !pv.IsAtLeast(11, 0) {
		return nil
	}
	return &workflowTaskSignatureChecker{ctx: ctx, sc: sc}
}

func (c *workflowTaskSignatureChecker) checkActivities(activities []ast.WorkflowActivityNode, contextEntity string) []string {
	var errs []string
	walkWorkflowActivities(activities, func(a ast.WorkflowActivityNode) {
		if cm, ok := a.(*ast.WorkflowCallMicroflowNode); ok && cm.Agent {
			errs = appendIfSet(errs, c.checkAgentMicroflow("AI agent task "+workflowCallMicroflowLabel(cm), cm.Microflow.String()))
			return
		}
		n, ok := a.(*ast.WorkflowUserTaskNode)
		if !ok {
			return
		}
		kind := "user task"
		if n.IsMultiUser {
			kind = "multi user task"
		}
		label := kind + " " + workflowUserTaskLabel(n)
		errs = appendIfSet(errs, c.checkPage(label, n.Page.String()))
		// "microflow" and "group_microflow"; the XPath kinds name no document.
		if strings.Contains(n.Targeting.Kind, "microflow") {
			errs = appendIfSet(errs, c.checkTargeting(label, n.Targeting.Microflow.String(), contextEntity))
		}
		errs = appendIfSet(errs, c.checkOnCreated(label, n.OnCreated.String(), contextEntity))
		if n.Completion != nil && n.Completion.Rule == "microflow" {
			errs = appendIfSet(errs, c.checkDecisionMicroflow(label, n.Completion.Microflow.String()))
		}
	})
	return errs
}

// checkPage reports a task page that cannot receive the task (CE7410, CE7412).
// A page that does not resolve is left to the missing-reference check.
func (c *workflowTaskSignatureChecker) checkPage(label, pageQN string) string {
	if pageQN == "" || pageQN == "." {
		return ""
	}
	params, ok := c.pageParams(pageQN)
	if !ok {
		return ""
	}
	if len(params) == 0 {
		return fmt.Sprintf(
			"%s: page %s takes no parameters — a task page is opened with the task, so it must take a %s parameter; "+
				"the build fails CE7410 (add `params: { $WorkflowUserTask: %s }`)",
			label, pageQN, workflowUserTaskEntity, workflowUserTaskEntity)
	}
	for _, e := range params {
		if strings.EqualFold(e, workflowUserTaskEntity) {
			return ""
		}
	}
	return fmt.Sprintf(
		"%s: page %s takes %s but no %s parameter — a task page receives the task, not the workflow's context object; "+
			"the build fails CE7412 (add `$WorkflowUserTask: %s` — the other parameters may stay)",
		label, pageQN, describeEntityList(params), workflowUserTaskEntity, workflowUserTaskEntity)
}

// checkTargeting reports a targeting microflow whose parameters are not exactly
// System.Workflow plus the context entity or a generalization of it (CE6677).
func (c *workflowTaskSignatureChecker) checkTargeting(label, mfQN, contextEntity string) string {
	if mfQN == "" || mfQN == "." || contextEntity == "" || contextEntity == "." {
		return ""
	}
	sig, ok := c.flowSignature(mfQN)
	if !ok || sig == nil || !c.targetingMismatch(sig, contextEntity) {
		return ""
	}
	return fmt.Sprintf(
		"%s: targeting microflow %s takes %s — a targeting microflow must take exactly two parameters, %s and the "+
			"workflow's context entity %s (or a generalization of it), in either order; the build fails CE6677",
		label, mfQN, describeFlowParams(sig), workflowEntity, contextEntity)
}

// targetingMismatch reports a signature that is PROVABLY wrong. When the
// context entity's inheritance chain leaves what can be resolved, the answer is
// "not proven", which is not a refusal.
func (c *workflowTaskSignatureChecker) targetingMismatch(sig *flowSignature, contextEntity string) bool {
	return c.pairMismatch(sig, workflowEntity, contextEntity)
}

type inheritance int

const (
	inheritanceUnknown inheritance = iota
	inheritanceYes
	inheritanceNo
)

// contextAssignableTo reports whether an object of contextQN can be passed where
// paramQN is declared: it IS paramQN, or derives from it. A specialization does
// not qualify — measured, CE6677.
func (c *workflowTaskSignatureChecker) contextAssignableTo(contextQN, paramQN string) inheritance {
	seen := map[string]bool{}
	for qn := contextQN; ; {
		if strings.EqualFold(qn, paramQN) {
			return inheritanceYes
		}
		key := strings.ToLower(qn)
		if seen[key] {
			return inheritanceUnknown // a cycle is not a model to judge
		}
		seen[key] = true
		parent, resolved := c.generalizationOf(qn)
		if !resolved {
			return inheritanceUnknown
		}
		if parent == "" {
			return inheritanceNo
		}
		qn = parent
	}
}

func (c *workflowTaskSignatureChecker) generalizationOf(qn string) (string, bool) {
	if c.sc != nil {
		if g, ok := c.sc.entityGeneralizations[strings.ToLower(qn)]; ok {
			return g, true
		}
	}
	e, ok := findEntityByQN(c.ctx.Backend, qn)
	if !ok || e == nil {
		return "", false
	}
	return e.GeneralizationRef, true
}

func (c *workflowTaskSignatureChecker) pageParams(qn string) ([]string, bool) {
	key := strings.ToLower(qn)
	if c.sc != nil {
		if p, ok := c.sc.pageParams[key]; ok {
			return p, true
		}
	}
	if c.pages == nil {
		c.pages = buildPageParamEntities(c.ctx)
	}
	p, ok := c.pages[key]
	return p, ok
}

func (c *workflowTaskSignatureChecker) flowSignature(qn string) (*flowSignature, bool) {
	key := strings.ToLower(qn)
	if c.sc != nil {
		if s, ok := c.sc.flowParams[key]; ok {
			return s, true
		}
	}
	if c.flows == nil {
		c.flows = buildFlowSignatures(c.ctx)
	}
	s, ok := c.flows[key]
	return s, ok
}

// buildPageParamEntities maps each stored page (lower-cased qualified name) to
// its parameters' entity names, "" for a non-entity parameter. A page with no
// parameters is present with an empty list: "known, takes nothing" is CE7410,
// "not found" is someone else's report.
func buildPageParamEntities(ctx *ExecContext) map[string][]string {
	out := map[string][]string{}
	h, err := getHierarchy(ctx)
	if err != nil || h == nil {
		return out
	}
	all, err := ctx.Backend.ListPages()
	if err != nil {
		return out
	}
	for _, p := range all {
		if p == nil {
			continue
		}
		entities := make([]string, 0, len(p.Parameters))
		for _, param := range p.Parameters {
			if param != nil {
				entities = append(entities, param.EntityName)
			}
		}
		out[strings.ToLower(h.GetQualifiedName(p.ContainerID, p.Name))] = entities
	}
	return out
}

// recordPageParams stores a script-declared page's parameter entities, for the
// task page check. Same reason as flowParams: the page and the workflow using it
// are usually one script.
func (sc *scriptContext) recordPageParams(qualifiedName string, params []ast.PageParameter) {
	entities := make([]string, 0, len(params))
	for _, p := range params {
		entity := astDataTypeEntity(p.Type)
		if entity == "" {
			if qn := p.EntityType.String(); qn != "." {
				entity = qn
			}
		}
		entities = append(entities, entity)
	}
	sc.pageParams[strings.ToLower(qualifiedName)] = entities
}

// recordEntityGeneralization stores a script-declared entity's generalization
// ("" for none), so the targeting check can walk an inheritance chain that
// starts in the script and continues in the project.
func (sc *scriptContext) recordEntityGeneralization(s *ast.CreateEntityStmt) {
	parent := ""
	if s.Generalization != nil {
		parent = s.Generalization.String()
	}
	sc.entityGeneralizations[strings.ToLower(s.Name.String())] = parent
}

func describeEntityList(entities []string) string {
	parts := make([]string, len(entities))
	for i, e := range entities {
		if e == "" {
			e = "a non-entity value"
		}
		parts[i] = e
	}
	return strings.Join(parts, ", ")
}

func describeFlowParams(sig *flowSignature) string {
	if len(sig.Params) == 0 {
		return "no parameters"
	}
	parts := make([]string, len(sig.Params))
	for i, p := range sig.Params {
		parts[i] = p.Entity
		if p.Entity == "" {
			parts[i] = "a non-entity value"
		}
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func appendIfSet(errs []string, msg string) []string {
	if msg == "" {
		return errs
	}
	return append(errs, msg)
}
