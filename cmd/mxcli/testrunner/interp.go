// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"fmt"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/mendixexpr"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// The interpreter route.
//
// A test normally becomes a generated microflow that has to be injected and
// built before it can run. When the app already hosts the endpoint with
// --allow-call, and every statement in a test is one the endpoint's executor
// supports, the test is compiled here instead into a list of steps and sent to
// the running app, which carries them out with the Mendix Core API and hands
// every expression to Core.evaluateExpression. Nothing is injected and nothing
// is built, so editing a test costs one HTTP round trip.
//
// .test.mdl stays the only test language: this is a second way to run the same
// test, never a second way to write one. The steps are an internal format
// between this compiler and the endpoint Java mxcli itself generates, so they
// are versioned and free to change.
//
// Everything that can be settled offline is settled here — which statements
// are supported, the XPath of a retrieve, the Mendix text of each expression,
// and that every variable is declared before it is read — so a mistake is
// reported before anything reaches the app. The executor re-checks variable
// names and enumeration values itself, because Core.evaluateExpression returns
// empty for an unknown one instead of failing (spike in
// mdl-examples/spikes/evaluate-expression.mdl), and an @expect comparing
// against that empty would report a pass it never earned.
//
// A test outside the supported set is not an error: it takes the generated
// microflow route, with the same verdicts. The verdict strings are the ones the
// generated microflows return (verdictPass, verdictFailPrefix,
// verdictSetupPrefix), so results are read by the same code either way.

// execProtocolVersion is the version of the step format. The endpoint refuses a
// version it does not know, so an attach to an app started by another mxcli
// says so instead of misreading the steps.
const execProtocolVersion = 1

// execProgram is one test, compiled for the executor.
type execProgram struct {
	V    int    `json:"v"`
	Test string `json:"test"`
	// Setups are the @setup microflows, called without arguments before the body.
	Setups []string   `json:"setups,omitempty"`
	Steps  []execStep `json:"steps"`
	// Expects are evaluated in order after the body; the first failure is the verdict.
	Expects []execExpect `json:"expects,omitempty"`
	// Throws is set for an @throws test; Want is empty for a bare @throws.
	Throws *execThrows `json:"throws,omitempty"`
	// Rollback wraps the test in a transaction that is rolled back afterwards.
	Rollback bool `json:"rollback"`
}

type execThrows struct {
	Want string `json:"want"`
}

// execStep is one statement of a test body. Op decides which fields apply.
type execStep struct {
	Op string `json:"op"` // declare, set, create, change, commit, delete, call, retrieve, count, aggregate
	// Var is the variable the step writes (declare, set, create, call output,
	// retrieve, count) or the object it acts on (change, commit, delete).
	Var    string `json:"var,omitempty"`
	Expr   string `json:"expr,omitempty"`   // declare, set
	List   bool   `json:"list,omitempty"`   // declare of a list type
	Entity string `json:"entity,omitempty"` // create, retrieve
	// Members are a create's or change's assignments, in the order written.
	Members []execMember `json:"members,omitempty"`
	// Commit is set on create/change ... commit, and on commit; Events says
	// whether before/after-commit events run.
	Commit bool      `json:"commit,omitempty"`
	Events bool      `json:"events,omitempty"`
	Flow   string    `json:"flow,omitempty"` // call
	Args   []execArg `json:"args,omitempty"` // call
	XPath  string    `json:"xpath,omitempty"`
	First  bool      `json:"first,omitempty"`
	Of     string    `json:"of,omitempty"`    // count, aggregate: the list variable
	Agg    string    `json:"agg,omitempty"`   // aggregate: count, sum, average, minimum, maximum
	Attr   string    `json:"attr,omitempty"`  // aggregate: the attribute summed or compared
	Assoc  string    `json:"assoc,omitempty"` // retrieve over an association: the association
	From   string    `json:"from,omitempty"`  // retrieve over an association: the start object
	Line   int       `json:"line,omitempty"`
}

type execMember struct {
	Name string `json:"name"`
	Expr string `json:"expr"`
}

type execArg struct {
	Name string `json:"name"`
	Expr string `json:"expr"`
}

type execExpect struct {
	Raw       string `json:"raw"`
	Condition string `json:"condition"`
	Actual    string `json:"actual,omitempty"`
}

