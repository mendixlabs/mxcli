// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// Verdict protocol. A test microflow returns one string: either verdictPass, or
// verdictFailPrefix followed by the reason. The endpoint hands that string back
// as the HTTP response's "result" field, so a result is a returned value rather
// than something recovered from the runtime log.
const (
	verdictPass       = "PASS"
	verdictFailPrefix = "FAIL:"
	// verdictSetupPrefix is followed by the setup microflow that threw. It is a
	// third outcome on purpose: the test never ran, so it neither passed nor
	// failed, and reporting a broken fixture as a FAIL blames the code under
	// test for it.
	verdictSetupPrefix = "SETUP:"
)

// GenerateTestFlows returns the MDL declaring one microflow per test case.
//
// This is the endpoint path's counterpart to GenerateTestRunner, which compiles
// the whole suite into a single after-startup microflow. One microflow per test
// buys three things that the monolith cannot give:
//
//   - Each test can be invoked, re-invoked, or skipped on its own, so --filter
//     and single-test runs are a matter of which URL is called.
//   - A test that throws fails only itself. In the monolith an uncaught error
//     ends the whole flow, and because that flow is the after-startup action it
//     also fails the boot.
//   - Every test gets its own variable scope, so the suffix-renaming the
//     monolith needs to keep `$result` in test 1 from colliding with `$result`
//     in test 2 is simply not required here.
//
// The script does not create the MxTest module: every caller runs it into a
// project that already holds one — straight after GenerateEndpointMDL, or under
// --watch and --attach into a project hosting the endpoint. A `CREATE MODULE`
// here made exec print "Module 'MxTest' already exists" on every run, on a
// project that had no MxTest module before it (ako/mxcli#1856).
func GenerateTestFlows(suite *TestSuite) string {
	var b strings.Builder
	v := suiteVersion(suite)
	writeScriptHeader(&b, v)
	b.WriteString("\n")
	if suiteHasFlows(suite) {
		writeErrorMessageFlow(&b)
		b.WriteString("\n")
	}
	for _, tc := range suite.Tests {
		// A test with an uncompilable @expect gets no microflow. The runner
		// reports it as an ERROR from the parse message, which is more useful
		// than a microflow that runs and cannot assert anything.
		if len(tc.AssertionErrors) > 0 {
			continue
		}
		writeTestFlow(&b, tc, v)
		b.WriteString("\n")
	}
	return b.String()
}

// writeTestFlow writes one test's microflow.
func writeTestFlow(b *strings.Builder, tc TestCase, v langver.Version) {
	fmt.Fprintf(b, "/** %s */\n", escapeMDLComment(tc.Name))
	if callerAppliesEntityAccess(tc) {
		b.WriteString(applyEntityAccessAnnotation + "\n")
	}
	fmt.Fprintf(b, "%s %s ()\n", createFlow, testFlowName(tc))
	b.WriteString("RETURNS String AS $Verdict\n")
	b.WriteString("BEGIN\n")
	fmt.Fprintf(b, "  DECLARE $Verdict String = '%s';\n", verdictPass)

	// Before the body, and before a @throws test pre-sets its failing verdict:
	// the fixture is not the thing expected to throw.
	writeSetupCalls(b, tc)

	if tc.expectsThrow() {
		writeThrowsFlowBody(b, tc)
	} else {
		writeExpectFlowBody(b, tc)
	}

	b.WriteString("  RETURN $Verdict;\n")
	writeFlowEnd(b)
}

// writeSetupCalls writes the @setup microflow calls that precede a test's body.
//
// Each is a plain call — a fixture is a microflow, so there is nothing to
// resolve and nothing to declare — with a handler that returns the SETUP verdict
// and stops. Continuing into a test whose preconditions were not established
// produces an assertion failure that says nothing about the code under test.
func writeSetupCalls(b *strings.Builder, tc TestCase) {
	for _, flow := range tc.Setups {
		fmt.Fprintf(b, "  CALL MICROFLOW %s() ON ERROR BEGIN\n", flow)
		fmt.Fprintf(b, "    SET $Verdict = '%s';\n",
			escapeMDLString(verdictSetupPrefix+flow))
		b.WriteString("    RETURN $Verdict;\n")
		b.WriteString("  END ERROR;\n")
	}
}

