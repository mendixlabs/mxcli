// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// StmtHandler executes a single statement type.
// Implementations receive the concrete statement via type assertion.
type StmtHandler func(ctx *ExecContext, stmt ast.Statement) error

// Registry maps AST statement types to their handler functions.
type Registry struct {
	handlers map[reflect.Type]StmtHandler
}

// NewRegistry creates a Registry with all statement handlers registered.
func NewRegistry() *Registry {
	r := &Registry{
		handlers: make(map[reflect.Type]StmtHandler),
	}
	// Registration functions are called here explicitly (no init()).
	// Each function registers handlers for its domain.
	registerConnectionHandlers(r)
	registerModuleHandlers(r)
	registerEnumerationHandlers(r)
	registerConstantHandlers(r)
	registerDatabaseConnectionHandlers(r)
	registerEntityHandlers(r)
	registerAssociationHandlers(r)
	registerMicroflowAndNanoflowHandlers(r)
	registerPageHandlers(r)
	registerSecurityHandlers(r)
	registerNavigationHandlers(r)
	registerTranslationHandlers(r)
	registerImageHandlers(r)
	registerQueueHandlers(r)
	registerScheduledEventHandlers(r)
	registerRegularExpressionHandlers(r)
	registerValidationRuleHandlers(r)
	registerWorkflowHandlers(r)
	registerBusinessEventHandlers(r)
	registerSettingsHandlers(r)
	registerODataHandlers(r)
	registerJSONStructureHandlers(r)
	registerMessageDefinitionHandlers(r)
	registerMappingHandlers(r)
	registerRESTHandlers(r)
	registerDataTransformerHandlers(r)
	registerQueryHandlers(r)
	registerStylingHandlers(r)
	registerRepositoryHandlers(r)
	registerSessionHandlers(r)
	registerLintHandlers(r)
	registerAlterPageHandlers(r)
	registerFragmentHandlers(r)
	registerSQLHandlers(r)
	registerImportHandlers(r)
	registerAgentEditorHandlers(r)
	return r
}

// Register maps a statement type to its handler. It panics on duplicate
// registrations to catch wiring errors at startup.
func (r *Registry) Register(stmt ast.Statement, handler StmtHandler) {
	t := reflect.TypeOf(stmt)
	if _, exists := r.handlers[t]; exists {
		panic(fmt.Sprintf("registry: duplicate handler registration for %s", t))
	}
	r.handlers[t] = handler
}

// Lookup returns the handler for the given statement, or nil if none is
// registered.
func (r *Registry) Lookup(stmt ast.Statement) StmtHandler {
	return r.handlers[reflect.TypeOf(stmt)]
}

// Dispatch finds and executes the handler for stmt. Returns an
// UnsupportedError if no handler is registered.
func (r *Registry) Dispatch(ctx *ExecContext, stmt ast.Statement) error {
	h := r.Lookup(stmt)
	if h == nil {
		return mdlerrors.NewUnsupported(fmt.Sprintf("unhandled statement type %T", stmt))
	}
	return skipMissingIfAsked(ctx, stmt, h(ctx, stmt))
}

// skipMissingIfAsked turns "<kind> not found" into a notice for a DROP written
// with IF EXISTS. Only the document the statement names counts (or its module,
// which leaves the document missing just the same): any other not-found error
// is a real failure and is returned as is.
func skipMissingIfAsked(ctx *ExecContext, stmt ast.Statement, err error) error {
	skipper, ok := stmt.(ast.MissingSkipper)
	if err == nil || !ok || !skipper.SkipsMissing() {
		return err
	}
	var missing *mdlerrors.NotFoundError
	if !errors.As(err, &missing) {
		return err
	}
	kind, name := stmtDropInfo(stmt)
	if missing.Kind != "module" && missing.Name != name {
		return err
	}
	label := strings.ReplaceAll(kind, "-", " ")
	if kind == "javaaction" {
		label = "java action"
	}
	fmt.Fprintf(ctx.Output, "%s %s does not exist, skipping\n", label, name)
	return nil
}

// Validate checks that every known AST statement type has a registered
// handler. Returns an error listing all unregistered types, or nil if
// the registry is complete.
func (r *Registry) Validate(knownTypes []ast.Statement) error {
	var missing []string
	for _, s := range knownTypes {
		t := reflect.TypeOf(s)
		if _, ok := r.handlers[t]; !ok {
			missing = append(missing, t.String())
		}
	}
	if len(missing) > 0 {
		return mdlerrors.NewValidationf("registry: %d unregistered statement type(s): %v", len(missing), missing)
	}
	return nil
}

// HandlerCount returns the number of registered handlers.
func (r *Registry) HandlerCount() int {
	return len(r.handlers)
}
