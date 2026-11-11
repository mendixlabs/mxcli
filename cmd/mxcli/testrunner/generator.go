// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// suiteVersion is the language version a generated script is written in: the
// suite's tests', which parseTestFiles has already made sure agree. The
// generated wrapper is spelled to mean the same under every version; the test
// bodies are not, so the script has to carry their header (ako/mxcli#847).
func suiteVersion(suite *TestSuite) langver.Version {
	if len(suite.Tests) == 0 {
		return langver.V0
	}
	return suite.Tests[0].Version
}

// writeScriptHeader starts a generated script with the suite's language
// header; mdl 0 has none.
func writeScriptHeader(b *strings.Builder, v langver.Version) {
	if h := langver.HeaderFor(v); h != "" {
		b.WriteString(h + "\n")
	}
}

// createFlow is the statement that (re)defines a generated microflow:
// `CREATE OR MODIFY` under every version. Under mdl 1 `or replace` is the
// deprecated alias (MDL-DEPR001), and mdl 0 reads both spellings the same way
// but warns on the old one too — so writing it made `mxcli test` print a
// warning per test about a script the author never wrote (ako/mxcli#1855).
const createFlow = "CREATE OR MODIFY MICROFLOW"

// writeBodyLines writes a test body's lines into a generated flow, indented —
// except a line that starts inside a string literal, whose leading whitespace
// is part of the value. A literal may span lines, and under mdl 1 that is the
// only way to put a line break in one (`fmt --upgrade` rewrites mdl 0's `\n`
// to it); indenting its continuation changed the string the test ran with
// (ako/mxcli#847).
func writeBodyLines(b *strings.Builder, lines []string, v langver.Version) {
	inString, inComment := false, false
	for _, line := range lines {
		if !inString && !inComment {
			b.WriteString("  ")
		}
		b.WriteString(line)
		b.WriteString("\n")
		inString, inComment = scanLiteralState(line, inString, inComment, v)
	}
}

// scanLiteralState reports whether the text after line is still inside a
// single-quoted string literal or a `/* */` comment, given the state line
// started in. A `--` comment ends with its line. A quote is escaped by
// doubling it, which two toggles handle; under mdl 0 a backslash also escapes
// the character after it (MDL-V1-ESCAPE).
func scanLiteralState(line string, inString, inComment bool, v langver.Version) (bool, bool) {
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case inComment:
			if c == '*' && i+1 < len(line) && line[i+1] == '/' {
				inComment = false
				i++
			}
		case inString:
			switch {
			case c == '\\' && v < langver.V1:
				i++
			case c == '\'':
				inString = false
			}
		case c == '\'':
			inString = true
		case c == '-' && i+1 < len(line) && line[i+1] == '-':
			return false, false
		case c == '/' && i+1 < len(line) && line[i+1] == '*':
			inComment = true
			i++
		}
	}
	return inString, inComment
}

// writeFlowEnd closes a generated microflow with `END;` alone. `;` ends the
// statement under every version; a `/` after it is refused under mdl 1 and drew
// an MDL-V1-SLASH warning under mdl 0, once per generated microflow — the test
// format's own separator, reported back to its author as if they had written
// it as a terminator (ako/mxcli#1855).
func writeFlowEnd(b *strings.Builder) {
	b.WriteString("END;\n")
}