// writeExpectFlowBody writes the body of a normal test: run the MDL, then check
// each @expect. An error during the body short-circuits to a FAIL verdict.
//
// Every assertion counts itself when it is evaluated, and the test fails if the
// count falls short (writeAssertionsEvaluatedGuard). See assertionsEvaluatedVar
// for the path that skips assertions without failing anything.
func writeExpectFlowBody(b *strings.Builder, tc TestCase) {
	if len(tc.Expects) > 0 {
		fmt.Fprintf(b, "  DECLARE %s Integer = 0;\n", assertionsEvaluatedVar)
	}
	writeBodyLines(b, rewriteBodyForVerdict(strings.Split(tc.MDL, "\n"), tc), tc.Version)
	writeExpectAggregates(b, "  ", tc.Expects)
	for _, exp := range tc.Expects {
		writeExpectCheck(b, exp)
	}
	writeAssertionsEvaluatedGuard(b, len(tc.Expects))
}

// assertionsEvaluatedVar counts the assertions a test microflow actually
// evaluated.
//
// The count exists because a test can reach its RETURN without passing through
// its assertions, and nothing else would notice. A call the author gave a
// custom handler that ends normally (`on error begin log …; end error;`)
// rejoins the flow — and the microflow builder routes that rejoin past every
// statement reading the failed call's output, since the variable does not
// exist on the error path. The assertions read exactly that output. Measured on
// 11.13.0: a body whose call threw into such a handler reported PASS on
// `@expect $r = 'X'`, with $r never assigned (ako/mxcli#1448).
//
// The increments sit in both branches of each assertion's decision, so running
// one proves that the condition was evaluated; the guard after the last one
// cannot be skipped, because it reads nothing the body produced.
const assertionsEvaluatedVar = "$AssertionsEvaluated"

// assertionsSkippedReason is the failure a short count reports.
const assertionsSkippedReason = "not every assertion was evaluated: an error in the body was caught by " +
	"a call's own ON ERROR handler, and the flow resumed past the assertions that read that call's result"

// writeAssertionsEvaluatedGuard fails a test that is still passing but did not
// evaluate all n of its assertions. A test that already failed keeps its own,
// more specific verdict.
func writeAssertionsEvaluatedGuard(b *strings.Builder, n int) {
	if n == 0 {
		return
	}
	fmt.Fprintf(b, "  IF $Verdict = '%s' and %s < %d THEN\n", verdictPass, assertionsEvaluatedVar, n)
	fmt.Fprintf(b, "    SET $Verdict = '%s';\n", escapeMDLString(verdictFailPrefix+assertionsSkippedReason))
	b.WriteString("  END IF;\n")
}

// writeExpectAggregates emits the Aggregate list activities the assertions need,
// after the body has produced the lists and before the first decision reads
// them. One activity per variable, however many assertions refer to it.
func writeExpectAggregates(b *strings.Builder, indent string, expects []Expect) {
	seen := map[string]bool{}
	for _, exp := range expects {
		for _, agg := range exp.Aggregates {
			if seen[agg.Var] {
				continue
			}
			seen[agg.Var] = true
			fmt.Fprintf(b, "%s%s = %s(%s);\n", indent, agg.Var, agg.Op, agg.List)
		}
	}
}

// writeThrowsFlowBody writes the body of an @throws test: the verdict starts as
// a failure and only the error handler can clear it, so a body that completes
// without throwing fails — which is the point of the annotation.
//
// When the annotation carried a message, the handler clears the verdict only if
// the raised error's message contains it. Before that, the message was
// decorative: the handler passed on any exception at all, so a stale or simply
// wrong expectation kept reporting PASS against a completely different error
// (ako/mxcli#301).
func writeThrowsFlowBody(b *strings.Builder, tc TestCase) {
	fmt.Fprintf(b, "  SET $Verdict = '%s';\n",
		escapeMDLString(verdictFailPrefix+"expected an exception but none was thrown"))
	writeBodyLines(b, rewriteBodyForThrows(strings.Split(tc.MDL, "\n"), tc.Throws, tc.Version), tc.Version)
}

