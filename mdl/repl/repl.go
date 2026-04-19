// SPDX-License-Identifier: Apache-2.0

// Package repl provides an interactive REPL for MDL commands.
package repl

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chzyer/readline"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mprbackend "github.com/mendixlabs/mxcli/mdl/backend/mpr"
	"github.com/mendixlabs/mxcli/mdl/diaglog"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// REPL is an interactive read-eval-print loop for MDL commands.
type REPL struct {
	executor *executor.Executor
	input    io.Reader
	output   io.Writer
	prompt   string
	rl       *readline.Instance
	logger   *diaglog.Logger
}

// SetLogger sets the diagnostics logger for the REPL and its executor.
func (r *REPL) SetLogger(l *diaglog.Logger) {
	r.logger = l
	r.executor.SetLogger(l)
}

// New creates a new REPL with the given input and output.
func New(input io.Reader, output io.Writer) *REPL {
	exec := executor.New(output)
	exec.SetBackendFactory(func() backend.FullBackend { return mprbackend.New() })
	return &REPL{
		executor: exec,
		input:    input,
		output:   output,
		prompt:   "mdl> ",
	}
}

// Run starts the REPL loop without readline support.
// When stdin is piped, this suppresses the banner and prompts for clean scripted usage.
func (r *REPL) Run() error {
	scanner := bufio.NewScanner(r.input)
	var buffer strings.Builder

	for {
		// Read line
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()

		// Handle empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Accumulate multi-line input
		buffer.WriteString(line)
		buffer.WriteString("\n")

		input := buffer.String()

		// Check if statement is complete (ends with ; or is a simple command)
		if isCompleteStatement(input) {
			err := r.execute(input)
			if err != nil {
				if errors.Is(err, executor.ErrExit) {
					return nil
				}
				fmt.Fprintf(r.output, "Error: %v\n", err)
			}
			buffer.Reset()
		}
	}

	// Execute any remaining buffered input (without trailing semicolon)
	if buffer.Len() > 0 {
		input := buffer.String()
		if strings.TrimSpace(input) != "" {
			err := r.execute(input)
			if err != nil && !errors.Is(err, executor.ErrExit) {
				fmt.Fprintf(r.output, "Error: %v\n", err)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read error: %w", err)
	}

	return nil
}

// RunWithReadline starts the REPL loop with readline support (history, arrow keys).
func (r *REPL) RunWithReadline() error {
	// Get history file path
	historyFile := getHistoryFilePath()

	// Configure readline with autocomplete
	config := &readline.Config{
		Prompt:            r.prompt,
		HistoryFile:       historyFile,
		HistoryLimit:      1000,
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
		HistorySearchFold: true,
		AutoComplete:      newMDLCompleter(r.executor),
	}

	rl, err := readline.NewEx(config)
	if err != nil {
		// Fall back to non-readline mode
		return r.Run()
	}
	r.rl = rl
	defer rl.Close()

	var buffer strings.Builder

	fmt.Fprintln(r.output, "MDL REPL - Mendix Definition Language")
	fmt.Fprintln(r.output, "Type 'help' or '?' for commands, 'exit' or 'quit' to quit")
	fmt.Fprintln(r.output, "Tab: autocomplete, ↑↓: history, Ctrl+R: search history")
	fmt.Fprintln(r.output)

	for {
		// Set prompt based on whether we're continuing a multi-line statement
		if buffer.Len() == 0 {
			rl.SetPrompt(r.prompt)
		} else {
			rl.SetPrompt("...> ")
		}

		// Read line with readline (supports history, arrow keys, etc.)
		line, err := rl.Readline()
		if err != nil {
			if err == readline.ErrInterrupt {
				if buffer.Len() > 0 {
					// Cancel current multi-line input
					buffer.Reset()
					fmt.Fprintln(r.output, "^C")
					continue
				}
				// Exit on double Ctrl-C
				fmt.Fprintln(r.output, "Use 'exit' or 'quit' to exit")
				continue
			}
			if err == io.EOF {
				fmt.Fprintln(r.output, "\nGoodbye!")
				return nil
			}
			return fmt.Errorf("readline error: %w", err)
		}

		// Handle empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Accumulate multi-line input
		buffer.WriteString(line)
		buffer.WriteString("\n")

		input := buffer.String()

		// Check if statement is complete
		if isCompleteStatement(input) {
			// Add complete statement to history
			trimmed := strings.TrimSpace(input)
			if trimmed != "" && !isHistoryExcluded(trimmed) {
				rl.SaveHistory(trimmed)
			}

			err := r.execute(input)
			if err != nil {
				if errors.Is(err, executor.ErrExit) {
					fmt.Fprintln(r.output, "Goodbye!")
					return nil
				}
				fmt.Fprintf(r.output, "Error: %v\n", err)
			}
			buffer.Reset()
		}
	}
}

// ExecuteString parses and executes an MDL string.
func (r *REPL) ExecuteString(input string) error {
	return r.execute(input)
}

// Close closes the REPL and releases resources.
func (r *REPL) Close() error {
	if r.rl != nil {
		r.rl.Close()
	}
	return r.executor.Close()
}

func (r *REPL) execute(input string) error {
	// Strip trailing slash terminator if present (SQL*Plus style)
	// The "/" allows multi-statement blocks when statements contain ";" internally
	input = stripSlashTerminator(input)

	// Parse the input
	prog, errs := visitor.Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			fmt.Fprintf(r.output, "Parse error: %v\n", err)
		}
		r.logger.ParseError(input, errs)
		return nil // Don't return error, just print and continue
	}

	// Execute all statements and run finalization (e.g. ReconcileMemberAccesses)
	if err := r.executor.ExecuteProgram(prog); err != nil {
		return err
	}

	return nil
}

