// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// errLineRegexp parses error messages in the format "line N:M msg".
var errLineRegexp = regexp.MustCompile(`^line (\d+):(\d+) (.+)$`)

// parseMDLDiagnostics runs the MDL parser on text and converts errors to LSP diagnostics.
func parseMDLDiagnostics(text string) []protocol.Diagnostic {
	_, errs := visitor.Build(text)
	if len(errs) == 0 {
		return nil
	}

	diagnostics := make([]protocol.Diagnostic, 0, len(errs))
	for _, e := range errs {
		msg := e.Error()
		line := uint32(0)
		col := uint32(0)

		if matches := errLineRegexp.FindStringSubmatch(msg); matches != nil {
			if l, err := strconv.ParseUint(matches[1], 10, 32); err == nil {
				// ANTLR lines are 1-based, LSP is 0-based
				if l > 0 {
					line = uint32(l - 1)
				}
			}
			if c, err := strconv.ParseUint(matches[2], 10, 32); err == nil {
				col = uint32(c)
			}
			msg = matches[3]
		}

		diagnostics = append(diagnostics, protocol.Diagnostic{
			Range: protocol.Range{
				Start: protocol.Position{Line: line, Character: col},
				End:   protocol.Position{Line: line, Character: col},
			},
			Severity: protocol.DiagnosticSeverityError,
			Source:   "mdl",
			Message:  msg,
		})
	}
	return diagnostics
}

// publishDiagnostics parses the document and sends diagnostics to the client.
func (s *mdlServer) publishDiagnostics(ctx context.Context, docURI uri.URI, text string) {
	diags := parseMDLDiagnostics(text)
	// If no parse errors, run semantic validation inline
	if len(diags) == 0 {
		diags = append(diags, runSemanticValidation(text)...)
	}
	if diags == nil {
		diags = []protocol.Diagnostic{} // send empty array to clear diagnostics
	}
	s.client.PublishDiagnostics(ctx, &protocol.PublishDiagnosticsParams{
		URI:         protocol.DocumentURI(docURI),
		Diagnostics: diags,
	})
}

// DidOpen handles textDocument/didOpen notifications.
func (s *mdlServer) DidOpen(ctx context.Context, params *protocol.DidOpenTextDocumentParams) error {
	docURI := uri.URI(params.TextDocument.URI)
	text := params.TextDocument.Text

	s.mu.Lock()
	s.docs[docURI] = text
	s.mu.Unlock()

	s.publishDiagnostics(ctx, docURI, text)
	return nil
}

// DidChange handles textDocument/didChange notifications.
func (s *mdlServer) DidChange(ctx context.Context, params *protocol.DidChangeTextDocumentParams) error {
	docURI := uri.URI(params.TextDocument.URI)

	// With Full sync, the last content change has the full text
	if len(params.ContentChanges) == 0 {
		return nil
	}
	text := params.ContentChanges[len(params.ContentChanges)-1].Text

	s.mu.Lock()
	s.docs[docURI] = text
	s.mu.Unlock()

	s.publishDiagnostics(ctx, docURI, text)
	return nil
}

// DidClose handles textDocument/didClose notifications.
func (s *mdlServer) DidClose(ctx context.Context, params *protocol.DidCloseTextDocumentParams) error {
	docURI := uri.URI(params.TextDocument.URI)

	s.mu.Lock()
	delete(s.docs, docURI)
	s.mu.Unlock()

	// Clear diagnostics for closed document
	s.client.PublishDiagnostics(ctx, &protocol.PublishDiagnosticsParams{
		URI:         params.TextDocument.URI,
		Diagnostics: []protocol.Diagnostic{},
	})
	return nil
}

// DidSave handles textDocument/didSave notifications.
func (s *mdlServer) DidSave(ctx context.Context, params *protocol.DidSaveTextDocumentParams) error {
	docURI := uri.URI(params.TextDocument.URI)

	s.mu.Lock()
	text := s.docs[docURI]
	s.mu.Unlock()

	// If there are parse errors, don't run semantic checks
	if diags := parseMDLDiagnostics(text); len(diags) > 0 {
		return nil
	}

	// Run semantic check in background
	go s.runSemanticCheck(ctx, docURI, text)
	return nil
}

// runSemanticCheck runs mxcli check with --references and publishes diagnostics.
func (s *mdlServer) runSemanticCheck(ctx context.Context, docURI uri.URI, text string) {
	mprPath := s.findMprPath()
	if mprPath == "" {
		return
	}

	// Write to a temp file path based on the URI
	filePath := uriToPath(string(docURI))
	if filePath == "" {
		return
	}

	output, err := s.runMxcli(ctx, "check", filePath, "--references")
	if err == nil && output == "" {
		// No errors — clear semantic diagnostics
		s.client.PublishDiagnostics(ctx, &protocol.PublishDiagnosticsParams{
			URI:         protocol.DocumentURI(docURI),
			Diagnostics: []protocol.Diagnostic{},
		})
		return
	}

	if output == "" {
		return
	}

	diags := parseSemanticCheckOutput(output, text)
	if len(diags) == 0 {
		return
	}

	s.client.PublishDiagnostics(ctx, &protocol.PublishDiagnosticsParams{
		URI:         protocol.DocumentURI(docURI),
		Diagnostics: diags,
	})
}

