// SPDX-License-Identifier: Apache-2.0

// The generated test callers apply entity access (ako/mxcli#2100, upstream
// mendixlabs/mxcli#1805).
//
// A test calls the project's microflows, and MxBuild refuses a microflow that
// does not apply entity access calling one that does (CE0114). The callers —
// each endpoint test microflow, the legacy runner's MxTest.TestRunner, and the
// MxTest.Check_* wrappers `mxcli check` and the editor see — were written
// without the setting, so a test could not call a production microflow that
// applies it, and changing the production microflow to suit the harness is no
// fix. The rule has one direction (MDL-SEC22's table; RegisterEndpoint applies
// entity access for the same reason, ako/mxcli#1654), so an applying caller
// may call either kind.
//
// Applying entity access costs one thing, measured with mx check on 11.12.1 at
// security Off and Production: the flow may not read an attribute of
// System.Error or System.SoapFault (CE0150). The generated handlers read the
// raised message, so they pass $latestError to errorMessageFlow, a helper that
// does not apply entity access — what CE0150's own text prescribes. An applying
// flow reading an ordinary entity's attribute is 0 errors at both levels, so
// nothing else in a test body is affected.
//
// Nothing changes at run time: every test runs under a system context
// (Core.createSystemContext), which entity access does not restrict.
package testrunner

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/langver"
)

const applyEntityAccessAnnotation = "@applyentityaccess"

// callerAppliesEntityAccess reports whether the generated caller of these
// tests applies entity access: yes, unless a test body itself reads an error
// object's attributes. Such a body — a custom handler inspecting $latestError —
// builds today in a non-applying caller and would fail CE0150 in an applying
// one, so it keeps the setting it had. A test that both reads the error and
// calls an applying microflow cannot be served by either setting; it has to
// pass the error to a non-applying microflow of its own, as Mendix says.
func callerAppliesEntityAccess(tests ...TestCase) bool {
	for _, tc := range tests {
		if readsErrorAttributes(tc.MDL, tc.Version) {
			return false
		}
	}
	return true
}

// errorAttributeAccess is a member read on the variables Mendix binds to a
// raised error inside a handler, matched without regard to case so no spelling
// the model accepts slips past: a miss would cost the test its build.
var errorAttributeAccess = regexp.MustCompile(`(?i)\$latest(error|soapfault)\s*/`)

// readsErrorAttributes reports whether a test body reads an attribute of the
// error being handled, outside string literals and comments.
func readsErrorAttributes(body string, v langver.Version) bool {
	return errorAttributeAccess.MatchString(maskLiteralsAndComments(body, v))
}

// maskLiteralsAndComments blanks the text inside string literals and comments,
// with the same rules as scanLiteralState, so a pattern matched against the
// result only sees code.
func maskLiteralsAndComments(body string, v langver.Version) string {
	b := []byte(body)
	inString, inLineComment, inBlockComment := false, false, false
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case c == '\n':
			inLineComment = false
		case inLineComment:
			b[i] = ' '
		case inBlockComment:
			if c == '*' && i+1 < len(b) && b[i+1] == '/' {
				inBlockComment = false
				b[i], b[i+1] = ' ', ' '
				i++
				continue
			}
			b[i] = ' '
		case inString:
			switch {
			case c == '\\' && v < langver.V1 && i+1 < len(b):
				b[i] = ' '
				if b[i+1] != '\n' {
					b[i+1] = ' '
				}
				i++
			case c == '\'':
				inString = false
			default:
				b[i] = ' '
			}
		case c == '\'':
			inString = true
		case c == '-' && i+1 < len(b) && b[i+1] == '-':
			inLineComment = true
			b[i] = ' '
		case c == '/' && i+1 < len(b) && b[i+1] == '*':
			inBlockComment = true
			b[i], b[i+1] = ' ', ' '
			i++
		}
	}
	return string(b)
}

// suiteHasFlows reports whether a generated script writes any test microflow,
// and so needs errorMessageFlow: a test whose @expect did not compile gets none.
func suiteHasFlows(suite *TestSuite) bool {
	for _, tc := range suite.Tests {
		if len(tc.AssertionErrors) == 0 {
			return true
		}
	}
	return false
}

// writeErrorMessageFlow defines errorMessageFlow. It does not apply entity
// access — that is its whole purpose. It is written with the test microflows
// rather than with the endpoint, so a run attached to an endpoint an older
// mxcli installed still has it, and it is removed with them.
func writeErrorMessageFlow(b *strings.Builder) {
	b.WriteString("/** Returns a raised error's message to a test microflow, which applies entity access and may not read it (CE0150). */\n")
	b.WriteString(createFlow + " " + errorMessageFlow + " (RaisedError: System.Error)\n")
	b.WriteString("RETURNS String AS $Message\n")
	b.WriteString("BEGIN\n")
	b.WriteString("  DECLARE $Message String = $RaisedError/Message;\n")
	b.WriteString("  RETURN $Message;\n")
	writeFlowEnd(b)
}

// errorMessageVar names the variable the nth handler of a test reads the
// message into. scope keeps the legacy runner's tests, which share one
// microflow, apart; the endpoint's tests each have their own.
func errorMessageVar(scope string, n int) string {
	return "$MxTestErrorMessage" + scope + "_" + strconv.Itoa(n)
}

// readErrorMessage is the handler line that puts the message of the error
// being handled in msgVar.
func readErrorMessage(msgVar string) string {
	return "  " + msgVar + " = CALL MICROFLOW " + errorMessageFlow + "(RaisedError = $latestError);"
}