// GenerateTestRunner generates the MDL for a TestRunner microflow from parsed test cases.
// The generated microflow:
// - Wraps each test in LOG+CALL+assertion pattern
// - Outputs structured MXTEST: log lines for result parsing
// - Returns Boolean (required by Mendix for after-startup)
func GenerateTestRunner(suite *TestSuite) string {
	var b strings.Builder
	v := suiteVersion(suite)
	writeScriptHeader(&b, v)

	// Module creation (idempotent — only if MxTest module doesn't exist)
	b.WriteString("CREATE MODULE MxTest;\n\n")

	if suiteHasFlows(suite) {
		writeErrorMessageFlow(&b)
		b.WriteString("\n")
	}
	// One microflow runs every test, so it applies entity access only if all of
	// them allow it; see callerAppliesEntityAccess.
	if callerAppliesEntityAccess(suite.Tests...) {
		b.WriteString(applyEntityAccessAnnotation + "\n")
	}
	b.WriteString(createFlow + " MxTest.TestRunner ()\n")
	b.WriteString("RETURNS Boolean AS $AllPassed\n")
	b.WriteString("BEGIN\n")
	b.WriteString("  DECLARE $AllPassed Boolean = true;\n")
	b.WriteString("  DECLARE $TestFailed Boolean = false;\n")
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  LOG INFO NODE 'MXTEST' 'MXTEST:START:%s';\n", escapeMDLString(suite.Name)))
	b.WriteString("\n")

	for i, tc := range suite.Tests {
		// A test whose @expect did not compile gets no block. The runner reports
		// it as an ERROR from the parse message; running it would report a pass
		// for an assertion that was never made.
		if len(tc.AssertionErrors) > 0 {
			continue
		}
		writeTestBlock(&b, tc, i)
		b.WriteString("\n")
	}

	b.WriteString(fmt.Sprintf("  LOG INFO NODE 'MXTEST' 'MXTEST:END:%s';\n", escapeMDLString(suite.Name)))
	b.WriteString("  RETURN $AllPassed;\n")
	writeFlowEnd(&b)

	return b.String()
}

// writeTestBlock writes the MDL for a single test case within the TestRunner.
func writeTestBlock(b *strings.Builder, tc TestCase, index int) {
	suffix := fmt.Sprintf("_%d", index+1)

	b.WriteString(fmt.Sprintf("  -- Test %d: %s\n", index+1, tc.Name))
	b.WriteString(fmt.Sprintf("  LOG INFO NODE 'MXTEST' 'MXTEST:RUN:%s:%s';\n",
		escapeMDLString(tc.ID), escapeMDLString(tc.Name)))
	b.WriteString("  SET $TestFailed = false;\n")
	writeSetupBlock(b, tc)

	if tc.expectsThrow() {
		writeThrowsTestBlock(b, tc, suffix)
		return
	}

	// Collect variable names from the MDL body and rename them with suffix
	varNames := extractVariableNames(tc.MDL, tc.Version)
	renamedMDL := renameVariables(tc.MDL, varNames, suffix)

	// Every assertion counts itself when evaluated; see assertionsEvaluatedVar.
	// On this runner a skipped assertion logs no line at all, so without the
	// guard the test would end with a RUN line and no verdict.
	evaluated := assertionsEvaluatedVar + suffix
	if len(tc.Expects) > 0 {
		b.WriteString(fmt.Sprintf("  DECLARE %s Integer = 0;\n", evaluated))
	}

	// Rewrite the MDL body: indent it and wrap CALL MICROFLOW with error handling
	lines := strings.Split(renamedMDL, "\n")
	rewritten := rewriteWithErrorHandling(lines, tc.ID, tc.Version)
	writeBodyLines(b, rewritten, tc.Version)

	// Generate assertion checks for @expect (with renamed variables)
	// Use flat IF blocks (no nesting) to avoid Mendix end-event issues
	if len(tc.Expects) > 0 {
		renamed := make([]Expect, len(tc.Expects))
		for i, exp := range tc.Expects {
			renamed[i] = renameExpect(exp, varNames, suffix)
		}
		writeExpectAggregates(b, "  ", renamed)
		for _, exp := range renamed {
			writeExpectAssertion(b, tc.ID, exp, evaluated)
		}
		b.WriteString(fmt.Sprintf("  IF %s < %d THEN\n", evaluated, len(tc.Expects)))
		b.WriteString(fmt.Sprintf("    LOG ERROR NODE 'MXTEST' '%s';\n",
			escapeMDLString("MXTEST:FAIL:"+tc.ID+":"+assertionsSkippedReason)))
		b.WriteString("    SET $AllPassed = false;\n")
		b.WriteString("  END IF;\n")
	} else if !tc.expectsThrow() {
		// No expectations — just check it didn't throw
		b.WriteString("  IF $TestFailed = false THEN\n")
		b.WriteString(fmt.Sprintf("    LOG INFO NODE 'MXTEST' 'MXTEST:PASS:%s';\n", escapeMDLString(tc.ID)))
		b.WriteString("  END IF;\n")
	}
}