// stripSlashTerminator removes the trailing "/" line from input if present.
// This allows "/" to be used as a statement terminator (SQL*Plus style)
// without requiring the grammar to support it.
func stripSlashTerminator(input string) string {
	lines := strings.Split(input, "\n")
	// Remove trailing empty lines
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	// If the last line is just "/", remove it
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "/" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func isCompleteStatement(input string) bool {
	input = strings.TrimSpace(input)

	// Empty input is not complete
	if input == "" {
		return false
	}

	// Simple commands that don't need semicolons
	lower := strings.ToLower(input)
	simpleCommands := []string{
		"help", "?", "exit", "quit", "status",
		"disconnect", "refresh", "update",
	}
	if slices.Contains(simpleCommands, lower) {
		return true
	}

	// SHOW and DESCRIBE commands
	if strings.HasPrefix(lower, "show ") || strings.HasPrefix(lower, "describe ") {
		return true
	}

	// Check for unbalanced $$ dollar-quoted blocks (Java action code)
	// If we're inside a $$ block, the statement is not complete until the closing $$
	if hasUnbalancedDollarQuote(input) {
		return false
	}

	// Check for unbalanced BEGIN/END blocks (microflow body)
	// If we're inside a BEGIN block, the statement is not complete until END
	if hasUnbalancedBeginEnd(input) {
		return false
	}

	// Commands ending with semicolon (only if not inside BEGIN/END block)
	if strings.HasSuffix(input, ";") {
		return true
	}

	// Slash terminator - only when "/" is alone on the last line
	// This avoids treating "*/" (end of doc comment) as a terminator
	lines := strings.Split(input, "\n")
	lastLine := strings.TrimSpace(lines[len(lines)-1])
	if lastLine == "/" {
		return true
	}

	// CONNECT commands
	if strings.HasPrefix(lower, "connect ") {
		return true
	}

	// SET commands
	if strings.HasPrefix(lower, "set ") {
		return true
	}

	return false
}

// hasUnbalancedBeginEnd checks if there are more BEGIN keywords than END keywords
// This is used to detect incomplete microflow/nanoflow bodies
func hasUnbalancedBeginEnd(input string) bool {
	lower := strings.ToLower(input)

	// Count BEGIN and END as whole words (not part of other words)
	// Important: "END IF" and "END LOOP" don't close BEGIN blocks, only standalone "END;" does
	beginCount := 0
	endCount := 0

	words := strings.Fields(lower)
	for i, word := range words {
		// Strip trailing punctuation like ; or ,
		word = strings.TrimRight(word, ";,")
		if word == "begin" {
			beginCount++
		} else if word == "end" {
			// Check if this is "END IF" or "END LOOP" - these don't close BEGIN
			nextWord := ""
			if i+1 < len(words) {
				nextWord = strings.TrimRight(strings.ToLower(words[i+1]), ";,")
			}
			// Only count END if it's not followed by IF or LOOP (control structure closers)
			if nextWord != "if" && nextWord != "loop" {
				endCount++
			}
		}
	}

	return beginCount > endCount
}

// hasUnbalancedDollarQuote checks if there's an unclosed $$ block.
// An odd number of $$ occurrences means we're still inside a dollar-quoted string.
func hasUnbalancedDollarQuote(input string) bool {
	return strings.Count(input, "$$")%2 != 0
}

// isHistoryExcluded returns true if the command should not be saved to history
func isHistoryExcluded(cmd string) bool {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	excluded := []string{"exit", "quit", "help", "?"}
	return slices.Contains(excluded, lower)
}

// getHistoryFilePath returns the path to the history file
func getHistoryFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".mxcli_history")
}