// compileForExec compiles a test for the executor. It returns the reason the
// test cannot take the interpreter route when it cannot; that is a routing
// decision, not a failure of the test.
func compileForExec(tc TestCase) (*execProgram, string) {
	if len(tc.AssertionErrors) > 0 {
		return nil, "the test has an assertion that cannot be evaluated"
	}
	body, err := parseTestBody(tc)
	if err != nil {
		return nil, err.Error()
	}

	p := &execProgram{
		V:        execProtocolVersion,
		Test:     tc.Name,
		Setups:   append([]string(nil), tc.Setups...),
		Rollback: rollsBack(tc),
	}
	if tc.expectsThrow() {
		p.Throws = &execThrows{Want: tc.Throws}
	}

	declared := map[string]bool{}
	use := func(expr string) string {
		if name := firstUndeclared(expr, declared); name != "" {
			return fmt.Sprintf("$%s is read before anything declares it", name)
		}
		return ""
	}
	for _, st := range body {
		step, reason := compileStep(st, use)
		if reason != "" {
			return nil, reason
		}
		if step.Var != "" && step.Op != "change" && step.Op != "commit" && step.Op != "delete" {
			declared[step.Var] = true
		}
		p.Steps = append(p.Steps, step)
	}

	if p.Throws == nil {
		for _, exp := range tc.Expects {
			for _, agg := range exp.Aggregates {
				if agg.Op != "COUNT" {
					return nil, "the @expect aggregate " + agg.Op + " is not supported by the executor"
				}
				v, list := strings.TrimPrefix(agg.Var, "$"), strings.TrimPrefix(agg.List, "$")
				if !declared[list] {
					return nil, fmt.Sprintf("$%s is read before anything declares it", list)
				}
				if !declared[v] {
					p.Steps = append(p.Steps, execStep{Op: "count", Var: v, Of: list})
					declared[v] = true
				}
			}
			if r := use(exp.Condition); r != "" {
				return nil, r
			}
			if r := use(exp.Actual); r != "" {
				return nil, r
			}
			p.Expects = append(p.Expects, execExpect{Raw: exp.Raw, Condition: exp.Condition, Actual: exp.Actual})
		}
	}
	return p, ""
}

// parseTestBody parses a test's statements exactly as the generators read them:
// inside the same wrapper and under the same language version.
func parseTestBody(tc TestCase) ([]ast.MicroflowStatement, error) {
	src := bodyParsePrefix(tc.Version) + tc.MDL + bodyParseSuffix
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		return nil, fmt.Errorf("the test body does not parse: %v", errs[0])
	}
	for _, st := range prog.Statements {
		if mf, ok := st.(*ast.CreateMicroflowStmt); ok {
			return mf.Body, nil
		}
	}
	return nil, fmt.Errorf("the test body does not parse as a microflow body")
}