// writeSetupBlock writes a test's @setup calls into the monolithic runner.
//
// The counterpart of writeSetupCalls, differing only in how it reports: this
// runner has no returned verdict to carry an outcome, so a failed setup goes out
// as an ERROR line on the log protocol. It ends the runner flow the same way a
// throwing test body does — that is this runner's existing behaviour, and one of
// the reasons the endpoint runner exists.
func writeSetupBlock(b *strings.Builder, tc TestCase) {
	for _, flow := range tc.Setups {
		fmt.Fprintf(b, "  CALL MICROFLOW %s() ON ERROR BEGIN\n", flow)
		fmt.Fprintf(b, "    LOG ERROR NODE 'MXTEST' 'MXTEST:ERROR:%s:Setup failed: %s';\n",
			escapeMDLString(tc.ID), escapeMDLString(flow))
		b.WriteString("    SET $TestFailed = true;\n")
		b.WriteString("    SET $AllPassed = false;\n")
		b.WriteString("    RETURN $AllPassed;\n")
		b.WriteString("  END ERROR;\n")
	}
}

// writeThrowsTestBlock generates code for a test that expects an error.
func writeThrowsTestBlock(b *strings.Builder, tc TestCase, suffix string) {
	didThrowVar := "$DidThrow" + suffix
	b.WriteString(fmt.Sprintf("  DECLARE %s Boolean = false;\n", didThrowVar))

	// Rename variables in the body
	varNames := extractVariableNames(tc.MDL, tc.Version)
	renamedMDL := renameVariables(tc.MDL, varNames, suffix)

	lines := strings.Split(renamedMDL, "\n")
	rewritten := rewriteForThrowsTest(lines, didThrowVar, suffix, tc.Throws, tc.Version)
	writeBodyLines(b, rewritten, tc.Version)

	// Check that it did throw
	b.WriteString(fmt.Sprintf("  IF %s THEN\n", didThrowVar))
	b.WriteString(fmt.Sprintf("    LOG INFO NODE 'MXTEST' 'MXTEST:PASS:%s';\n", escapeMDLString(tc.ID)))
	b.WriteString("  ELSE\n")
	b.WriteString(fmt.Sprintf("    LOG ERROR NODE 'MXTEST' 'MXTEST:FAIL:%s:Expected exception but none was thrown';\n",
		escapeMDLString(tc.ID)))
	b.WriteString("    SET $AllPassed = false;\n")
	b.WriteString("  END IF;\n")
}