// ----------------------------------------------------------------------------
// Autocomplete and History
// ----------------------------------------------------------------------------

// mdlCompleter implements case-insensitive prefix completion for MDL commands.
type mdlCompleter struct {
	prefixCompleter *readline.PrefixCompleter
	executor        *executor.Executor
}

// newMDLCompleter creates a case-insensitive readline completer for MDL commands.
func newMDLCompleter(exec *executor.Executor) *mdlCompleter {
	return &mdlCompleter{
		prefixCompleter: newPrefixCompleter(),
		executor:        exec,
	}
}

// Do performs tab completion with case-insensitive matching.
func (c *mdlCompleter) Do(line []rune, pos int) (newLine [][]rune, length int) {
	lineStr := string(line[:pos])
	lineUpper := strings.ToUpper(lineStr)

	// Try dynamic completions first (for object names)
	if c.executor != nil && c.executor.IsConnected() {
		if completions, length := c.dynamicComplete(lineStr, lineUpper); len(completions) > 0 {
			return completions, length
		}
	}

	// Fall back to static keyword completion
	lineRunes := []rune(lineUpper)
	completions, length := c.prefixCompleter.Do(lineRunes, len(lineRunes))
	return completions, length
}

// dynamicComplete provides context-aware completion for object names.
func (c *mdlCompleter) dynamicComplete(line, lineUpper string) ([][]rune, int) {
	// Patterns that need module name completion
	modulePatterns := []string{
		"SHOW ENTITIES IN ",
		"SHOW MICROFLOWS IN ",
		"SHOW NANOFLOWS IN ",
		"SHOW PAGES IN ",
		"SHOW SNIPPETS IN ",
		"SHOW LAYOUTS IN ",
		"SHOW ENUMERATIONS IN ",
		"SHOW ASSOCIATIONS IN ",
		"SHOW JAVA ACTIONS IN ",
		"SHOW ODATA CLIENTS IN ",
		"SHOW ODATA SERVICES IN ",
		"SHOW EXTERNAL ENTITIES IN ",
		"SHOW MODULE ROLES IN ",
		"SHOW SECURITY MATRIX IN ",
		"SHOW STRUCTURE IN ",
		"SHOW WIDGETS IN ",
		"SHOW DATABASE CONNECTIONS IN ",
		"SHOW REST CLIENTS IN ",
		"SHOW BUSINESS EVENT SERVICES IN ",
		"DROP MODULE ",
		"DESCRIBE MODULE ",
	}

	for _, pattern := range modulePatterns {
		if strings.HasPrefix(lineUpper, pattern) {
			prefix := line[len(pattern):]
			return c.completeModuleNames(prefix)
		}
	}

	// Patterns that need qualified name completion (Module.Object)
	qualifiedPatterns := map[string]func(string) []string{
		"DESCRIBE MICROFLOW ":              c.executor.GetMicroflowNames,
		"DESCRIBE ENTITY ":                 c.executor.GetEntityNames,
		"DESCRIBE ENUMERATION ":            c.executor.GetEnumerationNames,
		"DESCRIBE ASSOCIATION ":            c.executor.GetAssociationNames,
		"DESCRIBE PAGE ":                   c.executor.GetPageNames,
		"DESCRIBE SNIPPET ":                c.executor.GetSnippetNames,
		"DESCRIBE LAYOUT ":                 c.executor.GetLayoutNames,
		"DESCRIBE JAVA ACTION ":            c.executor.GetJavaActionNames,
		"DESCRIBE ODATA CLIENT ":           c.executor.GetODataClientNames,
		"DESCRIBE ODATA SERVICE ":          c.executor.GetODataServiceNames,
		"DESCRIBE EXTERNAL ENTITY ":        c.executor.GetEntityNames,
		"DESCRIBE DATABASE CONNECTION ":    c.executor.GetDatabaseConnectionNames,
		"DESCRIBE REST CLIENT ":            c.executor.GetRestClientNames,
		"DESCRIBE BUSINESS EVENT SERVICE ": c.executor.GetBusinessEventServiceNames,
		"DESCRIBE JSON STRUCTURE ":         c.executor.GetJsonStructureNames,
		"DROP ENTITY ":                     c.executor.GetEntityNames,
		"DROP DATABASE CONNECTION ":        c.executor.GetDatabaseConnectionNames,
		"DROP BUSINESS EVENT SERVICE ":     c.executor.GetBusinessEventServiceNames,
		"DROP MICROFLOW ":                  c.executor.GetMicroflowNames,
		"DROP ENUMERATION ":                c.executor.GetEnumerationNames,
		"DROP ASSOCIATION ":                c.executor.GetAssociationNames,
		"DROP PAGE ":                       c.executor.GetPageNames,
		"DROP SNIPPET ":                    c.executor.GetSnippetNames,
		"DROP REST CLIENT ":                c.executor.GetRestClientNames,
		"DROP JSON STRUCTURE ":             c.executor.GetJsonStructureNames,
	}

	for pattern, getter := range qualifiedPatterns {
		if strings.HasPrefix(lineUpper, pattern) {
			prefix := line[len(pattern):]
			return c.completeQualifiedNames(prefix, getter)
		}
	}

	return nil, 0
}