// compileStep compiles one statement, or says why the executor cannot run it.
func compileStep(st ast.MicroflowStatement, use func(string) string) (execStep, string) {
	expr := func(e ast.Expression) (string, string) {
		s := mendixexpr.String(e)
		return s, use(s)
	}
	members := func(changes []ast.ChangeItem) ([]execMember, string) {
		var out []execMember
		for _, ch := range changes {
			if ch.Op != ast.MemberSet {
				return nil, "adding to or removing from a reference set (+= / -=)"
			}
			s, r := expr(ch.Value)
			if r != "" {
				return nil, r
			}
			out = append(out, execMember{Name: ch.Attribute, Expr: s})
		}
		return out, ""
	}

	switch s := st.(type) {
	case *ast.DeclareStmt:
		if r := plainErrorHandling(s.ErrorHandling); r != "" {
			return execStep{}, r
		}
		step := execStep{Op: "declare", Var: s.Variable, List: s.Type.Kind == ast.TypeListOf}
		if s.InitialValue != nil {
			e, r := expr(s.InitialValue)
			if r != "" {
				return execStep{}, r
			}
			step.Expr = e
		}
		return step, ""

	case *ast.MfSetStmt:
		if r := plainErrorHandling(s.ErrorHandling); r != "" {
			return execStep{}, r
		}
		target := strings.TrimPrefix(s.Target, "$")
		if strings.Contains(target, "/") {
			return execStep{}, "set on an attribute path (write change instead)"
		}
		if r := use("$" + target); r != "" {
			return execStep{}, r
		}
		e, r := expr(s.Value)
		if r != "" {
			return execStep{}, r
		}
		return execStep{Op: "set", Var: target, Expr: e}, ""

	case *ast.CreateObjectStmt:
		if r := plainErrorHandling(s.ErrorHandling); r != "" {
			return execStep{}, r
		}
		m, r := members(s.Changes)
		if r != "" {
			return execStep{}, r
		}
		commit, events := commitFlag(s.Commit)
		return execStep{Op: "create", Var: s.Variable, Entity: s.EntityType.String(), Members: m,
			Commit: commit, Events: events}, ""

	case *ast.ChangeObjectStmt:
		if r := plainErrorHandling(s.ErrorHandling); r != "" {
			return execStep{}, r
		}
		if r := use("$" + s.Variable); r != "" {
			return execStep{}, r
		}
		m, r := members(s.Changes)
		if r != "" {
			return execStep{}, r
		}
		commit, events := commitFlag(s.Commit)
		return execStep{Op: "change", Var: s.Variable, Members: m, Commit: commit, Events: events}, ""

	case *ast.MfCommitStmt:
		if r := plainErrorHandling(s.ErrorHandling); r != "" {
			return execStep{}, r
		}
		if r := use("$" + s.Variable); r != "" {
			return execStep{}, r
		}
		return execStep{Op: "commit", Var: s.Variable, Commit: true, Events: !s.WithoutEvents}, ""

	case *ast.DeleteObjectStmt:
		if r := plainErrorHandling(s.ErrorHandling); r != "" {
			return execStep{}, r
		}
		if r := use("$" + s.Variable); r != "" {
			return execStep{}, r
		}
		return execStep{Op: "delete", Var: s.Variable}, ""

	case *ast.CallMicroflowStmt:
		if s.Queue != nil {
			return execStep{}, "a call in a task queue"
		}
		if r := plainErrorHandling(s.ErrorHandling); r != "" {
			return execStep{}, r
		}
		step := execStep{Op: "call", Var: s.OutputVariable, Flow: s.MicroflowName.String()}
		for _, a := range s.Arguments {
			e, r := expr(a.Value)
			if r != "" {
				return execStep{}, r
			}
			step.Args = append(step.Args, execArg{Name: a.Name, Expr: e})
		}
		return step, ""

	case *ast.RetrieveStmt:
		if r := plainErrorHandling(s.ErrorHandling); r != "" {
			return execStep{}, r
		}
		switch {
		case s.StartVariable != "" && (s.Where != nil || len(s.SortColumns) > 0 || s.First || s.Limit != "" || s.Offset != ""):
			return execStep{}, "a retrieve over an association with where, sort by, first, limit or offset"
		case s.StartVariable != "":
			if r := use("$" + s.StartVariable); r != "" {
				return execStep{}, r
			}
			return execStep{Op: "retrieve", Var: s.Variable, Assoc: s.Source.String(), From: s.StartVariable}, ""
		case len(s.SortColumns) > 0:
			return execStep{}, "a retrieve with sort by"
		case s.Limit != "" || s.Offset != "":
			return execStep{}, "a retrieve with limit or offset"
		}
		entity := s.Source.String()
		xpath := "//" + entity
		if s.Where != nil {
			c := executor.RetrieveXPathConstraint(s.Where, entity)
			if r := use(c); r != "" {
				return execStep{}, r
			}
			xpath += strings.Join(strings.Fields(c), " ")
		}
		return execStep{Op: "retrieve", Var: s.Variable, Entity: entity, XPath: xpath, First: s.First}, ""

	case *ast.AggregateListStmt:
		if r := plainErrorHandling(s.ErrorHandling); r != "" {
			return execStep{}, r
		}
		agg, ok := execAggregates[s.Operation]
		if !ok || s.IsExpression {
			return execStep{}, "the statement " + statementKind(st)
		}
		if r := use("$" + s.InputVariable); r != "" {
			return execStep{}, r
		}
		return execStep{Op: "aggregate", Var: s.OutputVariable, Of: s.InputVariable, Agg: agg, Attr: s.Attribute}, ""
	}
	return execStep{}, "the statement " + statementKind(st)
}

// execAggregates are the list aggregates the executor computes. The expression
// form (`sum $l by expression`), reduce, all and any are not among them.
var execAggregates = map[ast.AggregateListOperationType]string{
	ast.AggregateCount:   "count",
	ast.AggregateSum:     "sum",
	ast.AggregateAverage: "average",
	ast.AggregateMinimum: "minimum",
	ast.AggregateMaximum: "maximum",
}

// plainErrorHandling accepts the error handling the executor reproduces: none,
// or `on error rollback`, which is the same model. Anything else changes what
// the test means when a call fails, and only the microflow route runs it as
// written.
func plainErrorHandling(h *ast.ErrorHandlingClause) string {
	if h == nil || h.Type == ast.ErrorHandlingRollback {
		return ""
	}
	return "an ON ERROR clause other than rollback"
}

// commitFlag maps a create/change commit setting to the executor's two flags.
func commitFlag(c ast.CommitFlag) (commit, events bool) {
	switch c {
	case ast.CommitYes:
		return true, true
	case ast.CommitYesWithoutEvents:
		return true, false
	}
	return false, false
}