// writeExpectCheck writes one @expect assertion.
//
// The assertion is emitted as the expression the author wrote, so whatever
// Mendix can evaluate is evaluated. `<>` never reaches the model — ParseExpect
// rewrites it to `!=`, the spelling Mendix's expression engine accepts — which
// is what the old branch-swapping workaround was for.
func writeExpectCheck(b *strings.Builder, exp Expect) {
	// An earlier statement may already have failed the test; never overwrite an
	// existing failure with a later assertion's result.
	fmt.Fprintf(b, "  IF $Verdict = '%s' THEN\n", verdictPass)
	fmt.Fprintf(b, "    IF %s THEN\n", exp.Condition)
	fmt.Fprintf(b, "      SET %s = %s + 1;\n", assertionsEvaluatedVar, assertionsEvaluatedVar)
	b.WriteString("    ELSE\n")
	fmt.Fprintf(b, "      SET %s = %s + 1;\n", assertionsEvaluatedVar, assertionsEvaluatedVar)
	fmt.Fprintf(b, "      SET $Verdict = %s;\n", failVerdictExpr(exp))
	b.WriteString("    END IF;\n")
	b.WriteString("  END IF;\n")
}

// failVerdictExpr builds the MDL expression assigned to $Verdict when an
// assertion fails.
//
// When the observed value can be rendered as a String without guessing its type,
// it is concatenated onto the message. A failure that says only what was expected
// tells you nothing about what came back, which is half the value of a failing
// test.
func failVerdictExpr(exp Expect) string {
	msg := verdictFailPrefix + "expected " + exp.Raw
	if exp.Actual == "" {
		return "'" + escapeMDLString(msg) + "'"
	}
	return "'" + escapeMDLString(msg+", actual: ") + "' + " + exp.Actual
}

// rewriteBodyForVerdict attaches an ON ERROR handler to every CALL in the test
// body, turning a thrown error into a FAIL verdict and an early return.
//
// The verdict carries the raised message: a bare "exception during execution"
// left the user guessing, e.g. at an unresolvable [%CurrentUser%] token (tests
// run in a system context with no user). The message is read through
// errorMessageFlow, as in throwsHandler: the test flow applies entity access.
func rewriteBodyForVerdict(lines []string, tc TestCase) []string {
	return attachOnError(lines, tc.Version, func(n int) []string {
		msg := errorMessageVar("", n)
		return []string{
			readErrorMessage(msg),
			fmt.Sprintf("  SET $Verdict = '%s' + %s;",
				escapeMDLString(verdictFailPrefix+"exception during execution: "), msg),
			"  RETURN $Verdict;",
		}
	})
}

// rewriteBodyForThrows attaches an ON ERROR handler that clears the pre-set
// failure verdict — the error is the expected outcome.
//
// With a want, the handler clears it only for a matching message and otherwise
// replaces it with one naming both sides, so a failing @throws says what was
// raised instead of only what was expected.
func rewriteBodyForThrows(lines []string, want string, v langver.Version) []string {
	return attachOnError(lines, v, func(n int) []string { return throwsHandler(want, errorMessageVar("", n)) })
}

// throwsHandler builds the ON ERROR body of an @throws test.
//
// $latestError is the error variable Mendix makes available inside a handler;
// its Message member is what the annotation is compared against. Measured
// against mxbuild 11.6.0 and 11.13.0 at 0 errors, with `$noSuchThing/Message` as
// the control (CE0109, "Undefined variable"). The handler reads it into msg
// through errorMessageFlow, because the test microflow applies entity access.
//
// The comparison is `contains`, so an expectation is a substring of the raised
// message: a Mendix error message routinely carries an activity name or an
// object id the test cannot predict, and demanding equality would make almost
// every real expectation unwritable.
func throwsHandler(want, msgVar string) []string {
	if want == "" {
		return []string{fmt.Sprintf("  SET $Verdict = '%s';", verdictPass)}
	}
	msg := verdictFailPrefix + "expected an error containing " + quoteForMessage(want) + ", actual: "
	return []string{
		readErrorMessage(msgVar),
		fmt.Sprintf("  IF contains(%s, '%s') THEN", msgVar, escapeMDLString(want)),
		fmt.Sprintf("    SET $Verdict = '%s';", verdictPass),
		"  ELSE",
		fmt.Sprintf("    SET $Verdict = '%s' + %s;", escapeMDLString(msg), msgVar),
		"  END IF;",
	}
}

// quoteForMessage renders the expected message for a human-readable verdict,
// before that verdict is itself escaped for MDL.
func quoteForMessage(s string) string {
	return "'" + s + "'"
}