// completeModuleNames returns completions for module names.
// Returns the suffix to append (readline prepends the typed prefix for display).
func (c *mdlCompleter) completeModuleNames(prefix string) ([][]rune, int) {
	modules := c.executor.GetModuleNames()
	if modules == nil {
		return nil, 0
	}

	// Trim leading/trailing spaces from prefix
	prefix = strings.TrimSpace(prefix)
	prefixUpper := strings.ToUpper(prefix)
	prefixLen := len(prefix)

	var completions [][]rune
	for _, mod := range modules {
		if strings.HasPrefix(strings.ToUpper(mod), prefixUpper) {
			// Return only the suffix after the typed prefix
			suffix := mod[prefixLen:]
			completions = append(completions, []rune(suffix))
		}
	}

	return completions, 0
}

// completeQualifiedNames returns completions for qualified names (Module.Object).
// Returns the suffix to append (readline prepends the typed prefix for display).
func (c *mdlCompleter) completeQualifiedNames(prefix string, getter func(string) []string) ([][]rune, int) {
	// Trim leading/trailing spaces from prefix
	prefix = strings.TrimSpace(prefix)
	prefixUpper := strings.ToUpper(prefix)
	prefixLen := len(prefix)

	// If prefix contains a dot, filter by module (case-insensitive)
	var moduleFilter string
	if before, _, ok := strings.Cut(prefix, "."); ok {
		// Find the actual module name with correct casing
		typedModule := before
		for _, mod := range c.executor.GetModuleNames() {
			if strings.EqualFold(mod, typedModule) {
				moduleFilter = mod
				break
			}
		}
	}

	names := getter(moduleFilter)
	if names == nil {
		return nil, 0
	}

	var completions [][]rune
	for _, name := range names {
		if strings.HasPrefix(strings.ToUpper(name), prefixUpper) {
			// Return only the suffix after the typed prefix
			suffix := name[prefixLen:]
			completions = append(completions, []rune(suffix))
		}
	}

	return completions, 0
}