// semanticCheckPattern parses output from mxcli check like "statement N: message".
var semanticCheckPattern = regexp.MustCompile(`^statement (\d+): (.+)$`)

// parseSemanticCheckOutput converts mxcli check output to LSP diagnostics.
func parseSemanticCheckOutput(output, docText string) []protocol.Diagnostic {
	// Build a map from statement index to start line number
	stmtLines := mapStatementLines(docText)

	var diags []protocol.Diagnostic
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		matches := semanticCheckPattern.FindStringSubmatch(line)
		if matches == nil {
			// Also try the "line N:M msg" format (same as parse errors)
			if errMatches := errLineRegexp.FindStringSubmatch(line); errMatches != nil {
				l, _ := strconv.ParseUint(errMatches[1], 10, 32)
				c, _ := strconv.ParseUint(errMatches[2], 10, 32)
				lineNum := uint32(0)
				if l > 0 {
					lineNum = uint32(l - 1)
				}
				diags = append(diags, protocol.Diagnostic{
					Range: protocol.Range{
						Start: protocol.Position{Line: lineNum, Character: uint32(c)},
						End:   protocol.Position{Line: lineNum, Character: uint32(c)},
					},
					Severity: protocol.DiagnosticSeverityWarning,
					Source:   "mdl-check",
					Message:  errMatches[3],
				})
			}
			continue
		}

		stmtIdx, _ := strconv.Atoi(matches[1])
		msg := matches[2]

		lineNum := uint32(0)
		if stmtIdx > 0 && stmtIdx <= len(stmtLines) {
			lineNum = stmtLines[stmtIdx-1] // 1-indexed to 0-indexed
		}

		diags = append(diags, protocol.Diagnostic{
			Range: protocol.Range{
				Start: protocol.Position{Line: lineNum, Character: 0},
				End:   protocol.Position{Line: lineNum, Character: 0},
			},
			Severity: protocol.DiagnosticSeverityWarning,
			Source:   "mdl-check",
			Message:  msg,
		})
	}
	return diags
}

// mapStatementLines scans document text and identifies the start line of each
// top-level statement (CREATE, ALTER, DROP, SHOW, etc.).
func mapStatementLines(text string) []uint32 {
	lines := strings.Split(text, "\n")
	var stmtLines []uint32

	topLevelPattern := regexp.MustCompile(`(?i)^\s*(CREATE|ALTER|DROP|SHOW|DESCRIBE|RENAME|MOVE|SELECT|REFRESH|UPDATE)\b`)

	for i, line := range lines {
		if topLevelPattern.MatchString(line) {
			stmtLines = append(stmtLines, uint32(i))
		}
	}
	return stmtLines
}

// runSemanticValidation runs the same validators as cmd_check.go directly on parsed text,
// returning LSP diagnostics with structured rule IDs.
func runSemanticValidation(text string) []protocol.Diagnostic {
	prog, errs := visitor.Build(text)
	if len(errs) > 0 || prog == nil {
		return nil
	}

	stmtLines := mapStatementLines(text)

	var diags []protocol.Diagnostic
	for i, stmt := range prog.Statements {
		var violations []linter.Violation
		if enumStmt, ok := stmt.(*ast.CreateEnumerationStmt); ok {
			violations = append(violations, executor.ValidateEnumeration(enumStmt)...)
		}
		if entityStmt, ok := stmt.(*ast.CreateEntityStmt); ok {
			violations = append(violations, executor.ValidateEntity(entityStmt)...)
		}
		if mfStmt, ok := stmt.(*ast.CreateMicroflowStmt); ok {
			violations = append(violations, executor.ValidateMicroflow(mfStmt)...)
		}
		if viewStmt, ok := stmt.(*ast.CreateViewEntityStmt); ok {
			if viewStmt.Query.RawQuery != "" {
				violations = append(violations, executor.ValidateOQLSyntax(viewStmt.Query.RawQuery)...)
				violations = append(violations, executor.ValidateOQLTypes(viewStmt.Query.RawQuery, viewStmt.Attributes)...)
			}
		}

		lineNum := uint32(0)
		if i < len(stmtLines) {
			lineNum = stmtLines[i]
		}

		for _, v := range violations {
			msg := v.Message
			if v.Suggestion != "" {
				msg += " → " + v.Suggestion
			}
			diags = append(diags, protocol.Diagnostic{
				Range: protocol.Range{
					Start: protocol.Position{Line: lineNum, Character: 0},
					End:   protocol.Position{Line: lineNum, Character: 0},
				},
				Severity: violationToLSPSeverity(v.Severity),
				Source:   "mdl-check",
				Code:     v.RuleID,
				Message:  msg,
			})
		}
	}
	return diags
}

// violationToLSPSeverity maps linter.Severity to protocol.DiagnosticSeverity.
func violationToLSPSeverity(s linter.Severity) protocol.DiagnosticSeverity {
	switch s {
	case linter.SeverityError:
		return protocol.DiagnosticSeverityError
	case linter.SeverityWarning:
		return protocol.DiagnosticSeverityWarning
	case linter.SeverityInfo:
		return protocol.DiagnosticSeverityInformation
	case linter.SeverityHint:
		return protocol.DiagnosticSeverityHint
	default:
		return protocol.DiagnosticSeverityWarning
	}
}