// writeExpectAssertion generates an IF/ELSE check for a single @expect assertion.
//
// The condition is the author's own expression, guarded by $TestFailed so an
// assertion is not evaluated after the body already threw. `<>` never reaches
// the model: ParseExpect rewrites it to `!=`, which is the spelling Mendix
// accepts — the branch-swapping this function used to do was a workaround for
// emitting `<>` verbatim.
//
// Unlike the endpoint generator this path reports failures through the log
// protocol, whose message is a single log line, so the observed value is
// concatenated into that line rather than into a returned verdict.
func writeExpectAssertion(b *strings.Builder, testID string, exp Expect, evaluated string) {
	passCondition := fmt.Sprintf("$TestFailed = false and (%s)", exp.Condition)
	failMsg := "MXTEST:FAIL:" + testID + ":Expected " + exp.Raw

	b.WriteString(fmt.Sprintf("  IF %s THEN\n", passCondition))
	b.WriteString(fmt.Sprintf("    SET %s = %s + 1;\n", evaluated, evaluated))
	b.WriteString(fmt.Sprintf("    LOG INFO NODE 'MXTEST' 'MXTEST:PASS:%s';\n", escapeMDLString(testID)))
	b.WriteString("  ELSE\n")
	b.WriteString(fmt.Sprintf("    SET %s = %s + 1;\n", evaluated, evaluated))
	if exp.Actual == "" {
		b.WriteString(fmt.Sprintf("    LOG ERROR NODE 'MXTEST' '%s';\n", escapeMDLString(failMsg)))
	} else {
		b.WriteString(fmt.Sprintf("    LOG ERROR NODE 'MXTEST' '%s' + %s;\n",
			escapeMDLString(failMsg+", actual: "), exp.Actual))
	}
	b.WriteString("    SET $AllPassed = false;\n")
	b.WriteString("  END IF;\n")
}

// varPattern matches $VariableName in MDL ($ followed by word characters).
var varPattern = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)

// extractVariableNames returns the variables a test body defines, without the
// `$`: every name a statement in it creates — a call, create, retrieve, declare,
// list operation or aggregate, send rest request, execute database query, loop
// iterator, anything with an output variable — nested bodies included.
//
// The after-startup runner joins every test into one microflow, whose variable
// namespace is flat (an IF branch, a loop body or an error handler does not
// open a scope), so each of these is renamed per test. The set used to come
// from a pattern over the text that matched `$x = call|create` only: two tests
// that defined one name any other way — even `retrieve $x` — declared it twice,
// and injection failed with MDL063 (CE0111 "Duplicate variable name")
// (ako/mxcli#1775). It is now read from the parsed body through the definition
// MDL063 itself counts, so what the runner renames and what the validator
// refuses cannot drift apart. A `$x = …` that changes a variable (with or
// without `set`) defines nothing and is not in the set; its references are
// renamed with the definition they change.
//
// A body that does not parse defines nothing here, as attachOnError leaves it
// unwrapped: no renaming makes it parse, and the error the author then sees is
// about their own text.
func extractVariableNames(body string, v langver.Version) map[string]bool {
	names := make(map[string]bool)
	prog, errs := visitor.Build(bodyParsePrefix(v) + body + bodyParseSuffix)
	if len(errs) > 0 || prog == nil {
		return names
	}
	for _, stmt := range prog.Statements {
		if mf, ok := stmt.(*ast.CreateMicroflowStmt); ok {
			for _, name := range executor.FlowDefinedVariables(mf.Body) {
				names[name] = true
			}
		}
	}
	return names
}

// renameVariables replaces all occurrences of $varName with $varName_suffix
// for each variable in the names set.
func renameVariables(mdl string, names map[string]bool, suffix string) string {
	if len(names) == 0 {
		return mdl
	}

	return varPattern.ReplaceAllStringFunc(mdl, func(match string) string {
		// match is like "$result" — extract the name part
		name := match[1:] // strip $
		if names[name] {
			return "$" + name + suffix
		}
		return match
	})
}

// renameExpect applies variable renaming to an Expect assertion.
//
// The monolithic runner compiles every test into one microflow, so `$result` in
// test 1 and `$result` in test 2 have to be told apart. Renaming runs over the
// rendered condition and the actual-value expression, which is why both are kept
// as text rather than as a tree.
func renameExpect(exp Expect, names map[string]bool, suffix string) Expect {
	// An aggregate's own variable is generated, so it is not in the body's name
	// set — but it is declared inside this one shared microflow, so two tests
	// counting a list of the same name would declare it twice. It is renamed
	// with everything else.
	all := names
	if len(exp.Aggregates) > 0 {
		all = make(map[string]bool, len(names)+len(exp.Aggregates))
		for n := range names {
			all[n] = true
		}
		for _, agg := range exp.Aggregates {
			all[strings.TrimPrefix(agg.Var, "$")] = true
		}
	}
	rename := func(src string) string {
		return varPattern.ReplaceAllStringFunc(src, func(match string) string {
			if all[match[1:]] {
				return match + suffix
			}
			return match
		})
	}
	renamed := exp
	renamed.Condition = rename(exp.Condition)
	renamed.Actual = rename(exp.Actual)
	if len(exp.Aggregates) > 0 {
		renamed.Aggregates = make([]ExpectAggregate, len(exp.Aggregates))
		for i, agg := range exp.Aggregates {
			renamed.Aggregates[i] = ExpectAggregate{
				Var:  rename(agg.Var),
				Op:   agg.Op,
				List: rename(agg.List),
			}
		}
	}
	return renamed
}