// GetName returns the completer name.
func (c *mdlCompleter) GetName() []rune {
	return c.prefixCompleter.GetName()
}

// GetChildren returns child completers.
func (c *mdlCompleter) GetChildren() []readline.PrefixCompleterInterface {
	return c.prefixCompleter.GetChildren()
}

// SetChildren sets child completers.
func (c *mdlCompleter) SetChildren(children []readline.PrefixCompleterInterface) {
	c.prefixCompleter.SetChildren(children)
}

// GetDynamicNames returns dynamic names for completion.
func (c *mdlCompleter) GetDynamicNames(line []rune) [][]rune {
	return nil
}

// newPrefixCompleter creates the static prefix completer for MDL commands.
func newPrefixCompleter() *readline.PrefixCompleter {
	return readline.NewPrefixCompleter(
		// Connection commands
		readline.PcItem("CONNECT",
			readline.PcItem("LOCAL"),
		),
		readline.PcItem("DISCONNECT"),
		readline.PcItem("STATUS"),

		// Show commands
		readline.PcItem("SHOW",
			readline.PcItem("MODULES"),
			readline.PcItem("ENTITIES",
				readline.PcItem("IN"),
			),
			readline.PcItem("ENTITY"),
			readline.PcItem("ENUMERATIONS",
				readline.PcItem("IN"),
			),
			readline.PcItem("ASSOCIATIONS",
				readline.PcItem("IN"),
			),
			readline.PcItem("ASSOCIATION"),
			readline.PcItem("MICROFLOWS",
				readline.PcItem("IN"),
			),
			readline.PcItem("MICROFLOW"),
			readline.PcItem("NANOFLOWS",
				readline.PcItem("IN"),
			),
			readline.PcItem("PAGES",
				readline.PcItem("IN"),
			),
			readline.PcItem("PAGE"),
			readline.PcItem("SNIPPETS",
				readline.PcItem("IN"),
			),
			readline.PcItem("LAYOUTS",
				readline.PcItem("IN"),
			),
			readline.PcItem("JAVA",
				readline.PcItem("ACTIONS",
					readline.PcItem("IN"),
				),
			),
			readline.PcItem("CATALOG",
				readline.PcItem("TABLES"),
				readline.PcItem("STATUS"),
			),
			readline.PcItem("CALLERS"),
			readline.PcItem("CALLEES"),
			readline.PcItem("REFERENCES"),
			readline.PcItem("IMPACT"),
			readline.PcItem("ODATA",
				readline.PcItem("CLIENTS",
					readline.PcItem("IN"),
				),
				readline.PcItem("SERVICES",
					readline.PcItem("IN"),
				),
			),
			readline.PcItem("EXTERNAL",
				readline.PcItem("ENTITIES",
					readline.PcItem("IN"),
				),
			),
			readline.PcItem("WIDGETS"),
			readline.PcItem("CONTEXT"),
			readline.PcItem("PROJECT",
				readline.PcItem("SECURITY"),
			),
			readline.PcItem("MODULE",
				readline.PcItem("ROLES",
					readline.PcItem("IN"),
				),
			),
			readline.PcItem("USER",
				readline.PcItem("ROLES"),
			),
			readline.PcItem("DEMO",
				readline.PcItem("USERS"),
			),
			readline.PcItem("ACCESS",
				readline.PcItem("ON"),
			),
			readline.PcItem("SECURITY",
				readline.PcItem("MATRIX",
					readline.PcItem("IN"),
				),
			),
			readline.PcItem("STRUCTURE",
				readline.PcItem("DEPTH"),
				readline.PcItem("IN"),
				readline.PcItem("ALL"),
			),
			readline.PcItem("DATABASE",
				readline.PcItem("CONNECTIONS",
					readline.PcItem("IN"),
				),
			),
			readline.PcItem("BUSINESS",
				readline.PcItem("EVENT",
					readline.PcItem("SERVICES",
						readline.PcItem("IN"),
					),
				),
			),
		),

		// Describe commands
		readline.PcItem("DESCRIBE",
			readline.PcItem("ENTITY"),
			readline.PcItem("ENUMERATION"),
			readline.PcItem("ASSOCIATION"),
			readline.PcItem("MICROFLOW"),
			readline.PcItem("PAGE"),
			readline.PcItem("SNIPPET"),
			readline.PcItem("LAYOUT"),
			readline.PcItem("JAVA",
				readline.PcItem("ACTION"),
			),
			readline.PcItem("MODULE"),
			readline.PcItem("ODATA",
				readline.PcItem("CLIENT"),
				readline.PcItem("SERVICE"),
			),
			readline.PcItem("EXTERNAL",
				readline.PcItem("ENTITY"),
			),
			readline.PcItem("CONSTANT"),
			readline.PcItem("DATABASE",
				readline.PcItem("CONNECTION"),
			),
			readline.PcItem("BUSINESS",
				readline.PcItem("EVENT",
					readline.PcItem("SERVICE"),
				),
			),
		),

		// Create commands
		readline.PcItem("CREATE",
			readline.PcItem("MODULE"),
			readline.PcItem("PERSISTENT",
				readline.PcItem("ENTITY"),
			),
			readline.PcItem("NON-PERSISTENT",
				readline.PcItem("ENTITY"),
			),
			readline.PcItem("VIEW",
				readline.PcItem("ENTITY"),
			),
			readline.PcItem("ENUMERATION"),
			readline.PcItem("ASSOCIATION"),
			readline.PcItem("MICROFLOW"),
			readline.PcItem("NANOFLOW"),
			readline.PcItem("PAGE"),
			readline.PcItem("SNIPPET"),
			readline.PcItem("OR",
				readline.PcItem("MODIFY",
					readline.PcItem("PERSISTENT",
						readline.PcItem("ENTITY"),
					),
					readline.PcItem("NON-PERSISTENT",
						readline.PcItem("ENTITY"),
					),
					readline.PcItem("MICROFLOW"),
					readline.PcItem("NANOFLOW"),
					readline.PcItem("PAGE"),
					readline.PcItem("SNIPPET"),
				),
			),
		),

		// Drop commands
		readline.PcItem("DROP",
			readline.PcItem("MODULE"),
			readline.PcItem("ENTITY"),
			readline.PcItem("ENUMERATION"),
			readline.PcItem("ASSOCIATION"),
			readline.PcItem("MICROFLOW"),
			readline.PcItem("NANOFLOW"),
			readline.PcItem("PAGE"),
			readline.PcItem("SNIPPET"),
			readline.PcItem("DATABASE",
				readline.PcItem("CONNECTION"),
			),
			readline.PcItem("BUSINESS",
				readline.PcItem("EVENT",
					readline.PcItem("SERVICE"),
				),
			),
		),

		// Alter commands
		readline.PcItem("ALTER",
			readline.PcItem("ENUMERATION"),
		),

		// Other commands
		readline.PcItem("REFRESH"),
		readline.PcItem("REFRESH",
			readline.PcItem("CATALOG"),
			readline.PcItem("CATALOG",
				readline.PcItem("FULL"),
			),
		),
		readline.PcItem("UPDATE"),
		readline.PcItem("SET"),
		readline.PcItem("SELECT"),
		readline.PcItem("EXECUTE",
			readline.PcItem("SCRIPT"),
		),
		readline.PcItem("CHECK"),
		readline.PcItem("LINT"),
		readline.PcItem("HELP"),
		readline.PcItem("EXIT"),
		readline.PcItem("QUIT"),
	)
}

// RunInteractive starts an interactive REPL session using stdin/stdout with readline support.
func RunInteractive() error {
	repl := New(os.Stdin, os.Stdout)
	defer repl.Close()
	return repl.RunWithReadline()
}