// attachOnError gives each call in the body that has no error handling of its
// own the runner's `ON ERROR BEGIN … END ERROR`, so an error it raises becomes a
// verdict instead of escaping the test microflow. Every rewriting generator —
// both runners, with and without @throws — goes through here, so they cannot
// disagree about which calls are wrapped.
//
// A call the author already gave a clause keeps it, and gets no second one: two
// clauses on one activity is not MDL, and the generated script stopped the run
// at injection with a parse error on a line the author never wrote
// (ako/mxcli#1448). What the author wrote is what runs:
//
//   - `on error continue` swallows the error, so the test goes on and its
//     assertions decide — exactly what the production statement does.
//   - A custom handler (`on error [without rollback] begin … end error`) runs
//     as written. One that ends normally rejoins the flow; one that re-raises
//     lets the error leave the test microflow, which the endpoint reports as an
//     ERROR — never a pass.
//   - `on error rollback` is the exception, because it is not a handler: it is
//     Mendix's default spelled out, the ErrorHandlingType a call without a
//     clause stores too. The two spellings are one model and get one verdict,
//     so the clause is replaced by the runner's handler, which also rolls back.
//     Keeping it would turn the same exception into an ERROR instead of a FAIL,
//     fail an @throws test that should pass, and under the legacy runner abort
//     the whole suite's after-startup flow.
//
// Statements are found by parsing the body, not by searching its text: the
// clause may be on a later line, a custom handler holds statements (and
// semicolons) of its own, and `on error` may be inside a string or a comment.
// A body that does not parse is returned unchanged — no handler can make it
// parse, and the error the author then sees is about their own text.
//
// handler is called once per wrapped call with its number, from 1 in source
// order, so a handler that declares a variable can give each its own name.
func attachOnError(lines []string, v langver.Version, handler func(n int) []string) []string {
	body := strings.Join(lines, "\n")
	prefix := bodyParsePrefix(v)
	stmts, ok := visitor.FlowStatements(prefix + body + bodyParseSuffix)
	if !ok {
		return lines
	}

	runes := []rune(body)
	// The handler is laid out at the indentation of the line its call starts
	// on, so a call nested in an IF or a loop reads as nested in --verbose.
	n := 0
	block := func(callStart int) string {
		n++
		lineStart := callStart
		for lineStart > 0 && runes[lineStart-1] != '\n' {
			lineStart--
		}
		indent := lineStart
		for indent < callStart && (runes[indent] == ' ' || runes[indent] == '\t') {
			indent++
		}
		pad := string(runes[lineStart:indent])
		var b strings.Builder
		b.WriteString("ON ERROR BEGIN\n")
		for _, l := range handler(n) {
			b.WriteString(pad + l + "\n")
		}
		b.WriteString(pad + "END ERROR")
		return b.String()
	}
	type edit struct {
		start, stop int
		text        string
	}
	var edits []edit
	offset := len([]rune(prefix))
	for _, s := range stmts {
		if !errorRaisingCallRules[s.Rule] {
			continue
		}
		switch {
		case s.OnError == nil:
			edits = append(edits, edit{s.Stop - offset, s.Stop - offset, " " + block(s.Start-offset)})
		case s.OnError.Type == ast.ErrorHandlingRollback:
			edits = append(edits, edit{s.OnError.Start - offset, s.OnError.Stop - offset, block(s.Start - offset)})
		}
	}
	if len(edits) == 0 {
		return lines
	}

	// Back to front, so an edit never moves the offsets of one still to come.
	// They cannot overlap: the only statements inside another are in a custom
	// handler's body, and a call with a custom handler is never edited itself.
	for i := len(edits) - 1; i >= 0; i-- {
		e := edits[i]
		runes = append(runes[:e.start:e.start], append([]rune(e.text), runes[e.stop:]...)...)
	}
	return strings.Split(string(runes), "\n")
}

// bodyParsePrefix is the text a test body is parsed after, wherever the
// generators have to read it rather than copy it: inside the same kind of
// wrapper they put it in and under the same language version, so it reads
// exactly as the generated script will. bodyParseSuffix closes it.
func bodyParsePrefix(v langver.Version) string {
	var wrap strings.Builder
	writeScriptHeader(&wrap, v)
	wrap.WriteString(createFlow + " " + mxTestModule + ".Body ()\nBEGIN\n")
	return wrap.String()
}

const bodyParseSuffix = "\nEND;\n"

// escapeMDLComment keeps a test name from closing the javadoc block it sits in.
func escapeMDLComment(s string) string {
	return strings.ReplaceAll(s, "*/", "* /")
}