// rewriteWithErrorHandling adds ON ERROR clauses to the calls in the body.
// The error handler ends with RETURN $AllPassed to properly terminate the flow path
// with a return value (required when the microflow returns a type). Which calls
// get one is attachOnError's decision, shared with the endpoint runner.
func rewriteWithErrorHandling(lines []string, testID string, v langver.Version) []string {
	return attachOnError(lines, v, func(int) []string {
		return []string{
			fmt.Sprintf("  LOG ERROR NODE 'MXTEST' 'MXTEST:FAIL:%s:Exception during execution';",
				escapeMDLString(testID)),
			"  SET $TestFailed = true;",
			"  SET $AllPassed = false;",
			"  RETURN $AllPassed;",
		}
	})
}

// rewriteForThrowsTest wraps each call in the body with ON ERROR that sets the throw flag.
//
// The flag is set only when the raised message matches the annotation's, so this
// runner reports a wrong expectation the same way the endpoint runner does — as
// "expected an exception but none was thrown", which is the only outcome its log
// protocol has for a throws test that did not get what it asked for.
func rewriteForThrowsTest(lines []string, didThrowVar, suffix, want string, v langver.Version) []string {
	return attachOnError(lines, v, func(n int) []string {
		return throwsFlagHandler(didThrowVar, want, errorMessageVar(suffix, n))
	})
}

// throwsFlagHandler sets the did-throw flag, gated on the expected message when
// the annotation carried one. See throwsHandler for why the match is a substring
// and where the message in msgVar comes from.
func throwsFlagHandler(didThrowVar, want, msgVar string) []string {
	if want == "" {
		return []string{fmt.Sprintf("  SET %s = true;", didThrowVar)}
	}
	return []string{
		readErrorMessage(msgVar),
		fmt.Sprintf("  IF contains(%s, '%s') THEN", msgVar, escapeMDLString(want)),
		fmt.Sprintf("    SET %s = true;", didThrowVar),
		"  END IF;",
	}
}

// errorRaisingCallRules are the statements the runner must catch an error from:
// the call-shaped activities, named by their rule in MDLMicroflow.g4, each of
// which takes an onErrorClause there. TestErrorRaisingCallRulesAreGrammarRules
// fails if one is renamed, which would otherwise silently stop wrapping it.
//
// It used to match CALL MICROFLOW and CALL NANOFLOW only, so an error raised by
// CALL JAVA ACTION escaped the test microflow — @throws could not catch it and
// a test without one was reported as ERROR instead of FAIL (ako/mxcli#1164).
// It is a set of parsed statements rather than a pattern over a line, so a
// string or a comment that mentions a call is not one.
var errorRaisingCallRules = map[string]bool{
	"callMicroflowStatement":        true,
	"callNanoflowStatement":         true,
	"callJavaActionStatement":       true,
	"callExternalActionStatement":   true,
	"callWebServiceStatement":       true,
	"callWorkflowStatement":         true,
	"restCallStatement":             true,
	"sendRestRequestStatement":      true,
	"executeDatabaseQueryStatement": true,
}

// escapeMDLString escapes single quotes for MDL string literals.
func escapeMDLString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