// statementKind names a statement for a routing message the way the test's
// author wrote it: "the statement count / sum … by" tells them what to rewrite,
// "the statement AggregateList" does not.
func statementKind(st ast.MicroflowStatement) string {
	switch s := st.(type) {
	case *ast.AggregateListStmt:
		switch {
		case s.Operation == ast.AggregateReduce:
			return "reduce"
		case s.Operation == ast.AggregateAll || s.Operation == ast.AggregateAny:
			return "all / any"
		case s.IsExpression:
			return "an aggregate by expression (" + strings.ToLower(s.Operation.String()) + " … by an expression)"
		}
	case *ast.ListOperationStmt:
		return "a list operation (" + strings.ToLower(s.Operation.String()) + ")"
	}
	name := strings.TrimSuffix(strings.TrimPrefix(fmt.Sprintf("%T", st), "*ast."), "Stmt")
	if mdl, ok := mdlStatementNames[name]; ok {
		return mdl
	}
	return name
}

// mdlStatementNames spells statement types the way MDL writes them.
var mdlStatementNames = map[string]string{
	"If": "if", "EnumSplit": "case", "InheritanceSplit": "split type", "Loop": "loop", "While": "while",
	"Break": "break", "Continue": "continue", "Return": "return", "RaiseError": "raise error",
	"CastObject": "cast", "Rollback": "rollback", "CallJavaAction": "call java action",
	"CallNanoflow": "call nanoflow", "CallJavaScriptAction": "call javascript action",
	"CallExternalAction": "call external action", "CallWebService": "call web service",
	"RestCall": "rest call", "SendRestRequest": "send rest request", "ImportFromMapping": "import from mapping",
	"ExportToMapping": "export to mapping", "TransformJson": "transform json",
	"ExecuteDatabaseQuery": "execute database query", "CreateList": "create list", "AddToList": "add to",
	"RemoveFromList": "remove from", "ClearList": "clear", "Log": "log", "ShowPage": "show page",
	"ClosePage": "close page", "ShowMessage": "show message", "ValidationFeedback": "validation feedback",
	"SendEmail": "send email", "DownloadFile": "download file",
}

// expressionVariables lists the $variables an expression in Mendix's grammar
// reads, in order, skipping string literals. The executor runs the same scan
// before every evaluation (endpoint_exec.go), so the two cannot disagree about
// what a name is.
func expressionVariables(expr string) []string {
	var out []string
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		if c == '\'' {
			// A literal ends at the next apostrophe that is not doubled.
			for i++; i < len(expr); i++ {
				if expr[i] == '\'' {
					if i+1 < len(expr) && expr[i+1] == '\'' {
						i++
						continue
					}
					break
				}
			}
			continue
		}
		if c != '$' {
			continue
		}
		j := i + 1
		for j < len(expr) && isIdentByte(expr[j]) {
			j++
		}
		if j > i+1 {
			out = append(out, expr[i+1:j])
		}
		i = j - 1
	}
	return out
}

func isIdentByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// executorBuiltins are the variables Mendix provides without a declaration.
var executorBuiltins = map[string]bool{"currentUser": true, "currentSession": true}

// firstUndeclared returns the first variable expr reads that nothing has declared.
func firstUndeclared(expr string, declared map[string]bool) string {
	for _, v := range expressionVariables(expr) {
		if !declared[v] && !executorBuiltins[v] {
			return v
		}
	}
	return ""
}

// compileSuiteForExec compiles every test it can for the executor. The others
// are returned with the reason each needs the microflow route: a run sends the
// first kind to the running app and injects and builds only the second, so one
// unsupported statement no longer costs the whole suite a build. A test whose
// assertions are already an ERROR is in neither; it is reported without running.
func compileSuiteForExec(suite *TestSuite) (programs map[string]*execProgram, needBuild map[string]string) {
	programs = make(map[string]*execProgram, len(suite.Tests))
	needBuild = map[string]string{}
	for _, tc := range suite.Tests {
		if len(tc.AssertionErrors) > 0 {
			continue
		}
		p, reason := compileForExec(tc)
		if reason != "" {
			needBuild[tc.ID] = reason
			continue
		}
		programs[tc.ID] = p
	}
	return programs, needBuild
}

// suiteWith is suite with only the tests keep accepts.
func suiteWith(suite *TestSuite, keep func(TestCase) bool) *TestSuite {
	sub := *suite
	sub.Tests = nil
	for _, tc := range suite.Tests {
		if keep(tc) {
			sub.Tests = append(sub.Tests, tc)
		}
	}
	return &sub
}

// staleModelError refuses to run tests straight against an app whose model on
// disk is newer than the build it serves: the tests would exercise the old
// model, and a pass would be stale. A dev loop in --watch applies the change on
// its own, so waiting is enough; without --watch it needs a restart.
func staleModelError(projectPath string, source, built time.Time, watching bool) error {
	if !source.After(built) {
		return nil
	}
	fix := "mxcli run wait -p " + projectPath + "    # the dev loop is watching; wait for it to apply the change"
	if !watching {
		fix = "mxcli run restart -p " + projectPath + "    # the dev loop is not watching, so it never applies the change"
	}
	return fmt.Errorf("the model changed after the running app was built, so these tests would run against the old model\n  %s", fix)
}
